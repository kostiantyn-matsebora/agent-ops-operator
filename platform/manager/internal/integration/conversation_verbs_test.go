package integration

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// The two manager-side verbs a surface can reach: reopen and delete.
//
// Their reach is the BINDING, and that is the amendment the archived-thread case
// forces on "no remote close verb exists". That rule protected a property, not a
// syntax — you may only end a conversation you are part of — and holding a live
// thread was how membership was PROVEN. A closed conversation holds no thread,
// so membership is read from the binding that put the thread there, off the
// conversation and never off the request.

func mkVerbConv(t *testing.T, name string, channels []string, phase agentopsv1alpha1.ConversationPhase) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "verb-profile"}
	for _, c := range channels {
		conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	if phase != "" {
		conv.Status.Phase = phase
		if phase == agentopsv1alpha1.ConversationClosed {
			now := metav1.Now()
			conv.Status.ClosedAt = &now
		}
		if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
			t.Fatal(err)
		}
	}
	return conv
}

func getConv(t *testing.T, name string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var got agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: name}, &got); err != nil {
		t.Fatal(err)
	}
	return &got
}

// A surface may act only on a conversation it is bound to, and the refusal has
// to name the binding — "forbidden" alone sends nobody anywhere.
func TestVerbsRefuseASurfaceWithNoBinding(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-a", "tg")
	mkChannel(t, "verb-ch-b", "tg")
	mkVerbConv(t, "verb-unbound", []string{"verb-ch-a"}, agentopsv1alpha1.ConversationClosed)

	for _, verb := range []string{"reopen", "delete"} {
		rec := adapterReq(srv, "POST", "/channel/conversations/verb-unbound/"+verb,
			map[string]string{"channel": "verb-ch-b"}, "test-adapter-token")
		if rec.Code != 403 {
			t.Fatalf("%s from an unbound surface must be refused, got %d: %s", verb, rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); !strings.Contains(body, "not bound") {
			t.Errorf("%s refusal must name the binding: %s", verb, body)
		}
	}
	// and the conversation is untouched by the refusal
	if getConv(t, "verb-unbound").Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Error("a refused verb must change nothing")
	}
}

// Delete refuses a live conversation rather than closing it first: a
// close-then-delete verb would do the irreversible thing to a conversation that
// was still working, behind a confirmation that named only the delete.
func TestDeleteRefusesAConversationThatIsNotClosed(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-live", "tg")
	mkVerbConv(t, "verb-live", []string{"verb-ch-live"}, agentopsv1alpha1.ConversationIdle)

	rec := adapterReq(srv, "POST", "/channel/conversations/verb-live/delete",
		map[string]string{"channel": "verb-ch-live"}, "test-adapter-token")
	if rec.Code != 409 {
		t.Fatalf("delete of a live conversation must be refused, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "close it first") {
		t.Errorf("the refusal must name the missing step: %s", body)
	}
	// NOT closed as a side effect — the point of refusing
	if got := getConv(t, "verb-live"); got.Status.Phase != agentopsv1alpha1.ConversationIdle {
		t.Fatalf("a refused delete must not close it, phase=%q", got.Status.Phase)
	}
}

// An unauthenticated or wrongly-scoped caller reaches neither verb.
func TestVerbsRequireAdapterAuth(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-auth", "tg")
	mkVerbConv(t, "verb-auth", []string{"verb-ch-auth"}, agentopsv1alpha1.ConversationClosed)

	for _, verb := range []string{"reopen", "delete"} {
		if rec := adapterReq(srv, "POST", "/channel/conversations/verb-auth/"+verb,
			map[string]string{"channel": "verb-ch-auth"}, ""); rec.Code != 401 {
			t.Fatalf("%s without a token: %d", verb, rec.Code)
		}
		if rec := adapterReq(srv, "POST", "/channel/conversations/verb-auth/"+verb,
			map[string]string{"channel": "verb-ch-auth"}, "wrong"); rec.Code != 401 {
			t.Fatalf("%s with a bad token: %d", verb, rec.Code)
		}
	}
	if getConv(t, "verb-auth").Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Error("an unauthenticated verb must change nothing")
	}
}

