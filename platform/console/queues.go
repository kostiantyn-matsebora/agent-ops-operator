package main

import (
	"context"
	"net/http"
	"sort"
	"time"
)

// Queues and capacity — its own view, because the question it answers is asked
// under pressure and has two completely different answers that look identical
// from outside: AN AGENT HAS NOT REPLIED — IS IT QUEUED, OR IS IT STUCK?
//
// Two queues, deliberately kept apart:
//
//   - WORK queue: conversations waiting for a runtime slot, and inputs waiting
//     behind an inflight run (dispatch is strictly serial per conversation, so a
//     busy conversation queues its own messages). From CR state and /status.
//   - DELIVERY queue: channel ops waiting for an adapter to claim them, and ops
//     claimed but never completed. From /status ONLY — the OpQueue is in-memory
//     by design and appears in no Kubernetes object.
//
// Every row carries an AGE, because age is what separates the two failure modes.
// The view flags stuck items rather than leaving an operator to compare
// timestamps by eye.

// StuckReason classifies why a row is not moving. Each maps to a different fix,
// which is the only reason to distinguish them.
const (
	StuckNothingClaiming = "nothing-claiming"   // ops queued, no adapter polling
	StuckAdapterWedged   = "adapter-wedged"     // claimed and never completed
	StuckAtCeiling       = "at-runtime-ceiling" // capacity-bound, not broken
	StuckRuntimeHung     = "runtime-hung"       // inflight far beyond typical
)

// Thresholds past which a row is called stuck rather than busy. Deliberately
// generous: a false "stuck" badge on a healthy backlog would teach an operator
// to ignore the badge.
const (
	queuedStuckAfter   = 2 * time.Minute
	claimedStuckAfter  = 2 * time.Minute
	inflightStuckAfter = 15 * time.Minute
)

// WorkRow is one conversation waiting on capacity or on its own serial lane.
type WorkRow struct {
	Conversation string `json:"conversation"`
	Title        string `json:"title,omitempty"`
	Pipeline     string `json:"pipeline,omitempty"`
	Phase        string `json:"phase,omitempty"`
	// Queued counts inputs not yet dispatched.
	Queued int `json:"queued"`
	// AgeSeconds is time since last activity (or creation) — how long this has
	// been waiting, which is the number that separates busy from stuck.
	AgeSeconds float64 `json:"ageSeconds"`
	// Inflight is set when a run is out; InflightAgeSeconds is how long.
	Inflight           string  `json:"inflight,omitempty"`
	InflightAgeSeconds float64 `json:"inflightAgeSeconds,omitempty"`
	Stuck              string  `json:"stuck,omitempty"`
}

// DeliveryRow is one adapter's outstanding ops.
type DeliveryRow struct {
	Adapter string `json:"adapter"`
	Queued  int    `json:"queued"`
	Claimed int    `json:"claimed"`

	OldestQueuedOpID       string  `json:"oldestQueuedOpId,omitempty"`
	OldestQueuedAgeSeconds float64 `json:"oldestQueuedAgeSeconds,omitempty"`
	OldestQueuedConv       string  `json:"oldestQueuedConversation,omitempty"`

	OldestClaimedOpID       string  `json:"oldestClaimedOpId,omitempty"`
	OldestClaimedAgeSeconds float64 `json:"oldestClaimedAgeSeconds,omitempty"`
	OldestClaimedConv       string  `json:"oldestClaimedConversation,omitempty"`

	Stuck string `json:"stuck,omitempty"`
	// AdapterHealth is the serving adapter's own Ready condition, so "nothing is
	// claiming" is immediately attributable to a pod that is down.
	AdapterHealth Health `json:"adapterHealth,omitempty"`
}

// Queues is GET /api/queues.
type Queues struct {
	Capacity struct {
		InUse   int `json:"inUse"`
		Max     int `json:"max"`
		Waiting int `json:"waiting"`
	} `json:"capacity"`
	Work     []WorkRow     `json:"work"`
	Delivery []DeliveryRow `json:"delivery"`
	// Cooldowns belong here and nowhere else: a suppressed signal lane looks
	// exactly like an idle one on a graph, and this is the view where "nothing
	// is arriving" needs distinguishing from "everything is being swallowed".
	Cooldowns []CooldownStat `json:"cooldowns"`
	Error     string         `json:"error,omitempty"`
}

