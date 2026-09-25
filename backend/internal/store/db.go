package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, name TEXT NOT NULL,
  title TEXT NOT NULL, password_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), expires_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS cases (
  id TEXT PRIMARY KEY, borrower_name TEXT NOT NULL, loan_number TEXT NOT NULL,
  loan_product TEXT NOT NULL, requested_amount REAL NOT NULL,
  status TEXT NOT NULL DEFAULT 'processing', created_at INTEGER NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS cases_loan_number ON cases(loan_number);
CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY, case_id TEXT NOT NULL REFERENCES cases(id), file_name TEXT NOT NULL,
  file_path TEXT NOT NULL, doc_type TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
  failure_reason TEXT NOT NULL DEFAULT '', uploaded_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS fields (
  document_id TEXT NOT NULL REFERENCES documents(id), key TEXT NOT NULL, value TEXT NOT NULL,
  flagged INTEGER NOT NULL DEFAULT 0, flag_reason TEXT NOT NULL DEFAULT '', edited INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (document_id, key));
CREATE TABLE IF NOT EXISTS judgments (
  owner_id TEXT NOT NULL, name TEXT NOT NULL, score REAL NOT NULL, reason TEXT NOT NULL,
  PRIMARY KEY (owner_id, name));
CREATE TABLE IF NOT EXISTS assessments (
  case_id TEXT PRIMARY KEY REFERENCES cases(id), monthly_income REAL, monthly_debt REAL, dti REAL,
  recommendation TEXT NOT NULL, reasons TEXT NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT, case_id TEXT NOT NULL REFERENCES cases(id), user_id TEXT NOT NULL,
  action TEXT NOT NULL, note TEXT NOT NULL, created_at INTEGER NOT NULL);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Close() error { return s.db.Close() }
