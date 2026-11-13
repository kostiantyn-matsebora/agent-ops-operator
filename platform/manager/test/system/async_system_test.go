//go:build system

package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Section 11 (async-command-bus, task 11) — system-tier lanes for the
// command bus: what only a real cluster decides (design D-14's own table —
// "the bundle, credential projection, the NetworkPolicy, a claim on one
// replica completed on another, restart durability, ordering under
// parallel consumers, backoff into a dead letter"), as opposed to section
// 9's own end-to-end flows through the real HTTP surface
// (async_e2e_test.go, same package — every helper this file needs
// (openTaskStream, taskPut, awaitTaskUIDSucceeded, getTask, waitTaskState,
// deadLetters, postChannelInbound, acceptedWire, waitThreadOn,
// postInboundDuringRestart, restartManagerSchedulable) is reused from
// there, never redefined) and replicas_test.go's own manager-replica
// mechanics (scaleManagerReplicas, assertManagerRunsAtLeastNReadyReplicas,
// clearAdmissionPoolForReplicaLane).
//
// 11.1's own ask ("run against THIS working copy's chart and images, not
// the default branch's") is a PACK-WIDE guarantee, not something an
// individual lane re-asserts: install.go's own Install always renders
// `filepath.Join(root, "chart")` off repoRoot() (this checkout's working
// tree, never a fetched ref) and valuesYAML() always pins `image:
// agentops-manager tag: e2e` — the tag images.go's BuildAndImport just
// built from this commit and imported into the cluster (confirmed:
// `agentops-manager` is unconditionally in images.go's `neededImages`, and
// Import loads a built image directly, never pulling one, so there is no
// path by which a stale or published tag could answer `:e2e`). The ONE
// exception, named precisely rather than glossed over: `commandBus.nats.
// image` (chart/values.yaml) is a pinned THIRD-PARTY image
// (`nats:2.10-alpine` by digest) the bus server bundle runs — genuinely
// unrelated to "this working copy," unlike `agentops-manager`, which
// carries every line this change's sections 1-8 wrote. No lane below
// re-proves the pack-wide guarantee; each targets only what design D-14
// assigns to the system tier.
//
// commandBus.backend is NEVER varied across this pack's own runs — unlike
// SYSTEM_STATE_BACKEND, which pins the UNRELATED conversation/signal state
// store (task 5.7's kube-fallback smoke run), nothing in install.go ever
// sets commandBus.backend away from the chart's own default (nats; the
// same finding TestAsyncManagerRestartLosesNothingDuplicatesNothing's own
// doc comment already records for section 9). So no lane below needs a
// requireNats-style skip gate: there is no OTHER backend this pack ever
// installs to skip for.

// By the chart's default the command bus rides the state store's own nats
// bundle (commandBus.nats.enabled is unset and stateStore.backend is nats):
// one StatefulSet (agentops-nats), one client Service, one credential Secret,
// no second bundle. natsPods, natsSelector and waitNatsStatefulSetReady
// (state_nats_test.go) name those servers. A dedicated bus bundle
// (agentops-commandbus) renders only when `commandBus.nats.enabled` opts into
// one, which this pack never does.

// busCredentialSecret is the Secret the manager's COMMANDBUS_NATS_USER and
// _PASSWORD resolve from at the chart default: the shared bundle's own.
const busCredentialSecret = "agentops-nats-credentials"

// managerReadyPodNames names every manager pod currently Running, for a
// lane that needs to target one of them precisely (11.3) rather than only
// count them (assertManagerRunsAtLeastNReadyReplicas, replicas_test.go).
func managerReadyPodNames(t *testing.T, ctx context.Context, e *Env) []string {
	t.Helper()
	pods, err := e.K.Pods(ctx, "app.kubernetes.io/name=agentops-manager")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range pods {
		// A pod being deleted keeps phase Running until its container stops.
		if p.Status.Phase == corev1.PodRunning && p.DeletionTimestamp == nil {
			names = append(names, p.Name)
		}
	}
	return names
}

// ---- 11.2 -------------------------------------------------------------

