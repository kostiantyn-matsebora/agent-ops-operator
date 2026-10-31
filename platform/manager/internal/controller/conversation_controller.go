// Package controller reconciles Conversations: chat topic, MCP ConfigMap,
// runtime pod lifecycle (cap + idle eviction), input pruning.
package controller

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/storagebreaker"
)

// ConditionContextContinuity reports whether this conversation still carries the
// context it accumulated. False means a run could not reach it and the thread
// restarted — the one failure that otherwise leaves a conversation looking
// entirely healthy while answering without memory.
//
// The MESSAGE is whatever the runtime reported, verbatim: the manager does not
// know where a given runtime keeps context, so it does not diagnose.
const ConditionContextContinuity = "ContextContinuity"

// ConditionToolingResolved reports whether a conversation's wiring-level
// tooling bindings (mcpConfigs / toolsets) could be resolved. Only set on
// conversations that carry a binding — binding-less conversations have nothing
// to resolve and stay condition-free.
const ConditionToolingResolved = "ToolingResolved"

// ConditionDeliveryPending reports that a run's answer is recorded on the
// conversation but has not reached every bound thread.
//
// It exists because the failure it names was INVISIBLE: on 2026-08-13 rejected
// sends left 22 threads empty for four and a half hours while the answers sat
// in status.runs[].result and the operator reported itself healthy. The reply
// was already derivable; nothing said it was owed.
//
// Derived on every reconcile from the same walk that re-enqueues, so it adds no
// second source of truth — it is a reading of runs[].delivered[], not a record.
const ConditionDeliveryPending = "DeliveryPending"

// ConditionRuntimeStarted reports whether this conversation's runtime pod
// reached a running state, and when it did not, WHY — in the kubelet's own
// words rather than ours.
//
// It exists because the failure it names was silent. On 2026-08-20 five pods
// sat in ContainerCreating for fifteen hours behind a corrupt filesystem,
// holding every capacity slot, and the only condition on any of them read
// DeliveryPending=False / AllDelivered — which looks like health. Nothing said
// a pod could not start, so nothing said the install was down.
//
// Set only on conversations that have actually failed to start one: a
// conversation whose pods have always come up carries no condition rather than
// a standing declaration of fine.
const ConditionRuntimeStarted = "RuntimeStarted"

// MCPConfigMapName returns the ConfigMap holding a conversation's compiled
// mcp.json. Always conversation-keyed: capabilities come from the wiring, so
// there is no shared profile-owned document to collide over.
func MCPConfigMapName(conv string) string { return convMCPConfigMapName(conv) }

func convMCPConfigMapName(conv string) string { return "agentops-mcp-conv-" + conv }

// ConversationReconciler reconciles Conversation objects.
type ConversationReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Runtime runtimepod.Config
	// Store is the only way the reconciler reaches a Conversation or a queued
	// input's payload (state-store design D-2). Its scope is one namespace;
	// Namespace names it for the pods, ConfigMaps and channels that live beside
	// the conversations, which stay Kubernetes objects under every backend.
	Store     state.Store
	Namespace string
	// MaxActiveConversations caps simultaneously ACTIVE conversations — a
	// conversation is active while a runtime pod exists for it. Counted from
	// live pods, never from status: a pod stuck Pending on an unschedulable
	// node, or a status patch lost to a conflict, must not inflate capacity.
	MaxActiveConversations int
	// Ops carries outbound channel operations (topic creation) to the serving
	// channel implementation; nil disables chat entirely (tests).
	Ops *chat.OpQueue
	// CloseTopicGrace overrides how long deletion waits on close-topic ops;
	// zero means DefaultCloseTopicGrace.
	CloseTopicGrace time.Duration
	// RuntimeStartDeadline bounds how long a runtime pod may sit without
	// reaching Running before it is reaped; zero means
	// DefaultRuntimeStartDeadline. A pod that never starts still COUNTS against
	// the cap for as long as it exists — this bounds how long that is, which is
	// a different thing from exempting it.
	RuntimeStartDeadline time.Duration
	// MasterToken derives AOPS_MCP_TOKEN for a Coordinator-rooted conversation
	// whose wiring binds the aops MCPConfig (design D-F) — the same
	// ADAPTER_TOKEN every other derived token in this manager is HMAC'd from.
	// Empty disables the injection (tests), same as every other optional field
	// here.
	MasterToken string
	// StorageBreaker is the install-wide judgement about whether context
	// storage is reachable. SHARED with the HTTP API, which feeds it the
	// reporting edge; the reconciler feeds it the PROVISIONING edge — a pod
	// that cannot attach its volume is the same incident as a run that cannot
	// reach its context. Nil disables the provisioning edge entirely (tests).
	StorageBreaker *storagebreaker.Breaker
	// DrainAware releases idle runtime pods from a node that is going down, so
	// a shared context volume detaches before the machine reboots. OFF by
	// default and gated in the chart: nodes are cluster-scoped, and reading
	// them is this manager's only cluster-scoped permission.
	DrainAware bool
	// Activity records the hops the reconciler mediates — currently the runtime
	// pod being created, which is otherwise the largest invisible gap in a
	// conversation's timeline. Nil is usable and ignores every emission.
	Activity *activity.Log
	// Recorder emits Kubernetes events on the Conversation. Nil disables them
	// (tests), because an event is a report and never a control path.
	Recorder record.EventRecorder
	// Router closes conversations. The reconciler depends on it so the TIMER
	// closes through the same path /close does — a farewell on every bound
	// thread, and one implementation of the close. The alternative is a second
	// farewell, which is exactly what the one-implementation rule forbids.
	// Nil disables autoclose.
	Router *chat.Router
	// AUTOCLOSE: a finished conversation is closed once it has been INACTIVE
	// this long. Measured from last activity, never from creation — a
	// conversation created last week that answered an hour ago is busy, not old.
	AutoCloseEnabled bool
	AutoCloseIdleAge time.Duration
	// AUTODELETE: a CLOSED conversation is deleted this long after ClosedAt.
	// A separate flag rather than "delete when both are set": autoclose with
	// autodelete off — a lane that tidies itself and keeps its record — is the
	// common configuration, and must not require declining the destructive half.
	AutoDeleteEnabled   bool
	AutoDeleteClosedAge time.Duration
	// ReplicaIdentity names THIS process for the claim this reconciler writes
	// (durable-chat-ops-broker). It needs no relation to the leader-election
	// Lease's own HolderIdentity string: this reconciler runs ONLY while this
	// process holds that Lease (controller-runtime gates every reconciler on
	// it), so any claim this process did not itself write was written by a
	// FORMER leader, by construction — comparing against "me" is exactly
	// comparing against "the current leader". Empty is tolerated (tests) and
	// behaves as a single, unvarying identity.
	ReplicaIdentity string
	// ClaimStaleness bounds how long an ensure-topic claim may sit with no
	// thread yet before the current leader treats it as abandoned and
	// re-dispatches — the SAME leader's own claim running legitimately slow,
	// never a second writer. Zero means DefaultClaimStalenessSeconds.
	ClaimStaleness time.Duration
}

// DefaultClaimStalenessSeconds is the fallback claim staleness bound when the
// chart's claimStalenessSeconds value is unset. Sized with margin over a
// measured ensure-topic round-trip against the slowest shipped adapter
// (Telegram, forum-topic creation): comfortably longer than any observed
// single call, short enough that a genuinely abandoned claim (the leader that
// wrote it crashed, rather than merely answering slowly) does not leave a
// conversation topic-less for long.
const DefaultClaimStalenessSeconds = 90

func (r *ConversationReconciler) claimStaleness() time.Duration {
	if r.ClaimStaleness > 0 {
		return r.ClaimStaleness
	}
	return DefaultClaimStalenessSeconds * time.Second
}

