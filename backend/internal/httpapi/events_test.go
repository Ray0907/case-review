package httpapi

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"tidalwave/backend/internal/pipeline"
)

func TestEventsStream(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	req := mustReq(t, "GET", env.url+"/api/cases/"+id+"/events")
	res, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		env.broker.Publish(pipeline.Event{CaseID: id, DocumentID: "d1", Stage: "extracting", Status: "running"})
	}()
	sc := bufio.NewScanner(res.Body)
	deadline := time.After(2 * time.Second)
	for {
		lineCh := make(chan string, 1)
		go func() {
			if sc.Scan() {
				lineCh <- sc.Text()
			}
		}()
		select {
		case line := <-lineCh:
			if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"stage":"extracting"`) {
				return
			}
		case <-deadline:
			t.Fatal("event not received")
		}
	}
}