// 11.2 The bus's servers come up, the credential is projected by the kubelet,
// and the NetworkPolicy admits only the manager. At the chart default the bus
// shares the state store's nats bundle (see above), so this pins that shared
// bundle as the bus's server.
func TestAsyncCommandBusBundleUpCredentialProjectedNetworkPolicyEnforced(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()

	t.Run("the bus servers come up at the chart default of three replicas", func(t *testing.T) {
		waitNatsStatefulSetReady(t, ctx, e, 3)
		if pods := natsPods(t, ctx, e); len(pods) < 3 {
			t.Fatalf("want at least 3 bus server pods, got %d", len(pods))
		}
	})

	t.Run("the credential is projected by the kubelet, never read by the manager", func(t *testing.T) {
		assertCommandBusCredentialProjected(t, ctx, e)
	})

	t.Run("the NetworkPolicy admits only the manager", func(t *testing.T) {
		assertCommandBusNetworkPolicyEnforced(t, ctx, e)
	})
}

// assertCommandBusCredentialProjected: the manager's own env carries
// COMMANDBUS_NATS_USER/PASSWORD as valueFrom.secretKeyRef against the
// bus server bundle's chart-generated credential Secret (chart/templates/
// deployment.yaml, cmd/manager/commandbus.go's own commandBusNatsUserEnv/
// commandBusNatsPasswordEnv) — never a literal value, which is what makes
// this the KUBELET's own resolution rather than the manager reading the
// Secret itself (invariants.md: "Manager reads NO secrets"). Available
// below is that connection having already succeeded WITH this exact
// value: the manager's own readiness check (commandBusReadinessCheck,
// cmd/manager/commandbus.go) reports not-ready while the nats connection
// it opens cannot be made, so a wrong or unresolved credential fails here,
// not silently.
func assertCommandBusCredentialProjected(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	pods, err := e.K.Pods(ctx, "app.kubernetes.io/name=agentops-manager")
	if err != nil || len(pods) == 0 {
		t.Fatalf("manager pod: %v %d", err, len(pods))
	}
	var seen []string
	for _, c := range pods[0].Spec.Containers {
		seen = append(seen, commandBusCredentialEnvNames(t, c.Env)...)
	}
	if len(seen) != 2 {
		t.Fatalf("the manager must carry both COMMANDBUS_NATS_USER and COMMANDBUS_NATS_PASSWORD, got: %v", seen)
	}
	if err := e.K.WaitDeploymentAvailable(ctx, "agentops-manager", 2*time.Minute); err != nil {
		t.Fatalf("the manager must be Available — i.e. already connected with the kubelet-resolved credential: %v", err)
	}
}

// commandBusCredentialEnvNames returns the names of the command bus
// credential variables in env, failing the test on one that is a literal or
// resolves from any Secret but the bundle's own.
func commandBusCredentialEnvNames(t *testing.T, env []corev1.EnvVar) []string {
	t.Helper()
	var seen []string
	for _, ev := range env {
		if ev.Name != "COMMANDBUS_NATS_USER" && ev.Name != "COMMANDBUS_NATS_PASSWORD" {
			continue
		}
		if ev.ValueFrom == nil || ev.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("%s must be a secretKeyRef, not a literal value", ev.Name)
		}
		if ev.ValueFrom.SecretKeyRef.Name != busCredentialSecret {
			t.Fatalf("%s must resolve from the bus server bundle's own credential Secret %q, got %q", ev.Name, busCredentialSecret, ev.ValueFrom.SecretKeyRef.Name)
		}
		seen = append(seen, ev.Name)
	}
	return seen
}

