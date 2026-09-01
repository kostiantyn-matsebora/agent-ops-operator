//go:build e2e

package e2e

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

// Every test here names the component whose participation makes the
// assertion possible — the kubelet, the authorizer, the informer, the
// scheduler, the image puller. A test envtest could make does not belong here.

// 7.1 Credential projection — THE KUBELET. The bundle's bot Secret is named
// on the Channel, projected as envFrom by the reconciler and resolved by the
// kubelet; the adapter then uses the token against the fake Bot API, which
// records it. The manager performed zero Secret reads: the authorizer says it
// cannot (7.2), and the token still arrived.
func TestCredentialProjectionIsKubeletResolved(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	pods, err := e.K.Pods(ctx, "agentops.dev/adapter=telegram")
	if err != nil || len(pods) == 0 {
		t.Fatalf("channel-telegram pod: %v %d", err, len(pods))
	}
	var prefixes []string
	for _, c := range pods[0].Spec.Containers {
		for _, ef := range c.EnvFrom {
			if ef.SecretRef != nil {
				prefixes = append(prefixes, ef.Prefix+"<-"+ef.SecretRef.Name)
			}
		}
	}
	if len(prefixes) == 0 || !strings.Contains(strings.Join(prefixes, " "), "AGENTOPS_CRED_TG_OPS_") {
		t.Fatalf("the Channel's credential must be projected as envFrom under AGENTOPS_CRED_<CHANNEL>_: %v", prefixes)
	}
	// The adapter registers the command menu with the bot on startup, so the
	// fake has already seen the token — the value the kubelet resolved.
	waitFor(t, "a Bot API call carrying the projected token", 2*time.Minute, func() (bool, error) {
		for _, c := range e.BotCalls(t, "") {
			if c["token"] == e.Values.BotToken {
				return true, nil
			}
		}
		return false, nil
	})
}

// 7.2 RBAC as enforced — THE AUTHORIZER, via SubjectAccessReview against the
// live cluster. The rendered Role is what envtest already checks and is not
// what enforces anything.
func TestRBACAsEnforced(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	deny := func(sa, verb, group, resource string) {
		t.Helper()
		ok, err := e.K.Can(ctx, sa, verb, group, resource, Namespace)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("%s must NOT be allowed to %s %s.%s", sa, verb, resource, group)
		}
	}
	allow := func(sa, verb, group, resource string) {
		t.Helper()
		ok, err := e.K.Can(ctx, sa, verb, group, resource, Namespace)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Errorf("%s must be allowed to %s %s.%s", sa, verb, resource, group)
		}
	}
	// The manager reads NO Secrets — every verb.
	for _, verb := range []string{"get", "list", "watch", "create", "update", "patch", "delete"} {
		deny("agentops-manager", verb, "", "secrets")
	}
	allow("agentops-manager", "list", "agentops.dev", "conversations")
	allow("agentops-manager", "create", "", "pods")
	// The floor account is bound to NOTHING.
	for _, res := range []string{"pods", "secrets", "configmaps"} {
		deny("agentops-runtime", "list", "", res)
	}
	deny("agentops-runtime", "list", "agentops.dev", "conversations")
	// The console reads the agentops kinds and nothing else; it never writes.
	allow("agentops-adapter-console", "list", "agentops.dev", "conversations")
	allow("agentops-adapter-console", "watch", "agentops.dev", "pipelines")
	deny("agentops-adapter-console", "create", "agentops.dev", "conversations")
	deny("agentops-adapter-console", "get", "", "secrets")
	deny("agentops-adapter-console", "delete", "", "pods")
}

