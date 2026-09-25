package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tidalwave/backend/internal/store"
)

func TestServesSPAFallback(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=root>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(s, Options{StaticDir: dir}).Handler())
	defer srv.Close()
	for path, want := range map[string]struct {
		status int
		body   string
	}{
		"/cases/abc":      {200, "<div id=root>"},
		"/asset.css":      {200, "body{}"},
		"/api/nope":       {404, ""},
		"/api/auth/login": {405, ""},
	} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != want.status || (want.body != "" && string(body) != want.body) {
			t.Fatalf("%s: %d %q", path, res.StatusCode, body)
		}
	}
}

func TestHealth(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(s, Options{}).Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}
