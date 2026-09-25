package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateWritesConsistentFixtures(t *testing.T) {
	out := t.TempDir()
	if err := generate(out); err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenarios {
		raw, err := os.ReadFile(filepath.Join(out, "fixtures", sc.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var fx fixture
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatal(err)
		}
		if len(fx.Documents) != len(sc.Docs) {
			t.Fatalf("%s: %d docs", sc.ID, len(fx.Documents))
		}
		for _, d := range fx.Documents {
			info, err := os.Stat(filepath.Join(out, d.File))
			if err != nil || info.Size() < 500 {
				t.Fatalf("%s: bad file %s (%v)", sc.ID, d.File, err)
			}
		}
	}
}

func TestPDFKeywordsMatchPrintedFields(t *testing.T) {
	out := t.TempDir()
	if err := generate(out); err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenarios {
		for _, d := range sc.Docs {
			raw, err := os.ReadFile(filepath.Join(out, "documents", sc.ID, d.Name))
			if err != nil {
				t.Fatal(err)
			}
			marker := []byte("HARBOR-FIELDS:")
			start := bytes.Index(raw, marker)
			if start < 0 {
				t.Fatalf("%s/%s: missing marker", sc.ID, d.Name)
			}
			payload := raw[start+len(marker):]
			end := bytes.IndexByte(payload, ')')
			if end < 0 {
				t.Fatalf("%s/%s: unterminated marker", sc.ID, d.Name)
			}
			var values map[string]any
			if err := json.Unmarshal(payload[:end], &values); err != nil {
				t.Fatal(err)
			}
			if values["doc_type"] != d.Type {
				t.Fatalf("%s: wrong type %v", d.Name, values["doc_type"])
			}
			delete(values, "doc_type")
			want, _ := json.Marshal(d.Fields)
			got, _ := json.Marshal(values)
			if !bytes.Equal(want, got) {
				t.Fatalf("%s: got %s want %s", d.Name, got, want)
			}
			for _, l := range lines(d)[1:] {
				if l != "" && !d.LowQ && !strings.Contains(string(raw), strings.NewReplacer("(", `\(`, ")", `\)`).Replace(l)) {
					t.Fatalf("%s: printed line missing %q", d.Name, l)
				}
			}
		}
	}
}

func TestDemoBorrowersAreDistinctAndConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, sc := range scenarios {
		byType := map[string]docSpec{}
		for _, d := range sc.Docs {
			byType[d.Type] = d
			if d.Holder == "" {
				t.Fatalf("%s/%s missing borrower", sc.ID, d.Name)
			}
		}
		borrower := byType["form_1003"].Fields["borrower_name"].(string)
		for _, d := range sc.Docs {
			if d.Holder != borrower {
				t.Fatalf("%s/%s borrower mismatch", sc.ID, d.Name)
			}
		}
		if seen[borrower] {
			t.Fatalf("reused borrower %s", borrower)
		}
		seen[borrower] = true
		if byType["w2"].Fields["employer_name"] != byType["pay_stub"].Fields["employer_name"] {
			t.Fatalf("%s employer mismatch", sc.ID)
		}
		if byType["bank_statement"].Fields["monthly_debt"] != sc.Debt {
			t.Fatalf("%s debt mismatch", sc.ID)
		}
		if byType["pay_stub"].Fields["monthly_income"] != sc.Income {
			t.Fatalf("%s income mismatch", sc.ID)
		}
	}
	if len(scenarios) != 7 {
		t.Fatalf("need seven scenarios, got %d", len(scenarios))
	}
}

func TestSyntheticNamesAvoidRealBrands(t *testing.T) {
	banned := []string{"harbor", "harbour", "harborview", "northwind", "pinecrest", "summit", "cedar", "linden", "elm grove", "contoso", "fabrikam", "coastal"}
	payeeLine := regexp.MustCompile(`^\d{2}/\d{2} \| ([^|]+) \| [+-]\$`)
	for _, sc := range append(append([]scenario{}, scenarios...), demoSets...) {
		check := func(role, value string, organization bool) {
			t.Helper()
			lower := strings.ToLower(value)
			for _, word := range banned {
				if strings.Contains(lower, word) {
					t.Errorf("%s %s %q contains forbidden name %q", sc.ID, role, value, word)
				}
			}
			if organization && !(strings.HasPrefix(value, "Example ") || strings.HasPrefix(value, "Sample ") || strings.HasPrefix(value, "Demo ")) {
				t.Errorf("%s %s %q lacks fictional prefix", sc.ID, role, value)
			}
		}
		for _, d := range sc.Docs {
			check("borrower", d.Holder, false)
			switch d.Type {
			case "w2", "pay_stub":
				check("employer", d.Fields["employer_name"].(string), true)
			case "form_1003":
				check("borrower", d.Fields["borrower_name"].(string), false)
			case "bank_statement":
				check("bank", d.Fields["bank_name"].(string), true)
				for _, line := range lines(d) {
					if match := payeeLine.FindStringSubmatch(line); match != nil {
						check("payee", strings.TrimSpace(match[1]), true)
					}
				}
			}
		}
	}
}