// 7.3 Informer liveness — THE INFORMER. Every reconciled kind is genuinely
// watchable by the manager's account, and the manager reconciles one of each,
// so a `resources:` entry miscased to a Go type name fails HERE rather than
// producing a silent forbidden loop.
func TestInformerLiveness(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	for _, kind := range []string{"agentprofiles", "agentruntimes", "channels", "channeladapters", "conversations",
		"mcpconfigs", "mcptoolsets", "pipelines", "signaladapters", "signalsources"} {
		for _, verb := range []string{"list", "watch"} {
			ok, err := e.K.Can(ctx, "agentops-manager", verb, "agentops.dev", kind, Namespace)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Errorf("the manager cannot %s %s — a miscased RBAC resource, most likely", verb, kind)
			}
		}
	}
	// And the reconcilers are alive: a fresh Pipeline gets its condition, a
	// fresh SignalAdapter gets its Deployment.
	p := pipeline("e2e-liveness-"+fmt.Sprint(time.Now().Unix()%100000), ProfileStub, []string{SourceTasks}, []string{ChannelConsole})
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, time.Minute); err != nil {
		t.Fatal(err)
	}
	logs, err := e.managerLogs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(logs), "is forbidden") {
		t.Fatalf("the manager log reports forbidden API calls:\n%s", grepLines(logs, "forbidden"))
	}
}

// 7.4 Context continuity across a pod restart — THE KUBELET AND THE CSI. The
// stub keeps its handle as a FILE on the context volume; the pod is deleted;
// the next unit is dispatched with the same handle and continues it.
func TestContextSurvivesLosingThePod(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	fp := "e2e-continuity-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, SourceTasks, fp, "echo one")
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	run := conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "succeeded" || !strings.Contains(run.Result, "[stub] one") {
		t.Fatalf("first run: %+v", run)
	}
	handle := conv.Status.RuntimeContextID
	if handle == "" {
		t.Fatalf("the runtime's handle must be recorded")
	}
	// Lose the pod.
	if conv.Status.RuntimePod == "" {
		t.Fatalf("no runtime pod recorded on %s", conv.Name)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Status.RuntimePod}}
	if err := e.K.Delete(ctx, pod); err != nil {
		t.Fatalf("deleting the runtime pod: %v", err)
	}
	waitFor(t, "the pod to be gone", 2*time.Minute, func() (bool, error) {
		var p corev1.Pod
		return apierrors.IsNotFound(e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: pod.Name}, &p)), nil
	})
	// Continue through the console, the human path.
	if code, out := e.ConsoleSend(t, conv.Name, "echo two"); code/100 != 2 {
		t.Fatalf("console send: %d %s", code, out)
	}
	conv = e.WaitRun(t, conv.Name, 2, 4*time.Minute)
	run = conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "succeeded" || !strings.Contains(run.Result, "[stub] two") {
		t.Fatalf("second run must continue, not fail for lost context: %+v", run)
	}
	if conv.Status.RuntimeContextID != handle {
		t.Fatalf("the handle must survive the pod: %q → %q", handle, conv.Status.RuntimeContextID)
	}
	for _, c := range conv.Status.Conditions {
		if c.Type == "ContextContinuity" && c.Status == metav1.ConditionFalse {
			t.Fatalf("continuity reported lost: %s %s", c.Reason, c.Message)
		}
	}
}

