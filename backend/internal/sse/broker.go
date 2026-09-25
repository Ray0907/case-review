package sse

import (
	"sync"

	"tidalwave/backend/internal/pipeline"
)

type Broker struct {
	mu   sync.Mutex
	subs map[string]map[chan pipeline.Event]struct{}
}

func New() *Broker { return &Broker{subs: map[string]map[chan pipeline.Event]struct{}{}} }

func (b *Broker) Subscribe(caseID string) (<-chan pipeline.Event, func()) {
	ch := make(chan pipeline.Event, 32)
	b.mu.Lock()
	if b.subs[caseID] == nil {
		b.subs[caseID] = map[chan pipeline.Event]struct{}{}
	}
	b.subs[caseID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs[caseID], ch)
		b.mu.Unlock()
	}
}

// Publish drops events for full subscribers; clients refetch case detail on every event, so a dropped intermediate stage is harmless.
func (b *Broker) Publish(e pipeline.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[e.CaseID] {
		select {
		case ch <- e:
		default:
		}
	}
}
