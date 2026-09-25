package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/schemas"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Case struct {
	ID              string  `json:"id"`
	BorrowerName    string  `json:"borrower_name"`
	LoanNumber      string  `json:"loan_number"`
	LoanProduct     string  `json:"loan_product"`
	RequestedAmount float64 `json:"requested_amount"`
	Status          string  `json:"status"`
	CreatedAt       int64   `json:"created_at"`
}

type CaseSummary struct {
	Case
	Recommendation string `json:"recommendation"`
	DocCount       int    `json:"doc_count"`
	Blocker        string `json:"blocker"`
}

type Document struct {
	ID            string `json:"id"`
	CaseID        string `json:"case_id"`
	FileName      string `json:"file_name"`
	FilePath      string `json:"-"`
	DocType       string `json:"doc_type"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason"`
	UploadedAt    int64  `json:"uploaded_at"`
}

var ErrDuplicateLoanNumber = errors.New("duplicate loan number")
var ErrAlreadyDecided = errors.New("case cannot be decided again")

func newID() string { return auth.NewToken()[:20] }

func (s *Store) CreateCase(c Case) (Case, error) {
	c.ID, c.Status, c.CreatedAt = newID(), "processing", time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO cases (id, borrower_name, loan_number, loan_product, requested_amount, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, c.ID, c.BorrowerName, c.LoanNumber, c.LoanProduct, c.RequestedAmount, c.Status, c.CreatedAt)
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return c, ErrDuplicateLoanNumber
	}
	return c, err
}

func (s *Store) GetCase(id string) (Case, error) {
	var c Case
	err := s.db.QueryRow(`SELECT id, borrower_name, loan_number, loan_product, requested_amount, status, created_at
		FROM cases WHERE id = ?`, id).Scan(&c.ID, &c.BorrowerName, &c.LoanNumber, &c.LoanProduct, &c.RequestedAmount, &c.Status, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) ListCases() ([]CaseSummary, error) {
	rows, err := s.db.Query(`SELECT c.id, c.borrower_name, c.loan_number, c.loan_product, c.requested_amount, c.status, c.created_at,
		COALESCE(a.recommendation, ''), (SELECT count(*) FROM documents d WHERE d.case_id = c.id AND d.status != 'superseded')
		FROM cases c LEFT JOIN assessments a ON a.case_id = c.id ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaseSummary{}
	for rows.Next() {
		var cs CaseSummary
		if err := rows.Scan(&cs.ID, &cs.BorrowerName, &cs.LoanNumber, &cs.LoanProduct, &cs.RequestedAmount, &cs.Status,
			&cs.CreatedAt, &cs.Recommendation, &cs.DocCount); err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *Store) CreateDocument(d Document) (Document, error) {
	if d.ID == "" {
		d.ID = newID()
	}
	d.Status, d.UploadedAt = "pending", time.Now().Unix()
	_, err := s.db.Exec(`INSERT INTO documents (id, case_id, file_name, file_path, status, uploaded_at) VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.CaseID, d.FileName, d.FilePath, d.Status, d.UploadedAt)
	if err == nil {
		_, err = s.db.Exec(`UPDATE cases SET status = 'processing' WHERE id = ? AND status IN ('needs_review', 'ready')`, d.CaseID)
	}
	return d, err
}

func scanDocument(row interface{ Scan(...any) error }) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.CaseID, &d.FileName, &d.FilePath, &d.DocType, &d.Status, &d.FailureReason, &d.UploadedAt)
	return d, err
}

const docCols = `id, case_id, file_name, file_path, doc_type, status, failure_reason, uploaded_at`

