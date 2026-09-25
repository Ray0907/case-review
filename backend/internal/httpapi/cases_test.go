package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

func TestCreateAndListCases(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	list := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases", nil))
	if len(list) != 1 || list[0]["id"] != id || list[0]["status"] != "processing" {
		t.Fatalf("unexpected list %v", list)
	}
}

func TestQueueBlockers(t *testing.T) {
	check := func(t *testing.T, env *testEnv, want string) {
		t.Helper()
		list := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases", nil))
		if len(list) != 1 || list[0]["blocker"] != want {
			t.Fatalf("blocker want %q, list %v", want, list)
		}
	}
	t.Run("empty", func(t *testing.T) { env := newTestEnv(t); env.createCase(t); check(t, env, "5 documents missing") })
	t.Run("one missing", func(t *testing.T) {
		env := newTestEnvWithRunner(t, fake.Stages())
		id := env.createCase(t)
		for _, name := range []string{"w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf"} {
			env.upload(t, id, name, minimalPDF)
		}
		env.realRunner.Wait()
		check(t, env, "1 document missing")
	})
	t.Run("partial", func(t *testing.T) {
		env := newTestEnvWithRunner(t, fake.Stages())
		id := env.createCase(t)
		for _, name := range []string{"w2-2025.pdf", "form-1040.pdf"} {
			env.upload(t, id, name, minimalPDF)
		}
		env.realRunner.Wait()
		check(t, env, "3 documents missing")
	})
	t.Run("flagged", func(t *testing.T) { env, _, _ := readyCase(t, true); check(t, env, "1 field to verify") })
	t.Run("failed", func(t *testing.T) {
		env, id, bank := readyCase(t, false)
		if err := env.store.SetDocumentStatus(bank, "failed", "test"); err != nil {
			t.Fatal(err)
		}
		if err := env.realRunner.Refresh(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		check(t, env, "Document failed")
	})
	t.Run("processing", func(t *testing.T) {
		env, _, bank := readyCase(t, false)
		if err := env.store.SetDocumentStatus(bank, "parsing", ""); err != nil {
			t.Fatal(err)
		}
		check(t, env, "Processing")
	})
	t.Run("near limit", func(t *testing.T) { env, _, _ := readyCase(t, false); check(t, env, "DTI near 43% limit") })
	t.Run("above limit and decided", func(t *testing.T) {
		env, id, bank := readyCase(t, false)
		res := env.do(t, "PATCH", "/api/documents/"+bank+"/fields/monthly_debt", map[string]any{"value": 4000})
		if res.StatusCode != 200 {
			t.Fatalf("edit %d", res.StatusCode)
		}
		check(t, env, "DTI above 43%")
		res = env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "reject", "note": "Verified"})
		if res.StatusCode != 200 {
			t.Fatalf("reject %d", res.StatusCode)
		}
		check(t, env, "Rejected")
	})
	t.Run("ready", func(t *testing.T) {
		env, id, bank := readyCase(t, false)
		res := env.do(t, "PATCH", "/api/documents/"+bank+"/fields/monthly_debt", map[string]any{"value": 2800})
		if res.StatusCode != 200 {
			t.Fatalf("edit %d", res.StatusCode)
		}
		check(t, env, "Ready for decision")
		detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
		if detail["case"].(map[string]any)["status"] != "ready" {
			t.Fatalf("case not ready: %v", detail["case"])
		}
	})
}

func TestCaseBlockerLowJudgmentBeforeDTI(t *testing.T) {
	dti := .5
	detail := store.Detail{Case: store.Case{Status: "needs_review"}, Assessment: &store.Assessment{DTI: &dti, Recommendation: "needs_review"}}
	for _, kind := range schemas.Types {
		detail.Documents = append(detail.Documents, store.DocumentDetail{Document: store.Document{DocType: kind, Status: "done"}})
	}
	bank := &detail.Documents[len(detail.Documents)-1]
	bank.Judgments = []store.Judgment{{Name: "document_authenticity", Score: .3}}
	if got := caseBlocker(detail); got != "Check document authenticity on the bank statement" {
		t.Fatalf("bank: %q", got)
	}
	bank.Fields = []store.Field{{Flagged: true}}
	if got := caseBlocker(detail); got != "1 field to verify" {
		t.Fatalf("flag precedence: %q", got)
	}
	bank.Fields = nil
	bank.Judgments[0].Score = .76
	detail.CaseJudgments = []store.Judgment{{Name: "income_consistency", Score: .49}}
	if got := caseBlocker(detail); got != "Check income consistency" {
		t.Fatalf("case: %q", got)
	}
	detail.CaseJudgments[0].Score = .79
	if got := caseBlocker(detail); got != "DTI above 43%" {
		t.Fatalf("DTI: %q", got)
	}
}

