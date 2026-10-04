package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tidalwave/backend/internal/grounding"
)

func TestLlamaParseSidecar(t *testing.T) {
	for _, kind := range []string{"valid", "failed page", "missing", "unavailable", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			var sidecarURL string
			sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("API key leaked to sidecar")
				}
				if kind == "unavailable" {
					w.WriteHeader(404)
					return
				}
				if kind == "malformed" {
					w.Write([]byte("bad JSON"))
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"page_number": 1, "page_width": 612, "page_height": 792, "success": kind != "failed page", "items": []any{map[string]any{"md": "1200", "grounding": map[string]any{"lines": []any{map[string]any{"words": []any{map[string]any{"span": []int{0, 4}, "bbox": map[string]int{"x": 10, "y": 20, "w": 30, "h": 10}}}}}}}}})
			}))
			defer sidecar.Close()
			sidecarURL = sidecar.URL
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					json.NewEncoder(w).Encode(map[string]string{"id": "j"})
					return
				}
				if r.URL.Query().Get("expand") == "markdown_full" {
					out := map[string]any{"markdown_full": "1200"}
					if kind != "missing" {
						out["result_content_metadata"] = map[string]any{"grounded_items": map[string]string{"presigned_url": sidecarURL}}
					}
					json.NewEncoder(w).Encode(out)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"job": map[string]string{"status": "COMPLETED"}})
			}))
			defer srv.Close()
			p := filepath.Join(t.TempDir(), "a.pdf")
			if err := os.WriteFile(p, []byte("%PDF-1.4"), 0600); err != nil {
				t.Fatal(err)
			}
			lp := NewLlamaParse(srv.URL, "secret")
			lp.pollEvery = 0
			text, tokens, err := lp.Parse(context.Background(), p)
			if err != nil || text != "1200" {
				t.Fatalf("text %q error %v", text, err)
			}
			got := grounding.Match(tokens, 1200.0)
			if (got != nil) != (kind == "valid") {
				t.Fatalf("unexpected grounding %+v", got)
			}
		})
	}
}
