// Coordinator claiming and addressing (design D-B, D-E, superseded on channel
// binding by coordinator-unconditional-channels; tasks 2.3/2.3b/2.5 of
// coordinated-agents): a Coordinator claims a signal source exactly as a
// Pipeline does, fans out its own root conversation with its OWN declared
// channels bound at creation — exactly like a Pipeline's — snapshots its
// limits into status.budget and its own channelRefs as EscalationChannelRefs
// too, and is addressable by name exactly as a Pipeline is, with an addressed
// root ALSO folding in the surface it was addressed from when that is not
// already one of its declared channels. Invoking members, enforcing the
// budget and escalating are later tasks (2.6 onward) and are not exercised
// here.
package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// mkCoordinator creates a Coordinator claiming sources with an inline
// capability, mirroring mkPipeline — the coordinated-agents sibling that
// claims sources exactly as a Pipeline does (design D-B). Its channels bind
// to a conversation it opens at creation, unconditionally
// (coordinator-unconditional-channels), and are ALSO snapshotted as that
// root's own EscalationChannelRefs.
func mkCoordinator(t *testing.T, name string, sources, channels []string, profile string, limits *agentopsv1alpha1.CoordinatorLimits) {
	t.Helper()
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = name, ns
	for _, s := range sources {
		co.Spec.SignalSourceRefs = append(co.Spec.SignalSourceRefs, agentopsv1alpha1.ObjectRef{Name: s})
	}
	for _, c := range channels {
		co.Spec.ChannelRefs = append(co.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile}
	co.Spec.Limits = limits
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
}

// convsFromCoordinator lists conversations whose recorded origin is this
// Coordinator — the coordinatorRef sibling of convsFromPipeline.
func convsFromCoordinator(t *testing.T, coordinator string) []agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	var out []agentopsv1alpha1.Conversation
	for i := range list.Items {
		if ref := list.Items[i].Spec.CoordinatorRef; ref != nil && ref.Name == coordinator {
			out = append(out, list.Items[i])
		}
	}
	return out
}