// The happy paths, and the one thing reopen must NOT do: re-resolve wiring.
func TestReopenRestoresIdleAndLeavesWiringExactlyAsItWas(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-ok", "tg")
	mkProfile(t, "verb-profile")
	conv := mkVerbConv(t, "verb-reopen", []string{"verb-ch-ok"}, agentopsv1alpha1.ConversationClosed)
	conv.Status.RuntimeContextID = "ctx-keep-me"
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "POST", "/channel/conversations/verb-reopen/reopen",
		map[string]string{"channel": "verb-ch-ok"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("reopen: %d %s", rec.Code, rec.Body.String())
	}
	got := getConv(t, "verb-reopen")
	if got.Status.Phase != agentopsv1alpha1.ConversationIdle {
		t.Fatalf("reopen must restore Idle, got %q", got.Status.Phase)
	}
	if got.Status.ClosedAt != nil {
		t.Error("reopen must clear closedAt — that timestamp IS the delete clock")
	}
	if got.Status.Reopens != 1 {
		t.Errorf("reopens must advance so the re-established topic op is distinct, got %d", got.Status.Reopens)
	}
	// the whole point: the SAME conversation, not a new one wearing its name
	if got.Status.RuntimeContextID != "ctx-keep-me" {
		t.Error("reopen must keep the context handle")
	}
	if len(got.Spec.ChannelRefs) != 1 || got.Spec.ChannelRefs[0].Name != "verb-ch-ok" {
		t.Errorf("reopen must leave materialized refs untouched: %+v", got.Spec.ChannelRefs)
	}
	if got.Spec.ProfileRef.Name != "verb-profile" {
		t.Errorf("reopen must not re-resolve the profile: %q", got.Spec.ProfileRef.Name)
	}
}

// A reopen whose wiring is gone fails NAMING the missing object, rather than
// producing a conversation that looks alive and can never dispatch.
func TestReopenFailsNamingAMissingProfile(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-gone", "tg")
	conv := mkVerbConv(t, "verb-gone", []string{"verb-ch-gone"}, agentopsv1alpha1.ConversationClosed)
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "profile-that-never-existed"}
	if err := k8sClient.Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}

	rec := adapterReq(srv, "POST", "/channel/conversations/verb-gone/reopen",
		map[string]string{"channel": "verb-ch-gone"}, "test-adapter-token")
	if rec.Code != 409 {
		t.Fatalf("reopen with a missing profile must fail, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "profile-that-never-existed") {
		t.Errorf("the failure must NAME the missing ref: %s", body)
	}
	if getConv(t, "verb-gone").Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Error("a failed reopen must not partially reopen")
	}
}

func TestDeleteReclaimsAClosedConversation(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-del", "tg")
	mkVerbConv(t, "verb-del", []string{"verb-ch-del"}, agentopsv1alpha1.ConversationClosed)

	rec := adapterReq(srv, "POST", "/channel/conversations/verb-del/delete",
		map[string]string{"channel": "verb-ch-del"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("delete of a closed conversation: %d %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "verb-del"}, &got)
	// Either gone, or held by the close-topics finalizer with a deletion stamp.
	if err == nil && got.DeletionTimestamp.IsZero() {
		t.Fatal("delete must remove the conversation")
	}
}

// mkCascadeConv builds a conversation for the delete-cascade test: a root
// with no parent, or a member/grandchild naming one via causedBy — the same
// label the cascade filters on, and the same field it confirms against.
func mkCascadeConv(t *testing.T, name, parent string) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "verb-profile"}
	if parent != "" {
		conv.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: "worker"}
		conv.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
	} else {
		conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "verb-ch-cascade"}}
	}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.ClosedAt = &now
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	return conv
}

// Deleting a root cascades to every conversation reachable from it by
// causedBy, AT ANY DEPTH — mirroring the close cascade, so a coordination's
// members are never left orphaned, unreachable and occupying the API after
// their root is gone. Two members, one itself a root with its own member.
func TestDeleteCascadesToDescendantsAtAnyDepth(t *testing.T) {
	srv := apiServer()
	mkChannel(t, "verb-ch-cascade", "tg")

	root := mkCascadeConv(t, "cascade-root", "")
	mkCascadeConv(t, "cascade-member-a", root.Name)
	memberB := mkCascadeConv(t, "cascade-member-b", root.Name)
	mkCascadeConv(t, "cascade-grandchild", memberB.Name)

	rec := adapterReq(srv, "POST", "/channel/conversations/cascade-root/delete",
		map[string]string{"channel": "verb-ch-cascade"}, "test-adapter-token")
	if rec.Code != 202 {
		t.Fatalf("delete of the root: %d %s", rec.Code, rec.Body.String())
	}

	for _, name := range []string{"cascade-root", "cascade-member-a", "cascade-member-b", "cascade-grandchild"} {
		var got agentopsv1alpha1.Conversation
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &got)
		// Either gone, or held by its OWN finalizer with a deletion stamp —
		// the same "either" TestDeleteReclaimsAClosedConversation already
		// allows for the root, extended here to every descendant.
		if err == nil && got.DeletionTimestamp.IsZero() {
			t.Fatalf("%s must be deleted along with its root", name)
		}
	}
}

