package fake

import (
	"context"
	"fmt"
	"github.com/go-pdf/fpdf"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkedPDFFeedsClassifierAndExtractor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mystery.pdf")
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetKeywords("HARBOR-FIELDS:{\"doc_type\":\"bank_statement\",\"monthly_debt\":4200}", false)
	pdf.AddPage()
	if err := pdf.OutputFileAndClose(path); err != nil {
		t.Fatal(err)
	}
	text, err := new(Parser).Parse(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	kind, _, err := (Classifier{}).Classify(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "bank_statement" {
		t.Fatalf("type %s", kind)
	}
	ex, err := (Extractor{}).Extract(context.Background(), kind, text)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Fields["monthly_debt"] != 4200.0 {
		t.Fatalf("fields: %v", ex.Fields)
	}
	if _, ok := ex.Fields["doc_type"]; ok {
		t.Fatal("doc_type leaked into extracted fields")
	}
}
func TestJudgeFlagsOnlyInconsistentMarkedBankBalance(t *testing.T) {
	for _, tc := range []struct{ ending, want float64 }{{140, .91}, {135, .3}} {
		text := `HARBOR-FIELDS:{"doc_type":"bank_statement","beginning_balance":100,"total_deposits":50,"total_withdrawals":10,"ending_balance":` + fmt.Sprint(tc.ending) + `}`
		js, err := (Judge{}).JudgeDocument(context.Background(), "bank_statement", text, nil)
		if err != nil {
			t.Fatal(err)
		}
		if js[1].Score != tc.want {
			t.Fatalf("ending %v: authenticity %v, want %v", tc.ending, js[1].Score, tc.want)
		}
	}
	js, err := (Judge{}).JudgeDocument(context.Background(), "bank_statement", "bank-statement.pdf", nil)
	if err != nil || js[1].Score != .91 {
		t.Fatalf("unmarked: %v %v", js, err)
	}
}

func TestUnmarkedFileStillUsesCannedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bank-statement.pdf")
	if err := os.WriteFile(path, []byte("old upload"), 0600); err != nil {
		t.Fatal(err)
	}
	text, err := new(Parser).Parse(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := (Extractor{}).Extract(context.Background(), "bank_statement", text)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Fields["monthly_debt"] != 3082.0 {
		t.Fatalf("fallback: %v", ex.Fields)
	}
}
