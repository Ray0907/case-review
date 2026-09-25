package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
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
	seen     map[string]bool
}

func (p *Parser) Parse(_ context.Context, path string) (string, error) {
	name := strings.ToLower(filepath.Base(path))
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.seen == nil {
		p.seen = map[string]bool{}
	}
	for k, enabled := range p.FailOnce {
		if enabled && strings.Contains(name, k) && !p.seen[path] {
			p.seen[path] = true
			return "", errors.New("simulated parser outage")
		}
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if i := bytes.Index(raw, []byte("HARBOR-FIELDS:")); i >= 0 {
			var fields map[string]any
			payload := raw[i+len("HARBOR-FIELDS:"):]
			if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&fields); err == nil {
				compact, _ := json.Marshal(fields)
				return name + " HARBOR-FIELDS:" + string(compact), nil
			}
		}
	}
	return name, nil
}

func markedFields(text string) map[string]any {
	i := strings.Index(text, "HARBOR-FIELDS:")
	if i < 0 {
		return nil
	}
	var fields map[string]any
	if json.NewDecoder(strings.NewReader(text[i+len("HARBOR-FIELDS:"):])).Decode(&fields) != nil {
		return nil
	}
	return fields
}

type Classifier struct{}

func (Classifier) Classify(_ context.Context, text string) (string, float64, error) {
	if kind, ok := markedFields(text)["doc_type"].(string); ok {
		switch kind {
		case schemas.W2, schemas.Form1040, schemas.Form1003, schemas.PayStub, schemas.BankStatement, schemas.Other:
			return kind, 0.99, nil
		}
	}
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
	schemas.W2:       {"employer_name": "Example Logistics LLC", "tax_year": 2025.0, "box1_wages": 86400.0, "box2_fed_tax": 11230.0},
	schemas.Form1040: {"tax_year": 2025.0, "adjusted_gross_income": 84900.0, "taxable_income": 70300.0, "total_tax": 9420.0},
	schemas.Form1003: {"borrower_name": "Jordan Alvarez", "property_address": "118 Example Ave, Hoboken NJ",
		"loan_amount": 410000.0, "loan_purpose": "purchase", "stated_monthly_income": 7200.0},
	schemas.PayStub: {"employer_name": "Example Logistics LLC", "pay_period_end": "2026-08-31",
		"gross_pay": 3600.0, "ytd_gross": 57600.0, "monthly_income": 7200.0},
	schemas.BankStatement: {"bank_name": "Example Community Bank", "account_last4": "4471", "statement_period": "2026-08",
		"beginning_balance": 17092.55, "ending_balance": 18482.0, "total_deposits": 5472.0,
		"total_withdrawals": 4082.55, "monthly_debt": 3082.0, "nsf_count": 0.0, "bnpl_hits": 2.0},
}

type Extractor struct{}

func (Extractor) Extract(_ context.Context, docType, text string) (pipeline.Extraction, error) {
	src, ok := canned[docType]
	if marked := markedFields(text); marked != nil {
		if kind, ok := marked["doc_type"].(string); ok && kind == docType {
			delete(marked, "doc_type")
			src = marked
		}
	}
	if !ok {
		return pipeline.Extraction{}, errors.New("no canned data for " + docType)
	}
	fields := make(map[string]any, len(src))
	for k, v := range src {
		fields[k] = v
	}
	ex := pipeline.Extraction{Fields: fields, Uncertain: map[string]string{}}
	if strings.Contains(text, "lowq") && docType == schemas.BankStatement {
		ex.Uncertain["ending_balance"] = "Scan is unclear on the tens digit (3 or 8). Check the source, then confirm or correct the value."
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
	auth, authReason := 0.91, "No signs of tampering or template mismatch"
	if docType == schemas.BankStatement {
		fields := markedFields(text)
		begin, okBegin := fields["beginning_balance"].(float64)
		deposits, okDeposits := fields["total_deposits"].(float64)
		withdrawals, okWithdrawals := fields["total_withdrawals"].(float64)
		ending, okEnding := fields["ending_balance"].(float64)
		if okBegin && okDeposits && okWithdrawals && okEnding && math.Round((begin+deposits-withdrawals-ending)*100) != 0 {
			auth, authReason = 0.3, "Statement ending balance does not reconcile"
		}
	}
	return []store.Judgment{
		{Name: "field_completeness", Score: 0.96, Reason: "All required fields present"},
		{Name: "document_authenticity", Score: auth, Reason: authReason},
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
	return pipeline.Stages{Parser: &Parser{FailOnce: map[string]bool{"fail-once": true}}, Classifier: Classifier{}, Extractor: Extractor{}, Judge: Judge{}}
}
