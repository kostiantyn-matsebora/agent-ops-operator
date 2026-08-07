// Package controller reconciles Conversations: chat topic, MCP ConfigMap,
// runtime pod lifecycle (cap + idle eviction), input pruning.
package controller

import (
	"context"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

// LabelSignatureHash indexes conversations by grouping signature.
const LabelSignatureHash = "agentops.dev/signature-hash"

// ConversationReconciler reconciles Conversation objects.
type ConversationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Runtime     runtimepod.Config
	MaxRuntimes int
	// Ops carries outbound channel operations (topic creation) to the serving
	// channel implementation; nil disables chat entirely (tests).
	Ops *chat.OpQueue
}

// Reconcile implements the reconciliation loop.
func (r *ConversationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var conv agentopsv1alpha1.Conversation
	if err := r.Get(ctx, req.NamespacedName, &conv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
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

	// chat topics: enqueue asynchronously, one ensure-topic per bound channel
	// still missing its thread binding; ids land via op completion (status
	// patch), which re-triggers reconciliation. Requeue as a fallback — op ids
	// are stable per conversation×channel, so re-enqueues dedup.
	topicPending := false
	if r.Ops != nil {
		pending, err := r.ensureTopics(ctx, &conv)
		if err != nil {
			logger.Error(err, "ensureTopics enqueue (continuing chat-less)")
		}
		topicPending = pending
	}

	// input pruning: drop processed inputs from spec, GC consumed payload objects
	if err := r.pruneProcessed(ctx, &conv); err != nil {
		return ctrl.Result{}, err
	}

	pending := dispatch.PendingInputs(&conv)
	needsWorker := len(pending) > 0 || conv.Status.Inflight != nil

	// runtime pod state
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

	if needsWorker && !podExists {
		created, err := r.createRuntimePod(ctx, &conv)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !created { // pool full, all busy — retry later
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.setPhase(ctx, &conv, agentopsv1alpha1.ConversationQueued)
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
	return ctrl.Result{}, nil
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
		if conv.ThreadFor(ref.Name) != nil {
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
		r.Ops.EnqueueEnsureTopic(ctx, &ch, conv)
		pending = true
	}
	return pending, firstErr
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

// createRuntimePod enforces the pool cap with idle eviction; returns created=false
// when the pool is full of busy workers.
func (r *ConversationReconciler) createRuntimePod(ctx context.Context, conv *agentopsv1alpha1.Conversation) (bool, error) {
	logger := log.FromContext(ctx)

	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(conv.Namespace),
		client.MatchingLabels{runtimepod.LabelApp: runtimepod.LabelAppValue}); err != nil {
		return false, err
	}
	var live []corev1.Pod
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending {
			live = append(live, p)
		}
	}
	if len(live) >= r.MaxRuntimes {
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
			if c.Status.Inflight == nil && len(dispatch.PendingInputs(&c)) == 0 {
				last := c.CreationTimestamp.Time
				if c.Status.LastActivity != nil {
					last = c.Status.LastActivity.Time
				}
				cands = append(cands, cand{p, last})
			}
		}
		if len(cands) == 0 {
			logger.Info("worker pool full, all busy — queued", "conversation", conv.Name)
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
	mcpRes, err := r.ensureMCPConfigMap(ctx, &profile)
	if err != nil {
		return false, err
	}

	// resolve the execution backend: profile.runtimeRef -> "default" CR -> bootstrap config
	cfg := r.Runtime
	runtimeName := "default"
	explicit := false
	if profile.Spec.RuntimeRef != nil {
		runtimeName = profile.Spec.RuntimeRef.Name
		explicit = true
	}
	var rt agentopsv1alpha1.AgentRuntime
	if err := r.Get(ctx, types.NamespacedName{Namespace: conv.Namespace, Name: runtimeName}, &rt); err == nil {
		cfg = runtimepod.FromRuntime(&rt.Spec, r.Runtime)
	} else if explicit {
		return false, err // named runtime must exist
	}

	pod := runtimepod.Build(conv, &profile, mcpRes, cfg)
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

func (r *ConversationReconciler) ensureMCPConfigMap(ctx context.Context, profile *agentopsv1alpha1.AgentProfile) (mcpcompile.Result, error) {
	refs := map[string]agentopsv1alpha1.MCPConfigSpec{}
	if profile.Spec.MCP != nil {
		for _, ref := range profile.Spec.MCP.ConfigRefs {
			var mc agentopsv1alpha1.MCPConfig
			if err := r.Get(ctx, types.NamespacedName{Namespace: profile.Namespace, Name: ref.Name}, &mc); err != nil {
				return mcpcompile.Result{}, err
			}
			refs[ref.Name] = mc.Spec
		}
	}
	res, err := mcpcompile.Compile(profile.Spec.MCP, refs)
	if err != nil {
		return res, err
	}
	if res.JSON == "" { // raw ref mounted directly
		return res, nil
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "agentops-mcp-" + profile.Name, Namespace: profile.Namespace,
	}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data = map[string]string{"mcp.json": res.JSON}
		return controllerutil.SetControllerReference(profile, cm, r.Scheme)
	})
	return res, err
}

// SetupWithManager wires the controller.
func (r *ConversationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Conversation{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
