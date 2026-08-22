// Shared signal sources: a source is listed by as many Ready pipelines as the
// adopter likes, and a signal there opens a conversation on EVERY one of them.
// The one lane that does not fan out is a bare chat message — a person asked
// one question and is owed one answer, so ambiguity is refused with the choices
// rather than answered twice.
package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// convsFromPipeline lists conversations whose recorded origin is this pipeline.
func convsFromPipeline(t *testing.T, pipeline string) []agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	var out []agentopsv1alpha1.Conversation
	for i := range list.Items {
		if ref := list.Items[i].Spec.PipelineRef; ref != nil && ref.Name == pipeline {
			out = append(out, list.Items[i])
		}
	}
	return out
}

// One alert on a source two pipelines watch opens TWO conversations, each with
// its own profile — and spends the source's cooldown exactly once, so the
// second pipeline is not starved by the first having "used up" the fingerprint.
func TestSharedAlertSourceFansOutToEveryPipeline(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-fan-a")
	mkProfile(t, "prof-fan-b")
	mkSignalSource(t, "src-fan", "am-fan", "")
	mkPipeline(t, "fan-a", []string{"src-fan"}, nil, "prof-fan-a")
	mkPipeline(t, "fan-b", []string{"src-fan"}, nil, "prof-fan-b")
	reconcilePipeline(t, "fan-a")
	reconcilePipeline(t, "fan-b")
	h := apiServer().Handler()

	rec := postSignal(t, h, testMasterToken, "src-fan", []map[string]any{
		{"fingerprint": "fan-1", "labels": map[string]string{"alertname": "DiskFull"}, "payload": "disk is full"},
	})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	// one SIGNAL landed, on two CONVERSATIONS — conflating the counters would
	// report a doubled intake on every shared source
	if out["queued"] != float64(1) || out["conversations"] != float64(2) {
		t.Fatalf("want queued 1 / conversations 2, got %v", out)
	}

	a, b := convsFromPipeline(t, "fan-a"), convsFromPipeline(t, "fan-b")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("each pipeline needs its own conversation: fan-a=%d fan-b=%d", len(a), len(b))
	}
	if a[0].Spec.ProfileRef.Name != "prof-fan-a" || b[0].Spec.ProfileRef.Name != "prof-fan-b" {
		t.Fatalf("each conversation runs its OWN pipeline's profile: %q / %q",
			a[0].Spec.ProfileRef.Name, b[0].Spec.ProfileRef.Name)
	}

	// cooldown is per-SOURCE and evaluated once, above the fan-out
	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "src-fan"}, &src); err != nil {
		t.Fatal(err)
	}
	if len(src.Status.Cooldown) != 1 || src.Status.Cooldown[0].Fingerprint != "fan-1" {
		t.Fatalf("one fingerprint, one cooldown entry: %+v", src.Status.Cooldown)
	}
	if src.Status.ReceivedTotal != 1 {
		t.Fatalf("receivedTotal counts SIGNALS, not conversations: %d", src.Status.ReceivedTotal)
	}

	// a re-delivery inside the window reaches NEITHER pipeline
	if rec := postSignal(t, h, testMasterToken, "src-fan", []map[string]any{
		{"fingerprint": "fan-1", "labels": map[string]string{"alertname": "DiskFull"}, "payload": "still full"},
	}); rec.Code != 200 {
		t.Fatalf("re-delivery: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(convsFromPipeline(t, "fan-a")) + len(convsFromPipeline(t, "fan-b")); n != 2 {
		t.Fatalf("a suppressed fingerprint must open nothing new, got %d conversation(s)", n)
	}
}

