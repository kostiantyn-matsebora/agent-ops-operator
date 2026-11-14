package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// Publication (signal-persistence D-2, D-8, D-9): /signal/inbound validates,
// applies attributeFilters, stores one Signal per admitted signal and
// acknowledges. Nothing is routed at publish.

func respBody(t *testing.T, s *Server, source string, signals ...map[string]any) (int, map[string]any) {
	t.Helper()
	rec := postSig(s, source, signals)
	out := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v: %s", err, rec.Body.String())
	}
	return rec.Code, out
}

// taskIDCount reads resp["taskIds"] (JSON-decoded into map[string]any, so a
// []any of strings) back to a plain count — the respBody-flavoured
// counterpart of acceptendpoints_test.go's acceptedIDs, which works from the
// *httptest.ResponseRecorder directly instead.
func taskIDCount(resp map[string]any) int {
	ids, _ := resp["taskIds"].([]any)
	return len(ids)
}

func TestPublishStoresASignalOwnedBySourceAndRoutesNothing(t *testing.T) {
	// No Pipeline claims the source: an unclaimed signal is STILL stored and
	// acknowledged — the drop is the consumer's decision, not publication's.
	source := sigTestSource("pub-src", "pub-adapter")
	s, c := sigServer(t, source)

	code, resp := respBody(t, s, source.Name, map[string]any{
		"initiationId": "pub-1", "kind": "alert", "title": "disk", "severity": "warning",
		"payload": "disk 91%", "labels": map[string]string{"team": "ops"},
	})
	if code != 202 || resp["queued"] != float64(1) || taskIDCount(resp) != 1 {
		t.Fatalf("want 202 queued=1 with one taskId, got %d %v", code, resp)
	}
	if _, has := resp["conversations"]; has {
		t.Fatalf("the response must no longer carry conversations: %v", resp)
	}
	if _, has := resp["reason"]; has {
		t.Fatalf("an unrouted-at-publish signal carries no reason: %v", resp)
	}

	// Accepted, not yet performed: the in.signal consumer (ingesttask.go)
	// does the actual store (async-signal-ingest D-1).
	drainAllIngestTasks(t, s, taskIDCount(resp))
	assertStoredSignal(t, c, source)
	assertNothingRouted(t, s, c, source)
}

func assertStoredSignal(t *testing.T, c client.Client, source *agentopsv1alpha1.SignalSource) {
	t.Helper()
	sigs := listSignals(t, c)
	if len(sigs) != 1 {
		t.Fatalf("want 1 Signal, got %d", len(sigs))
	}
	got := sigs[0]
	if got.Spec.SourceRef.Name != source.Name || got.Spec.Adapter != "pub-adapter" ||
		got.Spec.InitiationID != "pub-1" || got.Spec.Payload != "disk 91%" ||
		got.Spec.Title != "disk" || got.Spec.Severity != agentopsv1alpha1.SeverityWarning {
		t.Fatalf("spec not stored verbatim: %+v", got.Spec)
	}
	if got.Status.Phase != "" {
		t.Fatalf("publication must leave the phase absent (Pending), got %q", got.Status.Phase)
	}
	if !strings.HasPrefix(got.Name, agentopsv1alpha1.SignalNamePrefix) {
		t.Fatalf("name %q lacks the sig- prefix", got.Name)
	}
	if len(got.OwnerReferences) != 1 || got.OwnerReferences[0].Kind != "SignalSource" || got.OwnerReferences[0].Name != source.Name {
		t.Fatalf("the Signal must be owned by its SignalSource: %+v", got.OwnerReferences)
	}
}

