//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// Section 11 — coordinator mode (coordinator-deployment-mode) end to end.
//
// What belongs here, per docs/testing.md's tier split: rendering a
// Coordinator, an AgentCapability or the reaper from the chart is a
// RENDERING decision already pinned by the envtest permutation matrix
// (platform/manager/internal/integration/charttemplate_test.go). The
// coordinatorRef-resolution walk and the widened close bound are likewise
// already walked against a REAL API server by
// internal/integration/coordinator_owner_reach_test.go (envtest runs a real
// kube-apiserver, just no kubelet). What is left for THIS tier, and nothing
// below it can decide: starting a real pod for a real member conversation,
// a real cron-adapter pod firing a real admitted signal on its own schedule,
// and the live DEPLOYED manager's `/coordinate/*` surface reached over the
// network with a derived token — the exact path `platform/mcp-aops`
// forwards through in production, never an in-process handler call.
//
// Both lanes are FULL TIER: each starts at least two real conversation pods
// and waits on two dispatch cycles, well past the smoke gate's wall-clock
// budget.

// 11.1 Coordinator mode end to end: an admitted signal on the bundle's
// source opens a root conversation running the chart-rendered (here,
// hand-authored — see coordinator.go's doc comment) Coordinator's own
// capability, that root invokes a domain AgentCapability as a member, and
// the member's result reaches the root back as an ordinary input
// (member-result routing, coordination-loop) — never a side channel.
func TestCoordinatorModeInvokeAndMemberResultRouting(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const coordName = "e2e-coord-1"
	const domainCap = "e2e-coord-domain"
	const srcName = "e2e-coord-src"

	mustCreate(t, e.K, agentCapability(domainCap, ProfileStub))
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	co := coordinatorObj(coordName, ProfileStub, []string{srcName},
		[]agentopsv1alpha1.CoordinatorAgentEntry{capabilityEntry("domain", domainCap, "handles domain tasks")})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, coordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	// The source claim: an admitted signal opens a conversation naming the
	// Coordinator, exactly as a Pipeline's own claim does.
	fp := "e2e-coord-root-" + stamp
	e.PostTask(t, srcName, fp, "echo coord-root-"+stamp)
	root := e.ConversationFor(t, fp, 2*time.Minute)
	if root.Spec.CoordinatorRef == nil || root.Spec.CoordinatorRef.Name != coordName {
		t.Fatalf("the admitted signal must open a conversation claimed by the Coordinator, got %+v", root.Spec.CoordinatorRef)
	}
	e.WaitRun(t, root.Name, 1, 3*time.Minute)

	// The root — a Coordinator's own uncaused conversation — invokes the
	// domain capability as an ordinary member. A REAL pod backs it.
	member := e.invokeMember(t, coordName, root.Name, "domain", "echo domain-task-"+stamp)
	memberConv := e.waitForConversation(t, member, time.Minute)
	if memberConv.Spec.CausedBy == nil || memberConv.Spec.CausedBy.Parent != root.Name || memberConv.Spec.CausedBy.Entry != "domain" {
		t.Fatalf("the member must carry causedBy naming the root and the invoked entry, got %+v", memberConv.Spec.CausedBy)
	}
	memberConv = e.WaitRun(t, member, 1, 3*time.Minute)
	lastResult := memberConv.Status.Runs[len(memberConv.Status.Runs)-1].Result
	if lastResult != "[stub] domain-task-"+stamp {
		t.Fatalf("the member's own run must have answered its own task, got %q", lastResult)
	}

	// Member-result routing (coordination-loop): the member's result reaches
	// the ROOT as an ordinary routed input — a second run on the root, never
	// a side channel.
	root = e.WaitRun(t, root.Name, 2, 3*time.Minute)
	foundRouted := false
	for _, r := range root.Status.Runs {
		for _, in := range r.Inputs {
			if strings.Contains(in.Text, "domain-task-"+stamp) {
				foundRouted = true
			}
		}
	}
	if !foundRouted {
		t.Fatalf("the member's result must reach the root as an ordinary routed input, root runs: %+v", root.Status.Runs)
	}
}

