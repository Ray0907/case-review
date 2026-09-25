package schemas

import (
	"fmt"
	"math"
)

const (
	W2            = "w2"
	Form1040      = "form_1040"
	Form1003      = "form_1003"
	PayStub       = "pay_stub"
	BankStatement = "bank_statement"
	Other         = "other"
)

var Types = []string{W2, Form1040, Form1003, PayStub, BankStatement}

type Kind string

const (
	KindString Kind = "string"
	KindNumber Kind = "number"
	KindInt    Kind = "int"
)

type FieldSpec struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  Kind   `json:"kind"`
}

var labels = map[string]string{
	W2: "W-2", Form1040: "Form 1040", Form1003: "Form 1003", PayStub: "Pay stub", BankStatement: "Bank statement",
}

func Label(docType string) string {
	if l, ok := labels[docType]; ok {
		return l
	}
	return docType
}

var Registry = map[string][]FieldSpec{
	W2: {
		{"employer_name", "Employer", KindString},
		{"tax_year", "Tax year", KindInt},
		{"box1_wages", "Box 1 wages", KindNumber},
		{"box2_fed_tax", "Box 2 federal tax withheld", KindNumber},
	},
	Form1040: {
		{"tax_year", "Tax year", KindInt},
		{"adjusted_gross_income", "Adjusted gross income", KindNumber},
		{"taxable_income", "Taxable income", KindNumber},
		{"total_tax", "Total tax", KindNumber},
	},
	Form1003: {
		{"borrower_name", "Borrower", KindString},
		{"property_address", "Property address", KindString},
		{"loan_amount", "Loan amount", KindNumber},
		{"loan_purpose", "Loan purpose", KindString},
		{"stated_monthly_income", "Stated monthly income", KindNumber},
	},
	PayStub: {
		{"employer_name", "Employer", KindString},
		{"pay_period_end", "Pay period end", KindString},
		{"gross_pay", "Gross pay this period", KindNumber},
		{"ytd_gross", "Year-to-date gross", KindNumber},
		{"monthly_income", "Monthly gross income", KindNumber},
	},
	BankStatement: {
		{"bank_name", "Bank", KindString},
		{"account_last4", "Account ending", KindString},
		{"statement_period", "Statement period", KindString},
		{"beginning_balance", "Beginning balance", KindNumber},
		{"ending_balance", "Ending balance", KindNumber},
		{"total_deposits", "Total deposits", KindNumber},
		{"total_withdrawals", "Total withdrawals", KindNumber},
		{"monthly_debt", "Recurring monthly debt", KindNumber},
		{"nsf_count", "NSF / overdraft events", KindInt},
		{"bnpl_hits", "BNPL installments", KindInt},
	},
}

func checkKind(kind Kind, v any) bool {
	switch kind {
	case KindString:
		_, ok := v.(string)
		return ok
	case KindNumber:
		f, ok := v.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
	case KindInt:
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	}
	return false
}

func IsOptional(docType, key string) bool {
	return docType == BankStatement && key == "total_withdrawals"
}

func Validate(docType string, values map[string]any) []string {
	specs, ok := Registry[docType]
	if !ok {
		return []string{fmt.Sprintf("unknown document type %q", docType)}
	}
	var errs []string
	for _, f := range specs {
		v, present := values[f.Key]
		if !present || v == nil {
			if IsOptional(docType, f.Key) {
				continue
			}
			errs = append(errs, fmt.Sprintf("missing field %s", f.Key))
			continue
		}
		if !checkKind(f.Kind, v) {
			errs = append(errs, fmt.Sprintf("field %s must be %s", f.Key, f.Kind))
		}
	}
	return errs
}

func CheckValue(docType, key string, v any) error {
	for _, f := range Registry[docType] {
		if f.Key == key {
			if !checkKind(f.Kind, v) {
				return fmt.Errorf("%s must be %s", f.Label, f.Kind)
			}
			return nil
		}
	}
	return fmt.Errorf("unknown field %s for %s", key, docType)
}
