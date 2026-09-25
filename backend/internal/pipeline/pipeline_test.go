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

func (r *recorder) Publish(e pipeline.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

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
