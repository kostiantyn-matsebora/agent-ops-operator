package kube

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// SignalStore is the kube backend's state.SignalStore (design D-14): the Signal
// custom resources, and the cooldown window on SignalSource.status, behind the
// contract, with the stored objects unchanged. It is a sibling of Store over
// the same client and namespace, not a method set of it, since a Signal's Get
// and Create differ in type from a Conversation's.
type SignalStore struct {
	client        client.WithWatch
	namespace     string
	sweepInterval time.Duration
	// expiryArmed has the same meaning as Store.expiryArmed: the next sweep
	// has something to look for. A signal sets ExpiresAt in the write that
	// makes it terminal, so an install with live signals only is not swept.
	expiryArmed atomic.Bool
}

// NewSignalStore builds a kube-backed state.SignalStore scoped to namespace.
// Run is not started here, as for NewStore.
func NewSignalStore(c client.WithWatch, namespace string) state.SignalStore {
	return NewSignalStoreWithSweepInterval(c, namespace, DefaultSweepInterval)
}

// NewSignalStoreWithSweepInterval is NewSignalStore with the sweep period
// overridden, returning the concrete type so a caller can reach Run.
func NewSignalStoreWithSweepInterval(c client.WithWatch, namespace string, interval time.Duration) *SignalStore {
	return &SignalStore{client: c, namespace: namespace, sweepInterval: interval}
}

func (s *SignalStore) namespacedName(key state.Key) types.NamespacedName {
	return types.NamespacedName{Namespace: s.namespace, Name: string(key)}
}

func signalExpired(sig *agentopsv1alpha1.Signal, now time.Time) bool {
	at := expiresAtFromAnnotations(sig.Annotations)
	return at != nil && at.Before(now)
}

// Get reads straight through to the apiserver. An expired record reads as
// ErrNotFound while the object still exists.
func (s *SignalStore) Get(ctx context.Context, key state.Key) (state.SignalRecord, state.Revision, error) {
	var sig agentopsv1alpha1.Signal
	if err := s.client.Get(ctx, s.namespacedName(key), &sig); err != nil {
		if apierrors.IsNotFound(err) {
			return state.SignalRecord{}, "", state.ErrNotFound
		}
		return state.SignalRecord{}, "", fmt.Errorf("kube: get signal %q: %w", key, err)
	}
	if signalExpired(&sig, time.Now()) {
		s.expiryArmed.Store(true)
		return state.SignalRecord{}, "", state.ErrNotFound
	}
	return toSignalRecord(&sig), state.Revision(sig.ResourceVersion), nil
}

// Create stores a new Signal. The apiserver's create is the single arbiter of
// a duplicate: AlreadyExists is state.ErrAlreadyExists, with no lookup first.
// A record carrying status gets it through the status subresource in a second
// call, as Store.Create does.
func (s *SignalStore) Create(ctx context.Context, key state.Key, rec state.SignalRecord) (state.Revision, error) {
	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = string(key), s.namespace
	if rec.ExpiresAt != nil {
		s.expiryArmed.Store(true)
	}
	applySignalSpec(sig, rec)
	if err := s.createOrReplaceExpired(ctx, key, sig); err != nil {
		return "", err
	}
	applySignalStatus(sig, rec)
	if equality.Semantic.DeepEqual(sig.Status, agentopsv1alpha1.SignalStatus{}) {
		return state.Revision(sig.ResourceVersion), nil
	}
	if err := s.client.Status().Update(ctx, sig); err != nil {
		return "", fmt.Errorf("kube: create signal %q status: %w", key, err)
	}
	return state.Revision(sig.ResourceVersion), nil
}

// createOrReplaceExpired creates sig, treating an existing object whose expiry
// has passed as already gone: it reads as ErrNotFound everywhere, so a re-send
// inside the sweep period must not be acked as a duplicate of a record nobody
// can see. The stale object is deleted under the UID and revision just read and
// the create is tried once more; losing either race is a duplicate, not a retry.
func (s *SignalStore) createOrReplaceExpired(ctx context.Context, key state.Key, sig *agentopsv1alpha1.Signal) error {
	err := s.client.Create(ctx, sig)
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("kube: create signal %q: %w", key, err)
	}
	var existing agentopsv1alpha1.Signal
	if gerr := s.client.Get(ctx, s.namespacedName(key), &existing); gerr != nil || !signalExpired(&existing, time.Now()) {
		return state.ErrAlreadyExists
	}
	derr := s.client.Delete(ctx, &existing, client.Preconditions{UID: &existing.UID, ResourceVersion: &existing.ResourceVersion})
	if derr != nil && !apierrors.IsNotFound(derr) {
		return state.ErrAlreadyExists
	}
	sig.ResourceVersion = ""
	if err := s.client.Create(ctx, sig); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return state.ErrAlreadyExists
		}
		return fmt.Errorf("kube: create signal %q: %w", key, err)
	}
	return nil
}