// Two pipelines fanning out produce conversations with the SAME signature. The
// recorded origin is what keeps the second pipeline's next signal out of the
// first pipeline's conversation — without it, the group would land under the
// wrong profile with the wrong tools.
func TestFannedOutConversationsNeverMerge(t *testing.T) {
	mkProfile(t, "prof-merge-a")
	mkProfile(t, "prof-merge-b")
	mkSignalSource(t, "src-merge", "am-merge", "")
	mkPipeline(t, "merge-a", []string{"src-merge"}, nil, "prof-merge-a")
	mkPipeline(t, "merge-b", []string{"src-merge"}, nil, "prof-merge-b")
	reconcilePipeline(t, "merge-a")
	reconcilePipeline(t, "merge-b")
	h := apiServer().Handler()

	// two signals sharing a signature (same alertname), distinct fingerprints so
	// cooldown admits both
	for _, fp := range []string{"merge-1", "merge-2"} {
		if rec := postSignal(t, h, testMasterToken, "src-merge", []map[string]any{
			{"fingerprint": fp, "labels": map[string]string{"alertname": "Flapping"}, "payload": fp},
		}); rec.Code != 200 {
			t.Fatalf("signal %s: %d %s", fp, rec.Code, rec.Body.String())
		}
	}

	a, b := convsFromPipeline(t, "merge-a"), convsFromPipeline(t, "merge-b")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("the second signal must GROUP into each pipeline's own conversation: a=%d b=%d", len(a), len(b))
	}
	if len(a[0].Spec.Inputs) != 2 || len(b[0].Spec.Inputs) != 2 {
		t.Fatalf("each conversation takes both signals: a=%d inputs, b=%d inputs",
			len(a[0].Spec.Inputs), len(b[0].Spec.Inputs))
	}
	if a[0].Name == b[0].Name {
		t.Fatal("the two pipelines shared one conversation — provenance scoping is not applied")
	}
}

// A conversation predating pipelineRef still groups while ONE pipeline serves
// the source (the state it was created in), and is left alone once a second
// joins — no invisible pick, and no re-opening every investigation on upgrade.
func TestLegacyConversationGroupsOnlyWhileOnePipelineServes(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-oldconv")
	mkSignalSource(t, "src-legacy", "am-legacy", "")
	mkPipeline(t, "legacy-a", []string{"src-legacy"}, nil, "prof-oldconv")
	reconcilePipeline(t, "legacy-a")
	h := apiServer().Handler()

	if rec := postSignal(t, h, testMasterToken, "src-legacy", []map[string]any{
		{"fingerprint": "leg-1", "labels": map[string]string{"alertname": "Legacy"}, "payload": "first"},
	}); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsFromPipeline(t, "legacy-a")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	// strip the ref to stand in for a conversation created before the field
	legacy := convs[0]
	patch := client.MergeFrom(legacy.DeepCopy())
	legacy.Spec.PipelineRef = nil
	if err := k8sClient.Patch(ctx, &legacy, patch); err != nil {
		t.Fatal(err)
	}

	// still ONE server: the legacy conversation absorbs the next signal
	if rec := postSignal(t, h, testMasterToken, "src-legacy", []map[string]any{
		{"fingerprint": "leg-2", "labels": map[string]string{"alertname": "Legacy"}, "payload": "second"},
	}); rec.Code != 200 {
		t.Fatalf("second signal: %d %s", rec.Code, rec.Body.String())
	}
	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: legacy.Name}, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Spec.Inputs) != 2 {
		t.Fatalf("a legacy conversation must keep grouping while one pipeline serves: %d inputs",
			len(after.Spec.Inputs))
	}

	// a SECOND pipeline joins — the unattributable conversation is now left
	// alone rather than handed to whichever pipeline reconciles first
	mkProfile(t, "prof-oldconv-b")
	mkPipeline(t, "legacy-b", []string{"src-legacy"}, nil, "prof-oldconv-b")
	reconcilePipeline(t, "legacy-b")
	if rec := postSignal(t, h, testMasterToken, "src-legacy", []map[string]any{
		{"fingerprint": "leg-3", "labels": map[string]string{"alertname": "Legacy"}, "payload": "third"},
	}); rec.Code != 200 {
		t.Fatalf("third signal: %d %s", rec.Code, rec.Body.String())
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: legacy.Name}, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Spec.Inputs) != 2 {
		t.Fatalf("a shared source must not absorb an unattributable conversation: %d inputs",
			len(after.Spec.Inputs))
	}
	if n := len(convsFromPipeline(t, "legacy-a")); n != 1 {
		t.Fatalf("legacy-a should open its own attributed conversation, got %d", n)
	}
	if n := len(convsFromPipeline(t, "legacy-b")); n != 1 {
		t.Fatalf("legacy-b should open its own conversation, got %d", n)
	}
}