// Reconcile implements the reconciliation loop.
func (r *ConversationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	key := state.Key(req.Name)
	conv, _, err := r.Store.Get(ctx, key)
	if err != nil {
		return ctrl.Result{}, state.IgnoreNotFound(err)
	}

	// A conversation on its way out archives its threads first — by /close or by
	// `kubectl delete conversation`, both take this path.
	if conv.DeletionRequestedAt != nil {
		return r.finalizeClose(ctx, &conv)
	}
	if !slices.Contains(conv.Finalizers, FinalizerCloseTopics) {
		if err := state.Patch(ctx, r.Store, &conv, func(c *state.ConversationRecord) {
			c.Finalizers = append(c.Finalizers, FinalizerCloseTopics)
		}); err != nil {
			return ctrl.Result{}, state.IgnoreNotFound(err)
		}
	}

	// CLOSED IS INERT. Everything below this line provisions something — a
	// topic, a ConfigMap, a pod, a work unit — and a closed conversation gets
	// none of it. A closed conversation is not a place work can land, so
	// there is nothing to prune toward either.
	if conv.Phase == state.PhaseClosed {
		return r.reconcileClosed(ctx, &conv)
	}
	runs, err := r.Store.ListRuns(ctx, key)
	if err != nil {
		return ctrl.Result{}, state.IgnoreNotFound(err)
	}

	// The DEADLINE budget edge (design D-E): a Coordinator-rooted conversation
	// past its own snapshotted deadline is closed budget-exceeded — its own
	// members first, then escalated — whatever else is happening on it. This is
	// the one edge the RECONCILER owns rather than a request handler, since
	// nothing else calls in when a conversation simply sits idle past its time.
	//
	// A deadline still ahead does NOT return early: everything below still runs
	// (dispatch, topics, delivery), and deadlineRequeue only CAPS whichever
	// requeue this pass would otherwise choose, so the reconciler is guaranteed
	// to revisit no later than the deadline itself.
	var deadlineRequeue time.Duration
	if budget := conv.Budget; budget != nil && budget.Deadline != nil && r.Router != nil {
		if !budget.Deadline.After(time.Now()) {
			digest := fmt.Sprintf("budget-exceeded: deadline (%s) passed", budget.Deadline.Format(time.RFC3339))
			return ctrl.Result{}, r.Router.CloseBudgetExceeded(ctx, &conv, digest)
		}
		deadlineRequeue = time.Until(*budget.Deadline)
	}

	// input pruning: drop processed inputs from spec, GC consumed payload objects
	if err := r.pruneProcessed(ctx, &conv); err != nil {
		return ctrl.Result{}, err
	}

	pending := dispatch.PendingInputs(&conv)
	needsWorker := dispatch.NeedsWorker(&conv)

	// runtime pod state. Reaping an exited pod comes FIRST: it is what stops
	// the conversation counting against the cap.
	var pod corev1.Pod
	podName := runtimepod.PodName(conv.Name)
	podErr := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: podName}, &pod)
	podExists := podErr == nil

	if podExists && (pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed) {
		// worker exited (idle TTL or crash) — delete; inflight (if any) re-dispatches
		if err := r.Delete(ctx, &pod); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		podExists = false
		if conv.Inflight != nil || conv.RuntimePod != "" {
			if err := state.Patch(ctx, r.Store, &conv, func(c *state.ConversationRecord) {
				c.Inflight = nil
				c.RuntimePod = ""
			}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// A pod that never STARTED is reaped too, and for the same reason the
	// exited one is: it holds a capacity slot it is not using. The difference
	// is that nothing used to reap it, so it held that slot forever — five of
	// them held an entire install for fifteen hours on 2026-08-20.
	//
	// It stays COUNTED until it is gone (liveRuntimePods is untouched):
	// un-counting a pod the cluster has not released is the invent-capacity
	// mistake, and would provision past the cap while the stuck pod still
	// exists. Deleting it is what frees the slot, through the DELETE watch that
	// already promotes the FIFO-first waiter.
	if podExists && runtimeStartOverdue(&pod, r.startDeadline(), time.Now()) {
		if err := r.reapUnstartedPod(ctx, &conv, &pod); err != nil {
			return ctrl.Result{}, err
		}
		podExists = false
	}
	// A pod that DID start clears the backoff and says so, so that a recovered
	// conversation does not keep waiting out an interval it has already earned
	// its way out of.
	if podExists && podStarted(&pod) {
		if err := r.clearRuntimeStartFailure(ctx, &conv); err != nil {
			return ctrl.Result{}, err
		}
	}
	// A node on its way down: hand back the pod while the handing back is still
	// orderly, so the context volume detaches before the machine goes.
	if podExists {
		released, err := r.releaseIfNodeDraining(ctx, &conv, &pod)
		if err != nil {
			return ctrl.Result{}, err
		}
		if released {
			podExists = false
		}
	}

	// Still inside the backoff after a failed start: do not recreate the pod
	// yet. Requeue for exactly as long as is left, so recovery is prompt
	// without polling.
	if !podExists && dispatch.NeedsWorker(&conv) {
		if wait := r.runtimeStartCooldown(&conv, time.Now()); wait > 0 {
			return ctrl.Result{RequeueAfter: wait}, nil
		}
	}

	// CAPACITY FIRST — before anything is provisioned. A conversation that
	// cannot be admitted gets no chat topic, no MCP ConfigMap, no pod and no
	// dispatch: suppressing the TOPIC is the point of the Pending phase, since
	// it is what stops a thousand signals from becoming a thousand chat threads
	// before anyone has looked at the first one. A conversation needing no
	// worker skips the gate entirely — it costs nothing and waits for nothing.
	// STORAGE BEFORE CAPACITY. While context storage is being treated as an
	// outage, provisioning a pod only reproduces the failure and burns the
	// volume's patience — 466 failed attaches in fifteen hours is what that
	// looks like. Hold the work instead: the input stays pending, the
	// conversation stays Pending with a reason that says STORAGE rather than
	// queue, and nothing is failed.
	//
	// One canary is let through per probe interval, because the provisioning
	// edge cannot close its own breaker: with no pod there is no run, and with
	// no run nothing ever reports success.
	if needsWorker && !podExists && r.StorageBreaker != nil {
		if open, since := r.StorageBreaker.Open(); open {
			if !r.StorageBreaker.ProbeDue(time.Now()) {
				return ctrl.Result{RequeueAfter: 30 * time.Second},
					r.enterPendingFor(ctx, &conv, ReasonStorageUnavailable,
						"context storage has been unreachable across this install since "+
							since.Format(time.RFC3339)+"; work is held, not failed")
			}
			logger.Info("letting one canary attempt through to re-test context storage",
				"conversation", conv.Name)
		}
	}

	if needsWorker && !podExists {
		admitted, err := r.admit(ctx, &conv)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !admitted {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.enterPending(ctx, &conv)
		}
	}

	// chat topics: enqueue asynchronously, one ensure-topic per bound channel
	// still missing its thread binding; ids land via op completion (status
	// patch), which re-triggers reconciliation. Requeue as a fallback — op ids
	// are stable per conversation×channel, so re-enqueues dedup.
	topicPending := false
	if r.Ops != nil {
		waiting, err := r.ensureTopics(ctx, &conv)
		if err != nil {
			logger.Error(err, "ensureTopics enqueue (continuing chat-less)")
		}
		topicPending = waiting
		// A reopened conversation announces itself on every bound thread. After
		// ensureTopics on purpose: ops are FIFO per adapter, so the un-archive
		// is claimed before this notice and it lands somewhere that can take
		// it. The op id is stable per reopen, so re-deriving it every pass is
		// what makes it survive a restart mid-reopen without saying it twice.
		if conv.Reopens > 0 && r.Router != nil {
			r.Router.FanOutReopenNotice(ctx, &conv)
		}
		// The escalation digest (design D-D): the ensure-topic enqueue above and
		// the topic actually existing are two different moments, so this waits
		// for a thread before posting — same ordering reason as the reopen
		// notice, and the op id is stable so re-deriving it every pass posts the
		// digest once.
		r.deliverEscalation(ctx, &conv)
		// Input delivery, PARALLEL to dispatch rather than sequenced with it:
		// the human reads the event while the agent is already working, and a
		// run that hangs or dies still leaves the thread saying what happened.
		chat.DeliverInputs(ctx, chat.Env{Reader: r.Client, Store: r.Store, Namespace: r.Namespace}, r.Ops, &conv)
		// The BACKSTOP that makes a reply derivable rather than queue-resident.
		if err := r.deliverRunReplies(ctx, &conv, runs); err != nil {
			logger.Error(err, "re-deriving undelivered run replies")
		}
	}
	// The BACKSTOP for a member's result reaching its parent
	// (coordination-loop): the FAST PATH in `/work/done` marks a run
	// RoutedToParent once its append succeeds, and this re-derives any it
	// missed — a manager restart between the two, or a conflict the fast path
	// gave up retrying. Independent of chat topics entirely: a caused
	// conversation binds no channel, so this must not be gated on r.Ops.
	if r.Router != nil {
		if err := r.deliverMemberResults(ctx, &conv, runs); err != nil {
			logger.Error(err, "re-deriving unrouted member results")
		}
	}

	if needsWorker && !podExists {
		created, err := r.createRuntimePod(ctx, &conv)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !created { // cap re-checked against a fresh list and lost the race
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.enterPending(ctx, &conv)
		}
	}

	// phase bookkeeping
	phase := state.PhaseIdle
	if conv.Inflight != nil {
		phase = state.PhaseWorking
	} else if len(pending) > 0 {
		phase = state.PhaseQueued
	}
	if err := r.setPhase(ctx, &conv, phase); err != nil {
		return ctrl.Result{}, err
	}

	if topicPending {
		return capRequeue(ctrl.Result{RequeueAfter: 15 * time.Second}, deadlineRequeue), nil
	}
	if needsWorker {
		return capRequeue(ctrl.Result{RequeueAfter: 60 * time.Second}, deadlineRequeue), nil
	}
	result, err := r.autoClose(ctx, &conv, runs, podExists)
	return capRequeue(result, deadlineRequeue), err
}

// capRequeue bounds a reconcile result's requeue to at most `extra`, when
// `extra` is set — the mechanism a Coordinator-rooted conversation's own
// deadline uses to guarantee a revisit no later than the deadline itself,
// without pre-empting whatever sooner requeue the rest of this pass already
// chose.
func capRequeue(result ctrl.Result, extra time.Duration) ctrl.Result {
	if extra <= 0 {
		return result
	}
	if result.RequeueAfter <= 0 || extra < result.RequeueAfter {
		result.RequeueAfter = extra
	}
	return result
}

// autoClose closes a FINISHED conversation that has been idle for the window,
// and requeues for the moment it expires when it has not.
//
// Self-scheduled rather than swept: the reconciler is already invoked per
// conversation with exactly the state this decision needs, so a periodic List
// would re-read everything to act on almost nothing — and would produce the
// burst the requeue avoids.
func (r *ConversationReconciler) autoClose(ctx context.Context, conv *state.ConversationRecord,
	runs []state.Run, podExists bool) (ctrl.Result, error) {

	if !r.AutoCloseEnabled || r.AutoCloseIdleAge <= 0 || r.Router == nil {
		return ctrl.Result{}, nil
	}
	if !finished(conv, runs, podExists) {
		return ctrl.Result{}, nil
	}
	idle := time.Since(lastActivity(conv, runs))
	if idle < r.AutoCloseIdleAge {
		return ctrl.Result{RequeueAfter: jitter(r.AutoCloseIdleAge - idle)}, nil
	}
	log.FromContext(ctx).Info("auto-closing an idle finished conversation",
		"conversation", conv.Name, "idleFor", idle.Round(time.Minute), "window", r.AutoCloseIdleAge)
	return ctrl.Result{}, r.Router.AutoCloseConversation(ctx, conv, idle)
}

// finished reports whether a conversation has nothing left to do. It gates
// autoclose and nothing else.
//
// The delivery clause is not decoration: a conversation reaches Idle the
// instant POST /work/done records the result, while the reply may still be an
// unclaimed send op. A long window makes that unlikely, not impossible — an
// adapter down for the length of the window is exactly when it happens — and
// closing then archives the thread out from under the answer.
func finished(conv *state.ConversationRecord, runs []state.Run, podExists bool) bool {
	if conv.Phase != state.PhaseIdle {
		return false
	}
	if dispatch.NeedsWorker(conv) || podExists {
		return false
	}
	return allRunsDelivered(conv, runs)
}

// allRunsDelivered reports whether every recorded run reached every bound
// channel. A conversation with no bound channels is trivially delivered — there
// is no thread for an answer to be owed to.
func allRunsDelivered(conv *state.ConversationRecord, runs []state.Run) bool {
	if len(conv.ChannelRefs) == 0 {
		return true
	}
	for i := range runs {
		run := &runs[i]
		if !run.DeliveryTracked {
			continue // pre-upgrade run: backfilled as delivered, never sent
		}
		for _, ref := range conv.ChannelRefs {
			if !run.DeliveredTo(ref) {
				return false
			}
		}
	}
	return true
}

// lastActivity is the autoclose window's origin: the most recent thing that
// happened, falling back to creation only for a conversation that never ran.
//
// Idle time, never lifetime. A creation clock would close a conversation that
// answered an hour ago simply because it was opened last week, which is the one
// outcome nobody asks for. This is the same instant the console shows as the
// conversation's age, so the list and the behaviour agree.
func lastActivity(conv *state.ConversationRecord, runs []state.Run) time.Time {
	var last time.Time
	note := func(t time.Time) {
		if t.After(last) {
			last = t
		}
	}
	if conv.LastActivity != nil {
		note(*conv.LastActivity)
	}
	for i := range conv.Inputs {
		note(conv.Inputs[i].ReceivedAt)
	}
	for i := range runs {
		if f := runs[i].FinishedAt; f != nil {
			note(*f)
		}
	}
	// Creation is the FALLBACK, not a floor: a conversation that has run is
	// measured from its work, never from when it was opened. Taking the max
	// with creation instead would make a freshly-adopted conversation look busy
	// for a whole window because the object is young.
	if last.IsZero() {
		return conv.CreatedAt
	}
	return last
}

// FinalizerCloseTopics holds a deleting conversation just long enough to
// archive its chat threads.
const FinalizerCloseTopics = "agentops.dev/close-topics"

// DefaultCloseTopicGrace bounds how long deletion may wait on close-topic ops.
// An adapter that is down, or one that does not implement the kind, must never
// wedge a deletion — after this the finalizer is released and the thread is
// left open, which a person can fix by hand.
const DefaultCloseTopicGrace = 2 * time.Minute

func (r *ConversationReconciler) closeGrace() time.Duration {
	if r.CloseTopicGrace > 0 {
		return r.CloseTopicGrace
	}
	return DefaultCloseTopicGrace
}

// startDeadline is how long a runtime pod may sit without reaching Running.
func (r *ConversationReconciler) startDeadline() time.Duration {
	if r.RuntimeStartDeadline > 0 {
		return r.RuntimeStartDeadline
	}
	return DefaultRuntimeStartDeadline
}

// runtimeStartCooldown reports how much of the post-failure backoff is left.
// Derived from the failure count and the last failure stamp — never stored —
// so nothing can disagree about when the next attempt is due.
func (r *ConversationReconciler) runtimeStartCooldown(conv *state.ConversationRecord, now time.Time) time.Duration {
	last := conv.LastRuntimeStartFailure
	if last == nil || conv.RuntimeStartFailures <= 0 {
		return 0
	}
	elapsed := now.Sub(*last)
	if wait := runtimeStartBackoff(conv.RuntimeStartFailures) - elapsed; wait > 0 {
		return wait
	}
	return 0
}

// reapUnstartedPod deletes a runtime pod that never started, records WHY on the
// conversation, and advances the backoff.
//
// The condition message carries the pod's own words. A message reading only
// "start deadline exceeded" would reproduce the very failure this exists to
// end: fifteen hours in which nothing said what was wrong.
func (r *ConversationReconciler) reapUnstartedPod(ctx context.Context, conv *state.ConversationRecord, pod *corev1.Pod) error {
	fail := classifyStuckPod(pod)
	logger := log.FromContext(ctx)
	logger.Info("reaping a runtime pod that never started",
		"conversation", conv.Name, "pod", pod.Name,
		"reason", fail.Reason, "storageAttributable", fail.Storage, "evidence", fail.Message)

	// GracePeriodSeconds(0), as every other runtime-pod release does: the pod
	// never started, so there is no process to drain — and a pod lingering
	// through a grace period is a pod still holding the slot this reap exists
	// to free.
	if err := r.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if r.Recorder != nil {
		r.Recorder.Event(r.eventRef(conv), corev1.EventTypeWarning, fail.Reason,
			"runtime pod "+pod.Name+" never started: "+fail.Message)
	}
	// ONLY a storage-attributable failure is evidence about storage. An
	// unschedulable pod or an unpullable image is a different fault, and
	// letting either open this breaker would hold every conversation in the
	// install for a reason that has nothing to do with storage.
	if fail.Storage && r.StorageBreaker != nil {
		if opened := r.StorageBreaker.Report(); opened {
			logger.Info("treating context storage as unavailable across the install; "+
				"holding work rather than failing it", "conversation", conv.Name)
		}
	}

	return state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.RuntimePod = ""
		// Inflight is NOT cleared here. A pod that never started ran nothing, so
		// there is no work to re-dispatch and nothing to reconcile away; the input
		// is still pending and will be dispatched to the replacement pod.
		c.RuntimeStartFailures++
		now := time.Now().UTC()
		c.LastRuntimeStartFailure = &now
		c.SetCondition(state.Condition{
			Type:    ConditionRuntimeStarted,
			Status:  state.ConditionFalse,
			Reason:  fail.Reason,
			Message: fail.Message,
		})
	})
}

