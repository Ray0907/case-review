# Harbor Underwriting — Mortgage Document Review Agent Implementation Plan

> **For agentic workers (any harness, including pi):** Read `PRODUCT.md` and `docs/superpowers/specs/case-review-mockup.html` first. Execute tasks in order; within each task follow the steps exactly (failing test → run → implement → run → commit). Commit once per task, staging only the paths the task lists. Never add a git remote or push. Load keys with `set -a && source .env && set +a` from the repo root; never print, echo, log, or `cat` key values, and never open `.env` or `.env.swp`. Tasks 1–8 and 10–16 run keyless (`PIPELINE_MODE=fake`); only Task 9 Step 7, the real eval run, and Task 17's real-mode check need `.env`. Tick checkboxes (`- [ ]` → `- [x]`) as you go.

**Goal:** A localhost web app where an underwriter uploads a borrower's documents, watches an AI pipeline classify/parse/extract/score them live, corrects flagged fields, sees DTI and an explainable recommendation, and personally records the decision in an audit log.

**Architecture:** One Go module (`backend/`) serves a REST + SSE API over SQLite and runs a per-document pipeline (LlamaParse → Jev classify → Claude extract → Jev judge) behind small interfaces, so every stage has a deterministic fake used by tests, e2e, and a keyless demo mode. A case-level finalize step computes DTI and a rule-based recommendation; the system never decides. A React/Vite/Tailwind v4 frontend (`frontend/`) reuses the locked mockup's CSS verbatim. `cmd/gendata` generates synthetic PDFs plus eval fixtures from one data table; `cmd/eval` scores the real pipeline against them with Claude traffic routed through spanbox.

**Tech Stack:** Go 1.23+, stdlib `net/http` (1.22 method routing), `modernc.org/sqlite`, `golang.org/x/crypto/bcrypt`, `github.com/anthropics/anthropic-sdk-go`, `github.com/go-pdf/fpdf`, `golang.org/x/image`; React 19 + TypeScript + Vite + Tailwind CSS v4 (`@tailwindcss/vite`) + `react-router-dom`; ego-browser for UI E2E.

