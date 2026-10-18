package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// Programmatic origination is an ordinary signal: `kind: task` posted to a
// SignalSource a Ready Pipeline claims. There is no endpoint that names a
// Pipeline.
//
// The alert/job/task grouping-by-label scenarios this file used to pin
// (TestAlertsWithNoSignatureLabelsStillGroup, TestJobTicksWithDistinctFingerprintsStillFold,
// TestTaskSignalsGroupWhenTheSourceDeclaresLabels) tested SignalSource.spec.grouping,
// which is retired: routing is now pure initiationId equality, decided by the
// sender, never derived from labels. The scenarios signal-correlation-id adds
// in its place — open-match recurrence, fresh-id creation, distinct-ids-never-merge,
// closed-conversation exclusion — live in correlation_test.go.

// taskSignal posts one `kind: task` signal as a machine caller would — no
// channel label, because replies go to the claiming Pipeline's channels.
func taskSignal(t *testing.T, srv *httpapi.Server, source, initiationID, text string,
	labels map[string]string) *httptest.ResponseRecorder {

	t.Helper()
	sig := map[string]any{"initiationId": initiationID, "kind": "task", "payload": text}
	if labels != nil {
		sig["labels"] = labels
	}
	return adapterReq(srv, "POST", "/signal/inbound", map[string]any{
		"source": source, "signals": []map[string]any{sig},
	}, testMasterToken)
}

// convsForProfile collects the conversations a test's own profile originated —
// every test makes its own, so this isolates counts from the shared namespace.
func convsForProfile(t *testing.T, profile string) []agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	var out []agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == profile {
			out = append(out, list.Items[i])
		}
	}
	return out
}

// 3.3 — each posted task is its own request: distinct initiationIds never merge.
func TestTaskSignalsOpenOneConversationEach(t *testing.T) {
	mkProfile(t, "prof-taskeach")
	mkSignalSource(t, "src-taskeach", "task-each", "")
	mkPipeline(t, "taskeach-pipe", []string{"src-taskeach"}, nil, "prof-taskeach")
	reconcilePipeline(t, "taskeach-pipe")
	srv := apiServer()

	for i, id := range []string{"te-1", "te-2"} {
		if rec := taskSignal(t, srv, "src-taskeach", id, fmt.Sprintf("ask %d", i), nil); rec.Code != 200 {
			t.Fatalf("task %s: %d %s", id, rec.Code, rec.Body.String())
		}
	}
	convs := convsForProfile(t, "prof-taskeach")
	if len(convs) != 2 {
		t.Fatalf("two posted tasks are two requests, want 2 conversations, got %d "+
			"— distinct initiationIds never merge", len(convs))
	}
	for _, c := range convs {
		if len(c.Spec.Inputs) != 1 {
			t.Fatalf("%s: want one input, got %+v", c.Name, c.Spec.Inputs)
		}
		in := c.Spec.Inputs[0]
		if in.Type != agentopsv1alpha1.InputTask {
			t.Fatalf("%s: task lane expected, got %q", c.Name, in.Type)
		}
		if in.Origin == nil || in.Origin.Kind != agentopsv1alpha1.OriginSignal ||
			in.Origin.SignalKind != "task" {
			t.Fatalf("%s: a posted task records a signal origin of kind task: %+v", c.Name, in.Origin)
		}
		if !in.DeliverTo("chan-multi-a", in.OriginSurface(nil), true) {
			t.Fatalf("%s: a posted task entered on no surface, so every channel is owed it", c.Name)
		}
	}
}

// 3.4 — a task needs no chat surface: it is accepted without the channel label
// a chat signal requires, and the reply goes to the claiming Pipeline's
// channels, which the conversation carries.
func TestTaskSignalNeedsNoChannelLabel(t *testing.T) {
	mkProfile(t, "prof-tasknochan")
	mkChannel(t, "chan-tasknochan", "telegram")
	mkSignalSource(t, "src-tasknochan", "task-nochan", "")
	mkPipeline(t, "tasknochan-pipe", []string{"src-tasknochan"}, []string{"chan-tasknochan"}, "prof-tasknochan")
	reconcilePipeline(t, "tasknochan-pipe")
	srv := apiServer()

	rec := taskSignal(t, srv, "src-tasknochan", "tnc-1", "check the cluster", nil)
	if rec.Code != 200 {
		t.Fatalf("a task carries no %s label and must still be accepted: %d %s",
			httpapi.LabelChatChannel, rec.Code, rec.Body.String())
	}
	convs := convsForProfile(t, "prof-tasknochan")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	conv := convs[0]
	if !conv.BoundTo("chan-tasknochan") {
		t.Fatalf("the reply goes to the claiming pipeline's channels: %+v", conv.Spec.ChannelRefs)
	}
	// a task payload is never parsed as a chat command: it does not go through
	// the chat router, so a leading slash is text
	if rec := taskSignal(t, srv, "src-tasknochan", "tnc-2", "/etc/hosts is wrong", nil); rec.Code != 200 {
		t.Fatalf("slash-prefixed task: %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range convsForProfile(t, "prof-tasknochan") {
		if c.Name == conv.Name {
			continue
		}
		// signal payloads are stored out of line
		ref := c.Spec.Inputs[0].PayloadRef
		if ref == nil {
			t.Fatalf("%s: signal input must carry a payload ref: %+v", c.Name, c.Spec.Inputs[0])
		}
		var ci agentopsv1alpha1.ConversationInput
		if err := k8sClient.Get(context.Background(),
			types.NamespacedName{Namespace: ns, Name: ref.Name}, &ci); err != nil {
			t.Fatal(err)
		}
		if ci.Spec.Payload != "/etc/hosts is wrong" {
			t.Fatalf("a task beginning with / is text, not a command: %q", ci.Spec.Payload)
		}
	}
}