func TestBankStatementsReconcileToNetPayroll(t *testing.T) {
	cents := func(v any) int64 { return int64(math.Round(v.(float64) * 100)) }
	for _, sc := range scenarios {
		var bank, pay docSpec
		for _, d := range sc.Docs {
			switch d.Type {
			case "bank_statement":
				bank = d
			case "pay_stub":
				pay = d
			}
		}
		b := bank.Fields
		if b["total_withdrawals"] == nil {
			t.Fatalf("%s: missing withdrawals", sc.ID)
		}
		if got, want := cents(b["beginning_balance"])+cents(b["total_deposits"])-cents(b["total_withdrawals"]), cents(b["ending_balance"]); got != want {
			if sc.ID != "tampered_statement" || got-want != 400000 {
				t.Errorf("%s: balance %d != %d cents", sc.ID, got, want)
			}
		} else if sc.ID == "tampered_statement" {
			t.Fatal("tampered statement reconciles")
		}
		if cents(b["total_deposits"]) >= cents(pay.Fields["monthly_income"]) {
			t.Errorf("%s: deposits not below gross pay", sc.ID)
		}
		if cents(b["total_withdrawals"]) < cents(b["monthly_debt"]) {
			t.Errorf("%s: debt exceeds all withdrawals", sc.ID)
		}
		payroll := fmt.Sprintf("Payroll deposits (2 x $%s net)", commas(b["total_deposits"].(float64)/2))
		netPay := fmt.Sprintf("%-34s $%s", "Net pay this period", commas(b["total_deposits"].(float64)/2))
		if !strings.Contains(strings.Join(lines(pay), "\n"), netPay) {
			t.Errorf("%s: pay stub net pay disagrees with deposits", sc.ID)
		}
		if !strings.Contains(strings.Join(lines(bank), "\n"), payroll) {
			t.Errorf("%s: missing two net payroll deposits %q", sc.ID, payroll)
		}
	}
}

func TestPrintedTransactionsSupportSummaryFields(t *testing.T) {
	row := regexp.MustCompile(`^(\d{2}/\d{2}) \| ((?:Example|Sample|Demo) [^|]+) \| ([+-])\$([\d,]+\.\d{2})$`)
	for _, sc := range scenarios {
		var bank, pay docSpec
		for _, d := range sc.Docs {
			switch d.Type {
			case "bank_statement":
				bank = d
			case "pay_stub":
				pay = d
			}
		}
		payText := strings.Join(lines(pay), "\n")
		if !strings.Contains(payText, "Pay frequency: Semimonthly") || !strings.Contains(payText, "Pay periods to date: 16") {
			t.Errorf("%s: pay stub lacks pay-frequency evidence", sc.ID)
		}
		bankText := lines(bank)
		if !strings.Contains(strings.Join(bankText, "\n"), "Transaction register") {
			t.Errorf("%s: no transaction register", sc.ID)
		}
		var deposits, payroll, debt, withdrawals int64
		var payrollCount, bnplCount int
		for _, line := range bankText {
			m := row.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			amount, err := strconv.ParseFloat(strings.ReplaceAll(m[4], ",", ""), 64)
			if err != nil {
				t.Fatal(err)
			}
			cents := int64(math.Round(amount * 100))
			if m[3] == "+" {
				deposits += cents
				if strings.Contains(m[2], "Payroll") {
					payroll += cents
					payrollCount++
				}
			} else {
				withdrawals += cents
				if strings.Contains(m[2], "(debt)") {
					debt += cents
				}
				if strings.Contains(m[2], "BNPL") {
					bnplCount++
				}
			}
		}
		asCents := func(v any) int64 { return int64(math.Round(v.(float64) * 100)) }
		if deposits != asCents(bank.Fields["total_deposits"]) || payroll != deposits || payrollCount != 2 ||
			payroll/2 != asCents(pay.Fields["gross_pay"].(float64)*.76) ||
			debt != asCents(bank.Fields["monthly_debt"]) || withdrawals != asCents(bank.Fields["total_withdrawals"]) ||
			bnplCount != int(bank.Fields["bnpl_hits"].(float64)) {
			t.Errorf("%s: register deposits=%d payroll=%d (%d) debt=%d withdrawals=%d BNPL=%d", sc.ID, deposits, payroll, payrollCount, debt, withdrawals, bnplCount)
		}
		ending := asCents(bank.Fields["beginning_balance"]) + deposits - withdrawals
		if sc.ID == "tampered_statement" {
			if ending-asCents(bank.Fields["ending_balance"]) != 400000 {
				t.Errorf("%s: tampered ending discrepancy = %d", sc.ID, ending-asCents(bank.Fields["ending_balance"]))
			}
		} else if ending != asCents(bank.Fields["ending_balance"]) {
			t.Errorf("%s: register does not reconcile to ending balance", sc.ID)
		}
	}
}

