package schemas

import "testing"

func validBank() map[string]any {
	return map[string]any{"bank_name": "Chase", "account_last4": "4471", "statement_period": "2026-08",
		"beginning_balance": 14210.55, "ending_balance": 18482.0, "total_deposits": 7412.2,
		"monthly_debt": 3082.0, "nsf_count": float64(0), "bnpl_hits": float64(2)}
}

func TestValidateAcceptsValid(t *testing.T) {
	if errs := Validate(BankStatement, validBank()); len(errs) != 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
}

func TestValidateReportsMissingAndWrongKinds(t *testing.T) {
	v := validBank()
	delete(v, "ending_balance")
	v["nsf_count"] = 1.5
	v["bank_name"] = 42.0
	errs := Validate(BankStatement, v)
	if len(errs) != 3 {
		t.Fatalf("want 3 errors got %v", errs)
	}
}

func TestOptionalBankWithdrawals(t *testing.T) {
	v := validBank()
	if errs := Validate(BankStatement, v); len(errs) != 0 {
		t.Fatalf("legacy statement: %v", errs)
	}
	v["total_withdrawals"] = 1200.55
	if errs := Validate(BankStatement, v); len(errs) != 0 {
		t.Fatalf("withdrawals present: %v", errs)
	}
	v["total_withdrawals"] = "not a number"
	if errs := Validate(BankStatement, v); len(errs) != 1 {
		t.Fatalf("withdrawals must be numeric: %v", errs)
	}
}

func TestValidateRejectsUnknownType(t *testing.T) {
	if errs := Validate("passport", map[string]any{}); len(errs) != 1 {
		t.Fatalf("want 1 error got %v", errs)
	}
}

func TestEveryTypeHasSchemaAndLabel(t *testing.T) {
	if len(Types) != 5 {
		t.Fatalf("exactly 5 types required, got %d", len(Types))
	}
	for _, ty := range Types {
		if len(Registry[ty]) == 0 || Label(ty) == ty {
			t.Fatalf("type %s missing schema or label", ty)
		}
	}
}

func TestCheckValue(t *testing.T) {
	if err := CheckValue(BankStatement, "ending_balance", 18432.0); err != nil {
		t.Fatal(err)
	}
	if err := CheckValue(BankStatement, "ending_balance", "abc"); err == nil {
		t.Fatal("want kind error")
	}
	if err := CheckValue(BankStatement, "nope", 1.0); err == nil {
		t.Fatal("want unknown field error")
	}
}
