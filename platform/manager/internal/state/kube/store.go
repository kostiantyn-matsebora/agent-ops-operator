// Package kube is the "kube" state.Store backend (design D-1/D-5 of the
// state-store change, openspec/changes/state-store/): today's Conversation
// and ConversationInput custom resources, behind the state.Store contract,
// byte-for-byte.
//
// It depends on nothing but client.WithWatch — no ctrl.Manager, no
// cache.Cache — so it is trivially constructable in a test
// (client.NewWithWatch against envtest) with no cache/informer
// startup-ordering to manage. Every List and Watch is scoped with
// client.InNamespace, and every Get/Update/Delete reads straight from the
// apiserver through the embedded client.Reader, the way APIReader already
// does for the manager's HTTP handlers (design D-4).
package kube

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// DefaultSweepInterval is how often Run sweeps the namespace for expired
// records when NewStore's caller takes the default. See expiry.go.
const DefaultSweepInterval = 30 * time.Second

// statusUpdateMaxAttempts bounds Update's status-write retry (see Update's
// own comment) and AppendRun's read-append-CAS loop.
const statusUpdateMaxAttempts = 20

// Store is the kube backend's concrete type. NewStore returns it behind the
// state.Store interface; a caller that needs Run (the expiry sweep, task
// 2.6) type-asserts back to *Store.
type Store struct {
	client        client.WithWatch
	namespace     string
	sweepInterval time.Duration
	// expiryArmed says the next sweep tick has something to look for. A sweep
	// lists the whole namespace, and nothing in the manager sets ExpiresAt
	// today, so an idle tick would be a pure apiserver cost. It is raised by
	// a write carrying ExpiresAt, by a read or watch event that meets an
	// expiry annotation, and at Run's start (a record from a previous
	// process); a sweep that finds no annotation left lowers it.
	expiryArmed atomic.Bool
}

// NewStore builds a kube-backed state.Store scoped to namespace (design D-2:
// scope fixed at construction). c is any client.WithWatch — production
// wires mgr.GetClient() wrapped to add Watch (or a client.NewWithWatch), a
// test wires client.NewWithWatch directly against envtest's rest.Config.
//
// The returned Store's Run method is NOT started here: nothing calls it yet
// (section 4 of the state-store change, out of scope for this package), and
// an unsolicited background goroutine in a package nobody has wired up yet
// would leak in every test that merely constructs a Store. The caller that
// eventually wires this backend into cmd/manager/main.go is the one that
// should call Run.
func NewStore(c client.WithWatch, namespace string) state.Store {
	return newStore(c, namespace, DefaultSweepInterval)
}

// NewStoreWithSweepInterval is NewStore with the expiry sweep's poll period
// overridden — exported so a test can observe a sweep without waiting out
// DefaultSweepInterval. Returns the concrete *Store (rather than
// state.Store) so the caller can reach Run directly without a type
// assertion.
func NewStoreWithSweepInterval(c client.WithWatch, namespace string, interval time.Duration) *Store {
	return newStore(c, namespace, interval)
}

func newStore(c client.WithWatch, namespace string, interval time.Duration) *Store {
	return &Store{client: c, namespace: namespace, sweepInterval: interval}
}

// Get reads straight through to the apiserver (design D-4). An expired
// record (task 2.6) reads as ErrNotFound even though it still exists in
// etcd — "invisible before it is gone" (spec: "An expired record is
// invisible before it is gone").
func (s *Store) Get(ctx context.Context, key state.Key) (state.ConversationRecord, state.Revision, error) {
	conv, err := s.getVisible(ctx, key)
	if err != nil {
		return state.ConversationRecord{}, "", err
	}
	return toRecord(conv), state.Revision(conv.ResourceVersion), nil
}

// GetWithRuns serves the record and its runs from one apiserver read: the runs
// are status.runs on the same object.
func (s *Store) GetWithRuns(ctx context.Context, key state.Key) (state.ConversationRecord, state.Revision, []state.Run, error) {
	conv, err := s.getVisible(ctx, key)
	if err != nil {
		return state.ConversationRecord{}, "", nil, err
	}
	return toRecord(conv), state.Revision(conv.ResourceVersion), toRuns(conv.Status.Runs), nil
}