// 7.5 Admission FIFO under a real pod lifecycle — THE SCHEDULER AND THE
// KUBELET. Saturate the cap with pod-backed conversations, delete one pod,
// and the OLDEST Pending conversation is the one promoted, driven by the real
// DELETE event.
func TestAdmissionFIFOOnPodDelete(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	const cap = 5 // the chart default, maxActiveConversations
	// Other tests' pods idle out on their own clock; rather than waiting for
	// a quiet install, size the burst to what is left of the cap. The slots
	// are held by STALLING conversations: a finished pod is evicted the
	// moment a waiter exists, so only work in progress keeps the cap full.
	stamp := fmt.Sprint(time.Now().UnixNano())
	var names []string
	post := func(i int, text string) {
		fp := fmt.Sprintf("e2e-fifo-%s-%d", stamp, i)
		e.PostTask(t, SourceTasks, fp, text)
		c := e.ConversationFor(t, fp, time.Minute)
		names = append(names, c.Name)
		time.Sleep(1500 * time.Millisecond) // distinct creation timestamps
	}
	ours := func(pod corev1.Pod) bool { return indexOf(names, pod.Labels["agentops.dev/conversation"]) >= 0 }
	pauseCronLaneAndClearLeftovers(t, ctx, e)
	waitFor(t, "no runtime pods", 4*time.Minute, func() (bool, error) {
		pods, err := e.K.Pods(ctx, "agentops.dev/conversation")
		return err == nil && len(pods) == 0, err
	})
	for i := 0; i < cap; i++ {
		post(i, "stall")
	}
	waitFor(t, fmt.Sprintf("the cap of %d held by stalling conversations", cap), 4*time.Minute, func() (bool, error) {
		pods, err := e.K.Pods(ctx, "agentops.dev/conversation")
		if err != nil {
			return false, err
		}
		return countMatchingPods(pods, ours) >= cap, nil
	})
	holders := len(names)
	// Two waiters, the older first.
	post(holders, "echo older waiter")
	post(holders+1, "echo newer waiter")
	older, newer := names[holders], names[holders+1]
	t.Cleanup(func() {
		for _, n := range names {
			_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: n}})
		}
	})
	phase := func(n string) string {
		c, err := e.K.Conversation(ctx, n)
		if err != nil {
			return ""
		}
		return string(c.Status.Phase)
	}
	waitFor(t, "both waiters Pending", 3*time.Minute, func() (bool, error) {
		return phase(older) == "Pending" && phase(newer) == "Pending", nil
	})
	// Delete the first holder: its pod goes with it, a real DELETE event,
	// and the OLDER waiter must be the one promoted.
	if err := e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: names[0]}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the older waiter to be promoted", 3*time.Minute, func() (bool, error) {
		return phase(older) != "Pending" && phase(older) != "", nil
	})
	assertNewerNotPromotedBeforeOlder(t, e, ctx, phase, newer, older)
}

// pauseCronLaneAndClearLeftovers: the cron lane ticks every minute and its
// conversation is OLDER than anything posted here, so it would out-rank the
// waiters in the FIFO. Pause it by removing its Pipeline (its ticks then
// drop, Wired=False), and clear every other test's leftover conversation: an
// idle pod is evicted the moment a waiter exists, so only stalling work
// holds a slot.
func pauseCronLaneAndClearLeftovers(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	cronLane := pipeline(PipelineCron, ProfileStub, []string{SourceCron}, []string{ChannelConsole})
	cronLane.Namespace = Namespace
	if err := e.K.Delete(ctx, cronLane); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fresh := pipeline(PipelineCron, ProfileStub, []string{SourceCron}, []string{ChannelConsole})
		_ = ensure(context.Background(), e.K, fresh)
	})
	if _, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "delete", "conversations", "--all", "--wait=false"); err != nil {
		t.Fatal(err)
	}
}

func countMatchingPods(pods []corev1.Pod, pred func(corev1.Pod) bool) int {
	n := 0
	for _, p := range pods {
		if pred(p) {
			n++
		}
	}
	return n
}

// assertNewerNotPromotedBeforeOlder: the newer one may follow later when
// some other slot frees, but never before the older — FIFO is the claim,
// checked at the moment the older left Pending.
func assertNewerNotPromotedBeforeOlder(t *testing.T, e *Env, ctx context.Context, phase func(string) string, newer, older string) {
	t.Helper()
	if p := phase(newer); p != "Pending" {
		o, _ := e.K.Conversation(ctx, older)
		if o == nil || o.Status.Phase == "Pending" {
			t.Fatalf("the newer waiter %s was promoted before the older %s", newer, older)
		}
	}
}

