// Tests for ingesttask.go: the "in.signal" queue's own consumer
// (HandleIngestSignalTask, answerCommands, publishOne, signalNameFromTask) —
// everything that used to run inside POST /signal/inbound (publish.go) and
// now runs later, on whichever replica's consumer.Pool next claims the task
// (async-signal-ingest design D-5/D-8). publish_test.go and signals_test.go
// keep the ACCEPT-step assertions (status, taskIds, what gets enqueued);
// this file owns what the task actually DOES once drained.
package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// isCoded reports whether a handler attached a commandbus error code (a
// terminal failure) rather than returning a plain, retried error.
func isCoded(err error) bool {
	var ce *consumer.CodedError
	return errors.As(err, &ce)
}

func TestHandleIngestSignalTaskUnreadablePayloadIsTerminal(t *testing.T) {
	s, _ := sigServer(t, sigTestSource("bad-src", "ad"))
	err := s.HandleIngestSignalTask(context.Background(), commandbus.Task{UID: "t-1", Payload: []byte("{not json")})
	if err == nil || !isCoded(err) {
		t.Fatalf("want a coded (terminal) error, got %v", err)
	}
}

func TestHandleIngestSignalTaskMissingSourceIsTerminalAndStoresNothing(t *testing.T) {
	s, c := sigServer(t, sigTestSource("present-src", "ad"))
	p := ingestSignalPayload{Source: "gone-src", Signal: NormalizedSignal{InitiationID: "i-1", Payload: "x"}}
	err := s.HandleIngestSignalTask(context.Background(), commandbus.Task{UID: "t-2", Payload: p.encode()})
	if err == nil || !isCoded(err) {
		t.Fatalf("want a coded (terminal) error, got %v", err)
	}
	if n := len(listSignals(t, c)); n != 0 {
		t.Fatalf("nothing may be stored for a vanished source, got %d", n)
	}
}

func TestHandleIngestSignalTaskSourceReadFailureIsRetryable(t *testing.T) {
	source := sigTestSource("read-src", "ad")
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(source).
		WithInterceptorFuncs(interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewInternalError(errors.New("apiserver down"))
		}}).Build()
	s := &Server{Reader: c, Client: c, Namespace: sigTestNamespace, Signals: testSignals(t, c, sigTestNamespace),
		Activity: activity.New(16), Bus: memory.Open()}
	p := ingestSignalPayload{Source: source.Name, Signal: NormalizedSignal{InitiationID: "i-1", Payload: "x"}}
	err := s.HandleIngestSignalTask(context.Background(), commandbus.Task{UID: "t-3", Payload: p.encode()})
	if err == nil || isCoded(err) {
		t.Fatalf("want a plain (retried) error, got %v", err)
	}
}

func TestChatChannelNilWhenUnnamedOrUnknown(t *testing.T) {
	s, _ := sigServer(t, sigTestSource("cc-src", "ad"), chatChannelObj("known", "tg"))
	ctx := context.Background()
	if ch := s.chatChannel(ctx, NormalizedSignal{}); ch != nil {
		t.Fatalf("no channel label must resolve nil, got %v", ch.Name)
	}
	if ch := s.chatChannel(ctx, NormalizedSignal{Labels: map[string]string{LabelChatChannel: "nope"}}); ch != nil {
		t.Fatalf("unknown channel must resolve nil, got %v", ch.Name)
	}
	if ch := s.chatChannel(ctx, NormalizedSignal{Labels: map[string]string{LabelChatChannel: "known"}}); ch == nil || ch.Name != "known" {
		t.Fatalf("known channel must resolve, got %v", ch)
	}
}

// drainOneIngestTask claims exactly one task off SignalIngestQueue and runs
// HandleIngestSignalTask against it DIRECTLY, settling or failing it on the
// bus by hand — the in.signal counterpart of busutil_test.go's
// drainOneConvTask, for a test that only wants the handler's own returned
// error. Every sigServer-built Server shares its OpQueue's Bus with s.Bus
// (signals_test.go's own sigServer), so there is no separate bus parameter
// to thread through.
func drainOneIngestTask(t *testing.T, s *Server) (commandbus.Task, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task, err := s.Bus.Claim(ctx, SignalIngestQueue, "test")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	handleErr := s.HandleIngestSignalTask(context.Background(), task)
	leaseToken := ""
	if task.Lease != nil {
		leaseToken = task.Lease.Token
	}
	if handleErr == nil {
		if err := s.Bus.Settle(context.Background(), task.UID, leaseToken); err != nil {
			t.Fatalf("settle: %v", err)
		}
	} else if err := s.Bus.Fail(context.Background(), task.UID, leaseToken,
		commandbus.ParseErrorCode(handleErr.Error()), handleErr.Error()); err != nil {
		t.Fatalf("fail: %v", err)
	}
	return task, handleErr
}