// 11.2 + 11.3 The self-heal reaper's survey against a live cluster
// (coordinator-self-heal, coordinator-owner-reach): a real cron-adapter pod
// fires an hourly claim (scheduled every minute here, never waited out for
// real — the same substitution TestCronLane already makes for its own
// Pipeline-claimed cron source) and opens a root running the coordinating
// capability, which invokes the reaper as an ordinary member — real
// kubelet-backed pods on both sides. The reaper then lists its own
// Coordinator's open roots, and the ANCESTOR-ROOT EXCLUSION
// (coordinator-owner-reach: "the caller's own ancestor root is excluded") is
// asserted against REAL conversation objects the real controller built,
// reached through the LIVE deployed manager over the network — the one
// thing beyond what internal/integration/coordinator_owner_reach_test.go
// already pins against envtest's in-process handler call.
//
// The reaper itself re-invokes the incident's domain capability to re-check
// it ("SHALL invoke the SAME domain AgentCapability entry named in that
// root's members field") — an ordinary `causedBy` member invoking a sibling
// entry of the Coordinator it resolves to, never only the coordinating root.
// `handleCoordinateInvoke` / `chat.Router.InvokeMember` resolve the caller
// via the SAME `ResolveActingCoordinator` walk `callerActingForCoordinator`
// already uses for `close` and `list_open_roots` (coordinator-owner-reach,
// design D-A), so this reaches the manager exactly as `close` and
// `list_open_roots` do below.
func TestCoordinatorSelfHealSurveyExcludesAncestorRoot(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const coordName = "e2e-coord-heal"
	const reaperCap = "e2e-heal-reaper"
	const domainCap = "e2e-heal-domain"
	const cronSrc = "e2e-heal-cron"
	const incidentSrc = "e2e-heal-incident-src"

	mustCreate(t, e.K, agentCapability(reaperCap, ProfileStub))
	mustCreate(t, e.K, agentCapability(domainCap, ProfileStub))
	// The hourly cron claim (coordinator-self-heal, design D-E) — scheduled
	// every minute, never waited out for real.
	mustCreate(t, e.K, source(cronSrc, AdapterCron, map[string]any{
		"schedule": "* * * * *", "input": "echo cron-heal-tick-" + stamp, "title": "e2e-heal-cron-" + stamp,
	}))
	mustCreate(t, e.K, source(incidentSrc, "e2e", nil))
	co := coordinatorObj(coordName, ProfileStub, []string{cronSrc, incidentSrc}, []agentopsv1alpha1.CoordinatorAgentEntry{
		capabilityEntry("reaper", reaperCap, "surveys open roots and closes healed ones"),
		capabilityEntry("domain-agent", domainCap, "investigates and re-checks the domain incident"),
	})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, coordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	// An ordinary "incident" root on the SAME Coordinator — the open sibling
	// root the reaper's survey must find and, eventually, close.
	incidentFP := "e2e-heal-incident-" + stamp
	e.PostTask(t, incidentSrc, incidentFP, "echo incident-"+stamp)
	incident := e.ConversationFor(t, incidentFP, 2*time.Minute)
	if incident.Spec.CoordinatorRef == nil || incident.Spec.CoordinatorRef.Name != coordName {
		t.Fatalf("the incident must be claimed by the Coordinator, got %+v", incident.Spec.CoordinatorRef)
	}
	e.WaitRun(t, incident.Name, 1, 3*time.Minute)
	// The incident's own original investigation: an ordinary invoke from a
	// real root (allowed today), populating `members` with "domain-agent"
	// for the reaper's survey to read back
	// (coordinator-owner-reach: list_open_roots' `members` projection).
	investigator := e.invokeMember(t, coordName, incident.Name, "domain-agent", "echo investigating-"+stamp)
	e.WaitRun(t, investigator, 1, 3*time.Minute)

	// The REAL cron adapter fires its real schedule and admits a signal the
	// Coordinator claims — a genuinely kubelet-backed root, not a posted
	// stand-in.
	cronRoot := waitForCronRoot(ctx, t, e, cronSrc)
	if cronRoot.Spec.CoordinatorRef == nil || cronRoot.Spec.CoordinatorRef.Name != coordName {
		t.Fatalf("the cron-triggered root must be claimed by the Coordinator, got %+v", cronRoot.Spec.CoordinatorRef)
	}
	e.WaitRun(t, cronRoot.Name, 1, 3*time.Minute)

	// Design D-E: the coordinating agent recognises the cron signal and
	// invokes the reaper. A real member, a real pod.
	reaperName := e.invokeMember(t, coordName, cronRoot.Name, "reaper", "echo survey-"+stamp)
	e.WaitRun(t, reaperName, 1, 3*time.Minute)

	// The reaper's own survey: list_open_roots over its OWN
	// coordinator:<name>:<reaper> token, resolved by walking causedBy to the
	// cron-triggered root exactly as design D-A specifies — against REAL
	// conversation objects the controller built, over the LIVE deployed
	// manager reached through its Service.
	reaperToken := coordinatorToken(e, coordName, reaperName)
	code, out := e.coordinateCall(t, "/coordinate/open-roots", reaperToken, map[string]any{"conversation": reaperName})
	if code != 200 {
		t.Fatalf("list_open_roots: %d %v", code, out)
	}
	assertOpenRootsSurvey(t, out, incident.Name, cronRoot.Name)

	// The reaper re-invokes the incident's own domain-agent entry to re-check
	// it, as a plain member of the cron-triggered root — scripted here to
	// report the condition cleared.
	healCode, healOut := e.coordinateCall(t, "/coordinate/invoke", reaperToken,
		map[string]any{"conversation": reaperName, "agent": "domain-agent", "task": "echo healed-" + stamp})
	if healCode != 200 {
		t.Fatalf("reaper re-check invoke refused (%d %v)", healCode, healOut)
	}
	healedMember, _ := healOut["member"].(string)
	e.WaitRun(t, healedMember, 1, 3*time.Minute)

	// Once the re-check reports healed, the reaper closes the incident root
	// through the WIDENED close bound (coordinator-owner-reach design D-B) —
	// never the ordinary bound, since the incident is a sibling the reaper
	// did not directly cause.
	closeCode, closeOut := e.coordinateCall(t, "/coordinate/close", reaperToken,
		map[string]any{"conversation": reaperName, "target": incident.Name, "reason": "resolved by the reaper's re-check"})
	if closeCode != 200 {
		t.Fatalf("reaper close of the healed incident root: %d %v", closeCode, closeOut)
	}
	waitFor(t, "the incident root to close", time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, incident.Name)
		if err != nil {
			return false, err
		}
		return c.Status.Phase == agentopsv1alpha1.ConversationClosed, nil
	})

	// And the reaper must never be able to close its OWN ancestor root
	// (coordinator-owner-reach: "the reaper cannot end its own existence by
	// closing the root that invoked it").
	ownCode, ownOut := e.coordinateCall(t, "/coordinate/close", reaperToken,
		map[string]any{"conversation": reaperName, "target": cronRoot.Name, "reason": "trying to close my own root"})
	if ownCode != 403 {
		t.Fatalf("want 403 closing the reaper's own ancestor root, got %d %v", ownCode, ownOut)
	}
}