// assertCommandBusNetworkPolicyEnforced: enables the chart's one shared
// `global.agentops.networkPolicy.enabled` flag (default false — nothing in
// install.go's own valuesYAML sets it, and no lane in this package has
// enabled it before now, confirmed by grep: the ONLY existing
// NetworkPolicy test, internal/integration/charttemplate_networkpolicy_
// test.go, is a chart-RENDER test with no kubelet, no CNI and no live
// enforcement behind it), then probes the bus's own CLIENT port (4222)
// from a pod the rendered policy never names as an allowed source — a
// cluster-internal "really denied" a cooperating double could never
// simulate. k3d's own cluster here runs k3s's bundled network-policy
// controller (EnsureCluster, cluster.go, disables only traefik and
// servicelb, never `--disable-network-policy`), so this is a genuine CNI
// enforcement check, not a render assertion repeated at a different
// layer.
func assertCommandBusNetworkPolicyEnforced(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	// UpgradeManager, not Upgrade: the policy change rolls the manager, whose
	// hard pod anti-affinity leaves the new ReplicaSet Pending behind the old
	// pods on this pack's one-node cluster (observed: available=2 updated=1
	// replicas=3 for the whole wait).
	t.Cleanup(func() {
		if err := e.UpgradeManager(context.Background()); err != nil {
			t.Errorf("restoring the install with the NetworkPolicy back off: %v", err)
		}
	})
	if err := e.UpgradeManager(ctx, "global.agentops.networkPolicy.enabled=true"); err != nil {
		t.Fatalf("enabling the NetworkPolicy: %v", err)
	}

	// One server of the bundle dials ANOTHER server's client port by its
	// stable pod DNS name, never the client Service, which could route the
	// call back to the very pod it came from (loopback is not filtered by a
	// policy). A server pod is not the manager, so the rendered rule must
	// refuse it. Reused for the same reason Dump() (cluster.go) already execs
	// `wget` against a nats pod: the image is alpine-based and always carries
	// busybox wget, with no shell needed beyond `exec` itself.
	if len(natsPods(t, ctx, e)) < 2 {
		t.Fatal("need two bus server pods, one to probe the other's client port")
	}
	out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "exec", "agentops-nats-0", "--",
		"wget", "-T", "4", "-qO-", "http://agentops-nats-1.agentops-nats-headless:4222/")
	if err == nil {
		t.Fatalf("a non-manager pod must NOT reach the bus server's client port, got: %s", out)
	}
	lower := strings.ToLower(out + err.Error())
	// A dropped SYN times out and a rejected one is refused. A reset or an
	// error reading a reply means the connection WAS made, so the policy
	// admitted it.
	if !strings.Contains(lower, "timed out") && !strings.Contains(lower, "refused") {
		t.Fatalf("the denial must read as a blocked connection, not some other failure: %v %s", err, out)
	}

	// The manager itself stays admitted — its own readiness still passes
	// through the now-enforced policy. This is the one assertion left for
	// the manager's own side: its image is distroless with no shell to
	// exec a probe from (platform/manager/Dockerfile: `FROM gcr.io/
	// distroless/static:nonroot`).
	if err := e.K.WaitDeploymentAvailable(ctx, "agentops-manager", 3*time.Minute); err != nil {
		t.Fatalf("the manager must stay Available through its own admitted rule: %v", err)
	}
}

// ---- 11.3 -------------------------------------------------------------

// 11.3 A claim on one manager replica completes on another, and a lapsed
// lease is redelivered (design D-2: "This is what lets a claim on one
// replica be completed on another. A message object's acknowledgement
// cannot do that.").
//
// DOCUMENTED DECISION on the exact race (the same kind of call 9.7's own
// doc comment makes, async_e2e_test.go): nothing this pack exposes names
// WHICH replica's own consumer.Pool goroutine currently holds a task's
// claim. consumer/pool.go's own `ConsumerID` defaults to the QUEUE's name
// with a per-goroutine index appended ("conv-0", "conv-1"...), never
// POD_NAME — confirmed against cmd/manager/main.go's own consumer.New
// call, which sets no ConsumerID — so /state/tasks' own `lease.holder`
// reads identically on every replica and cannot be used to target one.
// And taskClose/taskDelete (convtasks.go) are one or two fast calls each,
// never a synchronous wait on anything slow, so there is no reliable way
// from outside the process to catch a task strictly BETWEEN "claimed" and
// "settled" without instrumentation this task's own scope does not extend
// to (test/fakebotapi, the one double with a controllable failure toggle,
// supports only `429`/`5xx` — no artificial delay — and this task's own
// file scope is this package alone).
//
// So rather than engineer a hold this environment has no double for, this
// lane forces the ONE thing it CAN pin deterministically: which replica is
// the SOLE POSSIBLE claimant at the moment work starts (scale to exactly
// one first), then adds a second, live, untouched sibling BEFORE killing
// the first — so a second consumer is always already running, ready to
// pick a lapsed lease back up whichever way the real race against the
// kubelet's SIGKILL actually falls. The 8-minute wait below is sized past
// the lease's own worst case (5 minutes, commandbus/nats's own
// defaultLeaseDuration, the conv queue's claim lease — the
// same margin TestAsyncManagerRestartLosesNothingDuplicatesNothing already
// uses for the identical reason), so a run that DOES catch the task still
// genuinely claimed at the instant of the kill still passes without
// flaking, not only the easier case where the original replica had
// already settled it before dying.
//
// Either way, the property actually asserted is the one D-2's safety
// argument names: the SAME task UID settles exactly once and the
// conversation closes exactly once. Unlike
// TestAsyncManagerRestartLosesNothingDuplicatesNothing (section 9.6's full
// -process restart, where the accept call itself must retry across a
// genuine total outage), the accept endpoint here never goes down at all —
// a sibling replica is continuously Ready throughout, which is the
// multi-replica availability property this lane proves ALONGSIDE the
// cross-replica completion one.
func TestAsyncClaimOnOneReplicaCompletesOnAnother(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	scaleManagerReplicas(t, ctx, e, 1)
	assertManagerRunsAtLeastNReadyReplicas(t, ctx, e, 1)
	solo := managerReadyPodNames(t, ctx, e)
	if len(solo) != 1 {
		t.Fatalf("want exactly one manager pod at this point, got %v", solo)
	}
	original := solo[0]

	fp := "e2e-async-cross-replica-" + stamp
	e.PostTask(t, SourceTasks, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)
	thread := waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)

	// A second, live sibling, added while `original` keeps running —
	// forceManagerRollout (replicas_test.go) leaves an already
	// affinity-free current pod alone when only the replica COUNT
	// changes, so from this point on there are genuinely two independent
	// consumer.Pools racing to claim from the SAME "conv" queue.
	scaleManagerReplicas(t, ctx, e, 2)
	assertManagerRunsAtLeastNReadyReplicas(t, ctx, e, 2)

	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, "/close", "system-pack-11.3")
	if code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("accepting the close: %d %+v", code, accepted)
	}
	closeUID := accepted.TaskIDs[0]

	// Race the kill against whatever `original` is doing with this task —
	// see this test's own doc comment above for why neither outcome of
	// that race is distinguished here.
	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "delete", "pod", original, "--grace-period=0", "--force", "--wait=false"); err != nil {
		t.Fatalf("force-deleting the original replica %s: %v\n%s", original, err, out)
	}

	awaitTaskUIDSucceeded(t, frames, closeUID, 8*time.Minute)
	waitFor(t, "the conversation closed exactly once", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})
	// Succeeded is removed after its 30s grace, so a read back is succeeded or
	// gone, and never anything that could still run the close again.
	if final, fcode := getTask(t, e, auth, closeUID); fcode == 200 && final.State != "succeeded" {
		t.Fatalf("the close task must not be left in any state but succeeded: %+v", final)
	}
}