// drainAllIngestTasks drains n tasks off SignalIngestQueue in submission
// order, failing the test on the first handler error — the common case
// where a test cares only about the eventual effect, matching
// acceptendpoints_test.go's drainAllConvTasks for ConvQueue.
func drainAllIngestTasks(t *testing.T, s *Server, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := drainOneIngestTask(t, s); err != nil {
			t.Fatalf("drain %d: %v", i, err)
		}
	}
}

// TestHandleIngestSignalTaskFailedStoreFailsTheTaskAndARetrySucceeds moves
// (and reframes) TestPublishFailedStoreIsNotAcknowledgedAndResendIsDeduplicated
// from publish_test.go: the store (publishOne) now runs in the consumer, so
// a Create failure fails the TASK — the accept call itself has already
// long since answered 202 (design D-1: "the adapter was told 202" even
// though nothing is stored yet). Re-running the SAME task (the bus's own
// bounded retry, not a fresh adapter re-send) is what eventually stores it.
func TestHandleIngestSignalTaskFailedStoreFailsTheTaskAndARetrySucceeds(t *testing.T) {
	source := sigTestSource("fail-src", "ad")
	base := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(source)
	failOnce := true
	failFirst := interceptor.Funcs{Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
		if _, isSignal := obj.(*agentopsv1alpha1.Signal); isSignal && failOnce {
			failOnce = false
			return apierrors.NewInternalError(errors.New("etcd unavailable"))
		}
		return cl.Create(ctx, obj, opts...)
	}}
	c := base.WithInterceptorFuncs(failFirst).Build()
	s := &Server{
		Reader: c, Client: c, Namespace: sigTestNamespace, AdapterToken: "sig-test-master",
		Conversations: testConversations(t, c, sigTestNamespace), Signals: testSignals(t, c, sigTestNamespace),
		Activity: activity.New(16), Bus: memory.Open(),
	}

	rec := postSig(s, source.Name, []map[string]any{{"initiationId": "i-1", "signalId": "m-1", "payload": "x"}})
	if rec.Code != 202 {
		t.Fatalf("accept must still be 202 even though the store behind it will fail: %d %s", rec.Code, rec.Body.String())
	}

	// First attempt: the store fails, so the task fails — nothing stored,
	// exactly the gap design D-1 names ("a signal whose task dies is
	// visible but not stored").
	if _, err := drainOneIngestTask(t, s); err == nil {
		t.Fatal("want the task to fail when the store fails")
	}
	if n := len(listSignals(t, c)); n != 0 {
		t.Fatalf("a failed store must leave no Signal, got %d", n)
	}

	// The bus's own retry of the SAME task (not a second adapter POST):
	// the interceptor now lets the Create through.
	if _, err := drainOneIngestTask(t, s); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("after the retry the signal must exist exactly once, got %d", n)
	}
}

// chatChannelObj and chatPost moved from publish_test.go — answerCommands
// (design D-5) now runs in HandleIngestSignalTask, not handleSignalInbound.
func chatChannelObj(name, adapter string) *agentopsv1alpha1.Channel {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, sigTestNamespace
	ch.Spec.Adapter = adapter
	return ch
}

func chatPost(t *testing.T, s *Server, source, channel, payload string) (int, map[string]any) {
	t.Helper()
	return respBody(t, s, source, map[string]any{
		"initiationId": "chat-" + payload, "kind": KindChat, "payload": payload,
		"labels": map[string]string{LabelChatChannel: channel},
	})
}

// TestHandleIngestSignalTaskChatCommandsThatOnlyAnswerStoreNoSignal moves
// TestPublishChatCommandsThatOnlyAnswerStoreNoSignal from publish_test.go:
// accept, then drain, THEN the command is answered and nothing is stored.
func TestHandleIngestSignalTaskChatCommandsThatOnlyAnswerStoreNoSignal(t *testing.T) {
	for _, text := range []string{"/pipelines", "/k8s-ops", "/close", "/exit"} {
		t.Run(text, func(t *testing.T) {
			source := sigTestSource("cmd-src", "tg")
			ch := chatChannelObj("cmd-chan", "cmdtype")
			s, c := sigServer(t, source, ch, sigTestPipeline("k8s-ops", source.Name))
			code, resp := chatPost(t, s, source.Name, ch.Name, text)
			if code != 202 {
				t.Fatalf("%d %v", code, resp)
			}
			drainAllIngestTasks(t, s, taskIDCount(resp))
			if n := len(listSignals(t, c)); n != 0 {
				t.Fatalf("%s answers in place and stores no Signal, got %d", text, n)
			}
			if op := claimOp(s.Ops, "cmdtype"); op == nil || op.Message == nil {
				t.Fatalf("%s must be answered on the surface", text)
			}
		})
	}
}

