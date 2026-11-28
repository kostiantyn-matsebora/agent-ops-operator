//go:build system

package system

// conversation-identity ("Several conversations share one correlation key")
// and conversation-naming ("Recurrence after close keeps the numbered
// suffix") together drive chat.Router.refuseReopenBehindOpenSuccessor
// (internal/chat/router.go): reopening a closed conversation while a `-N`
// successor already carries the SAME correlationId and is open must be
// refused, naming the successor — otherwise two open conversations would
// answer for one correlation id. The pure refusal is pinned at envtest; this
// lane is what only a real cluster adds: the refusal reached through the
// real async /reopen task (design D-6, PreconditionFailed, terminal, never
// retried), the same observation pattern coordinator_test.go's own
// ancestor-root-close refusal already uses.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func TestReopenRefusedWhileAnOpenSuccessorSharesTheCorrelationID(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	// namewords.Words keeps only the first THREE non-stopword tokens and
	// drops whole trailing words past MaxWordChainLength (24) — a title
	// built from three fixed words plus a full UnixNano stamp would have its
	// stamp dropped at the 3-word cap before length even enters it, so the
	// title's own stamp is SHORT (the same idiom coordinator_test.go's
	// moveLane uses): two fixed words plus this short stamp is exactly three
	// tokens, 23 characters, well inside the bound. ids/names stay on the
	// FULL stamp: neither is ever passed through namewords.
	shortStamp := stamp[len(stamp)-6:]
	title := "reopen-successor-" + shortStamp
	id := "e2e-reopen-successor-id-" + stamp
	auth := bearerPrefix + e.Values.AdapterToken

	// A DEDICATED source and Pipeline, reused for both posts below. The
	// recurrence posts the SAME initiationId on purpose — reposting into a
	// claimed, unaddressed source would hit the source's own cooldown
	// (signal-persistence spec step 3, "Cooldown is consulted and spent... a
	// suppressed signal ends Dropped, reason cooldown"), which this lane is
	// not testing and would otherwise have to wait out (the chart default is
	// 6h, and signalsource_types.go's own CooldownHours doc notwithstanding,
	// internal/signalroute/cooldown.go's recoverCooldown only special-cases
	// kind=="chat", not "task", so a zero on the SignalSource itself cannot
	// express "no suppression" for a task-kind signal at all).
	//
	// An ADDRESSED chat command (`/<pipeline> <task>`) is the one path the
	// spec itself exempts from cooldown entirely (signal-persistence spec,
	// Requirement "The consumer makes every routing decision", step 1: "no
	// claim check, no Ready check, no cooldown") — so both posts below
	// address the pipeline by name directly, which is also what makes which
	// Pipeline claims the source irrelevant here.
	srcName := "e2e-reopen-successor-src-" + stamp
	src := source(srcName, "e2e", nil)
	mustCreate(t, e.K, src)
	p := pipeline(src.Name, ProfileStub, []string{src.Name}, []string{ChannelConsole})
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	chatLabels := map[string]string{"agentops.dev/channel": ChannelConsole, "agentops.dev/sender": "e2e-reopen-successor"}

	var names []string
	t.Cleanup(func() {
		for _, n := range names {
			_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: n}})
		}
	})

	e.PostSignal(t, src.Name, map[string]any{
		"initiationId": id, "kind": "chat", "payload": "/" + p.Name + " echo first-life",
		"title": title, "labels": chatLabels,
	})
	first := e.ConversationFor(t, id, 2*time.Minute)
	names = append(names, first.Name)
	first = e.WaitRun(t, first.Name, 1, 3*time.Minute)
	if first.Spec.CorrelationID != id {
		t.Fatalf("the first life must adopt the posted initiationId verbatim, got %q want %q", first.Spec.CorrelationID, id)
	}

	thread := waitThreadOn(t, ctx, e, first.Name, ChannelConsole, 2*time.Minute)
	closeCode, closeAccepted := postChannelInbound(t, e, auth, ChannelConsole, thread, "/close", "e2e-reopen-successor")
	if closeCode != 202 || len(closeAccepted.TaskIDs) != 1 {
		t.Fatalf("closing the first life: %d %+v", closeCode, closeAccepted)
	}
	waitTaskSettled(t, e, auth, closeAccepted.TaskIDs[0], 2*time.Minute)
	waitFor(t, "the first life closed", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, first.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})

	// Recurrence: the SAME initiationId, posted again, must open a SECOND
	// life adopting the same correlationId under the next numbered suffix,
	// never land back on the now-closed first life (conversation-
	// housekeeping: "A matching signal does not land on a closed
	// conversation").
	e.PostSignal(t, src.Name, map[string]any{
		"initiationId": id, "kind": "chat", "payload": "/" + p.Name + " echo second-life",
		"title": title, "labels": chatLabels,
	})
	successor := waitSuccessorLife(t, ctx, e, id, first.Name)
	names = append(names, successor.Name)
	// The successor's name carries the SAME base as the first life's own
	// stored name (which also carries namewords' own "<kind>-" prefix, never
	// hard-coded here) plus the next numbered suffix.
	wantSuccessor := first.Name + "-2"
	if successor.Name != wantSuccessor {
		t.Fatalf("recurrence after close must keep the numbered suffix, got %q want %q", successor.Name, wantSuccessor)
	}
	successor = e.WaitRun(t, successor.Name, 1, 3*time.Minute)
	if successor.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatalf("the successor must be OPEN for this test's own refusal to mean anything, got Closed")
	}

	// Reopening the FIRST life now, while its successor is open and shares
	// its correlationId, must be refused rather than silently producing two
	// open conversations claiming the same id.
	requireReopenRefused(t, ctx, e, auth, first.Name)
}

