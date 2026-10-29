package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/signalroute"
)

// The Signal consumer and housekeeping (signal-persistence D-3..D-6, D-10,
// D-11, D-13), driven through Reconcile against a fake client — the shape the
// production reconciler sees, minus the work queue (envtest covers that).

const sigNS = "agent-ops"

// sigEnv is one reconciler over a fake cluster, with its activity log, its
// channel-operation queue and a controllable clock.
type sigEnv struct {
	t    *testing.T
	c    client.Client
	r    *SignalReconciler
	acts *activity.Log
	ops  *chat.OpQueue
	now  time.Time
}

func newSigEnv(t *testing.T, objs ...client.Object) *sigEnv {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentopsv1alpha1.Signal{}, &agentopsv1alpha1.Conversation{}, &agentopsv1alpha1.SignalSource{}).
		WithObjects(objs...).Build()
	acts := activity.New(256)
	ops := &chat.OpQueue{Client: c, Namespace: sigNS, Registry: chat.NewRegistry(), Activity: acts}
	e := &sigEnv{t: t, c: c, acts: acts, ops: ops, now: time.Now()}
	e.r = &SignalReconciler{
		Client: c, Reader: c, Activity: acts, Ops: ops, Cooldowns: ingest.NewCooldownBook(),
		MaxQueued: 50, Identity: "replica-a", Now: func() time.Time { return e.now },
	}
	return e
}

func (e *sigEnv) reconcile(name string) ctrl.Result {
	e.t.Helper()
	res, err := e.r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: sigNS, Name: name}})
	if err != nil {
		e.t.Fatalf("Reconcile(%s): %v", name, err)
	}
	return res
}

func (e *sigEnv) signal(name string) *agentopsv1alpha1.Signal {
	e.t.Helper()
	var s agentopsv1alpha1.Signal
	if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: sigNS, Name: name}, &s); err != nil {
		e.t.Fatal(err)
	}
	return &s
}

func (e *sigEnv) conversations() []agentopsv1alpha1.Conversation {
	e.t.Helper()
	var l agentopsv1alpha1.ConversationList
	if err := e.c.List(context.Background(), &l, client.InNamespace(sigNS)); err != nil {
		e.t.Fatal(err)
	}
	return l.Items
}

func (e *sigEnv) inputs() []agentopsv1alpha1.ConversationInput {
	e.t.Helper()
	var l agentopsv1alpha1.ConversationInputList
	if err := e.c.List(context.Background(), &l, client.InNamespace(sigNS)); err != nil {
		e.t.Fatal(err)
	}
	return l.Items
}

func (e *sigEnv) source(name string) *agentopsv1alpha1.SignalSource {
	e.t.Helper()
	var s agentopsv1alpha1.SignalSource
	if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: sigNS, Name: name}, &s); err != nil {
		e.t.Fatal(err)
	}
	return &s
}

func (e *sigEnv) events(kind, code string) int {
	evs, _ := e.acts.Since("", 0)
	n := 0
	for _, ev := range evs {
		if ev.Kind == kind && (code == "" || ev.Code == code) {
			n++
		}
	}
	return n
}

func (e *sigEnv) exists(name string) bool {
	err := e.c.Get(context.Background(), types.NamespacedName{Namespace: sigNS, Name: name}, &agentopsv1alpha1.Signal{})
	if err != nil && !apierrors.IsNotFound(err) {
		e.t.Fatal(err)
	}
	return err == nil
}

func fxSource(name string) *agentopsv1alpha1.SignalSource {
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, sigNS
	src.UID = types.UID("uid-" + name)
	src.Spec.Adapter = "adapter-" + name
	return src
}

func fxPipeline(name string, sources ...string) *agentopsv1alpha1.Pipeline {
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = name, sigNS
	for _, s := range sources {
		p.Spec.SignalSourceRefs = append(p.Spec.SignalSourceRefs, agentopsv1alpha1.ObjectRef{Name: s})
	}
	p.Spec.AgentCapabilitySpec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "prof-" + name}
	p.Status.Conditions = []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready", LastTransitionTime: metav1.Now(),
	}}
	return p
}

func fxChannel(name string) *agentopsv1alpha1.Channel {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, sigNS
	ch.Spec.Adapter = "chat-" + name
	return ch
}

// fxSignal builds a Signal owned by source, created at the given time.
func fxSignal(name string, source *agentopsv1alpha1.SignalSource, initiationID string, created time.Time) *agentopsv1alpha1.Signal {
	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = name, sigNS
	sig.CreationTimestamp = metav1.NewTime(created)
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: source.Name},
		Adapter:      source.Spec.Adapter,
		InitiationID: initiationID,
		Payload:      "payload of " + name,
	}
	scheme := runtime.NewScheme()
	_ = agentopsv1alpha1.AddToScheme(scheme)
	if err := controllerutil.SetControllerReference(source, sig, scheme); err != nil {
		panic(err)
	}
	return sig
}

func chatSignal(name string, source *agentopsv1alpha1.SignalSource, initiationID, channel, payload string, created time.Time) *agentopsv1alpha1.Signal {
	sig := fxSignal(name, source, initiationID, created)
	sig.Spec.Kind = "chat"
	sig.Spec.Payload = payload
	sig.Spec.Labels = map[string]string{
		agentopsv1alpha1.LabelChatChannel: channel,
		agentopsv1alpha1.LabelChatMessage: "msg-" + name,
	}
	sig.Spec.Reader = "reader-key"
	return sig
}

func ref(kind agentopsv1alpha1.ClaimantKind, name string) agentopsv1alpha1.ClaimantRef {
	return agentopsv1alpha1.ClaimantRef{Kind: kind, Name: name}
}

func claimant(name string, state agentopsv1alpha1.ClaimantState) agentopsv1alpha1.SignalClaimant {
	return agentopsv1alpha1.SignalClaimant{Ref: ref(agentopsv1alpha1.ClaimantKindPipeline, name), State: state}
}

func pendingConversation(name string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name, c.Namespace = name, sigNS
	c.Status.Phase = agentopsv1alpha1.ConversationPending
	return c
}

