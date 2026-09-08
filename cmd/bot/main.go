package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tishin-serg/kurskonverter/internal/app"
	"github.com/tishin-serg/kurskonverter/internal/config"
	"github.com/tishin-serg/kurskonverter/internal/market"
	"github.com/tishin-serg/kurskonverter/internal/provider/demo"
	"github.com/tishin-serg/kurskonverter/internal/route"
	"github.com/tishin-serg/kurskonverter/internal/storage"
	"github.com/tishin-serg/kurskonverter/internal/telegram"
	"golang.org/x/sync/errgroup"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check-usd":
			c, err := config.Load()
			if len(os.Args) > 2 && os.Args[2] == "any" {
				c.Filter.PaymentMethod = ""
			}
			if err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				err = app.CheckUSD(ctx, c, os.Stdout)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "check-routes":
			c, err := config.Load()
			if len(os.Args) > 2 && os.Args[2] == "any" {
				c.Filter.PaymentMethod = ""
				fmt.Println("Диагностика без фильтра банка; сохранённые настройки не изменяются.")
			}
			if err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
				defer cancel()
				err = app.CheckRoutes(ctx, c, os.Stdout)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "check-bybit":
			c, err := config.Load()
			if err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				err = app.CheckBybit(ctx, c, os.Stdout)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Printf("version=%s commit=%s date=%s\n", Version, Commit, Date)
			return
		case "healthcheck":
			if err := probe(); err != nil {
				os.Exit(1)
			}
			return
		case "demo-quote":
			n, _ := telegram.ParseAmount("0.01 BTC")
			c, _ := config.Load()
			r := route.DefaultEngine().Calculate(context.Background(), n, demo.Snapshot(time.Now()), c.Filter, time.Now())
			fmt.Println(telegram.Summary(n, r, true))
			return
		}
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("application stopped", "error", err.Error())
		os.Exit(1)
	}
}
func probe() error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/health"
	if len(os.Args) > 2 && os.Args[2] == "ready" {
		url = "http://" + net.JoinHostPort("127.0.0.1", port) + "/ready"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	r, e := c.Get(url)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("unhealthy")
	}
	return nil
}
func run(log *slog.Logger) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	if c.Token == "" && c.Mode != "demo" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN required in live mode")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := storage.Open(ctx, c.DBPath)
	if err != nil {
		return err
	}
	defer db.DB.Close()
	snap := market.New()
	stats := market.NewStatistics()
	h := &app.Health{Storage: db, Market: snap, Stats: stats, Version: Version, Commit: Commit, Date: Date, Mode: c.Mode}
	ui := &telegram.UI{Market: snap, Storage: db, Engine: route.DefaultEngine(), Filter: c.Filter, Demo: c.Mode == "demo", Log: log}
	group, gctx := errgroup.WithContext(ctx)
	if c.Token != "" {
		b, e := ui.New(c.Token)
		if e != nil {
			return fmt.Errorf("telegram initialization failed; verify token and connectivity")
		}
		h.TelegramReady.Store(true)
		group.Go(func() error { b.Start(gctx); h.TelegramReady.Store(false); return nil })
	} else {
		h.TelegramReady.Store(true)
		log.Warn("demo mode: Telegram disabled")
	}
	server := &http.Server{Addr: c.HTTPAddr, Handler: h.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	group.Go(func() error {
		h.Started.Store(true)
		defer h.Started.Store(false)
		return market.Run(gctx, app.Workers(c, snap), log, stats)
	})
	group.Go(func() error {
		e := server.ListenAndServe()
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	})
	group.Go(func() error {
		<-gctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	})
	log.Info("bot starting", "version", Version, "mode", c.Mode)
	return group.Wait()
}
