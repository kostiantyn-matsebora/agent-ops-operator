//go:build system

// Section — the members pool against a REAL cluster (separate-member-slot-pool,
// task 5.1). What envtest's nestedcoordinator_test.go and memberpool_test.go
// already pin — the classifier, the per-pool FIFO/eviction scoping, and a
// nested-Coordinator chain deeper than the members ceiling completing — is a
// RECONCILER decision, already proven against a real API server with no
// kubelet. What is left for THIS tier, and nothing below it can decide: a
// real kubelet actually scheduling and starting every one of those pods,
// across real wall-clock time, on the live DEPLOYED manager's own
// `/coordinate/*` surface — the same substrate split
// coordinator_test.go's own doc comment (section 11) already draws.
package system

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// coordinatorRefEntry is one agents[] entry invoking ANOTHER Coordinator as a
// member — coordinator.go's capabilityEntry's nesting sibling, needed only
// here: every other lane in this package invokes an ordinary AgentCapability
// leaf.
func coordinatorRefEntry(name, nextCoordinator, description string) agentopsv1alpha1.CoordinatorAgentEntry {
	return agentopsv1alpha1.CoordinatorAgentEntry{
		Name:           name,
		CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: nextCoordinator},
		Description:    description,
	}
}

// memberPodReady waits for the given conversation's own runtime pod to exist
// in the conversations namespace — "admitted", in this tier's terms, is a
// real kubelet-scheduled pod object, never merely a phase. Mirrors
// coordinator_test.go's own inline wait for a member's pod (11.4).
func memberPodReady(t *testing.T, ctx context.Context, e *Env, conv string, timeout time.Duration) {
	t.Helper()
	podName := types.NamespacedName{Namespace: ConversationsNamespace, Name: runtimepod.PodName(conv)}
	waitFor(t, "runtime pod for "+conv+" to exist", timeout, func() (bool, error) {
		var pod corev1.Pod
		return e.K.Get(ctx, podName, &pod) == nil, nil
	})
}

// memberPodNeverAppears asserts the given conversation's pod does NOT appear
// within the window — the real-cluster shape of memberpool_test.go's
// `podExists(t, name)` must be false assertions: a Pending member must have
// no runtime pod, and that must stay true for a real, bounded wall-clock
// window, not just the instant checked.
func memberPodNeverAppears(t *testing.T, ctx context.Context, e *Env, conv string, window time.Duration) {
	t.Helper()
	podName := types.NamespacedName{Namespace: ConversationsNamespace, Name: runtimepod.PodName(conv)}
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		var pod corev1.Pod
		if e.K.Get(ctx, podName, &pod) == nil {
			t.Fatalf("a member over the members ceiling must get no runtime pod, found %s", podName.Name)
		}
		time.Sleep(2 * time.Second)
	}
}

