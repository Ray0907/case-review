package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSetDocumentTypeSupersedesOlder(t *testing.T) {
	s := newStore(t)
	c, _ := s.CreateCase(Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	old, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "a.pdf", FilePath: "/x"})
	s.SetDocumentType(old.ID, "bank_statement")
	s.SetDocumentStatus(old.ID, "done", "")
	newer, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "b.pdf", FilePath: "/y"})
	if err := s.SetDocumentType(newer.ID, "bank_statement"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDocument(old.ID)
	if got.Status != "superseded" {
		t.Fatalf("older doc status %q, want superseded", got.Status)
	}
	if n, _ := s.GetDocument(newer.ID); n.Status == "superseded" {
		t.Fatal("newer doc must not be superseded")
	}
}

func TestSetDocumentTypeKeepsNewestWhenOlderClassifiesLast(t *testing.T) {
	s := newStore(t)
	c, _ := s.CreateCase(Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	old, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "a.pdf", FilePath: "/x"})
	newer, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "b.pdf", FilePath: "/y"})
	if err := s.SetDocumentType(newer.ID, "bank_statement"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDocumentType(old.ID, "bank_statement"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDocument(newer.ID)
	if got.Status == "superseded" {
		t.Fatal("newest document superseded by slow older classification")
	}
	got, _ = s.GetDocument(old.ID)
	if got.Status != "superseded" {
		t.Fatalf("older document status %q, want superseded", got.Status)
	}
}

func TestDecideCaseOnlyOnce(t *testing.T) {
	s := newStore(t)
	if err := s.EnsureUser("reviewer@casereview.test", "Maya Park", "Underwriter", "hash"); err != nil {
		t.Fatal(err)
	}
	u, _, err := s.UserByEmail("reviewer@casereview.test")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCase(Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCaseStatus(c.ID, "ready"); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideCase(c.ID, u.ID, "approved", "verified"); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideCase(c.ID, u.ID, "approved", "again"); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("second decision: %v, want ErrAlreadyDecided", err)
	}
	if err := s.DecideCase("missing", u.ID, "approved", "no case"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing case: %v, want ErrNotFound", err)
	}
	entries, err := s.AuditLog(c.ID)
	if err != nil || len(entries) != 1 || entries[0].Action != "approved" || entries[0].Note != "verified" {
		t.Fatalf("audit %+v err %v", entries, err)
	}
}

func TestSaveExtractionReplacesRows(t *testing.T) {
	s := newStore(t)
	c, _ := s.CreateCase(Case{BorrowerName: "A", LoanNumber: "1", LoanProduct: "p", RequestedAmount: 1})
	d, _ := s.CreateDocument(Document{CaseID: c.ID, FileName: "a.pdf", FilePath: "/x"})
	s.SetDocumentType(d.ID, "bank_statement")
	s.SaveExtraction(d.ID, map[string]any{"ending_balance": 1.0, "bank_name": "Chase"}, map[string]string{"ending_balance": "blurry"})
	if err := s.SaveExtraction(d.ID, map[string]any{"ending_balance": 2.0}, nil); err != nil {
		t.Fatal(err)
	}
	detail, _ := s.CaseDetail(c.ID)
	fields := detail.Documents[0].Fields
	if len(fields) != 1 || fields[0].Value != 2.0 || fields[0].Flagged {
		t.Fatalf("fields %+v", fields)
	}
}