func assertNothingRouted(t *testing.T, s *Server, c client.Client, source *agentopsv1alpha1.SignalSource) {
	t.Helper()
	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil || len(convs.Items) != 0 {
		t.Fatalf("publication must create no conversation: %v %d", err, len(convs.Items))
	}
	var src agentopsv1alpha1.SignalSource
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigTestNamespace, Name: source.Name}, &src); err != nil {
		t.Fatal(err)
	}
	if len(src.Status.Cooldown) != 0 || src.Status.ReceivedTotal != 0 {
		t.Fatalf("publication must spend no cooldown and move no counter: %+v", src.Status)
	}
	events, _ := s.Activity.Since("", 0)
	for _, e := range events {
		if e.Kind == activity.KindSignalClaimed || e.Kind == activity.KindSignalDropped {
			t.Fatalf("publication must emit no claim or drop, got %+v", e)
		}
	}
}

func TestPublishEveryAdmittedSignalIsItsOwnSignal(t *testing.T) {
	source := sigTestSource("own-src", "own-adapter")
	s, c := sigServer(t, source)

	code, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "same", "payload": "a"},
		map[string]any{"initiationId": "same", "payload": "b"},
		map[string]any{"initiationId": "other", "payload": "c"},
	)
	if code != 202 || resp["queued"] != float64(3) {
		t.Fatalf("want 202 queued=3, got %d %v", code, resp)
	}
	drainAllIngestTasks(t, s, taskIDCount(resp))
	if n := len(listSignals(t, c)); n != 3 {
		t.Fatalf("three signals, two sharing an initiationId, are three Signals — got %d", n)
	}
}

func TestPublishSignalIDDeduplicates(t *testing.T) {
	source := sigTestSource("dup-src", "dup-adapter")
	s, c := sigServer(t, source)
	sig := map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"}

	// The FIRST accept actually enqueues; drain it once so the Signal exists.
	code, resp := respBody(t, s, source.Name, sig)
	if code != 202 || resp["queued"] != float64(1) || taskIDCount(resp) != 1 {
		t.Fatalf("send 0: want 202 queued=1 with one taskId, got %d %v", code, resp)
	}
	drainAllIngestTasks(t, s, taskIDCount(resp))

	// The SECOND accept of the identical signalId dedups at the commandbus's
	// own Submit layer (ingestSignalCommandID keys on source+signalId,
	// design D-1): it resolves to the SAME already-settled task, so there is
	// nothing new to drain — publishOne's own AlreadyExists dedup is never
	// even reached for THIS resend (see
	// TestHandleIngestSignalTaskSignalIDRedeliveryAcrossDifferentTasksDeduplicates
	// in ingesttask_test.go for that path, exercised directly).
	code, resp = respBody(t, s, source.Name, sig)
	if code != 202 || resp["queued"] != float64(1) {
		t.Fatalf("send 1: want 202 queued=1, got %d %v", code, resp)
	}

	sigs := listSignals(t, c)
	if len(sigs) != 1 {
		t.Fatalf("a re-send with the same signalId must yield one Signal, got %d", len(sigs))
	}
	if want := agentopsv1alpha1.SignalNameFor(source.Name, "m-1"); sigs[0].Name != want {
		t.Fatalf("name = %q, want the derived %q", sigs[0].Name, want)
	}
}

func countCode(s *Server, code string) int {
	events, _ := s.Activity.Since("", 0)
	n := 0
	for _, e := range events {
		if e.Code == code {
			n++
		}
	}
	return n
}

func TestPublishSameSignalIDTwiceInOneBatchYieldsOne(t *testing.T) {
	source := sigTestSource("batch-src", "batch-adapter")
	s, c := sigServer(t, source)

	code, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
		map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
	)
	if code != 202 || resp["queued"] != float64(2) {
		t.Fatalf("want 202 queued=2 (both accepted), got %d %v", code, resp)
	}
	// Both entries share one CommandID (ingestSignalCommandID keys on
	// source+signalId, design D-1), so the SECOND Submit call resolves to
	// the first (still-Queued) task rather than minting its own — one real
	// task behind two accepted entries, never two. (The Signal-layer
	// AlreadyExists dedup this used to exercise is covered directly, at the
	// consumer, by
	// TestHandleIngestSignalTaskSignalIDRedeliveryAcrossDifferentTasksDeduplicates
	// in ingesttask_test.go.)
	ids, _ := resp["taskIds"].([]any)
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("both entries of one batch sharing a signalId must resolve to the SAME task, got %v", ids)
	}
	if _, err := drainOneIngestTask(t, s); err != nil {
		t.Fatal(err)
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("want one Signal, got %d", n)
	}
}

