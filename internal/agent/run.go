// Package agent runs tool-calling conversations against the configured
// upstream LLM. Runs are state machines: a run can park itself in
// waiting_approval and be resumed later (even after a server restart).
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/sumce/openwrt-ai-assistant/internal/llm"
	"github.com/sumce/openwrt-ai-assistant/internal/runstate"
)

// Run states.
const (
	StateQueued          = "queued"
	StateRunning         = "running"
	StateWaitingApproval = "waiting_approval"
	StateDone            = "done"
	StateFailed          = "failed"
	StateCancelled       = "cancelled"
)

// Event is pushed to SSE subscribers; Seq makes replay-after-disconnect
// possible.
type Event struct {
	Seq     int                     `json:"seq"`
	Type    string                  `json:"type"` // state|text_delta|tool_start|tool_result|approval_request|done
	State   string                  `json:"state,omitempty"`
	Text    string                  `json:"text,omitempty"`
	Tool    string                  `json:"tool,omitempty"`
	Desc    string                  `json:"desc,omitempty"`
	Level   string                  `json:"level,omitempty"`
	Result  string                  `json:"result,omitempty"`
	OK      bool                    `json:"ok,omitempty"`
	Error   string                  `json:"error,omitempty"`
	Pending *runstate.PendingAction `json:"pending,omitempty"`
}

func isTerminal(state string) bool {
	return state == StateDone || state == StateFailed || state == StateCancelled
}

const maxBufferedEvents = 2000

// Run is one agent execution unit. The executing goroutine owns Messages;
// everything else is guarded by mu.
type Run struct {
	mu             sync.Mutex
	ID             string
	ConversationID string
	State          string
	Err            string
	Messages       []llm.Message
	Pending        *runstate.PendingAction
	cancel         contextCancelFunc

	seq    int
	events []Event
	subs   map[int]chan Event
	subSeq int
}

type contextCancelFunc = func()

func newRun(conversationID string) *Run {
	return &Run{
		ID:             newID(),
		ConversationID: conversationID,
		State:          StateQueued,
		subs:           map[int]chan Event{},
	}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// emit stamps an event with a seq number, buffers it and fans it out.
func (r *Run) emit(e Event) Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq = r.seq
	r.events = append(r.events, e)
	if len(r.events) > maxBufferedEvents {
		r.events = r.events[len(r.events)-maxBufferedEvents:]
	}
	for _, ch := range r.subs {
		select {
		case ch <- e:
		default: // slow consumer: drop rather than block the agent loop
		}
	}
	return e
}

// Attach atomically snapshots events after `after` and subscribes for live
// ones — no gaps between replay and stream.
func (r *Run) Attach(after int) (replay []Event, subID int, ch chan Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Seq > after {
			replay = append(replay, e)
		}
	}
	r.subSeq++
	subID = r.subSeq
	ch = make(chan Event, 512)
	r.subs[subID] = ch
	return replay, subID, ch
}

func (r *Run) Detach(subID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ch, ok := r.subs[subID]; ok {
		delete(r.subs, subID)
		close(ch)
	}
}

func (r *Run) setState(s string) {
	r.mu.Lock()
	r.State = s
	r.mu.Unlock()
	r.emit(Event{Type: "state", State: s})
}

func (r *Run) fail(msg string) {
	r.mu.Lock()
	r.Err = msg
	r.State = StateFailed
	r.mu.Unlock()
	r.emit(Event{Type: "error", Error: msg})
	r.emit(Event{Type: "done", State: StateFailed, Error: msg})
}

func (r *Run) finishCancelled() {
	r.setState(StateCancelled)
	r.emit(Event{Type: "done", State: StateCancelled})
}

func (r *Run) finishDone() {
	r.setState(StateDone)
	r.emit(Event{Type: "done", State: StateDone})
}

// Snapshot is the REST view of a run.
type Snapshot struct {
	ID             string                  `json:"id"`
	ConversationID string                  `json:"conversation_id"`
	State          string                  `json:"state"`
	Error          string                  `json:"error,omitempty"`
	Pending        *runstate.PendingAction `json:"pending,omitempty"`
}

func (r *Run) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Snapshot{
		ID:             r.ID,
		ConversationID: r.ConversationID,
		State:          r.State,
		Error:          r.Err,
		Pending:        r.Pending,
	}
}

func (r *Run) state() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.State
}

func (r *Run) isTerminal() bool { return isTerminal(r.state()) }
