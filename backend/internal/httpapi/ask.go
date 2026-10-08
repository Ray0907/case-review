package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"tidalwave/backend/internal/qa"
	"tidalwave/backend/internal/store"
)

const maxQuestionChars = 500

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.store.GetCase(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	if !decided[c.Status] {
		writeError(w, http.StatusConflict, "Questions open once a decision is recorded for this case.")
		return
	}
	if s.opts.Asker == nil {
		writeError(w, http.StatusServiceUnavailable, "Question answering is not set up on this server.")
		return
	}
	var in struct {
		Question string `json:"question"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "send {\"question\": ...}")
		return
	}
	question := strings.TrimSpace(in.Question)
	if question == "" || utf8.RuneCountInString(question) > maxQuestionChars {
		writeError(w, http.StatusBadRequest, "Ask a question of 1 to 500 characters.")
		return
	}
	detail, err := s.store.CaseDetail(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load case")
		return
	}
	audit, err := s.store.AuditLog(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load audit log")
		return
	}
	facts := caseFacts(detail, audit)
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.streamAnswer(ctx, w, id, facts, question)
		return
	}
	res, err := s.opts.Asker.Ask(ctx, facts, question)
	if err != nil {
		log.Printf("ask case %s: %v", id, err)
		writeError(w, http.StatusBadGateway, "Could not get an answer. Try again.")
		return
	}
	res.Citations = qa.ValidCitations(facts, res.Citations)
	writeJSON(w, http.StatusOK, res)
}

func caseFacts(d store.Detail, audit []store.AuditEntry) qa.Facts {
	f := qa.Facts{Case: qa.CaseFacts{Borrower: d.Case.BorrowerName, LoanNumber: d.Case.LoanNumber,
		LoanProduct: d.Case.LoanProduct, RequestedAmount: d.Case.RequestedAmount, Status: d.Case.Status},
		Documents: []qa.DocFact{}, CaseJudgments: judgmentFacts(d.CaseJudgments), Audit: []qa.AuditLine{}}
	if a := d.Assessment; a != nil {
		f.Assessment = &qa.Assessment{MonthlyIncome: a.MonthlyIncome, MonthlyDebt: a.MonthlyDebt, DTI: a.DTI,
			Recommendation: a.Recommendation, Reasons: a.Reasons}
	}
	for _, doc := range d.Documents {
		if doc.Status == "superseded" {
			continue
		}
		df := qa.DocFact{Type: doc.DocType, FileName: doc.FileName, Fields: []qa.FieldFact{}, Judgments: judgmentFacts(doc.Judgments)}
		for _, fld := range doc.Fields {
			df.Fields = append(df.Fields, qa.FieldFact{Key: fld.Key, Label: fld.Label, Value: fld.Value, Flagged: fld.Flagged,
				FlagReason: fld.FlagReason, EditedByReviewer: fld.Edited})
		}
		f.Documents = append(f.Documents, df)
	}
	for _, e := range audit {
		f.Audit = append(f.Audit, qa.AuditLine{Time: time.Unix(e.CreatedAt, 0).UTC().Format(time.RFC3339),
			Reviewer: e.UserName, Action: e.Action, Note: e.Note})
	}
	return f
}

func judgmentFacts(in []store.Judgment) []qa.Judgment {
	out := []qa.Judgment{}
	for _, j := range in {
		out = append(out, qa.Judgment{Name: j.Name, Score: j.Score, Reason: j.Reason})
	}
	return out
}

// streamAnswer writes the answer in the AI SDK UI message stream format: text parts as it is written,
// then the citations as a data part. Errors after the stream starts travel inside the stream.
func (s *Server) streamAnswer(ctx context.Context, w http.ResponseWriter, id string, facts qa.Facts, question string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not available on this connection")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	h.Set("x-vercel-ai-ui-message-stream", "v1")
	w.WriteHeader(http.StatusOK)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}
	finish := func() {
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
	type obj = map[string]any
	textID := "text-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	started := false
	send(obj{"type": "start", "messageId": "msg-" + strconv.FormatInt(time.Now().UnixNano(), 36)})
	onText := func(delta string) {
		if delta == "" {
			return
		}
		if !started {
			started = true
			send(obj{"type": "text-start", "id": textID})
		}
		send(obj{"type": "text-delta", "id": textID, "delta": delta})
	}
	var res qa.Result
	var err error
	if sa, ok := s.opts.Asker.(qa.StreamAsker); ok {
		res, err = sa.AskStream(ctx, facts, question, onText)
	} else {
		res, err = s.opts.Asker.Ask(ctx, facts, question)
	}
	if err == nil && !started && strings.TrimSpace(res.Answer) == "" {
		err = fmt.Errorf("empty answer")
	}
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("ask case %s: %v", id, err)
		}
		if started {
			send(obj{"type": "text-end", "id": textID})
		}
		send(obj{"type": "error", "errorText": "Could not get an answer. Try again."})
		finish()
		return
	}
	if !started {
		onText(res.Answer)
	}
	send(obj{"type": "text-end", "id": textID})
	send(obj{"type": "data-citations", "data": qa.ValidCitations(facts, res.Citations)})
	send(obj{"type": "finish"})
	finish()
}