func TestPublishResendAfterRoutedChangesNothing(t *testing.T) {
	source := sigTestSource("routed-src", "routed-adapter")
	s, c := sigServer(t, source)
	sig := map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"}
	_, resp0 := respBody(t, s, source.Name, sig)
	drainAllIngestTasks(t, s, taskIDCount(resp0))

	got := listSignals(t, c)[0]
	got.Status.Phase = agentopsv1alpha1.SignalRouted
	got.Status.Claimants = []agentopsv1alpha1.SignalClaimant{{
		Ref:             agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: "p"},
		State:           agentopsv1alpha1.ClaimantRouted,
		ConversationRef: &agentopsv1alpha1.ObjectRef{Name: "conv"},
	}}
	if err := c.Status().Update(context.Background(), &got); err != nil {
		t.Fatal(err)
	}

	code, resp := respBody(t, s, source.Name, sig)
	if code != 202 || resp["queued"] != float64(1) {
		t.Fatalf("want 202 queued=1, got %d %v", code, resp)
	}
	// The resend dedups at Submit (same CommandID, design D-1) to the
	// already-settled first task — nothing new to drain.
	after := listSignals(t, c)
	if len(after) != 1 || after[0].Status.Phase != agentopsv1alpha1.SignalRouted ||
		after[0].ResourceVersion != got.ResourceVersion {
		t.Fatalf("a re-send after Routed must leave the Signal untouched: %+v", after)
	}
}

func TestPublishConcurrentRaceYieldsOneSignal(t *testing.T) {
	source := sigTestSource("race-src", "race-adapter")
	s, c := sigServer(t, source)

	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := postSig(s, source.Name, []map[string]any{{"initiationId": "i-1", "signalId": "m-race", "payload": "x"}})
			codes[i] = rec.Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != 202 {
			t.Fatalf("request %d answered %d, every racer must be accepted", i, code)
		}
	}
	// Every racer's Submit shares one CommandID (source+signalId): the bus
	// mutex serializes them, so exactly one real task was ever created —
	// one to drain, regardless of how many racers there were.
	if _, err := drainOneIngestTask(t, s); err != nil {
		t.Fatal(err)
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("a concurrent race on one signalId must yield one Signal, got %d", n)
	}
}

func TestPublishSameSignalIDOnTwoSourcesYieldsTwoSignals(t *testing.T) {
	a, b := sigTestSource("src-a", "ad"), sigTestSource("src-b", "ad")
	s, c := sigServer(t, a, b)
	for _, src := range []string{a.Name, b.Name} {
		_, resp := respBody(t, s, src, map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"})
		drainAllIngestTasks(t, s, taskIDCount(resp))
	}
	sigs := listSignals(t, c)
	if len(sigs) != 2 || sigs[0].Name == sigs[1].Name {
		t.Fatalf("two sources never collide on one signalId: %+v", sigs)
	}
}

func TestPublishSameInitiationIDDifferentSignalIDsAreTwoMessages(t *testing.T) {
	source := sigTestSource("two-src", "ad")
	s, c := sigServer(t, source)
	_, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "i-1", "signalId": "m-1", "payload": "x"},
		map[string]any{"initiationId": "i-1", "signalId": "m-2", "payload": "y"})
	drainAllIngestTasks(t, s, taskIDCount(resp))
	if n := len(listSignals(t, c)); n != 2 {
		t.Fatalf("want 2 Signals, got %d", n)
	}
}