// Deletion tells every bound thread that the conversation is gone — whether or
// not it was closed first. A closed conversation's threads were told it could
// be reopened; deletion makes that false, and correcting it is the point.
func TestDeletionTellsEveryBoundThread(t *testing.T) {
	ops := testOps()
	r := &controller.ConversationReconciler{
		Client: k8sClient, Scheme: scheme, MaxActiveConversations: 100, Ops: ops,
		Runtime: runtimepod.Config{Image: "busybox:stub", ServiceAccount: "default"},
	}
	mkChannel(t, "gone-ch", "tg")
	conv := mkVerbConv(t, "gone-conv", []string{"gone-ch"}, agentopsv1alpha1.ConversationClosed)
	// already archived by its close: deletion must still speak
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "gone-ch", ThreadID: "77"}}
	conv.Status.ThreadsArchived = []string{"gone-ch"}
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "gone-conv"}}
	// One pass first so the close-topics finalizer is on the object; without it
	// the API server removes the conversation immediately and there is nobody
	// left to tell.
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Delete(context.Background(), getConv(t, "gone-conv")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	op := ops.Claim("tg")
	if op == nil {
		t.Fatal("deletion must enqueue an operation for the bound thread")
	}
	if op.Kind != chat.OpDeleteConversation {
		t.Fatalf("deletion must send delete-conversation, got %q", op.Kind)
	}
	if op.ThreadID == nil || *op.ThreadID != "77" {
		t.Fatalf("it must carry the thread id: %+v", op.ThreadID)
	}
	if op.Message == nil || !strings.Contains(op.Message.Body, "deleted") {
		t.Fatalf("it must carry the notice: %+v", op.Message)
	}
	// and NOT close-topic: one lifecycle event, one operation
	if again := ops.Claim("tg"); again != nil {
		t.Fatalf("deletion must send exactly one operation per thread, also got %q", again.Kind)
	}
}

// A thread whose topic was never created (ensure-topic never succeeded,
// ThreadID stays "") must not get a close-topic op — there is nothing to
// close, and every adapter rejects an empty thread id. Enqueueing it anyway
// retries forever and logs an error on every reconcile pass.
func TestCloseTopicSkipsAThreadWithNoTopicYet(t *testing.T) {
	ops := testOps()
	r := &controller.ConversationReconciler{
		Client: k8sClient, Scheme: scheme, MaxActiveConversations: 100, Ops: ops,
		Runtime: runtimepod.Config{Image: "busybox:stub", ServiceAccount: "default"},
	}
	mkChannel(t, "no-topic-ch", "tg")
	conv := mkVerbConv(t, "no-topic-conv", []string{"no-topic-ch"}, agentopsv1alpha1.ConversationClosed)
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "no-topic-ch", ThreadID: ""}}
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "no-topic-conv"}}

	// Two passes: a single pass proves nothing was enqueued once, a second
	// proves it is not a one-shot skip that still archives silently.
	for i := 0; i < 2; i++ {
		if _, err := r.Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}

	if op := ops.Claim("tg"); op != nil {
		t.Fatalf("a thread with no topic yet must get no close-topic op, got %q", op.Kind)
	}
	if got := getConv(t, "no-topic-conv"); got.Status.ThreadArchived("no-topic-ch") {
		t.Fatal("a thread with no topic yet must not be recorded as archived")
	}
}