// TestHandleIngestSignalTaskAddressedAndPlainChatAreStoredVerbatim moves
// TestPublishAddressedAndPlainChatAreStoredVerbatim from publish_test.go.
func TestHandleIngestSignalTaskAddressedAndPlainChatAreStoredVerbatim(t *testing.T) {
	source := sigTestSource("chat-src", "tg")
	ch := chatChannelObj("chat-chan", "chattype")
	s, c := sigServer(t, source, ch, sigTestPipeline("k8s-ops", source.Name))

	for _, text := range []string{"/k8s-ops check nodes", "are we up?"} {
		code, resp := chatPost(t, s, source.Name, ch.Name, text)
		if code != 202 || resp["queued"] != float64(1) {
			t.Fatalf("%q: %d %v", text, code, resp)
		}
		drainAllIngestTasks(t, s, taskIDCount(resp))
	}
	sigs := listSignals(t, c)
	if len(sigs) != 2 {
		t.Fatalf("want 2 Signals, got %d", len(sigs))
	}
	seen := map[string]bool{}
	for _, sg := range sigs {
		seen[sg.Spec.Payload] = true
		if sg.Spec.Kind != KindChat || sg.Spec.Labels[LabelChatChannel] != ch.Name {
			t.Fatalf("chat Signal lost its kind or surface label: %+v", sg.Spec)
		}
	}
	if !seen["/k8s-ops check nodes"] || !seen["are we up?"] {
		t.Fatalf("payloads must be stored verbatim: %v", seen)
	}
	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil || len(convs.Items) != 0 {
		t.Fatalf("no conversation exists until a THIRD consumer (SignalReconciler) routes it: %v %d", err, len(convs.Items))
	}
	if op := claimOp(s.Ops, "chattype"); op != nil {
		t.Fatalf("the in.signal consumer tells no surface anything for a signal it merely stores: %+v", op)
	}
}

// TestHandleIngestSignalTaskSignalIDRedeliveryAcrossDifferentTasksDeduplicates
// is what TestPublishSignalIDDeduplicates (publish_test.go) could no longer
// prove once the commandbus's own CommandID dedup (ingestSignalCommandID,
// publish.go) started intercepting an accept-level resend before a second
// task — and so a second publishOne call — is ever created: within the
// dedup window, every resend of the same signalId resolves to the SAME
// task. Two genuinely distinct tasks (as a redelivery past that window, or
// any other reason the bus ends up handing out two, would look like)
// still dedupe at the Signal layer (publishOne's own AlreadyExists check,
// unchanged) — this exercises that path directly.
func TestHandleIngestSignalTaskSignalIDRedeliveryAcrossDifferentTasksDeduplicates(t *testing.T) {
	source := sigTestSource("dup-consumer-src", "ad")
	s, c := sigServer(t, source)
	payload := ingestSignalPayload{
		Source: source.Name,
		Signal: NormalizedSignal{InitiationID: "i-1", SignalID: "m-1", Payload: "x"},
	}.encode()

	for _, uid := range []string{"task-a", "task-b"} {
		if err := s.HandleIngestSignalTask(context.Background(), commandbus.Task{UID: uid, Payload: payload}); err != nil {
			t.Fatalf("task %s: %v", uid, err)
		}
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("a signalId that already landed must still dedupe across two independent tasks, got %d Signals", n)
	}
	if want := agentopsv1alpha1.SignalNameFor(source.Name, "m-1"); listSignals(t, c)[0].Name != want {
		t.Fatalf("name = %q, want the derived %q", listSignals(t, c)[0].Name, want)
	}
	if dup := countCode(s, activity.CodeDuplicate); dup != 1 {
		t.Fatalf("want exactly one duplicate event, got %d", dup)
	}
}