// eventRef names a conversation for a Kubernetes event. An event is a report
// and never a control path, so a reference is all it needs — the record is not
// a Kubernetes object under every backend.
func (r *ConversationReconciler) eventRef(conv *state.ConversationRecord) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		APIVersion: agentopsv1alpha1.GroupVersion.String(), Kind: conversationKind,
		Namespace: r.Namespace, Name: conv.Name, UID: types.UID(conv.ID),
	}
}

// conversationKind is the kind an owner reference or event reference names.
const conversationKind = "Conversation"

// ownerOf is the owner a runtime pod or ConfigMap carries a controller
// reference to. Built from the record's identity alone (design D-7): the
// owner link is the second net under explicit release, and it only needs the
// kind, name and UID, not a Kubernetes object to read them from.
func (r *ConversationReconciler) ownerOf(conv *state.ConversationRecord) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: agentopsv1alpha1.GroupVersion.String(), Kind: conversationKind},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.Namespace, Name: conv.Name, UID: types.UID(conv.ID),
		},
	}
}

// clearRuntimeStartFailure records that a runtime pod finally started, resetting
// the backoff. Writes only on an actual change, so a healthy conversation does
// not spend a status patch per reconcile.
func (r *ConversationReconciler) clearRuntimeStartFailure(ctx context.Context, conv *state.ConversationRecord) error {
	existing := conv.FindCondition(ConditionRuntimeStarted)
	if conv.RuntimeStartFailures == 0 && conv.LastRuntimeStartFailure == nil &&
		existing != nil && existing.Status == state.ConditionTrue {
		return nil
	}
	return state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.RuntimeStartFailures = 0
		c.LastRuntimeStartFailure = nil
		// Only conversations that have actually failed to start carry this
		// condition. A conversation whose pods have always come up stays
		// condition-free rather than accumulating a permanent "everything is fine".
		if c.FindCondition(ConditionRuntimeStarted) != nil {
			c.SetCondition(state.Condition{
				Type:    ConditionRuntimeStarted,
				Status:  state.ConditionTrue,
				Reason:  ReasonStarted,
				Message: "runtime pod is running",
			})
		}
	})
}

