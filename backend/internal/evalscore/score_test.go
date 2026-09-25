package evalscore

import (
	"testing"
	"tidalwave/backend/internal/pipeline"
)

func f(v float64) *float64 { return &v }

func TestScore(t *testing.T) {
	fixtures := []Fixture{{ID: "boundary", Documents: []FixtureDoc{
		{File: "a.pdf", ExpectedDocType: "bank_statement", ExpectedFields: map[string]any{"ending_balance": 18482.0, "bank_name": "Harborview Community Bank"}},
		{File: "b.pdf", ExpectedDocType: "other", ExpectedFields: map[string]any{}},
	}, Expected: Expected{DTI: f(0.4281), Recommendation: "needs_review", ShouldFlag: false}}}
	results := []CaseResult{{FixtureID: "boundary", Docs: []DocResult{
		{File: "a.pdf", DocType: "bank_statement", Fields: map[string]any{"ending_balance": 18482.4, "bank_name": " harborview community bank"}},
		{File: "b.pdf", DocType: "pay_stub"},
	}, DTI: f(0.43), Recommendation: "needs_review", Flagged: true}}
	r := Score(fixtures, results)
	if r.Classification != (Layer{1, 2, 0.5}) {
		t.Fatalf("classification %+v", r.Classification)
	}
	if r.Extraction != (Layer{2, 2, 1}) {
		t.Fatalf("extraction %+v", r.Extraction)
	}
	if r.EndToEnd != (Layer{1, 1, 1}) {
		t.Fatalf("end-to-end %+v", r.EndToEnd)
	}
	if r.Calibration != (Layer{0, 1, 0}) {
		t.Fatalf("calibration %+v", r.Calibration)
	}
	if len(r.Misses) != 2 {
		t.Fatalf("misses %+v", r.Misses)
	}
}
func TestCalibrationTableTracksAllJudgments(t *testing.T) {
	fixtures := []Fixture{
		{ID: "clean", Expected: Expected{ShouldFlag: false}},
		{ID: "flag", Expected: Expected{ShouldFlag: true}},
	}
	results := []CaseResult{
		{FixtureID: "clean", Docs: []DocResult{{Judgments: []JudgmentScore{{"document_authenticity", .76}, {"document_authenticity", .91}, {"ocr_quality", .79}}}}, CaseJudgments: []JudgmentScore{{"income_consistency", .79}}},
		{FixtureID: "flag", Docs: []DocResult{{Judgments: []JudgmentScore{{"document_authenticity", .3}, {"ocr_quality", .81}, {"ocr_quality", .54}}}}, CaseJudgments: []JudgmentScore{{"income_consistency", .68}}},
	}
	table := Score(fixtures, results).CalibrationTable
	auth := table["document_authenticity"]
	if auth.Threshold != pipeline.Threshold("document_authenticity") || auth.Clean.Count != 2 || *auth.Clean.Min != .76 || *auth.Clean.Max != .91 || auth.Flag.Count != 1 || *auth.Flag.Min != .3 || auth.FalsePositiveRisk {
		t.Fatalf("auth: %+v", auth)
	}
	ocr := table["ocr_quality"]
	if ocr.Clean.Count != 1 || *ocr.Clean.Min != .79 || ocr.Flag.Count != 2 || *ocr.Flag.Max != .81 || !ocr.FalsePositiveRisk {
		t.Fatalf("ocr: %+v", ocr)
	}
	income := table["income_consistency"]
	if income.Clean.Count != 1 || income.Flag.Count != 1 || *income.Clean.Min != .79 || *income.Flag.Max != .68 || income.FalsePositiveRisk {
		t.Fatalf("income: %+v", income)
	}
}

func TestScoreMissingResultCountsAsWrong(t *testing.T) {
	r := Score([]Fixture{{ID: "x", Documents: []FixtureDoc{{File: "a", ExpectedDocType: "w2", ExpectedFields: map[string]any{"tax_year": 2025.0}}}, Expected: Expected{Recommendation: "eligible"}}}, nil)
	if r.Classification.Total != 1 || r.Classification.Correct != 0 || r.EndToEnd.Correct != 0 || r.Extraction.Total != 1 {
		t.Fatalf("%+v", r)
	}
}