func (s *Store) GetDocument(id string) (Document, error) {
	d, err := scanDocument(s.db.QueryRow(`SELECT `+docCols+` FROM documents WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *Store) ListDocuments(caseID string) ([]Document, error) {
	rows, err := s.db.Query(`SELECT `+docCols+` FROM documents WHERE case_id = ? ORDER BY uploaded_at, id`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type Field struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Value      any    `json:"value"`
	Flagged    bool   `json:"flagged"`
	FlagReason string `json:"flag_reason"`
	Edited     bool   `json:"edited"`
}

type Judgment struct {
	Name         string  `json:"name"`
	Score        float64 `json:"score"`
	Low          bool    `json:"low"`
	Reason       string  `json:"reason"`
	DocumentType string  `json:"-"`
}

type DocumentDetail struct {
	Document
	Fields    []Field    `json:"fields"`
	Judgments []Judgment `json:"judgments"`
}

type Assessment struct {
	MonthlyIncome  *float64 `json:"monthly_income"`
	MonthlyDebt    *float64 `json:"monthly_debt"`
	DTI            *float64 `json:"dti"`
	Recommendation string   `json:"recommendation"`
	Reasons        []string `json:"reasons"`
}

type Detail struct {
	Case          Case             `json:"case"`
	Documents     []DocumentDetail `json:"documents"`
	CaseJudgments []Judgment       `json:"case_judgments"`
	Assessment    *Assessment      `json:"assessment"`
}

func (s *Store) CaseDetail(id string) (Detail, error) {
	var d Detail
	c, err := s.GetCase(id)
	if err != nil {
		return d, err
	}
	d.Case = c
	docs, err := s.ListDocuments(id)
	if err != nil {
		return d, err
	}
	d.Documents = []DocumentDetail{}
	for _, doc := range docs {
		fields, err := s.fieldsFor(doc.ID, doc.DocType)
		if err != nil {
			return d, err
		}
		js, err := s.judgmentsFor(doc.ID)
		if err != nil {
			return d, err
		}
		d.Documents = append(d.Documents, DocumentDetail{Document: doc, Fields: fields, Judgments: js})
	}
	if d.CaseJudgments, err = s.judgmentsFor(id); err != nil {
		return d, err
	}
	if d.Assessment, err = s.assessment(id); err != nil {
		return d, err
	}
	return d, nil
}

func (s *Store) SetDocumentStatus(id, status, reason string) error {
	_, err := s.db.Exec(`UPDATE documents SET status = ?, failure_reason = ? WHERE id = ? AND status != 'superseded'`, status, reason, id)
	return err
}

func (s *Store) SetDocumentType(id, docType string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE documents SET doc_type = ? WHERE id = ?`, docType, id); err != nil {
		return err
	}
	if docType != schemas.Other {
		if _, err := tx.Exec(`UPDATE documents AS old SET status = 'superseded'
			WHERE old.case_id = (SELECT case_id FROM documents WHERE id = ?) AND old.doc_type = ?
			AND old.rowid < (SELECT max(newer.rowid) FROM documents AS newer
				WHERE newer.case_id = old.case_id AND newer.doc_type = old.doc_type)`, id, docType); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveExtraction(docID string, fields map[string]any, uncertain map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM fields WHERE document_id = ?`, docID); err != nil {
		return err
	}
	for k, v := range fields {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		reason, flagged := uncertain[k]
		if _, err := tx.Exec(`INSERT INTO fields (document_id, key, value, flagged, flag_reason) VALUES (?, ?, ?, ?, ?)`,
			docID, k, string(raw), flagged, reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveJudgments(ownerID string, js []Judgment) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM judgments WHERE owner_id = ?`, ownerID); err != nil {
		return err
	}
	for _, j := range js {
		if _, err := tx.Exec(`INSERT INTO judgments (owner_id, name, score, reason) VALUES (?, ?, ?, ?)`,
			ownerID, j.Name, j.Score, j.Reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) fieldsFor(docID, docType string) ([]Field, error) {
	rows, err := s.db.Query(`SELECT key, value, flagged, flag_reason, edited FROM fields WHERE document_id = ?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]Field{}
	for rows.Next() {
		var f Field
		var raw string
		if err := rows.Scan(&f.Key, &raw, &f.Flagged, &f.FlagReason, &f.Edited); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &f.Value); err != nil {
			return nil, err
		}
		byKey[f.Key] = f
	}
	out := []Field{}
	for _, spec := range schemas.Registry[docType] {
		if f, ok := byKey[spec.Key]; ok {
			f.Label = spec.Label
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

func (s *Store) judgmentsFor(ownerID string) ([]Judgment, error) {
	rows, err := s.db.Query(`SELECT name, score, reason FROM judgments WHERE owner_id = ? ORDER BY score DESC, name`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Judgment{}
	for rows.Next() {
		var j Judgment
		if err := rows.Scan(&j.Name, &j.Score, &j.Reason); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) SaveAssessment(caseID string, a Assessment) error {
	reasons, _ := json.Marshal(a.Reasons)
	_, err := s.db.Exec(`INSERT INTO assessments (case_id, monthly_income, monthly_debt, dti, recommendation, reasons, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(case_id) DO UPDATE SET monthly_income = excluded.monthly_income,
		monthly_debt = excluded.monthly_debt, dti = excluded.dti, recommendation = excluded.recommendation,
		reasons = excluded.reasons, updated_at = excluded.updated_at`,
		caseID, a.MonthlyIncome, a.MonthlyDebt, a.DTI, a.Recommendation, string(reasons), time.Now().Unix())
	return err
}

func (s *Store) assessment(caseID string) (*Assessment, error) {
	var a Assessment
	var reasons string
	err := s.db.QueryRow(`SELECT monthly_income, monthly_debt, dti, recommendation, reasons FROM assessments WHERE case_id = ?`, caseID).
		Scan(&a.MonthlyIncome, &a.MonthlyDebt, &a.DTI, &a.Recommendation, &reasons)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, json.Unmarshal([]byte(reasons), &a.Reasons)
}

func (s *Store) SetCaseStatus(caseID, status string) error {
	_, err := s.db.Exec(`UPDATE cases SET status = ? WHERE id = ?`, status, caseID)
	return err
}

func (s *Store) ActiveDocuments(caseID string) ([]Document, error) {
	all, err := s.ListDocuments(caseID)
	if err != nil {
		return nil, err
	}
	out := []Document{}
	for _, d := range all {
		if d.Status != "superseded" {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *Store) FieldValues(docID string) (map[string]any, error) {
	rows, err := s.db.Query(`SELECT key, value FROM fields WHERE document_id = ?`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var k, raw string
		var v any
		if err := rows.Scan(&k, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) UnresolvedFlags(caseID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM fields f JOIN documents d ON d.id = f.document_id
		WHERE d.case_id = ? AND d.status != 'superseded' AND f.flagged = 1 AND f.edited = 0`, caseID).Scan(&n)
	return n, err
}

func (s *Store) AllJudgments(caseID string) ([]Judgment, error) {
	out, err := s.judgmentsFor(caseID)
	if err != nil {
		return nil, err
	}
	docs, err := s.ActiveDocuments(caseID)
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		js, err := s.judgmentsFor(d.ID)
		if err != nil {
			return nil, err
		}
		for _, j := range js {
			j.DocumentType = d.DocType
			out = append(out, j)
		}
	}
	return out, nil
}

type AuditEntry struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	Note      string `json:"note"`
	UserName  string `json:"user_name"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) UpdateField(docID, key string, value any) (any, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM fields WHERE document_id = ? AND key = ?`, docID, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var old any
	_ = json.Unmarshal([]byte(raw), &old)
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`UPDATE fields SET value = ?, edited = 1 WHERE document_id = ? AND key = ?`, string(b), docID, key)
	return old, err
}

func (s *Store) DecideCase(caseID, userID, status, note string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE cases SET status = ? WHERE id = ? AND (status IN ('needs_review', 'ready') OR (status = 'processing' AND ? IN ('rejected', 'sent_back')))`, status, caseID, status)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		var exists int
		err := tx.QueryRow(`SELECT 1 FROM cases WHERE id = ?`, caseID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return ErrAlreadyDecided
	}
	if _, err := tx.Exec(`INSERT INTO audit_log (case_id, user_id, action, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		caseID, userID, status, note, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AppendAudit(caseID, userID, action, note string) error {
	_, err := s.db.Exec(`INSERT INTO audit_log (case_id, user_id, action, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		caseID, userID, action, note, time.Now().Unix())
	return err
}

func (s *Store) AuditLog(caseID string) ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT a.id, a.action, a.note, u.name, a.created_at FROM audit_log a JOIN users u ON u.id = a.user_id
		WHERE a.case_id = ? ORDER BY a.id DESC`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.Note, &e.UserName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
