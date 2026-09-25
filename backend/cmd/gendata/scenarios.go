package main

import "math"

type docSpec struct {
	Name, Type, Holder string
	Fields             map[string]any
	LowQ               bool
	Tampered           bool
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
		{Name: "w2-2025.pdf", Type: "w2", Fields: map[string]any{"employer_name": "Example Logistics LLC", "tax_year": 2025.0, "box1_wages": 86400.0, "box2_fed_tax": 11230.0}},
		{Name: "form-1040.pdf", Type: "form_1040", Fields: map[string]any{"tax_year": 2025.0, "adjusted_gross_income": 84900.0, "taxable_income": 70300.0, "total_tax": 9420.0}},
		{Name: "form-1003.pdf", Type: "form_1003", Fields: map[string]any{"borrower_name": "Jordan Alvarez", "property_address": "118 Example Ave, Hoboken NJ 07030", "loan_amount": 410000.0, "loan_purpose": "Purchase", "stated_monthly_income": 7200.0}},
		{Name: "pay-stub.pdf", Type: "pay_stub", Fields: map[string]any{"employer_name": "Example Logistics LLC", "pay_period_end": "2026-08-31", "gross_pay": 3600.0, "ytd_gross": 57600.0, "monthly_income": 7200.0}},
		{Name: "bank-statement.pdf", Type: "bank_statement", Fields: map[string]any{"bank_name": "Example Community Bank", "account_last4": "4471", "statement_period": "2026-08", "beginning_balance": 14010.55 + debt, "ending_balance": 18482.0, "total_deposits": 5472.0, "total_withdrawals": debt + 1000.55, "monthly_debt": debt, "nsf_count": 0.0, "bnpl_hits": 2.0}},
	}
}

var scenarios = func() []scenario {
	standard := scenario{ID: "standard", Description: "Complete file, comfortable DTI", Income: 7200, Debt: 2100, Docs: baseDocs(2100), Expected: expected{DTI: ptr(0.2917), Recommendation: "eligible"}}
	boundary := scenario{ID: "boundary", Description: "Complete file, DTI just under the 43% QM threshold", Income: 7200, Debt: 3082, Docs: baseDocs(3082), Expected: expected{DTI: ptr(0.4281), Recommendation: "needs_review"}}
	missing := scenario{ID: "missing_docs", Description: "Form 1040 not uploaded", Income: 7200, Debt: 2100, Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review"}}
	for _, d := range baseDocs(2100) {
		if d.Type != "form_1040" {
			missing.Docs = append(missing.Docs, d)
		}
	}
	lowq := scenario{ID: "low_quality_scan", Description: "Bank statement is a noisy low-resolution scan", Income: 7200, Debt: 2100, Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review", ShouldFlag: true}}
	for _, d := range baseDocs(2100) {
		if d.Type == "bank_statement" {
			d.Name, d.LowQ = "bank-statement-lowq.pdf", true
		}
		lowq.Docs = append(lowq.Docs, d)
	}
	unsupported := scenario{ID: "unsupported_doc", Description: "A driver's license was uploaded alongside the file", Income: 7200, Debt: 2100, Docs: append(baseDocs(2100), docSpec{Name: "drivers-license.pdf", Type: "other", Fields: map[string]any{}}), Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review"}}
	above := scenario{ID: "above_limit", Description: "Complete file, DTI above the 43% QM threshold", Income: 7200, Debt: 3600, Docs: baseDocs(3600), Expected: expected{DTI: ptr(0.5), Recommendation: "ineligible"}}
	tampered := scenario{ID: "tampered_statement", Description: "Bank statement ending balance altered by $4,000 with mismatched font", Income: 7200, Debt: 2100, Docs: baseDocs(2100), Expected: expected{DTI: ptr(0.2917), Recommendation: "needs_review", ShouldFlag: true}}
	for i := range tampered.Docs {
		if tampered.Docs[i].Type == "bank_statement" {
			tampered.Docs[i].Tampered = true
			tampered.Docs[i].Fields["ending_balance"] = 14482.0
		}
	}
	all := []scenario{standard, boundary, missing, lowq, unsupported, above, tampered}
	identities := [][3]string{
		{"Jordan Alvarez", "Example Logistics LLC", "Example Community Bank"},
		{"Maya Bennett", "Sample Rail Works LLC", "Sample Valley Credit Union"},
		{"Alex Chen", "Demo Supply Co", "Demo Savings Bank"},
		{"Riley Morgan", "Example Paper Company", "Example Regional Bank"},
		{"Casey Rivera", "Sample Freight LLC", "Sample Mutual Bank"},
		{"Taylor Brooks", "Demo Studios LLC", "Demo Community Credit Union"},
		{"Morgan Ellis", "Example Print Works LLC", "Demo Trust Bank"},
	}
	for i := range all {
		name, employer, bank := identities[i][0], identities[i][1], identities[i][2]
		for j := range all[i].Docs {
			d := &all[i].Docs[j]
			d.Holder = name
			switch d.Type {
			case "w2", "pay_stub":
				d.Fields["employer_name"] = employer
			case "form_1003":
				d.Fields["borrower_name"] = name
			case "bank_statement":
				d.Fields["bank_name"] = bank
			}
		}
	}
	return all
}()

// demoSets are document folders for live walkthroughs; they get no eval fixture.
var demoSets = func() []scenario {
	live := scenario{ID: "live_demo", Description: "Live walkthrough: low-quality bank statement for Ray Tien", Income: 7200, Debt: 2100}
	for _, d := range baseDocs(2100) {
		d.Holder = "Ray Tien"
		switch d.Type {
		case "w2", "pay_stub":
			d.Fields["employer_name"] = "Demo Analytics LLC"
		case "form_1003":
			d.Fields["borrower_name"] = "Ray Tien"
		case "bank_statement":
			d.Name, d.LowQ = "bank-statement-lowq.pdf", true
			d.Fields["bank_name"] = "Sample Neighborhood Bank"
		}
		live.Docs = append(live.Docs, d)
	}
	return []scenario{live}
}()
