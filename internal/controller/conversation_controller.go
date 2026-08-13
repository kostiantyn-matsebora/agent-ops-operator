// Package controller reconciles Conversations: chat topic, MCP ConfigMap,
// runtime pod lifecycle (cap + idle eviction), input pruning.
package controller

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

// LabelSignatureHash indexes conversations by grouping signature.
const LabelSignatureHash = "agentops.dev/signature-hash"

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
}

// Reconcile implements the reconciliation loop.
func (r *ConversationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var conv agentopsv1alpha1.Conversation
	if err := r.Get(ctx, req.NamespacedName, &conv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// A conversation on its way out archives its threads first — by /close or by
	// `kubectl delete conversation`, both take this path.
	if !conv.DeletionTimestamp.IsZero() {
		return r.finalizeClose(ctx, &conv)
	}
	if !controllerutil.ContainsFinalizer(&conv, FinalizerCloseTopics) {
		patch := client.MergeFrom(conv.DeepCopy())
		controllerutil.AddFinalizer(&conv, FinalizerCloseTopics)
		if err := r.Patch(ctx, &conv, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	// CLOSED IS INERT. Everything below this line provisions something — a
	// topic, a ConfigMap, a pod, a work unit — and a closed conversation gets
	// none of it. Placed before the signature label and the input pruning on
	// purpose: a closed conversation is not a place work can land, so there is
	// nothing to keep groupable or to prune toward.
	if conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
		return r.reconcileClosed(ctx, &conv)
	}

	// signature label for grouping lookups
	if conv.Spec.Signature != "" {
		want := ingest.SignatureHash(conv.Spec.Signature)
		if conv.Labels[LabelSignatureHash] != want {
			patch := client.MergeFrom(conv.DeepCopy())
			if conv.Labels == nil {
				conv.Labels = map[string]string{}
			}
			conv.Labels[LabelSignatureHash] = want
			if err := r.Patch(ctx, &conv, patch); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// input pruning: drop processed inputs from spec, GC consumed payload objects
	if err := r.pruneProcessed(ctx, &conv); err != nil {
		return ctrl.Result{}, err
	}

	pending := dispatch.PendingInputs(&conv)
	needsWorker := len(pending) > 0 || conv.Status.Inflight != nil

	// runtime pod state. Reaping an exited pod comes FIRST: it is what stops
	// the conversation counting against the cap.
	var pod corev1.Pod
	podName := runtimepod.PodName(conv.Name)
	podErr := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: podName}, &pod)
	podExists := podErr == nil

	if podExists && (pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed) {
		// worker exited (idle TTL or crash) — delete; inflight (if any) re-dispatches
		if err := r.Delete(ctx, &pod); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		podExists = false
		if conv.Status.Inflight != nil || conv.Status.RuntimePod != "" {
			patch := client.MergeFrom(conv.DeepCopy())
			conv.Status.Inflight = nil
			conv.Status.RuntimePod = ""
			if err := r.Status().Patch(ctx, &conv, patch); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// CAPACITY FIRST — before anything is provisioned. A conversation that
	// cannot be admitted gets no chat topic, no MCP ConfigMap, no pod and no
	// dispatch: suppressing the TOPIC is the point of the Pending phase, since
	// it is what stops a thousand signals from becoming a thousand chat threads
	// before anyone has looked at the first one. A conversation needing no
	// worker skips the gate entirely — it costs nothing and waits for nothing.
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
		// Input cards, posted PARALLEL to dispatch rather than sequenced with
		// it: the human reads the event while the agent is already working, and
		// a run that hangs or dies still leaves the thread saying what happened.
		r.postInputCards(ctx, &conv)
		// The BACKSTOP that makes a reply derivable rather than queue-resident.
		if err := r.deliverRunReplies(ctx, &conv); err != nil {
			logger.Error(err, "re-deriving undelivered run replies")
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
	phase := agentopsv1alpha1.ConversationIdle
	if conv.Status.Inflight != nil {
		phase = agentopsv1alpha1.ConversationWorking
	} else if len(pending) > 0 {
		phase = agentopsv1alpha1.ConversationQueued
	}
	if err := r.setPhase(ctx, &conv, phase); err != nil {
		return ctrl.Result{}, err
	}

	if topicPending {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	if needsWorker {
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}
	return r.autoClose(ctx, &conv, podExists)
}

// autoClose closes a FINISHED conversation that has been idle for the window,
// and requeues for the moment it expires when it has not.
//
// Self-scheduled rather than swept: the reconciler is already invoked per
// conversation with exactly the state this decision needs, so a periodic List
// would re-read everything to act on almost nothing — and would produce the
// burst the requeue avoids.
func (r *ConversationReconciler) autoClose(ctx context.Context, conv *agentopsv1alpha1.Conversation, podExists bool) (ctrl.Result, error) {
	if !r.AutoCloseEnabled || r.AutoCloseIdleAge <= 0 || r.Router == nil {
		return ctrl.Result{}, nil
	}
	if !finished(conv, podExists) {
		return ctrl.Result{}, nil
	}
	idle := time.Since(lastActivity(conv))
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
func finished(conv *agentopsv1alpha1.Conversation, podExists bool) bool {
	if conv.Status.Phase != agentopsv1alpha1.ConversationIdle {
		return false
	}
	if needsWorker(conv) || podExists {
		return false
	}
	return allRunsDelivered(conv)
}

// allRunsDelivered reports whether every recorded run reached every bound
// channel. A conversation with no bound channels is trivially delivered — there
// is no thread for an answer to be owed to.
func allRunsDelivered(conv *agentopsv1alpha1.Conversation) bool {
	if len(conv.Spec.ChannelRefs) == 0 {
		return true
	}
	for i := range conv.Status.Runs {
		run := &conv.Status.Runs[i]
		if !run.DeliveryTracked {
			continue // pre-upgrade run: backfilled as delivered, never sent
		}
		for _, ref := range conv.Spec.ChannelRefs {
			if !run.DeliveredTo(ref.Name) {
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
func lastActivity(conv *agentopsv1alpha1.Conversation) time.Time {
	var last time.Time
	note := func(t time.Time) {
		if t.After(last) {
			last = t
		}
	}
	if conv.Status.LastActivity != nil {
		note(conv.Status.LastActivity.Time)
	}
	for i := range conv.Spec.Inputs {
		note(conv.Spec.Inputs[i].ReceivedAt.Time)
	}
	for i := range conv.Status.Runs {
		if f := conv.Status.Runs[i].FinishedAt; f != nil {
			note(f.Time)
		}
	}
	// Creation is the FALLBACK, not a floor: a conversation that has run is
	// measured from its work, never from when it was opened. Taking the max
	// with creation instead would make a freshly-adopted conversation look busy
	// for a whole window because the object is young.
	if last.IsZero() {
		return conv.CreationTimestamp.Time
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
func (r *ConversationReconciler) reconcileClosed(ctx context.Context, conv *agentopsv1alpha1.Conversation) (ctrl.Result, error) {
	// Grace 0: the run, if any, was abandoned at the close and nobody is
	// waiting for its output, so a graceful shutdown would only hold the slot.
	pod := &corev1.Pod{}
	pod.Namespace, pod.Name = conv.Namespace, runtimepod.PodName(conv.Name)
	if err := r.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	cm := &corev1.ConfigMap{}
	cm.Namespace, cm.Name = conv.Namespace, convMCPConfigMapName(conv.Name)
	if err := r.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if conv.Status.Inflight != nil || conv.Status.RuntimePod != "" {
		patch := client.MergeFrom(conv.DeepCopy())
		conv.Status.Inflight = nil
		conv.Status.RuntimePod = ""
		if err := r.Status().Patch(ctx, conv, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	if r.Ops != nil {
		for _, t := range conv.Status.Threads {
			if conv.Status.ThreadArchived(t.Channel) {
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: t.Channel}, &ch); err != nil {
				continue // channel gone: nothing left to archive on it
			}
			if ch.Spec.Adapter == "" {
				continue
			}
			r.Ops.EnqueueCloseTopic(ctx, &ch, t.ThreadID, conv.Name)
		}
	}
	return r.autoDelete(ctx, conv)
}

// autoDelete reclaims a conversation that has been CLOSED for longer than the
// window, and requeues for the moment it expires when it has not.
//
// Measured from status.closedAt, so a reopen — which clears that stamp — stops
// the clock. Applies to Closed conversations only: a never-closed conversation
// is never auto-deleted however idle it is, because the two stages are separate
// decisions with separate flags.
func (r *ConversationReconciler) autoDelete(ctx context.Context, conv *agentopsv1alpha1.Conversation) (ctrl.Result, error) {
	if !r.AutoDeleteEnabled || r.AutoDeleteClosedAge <= 0 || conv.Status.ClosedAt == nil {
		return ctrl.Result{}, nil
	}
	elapsed := time.Since(conv.Status.ClosedAt.Time)
	if elapsed < r.AutoDeleteClosedAge {
		return ctrl.Result{RequeueAfter: jitter(r.AutoDeleteClosedAge - elapsed)}, nil
	}
	log.FromContext(ctx).Info("auto-deleting a long-closed conversation",
		"conversation", conv.Name, "closedFor", elapsed.Round(time.Minute), "window", r.AutoDeleteClosedAge)
	return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, conv))
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

// finalizeClose archives the threads of a deleting conversation, then lets go.
//
// Since closing stopped deleting, this covers exactly ONE case: a Conversation
// deleted without having been closed — a direct `kubectl delete`, or the
// autodelete of a conversation whose archive never completed. A conversation
// deleted after a proper close finds every thread already in
// status.threadsArchived and releases immediately, archiving nothing twice.
//
// Enqueueing on every pass is safe — the op id is stable per
// conversation×channel, so it dedups against both the pending op and the
// completed one.
func (r *ConversationReconciler) finalizeClose(ctx context.Context, conv *agentopsv1alpha1.Conversation) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(conv, FinalizerCloseTopics) {
		return ctrl.Result{}, nil
	}

	// Free the slot now rather than at the end of the grace: /close on a
	// working conversation abandons the run, and a pod nobody is waiting on
	// must not hold capacity for two more minutes.
	pod := &corev1.Pod{}
	pod.Namespace, pod.Name = conv.Namespace, runtimepod.PodName(conv.Name)
	// Grace 0: the run is abandoned and nobody is waiting for its output, so a
	// graceful shutdown would only hold the slot longer.
	if err := r.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	outstanding := false
	if r.Ops != nil {
		for _, t := range conv.Status.Threads {
			if conv.Status.ThreadArchived(t.Channel) {
				continue // archived at the close: nothing owed, nothing to wait for
			}
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: t.Channel}, &ch); err != nil {
				continue // channel gone: nothing left to archive on it
			}
			if ch.Spec.Adapter == "" {
				continue
			}
			r.Ops.EnqueueCloseTopic(ctx, &ch, t.ThreadID, conv.Name)
			if r.Ops.Pending(chat.CloseTopicOpID(conv.Name, t.Channel)) {
				outstanding = true
			}
		}
	}
	if outstanding && time.Since(conv.DeletionTimestamp.Time) < r.closeGrace() {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if outstanding {
		logger.Info("close-topic grace expired; releasing the conversation with threads still open",
			"conversation", conv.Name)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	controllerutil.RemoveFinalizer(conv, FinalizerCloseTopics)
	return ctrl.Result{}, client.IgnoreNotFound(r.Patch(ctx, conv, patch))
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

// needsWorker reports whether a conversation has work that requires a runtime
// pod. It is the same question everywhere: something queued, or something
// already dispatched.
func needsWorker(c *agentopsv1alpha1.Conversation) bool {
	return len(dispatch.PendingInputs(c)) > 0 || c.Status.Inflight != nil
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
func (r *ConversationReconciler) admit(ctx context.Context, conv *agentopsv1alpha1.Conversation) (bool, error) {
	live, err := r.liveRuntimePods(ctx, conv.Namespace)
	if err != nil {
		return false, err
	}
	hasPod := map[string]bool{}
	for _, p := range live {
		hasPod[p.Labels[runtimepod.LabelConversation]] = true
	}

	free := r.MaxActiveConversations - len(live)
	if free <= 0 {
		free = r.evictableCount(ctx, conv.Namespace, live)
	}
	if free <= 0 {
		return false, nil
	}

	var list agentopsv1alpha1.ConversationList
	if err := r.List(ctx, &list, client.InNamespace(conv.Namespace)); err != nil {
		return false, err
	}
	// The waiting set is defined by PODS, not by phase: a conversation that
	// needs a worker and holds none is waiting, whether it has been marked
	// Pending yet or has only just been created. Keying on phase would let a
	// brand-new conversation reconciled first jump an older one.
	var waiting []agentopsv1alpha1.Conversation
	for i := range list.Items {
		c := &list.Items[i]
		if !c.DeletionTimestamp.IsZero() || hasPod[c.Name] || !needsWorker(c) {
			continue
		}
		// Closed is inert: it will never be given a pod, so leaving it in the
		// waiting set would let a conversation nobody will ever admit sit ahead
		// of a Pending one in FIFO order and starve it. Its inputs, if it was
		// closed holding any, stay on the object untouched — they are part of
		// the record a reopen restores, not work owed.
		if c.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		waiting = append(waiting, *c)
	}
	sort.Slice(waiting, func(i, j int) bool {
		if !waiting[i].CreationTimestamp.Equal(&waiting[j].CreationTimestamp) {
			return waiting[i].CreationTimestamp.Before(&waiting[j].CreationTimestamp)
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
func (r *ConversationReconciler) evictableCount(ctx context.Context, ns string, live []corev1.Pod) int {
	n := 0
	for _, p := range live {
		var c agentopsv1alpha1.Conversation
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: p.Labels[runtimepod.LabelConversation]}, &c); err != nil {
			continue
		}
		if !needsWorker(&c) {
			n++
		}
	}
	return n
}

// enterPending parks an unadmitted conversation and tells the person waiting,
// ONCE. The phase is patched before the notice so a reconcile storm cannot
// produce a second one; a notice lost to a crash in that window is preferable
// to a channel repeating itself every 30 seconds.
func (r *ConversationReconciler) enterPending(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	if conv.Status.Phase == agentopsv1alpha1.ConversationPending {
		return nil
	}
	if err := r.setPhase(ctx, conv, agentopsv1alpha1.ConversationPending); err != nil {
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
func (r *ConversationReconciler) notifyQueued(ctx context.Context, conv *agentopsv1alpha1.Conversation) {
	if r.Ops == nil || len(conv.Spec.ChannelRefs) == 0 {
		return
	}
	ref := conv.Spec.ChannelRefs[0]
	var ch agentopsv1alpha1.Channel
	if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: ref.Name}, &ch); err != nil || ch.Spec.Adapter == "" {
		return
	}
	r.Ops.EnqueueMessage(ctx, &ch, conv.ThreadFor(ref.Name), chat.Notice(
		"⏳ Queued for capacity — every agent slot is busy. This starts as soon as one frees up."))
}

func (r *ConversationReconciler) setPhase(ctx context.Context, conv *agentopsv1alpha1.Conversation, phase agentopsv1alpha1.ConversationPhase) error {
	if conv.Status.Phase == phase {
		return nil
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Phase = phase
	return r.Status().Patch(ctx, conv, patch)
}

// ensureTopics enqueues topic creation for every bound channel lacking a
// thread binding; reports whether any are still pending. Dangling channelRefs
// are skipped (that channel stays chat-less) without blocking the others.
func (r *ConversationReconciler) ensureTopics(ctx context.Context, conv *agentopsv1alpha1.Conversation) (bool, error) {
	pending := false
	var firstErr error
	for _, ref := range conv.Spec.ChannelRefs {
		existing := conv.ThreadFor(ref.Name)
		archived := conv.Status.ThreadArchived(ref.Name)
		// A thread that exists and is not archived is a live thread: nothing to
		// do. A thread that exists and IS archived belongs to a conversation
		// that was closed and reopened, and needs re-establishing — carrying
		// its old id as a hint the adapter may honour or ignore.
		if existing != nil && !archived {
			continue
		}
		var ch agentopsv1alpha1.Channel
		if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: ref.Name}, &ch); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ch.Spec.Adapter == "" {
			continue
		}
		d := r.topicDescriptor(ctx, conv)
		if existing != nil {
			d.PreviousThreadID = *existing
		}
		r.Ops.EnqueueEnsureTopic(ctx, &ch, conv, d)
		pending = true
	}
	return pending, firstErr
}

// topicDescriptor describes a conversation's thread for the adapter to NAME.
// The manager supplies facts — route, source, title, labels, lane — and no
// formatting: Telegram caps forum topics at 128 characters and takes no markup,
// a web chat has neither limit, and neither constraint belongs here.
func (r *ConversationReconciler) topicDescriptor(ctx context.Context, conv *agentopsv1alpha1.Conversation) chat.TopicDescriptor {
	d := chat.TopicDescriptor{
		Conversation: conv.Name,
		Title:        conv.Spec.Title,
		Pipeline:     r.inferredPipeline(ctx, conv),
	}
	// The FIRST input is what opened the conversation, so it names the lane and
	// the source the topic is about; later recurrences share both.
	for i := range conv.Spec.Inputs {
		if o := conv.Spec.Inputs[i].Origin; o != nil {
			d.Source, d.Kind = o.Name, o.SignalKind
			if payload := r.inputPayload(ctx, conv, &conv.Spec.Inputs[i]); payload != nil {
				d.Labels = payload.Spec.Labels
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
func (r *ConversationReconciler) inferredPipeline(ctx context.Context, conv *agentopsv1alpha1.Conversation) string {
	if p := chat.PipelineForConversation(ctx, r.Client, conv.Namespace, conv); p != nil {
		return p.Name
	}
	return ""
}

// inputPayload reads an input's out-of-line ConversationInput, or nil when the
// payload is inline or the object is gone (pruned after processing).
func (r *ConversationReconciler) inputPayload(ctx context.Context, conv *agentopsv1alpha1.Conversation,
	item *agentopsv1alpha1.InputItem) *agentopsv1alpha1.ConversationInput {

	if item.PayloadRef == nil {
		return nil
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: item.PayloadRef.Name}, &ci); err != nil {
		return nil
	}
	return &ci
}

// postInputCards posts every input a human has not already seen to every bound
// channel that has a thread.
//
// Three properties make this safe to run on EVERY reconcile:
//
//   - the op id is stable per conversation×input×channel, so a re-enqueue
//     dedups against both the pending map and the completed window;
//   - the posting rule is read off the input's recorded origin
//     (InputItem.PostToChannels), so a channel never sees its own echo and
//     pre-provenance inputs post nothing at all;
//   - a channel with no thread binding yet is skipped, and picked up on the
//     reconcile the binding triggers — enqueuing earlier would drop the card.
func (r *ConversationReconciler) postInputCards(ctx context.Context, conv *agentopsv1alpha1.Conversation) {
	pipeline := ""
	resolved := false
	for i := range conv.Spec.Inputs {
		item := &conv.Spec.Inputs[i]
		if !item.PostToChannels() {
			continue
		}
		if !resolved { // one lookup per reconcile, and only when something posts
			pipeline, resolved = r.inferredPipeline(ctx, conv), true
		}
		body, inputRef, labels := item.Payload, "", map[string]string(nil)
		if ci := r.inputPayload(ctx, conv, item); ci != nil {
			body, inputRef, labels = ci.Spec.Payload, ci.Name, ci.Spec.Labels
		}
		msg := chat.SignalMessage(pipeline, item.Origin.Name, conv.Spec.Title, inputRef, labels, body)
		for _, ref := range conv.Spec.ChannelRefs {
			tid := conv.ThreadFor(ref.Name)
			if tid == nil {
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: ref.Name}, &ch); err != nil ||
				ch.Spec.Adapter == "" {
				continue
			}
			r.Ops.EnqueueInputCard(ctx, &ch, conv.Name, item.ID, tid, msg)
		}
	}
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
func (r *ConversationReconciler) deliverRunReplies(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	if len(conv.Spec.ChannelRefs) == 0 || len(conv.Status.Runs) == 0 {
		return nil
	}
	// Threads, not channelRefs: a channel whose topic does not exist yet has
	// nowhere to receive the reply, and the reconcile its binding triggers will
	// come back here.
	var backfill []string
	for i := range conv.Status.Runs {
		run := &conv.Status.Runs[i]
		if !run.DeliveryTracked {
			backfill = append(backfill, run.RunID)
			continue
		}
		msg := chat.RunReplyMessage(run)
		for _, t := range conv.Status.Threads {
			if run.DeliveredTo(t.Channel) || !conv.BoundTo(t.Channel) {
				continue
			}
			var ch agentopsv1alpha1.Channel
			if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: t.Channel}, &ch); err != nil ||
				ch.Spec.Adapter == "" {
				continue
			}
			tid := t.ThreadID
			r.Ops.EnqueueRunReply(ctx, &ch, conv.Name, run.RunID, &tid, msg)
		}
	}
	if len(backfill) == 0 {
		return nil
	}
	return r.backfillDelivered(ctx, conv, backfill)
}

// backfillDelivered records pre-upgrade runs as delivered to every bound
// channel WITHOUT enqueueing anything. Their replies were posted by the manager
// that ran them; the markers simply did not exist yet.
func (r *ConversationReconciler) backfillDelivered(ctx context.Context, conv *agentopsv1alpha1.Conversation, runIDs []string) error {
	want := map[string]bool{}
	for _, id := range runIDs {
		want[id] = true
	}
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: conv.Name}, &fresh); err != nil {
			return client.IgnoreNotFound(err)
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		changed := false
		for i := range fresh.Status.Runs {
			run := &fresh.Status.Runs[i]
			if !want[run.RunID] || run.DeliveryTracked {
				continue
			}
			// Tracked from here on, and already delivered as far as anyone can
			// tell — which is the honest record: this manager never owed it.
			run.DeliveryTracked = true
			for _, ref := range conv.Spec.ChannelRefs {
				if !run.DeliveredTo(ref.Name) {
					run.Delivered = append(run.Delivered, ref.Name)
				}
			}
			changed = true
		}
		if !changed {
			return nil
		}
		err := r.Status().Patch(ctx, &fresh, patch)
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
	}
	return fmt.Errorf("conflict backfilling delivery markers on %s", conv.Name)
}

func (r *ConversationReconciler) pruneProcessed(ctx context.Context, conv *agentopsv1alpha1.Conversation) error {
	if len(conv.Status.ProcessedInputIDs) == 0 {
		return nil
	}
	done := map[string]bool{}
	for _, id := range conv.Status.ProcessedInputIDs {
		done[id] = true
	}
	var keep []agentopsv1alpha1.InputItem
	var prune []agentopsv1alpha1.InputItem
	for _, in := range conv.Spec.Inputs {
		if done[in.ID] {
			prune = append(prune, in)
		} else {
			keep = append(keep, in)
		}
	}
	if len(prune) == 0 {
		return nil
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Spec.Inputs = keep
	if err := r.Patch(ctx, conv, patch); err != nil {
		return err
	}
	for _, in := range prune {
		if in.PayloadRef != nil {
			ci := &agentopsv1alpha1.ConversationInput{}
			ci.Namespace = conv.Namespace
			ci.Name = in.PayloadRef.Name
			if err := r.Delete(ctx, ci); err != nil && !apierrors.IsNotFound(err) {
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
func (r *ConversationReconciler) createRuntimePod(ctx context.Context, conv *agentopsv1alpha1.Conversation) (bool, error) {
	logger := log.FromContext(ctx)

	live, err := r.liveRuntimePods(ctx, conv.Namespace)
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
			var c agentopsv1alpha1.Conversation
			if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: cn}, &c); err != nil {
				continue
			}
			if !needsWorker(&c) {
				last := c.CreationTimestamp.Time
				if c.Status.LastActivity != nil {
					last = c.Status.LastActivity.Time
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
	if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: conv.Spec.ProfileRef.Name}, &profile); err != nil {
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
		r.setToolingCondition(ctx, conv, metav1.ConditionFalse, reason, err.Error())
		return false, err
	}
	r.setToolingCondition(ctx, conv, metav1.ConditionTrue, "Resolved", "")

	// resolve the execution backend: profile.runtimeRef -> "default" CR -> bootstrap
	resolved, err := runtimepod.ResolveFor(ctx, r.Client, conv.Namespace, &profile, r.Runtime)
	if err != nil {
		return false, err
	}

	pod := runtimepod.Build(conv, &profile, mcpRes, mcpCM, resolved.Config)
	pod.Namespace = conv.Namespace
	if err := controllerutil.SetControllerReference(conv, pod, r.Scheme); err != nil {
		return false, err
	}
	if err := r.Create(ctx, pod); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.RuntimePod = pod.Name
	return true, r.Status().Patch(ctx, conv, patch)
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
	conv *agentopsv1alpha1.Conversation) (mcpcompile.Result, string, error) {

	var (
		configs []agentopsv1alpha1.MCPConfigSpec
		names   []string
	)
	if binding := conv.Spec.MCPConfigs; binding != nil {
		for _, ref := range binding.Refs {
			var mc agentopsv1alpha1.MCPConfig
			if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: ref.Name}, &mc); err != nil {
				return mcpcompile.Result{}, "", fmt.Errorf("bound MCPConfig %q: %w", ref.Name, err)
			}
			configs = append(configs, mc.Spec)
			names = append(names, ref.Name)
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
		Name: convMCPConfigMapName(conv.Name), Namespace: conv.Namespace,
	}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = map[string]string{"mcp.json": res.JSON}
		return controllerutil.SetControllerReference(conv, cm, r.Scheme)
	})
	return res, cm.Name, err
}

// setToolingCondition surfaces binding resolution on the conversation.
// Conversations without bindings carry no such condition.
func (r *ConversationReconciler) setToolingCondition(ctx context.Context, conv *agentopsv1alpha1.Conversation,
	status metav1.ConditionStatus, reason, message string) {

	if conv.Spec.MCPConfigs == nil && conv.Spec.Toolsets == nil {
		return
	}
	patch := client.MergeFrom(conv.DeepCopy())
	apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
		Type: ConditionToolingResolved, Status: status, Reason: reason, Message: message,
	})
	if err := r.Status().Patch(ctx, conv, patch); err != nil {
		log.FromContext(ctx).Error(err, "patching ToolingResolved condition", "conversation", conv.Name)
	}
}

// MapRuntimePodToPending maps a freed capacity slot to the conversations that
// might take it: the oldest Pending ones, up to the cap.
//
// Owns(&corev1.Pod{}) cannot do this — it routes a pod event to the pod's OWN
// conversation, which is precisely the conversation that no longer needs the
// slot. Without this watch a freed slot waits on the 30 s requeue backstop.
func (r *ConversationReconciler) MapRuntimePodToPending(ctx context.Context, _ client.Object) []reconcile.Request {
	var list agentopsv1alpha1.ConversationList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	var pending []agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].Status.Phase == agentopsv1alpha1.ConversationPending && list.Items[i].DeletionTimestamp.IsZero() {
			pending = append(pending, list.Items[i])
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		if !pending[i].CreationTimestamp.Equal(&pending[j].CreationTimestamp) {
			return pending[i].CreationTimestamp.Before(&pending[j].CreationTimestamp)
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
			Namespace: pending[i].Namespace, Name: pending[i].Name,
		}})
	}
	return out
}

// isRuntimePod: the watch above is about capacity, so only runtime pods count.
func isRuntimePod(obj client.Object) bool {
	return obj.GetLabels()[runtimepod.LabelApp] == runtimepod.LabelAppValue
}

// SetupWithManager wires the controller.
func (r *ConversationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Conversation{}).
		Owns(&corev1.Pod{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.MapRuntimePodToPending),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc:  func(event.CreateEvent) bool { return false },
				UpdateFunc:  func(event.UpdateEvent) bool { return false },
				GenericFunc: func(event.GenericEvent) bool { return false },
				DeleteFunc:  func(e event.DeleteEvent) bool { return isRuntimePod(e.Object) },
			})).
		Complete(r)
}