// ---- the consumer routes ------------------------------------------------------

func TestConsumerRoutesAndRecordsTheTerminalWrite(t *testing.T) {
	src := fxSource("alerts")
	sig := fxSignal("sig-1", src, "disk-full", time.Now())
	sig.Spec.Severity = agentopsv1alpha1.SeverityWarning
	sig.Spec.Labels = map[string]string{"node": "a"}
	lease := &coordinationv1.Lease{}
	lease.Name, lease.Namespace = LeaderLeaseName, sigNS
	holder := "replica-a_6b1e"
	lease.Spec.HolderIdentity = &holder
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"), sig, lease)

	e.reconcile("sig-1")

	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || got.Status.Attempts != 1 ||
		got.Status.FirstAttemptAt == nil || got.Status.CompletedAt == nil || got.Status.ConsumedAt == nil {
		t.Fatalf("a terminal Signal must record phase, attempts, firstAttemptAt, completedAt and consumedAt: %+v", got.Status)
	}
	if got.Status.ConsumedBy != holder {
		t.Fatalf("consumedBy = %q, want the leader Lease's holderIdentity %q", got.Status.ConsumedBy, holder)
	}
	if len(got.Status.Claimants) != 1 || got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouted ||
		got.Status.Claimants[0].ConversationRef == nil {
		t.Fatalf("claimant not recorded Routed with its conversation: %+v", got.Status.Claimants)
	}
	convs := e.conversations()
	if len(convs) != 1 || convs[0].Name != got.Status.Claimants[0].ConversationRef.Name {
		t.Fatalf("want exactly the recorded conversation, got %d", len(convs))
	}
	conv := convs[0]
	if conv.Spec.ConversationID != "disk-full" || conv.Spec.PipelineRef == nil || conv.Spec.PipelineRef.Name != "ops" {
		t.Fatalf("conversation must adopt the initiationId and record its claimant: %+v", conv.Spec)
	}
	if conv.Spec.Signal == nil || conv.Spec.Signal.Severity != agentopsv1alpha1.SeverityWarning {
		t.Fatalf("severity must reach the provenance: %+v", conv.Spec.Signal)
	}
	if len(conv.Spec.Inputs) != 1 || conv.Spec.Inputs[0].PayloadRef == nil ||
		conv.Spec.Inputs[0].PayloadRef.Name != signalroute.InputName("sig-1", ref(agentopsv1alpha1.ClaimantKindPipeline, "ops")) {
		t.Fatalf("the input must be named for the Signal and claimant: %+v", conv.Spec.Inputs)
	}
	ins := e.inputs()
	if len(ins) != 1 || ins[0].Spec.Severity != agentopsv1alpha1.SeverityWarning || ins[0].Spec.Payload != "payload of sig-1" {
		t.Fatalf("input not carrying the message: %+v", ins)
	}
	if s := e.source("alerts"); s.Status.ReceivedTotal != 1 || s.Status.LastReceived == nil {
		t.Fatalf("the source's receipt counters must move once: %+v", s.Status)
	}
	if e.events(activity.KindSignalClaimed, "") != 1 {
		t.Fatal("the claim must be recorded")
	}
}

func TestConsumerFallsBackToItsOwnIdentityWithoutALease(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"), fxSignal("sig-1", src, "x", time.Now()))
	e.reconcile("sig-1")
	if got := e.signal("sig-1").Status.ConsumedBy; got != "replica-a" {
		t.Fatalf("consumedBy = %q, want the replica identity", got)
	}
}

func TestConsumerFansOutToEveryReadyClaimantOnce(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t, src, fxPipeline("one", "alerts"), fxPipeline("two", "alerts"), fxSignal("sig-1", src, "x", time.Now()))
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || len(got.Status.Claimants) != 2 || len(e.conversations()) != 2 {
		t.Fatalf("both claimants must be routed to their own conversation: %+v / %d", got.Status, len(e.conversations()))
	}
	if e.source("alerts").Status.ReceivedTotal != 1 {
		t.Fatal("receivedTotal moves once per signal, not once per claimant")
	}
}

func TestConsumerOneClaimantsConversationNeverAnswersForAnother(t *testing.T) {
	src := fxSource("alerts")
	// "one" already has an open conversation with the id; "two" does not.
	existing := &agentopsv1alpha1.Conversation{}
	existing.Name, existing.Namespace = "existing", sigNS
	existing.Spec.ConversationID = "x"
	existing.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: "one"}
	e := newSigEnv(t, src, fxPipeline("one", "alerts"), fxPipeline("two", "alerts"), existing, fxSignal("sig-1", src, "x", time.Now()))
	e.reconcile("sig-1")

	convs := e.conversations()
	if len(convs) != 2 {
		t.Fatalf("two's attempt must create its own conversation; got %d", len(convs))
	}
	for _, c := range convs {
		if c.Name == "existing" {
			if len(c.Spec.Inputs) != 1 {
				t.Fatalf("one's conversation takes exactly one's append: %+v", c.Spec.Inputs)
			}
			continue
		}
		if c.Spec.PipelineRef == nil || c.Spec.PipelineRef.Name != "two" {
			t.Fatalf("the new conversation belongs to the second claimant: %+v", c.Spec)
		}
	}
}

// ---- drops --------------------------------------------------------------------

func TestConsumerDropsUnclaimedLoudly(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t, src, fxSignal("sig-1", src, "x", time.Now()))
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalDropped || got.Status.Reason != agentopsv1alpha1.SignalDropUnclaimed ||
		got.Status.CompletedAt == nil || len(got.Status.Claimants) != 0 {
		t.Fatalf("want Dropped/unclaimed before any snapshot: %+v", got.Status)
	}
	if len(e.conversations()) != 0 {
		t.Fatal("an unclaimed signal creates no conversation")
	}
	if e.events(activity.KindSignalDropped, activity.CodeUnclaimed) != 1 {
		t.Fatal("an unclaimed drop is as loud as the unwired drop always was")
	}
}