// 11.4 The console's close/delete cascade against a REAL coordinator tree
// (conversation-close, console-conversation-tree): closing the root through
// the console's bulk-close surface cascades to a LIVE member — real
// reconciler-driven pod teardown, which is exactly the kind of thing
// envtest's fake-less-kubelet world cannot observe (docs/testing.md gives the
// kubelet to this tier alone) — and deleting the Closed root through the same
// surface removes the member too, never orphaning it.
func TestCoordinatorCloseAndDeleteCascadeThroughConsole(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const coordName = "e2e-coord-cascade"
	const domainCap = "e2e-coord-cascade-domain"
	const srcName = "e2e-coord-cascade-src"

	mustCreate(t, e.K, agentCapability(domainCap, ProfileStub))
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	co := coordinatorObjWithChannels(coordName, ProfileStub, []string{srcName}, []string{ChannelConsole},
		[]agentopsv1alpha1.CoordinatorAgentEntry{capabilityEntry("domain", domainCap, "handles domain tasks")})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, coordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-coord-cascade-root-" + stamp
	e.PostTask(t, srcName, fp, "echo cascade-root-"+stamp)
	root := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, root.Name, 1, 3*time.Minute)
	waitFor(t, "the root's console thread bound (coordinator-unconditional-channels)", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, root.Name)
		return err == nil && len(c.Status.Threads) >= 1, err
	})

	// A LIVE member — a real pod, still present (idle, not yet evicted) when
	// the cascade below reaches it.
	member := e.invokeMember(t, coordName, root.Name, "domain", "echo cascade-member-"+stamp)
	e.WaitRun(t, member, 1, 3*time.Minute)

	memberPodName := types.NamespacedName{Namespace: e.K.Namespace, Name: runtimepod.PodName(member)}
	waitFor(t, "the member's runtime pod to exist", time.Minute, func() (bool, error) {
		var pod corev1.Pod
		return e.K.Get(ctx, memberPodName, &pod) == nil, nil
	})

	// Close the ROOT ALONE through the console's bulk-close surface — never
	// the member — and let the cascade find it.
	closeBody, _ := json.Marshal(map[string]any{"names": []string{root.Name}})
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/close", closeBody, "Bearer "+e.Values.UIToken); code/100 != 2 || !strings.Contains(out, `"closed":1`) {
		t.Fatalf("bulk close the root: %d %s", code, out)
	}

	waitFor(t, "the root closed", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, root.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})
	waitFor(t, "the cascade closes the live member too", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, member)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})
	// The REAL, reconciler-driven teardown: the member's runtime pod is gone,
	// not merely its phase flipped.
	waitFor(t, "the member's runtime pod torn down by the real reconciler/kubelet", 2*time.Minute, func() (bool, error) {
		var pod corev1.Pod
		err := e.K.Get(ctx, memberPodName, &pod)
		return apierrors.IsNotFound(err), nil
	})

	// Delete cascades too: deleting the now-Closed root through the SAME
	// console surface removes the member as well.
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/delete", closeBody, "Bearer "+e.Values.UIToken); code/100 != 2 || !strings.Contains(out, `"deleted":1`) {
		t.Fatalf("bulk delete the root: %d %s", code, out)
	}
	waitFor(t, "the root object gone", 3*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, root.Name)
		return err != nil, nil
	})
	waitFor(t, "the cascade-deleted member gone too — never left orphaned", 3*time.Minute, func() (bool, error) {
		_, err := e.K.Conversation(ctx, member)
		return err != nil, nil
	})
}

