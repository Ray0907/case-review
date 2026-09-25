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
