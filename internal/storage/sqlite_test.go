package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tishin-serg/kurskonverter/internal/route"
)

func TestStorage(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "db.sqlite")
	s, e := Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.EnsureUser(ctx, 1); e != nil {
		t.Fatal(e)
	}
	if e = s.SetPayment(ctx, 1, "bank"); e != nil {
		t.Fatal(e)
	}
	id, e := s.Save(ctx, 1, ".01", route.Result{})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Load(ctx, 2, id); e == nil {
		t.Fatal("history ownership bypass")
	}
	_ = s.DB.Close()
	s, e = Open(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	v, e := s.Payment(ctx, 1)
	if e != nil || v != "bank" {
		t.Fatal(v, e)
	}
	target, _, e := s.Load(ctx, 1, id)
	if e != nil || target != ".01" {
		t.Fatal(target, e)
	}
}