// waitForCronRoot waits for the conversation the cron source's signal opened.
func waitForCronRoot(ctx context.Context, t *testing.T, e *Env, cronSrc string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var cronRoot *agentopsv1alpha1.Conversation
	waitFor(t, "a conversation from the self-heal cron source", 4*time.Minute, func() (bool, error) {
		items, err := e.K.Conversations(ctx)
		if err != nil {
			return false, err
		}
		for i := range items {
			c := &items[i]
			if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == cronSrc {
				cronRoot = c
				return true, nil
			}
		}
		return false, nil
	})
	return cronRoot
}

// assertOpenRootsSurvey checks a list_open_roots answer: it names the incident
// sibling with its domain-agent member and excludes the caller's ancestor root.
func assertOpenRootsSurvey(t *testing.T, out map[string]any, incident, ancestor string) {
	t.Helper()
	roots, _ := out["roots"].([]any)
	var sawIncident, sawOwnAncestor bool
	var incidentMembers []string
	for _, r := range roots {
		root, _ := r.(map[string]any)
		switch name, _ := root["name"].(string); name {
		case incident:
			sawIncident = true
			incidentMembers = stringMembers(root["members"])
		case ancestor:
			sawOwnAncestor = true
		}
	}
	if !sawIncident {
		t.Fatalf("list_open_roots must return the open incident sibling root, got %v", roots)
	}
	if sawOwnAncestor {
		t.Fatalf("list_open_roots must EXCLUDE the reaper's own ancestor root (the cron-triggered root), got %v", roots)
	}
	if len(incidentMembers) == 0 || incidentMembers[0] != "domain-agent" {
		t.Fatalf("the incident root's members projection must name its domain-agent entry, got %v", incidentMembers)
	}
}

// stringMembers reads a decoded JSON array of strings, skipping anything else.
func stringMembers(v any) []string {
	var out []string
	ms, _ := v.([]any)
	for _, m := range ms {
		if s, ok := m.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
