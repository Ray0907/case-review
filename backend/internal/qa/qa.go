// Package qa answers questions about a closed case from its recorded file only.
package qa

import "context"

type CaseFacts struct {
	Borrower        string  `json:"borrower"`
	LoanNumber      string  `json:"loan_number"`
	LoanProduct     string  `json:"loan_product"`
	RequestedAmount float64 `json:"requested_amount"`
	Status          string  `json:"status"`
}

type Assessment struct {
	MonthlyIncome  *float64 `json:"monthly_income"`
	MonthlyDebt    *float64 `json:"monthly_debt"`
	DTI            *float64 `json:"dti"`
	Recommendation string   `json:"recommendation"`
	Reasons        []string `json:"reasons"`
}

type FieldFact struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	Value            any    `json:"value"`
	Flagged          bool   `json:"flagged"`
	FlagReason       string `json:"flag_reason,omitempty"`
	EditedByReviewer bool   `json:"edited_by_reviewer"`
}

type Judgment struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type DocFact struct {
	Type      string      `json:"type"`
	FileName  string      `json:"file_name"`
	Fields    []FieldFact `json:"fields"`
	Judgments []Judgment  `json:"judgments"`
}

type AuditLine struct {
	Time     string `json:"time"`
	Reviewer string `json:"reviewer"`
	Action   string `json:"action"`
	Note     string `json:"note,omitempty"`
}

// Facts is everything an answer may use. Audit is newest first.
type Facts struct {
	Case          CaseFacts   `json:"case"`
	Assessment    *Assessment `json:"assessment"`
	Documents     []DocFact   `json:"documents"`
	CaseJudgments []Judgment  `json:"case_judgments"`
	Audit         []AuditLine `json:"audit"`
}

// Citation points at a recorded value. Document is a document type or "assessment".
type Citation struct {
	Document string `json:"document"`
	Key      string `json:"key"`
}

type Result struct {
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
}

type Asker interface {
	Ask(ctx context.Context, facts Facts, question string) (Result, error)
}

// StreamAsker also reports the answer text as it is written, so a client can show it live.
type StreamAsker interface {
	Asker
	AskStream(ctx context.Context, facts Facts, question string, onText func(string)) (Result, error)
}

var assessmentKeys = map[string]bool{"dti": true, "monthly_income": true, "monthly_debt": true, "recommendation": true}

// ValidCitations keeps only citations that point at a value present in facts, once each.
func ValidCitations(facts Facts, in []Citation) []Citation {
	known := map[Citation]bool{}
	for _, d := range facts.Documents {
		for _, f := range d.Fields {
			known[Citation{Document: d.Type, Key: f.Key}] = true
		}
	}
	out := []Citation{}
	seen := map[Citation]bool{}
	for _, c := range in {
		ok := known[c] || (c.Document == "assessment" && assessmentKeys[c.Key] && facts.Assessment != nil)
		if ok && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}