func TestPublishWithoutSignalIDDoesNotDeduplicate(t *testing.T) {
	source := sigTestSource("nodedup-src", "ad")
	s, c := sigServer(t, source)
	sig := map[string]any{"initiationId": "i-1", "payload": "x"}
	// A signal with no signalId gets a FRESH CommandID every accept
	// (ingestSignalCommandID), so each resend is its own task — no Submit-
	// level dedup to interfere with here, unlike the signalId cases above.
	_, resp1 := respBody(t, s, source.Name, sig)
	_, resp2 := respBody(t, s, source.Name, sig)
	drainAllIngestTasks(t, s, taskIDCount(resp1)+taskIDCount(resp2))
	sigs := listSignals(t, c)
	if len(sigs) != 2 || sigs[0].Name == sigs[1].Name {
		t.Fatalf("no signalId means two Signals under generated names: %+v", sigs)
	}
	if dup := countCode(s, activity.CodeDuplicate); dup != 0 {
		t.Fatalf("no duplicate event without a signalId, got %d", dup)
	}
}

func TestPublishInvalidSignalIDRefusesTheWholeBatch(t *testing.T) {
	for name, id := range map[string]string{
		"too long":    strings.Repeat("a", 257),
		"has a space": "m 1",
		"non-ascii":   "m-é",
		"control":     "m\x01",
	} {
		t.Run(name, func(t *testing.T) {
			source := sigTestSource("badid-src", "ad")
			s, c := sigServer(t, source)
			code, resp := respBody(t, s, source.Name,
				map[string]any{"initiationId": "i-0", "payload": "fine"},
				map[string]any{"initiationId": "i-1", "signalId": id, "payload": "x"})
			if code != 400 || !strings.Contains(fmt.Sprint(resp["error"]), "signalId") {
				t.Fatalf("want 400 naming signalId, got %d %v", code, resp)
			}
			if n := len(listSignals(t, c)); n != 0 {
				t.Fatalf("a refused batch stores nothing, got %d Signals", n)
			}
		})
	}
	// the 256-byte boundary is accepted
	source := sigTestSource("okid-src", "ad")
	s, _ := sigServer(t, source)
	if code, resp := respBody(t, s, source.Name,
		map[string]any{"initiationId": "i-1", "signalId": strings.Repeat("a", 256), "payload": "x"}); code != 202 {
		t.Fatalf("a 256-byte signalId is valid: %d %v", code, resp)
	}
}

// TestPublishFailedStoreIsNotAcknowledgedAndResendIsDeduplicated moved to
// ingesttask_test.go as
// TestHandleIngestSignalTaskFailedStoreFailsTheTaskAndARetrySucceeds — the
// store (publishOne) now runs in the in.signal consumer, not inside the
// request, so a store failure no longer answers the ACCEPT call with a
// 5xx (design D-1: the adapter is told 202 regardless, and the dead/
// retried task is what's visible instead).

// chatChannelObj, chatPost and the chat-command tests that used them moved
// to ingesttask_test.go — answerCommands now runs in the in.signal consumer
// (design D-5), not inside handleSignalInbound.

func TestBoundedLabelsKeepsTheReservedSurfaceLabel(t *testing.T) {
	in := map[string]string{LabelChatChannel: "c"}
	for i := 0; i < agentopsv1alpha1.MaxSignalLabels+5; i++ {
		in[fmt.Sprintf("a-%03d", i)] = "v"
	}
	out := boundedLabels(in)
	if len(out) != agentopsv1alpha1.MaxSignalLabels || out[LabelChatChannel] != "c" {
		t.Fatalf("the reserved label must survive the cap: %d %v", len(out), out[LabelChatChannel])
	}
}

// validObjectName and the test of the generated-name scheme moved to
// ingesttask_test.go — generatedSignalName/generatedTimeLen/generatedSuffixLen
// no longer exist (async-signal-ingest D-6): a signal with no signalId is
// now named via signalNameFromTask, tested there as
// TestSignalNameFromTaskIsDeterministicDistinctAndValid.