func TestLowQualityScanFitsWholeRegister(t *testing.T) {
	out := t.TempDir()
	if err := generate(out); err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenarios {
		for _, d := range sc.Docs {
			if !d.LowQ {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(out, "documents", sc.ID, d.Name))
			if err != nil {
				t.Fatal(err)
			}
			wantHeight := 18 + (len(lines(d))+2)*15
			if !bytes.Contains(raw, []byte(fmt.Sprintf("/Height %d", wantHeight))) {
				t.Fatalf("%s: scan clipped below register (need height %d)", sc.ID, wantHeight)
			}
		}
	}
}

func TestLowQualityBlurTargetsOnlyEndingBalanceTensDigit(t *testing.T) {
	var bank docSpec
	for _, d := range scenarios[0].Docs {
		if d.Type == "bank_statement" {
			bank = d
		}
	}
	body := lines(bank)
	img := image.NewGray(image.Rect(0, 0, 520, 300))
	for i := range img.Pix {
		img.Pix[i] = 225
	}
	drawer := &font.Drawer{Dst: img, Src: &image.Uniform{color.Gray{Y: 70}}, Face: basicfont.Face7x13}
	for i, l := range body {
		drawer.Dot = fixed.P(12, 18+i*15)
		drawer.DrawString(l)
	}
	original := append([]byte(nil), img.Pix...)
	obscureEndingBalance(img, body)
	var changed int
	for i, v := range img.Pix {
		if v != original[i] {
			changed++
			x, y := i%img.Stride, i/img.Stride
			if !image.Pt(x, y).In(endingDigitBounds(body)) {
				t.Fatalf("changed unrelated pixel at %d,%d", x, y)
			}
		}
	}
	if changed < 10 {
		t.Fatalf("ending digit not degraded: %d pixels", changed)
	}
}

func TestTamperedStatementPrintsAlteredBalanceInDifferentFont(t *testing.T) {
	out := t.TempDir()
	if err := generate(out); err != nil {
		t.Fatal(err)
	}
	var tampered scenario
	for _, sc := range scenarios {
		if sc.ID == "tampered_statement" {
			tampered = sc
		}
	}
	if len(tampered.Docs) != 5 || tampered.Expected.Recommendation != "needs_review" || !tampered.Expected.ShouldFlag {
		t.Fatalf("scenario: %+v", tampered)
	}
	for _, d := range tampered.Docs {
		if d.Type != "bank_statement" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(out, "documents", tampered.ID, d.Name))
		if err != nil {
			t.Fatal(err)
		}
		printed := fmt.Sprintf("%-34s $%s", "Ending balance", commas(d.Fields["ending_balance"].(float64)))
		if !bytes.Contains(raw, []byte(printed)) || !bytes.Contains(raw, []byte("/Helvetica")) {
			t.Fatalf("balance not printed in altered font: %s", d.Name)
		}
		return
	}
	t.Fatal("missing bank statement")
}

func TestExpectedDTIMatchesInputs(t *testing.T) {
	for _, sc := range scenarios {
		if sc.Expected.DTI == nil {
			continue
		}
		got := round4(sc.Debt / sc.Income)
		if got != *sc.Expected.DTI {
			t.Fatalf("%s: expected %v computed %v", sc.ID, *sc.Expected.DTI, got)
		}
	}
}

func TestLiveDemoSetIsRayTienWithoutFixture(t *testing.T) {
	out := t.TempDir()
	if err := generate(out); err != nil {
		t.Fatal(err)
	}
	if len(demoSets) != 1 || demoSets[0].ID != "live_demo" || len(demoSets[0].Docs) != 5 {
		t.Fatalf("demo sets: %+v", demoSets)
	}
	lowq := false
	for _, d := range demoSets[0].Docs {
		lowq = lowq || d.LowQ
		raw, err := os.ReadFile(filepath.Join(out, "documents", "live_demo", d.Name))
		if err != nil {
			t.Fatal(err)
		}
		// The degraded scan is an image; check the lines it renders instead of the bytes.
		named := bytes.Contains(raw, []byte("Ray Tien")) || (d.LowQ && strings.Contains(strings.Join(lines(d), "\n"), "Borrower: Ray Tien"))
		if d.Holder != "Ray Tien" || !named {
			t.Fatalf("%s does not name Ray Tien", d.Name)
		}
	}
	if !lowq {
		t.Fatal("live demo must carry the degraded bank statement")
	}
	if _, err := os.Stat(filepath.Join(out, "fixtures", "live_demo.json")); !os.IsNotExist(err) {
		t.Fatalf("live_demo must not be an eval fixture: %v", err)
	}
}
