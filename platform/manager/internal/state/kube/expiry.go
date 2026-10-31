package kube

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// ExpiresAtAnnotation stores state.ConversationRecord.ExpiresAt. There is
// no CRD schema change in this backend (the proposal's own acceptance
// criterion), so the generic, backend-agnostic expiry field the contract
// declares (record.go: "no conversation written by this change's manager
// sets it — it exists for signal-persistence's terminal records") is
// carried as plain ObjectMeta, not a typed field.
const ExpiresAtAnnotation = "agentops.dev/state-expires-at"

// isExpired reports whether conv's expiry annotation names an instant
// strictly before now. Used by Get and List to make an expired record
// "invisible before it is gone" (spec: "An expired record is invisible
// before it is gone") even though the object still exists.
func isExpired(conv *agentopsv1alpha1.Conversation, now time.Time) bool {
	at := expiresAtFromAnnotations(conv.Annotations)
	return at != nil && at.Before(now)
}

func expiresAtFromAnnotations(annotations map[string]string) *time.Time {
	v, ok := annotations[ExpiresAtAnnotation]
	if !ok || v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		// A malformed value never written by this backend. Treat it the
		// conservative way every other unparsable-timestamp path in this
		// codebase does: as carrying no usable information, rather than
		// panicking on data this process did not itself produce.
		return nil
	}
	return &t
}

// setExpiresAtAnnotation writes or clears the annotation on conv to match
// at, called from applySpecAndMeta on every Create/Update so the annotation
// always reflects the caller's record exactly (never merged with whatever
// was there before).
func setExpiresAtAnnotation(conv *agentopsv1alpha1.Conversation, at *time.Time) {
	if at == nil {
		if conv.Annotations != nil {
			delete(conv.Annotations, ExpiresAtAnnotation)
		}
		return
	}
	if conv.Annotations == nil {
		conv.Annotations = map[string]string{}
	}
	conv.Annotations[ExpiresAtAnnotation] = at.UTC().Format(time.RFC3339Nano)
}

// Run starts the expiry sweep: every sweepInterval (DefaultSweepInterval
// unless overridden via NewStoreWithSweepInterval), list the namespace's
// Conversations and Delete every one whose expiry has passed (store.go's
// Store: "SHALL eventually remove it. How the removal happens is the
// backend's own"). The resulting Delete flows through the ordinary
// apiserver watch like any other delete, so a Watch consumer sees the
// EventDelete the spec requires ("Expiry is observable on the feed") with
// no special-casing in watch.go.
//
// Not started by NewStore (see NewStore's own comment) — call this only
// once something actually wires this backend up (section 4 of the
// state-store change).
func (s *Store) Run(ctx context.Context) {
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepExpired(ctx)
		}
	}
}

func (s *Store) sweepExpired(ctx context.Context) {
	var list agentopsv1alpha1.ConversationList
	if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		// Best-effort: the next tick tries again. Run carries no logger
		// (NewStore depends on nothing but client.WithWatch), and a
		// transient list failure here is indistinguishable from "nothing
		// to sweep this tick" to any caller — there is nothing yet for
		// either to act on.
		return
	}
	now := time.Now()
	for i := range list.Items {
		conv := &list.Items[i]
		if isExpired(conv, now) {
			_ = s.client.Delete(ctx, conv)
		}
	}
}