// getVisible reads the Conversation and applies the expiry rule of Get.
func (s *Store) getVisible(ctx context.Context, key state.Key) (*agentopsv1alpha1.Conversation, error) {
	conv := &agentopsv1alpha1.Conversation{}
	if err := s.client.Get(ctx, s.namespacedName(key), conv); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, state.ErrNotFound
		}
		return nil, fmt.Errorf("kube: get conversation %q: %w", key, err)
	}
	if isExpired(conv, time.Now()) {
		s.expiryArmed.Store(true)
		return nil, state.ErrNotFound
	}
	return conv, nil
}

// Create writes a new Conversation. The main Create call persists spec and
// metadata only — the apiserver drops anything under .status on this
// endpoint (a status subresource is in play) and returns the object with a
// zero-value status, there being no prior object to echo back. A second,
// explicit write through the status subresource lands whatever status the
// caller's record carries, the same two-call shape Update uses below.
func (s *Store) Create(ctx context.Context, key state.Key, rec state.ConversationRecord) (state.Revision, error) {
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = string(key), s.namespace
	s.armExpiry(rec.ExpiresAt)
	applySpecAndMeta(conv, rec)
	if err := s.client.Create(ctx, conv); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return "", state.ErrAlreadyExists
		}
		return "", fmt.Errorf("kube: create conversation %q: %w", key, err)
	}
	applyStatus(conv, rec)
	if err := s.client.Status().Update(ctx, conv); err != nil {
		return "", fmt.Errorf("kube: create conversation %q status: %w", key, err)
	}
	return state.Revision(conv.ResourceVersion), nil
}

// Update writes rec in place of the record at rev (design D-2's
// get-modify-put CAS).
//
// THE ONE PLACE THIS BACKEND'S CAS IS NOT ONE ATOMIC APISERVER CALL. The
// contract's ConversationRecord carries both spec-shaped and status-shaped
// fields in one flat struct (design D-1: the contract drops the
// spec/status split, since that is the apiserver's own feature, not a
// manager need), but the apiserver itself still splits a write into two
// REST calls — the main object endpoint for spec+metadata, the status
// subresource for status — each bumping its own resourceVersion.
//
// Resolved pragmatically:
//  1. The stored object is read first. Its resourceVersion must equal rev,
//     and the main object write carries it as the precondition: the
//     CAS-checked call. A mismatch at either point is ErrConflict and the
//     status call never runs — nothing of the caller's write reaches the
//     object. Writing onto the object just read, rather than onto a bare
//     one, leaves every piece of metadata the record does not model (other
//     labels, annotations, owner references) as the apiserver holds it.
//  2. Having passed that check, the status write races nothing the caller
//     asked this call to detect. It is retried on its own conflict,
//     re-reading just before each retry, but only while the re-read status
//     still equals the one the main write returned apart from Runs (which
//     only grow and which Update never writes). A status that changed
//     otherwise is a competing writer, and re-applying the caller's whole
//     status would overwrite it: that returns ErrConflict.
//
// The revision handed back is the FINAL resourceVersion, after both calls
// land.
//
// A record marked for deletion (design D-11) is written the same way, with
// two differences. A finalizer the stored object does not already hold is
// refused (ErrDeleting) — the apiserver refuses it too, and refusing it up
// front keeps the refusal a contract error. And an Update that leaves
// Finalizers empty writes status FIRST: the main write that drops the last
// finalizer deletes the object, and the status subresource of a deleted
// object is gone.
func (s *Store) Update(ctx context.Context, key state.Key, rev state.Revision, rec state.ConversationRecord) (state.Revision, error) {
	s.armExpiry(rec.ExpiresAt)
	conv := &agentopsv1alpha1.Conversation{}
	if err := s.client.Get(ctx, s.namespacedName(key), conv); err != nil {
		if apierrors.IsNotFound(err) {
			return "", state.ErrNotFound
		}
		return "", fmt.Errorf("kube: update conversation %q: %w", key, err)
	}
	if conv.ResourceVersion != string(rev) {
		return "", state.ErrConflict
	}
	deleting := !conv.DeletionTimestamp.IsZero()
	if deleting && addsFinalizer(conv.Finalizers, rec.Finalizers) {
		return "", state.ErrDeleting
	}
	if deleting && len(rec.Finalizers) == 0 {
		return s.updateReleasingLastFinalizer(ctx, key, conv, rec)
	}

	applySpecAndMeta(conv, rec)
	if err := s.client.Update(ctx, conv); err != nil {
		return "", updateError(key, "update conversation", err)
	}

	// conv now carries the apiserver's response to the main-object write:
	// the real current status (Runs included — AppendRun's own append-only
	// history is never touched here), since the main endpoint's write
	// leaves .status exactly as it was. applyStatus below overlays every
	// status field the record carries except Runs, which this call must
	// never rewrite (design D-5: runs are append-only, never part of
	// Update's own record).
	base := conv.Status.DeepCopy()
	applyStatus(conv, rec)
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		err := s.client.Status().Update(ctx, conv)
		if err == nil {
			return state.Revision(conv.ResourceVersion), nil
		}
		if !apierrors.IsConflict(err) {
			return "", fmt.Errorf("kube: update conversation %q status: %w", key, err)
		}
		if err := s.client.Get(ctx, s.namespacedName(key), conv); err != nil {
			return "", fmt.Errorf("kube: re-read conversation %q before status retry: %w", key, err)
		}
		// The caller's status was computed from base. If anything other than a
		// run append (which commutes) changed it since, re-applying the
		// caller's whole status would silently overwrite that writer.
		if statusChangedBeyondRuns(base, &conv.Status) {
			return "", state.ErrConflict
		}
		applyStatus(conv, rec)
	}
	return "", fmt.Errorf("kube: update conversation %q status: exhausted %d attempts on conflict", key, statusUpdateMaxAttempts)
}

