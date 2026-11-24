package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/sumce/openwrt-ai-assistant/internal/agent"
)

// handleRunEvents streams run events over SSE. `after=<seq>` replays events
// newer than seq first — this is how the frontend recovers after a
// disconnect (e.g. Wi-Fi switch on a phone).
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	run := s.cfg.Manager.Get(r.PathValue("id"))
	if run == nil {
		fail(w, http.StatusNotFound, "任务不存在")
		return
	}
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))

	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeEvent := func(e agent.Event) bool {
		data, err := json.Marshal(e)
		if err != nil {
			return true
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return e.Type != "done"
	}

	replay, subID, ch := run.Attach(after)
	defer run.Detach(subID)

	for _, e := range replay {
		if !writeEvent(e) {
			return
		}
	}
	// Run finished but the replay window didn't include the done event:
	// close the stream cleanly with a synthetic one.
	if run.Snapshot().State != agent.StateRunning && run.Snapshot().State != agent.StateQueued {
		if !isPendingState(run.Snapshot().State) {
			writeEvent(agent.Event{Type: "done", State: run.Snapshot().State, Error: run.Snapshot().Error})
			return
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if !writeEvent(e) {
				return
			}
		}
	}
}

func isPendingState(state string) bool {
	return state == agent.StateRunning || state == agent.StateQueued ||
		state == agent.StateWaitingApproval
}
