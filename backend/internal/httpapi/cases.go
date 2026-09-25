package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tidalwave/backend/internal/auth"
	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/schemas"
	"tidalwave/backend/internal/store"
)

const maxUpload = 20 << 20

var allowedTypes = map[string]string{"application/pdf": ".pdf", "image/png": ".png", "image/jpeg": ".jpg"}

func storedFileStem(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	var stem strings.Builder
	previousUnderscore := false
	for i := 0; i < len(name) && stem.Len() < 64; i++ {
		c := name[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			c = '_'
		}
		if c == '_' && previousUnderscore {
			continue
		}
		stem.WriteByte(c)
		previousUnderscore = c == '_'
	}
	if result := strings.Trim(stem.String(), "._"); result != "" {
		return result
	}
	return "file"
}

func (s *Server) createCase(w http.ResponseWriter, r *http.Request) {
	var in store.Case
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(in.BorrowerName) == "" || strings.TrimSpace(in.LoanNumber) == "" ||
		strings.TrimSpace(in.LoanProduct) == "" || in.RequestedAmount <= 0 {
		writeError(w, http.StatusBadRequest, "borrower name, loan number, loan product and a positive amount are required")
		return
	}
	c, err := s.store.CreateCase(in)
	if errors.Is(err, store.ErrDuplicateLoanNumber) {
		writeError(w, http.StatusConflict, "a case with loan number "+in.LoanNumber+" already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create case")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) listCases(w http.ResponseWriter, r *http.Request) {
	cases, err := s.store.ListCases()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load cases")
		return
	}
	for i := range cases {
		detail, err := s.store.CaseDetail(cases[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load case blockers")
			return
		}
		cases[i].Blocker = caseBlocker(detail)
	}
	writeJSON(w, http.StatusOK, cases)
}

func caseBlocker(detail store.Detail) string {
	switch detail.Case.Status {
	case "approved":
		return "Approved"
	case "rejected":
		return "Rejected"
	case "sent_back":
		return "Sent back"
	}
	present := map[string]bool{}
	flags, unsupported, failed, processing := 0, 0, false, false
	for _, d := range detail.Documents {
		if d.Status == "superseded" {
			continue
		}
		if d.DocType == schemas.Other {
			unsupported++
		}
		if d.Status == "failed" {
			failed = true
		}
		if d.Status != "done" && d.Status != "failed" && d.Status != "unsupported" {
			processing = true
		}
		if d.Status == "done" {
			present[d.DocType] = true
		}
		for _, f := range d.Fields {
			if f.Flagged && !f.Edited {
				flags++
			}
		}
	}
	if failed {
		return "Document failed"
	}
	if processing {
		return "Processing"
	}
	missing := 0
	for _, kind := range schemas.Types {
		if !present[kind] {
			missing++
		}
	}
	if missing > 0 {
		if missing == 1 {
			return "1 document missing"
		}
		return fmt.Sprintf("%d documents missing", missing)
	}
	if flags == 1 {
		return "1 field to verify"
	}
	if flags > 1 {
		return fmt.Sprintf("%d fields to verify", flags)
	}
	if detail.Case.Status == "processing" {
		return "Processing"
	}
	if unsupported == 1 {
		return "1 unsupported document"
	}
	if unsupported > 1 {
		return fmt.Sprintf("%d unsupported documents", unsupported)
	}
	for _, d := range detail.Documents {
		if d.Status == "superseded" {
			continue
		}
		for _, j := range d.Judgments {
			if !pipeline.IsLow(j.Name, j.Score) {
				continue
			}
			label := schemas.Label(d.DocType)
			if d.DocType == schemas.BankStatement || d.DocType == schemas.PayStub {
				label = strings.ToLower(label)
			}
			return "Check " + strings.ToLower(pipeline.JudgmentLabel(j.Name)) + " on the " + label
		}
	}
	for _, j := range detail.CaseJudgments {
		if pipeline.IsLow(j.Name, j.Score) {
			return "Check " + strings.ToLower(pipeline.JudgmentLabel(j.Name))
		}
	}
	if detail.Assessment != nil && detail.Assessment.DTI != nil {
		dti := *detail.Assessment.DTI
		if dti > pipeline.QMThreshold {
			return "DTI above 43%"
		}
		if dti >= pipeline.NearThresholdFloor {
			return "DTI near 43% limit"
		}
	}
	if detail.Assessment != nil && detail.Assessment.Recommendation == "eligible" && len(detail.Assessment.Reasons) == 0 {
		return "Ready for decision"
	}
	return "Needs review"
}

func (s *Server) getCase(w http.ResponseWriter, r *http.Request) {
	detail, err := s.store.CaseDetail(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load case")
		return
	}
	writeCaseDetail(w, detail)
}

func writeCaseDetail(w http.ResponseWriter, detail store.Detail) {
	for i := range detail.Documents {
		for j := range detail.Documents[i].Judgments {
			v := &detail.Documents[i].Judgments[j]
			v.Low = pipeline.IsLow(v.Name, v.Score)
		}
	}
	for i := range detail.CaseJudgments {
		v := &detail.CaseJudgments[i]
		v.Low = pipeline.IsLow(v.Name, v.Score)
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	if _, err := s.store.GetCase(caseID); err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	if s.opts.Runner == nil {
		writeError(w, http.StatusServiceUnavailable, "document processing is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "attach a file under the form field \"file\" (max 20 MB)")
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	ext, ok := allowedTypes[http.DetectContentType(head[:n])]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "upload a PDF, PNG or JPEG")
		return
	}
	doc := store.Document{ID: newDocID(), CaseID: caseID, FileName: filepath.Base(header.Filename)}
	if len(doc.FileName) > 255 {
		doc.FileName = doc.FileName[:255]
	}
	dir := filepath.Join(s.opts.UploadDir, caseID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}
	doc.FilePath = filepath.Join(dir, doc.ID+"-"+storedFileStem(doc.FileName)+ext)
	out, err := os.Create(doc.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}
	_, err = io.Copy(out, io.MultiReader(strings.NewReader(string(head[:n])), file))
	out.Close()
	if err != nil {
		os.Remove(doc.FilePath)
		writeError(w, http.StatusBadRequest, "upload interrupted or larger than 20 MB")
		return
	}
	doc, err = s.store.CreateDocument(doc)
	if err != nil {
		os.Remove(doc.FilePath)
		writeError(w, http.StatusInternalServerError, "could not record document")
		return
	}
	s.opts.Runner.Enqueue(caseID, doc.ID)
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) documentFile(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDocument(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, d.FilePath)
}

func newDocID() string { return auth.NewToken()[:20] }

func (s *Server) retryDocument(w http.ResponseWriter, r *http.Request) {
	err := s.opts.Runner.Retry(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "document not found")
	case errors.Is(err, pipeline.ErrNotRetryable):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not retry document")
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}
