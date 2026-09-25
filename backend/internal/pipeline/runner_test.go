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

func TestFakePayStubMatchesBoundaryScenario(t *testing.T) {
	s, c := newCase(t)
	r := pipeline.NewRunner(s, fake.Stages(), &recorder{})
	ids := addDocs(t, s, r, c.ID, "pay-stub.pdf")
	values, err := s.FieldValues(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]float64{"gross_pay": 3600, "ytd_gross": 57600, "monthly_income": 7200} {
		if values[key] != want {
			t.Fatalf("%s: %v, want %v", key, values[key], want)
		}
	}
}

func TestFakeParserFailsOncePerFile(t *testing.T) {
	parser := fake.Stages().Parser
	first := "/tmp/one-pay-stub-fail-once.pdf"
	second := "/tmp/two-pay-stub-fail-once.pdf"
	for _, path := range []string{first, second} {
		if _, err := parser.Parse(context.Background(), path); err == nil {
			t.Fatalf("first parse of %s should fail", path)
		}
	}
	for _, path := range []string{first, second} {
		if text, err := parser.Parse(context.Background(), path); err != nil || text == "" {
			t.Fatalf("retry %s: %q, %v", path, text, err)
		}
	}
	if _, err := parser.Parse(context.Background(), "/tmp/ordinary-pay-stub.pdf"); err != nil {
		t.Fatalf("ordinary file failed: %v", err)
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