// reconcileClosed is the whole of a closed conversation's lifecycle: tear down
// what costs something, archive the threads, and stop.
//
// The teardown is the SAME teardown deletion used to get for free through
// ownerRef GC — the runtime pod and the MCP ConfigMap — done explicitly now
// that the object survives its close. Releasing the pod is what returns the
// capacity slot, so a Pending conversation is admitted on the next pass.
//
// Archiving here rather than in the finalizer is what retires close-topic's
// status as the one non-derivable op. It was the exception only because it was
// enqueued while the object was disappearing; the object now survives, so a
// thread missing from status.threadsArchived is an archive still owed, and this
// pass re-derives it after any restart. The op id is stable per
// conversation×channel, so enqueueing on every pass dedups.
func (r *ConversationReconciler) reconcileClosed(ctx context.Context, conv *state.ConversationRecord) (ctrl.Result, error) {
	if err := r.releaseWorkload(ctx, conv); err != nil {
		return ctrl.Result{}, err
	}
	if conv.Inflight != nil || conv.RuntimePod != "" {
		if err := state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
			c.Inflight = nil
			c.RuntimePod = ""
		}); err != nil {
			return ctrl.Result{}, err
		}
	}

	if r.Ops != nil {
		for _, t := range conv.Threads {
			if conv.ThreadArchived(t.Channel) {
				continue
			}
			if t.ThreadID == "" {
				// No topic was ever created on this channel — ensure-topic
				// never succeeded. There is nothing to close yet, and an op
				// carrying an empty thread id can only ever be rejected
				// (every adapter's close-topic handler validates it), so
				// enqueueing it would retry forever and log an error every
				// pass. Leave it unarchived: the next pass re-evaluates, and
				// a later successful ensure-topic enqueues close-topic
				// normally once a real thread id exists.
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: t.Channel}, &ch); err != nil {
				continue // channel gone: nothing left to archive on it
			}
			if ch.Spec.Adapter == "" {
				continue
			}
			r.Ops.EnqueueCloseTopic(ctx, &ch, t.ThreadID, conv)
		}
	}
	return r.autoDelete(ctx, conv)
}

// releaseWorkload deletes a conversation's runtime pod and MCP ConfigMap,
// explicitly, on close and on delete (state-store design D-7). The owner
// reference SetControllerReference puts on both stays as a second net, but
// it only works while the owner is a Kubernetes object — and envtest runs no
// garbage collector — so the release must not depend on it.
//
// Grace 0: the run, if any, was abandoned and nobody is waiting for its
// output, so a graceful shutdown would only hold the capacity slot longer.
// Absence is not an error: both are routinely already gone.
func (r *ConversationReconciler) releaseWorkload(ctx context.Context, conv *state.ConversationRecord) error {
	pod := &corev1.Pod{}
	pod.Namespace, pod.Name = r.Namespace, runtimepod.PodName(conv.Name)
	if err := r.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	cm := &corev1.ConfigMap{}
	cm.Namespace, cm.Name = r.Namespace, convMCPConfigMapName(conv.Name)
	if err := r.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// autoDelete reclaims a conversation that has been CLOSED for longer than the
// window, and requeues for the moment it expires when it has not.
//
// Measured from status.closedAt, so a reopen — which clears that stamp — stops
// the clock. Applies to Closed conversations only: a never-closed conversation
// is never auto-deleted however idle it is, because the two stages are separate
// decisions with separate flags.
func (r *ConversationReconciler) autoDelete(ctx context.Context, conv *state.ConversationRecord) (ctrl.Result, error) {
	if !r.AutoDeleteEnabled || r.AutoDeleteClosedAge <= 0 || conv.ClosedAt == nil {
		return ctrl.Result{}, nil
	}
	elapsed := time.Since(*conv.ClosedAt)
	if elapsed < r.AutoDeleteClosedAge {
		return ctrl.Result{RequeueAfter: jitter(r.AutoDeleteClosedAge - elapsed)}, nil
	}
	log.FromContext(ctx).Info("auto-deleting a long-closed conversation",
		"conversation", conv.Name, "closedFor", elapsed.Round(time.Minute), "window", r.AutoDeleteClosedAge)
	return ctrl.Result{}, r.Store.Delete(ctx, state.Key(conv.Name))
}

// jitter spreads a requeue by up to 10%, so an install whose conversations all
// become eligible at manager start does not act on them in one instant. The
// first pass on an established backlog is the dangerous one: every close
// enqueues a farewell and a close-topic op per bound thread.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	return d + time.Duration(rand.Int63n(int64(d/10)+1))
}

// finalizeClose tells a deleting conversation's threads that it is gone, then
// lets go.
//
// EVERY deletion takes this path — autodelete, a surface's delete verb, and a
// direct `kubectl delete` — and every one of them owes the bound threads a
// correction, because a closed conversation's last word to them was that it
// could be reopened.
//
// Enqueueing on every pass is safe — the op id is stable per
// conversation×channel, so it dedups against both the pending op and the
// completed one.
func (r *ConversationReconciler) finalizeClose(ctx context.Context, conv *state.ConversationRecord) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !slices.Contains(conv.Finalizers, FinalizerCloseTopics) {
		return ctrl.Result{}, nil
	}

	// Free the slot now rather than at the end of the grace: /close on a
	// working conversation abandons the run, and a pod nobody is waiting on
	// must not hold capacity for two more minutes.
	if err := r.releaseWorkload(ctx, conv); err != nil {
		return ctrl.Result{}, err
	}

	outstanding := false
	if r.Ops != nil {
		for _, t := range conv.Threads {
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: t.Channel}, &ch); err != nil {
				continue // channel gone: nobody left to tell
			}
			if ch.Spec.Adapter == "" {
				continue
			}
			// delete-conversation, NOT close-topic — whether or not this
			// conversation was closed first. A closed conversation's threads
			// were told it could be reopened; deletion makes that false, and
			// correcting it is the whole point. Sending both would leave an
			// adapter guessing whether the pair meant one ending or two.
			r.Ops.EnqueueDeleteConversation(ctx, &ch, t.ThreadID, conv)
			if r.Ops.Pending(chat.DeleteConversationOpID(conv.Name, t.Channel)) {
				outstanding = true
			}
		}
	}
	if outstanding && time.Since(*conv.DeletionRequestedAt) < r.closeGrace() {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if outstanding {
		logger.Info("close-topic grace expired; releasing the conversation with threads still open",
			"conversation", conv.Name)
	}
	err := state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.Finalizers = slices.DeleteFunc(c.Finalizers, func(f string) bool { return f == FinalizerCloseTopics })
	})
	return ctrl.Result{}, state.IgnoreNotFound(err)
}

