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
