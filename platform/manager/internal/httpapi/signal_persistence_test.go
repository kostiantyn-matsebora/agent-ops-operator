// signal-persistence section 2: persist-then-ack ingest and the shared
// routing path. These tests pin the NEW behaviour the spec adds on top of
// signals_test.go's existing coverage (which stays green unchanged) —
// CR-before-response ordering, absorb into a still-open Signal, the
// backlog-full split between the machine and chat lanes, and that calling
// the shared routing function twice on one Signal never duplicates a
// conversation.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/signalroute"
)

// onlySignal returns the single Signal CR carrying initiationID, failing
// the test if there is not exactly one.
func onlySignal(t *testing.T, c client.Client, initiationID string) *agentopsv1alpha1.Signal {
	t.Helper()
	var list agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &list, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	var matches []agentopsv1alpha1.Signal
	for i := range list.Items {
		if list.Items[i].Spec.InitiationID == initiationID {
			matches = append(matches, list.Items[i])
		}
	}
	if len(matches) != 1 {
		t.Fatalf("want exactly one Signal for initiationId %q, got %d: %+v", initiationID, len(matches), matches)
	}
	return &matches[0]
}

// "The CR exists before the acknowledgement": the Signal CR is created,
// owned by the named SignalSource, before the 202/200 response is ever
// written — exercised here by checking its existence and ownerRef right
// after the synchronous call returns, which is the only window there is in
// a single-process test.
func TestSignalInboundPersistsSignalCRBeforeResponding(t *testing.T) {
	source := sigTestSource("persist-src", "persist-t")
	pipeline := sigTestPipeline("persist-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "persist-1", "payload": "hello"},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	sig := onlySignal(t, c, "persist-1")
	if len(sig.OwnerReferences) != 1 || sig.OwnerReferences[0].Name != source.Name || sig.OwnerReferences[0].Kind != "SignalSource" {
		t.Fatalf("Signal must be owned by its SignalSource at creation, got %+v", sig.OwnerReferences)
	}
	if sig.Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatalf("a signal with one claimant and nothing blocking it must route to completion, got %q", sig.Status.Phase)
	}
}

// "A signal that cannot be persisted is not acknowledged": a Signal Create
// that fails must surface as a 500, never a 202/200 queued response — the
// adapter's at-least-once re-send is what recovers, and it must believe
// nothing landed.
func TestSignalInboundUnpersistableSignalIsNotAcknowledged(t *testing.T) {
	source := sigTestSource("unpersist-src", "unpersist-t")
	pipeline := sigTestPipeline("unpersist-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)
	// A Server whose Client cannot create a Signal — the fake client refuses
	// a Create for a GVK with no registered scheme entry, so unregistering
	// Signal from THIS server's view is enough to force the failure without
	// a bespoke failing-client wrapper.
	s.Client = failingCreateClient{c}

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "unpersist-1", "payload": "hello"},
	})
	if rec.Code == 200 {
		t.Fatalf("an unpersistable signal must never be acknowledged as accepted, got 200: %s", rec.Body.String())
	}
}

// failingCreateClient wraps a client.Client, refusing every Create — the
// smallest double that can stand in for "the Signal CR cannot be written".
type failingCreateClient struct{ client.Client }

func (f failingCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return errCreateRefused
}

var errCreateRefused = errors.New("create refused (test double)")

