package chat

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// /exit is the second command the REPLY path handles, and it is one word from
// the one that archives a thread. So the tests here are as much about the
// BOUNDARY as the behaviour: what it releases, what it must leave standing, and
// when it refuses rather than acts.

func runtimePod(conv string) *corev1.Pod {
	p := &corev1.Pod{}
	p.Name, p.Namespace = runtimepod.PodName(conv), testNS
	p.Status.Phase = corev1.PodRunning
	return p
}

func podGone(t *testing.T, r *Router, conv string) bool {
	t.Helper()
	var pod corev1.Pod
	err := r.Client.Get(context.Background(), types.NamespacedName{
		Namespace: testNS, Name: runtimepod.PodName(conv)}, &pod)
	return err != nil
}

func exit(t *testing.T, r *Router, ch string) {
	t.Helper()
	thread := "thread-" + ch
	if err := r.HandleMessage(context.Background(), nsChannel(ch, "slack"),
		InboundMessage{ThreadID: &thread, Text: "/exit"}); err != nil {
		t.Fatal(err)
	}
}

// The whole point: the pod goes, everything else stays. A conversation that
// lost its threads or its inputs to /exit would be /close wearing another name.
func TestExitReleasesTheRuntimeAndKeepsTheConversation(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "in-1", Payload: "done"}}
	conv.Status.ProcessedInputIDs = []string{"in-1"}
	conv.Status.RuntimeContextID = "ctx-abc"
	r, q, c := closeFixture(t, nsChannel("c1", "slack"), conv, runtimePod("conv-1"))

	exit(t, r, "c1")

	if !podGone(t, r, "conv-1") {
		t.Fatal("the runtime pod must be deleted")
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatalf("/exit must NOT delete the conversation: %v", err)
	}
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("/exit must not close the conversation — that is /close")
	}
	if got.Status.ClosedAt != nil {
		t.Fatal("/exit must not stamp closedAt")
	}
	if len(got.Status.Threads) != 1 {
		t.Fatalf("thread bindings must survive: %+v", got.Status.Threads)
	}
	if got.ContextID() != "ctx-abc" {
		t.Fatalf("the context handle must survive: %q", got.ContextID())
	}
	if len(got.Spec.Inputs) != 1 {
		t.Fatalf("inputs must survive: %+v", got.Spec.Inputs)
	}
	// the command itself never becomes one
	for _, in := range got.Spec.Inputs {
		if strings.Contains(in.Payload, "/exit") {
			t.Fatalf("the command must not become an input: %+v", got.Spec.Inputs)
		}
	}
	ops := drain(q, "slack")
	if len(ops) != 1 {
		t.Fatalf("want one reply, got %d", len(ops))
	}
	// Every success reply says what SURVIVED, because the risk this command
	// carries is being mistaken for /close.
	body := opBody(ops[0])
	if !strings.Contains(body, "stay open") {
		t.Fatalf("the reply must say the conversation and thread remain open: %q", body)
	}
}

// Mid-run release is the one outcome that must never happen: the replacement pod
// is created at once, gets nothing from /work, idles out the LONG TTL, is reaped
// as Succeeded — which clears Inflight and re-dispatches work that may already
// have acted.
func TestExitRefusesMidRunAndNamesTheAlternative(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "run-7"}
	r, q, c := closeFixture(t, nsChannel("c1", "slack"), conv, runtimePod("conv-1"))

	exit(t, r, "c1")

	if podGone(t, r, "conv-1") {
		t.Fatal("a pod must not be deleted while a run is in flight")
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Inflight == nil {
		t.Fatal("the inflight run must be untouched — clearing it is what repeats the work")
	}
	ops := drain(q, "slack")
	if len(ops) != 1 {
		t.Fatalf("want one refusal, got %d", len(ops))
	}
	body := opBody(ops[0])
	if !strings.Contains(body, "run-7") {
		t.Fatalf("the refusal must name the run: %q", body)
	}
	if !strings.Contains(body, "/close") {
		t.Fatalf("the refusal must offer /close for abandoning it: %q", body)
	}
}

// Not dangerous, merely pointless — and a command that appears to work while
// changing nothing is worse than one that explains itself.
func TestExitRefusesWhileWorkIsQueued(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "in-1", Payload: "check the disks"}}
	r, q, _ := closeFixture(t, nsChannel("c1", "slack"), conv, runtimePod("conv-1"))

	exit(t, r, "c1")

	if podGone(t, r, "conv-1") {
		t.Fatal("the pod must survive: it would be recreated immediately anyway")
	}
	ops := drain(q, "slack")
	if len(ops) != 1 || !strings.Contains(opBody(ops[0]), "queued work") {
		t.Fatalf("the refusal must say why nothing was freed: %+v", ops)
	}
}

// The desired state already holds, so this is not an error — but it must not be
// silent either: "released it" and "there was nothing running" are different
// answers to the same command.
func TestExitWithNoPodReportsNothingToRelease(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	r, q, c := closeFixture(t, nsChannel("c1", "slack"), conv)

	exit(t, r, "c1")

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatalf("conversation must survive: %v", err)
	}
	ops := drain(q, "slack")
	if len(ops) != 1 || !strings.Contains(opBody(ops[0]), "Nothing to release") {
		t.Fatalf("want a nothing-to-release reply, got %+v", ops)
	}
}