// ---- 11.4 -------------------------------------------------------------

// 11.4 Ordering holds under parallel consumers, across two manager
// replicas running at once: several inbound messages submitted in rapid
// succession against the SAME conversation — one Key
// (commandbus/commandbus.go's own Key doc comment: "the next is not
// claimable until the previous has settled") — must settle in the order
// they were submitted even while two independent consumer.Pools are
// genuinely racing to claim from the SAME "conv" queue. This watches
// design D-2's own claim-time rule hold ("A consumer claims a task only
// when no earlier task of its key is unsettled"), rather than assuming
// which replica wins any one race — unlike 11.3, there is no artificial
// construction needed here: the ordering guarantee is about SUBMIT order
// versus SETTLE order, and taskInboundMessage's own processing
// (convtasks.go: "replays HandleMessage wholesale") is a fast, synchronous
// enqueue, not a wait for the eventual agent run — exactly the layer this
// lane means to pin.
func TestAsyncOrderingHoldsAcrossTwoReplicasUnderParallelConsumers(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	scaleManagerReplicas(t, ctx, e, 2)
	assertManagerRunsAtLeastNReadyReplicas(t, ctx, e, 2)

	fp := "e2e-async-order-" + stamp
	e.PostTask(t, SourceTasks, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)
	thread := waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)

	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	const n = 5
	var submitted []string
	for i := 0; i < n; i++ {
		text := fmt.Sprintf("echo order-step-%d-%s", i, stamp)
		code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, text, "system-pack-11.4")
		if code != 202 || len(accepted.TaskIDs) != 1 {
			t.Fatalf("accepting step %d: %d %+v", i, code, accepted)
		}
		submitted = append(submitted, accepted.TaskIDs[0])
	}

	observed := awaitSettleOrder(t, frames, submitted)
	for i, uid := range submitted {
		if observed[i] != uid {
			t.Fatalf("submission order must equal settle order under two parallel consumers: submitted %v, observed %v",
				submitted, observed)
		}
	}

	code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, "/close", "system-pack-11.4")
	if code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("closing: %d %+v", code, accepted)
	}
	waitTaskSettled(t, e, auth, accepted.TaskIDs[0], 2*time.Minute)
}