// "A repeat is absorbed into the open Signal": a second arrival sharing the
// first's initiationId, while the Signal is still Pending (its one claimant
// blocked on a full backlog), is folded into the SAME CR — payload
// appended, occurrence count bumped — never a second CR, and the claimant
// is retried rather than re-snapshotted.
func TestSignalInboundAbsorbsRepeatIntoPendingSignal(t *testing.T) {
	source := sigTestSource("absorb-pending-src", "absorb-pending-t")
	pipeline := sigTestPipeline("absorb-pending-pipe", source.Name)
	// One Pending conversation already occupies the only slot, so this
	// signal's own claimant can never create one.
	blocker := &agentopsv1alpha1.Conversation{}
	blocker.Name, blocker.Namespace = "blocker", sigTestNamespace
	blocker.Status.Phase = agentopsv1alpha1.ConversationPending
	s, c := sigServer(t, source, pipeline, blocker)
	s.MaxQueuedConversations = 1

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "absorb-pending-1", "payload": "first"},
	})
	if rec.Code != 200 {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	sig := onlySignal(t, c, "absorb-pending-1")
	if sig.Status.Phase != agentopsv1alpha1.SignalPending {
		t.Fatalf("a blocked claimant must leave the signal Pending, got %q", sig.Status.Phase)
	}
	if len(sig.Status.Claimants) != 1 || sig.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouting {
		t.Fatalf("the one claimant must stay Routing, got %+v", sig.Status.Claimants)
	}
	if sig.Spec.Occurrences != 1 || sig.Spec.Payload != "first" {
		t.Fatalf("unexpected spec after first arrival: %+v", sig.Spec)
	}

	rec = postSig(s, source.Name, []map[string]any{
		{"initiationId": "absorb-pending-1", "payload": "second"},
	})
	if rec.Code != 200 {
		t.Fatalf("second: %d %s", rec.Code, rec.Body.String())
	}

	if matches := countSignalsByInitiation(t, c, "absorb-pending-1"); matches != 1 {
		t.Fatalf("absorb must never create a second CR, got %d", matches)
	}
	absorbed := onlySignal(t, c, "absorb-pending-1")
	if absorbed.Spec.Occurrences != 2 {
		t.Fatalf("occurrences must be bumped by absorb, got %d", absorbed.Spec.Occurrences)
	}
	if absorbed.Spec.Payload != "first\n---\nsecond" {
		t.Fatalf("absorbed payload must join both occurrences, got %q", absorbed.Spec.Payload)
	}
	if absorbed.Status.Phase != agentopsv1alpha1.SignalPending {
		t.Fatalf("still blocked: phase must stay Pending, got %q", absorbed.Status.Phase)
	}

	if convs := listSigConversations(t, c); len(convs.Items) != 1 { // only the pre-seeded blocker
		t.Fatalf("a blocked claimant must never create a conversation, got %d", len(convs.Items))
	}
}

