//go:build system

package system

// split-conversation-namespace tasks.md section 10 (10.1-10.4) — what only a
// real cluster decides about the split: where the kubelet actually starts a
// runtime pod, what the live authorizer actually grants the manager's own
// ServiceAccount in each namespace, and what a real CNI actually does with
// the rendered NetworkPolicy. Nothing here is exercised by envtest or the
// chart's own render tests (internal/integration/charttemplate_*_test.go),
// which run no kubelet, no scheduler and no live authorizer.
//
// The pack's install is UNCHANGED for this file: install.go's own
// valuesYAML sets neither `global.agentops.conversationNamespace` nor
// `createConversationNamespace`, so TestMain's install already renders the
// chart's own default layout — SPLIT (design.md decision 9, chart/values.yaml
// `conversationNamespace: agent-ops-conversations`,
// `createConversationNamespace: true`). "A split release" (10.1's own
// wording) is therefore simply what TestMain already stood up, with no
// `--set` of this file's own.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)


// 10.1 The most basic end-to-end proof the split actually works: an
// ordinary signal opens a conversation whose runtime pod lands in the
// CONVERSATIONS namespace (never the release namespace), runs as an
// account the chart rendered in THAT namespace, mounts its context claim
// there, and still reaches the manager across the split and answers —
// spec.md's own "A dispatched conversation's workload lands in the
// conversations namespace" scenario.
func TestSplitConversationPodRunsInConversationsNamespace(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	fp := "e2e-split-basic-" + stamp

	e.PostTask(t, SourceTasks, fp, "echo split-ns "+stamp)
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	run := conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "succeeded" || !strings.Contains(run.Result, "[stub] split-ns "+stamp) {
		t.Fatalf("the split layout must not stop an ordinary run from reaching the manager and answering: %+v", run)
	}

	if conv.Status.RuntimePod == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}
	var pod corev1.Pod
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: conv.Status.RuntimePod}, &pod); err != nil {
		t.Fatalf("the runtime pod %s must be found in the conversations namespace %q: %v", conv.Status.RuntimePod, ConversationsNamespace, err)
	}

	// The placement is EXCLUSIVE, not a copy: the same name must not also
	// resolve in the release namespace.
	var inReleaseNamespace corev1.Pod
	err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: conv.Status.RuntimePod}, &inReleaseNamespace)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("the runtime pod must not also exist in the release namespace %q: err=%v", Namespace, err)
	}

	// It mounts its context claim, in the SAME namespace as the pod
	// (design.md decision 1: "chart claims ... manager claims | conversations
	// | mounted").
	assertPodMountsClaim(t, &pod, "agentops-context")

	// It runs as an account the chart actually rendered in the
	// conversations namespace — never a bare, unnamed "default".
	sa := pod.Spec.ServiceAccountName
	if sa == "" || sa == "default" {
		t.Fatalf("the pod must run as a named conversations-namespace account, got %q", sa)
	}
	var account corev1.ServiceAccount
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: sa}, &account); err != nil {
		t.Fatalf("the account %q the pod runs as must exist in the conversations namespace: %v", sa, err)
	}
}

// assertPodMountsClaim fails unless one of the pod's volumes names claim.
func assertPodMountsClaim(t *testing.T, pod *corev1.Pod, claim string) {
	t.Helper()
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == claim {
			return
		}
	}
	t.Fatalf("the pod must mount the claim %q: %+v", claim, pod.Spec.Volumes)
}

// 10.2 The same lane with NetworkPolicies enforced
// (`global.agentops.networkPolicy.enabled=true`, off by default): every
// wired flow still passes split (spec.md's "Every wired flow survives the
// split"), and a runtime pod in the conversations namespace accepts NO
// ingress at all (spec.md's "Runtime pods accept no ingress") — not even
// from a pod in the CONTROL namespace, which is exactly the cross-namespace
// case this change's own NetworkPolicy peers (design.md decision 6) exist
// to cover.
//
// Invasive to the shared install (rolls the manager, same as
// assertCommandBusNetworkPolicyEnforced, async_system_test.go, whose own
// doc comment this one leans on for why a chart-render test
// (internal/integration/charttemplate_networkpolicy_test.go) cannot stand
// in for a real CNI's enforcement) — FULL TIER ONLY, cleanup registered
// BEFORE the risky upgrade (gotchas.md).
func TestSplitNetworkPolicyEnforcedRuntimePodAcceptsNoIngress(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()

	t.Cleanup(func() {
		if err := e.UpgradeManager(context.Background()); err != nil {
			t.Errorf("restoring the install with the NetworkPolicy back off: %v", err)
		}
	})
	if err := e.UpgradeManager(ctx, "global.agentops.networkPolicy.enabled=true"); err != nil {
		t.Fatalf("enabling the NetworkPolicy: %v", err)
	}

	var pod corev1.Pod
	t.Run("every wired flow survives the split with policies enforced", func(t *testing.T) {
		assertSplitNetpolFlowPasses(t, ctx, e, &pod)
	})
	t.Run("a runtime pod accepts no ingress", func(t *testing.T) {
		assertRuntimePodAcceptsNoIngress(t, ctx, e, &pod)
	})

	// The manager itself must stay admitted through its own rendered rule —
	// the one assertion left for its own side (distroless, no shell to exec
	// a probe from, same as assertCommandBusNetworkPolicyEnforced's own).
	if err := e.K.WaitDeploymentAvailable(ctx, "agentops-manager", 3*time.Minute); err != nil {
		t.Fatalf("the manager must stay Available through its own admitted rule: %v", err)
	}
}

