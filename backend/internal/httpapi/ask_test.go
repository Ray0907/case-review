package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"tidalwave/backend/internal/qa"
)

type fakeAsker struct {
	calls    int
	facts    qa.Facts
	question string
	result   qa.Result
	err      error
}

func (f *fakeAsker) Ask(ctx context.Context, facts qa.Facts, question string) (qa.Result, error) {
	f.calls++
	f.facts, f.question = facts, question
	return f.result, f.err
}

// A decided case with an asker attached. The case is rejected, so the audit log has a note to look for.
func decidedCaseWithAsker(t *testing.T, asker *fakeAsker) (*testEnv, string) {
	env, id, _ := readyCase(t, false)
	env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "reject", "note": "DTI too high after review"})
	if asker != nil {
		env.server.opts.Asker = asker
	}
	return env, id
}

func ask(env *testEnv, t *testing.T, id, question string) *http.Response {
	return env.do(t, "POST", "/api/cases/"+id+"/ask", map[string]string{"question": question})
}

func TestAskUnknownCase(t *testing.T) {
	env := newTestEnv(t)
	env.server.opts.Asker = &fakeAsker{}
	if res := ask(env, t, "missing", "What happened?"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 got %d", res.StatusCode)
	}
}

func TestAskOnlyAfterDecision(t *testing.T) {
	env, id, _ := readyCase(t, false)
	asker := &fakeAsker{}
	env.server.opts.Asker = asker
	if res := ask(env, t, id, "What was the DTI?"); res.StatusCode != http.StatusConflict {
		t.Fatalf("open case want 409 got %d", res.StatusCode)
	}
	if asker.calls != 0 {
		t.Fatal("asker was called for an open case")
	}
}

func TestAskNotConfigured(t *testing.T) {
	env, id := decidedCaseWithAsker(t, nil)
	if res := ask(env, t, id, "What was the DTI?"); res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 got %d", res.StatusCode)
	}
}

func TestAskRejectsBadQuestions(t *testing.T) {
	asker := &fakeAsker{}
	env, id := decidedCaseWithAsker(t, asker)
	for _, q := range []string{"", "   ", strings.Repeat("a", 501)} {
		if res := ask(env, t, id, q); res.StatusCode != http.StatusBadRequest {
			t.Fatalf("question of %d chars want 400 got %d", len(q), res.StatusCode)
		}
	}
	if asker.calls != 0 {
		t.Fatal("asker was called with a bad question")
	}
}

func TestAskHidesUpstreamErrors(t *testing.T) {
	env, id := decidedCaseWithAsker(t, &fakeAsker{err: errors.New("401 invalid key sk-secret-123")})
	res := ask(env, t, id, "What was the DTI?")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("want 502 got %d", res.StatusCode)
	}
	if strings.Contains(string(body), "sk-secret") {
		t.Fatalf("upstream error leaked: %s", body)
	}
}

func TestAskKeepsOnlyRealCitations(t *testing.T) {
	asker := &fakeAsker{result: qa.Result{Answer: "The ending balance was lower than expected.", Citations: []qa.Citation{
		{Document: "bank_statement", Key: "ending_balance"},
		{Document: "bank_statement", Key: "made_up_field"},
		{Document: "assessment", Key: "dti"},
		{Document: "passport", Key: "number"},
	}}}
	env, id := decidedCaseWithAsker(t, asker)
	got := decode[qa.Result](t, ask(env, t, id, "Why was the ending balance flagged?"))
	if got.Answer == "" || len(got.Citations) != 2 {
		t.Fatalf("answer %q citations %v", got.Answer, got.Citations)
	}
	if got.Citations[0].Key != "ending_balance" || got.Citations[1].Document != "assessment" {
		t.Fatalf("wrong citations kept: %v", got.Citations)
	}
}

func TestAskReceivesTheRecordedFacts(t *testing.T) {
	asker := &fakeAsker{result: qa.Result{Answer: "ok"}}
	env, id := decidedCaseWithAsker(t, asker)
	// A replaced upload must not reach the model.
	env.upload(t, id, "pay-stub.pdf", minimalPDF)
	env.realRunner.Wait()
	ask(env, t, id, "Summarize the case")
	f := asker.facts
	if asker.question != "Summarize the case" || f.Case.Status != "rejected" {
		t.Fatalf("question %q status %q", asker.question, f.Case.Status)
	}
	if len(f.Audit) == 0 || f.Audit[0].Note != "DTI too high after review" || f.Audit[0].Reviewer != "Maya Park" {
		t.Fatalf("audit %+v", f.Audit)
	}
	if f.Assessment == nil {
		t.Fatal("assessment missing")
	}
	stubs, bank := 0, false
	for _, d := range f.Documents {
		if d.Type == "pay_stub" {
			stubs++
		}
		if d.Type == "bank_statement" {
			for _, field := range d.Fields {
				bank = bank || field.Key == "ending_balance"
			}
		}
	}
	if stubs != 1 || !bank {
		t.Fatalf("pay stubs %d bank fields %v", stubs, bank)
	}
}

func TestAskNeedsSignIn(t *testing.T) {
	asker := &fakeAsker{}
	env, id := decidedCaseWithAsker(t, asker)
	req, _ := http.NewRequest("POST", env.url+"/api/cases/"+id+"/ask", strings.NewReader(`{"question":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized || asker.calls != 0 {
		t.Fatalf("status %d calls %d", res.StatusCode, asker.calls)
	}
}