// Attribution is a READ, not a guess. Two pipelines wired identically used to be
// indistinguishable — both conversations showed as unattributed.
func TestAttributionIsExactForIdenticallyWiredPipelines(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-twin")
	mkChannel(t, "chan-twin", "telegram")
	mkSignalSource(t, "src-twin", "am-twin", "")
	mkPipeline(t, "twin-a", []string{"src-twin"}, []string{"chan-twin"}, "prof-twin")
	mkPipeline(t, "twin-b", []string{"src-twin"}, []string{"chan-twin"}, "prof-twin")
	reconcilePipeline(t, "twin-a")
	reconcilePipeline(t, "twin-b")
	h := apiServer().Handler()

	if rec := postSignal(t, h, testMasterToken, "src-twin", []map[string]any{
		{"fingerprint": "twin-1", "labels": map[string]string{"alertname": "Twin"}, "payload": "x"},
	}); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	for _, name := range []string{"twin-a", "twin-b"} {
		convs := convsFromPipeline(t, name)
		if len(convs) != 1 {
			t.Fatalf("%s: want 1 conversation, got %d", name, len(convs))
		}
		p := chat.PipelineForConversation(ctx, k8sClient, ns, &convs[0])
		if p == nil || p.Name != name {
			t.Fatalf("%s: attribution must read the recorded origin, got %v", name, p)
		}
	}
}

