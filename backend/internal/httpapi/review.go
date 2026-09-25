package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

var decided = map[string]bool{"approved": true, "rejected": true, "sent_back": true}

func (s *Server) editField(w http.ResponseWriter, r *http.Request) {
	doc, err := s.store.GetDocument(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if doc.Status == "superseded" {
		writeError(w, http.StatusConflict, "this document was replaced by a newer upload; edit the current one")
		return
	}
	c, err := s.store.GetCase(doc.CaseID)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	if decided[c.Status] {
		writeError(w, http.StatusConflict, "this case already has a decision; fields are locked")
		return
	}
	var in struct {
		Value json.RawMessage `json:"value"`
	}
	if err := readJSON(r, &in); err != nil || len(in.Value) == 0 {
		writeError(w, http.StatusBadRequest, "send {\"value\": ...}")
		return
	}
	var value any
	if err := json.Unmarshal(in.Value, &value); err != nil {
		writeError(w, http.StatusBadRequest, "value must be valid JSON")
		return
	}
	key := r.PathValue("key")
	if err := schemas.CheckValue(doc.DocType, key, value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	old, err := s.store.UpdateField(doc.ID, key, value)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "field not found on this document")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save field")
		return
	}
	label := key
	money := false
	for _, spec := range schemas.Registry[doc.DocType] {
		if spec.Key == key {
			label, money = spec.Label, spec.Kind == schemas.KindNumber
			break
		}
	}
	format := func(v any) string {
		if n, ok := v.(float64); ok && money {
			s := fmt.Sprintf("%.2f", n)
			parts := strings.SplitN(s, ".", 2)
			for i := len(parts[0]) - 3; i > 0 && parts[0][i-1] != '-'; i -= 3 {
				parts[0] = parts[0][:i] + "," + parts[0][i:]
			}
			return "$" + parts[0] + "." + parts[1]
		}
		return fmt.Sprint(v)
	}
	note := fmt.Sprintf("%s · %s: %s", schemas.Label(doc.DocType), label, format(old))
	if format(old) == format(value) {
		note += " confirmed"
	} else {
		note += " → " + format(value)
	}
	if err := s.store.AppendAudit(c.ID, userFrom(r).ID, "field_edited", note); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write audit log")
		return
	}
	if err := s.opts.Runner.Refresh(r.Context(), c.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "field saved but assessment refresh failed")
		return
	}
	s.getCaseByID(w, c.ID)
}

var actions = map[string]string{"approve": "approved", "reject": "rejected", "send_back": "sent_back"}

func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	c, err := s.store.GetCase(caseID)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	var in struct{ Action, Note string }
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status, ok := actions[in.Action]
	if !ok {
		writeError(w, http.StatusBadRequest, "action must be approve, reject or send_back")
		return
	}
	if decided[c.Status] {
		writeError(w, http.StatusConflict, "this case already has a decision")
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if in.Action != "approve" && in.Note == "" {
		writeError(w, http.StatusBadRequest, "add a note explaining the decision")
		return
	}
	if in.Action == "approve" {
		docs, err := s.store.ActiveDocuments(caseID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check documents")
			return
		}
		present := map[string]bool{}
		failed, processing := false, c.Status == "processing"
		for _, d := range docs {
			switch d.Status {
			case "done":
				present[d.DocType] = true
			case "failed":
				failed = true
			case "unsupported":
			default:
				processing = true
			}
		}
		if processing {
			writeError(w, http.StatusConflict, "Wait for document processing to finish.")
			return
		}
		if failed {
			writeError(w, http.StatusConflict, "Retry the failed document before approving.")
			return
		}
		var missing []string
		for _, kind := range schemas.Types {
			if !present[kind] {
				label := schemas.Label(kind)
				if kind == schemas.PayStub || kind == schemas.BankStatement {
					label = strings.ToLower(label)
				}
				missing = append(missing, label)
			}
		}
		if len(missing) > 0 {
			label := missing[0]
			if len(missing) > 1 {
				label = strings.Join(missing[:len(missing)-1], ", ") + " and " + missing[len(missing)-1]
			}
			writeError(w, http.StatusConflict, "Upload the "+label+" before approving.")
			return
		}
		n, err := s.store.UnresolvedFlags(caseID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check flagged fields")
			return
		}
		if n > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "Verify flagged fields before approving", "unresolved": n})
			return
		}
	}
	if err := s.store.DecideCase(caseID, userFrom(r).ID, status, in.Note); err != nil {
		switch {
		case errors.Is(err, store.ErrAlreadyDecided):
			writeError(w, http.StatusConflict, "this case already has a decision")
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "case not found")
		default:
			writeError(w, http.StatusInternalServerError, "could not record decision")
		}
		return
	}
	s.getCaseByID(w, caseID)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.AuditLog(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) getCaseByID(w http.ResponseWriter, id string) {
	detail, err := s.store.CaseDetail(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load case")
		return
	}
	writeCaseDetail(w, detail)
}
