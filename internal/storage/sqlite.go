package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tishin-serg/kurskonverter/internal/route"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *sql.DB }

func (s *Store) ReferenceRate(ctx context.Context, user int64) (string, error) {
	var rate string
	err := s.DB.QueryRowContext(ctx, "SELECT btc_rub_rate FROM user_rates WHERE user_id=?", user).Scan(&rate)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return rate, err
}

func (s *Store) SetReferenceRate(ctx context.Context, user int64, rate string) error {
	_, err := s.DB.ExecContext(ctx, "INSERT INTO user_rates(user_id,btc_rub_rate) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET btc_rub_rate=excluded.btc_rub_rate", user, rate)
	return err
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return nil, e
		}
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if e = s.init(ctx); e != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize SQLite: %w", e)
	}
	return s, nil
}
func (s *Store) init(ctx context.Context) error {
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, e := s.DB.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if _, e = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)"); e != nil {
		return e
	}
	files, e := migrations.ReadDir("migrations")
	if e != nil {
		return e
	}
	for _, f := range files {
		var count int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=?", f.Name()).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			continue
		}
		b, e := migrations.ReadFile("migrations/" + f.Name())
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, string(b)); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES(?)", f.Name()); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) EnsureUser(ctx context.Context, id int64) error {
	_, e := s.DB.ExecContext(ctx, "INSERT OR IGNORE INTO users(id) VALUES(?)", id)
	return e
}
func (s *Store) Payment(ctx context.Context, id int64) (string, error) {
	var p string
	e := s.DB.QueryRowContext(ctx, "SELECT payment_method FROM user_settings WHERE user_id=?", id).Scan(&p)
	if e == sql.ErrNoRows {
		return "", nil
	}
	return p, e
}
func (s *Store) SetPayment(ctx context.Context, id int64, p string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	if _, e = tx.ExecContext(ctx, "INSERT INTO user_settings(user_id,payment_method) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET payment_method=excluded.payment_method", id, p); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM provider_payments WHERE user_id=?", id); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Save(ctx context.Context, id int64, target string, r route.Result) (int64, error) {
	b, e := json.Marshal(r)
	if e != nil {
		return 0, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer func() { _ = tx.Rollback() }()
	res, e := tx.ExecContext(ctx, "INSERT INTO quote_history(user_id,target_btc,result_json) VALUES(?,?,?)", id, target, string(b))
	if e != nil {
		return 0, e
	}
	n, e := res.LastInsertId()
	if e != nil {
		return 0, e
	}
	_, e = tx.ExecContext(ctx, "DELETE FROM quote_history WHERE user_id=? AND id NOT IN (SELECT id FROM quote_history WHERE user_id=? ORDER BY id DESC LIMIT 100)", id, id)
	if e != nil {
		return 0, e
	}
	return n, tx.Commit()
}
func (s *Store) Load(ctx context.Context, user, id int64) (string, route.Result, error) {
	var target, raw string
	var r route.Result
	e := s.DB.QueryRowContext(ctx, "SELECT target_btc,result_json FROM quote_history WHERE user_id=? AND id=?", user, id).Scan(&target, &raw)
	if e != nil {
		return target, r, e
	}
	e = json.Unmarshal([]byte(raw), &r)
	return target, r, e
}

func (s *Store) ProviderPayments(ctx context.Context, id int64) (map[string]string, error) {
	rows, e := s.DB.QueryContext(ctx, "SELECT provider,payment_method FROM provider_payments WHERE user_id=?", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if e = rows.Scan(&k, &v); e != nil {
			return nil, e
		}
		out[k] = v
	}
	return out, rows.Err()
}
func (s *Store) SetProviderPayment(ctx context.Context, id int64, provider, payment string) error {
	_, e := s.DB.ExecContext(ctx, "INSERT INTO provider_payments(user_id,provider,payment_method) VALUES(?,?,?) ON CONFLICT(user_id,provider) DO UPDATE SET payment_method=excluded.payment_method", id, provider, payment)
	return e
}