func assertSplitNetpolFlowPasses(t *testing.T, ctx context.Context, e *Env, pod *corev1.Pod) {
	t.Helper()
	stamp := fmt.Sprint(time.Now().UnixNano())
	fp := "e2e-split-netpol-" + stamp
	e.PostTask(t, SourceTasks, fp, "echo split-netpol "+stamp)
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	run := conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "succeeded" || !strings.Contains(run.Result, "[stub] split-netpol "+stamp) {
		t.Fatalf("every wired flow must still pass split with policies enforced: %+v", run)
	}
	if conv.Status.RuntimePod == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: conv.Status.RuntimePod}, pod); err != nil {
		t.Fatalf("getting the runtime pod: %v", err)
	}
	if pod.Status.PodIP == "" {
		t.Fatalf("the runtime pod must carry a pod IP once its run has succeeded: %+v", pod.Status)
	}
}

func assertRuntimePodAcceptsNoIngress(t *testing.T, ctx context.Context, e *Env, pod *corev1.Pod) {
	t.Helper()
	if pod.Status.PodIP == "" {
		t.Skip("no runtime pod IP from the prior subtest")
	}
	// A runtime pod declares no container port of its own (it only
	// long-polls the manager) — the one thing inside it with a socket
	// bound to more than loopback is the egress-proxy sidecar
	// (chart default global.agentops.runtimeDefaults.egressMediation.
	// enabled: true, platform/egress-proxy/proxy.go: `net.Listen("tcp",
	// ":"+LISTEN_PORT)`, default 15001) — mediating the AGENT's own
	// outbound calls, never an inbound surface, so it is also the one
	// socket this test can aim an actual TCP connection at to tell a
	// policy DROP apart from "nothing is listening here anyway".
	//
	// Probed from a CONTROL-namespace pod with no rule admitting it —
	// agentops-nats-0, alpine-based and known to carry busybox wget
	// (the same double Dump(), cluster.go, and
	// assertCommandBusNetworkPolicyEnforced, async_system_test.go,
	// already rely on for exactly this reason).
	out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "exec", "agentops-nats-0", "--",
		"wget", "-T", "4", "-qO-", fmt.Sprintf("http://%s:15001/", pod.Status.PodIP))
	if err == nil {
		t.Fatalf("a runtime pod must accept NO ingress, got: %s", out)
	}
	lower := strings.ToLower(out + err.Error())
	// A dropped SYN times out, a rejected one is refused — either reads
	// as the policy having admitted nothing. A reset or an error reading
	// a reply would mean the connection WAS made.
	if !strings.Contains(lower, "timed out") && !strings.Contains(lower, "refused") {
		t.Fatalf("the denial must read as a blocked connection, not some other failure: %v %s", err, out)
	}
}