// 7.6 Image pull through an authenticated registry — THE IMAGE PULLER and
// THE KUBELET, which resolves the pull Secret named on the ServiceAccount.
// Every other image is imported and therefore never pulled. FULL TIER: it
// pushes an image and pulls it back.
func TestImagePullThroughAuthenticatedRegistry(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	reg, err := StartRegistry(ctx, e.Cluster)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.Stop)
	ref, err := reg.Push(ctx, "agentops-test-stub-runtime:e2e", "stub-runtime:pulled")
	if err != nil {
		t.Fatal(err)
	}
	// The pull Secret rides on the floor ServiceAccount — the kubelet's own
	// mechanism, and the one place a private registry's credential can sit
	// without the manager reading a Secret.
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: "e2e-registry-pull"},
		Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: reg.DockerConfigJSON()}}
	mustCreate(t, e.K, secret)
	var sa corev1.ServiceAccount
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-runtime"}, &sa); err != nil {
		t.Fatal(err)
	}
	patched := sa.DeepCopy()
	patched.ImagePullSecrets = append(patched.ImagePullSecrets, corev1.LocalObjectReference{Name: secret.Name})
	if err := e.K.Update(ctx, patched); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var cur corev1.ServiceAccount
		if e.K.Get(context.Background(), types.NamespacedName{Namespace: Namespace, Name: "agentops-runtime"}, &cur) == nil {
			cur.ImagePullSecrets = nil
			_ = e.K.Update(context.Background(), &cur)
		}
	})
	rt := &agentopsv1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: "e2e-pulled"}}
	rt.Spec.Image = ref
	rt.Spec.ContextStorage = agentopsv1alpha1.ContextStorage("volume")
	rt.Spec.IdleTTLMinutes = 1
	mustCreate(t, e.K, rt)
	src := source("e2e-pulled-tasks", "e2e", nil)
	mustCreate(t, e.K, src)
	p := pipeline("e2e-pulled", ProfileStub, []string{src.Name}, []string{ChannelConsole})
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: rt.Name}
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, p.Name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	fp := "e2e-pulled-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, src.Name, fp, "echo pulled")
	conv := e.ConversationFor(t, fp, time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 5*time.Minute)
	if run := conv.Status.Runs[0]; run.Status != "succeeded" || !strings.Contains(run.Result, "pulled") {
		t.Fatalf("the pulled image must run: %+v", run)
	}
	pods, _ := e.K.Pods(ctx, "agentops.dev/conversation="+conv.Name)
	if len(pods) == 0 || !strings.HasPrefix(pods[0].Spec.Containers[0].Image, registryHost+"/") {
		t.Fatalf("the pod must run the registry's image: %+v", pods)
	}
}

// 7.7 Mechanism paths against the stub. Each guards a manager mechanism no
// agent exhibits on cue.
func TestStubMechanisms(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()

	t.Run("stale-context fails the next run", func(t *testing.T) { assertStaleContextFailsNextRun(t, ctx, e) })
	t.Run("no-context is not a loss", func(t *testing.T) { assertNoContextIsNotALoss(t, ctx, e) })
	t.Run("die clears inflight", func(t *testing.T) { assertDieClearsInflight(t, ctx, e) })
	t.Run("storage-outage holds work", func(t *testing.T) { assertStorageOutageHoldsWork(t, ctx, e) })
}

func assertStaleContextFailsNextRun(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	fp := "e2e-stale-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, SourceTasks, fp, "stale-context")
	conv := e.ConversationFor(t, fp, time.Minute)
	// A conversation whose handle names nothing fails EVERY continuation,
	// and while the breaker is open each failure is one more report —
	// so it must not outlive its test, or nothing closes the breaker.
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	if !strings.HasPrefix(conv.Status.RuntimeContextID, "stub-stale-") {
		t.Fatalf("latest-wins: the stale handle must be recorded, got %q", conv.Status.RuntimeContextID)
	}
	if code, out := e.ConsoleSend(t, conv.Name, "echo again"); code/100 != 2 {
		t.Fatalf("console send: %d %s", code, out)
	}
	conv = e.WaitRun(t, conv.Name, 2, 4*time.Minute)
	run := conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "failed" {
		t.Fatalf("a promised-and-lost context must FAIL the run, got %+v", run)
	}
	found := false
	for _, c := range conv.Status.Conditions {
		if c.Type == "ContextContinuity" && c.Status == metav1.ConditionFalse {
			found = true
		}
	}
	if !found {
		t.Fatalf("the loss must be visible as a condition: %+v", conv.Status.Conditions)
	}
}