// countSignalsByInitiation counts the Signal CRs carrying initiationID.
func countSignalsByInitiation(t *testing.T, c client.Client, initiationID string) int {
	t.Helper()
	var list agentopsv1alpha1.SignalList
	if err := c.List(context.Background(), &list, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	var matches int
	for i := range list.Items {
		if list.Items[i].Spec.InitiationID == initiationID {
			matches++
		}
	}
	return matches
}

// listSigConversations lists every Conversation in the test namespace.
func listSigConversations(t *testing.T, c client.Client) *agentopsv1alpha1.ConversationList {
	t.Helper()
	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	return &convs
}

// "A repeat is absorbed into the open Signal", exercised against a
// PartiallyRouted-shaped Signal: one claimant already Routed (with its own
// open conversation), the other still Routing. Nothing in this change's own
// code path can REACH PartiallyRouted without a second claimant stuck
// behind the backlog at the exact moment the first one is not — this test
// manufactures that status directly (legitimate: it is RouteClaimants' own
// contract under test, not a claim about how the state was reached) and
// then exercises it exactly as the reconciler-era retry would: absorb, then
// re-attempt only the non-terminal claimant, leaving the Routed one and its
// conversation untouched.
func TestSignalInboundAbsorbsRepeatIntoPartiallyRoutedSignal(t *testing.T) {
	source := sigTestSource("absorb-partial-src", "absorb-partial-t")
	pipeA := sigTestPipeline("absorb-partial-a", source.Name)
	pipeB := sigTestPipeline("absorb-partial-b", source.Name)

	existing := &agentopsv1alpha1.Conversation{}
	existing.Name, existing.Namespace = "absorb-partial-existing", sigTestNamespace
	existing.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: pipeA.Name}
	existing.Spec.ConversationID = "absorb-partial-1"
	existing.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-absorb-partial-a"}

	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = "sig-absorb-partial", sigTestNamespace
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: source.Name},
		Adapter:      source.Spec.Adapter,
		InitiationID: "absorb-partial-1",
		Payload:      "first",
		Occurrences:  1,
	}
	sig.Status = agentopsv1alpha1.SignalStatus{
		Phase: agentopsv1alpha1.SignalPartiallyRouted,
		Claimants: []agentopsv1alpha1.SignalClaimant{
			{
				Ref:             agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: pipeA.Name},
				State:           agentopsv1alpha1.ClaimantRouted,
				ConversationRef: &agentopsv1alpha1.ObjectRef{Name: existing.Name},
			},
			{
				Ref:   agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: pipeB.Name},
				State: agentopsv1alpha1.ClaimantRouting,
			},
		},
	}

	s, c := sigServer(t, source, pipeA, pipeB, existing, sig)

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "absorb-partial-1", "payload": "second"},
	})
	if rec.Code != 200 {
		t.Fatalf("absorbed repeat: %d %s", rec.Code, rec.Body.String())
	}

	absorbed := onlySignal(t, c, "absorb-partial-1")
	if absorbed.Name != sig.Name {
		t.Fatalf("absorb must reuse the SAME CR, got a different object %q", absorbed.Name)
	}
	if absorbed.Spec.Occurrences != 2 || absorbed.Spec.Payload != "first\n---\nsecond" {
		t.Fatalf("unexpected absorbed spec: %+v", absorbed.Spec)
	}
	if absorbed.Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatalf("both claimants terminal with one Routed must roll up to Routed, got %q", absorbed.Status.Phase)
	}
	claimantA := claimantNamed(absorbed, pipeA.Name)
	claimantB := claimantNamed(absorbed, pipeB.Name)
	if claimantA == nil || claimantA.State != agentopsv1alpha1.ClaimantRouted || claimantA.ConversationRef == nil || claimantA.ConversationRef.Name != existing.Name {
		t.Fatalf("the already-Routed claimant must be left untouched, got %+v", claimantA)
	}
	if claimantB == nil || claimantB.State != agentopsv1alpha1.ClaimantRouted {
		t.Fatalf("the Routing claimant must now be routed, got %+v", claimantB)
	}

	convs := listSigConversations(t, c)
	if len(convs.Items) != 2 { // the pre-existing one, plus pipeB's own new one
		t.Fatalf("want exactly 2 conversations (one per claimant), got %d: %+v", len(convs.Items), convs.Items)
	}
	for i := range convs.Items {
		if convs.Items[i].Name == existing.Name && len(convs.Items[i].Spec.Inputs) != 0 {
			t.Fatalf("the already-Routed claimant's conversation must be untouched by this absorb, got inputs %+v",
				convs.Items[i].Spec.Inputs)
		}
	}
}

// claimantNamed returns the claimant whose ref names pipeline, or nil.
func claimantNamed(sig *agentopsv1alpha1.Signal, pipeline string) *agentopsv1alpha1.SignalClaimant {
	for i := range sig.Status.Claimants {
		if sig.Status.Claimants[i].Ref.Name == pipeline {
			return &sig.Status.Claimants[i]
		}
	}
	return nil
}

