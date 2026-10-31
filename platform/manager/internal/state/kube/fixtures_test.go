package kube

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// fixtureRecord builds a ConversationRecord exercising every field
// record.go declares, so a round trip through the kube backend is a
// round trip of the whole contract, not just the fields a hand test
// happened to think of.
//
// Every timestamp here is whole-second (zero nanoseconds): most of them
// cross a *metav1.Time on the CRD side, and metav1.Time's own MarshalJSON
// truncates to whole-second precision on the way to storage (gotchas.md).
// A fixture with sub-second timestamps would fail the round-trip
// assertion for a reason that has nothing to do with this package's own
// mapping.
func fixtureRecord(name string) state.ConversationRecord {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tick := func(n int) *time.Time {
		t := t0.Add(time.Duration(n) * time.Minute)
		return &t
	}
	readAt := "2026-01-02T03:04:05.123456789Z" // ReadAt is a raw string field: full precision survives.

	return state.ConversationRecord{
		Name: name,

		CausedBy:              &state.Provenance{Parent: "parent-conv", Entry: "agent-a"},
		CoordinatorRef:        "coord-1",
		EscalationChannelRefs: []state.Ref{"esc-chan"},
		ChannelRefs:           []state.Ref{"chan-a", "chan-b"},
		ProfileRef:            "profile-1",
		PipelineRef:           "pipeline-1",
		Signal: &state.SignalProvenance{
			SourceRef: "source-1",
			Labels:    map[string]string{"alertname": "Test"},
			Severity:  "warning",
		},
		Title:              "a fixture conversation",
		ConversationID:     "conv-id-1",
		OriginReader:       &state.OriginReader{Channel: "chan-a", Key: "reader-key"},
		Toolsets:           &state.ToolsetBinding{Mode: "merge", Refs: []state.Ref{"toolset-1"}},
		MCPConfigs:         &state.ToolingBinding{Refs: []state.Ref{"mcp-1"}},
		RuntimeRef:         "runtime-1",
		ServiceAccountName: "sa-1",
		ContextClaimName:   "ctx-claim",
		WorkspaceClaimName: "ws-claim",
		Inputs: []state.InputItem{
			{
				ID: "input-1", Type: "task", Payload: "inline payload",
				Origin:     &state.InputOrigin{Kind: "channel", Name: "chan-a", Entry: "entry-a", SignalKind: "chat", Sender: "alice"},
				ReceivedAt: *tick(0),
			},
			{
				ID: "input-2", Type: "alert", PayloadRef: "blob-1",
				ReceivedAt: *tick(1),
			},
		},

		Phase: "Working",
		Threads: []state.ThreadBinding{
			{
				Channel: "chan-a", ThreadID: "thread-1", ReadAt: &readAt, ReadTracked: true,
				Readers:          []state.ReaderMark{{Key: "reader-1", ReadAt: &readAt}},
				UndeliveredReply: "run-1",
				Claim:            &state.OpClaim{Holder: "manager-a", ClaimedAt: *tick(2)},
			},
			{Channel: "chan-b", ThreadID: "thread-2"},
		},
		RuntimeContextID:  "runtime-context-1",
		SessionID:         "session-legacy-1",
		RuntimePod:        "pod-1",
		Inflight:          &state.InflightRun{RunID: "run-1", InputIDs: []string{"input-1"}, DispatchedAt: *tick(3), Moved: true},
		ProcessedInputIDs: []string{"input-0"},
		LastActivity:      tick(4),
		ClosedAt:          tick(5),
		Budget: &state.ConversationBudget{
			MaxAgents: 3, MaxTurns: 10, Deadline: tick(6), AgentsInvoked: 1, Turns: 2,
		},
		EscalatedAt:             tick(7),
		EscalationMessage:       "please help",
		CloseReason:             "done",
		Brief:                   "a fixture conversation about testing",
		ThreadsArchived:         []string{"chan-b"},
		Reopens:                 1,
		ContextCheckpoint:       &state.ContextCheckpoint{At: *tick(8), Generation: "gen-1", Quiesced: true, Bytes: 1024},
		RuntimeStartFailures:    2,
		LastRuntimeStartFailure: tick(9),
		Conditions: []state.Condition{
			{Type: "Ready", Status: "True", Reason: "AllGood", Message: "ready", ObservedGeneration: 1, LastTransitionTime: *tick(10)},
		},
	}
}

// assertRecordsEqual compares two records by their JSON encoding rather
// than reflect.DeepEqual: a time.Time's internal representation can differ
// (wall/monotonic bits, the *Location pointer) between a value built by
// hand and one that round-tripped through RFC3339 parsing, while meaning
// exactly the same instant — json.Marshal's own time.Time.MarshalJSON
// normalizes both to the same RFC3339Nano string, so this compares what
// the fixture actually asserts: the same information, not the same bytes
// in memory.
func assertRecordsEqual(t *testing.T, got, want state.ConversationRecord) {
	t.Helper()
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	wantJSON, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("record mismatch:\n--- got ---\n%s\n--- want ---\n%s", gotJSON, wantJSON)
	}
}