// Update writes the status fields and ExpiresAt. The status subresource is
// written first, under the revision the caller read: that write is the
// compare-and-swap. The expiry annotation is metadata, so it needs a second
// call, made only when it changed; a crash between the two leaves a terminal
// signal without an expiry, which the consumer's next pass stamps. The reverse
// order could leave a live signal carrying a future expiry.
func (s *SignalStore) Update(ctx context.Context, key state.Key, rev state.Revision, rec state.SignalRecord) (state.Revision, error) {
	if rec.ExpiresAt != nil {
		s.expiryArmed.Store(true)
	}
	sig := &agentopsv1alpha1.Signal{}
	if err := s.client.Get(ctx, s.namespacedName(key), sig); err != nil {
		if apierrors.IsNotFound(err) {
			return "", state.ErrNotFound
		}
		return "", fmt.Errorf("kube: update signal %q: %w", key, err)
	}
	if signalExpired(sig, time.Now()) {
		return "", state.ErrNotFound
	}
	if sig.ResourceVersion != string(rev) {
		return "", state.ErrConflict
	}
	applySignalStatus(sig, rec)
	if err := s.client.Status().Update(ctx, sig); err != nil {
		return "", signalUpdateError(key, "update signal status", err)
	}
	return s.settleExpiry(ctx, key, sig, rec.ExpiresAt)
}

// settleExpiry brings the stored expiry annotation to want and returns the
// final revision. The status write has already won the compare-and-swap, so a
// conflict here is a writer touching only metadata and is retried against a
// fresh read.
func (s *SignalStore) settleExpiry(ctx context.Context, key state.Key, sig *agentopsv1alpha1.Signal, want *time.Time) (state.Revision, error) {
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		if sameInstant(expiresAtFromAnnotations(sig.Annotations), want) {
			return state.Revision(sig.ResourceVersion), nil
		}
		setExpiresAt(&sig.ObjectMeta, want)
		err := s.client.Update(ctx, sig)
		if err == nil {
			return state.Revision(sig.ResourceVersion), nil
		}
		if !apierrors.IsConflict(err) {
			return "", signalUpdateError(key, "update signal expiry", err)
		}
		if err := s.client.Get(ctx, s.namespacedName(key), sig); err != nil {
			return "", signalUpdateError(key, "re-read signal before expiry retry", err)
		}
	}
	return "", fmt.Errorf("kube: update signal %q expiry: exhausted %d attempts on conflict", key, statusUpdateMaxAttempts)
}

func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func signalUpdateError(key state.Key, what string, err error) error {
	switch {
	case apierrors.IsConflict(err):
		return state.ErrConflict
	case apierrors.IsNotFound(err):
		return state.ErrNotFound
	}
	return fmt.Errorf("kube: %s %q: %w", what, key, err)
}

// Delete removes a Signal at once. Not an error when it is absent.
func (s *SignalStore) Delete(ctx context.Context, key state.Key) error {
	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = string(key), s.namespace
	if err := s.client.Delete(ctx, sig); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kube: delete signal %q: %w", key, err)
	}
	return nil
}

// List serves every declared signal index with one client.List of the
// namespace, filtered in Go, as Store.List does.
func (s *SignalStore) List(ctx context.Context, index state.SignalIndex, key state.SignalListKey) ([]state.SignalRecord, error) {
	if !declaredSignalIndex(index) {
		return nil, state.ErrUnknownIndex
	}
	var list agentopsv1alpha1.SignalList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("kube: list signals: %w", err)
	}
	now := time.Now()
	var out []state.SignalRecord
	for i := range list.Items {
		sig := &list.Items[i]
		if signalExpired(sig, now) {
			s.expiryArmed.Store(true)
			continue
		}
		if matchesSignalIndex(sig, index, key) {
			out = append(out, toSignalRecord(sig))
		}
	}
	return out, nil
}

func declaredSignalIndex(index state.SignalIndex) bool {
	for _, d := range state.DeclaredSignalIndexes {
		if d == index {
			return true
		}
	}
	return false
}

