package sse

import (
	"testing"
	"time"

	"tidalwave/backend/internal/pipeline"
)

func TestBrokerDeliversOnlyToCase(t *testing.T) {
	b := New()
	a, cancelA := b.Subscribe("case-a")
	defer cancelA()
	other, cancelB := b.Subscribe("case-b")
	defer cancelB()
	b.Publish(pipeline.Event{CaseID: "case-a", Stage: "parsing", Status: "running"})
	select {
	case e := <-a:
		if e.Stage != "parsing" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	select {
	case e := <-other:
		t.Fatalf("leaked %+v", e)
	default:
	}
}

func TestPublishNeverBlocksOnSlowSubscriber(t *testing.T) {
	b := New()
	_, cancel := b.Subscribe("c")
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.Publish(pipeline.Event{CaseID: "c"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked")
	}
}
