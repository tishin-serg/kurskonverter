package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fail       bool
	}{{"ok", `{"ok":true}`, 200, false}, {"json", `{`, 200, true}, {"status", `{}`, 401, true}, {"trailing", `{} {}`, 200, true}} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			var out struct{ OK bool }
			err := New().Get(context.Background(), s.URL, &out)
			if (err != nil) != tc.fail {
				t.Fatal(err)
			}
		})
	}
}
func TestRetryAndCancel(t *testing.T) {
	n := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer s.Close()
	var v any
	if err := New().Get(context.Background(), s.URL, &v); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New().Get(ctx, s.URL, &v); err == nil {
		t.Fatal("ignored cancellation")
	}
}
