package integration

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// AN ANSWER REACHES A BOUND THREAD WITH ITS TAGS INTACT.
//
// End to end, because the claim spans three components: the run is recorded,
// the reconciler backstop composes the reply from that record alone, and the
// adapter receives it at the door. The manager must not have touched a
// character on the way — the adapters parse, and a body it rewrote could not be
// the same text a viewer reads back from `status.runs[].result`.
func TestAgentTextReachesTheThreadUnaltered(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-blocks")
	mkChannel(t, "chan-blocks", "tg-blocks")
	conv := deliveryConv(t, "blocks-conv", "prof-blocks", "chan-blocks")

	const agentOutput = "<title>\nDisk filling on node/1\n</title>\n" +
		"<root-cause>\nLog rotation stopped 4 days ago.\n</root-cause>\n" +
		"<details>\nthe long tail nobody reads first\n</details>"

	patch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "r-blocks", Status: "succeeded", Result: agentOutput,
		FinishedAt: &now, DeliveryTracked: true,
	}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	reconcileWithOps(t, srv, conv.Name)

	answers := answersFor(drainOps(t, srv, "tg-blocks"))
	if len(answers) != 1 {
		t.Fatalf("want one answer on the bound thread, got %d", len(answers))
	}
	if got := answers[0].Message.Body; got != agentOutput {
		t.Fatalf("the manager altered the agent's text:\n got  %q\n want %q", got, agentOutput)
	}
	// And the recorded run still holds exactly the same characters, which is
	// what lets a rehydrated transcript render like a live one.
	if rec := runNamed(t, loadConv(t, conv.Name), "r-blocks").Result; rec != agentOutput {
		t.Fatalf("the record diverged from the delivered body:\n%q", rec)
	}
}

// A PERSON'S TYPED CHARACTERS SURVIVE.
//
// A chat signal's body is somebody's words, so it is raw unless its adapter
// declared otherwise. Somebody asking why `<details>` will not render in their
// docs must see their own characters reach the thread — and this is the test
// that fails if the parse is ever made unconditional "for consistency".
func TestChatSignalWithTagShapedTextIsVerbatim(t *testing.T) {
	// TWO channels, because the delivery rule is per DESTINATION: the surface
	// somebody typed on already displayed their message, so the card is owed to
	// the OTHER bound channel. Testing on the origin surface would assert on an
	// empty queue and pass for the wrong reason.
	mkChannel(t, "chan-verbatim", "tg-verbatim")
	mkChannel(t, "chan-mirror", "tg-mirror")
	mkChatSource(t, "src-verbatim", "chan-verbatim")
	mkProfile(t, "prof-verbatim")
	mkPipeline(t, "pipe-verbatim", []string{"src-verbatim"},
		[]string{"chan-verbatim", "chan-mirror"}, "prof-verbatim")
	reconcilePipeline(t, "pipe-verbatim")

	srv := apiServer()
	const typed = "why won't\n<details>\nrender in my docs?\n</details>"
	if rec := chatSignal(t, srv, "src-verbatim", "chan-verbatim", typed); rec.Code != 200 {
		t.Fatalf("chat signal: %d %s", rec.Code, rec.Body.String())
	}

	convs := convsBoundTo(t, "chan-verbatim")
	if len(convs) != 1 {
		t.Fatalf("want one conversation, got %d", len(convs))
	}
	// This one is created by the signal path, so nothing registered a cleanup
	// for it. Left behind it holds a capacity slot for the rest of the package
	// and the admission tests fail somewhere else entirely.
	t.Cleanup(func() { cleanupConversation(t, convs[0].Name) })

	// The mirror channel needs a THREAD before anything can be delivered to it,
	// which a real adapter supplies by completing ensure-topic.
	reconcileWithOps(t, srv, convs[0].Name)
	bindThread(t, srv, drainOps(t, srv, "tg-mirror"), "t-mirror")
	bindThread(t, srv, drainOps(t, srv, "tg-verbatim"), "t-verbatim")

	// The card the manager composes from it carries the characters as typed,
	// and no blocks — nothing folded anything a person wrote.
	reconcileWithOps(t, srv, convs[0].Name)
	var sawCard bool
	for _, op := range drainOps(t, srv, "tg-mirror") {
		if op.Kind != chat.OpSend || op.Message == nil {
			continue
		}
		if !strings.Contains(op.Message.Body, "<details>") {
			continue
		}
		sawCard = true
		for _, want := range []string{"<details>", "</details>", "why won't", "render in my docs?"} {
			if !strings.Contains(op.Message.Body, want) {
				t.Errorf("lost %q from what somebody typed: %q", want, op.Message.Body)
			}
		}
	}
	if !sawCard {
		t.Fatal("the typed message never reached the thread at all")
	}
}