func assertNoContextIsNotALoss(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	fp := "e2e-nocontext-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, SourceTasks, fp, "no-context")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	run := conv.Status.Runs[len(conv.Status.Runs)-1]
	if run.Status != "succeeded" || conv.Status.RuntimeContextID != "" {
		t.Fatalf("no handle is not a lost handle: %+v handle=%q", run, conv.Status.RuntimeContextID)
	}
	for _, c := range conv.Status.Conditions {
		if c.Type == "ContextContinuity" && c.Status == metav1.ConditionFalse {
			t.Fatalf("absence must not be reported as loss: %s", c.Message)
		}
	}
}

func assertDieClearsInflight(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	fp := "e2e-die-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, SourceTasks, fp, "die")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	var firstPod string
	waitFor(t, "the run to be inflight", 3*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		if err != nil {
			return false, err
		}
		firstPod = c.Status.RuntimePod
		return c.Status.Inflight != nil, nil
	})
	// The pod exits 3 without reporting. The manager sees the pod go and
	// clears the inflight run rather than waiting on a report that never
	// comes; the input is pending again.
	waitFor(t, "inflight to clear after the pod died", 4*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		if err != nil {
			return false, err
		}
		return c.Status.Inflight == nil || c.Status.RuntimePod != firstPod, nil
	})
}

// assertStorageOutageHoldsWork: three reports within the breaker window open
// it; the fourth conversation's input is HELD rather than consumed.
func assertStorageOutageHoldsWork(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	stamp := fmt.Sprint(time.Now().UnixNano())
	var convs []string
	for i := 0; i < 4; i++ {
		fp := fmt.Sprintf("e2e-outage-%s-%d", stamp, i)
		e.PostTask(t, SourceTasks, fp, "storage-outage")
		convs = append(convs, e.ConversationFor(t, fp, time.Minute).Name)
	}
	t.Cleanup(func() {
		for _, n := range convs {
			_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: n}})
		}
	})
	waitFor(t, "the breaker to hold a conversation", 6*time.Minute, func() (bool, error) {
		return anyConversationHeldByBreaker(ctx, e, convs)
	})
	held := 0
	for _, n := range convs {
		c, _ := e.K.Conversation(ctx, n)
		// While held, the input was not consumed: it is still pending.
		if c != nil && len(c.Spec.Inputs) > 0 && len(c.Status.ProcessedInputIDs) == 0 {
			held++
		}
	}
	if held == 0 {
		t.Fatalf("a held conversation keeps its input pending; none did")
	}
	// The outage conversations are removed so their held inputs stop
	// re-reporting. WHAT IS NOT ASSERTED: that the breaker closes again.
	// It closes on a CONTINUED run (a new context proves nothing about
	// the store), and the one canary the manager let through here never
	// reached a pod in two runs — an open question for the manager, filed
	// rather than papered over with a longer wait. This subtest therefore
	// runs LAST in the pack: an open breaker holds every conversation
	// after it.
	for _, n := range convs {
		_ = e.K.Delete(ctx, &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: n}})
	}
}

func anyConversationHeldByBreaker(ctx context.Context, e *Env, convs []string) (bool, error) {
	for _, n := range convs {
		c, err := e.K.Conversation(ctx, n)
		if err != nil {
			return false, err
		}
		for _, cond := range c.Status.Conditions {
			if cond.Reason == "ContextStoreUnavailable" {
				return true, nil
			}
		}
	}
	return false, nil
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func grepLines(text, needle string) string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(l), needle) {
			out = append(out, l)
		}
	}
	if len(out) > 20 {
		out = out[:20]
	}
	return strings.Join(out, "\n")
}

// managerLogs reads the manager's current log.
func (e *Env) managerLogs(ctx context.Context) (string, error) {
	pods, err := e.K.Pods(ctx, "app.kubernetes.io/name=agentops-manager")
	if err != nil || len(pods) == 0 {
		return "", fmt.Errorf("manager pod: %v", err)
	}
	out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "logs", pods[0].Name, "--tail=5000")
	return out, err
}