**Spec:** `PRODUCT.md` (product truth, all settled decisions) and `docs/superpowers/specs/case-review-mockup.html` (locked visual system; published copy: https://claude.ai/artifact/PZ4exyTVyxMpedZfEJZRHC). Executors read both.

## Global Constraints

- Backend is Go REST API only; frontend is React + Vite + TypeScript + Tailwind CSS v4.
- Structured data in SQLite (`modernc.org/sqlite`, no cgo); uploaded files on local disk under `$DATA_DIR/uploads/`, path stored in DB.
- Auth is native Go: bcrypt password hash + opaque session token in an HttpOnly cookie `tw_session`. No third-party auth library (never better-auth).
- Live processing status uses Server-Sent Events, never WebSockets.
- Exactly 5 document types: `w2`, `form_1040`, `form_1003`, `pay_stub`, `bank_statement`. Anything else classifies as `other` → document status `unsupported`. Never add types.
- Single tenant, single reviewer role. No orgs, no role hierarchy, no borrower login.
- Localhost only. Local git only — never add a remote, never push.
- Secrets only from env: `ANTHROPIC_API_KEY`, `LLAMAPARSE_API_KEY`, `TYPESAFE_API_KEY`, loaded from the repo-root `.env` (already present, gitignored). Never commit keys. Never print, log, echo, `cat`, or include key values in errors, test output, or commit messages — load with `set -a && source .env && set +a`; to check presence, print variable names only (`grep -o '^[A-Z_]*=' .env`).
- DTI = monthly debt ÷ monthly income; QM threshold is **0.43**; "near threshold" band is **0.41 ≤ DTI ≤ 0.43**.
- Judgment scores below **0.8** are low confidence.
- The system only recommends (`eligible` / `ineligible` / `needs_review`). Case status only becomes `approved` / `rejected` / `sent_back` through a reviewer's `POST /api/cases/{id}/decision`.
- Approve is refused server-side (HTTP 409) while any flagged field is unedited.
- Claude model id: `claude-opus-5`. Claude calls go through `ANTHROPIC_BASE_URL` (spanbox proxy `http://localhost:4318/proxy/anthropic` when running with observability). Eval runs send `X-Spanbox-Session: eval-<unix-ts>`.
- Visual tokens and class names come from the mockup's `<style>` block unchanged. Font: Manrope 500/600/700/800. Teal accent (`--accent`) appears only on AI-authored content.
- Code comments: none unless the why is non-obvious.

## Milestones (git checkpoints)

Work stops at each milestone for an external review. At a milestone: all tasks in it are committed (one commit per task), `cd backend && go vet ./... && go test -race ./...` passes (plus the milestone's extra check), then create an annotated tag and **stop and report** — do not start the next milestone until told to continue.

| Tag | After task | Extra check |
|---|---|---|
| `m1-backend-foundation` | 4 | — |
| `m2-pipeline` | 8 | fake-mode smoke run from Task 8 Step 7 |
| `m3-review-api` | 10 | — |
| `m4-frontend` | 14 | `cd frontend && npm run e2e` (ego-browser), then a UI quality gate run by the reviewer with `/impeccable` (AI-slop detector + critique: visual quality vs the mockup, clear copy on every control and error, a first-time user can tell status, next step, and why Approve is disabled without instructions); findings are fixed before m4 passes |
| `m5-data-eval` | 17 | `go run ./cmd/eval -fake` from `backend/` |

```bash
git tag -a m1-backend-foundation -m "Milestone 1: tasks 1-4, tests green"
```
Report format: tag name, `git log --oneline <previous-tag>..HEAD`, test command output summary, any deviation from the plan and why.

## Review Focus

1. **Duplicate upload of the same document type** (a second bank statement): the newest document of a type wins; older ones become `superseded` and stop counting toward fields, DTI, and flags. Test: Task 5 `TestSetDocumentTypeSupersedesOlder`.
2. **Approve through the API while a field is still flagged** (bypassing the disabled button): must return 409 and leave status unchanged. Test: Task 10 `TestApproveBlockedByUnresolvedFlag`.
3. **Monthly income missing or zero**: DTI is `null`, recommendation `needs_review` with reason, no NaN/Inf anywhere in JSON. Test: Task 7 `TestAssessNoIncome`.
4. **Retry after a mid-pipeline failure** (e.g. extractor timeout): retry reprocesses without duplicating field rows and the case finalizes. Test: Task 8 `TestRetryAfterFailure`.
5. **Acting on an already-decided case** (second decision, or editing a field after approval): 409, audit log unchanged. Test: Task 10 `TestDecidedCaseIsFrozen`.

---

## File Structure

```
.gitignore
README.md                               runbook (Task 17)
backend/
  go.mod
  cmd/server/main.go                    wiring, env, seed user, fake/real pipeline mode
  cmd/gendata/main.go                   synthetic PDFs + eval fixtures (Task 15)
  cmd/eval/main.go                      eval runner (Task 16)
  internal/config/config.go             env config
  internal/store/db.go                  Open + schema
  internal/store/users.go               users, sessions
  internal/store/cases.go               cases, documents, fields, judgments, assessments, audit
  internal/auth/auth.go                 bcrypt + token helpers
  internal/schemas/schemas.go           5 doc-type field specs + Validate
  internal/pipeline/pipeline.go         stage interfaces, Process
  internal/pipeline/assess.go           ComputeDTI, Assess
  internal/pipeline/runner.go           async runner, Finalize, Retry
  internal/pipeline/fake/fake.go        deterministic stages
  internal/clients/llamaparse.go        LlamaParse v2 Parser
  internal/clients/jev.go               TypeSafe Jev Classifier + Judge
  internal/clients/claude.go            Claude Extractor
  internal/sse/broker.go                per-case pub/sub
  internal/httpapi/server.go            Server, routes, JSON helpers
  internal/httpapi/auth.go              login/logout/me, middleware
  internal/httpapi/cases.go             cases, uploads, files, retry
  internal/httpapi/review.go            field edit, decision, audit
  internal/httpapi/events.go            SSE endpoint
  internal/evalscore/score.go           eval scoring (pure)
frontend/
  package.json, vite.config.ts, tsconfig*.json, index.html
  src/main.tsx, src/App.tsx
  src/api.ts                            fetch client + types
  src/styles/mockup.css                 extracted from mockup
  src/styles/app.css                    tailwind + mockup + additions
  src/pages/Login.tsx
  src/pages/Workspace.tsx               queue + case layout
  src/components/*.tsx                  Queue, NewCase, SourceDocs, Fields, Dti, Confidence, Decision
  src/useCaseEvents.ts                  SSE hook
  e2e/run.sh, e2e/review.ego.mjs     ego-browser E2E (Task 14)
testdata/                               generated by cmd/gendata
```

---

### Task 1: Backend scaffold, SQLite schema, health endpoint

**Files:**
- Create: `.gitignore`, `backend/go.mod`, `backend/internal/config/config.go`, `backend/internal/store/db.go`, `backend/internal/httpapi/server.go`, `backend/cmd/server/main.go`
- Test: `backend/internal/httpapi/server_test.go`, `backend/internal/store/db_test.go`

**Interfaces:**
- Produces: `config.Load() config.Config`; `store.Open(path string) (*store.Store, error)`; `(*store.Store).DB() *sql.DB`; `httpapi.New(s *store.Store) *httpapi.Server`; `(*Server).Handler() http.Handler`; helpers `writeJSON(w, status, v)`, `writeError(w, status, msg)`, `readJSON(r, v) error`.

- [ ] **Step 1: Init module and ignore file**

```bash
cd /Users/ray/Documents/ray/tidalwave
mkdir -p backend && cd backend
go mod init tidalwave/backend
go get modernc.org/sqlite@latest
cd ..
cat .gitignore
```
`.gitignore` already exists (created during planning to protect the user's `.env`). Confirm it contains `.env`, `.env.*`, `!.env.example`, `*.swp`, `data/`, `backend/data/`, `frontend/node_modules/`, `frontend/dist/`, `frontend/test-results/`, `frontend/playwright-report/`, `eval/report.json`. Run `git check-ignore .env` — it must print `.env`.

- [ ] **Step 2: Write the failing tests**

`backend/internal/store/db_test.go`:
```go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesSchema(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, table := range []string{"users", "sessions", "cases", "documents", "fields", "judgments", "assessments", "audit_log"} {
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s missing (n=%d err=%v)", table, n, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.db")
	for i := 0; i < 2; i++ {
		s, err := Open(p)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		s.Close()
	}
}
```

`backend/internal/httpapi/server_test.go`:
```go
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tidalwave/backend/internal/store"
)

func TestHealth(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(s).Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd backend && go test ./...`
Expected: FAIL — `undefined: Open`, `undefined: New`.

- [ ] **Step 4: Implement**

`backend/internal/config/config.go`:
```go
package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Port             string
	DataDir          string
	PipelineMode     string
	AnthropicBaseURL string
	SpanboxSession   string
	SpanboxToken     string
	LlamaParseKey    string
	LlamaParseURL    string
	TypeSafeKey      string
	SeedEmail        string
	SeedPassword     string
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// The user's .env uses LLAMAPARSE_API and TYPESAVE_API; the *_KEY names are accepted too.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func Load() Config {
	return Config{
		Port:             env("PORT", "8080"),
		DataDir:          env("DATA_DIR", filepath.Join(".", "data")),
		PipelineMode:     env("PIPELINE_MODE", "real"),
		AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
		SpanboxSession:   os.Getenv("SPANBOX_SESSION"),
		SpanboxToken:     os.Getenv("SPANBOX_TOKEN"),
		LlamaParseKey:    firstEnv("LLAMAPARSE_API_KEY", "LLAMAPARSE_API"),
		LlamaParseURL:    env("LLAMAPARSE_BASE_URL", "https://api.cloud.llamaindex.ai"),
		TypeSafeKey:      firstEnv("TYPESAFE_API_KEY", "TYPESAVE_API"),
		SeedEmail:        env("SEED_EMAIL", "maya@harbor.test"),
		SeedPassword:     env("SEED_PASSWORD", "harbor-demo"),
	}
}
```

`backend/internal/store/db.go`:
```go
package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, name TEXT NOT NULL,
  title TEXT NOT NULL, password_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), expires_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS cases (
  id TEXT PRIMARY KEY, borrower_name TEXT NOT NULL, loan_number TEXT NOT NULL,
  loan_product TEXT NOT NULL, requested_amount REAL NOT NULL,
  status TEXT NOT NULL DEFAULT 'processing', created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY, case_id TEXT NOT NULL REFERENCES cases(id), file_name TEXT NOT NULL,
  file_path TEXT NOT NULL, doc_type TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
  failure_reason TEXT NOT NULL DEFAULT '', uploaded_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS fields (
  document_id TEXT NOT NULL REFERENCES documents(id), key TEXT NOT NULL, value TEXT NOT NULL,
  flagged INTEGER NOT NULL DEFAULT 0, flag_reason TEXT NOT NULL DEFAULT '', edited INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (document_id, key));
CREATE TABLE IF NOT EXISTS judgments (
  owner_id TEXT NOT NULL, name TEXT NOT NULL, score REAL NOT NULL, reason TEXT NOT NULL,
  PRIMARY KEY (owner_id, name));
CREATE TABLE IF NOT EXISTS assessments (
  case_id TEXT PRIMARY KEY REFERENCES cases(id), monthly_income REAL, monthly_debt REAL, dti REAL,
  recommendation TEXT NOT NULL, reasons TEXT NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT, case_id TEXT NOT NULL REFERENCES cases(id), user_id TEXT NOT NULL,
  action TEXT NOT NULL, note TEXT NOT NULL, created_at INTEGER NOT NULL);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Close() error { return s.db.Close() }
```

`backend/internal/httpapi/server.go`:
```go
package httpapi

import (
	"encoding/json"
	"net/http"

	"tidalwave/backend/internal/store"
)

type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

func New(s *store.Store) *Server {
	srv := &Server{store: s, mux: http.NewServeMux()}
	srv.routes()
	return srv
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
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
```

`backend/cmd/server/main.go`:
```go
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/httpapi"
	"tidalwave/backend/internal/store"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "uploads"), 0o755); err != nil {
		log.Fatal(err)
	}
	s, err := store.Open(filepath.Join(cfg.DataDir, "harbor.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, httpapi.New(s).Handler()))
}
```

- [ ] **Step 5: Run tests**

Run: `cd backend && go mod tidy && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add .gitignore backend
git commit -m "feat(backend): scaffold module, sqlite schema, health endpoint"
```

---

### Task 2: Native session auth

**Files:**
- Create: `backend/internal/auth/auth.go`, `backend/internal/store/users.go`, `backend/internal/httpapi/auth.go`, `backend/internal/httpapi/helpers_test.go`
- Modify: `backend/internal/httpapi/server.go` (register routes), `backend/cmd/server/main.go` (seed user)
- Test: `backend/internal/auth/auth_test.go`, `backend/internal/httpapi/auth_test.go`

**Interfaces:**
- Consumes: `store.Store`, `writeJSON`, `writeError`, `readJSON`.
- Produces: `auth.HashPassword(pw string) (string, error)`, `auth.CheckPassword(hash, pw string) bool`, `auth.NewToken() string`; `store.User{ID, Email, Name, Title string}`; `(*Store).EnsureUser(email, name, title, hash string) error`; `(*Store).UserByEmail(email string) (User, string, error)` (returns hash); `(*Store).CreateSession(userID, token string, ttl time.Duration) error`; `(*Store).UserBySession(token string) (User, error)`; `(*Store).DeleteSession(token string) error`; `(*Server).requireUser(h http.HandlerFunc) http.HandlerFunc`; `userFrom(r *http.Request) store.User`; test helpers `newTestEnv(t) *testEnv` with `.client` (cookie jar, logged in), `.url`, `.store`.

- [ ] **Step 1: Write the failing tests**

`backend/internal/auth/auth_test.go`:
```go
package auth

import "testing"

func TestHashAndCheck(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "s3cret") || CheckPassword(h, "wrong") {
		t.Fatal("password check mismatch")
	}
}

func TestNewTokenUnique(t *testing.T) {
	a, b := NewToken(), NewToken()
	if a == b || len(a) != 64 {
		t.Fatalf("bad tokens %q %q", a, b)
	}
}
```

`backend/internal/httpapi/helpers_test.go`:
```go
package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/store"
)

type testEnv struct {
	url    string
	store  *store.Store
	client *http.Client
	server *Server
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h, _ := auth.HashPassword("pw")
	if err := s.EnsureUser("maya@harbor.test", "Maya Park", "Senior Underwriter", h); err != nil {
		t.Fatal(err)
	}
	srv := New(s)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	env := &testEnv{url: ts.URL, store: s, client: &http.Client{Jar: jar}, server: srv}
	res := env.do(t, "POST", "/api/auth/login", map[string]string{"email": "maya@harbor.test", "password": "pw"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d", res.StatusCode)
	}
	return env
}

func (e *testEnv) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.url+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
```

`backend/internal/httpapi/auth_test.go`:
```go
package httpapi

import (
	"net/http"
	"testing"
)

func TestLoginWrongPassword(t *testing.T) {
	env := newTestEnv(t)
	res := env.do(t, "POST", "/api/auth/login", map[string]string{"email": "maya@harbor.test", "password": "nope"})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", res.StatusCode)
	}
}

func TestMeRequiresSession(t *testing.T) {
	env := newTestEnv(t)
	res, err := http.Get(env.url + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous: want 401 got %d", res.StatusCode)
	}
	me := decode[map[string]string](t, env.do(t, "GET", "/api/auth/me", nil))
	if me["name"] != "Maya Park" || me["title"] != "Senior Underwriter" {
		t.Fatalf("unexpected me %v", me)
	}
}

func TestLogoutEndsSession(t *testing.T) {
	env := newTestEnv(t)
	env.do(t, "POST", "/api/auth/logout", nil)
	if res := env.do(t, "GET", "/api/auth/me", nil); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout want 401 got %d", res.StatusCode)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/auth/ ./internal/httpapi/`
Expected: FAIL — undefined `HashPassword`, `EnsureUser`.

- [ ] **Step 3: Implement**

```bash
cd backend && go get golang.org/x/crypto/bcrypt@latest
```

`backend/internal/auth/auth.go`:
```go
package auth

import (
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func NewToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
```

`backend/internal/store/users.go`:
```go
package store

import (
	"database/sql"
	"errors"
	"time"

	"tidalwave/backend/internal/auth"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Title string `json:"title"`
}

func (s *Store) EnsureUser(email, name, title, hash string) error {
	_, err := s.db.Exec(`INSERT INTO users (id, email, name, title, password_hash) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(email) DO NOTHING`, auth.NewToken()[:16], email, name, title, hash)
	return err
}

func (s *Store) UserByEmail(email string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRow(`SELECT id, email, name, title, password_hash FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.Title, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) CreateSession(userID, token string, ttl time.Duration) error {
	_, err := s.db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`,
		token, userID, time.Now().Add(ttl).Unix())
	return err
}

func (s *Store) UserBySession(token string) (User, error) {
	var u User
	err := s.db.QueryRow(`SELECT u.id, u.email, u.name, u.title FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ?`, token, time.Now().Unix()).Scan(&u.ID, &u.Email, &u.Name, &u.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
```

`backend/internal/httpapi/auth.go`:
```go
package httpapi

import (
	"context"
	"net/http"
	"time"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/store"
)

const sessionCookie = "tw_session"

type ctxKey struct{}

func userFrom(r *http.Request) store.User { return r.Context().Value(ctxKey{}).(store.User) }

func (s *Server) requireUser(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		u, err := s.store.UserBySession(c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired, sign in again")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, hash, err := s.store.UserByEmail(in.Email)
	if err != nil || !auth.CheckPassword(hash, in.Password) {
		writeError(w, http.StatusUnauthorized, "email or password is incorrect")
		return
	}
	token := auth.NewToken()
	if err := s.store.CreateSession(u.ID, token, 12*time.Hour); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int((12 * time.Hour).Seconds())})
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, userFrom(r)) }
```

Modify `routes()` in `server.go` — append:
```go
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("POST /api/auth/logout", s.logout)
	s.mux.HandleFunc("GET /api/auth/me", s.requireUser(s.me))
```

Modify `cmd/server/main.go` — after `store.Open`, before listen:
```go
	hash, err := auth.HashPassword(cfg.SeedPassword)
	if err != nil {
		log.Fatal(err)
	}
	if err := s.EnsureUser(cfg.SeedEmail, "Maya Park", "Senior Underwriter", hash); err != nil {
		log.Fatal(err)
	}
```
and add import `"tidalwave/backend/internal/auth"`.

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend
git commit -m "feat(backend): bcrypt login with session cookie and seeded reviewer"
```

---

### Task 3: Document-type schemas

**Files:**
- Create: `backend/internal/schemas/schemas.go`
- Test: `backend/internal/schemas/schemas_test.go`

**Interfaces:**
- Produces: consts `W2="w2"`, `Form1040="form_1040"`, `Form1003="form_1003"`, `PayStub="pay_stub"`, `BankStatement="bank_statement"`, `Other="other"`; `Types []string` (the 5, fixed order); `Kind` (`KindString`, `KindNumber`, `KindInt`); `FieldSpec{Key, Label string; Kind Kind}`; `Registry map[string][]FieldSpec`; `Validate(docType string, values map[string]any) []string`; `CheckValue(docType, key string, v any) error`; `Label(docType string) string`.

- [ ] **Step 1: Write the failing test**

```go
package schemas

import "testing"

func validBank() map[string]any {
	return map[string]any{"bank_name": "Chase", "account_last4": "4471", "statement_period": "2026-08",
		"beginning_balance": 14210.55, "ending_balance": 18482.0, "total_deposits": 7412.2,
		"monthly_debt": 3082.0, "nsf_count": float64(0), "bnpl_hits": float64(2)}
}

func TestValidateAcceptsValid(t *testing.T) {
	if errs := Validate(BankStatement, validBank()); len(errs) != 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
}

func TestValidateReportsMissingAndWrongKinds(t *testing.T) {
	v := validBank()
	delete(v, "ending_balance")
	v["nsf_count"] = 1.5
	v["bank_name"] = 42.0
	errs := Validate(BankStatement, v)
	if len(errs) != 3 {
		t.Fatalf("want 3 errors got %v", errs)
	}
}

func TestValidateRejectsUnknownType(t *testing.T) {
	if errs := Validate("passport", map[string]any{}); len(errs) != 1 {
		t.Fatalf("want 1 error got %v", errs)
	}
}

func TestEveryTypeHasSchemaAndLabel(t *testing.T) {
	if len(Types) != 5 {
		t.Fatalf("exactly 5 types required, got %d", len(Types))
	}
	for _, ty := range Types {
		if len(Registry[ty]) == 0 || Label(ty) == ty {
			t.Fatalf("type %s missing schema or label", ty)
		}
	}
}

func TestCheckValue(t *testing.T) {
	if err := CheckValue(BankStatement, "ending_balance", 18432.0); err != nil {
		t.Fatal(err)
	}
	if err := CheckValue(BankStatement, "ending_balance", "abc"); err == nil {
		t.Fatal("want kind error")
	}
	if err := CheckValue(BankStatement, "nope", 1.0); err == nil {
		t.Fatal("want unknown field error")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/schemas/`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Implement**

```go
package schemas

import (
	"fmt"
	"math"
)

const (
	W2            = "w2"
	Form1040      = "form_1040"
	Form1003      = "form_1003"
	PayStub       = "pay_stub"
	BankStatement = "bank_statement"
	Other         = "other"
)

var Types = []string{W2, Form1040, Form1003, PayStub, BankStatement}

type Kind string

const (
	KindString Kind = "string"
	KindNumber Kind = "number"
	KindInt    Kind = "int"
)

type FieldSpec struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  Kind   `json:"kind"`
}

var labels = map[string]string{
	W2: "W-2", Form1040: "Form 1040", Form1003: "Form 1003", PayStub: "Pay stub", BankStatement: "Bank statement",
}

func Label(docType string) string {
	if l, ok := labels[docType]; ok {
		return l
	}
	return docType
}

var Registry = map[string][]FieldSpec{
	W2: {
		{"employer_name", "Employer", KindString},
		{"tax_year", "Tax year", KindInt},
		{"box1_wages", "Box 1 wages", KindNumber},
		{"box2_fed_tax", "Box 2 federal tax withheld", KindNumber},
	},
	Form1040: {
		{"tax_year", "Tax year", KindInt},
		{"adjusted_gross_income", "Adjusted gross income", KindNumber},
		{"taxable_income", "Taxable income", KindNumber},
		{"total_tax", "Total tax", KindNumber},
	},
	Form1003: {
		{"borrower_name", "Borrower", KindString},
		{"property_address", "Property address", KindString},
		{"loan_amount", "Loan amount", KindNumber},
		{"loan_purpose", "Loan purpose", KindString},
		{"stated_monthly_income", "Stated monthly income", KindNumber},
	},
	PayStub: {
		{"employer_name", "Employer", KindString},
		{"pay_period_end", "Pay period end", KindString},
		{"gross_pay", "Gross pay this period", KindNumber},
		{"ytd_gross", "Year-to-date gross", KindNumber},
		{"monthly_income", "Monthly gross income", KindNumber},
	},
	BankStatement: {
		{"bank_name", "Bank", KindString},
		{"account_last4", "Account ending", KindString},
		{"statement_period", "Statement period", KindString},
		{"beginning_balance", "Beginning balance", KindNumber},
		{"ending_balance", "Ending balance", KindNumber},
		{"total_deposits", "Total deposits", KindNumber},
		{"monthly_debt", "Recurring monthly debt", KindNumber},
		{"nsf_count", "NSF / overdraft events", KindInt},
		{"bnpl_hits", "BNPL / recurring debt hits", KindInt},
	},
}

func checkKind(kind Kind, v any) bool {
	switch kind {
	case KindString:
		_, ok := v.(string)
		return ok
	case KindNumber:
		f, ok := v.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
	case KindInt:
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	}
	return false
}

func Validate(docType string, values map[string]any) []string {
	specs, ok := Registry[docType]
	if !ok {
		return []string{fmt.Sprintf("unknown document type %q", docType)}
	}
	var errs []string
	for _, f := range specs {
		v, present := values[f.Key]
		if !present || v == nil {
			errs = append(errs, fmt.Sprintf("missing field %s", f.Key))
			continue
		}
		if !checkKind(f.Kind, v) {
			errs = append(errs, fmt.Sprintf("field %s must be %s", f.Key, f.Kind))
		}
	}
	return errs
}

func CheckValue(docType, key string, v any) error {
	for _, f := range Registry[docType] {
		if f.Key == key {
			if !checkKind(f.Kind, v) {
				return fmt.Errorf("%s must be %s", f.Label, f.Kind)
			}
			return nil
		}
	}
	return fmt.Errorf("unknown field %s for %s", key, docType)
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/schemas/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/schemas
git commit -m "feat(backend): field schemas for the five document types"
```

---

### Task 4: Cases, uploads, file serving

**Files:**
- Create: `backend/internal/store/cases.go`, `backend/internal/httpapi/cases.go`
- Modify: `backend/internal/httpapi/server.go` (Runner interface, uploadDir, routes), `backend/internal/httpapi/helpers_test.go` (fake runner), `backend/cmd/server/main.go`
- Test: `backend/internal/httpapi/cases_test.go`

**Interfaces:**
- Consumes: `requireUser`, `userFrom`, helpers.
- Produces:
  - `store.Case{ID, BorrowerName, LoanNumber, LoanProduct string; RequestedAmount float64; Status string; CreatedAt int64}` (json snake_case), `store.Document{ID, CaseID, FileName, FilePath, DocType, Status, FailureReason string; UploadedAt int64}` (`FilePath` json `-`).
  - `(*Store).CreateCase(c Case) (Case, error)`, `ListCases() ([]CaseSummary, error)` where `CaseSummary{Case; Recommendation string; DocCount int}`, `GetCase(id) (Case, error)`, `CreateDocument(d Document) (Document, error)`, `GetDocument(id) (Document, error)`, `ListDocuments(caseID) ([]Document, error)`.
  - `httpapi.Runner` interface: `Enqueue(caseID, docID string)`; `Refresh(ctx context.Context, caseID string) error`.
  - `httpapi.New(s *store.Store, opts Options) *Server`, `Options{UploadDir string; Runner Runner; Broker *sse.Broker}` (Broker used from Task 6; nil-safe until then).
  - Routes: `POST /api/cases`, `GET /api/cases`, `GET /api/cases/{id}`, `POST /api/cases/{id}/documents`, `GET /api/documents/{id}/file`.
  - Test helper `fakeRunner{enqueued []string}`.

Document status values used everywhere: `pending`, `parsing`, `classifying`, `extracting`, `judging`, `done`, `failed`, `unsupported`, `superseded`. Case status: `processing`, `needs_review`, `ready`, `approved`, `rejected`, `sent_back`.

- [ ] **Step 1: Update constructor signature first (keeps later diffs small)**

Replace the `Server` struct and `New` in `server.go`:
```go
type Runner interface {
	Enqueue(caseID, docID string)
	Refresh(ctx context.Context, caseID string) error
}

type Options struct {
	UploadDir string
	Runner    Runner
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
```
(add `"context"` import). Update `TestHealth` to `New(s, Options{})`, update `main.go` to `httpapi.New(s, httpapi.Options{UploadDir: filepath.Join(cfg.DataDir, "uploads")})` (runner wired in Task 8).

In `helpers_test.go` add the fake and pass it:
```go
type fakeRunner struct{ enqueued []string }

func (f *fakeRunner) Enqueue(caseID, docID string)                    { f.enqueued = append(f.enqueued, docID) }
func (f *fakeRunner) Refresh(ctx context.Context, caseID string) error { return nil }
```
Add field `runner *fakeRunner` to `testEnv`; in `newTestEnv` build `fr := &fakeRunner{}` and `srv := New(s, Options{UploadDir: t.TempDir(), Runner: fr})`, set `env.runner = fr`. Add import `"context"`.

Add upload helper to `helpers_test.go`:
```go
func (e *testEnv) upload(t *testing.T, caseID, name string, content []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(content)
	mw.Close()
	req, _ := http.NewRequest("POST", e.url+"/api/cases/"+caseID+"/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

var minimalPDF = []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")

func (e *testEnv) createCase(t *testing.T) string {
	t.Helper()
	res := e.do(t, "POST", "/api/cases", map[string]any{"borrower_name": "Jordan Alvarez",
		"loan_number": "HB-20486", "loan_product": "Conventional 30yr fixed", "requested_amount": 410000})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create case %d", res.StatusCode)
	}
	return decode[map[string]any](t, res)["id"].(string)
}
```
(imports `"mime/multipart"`).

- [ ] **Step 2: Write the failing tests** — `backend/internal/httpapi/cases_test.go`:

```go
package httpapi

import (
	"io"
	"net/http"
	"testing"
)

func TestCreateAndListCases(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	list := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases", nil))
	if len(list) != 1 || list[0]["id"] != id || list[0]["status"] != "processing" {
		t.Fatalf("unexpected list %v", list)
	}
}

func TestCreateCaseValidates(t *testing.T) {
	env := newTestEnv(t)
	res := env.do(t, "POST", "/api/cases", map[string]any{"borrower_name": "", "loan_number": "x",
		"loan_product": "y", "requested_amount": 0})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
}

func TestUploadStoresFileAndEnqueues(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	res := env.upload(t, id, "bank-statement.pdf", minimalPDF)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload %d", res.StatusCode)
	}
	doc := decode[map[string]any](t, res)
	if doc["status"] != "pending" || len(env.runner.enqueued) != 1 || env.runner.enqueued[0] != doc["id"] {
		t.Fatalf("doc %v enqueued %v", doc, env.runner.enqueued)
	}
	file := env.do(t, "GET", "/api/documents/"+doc["id"].(string)+"/file", nil)
	body, _ := io.ReadAll(file.Body)
	if file.StatusCode != http.StatusOK || string(body) != string(minimalPDF) {
		t.Fatalf("file %d %q", file.StatusCode, body)
	}
}

func TestUploadRejectsNonDocument(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	if res := env.upload(t, id, "notes.txt", []byte("hello world")); res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("want 415 got %d", res.StatusCode)
	}
}

func TestUploadUnknownCase(t *testing.T) {
	env := newTestEnv(t)
	if res := env.upload(t, "missing", "a.pdf", minimalPDF); res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 got %d", res.StatusCode)
	}
}

func TestCaseDetailIncludesDocuments(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	env.upload(t, id, "w2.pdf", minimalPDF)
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	docs := detail["documents"].([]any)
	if len(docs) != 1 || detail["case"].(map[string]any)["borrower_name"] != "Jordan Alvarez" {
		t.Fatalf("detail %v", detail)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd backend && go test ./internal/httpapi/`
Expected: FAIL — routes 404 / undefined store methods.

- [ ] **Step 4: Implement**

`backend/internal/store/cases.go`:
```go
package store

import (
	"database/sql"
	"errors"
	"time"

	"tidalwave/backend/internal/auth"
)

type Case struct {
	ID              string  `json:"id"`
	BorrowerName    string  `json:"borrower_name"`
	LoanNumber      string  `json:"loan_number"`
	LoanProduct     string  `json:"loan_product"`
	RequestedAmount float64 `json:"requested_amount"`
	Status          string  `json:"status"`
	CreatedAt       int64   `json:"created_at"`
}

type CaseSummary struct {
	Case
	Recommendation string `json:"recommendation"`
	DocCount       int    `json:"doc_count"`
}

type Document struct {
	ID            string `json:"id"`
	CaseID        string `json:"case_id"`
	FileName      string `json:"file_name"`
	FilePath      string `json:"-"`
	DocType       string `json:"doc_type"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason"`
	UploadedAt    int64  `json:"uploaded_at"`
}

func newID() string { return auth.NewToken()[:20] }

func (s *Store) CreateCase(c Case) (Case, error) {
	c.ID, c.Status, c.CreatedAt = newID(), "processing", time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO cases (id, borrower_name, loan_number, loan_product, requested_amount, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ID, c.BorrowerName, c.LoanNumber, c.LoanProduct, c.RequestedAmount, c.Status, c.CreatedAt)
	return c, err
}

func (s *Store) GetCase(id string) (Case, error) {
	var c Case
	err := s.db.QueryRow(`SELECT id, borrower_name, loan_number, loan_product, requested_amount, status, created_at
		FROM cases WHERE id = ?`, id).Scan(&c.ID, &c.BorrowerName, &c.LoanNumber, &c.LoanProduct, &c.RequestedAmount, &c.Status, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) ListCases() ([]CaseSummary, error) {
	rows, err := s.db.Query(`SELECT c.id, c.borrower_name, c.loan_number, c.loan_product, c.requested_amount, c.status, c.created_at,
		COALESCE(a.recommendation, ''), (SELECT count(*) FROM documents d WHERE d.case_id = c.id AND d.status != 'superseded')
		FROM cases c LEFT JOIN assessments a ON a.case_id = c.id ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaseSummary{}
	for rows.Next() {
		var cs CaseSummary
		if err := rows.Scan(&cs.ID, &cs.BorrowerName, &cs.LoanNumber, &cs.LoanProduct, &cs.RequestedAmount, &cs.Status,
			&cs.CreatedAt, &cs.Recommendation, &cs.DocCount); err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *Store) CreateDocument(d Document) (Document, error) {
	if d.ID == "" {
		d.ID = newID()
	}
	d.Status, d.UploadedAt = "pending", time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO documents (id, case_id, file_name, file_path, status, uploaded_at) VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.CaseID, d.FileName, d.FilePath, d.Status, d.UploadedAt)
	if err == nil {
		_, err = s.db.Exec(`UPDATE cases SET status = 'processing' WHERE id = ? AND status IN ('needs_review', 'ready')`, d.CaseID)
	}
	return d, err
}

func scanDocument(row interface{ Scan(...any) error }) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.CaseID, &d.FileName, &d.FilePath, &d.DocType, &d.Status, &d.FailureReason, &d.UploadedAt)
	return d, err
}

const docCols = `id, case_id, file_name, file_path, doc_type, status, failure_reason, uploaded_at`

func (s *Store) GetDocument(id string) (Document, error) {
	d, err := scanDocument(s.db.QueryRow(`SELECT `+docCols+` FROM documents WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *Store) ListDocuments(caseID string) ([]Document, error) {
	rows, err := s.db.Query(`SELECT `+docCols+` FROM documents WHERE case_id = ? ORDER BY uploaded_at, id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
```

`backend/internal/httpapi/cases.go`:
```go
package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tidalwave/backend/internal/store"
)

const maxUpload = 20 << 20

var allowedTypes = map[string]string{"application/pdf": ".pdf", "image/png": ".png", "image/jpeg": ".jpg"}

func (s *Server) createCase(w http.ResponseWriter, r *http.Request) {
	var in store.Case
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(in.BorrowerName) == "" || strings.TrimSpace(in.LoanNumber) == "" ||
		strings.TrimSpace(in.LoanProduct) == "" || in.RequestedAmount <= 0 {
		writeError(w, http.StatusBadRequest, "borrower name, loan number, loan product and a positive amount are required")
		return
	}
	c, err := s.store.CreateCase(in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create case")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) listCases(w http.ResponseWriter, r *http.Request) {
	cases, err := s.store.ListCases()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load cases")
		return
	}
	writeJSON(w, http.StatusOK, cases)
}

func (s *Server) getCase(w http.ResponseWriter, r *http.Request) {
	detail, err := s.store.CaseDetail(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load case")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	if _, err := s.store.GetCase(caseID); err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "attach a file under the form field \"file\" (max 20 MB)")
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	ext, ok := allowedTypes[http.DetectContentType(head[:n])]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "upload a PDF, PNG or JPEG")
		return
	}
	doc := store.Document{ID: newDocID(), CaseID: caseID, FileName: filepath.Base(header.Filename)}
	dir := filepath.Join(s.opts.UploadDir, caseID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}
	doc.FilePath = filepath.Join(dir, doc.ID+ext)
	out, err := os.Create(doc.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}
	_, err = io.Copy(out, io.MultiReader(strings.NewReader(string(head[:n])), file))
	out.Close()
	if err != nil {
		os.Remove(doc.FilePath)
		writeError(w, http.StatusBadRequest, "upload interrupted or larger than 20 MB")
		return
	}
	doc, err = s.store.CreateDocument(doc)
	if err != nil {
		os.Remove(doc.FilePath)
		writeError(w, http.StatusInternalServerError, "could not record document")
		return
	}
	s.opts.Runner.Enqueue(caseID, doc.ID)
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) documentFile(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDocument(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, d.FilePath)
}
```

Add to `cases.go`:
```go
func newDocID() string { return auth.NewToken()[:20] }
```
with import `"tidalwave/backend/internal/auth"`.

Add `CaseDetail` to `store/cases.go` (fields/judgments/assessment are filled in later tasks; the struct is final now):
```go
type Field struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Value      any    `json:"value"`
	Flagged    bool   `json:"flagged"`
	FlagReason string `json:"flag_reason"`
	Edited     bool   `json:"edited"`
}

type Judgment struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type DocumentDetail struct {
	Document
	Fields    []Field    `json:"fields"`
	Judgments []Judgment `json:"judgments"`
}

type Assessment struct {
	MonthlyIncome  *float64 `json:"monthly_income"`
	MonthlyDebt    *float64 `json:"monthly_debt"`
	DTI            *float64 `json:"dti"`
	Recommendation string   `json:"recommendation"`
	Reasons        []string `json:"reasons"`
}

type Detail struct {
	Case           Case             `json:"case"`
	Documents      []DocumentDetail `json:"documents"`
	CaseJudgments  []Judgment       `json:"case_judgments"`
	Assessment     *Assessment      `json:"assessment"`
}

func (s *Store) CaseDetail(id string) (Detail, error) {
	var d Detail
	c, err := s.GetCase(id)
	if err != nil {
		return d, err
	}
	d.Case = c
	docs, err := s.ListDocuments(id)
	if err != nil {
		return d, err
	}
	d.Documents = []DocumentDetail{}
	for _, doc := range docs {
		dd := DocumentDetail{Document: doc, Fields: []Field{}, Judgments: []Judgment{}}
		d.Documents = append(d.Documents, dd)
	}
	d.CaseJudgments = []Judgment{}
	return d, nil
}
```

Register in `routes()`:
```go
	s.mux.HandleFunc("POST /api/cases", s.requireUser(s.createCase))
	s.mux.HandleFunc("GET /api/cases", s.requireUser(s.listCases))
	s.mux.HandleFunc("GET /api/cases/{id}", s.requireUser(s.getCase))
	s.mux.HandleFunc("POST /api/cases/{id}/documents", s.requireUser(s.uploadDocument))
	s.mux.HandleFunc("GET /api/documents/{id}/file", s.requireUser(s.documentFile))
```

`readJSON` uses `DisallowUnknownFields`; `store.Case` json tags match the request keys, so the create payload decodes cleanly.

- [ ] **Step 5: Run tests**

Run: `cd backend && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend
git commit -m "feat(backend): cases, document upload with type sniffing, file serving"
```

---

### Task 5: Pipeline core with fakeable stages

**Files:**
- Create: `backend/internal/pipeline/pipeline.go`, `backend/internal/pipeline/fake/fake.go`
- Modify: `backend/internal/store/cases.go` (pipeline persistence methods, CaseDetail fills fields/judgments)
- Test: `backend/internal/pipeline/pipeline_test.go`, `backend/internal/store/cases_test.go`

**Interfaces:**
- Consumes: `schemas.Validate`, `schemas.Other`, `store.Judgment`.
- Produces (package `pipeline`):
```go
type Parser interface{ Parse(ctx context.Context, path string) (string, error) }
type Classifier interface{ Classify(ctx context.Context, text string) (docType string, confidence float64, err error) }
type Extraction struct {
	Fields    map[string]any
	Uncertain map[string]string // field key -> reason
}
type Extractor interface{ Extract(ctx context.Context, docType, text string) (Extraction, error) }
type Judge interface {
	JudgeDocument(ctx context.Context, docType, text string, fields map[string]any) ([]store.Judgment, error)
	JudgeIncome(ctx context.Context, fieldsByType map[string]map[string]any) (store.Judgment, error)
}
type Event struct {
	CaseID     string `json:"case_id"`
	DocumentID string `json:"document_id,omitempty"`
	Stage      string `json:"stage"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}
type Publisher interface{ Publish(e Event) }
type Stages struct { Parser Parser; Classifier Classifier; Extractor Extractor; Judge Judge }
func Process(ctx context.Context, st *store.Store, stages Stages, pub Publisher, doc store.Document) error
```
- Store additions: `SetDocumentStatus(id, status, reason string) error`, `SetDocumentType(id, docType string) error` (supersedes older same-type docs in the case), `SaveExtraction(docID string, fields map[string]any, uncertain map[string]string) error` (replaces rows), `SaveJudgments(ownerID string, js []Judgment) error` (replaces rows).
- `fake` package: `fake.Stages() pipeline.Stages` keyed on file name keywords; `fake.Parser{FailOnce map[string]bool}`.

- [ ] **Step 1: Write the failing store test** — `backend/internal/store/cases_test.go`:

```go
package store

import (
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSetDocumentTypeSupersedesOlder(t *testing.T) {
	s := newStore(t)
	c, _ := s.CreateCase(Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	old, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "a.pdf", FilePath: "/x"})
	s.SetDocumentType(old.ID, "bank_statement")
	s.SetDocumentStatus(old.ID, "done", "")
	newer, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "b.pdf", FilePath: "/y"})
	if err := s.SetDocumentType(newer.ID, "bank_statement"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDocument(old.ID)
	if got.Status != "superseded" {
		t.Fatalf("older doc status %q, want superseded", got.Status)
	}
	if n, _ := s.GetDocument(newer.ID); n.Status == "superseded" {
		t.Fatal("newer doc must not be superseded")
	}
}

func TestSaveExtractionReplacesRows(t *testing.T) {
	s := newStore(t)
	c, _ := s.CreateCase(Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	d, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "a.pdf", FilePath: "/x"})
	s.SetDocumentType(d.ID, "bank_statement")
	s.SaveExtraction(d.ID, map[string]any{"ending_balance": 1.0, "bank_name": "Chase"}, map[string]string{"ending_balance": "blurry"})
	if err := s.SaveExtraction(d.ID, map[string]any{"ending_balance": 2.0}, nil); err != nil {
		t.Fatal(err)
	}
	detail, _ := s.CaseDetail(c.ID)
	fields := detail.Documents[0].Fields
	if len(fields) != 1 || fields[0].Value != 2.0 || fields[0].Flagged {
		t.Fatalf("fields %+v", fields)
	}
}
```

- [ ] **Step 2: Write the failing pipeline test** — `backend/internal/pipeline/pipeline_test.go`:

```go
package pipeline_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/store"
)

type recorder struct {
	mu     sync.Mutex
	events []pipeline.Event
}

func (r *recorder) Publish(e pipeline.Event) { r.mu.Lock(); r.events = append(r.events, e); r.mu.Unlock() }

func setup(t *testing.T, fileName string) (*store.Store, store.Document) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, _ := s.CreateCase(store.Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	d, _ := s.CreateDocument(store.Document{CaseID: c.ID, FileName: fileName, FilePath: "/tmp/" + fileName})
	return s, d
}

func TestProcessHappyPath(t *testing.T) {
	s, d := setup(t, "bank-statement.pdf")
	rec := &recorder{}
	if err := pipeline.Process(context.Background(), s, fake.Stages(), rec, d); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDocument(d.ID)
	if got.Status != "done" || got.DocType != "bank_statement" {
		t.Fatalf("doc %+v", got)
	}
	var stages []string
	for _, e := range rec.events {
		if e.Status == "done" {
			stages = append(stages, e.Stage)
		}
	}
	want := []string{"parsing", "classifying", "extracting", "judging", "done"}
	if len(stages) != len(want) {
		t.Fatalf("stages %v", stages)
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Fatalf("stages %v want %v", stages, want)
		}
	}
}

func TestProcessUnsupported(t *testing.T) {
	s, d := setup(t, "drivers-license.png")
	if err := pipeline.Process(context.Background(), s, fake.Stages(), &recorder{}, d); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDocument(d.ID)
	if got.Status != "unsupported" || got.DocType != "other" {
		t.Fatalf("doc %+v", got)
	}
}

type failingExtractor struct{}

func (failingExtractor) Extract(context.Context, string, string) (pipeline.Extraction, error) {
	return pipeline.Extraction{}, errors.New("upstream timeout")
}

func TestProcessFailureRecordsStageAndReason(t *testing.T) {
	s, d := setup(t, "w2.pdf")
	stages := fake.Stages()
	stages.Extractor = failingExtractor{}
	rec := &recorder{}
	if err := pipeline.Process(context.Background(), s, stages, rec, d); err == nil {
		t.Fatal("want error")
	}
	got, _ := s.GetDocument(d.ID)
	if got.Status != "failed" || got.FailureReason != "extracting: upstream timeout" {
		t.Fatalf("doc %+v", got)
	}
	last := rec.events[len(rec.events)-1]
	if last.Stage != "extracting" || last.Status != "failed" {
		t.Fatalf("last event %+v", last)
	}
}

type badSchemaExtractor struct{}

func (badSchemaExtractor) Extract(context.Context, string, string) (pipeline.Extraction, error) {
	return pipeline.Extraction{Fields: map[string]any{"employer_name": "Acme"}}, nil
}

func TestProcessRejectsSchemaInvalidExtraction(t *testing.T) {
	s, d := setup(t, "w2.pdf")
	stages := fake.Stages()
	stages.Extractor = badSchemaExtractor{}
	if err := pipeline.Process(context.Background(), s, stages, &recorder{}, d); err == nil {
		t.Fatal("want schema error")
	}
	got, _ := s.GetDocument(d.ID)
	if got.Status != "failed" {
		t.Fatalf("doc %+v", got)
	}
}

func TestLowQualityScanFlagsField(t *testing.T) {
	s, d := setup(t, "bank-statement-lowq.png")
	pipeline.Process(context.Background(), s, fake.Stages(), &recorder{}, d)
	detail, _ := s.CaseDetail(d.CaseID)
	var flagged int
	for _, f := range detail.Documents[0].Fields {
		if f.Flagged {
			flagged++
		}
	}
	if flagged != 1 {
		t.Fatalf("want exactly 1 flagged field, got %d", flagged)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd backend && go test ./internal/store/ ./internal/pipeline/...`
Expected: FAIL — undefined `SetDocumentType`, package `pipeline` missing.

- [ ] **Step 4: Implement store methods** — append to `backend/internal/store/cases.go` (add `"encoding/json"` and `"tidalwave/backend/internal/schemas"` imports):

```go
func (s *Store) SetDocumentStatus(id, status, reason string) error {
	_, err := s.db.Exec(`UPDATE documents SET status = ?, failure_reason = ? WHERE id = ?`, status, reason, id)
	return err
}

func (s *Store) SetDocumentType(id, docType string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE documents SET doc_type = ? WHERE id = ?`, docType, id); err != nil {
		return err
	}
	if docType != schemas.Other {
		if _, err := tx.Exec(`UPDATE documents SET status = 'superseded'
			WHERE case_id = (SELECT case_id FROM documents WHERE id = ?) AND doc_type = ? AND id != ?`, id, docType, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveExtraction(docID string, fields map[string]any, uncertain map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM fields WHERE document_id = ?`, docID); err != nil {
		return err
	}
	for k, v := range fields {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		reason, flagged := uncertain[k]
		if _, err := tx.Exec(`INSERT INTO fields (document_id, key, value, flagged, flag_reason) VALUES (?, ?, ?, ?, ?)`,
			docID, k, string(raw), flagged, reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveJudgments(ownerID string, js []Judgment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM judgments WHERE owner_id = ?`, ownerID); err != nil {
		return err
	}
	for _, j := range js {
		if _, err := tx.Exec(`INSERT INTO judgments (owner_id, name, score, reason) VALUES (?, ?, ?, ?)`,
			ownerID, j.Name, j.Score, j.Reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) fieldsFor(docID, docType string) ([]Field, error) {
	rows, err := s.db.Query(`SELECT key, value, flagged, flag_reason, edited FROM fields WHERE document_id = ?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]Field{}
	for rows.Next() {
		var f Field
		var raw string
		if err := rows.Scan(&f.Key, &raw, &f.Flagged, &f.FlagReason, &f.Edited); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &f.Value); err != nil {
			return nil, err
		}
		byKey[f.Key] = f
	}
	out := []Field{}
	for _, spec := range schemas.Registry[docType] {
		if f, ok := byKey[spec.Key]; ok {
			f.Label = spec.Label
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

func (s *Store) judgmentsFor(ownerID string) ([]Judgment, error) {
	rows, err := s.db.Query(`SELECT name, score, reason FROM judgments WHERE owner_id = ? ORDER BY score DESC, name`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Judgment{}
	for rows.Next() {
		var j Judgment
		if err := rows.Scan(&j.Name, &j.Score, &j.Reason); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
```

Note `fieldsFor` returns fields in schema order and, because `SaveExtraction` fully replaces rows, `TestSaveExtractionReplacesRows` expects only `ending_balance`. For that test the doc type is `bank_statement`, so the field is found in the registry.

Replace the loop body in `CaseDetail`:
```go
	for _, doc := range docs {
		fields, err := s.fieldsFor(doc.ID, doc.DocType)
		if err != nil {
			return d, err
		}
		js, err := s.judgmentsFor(doc.ID)
		if err != nil {
			return d, err
		}
		d.Documents = append(d.Documents, DocumentDetail{Document: doc, Fields: fields, Judgments: js})
	}
	if d.CaseJudgments, err = s.judgmentsFor(id); err != nil {
		return d, err
	}
```
(and delete the old `d.CaseJudgments = []Judgment{}` line).

- [ ] **Step 5: Implement pipeline** — `backend/internal/pipeline/pipeline.go`:

```go
package pipeline

import (
	"context"
	"fmt"
	"strings"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

type Parser interface {
	Parse(ctx context.Context, path string) (string, error)
}

type Classifier interface {
	Classify(ctx context.Context, text string) (docType string, confidence float64, err error)
}

type Extraction struct {
	Fields    map[string]any
	Uncertain map[string]string
}

type Extractor interface {
	Extract(ctx context.Context, docType, text string) (Extraction, error)
}

type Judge interface {
	JudgeDocument(ctx context.Context, docType, text string, fields map[string]any) ([]store.Judgment, error)
	JudgeIncome(ctx context.Context, fieldsByType map[string]map[string]any) (store.Judgment, error)
}

type Event struct {
	CaseID     string `json:"case_id"`
	DocumentID string `json:"document_id,omitempty"`
	Stage      string `json:"stage"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}

type Publisher interface{ Publish(e Event) }

type Stages struct {
	Parser     Parser
	Classifier Classifier
	Extractor  Extractor
	Judge      Judge
}

func Process(ctx context.Context, st *store.Store, stages Stages, pub Publisher, doc store.Document) error {
	emit := func(stage, status, detail string) {
		pub.Publish(Event{CaseID: doc.CaseID, DocumentID: doc.ID, Stage: stage, Status: status, Detail: detail})
	}
	begin := func(stage string) {
		_ = st.SetDocumentStatus(doc.ID, stage, "")
		emit(stage, "running", "")
	}
	fail := func(stage string, err error) error {
		reason := fmt.Sprintf("%s: %v", stage, err)
		_ = st.SetDocumentStatus(doc.ID, "failed", reason)
		emit(stage, "failed", err.Error())
		return fmt.Errorf("document %s %s", doc.ID, reason)
	}

	begin("parsing")
	text, err := stages.Parser.Parse(ctx, doc.FilePath)
	if err != nil {
		return fail("parsing", err)
	}
	emit("parsing", "done", "")

	begin("classifying")
	docType, _, err := stages.Classifier.Classify(ctx, text)
	if err != nil {
		return fail("classifying", err)
	}
	if err := st.SetDocumentType(doc.ID, docType); err != nil {
		return fail("classifying", err)
	}
	emit("classifying", "done", docType)
	if docType == schemas.Other {
		_ = st.SetDocumentStatus(doc.ID, "unsupported", "not one of W-2, 1040, 1003, pay stub, bank statement")
		emit("done", "unsupported", "")
		return nil
	}

	begin("extracting")
	ex, err := stages.Extractor.Extract(ctx, docType, text)
	if err != nil {
		return fail("extracting", err)
	}
	if errs := schemas.Validate(docType, ex.Fields); len(errs) > 0 {
		return fail("extracting", fmt.Errorf("schema: %s", strings.Join(errs, "; ")))
	}
	if err := st.SaveExtraction(doc.ID, ex.Fields, ex.Uncertain); err != nil {
		return fail("extracting", err)
	}
	emit("extracting", "done", "")

	begin("judging")
	js, err := stages.Judge.JudgeDocument(ctx, docType, text, ex.Fields)
	if err != nil {
		return fail("judging", err)
	}
	if err := st.SaveJudgments(doc.ID, js); err != nil {
		return fail("judging", err)
	}
	emit("judging", "done", "")

	_ = st.SetDocumentStatus(doc.ID, "done", "")
	emit("done", "done", "")
	return nil
}
```

Note: `SetDocumentType` supersedes the older same-type doc even if it is mid-flight; `Process` for that older doc then overwrites status on its next `SetDocumentStatus`. Guard it: in `SetDocumentStatus`, add `AND status != 'superseded'` to the `WHERE` clause so a superseded document never un-supersedes itself:
```go
	_, err := s.db.Exec(`UPDATE documents SET status = ?, failure_reason = ? WHERE id = ? AND status != 'superseded'`, status, reason, id)
```

- [ ] **Step 6: Implement fakes** — `backend/internal/pipeline/fake/fake.go`:

```go
package fake

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"sync"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

// Parser returns the file name as "text" so later stages can key off it.
type Parser struct {
	mu       sync.Mutex
	FailOnce map[string]bool
}

func (p *Parser) Parse(_ context.Context, path string) (string, error) {
	name := strings.ToLower(filepath.Base(path))
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, pending := range p.FailOnce {
		if pending && strings.Contains(name, k) {
			p.FailOnce[k] = false
			return "", errors.New("simulated parser outage")
		}
	}
	return name, nil
}

type Classifier struct{}

func (Classifier) Classify(_ context.Context, text string) (string, float64, error) {
	switch {
	case strings.Contains(text, "license"):
		return schemas.Other, 0.97, nil
	case strings.Contains(text, "1040"):
		return schemas.Form1040, 0.98, nil
	case strings.Contains(text, "1003"):
		return schemas.Form1003, 0.98, nil
	case strings.Contains(text, "w2") || strings.Contains(text, "w-2"):
		return schemas.W2, 0.99, nil
	case strings.Contains(text, "pay"):
		return schemas.PayStub, 0.97, nil
	case strings.Contains(text, "bank"):
		return schemas.BankStatement, 0.98, nil
	}
	return schemas.Other, 0.55, nil
}

var canned = map[string]map[string]any{
	schemas.W2:       {"employer_name": "Northwind Logistics", "tax_year": 2025.0, "box1_wages": 86400.0, "box2_fed_tax": 11230.0},
	schemas.Form1040: {"tax_year": 2025.0, "adjusted_gross_income": 84900.0, "taxable_income": 70300.0, "total_tax": 9420.0},
	schemas.Form1003: {"borrower_name": "Jordan Alvarez", "property_address": "118 Linden Ave, Hoboken NJ",
		"loan_amount": 410000.0, "loan_purpose": "purchase", "stated_monthly_income": 7200.0},
	schemas.PayStub: {"employer_name": "Northwind Logistics", "pay_period_end": "2026-08-31",
		"gross_pay": 3323.08, "ytd_gross": 57200.0, "monthly_income": 7200.0},
	schemas.BankStatement: {"bank_name": "Chase", "account_last4": "4471", "statement_period": "2026-08",
		"beginning_balance": 14210.55, "ending_balance": 18482.0, "total_deposits": 7412.2,
		"monthly_debt": 3082.0, "nsf_count": 0.0, "bnpl_hits": 2.0},
}

type Extractor struct{}

func (Extractor) Extract(_ context.Context, docType, text string) (pipeline.Extraction, error) {
	src, ok := canned[docType]
	if !ok {
		return pipeline.Extraction{}, errors.New("no canned data for " + docType)
	}
	fields := make(map[string]any, len(src))
	for k, v := range src {
		fields[k] = v
	}
	ex := pipeline.Extraction{Fields: fields, Uncertain: map[string]string{}}
	if strings.Contains(text, "lowq") && docType == schemas.BankStatement {
		ex.Uncertain["ending_balance"] = "Scan is low resolution; the hundreds digit reads as 3 or 8"
	}
	return ex, nil
}

type Judge struct{}

func (Judge) JudgeDocument(_ context.Context, docType, text string, _ map[string]any) ([]store.Judgment, error) {
	ocr := 0.94
	ocrReason := "Clean digital document"
	if strings.Contains(text, "lowq") {
		ocr, ocrReason = 0.54, "Low-resolution scan with ambiguous digits"
	}
	return []store.Judgment{
		{Name: "field_completeness", Score: 0.96, Reason: "All required fields present"},
		{Name: "document_authenticity", Score: 0.91, Reason: "No signs of tampering or template mismatch"},
		{Name: "ocr_quality", Score: ocr, Reason: ocrReason},
	}, nil
}

func (Judge) JudgeIncome(_ context.Context, byType map[string]map[string]any) (store.Judgment, error) {
	w2, okW := byType[schemas.W2]["box1_wages"].(float64)
	pay, okP := byType[schemas.PayStub]["monthly_income"].(float64)
	if !okW || !okP || pay <= 0 {
		return store.Judgment{Name: "income_consistency", Score: 0.5, Reason: "W-2 or pay stub income missing"}, nil
	}
	gap := math.Abs(w2/12-pay) / pay
	if gap <= 0.05 {
		return store.Judgment{Name: "income_consistency", Score: 0.93, Reason: "Pay stub matches W-2 annualized income"}, nil
	}
	return store.Judgment{Name: "income_consistency", Score: 0.68, Reason: "Pay stub and W-2 income differ by more than 5%"}, nil
}

func Stages() pipeline.Stages {
	return pipeline.Stages{Parser: &Parser{FailOnce: map[string]bool{}}, Classifier: Classifier{}, Extractor: Extractor{}, Judge: Judge{}}
}
```

`box1_wages` 86400/12 = 7200 matches pay stub, so the canned set is internally consistent; the boundary DTI 3082/7200 = 0.428 matches the mockup.

- [ ] **Step 7: Run tests**

Run: `cd backend && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend
git commit -m "feat(backend): pipeline stages with deterministic fakes, supersede duplicates"
```

---

### Task 6: SSE broker and case events endpoint

**Files:**
- Create: `backend/internal/sse/broker.go`, `backend/internal/httpapi/events.go`
- Modify: `backend/internal/httpapi/server.go` (`Options.Broker`, route), `backend/internal/httpapi/helpers_test.go` (pass a broker)
- Test: `backend/internal/sse/broker_test.go`, `backend/internal/httpapi/events_test.go`

**Interfaces:**
- Consumes: `pipeline.Event`.
- Produces: `sse.New() *sse.Broker`; `(*Broker).Publish(e pipeline.Event)` (satisfies `pipeline.Publisher`); `(*Broker).Subscribe(caseID string) (<-chan pipeline.Event, func())`; route `GET /api/cases/{id}/events` emitting `event: stage\ndata: <Event JSON>\n\n` plus `: ping` every 15s.

- [ ] **Step 1: Write the failing tests**

`backend/internal/sse/broker_test.go`:
```go
package sse

import (
	"testing"
	"time"

	"tidalwave/backend/internal/pipeline"
)

func TestBrokerDeliversOnlyToCase(t *testing.T) {
	b := New()
	a, cancelA := b.Subscribe("case-a")
	defer cancelA()
	other, cancelB := b.Subscribe("case-b")
	defer cancelB()
	b.Publish(pipeline.Event{CaseID: "case-a", Stage: "parsing", Status: "running"})
	select {
	case e := <-a:
		if e.Stage != "parsing" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	select {
	case e := <-other:
		t.Fatalf("leaked %+v", e)
	default:
	}
}

func TestPublishNeverBlocksOnSlowSubscriber(t *testing.T) {
	b := New()
	_, cancel := b.Subscribe("c")
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.Publish(pipeline.Event{CaseID: "c"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked")
	}
}
```

`backend/internal/httpapi/events_test.go`:
```go
package httpapi

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"tidalwave/backend/internal/pipeline"
)

func TestEventsStream(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	req := mustReq(t, "GET", env.url+"/api/cases/"+id+"/events")
	res, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		env.broker.Publish(pipeline.Event{CaseID: id, DocumentID: "d1", Stage: "extracting", Status: "running"})
	}()
	sc := bufio.NewScanner(res.Body)
	deadline := time.After(2 * time.Second)
	for {
		lineCh := make(chan string, 1)
		go func() {
			if sc.Scan() {
				lineCh <- sc.Text()
			}
		}()
		select {
		case line := <-lineCh:
			if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"stage":"extracting"`) {
				return
			}
		case <-deadline:
			t.Fatal("event not received")
		}
	}
}
```

Add to `helpers_test.go`: field `broker *sse.Broker`; in `newTestEnv` create `b := sse.New()`, pass `Broker: b` in `Options`, set `env.broker = b`; helper:
```go
func mustReq(t *testing.T, method, url string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/sse/ ./internal/httpapi/`
Expected: FAIL — package `sse` missing.

- [ ] **Step 3: Implement**

`backend/internal/sse/broker.go`:
```go
package sse

import (
	"sync"

	"tidalwave/backend/internal/pipeline"
)

type Broker struct {
	mu   sync.Mutex
	subs map[string]map[chan pipeline.Event]struct{}
}

func New() *Broker { return &Broker{subs: map[string]map[chan pipeline.Event]struct{}{}} }

func (b *Broker) Subscribe(caseID string) (<-chan pipeline.Event, func()) {
	ch := make(chan pipeline.Event, 32)
	b.mu.Lock()
	if b.subs[caseID] == nil {
		b.subs[caseID] = map[chan pipeline.Event]struct{}{}
	}
	b.subs[caseID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs[caseID], ch)
		b.mu.Unlock()
	}
}

// Publish drops events for full subscribers; clients refetch case detail on every event, so a dropped intermediate stage is harmless.
func (b *Broker) Publish(e pipeline.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[e.CaseID] {
		select {
		case ch <- e:
		default:
		}
	}
}
```

`backend/internal/httpapi/events.go`:
```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) caseEvents(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	if _, err := s.store.GetCase(caseID); err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, cancel := s.opts.Broker.Subscribe(caseID)
	defer cancel()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e := <-events:
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: stage\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
```

In `server.go`: add `Broker *sse.Broker` to `Options` (import `"tidalwave/backend/internal/sse"`), and route:
```go
	s.mux.HandleFunc("GET /api/cases/{id}/events", s.requireUser(s.caseEvents))
```
In `main.go` create `broker := sse.New()` and pass `Broker: broker`.

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend
git commit -m "feat(backend): per-case SSE broker and events endpoint"
```

---

### Task 7: DTI and recommendation rules

**Files:**
- Create: `backend/internal/pipeline/assess.go`
- Test: `backend/internal/pipeline/assess_test.go`

**Interfaces:**
- Consumes: `store.Judgment`, `store.Assessment`, `schemas.Types`, `schemas.Label`.
- Produces:
```go
const QMThreshold = 0.43
const NearThresholdFloor = 0.41
const LowConfidence = 0.8
func ComputeDTI(monthlyIncome, monthlyDebt float64) (float64, error)
type AssessInput struct {
	PresentTypes     []string
	MonthlyIncome    *float64
	MonthlyDebt      *float64
	Judgments        []store.Judgment
	UnresolvedFlags  int
	UnsupportedDocs  int
	FailedDocs       int
}
func Assess(in AssessInput) store.Assessment
```
Rules (in this order, every hit appends a reason):
1. Missing any of the 5 types → `"Missing documents: <labels, schema order>"`.
2. `FailedDocs > 0` → `"<n> document(s) failed processing"`.
3. `UnsupportedDocs > 0` → `"<n> unsupported document(s) uploaded"`.
4. Income nil or ≤ 0 → `"Monthly income unavailable, DTI not computed"`, DTI nil. Debt nil → treat as unavailable the same way with `"Monthly debt unavailable, DTI not computed"`.
5. `NearThresholdFloor ≤ DTI ≤ QMThreshold` → `"DTI within 2 points of the 43% QM threshold"`.
6. Each judgment `< LowConfidence` → `"Low confidence: <name> (<pct>%)"`.
7. `UnresolvedFlags > 0` → `"<n> extracted field(s) need verification"`.
Recommendation: any reason → `needs_review`; else DTI > 0.43 → `ineligible`; else `eligible`. DTI rounded to 4 decimals.

- [ ] **Step 1: Write the failing tests**

```go
package pipeline

import (
	"testing"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

func f(v float64) *float64 { return &v }

func good() []store.Judgment {
	return []store.Judgment{{Name: "field_completeness", Score: 0.96}, {Name: "income_consistency", Score: 0.93}}
}

func TestComputeDTI(t *testing.T) {
	cases := []struct {
		income, debt, want float64
		err                bool
	}{
		{7200, 3082, 0.4281, false},
		{7200, 2100, 0.2917, false},
		{7200, 0, 0, false},
		{0, 100, 0, true},
		{-1, 100, 0, true},
		{7200, -5, 0, true},
	}
	for _, c := range cases {
		got, err := ComputeDTI(c.income, c.debt)
		if (err != nil) != c.err || (!c.err && got != c.want) {
			t.Errorf("ComputeDTI(%v,%v)=%v,%v want %v err=%v", c.income, c.debt, got, err, c.want, c.err)
		}
	}
}

func TestAssessEligible(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(2100), Judgments: good()})
	if a.Recommendation != "eligible" || len(a.Reasons) != 0 || *a.DTI != 0.2917 {
		t.Fatalf("%+v", a)
	}
}

func TestAssessIneligibleWhenClean(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(3600), Judgments: good()})
	if a.Recommendation != "ineligible" {
		t.Fatalf("%+v", a)
	}
}

func TestAssessBoundaryNeedsReview(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(3082), Judgments: good()})
	if a.Recommendation != "needs_review" || a.Reasons[0] != "DTI within 2 points of the 43% QM threshold" {
		t.Fatalf("%+v", a)
	}
}

func TestAssessNoIncome(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(0), MonthlyDebt: f(3082), Judgments: good()})
	if a.DTI != nil || a.Recommendation != "needs_review" || a.Reasons[0] != "Monthly income unavailable, DTI not computed" {
		t.Fatalf("%+v", a)
	}
	a = Assess(AssessInput{PresentTypes: schemas.Types, Judgments: good()})
	if a.DTI != nil || a.Recommendation != "needs_review" {
		t.Fatalf("%+v", a)
	}
}

func TestAssessMissingDocsLowConfidenceFlags(t *testing.T) {
	a := Assess(AssessInput{
		PresentTypes:    []string{schemas.W2, schemas.PayStub, schemas.BankStatement, schemas.Form1003},
		MonthlyIncome:   f(7200), MonthlyDebt: f(2100),
		Judgments:       []store.Judgment{{Name: "ocr_quality", Score: 0.54}},
		UnresolvedFlags: 1, UnsupportedDocs: 1, FailedDocs: 1,
	})
	want := []string{
		"Missing documents: Form 1040",
		"1 document(s) failed processing",
		"1 unsupported document(s) uploaded",
		"Low confidence: ocr_quality (54%)",
		"1 extracted field(s) need verification",
	}
	if a.Recommendation != "needs_review" || len(a.Reasons) != len(want) {
		t.Fatalf("%+v", a)
	}
	for i := range want {
		if a.Reasons[i] != want[i] {
			t.Fatalf("reason %d = %q want %q", i, a.Reasons[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/pipeline/ -run 'DTI|Assess'`
Expected: FAIL — undefined `ComputeDTI`.

- [ ] **Step 3: Implement** — `backend/internal/pipeline/assess.go`:

```go
package pipeline

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

const (
	QMThreshold        = 0.43
	NearThresholdFloor = 0.41
	LowConfidence      = 0.8
)

func ComputeDTI(monthlyIncome, monthlyDebt float64) (float64, error) {
	if monthlyIncome <= 0 {
		return 0, errors.New("monthly income must be positive")
	}
	if monthlyDebt < 0 {
		return 0, errors.New("monthly debt cannot be negative")
	}
	return math.Round(monthlyDebt/monthlyIncome*10000) / 10000, nil
}

type AssessInput struct {
	PresentTypes    []string
	MonthlyIncome   *float64
	MonthlyDebt     *float64
	Judgments       []store.Judgment
	UnresolvedFlags int
	UnsupportedDocs int
	FailedDocs      int
}

func Assess(in AssessInput) store.Assessment {
	a := store.Assessment{MonthlyIncome: in.MonthlyIncome, MonthlyDebt: in.MonthlyDebt, Reasons: []string{}}
	present := map[string]bool{}
	for _, t := range in.PresentTypes {
		present[t] = true
	}
	var missing []string
	for _, t := range schemas.Types {
		if !present[t] {
			missing = append(missing, schemas.Label(t))
		}
	}
	if len(missing) > 0 {
		a.Reasons = append(a.Reasons, "Missing documents: "+strings.Join(missing, ", "))
	}
	if in.FailedDocs > 0 {
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d document(s) failed processing", in.FailedDocs))
	}
	if in.UnsupportedDocs > 0 {
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d unsupported document(s) uploaded", in.UnsupportedDocs))
	}
	switch {
	case in.MonthlyIncome == nil || *in.MonthlyIncome <= 0:
		a.Reasons = append(a.Reasons, "Monthly income unavailable, DTI not computed")
	case in.MonthlyDebt == nil:
		a.Reasons = append(a.Reasons, "Monthly debt unavailable, DTI not computed")
	default:
		if dti, err := ComputeDTI(*in.MonthlyIncome, *in.MonthlyDebt); err == nil {
			a.DTI = &dti
			if dti >= NearThresholdFloor && dti <= QMThreshold {
				a.Reasons = append(a.Reasons, "DTI within 2 points of the 43% QM threshold")
			}
		} else {
			a.Reasons = append(a.Reasons, "DTI not computed: "+err.Error())
		}
	}
	for _, j := range in.Judgments {
		if j.Score < LowConfidence {
			a.Reasons = append(a.Reasons, fmt.Sprintf("Low confidence: %s (%.0f%%)", j.Name, j.Score*100))
		}
	}
	if in.UnresolvedFlags > 0 {
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d extracted field(s) need verification", in.UnresolvedFlags))
	}
	switch {
	case len(a.Reasons) > 0:
		a.Recommendation = "needs_review"
	case a.DTI != nil && *a.DTI > QMThreshold:
		a.Recommendation = "ineligible"
	default:
		a.Recommendation = "eligible"
	}
	return a
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/pipeline/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/pipeline/assess.go backend/internal/pipeline/assess_test.go
git commit -m "feat(backend): DTI and rule-based recommendation with explicit reasons"
```

---

### Task 8: Runner, finalize, retry, server wiring (fake mode)

**Files:**
- Create: `backend/internal/pipeline/runner.go`
- Modify: `backend/internal/store/cases.go` (assessment persistence, case status, case snapshot), `backend/internal/httpapi/cases.go` (retry), `backend/internal/httpapi/server.go` (route), `backend/cmd/server/main.go`
- Test: `backend/internal/pipeline/runner_test.go`, `backend/internal/httpapi/flow_test.go`

**Interfaces:**
- Consumes: `Process`, `Assess`, `Stages`, `Publisher`, store methods.
- Produces:
  - `pipeline.NewRunner(st *store.Store, stages Stages, pub Publisher) *Runner`; `(*Runner).Enqueue(caseID, docID string)`; `(*Runner).Refresh(ctx, caseID string) error` (= finalize); `(*Runner).Wait()` (test helper: waits for in-flight jobs).
  - Store: `SaveAssessment(caseID string, a Assessment) error`; `SetCaseStatus(caseID, status string) error`; `ActiveDocuments(caseID string) ([]Document, error)` (status != superseded); `FieldValues(docID string) (map[string]any, error)`; `UnresolvedFlags(caseID string) (int, error)`; `AllJudgments(caseID string) ([]Judgment, error)` (active docs + case owner).
  - Case detail now includes `assessment`.
  - Route `POST /api/documents/{id}/retry` → 202, or 409 unless status is `failed`.
  - `main.go`: `PIPELINE_MODE=fake` uses `fake.Stages()`; `real` wiring arrives in Task 9.

Finalize (in `Refresh`): if any active doc is in a non-terminal status (`pending|parsing|classifying|extracting|judging`) do nothing. Otherwise: collect `done` docs' fields by type; run `JudgeIncome` when both W-2 and pay stub are done (save as case judgment, else clear case judgments); monthly income = pay stub `monthly_income`, falling back to 1003 `stated_monthly_income`; monthly debt = bank statement `monthly_debt`; build `AssessInput`; save; set case status `needs_review` or `ready` unless the case is already `approved|rejected|sent_back`; publish `Event{Stage:"assessment", Status:"done"}`.

- [ ] **Step 1: Write the failing tests**

`backend/internal/pipeline/runner_test.go`:
```go
package pipeline_test

import (
	"context"
	"path/filepath"
	"testing"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/store"
)

func newCase(t *testing.T) (*store.Store, store.Case) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c, _ := s.CreateCase(store.Case{BorrowerName: "Jordan Alvarez", LoanNumber: "HB-1", LoanProduct: "p", RequestedAmount: 410000})
	return s, c
}

func addDocs(t *testing.T, s *store.Store, r *pipeline.Runner, caseID string, names ...string) []string {
	var ids []string
	for _, n := range names {
		d, err := s.CreateDocument(store.Document{CaseID: caseID, FileName: n, FilePath: "/tmp/" + n})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
		r.Enqueue(caseID, d.ID)
	}
	r.Wait()
	return ids
}

var fullSet = []string{"w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", "bank-statement.pdf"}

func TestRunnerFinalizesBoundaryCase(t *testing.T) {
	s, c := newCase(t)
	r := pipeline.NewRunner(s, fake.Stages(), &recorder{})
	addDocs(t, s, r, c.ID, fullSet...)
	d, _ := s.CaseDetail(c.ID)
	if d.Assessment == nil || d.Assessment.DTI == nil || *d.Assessment.DTI != 0.4281 {
		t.Fatalf("assessment %+v", d.Assessment)
	}
	if d.Assessment.Recommendation != "needs_review" || d.Case.Status != "needs_review" {
		t.Fatalf("rec %s status %s", d.Assessment.Recommendation, d.Case.Status)
	}
	if len(d.CaseJudgments) != 1 || d.CaseJudgments[0].Name != "income_consistency" {
		t.Fatalf("case judgments %+v", d.CaseJudgments)
	}
}

func TestRetryAfterFailure(t *testing.T) {
	s, c := newCase(t)
	stages := fake.Stages()
	stages.Parser = &fake.Parser{FailOnce: map[string]bool{"pay-stub": true}}
	r := pipeline.NewRunner(s, stages, &recorder{})
	ids := addDocs(t, s, r, c.ID, fullSet...)
	payID := ids[3]
	if d, _ := s.GetDocument(payID); d.Status != "failed" {
		t.Fatalf("pay stub status %s", d.Status)
	}
	if err := r.Retry(context.Background(), payID); err != nil {
		t.Fatal(err)
	}
	r.Wait()
	if err := r.Retry(context.Background(), payID); err == nil {
		t.Fatal("retry of a done document must fail")
	}
	d, _ := s.CaseDetail(c.ID)
	var payFields int
	for _, doc := range d.Documents {
		if doc.ID == payID {
			payFields = len(doc.Fields)
		}
	}
	if payFields != 5 || d.Assessment.DTI == nil {
		t.Fatalf("pay fields %d assessment %+v", payFields, d.Assessment)
	}
}

func TestRunnerDoesNotFinalizeWhileProcessing(t *testing.T) {
	s, c := newCase(t)
	r := pipeline.NewRunner(s, fake.Stages(), &recorder{})
	s.CreateDocument(store.Document{CaseID: c.ID, FileName: "w2.pdf", FilePath: "/tmp/w2.pdf"})
	if err := r.Refresh(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.CaseDetail(c.ID); d.Assessment != nil {
		t.Fatal("assessment must wait for pending documents")
	}
}
```

`backend/internal/httpapi/flow_test.go` (end-to-end over HTTP with the real runner and fakes):
```go
package httpapi

import (
	"net/http"
	"testing"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
)

func TestUploadFlowProducesAssessment(t *testing.T) {
	env := newTestEnvWithRunner(t, fake.Stages())
	id := env.createCase(t)
	for _, n := range []string{"w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", "bank-statement.pdf"} {
		if res := env.upload(t, id, n, minimalPDF); res.StatusCode != http.StatusCreated {
			t.Fatalf("upload %s: %d", n, res.StatusCode)
		}
	}
	env.realRunner.Wait()
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	a := detail["assessment"].(map[string]any)
	if a["recommendation"] != "needs_review" || a["dti"].(float64) != 0.4281 {
		t.Fatalf("assessment %v", a)
	}
}

var _ = pipeline.Stages{}
```

Add to `helpers_test.go`:
```go
func newTestEnvWithRunner(t *testing.T, stages pipeline.Stages) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	r := pipeline.NewRunner(env.store, stages, env.broker)
	env.server.opts.Runner = r
	env.realRunner = r
	return env
}
```
with `realRunner *pipeline.Runner` on `testEnv`, import `"tidalwave/backend/internal/pipeline"`.

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./...`
Expected: FAIL — `NewRunner` undefined.

- [ ] **Step 3: Implement store additions** — append to `store/cases.go`:

```go
func (s *Store) SaveAssessment(caseID string, a Assessment) error {
	reasons, _ := json.Marshal(a.Reasons)
	_, err := s.db.Exec(`INSERT INTO assessments (case_id, monthly_income, monthly_debt, dti, recommendation, reasons, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(case_id) DO UPDATE SET monthly_income = excluded.monthly_income,
		monthly_debt = excluded.monthly_debt, dti = excluded.dti, recommendation = excluded.recommendation,
		reasons = excluded.reasons, updated_at = excluded.updated_at`,
		caseID, a.MonthlyIncome, a.MonthlyDebt, a.DTI, a.Recommendation, string(reasons), time.Now().Unix())
	return err
}

func (s *Store) assessment(caseID string) (*Assessment, error) {
	var a Assessment
	var reasons string
	err := s.db.QueryRow(`SELECT monthly_income, monthly_debt, dti, recommendation, reasons FROM assessments WHERE case_id = ?`, caseID).
		Scan(&a.MonthlyIncome, &a.MonthlyDebt, &a.DTI, &a.Recommendation, &reasons)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, json.Unmarshal([]byte(reasons), &a.Reasons)
}

func (s *Store) SetCaseStatus(caseID, status string) error {
	_, err := s.db.Exec(`UPDATE cases SET status = ? WHERE id = ?`, status, caseID)
	return err
}

func (s *Store) ActiveDocuments(caseID string) ([]Document, error) {
	all, err := s.ListDocuments(caseID)
	if err != nil {
		return nil, err
	}
	out := []Document{}
	for _, d := range all {
		if d.Status != "superseded" {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *Store) FieldValues(docID string) (map[string]any, error) {
	rows, err := s.db.Query(`SELECT key, value FROM fields WHERE document_id = ?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var k, raw string
		var v any
		if err := rows.Scan(&k, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) UnresolvedFlags(caseID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM fields f JOIN documents d ON d.id = f.document_id
		WHERE d.case_id = ? AND d.status != 'superseded' AND f.flagged = 1 AND f.edited = 0`, caseID).Scan(&n)
	return n, err
}

func (s *Store) AllJudgments(caseID string) ([]Judgment, error) {
	out, err := s.judgmentsFor(caseID)
	if err != nil {
		return nil, err
	}
	docs, err := s.ActiveDocuments(caseID)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		js, err := s.judgmentsFor(d.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, js...)
	}
	return out, nil
}
```
In `CaseDetail`, before `return d, nil`, add:
```go
	if d.Assessment, err = s.assessment(id); err != nil {
		return d, err
	}
```

- [ ] **Step 4: Implement runner** — `backend/internal/pipeline/runner.go`:

```go
package pipeline

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

type Runner struct {
	st     *store.Store
	stages Stages
	pub    Publisher
	wg     sync.WaitGroup
	mu     sync.Mutex // serializes finalize so concurrent documents cannot interleave assessments
}

func NewRunner(st *store.Store, stages Stages, pub Publisher) *Runner {
	return &Runner{st: st, stages: stages, pub: pub}
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) Enqueue(caseID, docID string) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		doc, err := r.st.GetDocument(docID)
		if err != nil {
			log.Printf("runner: load document %s: %v", docID, err)
			return
		}
		if err := Process(ctx, r.st, r.stages, r.pub, doc); err != nil {
			log.Printf("runner: %v", err)
		}
		if err := r.Refresh(ctx, caseID); err != nil {
			log.Printf("runner: finalize %s: %v", caseID, err)
		}
	}()
}

var ErrNotRetryable = errors.New("only failed documents can be retried")

func (r *Runner) Retry(ctx context.Context, docID string) error {
	doc, err := r.st.GetDocument(docID)
	if err != nil {
		return err
	}
	if doc.Status != "failed" {
		return ErrNotRetryable
	}
	if err := r.st.SetDocumentStatus(docID, "pending", ""); err != nil {
		return err
	}
	r.Enqueue(doc.CaseID, docID)
	return nil
}

var inFlight = map[string]bool{"pending": true, "parsing": true, "classifying": true, "extracting": true, "judging": true}

func num(m map[string]any, k string) *float64 {
	if v, ok := m[k].(float64); ok {
		return &v
	}
	return nil
}

func (r *Runner) Refresh(ctx context.Context, caseID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, err := r.st.GetCase(caseID)
	if err != nil {
		return err
	}
	docs, err := r.st.ActiveDocuments(caseID)
	if err != nil {
		return err
	}
	in := AssessInput{}
	byType := map[string]map[string]any{}
	for _, d := range docs {
		if inFlight[d.Status] {
			return nil
		}
		switch d.Status {
		case "failed":
			in.FailedDocs++
		case "unsupported":
			in.UnsupportedDocs++
		case "done":
			vals, err := r.st.FieldValues(d.ID)
			if err != nil {
				return err
			}
			byType[d.DocType] = vals
			in.PresentTypes = append(in.PresentTypes, d.DocType)
		}
	}
	var caseJudgments []store.Judgment
	if byType[schemas.W2] != nil && byType[schemas.PayStub] != nil {
		j, err := r.stages.Judge.JudgeIncome(ctx, byType)
		if err != nil {
			return err
		}
		caseJudgments = []store.Judgment{j}
	}
	if err := r.st.SaveJudgments(caseID, caseJudgments); err != nil {
		return err
	}
	in.MonthlyIncome = num(byType[schemas.PayStub], "monthly_income")
	if in.MonthlyIncome == nil {
		in.MonthlyIncome = num(byType[schemas.Form1003], "stated_monthly_income")
	}
	in.MonthlyDebt = num(byType[schemas.BankStatement], "monthly_debt")
	if in.Judgments, err = r.st.AllJudgments(caseID); err != nil {
		return err
	}
	if in.UnresolvedFlags, err = r.st.UnresolvedFlags(caseID); err != nil {
		return err
	}
	a := Assess(in)
	if err := r.st.SaveAssessment(caseID, a); err != nil {
		return err
	}
	switch c.Status {
	case "approved", "rejected", "sent_back":
	default:
		status := "ready"
		if a.Recommendation == "needs_review" {
			status = "needs_review"
		}
		if err := r.st.SetCaseStatus(caseID, status); err != nil {
			return err
		}
	}
	r.pub.Publish(Event{CaseID: caseID, Stage: "assessment", Status: "done", Detail: a.Recommendation})
	return nil
}
```

`AllJudgments` returns the case-owner judgments too, so `income_consistency` below 0.8 produces a reason; the canned consistent data scores 0.93, so the boundary case's only reason is the DTI band.

- [ ] **Step 5: Retry endpoint** — extend the `Runner` interface in `server.go`:

```go
type Runner interface {
	Enqueue(caseID, docID string)
	Refresh(ctx context.Context, caseID string) error
	Retry(ctx context.Context, docID string) error
}
```
Add `Retry` to `fakeRunner` in `helpers_test.go`:
```go
func (f *fakeRunner) Retry(ctx context.Context, docID string) error { f.enqueued = append(f.enqueued, docID); return nil }
```
In `cases.go`:
```go
func (s *Server) retryDocument(w http.ResponseWriter, r *http.Request) {
	err := s.opts.Runner.Retry(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "document not found")
	case errors.Is(err, pipeline.ErrNotRetryable):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not retry document")
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}
```
(import `"tidalwave/backend/internal/pipeline"`). Route:
```go
	s.mux.HandleFunc("POST /api/documents/{id}/retry", s.requireUser(s.retryDocument))
```

- [ ] **Step 6: Wire main.go** — replace the handler construction:

```go
	broker := sse.New()
	var stages pipeline.Stages
	switch cfg.PipelineMode {
	case "fake":
		stages = fake.Stages()
		log.Print("pipeline: fake stages (no external calls)")
	default:
		log.Fatal("PIPELINE_MODE=real is wired in Task 9; use PIPELINE_MODE=fake for now")
	}
	runner := pipeline.NewRunner(s, stages, broker)
	handler := httpapi.New(s, httpapi.Options{UploadDir: filepath.Join(cfg.DataDir, "uploads"), Runner: runner, Broker: broker}).Handler()
	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, handler))
```
(imports `pipeline`, `pipeline/fake`, `sse`).

- [ ] **Step 7: Run tests and a smoke run**

Run: `cd backend && go test -race ./... && go vet ./...`
Expected: PASS.

Run: `cd backend && PIPELINE_MODE=fake DATA_DIR=$(mktemp -d) go run ./cmd/server` then in another shell:
```bash
curl -s -c /tmp/c -H 'Content-Type: application/json' -d '{"email":"maya@harbor.test","password":"harbor-demo"}' localhost:8080/api/auth/login
curl -s -b /tmp/c localhost:8080/api/cases
```
Expected: login returns the user JSON; cases returns `[]`. Stop the server.

- [ ] **Step 8: Commit**

```bash
git add backend
git commit -m "feat(backend): async runner, case finalize, document retry, fake pipeline mode"
```

---

### Task 9: Real stage clients (LlamaParse, Jev, Claude via spanbox)

**Files:**
- Create: `backend/internal/clients/llamaparse.go`, `backend/internal/clients/jev.go`, `backend/internal/clients/claude.go`
- Modify: `backend/cmd/server/main.go` (real mode)
- Test: `backend/internal/clients/llamaparse_test.go`, `backend/internal/clients/jev_test.go`, `backend/internal/clients/claude_test.go`

**Interfaces:**
- Consumes: `pipeline.Parser`, `Classifier`, `Extractor`, `Judge`, `schemas.Registry`.
- Produces: `clients.NewLlamaParse(baseURL, apiKey string) *LlamaParse`; `clients.NewJev(apiKey string, baseURL string) *Jev` (implements `Classifier` + `Judge`); `clients.NewClaude(opts ClaudeOptions) *Claude` with `ClaudeOptions{BaseURL, SpanboxSession, SpanboxToken, Model string}`; `clients.RealStages(cfg config.Config) (pipeline.Stages, error)` (errors when a key is missing).

Verified contracts (checked 2026-09-23; re-verify before coding if anything 4xx's):
- **LlamaParse v2** (`https://api.cloud.llamaindex.ai`, `Authorization: Bearer $LLAMAPARSE_API_KEY`): `POST /api/v2/parse/upload` multipart fields `file` and `configuration` (JSON string, `{"tier":"agentic","version":"latest"}`) → top-level `id`; `GET /api/v2/parse/{id}` → status at `job.status` ∈ `PENDING|RUNNING|COMPLETED|FAILED|CANCELLED`; `GET /api/v2/parse/{id}?expand=markdown_full` → top-level `markdown_full` string. v1 is deprecated. Docs: https://developers.llamaindex.ai/llamaparse/parse/guides/api-reference/
- **TypeSafe Jev** (`POST https://api.typesafe.ai/v1/systemone`, `Authorization: Bearer $TYPESAFE_API_KEY`): body `{"state": <any>, "model": "jev-latest", "questions": {"<id>": {"type": "choice|noul|score", "instructions": "...", "criteria": {...}}}}`; response `answers.<id>` with `choice`, `noul` (P(yes)), `probabilities`, `confidence`. Docs: https://docs.typesafe.ai/api.md
- **Claude** Go SDK `github.com/anthropics/anthropic-sdk-go`; `option.WithBaseURL` routes through spanbox (`http://localhost:4318/proxy/anthropic`), `option.WithHeader("X-Spanbox-Session", …)` tags traces (spanbox strips its headers before forwarding). Model `claude-opus-5`. Extraction uses one tool `record_fields` with a JSON schema built from `schemas.Registry`; the prompt instructs Claude to call it (`tool_choice` auto — forced tool choice 400s on newer models, and auto keeps the code model-portable). Check `StopReason == "refusal"` before reading content.

Judgment mapping (Jev): classification is one `choice` question with criteria for the 5 types plus `other`; confidence = `answers.doc_type.confidence`. Document judgments are three `noul` questions asked together: `field_completeness`, `document_authenticity`, `ocr_quality`; score = `noul`. Income consistency is one `noul` question over W-2 + pay stub fields.

- [ ] **Step 1: Write the failing tests (httptest-backed, no network)**

`backend/internal/clients/llamaparse_test.go`:
```go
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
```

`backend/internal/clients/jev_test.go`:
```go
package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jevServer(t *testing.T, answers map[string]any, check func(body map[string]any)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("bad request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "jev-latest" {
			t.Errorf("model %v", body["model"])
		}
		if check != nil {
			check(body)
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.13.0", "answers": answers})
	}))
}

func TestJevClassify(t *testing.T) {
	srv := jevServer(t, map[string]any{"doc_type": map[string]any{"type": "choice", "choice": "bank_statement", "confidence": 0.91}},
		func(body map[string]any) {
			q := body["questions"].(map[string]any)["doc_type"].(map[string]any)
			crit := q["criteria"].(map[string]any)
			if q["type"] != "choice" || len(crit) != 6 {
				t.Errorf("question %v", q)
			}
		})
	defer srv.Close()
	ty, conf, err := NewJev("k", srv.URL).Classify(context.Background(), "Chase statement ...")
	if err != nil || ty != "bank_statement" || conf != 0.91 {
		t.Fatalf("%s %v %v", ty, conf, err)
	}
}

func TestJevClassifyRejectsUnknownChoice(t *testing.T) {
	srv := jevServer(t, map[string]any{"doc_type": map[string]any{"type": "choice", "choice": "passport", "confidence": 0.9}}, nil)
	defer srv.Close()
	if _, _, err := NewJev("k", srv.URL).Classify(context.Background(), "x"); err == nil {
		t.Fatal("want error for choice outside criteria")
	}
}

func TestJevJudgeDocument(t *testing.T) {
	srv := jevServer(t, map[string]any{
		"field_completeness":    map[string]any{"type": "noul", "noul": 0.96},
		"document_authenticity": map[string]any{"type": "noul", "noul": 0.91},
		"ocr_quality":           map[string]any{"type": "noul", "noul": 0.54},
	}, nil)
	defer srv.Close()
	js, err := NewJev("k", srv.URL).JudgeDocument(context.Background(), "bank_statement", "text", map[string]any{"ending_balance": 1.0})
	if err != nil || len(js) != 3 {
		t.Fatalf("%v %v", js, err)
	}
	for _, j := range js {
		if j.Name == "ocr_quality" && j.Score != 0.54 {
			t.Fatalf("ocr %+v", j)
		}
	}
}
```

`backend/internal/clients/claude_test.go` (fake Messages endpoint returning a `tool_use` block; exercises the proxy base URL and spanbox header):
```go
package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaudeExtractUsesProxyAndSessionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("X-Spanbox-Session") != "eval-1" {
			t.Errorf("session header %q", r.Header.Get("X-Spanbox-Session"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "claude-opus-5" {
			t.Errorf("model %v", body["model"])
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5",
			"stop_reason": "tool_use", "usage": map[string]int{"input_tokens": 10, "output_tokens": 10},
			"content": []any{map[string]any{"type": "tool_use", "id": "tu_1", "name": "record_fields",
				"input": map[string]any{
					"fields":    map[string]any{"employer_name": "Northwind", "tax_year": 2025, "box1_wages": 86400, "box2_fed_tax": 11230},
					"uncertain": []any{map[string]any{"field": "box2_fed_tax", "reason": "smudged"}},
				}}},
		})
	}))
	defer srv.Close()
	c := NewClaude(ClaudeOptions{BaseURL: srv.URL, SpanboxSession: "eval-1", APIKey: "test", Model: "claude-opus-5"})
	ex, err := c.Extract(context.Background(), "w2", "W-2 text")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Fields["box1_wages"] != 86400.0 || ex.Uncertain["box2_fed_tax"] != "smudged" {
		t.Fatalf("%+v", ex)
	}
}

func TestClaudeRefusalIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "m", "type": "message", "role": "assistant", "model": "claude-opus-5",
			"stop_reason": "refusal", "content": []any{}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 0}})
	}))
	defer srv.Close()
	c := NewClaude(ClaudeOptions{BaseURL: srv.URL, APIKey: "test", Model: "claude-opus-5"})
	if _, err := c.Extract(context.Background(), "w2", "x"); err == nil {
		t.Fatal("want refusal error")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/clients/`
Expected: FAIL — package `clients` missing.

- [ ] **Step 3: Implement LlamaParse** — `backend/internal/clients/llamaparse.go`:

```go
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type LlamaParse struct {
	baseURL, apiKey string
	http            *http.Client
	pollEvery       time.Duration
}

func NewLlamaParse(baseURL, apiKey string) *LlamaParse {
	return &LlamaParse{baseURL: baseURL, apiKey: apiKey, http: &http.Client{Timeout: 60 * time.Second}, pollEvery: 2 * time.Second}
}

func (l *LlamaParse) do(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := l.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("llamaparse %s %s: %d %s", req.Method, req.URL.Path, res.StatusCode, b)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (l *LlamaParse) Parse(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filepath.Base(path))
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	mw.WriteField("configuration", `{"tier":"agentic","version":"latest"}`)
	mw.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", l.baseURL+"/api/v2/parse/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var created struct{ ID string `json:"id"` }
	if err := l.do(req, &created); err != nil {
		return "", err
	}
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", l.baseURL+"/api/v2/parse/"+created.ID, nil)
		var st struct {
			Job struct{ Status string `json:"status"` } `json:"job"`
		}
		if err := l.do(req, &st); err != nil {
			return "", err
		}
		switch st.Job.Status {
		case "COMPLETED":
			req, _ := http.NewRequestWithContext(ctx, "GET", l.baseURL+"/api/v2/parse/"+created.ID+"?expand=markdown_full", nil)
			var out struct{ MarkdownFull string `json:"markdown_full"` }
			if err := l.do(req, &out); err != nil {
				return "", err
			}
			return out.MarkdownFull, nil
		case "FAILED", "CANCELLED":
			return "", fmt.Errorf("llamaparse job %s %s", created.ID, st.Job.Status)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(l.pollEvery):
		}
	}
}
```

- [ ] **Step 4: Implement Jev** — `backend/internal/clients/jev.go`:

```go
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

type Jev struct {
	apiKey, baseURL string
	http            *http.Client
}

func NewJev(apiKey, baseURL string) *Jev {
	if baseURL == "" {
		baseURL = "https://api.typesafe.ai"
	}
	return &Jev{apiKey: apiKey, baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

type jevQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type jevAnswer struct {
	Choice     string  `json:"choice"`
	Noul       float64 `json:"noul"`
	Confidence float64 `json:"confidence"`
}

func (j *Jev) ask(ctx context.Context, state any, qs map[string]jevQuestion) (map[string]jevAnswer, error) {
	body, _ := json.Marshal(map[string]any{"state": state, "model": "jev-latest", "questions": qs})
	req, _ := http.NewRequestWithContext(ctx, "POST", j.baseURL+"/v1/systemone", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+j.apiKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := j.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("jev: %d %s", res.StatusCode, b)
	}
	var out struct {
		Answers map[string]jevAnswer `json:"answers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	for id := range qs {
		if _, ok := out.Answers[id]; !ok {
			return nil, fmt.Errorf("jev: no answer for %s", id)
		}
	}
	return out.Answers, nil
}

var docTypeCriteria = map[string]string{
	schemas.W2:            "IRS Form W-2 wage and tax statement from an employer",
	schemas.Form1040:      "IRS Form 1040 individual income tax return",
	schemas.Form1003:      "Fannie Mae Form 1003 Uniform Residential Loan Application",
	schemas.PayStub:       "Employer pay stub / earnings statement for one pay period",
	schemas.BankStatement: "Monthly bank account statement with balances and transactions",
	schemas.Other:         "Anything else, including IDs, letters, or unreadable pages",
}

func (j *Jev) Classify(ctx context.Context, text string) (string, float64, error) {
	ans, err := j.ask(ctx, map[string]string{"document_text": truncate(text, 12000)}, map[string]jevQuestion{
		"doc_type": {Type: "choice", Instructions: "Which kind of mortgage document is `document_text`?", Criteria: docTypeCriteria},
	})
	if err != nil {
		return "", 0, err
	}
	a := ans["doc_type"]
	if _, ok := docTypeCriteria[a.Choice]; !ok {
		return "", 0, fmt.Errorf("jev returned unknown document type %q", a.Choice)
	}
	return a.Choice, a.Confidence, nil
}

func (j *Jev) JudgeDocument(ctx context.Context, docType, text string, fields map[string]any) ([]store.Judgment, error) {
	state := map[string]any{"document_type": schemas.Label(docType), "document_text": truncate(text, 12000), "extracted_fields": fields}
	qs := map[string]jevQuestion{
		"field_completeness": {Type: "noul", Instructions: "Are all values in `extracted_fields` actually present and legible in `document_text`?",
			Criteria: map[string]string{"true": "Every extracted value appears in the document", "false": "Some values are absent, guessed, or illegible"}},
		"document_authenticity": {Type: "noul", Instructions: "Does `document_text` look like an unaltered genuine document of `document_type`?",
			Criteria: map[string]string{"true": "Consistent layout, totals, and dates", "false": "Inconsistent totals, mismatched fonts or dates, or signs of editing"}},
		"ocr_quality": {Type: "noul", Instructions: "Is `document_text` a clean, unambiguous reading of the source, with no garbled or ambiguous digits?",
			Criteria: map[string]string{"true": "Clean text, numbers unambiguous", "false": "Garbled characters or digits that could be misread"}},
	}
	ans, err := j.ask(ctx, state, qs)
	if err != nil {
		return nil, err
	}
	reasons := map[string][2]string{
		"field_completeness":    {"All required fields found in the source", "Some extracted values are not clearly present in the source"},
		"document_authenticity": {"No signs of tampering or template mismatch", "Possible alteration: totals, fonts, or dates look inconsistent"},
		"ocr_quality":           {"Clean reading of the source", "Scan quality makes some digits ambiguous"},
	}
	out := make([]store.Judgment, 0, len(qs))
	for _, name := range []string{"field_completeness", "document_authenticity", "ocr_quality"} {
		score := ans[name].Noul
		reason := reasons[name][0]
		if score < 0.8 {
			reason = reasons[name][1]
		}
		out = append(out, store.Judgment{Name: name, Score: score, Reason: reason})
	}
	return out, nil
}

func (j *Jev) JudgeIncome(ctx context.Context, byType map[string]map[string]any) (store.Judgment, error) {
	ans, err := j.ask(ctx, map[string]any{"w2": byType[schemas.W2], "pay_stub": byType[schemas.PayStub]}, map[string]jevQuestion{
		"income_consistency": {Type: "noul",
			Instructions: "Is the pay stub's monthly income consistent (within about 5%) with the W-2's annual wages divided by 12, allowing for raises?",
			Criteria:     map[string]string{"true": "Income sources agree", "false": "Income sources disagree beyond normal variation"}},
	})
	if err != nil {
		return store.Judgment{}, err
	}
	score := ans["income_consistency"].Noul
	reason := "Pay stub matches W-2 annualized income"
	if score < 0.8 {
		reason = "Pay stub and W-2 income do not reconcile"
	}
	return store.Judgment{Name: "income_consistency", Score: score, Reason: reason}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
```

- [ ] **Step 5: Implement Claude** — `backend/internal/clients/claude.go`:

```bash
cd backend && go get github.com/anthropics/anthropic-sdk-go@latest
```

```go
package clients

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/schemas"
)

type ClaudeOptions struct {
	APIKey         string
	BaseURL        string
	SpanboxSession string
	SpanboxToken   string
	Model          string
}

type Claude struct {
	client anthropic.Client
	model  string
}

func NewClaude(o ClaudeOptions) *Claude {
	var opts []option.RequestOption
	if o.APIKey != "" {
		opts = append(opts, option.WithAPIKey(o.APIKey))
	}
	if o.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(o.BaseURL))
	}
	if o.SpanboxSession != "" {
		opts = append(opts, option.WithHeader("X-Spanbox-Session", o.SpanboxSession))
	}
	if o.SpanboxToken != "" {
		opts = append(opts, option.WithHeader("X-Spanbox-Token", o.SpanboxToken))
	}
	if o.Model == "" {
		o.Model = "claude-opus-5"
	}
	return &Claude{client: anthropic.NewClient(opts...), model: o.Model}
}

func fieldSchema(docType string) map[string]any {
	props := map[string]any{}
	var required []string
	for _, f := range schemas.Registry[docType] {
		t := "string"
		switch f.Kind {
		case schemas.KindNumber:
			t = "number"
		case schemas.KindInt:
			t = "integer"
		}
		props[f.Key] = map[string]any{"type": t, "description": f.Label}
		required = append(required, f.Key)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

const extractSystem = `You extract fields from mortgage documents for an underwriter. Copy values exactly as printed; convert money to plain numbers without symbols or commas. For pay stubs, monthly_income is gross pay normalized to a month (weekly x 52 / 12, biweekly x 26 / 12, semimonthly x 2). For bank statements, monthly_debt is the sum of recurring debt payments visible in the statement period. List every field whose value you are not certain of in "uncertain" with a one-sentence reason. Always answer by calling record_fields.`

func (c *Claude) Extract(ctx context.Context, docType, text string) (pipeline.Extraction, error) {
	tool := anthropic.ToolParam{
		Name:        "record_fields",
		Description: anthropic.String("Record the extracted fields and any uncertain ones."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"fields": fieldSchema(docType),
				"uncertain": map[string]any{"type": "array", "items": map[string]any{
					"type": "object", "required": []string{"field", "reason"},
					"properties": map[string]any{"field": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}},
				}},
			},
			Required: []string{"fields", "uncertain"},
		},
	}
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4000,
		System:    []anthropic.TextBlockParam{{Text: extractSystem}},
		Tools:     []anthropic.ToolUnionParam{{OfTool: &tool}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(
			fmt.Sprintf("Document type: %s\n\n<document>\n%s\n</document>", schemas.Label(docType), text)))},
	})
	if err != nil {
		return pipeline.Extraction{}, err
	}
	if resp.StopReason == "refusal" {
		return pipeline.Extraction{}, fmt.Errorf("claude declined to extract this document")
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == "record_fields" {
			var in struct {
				Fields    map[string]any `json:"fields"`
				Uncertain []struct{ Field, Reason string } `json:"uncertain"`
			}
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &in); err != nil {
				return pipeline.Extraction{}, fmt.Errorf("claude tool input: %w", err)
			}
			ex := pipeline.Extraction{Fields: in.Fields, Uncertain: map[string]string{}}
			for _, u := range in.Uncertain {
				ex.Uncertain[u.Field] = u.Reason
			}
			return ex, nil
		}
	}
	return pipeline.Extraction{}, fmt.Errorf("claude did not call record_fields (stop_reason %s)", resp.StopReason)
}
```

If the compiler rejects a symbol (SDK names drift between releases), fix it against the installed SDK (`go doc github.com/anthropics/anthropic-sdk-go ToolParam`, `go doc .../option WithHeader`) — do not change the request shape. `ClaudeOptions` gained `APIKey` for tests; in production leave it empty so the SDK reads `ANTHROPIC_API_KEY`.

Add `RealStages` in `backend/internal/clients/stages.go`:
```go
package clients

