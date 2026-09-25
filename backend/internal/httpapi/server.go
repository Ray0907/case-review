package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"tidalwave/backend/internal/sse"
	"tidalwave/backend/internal/store"
)

type Runner interface {
	Enqueue(caseID, docID string)
	Refresh(ctx context.Context, caseID string) error
	Retry(ctx context.Context, docID string) error
}

type Options struct {
	StaticDir string
	UploadDir string
	Runner    Runner
	Broker    *sse.Broker
}

type Server struct {
	store *store.Store
	mux   *http.ServeMux
	opts  Options
}

func New(s *store.Store, opts Options) *Server {
	srv := &Server{store: s, mux: http.NewServeMux(), opts: opts}
	srv.routes()
	return srv
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("GET /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "Sign in with POST")
	})
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)
	s.mux.HandleFunc("GET /api/auth/me", s.requireUser(s.me))
	s.mux.HandleFunc("POST /api/cases", s.requireUser(s.createCase))
	s.mux.HandleFunc("GET /api/cases", s.requireUser(s.listCases))
	s.mux.HandleFunc("GET /api/cases/{id}", s.requireUser(s.getCase))
	s.mux.HandleFunc("GET /api/cases/{id}/events", s.requireUser(s.caseEvents))
	s.mux.HandleFunc("POST /api/cases/{id}/documents", s.requireUser(s.uploadDocument))
	s.mux.HandleFunc("GET /api/documents/{id}/file", s.requireUser(s.documentFile))
	s.mux.HandleFunc("POST /api/documents/{id}/retry", s.requireUser(s.retryDocument))
	s.mux.HandleFunc("PATCH /api/documents/{id}/fields/{key}", s.requireUser(s.editField))
	s.mux.HandleFunc("POST /api/cases/{id}/decision", s.requireUser(s.decide))
	s.mux.HandleFunc("GET /api/cases/{id}/audit", s.requireUser(s.audit))
	if s.opts.StaticDir != "" {
		files := http.FileServer(http.Dir(s.opts.StaticDir))
		s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusNotFound, "API route not found; check the URL and method")
				return
			}
			name := filepath.Join(s.opts.StaticDir, filepath.FromSlash(strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")))
			if info, err := os.Stat(name); err != nil || info.IsDir() {
				http.ServeFile(w, r, filepath.Join(s.opts.StaticDir, "index.html"))
				return
			}
			files.ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
