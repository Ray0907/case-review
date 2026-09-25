package pipeline

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

const (
	QMThreshold        = 0.43
	NearThresholdFloor = 0.41
)

// Threshold is the minimum acceptable confidence for a judgment.
func Threshold(name string) float64 {
	switch name {
	case "document_authenticity", "income_consistency":
		return 0.5
	default:
		return 0.8
	}
}

func IsLow(name string, score float64) bool { return score < Threshold(name) }

func ComputeDTI(monthlyIncome, monthlyDebt float64) (float64, error) {
	if monthlyIncome <= 0 {
		return 0, errors.New("monthly income must be positive")
	}
	if monthlyDebt < 0 {
		return 0, errors.New("monthly debt cannot be negative")
	}
	return math.Round(monthlyDebt/monthlyIncome*10000) / 10000, nil
}

type AssessInput struct {
	PresentTypes    []string
	MonthlyIncome   *float64
	MonthlyDebt     *float64
	Judgments       []store.Judgment
	UnresolvedFlags int
	UnsupportedDocs int
	FailedDocs      int
}

func countReason(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

var judgmentNames = map[string]string{
	"ocr_quality": "OCR extraction quality", "document_authenticity": "Document authenticity",
	"field_completeness": "Field completeness", "income_consistency": "Income consistency",
}

func JudgmentLabel(name string) string {
	if label := judgmentNames[name]; label != "" {
		return label
	}
	return name
}

func Assess(in AssessInput) store.Assessment {
	a := store.Assessment{MonthlyIncome: in.MonthlyIncome, MonthlyDebt: in.MonthlyDebt, Reasons: []string{}}
	present := map[string]bool{}
	for _, t := range in.PresentTypes {
		present[t] = true
	}
	var missing []string
	for _, t := range schemas.Types {
		if !present[t] {
			missing = append(missing, schemas.Label(t))
		}
	}
	if len(missing) > 0 {
		a.Reasons = append(a.Reasons, "Missing documents: "+strings.Join(missing, ", "))
	}
	if in.FailedDocs > 0 {
		a.Reasons = append(a.Reasons, countReason(in.FailedDocs, "document failed processing", "documents failed processing"))
	}
	if in.UnsupportedDocs > 0 {
		a.Reasons = append(a.Reasons, countReason(in.UnsupportedDocs, "unsupported document uploaded", "unsupported documents uploaded"))
	}
	switch {
	case in.MonthlyIncome == nil || *in.MonthlyIncome <= 0:
		a.Reasons = append(a.Reasons, "Monthly income unavailable, DTI not computed")
	case in.MonthlyDebt == nil:
		a.Reasons = append(a.Reasons, "Monthly debt unavailable, DTI not computed")
	default:
		if dti, err := ComputeDTI(*in.MonthlyIncome, *in.MonthlyDebt); err == nil {
			a.DTI = &dti
			if dti >= NearThresholdFloor && dti <= QMThreshold {
				a.Reasons = append(a.Reasons, fmt.Sprintf("DTI is %.1f%%, within 2 points of the 43%% limit", dti*100))
			}
		} else {
			a.Reasons = append(a.Reasons, "DTI not computed: "+err.Error())
		}
	}
	for _, j := range in.Judgments {
		if IsLow(j.Name, j.Score) {
			name := JudgmentLabel(j.Name)
			if j.DocumentType != "" {
				label := schemas.Label(j.DocumentType)
				if j.DocumentType == schemas.PayStub || j.DocumentType == schemas.BankStatement {
					label = strings.ToLower(label)
				}
				name += " on the " + label
			}
			a.Reasons = append(a.Reasons, fmt.Sprintf("Low confidence: %s (%.0f%%)", name, j.Score*100))
		}
	}
	if in.UnresolvedFlags > 0 {
		a.Reasons = append(a.Reasons, countReason(in.UnresolvedFlags, "field needs verification", "fields need verification"))
	}
	switch {
	case len(a.Reasons) > 0:
		a.Recommendation = "needs_review"
	case a.DTI != nil && *a.DTI > QMThreshold:
		a.Recommendation = "ineligible"
	default:
		a.Recommendation = "eligible"
	}
	return a
}