// statusChangedBeyondRuns reports whether current differs from base in any
// field other than Runs. Update never writes Runs, and AppendRun/UpdateRun
// only grow them, so a difference there is not a competing write to Update's
// fields.
func statusChangedBeyondRuns(base, current *agentopsv1alpha1.ConversationStatus) bool {
	b := base.DeepCopy()
	b.Runs = current.Runs
	return !equality.Semantic.DeepEqual(b, current)
}

// updateReleasingLastFinalizer is Update for a record marked for deletion
// whose new Finalizers is empty: status first (the CAS precondition rides
// on conv's resourceVersion, already checked equal to rev), then the main
// write that drops the finalizer and so deletes the object. The main write
// is retried on a conflict, since the status write has landed and must not
// be re-asked of the caller.
func (s *Store) updateReleasingLastFinalizer(ctx context.Context, key state.Key, conv *agentopsv1alpha1.Conversation, rec state.ConversationRecord) (state.Revision, error) {
	applyStatus(conv, rec)
	if err := s.client.Status().Update(ctx, conv); err != nil {
		return "", updateError(key, "update conversation status", err)
	}
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		applySpecAndMeta(conv, rec)
		err := s.client.Update(ctx, conv)
		if err == nil || apierrors.IsNotFound(err) {
			return state.Revision(conv.ResourceVersion), nil
		}
		if !apierrors.IsConflict(err) {
			return "", fmt.Errorf("kube: release conversation %q: %w", key, err)
		}
		if err := s.client.Get(ctx, s.namespacedName(key), conv); err != nil {
			if apierrors.IsNotFound(err) {
				return state.Revision(conv.ResourceVersion), nil
			}
			return "", fmt.Errorf("kube: re-read conversation %q before release retry: %w", key, err)
		}
	}
	return "", fmt.Errorf("kube: release conversation %q: exhausted %d attempts on conflict", key, statusUpdateMaxAttempts)
}

func updateError(key state.Key, what string, err error) error {
	switch {
	case apierrors.IsConflict(err):
		return state.ErrConflict
	case apierrors.IsNotFound(err):
		return state.ErrNotFound
	}
	return fmt.Errorf("kube: %s %q: %w", what, key, err)
}

// addsFinalizer reports whether next holds a finalizer cur does not.
func addsFinalizer(cur, next []string) bool {
	for _, f := range next {
		if !slices.Contains(cur, f) {
			return true
		}
	}
	return false
}

// Delete requests a Conversation's deletion (store.go's Store.Delete
// contract). The apiserver's own mark-for-deletion is the contract's
// marker: with a finalizer held the object stays, deletionTimestamp set
// once and kept by a repeated Delete, and the watch carries it as a
// modification; without one it is removed at once. Not an error when the
// key is already absent.
func (s *Store) Delete(ctx context.Context, key state.Key) error {
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = string(key), s.namespace
	if err := s.client.Delete(ctx, conv); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kube: delete conversation %q: %w", key, err)
	}
	return nil
}

func (s *Store) namespacedName(key state.Key) types.NamespacedName {
	return types.NamespacedName{Namespace: s.namespace, Name: string(key)}
}