// awaitSettleOrder reads the task stream until every submitted task has
// succeeded and returns their UIDs in the order the stream showed them settle.
func awaitSettleOrder(t *testing.T, frames <-chan string, submitted []string) []string {
	t.Helper()
	var observed []string
	settled := map[string]bool{}
	deadline := time.After(5 * time.Minute)
	for len(settled) < len(submitted) {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("the task stream ended before every step settled: %v", observed)
			}
			w, ok := taskPut(f)
			if !ok || w.State != "succeeded" || settled[w.UID] {
				continue
			}
			if slices.Contains(submitted, w.UID) {
				settled[w.UID] = true
				observed = append(observed, w.UID)
			}
		case <-deadline:
			t.Fatalf("not every step settled within the deadline: %v of %v", observed, submitted)
		}
	}
	return observed
}

// ---- 11.5 -------------------------------------------------------------

// 11.5 A manager restart, and separately a bus-server rolling restart,
// lose no accepted task.
//
// The manager half reuses EXACTLY the mechanism section 9.6's own
// TestAsyncManagerRestartLosesNothingDuplicatesNothing already pins at the
// end-to-end tier (restartManagerSchedulable + postInboundDuringRestart,
// both async_e2e_test.go, same package): this is the SAME underlying fact,
// asked for again at the SYSTEM tier by design D-14's own table ("System:
// ... restart durability"). Kept minimal here rather than re-deriving a
// second, divergent way to prove it — see 9.6's own doc comment for the
// full argument (why the SAME task UID, not merely the same eventual
// effect, is the claim this pack's nats-backed bus supports).
//
// The bus-server half is different in KIND: unlike a manager replica's own
// claim (not durably placed on any one node — see 11.3's own doc
// comment), the LOG ITSELF is a 3-way replicated JetStream store
// (design D-8's shape, reused verbatim for the command bus per design
// D-13's "same shape as the state bundle"). One server down at a time is
// comfortably inside quorum for every task regardless of exactly when it
// is submitted relative to the kill, so unlike 11.3 there is no claim-
// timing race to document here — this mirrors state_nats_test.go's own
// TestNatsServerPodDeletionMidConversationCompletes, applied to the
// command bus's OWN StatefulSet instead of the state store's.
func TestAsyncManagerRestartAndBusServerRestartLoseNoAcceptedTask(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()

	t.Run("manager restart", func(t *testing.T) {
		assertManagerRestartLosesNoAcceptedTask(t, ctx, e)
	})
	t.Run("bus-server rolling restart", func(t *testing.T) {
		assertBusServerRollingRestartLosesNoAcceptedTask(t, ctx, e)
	})
}

func assertManagerRestartLosesNoAcceptedTask(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	fp := "e2e-async-sys-restart-" + stamp
	e.PostTask(t, SourceTasks, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)
	thread := waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)

	body, _ := json.Marshal(map[string]any{"channel": ChannelConsole, "threadId": thread, "text": "/close", "sender": "system-pack-11.5"})

	var closeCode int
	var closeAccepted acceptedWire
	var closeErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		closeCode, closeAccepted, closeErr = postInboundDuringRestart(ctx, e.Manager.URL()+"/channel/inbound", auth, body, 6*time.Minute)
	}()
	if err := restartManagerSchedulable(ctx, e.Cluster); err != nil {
		t.Fatalf("restart the manager: %v", err)
	}
	wg.Wait()
	if closeErr != nil || closeCode != 202 || len(closeAccepted.TaskIDs) != 1 {
		t.Fatalf("accepting the close across the restart: code=%d err=%v accepted=%+v", closeCode, closeErr, closeAccepted)
	}
	waitTaskSettled(t, e, auth, closeAccepted.TaskIDs[0], 7*time.Minute)
	waitFor(t, "the conversation closed", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		return err == nil && c.Status.Phase == agentopsv1alpha1.ConversationClosed, err
	})
}