func TestConsumerChatDropsTellTheSurface(t *testing.T) {
	cases := []struct {
		name    string
		objs    func(src *agentopsv1alpha1.SignalSource) []client.Object
		backlog int
		payload string
		reason  agentopsv1alpha1.SignalDropReason
		code    string
		notice  string
	}{
		{"unclaimed", func(*agentopsv1alpha1.SignalSource) []client.Object { return nil }, 0, "hello",
			agentopsv1alpha1.SignalDropUnclaimed, activity.CodeUnclaimed, "Nothing here is wired"},
		{"ambiguous", func(*agentopsv1alpha1.SignalSource) []client.Object {
			return []client.Object{fxPipeline("one", "chat"), fxPipeline("two", "chat")}
		}, 0, "hello", agentopsv1alpha1.SignalDropAmbiguous, activity.CodeAmbiguous, "don't know who you meant"},
		{"capacity", func(*agentopsv1alpha1.SignalSource) []client.Object {
			return []client.Object{fxPipeline("one", "chat")}
		}, 50, "hello", agentopsv1alpha1.SignalDropCapacity, activity.CodeAtCapacity, "At capacity"},
		{"capacity-addressed", func(*agentopsv1alpha1.SignalSource) []client.Object {
			return []client.Object{fxPipeline("one", "chat")}
		}, 50, "/one do it", agentopsv1alpha1.SignalDropCapacity, activity.CodeAtCapacity, "At capacity"},
		{"unknown-target", func(*agentopsv1alpha1.SignalSource) []client.Object {
			return []client.Object{fxPipeline("one", "chat")}
		}, 0, "/nope do it", agentopsv1alpha1.SignalDropUnclaimed, activity.CodeUnclaimed, "Unknown pipeline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := fxSource("chat")
			ch := fxChannel("surface")
			objs := []client.Object{src, ch, chatSignal("sig-1", src, "c-1", "surface", tc.payload, time.Now())}
			objs = append(objs, tc.objs(src)...)
			for i := 0; i < tc.backlog; i++ {
				objs = append(objs, pendingConversation(fmt.Sprintf("pending-%d", i)))
			}
			e := newSigEnv(t, objs...)
			e.reconcile("sig-1")

			got := e.signal("sig-1")
			if got.Status.Phase != agentopsv1alpha1.SignalDropped || got.Status.Reason != tc.reason {
				t.Fatalf("want Dropped/%s, got %+v", tc.reason, got.Status)
			}
			if e.events(activity.KindSignalDropped, tc.code) != 1 {
				t.Fatalf("want one %s drop event", tc.code)
			}
			op := e.ops.Claim(ch.Spec.Adapter)
			if op == nil || op.Message == nil || !strings.Contains(op.Message.Body, tc.notice) {
				t.Fatalf("the surface must be told %q, got %+v", tc.notice, op)
			}
			if op.Message.InReplyTo != "msg-sig-1" {
				t.Fatalf("the notice must answer the message that provoked it, got %q", op.Message.InReplyTo)
			}
			if tc.name == "ambiguous" && len(op.Message.Choices) != 2 {
				t.Fatalf("the ambiguous notice offers each claimant as a choice: %+v", op.Message.Choices)
			}
		})
	}
}

func TestConsumerChatWithoutAChannelIsUnaddressable(t *testing.T) {
	src := fxSource("chat")
	e := newSigEnv(t, src, fxPipeline("one", "chat"), chatSignal("sig-1", src, "c-1", "ghost", "hello", time.Now()))
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalDropped || got.Status.Reason != agentopsv1alpha1.SignalDropUnaddressable {
		t.Fatalf("want Dropped/unaddressable, got %+v", got.Status)
	}
	if e.events(activity.KindSignalDropped, activity.CodeUnaddressable) != 1 {
		t.Fatal("an unaddressable drop is loud")
	}
}

func TestConsumerCooldownSuppressesARepeatAndIsNotLoud(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now()
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"),
		fxSignal("sig-1", src, "flap", base), fxSignal("sig-2", src, "flap", base.Add(time.Second)))
	e.reconcile("sig-1")
	e.reconcile("sig-2")

	if got := e.signal("sig-1").Status.Phase; got != agentopsv1alpha1.SignalRouted {
		t.Fatalf("the first is routed, got %s", got)
	}
	second := e.signal("sig-2")
	if second.Status.Phase != agentopsv1alpha1.SignalDropped || second.Status.Reason != agentopsv1alpha1.SignalDropCooldown {
		t.Fatalf("the repeat inside the window is Dropped/cooldown, got %+v", second.Status)
	}
	if len(e.conversations()) != 1 {
		t.Fatal("a suppressed repeat opens nothing")
	}
	if e.events(activity.KindSignalDropped, "") != 0 {
		t.Fatal("cooldown is the one drop that is not surfaced as an event")
	}
	if cd := e.source("alerts").Status.Cooldown; len(cd) != 1 || cd[0].InitiationID != "flap" {
		t.Fatalf("the window must be recorded on the source: %+v", cd)
	}
}

func TestConsumerChatIsNotSubjectToCooldown(t *testing.T) {
	src := fxSource("chat")
	base := time.Now()
	e := newSigEnv(t, src, fxChannel("surface"), fxPipeline("one", "chat"),
		chatSignal("sig-1", src, "same", "surface", "hi", base), chatSignal("sig-2", src, "same", "surface", "hi again", base.Add(time.Second)))
	e.reconcile("sig-1")
	e.reconcile("sig-2")
	if got := e.signal("sig-2").Status.Phase; got != agentopsv1alpha1.SignalRouted {
		t.Fatalf("a person asking twice means it twice, got %s", got)
	}
	convs := e.conversations()
	if len(convs) != 1 || len(convs[0].Spec.Inputs) != 2 {
		t.Fatalf("the repeat is appended to the open conversation: %d convs", len(convs))
	}
}

// ---- addressed chat -----------------------------------------------------------