import (
	"errors"

	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/pipeline"
)

func RealStages(cfg config.Config) (pipeline.Stages, error) {
	// ANTHROPIC_API_KEY is not required here: the SDK also resolves an `ant auth login` profile.
	var missing []string
	if cfg.LlamaParseKey == "" {
		missing = append(missing, "LLAMAPARSE_API_KEY")
	}
	if cfg.TypeSafeKey == "" {
		missing = append(missing, "TYPESAFE_API_KEY")
	}
	if len(missing) > 0 {
		return pipeline.Stages{}, errors.New("missing env: " + joinComma(missing) + " (or run with PIPELINE_MODE=fake)")
	}
	jev := NewJev(cfg.TypeSafeKey, "")
	return pipeline.Stages{
		Parser:     NewLlamaParse(cfg.LlamaParseURL, cfg.LlamaParseKey),
		Classifier: jev,
		Extractor: NewClaude(ClaudeOptions{BaseURL: cfg.AnthropicBaseURL, SpanboxSession: cfg.SpanboxSession,
			SpanboxToken: cfg.SpanboxToken, Model: "claude-opus-5"}),
		Judge: jev,
	}, nil
}

func joinComma(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
```

Update `main.go`'s `default:` branch:
```go
	default:
		var err error
		if stages, err = clients.RealStages(cfg); err != nil {
			log.Fatal(err)
		}
		if cfg.AnthropicBaseURL != "" {
			log.Printf("claude traffic via %s", cfg.AnthropicBaseURL)
		}
```

- [ ] **Step 6: Run tests**

Run: `cd backend && go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 7: Manual live check (needs the user's keys; skip if absent)**

```bash
cd backend && set -a && source ../.env && set +a
PIPELINE_MODE=real DATA_DIR=$(mktemp -d) go run ./cmd/server
```
Upload one generated PDF (after Task 15) through the UI or curl; confirm the document reaches `done` and fields look right. If LlamaParse returns 404 on `/api/v2/parse/upload`, re-check the host in the docs page above and set `LLAMAPARSE_BASE_URL`.

- [ ] **Step 8: Commit**

```bash
git add backend
git commit -m "feat(backend): LlamaParse v2, Jev and Claude stage clients with spanbox proxy support"
```

---

### Task 10: Field edits, decisions, audit log

**Files:**
- Create: `backend/internal/httpapi/review.go`
- Modify: `backend/internal/store/cases.go` (edit field, audit), `backend/internal/httpapi/server.go` (routes)
- Test: `backend/internal/httpapi/review_test.go`

**Interfaces:**
- Consumes: `schemas.CheckValue`, `Runner.Refresh`, `userFrom`.
- Produces:
  - Store: `UpdateField(docID, key string, value any) (old any, err error)` (sets `edited=1`, keeps `flagged` for history); `AppendAudit(caseID, userID, action, note string) error`; `AuditLog(caseID string) ([]AuditEntry, error)` where `AuditEntry{ID int64; Action, Note, UserName string; CreatedAt int64}` (newest first).
  - Routes: `PATCH /api/documents/{id}/fields/{key}` body `{"value": <json>}` → 200 case detail; `POST /api/cases/{id}/decision` body `{"action":"approve|reject|send_back","note":"..."}` → 200 case detail; `GET /api/cases/{id}/audit` → entries.
- Rules: decided cases (`approved|rejected|sent_back`) → 409 for edits and decisions; `processing` case → 409 for decisions ("wait for processing to finish"); `approve` with `UnresolvedFlags > 0` → 409 `{"error":"Verify flagged fields before approving","unresolved":n}`; `send_back` and `reject` require a non-empty note (400); invalid value kind → 400 with the schema message. Every accepted edit and decision appends an audit row; edits then call `Runner.Refresh` so DTI reflects corrected values.

- [ ] **Step 1: Write the failing tests** — `backend/internal/httpapi/review_test.go`:

```go
package httpapi

import (
	"net/http"
	"testing"

	"tidalwave/backend/internal/pipeline/fake"
)

func readyCase(t *testing.T, lowQuality bool) (*testEnv, string, string) {
	env := newTestEnvWithRunner(t, fake.Stages())
	id := env.createCase(t)
	bank := "bank-statement.pdf"
	if lowQuality {
		bank = "bank-statement-lowq.png"
	}
	var bankID string
	for _, n := range []string{"w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", bank} {
		content := minimalPDF
		if n == bank && lowQuality {
			content = pngBytes
		}
		doc := decode[map[string]any](t, env.upload(t, id, n, content))
		if n == bank {
			bankID = doc["id"].(string)
		}
	}
	env.realRunner.Wait()
	return env, id, bankID
}

func TestApproveBlockedByUnresolvedFlag(t *testing.T) {
	env, id, _ := readyCase(t, true)
	res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}
	body := decode[map[string]any](t, res)
	if body["unresolved"].(float64) != 1 {
		t.Fatalf("body %v", body)
	}
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	if detail["case"].(map[string]any)["status"] != "needs_review" {
		t.Fatal("status must be unchanged")
	}
}

func TestEditClearsFlagUpdatesDTIThenApprove(t *testing.T) {
	env, id, bankID := readyCase(t, true)
	res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/monthly_debt", map[string]any{"value": 2900})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit monthly_debt %d", res.StatusCode)
	}
	res = env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": 18432})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit ending_balance %d", res.StatusCode)
	}
	env.realRunner.Wait()
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	if dti := detail["assessment"].(map[string]any)["dti"].(float64); dti != 0.4028 {
		t.Fatalf("dti after edit %v", dti)
	}
	res = env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve", "note": "Verified balance against source"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve %d", res.StatusCode)
	}
	audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil))
	if len(audit) != 3 || audit[0]["action"] != "approved" || audit[0]["user_name"] != "Maya Park" {
		t.Fatalf("audit %v", audit)
	}
}

func TestEditRejectsWrongKind(t *testing.T) {
	env, _, bankID := readyCase(t, false)
	res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": "lots"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
}

func TestSendBackRequiresNote(t *testing.T) {
	env, id, _ := readyCase(t, false)
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "send_back"}); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "send_back", "note": "Need 2024 W-2"}); res.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", res.StatusCode)
	}
}

func TestDecidedCaseIsFrozen(t *testing.T) {
	env, id, bankID := readyCase(t, false)
	env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "reject", "note": "DTI too high after review"})
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"}); res.StatusCode != http.StatusConflict {
		t.Fatalf("second decision want 409 got %d", res.StatusCode)
	}
	if res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": 1}); res.StatusCode != http.StatusConflict {
		t.Fatalf("edit after decision want 409 got %d", res.StatusCode)
	}
	if audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil)); len(audit) != 1 {
		t.Fatalf("audit %v", audit)
	}
}

func TestDecisionWhileProcessing(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"}); res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}
}
```

Add to `helpers_test.go` a minimal valid PNG (1×1) so type sniffing accepts it:
```go
var pngBytes = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R',
	0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0, 0x0d, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4, 0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82}
```

Expected DTI after edit: 2900 / 7200 = 0.4028 (below the near-threshold band). The recommendation stays `needs_review` because the document-level `ocr_quality` judgment (0.54) still produces a reason — only the flag gate is lifted, which is what lets Approve through.

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/httpapi/ -run 'Approve|Edit|SendBack|Decided|Processing'`
Expected: FAIL — routes 404.

- [ ] **Step 3: Implement store methods** — append to `store/cases.go`:

```go
type AuditEntry struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	Note      string `json:"note"`
	UserName  string `json:"user_name"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) UpdateField(docID, key string, value any) (any, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM fields WHERE document_id = ? AND key = ?`, docID, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var old any
	_ = json.Unmarshal([]byte(raw), &old)
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`UPDATE fields SET value = ?, edited = 1 WHERE document_id = ? AND key = ?`, string(b), docID, key)
	return old, err
}

