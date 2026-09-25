package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) caseEvents(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("id")
	if _, err := s.store.GetCase(caseID); err != nil {
		writeError(w, http.StatusNotFound, "case not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, cancel := s.opts.Broker.Subscribe(caseID)
	defer cancel()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e := <-events:
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: stage\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
