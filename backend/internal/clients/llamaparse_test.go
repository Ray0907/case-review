package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestLlamaParsePollsUntilCompleted(t *testing.T) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v2/parse/upload":
			if _, _, err := r.FormFile("file"); err != nil {
				t.Errorf("missing file: %v", err)
			}
			if r.FormValue("configuration") == "" {
				t.Error("missing configuration")
			}
			json.NewEncoder(w).Encode(map[string]string{"id": "job1"})
		case r.URL.Path == "/api/v2/parse/job1" && r.URL.Query().Get("expand") == "markdown_full":
			json.NewEncoder(w).Encode(map[string]any{"markdown_full": "# Bank statement\nEnding balance 18,482.00"})
		case r.URL.Path == "/api/v2/parse/job1":
			status := "RUNNING"
			if atomic.AddInt32(&polls, 1) >= 2 {
				status = "COMPLETED"
			}
			json.NewEncoder(w).Encode(map[string]any{"job": map[string]string{"status": status}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := filepath.Join(t.TempDir(), "a.pdf")
	os.WriteFile(p, []byte("%PDF-1.4"), 0o644)
	lp := NewLlamaParse(srv.URL, "k")
	lp.pollEvery = 0
	text, err := lp.Parse(context.Background(), p)
	if err != nil || text != "# Bank statement\nEnding balance 18,482.00" {
		t.Fatalf("text %q err %v", text, err)
	}
}

func TestLlamaParseFailedJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]string{"id": "j"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"job": map[string]string{"status": "FAILED"}})
	}))
	defer srv.Close()
	p := filepath.Join(t.TempDir(), "a.pdf")
	os.WriteFile(p, []byte("%PDF-1.4"), 0o644)
	lp := NewLlamaParse(srv.URL, "k")
	lp.pollEvery = 0
	if _, err := lp.Parse(context.Background(), p); err == nil {
		t.Fatal("want error")
	}
}