// liveRuntimePods lists the runtime pods that consume capacity: Running or
// Pending. A pod that has exited holds nothing.
func (r *ConversationReconciler) liveRuntimePods(ctx context.Context, ns string) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(ns),
		client.MatchingLabels{runtimepod.LabelApp: runtimepod.LabelAppValue}); err != nil {
		return nil, err
	}
	var live []corev1.Pod
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending {
			live = append(live, p)
		}
	}
	return live, nil
}

// admit decides whether this conversation may take a capacity slot now.
//
// Both halves come from the same two lists every conversation reads — the live
// runtime pods and the conversations waiting for one — so admission order is
// stable across reconciles without a leader-elected scheduler or a queue
// object. Order is FIFO by creation time, full stop: no priority, no fairness
// classes between pipelines.
//
// A slot counts as free when the cap is not reached, or when a live pod is
// EVICTABLE (its conversation has nothing inflight and nothing queued) —
// createRuntimePod does the eviction, and admitting on that basis is what keeps
// a burst from blocking behind a worker that is doing nothing.
func (r *ConversationReconciler) admit(ctx context.Context, conv *state.ConversationRecord) (bool, error) {
	live, err := r.liveRuntimePods(ctx, r.Namespace)
	if err != nil {
		return false, err
	}
	hasPod := map[string]bool{}
	for _, p := range live {
		hasPod[p.Labels[runtimepod.LabelConversation]] = true
	}

	free := r.MaxActiveConversations - len(live)
	if free <= 0 {
		free = r.evictableCount(ctx, live)
	}
	if free <= 0 {
		return false, nil
	}

	all, err := r.Store.List(ctx, state.ByAll, state.ListKey{})
	if err != nil {
		return false, err
	}
	now := time.Now()
	// The waiting set is defined by PODS, not by phase: a conversation that
	// needs a worker and holds none is waiting, whether it has been marked
	// Pending yet or has only just been created. Keying on phase would let a
	// brand-new conversation reconciled first jump an older one.
	var waiting []state.ConversationRecord
	for i := range all {
		c := &all[i]
		if c.DeletionRequestedAt != nil || hasPod[c.Name] || !dispatch.NeedsWorker(c) {
			continue
		}
		// Closed is inert: it will never be given a pod, so leaving it in the
		// waiting set would let a conversation nobody will ever admit sit ahead
		// of a Pending one in FIFO order and starve it. Its inputs, if it was
		// closed holding any, stay on the object untouched — they are part of
		// the record a reopen restores, not work owed.
		if c.Phase == state.PhaseClosed {
			continue
		}
		// A conversation still serving out a start-failure backoff will not be
		// given a pod on this pass, so it must not hold the FIFO head either.
		// Leaving it there reproduces the outage this whole mechanism exists to
		// end, one layer up: the oldest conversation cannot start, and every
		// conversation behind it waits on a slot nobody is going to take.
		if r.runtimeStartCooldown(c, now) > 0 {
			continue
		}
		waiting = append(waiting, *c)
	}
	sort.Slice(waiting, func(i, j int) bool {
		if !waiting[i].CreatedAt.Equal(waiting[j].CreatedAt) {
			return waiting[i].CreatedAt.Before(waiting[j].CreatedAt)
		}
		return waiting[i].Name < waiting[j].Name // stable tiebreak within a second
	})
	for i := range waiting {
		if waiting[i].Name == conv.Name {
			return i < free, nil
		}
	}
	// Not in the list (a stale cache read of our own inputs) — the fresh
	// re-check in createRuntimePod is the backstop.
	return len(waiting) < free, nil
}

// evictableCount reports how many live pods could be freed for waiting work:
// those whose conversation has nothing inflight and nothing queued. Eviction
// deletes only the pod — the conversation and its session survive and it gets a
// fresh pod on its next input.
func (r *ConversationReconciler) evictableCount(ctx context.Context, live []corev1.Pod) int {
	n := 0
	for _, p := range live {
		c, _, err := r.Store.Get(ctx, state.Key(p.Labels[runtimepod.LabelConversation]))
		if err != nil {
			continue
		}
		if !dispatch.NeedsWorker(&c) {
			n++
		}
	}
	return n
}

// enterPending parks an unadmitted conversation and tells the person waiting,
// ONCE. The phase is patched before the notice so a reconcile storm cannot
// produce a second one; a notice lost to a crash in that window is preferable
// to a channel repeating itself every 30 seconds.
// ReasonStorageUnavailable marks a conversation held because context storage
// is unreachable, as opposed to one waiting for a capacity slot.
//
// The two look identical from outside — a queue that has stopped moving — and
// demand opposite responses: one is "wait", the other is "your storage is
// broken". Fifteen hours were lost to not being able to tell them apart.
const ReasonStorageUnavailable = "StorageUnavailable"

// enterPendingFor holds a conversation in Pending and records WHY on the
// RuntimeStarted condition, so a queue that has stopped moving explains itself
// where an operator is already looking.
func (r *ConversationReconciler) enterPendingFor(ctx context.Context, conv *state.ConversationRecord, reason, message string) error {
	// Written only on an actual change: Mutate skips a write that changes nothing.
	if err := state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.SetCondition(state.Condition{
			Type:    ConditionRuntimeStarted,
			Status:  state.ConditionFalse,
			Reason:  reason,
			Message: message,
		})
	}); err != nil {
		return err
	}
	return r.enterPending(ctx, conv)
}

func (r *ConversationReconciler) enterPending(ctx context.Context, conv *state.ConversationRecord) error {
	if conv.Phase == state.PhasePending {
		return nil
	}
	if err := r.setPhase(ctx, conv, state.PhasePending); err != nil {
		return err
	}
	r.notifyQueued(ctx, conv)
	return nil
}

// notifyQueued posts one "waiting for capacity" notice for a conversation that
// just entered Pending. A pending conversation usually has NO thread — that is
// what the phase suppresses — so the notice goes to the originating channel's
// general surface; a conversation that already earned a thread on an earlier
// admission is told there instead, where the person is looking.
func (r *ConversationReconciler) notifyQueued(ctx context.Context, conv *state.ConversationRecord) {
	if r.Ops == nil || len(conv.ChannelRefs) == 0 {
		return
	}
	ref := conv.ChannelRefs[0]
	var ch agentopsv1alpha1.Channel
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ref}, &ch); err != nil || ch.Spec.Adapter == "" {
		return
	}
	r.Ops.EnqueueMessage(ctx, &ch, conv, conv.ThreadFor(ref), chat.Notice(
		"⏳ Queued for capacity — every agent slot is busy. This starts as soon as one frees up."))
}

func (r *ConversationReconciler) setPhase(ctx context.Context, conv *state.ConversationRecord, phase string) error {
	if conv.Phase == phase {
		return nil
	}
	return state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) { c.Phase = phase })
}

// ensureTopics enqueues topic creation for every bound channel lacking a
// thread binding; reports whether any are still pending. Dangling channelRefs
// are skipped (that channel stays chat-less) without blocking the others.
func (r *ConversationReconciler) ensureTopics(ctx context.Context, conv *state.ConversationRecord) (bool, error) {
	pending := false
	var firstErr error
	for _, ref := range conv.ChannelRefs {
		existing := conv.ThreadFor(ref)
		archived := conv.ThreadArchived(ref)
		// A thread that exists and is not archived is a live thread: nothing to
		// do. A thread that exists and IS archived belongs to a conversation
		// that was closed and reopened, and needs re-establishing — carrying
		// its old id as a hint the adapter may honour or ignore.
		if existing != nil && !archived {
			continue
		}
		var ch agentopsv1alpha1.Channel
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ref}, &ch); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ch.Spec.Adapter == "" {
			continue
		}
		pending = true
		// durable-chat-ops-broker: only a channel binding with no LIVE claim —
		// none at all, one held by a former leader, or one of this leader's own
		// gone stale — is dispatched this pass. One already claimed and fresh is
		// an op genuinely in flight with an adapter; re-enqueuing it here would
		// not change what the in-memory queue already holds, but it would race
		// the claim write against nothing, for no reason. See claimEnsureTopic.
		claimed, err := r.claimEnsureTopic(ctx, conv.Name, ref)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !claimed {
			continue
		}
		d := r.topicDescriptor(ctx, conv)
		if existing != nil {
			d.PreviousThreadID = *existing
		}
		r.Ops.EnqueueEnsureTopic(ctx, &ch, conv, d)
	}
	return pending, firstErr
}