func (a *API) handleQueues(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, a.queues(ctx))
}

func (a *API) queues(ctx context.Context) Queues {
	out := Queues{Work: []WorkRow{}, Delivery: []DeliveryRow{}, Cooldowns: []CooldownStat{}}

	st, err := a.mgr.Status(ctx)
	if err != nil {
		// The delivery queue exists ONLY in the manager's memory, so without
		// /status it is unavailable — reported as such rather than rendered
		// empty, which would read as "no ops pending".
		out.Error = "manager /status unreachable: " + err.Error()
	} else {
		out.Capacity.InUse, out.Capacity.Max, out.Capacity.Waiting =
			st.RuntimeSlots.InUse, st.RuntimeSlots.Max, st.RuntimeSlots.Waiting
		out.Cooldowns = st.Cooldowns
		for _, q := range st.Queues {
			row := DeliveryRow{
				Adapter: q.Adapter, Queued: q.Queued, Claimed: q.Claimed,
				OldestQueuedOpID: q.OldestQueuedOpID, OldestQueuedAgeSeconds: q.OldestQueuedAgeSeconds,
				OldestQueuedConv:  q.OldestQueuedConv,
				OldestClaimedOpID: q.OldestClaimedOpID, OldestClaimedAgeSeconds: q.OldestClaimedAgeSeconds,
				OldestClaimedConv: q.OldestClaimedConv,
			}
			if adapter := a.cache.Get("channeladapters", q.Adapter); adapter != nil {
				row.AdapterHealth, _, _ = health(adapter)
			}
			// Claimed-and-wedged is reported first: it means an adapter took the
			// work and stopped, which is worse than one that never took it.
			switch {
			case q.Claimed > 0 && q.OldestClaimedAgeSeconds > claimedStuckAfter.Seconds():
				row.Stuck = StuckAdapterWedged
			case q.Queued > 0 && q.OldestQueuedAgeSeconds > queuedStuckAfter.Seconds():
				row.Stuck = StuckNothingClaiming
			}
			out.Delivery = append(out.Delivery, row)
		}
	}

	atCeiling := out.Capacity.Max > 0 && out.Capacity.InUse >= out.Capacity.Max
	now := time.Now()
	pipelines := a.cache.List("pipelines")
	for _, obj := range a.cache.List("conversations") {
		v := conversationView(obj)
		queued := len(v.Spec.Inputs)
		if queued == 0 && v.Status.Inflight == nil {
			continue // nothing waiting and nothing out: not in any queue
		}
		row := WorkRow{
			Conversation: obj.Metadata.Name, Title: v.Spec.Title,
			Pipeline: AttributePipeline(obj, pipelines), Phase: v.Status.Phase, Queued: queued,
			AgeSeconds: ageSeconds(now, firstNonEmpty(v.Status.LastActivity, obj.Metadata.CreationTimestamp)),
		}
		if v.Status.Inflight != nil {
			row.Inflight = v.Status.Inflight.RunID
			row.InflightAgeSeconds = ageSeconds(now, v.Status.Inflight.DispatchedAt)
			if row.InflightAgeSeconds > inflightStuckAfter.Seconds() {
				// inflight far beyond typical run time: the RUNTIME is hung, and
				// this conversation is not queued at all
				row.Stuck = StuckRuntimeHung
			}
		} else if atCeiling && v.Status.Phase == "Pending" {
			// Capacity-bound is not broken. Naming it apart is what stops an
			// operator restarting a healthy system that is merely full.
			row.Stuck = StuckAtCeiling
		}
		out.Work = append(out.Work, row)
	}
	sort.Slice(out.Work, func(i, j int) bool { return out.Work[i].AgeSeconds > out.Work[j].AgeSeconds })
	sort.Slice(out.Delivery, func(i, j int) bool { return out.Delivery[i].Adapter < out.Delivery[j].Adapter })
	return out
}

func ageSeconds(now time.Time, stamp string) float64 {
	if stamp == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		if t, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
			return 0
		}
	}
	d := now.Sub(t).Seconds()
	if d < 0 {
		return 0
	}
	return d
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
