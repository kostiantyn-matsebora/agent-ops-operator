// The task read surface (async-command-bus task 5.8): GET /state/tasks,
// /state/tasks/{id}, /state/tasks/stream and GET /state/dead-letters — all
// read-only, on the SAME /state/* token scheme state.go's conversation and
// signal surfaces already use (stateAuth).
//
// GET /state/tasks is NOT a flat item list like /state/conversations' own —
// see its own doc comment below for the interface gap that forced this,
// named rather than papered over.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// taskWire is one commandbus.Task, rendered field-for-field — the same
// "published field names, no metadata" rule state.go's conversation/signal
// wire types follow.
type taskWire struct {
	UID          string         `json:"uid"`
	CommandID    string         `json:"commandId"`
	Queue        string         `json:"queue"`
	Kind         string         `json:"kind,omitempty"`
	Key          string         `json:"key"`
	InvocationID string         `json:"invocationId,omitempty"`
	State        string         `json:"state"`
	Attempt      int            `json:"attempt"`
	MaxAttempts  int            `json:"maxAttempts"`
	Code         string         `json:"code,omitempty"`
	Detail       string         `json:"detail,omitempty"`
	NotBefore    *time.Time     `json:"notBefore,omitempty"`
	Lease        *taskLeaseWire `json:"lease,omitempty"`
	SubmittedAt  time.Time      `json:"submittedAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	SucceededAt  *time.Time     `json:"succeededAt,omitempty"`
	DeadAt       *time.Time     `json:"deadAt,omitempty"`
	ParkedUntil  *time.Time     `json:"parkedUntil,omitempty"`
}

type taskLeaseWire struct {
	Holder string    `json:"holder"`
	Until  time.Time `json:"until"`
}

// orNilTime returns nil for the zero time.Time, &t otherwise — the same
// "absent means never happened" rule state.go's timeWire follows for a
// *time.Time field, applied to commandbus.Task's own plain (non-pointer)
// time.Time fields.
func orNilTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func newTaskWire(t commandbus.Task) taskWire {
	w := taskWire{
		UID: t.UID, CommandID: string(t.CommandID), Queue: string(t.Queue), Kind: string(t.Kind),
		Key: string(t.Key), InvocationID: t.InvocationID, State: string(t.State),
		Attempt: t.Attempt, MaxAttempts: t.MaxAttempts, Code: string(t.Code), Detail: t.Detail,
		SubmittedAt: t.SubmittedAt, UpdatedAt: t.UpdatedAt,
		NotBefore: orNilTime(t.NotBefore), SucceededAt: orNilTime(t.SucceededAt),
		DeadAt: orNilTime(t.DeadAt), ParkedUntil: orNilTime(t.ParkedUntil),
	}
	if t.Lease != nil {
		w.Lease = &taskLeaseWire{Holder: t.Lease.Holder, Until: t.Lease.Until}
	}
	return w
}

// busUnavailable answers 503 for every handler below when no Bus was wired
// — the same "unconfigured is unauthorized/unavailable" posture
// AdapterToken's own absence already gets, rather than a nil-pointer panic.
func (s *Server) busUnavailable(w http.ResponseWriter) bool {
	if s.Bus == nil {
		writeJSON(w, 503, map[string]string{"error": "command bus not configured"})
		return true
	}
	return false
}

// handleStateTaskGet serves GET /state/tasks/{id} — a direct
// commandbus.Bus.GetTask, same shape /state/conversations/{name} already
// gives a single record.
func (s *Server) handleStateTaskGet(w http.ResponseWriter, r *http.Request) {
	if s.busUnavailable(w) || !onlyParams(w, r) {
		return
	}
	task, err := s.Bus.GetTask(r.Context(), r.PathValue("id"))
	if err != nil {
		code := 500
		if errors.Is(err, commandbus.ErrNotFound) {
			code = 404
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, newTaskWire(task))
}

// handleStateTasksList serves GET /state/tasks.
//
// commandbus.Bus (sections 1-4 of this change, not this section's to edit)
// exposes Submit/Claim/Settle/Fail/GetTask/DeadLetters/Feed/Stats — no
// method lists every currently-queued-or-running task. Feed only resumes
// strictly after a Sequence and treats a zero resume as "from now" (its own
// doc comment, mirroring state.Watch's empty-Revision convention), so it
// cannot replay history back to "every task that exists right now" either.
//
// This is a GENUINE INTERFACE GAP, not a shortcut taken here: a true flat
// item list (what /state/conversations gives) needs a Bus method this
// change's bus package does not carry yet. Reported in this change's
// hand-back rather than added to internal/commandbus from this package,
// per this role's own boundary. What IS buildable from the existing
// contract — Stats, one call per (queue, kind) pair in DefaultInventory —
// is served instead, so the endpoint exists and is useful (queue depth,
// oldest-pending age) without pretending to be the list it is not.
func (s *Server) handleStateTasksList(w http.ResponseWriter, r *http.Request) {
	if s.busUnavailable(w) || !onlyParams(w, r) {
		return
	}
	type statWire struct {
		Queue            string `json:"queue"`
		Kind             string `json:"kind,omitempty"`
		Pending          int    `json:"pending"`
		Scheduled        int    `json:"scheduled"`
		Running          int    `json:"running"`
		Dead             int    `json:"dead"`
		Parked           int    `json:"parked"`
		OldestPendingAge string `json:"oldestPendingAge,omitempty"`
	}
	out := struct {
		Stats []statWire `json:"stats"`
	}{}
	for _, decl := range commandbus.DefaultInventory() {
		st, err := s.Bus.Stats(r.Context(), decl.Queue, decl.Kind)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		sw := statWire{
			Queue: string(decl.Queue), Kind: string(decl.Kind), Pending: st.Pending,
			Scheduled: st.Scheduled, Running: st.Running, Dead: st.Dead, Parked: st.Parked,
		}
		if st.OldestPendingAge > 0 {
			sw.OldestPendingAge = st.OldestPendingAge.String()
		}
		out.Stats = append(out.Stats, sw)
	}
	writeJSON(w, 200, out)
}

// handleStateTasksStream serves GET /state/tasks/stream?since= — the same
// resumable-SSE shape state.go's streamSSE already gives the conversation
// and signal feeds, over commandbus.Bus.Feed instead of state.Watch.
// Ordered by Sequence (design D-2/D-14 2.6's own "gap-free resumable
// feed"), carried as the frame's own `id` exactly as a Revision is — the
// numeric Sequence rendered as a decimal string, since state.Revision
// (streamSSE's resume-token type) is itself a plain string.
//
// Feed's own "zero resume starts from now" rule means a client that has
// never connected sees no backlog — consistent with this package's other
// streams, and with the interface gap handleStateTasksList's own doc
// comment names: there is no history to resync FROM here either.
func (s *Server) handleStateTasksStream(w http.ResponseWriter, r *http.Request) {
	if s.busUnavailable(w) {
		return
	}
	s.streamSSE(w, r, func(ctx context.Context, resume state.Revision) (<-chan sseFrame, error) {
		seq, err := parseSequence(string(resume))
		if err != nil {
			return nil, err
		}
		feed, err := s.Bus.Feed(ctx, seq)
		if err != nil {
			return nil, err
		}
		return translateTaskFeed(feed), nil
	})
}

func parseSequence(raw string) (commandbus.Sequence, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return commandbus.Sequence(n), nil
}

func translateTaskFeed(feed <-chan commandbus.TaskEvent) <-chan sseFrame {
	out := make(chan sseFrame)
	go func() {
		defer close(out)
		for ev := range feed {
			data, _ := json.Marshal(map[string]any{
				"uid": ev.UID, "queue": ev.Queue, "kind": ev.Kind, "key": ev.Key,
				"commandId": ev.CommandID, "invocationId": ev.InvocationID,
				"state": ev.State, "attempt": ev.Attempt, "code": ev.Code,
			})
			out <- sseFrame{event: "put", id: state.Revision(strconv.FormatUint(uint64(ev.Sequence), 10)), data: data}
		}
	}()
	return out
}

// handleStateDeadLetters serves GET /state/dead-letters?queue= — a direct
// commandbus.Bus.DeadLetters, queue optional (empty = across every queue,
// the same convention DeadLetters' own doc comment states).
func (s *Server) handleStateDeadLetters(w http.ResponseWriter, r *http.Request) {
	if s.busUnavailable(w) || !onlyParams(w, r, "queue") {
		return
	}
	tasks, err := s.Bus.DeadLetters(r.Context(), commandbus.Queue(r.URL.Query().Get("queue")))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := make([]taskWire, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, newTaskWire(t))
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