// A bare message on a surface two pipelines serve is refused with the choices —
// it creates nothing, wakes neither agent, and teaches the addressed form.
func TestAmbiguousBareChatMessageIsRefusedWithTheChoices(t *testing.T) {
	mkProfile(t, "prof-amb-a")
	mkProfile(t, "prof-amb-b")
	mkChannel(t, "chan-amb", "telegram")
	mkChatSource(t, "src-amb", "chan-amb")
	mkPipeline(t, "amb-a", []string{"src-amb"}, []string{"chan-amb"}, "prof-amb-a")
	mkPipeline(t, "amb-b", []string{"src-amb"}, []string{"chan-amb"}, "prof-amb-b")
	reconcilePipeline(t, "amb-a")
	reconcilePipeline(t, "amb-b")
	srv := apiServer()

	rec := chatSignal(t, srv, "src-amb", "chan-amb", "who is on call?")
	if rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if reason, _ := out["reason"].(string); !strings.Contains(reason, "several Ready pipelines") {
		t.Fatalf("want an ambiguity reason, got %v", out)
	}
	if n := len(convsBoundTo(t, "chan-amb")); n != 0 {
		t.Fatalf("an ambiguous message must create nothing, got %d conversation(s)", n)
	}

	// the refusal reaches the surface and names EVERY server with its profile
	rec = adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("the refusal must reach the surface: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	body := opBody(op)
	for _, want := range []string{"/amb-a", "/amb-b", "prof-amb-a", "prof-amb-b", "/" + chat.ListCommand} {
		if !strings.Contains(body, want) {
			t.Fatalf("the refusal must carry %q — it is the teaching moment: %q", want, body)
		}
	}
	if strings.Contains(body, "agents serve this chat") {
		t.Fatalf("the refusal calls pipelines agents: %q", body)
	}

	// THE MOMENT CONTROLS EARN MOST. The person has already typed their task,
	// so a surface with controls can send THAT text to the pipeline they pick.
	// The manager states the offer; how it looks is the adapter's business.
	got := map[string]string{}
	for _, c := range op.Message.Choices {
		got[c.Label] = c.Command
	}
	if len(got) != 2 || got["amb-a"] != "/amb-a" || got["amb-b"] != "/amb-b" {
		t.Fatalf("refusal must offer each server as a choice, got %v", got)
	}
	// ...and it is LINKED to the message that provoked it, which is what lets a
	// selection carry the original forward with nothing held in between.
	if op.Message.InReplyTo != chatMessageHandle {
		t.Fatalf("refusal not linked to the original message: %q", op.Message.InReplyTo)
	}
}

// `/agents` and the console's typeahead answer the same question, so they must
// answer it the same way: Ready pipelines, each with its answering profile. The
// console side pins its half in console/agents_test.go; this pins the command's.
func TestAgentsListingMatchesWhatTheTypeaheadOffers(t *testing.T) {
	mkProfile(t, "prof-list-a")
	mkProfile(t, "prof-list-b")
	mkChannel(t, "chan-list", "telegram")
	mkChatSource(t, "src-list", "chan-list")
	mkPipeline(t, "list-ready", []string{"src-list"}, []string{"chan-list"}, "prof-list-a")
	reconcilePipeline(t, "list-ready")
	// dangling profile → Ready=False, so it is addressable by nothing useful
	mkPipeline(t, "list-broken", nil, []string{"chan-list"}, "no-such-profile")
	reconcilePipeline(t, "list-broken")
	srv := apiServer()

	if rec := chatSignal(t, srv, "src-list", "chan-list", "/agents"); rec.Code != 200 {
		t.Fatalf("/agents: %d %s", rec.Code, rec.Body.String())
	}
	rec := adapterReq(srv, "GET", "/channel/ops?adapter=telegram&contract=2&wait=0", nil, "test-adapter-token")
	if rec.Code != 200 {
		t.Fatalf("/agents must emit a send op: %d", rec.Code)
	}
	var op chat.Op
	_ = json.Unmarshal(rec.Body.Bytes(), &op)
	body := opBody(op)
	if !strings.Contains(body, "/list-ready") {
		t.Fatalf("a Ready pipeline must be listed: %q", body)
	}
	if !strings.Contains(body, "prof-list-a") {
		t.Fatalf("each entry carries its profile, as the typeahead does: %q", body)
	}
	if strings.Contains(body, "list-broken") {
		t.Fatalf("an unready pipeline must not be offered: %q", body)
	}
}

// Addressing is independent of how many pipelines serve the surface: it
// resolves by NAME and consults no source refs.
func TestAddressingWorksWithSeveralServers(t *testing.T) {
	mkProfile(t, "prof-addr-a")
	mkProfile(t, "prof-addr-b")
	mkChannel(t, "chan-addr", "telegram")
	mkChatSource(t, "src-addr", "chan-addr")
	mkPipeline(t, "addr-a", []string{"src-addr"}, []string{"chan-addr"}, "prof-addr-a")
	mkPipeline(t, "addr-b", []string{"src-addr"}, []string{"chan-addr"}, "prof-addr-b")
	reconcilePipeline(t, "addr-a")
	reconcilePipeline(t, "addr-b")
	srv := apiServer()

	for _, tc := range []struct{ pipeline, profile string }{
		{"addr-a", "prof-addr-a"},
		{"addr-b", "prof-addr-b"},
	} {
		if rec := chatSignal(t, srv, "src-addr", "chan-addr",
			"/"+tc.pipeline+" check nodes"); rec.Code != 200 {
			t.Fatalf("addressed to %s: %d %s", tc.pipeline, rec.Code, rec.Body.String())
		}
		convs := convsFromPipeline(t, tc.pipeline)
		if len(convs) != 1 {
			t.Fatalf("%s: want 1 conversation, got %d", tc.pipeline, len(convs))
		}
		if convs[0].Spec.ProfileRef.Name != tc.profile {
			t.Fatalf("%s answered with profile %q", tc.pipeline, convs[0].Spec.ProfileRef.Name)
		}
	}
}

// THE OBVIOUS WAY TO IMPLEMENT THIS WRONG. A reply inside a thread carries a
// threadId, arrives on /channel/inbound, and never travels the signal path — so
// it needs no prefix even where a bare general-surface message would be refused.
func TestThreadReplyNeedsNoAddressOnASharedSurface(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-thread")
	mkChannel(t, "chan-thread", "tg-thread")
	mkChatSource(t, "src-thread", "chan-thread")
	mkPipeline(t, "thread-a", []string{"src-thread"}, []string{"chan-thread"}, "prof-thread")
	mkPipeline(t, "thread-b", []string{"src-thread"}, []string{"chan-thread"}, "prof-thread")
	reconcilePipeline(t, "thread-a")
	reconcilePipeline(t, "thread-b")
	srv := apiServer()

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "thread-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-thread"}
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "chan-thread"}}
	conv.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: "thread-a"}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "chan-thread", ThreadID: "7000"}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "POST", "/channel/inbound",
		map[string]any{"channel": "chan-thread", "threadId": "7000", "text": "and the disks?"},
		"test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("threaded reply: %d %s", rec.Code, rec.Body.String())
	}
	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "thread-conv"}, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Spec.Inputs) != 1 || after.Spec.Inputs[0].Type != agentopsv1alpha1.InputReply {
		t.Fatalf("a prefix-free thread reply must be appended as an input: %+v", after.Spec.Inputs)
	}
}
