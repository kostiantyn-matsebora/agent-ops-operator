package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// The handle is what carries a conversation's context between runs, and it used
// to be written ONCE. That was unsound: a run may legitimately end in a
// different context than it was asked to continue, and keeping the first handle
// then named something that no longer existed — so every later message repeated
// the same failed continuation. One recoverable loss became permanent.

func loadConversation(t *testing.T, name string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var c agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

// reportRun completes a run, reporting the context handle under the CURRENT name.
func reportRun(t *testing.T, srv *httpapi.Server, convo, runID, contextID string) {
	t.Helper()
	rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": convo, "runId": runID, "status": "succeeded", "result": "ok",
		"runtimeContextId": contextID,
	}, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
}

func contextConv(t *testing.T, name, profile string) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	return conv
}

// The behaviour this change exists to fix: after a run ends in a different
// context than it was asked to continue, the NEXT message must continue the one
// that now exists — not repeat the failed continuation forever.
func TestAFallbackHandleIsAdoptedSoOneLossStaysOneLoss(t *testing.T) {
	mkProfile(t, "prof-ctx-adopt")
	conv := contextConv(t, "ctx-adopt", "prof-ctx-adopt")
	srv := apiServer()

	reportRun(t, srv, conv.Name, "r-1", "ctx-first")
	if got := loadConversation(t, conv.Name).ContextID(); got != "ctx-first" {
		t.Fatalf("first handle = %q", got)
	}

	// A later run could not continue ctx-first and completed in a new context.
	reportRun(t, srv, conv.Name, "r-2", "ctx-after-loss")

	after := loadConversation(t, conv.Name)
	if got := after.ContextID(); got != "ctx-after-loss" {
		t.Fatalf("the conversation still names a context that no longer exists: %q", got)
	}
	// Write-once was the bug: it would have left ctx-first here forever, so every
	// subsequent message would repeat the same failed continuation.
	if after.Status.RuntimeContextID != "ctx-after-loss" {
		t.Fatalf("handle not recorded under the current field: %+v", after.Status)
	}
}

// A rename that simply moved the field would strand every in-flight handle at
// the moment of upgrade — restarting the context of every conversation at once,
// which is the exact failure this area exists to prevent.
func TestAConversationFromBeforeTheRenameKeepsItsContext(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-ctx-legacy")
	conv := contextConv(t, "ctx-legacy", "prof-ctx-legacy")

	// As an older manager wrote it: only the retired spelling.
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.SessionID = "ctx-legacy-handle"
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	if got := loadConversation(t, conv.Name).ContextID(); got != "ctx-legacy-handle" {
		t.Fatalf("the upgrade lost the handle: %q", got)
	}

	// ...and the next completed run adopts it under the current name, so the
	// fallback can be removed a release later without stranding anything.
	srv := apiServer()
	reportRun(t, srv, conv.Name, "r-1", "ctx-legacy-handle")

	after := loadConversation(t, conv.Name)
	if after.Status.RuntimeContextID != "ctx-legacy-handle" || after.Status.SessionID != "" {
		t.Fatalf("handle not migrated to the current field: %+v", after.Status)
	}
}

// An older RUNTIME still reports the retired name. Dropping its handle would
// restart the conversation's context on every message.
func TestAnOlderRuntimeStillReportingTheRetiredNameIsHonoured(t *testing.T) {
	mkProfile(t, "prof-ctx-oldrt")
	conv := contextConv(t, "ctx-oldrt", "prof-ctx-oldrt")
	srv := apiServer()

	rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": conv.Name, "runId": "r-1", "status": "succeeded", "result": "ok",
		"sessionId": "ctx-from-old-runtime",
	}, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}

	if got := loadConversation(t, conv.Name).ContextID(); got != "ctx-from-old-runtime" {
		t.Fatalf("an older runtime's handle was dropped: %q", got)
	}
}

// Continuity is promised only where it is possible. Without it, a conversation
// is single-run BY DECLARATION and answers fresh — it does not fail, because an
// install with no durable context volume is a configuration the operator chose.
func TestNoHandleIsIssuedWhereContextCannotBeCarried(t *testing.T) {
	mkProfile(t, "prof-ctx-ephemeral")
	conv := contextConv(t, "ctx-ephemeral", "prof-ctx-ephemeral")

	srv := apiServer()
	reportRun(t, srv, conv.Name, "r-1", "ctx-established")

	// Same server, but a deployment that provides no context volume: the reference
	// runtime keeps context there, so continuity is impossible here.
	ephemeral := apiServer()
	ephemeral.Runtime = runtimepod.Config{}

	possible := runtimepod.Resolved{Config: ephemeral.Runtime}.ContinuityPossible()
	if possible {
		t.Fatal("a deployment with no context volume must not promise continuity")
	}
	// The handle is still RECORDED — it is the runtime's, and a later deployment
	// with storage could use it. What changes is that it is not handed back.
	if got := loadConversation(t, conv.Name).ContextID(); got != "ctx-established" {
		t.Fatalf("the handle should still be recorded, got %q", got)
	}
}