// claimEnsureTopic writes (or refreshes) THIS leader's claim on one channel
// binding's ensure-topic dispatch, so a crashed leader's in-flight work is
// recoverable from the Conversation's own status rather than from a second,
// hand-rolled heartbeat (durable-chat-ops-broker).
//
// Returns claimed=true when the caller should (re-)dispatch the op now: no
// claim existed, the existing one belongs to a replica that is not this one
// (a former leader — this reconciler runs only while its process holds the
// leader-election Lease, so any claim it did not itself write is by
// construction a former leader's), or this leader's own claim has sat past
// the staleness bound with no thread yet. Returns false when a live claim
// already held by this leader is outstanding — the op is genuinely in flight
// with an adapter and nothing new is dispatched this pass.
//
// Does its own fresh read-and-swap retry rather than reusing the caller's
// (possibly several-reconcile-functions-old) copy: every other writer in this
// file that touches the conversation does the same, for the same
// optimistic-concurrency reason (see finishEnsureTopic, markDelivered).
func (r *ConversationReconciler) claimEnsureTopic(ctx context.Context, name, channel string) (bool, error) {
	claimed := false
	_, err := state.Mutate(ctx, r.Store, state.Key(name), func(conv *state.ConversationRecord) error {
		claimed = false
		if r.holdsLiveClaim(conv, channel) {
			return nil
		}
		r.setClaim(conv, channel)
		claimed = true
		return nil
	})
	if err != nil {
		if errors.Is(err, state.ErrConflict) {
			return false, fmt.Errorf("conflict claiming ensure-topic for %s on conversation %s: %w", channel, name, err)
		}
		return false, state.IgnoreNotFound(err)
	}
	return claimed, nil
}

// holdsLiveClaim reports whether THIS replica's claim on the channel's
// ensure-topic is still inside the staleness bound.
func (r *ConversationReconciler) holdsLiveClaim(conv *state.ConversationRecord, channel string) bool {
	existing := conv.Thread(channel)
	if existing == nil || existing.Claim == nil {
		return false
	}
	return existing.Claim.Holder == r.ReplicaIdentity &&
		time.Since(existing.Claim.ClaimedAt) <= r.claimStaleness()
}

// setClaim stamps this replica's claim on the channel's binding, creating the
// binding when none exists yet.
func (r *ConversationReconciler) setClaim(conv *state.ConversationRecord, channel string) {
	claim := &state.OpClaim{Holder: r.ReplicaIdentity, ClaimedAt: time.Now().UTC()}
	if binding := conv.Thread(channel); binding != nil {
		binding.Claim = claim
		return
	}
	conv.Threads = append(conv.Threads, state.ThreadBinding{Channel: channel, Claim: claim})
}

// deliverEscalation posts the digest an `escalate` call snapshotted
// (status.escalationMessage) into every bound channel's already-open thread
// (coordinator-unconditional-channels: a coordinator root's channels are
// bound at creation, so this is no longer a thread's opening post, merely
// the decision made visible on one already running). A conversation that
// never escalated, or whose threads are not established yet, gets nothing —
// Ops itself is what makes a second delivery a no-op.
func (r *ConversationReconciler) deliverEscalation(ctx context.Context, conv *state.ConversationRecord) {
	if conv.EscalatedAt == nil || r.Ops == nil {
		return
	}
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueEscalationMessage(ctx, ch, conv, tid, chat.Notice(conv.EscalationMessage))
	})
}

// eachBoundThread mirrors chat.Router's own helper of the same shape: every
// bound channel that already has a thread, so a send to one that does not
// exist yet is never queued.
func (r *ConversationReconciler) eachBoundThread(ctx context.Context, conv *state.ConversationRecord,
	skip string, fn func(ch *agentopsv1alpha1.Channel, threadID *string)) {

	for _, ref := range conv.ChannelRefs {
		if ref == skip {
			continue
		}
		tid := conv.ThreadFor(ref)
		if tid == nil {
			continue
		}
		var ch agentopsv1alpha1.Channel
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ref}, &ch); err != nil {
			continue
		}
		fn(&ch, tid)
	}
}

// topicDescriptor describes a conversation's thread for the adapter to NAME.
// The manager supplies facts — route, source, title, labels, lane — and no
// formatting: Telegram caps forum topics at 128 characters and takes no markup,
// a web chat has neither limit, and neither constraint belongs here.
func (r *ConversationReconciler) topicDescriptor(ctx context.Context, conv *state.ConversationRecord) chat.TopicDescriptor {
	d := chat.TopicDescriptor{
		Conversation: conv.Name,
		Title:        conv.Title,
		Pipeline:     r.inferredPipeline(ctx, conv),
	}
	// The FIRST input is what opened the conversation, so it names the lane and
	// the source the topic is about; later recurrences share both.
	for i := range conv.Inputs {
		if o := conv.Inputs[i].Origin; o != nil {
			d.Source, d.Kind = o.Name, o.SignalKind
			if payload, ok := chat.InputPayload(ctx, r.Store, &conv.Inputs[i]); ok {
				d.Labels = payload.Labels
			}
			break
		}
	}
	return d
}

// inferredPipeline names the route that originated a conversation, or "" when
// the bindings are ambiguous. Conversations record no pipelineRef on purpose —
// this is the same inference /status and the console already use, and a blank
// answer is the honest one rather than a guess.
func (r *ConversationReconciler) inferredPipeline(ctx context.Context, conv *state.ConversationRecord) string {
	if p := chat.PipelineForConversation(ctx, r.Client, r.Namespace, conv); p != nil {
		return p.Name
	}
	return ""
}

// deliverRunReplies re-derives the answer for every completed run that a bound
// thread has not received. It is the backstop that makes `send` derivable:
// `POST /work/done` enqueues the reply on the fast path, but the queue is
// in-memory, so without this a manager restart in the window between recording
// the result and an adapter claiming the op would lose the answer permanently —
// durably written to status.runs[].result and delivered to nobody.
//
// Two facts keep it safe to run on EVERY reconcile:
//
//   - the op id is stable per conversation×channel×run, so re-enqueues dedup
//     against the pending map and the completed window, and the CR's markers
//     cover the window after a restart, when both are empty;
//   - a run written by a manager that did NOT track delivery is BACKFILLED as
//     delivered rather than sent (see RunStatus.DeliveryTracked). That is the
//     one migration hazard: without it, upgrading would re-post every recent
//     answer to every bound thread.
func (r *ConversationReconciler) deliverRunReplies(ctx context.Context, conv *state.ConversationRecord,
	runs []state.Run) error {

	if len(conv.ChannelRefs) == 0 || len(runs) == 0 {
		return nil
	}
	// Threads, not channelRefs: a channel whose topic does not exist yet has
	// nowhere to receive the reply, and the reconcile its binding triggers will
	// come back here.
	var backfill []string
	owed := map[string]bool{}
	for i := range runs {
		run := &runs[i]
		if !run.DeliveryTracked {
			backfill = append(backfill, run.RunID)
			continue
		}
		msg := chat.RunReplyMessage(run)
		for _, t := range conv.Threads {
			if !conv.BoundTo(t.Channel) {
				continue
			}
			if run.DeliveredTo(t.Channel) {
				// A crash between markDelivered's run write and its note
				// write leaves the note naming a delivered run; this
				// backstop is the only pass that visits it.
				chat.SettleDeliveredNote(ctx, r.Store, conv, t.Channel, run.RunID)
				continue
			}
			owed[t.Channel] = true
			// durable-chat-ops-broker: a binding carrying only a claim — no
			// ThreadID yet — is bookkeeping about a dispatch in flight, not a
			// topic. EnqueueRunReply would still accept the empty id and the
			// adapter would park the reply on a channel-level pseudo-thread,
			// marking it delivered there PERMANENTLY: once DeliveredTo is
			// true this run is never re-enqueued, so the real thread
			// ensureTopics creates moments later would never receive the
			// answer the person is waiting for. Leaving it owed (above) is
			// what brings this reconcile back once the thread exists.
			if t.ThreadID == "" {
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: t.Channel}, &ch); err != nil ||
				ch.Spec.Adapter == "" {
				continue
			}
			tid := t.ThreadID
			r.Ops.EnqueueRunReply(ctx, &ch, conv, run.RunID, &tid, msg)
		}
	}
	r.setDeliveryPendingCondition(ctx, conv, owed)
	if len(backfill) == 0 {
		return nil
	}
	return r.backfillDelivered(ctx, conv, backfill, runs)
}