func TestConsumerAddressedChatTakesTheNamedTargetAsItsOneClaimant(t *testing.T) {
	src := fxSource("chat")
	ch := fxChannel("surface")
	// two Pipelines serve the source; the message names a third that does not.
	e := newSigEnv(t, src, ch, fxPipeline("one", "chat"), fxPipeline("two", "chat"), fxPipeline("k8s-ops"),
		chatSignal("sig-1", src, "c-1", "surface", "/k8s-ops check nodes", time.Now()))
	e.reconcile("sig-1")

	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || len(got.Status.Claimants) != 1 || got.Status.Claimants[0].Ref.Name != "k8s-ops" {
		t.Fatalf("the named target is the one claimant: %+v", got.Status)
	}
	convs := e.conversations()
	if len(convs) != 1 || convs[0].Spec.ConversationID != "c-1" {
		t.Fatalf("an addressed conversation adopts the initiationId: %+v", convs)
	}
	if ins := e.inputs(); len(ins) != 1 || ins[0].Spec.Payload != "check nodes" {
		t.Fatalf("the task, not the address, is what the agent receives: %+v", ins)
	}
	bound := false
	for _, r := range convs[0].Spec.ChannelRefs {
		bound = bound || r.Name == "surface"
	}
	if !bound {
		t.Fatalf("an addressed conversation binds the addressing channel: %+v", convs[0].Spec.ChannelRefs)
	}
	if convs[0].Spec.OriginReader == nil || convs[0].Spec.OriginReader.Channel != "surface" {
		t.Fatalf("the person who typed it is owed the read mark: %+v", convs[0].Spec.OriginReader)
	}
	if op := e.ops.Claim(ch.Spec.Adapter); op != nil {
		t.Fatalf("a routed message tells the surface nothing: %+v", op)
	}
}

// ---- idempotency --------------------------------------------------------------

func TestConsumerRedoAfterALostStatusWriteCreatesNothingTwice(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"), fxSignal("sig-1", src, "x", time.Now()))
	e.reconcile("sig-1")
	convs, ins := len(e.conversations()), len(e.inputs())

	// The leader died after creating the conversation and the input but before
	// the status write landed: the Signal reads as never processed, while the
	// snapshot-then-route ordering means a real crash leaves the snapshot. Wipe
	// to the snapshot to model that, then to nothing for the harsher case.
	for _, keepSnapshot := range []bool{true, false} {
		sig := e.signal("sig-1")
		sig.Status = agentopsv1alpha1.SignalStatus{}
		if keepSnapshot {
			sig.Status.Claimants = []agentopsv1alpha1.SignalClaimant{claimant("ops", agentopsv1alpha1.ClaimantRouting)}
		}
		if err := e.c.Status().Update(context.Background(), sig); err != nil {
			t.Fatal(err)
		}
		if !keepSnapshot {
			// a new leader has no memory of the old one's cooldown spend
			e.r.Cooldowns = ingest.NewCooldownBook()
			if err := e.c.Get(context.Background(), types.NamespacedName{Namespace: sigNS, Name: "alerts"}, src); err != nil {
				t.Fatal(err)
			}
			src.Status.Cooldown = nil
			if err := e.c.Status().Update(context.Background(), src); err != nil {
				t.Fatal(err)
			}
		}
		e.reconcile("sig-1")
		got := e.signal("sig-1")
		if got.Status.Phase != agentopsv1alpha1.SignalRouted {
			t.Fatalf("keepSnapshot=%v: the repeated pass records Routed, got %+v", keepSnapshot, got.Status)
		}
		if len(e.conversations()) != convs || len(e.inputs()) != ins {
			t.Fatalf("keepSnapshot=%v: a repeated pass must create nothing twice (convs %d->%d, inputs %d->%d)",
				keepSnapshot, convs, len(e.conversations()), ins, len(e.inputs()))
		}
		if conv := e.conversations()[0]; len(conv.Spec.Inputs) != 1 {
			t.Fatalf("keepSnapshot=%v: the input is queued once, got %d", keepSnapshot, len(conv.Spec.Inputs))
		}
	}
}

func TestConsumerRedoDoesNotAppendAnInputAlreadyConsumed(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"), fxSignal("sig-1", src, "x", time.Now()))
	e.reconcile("sig-1")

	// dispatch consumed the input: queue pruned, run history cites the payload
	// ref, and the ConversationInput object went with the queue entry.
	conv := e.conversations()[0]
	name := conv.Spec.Inputs[0].PayloadRef.Name
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID:  "run-1",
		Inputs: []agentopsv1alpha1.RecordedInput{{ID: conv.Spec.Inputs[0].ID, PayloadRef: &agentopsv1alpha1.ObjectRef{Name: name}}},
	}}
	if err := e.c.Status().Update(context.Background(), &conv); err != nil {
		t.Fatal(err)
	}
	conv.Spec.Inputs = nil
	if err := e.c.Update(context.Background(), &conv); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Delete(context.Background(), &agentopsv1alpha1.ConversationInput{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: sigNS}}); err != nil {
		t.Fatal(err)
	}

	sig := e.signal("sig-1")
	sig.Status = agentopsv1alpha1.SignalStatus{Claimants: []agentopsv1alpha1.SignalClaimant{claimant("ops", agentopsv1alpha1.ClaimantRouting)}}
	if err := e.c.Status().Update(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	e.reconcile("sig-1")

	if conv := e.conversations()[0]; len(conv.Spec.Inputs) != 0 {
		t.Fatalf("an input the run history already cites is delivered: %+v", conv.Spec.Inputs)
	}
	if len(e.inputs()) != 0 {
		t.Fatal("no ConversationInput is recreated for a delivered message")
	}
	if e.signal("sig-1").Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatal("the repeated pass still records Routed")
	}
}