// Continuity is computed, never guessed: the same call the dispatch path uses to
// decide whether to hand back a context handle at all.
func TestExitReplyStatesWhatHappensToTheContext(t *testing.T) {
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Name, profile.Namespace = "p1", testNS

	for _, tc := range []struct {
		name       string
		contextPVC string
		want       string
		absent     string
	}{
		{"continuity survives", "agentops-context", "picks up where this left off", "starts fresh"},
		{"continuity does not survive", "", "starts fresh", "picks up where this left off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := boundConv("conv-1", "c1")
			conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "p1"}
			r, q, _ := closeFixture(t, nsChannel("c1", "slack"), profile, conv, runtimePod("conv-1"))
			// No AgentRuntime CR: the bootstrap fallback, whose continuity turns on
			// whether this deployment provides a context volume at all.
			r.Runtime = runtimepod.Config{ContextPVC: tc.contextPVC}

			exit(t, r, "c1")

			ops := drain(q, "slack")
			if len(ops) != 1 {
				t.Fatalf("want one reply, got %d", len(ops))
			}
			body := opBody(ops[0])
			if !strings.Contains(body, tc.want) {
				t.Fatalf("reply must say %q: %s", tc.want, body)
			}
			if strings.Contains(body, tc.absent) {
				t.Fatalf("reply must NOT say %q: %s", tc.absent, body)
			}
		})
	}
}

// A profile that cannot be read must not cost the report: the pod is already
// gone, so refusing to say so because a Get failed is the worst of both.
func TestExitReportsTheReleaseEvenWhenContinuityCannotBeResolved(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "missing"}
	r, q, _ := closeFixture(t, nsChannel("c1", "slack"), conv, runtimePod("conv-1"))

	exit(t, r, "c1")

	if !podGone(t, r, "conv-1") {
		t.Fatal("the pod must still be released")
	}
	ops := drain(q, "slack")
	if len(ops) != 1 || !strings.Contains(opBody(ops[0]), "Runtime released") {
		t.Fatalf("the release must still be reported: %+v", ops)
	}
}

func TestExitOnGeneralSurfaceAnswersWithUsage(t *testing.T) {
	r, q, c := closeFixture(t, nsChannel("c1", "slack"))
	cmd, ok := addressing.Parse("/exit")
	if !ok {
		t.Fatal("parse")
	}
	if err := r.HandleCommand(context.Background(), nsChannel("c1", "slack"), cmd, "", ""); err != nil {
		t.Fatal(err)
	}
	ops := drain(q, "slack")
	if len(ops) != 1 || ops[0].ThreadID != nil {
		t.Fatalf("want one general-surface reply, got %+v", ops)
	}
	body := opBody(ops[0])
	if !strings.Contains(body, "/exit") || strings.Contains(body, "Unknown agent") {
		t.Fatalf("must explain usage rather than report an unknown pipeline: %q", body)
	}
	var list agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("general-surface /exit created %d conversation(s)", len(list.Items))
	}
}

// Both are listed together because guessing between them costs a thread.
func TestAgentsListingNamesBothThreadCommands(t *testing.T) {
	r, q, _ := closeFixture(t, nsChannel("c1", "slack"))
	cmd, ok := addressing.Parse("/agents")
	if !ok {
		t.Fatal("parse")
	}
	if err := r.HandleCommand(context.Background(), nsChannel("c1", "slack"), cmd, "", ""); err != nil {
		t.Fatal(err)
	}
	ops := drain(q, "slack")
	if len(ops) != 1 {
		t.Fatalf("want one listing, got %d", len(ops))
	}
	body := opBody(ops[0])
	for _, needle := range []string{"/exit", "/close", "releases", "archives"} {
		if !strings.Contains(body, needle) {
			t.Errorf("the listing must mention %q: %s", needle, body)
		}
	}
}

func TestIsExitCommand(t *testing.T) {
	for text, want := range map[string]bool{
		"/exit":                              true,
		"/exit@AgentOpsBot":                  true,
		"/exit ":                             true,
		"/exits":                             false,
		"/exit:agent":                        false,
		"/exit the maintenance window":       false,
		"exit":                               false,
		"please /exit":                       false,
		"/exit once the rollout has drained": false,
	} {
		if got := isExitCommand(strings.TrimSpace(text)); got != want {
			t.Errorf("isExitCommand(%q) = %v, want %v", text, got, want)
		}
	}
}

// Trailing text is an instruction for the agent, so it takes the ordinary reply
// path and releases nothing.
func TestExitWithTrailingTextReachesTheAgent(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	r, _, c := closeFixture(t, nsChannel("c1", "slack"), conv, runtimePod("conv-1"))

	thread := "thread-c1"
	if err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
		InboundMessage{ThreadID: &thread, Text: "/exit the node from the cluster once it drains"}); err != nil {
		t.Fatal(err)
	}
	if podGone(t, r, "conv-1") {
		t.Fatal("trailing text is not the command — no pod may be released")
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Inputs) != 1 || got.Spec.Inputs[0].Type != agentopsv1alpha1.InputReply {
		t.Fatalf("it must reach the agent as an ordinary reply: %+v", got.Spec.Inputs)
	}
}