// deliverMemberResults is the reconciler backstop for coordination-loop's
// "member's result becomes an input on its parent" rule: any run on THIS
// conversation not yet marked RoutedToParent gets appended again (idempotent
// per its dedup id) and marked once that succeeds.
func (r *ConversationReconciler) deliverMemberResults(ctx context.Context, conv *state.ConversationRecord,
	runs []state.Run) error {

	if conv.CausedBy == nil {
		return nil
	}
	var firstErr error
	for i := range runs {
		run := &runs[i]
		if run.RoutedToParent {
			continue
		}
		dedup := "member:" + conv.Name + ":" + run.RunID
		if err := r.Router.AppendMemberResult(ctx, conv.CausedBy, conv.Name, dedup, run.Result); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := r.Store.UpdateRun(ctx, state.Key(conv.Name), run.RunID,
			state.RunUpdate{MarkRoutedToParent: true}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// setDeliveryPendingCondition names the channels still owed an answer.
//
// True carries the channel names, because "delivery is pending" without saying
// where is not actionable — one wedged transport among three bound channels
// looks identical to all three being down.
func (r *ConversationReconciler) setDeliveryPendingCondition(ctx context.Context,
	conv *state.ConversationRecord, owed map[string]bool) {

	cond := state.Condition{
		Type: ConditionDeliveryPending, Status: state.ConditionFalse,
		Reason: "AllDelivered", Message: "every recorded run has reached every bound thread",
	}
	if len(owed) > 0 {
		names := make([]string, 0, len(owed))
		for c := range owed {
			names = append(names, c)
		}
		sort.Strings(names) // stable: a condition that reshuffles reads as a change
		cond.Status = state.ConditionTrue
		cond.Reason = "Undelivered"
		cond.Message = "answers recorded but not yet posted to: " + strings.Join(names, ", ")
	}
	// Nothing to say about a conversation that has never produced an answer.
	if cond.Status == state.ConditionFalse && conv.FindCondition(ConditionDeliveryPending) == nil {
		return
	}
	// Unchanged writes nothing: Mutate does not spend a write per reconcile.
	if err := state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) { c.SetCondition(cond) }); err != nil {
		log.FromContext(ctx).Error(err, "patching DeliveryPending condition", "conversation", conv.Name)
	}
}

// backfillDelivered records pre-upgrade runs as delivered to every bound
// channel WITHOUT enqueueing anything. Their replies were posted by the manager
// that ran them; the markers simply did not exist yet.
func (r *ConversationReconciler) backfillDelivered(ctx context.Context, conv *state.ConversationRecord,
	runIDs []string, runs []state.Run) error {

	byID := map[string]*state.Run{}
	for i := range runs {
		byID[runs[i].RunID] = &runs[i]
	}
	for _, id := range runIDs {
		run := byID[id]
		if run == nil || run.DeliveryTracked {
			continue
		}
		// Tracked from here on, and already delivered as far as anyone can
		// tell — which is the honest record: this manager never owed it.
		err := r.Store.UpdateRun(ctx, state.Key(conv.Name), id, state.RunUpdate{
			MarkDeliveryTracked: true, MarkDelivered: conv.ChannelRefs,
		})
		if err != nil && !errors.Is(err, state.ErrNotFound) {
			return fmt.Errorf("backfilling delivery markers on %s: %w", conv.Name, err)
		}
	}
	return nil
}

// pruneProcessed removes queue entries dispatch has consumed, and deletes the
// out-of-line payload object with them. UNCHANGED, deliberately: pruning is
// what stops answered work from running twice, and that is all it was ever for.
//
// What changed is what it is allowed to be the last copy of. The RECORD is
// written into status.runs[].inputs by the same write that marks these ids
// processed (see httpapi.handleWorkDone), so by the time an id can appear in
// ProcessedInputIDs its message is already durable. That ordering is the
// guarantee this depends on, and it is one-directional: recording afterwards
// would let a crash in between destroy what somebody said, with the queue entry
// gone and nothing to re-read it from.
func (r *ConversationReconciler) pruneProcessed(ctx context.Context, conv *state.ConversationRecord) error {
	if len(conv.ProcessedInputIDs) == 0 {
		return nil
	}
	done := map[string]bool{}
	for _, id := range conv.ProcessedInputIDs {
		done[id] = true
	}
	var keep []state.InputItem
	var prune []state.InputItem
	for _, in := range conv.Inputs {
		if done[in.ID] {
			prune = append(prune, in)
		} else {
			keep = append(keep, in)
		}
	}
	if len(prune) == 0 {
		return nil
	}
	if err := state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.Inputs = slices.DeleteFunc(c.Inputs, func(in state.InputItem) bool { return done[in.ID] })
	}); err != nil {
		return err
	}
	for _, in := range prune {
		if in.PayloadRef != "" {
			if err := r.Store.DeleteBlob(ctx, in.PayloadRef); err != nil {
				return err
			}
		}
	}
	return nil
}

// createRuntimePod re-checks the conversation cap against a FRESH pod list —
// admission decided against a possibly stale cache, so this is the last word —
// and evicts an idle worker when that is what a slot costs. Returns
// created=false when every slot is held by a busy conversation.
func (r *ConversationReconciler) createRuntimePod(ctx context.Context, conv *state.ConversationRecord) (bool, error) {
	logger := log.FromContext(ctx)

	live, err := r.liveRuntimePods(ctx, r.Namespace)
	if err != nil {
		return false, err
	}
	if len(live) >= r.MaxActiveConversations {
		// evict the longest-idle worker whose conversation has nothing queued
		type cand struct {
			pod  corev1.Pod
			last time.Time
		}
		var cands []cand
		for _, p := range live {
			cn := p.Labels[runtimepod.LabelConversation]
			c, _, err := r.Store.Get(ctx, state.Key(cn))
			if err != nil {
				continue
			}
			if !dispatch.NeedsWorker(&c) {
				last := c.CreatedAt
				if c.LastActivity != nil {
					last = *c.LastActivity
				}
				cands = append(cands, cand{p, last})
			}
		}
		if len(cands) == 0 {
			logger.Info("at the active-conversation cap, all busy — pending",
				"conversation", conv.Name, "cap", r.MaxActiveConversations)
			return false, nil
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].last.Before(cands[j].last) })
		logger.Info("evicting idle worker to make room", "pod", cands[0].pod.Name)
		if err := r.Delete(ctx, &cands[0].pod); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
	}

	var profile agentopsv1alpha1.AgentProfile
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: conv.ProfileRef}, &profile); err != nil {
		return false, err
	}
	mcpRes, mcpCM, err := r.ensureMCPConfigMap(ctx, conv)
	if err != nil {
		// A binding that cannot resolve stays visible on the conversation and
		// the pod is not created with silently reduced capability.
		reason := "MCPResolutionFailed"
		var rawExclusive *mcpcompile.RawExclusiveError
		if errors.As(err, &rawExclusive) {
			reason = "RawConfigNotExclusive"
		}
		r.setToolingCondition(ctx, conv, state.ConditionFalse, reason, err.Error())
		return false, err
	}
	r.setToolingCondition(ctx, conv, state.ConditionTrue, "Resolved", "")

	// resolve the execution backend from the conversation's OWN snapshot:
	// conv.runtimeRef -> profile.runtimeRef (deprecated) -> "default" CR ->
	// bootstrap, with conv.serviceAccountName overriding the runtime's identity.
	// The Pipeline is deliberately absent — its choice was resolved into these
	// fields at creation, so editing it cannot re-wire this pod.
	resolved, err := runtimepod.ResolveFor(ctx, r.Client, r.Store, r.Namespace, conv, r.Runtime)
	if err != nil {
		return false, err
	}

	// A KNOWN-BAD volume costs CONTINUITY, not availability.
	//
	// Once the same conversation has failed to start repeatedly for a
	// storage-attributable reason, waiting longer buys nothing: the volume is
	// not coming back on its own, and every further attempt reproduces the same
	// unattachable mount. Starting WITHOUT the volume lets the agent answer —
	// and the conversation says it lost its memory, because a run that silently
	// answered fresh is the degradation the continuity rules forbid.
	//
	// Deliberately NOT triggered by the install-wide breaker: that one HOLDS
	// work, which is right while an outage may still be transient. This is the
	// end of that road, for one conversation that has exhausted its retries.
	degraded := r.contextLost(conv)
	if degraded {
		resolved.Config.ContextPVC = ""
		resolved.ContextSync = nil
		logger.Info("starting a runtime pod WITHOUT its context volume; the conversation has lost its memory",
			"conversation", conv.Name, "failures", conv.RuntimeStartFailures)
	}

	pod := runtimepod.Build(conv, &profile, mcpRes, mcpCM, resolved, r.MasterToken)
	pod.Namespace = r.Namespace
	if err := controllerutil.SetControllerReference(r.ownerOf(conv), pod, r.Scheme); err != nil {
		return false, err
	}
	if err := r.Create(ctx, pod); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	}
	// The pod exists; everything from here until the agent's first /work is
	// scheduling and boot, and used to be a silent gap in the sequence.
	r.Activity.Emit(activity.Event{
		Kind:         activity.KindRuntimeStarting,
		Status:       activity.StatusOK,
		Conversation: conv.Name,
		From:         &activity.NodeRef{Kind: activity.NodeConversation, Name: conv.Name},
		To:           &activity.NodeRef{Kind: activity.NodeRuntime, Name: pod.Name},
		Detail:       "runtime pod created" + degradedSuffix(degraded),
	})

	return true, state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.RuntimePod = pod.Name
		// What mediation actually covers, judged from the endpoints just compiled.
		// A conversation with one unenforceable endpoint is not mostly mediated, and
		// an operator who enabled the feature has every reason to think otherwise
		// unless told.
		applyMediationCondition(c, mediationCondition(mcpRes, resolved.EgressMediation))
		if degraded {
			// The handle names a context this pod cannot reach. Clearing it is what
			// stops the next run failing a continuation that can never succeed —
			// and the condition is what stops the loss being silent.
			c.RuntimeContextID = ""
			c.SessionID = ""
			c.SetCondition(state.Condition{
				Type:   ConditionContextContinuity,
				Status: state.ConditionFalse,
				Reason: "ContextVolumeUnusable",
				Message: "the context volume could not be attached after repeated attempts; " +
					"this conversation is running without its previous memory",
			})
		}
	})
}