// mkChainCoordinators creates the depth nested Coordinators. Level 1 alone
// claims a source: every deeper level's root is created by its parent's invoke.
func mkChainCoordinators(t *testing.T, ctx context.Context, e *Env, stamp string, depth int, coordName func(int) string, leafCapName string) {
	t.Helper()
	for i := 1; i <= depth; i++ {
		var entry agentopsv1alpha1.CoordinatorAgentEntry
		if i < depth {
			entry = coordinatorRefEntry("child", coordName(i+1), "the next level of the chain")
		} else {
			entry = capabilityEntry("child", leafCapName, "the leaf of the chain")
		}
		var sources []string
		if i == 1 {
			srcName := "e2e-chain-src-" + stamp
			mustCreate(t, e.K, source(srcName, "e2e", nil))
			sources = []string{srcName}
		}
		mustCreate(t, e.K, coordinatorObj(coordName(i), ProfileStub, sources, []agentopsv1alpha1.CoordinatorAgentEntry{entry}))
		if err := waitCoordinatorReady(ctx, e.K, coordName(i), 2*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
}

// chainWalk groups walkChain's chain-shaped inputs.
type chainWalk struct {
	stamp, rootName string
	depth, hold     int
	coordName       func(int) string
}

// walkChain invokes each level from its parent once the parent's run is
// inflight (the real "busy parent" window, design D-7) and waits for the
// level's pod. It returns the member conversations in level order.
func walkChain(t *testing.T, ctx context.Context, e *Env, w chainWalk) []string {
	t.Helper()
	stamp, rootName, depth, hold, coordName := w.stamp, w.rootName, w.depth, w.hold, w.coordName
	callerName := rootName
	var members []string
	for i := 1; i <= depth; i++ {
		waitFor(t, fmt.Sprintf("caller %s inflight before invoking level %d", callerName, i+1), 2*time.Minute, func() (bool, error) {
			c, err := e.K.Conversation(ctx, callerName)
			if err != nil {
				return false, err
			}
			return c.Status.Inflight != nil, nil
		})
		task := "echo leaf-done-" + stamp
		if i < depth {
			task = fmt.Sprintf("hold %d level-%d-"+stamp, hold, i+1)
		}
		member := e.invokeMember(t, coordName(i), callerName, "child", task)
		memberPodReady(t, ctx, e, member, 3*time.Minute)
		members = append(members, member)
		callerName = member
	}
	return members
}

// 5.1a — a chain of nested Coordinator members one level deeper than the
// members ceiling (chart default maxActiveMembers: 5, chart/values.yaml)
// still completes end to end, against a REAL kubelet: no level is ever left
// permanently Pending, driven entirely through the real /coordinate/invoke
// surface — mirroring
// internal/integration/nestedcoordinator_test.go's
// TestNestedCoordinatorChainLongerThanTheMembersCeilingCompletes, but with a
// real pod backing every level instead of envtest's reconcile-by-hand.
//
// Each intermediate level's own task is `hold N`, giving it a real inflight
// window (design D-7: "the parent is BUSY when it invokes its own child") in
// which to invoke the next level before its own hold elapses and the stub
// reports the run done — the real-cluster analogue of
// markMemberDispatched/reportMemberRunDone's manual bookkeeping. The leaf
// (depth) entry is an ordinary AgentCapability answering `echo`.
func TestNestedCoordinatorChainLongerThanTheMembersCeilingCompletes(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const depth = 6      // one deeper than...
	const membersCap = 5 // ...the chart's own default, maxActiveMembers.
	const hold = 20      // seconds a non-leaf level's own run stays inflight —
	// ample margin for this cluster's reconcile cadence to admit the next
	// level's pod before the window closes.

	coordName := func(i int) string { return fmt.Sprintf("e2e-chain-co-%s-%d", stamp, i) }
	const leafCap = "e2e-chain-leaf-"
	leafCapName := leafCap + stamp
	mustCreate(t, e.K, agentCapability(leafCapName, ProfileStub))

	mkChainCoordinators(t, ctx, e, stamp, depth, coordName, leafCapName)

	// Level 1's own root, opened by an ordinary admitted signal — holding long
	// enough to invoke level 2 within its own window.
	fp := "e2e-chain-root-" + stamp
	e.PostTask(t, "e2e-chain-src-"+stamp, fp, fmt.Sprintf("hold %d level-1-"+stamp, hold))
	root := e.ConversationFor(t, fp, 2*time.Minute)
	if root.Spec.CoordinatorRef == nil || root.Spec.CoordinatorRef.Name != coordName(1) {
		t.Fatalf("the admitted signal must open a root claimed by level 1's Coordinator, got %+v", root.Spec.CoordinatorRef)
	}

	members := walkChain(t, ctx, e, chainWalk{stamp, root.Name, depth, hold, coordName})

	// The leaf never invokes further, so it is the chain's own last run — wait
	// it out so the chain is fully quiescent before the ceiling check below.
	e.WaitRun(t, members[len(members)-1], 1, 3*time.Minute)

	// The ceiling itself (design D-1/D-2): at no point does this chain hold
	// more real pods, among its own members, than the members pool's own
	// cap — unlike envtest's reconcile-by-hand, a live cluster's reconcile
	// and idle-eviction run on their own clock, so this checks the INVARIANT
	// (never exceeded) rather than pinning an exact count at an exact
	// instant.
	alive := 0
	for _, m := range members {
		var pod corev1.Pod
		if e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: runtimepod.PodName(m)}, &pod) == nil {
			alive++
		}
	}
	if alive > membersCap {
		t.Fatalf("a chain of %d levels must never hold more than the members cap of %d pods at once, got %d",
			depth, membersCap, alive)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_ = e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: root.Name}})
		for _, m := range members {
			_ = e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: m}})
		}
	})
}