// TestHandleIngestSignalTaskLogFilteredFalseDoesNotSuppressTheDuplicateEvent
// moves TestSignalInboundLogFilteredFalseDoesNotSuppressOtherDropCodes from
// signals_test.go, exercised directly at the consumer for the same reason
// as the test above: logFiltered gates CodeFiltered only (handleSignalInbound,
// publish.go), never publishOne's own CodeDuplicate.
func TestHandleIngestSignalTaskLogFilteredFalseDoesNotSuppressTheDuplicateEvent(t *testing.T) {
	source := sigTestSource("attrfilter-dup-src", "attrfilter-t")
	source.Spec.LogFiltered = falsePtr()
	s, c := sigServer(t, source)
	payload := ingestSignalPayload{
		Source: source.Name,
		Signal: NormalizedSignal{InitiationID: "attrfilter-dup-1", SignalID: "m-1", Payload: "hello"},
	}.encode()

	for _, uid := range []string{"task-a", "task-b"} {
		if err := s.HandleIngestSignalTask(context.Background(), commandbus.Task{UID: uid, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(listSignals(t, c)); n != 1 {
		t.Fatalf("want one Signal, got %d", n)
	}

	events, _ := s.Activity.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalDropped && e.Code == activity.CodeDuplicate {
			found = true
		}
	}
	if !found {
		t.Fatal("logFiltered must not suppress the duplicate event")
	}
}

// TestHandleIngestSignalTaskRedeliveryOfTheSameTaskUIDCreatesOneSignal pins
// design D-6: a signal with NO signalId is named from the task's own UID
// (signalNameFromTask), so a redelivery of the exact same task — a lease
// lapse, or the bus's own retry after an ambiguous failure — derives the
// SAME name and Create's AlreadyExists dedups it too, same as a
// sender-given signalId already did.
func TestHandleIngestSignalTaskRedeliveryOfTheSameTaskUIDCreatesOneSignal(t *testing.T) {
	source := sigTestSource("redeliver-src", "ad")
	s, c := sigServer(t, source)
	task := commandbus.Task{UID: "task-uid-fixed", Payload: ingestSignalPayload{
		Source: source.Name,
		Signal: NormalizedSignal{InitiationID: "i-1", Payload: "x"}, // no SignalID
	}.encode()}

	for i := 0; i < 2; i++ {
		if err := s.HandleIngestSignalTask(context.Background(), task); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	sigs := listSignals(t, c)
	if len(sigs) != 1 {
		t.Fatalf("a redelivered task (same UID) must create exactly one Signal, got %d", len(sigs))
	}
	if want := signalNameFromTask(task.UID); sigs[0].Name != want {
		t.Fatalf("name = %q, want the task-derived %q", sigs[0].Name, want)
	}
}

// validObjectName moved from publish_test.go, alongside the test it now
// serves.
func validObjectName(name string) bool {
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// TestSignalNameFromTaskIsDeterministicDistinctAndValid replaces
// TestGeneratedSignalNamesSortInPublishOrderAndAreValid (publish_test.go):
// generatedSignalName/generatedTimeLen/generatedSuffixLen no longer exist
// (design D-6) — mirrors api/v1alpha1's own
// TestSignalNameForIsDeterministicSourceScopedAndBounded for the shape
// check, since SignalNameFor and signalNameFromTask share one scheme
// (sig-<32 hex>), just hashing a different input.
func TestSignalNameFromTaskIsDeterministicDistinctAndValid(t *testing.T) {
	a := signalNameFromTask("task-uid-1")
	if a != signalNameFromTask("task-uid-1") {
		t.Fatal("deterministic: the same task UID must always derive the same name")
	}
	if a == signalNameFromTask("task-uid-2") {
		t.Fatal("two different task UIDs must derive different names")
	}
	if len(a) != len("sig-")+32 || !strings.HasPrefix(a, "sig-") {
		t.Fatalf("name %q is not sig-<32 hex>", a)
	}
	if !validObjectName(a) {
		t.Fatalf("%q is not a valid object name", a)
	}
}

// TestIngestSignalCommandIDDerivesFromSignalIDWhenPresentElseFresh pins
// publish.go's own dedup-key contract (design D-1), which every Submit-level
// dedup assertion in publish_test.go and signals_test.go relies on: a
// signalId always derives the SAME CommandID scoped to its source, and a
// signal with none never collides with any other accept of it.
func TestIngestSignalCommandIDDerivesFromSignalIDWhenPresentElseFresh(t *testing.T) {
	a := ingestSignalCommandID("src-a", &NormalizedSignal{SignalID: "m-1"})
	if a != ingestSignalCommandID("src-a", &NormalizedSignal{SignalID: "m-1"}) {
		t.Fatal("the same source+signalId must always derive the same CommandID")
	}
	if a == ingestSignalCommandID("src-b", &NormalizedSignal{SignalID: "m-1"}) {
		t.Fatal("two sources must never collide on one signalId")
	}
	if a == ingestSignalCommandID("src-a", &NormalizedSignal{SignalID: "m-2"}) {
		t.Fatal("two signalIds must never collide")
	}
	noID1 := ingestSignalCommandID("src-a", &NormalizedSignal{})
	noID2 := ingestSignalCommandID("src-a", &NormalizedSignal{})
	if noID1 == noID2 {
		t.Fatal("a signal with no signalId must get a fresh CommandID every call, never reused")
	}
}