// A Coordinator and a Pipeline sharing one source both count on Wired, and
// each opens its own conversation per signal (coordinator-model: "A
// Coordinator and a Pipeline share a source").
func TestSharedSourceWiredNamesPipelineAndCoordinator(t *testing.T) {
	mkProfile(t, "prof-mix-pipe")
	mkProfile(t, "prof-mix-co")
	mkSignalSource(t, "src-mix", "am-mix", "")
	mkPipeline(t, "mix-pipe", []string{"src-mix"}, nil, "prof-mix-pipe")
	mkCoordinator(t, "mix-co", []string{"src-mix"}, nil, "prof-mix-co", nil)
	reconcilePipeline(t, "mix-pipe")
	reconcileCoordinator(t, "mix-co")

	src := reconcileSignalSource(t, "src-mix")
	wired := apimeta.FindStatusCondition(src.Status.Conditions, controller.ConditionWired)
	if wired == nil || wired.Status != "True" ||
		!strings.Contains(wired.Message, "mix-pipe") || !strings.Contains(wired.Message, "mix-co") {
		t.Fatalf("Wired must name both the Pipeline and the Coordinator claimant: %+v", wired)
	}

	h := apiServer().Handler()
	rec := postSignal(t, h, testMasterToken, "src-mix", []map[string]any{
		{"fingerprint": "mix-1", "labels": map[string]string{"alertname": "SharedMix"}, "payload": "boom"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	pipeConvs, coConvs := convsFromPipeline(t, "mix-pipe"), convsFromCoordinator(t, "mix-co")
	if len(pipeConvs) != 1 || len(coConvs) != 1 {
		t.Fatalf("each claimant needs its own conversation: pipeline=%d coordinator=%d", len(pipeConvs), len(coConvs))
	}
}

// A Coordinator-claimed alert opens a root with its OWN declared channels
// ALREADY bound (coordinator-unconditional-channels) — exactly as a
// Pipeline's own channelRefs bind — and its own limits snapshotted into
// status.budget (design D-E), its channelRefs ALSO snapshotted as
// EscalationChannelRefs.
func TestCoordinatorClaimedSignalOpensRootWithBoundChannelsAndBudget(t *testing.T) {
	mkProfile(t, "prof-co-root")
	mkChannel(t, "co-root-escalate", "co-root-ta")
	mkSignalSource(t, "src-co-root", "am-co-root", "")
	deadline := &metav1.Duration{Duration: time.Hour}
	mkCoordinator(t, "co-root", []string{"src-co-root"}, []string{"co-root-escalate"}, "prof-co-root",
		&agentopsv1alpha1.CoordinatorLimits{MaxAgents: 3, MaxTurns: 5, Deadline: deadline})
	reconcileCoordinator(t, "co-root")

	h := apiServer().Handler()
	rec := postSignal(t, h, testMasterToken, "src-co-root", []map[string]any{
		{"fingerprint": "co-root-1", "labels": map[string]string{"alertname": "CoRootAlert"}, "payload": "boom"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsFromCoordinator(t, "co-root")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	conv := convs[0]
	if len(conv.Spec.ChannelRefs) != 1 || conv.Spec.ChannelRefs[0].Name != "co-root-escalate" {
		t.Fatalf("a Coordinator-rooted conversation must bind its own declared channels at creation: %+v", conv.Spec.ChannelRefs)
	}
	if conv.Spec.PipelineRef != nil {
		t.Fatalf("a Coordinator-rooted conversation must carry no pipelineRef: %+v", conv.Spec.PipelineRef)
	}
	if len(conv.Spec.EscalationChannelRefs) != 1 || conv.Spec.EscalationChannelRefs[0].Name != "co-root-escalate" {
		t.Fatalf("escalationChannelRefs must snapshot the Coordinator's own channelRefs: %+v", conv.Spec.EscalationChannelRefs)
	}
	if conv.Status.Budget == nil {
		t.Fatal("a Coordinator-rooted conversation must carry a status.budget snapshot")
	}
	if conv.Status.Budget.MaxAgents != 3 || conv.Status.Budget.MaxTurns != 5 {
		t.Fatalf("budget must snapshot the Coordinator's limits: %+v", conv.Status.Budget)
	}
	if conv.Status.Budget.Deadline == nil {
		t.Fatal("a relative deadline must resolve to an absolute one at snapshot time")
	}
	if until := conv.Status.Budget.Deadline.Time.Sub(time.Now()); until <= 0 || until > time.Hour {
		t.Fatalf("deadline must resolve against creation time, got %v from now", until)
	}
}

// The bare-chat lane's ONE claimant may be a Coordinator: the message routes
// to it exactly as it would to a sole Pipeline. This Coordinator declares NO
// channels of its own, so its root still binds none — a bare-chat
// origination is NOT folded in the way an ADDRESSED one is (boundChannels'
// origin-surface guarantee applies only to the addressed path).
func TestBareChatRoutesToSoleCoordinatorClaimant(t *testing.T) {
	mkProfile(t, "prof-co-chat")
	mkChannel(t, "co-chat-chan", "co-chat-ta")
	mkChatSource(t, "src-co-chat", "co-chat-chan")
	mkCoordinator(t, "co-chat", []string{"src-co-chat"}, nil, "prof-co-chat", nil)
	reconcileCoordinator(t, "co-chat")
	srv := apiServer()

	rec := chatSignal(t, srv, "src-co-chat", "co-chat-chan", "investigate the outage")
	if rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsFromCoordinator(t, "co-chat")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation routed to the sole Coordinator claimant, got %d", len(convs))
	}
	if len(convs[0].Spec.ChannelRefs) != 0 {
		t.Fatalf("a Coordinator declaring no channels still binds none, even the origin one: %+v",
			convs[0].Spec.ChannelRefs)
	}
	if n := len(convsBoundTo(t, "co-chat-chan")); n != 0 {
		t.Fatalf("with no channel bound, the origin channel shows no thread: %d bound", n)
	}
}

// A bare message on a surface a Pipeline AND a Coordinator both serve is
// refused with the choices, naming both by their addressed form.
func TestAmbiguousBareChatNamesPipelineAndCoordinator(t *testing.T) {
	mkProfile(t, "prof-amb2-pipe")
	mkProfile(t, "prof-amb2-co")
	mkChannel(t, "chan-amb2", "telegram")
	mkChatSource(t, "src-amb2", "chan-amb2")
	mkPipeline(t, "amb2-pipe", []string{"src-amb2"}, []string{"chan-amb2"}, "prof-amb2-pipe")
	mkCoordinator(t, "amb2-co", []string{"src-amb2"}, nil, "prof-amb2-co", nil)
	reconcilePipeline(t, "amb2-pipe")
	reconcileCoordinator(t, "amb2-co")
	srv := apiServer()

	rec := chatSignal(t, srv, "src-amb2", "chan-amb2", "who is on call?")
	if rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(convsBoundTo(t, "chan-amb2")); n != 0 {
		t.Fatalf("an ambiguous message must create nothing, got %d conversation(s)", n)
	}
	if n := len(convsFromPipeline(t, "amb2-pipe")) + len(convsFromCoordinator(t, "amb2-co")); n != 0 {
		t.Fatalf("an ambiguous message must open no conversation on either claimant, got %d", n)
	}

	rec2 := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec2.Code != 200 {
		t.Fatalf("the refusal must reach the surface: %d", rec2.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec2.Body.Bytes(), &op)
	body := opBody(op)
	for _, want := range []string{"/amb2-pipe", "/amb2-co", "prof-amb2-pipe", "prof-amb2-co"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the refusal must carry %q, naming both claimants: %q", want, body)
		}
	}
	got := map[string]string{}
	for _, c := range op.Message.Choices {
		got[c.Label] = c.Command
	}
	if len(got) != 2 || got["amb2-pipe"] != "/amb2-pipe" || got["amb2-co"] != "/amb2-co" {
		t.Fatalf("refusal must offer BOTH claimants as choices, got %v", got)
	}
}

// Addressing a Coordinator by name binds its OWN declared channels PLUS the
// surface it was addressed from (coordinator-unconditional-channels), with
// its own channelRefs ALSO snapshotted as EscalationChannelRefs.
func TestAddressedCoordinatorBindsItsOwnChannelsPlusTheOriginSurface(t *testing.T) {
	mkProfile(t, "prof-co-addr")
	mkChannel(t, "co-addr-origin", "co-addr-ta")
	mkChannel(t, "co-addr-escalate", "co-addr-tb")
	mkChatSource(t, "src-co-addr", "co-addr-origin")
	mkCoordinator(t, "co-addr", []string{"src-co-addr"}, []string{"co-addr-escalate"}, "prof-co-addr",
		&agentopsv1alpha1.CoordinatorLimits{MaxAgents: 2})
	reconcileCoordinator(t, "co-addr")
	srv := apiServer()

	rec := chatSignal(t, srv, "src-co-addr", "co-addr-origin", "/co-addr investigate api latency")
	if rec.Code != 200 {
		t.Fatalf("addressed chat signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsFromCoordinator(t, "co-addr")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	conv := convs[0]
	gotChannels := map[string]bool{}
	for _, ref := range conv.Spec.ChannelRefs {
		gotChannels[ref.Name] = true
	}
	if len(gotChannels) != 2 || !gotChannels["co-addr-escalate"] || !gotChannels["co-addr-origin"] {
		t.Fatalf("an addressed Coordinator conversation binds its OWN channels plus the origin surface: %+v",
			conv.Spec.ChannelRefs)
	}
	if len(conv.Spec.EscalationChannelRefs) != 1 || conv.Spec.EscalationChannelRefs[0].Name != "co-addr-escalate" {
		t.Fatalf("the Coordinator's own channelRefs are still snapshotted for later escalation: %+v",
			conv.Spec.EscalationChannelRefs)
	}
	if conv.Status.Budget == nil || conv.Status.Budget.MaxAgents != 2 {
		t.Fatalf("an addressed root still snapshots the Coordinator's limits: %+v", conv.Status.Budget)
	}
	if len(conv.Spec.Inputs) != 1 || conv.Spec.Inputs[0].Payload != "investigate api latency" {
		t.Fatalf("the task text must be the first input: %+v", conv.Spec.Inputs)
	}
}