func (s *Store) AppendAudit(caseID, userID, action, note string) error {
	_, err := s.db.Exec(`INSERT INTO audit_log (case_id, user_id, action, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		caseID, userID, action, note, time.Now().Unix())
	return err
}

func (s *Store) AuditLog(caseID string) ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT a.id, a.action, a.note, u.name, a.created_at FROM audit_log a JOIN users u ON u.id = a.user_id
		WHERE a.case_id = ? ORDER BY a.id DESC`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.Note, &e.UserName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Implement handlers** — `backend/internal/httpapi/review.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

var decided = map[string]bool{"approved": true, "rejected": true, "sent_back": true}

func (s *Server) editField(w http.ResponseWriter, r *http.Request) {
	doc, err := s.store.GetDocument(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	c, err := s.store.GetCase(doc.CaseID)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	if decided[c.Status] {
		writeError(w, http.StatusConflict, "this case already has a decision; fields are locked")
		return
	}
	var in struct {
		Value json.RawMessage `json:"value"`
	}
	if err := readJSON(r, &in); err != nil || len(in.Value) == 0 {
		writeError(w, http.StatusBadRequest, "send {\"value\": ...}")
		return
	}
	var value any
	if err := json.Unmarshal(in.Value, &value); err != nil {
		writeError(w, http.StatusBadRequest, "value must be valid JSON")
		return
	}
	key := r.PathValue("key")
	if err := schemas.CheckValue(doc.DocType, key, value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	old, err := s.store.UpdateField(doc.ID, key, value)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "field not found on this document")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save field")
		return
	}
	note := fmt.Sprintf("%s · %s: %v → %v", schemas.Label(doc.DocType), key, old, value)
	if err := s.store.AppendAudit(c.ID, userFrom(r).ID, "field_edited", note); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write audit log")
		return
	}
	if err := s.opts.Runner.Refresh(r.Context(), c.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "field saved but assessment refresh failed")
		return
	}
	s.getCaseByID(w, c.ID)
}

var actions = map[string]string{"approve": "approved", "reject": "rejected", "send_back": "sent_back"}

func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	c, err := s.store.GetCase(caseID)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	var in struct{ Action, Note string }
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status, ok := actions[in.Action]
	if !ok {
		writeError(w, http.StatusBadRequest, "action must be approve, reject or send_back")
		return
	}
	if decided[c.Status] {
		writeError(w, http.StatusConflict, "this case already has a decision")
		return
	}
	if c.Status == "processing" {
		writeError(w, http.StatusConflict, "wait for document processing to finish")
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if in.Action != "approve" && in.Note == "" {
		writeError(w, http.StatusBadRequest, "add a note explaining the decision")
		return
	}
	if in.Action == "approve" {
		n, err := s.store.UnresolvedFlags(caseID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check flagged fields")
			return
		}
		if n > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "Verify flagged fields before approving", "unresolved": n})
			return
		}
	}
	if err := s.store.SetCaseStatus(caseID, status); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record decision")
		return
	}
	if err := s.store.AppendAudit(caseID, userFrom(r).ID, status, in.Note); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write audit log")
		return
	}
	s.getCaseByID(w, caseID)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.AuditLog(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) getCaseByID(w http.ResponseWriter, id string) {
	detail, err := s.store.CaseDetail(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load case")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
```

Routes:
```go
	s.mux.HandleFunc("PATCH /api/documents/{id}/fields/{key}", s.requireUser(s.editField))
	s.mux.HandleFunc("POST /api/cases/{id}/decision", s.requireUser(s.decide))
	s.mux.HandleFunc("GET /api/cases/{id}/audit", s.requireUser(s.audit))
```

The `readJSON` decoder uses `DisallowUnknownFields`; the decision struct has fields `Action`, `Note`, which Go matches case-insensitively to `action`, `note`.

- [ ] **Step 5: Run tests**

Run: `cd backend && go test -race ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend
git commit -m "feat(backend): reviewer field edits, server-enforced decisions, audit log"
```

---

### Task 11: Frontend scaffold, tokens from the mockup, login

**Files:**
- Create: `frontend/` (Vite React TS), `frontend/vite.config.ts`, `frontend/src/styles/mockup.css` (extracted), `frontend/src/styles/app.css`, `frontend/src/api.ts`, `frontend/src/App.tsx`, `frontend/src/main.tsx`, `frontend/src/pages/Login.tsx`, `frontend/index.html`
- Test: `npm run build` (typecheck + bundle); login exercised in Task 14 e2e.

**Interfaces:**
- Consumes: backend routes from Tasks 2, 4, 6, 8, 10.
- Produces (`src/api.ts`): types `User`, `CaseSummary`, `CaseDetail`, `DocumentDetail`, `Field`, `Judgment`, `Assessment`, `AuditEntry`, `StageEvent`; `api` object: `login(email, password)`, `logout()`, `me()`, `listCases()`, `createCase(input)`, `getCase(id)`, `uploadDocument(caseId, file)`, `retryDocument(id)`, `editField(docId, key, value)`, `decide(caseId, action, note)`, `audit(caseId)`; `ApiError` class with `status` and `body`.

- [ ] **Step 1: Scaffold**

```bash
cd /Users/ray/Documents/ray/tidalwave
npm create vite@latest -- --help
# If --help lists a flag that skips interactive prompts (e.g. --no-interactive / --immediate false), add it below.
# Otherwise pipe answers: `yes n | npm create vite@latest frontend -- --template react-ts`.
npm create vite@latest frontend -- --template react-ts
cd frontend
npm install
npm install react-router-dom
npm install -D tailwindcss @tailwindcss/vite
rm -f src/App.css src/index.css src/assets/react.svg public/vite.svg
mkdir -p src/styles src/pages src/components e2e
awk '/<style>/{f=1;next}/<\/style>/{f=0}f' ../docs/superpowers/specs/case-review-mockup.html > src/styles/mockup.css
grep -c -- '--accent:#215761' src/styles/mockup.css
```
Expected: last command prints `1` (tokens extracted).

- [ ] **Step 2: Config files**

`frontend/vite.config.ts`:
```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: { "/api": { target: "http://localhost:8080", changeOrigin: false } },
  },
});
```

`frontend/index.html`:
```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Manrope:wght@500;600;700;800&display=swap" />
    <title>Harbor Underwriting</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`frontend/src/styles/app.css`:
```css
@import "tailwindcss";
@import "./mockup.css";

.login-shell { min-height: 100dvh; display: grid; place-items: center; }
.login-card { width: min(380px, 100%); padding: 24px; display: flex; flex-direction: column; gap: 14px; }
.login-card h1 { margin: 0; font-size: 20px; font-weight: 800; letter-spacing: -0.02em; }
.input {
  font: inherit; font-size: 13px; font-weight: 500; color: var(--ink);
  background: var(--surface-sunken); border: 1px solid var(--border-strong);
  border-radius: 10px; padding: 10px 12px;
}
.input:focus-visible { outline: 2px solid var(--ink); outline-offset: 1px; }
.form-label { font-size: 12.5px; font-weight: 600; color: var(--ink-soft); display: flex; flex-direction: column; gap: 6px; }
.form-error { font-size: 12.5px; font-weight: 600; color: var(--bad); }
.empty { padding: 32px 16px; text-align: center; color: var(--ink-mute); font-size: 13px; font-weight: 500; }
.stage-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 10px;
  border-radius: 10px; background: var(--surface-sunken); border: 1px solid var(--border); font-size: 12.5px; font-weight: 600; }
.doc-frame { width: 100%; aspect-ratio: 4 / 5; border: 0; border-radius: 12px; background: var(--surface-sunken); }
.btn:focus-visible, .queue-item:focus-visible, .doc-thumb:focus-visible { outline: 2px solid var(--ink); outline-offset: 2px; }
```

- [ ] **Step 3: API client** — `frontend/src/api.ts`:

```ts
export type User = { id: string; email: string; name: string; title: string };
export type CaseStatus = "processing" | "needs_review" | "ready" | "approved" | "rejected" | "sent_back";
export type DocStatus = "pending" | "parsing" | "classifying" | "extracting" | "judging" | "done" | "failed" | "unsupported" | "superseded";
export type Recommendation = "" | "eligible" | "ineligible" | "needs_review";

export type CaseRecord = {
  id: string; borrower_name: string; loan_number: string; loan_product: string;
  requested_amount: number; status: CaseStatus; created_at: number;
};
export type CaseSummary = CaseRecord & { recommendation: Recommendation; doc_count: number };
export type Field = { key: string; label: string; value: string | number; flagged: boolean; flag_reason: string; edited: boolean };
export type Judgment = { name: string; score: number; reason: string };
export type DocumentDetail = {
  id: string; case_id: string; file_name: string; doc_type: string; status: DocStatus;
  failure_reason: string; uploaded_at: number; fields: Field[]; judgments: Judgment[];
};
export type Assessment = {
  monthly_income: number | null; monthly_debt: number | null; dti: number | null;
  recommendation: Exclude<Recommendation, "">; reasons: string[];
};
export type CaseDetail = { case: CaseRecord; documents: DocumentDetail[]; case_judgments: Judgment[]; assessment: Assessment | null };
export type AuditEntry = { id: number; action: string; note: string; user_name: string; created_at: number };
export type StageEvent = { case_id: string; document_id?: string; stage: string; status: string; detail?: string };
export type NewCase = { borrower_name: string; loan_number: string; loan_product: string; requested_amount: number };

export class ApiError extends Error {
  constructor(public status: number, public body: Record<string, unknown>) {
    super(typeof body.error === "string" ? body.error : `Request failed (${status})`);
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = init.body instanceof FormData ? undefined : { "Content-Type": "application/json" };
  const res = await fetch(path, { credentials: "same-origin", headers, ...init });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, body);
  return body as T;
}

const json = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) });