// conversation-capacity's MODIFIED "pending backlog is bounded": a machine
// signal (default kind, alert) hitting a full backlog waits — no drop
// reason, no conversation, and the Signal CR stays Pending with its
// claimant Routing.
func TestSignalInboundMachineSignalWaitsWhenBacklogFull(t *testing.T) {
	source := sigTestSource("wait-src", "wait-t")
	pipeline := sigTestPipeline("wait-pipe", source.Name)
	blocker := &agentopsv1alpha1.Conversation{}
	blocker.Name, blocker.Namespace = "wait-blocker", sigTestNamespace
	blocker.Status.Phase = agentopsv1alpha1.ConversationPending
	s, c := sigServer(t, source, pipeline, blocker)
	s.MaxQueuedConversations = 1

	rec := postSig(s, source.Name, []map[string]any{
		{"initiationId": "wait-1", "payload": "hello"},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["reason"]; ok {
		t.Fatalf("a waiting machine signal must carry no drop reason, got %v", resp)
	}
	if resp["queued"] != float64(0) || resp["conversations"] != float64(0) {
		t.Fatalf("a waiting machine signal must report nothing landed yet, got %v", resp)
	}

	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 { // only the pre-seeded blocker
		t.Fatalf("a waiting signal must create no conversation, got %d", len(convs.Items))
	}

	sig := onlySignal(t, c, "wait-1")
	if sig.Status.Phase != agentopsv1alpha1.SignalPending {
		t.Fatalf("a backlog-blocked machine signal must stay Pending, got %q", sig.Status.Phase)
	}
	if len(sig.Status.Claimants) != 1 || sig.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouting {
		t.Fatalf("its one claimant must stay Routing, got %+v", sig.Status.Claimants)
	}
}

// conversation-capacity's MODIFIED requirement, chat half: unlike a machine
// signal, chat keeps its immediate decline — the Signal is recorded
// Dropped(capacity), straight to a terminal phase with NO claimant snapshot
// at all, and the person is told on the surface they typed on.
func TestSignalInboundChatSignalDeclinesWhenBacklogFull(t *testing.T) {
	source := sigTestSource("chat-wait-src", "chat-wait-t")
	source.Spec.Config = nil
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = "chat-wait-chan", sigTestNamespace
	pipeline := sigTestPipeline("chat-wait-pipe", source.Name)
	blocker := &agentopsv1alpha1.Conversation{}
	blocker.Name, blocker.Namespace = "chat-wait-blocker", sigTestNamespace
	blocker.Status.Phase = agentopsv1alpha1.ConversationPending
	s, c := sigServer(t, source, pipeline, ch, blocker)
	s.MaxQueuedConversations = 1
	s.Ops = &chat.OpQueue{Client: c, Namespace: sigTestNamespace}

	rec := postSig(s, source.Name, []map[string]any{
		{
			"initiationId": "chat-wait-1", "kind": KindChat, "payload": "are we up?",
			"labels": map[string]string{LabelChatChannel: ch.Name},
		},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["reason"] != ReasonAtCapacity {
		t.Fatalf("a declined chat signal must report reason=%q, got %v", ReasonAtCapacity, resp["reason"])
	}

	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 { // only the pre-seeded blocker
		t.Fatalf("a declined chat signal must create no conversation, got %d", len(convs.Items))
	}

	sig := onlySignal(t, c, "chat-wait-1")
	if sig.Status.Phase != agentopsv1alpha1.SignalDropped || sig.Status.Reason != agentopsv1alpha1.SignalDropCapacity {
		t.Fatalf("a declined chat signal must be Dropped(capacity), got phase=%q reason=%q",
			sig.Status.Phase, sig.Status.Reason)
	}
	if len(sig.Status.Claimants) != 0 {
		t.Fatalf("a Dropped signal must never snapshot claimants, got %+v", sig.Status.Claimants)
	}
}

// "A crash between create and status write does not duplicate": calling
// signalroute.RouteClaimants twice on the same Signal — standing in for a
// retry after a would-be crash, since there is no reconciler yet to crash
// mid-attempt — must never create a second conversation or a second input.
func TestRouteClaimantsCalledTwiceDoesNotDuplicate(t *testing.T) {
	source := sigTestSource("idem-src", "idem-t")
	pipeline := sigTestPipeline("idem-pipe", source.Name)
	s, c := sigServer(t, source, pipeline)

	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = "sig-idem", sigTestNamespace
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: source.Name},
		InitiationID: "idem-1",
		Payload:      "hello",
		Occurrences:  1,
	}
	if err := c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}

	deps := signalroute.Deps{Client: s.Client, Reader: s.Reader, Namespace: sigTestNamespace, MaxQueued: 50}
	if err := signalroute.RouteClaimants(context.Background(), deps, sig); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if sig.Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatalf("want Routed after the first call, got %q", sig.Status.Phase)
	}

	// Re-fetch, exactly as a retrying caller would — never reuse the
	// in-memory object a crash would have lost.
	var reloaded agentopsv1alpha1.Signal
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigTestNamespace, Name: sig.Name}, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := signalroute.RouteClaimants(context.Background(), deps, &reloaded); err != nil {
		t.Fatalf("second call: %v", err)
	}

	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigTestNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 1 {
		t.Fatalf("a second call must never create a second conversation, got %d", len(convs.Items))
	}
	if len(convs.Items[0].Spec.Inputs) != 1 {
		t.Fatalf("a second call must never append a second input, got %d", len(convs.Items[0].Spec.Inputs))
	}
}