// Once ensure-topic finally succeeds and the thread gets a real thread id,
// the next reconcile pass must enqueue close-topic for it normally — the
// skip above is a re-evaluated condition, not a permanent exclusion.
func TestCloseTopicResumesOnceATopicArrives(t *testing.T) {
	ops := testOps()
	r := &controller.ConversationReconciler{
		Client: k8sClient, Scheme: scheme, MaxActiveConversations: 100, Ops: ops,
		Runtime: runtimepod.Config{Image: "busybox:stub", ServiceAccount: "default"},
	}
	mkChannel(t, "late-topic-ch", "tg")
	conv := mkVerbConv(t, "late-topic-conv", []string{"late-topic-ch"}, agentopsv1alpha1.ConversationClosed)
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "late-topic-ch", ThreadID: ""}}
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "late-topic-conv"}}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if op := ops.Claim("tg"); op != nil {
		t.Fatalf("still no topic: no op expected yet, got %q", op.Kind)
	}

	// ensure-topic succeeds later: the thread id arrives.
	conv = getConv(t, "late-topic-conv")
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "late-topic-ch", ThreadID: "99"}}
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	op := ops.Claim("tg")
	if op == nil {
		t.Fatal("a thread that just got a real topic must have close-topic enqueued")
	}
	if op.Kind != chat.OpCloseTopic {
		t.Fatalf("expected close-topic, got %q", op.Kind)
	}
	if op.ThreadID == nil || *op.ThreadID != "99" {
		t.Fatalf("it must carry the thread id: %+v", op.ThreadID)
	}
}

// mkCorrelatedConv creates a conversation carrying a conversationId and a
// pipelineRef directly — the two fields reopen's at-most-one-open check
// (design: "Reopen keeps the at-most-one-open invariant") reads — without
// going through the signal-ingest path, which is pinned separately in
// correlation_test.go. No channel is bound: validateRefs only walks
// ChannelRefs, so leaving it empty keeps these fixtures about reopen's own
// successor check rather than about topic binding.
func mkCorrelatedConv(t *testing.T, name, profile, pipeline, conversationID string, phase agentopsv1alpha1.ConversationPhase) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	conv.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: pipeline}
	conv.Spec.ConversationID = conversationID
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	conv.Status.Phase = phase
	if phase == agentopsv1alpha1.ConversationClosed {
		now := metav1.Now()
		conv.Status.ClosedAt = &now
	}
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	return conv
}

// Reopen's at-most-one-open invariant: a closed conversation MUST NOT reopen
// while another OPEN conversation already carries the same conversationId
// for the same pipeline — design.md's "Reopen keeps the at-most-one-open
// invariant", which the proposal's reopen-tiebreak decision states as a
// SHALL. Without this refusal, two conversations would answer for one
// correlation id — the bug this whole change exists to kill, in mirror
// image on the reopen path rather than on ingest.
func TestReopenRefusedWhileOpenSuccessorSharesConversationID(t *testing.T) {
	mkProfile(t, "profile-reopen-guard")
	closed := mkCorrelatedConv(t, "reopen-guard-closed", "profile-reopen-guard", "pipe-reopen-guard", "reopen-guard-1", agentopsv1alpha1.ConversationClosed)
	successor := mkCorrelatedConv(t, "reopen-guard-open", "profile-reopen-guard", "pipe-reopen-guard", "reopen-guard-1", agentopsv1alpha1.ConversationIdle)
	waitForRoutingCacheToObserve(t, successor, successor.ResourceVersion)

	srv := apiServer()
	err := srv.Router.ReopenConversation(context.Background(), getConv(t, closed.Name))
	if err == nil {
		t.Fatal("reopen must be refused while an open successor shares the conversationId")
	}
	if !strings.Contains(err.Error(), successor.Name) {
		t.Errorf("the refusal must NAME the successor, got: %v", err)
	}
	if getConv(t, closed.Name).Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Error("a refused reopen must not partially reopen")
	}
}

// The mirror of the refusal above: once no OPEN conversation carries the
// same conversationId for the same pipeline, reopen proceeds exactly as it
// always did — the successor check adds a refusal, never a NEW permission
// requirement.
func TestReopenPermittedOnceNoOpenSuccessorSharesConversationID(t *testing.T) {
	mkProfile(t, "profile-reopen-free")
	closed := mkCorrelatedConv(t, "reopen-free-closed", "profile-reopen-free", "pipe-reopen-free", "reopen-free-1", agentopsv1alpha1.ConversationClosed)

	srv := apiServer()
	if err := srv.Router.ReopenConversation(context.Background(), getConv(t, closed.Name)); err != nil {
		t.Fatalf("reopen with no open successor must be permitted, got: %v", err)
	}
	if got := getConv(t, closed.Name).Status.Phase; got != agentopsv1alpha1.ConversationIdle {
		t.Errorf("reopen must restore Idle, got %q", got)
	}
}