func TestConsumerRepeatOfAnOpenConversationIsAppendedAsARecurrence(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now()
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"),
		fxSignal("sig-1", src, "x", base), fxSignal("sig-2", src, "x", base.Add(time.Second)))
	e.r.Cooldowns = nil // no cooldown book: "outside the window"
	e.reconcile("sig-1")

	conv := e.conversations()[0]
	conv.Status.SessionID = "sess-1"
	if err := e.c.Status().Update(context.Background(), &conv); err != nil {
		t.Fatal(err)
	}
	e.reconcile("sig-2")

	convs := e.conversations()
	if len(convs) != 1 || len(convs[0].Spec.Inputs) != 2 {
		t.Fatalf("one conversation, two inputs: %d", len(convs))
	}
	if convs[0].Spec.Inputs[1].Type != agentopsv1alpha1.InputRecurrence {
		t.Fatalf("the repeat resumes with context: %+v", convs[0].Spec.Inputs[1])
	}
	if got := e.signal("sig-2").Status.Claimants[0].ConversationRef.Name; got != convs[0].Name {
		t.Fatalf("the second Signal records the same conversation, got %q", got)
	}
}

func TestConsumerSnapshotIsNeverRewired(t *testing.T) {
	src := fxSource("alerts")
	sig := fxSignal("sig-1", src, "x", time.Now())
	sig.Status.Claimants = []agentopsv1alpha1.SignalClaimant{claimant("old", agentopsv1alpha1.ClaimantRouting)}
	// "new" now claims the source; "old" still exists but no longer does.
	e := newSigEnv(t, src, fxPipeline("old"), fxPipeline("new", "alerts"), sig)
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if len(got.Status.Claimants) != 1 || got.Status.Claimants[0].Ref.Name != "old" ||
		got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouted {
		t.Fatalf("a re-wire must not reach an in-flight signal: %+v", got.Status.Claimants)
	}
	for _, c := range e.conversations() {
		if c.Spec.PipelineRef.Name == "new" {
			t.Fatal("the new claimant must not receive the in-flight signal")
		}
	}
}

func TestConsumerRoutesOnlyNonTerminalClaimants(t *testing.T) {
	src := fxSource("alerts")
	sig := fxSignal("sig-1", src, "x", time.Now())
	done := claimant("done", agentopsv1alpha1.ClaimantRouted)
	done.ConversationRef = &agentopsv1alpha1.ObjectRef{Name: "conv-done"}
	sig.Status.Phase = agentopsv1alpha1.SignalPartiallyRouted
	sig.Status.Claimants = []agentopsv1alpha1.SignalClaimant{done, claimant("todo", agentopsv1alpha1.ClaimantRouting)}
	e := newSigEnv(t, src, fxPipeline("done", "alerts"), fxPipeline("todo", "alerts"), sig)
	e.reconcile("sig-1")

	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || got.Status.Claimants[0].ConversationRef.Name != "conv-done" {
		t.Fatalf("the Routed entry is untouched and the phase rolls up: %+v", got.Status)
	}
	convs := e.conversations()
	if len(convs) != 1 || convs[0].Spec.PipelineRef.Name != "todo" {
		t.Fatalf("only the non-terminal claimant is attempted: %+v", convs)
	}
}

func TestConsumerDeletedClaimantIsSkippedNeverFailed(t *testing.T) {
	src := fxSource("alerts")
	sig := fxSignal("sig-1", src, "x", time.Now())
	sig.Status.Claimants = []agentopsv1alpha1.SignalClaimant{claimant("gone", agentopsv1alpha1.ClaimantRouting)}
	e := newSigEnv(t, src, sig)
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantSkipped || got.Status.Claimants[0].Reason == "" {
		t.Fatalf("a deleted claimant is Skipped with a reason: %+v", got.Status.Claimants[0])
	}
	if got.Status.Phase != agentopsv1alpha1.SignalFailed {
		t.Fatalf("every claimant terminal and none Routed rolls up to Failed, got %s", got.Status.Phase)
	}
	if e.events(activity.KindSignalFailed, "") != 1 {
		t.Fatal("Failed is surfaced as loudly as the unwired drop")
	}
}

// ---- backlog full -------------------------------------------------------------