func TestCaseBlockerUnsupportedDocuments(t *testing.T) {
	env, id, _ := readyCase(t, false)
	check := func(want string) {
		t.Helper()
		list := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases", nil))
		if got := list[0]["blocker"]; got != want {
			t.Fatalf("want %q got %v", want, got)
		}
	}
	for i, name := range []string{"drivers-license.pdf", "second-license.pdf"} {
		env.upload(t, id, name, minimalPDF)
		env.realRunner.Wait()
		if i == 0 {
			check("1 unsupported document")
		} else {
			check("2 unsupported documents")
		}
	}
}

func TestCaseBlockerReadyRequiresCleanEligibleAssessment(t *testing.T) {
	detail := store.Detail{Case: store.Case{Status: "ready"}}
	for _, kind := range schemas.Types {
		detail.Documents = append(detail.Documents, store.DocumentDetail{Document: store.Document{DocType: kind, Status: "done"}})
	}
	for _, tc := range []struct {
		name       string
		assessment *store.Assessment
		want       string
	}{
		{"no assessment", nil, "Needs review"},
		{"review recommendation", &store.Assessment{Recommendation: "needs_review"}, "Needs review"},
		{"ineligible without DTI", &store.Assessment{Recommendation: "ineligible"}, "Needs review"},
		{"eligible with reasons", &store.Assessment{Recommendation: "eligible", Reasons: []string{"Unexpected concern"}}, "Needs review"},
		{"eligible clean", &store.Assessment{Recommendation: "eligible"}, "Ready for decision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detail.Assessment = tc.assessment
			if got := caseBlocker(detail); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	detail.Documents = append(detail.Documents, store.DocumentDetail{Document: store.Document{DocType: "other", Status: "superseded"}})
	if got := caseBlocker(detail); got != "Ready for decision" {
		t.Fatalf("superseded other: %q", got)
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

func TestCreateCaseRejectsDuplicateLoanNumber(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	res := env.do(t, "POST", "/api/cases", map[string]any{
		"borrower_name": "Another Borrower", "loan_number": "HB-20486",
		"loan_product": "fixed", "requested_amount": 250000,
	})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}
	if body := decode[map[string]string](t, res); body["error"] != "a case with loan number HB-20486 already exists" {
		t.Fatalf("unexpected response %v", body)
	}
	list := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases", nil))
	if len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("duplicate was created: %v", list)
	}
}

func TestStoredFileStem(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"/", "file"},
		{strings.Repeat("a", 300) + ".pdf", strings.Repeat("a", 64)},
		{`..\..\evil.pdf`, "evil"},
		{"bank\nstatement.pdf", "bank_statement"},
		{"bank-statement.pdf", "bank-statement"},
		{"", "file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := storedFileStem(tc.name); got != tc.want {
				t.Fatalf("stem = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUploadHostileFilenames(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	for _, name := range []string{"/", strings.Repeat("a", 300) + ".pdf"} {
		res := env.upload(t, id, name, minimalPDF)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("upload %q: want 201 got %d", name, res.StatusCode)
		}
		doc := decode[map[string]any](t, res)
		stored, err := env.store.GetDocument(doc["id"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(stored.FilePath) != filepath.Join(env.server.opts.UploadDir, id) {
			t.Fatalf("file escaped upload dir: %q", stored.FilePath)
		}
		if _, err := os.Stat(stored.FilePath); err != nil {
			t.Fatal(err)
		}
		if len(stored.FileName) > 255 || stored.FileName != doc["file_name"] {
			t.Fatalf("display filename %q", stored.FileName)
		}
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

func TestUploadWithoutRunnerIsRejected(t *testing.T) {
	env := newTestEnv(t)
	dir := t.TempDir()
	ts := httptest.NewServer(New(env.store, Options{UploadDir: dir}).Handler())
	defer ts.Close()
	env.url = ts.URL
	if res := env.do(t, "POST", "/api/auth/login", map[string]string{"email": "reviewer@casereview.test", "password": "pw"}); res.StatusCode != http.StatusOK {
		t.Fatalf("login %d", res.StatusCode)
	}
	id := env.createCase(t)
	res := env.upload(t, id, "w2.pdf", minimalPDF)
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 got %d", res.StatusCode)
	}
	if body := decode[map[string]string](t, res); body["error"] != "document processing is not configured" {
		t.Fatalf("unexpected error %v", body)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("upload dir not empty: %v, err %v", entries, err)
	}
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	if docs := detail["documents"].([]any); len(docs) != 0 {
		t.Fatalf("orphaned documents: %v", docs)
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