func matchesSignalIndex(sig *agentopsv1alpha1.Signal, index state.SignalIndex, key state.SignalListKey) bool {
	switch index {
	case state.SignalAll:
		return true
	case state.SignalNonTerminalBySource:
		return sig.Spec.SourceRef.Name == key.Source && !state.TerminalSignalPhase(string(sig.Status.Phase))
	case state.SignalNonTerminalByInitiation:
		return sig.Spec.SourceRef.Name == key.Source && sig.Spec.InitiationID == key.InitiationID &&
			!state.TerminalSignalPhase(string(sig.Status.Phase))
	default:
		return false
	}
}

// Watch delivers the namespace's Signals over the apiserver's own watch, with
// Store.Watch's resume rules.
func (s *SignalStore) Watch(ctx context.Context, resume state.Revision) (<-chan state.SignalEvent, error) {
	startRV := string(resume)
	if startRV == "" {
		var list agentopsv1alpha1.SignalList
		if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
			return nil, fmt.Errorf("kube: list signals to start watch: %w", err)
		}
		startRV = list.ResourceVersion
	}
	w, err := s.client.Watch(ctx, &agentopsv1alpha1.SignalList{},
		client.InNamespace(s.namespace),
		&client.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: startRV, AllowWatchBookmarks: true}})
	if err != nil {
		return nil, fmt.Errorf("kube: watch signals: %w", err)
	}
	events := make(chan state.SignalEvent)
	go s.runWatch(ctx, w, events)
	return events, nil
}

func (s *SignalStore) runWatch(ctx context.Context, w watch.Interface, events chan<- state.SignalEvent) {
	defer close(events)
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.ResultChan():
			if !ok || ev.Type == watch.Error {
				return
			}
			if sig, ok := ev.Object.(*agentopsv1alpha1.Signal); ok && expiresAtFromAnnotations(sig.Annotations) != nil {
				s.expiryArmed.Store(true)
			}
			out, forward := translateSignalEvent(ev)
			if !forward {
				continue
			}
			select {
			case events <- out:
			case <-ctx.Done():
				return
			}
		}
	}
}

// translateSignalEvent maps one apiserver watch event, trusting the payload as
// translateWatchEvent does.
func translateSignalEvent(ev watch.Event) (state.SignalEvent, bool) {
	sig, ok := ev.Object.(*agentopsv1alpha1.Signal)
	if !ok {
		return state.SignalEvent{}, false
	}
	rev := state.Revision(sig.ResourceVersion)
	switch ev.Type {
	case watch.Added, watch.Modified:
		return state.SignalEvent{Type: state.EventPut, Key: state.Key(sig.Name), Revision: rev,
			Record: toSignalRecord(sig), Resume: rev}, true
	case watch.Deleted:
		return state.SignalEvent{Type: state.EventDelete, Key: state.Key(sig.Name), Resume: rev}, true
	case watch.Bookmark:
		if rev == "" {
			return state.SignalEvent{}, false
		}
		return state.SignalEvent{Type: state.EventBookmark, Resume: rev}, true
	default:
		return state.SignalEvent{}, false
	}
}

// Run starts the expiry sweep, as Store.Run does for conversations. The
// delete carries the revision and UID it read as preconditions, so a write that
// landed since keeps the record.
func (s *SignalStore) Run(ctx context.Context) {
	s.expiryArmed.Store(true)
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.expiryArmed.Swap(false) {
				s.sweepExpired(ctx)
			}
		}
	}
}

func (s *SignalStore) sweepExpired(ctx context.Context) {
	var list agentopsv1alpha1.SignalList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		s.expiryArmed.Store(true)
		return
	}
	now := time.Now()
	for i := range list.Items {
		sig := &list.Items[i]
		switch {
		case signalExpired(sig, now):
			err := s.client.Delete(ctx, sig, client.Preconditions{UID: &sig.UID, ResourceVersion: &sig.ResourceVersion})
			if err != nil && !apierrors.IsNotFound(err) {
				s.expiryArmed.Store(true) // a conflict or a failure: look again
			}
		case expiresAtFromAnnotations(sig.Annotations) != nil:
			s.expiryArmed.Store(true)
		}
	}
}

// GetCooldown reads SignalSource.status.cooldown. A source that is gone, or
// holds nothing, reads as an empty window.
func (s *SignalStore) GetCooldown(ctx context.Context, source string) ([]state.CooldownEntry, error) {
	var src agentopsv1alpha1.SignalSource
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: source}, &src); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("kube: get cooldown of source %q: %w", source, err)
	}
	return toCooldownEntries(src.Status.Cooldown), nil
}