// 10.3 The authorizer — not the rendered Role, which the chart's own render
// tests already check — denies what design.md decision 4 withholds from
// both manager Roles: pods in a namespace that is neither control nor
// conversations (spec.md's "The manager cannot reach a third namespace"),
// and PVC delete in the conversations namespace (spec.md's "The storage
// invariant holds in the new namespace").
func TestSplitRBACDeniesThirdNamespaceAndPVCDelete(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()

	third := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-split-third-namespace"}}
	if err := e.K.Create(ctx, third); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("creating a third namespace (neither control nor conversations): %v", err)
	}
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), third)
	})

	deny := func(sa, verb, group, resource, namespace string) {
		t.Helper()
		ok, err := e.K.Can(ctx, sa, verb, group, resource, namespace)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("%s must NOT be allowed to %s %s.%s in %s", sa, verb, resource, group, namespace)
		}
	}
	allow := func(sa, verb, group, resource, namespace string) {
		t.Helper()
		ok, err := e.K.Can(ctx, sa, verb, group, resource, namespace)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("%s must be allowed to %s %s.%s in %s", sa, verb, resource, group, namespace)
		}
	}

	// Neither the control Role (which never carries pods at all under the
	// split, decision 1) nor the conversations Role (scoped to the
	// conversations namespace alone) grants the manager anything in a third
	// namespace.
	for _, verb := range []string{"get", "list", "watch"} {
		deny("agentops-manager", verb, "", "pods", third.Name)
	}

	// The storage invariant, now checked in the namespace it actually
	// applies to post-split: create allowed, delete/update/patch denied —
	// never a built-in role papering over either half.
	allow("agentops-manager", "create", "", "persistentvolumeclaims", ConversationsNamespace)
	for _, verb := range []string{"delete", "update", "patch"} {
		deny("agentops-manager", verb, "", "persistentvolumeclaims", ConversationsNamespace)
	}
}

// 10.4 A Secret a runtime pod needs is absent from the conversations
// namespace before the conversation ever starts: the pod fails at the
// KUBELET, in its own words, and the condition names the expected
// reference (spec.md's "A missing Secret fails at the pod, with a
// pointer") — proving decision 5's "naming a reference is not reading
// one" the way only a real kubelet, refusing to resolve something that
// genuinely is not there, can.
func TestSplitMissingSecretFailsPodAndConditionNamesIt(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	const missingSecret = "e2e-split-missing-secret"

	profile := &agentopsv1alpha1.AgentProfile{ObjectMeta: metav1.ObjectMeta{Name: "e2e-split-missing-secret-profile"}}
	profile.Spec.Agent = "stub"
	profile.Spec.OutputFormat = agentopsv1alpha1.OutputFormat("none")
	profile.Spec.SystemPrompt = "You are the e2e stub."
	profile.Spec.Env = []corev1.EnvVar{{
		Name: "E2E_MISSING_CRED",
		ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: missingSecret},
			Key:                  "token",
		}},
	}}
	mustCreate(t, e.K, profile)

	src := source("e2e-split-missing-secret-src", "e2e", nil)
	mustCreate(t, e.K, src)
	pipe := pipeline("e2e-split-missing-secret-pipe", profile.Name, []string{src.Name}, []string{ChannelConsole})
	mustCreate(t, e.K, pipe)
	if err := waitPipelineReady(ctx, e.K, pipe.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	fp := "e2e-split-missing-secret-" + stamp
	e.PostTask(t, src.Name, fp, "echo should-not-run")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})

	// The pod fails resolving a Secret that genuinely is not in the
	// conversations namespace — the manager read none of this in advance
	// (decision 5: the manager holds no `secrets` verb at all).
	waitFor(t, "the runtime pod to fail resolving the missing Secret", 3*time.Minute, func() (bool, error) {
		cur, err := e.K.Conversation(ctx, conv.Name)
		if err != nil || cur.Status.RuntimePod == "" {
			return false, err
		}
		var pod corev1.Pod
		if err := e.K.Get(ctx, types.NamespacedName{Namespace: ConversationsNamespace, Name: cur.Status.RuntimePod}, &pod); err != nil {
			return false, nil
		}
		return podFailedResolvingASecret(&pod), nil
	})

	// The condition names the reference, in the conversations namespace,
	// without the manager ever having read it.
	waitFor(t, "ExpectedReferences to name the missing secret in the conversations namespace", time.Minute, func() (bool, error) {
		cur, err := e.K.Conversation(ctx, conv.Name)
		if err != nil {
			return false, err
		}
		for _, c := range cur.Status.Conditions {
			if c.Type == "ExpectedReferences" {
				return strings.Contains(c.Message, missingSecret) && strings.Contains(c.Message, ConversationsNamespace), nil
			}
		}
		return false, nil
	})
}

// podFailedResolvingASecret reports whether any of the pod's containers (or
// init containers) are stuck unable to build their container config — the
// kubelet's own reason when a referenced Secret key cannot be resolved.
func podFailedResolvingASecret(pod *corev1.Pod) bool {
	waiting := func(statuses []corev1.ContainerStatus) bool {
		for _, cs := range statuses {
			if cs.State.Waiting == nil {
				continue
			}
			if cs.State.Waiting.Reason == "CreateContainerConfigError" ||
				strings.Contains(strings.ToLower(cs.State.Waiting.Message), "not found") {
				return true
			}
		}
		return false
	}
	return waiting(pod.Status.ContainerStatuses) || waiting(pod.Status.InitContainerStatuses)
}
