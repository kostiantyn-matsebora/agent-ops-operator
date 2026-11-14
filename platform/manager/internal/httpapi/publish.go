package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// handleSignalInbound is the ACCEPT step (async-signal-ingest design D-1/
// D-3): validate the batch, look up the source, record the "signal
// received" receipt and apply the source's attributeFilters, then enqueue
// one ingest-signal task per ADMITTED signal on in.signal and reply. It
// answers no chat command and stores no Signal itself — the in.signal
// consumer (HandleIngestSignalTask, ingesttask.go) does both, later, with
// the bus's own bounded retry and dead-letter record standing in for what
// used to be the adapter's at-least-once re-send on a lost 5xx.
//
// The response is `202 {"invocationId","taskIds"}` plus `queued` (=
// len(taskIds) — ACCEPTED, not stored, design D-4) and `reason` whenever
// any signal was filtered. A fully filtered batch still replies 202, with
// empty taskIds and queued: 0 (design D-3/Open Question 2): acceptAndEnqueue
// runs with zero requests rather than this handler special-casing it.
func (s *Server) handleSignalInbound(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in signalInboundReq
	if err := json.Unmarshal(body, &in); err != nil || in.Source == "" || len(in.Signals) == 0 {
		writeJSON(w, 400, map[string]string{"error": `need {"source","signals":[...]}`})
		return
	}
	if msg := validateSignals(in.Signals); msg != "" {
		writeJSON(w, 400, map[string]string{"error": msg})
		return
	}
	source := s.signalSource(r, w, in.Source)
	if source == nil {
		return
	}
	// Receipt is recorded before anything is enqueued: the hop that happened
	// is "this adapter handed the manager a signal", true whether the signal
	// is later claimed, suppressed by cooldown or dropped. The initiationId
	// in `detail` correlates it with the claim that follows.
	for _, sig := range in.Signals {
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalReceived,
			From:   activity.Node(activity.NodeSignalAdapter, source.Spec.Adapter),
			To:     activity.Node(activity.NodeSignalSource, source.Name),
			Code:   orDefault(sig.Kind, KindAlert),
			Detail: sig.InitiationID,
		})
	}
	admitted, filteredOut := partitionByAttributeFilters(source, in.Signals)
	if len(filteredOut) > 0 && logFiltered(source) {
		s.emitDropped(source, filteredOut, activity.CodeFiltered, ReasonFiltered)
	}

	reqs := make([]commandbus.SubmitRequest, 0, len(admitted))
	for i := range admitted {
		sig := admitted[i]
		reqs = append(reqs, commandbus.SubmitRequest{
			Queue:     SignalIngestQueue,
			CommandID: ingestSignalCommandID(source.Name, &sig),
			Key:       ingestSignalKey(source.Name, sig.InitiationID),
			Payload:   ingestSignalPayload{Source: source.Name, Signal: sig}.encode(),
		})
	}
	resp, err := s.acceptAndEnqueue(r.Context(), reqs...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{
		"invocationId": resp.InvocationID,
		"taskIds":      resp.TaskIDs,
		"queued":       len(resp.TaskIDs),
	}
	if len(filteredOut) > 0 {
		out["reason"] = ReasonFiltered
	}
	writeJSON(w, 202, out)
}

// ingestSignalCommandID is the Submit-level dedup key for one admitted
// signal (design D-1: "the deduplication key is the signalId"). A signal
// carrying one dedups a repeated POST at the bus layer too, scoped to its
// source — two sources naming the same signalId are unrelated messages,
// exactly as publishOne's own AlreadyExists check already scopes by source
// (ingesttask.go). A signal with none carries no sender-given dedup key at
// all (today's publish never deduplicated one either), so it gets a fresh
// commandID every accept: Submit-level dedup only ever suppresses a
// signalId redelivery, never a signalId-less one.
func ingestSignalCommandID(sourceName string, sig *NormalizedSignal) commandbus.CommandID {
	if sig.SignalID != "" {
		return commandbus.CommandID("ingest-signal:" + sourceName + ":" + sig.SignalID)
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return commandbus.CommandID("ingest-signal:" + hex.EncodeToString(buf[:]))
}

// ingestSignalKey is the in.signal queue's ordering Key (design D-2):
// source plus initiationId, NUL-separated so the two can never alias — the
// same scoping v1alpha1.SignalNameFor's own scheme uses for (source,
// signalId).
func ingestSignalKey(sourceName, initiationID string) commandbus.Key {
	return commandbus.Key(sourceName + "\x00" + initiationID)
}

// emitDropped records one signal.dropped per signal a batch lost, carrying the
// reason. A drop is the event an operator most needs and the one a
// status-derived graph can never show: nothing changed, so there is nothing to
// infer from.
func (s *Server) emitDropped(source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, code, reason string) {
	for _, sig := range signals {
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalDropped,
			From:   activity.Node(activity.NodeSignalSource, source.Name),
			Status: activity.StatusError,
			Code:   code,
			Detail: reason + " (" + sig.InitiationID + ")",
		})
	}
}

// orDefault fills an empty string with a fallback (telemetry labelling only).
func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
