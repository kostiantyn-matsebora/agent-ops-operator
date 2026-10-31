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
	"time"

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
	var conv agentopsv1alpha1.Conversation
	if err := s.client.Get(ctx, s.namespacedName(key), &conv); err != nil {
		if apierrors.IsNotFound(err) {
			return state.ConversationRecord{}, "", state.ErrNotFound
		}
		return state.ConversationRecord{}, "", fmt.Errorf("kube: get conversation %q: %w", key, err)
	}
	if isExpired(&conv, time.Now()) {
		return state.ConversationRecord{}, "", state.ErrNotFound
	}
	return toRecord(&conv), state.Revision(conv.ResourceVersion), nil
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
//  1. The main object Update carries rev as the precondition. This is the
//     CAS-checked call: a 409 here means the caller's own revision lost the
//     race, so it is translated to ErrConflict immediately and the status
//     call never runs — nothing of the caller's write reaches the object.
//  2. Having passed that check, the status write races nothing the caller
//     asked this call to detect: nothing else should be writing between
//     these two calls within one logical Update. It is retried on its own
//     conflict, re-reading just before each retry, WITHOUT re-applying the
//     caller's CAS check — the contract's revision is a precondition on
//     the WHOLE record as the caller last read it, already honoured by
//     step 1.
//
// The revision handed back is the FINAL resourceVersion, after both calls
// land.
func (s *Store) Update(ctx context.Context, key state.Key, rev state.Revision, rec state.ConversationRecord) (state.Revision, error) {
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = string(key), s.namespace
	conv.ResourceVersion = string(rev)
	applySpecAndMeta(conv, rec)

	if err := s.client.Update(ctx, conv); err != nil {
		if apierrors.IsConflict(err) {
			return "", state.ErrConflict
		}
		if apierrors.IsNotFound(err) {
			return "", state.ErrNotFound
		}
		return "", fmt.Errorf("kube: update conversation %q: %w", key, err)
	}

	// conv now carries the apiserver's response to the main-object write:
	// the real current status (Runs included — AppendRun's own append-only
	// history is never touched here), since the main endpoint's write
	// leaves .status exactly as it was. applyStatus below overlays every
	// status field the record carries except Runs, which this call must
	// never rewrite (design D-5: runs are append-only, never part of
	// Update's own record).
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
		applyStatus(conv, rec)
	}
	return "", fmt.Errorf("kube: update conversation %q status: exhausted %d attempts on conflict", key, statusUpdateMaxAttempts)
}

// Delete removes a Conversation. Not an error when the key is already
// absent (store.go's Store.Delete contract).
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