export const api = {
  login: (email: string, password: string) => request<User>("/api/auth/login", json("POST", { email, password })),
  logout: () => request<void>("/api/auth/logout", json("POST")),
  me: () => request<User>("/api/auth/me"),
  listCases: () => request<CaseSummary[]>("/api/cases"),
  createCase: (input: NewCase) => request<CaseRecord>("/api/cases", json("POST", input)),
  getCase: (id: string) => request<CaseDetail>(`/api/cases/${id}`),
  uploadDocument: (caseId: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<DocumentDetail>(`/api/cases/${caseId}/documents`, { method: "POST", body: form });
  },
  retryDocument: (id: string) => request<void>(`/api/documents/${id}/retry`, json("POST")),
  editField: (docId: string, key: string, value: string | number) =>
    request<CaseDetail>(`/api/documents/${docId}/fields/${key}`, json("PATCH", { value })),
  decide: (caseId: string, action: "approve" | "reject" | "send_back", note: string) =>
    request<CaseDetail>(`/api/cases/${caseId}/decision`, json("POST", { action, note })),
  audit: (caseId: string) => request<AuditEntry[]>(`/api/cases/${caseId}/audit`),
};
```

- [ ] **Step 4: App shell and login**

`frontend/src/main.tsx`:
```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import "./styles/app.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
```

`frontend/src/App.tsx`:
```tsx
import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { api, type User } from "./api";
import Login from "./pages/Login";
import Workspace from "./pages/Workspace";

