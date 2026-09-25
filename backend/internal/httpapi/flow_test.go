package httpapi

import (
	"net/http"
	"testing"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/pipeline/fake"
	"tidalwave/backend/internal/store"
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

func TestCaseJudgmentLowInAPI(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	doc, err := env.store.CreateDocument(store.Document{CaseID: id, FileName: "bank.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SaveJudgments(doc.ID, []store.Judgment{{Name: "document_authenticity", Score: .76}, {Name: "ocr_quality", Score: .79}}); err != nil {
		t.Fatal(err)
	}
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	js := detail["documents"].([]any)[0].(map[string]any)["judgments"].([]any)
	for _, j := range js {
		v := j.(map[string]any)
		want := v["name"] == "ocr_quality"
		if v["low"] != want {
			t.Fatalf("%s low = %v, want %v", v["name"], v["low"], want)
		}
	}
}

var _ = pipeline.Stages{}
