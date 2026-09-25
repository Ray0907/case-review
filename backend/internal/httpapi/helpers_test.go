package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/sse"
	"tidalwave/backend/internal/store"
)

type fakeRunner struct{ enqueued []string }

func (f *fakeRunner) Enqueue(caseID, docID string)                     { f.enqueued = append(f.enqueued, docID) }
func (f *fakeRunner) Refresh(ctx context.Context, caseID string) error { return nil }
func (f *fakeRunner) Retry(ctx context.Context, docID string) error {
	f.enqueued = append(f.enqueued, docID)
	return nil
}

type testEnv struct {
	url        string
	store      *store.Store
	client     *http.Client
	server     *Server
	runner     *fakeRunner
	broker     *sse.Broker
	realRunner *pipeline.Runner
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h, _ := auth.HashPassword("pw")
	if err := s.EnsureUser("reviewer@casereview.test", "Maya Park", "Senior Underwriter", h); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}
	b := sse.New()
	srv := New(s, Options{UploadDir: t.TempDir(), Runner: fr, Broker: b})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	env := &testEnv{url: ts.URL, store: s, client: &http.Client{Jar: jar}, server: srv, runner: fr, broker: b}
	res := env.do(t, "POST", "/api/auth/login", map[string]string{"email": "reviewer@casereview.test", "password": "pw"})
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

var pngBytes = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R',
	0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89, 0, 0, 0x0d, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82}

func (e *testEnv) createCase(t *testing.T) string {
	t.Helper()
	res := e.do(t, "POST", "/api/cases", map[string]any{"borrower_name": "Jordan Alvarez",
		"loan_number": "HB-20486", "loan_product": "Conventional 30yr fixed", "requested_amount": 410000})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create case %d", res.StatusCode)
	}
	return decode[map[string]any](t, res)["id"].(string)
}

func mustReq(t *testing.T, method, url string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func newTestEnvWithRunner(t *testing.T, stages pipeline.Stages) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	r := pipeline.NewRunner(env.store, stages, env.broker)
	env.server.opts.Runner = r
	env.realRunner = r
	return env
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