// degradedStartAfter is how many consecutive storage-attributable start
// failures mean the volume is not coming back for this conversation.
//
// Generous, because the alternative to waiting is losing memory: a transient
// attach problem must be given every chance the backoff already provides. By
// the fifth failure the backoff has spread the attempts over many minutes, and
// the same mount has failed every time.
const degradedStartAfter = 5

// contextLost reports whether this conversation should start without its
// context volume.
func (r *ConversationReconciler) contextLost(conv *state.ConversationRecord) bool {
	if conv.RuntimeStartFailures < degradedStartAfter {
		return false
	}
	// Only when the recorded reason was about STORAGE. A conversation whose
	// pods could not be scheduled must never be stripped of its context — it
	// would lose its memory for a capacity problem.
	cond := conv.FindCondition(ConditionRuntimeStarted)
	return cond != nil && cond.Reason == ReasonVolumeUnavailable
}

// ensureMCPConfigMap renders the conversation's MCP from the configs its wiring
// bound, into a conversation-OWNED agentops-mcp-conv-<conversation> that GCs
// with it. There is no profile branch and no shared profile-keyed ConfigMap:
// capabilities come only from the Pipeline, so two pipelines binding different
// configs to one profile cannot collide by construction.
//
// A conversation with no binding compiles to an empty document — no servers,
// which is what an unwired conversation should get.
func (r *ConversationReconciler) ensureMCPConfigMap(ctx context.Context,
	conv *state.ConversationRecord) (mcpcompile.Result, string, error) {

	var (
		configs []agentopsv1alpha1.MCPConfigSpec
		names   []string
	)
	if binding := conv.MCPConfigs; binding != nil {
		for _, ref := range binding.Refs {
			var mc agentopsv1alpha1.MCPConfig
			if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ref}, &mc); err != nil {
				return mcpcompile.Result{}, "", fmt.Errorf("bound MCPConfig %q: %w", ref, err)
			}
			configs = append(configs, mc.Spec)
			names = append(names, ref)
		}
	}
	res, err := mcpcompile.Compile(configs, names)
	if err != nil {
		return mcpcompile.Result{}, "", err
	}
	if res.JSON == "" { // raw ref is mounted directly
		return res, "", nil
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: convMCPConfigMapName(conv.Name), Namespace: r.Namespace,
	}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = map[string]string{"mcp.json": res.JSON}
		return controllerutil.SetControllerReference(r.ownerOf(conv), cm, r.Scheme)
	})
	return res, cm.Name, err
}

// setToolingCondition surfaces binding resolution on the conversation.
// Conversations without bindings carry no such condition.
func (r *ConversationReconciler) setToolingCondition(ctx context.Context, conv *state.ConversationRecord,
	status, reason, message string) {

	if conv.MCPConfigs == nil && conv.Toolsets == nil {
		return
	}
	err := state.Patch(ctx, r.Store, conv, func(c *state.ConversationRecord) {
		c.SetCondition(state.Condition{Type: ConditionToolingResolved, Status: status, Reason: reason, Message: message})
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "patching ToolingResolved condition", "conversation", conv.Name)
	}
}

// MapRuntimePodToPending maps a freed capacity slot to the conversations that
// might take it: the oldest Pending ones, up to the cap.
//
// Owns(&corev1.Pod{}) cannot do this — it routes a pod event to the pod's OWN
// conversation, which is precisely the conversation that no longer needs the
// slot. Without this watch a freed slot waits on the 30 s requeue backstop.
func (r *ConversationReconciler) MapRuntimePodToPending(ctx context.Context, obj client.Object) []reconcile.Request {
	// The store is scoped to one namespace (design D-2), so a pod beside another
	// install's conversations has nothing to wake here.
	if obj != nil && obj.GetNamespace() != "" && obj.GetNamespace() != r.Namespace {
		return nil
	}
	pending, err := r.Store.List(ctx, state.ByPhase, state.ListKey{Phase: state.PhasePending})
	if err != nil {
		return nil
	}
	pending = slices.DeleteFunc(pending, func(c state.ConversationRecord) bool { return c.DeletionRequestedAt != nil })
	sort.Slice(pending, func(i, j int) bool {
		if !pending[i].CreatedAt.Equal(pending[j].CreatedAt) {
			return pending[i].CreatedAt.Before(pending[j].CreatedAt)
		}
		return pending[i].Name < pending[j].Name
	})
	// One deletion frees one slot, but waking a few costs a no-op reconcile
	// each and covers the case where several exited at once. Each still makes
	// its own FIFO decision, so waking a younger one cannot let it jump ahead.
	n := r.MaxActiveConversations
	if n < 1 {
		n = 1
	}
	if len(pending) > n {
		pending = pending[:n]
	}
	out := make([]reconcile.Request, 0, len(pending))
	for i := range pending {
		out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: r.Namespace, Name: pending[i].Name,
		}})
	}
	return out
}

// isRuntimePod: the watch above is about capacity, so only runtime pods count.
func isRuntimePod(obj client.Object) bool {
	return obj.GetLabels()[runtimepod.LabelApp] == runtimepod.LabelAppValue
}

// SetupWithManager wires the controller. The conversations themselves arrive on
// the store's feed (design D-6) rather than from an informer on the CRD; a
// runtime pod is still a Kubernetes object under every backend, so its events
// stay on the manager's cache.
//
// Owns(&corev1.Pod{}) is spelled out as a Watches on the pod with the owner
// read off its controller reference: the builder's Owns needs For(<the
// Conversation type>), which is the dependency this change removes.
func (r *ConversationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("conversation").
		WatchesRawSource(source.Func(r.startFeed)).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.mapPodToOwner)).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.MapRuntimePodToPending),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc:  func(event.CreateEvent) bool { return false },
				UpdateFunc:  func(event.UpdateEvent) bool { return false },
				GenericFunc: func(event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return isRuntimePod(e.Object) },
			})).
		Complete(r)
}

// mapPodToOwner routes a pod event to the conversation that controls the pod,
// exactly what Owns(&corev1.Pod{}) did.
func (r *ConversationReconciler) mapPodToOwner(_ context.Context, obj client.Object) []reconcile.Request {
	ref := metav1.GetControllerOf(obj)
	if ref == nil || ref.Kind != conversationKind {
		return nil
	}
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil || gv.Group != agentopsv1alpha1.GroupVersion.Group {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: ref.Name}}}
}

// degradedSuffix names a pod started without its context volume, so the reason
// is on the hop rather than only in a condition.
func degradedSuffix(degraded bool) string {
	if degraded {
		return " WITHOUT its context volume (repeated storage failures)"
	}
	return ""
}