export default function App() {
  const [user, setUser] = useState<User | null | undefined>(undefined);
  useEffect(() => {
    api.me().then(setUser).catch(() => setUser(null));
  }, []);
  if (user === undefined) return <div className="empty">Loading…</div>;
  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/cases" replace /> : <Login onSignedIn={setUser} />} />
      <Route path="/cases/:caseId?" element={user ? <Workspace user={user} onSignedOut={() => setUser(null)} /> : <Navigate to="/login" replace />} />
      <Route path="*" element={<Navigate to={user ? "/cases" : "/login"} replace />} />
    </Routes>
  );
}
```

`frontend/src/pages/Login.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { api, type User } from "../api";

export default function Login({ onSignedIn }: { onSignedIn: (u: User) => void }) {
  const [email, setEmail] = useState("maya@harbor.test");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      onSignedIn(await api.login(email, password));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign-in failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-shell">
      <form className="card login-card" onSubmit={submit}>
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none"><path d="M3 12c3-4 6-4 9 0s6 4 9 0" stroke="white" strokeWidth="2.4" strokeLinecap="round" /></svg>
          </span>
          Harbor Underwriting
        </div>
        <h1>Sign in to review cases</h1>
        <label className="form-label">Email
          <input id="email" className="input" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </label>
        <label className="form-label">Password
          <input id="password" className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </label>
        {error && <p className="form-error" role="alert">{error}</p>}
        <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
      </form>
    </main>
  );
}
```

Create a placeholder-free minimal `src/pages/Workspace.tsx` so the build passes; Task 12 replaces it:
```tsx
import { api, type User } from "../api";

export default function Workspace({ user, onSignedOut }: { user: User; onSignedOut: () => void }) {
  return (
    <div className="topbar">
      <span>{user.name}</span>
      <button className="btn btn-ghost" onClick={() => api.logout().then(onSignedOut)}>Sign out</button>
    </div>
  );
}
```

- [ ] **Step 5: Build**

Run: `cd frontend && npm run build`
Expected: exit 0, `dist/` produced, no TypeScript errors.

- [ ] **Step 6: Commit**

```bash
git add frontend
git commit -m "feat(frontend): vite react scaffold, mockup tokens, api client, login"
```

---

### Task 12: Review workspace UI (queue, documents, fields, DTI, confidence, decision, live status)

**Files:**
- Create: `frontend/src/useCaseEvents.ts`, `frontend/src/format.ts`, `frontend/src/components/Queue.tsx`, `NewCase.tsx`, `SourceDocs.tsx`, `Fields.tsx`, `Dti.tsx`, `Confidence.tsx`, `Decision.tsx`
- Modify: `frontend/src/pages/Workspace.tsx`
- Test: `npm run build`; behavior covered by Task 14.

**Interfaces:**
- Consumes: `api`, types from `src/api.ts`, mockup class names (`topbar`, `brand`, `app`, `queue`, `queue-item`, `main`, `case-head`, `case-sub`, `m-label`, `m-val`, `pill pill-*`, `columns`, `card`, `card-head`, `card-body`, `doc-strip`, `doc-thumb`, `field-row`, `field-label`, `field-value`, `field-input`, `dti-numbers`, `dti-big`, `dti-of`, `dti-bar`, `dti-fill`, `dti-threshold`, `dti-legend`, `conf-grid`, `conf-row`, `conf-meta`, `conf-name`, `conf-reason`, `conf-score good|warn|bad`, `verdict`, `verdict-icon`, `actions`, `note`, `btn btn-primary|btn-ghost|btn-bad`, `audit`, `dot good|warn|bad`, `tabular`).
- Produces: `useCaseEvents(caseId: string | undefined, onEvent: (e: StageEvent) => void): void`; format helpers `money(n)`, `pct(n)`, `docLabel(t)`, `stageLabel(status)`, `scoreTone(s)`.

Behavior contract:
- Queue lists cases, ordered `needs_review`, `processing`, `ready`, then decided; dot color: warn for `needs_review`, bad for failed docs present is not known in summary so use: `needs_review` → warn, `ready` → good, `processing` → info-style (`pill-info` text "Processing"), decided → no dot, muted.
- Selecting a case navigates to `/cases/:id`; detail refetches on every SSE event for that case and every 5 s while `processing` (fallback if the stream drops).
- Source documents card: one thumb per non-superseded document labeled with `docLabel(doc_type)` or file name while unclassified; the selected document renders in `<iframe className="doc-frame">` (PDF) or `<img>` (png/jpg) from `/api/documents/{id}/file`; a document in progress shows its live stage; `failed` shows reason + "Retry" button; `unsupported` shows "Not a supported document type".
- Fields card shows the selected document's fields in schema order. Flagged + unedited fields render as `field-input` with the flag reason under the label; Enter/blur saves via `editField` (numbers parsed with `Number()`; non-numeric input shows the server's 400 message inline). Edited fields show "Edited" in `edit-hint` style with `--ink-mute`.
- DTI card: `dti-big` colored `--warn` when 0.41–0.43, `--bad` when > 0.43, `--good` otherwise; null DTI shows "Not computed" and the reason.
- Confidence card: document judgments of the selected document plus case judgments; score tone good ≥ 0.8, warn 0.6–0.8, bad < 0.6; the `verdict` block (teal) shows the recommendation title and the reasons list. Title strings: `eligible` → "Recommendation: eligible", `ineligible` → "Recommendation: ineligible (DTI above 43%)", `needs_review` → "Recommendation: needs review". Always ends "This is a recommendation, not a decision."
- Decision bar: note input; "Send back for documents" and "Reject" require a note (disabled with title hint until non-empty); "Approve" disabled while any flagged unedited field exists (title "Verify flagged fields first") or case is `processing`; after a decision the bar is replaced by a line "Approved by Maya Park · <time>" from the latest audit entry and all edits are disabled. Server 409s show inline in `form-error`.
- Audit line under the bar lists the latest 5 audit entries.
- New case: button "New case" at the top of the queue opens an inline form (borrower, loan #, product, amount) → creates, navigates, then shows a file picker "Add documents" (multiple, accept `.pdf,.png,.jpg,.jpeg`) that uploads each file sequentially and reports per-file errors.

- [ ] **Step 1: Helpers** — `frontend/src/format.ts`:

```ts
const usd = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 2 });
const whole = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 0 });

export const money = (n: number | null | undefined) => (n == null ? "—" : usd.format(n));
export const moneyWhole = (n: number | null | undefined) => (n == null ? "—" : whole.format(n));
export const pct = (n: number | null | undefined, digits = 1) => (n == null ? "—" : `${(n * 100).toFixed(digits)}%`);

const labels: Record<string, string> = {
  w2: "W‑2", form_1040: "Form 1040", form_1003: "Form 1003", pay_stub: "Pay stub", bank_statement: "Bank statement", other: "Unsupported",
};
export const docLabel = (t: string, fallback = "Classifying…") => labels[t] ?? fallback;

const stages: Record<string, string> = {
  pending: "Queued", parsing: "Reading document", classifying: "Identifying type", extracting: "Extracting fields",
  judging: "Scoring confidence", done: "Processed", failed: "Failed", unsupported: "Unsupported", superseded: "Replaced",
};
export const stageLabel = (s: string) => stages[s] ?? s;

export const scoreTone = (s: number) => (s >= 0.8 ? "good" : s >= 0.6 ? "warn" : "bad");
export const judgmentLabel = (n: string) =>
  ({ field_completeness: "Field completeness", document_authenticity: "Document authenticity", ocr_quality: "OCR extraction quality", income_consistency: "Income consistency" } as Record<string, string>)[n] ?? n;

export const moneyKeys = new Set(["box1_wages", "box2_fed_tax", "adjusted_gross_income", "taxable_income", "total_tax", "loan_amount",
  "stated_monthly_income", "gross_pay", "ytd_gross", "monthly_income", "beginning_balance", "ending_balance", "total_deposits", "monthly_debt"]);
export const fieldDisplay = (key: string, v: string | number) => (typeof v === "number" && moneyKeys.has(key) ? money(v) : String(v));
```

- [ ] **Step 2: SSE hook** — `frontend/src/useCaseEvents.ts`:

```ts
import { useEffect, useRef } from "react";
import type { StageEvent } from "./api";

export function useCaseEvents(caseId: string | undefined, onEvent: (e: StageEvent) => void) {
  const handler = useRef(onEvent);
  handler.current = onEvent;
  useEffect(() => {
    if (!caseId) return;
    const source = new EventSource(`/api/cases/${caseId}/events`);
    source.addEventListener("stage", (msg) => handler.current(JSON.parse((msg as MessageEvent).data)));
    return () => source.close();
  }, [caseId]);
}
```

- [ ] **Step 3: Components**

`frontend/src/components/Queue.tsx`:
```tsx
import type { CaseSummary } from "../api";
import { moneyWhole } from "../format";

const order: Record<string, number> = { needs_review: 0, processing: 1, ready: 2, approved: 3, rejected: 3, sent_back: 3 };
const dot: Record<string, string> = { needs_review: "warn", ready: "good" };

export default function Queue({ cases, selected, onSelect, onNew }: {
  cases: CaseSummary[]; selected?: string; onSelect: (id: string) => void; onNew: () => void;
}) {
  const sorted = [...cases].sort((a, b) => order[a.status] - order[b.status] || b.created_at - a.created_at);
  return (
    <aside className="queue" aria-label="Review queue">
      <div className="queue-row1" style={{ padding: "0 6px 10px" }}>
        <span className="queue-title" style={{ padding: 0 }}>Review queue</span>
        <button className="btn btn-ghost" style={{ padding: "6px 10px", fontSize: 12 }} onClick={onNew}>New case</button>
      </div>
      {sorted.length === 0 && <p className="empty">No cases yet. Create one to start a review.</p>}
      {sorted.map((c) => (
        <button key={c.id} type="button" className={`queue-item${c.id === selected ? " active" : ""}`}
          style={{ width: "100%", textAlign: "left", font: "inherit", background: undefined }}
          aria-current={c.id === selected ? "page" : undefined} onClick={() => onSelect(c.id)}>
          <span className="queue-row1">
            <span className="queue-name">{c.borrower_name}</span>
            {dot[c.status] ? <span className={`dot ${dot[c.status]}`} aria-label={c.status.replace("_", " ")} />
              : <span className="queue-loan">{c.status === "processing" ? "Processing" : c.status.replace("_", " ")}</span>}
          </span>
          <span className="queue-loan">{c.loan_product} · {moneyWhole(c.requested_amount)}</span>
        </button>
      ))}
    </aside>
  );
}
```

`frontend/src/components/NewCase.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { api, type CaseRecord } from "../api";

