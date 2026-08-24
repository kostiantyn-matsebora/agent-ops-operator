package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// defaultBoundClaimSize is what a claim rendered for a named PersistentVolume
// requests when the binding says nothing.
//
// A claim binding to a pre-created volume gets that volume's capacity whatever
// it asks for, so this is a floor that lets the request be well-formed rather
// than a size anyone chose.
const defaultBoundClaimSize = "5Gi"

// PipelineReconciler validates pipeline wiring and keeps its conditions
// current, and renders the ONE object this system ever creates from a name: the
// claim on a PersistentVolume a persistence binding points at.
//
// Routing itself still creates nothing — it reads Ready pipelines at decision
// time.
//
// There is NO source-exclusivity check here, and adding one back would be a
// regression. A SignalSource is shareable exactly as a Channel is: any number
// of Ready pipelines may list one, and a signal admitted there fans out to
// every one of them, one conversation each. Whether two pipelines watch one
// source is the ADOPTER's decision. The rule that used to live here
// (`sourceConflicts`, oldest claimant wins, newer at Ready=False) existed to
// keep a single invisible default for bare chat messages, and charged every
// source kind for it; that ambiguity is now handled where it actually occurs,
// in the chat lane, by refusing rather than guessing.
type PipelineReconciler struct {
	client.Client
}

// Reconcile validates one Pipeline.
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var p agentopsv1alpha1.Pipeline
	if err := r.Get(ctx, req.NamespacedName, &p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var missing []string
	for _, ref := range p.Spec.SignalSourceRefs {
		var src agentopsv1alpha1.SignalSource
		if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &src); err != nil {
			missing = append(missing, "signalsource/"+ref.Name)
		}
	}
	for _, ref := range p.Spec.ChannelRefs {
		var ch agentopsv1alpha1.Channel
		if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &ch); err != nil {
			missing = append(missing, "channel/"+ref.Name)
		}
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: p.Spec.ProfileRef.Name}, &profile); err != nil {
		missing = append(missing, "agentprofile/"+p.Spec.ProfileRef.Name)
	}
	// tooling bindings: refs only — the CRs' content is resolved at use time,
	// so Ready checks existence, nothing else.
	if p.Spec.Toolsets != nil {
		for _, ref := range p.Spec.Toolsets.Refs {
			var ts agentopsv1alpha1.MCPToolset
			if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &ts); err != nil {
				missing = append(missing, "mcptoolset/"+ref.Name)
			}
		}
	}
	if p.Spec.MCPConfigs != nil {
		for _, ref := range p.Spec.MCPConfigs.Refs {
			var mc agentopsv1alpha1.MCPConfig
			if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &mc); err != nil {
				missing = append(missing, "mcpconfig/"+ref.Name)
			}
		}
	}

	// STORAGE: the one place naming a resource creates it. A pod cannot mount a
	// PersistentVolume, only a claim on one, so a binding that names a VOLUME
	// requires something to render the claim — and on a Pipeline that something
	// is the manager.
	//
	// A failure here does NOT block the Ready condition below it. The claim is
	// wanted before the first conversation, not before the wiring is valid, and
	// an unwritable claim reported as invalid wiring sends the operator looking
	// at their refs.
	if err := r.ensureBoundClaims(ctx, &p); err != nil {
		missing = append(missing, err.Error())
	}

	patch := client.MergeFrom(p.DeepCopy())
	// MIGRATION (one release): clear a SourceConflict left by a manager that
	// still enforced exclusivity. Deleting the writer does not delete what it
	// already wrote, so without this the condition is IMMORTAL — and it lands
	// exactly on the pipelines this change unblocks, which are the ones whose
	// operators are most likely to look. Seen live on upgrade: a pipeline
	// created seconds before the rollout kept `SourceConflict=True` beside a
	// perfectly valid `Ready=True`.
	apimeta.RemoveStatusCondition(&p.Status.Conditions, "SourceConflict")

	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "WiringValid"}
	if len(missing) > 0 {
		ready.Status = metav1.ConditionFalse
		ready.Reason = "MissingReferences"
		ready.Message = "unresolved references: " + strings.Join(missing, ", ")
	}
	apimeta.SetStatusCondition(&p.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &p, patch)
}

