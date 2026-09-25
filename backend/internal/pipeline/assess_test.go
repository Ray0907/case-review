package pipeline

import (
	"testing"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

func f(v float64) *float64 { return &v }

func good() []store.Judgment {
	return []store.Judgment{{Name: "field_completeness", Score: 0.96}, {Name: "income_consistency", Score: 0.93}}
}

func TestComputeDTI(t *testing.T) {
	cases := []struct {
		income, debt, want float64
		err                bool
	}{
		{7200, 3082, 0.4281, false},
		{7200, 2100, 0.2917, false},
		{7200, 0, 0, false},
		{0, 100, 0, true},
		{-1, 100, 0, true},
		{7200, -5, 0, true},
	}
	for _, c := range cases {
		got, err := ComputeDTI(c.income, c.debt)
		if (err != nil) != c.err || (!c.err && got != c.want) {
			t.Errorf("ComputeDTI(%v,%v)=%v,%v want %v err=%v", c.income, c.debt, got, err, c.want, c.err)
		}
	}
}

func TestAssessEligible(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(2100), Judgments: good()})
	if a.Recommendation != "eligible" || len(a.Reasons) != 0 || *a.DTI != 0.2917 {
		t.Fatalf("%+v", a)
	}
}

func TestAssessIneligibleWhenClean(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(3600), Judgments: good()})
	if a.Recommendation != "ineligible" {
		t.Fatalf("%+v", a)
	}
}

func TestAssessBoundaryNeedsReview(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(3082), Judgments: good()})
	if a.Recommendation != "needs_review" || a.Reasons[0] != "DTI is 42.8%, within 2 points of the 43% limit" {
		t.Fatalf("%+v", a)
	}
}

func TestAssessProperFormNameInConfidenceReason(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(2100),
		Judgments: []store.Judgment{{Name: "ocr_quality", Score: .54, DocumentType: schemas.W2}}})
	if len(a.Reasons) != 1 || a.Reasons[0] != "Low confidence: OCR extraction quality on the W-2 (54%)" {
		t.Fatalf("reasons: %v", a.Reasons)
	}
}

func TestJudgmentThresholds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		score float64
		low   bool
	}{
		{"document_authenticity", .76, false}, {"income_consistency", .79, false},
		{"document_authenticity", .49, true}, {"ocr_quality", .79, true},
		{"field_completeness", .79, true}, {"unknown", .79, true},
	} {
		if got := IsLow(tc.name, tc.score); got != tc.low {
			t.Errorf("IsLow(%q, %v) = %v, want %v", tc.name, tc.score, got, tc.low)
		}
	}
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(2100), Judgments: []store.Judgment{
		{Name: "document_authenticity", Score: .76}, {Name: "income_consistency", Score: .79},
	}})
	if a.Recommendation != "eligible" || len(a.Reasons) != 0 {
		t.Fatalf("%+v", a)
	}
}

func TestAssessNoIncome(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(0), MonthlyDebt: f(3082), Judgments: good()})
	if a.DTI != nil || a.Recommendation != "needs_review" || a.Reasons[0] != "Monthly income unavailable, DTI not computed" {
		t.Fatalf("%+v", a)
	}
	a = Assess(AssessInput{PresentTypes: schemas.Types, Judgments: good()})
	if a.DTI != nil || a.Recommendation != "needs_review" {
		t.Fatalf("%+v", a)
	}
}

func TestAssessPluralReasonsAndHumanJudgments(t *testing.T) {
	a := Assess(AssessInput{PresentTypes: schemas.Types, MonthlyIncome: f(7200), MonthlyDebt: f(2100), FailedDocs: 2, UnsupportedDocs: 2,
		UnresolvedFlags: 2, Judgments: []store.Judgment{{Name: "document_authenticity", Score: .7}, {Name: "field_completeness", Score: .6}, {Name: "income_consistency", Score: .7}}})
	want := []string{"2 documents failed processing", "2 unsupported documents uploaded", "Low confidence: Field completeness (60%)", "2 fields need verification"}
	if len(a.Reasons) != len(want) {
		t.Fatalf("reasons %v", a.Reasons)
	}
	for i, reason := range want {
		if a.Reasons[i] != reason {
			t.Fatalf("reason %d: %q != %q", i, a.Reasons[i], reason)
		}
	}
}

func TestAssessMissingDocsLowConfidenceFlags(t *testing.T) {
	a := Assess(AssessInput{
		PresentTypes:  []string{schemas.W2, schemas.PayStub, schemas.BankStatement, schemas.Form1003},
		MonthlyIncome: f(7200), MonthlyDebt: f(2100),
		Judgments:       []store.Judgment{{Name: "ocr_quality", Score: 0.54, DocumentType: schemas.BankStatement}},
		UnresolvedFlags: 1, UnsupportedDocs: 1, FailedDocs: 1,
	})
	want := []string{
		"Missing documents: Form 1040",
		"1 document failed processing",
		"1 unsupported document uploaded",
		"Low confidence: OCR extraction quality on the bank statement (54%)",
		"1 field needs verification",
	}
	if a.Recommendation != "needs_review" || len(a.Reasons) != len(want) {
		t.Fatalf("%+v", a)
	}
	for i := range want {
		if a.Reasons[i] != want[i] {
			t.Fatalf("reason %d = %q want %q", i, a.Reasons[i], want[i])
		}
	}
}
