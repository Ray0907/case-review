package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestExistingDatabaseGetsOptionalSourceColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE fields (document_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, flagged INTEGER NOT NULL DEFAULT 0, flag_reason TEXT NOT NULL DEFAULT '', edited INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(document_id,key)); INSERT INTO fields(document_id,key,value) VALUES ('old','ending_balance','42')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var value, source string
		err = s.DB().QueryRow(`SELECT value, source FROM fields WHERE document_id='old'`).Scan(&value, &source)
		s.Close()
		if err != nil || value != "42" || source != "null" {
			t.Fatalf("migration: value=%q source=%q err=%v", value, source, err)
		}
	}
}