func TestConsumerMachineSignalWaitsForABacklogThatDrains(t *testing.T) {
	src := fxSource("alerts")
	objs := []client.Object{src, fxPipeline("ops", "alerts"), fxSignal("sig-1", src, "x", time.Now())}
	for i := 0; i < 50; i++ {
		objs = append(objs, pendingConversation(fmt.Sprintf("pending-%d", i)))
	}
	e := newSigEnv(t, objs...)

	res := e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Phase == agentopsv1alpha1.SignalDropped || terminalSignalPhase(got.Status.Phase) {
		t.Fatalf("a machine signal at a full backlog must not end, got %s", got.Status.Phase)
	}
	if len(got.Status.Claimants) != 1 || got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouting {
		t.Fatalf("its claimant stays Routing: %+v", got.Status.Claimants)
	}
	if res.RequeueAfter != DefaultSignalRetryInterval {
		t.Fatalf("it is requeued with a delay, got %+v", res)
	}
	if got.Status.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", got.Status.Attempts)
	}
	first := got.Status.FirstAttemptAt.Time

	// the backlog drains
	for i := 0; i < 50; i++ {
		if err := e.c.Delete(context.Background(), pendingConversation(fmt.Sprintf("pending-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	e.now = e.now.Add(time.Minute)
	e.reconcile("sig-1")
	got = e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || got.Status.Attempts != 2 {
		t.Fatalf("a later pass routes it as attempt 2, got %+v", got.Status)
	}
	if !got.Status.FirstAttemptAt.Time.Equal(first) {
		t.Fatal("firstAttemptAt keeps the first pass's time")
	}
}

// ---- ordering and exclusion ---------------------------------------------------

func TestConsumerMessagesOfOneConversationGoInArrivalOrder(t *testing.T) {
	src := fxSource("chat")
	base := time.Now()
	e := newSigEnv(t, src, fxChannel("surface"), fxPipeline("one", "chat"),
		chatSignal("sig-b", src, "conv", "surface", "second", base.Add(2*time.Second)),
		chatSignal("sig-a", src, "conv", "surface", "first", base))
	e.r.OrderRetry = time.Millisecond

	// the later message is reconciled first: it must wait its turn
	res := e.reconcile("sig-b")
	if res.RequeueAfter == 0 || len(e.conversations()) != 0 {
		t.Fatalf("an earlier in-flight message goes first: %+v, %d conversations", res, len(e.conversations()))
	}
	if e.signal("sig-b").Status.Attempts != 0 {
		t.Fatal("a message waiting its turn has not started an attempt")
	}
	e.reconcile("sig-a")
	e.reconcile("sig-b")

	convs := e.conversations()
	if len(convs) != 1 || len(convs[0].Spec.Inputs) != 2 {
		t.Fatalf("exactly one conversation with both messages: %d", len(convs))
	}
	ins := map[string]string{}
	for _, in := range e.inputs() {
		ins[in.Name] = in.Spec.Payload
	}
	first := ins[convs[0].Spec.Inputs[0].PayloadRef.Name]
	second := ins[convs[0].Spec.Inputs[1].PayloadRef.Name]
	if first != "first" || second != "second" {
		t.Fatalf("appended out of order: %q then %q", first, second)
	}
}

func TestConsumerOrderTiesBreakByName(t *testing.T) {
	src := fxSource("alerts")
	same := time.Now().Truncate(time.Second)
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"),
		fxSignal("sig-b", src, "k", same), fxSignal("sig-a", src, "k", same))
	if res := e.reconcile("sig-b"); res.RequeueAfter == 0 {
		t.Fatal("same second: the lower name goes first")
	}
	e.reconcile("sig-a")
	if e.signal("sig-a").Status.Attempts != 1 {
		t.Fatal("sig-a is the head")
	}
}

func TestConsumerTerminalPredecessorDoesNotBlock(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now()
	first := fxSignal("sig-a", src, "k", base)
	first.Status.Phase = agentopsv1alpha1.SignalDropped
	first.Status.Reason = agentopsv1alpha1.SignalDropCooldown
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"), first, fxSignal("sig-b", src, "k", base.Add(time.Second)))
	e.reconcile("sig-b")
	if e.signal("sig-b").Status.Attempts != 1 {
		t.Fatal("a finished predecessor is no reason to wait")
	}
}

func TestConsumerDifferentConversationsDoNotBlockEachOther(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now()
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"),
		fxSignal("sig-a", src, "k1", base), fxSignal("sig-b", src, "k2", base.Add(time.Second)))
	// sig-a's key is held by a consumer mid-pass; sig-b proceeds.
	if !e.r.acquire("alerts\x00k1") {
		t.Fatal("acquire")
	}
	e.reconcile("sig-b")
	if e.signal("sig-b").Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatal("another conversation runs in parallel")
	}
	if res := e.reconcile("sig-a"); res.RequeueAfter == 0 || e.signal("sig-a").Status.Attempts != 0 {
		t.Fatal("a held key is not entered twice")
	}
	e.r.release("alerts\x00k1")
	e.reconcile("sig-a")
	if e.signal("sig-a").Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatal("released: processed")
	}
}

func TestConsumerCooldownIsConsistentUnderConcurrentConsumers(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now()
	objs := []client.Object{src, fxPipeline("ops", "alerts")}
	for i := 0; i < 8; i++ {
		objs = append(objs, fxSignal(fmt.Sprintf("sig-%d", i), src, fmt.Sprintf("k%d", i), base.Add(time.Duration(i)*time.Second)))
	}
	objs = append(objs, fxSignal("sig-dup", src, "k0", base.Add(time.Hour)))
	e := newSigEnv(t, objs...)

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			_, _ = e.r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: sigNS, Name: fmt.Sprintf("sig-%d", i)}})
		}(i)
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	e.reconcile("sig-dup")
	if got := e.signal("sig-dup"); got.Status.Reason != agentopsv1alpha1.SignalDropCooldown {
		t.Fatalf("a repeated initiationId is suppressed exactly once, got %+v", got.Status)
	}
	if n := len(e.source("alerts").Status.Cooldown); n != 8 {
		t.Fatalf("every concurrent admission is recorded, neither overwriting the other: %d of 8", n)
	}
}

// ---- housekeeping -------------------------------------------------------------

func TestHousekeepingRetryDeadlineFailsAnExhaustedClaimantAndRollsUpRouted(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	past := metav1.NewTime(e.now.Add(-2 * time.Hour))
	sig := fxSignal("sig-1", src, "x", past.Time)
	sig.Status.Phase = agentopsv1alpha1.SignalPartiallyRouted
	sig.Status.FirstAttemptAt = &past
	sig.Status.Claimants = []agentopsv1alpha1.SignalClaimant{
		claimant("routed", agentopsv1alpha1.ClaimantRouted), claimant("stuck", agentopsv1alpha1.ClaimantRouting)}
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	e.reconcile("sig-1")

	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || got.Status.CompletedAt == nil || got.Status.ConsumedBy == "" {
		t.Fatalf("the exhausted claimant rolls up to Routed and the write records the leader: %+v", got.Status)
	}
	if c := got.Status.Claimants[1]; c.State != agentopsv1alpha1.ClaimantFailed || !strings.Contains(c.Reason, "retry deadline") {
		t.Fatalf("exhausted claimant must be Failed with the exhaustion recorded: %+v", c)
	}
	if got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouted {
		t.Fatal("the routed claimant is untouched")
	}
	if e.events(activity.KindSignalFailed, "") != 0 {
		t.Fatal("a Signal that rolled up to Routed is not a Failed one")
	}
}

func TestHousekeepingPoisonSignalEndsFailedLoudly(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	past := metav1.NewTime(e.now.Add(-2 * time.Hour))
	sig := fxSignal("sig-1", src, "x", past.Time)
	sig.Status.FirstAttemptAt = &past
	sig.Status.Claimants = []agentopsv1alpha1.SignalClaimant{claimant("stuck", agentopsv1alpha1.ClaimantRouting)}
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if e.events(activity.KindSignalFailed, activity.CodeRetryExhausted) != 1 {
		t.Fatal("Failed is surfaced as loudly as the unwired drop")
	}
}