export default function NewCase({ onCreated, onCancel }: { onCreated: (c: CaseRecord) => void; onCancel: () => void }) {
  const [form, setForm] = useState({ borrower_name: "", loan_number: "", loan_product: "Conventional 30yr fixed", requested_amount: "" });
  const [error, setError] = useState("");
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value });

  async function submit(e: FormEvent) {
    e.preventDefault();
    try {
      onCreated(await api.createCase({ ...form, requested_amount: Number(form.requested_amount) }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create case");
    }
  }

  return (
    <form className="card" onSubmit={submit} style={{ padding: 16, display: "grid", gap: 12 }}>
      <h2 style={{ margin: 0, fontSize: 13, fontWeight: 700 }}>New case</h2>
      <label className="form-label">Borrower<input id="borrower" className="input" value={form.borrower_name} onChange={set("borrower_name")} required /></label>
      <label className="form-label">Loan number<input id="loan-number" className="input" value={form.loan_number} onChange={set("loan_number")} required /></label>
      <label className="form-label">Loan product<input id="loan-product" className="input" value={form.loan_product} onChange={set("loan_product")} required /></label>
      <label className="form-label">Requested amount (USD)<input id="amount" className="input tabular" inputMode="decimal" value={form.requested_amount} onChange={set("requested_amount")} required /></label>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div style={{ display: "flex", gap: 10 }}>
        <button className="btn btn-primary" type="submit">Create case</button>
        <button className="btn btn-ghost" type="button" onClick={onCancel}>Cancel</button>
      </div>
    </form>
  );
}
```

`frontend/src/components/SourceDocs.tsx`:
```tsx
import { useState } from "react";
import { api, type DocumentDetail } from "../api";
import { docLabel, stageLabel } from "../format";

const busy = new Set(["pending", "parsing", "classifying", "extracting", "judging"]);

export default function SourceDocs({ caseId, docs, selected, onSelect, locked, onChanged }: {
  caseId: string; docs: DocumentDetail[]; selected?: DocumentDetail; onSelect: (id: string) => void; locked: boolean; onChanged: () => void;
}) {
  const [uploadErrors, setUploadErrors] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const received = docs.filter((d) => d.status === "done").length;

  async function upload(files: FileList | null) {
    if (!files) return;
    setUploading(true);
    const errors: string[] = [];
    for (const f of Array.from(files)) {
      try {
        await api.uploadDocument(caseId, f);
      } catch (err) {
        errors.push(`${f.name}: ${err instanceof Error ? err.message : "upload failed"}`);
      }
    }
    setUploadErrors(errors);
    setUploading(false);
    onChanged();
  }

  const isImage = selected && /\.(png|jpe?g)$/i.test(selected.file_name);
  return (
    <section className="card">
      <div className="card-head">
        <h2>Source documents</h2>
        <span className="pill pill-info">{received} of 5 processed</span>
      </div>
      <div className="doc-strip" role="tablist" aria-label="Documents">
        {docs.map((d) => (
          <button key={d.id} role="tab" aria-selected={d.id === selected?.id} type="button"
            className={`doc-thumb${d.id === selected?.id ? " active" : ""}`} onClick={() => onSelect(d.id)}>
            <span className="doc-icon" />
            {d.doc_type ? docLabel(d.doc_type) : d.file_name.slice(0, 10)}
          </button>
        ))}
        {!locked && (
          <label className="doc-thumb" style={{ cursor: "pointer" }}>
            <span aria-hidden="true" style={{ fontSize: 18 }}>+</span>{uploading ? "Uploading…" : "Add"}
            <input type="file" multiple accept=".pdf,.png,.jpg,.jpeg" hidden onChange={(e) => upload(e.target.files)} data-testid="file-input" />
          </label>
        )}
      </div>
      {uploadErrors.map((e) => <p key={e} className="form-error" style={{ padding: "8px 16px 0" }}>{e}</p>)}
      {!selected && <p className="empty">Add the borrower's W‑2, 1040, Form 1003, pay stub and bank statement.</p>}
      {selected && (
        <div style={{ padding: 12, display: "grid", gap: 10 }}>
          {busy.has(selected.status) && <div className="stage-row" aria-live="polite"><span>{stageLabel(selected.status)}</span><span className="pill pill-info">Live</span></div>}
          {selected.status === "failed" && (
            <div className="scan-flag" style={{ marginTop: 0, justifyContent: "space-between" }} role="alert">
              <span>Processing failed — {selected.failure_reason}</span>
              <button className="btn btn-ghost" onClick={() => api.retryDocument(selected.id).then(onChanged)}>Retry</button>
            </div>
          )}
          {selected.status === "unsupported" && <div className="scan-flag" style={{ marginTop: 0 }}>Not a supported document type. Upload a W‑2, 1040, Form 1003, pay stub or bank statement.</div>}
          {isImage
            ? <img className="doc-frame" style={{ objectFit: "contain" }} src={`/api/documents/${selected.id}/file`} alt={`${docLabel(selected.doc_type, "Document")} source scan`} />
            : <iframe className="doc-frame" src={`/api/documents/${selected.id}/file`} title={`${docLabel(selected.doc_type, "Document")} source`} />}
          <p className="scan-caption" style={{ padding: 0 }}>{selected.file_name}</p>
        </div>
      )}
    </section>
  );
}
```

`frontend/src/components/Fields.tsx`:
```tsx
import { useState } from "react";
import { api, type CaseDetail, type DocumentDetail, type Field } from "../api";
import { docLabel, fieldDisplay } from "../format";

function FlaggedInput({ doc, field, onSaved }: { doc: DocumentDetail; field: Field; onSaved: (d: CaseDetail) => void }) {
  const [value, setValue] = useState(String(field.value));
  const [error, setError] = useState("");
  async function save() {
    if (value === String(field.value)) return;
    const parsed = typeof field.value === "number" ? Number(value.replace(/[$,\s]/g, "")) : value;
    try {
      onSaved(await api.editField(doc.id, field.key, typeof parsed === "number" && Number.isNaN(parsed) ? value : parsed));
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save");
    }
  }
  return (
    <div className="field-edit" style={{ flexDirection: "column", alignItems: "flex-end", gap: 4 }}>
      <input className="field-input tabular" id={`field-${field.key}`} aria-label={`${field.label}, flagged for review`}
        value={value} onChange={(e) => setValue(e.target.value)} onBlur={save}
        onKeyDown={(e) => e.key === "Enter" && (e.currentTarget as HTMLInputElement).blur()} />
      {error && <span className="edit-hint" role="alert">{error}</span>}
    </div>
  );
}

export default function Fields({ doc, locked, onSaved }: { doc?: DocumentDetail; locked: boolean; onSaved: (d: CaseDetail) => void }) {
  return (
    <section className="card">
      <div className="card-head">
        <h2>Extracted fields</h2>
        {doc && <span style={{ fontSize: 12, fontWeight: 500, color: "var(--ink-mute)" }}>{docLabel(doc.doc_type, doc.file_name)}</span>}
      </div>
      <div className="card-body">
        {(!doc || doc.fields.length === 0) && <p className="empty">Fields appear here once the document is processed.</p>}
        {doc?.fields.map((f) => {
          const open = f.flagged && !f.edited && !locked;
          return (
            <div key={f.key} className={`field-row${open ? " field-flagged" : ""}`}>
              <span className="field-label">
                {open ? "⚠ " : ""}{f.label}
                {open && <span style={{ display: "block", fontSize: 11, marginTop: 2 }}>{f.flag_reason}</span>}
                {f.edited && <span style={{ display: "block", fontSize: 11, marginTop: 2, color: "var(--ink-mute)" }}>Edited by reviewer</span>}
              </span>
              {open ? <FlaggedInput doc={doc} field={f} onSaved={onSaved} />
                : <span className="field-value tabular" style={f.key === "bnpl_hits" && Number(f.value) > 0 ? { color: "var(--warn)" } : undefined}>
                    {fieldDisplay(f.key, f.value)}{f.key === "bnpl_hits" && Number(f.value) > 0 ? " detected" : ""}
                  </span>}
            </div>
          );
        })}
      </div>
    </section>
  );
}
```

The warning glyph `⚠` mirrors the mockup; it is a text marker next to a text label (with the reason in words beside it), not an icon system.

`frontend/src/components/Dti.tsx`:
```tsx
import type { Assessment } from "../api";
import { money, pct } from "../format";

export default function Dti({ a }: { a: Assessment | null }) {
  const dti = a?.dti ?? null;
  const tone = dti == null ? "var(--ink)" : dti > 0.43 ? "var(--bad)" : dti >= 0.41 ? "var(--warn)" : "var(--good)";
  return (
    <section className="card">
      <div className="card-head"><h2>Debt‑to‑income</h2></div>
      <div className="card-body">
        {!a && <p className="empty">Calculated once every document is processed.</p>}
        {a && (
          <>
            <div className="dti-numbers">
              <span className="dti-big tabular" style={{ color: tone }}>{dti == null ? "Not computed" : pct(dti)}</span>
              <span className="dti-of tabular">{money(a.monthly_debt)} monthly debt ÷ {money(a.monthly_income)} monthly income</span>
            </div>
            <div className="dti-bar" aria-hidden="true">
              <div className="dti-fill" style={{ width: `${Math.min((dti ?? 0) * 100, 100)}%` }} />
              <div className="dti-threshold" />
            </div>
            <div className="dti-legend"><span>0%</span><span>QM threshold 43%</span><span>100%</span></div>
          </>
        )}
      </div>
    </section>
  );
}
```

`frontend/src/components/Confidence.tsx`:
```tsx
import type { Assessment, Judgment } from "../api";
import { judgmentLabel, scoreTone } from "../format";

const titles = {
  eligible: "Recommendation: eligible",
  ineligible: "Recommendation: ineligible (DTI above 43%)",
  needs_review: "Recommendation: needs review",
} as const;

export default function Confidence({ judgments, a }: { judgments: Judgment[]; a: Assessment | null }) {
  return (
    <>
      <div className="card-head"><h2>AI confidence breakdown</h2></div>
      <div className="card-body tight">
        {judgments.length === 0 && <p className="empty">Scores appear after documents are processed.</p>}
        <div className="conf-grid">
          {judgments.map((j) => (
            <div className="conf-row" key={j.name}>
              <div className="conf-meta">
                <div className="conf-name">{judgmentLabel(j.name)}</div>
                <div className="conf-reason">{j.reason}</div>
              </div>
              <span className={`conf-score ${scoreTone(j.score)} tabular`}>{Math.round(j.score * 100)}%</span>
            </div>
          ))}
        </div>
      </div>
      {a && (
        <div className="verdict" aria-live="polite">
          <span className="verdict-icon">AI</span>
          <div>
            <h3>{titles[a.recommendation]}</h3>
            <p>
              {a.reasons.length > 0 ? a.reasons.join(". ") + ". " : "All checks passed. "}
              This is a recommendation, not a decision.
            </p>
          </div>
        </div>
      )}
    </>
  );
}
```

`frontend/src/components/Decision.tsx`:
```tsx
import { useEffect, useState } from "react";
import { api, ApiError, type AuditEntry, type CaseDetail } from "../api";

const done: Record<string, string> = { approved: "Approved", rejected: "Rejected", sent_back: "Sent back" };

export default function Decision({ detail, unresolved, onDecided }: { detail: CaseDetail; unresolved: number; onDecided: (d: CaseDetail) => void }) {
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const status = detail.case.status;
  const decided = status in done;

  useEffect(() => {
    api.audit(detail.case.id).then(setAudit).catch(() => setAudit([]));
  }, [detail]);

  async function decide(action: "approve" | "reject" | "send_back") {
    try {
      onDecided(await api.decide(detail.case.id, action, note));
      setError("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not record decision");
    }
  }

  const processing = status === "processing";
  const approveBlocked = processing || unresolved > 0;
  return (
    <>
      {decided ? (
        <div className="actions"><strong>{done[status]}</strong><span className="queue-loan">by {audit[0]?.user_name} · {audit[0] && new Date(audit[0].created_at * 1000).toLocaleString()}</span></div>
      ) : (
        <div className="actions">
          <input className="note" id="decision-note" placeholder="Add a note for the file (required to reject or send back)" value={note} onChange={(e) => setNote(e.target.value)} />
          <button className="btn btn-bad" disabled={processing || !note.trim()} title={!note.trim() ? "Add a note first" : undefined} onClick={() => decide("send_back")}>Send back for documents</button>
          <button className="btn btn-ghost" disabled={processing || !note.trim()} title={!note.trim() ? "Add a note first" : undefined} onClick={() => decide("reject")}>Reject</button>
          <button className="btn btn-primary" disabled={approveBlocked} title={unresolved > 0 ? "Verify flagged fields first" : processing ? "Wait for processing to finish" : undefined} onClick={() => decide("approve")}>Approve</button>
        </div>
      )}
      {error && <p className="form-error" role="alert" style={{ padding: "0 16px 10px" }}>{error}</p>}
      <div className="audit" style={{ flexDirection: "column", alignItems: "flex-start" }}>
        <span>Every decision and field edit is written to the audit log with reviewer and time.</span>
        {audit.slice(0, 5).map((e) => (
          <span key={e.id} className="tabular">{new Date(e.created_at * 1000).toLocaleTimeString()} · {e.user_name} · {e.action.replace("_", " ")}{e.note ? ` — ${e.note}` : ""}</span>
        ))}
      </div>
    </>
  );
}
```

- [ ] **Step 4: Workspace page** — replace `frontend/src/pages/Workspace.tsx`:

```tsx
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api, type CaseDetail, type CaseSummary, type User } from "../api";
import { moneyWhole } from "../format";
import { useCaseEvents } from "../useCaseEvents";
import Queue from "../components/Queue";
import NewCase from "../components/NewCase";
import SourceDocs from "../components/SourceDocs";
import Fields from "../components/Fields";
import Dti from "../components/Dti";
import Confidence from "../components/Confidence";
import Decision from "../components/Decision";

const statusPill: Record<string, [string, string]> = {
  processing: ["pill-info", "Processing"], needs_review: ["pill-warn", "Needs review"], ready: ["pill-good", "Ready for decision"],
  approved: ["pill-good", "Approved"], rejected: ["pill-bad", "Rejected"], sent_back: ["pill-warn", "Sent back"],
};

export default function Workspace({ user, onSignedOut }: { user: User; onSignedOut: () => void }) {
  const { caseId } = useParams();
  const navigate = useNavigate();
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [detail, setDetail] = useState<CaseDetail | null>(null);
  const [docId, setDocId] = useState<string>();
  const [creating, setCreating] = useState(false);

  const loadCases = useCallback(() => api.listCases().then(setCases), []);
  const loadDetail = useCallback(() => {
    if (caseId) api.getCase(caseId).then(setDetail).catch(() => setDetail(null));
  }, [caseId]);

  useEffect(() => { loadCases(); }, [loadCases]);
  useEffect(() => { setDetail(null); setDocId(undefined); loadDetail(); }, [loadDetail]);
  useCaseEvents(caseId, () => { loadDetail(); loadCases(); });
  useEffect(() => {
    if (detail?.case.status !== "processing") return;
    const t = setInterval(loadDetail, 5000);
    return () => clearInterval(t);
  }, [detail?.case.status, loadDetail]);

  const docs = useMemo(() => (detail?.documents ?? []).filter((d) => d.status !== "superseded"), [detail]);
  const selected = docs.find((d) => d.id === docId) ?? docs.find((d) => d.fields.some((f) => f.flagged && !f.edited)) ?? docs[0];
  const unresolved = docs.reduce((n, d) => n + d.fields.filter((f) => f.flagged && !f.edited).length, 0);
  const locked = !!detail && ["approved", "rejected", "sent_back"].includes(detail.case.status);
  const judgments = [...(selected?.judgments ?? []), ...(detail?.case_judgments ?? [])];

  function applyDetail(d: CaseDetail) { setDetail(d); loadCases(); }

  return (
    <>
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none"><path d="M3 12c3-4 6-4 9 0s6 4 9 0" stroke="white" strokeWidth="2.4" strokeLinecap="round" /></svg>
          </span>
          Harbor Underwriting
        </div>
        <div className="topbar-meta">
          <span className="pill pill-info">{cases.filter((c) => c.status === "needs_review").length} need review</span>
          <div className="reviewer">
            <span className="reviewer-avatar">{user.name.split(" ").map((p) => p[0]).join("")}</span>
            {user.name}, {user.title}
          </div>
          <button className="btn btn-ghost" onClick={() => api.logout().then(onSignedOut)}>Sign out</button>
        </div>
      </header>
      <div className="app">
        <Queue cases={cases} selected={caseId} onSelect={(id) => { setCreating(false); navigate(`/cases/${id}`); }} onNew={() => setCreating(true)} />
        <main className="main">
          {creating && <NewCase onCancel={() => setCreating(false)} onCreated={(c) => { setCreating(false); loadCases(); navigate(`/cases/${c.id}`); }} />}
          {!creating && !caseId && <p className="empty">Select a case from the queue, or create a new one.</p>}
          {!creating && detail && (
            <>
              <div className="case-head">
                <div>
                  <h1>{detail.case.borrower_name}</h1>
                  <div className="case-sub">
                    <span><span className="m-label">Loan #</span> <span className="m-val">{detail.case.loan_number}</span></span>
                    <span className="m-val">{detail.case.loan_product}</span>
                    <span><span className="m-label">Requested</span> <span className="m-val tabular">{moneyWhole(detail.case.requested_amount)}</span></span>
                  </div>
                </div>
                <span className={`pill ${statusPill[detail.case.status][0]}`}>{statusPill[detail.case.status][1]}</span>
              </div>
              <div className="columns">
                <SourceDocs caseId={detail.case.id} docs={docs} selected={selected} onSelect={setDocId} locked={locked} onChanged={loadDetail} />
                <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
                  <Fields doc={selected} locked={locked} onSaved={applyDetail} />
                  <Dti a={detail.assessment} />
                  <section className="card">
                    <Confidence judgments={judgments} a={detail.assessment} />
                    <Decision detail={detail} unresolved={unresolved} onDecided={applyDetail} />
                  </section>
                </div>
              </div>
            </>
          )}
        </main>
      </div>
    </>
  );
}
```

- [ ] **Step 5: Build and manual check**

Run: `cd frontend && npm run build`
Expected: exit 0.

Manual: terminal A `cd backend && PIPELINE_MODE=fake DATA_DIR=$(mktemp -d) go run ./cmd/server`; terminal B `cd frontend && npm run dev`; open http://localhost:5173, sign in (`maya@harbor.test` / `harbor-demo`), create a case, add any five PDFs named `w2.pdf`, `form-1040.pdf`, `form-1003.pdf`, `pay-stub.pdf`, `bank-statement-lowq.png` (any valid PDF/PNG content; fake mode keys off names). Confirm: live stage row updates, ending balance is an input, Approve disabled, editing it enables Approve, decision locks the case. Compare against `docs/superpowers/specs/case-review-mockup.html` side by side at 1360px and at 390px width; fix visual drift in one batch.

- [ ] **Step 6: Commit**

```bash
git add frontend/src
git commit -m "feat(frontend): review workspace with live status, editable flags, DTI, confidence, decisions"
```

---

### Task 13: Serve the built frontend from Go (single localhost origin for demo)

**Files:**
- Modify: `backend/internal/httpapi/server.go`, `backend/internal/config/config.go`, `backend/cmd/server/main.go`
- Test: `backend/internal/httpapi/server_test.go`

**Interfaces:**
- Produces: `Options.StaticDir string`; `config.Config.StaticDir` from `STATIC_DIR` (default `../frontend/dist`); non-`/api/` GET requests serve files from `StaticDir`, falling back to `index.html` for client routes.

- [ ] **Step 1: Failing test** — append to `server_test.go`:

```go
func TestServesSPAFallback(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=root>"), 0o644)
	srv := httptest.NewServer(New(s, Options{StaticDir: dir}).Handler())
	defer srv.Close()
	res, _ := http.Get(srv.URL + "/cases/abc")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "<div id=root>" {
		t.Fatalf("%d %q", res.StatusCode, body)
	}
	if res, _ := http.Get(srv.URL + "/api/nope"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("api 404 expected, got %d", res.StatusCode)
	}
}
```
(imports `io`, `os`).

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/httpapi/ -run SPA`
Expected: FAIL — `unknown field StaticDir`.

- [ ] **Step 3: Implement** — add `StaticDir string` to `Options`; at the end of `routes()`:

```go
	if s.opts.StaticDir != "" {
		files := http.FileServer(http.Dir(s.opts.StaticDir))
		s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			if _, err := os.Stat(filepath.Join(s.opts.StaticDir, filepath.Clean(r.URL.Path))); err != nil {
				http.ServeFile(w, r, filepath.Join(s.opts.StaticDir, "index.html"))
				return
			}
			files.ServeHTTP(w, r)
		})
	}
```
(imports `os`, `path/filepath`, `strings`). Add `StaticDir: env("STATIC_DIR", filepath.Join("..", "frontend", "dist"))` to config and pass `StaticDir: cfg.StaticDir` in `main.go`.

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend
git commit -m "feat(backend): serve built frontend with SPA fallback"
```

---

### Task 14: ego-browser end-to-end (fake pipeline)

E2E runs through ego-browser (`ego-browser nodejs`, custom API, not Playwright; read `.pi/skills/ego-browser/SKILL.md` and `references/api.md` before writing). It drives the built UI served by the Go binary on one origin, and leaves a repeatable artifact per run.

**Files:**
- Create: `frontend/e2e/run.sh` (launch, run, teardown), `frontend/e2e/review.ego.mjs` (browser script), `frontend/e2e/fixtures/` (tiny files)
- Modify: `frontend/package.json` (script `e2e`)

**Interfaces:**
- Consumes: built frontend + Go server in fake mode (Task 13 single origin).
- Produces: `artifacts/e2e/<git describe --tags --always>/` containing `step-NN-<name>.png`, `summary.txt` (PASS/FAIL per check, final `Result:` line), `results.json` (`[{step, check, ok, detail}]`). Exit code non-zero on any FAIL.

- [ ] **Step 1: Fixtures**

```bash
cd frontend
mkdir -p e2e/fixtures
for n in w2-2025 form-1040 form-1003 pay-stub; do printf '%%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%%%EOF\n' > e2e/fixtures/$n.pdf; done
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82' > e2e/fixtures/bank-statement-lowq.png
npm pkg set scripts.e2e="bash e2e/run.sh"
```

- [ ] **Step 2: Runner** — `frontend/e2e/run.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
port=18097
if lsof -nP -iTCP:$port -sTCP:LISTEN -t >/dev/null 2>&1; then echo "port $port busy" >&2; exit 1; fi
scratch=$(mktemp -d)
pid=
cleanup() { [[ -n "$pid" ]] && { kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; }; trash "$scratch"; }
trap cleanup EXIT
(cd "$root/frontend" && npm run build >/dev/null)
(cd "$root/backend" && go build -o "$scratch/harbor" ./cmd/server)
PORT=$port DATA_DIR="$scratch/data" PIPELINE_MODE=fake STATIC_DIR="$root/frontend/dist" "$scratch/harbor" > "$scratch/server.log" 2>&1 &
pid=$!
for _ in $(seq 100); do curl -fs -o /dev/null "http://127.0.0.1:$port/api/health" && break; sleep 0.1; done
artifacts="$root/artifacts/e2e/$(cd "$root" && git describe --tags --always)"
mkdir -p "$artifacts"
E2E_BASE_URL="http://127.0.0.1:$port" E2E_FIXTURES="$root/frontend/e2e/fixtures" E2E_ARTIFACTS="$artifacts" \
  ego-browser nodejs < "$root/frontend/e2e/review.ego.mjs"
```
`ego-browser nodejs` reads the script from stdin only (it rejects a file argument). If `process.env` is not visible inside the ego runtime, have `run.sh` write the three values into `$scratch/e2e-env.json` and read that path from a fixed location instead; record the choice as a deviation.

- [ ] **Step 3: Browser script** — `frontend/e2e/review.ego.mjs`:

```js
const fs = await import("node:fs/promises");
const path = await import("node:path");
const base = process.env.E2E_BASE_URL;
const fixtures = process.env.E2E_FIXTURES;
const out = process.env.E2E_ARTIFACTS;
const results = [];
let step = 0;
const task = await taskSpace("harbor e2e review flow");
const page = task.page("p1");

async function shot(name) {
  step += 1;
  const file = path.join(out, `step-${String(step).padStart(2, "0")}-${name}.png`);
  try { await page.screenshot({ path: file }); }
  catch { await page.waitForTimeout(500); await page.screenshot({ path: file }); }
  return file;
}
async function check(name, fn) {
  try { const detail = await fn(); results.push({ step, check: name, ok: true, detail: detail ?? "" }); }
  catch (e) { results.push({ step, check: name, ok: false, detail: String(e?.message ?? e) }); }
}
const disabled = (label) => page.evaluate((l) => {
  const b = [...document.querySelectorAll("button")].find((x) => x.textContent.trim() === l);
  if (!b) throw new Error(`button ${l} not found`);
  return b.disabled;
}, label);

try {
  await page.goto(base);
  await shot("warmup");

  await check("wrong password shows the server message", async () => {
    await page.fill("#password", "wrong");
    await page.click("loc=role:button[name='Sign in']");
    await page.waitForSelector("text=email or password is incorrect", { timeout: 5000 });
    await shot("login-error");
  });

  await check("reviewer signs in", async () => {
    await page.fill("#password", "harbor-demo");
    await page.click("loc=role:button[name='Sign in']");
    await page.waitForSelector("text=Select a case from the queue", { timeout: 5000 });
    await shot("signed-in");
  });

  await check("create case", async () => {
    await page.click("loc=role:button[name='New case']");
    await page.fill("#borrower", "Jordan Alvarez");
    await page.fill("#loan-number", `HB-${Date.now()}`);
    await page.fill("#amount", "410000");
    await page.click("loc=role:button[name='Create case']");
    await page.waitForSelector("loc=role:heading[name='Jordan Alvarez']", { timeout: 5000 });
    await shot("case-created");
  });

  await check("five documents process and recommendation shows", async () => {
    const files = ["w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", "bank-statement-lowq.png"].map((f) => path.join(fixtures, f));
    await page.setInputFiles("[data-testid=file-input]", files);
    await page.waitForSelector("text=5 of 5 processed", { timeout: 20000 });
    await page.waitForSelector("text=Recommendation: needs review", { timeout: 10000 });
    await shot("processed");
  });

  await check("approve blocked while a field is flagged", async () => {
    if (!(await disabled("Approve"))) throw new Error("Approve was enabled with an unresolved flag");
  });

  await check("reviewer corrects the flagged field", async () => {
    const sel = '[aria-label="Ending balance, flagged for review"]';
    await page.fill(sel, "18432");
    await page.press(sel, "Enter");
    await page.waitForSelector("text=Edited by reviewer", { timeout: 5000 });
    if (await disabled("Approve")) throw new Error("Approve still disabled after correction");
    await shot("field-corrected");
  });

  await check("approve records the decision and audit trail", async () => {
    await page.fill("#decision-note", "Verified ending balance against source scan");
    await page.click("loc=role:button[name='Approve']");
    await page.waitForFunction(() => document.querySelector(".actions strong")?.textContent === "Approved", undefined, { timeout: 5000 });
    await page.waitForSelector("text=/field edited/i", { timeout: 5000 }).catch(async () => {
      const txt = await page.evaluate(() => document.querySelector(".audit")?.textContent ?? "");
      if (!/field edited/i.test(txt)) throw new Error(`audit line missing field edit: ${txt}`);
    });
    const inputs = await page.evaluate(() => document.querySelectorAll('[aria-label$="flagged for review"]').length);
    if (inputs !== 0) throw new Error("fields still editable after decision");
    await shot("approved");
  });
} finally {
  const failed = results.filter((r) => !r.ok).length;
  const lines = results.map((r) => `${r.ok ? "PASS" : "FAIL"} ${r.check}${r.ok ? "" : ` — ${r.detail}`}`);
  lines.push(`Result: ${failed ? "FAIL" : "PASS"}; evidence: ${out}`);
  await fs.writeFile(path.join(out, "summary.txt"), lines.join("\n") + "\n");
  await fs.writeFile(path.join(out, "results.json"), JSON.stringify(results, null, 2));
  console.log(lines.join("\n"));
  await task.finish({ keep: [] });
  if (failed) process.exitCode = 1;
}
```
Selector forms (`loc=role:`, `text=`, CSS) and methods (`fill`, `click`, `press`, `setInputFiles`, `waitForSelector`, `waitForFunction`, `evaluate`, `screenshot`) are from the ego-browser skill. The `text=/field edited/i` regex form may not be supported; the fallback reads `.audit` text directly. If any other call is rejected, fix it against `references/api.md`, never by importing Playwright. The first `screenshot` can time out on a cold browser; `shot()` retries once.

- [ ] **Step 4: Run**

Run: `cd frontend && npm run e2e`
Expected: `summary.txt` shows 7 PASS and `Result: PASS`; 7 screenshots saved; after the run `ego-browser nodejs -e 'console.log(await listTaskSpaces())'` prints `[]` and port 18097 is free. Open two screenshots and confirm they show the real app, not a blank page.

- [ ] **Step 5: Commit**

```bash
git add frontend/e2e frontend/package.json
git commit -m "test(e2e): review flow through ego-browser with screenshot artifacts"
```

---

### Task 15: Synthetic documents and eval fixtures

**Files:**
- Create: `backend/cmd/gendata/main.go`, `backend/cmd/gendata/scenarios.go`
- Test: `backend/cmd/gendata/main_test.go`
- Output (committed): `testdata/documents/<scenario>/*.pdf|png`, `testdata/fixtures/<scenario>.json`

**Interfaces:**
- Produces fixture JSON (consumed by Task 16):
```json
{
  "id": "boundary",
  "description": "...",
  "documents": [{"file": "documents/boundary/bank-statement.pdf", "expected_doc_type": "bank_statement", "expected_fields": {"ending_balance": 18482.0}}],
  "expected": {"dti": 0.4281, "recommendation": "needs_review", "should_flag_low_confidence": false}
}
```
File paths are relative to `testdata/`. File names deliberately avoid the words the fake classifier keys on? No — names match the fake so the same set also drives fake-mode demos; the real pipeline never sees file names (LlamaParse reads content).

Scenarios (all names fictional, all employers/banks fictional except generic words; every page prints "SYNTHETIC TEST DOCUMENT — NOT A REAL RECORD" in the footer):
1. `standard` — all 5 docs, income 7200, debt 2100 → DTI 0.2917, `eligible`, no flag.
2. `boundary` — all 5, income 7200, debt 3082 → 0.4281, `needs_review`, no flag.
3. `missing_docs` — no Form 1040, income 7200, debt 2100 → 0.2917, `needs_review`.
4. `low_quality_scan` — all 5; bank statement rendered as a noisy low-res image embedded in the PDF (`bank-statement-lowq.pdf`) → DTI 0.2917, `needs_review`, `should_flag_low_confidence: true`.
5. `unsupported_doc` — all 5 plus `drivers-license.pdf` → `other`; `needs_review`.

- [ ] **Step 1: Dependencies**

```bash
cd backend && go get github.com/go-pdf/fpdf@latest golang.org/x/image@latest
```

- [ ] **Step 2: Failing test** — `backend/cmd/gendata/main_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateWritesConsistentFixtures(t *testing.T) {
	out := t.TempDir()
	if err := generate(out); err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenarios {
		raw, err := os.ReadFile(filepath.Join(out, "fixtures", sc.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var fx fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatal(err)
		}
		if len(fx.Documents) != len(sc.Docs) {
			t.Fatalf("%s: %d docs", sc.ID, len(fx.Documents))
		}
		for _, d := range fx.Documents {
			info, err := os.Stat(filepath.Join(out, d.File))
			if err != nil || info.Size() < 500 {
				t.Fatalf("%s: bad file %s (%v)", sc.ID, d.File, err)
			}
		}
	}
}

func TestExpectedDTIMatchesInputs(t *testing.T) {
	for _, sc := range scenarios {
		if sc.Expected.DTI == nil {
			continue
		}
		got := round4(sc.Debt / sc.Income)
		if got != *sc.Expected.DTI {
			t.Fatalf("%s: expected %v computed %v", sc.ID, *sc.Expected.DTI, got)
		}
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `cd backend && go test ./cmd/gendata/`
Expected: FAIL — undefined `generate`.

- [ ] **Step 4: Implement scenarios** — `backend/cmd/gendata/scenarios.go`:

```go
package main

import "math"

type docSpec struct {
	Name   string
	Type   string
	Fields map[string]any
	LowQ   bool
}

type expected struct {
	DTI            *float64 `json:"dti"`
	Recommendation string   `json:"recommendation"`
	ShouldFlag     bool     `json:"should_flag_low_confidence"`
}

type scenario struct {
	ID, Description string
	Income, Debt    float64
	Docs            []docSpec
	Expected        expected
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
func ptr(v float64) *float64   { return &v }

func baseDocs(debt float64) []docSpec {
	return []docSpec{
		{Name: "w2-2025.pdf", Type: "w2", Fields: map[string]any{"employer_name": "Northwind Logistics LLC", "tax_year": 2025.0, "box1_wages": 86400.0, "box2_fed_tax": 11230.0}},
		{Name: "form-1040.pdf", Type: "form_1040", Fields: map[string]any{"tax_year": 2025.0, "adjusted_gross_income": 84900.0, "taxable_income": 70300.0, "total_tax": 9420.0}},
		{Name: "form-1003.pdf", Type: "form_1003", Fields: map[string]any{"borrower_name": "Jordan Alvarez", "property_address": "118 Linden Ave, Hoboken NJ 07030", "loan_amount": 410000.0, "loan_purpose": "Purchase", "stated_monthly_income": 7200.0}},
		{Name: "pay-stub.pdf", Type: "pay_stub", Fields: map[string]any{"employer_name": "Northwind Logistics LLC", "pay_period_end": "2026-08-31", "gross_pay": 3600.0, "ytd_gross": 57600.0, "monthly_income": 7200.0}},
		{Name: "bank-statement.pdf", Type: "bank_statement", Fields: map[string]any{"bank_name": "Harborview Community Bank", "account_last4": "4471", "statement_period": "2026-08", "beginning_balance": 14210.55, "ending_balance": 18482.0, "total_deposits": 7412.2, "monthly_debt": debt, "nsf_count": 0.0, "bnpl_hits": 2.0}},
	}
}

var scenarios = func() []scenario {
	standard := scenario{ID: "standard", Description: "Complete file, comfortable DTI", Income: 7200, Debt: 2100, Docs: baseDocs(2100),
		Expected: expected{DTI: ptr(0.2917), Recommendation: "eligible"}}
	boundary := scenario{ID: "boundary", Description: "Complete file, DTI just under the 43% QM threshold", Income: 7200, Debt: 3082, Docs: baseDocs(3082),
		Expected: expected{DTI: ptr(0.4281), Recommendation: "needs_review"}}
	missing := scenario{ID: "missing_docs", Description: "Form 1040 not uploaded", Income: 7200, Debt: 2100,
		Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review"}}
	for _, d := range baseDocs(2100) {
		if d.Type != "form_1040" {
			missing.Docs = append(missing.Docs, d)
		}
	}
	lowq := scenario{ID: "low_quality_scan", Description: "Bank statement is a noisy low-resolution scan", Income: 7200, Debt: 2100,
		Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review", ShouldFlag: true}}
	for _, d := range baseDocs(2100) {
		if d.Type == "bank_statement" {
			d.Name, d.LowQ = "bank-statement-lowq.pdf", true
		}
		lowq.Docs = append(lowq.Docs, d)
	}
	unsupported := scenario{ID: "unsupported_doc", Description: "A driver's license was uploaded alongside the file", Income: 7200, Debt: 2100,
		Docs: append(baseDocs(2100), docSpec{Name: "drivers-license.pdf", Type: "other", Fields: map[string]any{}}),
		Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review"}}
	return []scenario{standard, boundary, missing, lowq, unsupported}
}()
```

W-2 86400/12 = 7200 and pay stub 3600 semimonthly × 2 = 7200 keep income consistent; YTD 57600 = 16 semimonthly periods through Aug 31.

- [ ] **Step 5: Implement generator** — `backend/cmd/gendata/main.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"tidalwave/backend/internal/schemas"
)

type fixtureDoc struct {
	File            string         `json:"file"`
	ExpectedDocType string         `json:"expected_doc_type"`
	ExpectedFields  map[string]any `json:"expected_fields"`
}

type fixture struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	Documents   []fixtureDoc `json:"documents"`
	Expected    expected     `json:"expected"`
}

const footer = "SYNTHETIC TEST DOCUMENT - NOT A REAL RECORD"

var titles = map[string]string{
	"w2": "Form W-2  Wage and Tax Statement  2025", "form_1040": "Form 1040  U.S. Individual Income Tax Return  2025",
	"form_1003": "Uniform Residential Loan Application (Form 1003)", "pay_stub": "Earnings Statement",
	"bank_statement": "Checking Account Statement", "other": "STATE OF NEW JERSEY  DRIVER LICENSE",
}

func lines(d docSpec) []string {
	out := []string{titles[d.Type], ""}
	if d.Type == "other" {
		return append(out, "Name: ALVAREZ, JORDAN", "DOB: 04/11/1990", "Class: D", "Expires: 04/11/2030", "ID: S1234 56789 01234")
	}
	for _, spec := range schemas.Registry[d.Type] {
		v := d.Fields[spec.Key]
		switch spec.Kind {
		case schemas.KindNumber:
			out = append(out, fmt.Sprintf("%-34s $%s", spec.Label, commas(v.(float64))))
		case schemas.KindInt:
			out = append(out, fmt.Sprintf("%-34s %d", spec.Label, int(v.(float64))))
		default:
			out = append(out, fmt.Sprintf("%-34s %s", spec.Label, v))
		}
	}
	return out
}

func commas(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	intPart, dec := s[:len(s)-3], s[len(s)-3:]
	var b bytes.Buffer
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String() + dec
}

func textPDF(path string, body []string) error {
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.AddPage()
	pdf.SetFont("Courier", "B", 13)
	pdf.CellFormat(0, 10, body[0], "", 1, "L", false, 0, "")
	pdf.SetFont("Courier", "", 11)
	for _, l := range body[1:] {
		pdf.CellFormat(0, 7, l, "", 1, "L", false, 0, "")
	}
	pdf.SetY(-20)
	pdf.SetFont("Courier", "I", 8)
	pdf.CellFormat(0, 6, footer, "", 0, "C", false, 0, "")
	return pdf.OutputFileAndClose(path)
}

func noisyScanPDF(path string, body []string) error {
	const w, h = 520, 300
	img := image.NewGray(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Gray{Y: 225}}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Src: &image.Uniform{color.Gray{Y: 70}}, Face: basicfont.Face7x13}
	for i, l := range append(body, "", footer) {
		d.Dot = fixed.P(12, 18+i*15)
		d.DrawString(l)
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < w*h/9; i++ {
		img.SetGray(rng.Intn(w), rng.Intn(h), color.Gray{Y: uint8(90 + rng.Intn(140))})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.AddPage()
	pdf.RegisterImageOptionsReader("scan", fpdf.ImageOptions{ImageType: "PNG"}, &buf)
	pdf.ImageOptions("scan", 10, 10, 195, 0, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	return pdf.OutputFileAndClose(path)
}

func generate(out string) error {
	for _, sc := range scenarios {
		dir := filepath.Join(out, "documents", sc.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		fx := fixture{ID: sc.ID, Description: sc.Description, Expected: sc.Expected}
		for _, d := range sc.Docs {
			path := filepath.Join(dir, d.Name)
			var err error
			if d.LowQ {
				err = noisyScanPDF(path, lines(d))
			} else {
				err = textPDF(path, lines(d))
			}
			if err != nil {
				return fmt.Errorf("%s/%s: %w", sc.ID, d.Name, err)
			}
			fx.Documents = append(fx.Documents, fixtureDoc{File: filepath.Join("documents", sc.ID, d.Name), ExpectedDocType: d.Type, ExpectedFields: d.Fields})
		}
		sort.Slice(fx.Documents, func(i, j int) bool { return fx.Documents[i].File < fx.Documents[j].File })
		if err := os.MkdirAll(filepath.Join(out, "fixtures"), 0o755); err != nil {
			return err
		}
		raw, _ := json.MarshalIndent(fx, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "fixtures", sc.ID+".json"), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	out := flag.String("out", filepath.Join("..", "testdata"), "output directory")
	flag.Parse()
	if err := generate(*out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d scenarios to %s", len(scenarios), *out)
}
```

- [ ] **Step 6: Run tests and generate**

Run: `cd backend && go test ./cmd/gendata/ && go run ./cmd/gendata`
Expected: tests PASS; `wrote 5 scenarios to ../testdata`. Open `testdata/documents/low_quality_scan/bank-statement-lowq.pdf` and confirm digits are hard but possible to read.

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/gendata backend/go.mod backend/go.sum testdata
git commit -m "feat(testdata): synthetic mortgage documents and eval fixtures for five scenarios"
```

---

### Task 16: Eval harness

**Files:**
- Create: `backend/internal/evalscore/score.go`, `backend/cmd/eval/main.go`
- Test: `backend/internal/evalscore/score_test.go`
- Output: `eval/report.json` (gitignored), console table

**Interfaces:**
- Consumes: fixtures from Task 15 (`fixture`/`fixtureDoc`/`expected` shapes — redeclared in `evalscore` since `cmd/gendata` is a main package), `pipeline.NewRunner`, `clients.RealStages`, `fake.Stages`, `store`.
- Produces:
```go
package evalscore
type Fixture struct { ID, Description string; Documents []FixtureDoc; Expected Expected }   // json tags as in Task 15
type FixtureDoc struct { File, ExpectedDocType string; ExpectedFields map[string]any }
type Expected struct { DTI *float64; Recommendation string; ShouldFlag bool }
type DocResult struct { File, DocType string; Fields map[string]any; Status string }
type CaseResult struct { FixtureID string; Docs []DocResult; DTI *float64; Recommendation string; Flagged bool }
type Layer struct { Correct, Total int; Rate float64 }
type Miss struct { Fixture, File, What, Want, Got string }
type Report struct { Classification, Extraction, EndToEnd, Calibration Layer; Misses []Miss; Session string }
func Score(fixtures []Fixture, results []CaseResult) Report
```
Scoring rules: classification = per document, `DocType == ExpectedDocType`. Extraction = per expected field of documents whose expected type is not `other`; numbers equal within 1.0, strings equal case-insensitively after trimming. End-to-end = per fixture, recommendation equal AND (both DTI nil OR |ΔDTI| ≤ 0.005). Calibration = per fixture, `Flagged == ShouldFlag` where `Flagged` means any extracted field flagged or any judgment < 0.8 on a document the fixture marks low-quality (runner computes: any flagged field in the case OR any `ocr_quality` < 0.8). Rates are `Correct/Total` (0 when Total is 0).

- [ ] **Step 1: Failing test** — `backend/internal/evalscore/score_test.go`:

```go
package evalscore

import "testing"

func f(v float64) *float64 { return &v }

func TestScore(t *testing.T) {
	fixtures := []Fixture{{
		ID: "boundary",
		Documents: []FixtureDoc{
			{File: "a.pdf", ExpectedDocType: "bank_statement", ExpectedFields: map[string]any{"ending_balance": 18482.0, "bank_name": "Harborview Community Bank"}},
			{File: "b.pdf", ExpectedDocType: "other", ExpectedFields: map[string]any{}},
		},
		Expected: Expected{DTI: f(0.4281), Recommendation: "needs_review", ShouldFlag: false},
	}}
	results := []CaseResult{{
		FixtureID: "boundary",
		Docs: []DocResult{
			{File: "a.pdf", DocType: "bank_statement", Fields: map[string]any{"ending_balance": 18482.4, "bank_name": " harborview community bank"}},
			{File: "b.pdf", DocType: "pay_stub"},
		},
		DTI: f(0.43), Recommendation: "needs_review", Flagged: true,
	}}
	r := Score(fixtures, results)
	if r.Classification != (Layer{1, 2, 0.5}) {
		t.Fatalf("classification %+v", r.Classification)
	}
	if r.Extraction != (Layer{2, 2, 1}) {
		t.Fatalf("extraction %+v", r.Extraction)
	}
	if r.EndToEnd != (Layer{1, 1, 1}) {
		t.Fatalf("end-to-end %+v", r.EndToEnd)
	}
	if r.Calibration != (Layer{0, 1, 0}) {
		t.Fatalf("calibration %+v", r.Calibration)
	}
	if len(r.Misses) != 2 {
		t.Fatalf("misses %+v", r.Misses)
	}
}

func TestScoreMissingResultCountsAsWrong(t *testing.T) {
	r := Score([]Fixture{{ID: "x", Documents: []FixtureDoc{{File: "a", ExpectedDocType: "w2", ExpectedFields: map[string]any{"tax_year": 2025.0}}},
		Expected: Expected{Recommendation: "eligible"}}}, nil)
	if r.Classification.Total != 1 || r.Classification.Correct != 0 || r.EndToEnd.Correct != 0 || r.Extraction.Total != 1 {
		t.Fatalf("%+v", r)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd backend && go test ./internal/evalscore/`
Expected: FAIL — package missing.

- [ ] **Step 3: Implement** — `backend/internal/evalscore/score.go`:

```go
package evalscore

import (
	"fmt"
	"math"
	"strings"
)

type Expected struct {
	DTI            *float64 `json:"dti"`
	Recommendation string   `json:"recommendation"`
	ShouldFlag     bool     `json:"should_flag_low_confidence"`
}

type FixtureDoc struct {
	File            string         `json:"file"`
	ExpectedDocType string         `json:"expected_doc_type"`
	ExpectedFields  map[string]any `json:"expected_fields"`
}

type Fixture struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	Documents   []FixtureDoc `json:"documents"`
	Expected    Expected     `json:"expected"`
}

type DocResult struct {
	File    string         `json:"file"`
	DocType string         `json:"doc_type"`
	Fields  map[string]any `json:"fields"`
	Status  string         `json:"status"`
}

type CaseResult struct {
	FixtureID      string      `json:"fixture_id"`
	Docs           []DocResult `json:"docs"`
	DTI            *float64    `json:"dti"`
	Recommendation string      `json:"recommendation"`
	Flagged        bool        `json:"flagged"`
}

type Layer struct {
	Correct int     `json:"correct"`
	Total   int     `json:"total"`
	Rate    float64 `json:"rate"`
}

type Miss struct {
	Fixture string `json:"fixture"`
	File    string `json:"file,omitempty"`
	What    string `json:"what"`
	Want    string `json:"want"`
	Got     string `json:"got"`
}

type Report struct {
	Session        string       `json:"session"`
	Classification Layer        `json:"classification"`
	Extraction     Layer        `json:"extraction"`
	EndToEnd       Layer        `json:"end_to_end"`
	Calibration    Layer        `json:"calibration"`
	Misses         []Miss       `json:"misses"`
	Results        []CaseResult `json:"results"`
}

func (l *Layer) add(ok bool) {
	l.Total++
	if ok {
		l.Correct++
	}
	l.Rate = float64(l.Correct) / float64(l.Total)
}

func sameValue(want, got any) bool {
	if w, ok := want.(float64); ok {
		g, ok := got.(float64)
		return ok && math.Abs(w-g) <= 1.0
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(want)), strings.TrimSpace(fmt.Sprint(got)))
}

func fmtDTI(d *float64) string {
	if d == nil {
		return "none"
	}
	return fmt.Sprintf("%.4f", *d)
}

func Score(fixtures []Fixture, results []CaseResult) Report {
	byID := map[string]CaseResult{}
	for _, r := range results {
		byID[r.FixtureID] = r
	}
	rep := Report{Misses: []Miss{}, Results: results}
	for _, fx := range fixtures {
		res := byID[fx.ID]
		docs := map[string]DocResult{}
		for _, d := range res.Docs {
			docs[d.File] = d
		}
		for _, fd := range fx.Documents {
			got := docs[fd.File]
			ok := got.DocType == fd.ExpectedDocType
			rep.Classification.add(ok)
			if !ok {
				rep.Misses = append(rep.Misses, Miss{fx.ID, fd.File, "doc_type", fd.ExpectedDocType, got.DocType})
			}
			if fd.ExpectedDocType == "other" {
				continue
			}
			for k, want := range fd.ExpectedFields {
				v, present := got.Fields[k]
				ok := present && sameValue(want, v)
				rep.Extraction.add(ok)
				if !ok {
					rep.Misses = append(rep.Misses, Miss{fx.ID, fd.File, "field " + k, fmt.Sprint(want), fmt.Sprint(v)})
				}
			}
		}
		dtiOK := (fx.Expected.DTI == nil && res.DTI == nil) ||
			(fx.Expected.DTI != nil && res.DTI != nil && math.Abs(*fx.Expected.DTI-*res.DTI) <= 0.005)
		e2e := res.Recommendation == fx.Expected.Recommendation && dtiOK
		rep.EndToEnd.add(e2e)
		if !e2e {
			rep.Misses = append(rep.Misses, Miss{Fixture: fx.ID, What: "recommendation/dti",
				Want: fx.Expected.Recommendation + " @ " + fmtDTI(fx.Expected.DTI), Got: res.Recommendation + " @ " + fmtDTI(res.DTI)})
		}
		cal := res.Flagged == fx.Expected.ShouldFlag
		rep.Calibration.add(cal)
		if !cal {
			rep.Misses = append(rep.Misses, Miss{Fixture: fx.ID, What: "low-confidence flag",
				Want: fmt.Sprint(fx.Expected.ShouldFlag), Got: fmt.Sprint(res.Flagged)})
		}
	}
	return rep
}
```

- [ ] **Step 4: Eval runner** — `backend/cmd/eval/main.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"tidalwave/backend/internal/clients"
	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/evalscore"
	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/store"
)

type nopPublisher struct{}

func (nopPublisher) Publish(pipeline.Event) {}

func main() {
	testdata := flag.String("testdata", filepath.Join("..", "testdata"), "directory with fixtures/ and documents/")
	reportPath := flag.String("report", filepath.Join("..", "eval", "report.json"), "report output")
	useFake := flag.Bool("fake", false, "use fake stages (checks the harness, not the models)")
	flag.Parse()

	cfg := config.Load()
	session := fmt.Sprintf("eval-%d", time.Now().Unix())
	cfg.SpanboxSession = session
	var stages pipeline.Stages
	if *useFake {
		stages = fake.Stages()
	} else {
		var err error
		if stages, err = clients.RealStages(cfg); err != nil {
			log.Fatal(err)
		}
	}

	files, err := filepath.Glob(filepath.Join(*testdata, "fixtures", "*.json"))
	if err != nil || len(files) == 0 {
		log.Fatalf("no fixtures under %s/fixtures (run: go run ./cmd/gendata)", *testdata)
	}
	var fixtures []evalscore.Fixture
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		var fx evalscore.Fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		fixtures = append(fixtures, fx)
	}

	tmp, _ := os.MkdirTemp("", "harbor-eval")
	defer os.RemoveAll(tmp)
	st, err := store.Open(filepath.Join(tmp, "eval.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	runner := pipeline.NewRunner(st, stages, nopPublisher{})

	var results []evalscore.CaseResult
	for _, fx := range fixtures {
		c, _ := st.CreateCase(store.Case{BorrowerName: fx.ID, LoanNumber: fx.ID, LoanProduct: "eval", RequestedAmount: 1})
		fileByDoc := map[string]string{}
		for _, fd := range fx.Documents {
			abs, _ := filepath.Abs(filepath.Join(*testdata, fd.File))
			d, err := st.CreateDocument(store.Document{CaseID: c.ID, FileName: filepath.Base(fd.File), FilePath: abs})
			if err != nil {
				log.Fatal(err)
			}
			fileByDoc[d.ID] = fd.File
			runner.Enqueue(c.ID, d.ID)
		}
		runner.Wait()
		detail, err := st.CaseDetail(c.ID)
		if err != nil {
			log.Fatal(err)
		}
		res := evalscore.CaseResult{FixtureID: fx.ID}
		for _, d := range detail.Documents {
			fields := map[string]any{}
			for _, f := range d.Fields {
				fields[f.Key] = f.Value
				if f.Flagged {
					res.Flagged = true
				}
			}
			for _, j := range d.Judgments {
				if j.Name == "ocr_quality" && j.Score < pipeline.LowConfidence {
					res.Flagged = true
				}
			}
			res.Docs = append(res.Docs, evalscore.DocResult{File: fileByDoc[d.ID], DocType: d.DocType, Fields: fields, Status: d.Status})
		}
		if detail.Assessment != nil {
			res.DTI, res.Recommendation = detail.Assessment.DTI, detail.Assessment.Recommendation
		}
		results = append(results, res)
		log.Printf("%-18s %s", fx.ID, res.Recommendation)
	}

	rep := evalscore.Score(fixtures, results)
	rep.Session = session
	os.MkdirAll(filepath.Dir(*reportPath), 0o755)
	raw, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(*reportPath, raw, 0o644); err != nil {
		log.Fatal(err)
	}
	line := func(name string, l evalscore.Layer) { fmt.Printf("%-28s %3d/%-3d %5.1f%%\n", name, l.Correct, l.Total, l.Rate*100) }
	fmt.Printf("\nspanbox session: %s\n", session)
	line("Document classification", rep.Classification)
	line("Field extraction", rep.Extraction)
	line("End-to-end recommendation", rep.EndToEnd)
	line("Confidence calibration", rep.Calibration)
	for _, m := range rep.Misses {
		fmt.Printf("  miss  %-16s %-32s %-26s want %s, got %s\n", m.Fixture, m.File, m.What, m.Want, m.Got)
	}
	fmt.Printf("\nreport: %s\n", *reportPath)
}
```

Each case uses its own store rows; retry/refresh behavior is identical to the server because it is the same `Runner`.

- [ ] **Step 5: Run**

Run: `cd backend && go test ./internal/evalscore/ && go run ./cmd/eval -fake`
Expected: tests PASS; fake run prints four layers. Fake mode classification should be 100% (file names drive it); fake extraction will miss on values that differ from the canned set (`standard` debt 2100 vs canned 3082, pay stub gross 3600 vs canned 3323.08) — that is expected and proves the scorer catches misses. Do not tune fakes to the fixtures.

Real run (user's keys, optional spanbox): see Task 17 runbook; `go run ./cmd/eval`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/evalscore backend/cmd/eval
git commit -m "feat(eval): four-layer eval harness over synthetic fixtures, spanbox-tagged sessions"
```

---

### Task 17: Runbook and final verification

**Files:**
- Create: `README.md`, `.env.example`
- Test: full suite + e2e + build.

- [ ] **Step 1: Write `.env.example`**

```bash
# Copy to .env (gitignored) and fill in. Never commit real keys.
ANTHROPIC_API_KEY=
LLAMAPARSE_API_KEY=
TYPESAFE_API_KEY=
# Route Claude through spanbox for traces/tokens/cost (leave empty to call Anthropic directly)
ANTHROPIC_BASE_URL=http://localhost:4318/proxy/anthropic
SPANBOX_TOKEN=
PIPELINE_MODE=real
```

- [ ] **Step 2: Write `README.md`**

````markdown
# Harbor Underwriting (demo)

AI-assisted mortgage document review. Uploads are parsed (LlamaParse), classified and scored (TypeSafe Jev), extracted (Claude), then turned into a DTI and a recommendation. A reviewer always makes the decision; every edit and decision is audit-logged.

Synthetic data only. Localhost only.

## Run without API keys (fake pipeline)

```bash
cd frontend && npm install && npm run build && cd ..
cd backend && PIPELINE_MODE=fake go run ./cmd/server
# open http://localhost:8080 — maya@harbor.test / harbor-demo
```
Upload files named like `w2.pdf`, `form-1040.pdf`, `form-1003.pdf`, `pay-stub.pdf`, `bank-statement-lowq.png` (fake mode keys off names).

## Run for real

```bash
cp .env.example .env   # fill keys
# optional observability: start spanbox (github.com/Ray0907/spanbox) on :4318
./spanbox
set -a && source .env && set +a
cd backend && go run ./cmd/gendata          # writes ../testdata
go run ./cmd/server                          # http://localhost:8080
```
Upload the PDFs from `testdata/documents/<scenario>/`.

## Eval

```bash
cd backend && set -a && source ../.env && set +a
go run ./cmd/eval            # real models; prints spanbox session id
go run ./cmd/eval -fake      # harness check only
```
Report: `eval/report.json`. In spanbox, filter traces by the printed `eval-<ts>` session to inspect the exact prompt/completion behind any miss.

## Tests

```bash
cd backend && go test -race ./...
cd frontend && npm run e2e
```

## Scenarios

| id | what it shows |
|---|---|
| standard | complete file, DTI 29.2%, eligible |
| boundary | DTI 42.8% next to the 43% QM line, needs review |
| missing_docs | Form 1040 absent |
| low_quality_scan | noisy bank statement, flagged field must be verified before approve |
| unsupported_doc | driver's license classified as unsupported |
````

- [ ] **Step 3: Full verification**

Run:
```bash
cd backend && go vet ./... && go test -race ./...
cd ../frontend && npm run build && npm run e2e
cd ../backend && go run ./cmd/eval -fake
git status --short
```
Expected: all green; eval prints four layers; `git status` shows only the new README/.env.example (no `.env`, no `data/`, no `node_modules`).

- [ ] **Step 4: Commit**

```bash
git add README.md .env.example
git commit -m "docs: runbook for fake and real modes, eval, tests"
```

---

## Self-Review Notes (for the reviewer of this plan)

- Spec coverage: auth (T2), upload + disk storage (T4), classify/parse/extract/judge (T5, T9), SSE (T6), DTI + recommendation-only (T7, T8), failure + retry (T8), editable fields + server-enforced approve gate + audit (T10), mockup-faithful UI (T11, T12), localhost single origin (T13), e2e (T14), synthetic data (T15), 4-layer eval + spanbox session tagging (T9, T16), runbook (T17).
- Known open decision for the user: server-side refusal fallbacks (`fallbacks: "default"`, beta header) are recommended for `claude-opus-5` but not in this plan because their Go SDK binding was not verified; refusals are handled as a failed `extracting` stage with a clear reason and a retry button.
- LlamaParse v2 upload/status/result paths and `markdown_full` were verified from docs; the host `api.cloud.llamaindex.ai` comes from the v1 curl example and is configurable via `LLAMAPARSE_BASE_URL`.
````