// PutCooldown replaces SignalSource.status.cooldown with one merge patch of the
// status, exactly the write the router made before the store existed:
// last-writer-wins, no revision.
func (s *SignalStore) PutCooldown(ctx context.Context, source string, entries []state.CooldownEntry) error {
	var src agentopsv1alpha1.SignalSource
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: source}, &src); err != nil {
		if apierrors.IsNotFound(err) {
			return state.ErrNotFound
		}
		return fmt.Errorf("kube: read source %q to record its cooldown: %w", source, err)
	}
	patch := client.MergeFrom(src.DeepCopy())
	src.Status.Cooldown = toAPICooldownEntries(entries)
	if err := s.client.Status().Patch(ctx, &src, patch); err != nil {
		if apierrors.IsNotFound(err) {
			return state.ErrNotFound
		}
		return fmt.Errorf("kube: record cooldown of source %q: %w", source, err)
	}
	return nil
}

// RecordReceived is the router's receipt counter, moved verbatim: a
// read-modify-write of status.receivedTotal and lastReceived under the
// resourceVersion it read, retried on conflict. The stored status is unchanged.
func (s *SignalStore) RecordReceived(ctx context.Context, source string, at time.Time) error {
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		var src agentopsv1alpha1.SignalSource
		if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: source}, &src); err != nil {
			if apierrors.IsNotFound(err) {
				return state.ErrNotFound
			}
			return fmt.Errorf("kube: read source %q to count a receipt: %w", source, err)
		}
		patch := client.MergeFromWithOptions(src.DeepCopy(), client.MergeFromWithOptimisticLock{})
		last := metav1.NewTime(at)
		src.Status.LastReceived = &last
		src.Status.ReceivedTotal++
		err := s.client.Status().Patch(ctx, &src, patch)
		switch {
		case err == nil:
			return nil
		case apierrors.IsNotFound(err):
			return state.ErrNotFound
		case !apierrors.IsConflict(err):
			return fmt.Errorf("kube: count a receipt on source %q: %w", source, err)
		}
	}
	return fmt.Errorf("kube: count a receipt on source %q: conflict after %d attempts", source, statusUpdateMaxAttempts)
}

// GetReceived reads status.receivedTotal and lastReceived.
func (s *SignalStore) GetReceived(ctx context.Context, source string) (state.Received, error) {
	var src agentopsv1alpha1.SignalSource
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: source}, &src); err != nil {
		if apierrors.IsNotFound(err) {
			return state.Received{}, nil
		}
		return state.Received{}, fmt.Errorf("kube: get receipts of source %q: %w", source, err)
	}
	out := state.Received{Total: src.Status.ReceivedTotal}
	if src.Status.LastReceived != nil {
		at := src.Status.LastReceived.Time.UTC()
		out.Last = &at
	}
	return out, nil
}

// ReleaseSource deletes the Signals naming the source and clears its window.
//
// A name alone cannot tell a source from the one that replaced it (a GitOps
// prune, a reinstall), so a caller that knows the departed identity passes it:
// signals recorded under another identity belong to the successor and stay,
// and so does the successor's window, while an object by that name exists with
// a different identity. Every delete carries the UID it listed, so a signal
// replaced between the list and the delete is not taken with it.
func (s *SignalStore) ReleaseSource(ctx context.Context, source, sourceID string) error {
	var list agentopsv1alpha1.SignalList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return fmt.Errorf("kube: list signals of source %q: %w", source, err)
	}
	for i := range list.Items {
		sig := &list.Items[i]
		if sig.Spec.SourceRef.Name != source || isSuccessorsSignal(sig, sourceID) {
			continue
		}
		err := s.client.Delete(ctx, sig, client.Preconditions{UID: &sig.UID})
		if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			return fmt.Errorf("kube: release signal %q of source %q: %w", sig.Name, source, err)
		}
	}
	successor, err := s.holdsSuccessor(ctx, source, sourceID)
	if err != nil || successor {
		return err
	}
	if err := s.PutCooldown(ctx, source, nil); err != nil && !errors.Is(err, state.ErrNotFound) {
		return err
	}
	return nil
}

// isSuccessorsSignal reports whether sig is recorded under an identity other
// than the departed one. A signal with no recorded identity is released.
func isSuccessorsSignal(sig *agentopsv1alpha1.Signal, departed string) bool {
	owner := ownerSourceID(sig)
	return departed != "" && owner != "" && owner != departed
}

// holdsSuccessor reports whether an object named source exists under an
// identity other than the departed one, whose window must not be cleared.
func (s *SignalStore) holdsSuccessor(ctx context.Context, source, departed string) (bool, error) {
	if departed == "" {
		return false, nil
	}
	var src agentopsv1alpha1.SignalSource
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: source}, &src); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("kube: read source %q to release it: %w", source, err)
	}
	return string(src.UID) != departed, nil
}

var _ state.SignalStore = (*SignalStore)(nil)
