package integration

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
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