func assertBusServerRollingRestartLosesNoAcceptedTask(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	fp := "e2e-async-sys-bus-roll-" + stamp
	e.PostTask(t, SourceTasks, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)
	thread := waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, time.Minute)

	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	// Captured ONCE, up front: the StatefulSet's own ordinal pod names
	// (agentops-nats-0/1/2), so rolling each of them exactly once is
	// a true rolling restart of the whole bundle — re-listing after each
	// delete would risk picking the SAME, already-recreated ordinal again
	// rather than moving on to the next.
	var names []string
	for _, p := range natsPods(t, ctx, e) {
		names = append(names, p.Name)
	}
	if len(names) < 3 {
		t.Fatalf("need the bundle's own 3 replicas to roll one at a time, got %d", len(names))
	}

	// One submitted close task per server rolled, interleaved with the
	// roll itself — every one of them must still succeed once the roll
	// finishes, regardless of which server was mid-restart when it was
	// accepted.
	var submitted []string
	for i, name := range names {
		text := fmt.Sprintf("echo bus-roll-%d-%s", i, stamp)
		code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, text, "system-pack-11.5")
		if code != 202 || len(accepted.TaskIDs) != 1 {
			t.Fatalf("accepting step %d: %d %+v", i, code, accepted)
		}
		submitted = append(submitted, accepted.TaskIDs[0])

		if err := e.K.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: name}},
			client.GracePeriodSeconds(0)); err != nil {
			t.Fatal(err)
		}
		waitNatsStatefulSetReady(t, ctx, e, int32(len(names)))
	}

	for _, uid := range submitted {
		awaitTaskUIDSucceeded(t, frames, uid, 3*time.Minute)
	}

	code, accepted := postChannelInbound(t, e, auth, ChannelConsole, thread, "/close", "system-pack-11.5")
	if code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("closing: %d %+v", code, accepted)
	}
	waitTaskSettled(t, e, auth, accepted.TaskIDs[0], 2*time.Minute)
}

// ---- 11.6 -------------------------------------------------------------

// 11.6 Backoff into a dead letter against a surface that really fails —
// over the REAL network, not a cooperating double's 5xx.
//
// Section 9.7 (async_e2e_test.go, TestAsyncChannelFailureBacksOffDead
// LettersThenRecovers) already proves the hand-over-then-dead-letter-then-
// recovery shape end to end against test/fakebotapi's own `/control/fail`
// toggle — a COOPERATING failure a unit test could simulate equally well.
// What only a real cluster decides (design D-14's own table: "System: ...
// backoff into a dead letter") is whether the SAME shape holds against a
// failure the manager cannot distinguish from a genuine outage: gateway-
// telegram's own TELEGRAM_API_BASE (invariants.md: "the router's own env
// var, chart-injected... NEVER Channel.spec.config") repointed at a host
// nothing answers on this cluster, so every outbound call fails with a
// genuine DNS/connection error the kubelet's own network stack produces,
// never a JSON body test/fakebotapi chose to return.
//
// `telegram.apiBase` is a plain chart value (install.go's own valuesYAML
// already sets it to the fake Bot API's real address), so repointing it
// is an UpgradeManager: the helm upgrade rolls the manager, whose hard
// anti-affinity leaves the new ReplicaSet Pending on this one-node cluster
// (observed: available=2 updated=1 replicas=3 for the whole wait).
func TestAsyncBackoffIntoADeadLetterAgainstARealFailure(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	t.Cleanup(func() {
		if err := e.UpgradeManager(context.Background()); err != nil {
			t.Errorf("restoring the real telegram apiBase: %v", err)
		}
	})
	if err := e.UpgradeManager(ctx, "telegram.apiBase=http://agentops-nonexistent-bot-api.agent-ops.svc.cluster.local:8081"); err != nil {
		t.Fatalf("repointing the gateway at an address nothing answers: %v", err)
	}

	// Opened before the conversation exists, as 9.7 does: the telegram
	// ensure-topic is enqueued by the reconciler, never an accept handler, so
	// the task stream is the only way to find its UID, and a feed that starts
	// from "now" would miss it once the console half had bound.
	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	fp := "e2e-async-real-failure-" + stamp
	e.PostTask(t, SourceFanout, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	// The console half of the same fanout must still bind fine — the
	// failure is scoped to the telegram surface's own router, which
	// console never reaches (the same isolation 9.7 already relies on).
	waitThreadOn(t, ctx, e, conv.Name, ChannelConsole, 2*time.Minute)

	uid := awaitChannelTaskUID(t, frames, ChannelTelegram, conv.Name)

	// Handed over: the adapter acknowledged it and is retrying against the
	// genuinely unreachable address inside the delivery TTL.
	waitTaskState(t, e, auth, uid, "running", 2*time.Minute)

	// The real dead-letter record: the adapter reports FAILED once its retry
	// budget (the 5 minute topic delivery TTL less a margin) is spent, and a
	// failure is final. 7 minutes covers the manager's TTL backstop too.
	waitTaskState(t, e, auth, uid, "dead", 7*time.Minute)
	dead := deadLetters(t, e, auth, "out."+ChannelTelegram)
	if !slices.ContainsFunc(dead, func(d taskStateWire) bool { return d.UID == uid }) {
		t.Fatalf("the dead task %s must be listed on GET /state/dead-letters?queue=out.%s", uid, ChannelTelegram)
	}
}

// awaitChannelTaskUID reads the task stream until a task on the Channel's own
// queue (out.<channel>) keyed by the named conversation appears and returns
// its UID. The queue is named for the Channel resource, not its adapter.
func awaitChannelTaskUID(t *testing.T, frames <-chan string, channel, conversation string) string {
	t.Helper()
	deadline := time.After(3 * time.Minute)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("the task stream ended before a task on out.%s appeared", channel)
			}
			if w, ok := taskPut(f); ok && w.Queue == "out."+channel && w.Key == conversation {
				return w.UID
			}
		case <-deadline:
			t.Fatalf("no task on out.%s for conversation %s ever appeared on the task stream", channel, conversation)
		}
	}
}

