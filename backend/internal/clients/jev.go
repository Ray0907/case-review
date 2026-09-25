package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		return nil, fmt.Errorf("jev: HTTP %d", res.StatusCode)
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
		"document_authenticity": {Type: "noul", Instructions: "Judge whether `document_text` is internally consistent for a `document_type`: totals, dates, names, and employer or bank details agree with each other and nothing looks edited. A footer reading SYNTHETIC TEST DOCUMENT marks demo data; do not treat that footer as a sign of forgery.",
			Criteria: map[string]string{"true": "Totals, dates, and names agree; no signs of editing", "false": "Totals or dates disagree, values look overwritten, or details contradict each other"}},
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