func TestHousekeepingPreFanOutDeadlineDropsExhausted(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	past := metav1.NewTime(e.now.Add(-2 * time.Hour))
	sig := fxSignal("sig-1", src, "x", past.Time)
	sig.Status.FirstAttemptAt = &past
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	e.reconcile("sig-1")
	got := e.signal("sig-1")
	if got.Status.Phase != agentopsv1alpha1.SignalDropped || got.Status.Reason != agentopsv1alpha1.SignalDropExhausted {
		t.Fatalf("want Dropped/exhausted, got %+v", got.Status)
	}
}

func TestHousekeepingASignalNeverPassedIsNotExpiredByItsAge(t *testing.T) {
	src := fxSource("alerts")
	old := time.Now().Add(-48 * time.Hour)
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"), fxSignal("sig-1", src, "x", old))
	e.reconcile("sig-1")
	if got := e.signal("sig-1").Status.Phase; got != agentopsv1alpha1.SignalRouted {
		t.Fatalf("the deadline runs from the first attempt, not the object's age: %s", got)
	}
}

func TestHousekeepingPerSourceCapEvictsTheOldestFirst(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now().Add(-time.Hour)
	objs := []client.Object{src}
	for i := 0; i < 4; i++ {
		objs = append(objs, fxSignal(fmt.Sprintf("sig-%d", i), src, fmt.Sprintf("k%d", i), base.Add(time.Duration(i)*time.Minute)))
	}
	// after fan-out, to prove Dropped stays pre-fan-out
	fanned := fxSignal("sig-fan", src, "kf", base.Add(-time.Minute))
	fanned.Status.Phase = agentopsv1alpha1.SignalPartiallyRouted
	fanned.Status.Claimants = []agentopsv1alpha1.SignalClaimant{
		claimant("routed", agentopsv1alpha1.ClaimantRouted), claimant("stuck", agentopsv1alpha1.ClaimantRouting)}
	objs = append(objs, fanned)
	e := newSigEnv(t, objs...)
	e.r.MaxPerSource = 4 // 5 live: one over

	e.reconcile("sig-fan")
	got := e.signal("sig-fan")
	if got.Status.Phase != agentopsv1alpha1.SignalRouted || got.Status.Claimants[1].State != agentopsv1alpha1.ClaimantFailed ||
		!strings.Contains(got.Status.Claimants[1].Reason, "overflow") {
		t.Fatalf("overflow after fan-out fails the routing claimant, never re-drops: %+v", got.Status)
	}
	if got.Status.Reason != "" || got.Status.CompletedAt == nil {
		t.Fatalf("no drop reason after fan-out, completedAt set: %+v", got.Status)
	}
	// a newer signal's pass leaves the (now terminal) oldest alone and evicts nobody else
	e.reconcile("sig-3")
	if got := e.signal("sig-3").Status.Phase; got != agentopsv1alpha1.SignalDropped && got != "" && got != agentopsv1alpha1.SignalRouted {
		t.Fatalf("unexpected phase %s", got)
	}
}

func TestHousekeepingOverflowDropsAPreFanOutSignal(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now().Add(-time.Hour)
	objs := []client.Object{src}
	for i := 0; i < 3; i++ {
		objs = append(objs, fxSignal(fmt.Sprintf("sig-%d", i), src, fmt.Sprintf("k%d", i), base.Add(time.Duration(i)*time.Minute)))
	}
	e := newSigEnv(t, objs...)
	e.r.MaxPerSource = 2

	e.reconcile("sig-0") // the oldest, in its own pass
	got := e.signal("sig-0")
	if got.Status.Phase != agentopsv1alpha1.SignalDropped || got.Status.Reason != agentopsv1alpha1.SignalDropOverflow {
		t.Fatalf("want Dropped/overflow, got %+v", got.Status)
	}
	if e.events(activity.KindSignalDropped, activity.CodeOverflow) != 1 {
		t.Fatal("an eviction is counted and logged")
	}
	if len(e.conversations()) != 0 {
		t.Fatal("an evicted signal routes nothing")
	}
	// the survivors are within the cap and are not evicted
	for _, n := range []string{"sig-1", "sig-2"} {
		if got := e.signal(n); got.Status.Reason == agentopsv1alpha1.SignalDropOverflow {
			t.Fatalf("%s must survive", n)
		}
	}
}

func TestHousekeepingCapIgnoresOtherSourcesAndTerminalSignals(t *testing.T) {
	a, b := fxSource("a"), fxSource("b")
	base := time.Now().Add(-time.Hour)
	done := fxSignal("done", a, "d", base)
	done.Status.Phase = agentopsv1alpha1.SignalRouted
	e := newSigEnv(t, a, b, fxPipeline("ops", "a", "b"), done,
		fxSignal("a1", a, "k1", base.Add(time.Minute)), fxSignal("b1", b, "k2", base.Add(time.Minute)),
		fxSignal("b2", b, "k3", base.Add(2*time.Minute)))
	e.r.MaxPerSource = 1
	e.reconcile("a1")
	if got := e.signal("a1").Status; got.Reason == agentopsv1alpha1.SignalDropOverflow {
		t.Fatalf("one live signal on source a is within the cap of 1: %+v", got)
	}
}