// ---- 11.7 -------------------------------------------------------------

// consoleOpFrame is one `op` event off GET /channel/ops/stream.
type consoleOpFrame struct {
	OpID string
	Kind string
}

// consoleStreamMark picks this lane's own ops out of the console queue: the
// conversations it starts are named for their fingerprint.
const consoleStreamMark = "e2e-async-console-stream"

// 11.7 The console's feed after a drop: nothing it never acknowledged is lost,
// nothing it acknowledged is delivered again (design D-17, the receipt-only
// adapter). The stream has no cursor and no Last-Event-ID: every connection is
// pushed exactly the messages not yet acknowledged, and the acknowledgement is
// what consumes a message. The live console is scaled to zero for the lane so
// this test is the only consumer of the console channel's queue while it is
// held away, then reads and acknowledges over the SAME endpoints the console
// uses, as the master token, and finally lets the real console return.
//
// The console is held away with a NetworkPolicy, since the ChannelAdapter
// reconciler undoes a scale to zero (observed: it never reached zero in 2m).
// If the isolation did not hold, the first read fails naming the console.
func TestAsyncConsoleStreamResumesAfterADropWithoutLossOrDuplicates(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	stamp := fmt.Sprint(time.Now().UnixNano())
	auth := bearerPrefix + e.Values.AdapterToken

	restore := stopConsoleConsumer(t, context.Background(), e)

	// Three conversations: each opens with an ensure-topic op on the console
	// channel, on its own key, so none is serialized behind another.
	for i := 0; i < 3; i++ {
		fp := fmt.Sprintf("%s-%s-%d", consoleStreamMark, stamp, i)
		e.PostTask(t, SourceTasks, fp, "echo "+fp)
	}

	first := readConsoleOpStream(t, e, auth, 3, 0, 3*time.Minute)

	// Acknowledge only the first, then drop (readConsoleOpStream closed the
	// connection). The other two stay pushed-and-unacknowledged.
	ackConsoleOp(t, e, auth, first[0])

	// A reconnect is pushed exactly the two it never acknowledged.
	replay := readConsoleOpStream(t, e, auth, 2, 0, time.Minute)
	if got, want := opIDs(replay), opIDs(first[1:]); !slices.Equal(sortedCopy(got), sortedCopy(want)) {
		t.Fatalf("a reconnect must be pushed exactly the unacknowledged ops %v, got %v", want, got)
	}
	for _, f := range replay {
		if f.OpID == first[0].OpID {
			t.Fatalf("op %s was acknowledged and must not be delivered again", f.OpID)
		}
	}

	// The console comes back: it is pushed the two still unacknowledged and
	// acknowledges them, so none is left to push again.
	restore()
	waitFor(t, "the returned console to acknowledge every op held for it, leaving none to push again", 3*time.Minute, func() (bool, error) {
		return len(readConsoleOpStream(t, e, auth, 0, 10*time.Second, time.Minute)) == 0, nil
	})
}

