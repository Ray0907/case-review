package qa

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Fake answers from the recorded facts with fixed templates. It makes no network call.
type Fake struct{}

func money(n float64) string {
	s := fmt.Sprintf("%.2f", n)
	whole, frac := s[:len(s)-3], s[len(s)-3:]
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	return "$" + whole + frac
}

// AskStream writes the same answer word by word so the live display can be tried without a model.
func (f Fake) AskStream(ctx context.Context, facts Facts, question string, onText func(string)) (Result, error) {
	res, err := f.Ask(ctx, facts, question)
	if err != nil {
		return res, err
	}
	words := strings.SplitAfter(res.Answer, " ")
	for _, w := range words {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(18 * time.Millisecond):
		}
		onText(w)
	}
	return res, nil
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func (Fake) Ask(_ context.Context, f Facts, question string) (Result, error) {
	q := strings.ToLower(question)
	a := f.Assessment
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(q, w) {
				return true
			}
		}
		return false
	}
	switch {
	case a != nil && a.DTI != nil && has("dti", "debt", "ratio"):
		return Result{
			Answer: fmt.Sprintf("The debt-to-income ratio was **%.1f%%**: %s of monthly debt against %s of monthly income. The qualified-mortgage limit is **43%%**.",
				*a.DTI*100, money(deref(a.MonthlyDebt)), money(deref(a.MonthlyIncome))),
			Citations: []Citation{{"assessment", "dti"}, {"assessment", "monthly_debt"}, {"assessment", "monthly_income"}},
		}, nil
	case a != nil && has("income", "salary", "wage", "pay"):
		return Result{
			Answer:    fmt.Sprintf("Monthly income used for the ratio was %s, taken from the income documents on file.", money(deref(a.MonthlyIncome))),
			Citations: []Citation{{"assessment", "monthly_income"}},
		}, nil
	case has("edit", "correct", "verified", "change"):
		cites := []Citation{}
		var names []string
		for _, d := range f.Documents {
			for _, fld := range d.Fields {
				if fld.EditedByReviewer {
					cites = append(cites, Citation{d.Type, fld.Key})
					names = append(names, fld.Label)
				}
			}
		}
		if len(names) == 0 {
			return Result{Answer: "The reviewer did not edit or confirm any extracted field on this case.", Citations: cites}, nil
		}
		return Result{Answer: "The reviewer confirmed or corrected:\n\n- " + strings.Join(names, "\n- "), Citations: cites}, nil
	}
	answer := fmt.Sprintf("Case %s for %s is %s.", f.Case.LoanNumber, f.Case.Borrower, strings.ReplaceAll(f.Case.Status, "_", " "))
	if len(f.Audit) > 0 {
		answer += fmt.Sprintf(" The last recorded action was %q by %s.", f.Audit[0].Action, f.Audit[0].Reviewer)
	}
	cites := []Citation{}
	if a != nil && a.DTI != nil {
		cites = append(cites, Citation{"assessment", "dti"})
	}
	return Result{Answer: answer, Citations: cites}, nil
}