// requireReopenRefused asks to reopen a closed conversation and requires the
// request to be accepted, the task to die terminally precondition_failed and
// the conversation to stay Closed.
func requireReopenRefused(t *testing.T, ctx context.Context, e *Env, auth, name string) {
	t.Helper()
	reopenCode, reopenOut := e.do(t, "POST",
		e.Manager.URL()+"/channel/conversations/"+url.PathEscape(name)+"/reopen",
		[]byte(`{"channel":"`+ChannelConsole+`"}`), auth)
	var accepted acceptedWire
	_ = json.Unmarshal([]byte(reopenOut), &accepted)
	if reopenCode != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("the reopen request itself must be ACCEPTED — the refusal is the task's own terminal outcome, never an immediate 4xx here: %d %s", reopenCode, reopenOut)
	}
	dead := waitTaskState(t, e, auth, accepted.TaskIDs[0], "dead", 2*time.Minute)
	if dead.Code != "precondition_failed" {
		t.Fatalf("a reopen refused behind an open correlationId successor must be precondition_failed, got %q", dead.Code)
	}
	if dead.Attempt > 1 {
		t.Fatalf("this refusal is terminal and must never be retried, got %d attempts", dead.Attempt)
	}

	// And the refusal actually refused: the first life is still Closed.
	final, err := e.K.Conversation(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("the first life must still be Closed after the refused reopen, got %q", final.Status.Phase)
	}
}

// waitSuccessorLife waits for a second conversation adopting correlationID.
func waitSuccessorLife(t *testing.T, ctx context.Context, e *Env, id, firstName string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var successor *agentopsv1alpha1.Conversation
	waitFor(t, "a second life opened for the same correlationId", 2*time.Minute, func() (bool, error) {
		items, err := e.K.Conversations(ctx)
		if err != nil {
			return false, err
		}
		for i := range items {
			c := &items[i]
			if c.Spec.CorrelationID == id && c.Name != firstName {
				successor = c
				return true, nil
			}
		}
		return false, nil
	})
	return successor
}