func opIDs(frames []consoleOpFrame) []string {
	ids := make([]string, 0, len(frames))
	for _, f := range frames {
		ids = append(ids, f.OpID)
	}
	return ids
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

// consoleIsolation is the NetworkPolicy that cuts the console adapter off from
// every destination, the manager included.
const consoleIsolation = "e2e-console-isolated"

// stopConsoleConsumer holds the console adapter away from the manager, and lifts
// the hold (restore) when the test calls it or ends. The ChannelAdapter
// reconciler pins the console Deployment to one replica, so a scale to zero is
// undone at once; cutting its egress with a NetworkPolicy holds, and the pod is
// replaced so its already-open stream (an established connection outlives a
// policy) is dropped. The adapter stays up and keeps retrying its stream, which
// is exactly the adapter that went away and comes back.
func stopConsoleConsumer(t *testing.T, ctx context.Context, e *Env) (restore func()) {
	t.Helper()
	const selector = "agentops.dev/adapter=console"
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: consoleIsolation},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"agentops.dev/adapter": "console"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}
	replacePods := func(ctx context.Context) error {
		pods, err := e.K.Pods(ctx, selector)
		if err != nil {
			return err
		}
		for i := range pods {
			if err := e.K.Delete(ctx, &pods[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}
	done := false
	restore = func() {
		if done {
			return
		}
		done = true
		bg := context.Background()
		if err := e.K.Delete(bg, policy); err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("lifting the console isolation: %v", err)
		}
		// Replaced so it reconnects at once instead of after its own backoff.
		if err := replacePods(bg); err != nil {
			t.Errorf("replacing the console pod: %v", err)
		}
		if err := e.K.WaitDeploymentAvailable(bg, "agentops-adapter-console", 3*time.Minute); err != nil {
			t.Errorf("the console adapter must come back: %v", err)
		}
	}
	// Registered BEFORE the risky call, as scaleManagerReplicas does.
	t.Cleanup(restore)
	if err := e.K.Create(ctx, policy); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("isolating the console adapter: %v", err)
	}
	if err := replacePods(ctx); err != nil {
		t.Fatalf("replacing the console pod under the isolation: %v", err)
	}
	return restore
}

// readConsoleOpStream opens GET /channel/ops/stream as the console adapter and
// collects this lane's `ensure-topic` `op` frames until `want` have arrived
// (want > 0), or until `idle` passes with none (idle > 0), or limit elapses. It
// then closes the connection, which is the drop.
func readConsoleOpStream(t *testing.T, e *Env, auth string, want int, idle, limit time.Duration) []consoleOpFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	url := e.Manager.URL() + "/channel/ops/stream?adapter=console&contract=2"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("opening the console op stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /channel/ops/stream = %d, want 200", resp.StatusCode)
	}

	frames := make(chan consoleOpFrame)
	go scanConsoleOpFrames(ctx, resp.Body, frames)

	var got []consoleOpFrame
	seen := map[string]bool{}
	quiet := time.NewTimer(limit)
	if idle > 0 {
		quiet.Reset(idle)
	}
	defer quiet.Stop()
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("the op stream ended on its own after %d frames", len(got))
			}
			if seen[f.OpID] {
				t.Fatalf("op %s was pushed twice on one connection", f.OpID)
			}
			seen[f.OpID] = true
			got = append(got, f)
			if want > 0 && len(got) >= want {
				return got
			}
			if idle > 0 {
				quiet.Reset(idle)
			}
		case <-quiet.C:
			if want > 0 {
				t.Fatalf("only %d of %d op frames arrived (is the console consumer really stopped?)", len(got), want)
			}
			return got
		case <-ctx.Done():
			t.Fatalf("the op stream read ran out of time with %d frames", len(got))
		}
	}
}

// scanConsoleOpFrames reads the SSE body and sends this lane's own
// ensure-topic `op` frames to frames, closing it when the body ends.
func scanConsoleOpFrames(ctx context.Context, body io.Reader, frames chan<- consoleOpFrame) {
	defer close(frames)
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	var event, data string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if event == "op" && !sendOwnEnsureTopic(ctx, data, frames) {
				return
			}
			event, data = "", ""
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// sendOwnEnsureTopic forwards the frame when it is this lane's own
// ensure-topic op (the console queue may hold others, a later card behind an
// unfinished topic). It returns false only when ctx ended mid-send.
func sendOwnEnsureTopic(ctx context.Context, data string, frames chan<- consoleOpFrame) bool {
	var op struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if json.Unmarshal([]byte(data), &op) != nil || op.Kind != "ensure-topic" || !strings.Contains(op.ID, consoleStreamMark) {
		return true
	}
	select {
	case frames <- consoleOpFrame{OpID: op.ID, Kind: op.Kind}:
		return true
	case <-ctx.Done():
		return false
	}
}

// ackConsoleOp acknowledges one op as the console does: the receipt. The
// manager consumes the message at the acknowledgement (`consumed: true`) and no
// report follows.
func ackConsoleOp(t *testing.T, e *Env, auth string, f consoleOpFrame) {
	t.Helper()
	code, out := e.do(t, "POST", e.Manager.URL()+"/channel/ops/"+url.PathEscape(f.OpID)+"/ack?adapter=console&contract=2", nil, auth)
	if code != 200 || !strings.Contains(out, `"consumed":true`) {
		t.Fatalf("acknowledging %s: %d %s (a receipt adapter's acknowledgement must consume it)", f.OpID, code, out)
	}
}