func TestHousekeepingTTLsAreMeasuredFromCompletionPerPhase(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	created := e.now.Add(-50 * time.Minute)
	mk := func(name string, phase agentopsv1alpha1.SignalPhase, completedAgo time.Duration) {
		sig := fxSignal(name, src, name, created)
		sig.Status.Phase = phase
		done := metav1.NewTime(e.now.Add(-completedAgo))
		sig.Status.CompletedAt = &done
		if err := e.c.Create(context.Background(), sig); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	// all went terminal 5h ago
	mk("dropped", agentopsv1alpha1.SignalDropped, 5*time.Hour)
	mk("routed", agentopsv1alpha1.SignalRouted, 5*time.Hour)
	mk("failed", agentopsv1alpha1.SignalFailed, 5*time.Hour)

	for _, n := range []string{"dropped", "routed", "failed"} {
		e.reconcile(n)
	}
	if e.exists("dropped") {
		t.Fatal("Dropped (4h) is deleted after 5h")
	}
	if !e.exists("routed") || !e.exists("failed") {
		t.Fatal("Routed (8h) and Failed (72h) are kept after 5h")
	}

	e.now = e.now.Add(4 * time.Hour) // routed is now 9h past completion
	e.reconcile("routed")
	e.reconcile("failed")
	if e.exists("routed") {
		t.Fatal("Routed is deleted after 8h")
	}
	if !e.exists("failed") {
		t.Fatal("Failed outlives Routed")
	}

	e.now = e.now.Add(70 * time.Hour)
	e.reconcile("failed")
	if e.exists("failed") {
		t.Fatal("Failed is deleted after 72h")
	}
}

func TestHousekeepingTTLRunsFromCompletionNotCreation(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	// created 9h ago (past the 8h TTL), completed only now
	sig := fxSignal("late", src, "x", e.now.Add(-9*time.Hour))
	sig.Status.Phase = agentopsv1alpha1.SignalRouted
	done := metav1.NewTime(e.now)
	sig.Status.CompletedAt = &done
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	res := e.reconcile("late")
	if !e.exists("late") || res.RequeueAfter < 7*time.Hour {
		t.Fatalf("a Signal that just completed lives its full TTL: exists=%v requeue=%v", e.exists("late"), res.RequeueAfter)
	}
}

func TestHousekeepingTTLOverrides(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	e.r.TTLRouted = 6 * time.Hour
	sig := fxSignal("s", src, "x", e.now.Add(-time.Hour))
	sig.Status.Phase = agentopsv1alpha1.SignalRouted
	done := metav1.NewTime(e.now.Add(-7 * time.Hour))
	sig.Status.CompletedAt = &done
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	e.reconcile("s")
	if e.exists("s") {
		t.Fatal("signalTtlRouted: 6h deletes a Signal 7h past completion")
	}
}

func TestSignalDefaults(t *testing.T) {
	r := &SignalReconciler{}
	if r.retryDeadline() != time.Hour || r.maxPerSource() != 200 || r.concurrency() != 4 ||
		r.ttlFor(agentopsv1alpha1.SignalRouted) != 8*time.Hour ||
		r.ttlFor(agentopsv1alpha1.SignalDropped) != 4*time.Hour ||
		r.ttlFor(agentopsv1alpha1.SignalFailed) != 72*time.Hour {
		t.Fatal("the six bounds default to 1h / 200 / 4 / 8h / 4h / 72h")
	}
}

func TestReconcileOfAMissingSignalOrSourceIsANoOp(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t, src)
	e.reconcile("ghost")
	orphan := fxSignal("orphan", fxSource("gone"), "x", time.Now())
	if err := e.c.Create(context.Background(), orphan); err != nil {
		t.Fatal(err)
	}
	e.reconcile("orphan")
	if e.signal("orphan").Status.Attempts != 0 {
		t.Fatal("nothing routes without its source")
	}
}

func TestHousekeepingAPassOverTheCapKicksTheOldestsOwnPass(t *testing.T) {
	src := fxSource("alerts")
	base := time.Now().Add(-time.Hour)
	e := newSigEnv(t, src, fxPipeline("ops", "alerts"),
		fxSignal("sig-0", src, "k0", base), fxSignal("sig-1", src, "k1", base.Add(time.Minute)),
		fxSignal("sig-2", src, "k2", base.Add(2*time.Minute)))
	e.r.MaxPerSource = 2
	e.r.kick = make(chan event.GenericEvent, 4)

	// sig-2 is the newest: its pass does not evict itself, it asks for the
	// oldest's pass, so the eviction goes through the work queue.
	e.reconcile("sig-2")
	select {
	case ev := <-e.r.kick:
		if ev.Object.GetName() != "sig-0" {
			t.Fatalf("kicked %s, want the oldest", ev.Object.GetName())
		}
	default:
		t.Fatal("the oldest's pass was not enqueued")
	}
	if e.signal("sig-0").Status.Phase != "" {
		t.Fatal("the pass over the cap must not write another Signal's status")
	}
}

func TestRollupPhaseWalksEveryClaimantStateCombination(t *testing.T) {
	st := []agentopsv1alpha1.ClaimantState{
		agentopsv1alpha1.ClaimantRouting, agentopsv1alpha1.ClaimantRouted,
		agentopsv1alpha1.ClaimantFailed, agentopsv1alpha1.ClaimantSkipped,
	}
	for _, a := range st {
		for _, b := range st {
			claimants := []agentopsv1alpha1.SignalClaimant{claimant("a", a), claimant("b", b)}
			var routing, routed bool
			for _, s := range []agentopsv1alpha1.ClaimantState{a, b} {
				routing = routing || s == agentopsv1alpha1.ClaimantRouting
				routed = routed || s == agentopsv1alpha1.ClaimantRouted
			}
			want := agentopsv1alpha1.SignalFailed
			switch {
			case routing && routed:
				want = agentopsv1alpha1.SignalPartiallyRouted
			case routing:
				want = agentopsv1alpha1.SignalPending
			case routed:
				want = agentopsv1alpha1.SignalRouted
			}
			if got := agentopsv1alpha1.RollupPhase(claimants); got != want {
				t.Errorf("(%s,%s): phase %s, want %s", a, b, got, want)
			}
		}
	}
}

func TestHousekeepingBackfillsCompletedAtFromTheObjectNotNow(t *testing.T) {
	src := fxSource("alerts")
	e := newSigEnv(t)
	// A terminal Signal written by an older build: no completedAt, finished
	// 9h ago. Its TTL clock must start there, not at this reconcile.
	sig := fxSignal("legacy", src, "x", e.now.Add(-9*time.Hour))
	sig.Status.Phase = agentopsv1alpha1.SignalRouted
	first := metav1.NewTime(e.now.Add(-9 * time.Hour))
	sig.Status.FirstAttemptAt = &first
	if err := e.c.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	e.reconcile("legacy")
	e.reconcile("legacy")
	if e.exists("legacy") {
		t.Fatal("a legacy Signal already past its TTL must be collected, not given a fresh clock")
	}
}