// ensureBoundClaims renders a claim for each binding that names a
// PersistentVolume, and NOTHING for a binding that names a claim — that claim
// already exists, which is what naming it means.
//
// IT NEVER CARRIES AN OWNERREF ON THE PIPELINE, and never deletes. Deleting a
// Pipeline must not delete the accumulated context of the conversations it
// started: storage is the one thing here whose loss cannot be repaired by
// reconciling again. The manager holds no `delete` verb on claims either, so
// this is guarded twice.
//
// It is IDEMPOTENT by existence rather than by patch. A claim's spec is
// immutable in the parts that matter, so re-reconciling an existing one has
// nothing to apply — and an operator who resized or relabelled it by hand keeps
// their edit.
func (r *PipelineReconciler) ensureBoundClaims(ctx context.Context, p *agentopsv1alpha1.Pipeline) error {
	if p.Spec.Persistence == nil {
		return nil
	}
	for _, b := range []struct {
		vol     runtimepod.Volume
		binding *agentopsv1alpha1.PersistenceBinding
	}{
		{runtimepod.VolumeContext, p.Spec.Persistence.Context},
		{runtimepod.VolumeWorkspace, p.Spec.Persistence.Workspace},
	} {
		if b.binding == nil || b.binding.VolumeName == "" {
			continue
		}
		if err := r.ensureClaim(ctx, p, b.vol, b.binding); err != nil {
			return fmt.Errorf("persistentvolumeclaim/%s: %w",
				runtimepod.PipelineClaimName(p.Name, b.vol), err)
		}
	}
	return nil
}

func (r *PipelineReconciler) ensureClaim(ctx context.Context, p *agentopsv1alpha1.Pipeline,
	vol runtimepod.Volume, b *agentopsv1alpha1.PersistenceBinding) error {

	name := runtimepod.PipelineClaimName(p.Name, vol)
	var existing corev1.PersistentVolumeClaim
	err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: name}, &existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	size := b.Size
	if size == "" {
		size = defaultBoundClaimSize
	}
	qty, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("size %q: %w", size, err)
	}
	modes := b.AccessModes
	if len(modes) == 0 {
		// What concurrent conversations on one volume need. A pre-created
		// volume supporting less will refuse the bind and say so.
		modes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	}
	// THE EMPTY STORAGE CLASS IS THE POINT, and it is a POINTER because that is
	// the only way to render an explicit empty string. Omitting the field lets
	// admission inject the cluster's default StorageClass, which provisions a
	// second volume beside the one that was named and leaves the operator's
	// untouched — the exact failure this whole capability exists to fix.
	sc := b.StorageClassName
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":        "agentops",
				"agentops.dev/pipeline":         p.Name,
				"agentops.dev/persisted-volume": string(vol),
			},
			Annotations: map[string]string{
				// The claim outlives the wiring that asked for it, deliberately,
				// and this is where the next reader finds out why.
				"agentops.dev/created-by": "pipeline/" + p.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      modes,
			VolumeName:       b.VolumeName,
			StorageClassName: &sc,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: qty},
			},
		},
	}
	if err := r.Create(ctx, claim); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// SetupWithManager wires the controller: pipelines, plus referenced-kind
// events mapped back to the pipelines naming them. A pipeline no longer
// watches its SIBLINGS — that watch existed so a conflict could converge when
// the older claimant went away, and there are no conflicts left to converge.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	allPipelines := func(ctx context.Context, namespace string) []ctrl.Request {
		var list agentopsv1alpha1.PipelineList
		if err := r.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil
		}
		var reqs []ctrl.Request
		for i := range list.Items {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
		return reqs
	}
	mapAny := func(ctx context.Context, obj client.Object) []ctrl.Request {
		// referenced kinds are few and pipelines fewer — requeue them all
		return allPipelines(ctx, obj.GetNamespace())
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Pipeline{}).
		Watches(&agentopsv1alpha1.SignalSource{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.Channel{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.AgentProfile{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.MCPToolset{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.MCPConfig{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Complete(r)
}