// 5.1b — a Coordinator root invoking members beyond the members ceiling,
// alongside a FULL conversations pool (chart default maxActiveConversations:
// 5), admits members up to the members ceiling and no further — proving
// design D-1/D-2's pool independence against a real kubelet, not only
// envtest's fake one. Mirrors
// internal/integration/memberpool_test.go's
// TestFullConversationsPoolStillAdmitsAMember and
// TestSixthMemberPendsAtTheMembersCeiling, combined into the one scenario
// this tier can afford (each lane here starts several real pods and waits
// out real scheduling).
func TestFullConversationsPoolStillAdmitsMembersUpToTheirOwnCeiling(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())

	const conversationsCap = 5 // the chart default, maxActiveConversations
	const membersCap = 5       // the chart default, maxActiveMembers

	// A clean admission pool: the cron lane paused, every leftover
	// conversation gone, no runtime pod left over from an earlier lane in
	// this same shared cluster — clearAdmissionPoolForReplicaLane's own
	// reasoning (replicas_test.go) applies here verbatim: this lane's own
	// cap-sized burst cannot be trusted against whatever earlier lanes left
	// behind.
	clearAdmissionPoolForReplicaLane(t, ctx, e)

	// Saturate the CONVERSATIONS pool with stalling, plain (uncaused)
	// conversations — never reporting, so each holds its slot for the whole
	// test. Exactly TestAdmissionFIFOOnPodDelete's own burst shape.
	var convNames []string
	for i := 0; i < conversationsCap; i++ {
		fp := fmt.Sprintf("e2e-memberpool-conv-%s-%d", stamp, i)
		e.PostTask(t, SourceTasks, fp, "stall")
		c := e.ConversationFor(t, fp, time.Minute)
		convNames = append(convNames, c.Name)
	}
	waitFor(t, fmt.Sprintf("the conversations pool's own cap of %d held by stalling conversations", conversationsCap),
		4*time.Minute, func() (bool, error) {
			pods, err := e.K.RuntimePods(ctx, "agentops.dev/conversation")
			if err != nil {
				return false, err
			}
			held := 0
			for _, p := range pods {
				if indexOf(convNames, p.Labels["agentops.dev/conversation"]) >= 0 {
					held++
				}
			}
			return held >= conversationsCap, nil
		})

	t.Cleanup(func() {
		ctx := context.Background()
		for _, n := range convNames {
			_ = e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: n}})
		}
	})

	// A Coordinator root with ONE entry, invoked membersCap+1 times — the
	// members pool's own, entirely separate ceiling.
	actualCoordName := "e2e-memberpool-coord-" + stamp
	domainCap := "e2e-memberpool-domain-" + stamp
	srcName := "e2e-memberpool-src-" + stamp
	mustCreate(t, e.K, agentCapability(domainCap, ProfileStub))
	mustCreate(t, e.K, source(srcName, "e2e", nil))
	co := coordinatorObj(actualCoordName, ProfileStub, []string{srcName},
		[]agentopsv1alpha1.CoordinatorAgentEntry{capabilityEntry("domain", domainCap, "handles domain tasks")})
	mustCreate(t, e.K, co)
	if err := waitCoordinatorReady(ctx, e.K, actualCoordName, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-memberpool-root-" + stamp
	e.PostTask(t, srcName, fp, "echo memberpool-root-"+stamp)
	root := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, root.Name, 1, 3*time.Minute)

	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: root.Name}})
	})

	// Exactly membersCap members, each invoked while the conversations pool
	// above stays saturated — every one must still be admitted a REAL pod,
	// proving the full conversations pool never blocks a members-pool
	// admission.
	var members []string
	for i := 0; i < membersCap; i++ {
		member := e.invokeMember(t, actualCoordName, root.Name, "domain", fmt.Sprintf("hold 30 member-%d-%s", i, stamp))
		memberPodReady(t, ctx, e, member, 3*time.Minute)
		members = append(members, member)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, m := range members {
			_ = e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: m}})
		}
	})

	// A (membersCap+1)th member, against a members pool now full of BUSY
	// (holding) members, and a members pool whose own ceiling is entirely
	// unaffected by the still-full conversations pool: it must pend, and
	// stay without a pod for a real, bounded window — never promoted by
	// whatever is happening in the other pool.
	sixth := e.invokeMember(t, actualCoordName, root.Name, "domain", "echo memberpool-sixth-"+stamp)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: sixth}})
	})
	memberPodNeverAppears(t, ctx, e, sixth, 45*time.Second)
}
