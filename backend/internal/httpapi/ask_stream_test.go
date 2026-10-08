package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"tidalwave/backend/internal/qa"
)

type streamAsker struct {
	fakeAsker
	chunks    []string
	streamErr error
	block     bool
	cancelled chan struct{}
}

func (s *streamAsker) AskStream(ctx context.Context, facts qa.Facts, question string, onText func(string)) (qa.Result, error) {
	s.calls++
	s.facts, s.question = facts, question
	for _, c := range s.chunks {
		onText(c)
	}
	if s.block {
		<-ctx.Done()
		close(s.cancelled)
		return qa.Result{}, ctx.Err()
	}
	return s.result, s.streamErr
}

type part map[string]any

func askStream(env *testEnv, t *testing.T, id, question string) (*http.Response, []part) {
	t.Helper()
	req, _ := http.NewRequest("POST", env.url+"/api/cases/"+id+"/ask", strings.NewReader(`{"question":"`+question+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	res, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res, readParts(t, res)
}

func readParts(t *testing.T, res *http.Response) []part {
	t.Helper()
	var parts []part
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			parts = append(parts, part{"type": "[DONE]"})
			continue
		}
		var p part
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			t.Fatalf("bad part %q: %v", payload, err)
		}
		parts = append(parts, p)
	}
	return parts
}

func types(parts []part) []string {
	out := []string{}
	for _, p := range parts {
		out = append(out, p["type"].(string))
	}
	return out
}

func TestAskStreamsAnswerThenSources(t *testing.T) {
	asker := &streamAsker{chunks: []string{"The DTI ", "was 29.2%."}, fakeAsker: fakeAsker{result: qa.Result{Answer: "The DTI was 29.2%.",
		Citations: []qa.Citation{{Document: "assessment", Key: "dti"}, {Document: "passport", Key: "number"}}}}}
	env, id := decidedCaseWithAsker(t, &asker.fakeAsker)
	env.server.opts.Asker = asker
	res, parts := askStream(env, t, id, "What was the DTI?")
	if res.StatusCode != 200 || res.Header.Get("x-vercel-ai-ui-message-stream") != "v1" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status %d headers %v", res.StatusCode, res.Header)
	}
	want := "start text-start text-delta text-delta text-end data-citations finish [DONE]"
	if got := strings.Join(types(parts), " "); got != want {
		t.Fatalf("parts\n got %s\nwant %s", got, want)
	}
	cites := parts[5]["data"].([]any)
	if len(cites) != 1 {
		t.Fatalf("invented citation survived: %v", cites)
	}
}

func TestAskStreamStillRefusesOpenCase(t *testing.T) {
	env, id, _ := readyCase(t, false)
	env.server.opts.Asker = &streamAsker{}
	res, _ := askStream(env, t, id, "What was the DTI?")
	if res.StatusCode != http.StatusConflict || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("status %d type %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func TestAskStreamMidwayErrorIsGeneric(t *testing.T) {
	asker := &streamAsker{chunks: []string{"partial"}, streamErr: errors.New("upstream said sk-secret-123")}
	env, id := decidedCaseWithAsker(t, &asker.fakeAsker)
	env.server.opts.Asker = asker
	_, parts := askStream(env, t, id, "What was the DTI?")
	got := strings.Join(types(parts), " ")
	if got != "start text-start text-delta text-end error [DONE]" {
		t.Fatalf("parts %s", got)
	}
	errPart := parts[4]
	if text := errPart["errorText"].(string); text != "Could not get an answer. Try again." || strings.Contains(text, "sk-secret") {
		t.Fatalf("error text %q", text)
	}
}

func TestAskStreamFallsBackToWholeAnswer(t *testing.T) {
	asker := &streamAsker{fakeAsker: fakeAsker{result: qa.Result{Answer: "Whole answer.", Citations: []qa.Citation{}}}}
	env, id := decidedCaseWithAsker(t, &asker.fakeAsker)
	env.server.opts.Asker = asker
	_, parts := askStream(env, t, id, "Summarize")
	if len(parts) < 3 || parts[2]["type"] != "text-delta" || parts[2]["delta"] != "Whole answer." {
		t.Fatalf("parts %v", parts)
	}
}

func TestAskStreamWorksWithAnAskerThatCannotStream(t *testing.T) {
	asker := &fakeAsker{result: qa.Result{Answer: "Plain answer.", Citations: []qa.Citation{}}}
	env, id := decidedCaseWithAsker(t, asker)
	_, parts := askStream(env, t, id, "Summarize")
	if got := strings.Join(types(parts), " "); got != "start text-start text-delta text-end data-citations finish [DONE]" || parts[2]["delta"] != "Plain answer." {
		t.Fatalf("parts %v", parts)
	}
}

func TestAskStreamStopsWhenTheClientLeaves(t *testing.T) {
	asker := &streamAsker{chunks: []string{"Working"}, block: true, cancelled: make(chan struct{})}
	env, id := decidedCaseWithAsker(t, &asker.fakeAsker)
	env.server.opts.Asker = asker
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", env.url+"/api/cases/"+id+"/ask", strings.NewReader(`{"question":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	res, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	bufio.NewReader(res.Body).ReadString('\n')
	cancel()
	res.Body.Close()
	select {
	case <-asker.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("asker kept running after the client left")
	}
}
