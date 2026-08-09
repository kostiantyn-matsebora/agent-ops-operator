package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// Programmatic origination is an ordinary signal: `kind: task` posted to a
// SignalSource a Ready Pipeline claims. There is no endpoint that names a
// Pipeline.
//
// The fixtures here also pin the SIGNATURE KEYING rule, which is invisible
// until a lane regresses. When a source declares no `signatureLabels`:
//
//	alert / job  — recurring-subject lanes, keyed by the DEFAULT alert labels so
//	               later signals fold into the open conversation and resume
//	task / chat  — one-shot lanes, keyed by the signal's own FINGERPRINT so each
//	               request gets its own conversation
//
// A blanket "always key on the fingerprint" rule passes the task fixtures and
// fails TestAlertsWithNoSignatureLabelsStillGroup and
// TestJobTicksWithDistinctFingerprintsStillFold. A blanket "always use the
// default labels" rule does the reverse. Both halves are load-bearing.

// taskSignal posts one `kind: task` signal as a machine caller would — no
// channel label, because replies go to the claiming Pipeline's channels.
func taskSignal(t *testing.T, srv *httpapi.Server, source, fingerprint, text string,
	labels map[string]string) *httptest.ResponseRecorder {

	t.Helper()
	sig := map[string]any{"fingerprint": fingerprint, "kind": "task", "payload": text}
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

// 3.1 — the vm-bundle shape: an Alertmanager source ships `grouping: {}` and
// relies entirely on the default labels. Two distinct fingerprints describing
// the same alert must share one investigation.
func TestAlertsWithNoSignatureLabelsStillGroup(t *testing.T) {
	mkProfile(t, "prof-alertgroup")
	mkSignalSource(t, "src-alertgroup", "am-group", "")
	mkPipeline(t, "alertgroup-pipe", []string{"src-alertgroup"}, nil, "prof-alertgroup")
	reconcilePipeline(t, "alertgroup-pipe")
	h := apiServer().Handler()

	labels := map[string]string{"alertname": "AlertGroupFixture", "namespace": "prod"}
	for i, fp := range []string{"ag-1", "ag-2"} {
		rec := postSignal(t, h, testMasterToken, "src-alertgroup", []map[string]any{
			{"fingerprint": fp, "labels": labels, "payload": fmt.Sprintf("firing %d", i)},
		})
		if rec.Code != 200 {
			t.Fatalf("alert %s: %d %s", fp, rec.Code, rec.Body.String())
		}
	}
	convs := convsForProfile(t, "prof-alertgroup")
	if len(convs) != 1 {
		t.Fatalf("two alerts sharing alertname/namespace must share one conversation, got %d "+
			"— the alert lane keeps the default signature labels", len(convs))
	}
	if len(convs[0].Spec.Inputs) != 2 {
		t.Fatalf("both alerts must land on it: %+v", convs[0].Spec.Inputs)
	}
}

// 3.2 — the signal-cron shape: the adapter fires a DISTINCT fingerprint per
// tick (`<source>@<tick>`) and depends on the constant signature folding them
// into one conversation, later ticks resuming the session as recurrences.
func TestJobTicksWithDistinctFingerprintsStillFold(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-cronfold")
	mkSignalSource(t, "src-cronfold", "cron-fold", "")
	mkPipeline(t, "cronfold-pipe", []string{"src-cronfold"}, nil, "prof-cronfold")
	reconcilePipeline(t, "cronfold-pipe")
	h := apiServer().Handler()

	// constant labels across ticks — what a cron source carries is the same
	// every time, which is exactly why the default signature collapses them
	labels := map[string]string{"namespace": "cron-fold-fixture"}
	if rec := postSignal(t, h, testMasterToken, "src-cronfold", []map[string]any{
		{"fingerprint": "src-cronfold@tick1", "kind": "job", "labels": labels, "payload": "nightly"},
	}); rec.Code != 200 {
		t.Fatalf("tick1: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsForProfile(t, "prof-cronfold")
	if len(convs) != 1 {
		t.Fatalf("tick1 must open one conversation, got %d", len(convs))
	}
	conv := convs[0]
	if conv.Spec.Inputs[0].Type != agentopsv1alpha1.InputJob || conv.Spec.Inputs[0].JobName != "src-cronfold" {
		t.Fatalf("a job carries the source as its job name: %+v", conv.Spec.Inputs[0])
	}

	// a session exists once the agent has run: the next tick resumes it
	conv.Status.SessionID = "sess-cronfold"
	if err := k8sClient.Status().Update(ctx, &conv); err != nil {
		t.Fatal(err)
	}
	if rec := postSignal(t, h, testMasterToken, "src-cronfold", []map[string]any{
		{"fingerprint": "src-cronfold@tick2", "kind": "job", "labels": labels, "payload": "nightly"},
	}); rec.Code != 200 {
		t.Fatalf("tick2: %d %s", rec.Code, rec.Body.String())
	}
	convs = convsForProfile(t, "prof-cronfold")
	if len(convs) != 1 {
		t.Fatalf("a second tick must NOT open a second conversation, got %d "+
			"— the job lane keeps the default signature labels", len(convs))
	}
	inputs := convs[0].Spec.Inputs
	if len(inputs) != 2 {
		t.Fatalf("want two inputs on the folded conversation: %+v", inputs)
	}
	if inputs[1].Type != agentopsv1alpha1.InputRecurrence {
		t.Fatalf("tick2 must resume the session as a recurrence, got %q", inputs[1].Type)
	}
}

// 3.3 — the one-shot half of the rule: each posted task is its own request.
func TestTaskSignalsOpenOneConversationEach(t *testing.T) {
	mkProfile(t, "prof-taskeach")
	mkSignalSource(t, "src-taskeach", "task-each", "")
	mkPipeline(t, "taskeach-pipe", []string{"src-taskeach"}, nil, "prof-taskeach")
	reconcilePipeline(t, "taskeach-pipe")
	srv := apiServer()

	for i, fp := range []string{"te-1", "te-2"} {
		if rec := taskSignal(t, srv, "src-taskeach", fp, fmt.Sprintf("ask %d", i), nil); rec.Code != 200 {
			t.Fatalf("task %s: %d %s", fp, rec.Code, rec.Body.String())
		}
	}
	convs := convsForProfile(t, "prof-taskeach")
	if len(convs) != 2 {
		t.Fatalf("two posted tasks are two requests, want 2 conversations, got %d "+
			"— a one-shot lane keys on its own fingerprint", len(convs))
	}
	for _, c := range convs {
		if len(c.Spec.Inputs) != 1 {
			t.Fatalf("%s: want one input, got %+v", c.Name, c.Spec.Inputs)
		}
		in := c.Spec.Inputs[0]
		if in.Type != agentopsv1alpha1.InputTask {
			t.Fatalf("%s: task lane expected, got %q", c.Name, in.Type)
		}
		if in.JobName != "" {
			t.Fatalf("%s: a one-off ask carries no job name, got %q", c.Name, in.JobName)
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

// 3.5 — explicit labels win in every lane: an operator who asks for grouping
// gets it, task included.
func TestTaskSignalsGroupWhenTheSourceDeclaresLabels(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-taskgroup")
	mkSignalSource(t, "src-taskgroup", "task-group", "")
	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "src-taskgroup"}, &src); err != nil {
		t.Fatal(err)
	}
	src.Spec.Grouping.SignatureLabels = []string{"team"}
	if err := k8sClient.Update(ctx, &src); err != nil {
		t.Fatal(err)
	}
	mkPipeline(t, "taskgroup-pipe", []string{"src-taskgroup"}, nil, "prof-taskgroup")
	reconcilePipeline(t, "taskgroup-pipe")
	srv := apiServer()

	labels := map[string]string{"team": "platform"}
	for i, fp := range []string{"tg-1", "tg-2"} {
		if rec := taskSignal(t, srv, "src-taskgroup", fp, fmt.Sprintf("ask %d", i), labels); rec.Code != 200 {
			t.Fatalf("task %s: %d %s", fp, rec.Code, rec.Body.String())
		}
	}
	convs := convsForProfile(t, "prof-taskgroup")
	if len(convs) != 1 {
		t.Fatalf("declared signatureLabels group tasks too, want 1 conversation, got %d", len(convs))
	}
	if len(convs[0].Spec.Inputs) != 2 {
		t.Fatalf("both tasks must land on the grouped conversation: %+v", convs[0].Spec.Inputs)
	}
}
