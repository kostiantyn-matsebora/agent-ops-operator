package main

import (
	"context"
	"errors"
	"strconv"
)

// kube_split.go: the console's one departure from a single-namespace Kube
// client (split-conversation-namespace, console-deployment spec). Runtime
// pods render in the CONVERSATIONS namespace, so the Infrastructure view's
// pod watch must cover both namespaces or a conversation's own pod becomes
// invisible the moment the release splits. Every other kind this module
// reads — every agentops.dev kind, cronjobs, deployments — stays
// control-plane only; this is NOT a general "watch everywhere" mechanism.
//
// Conversations and Signals are unaffected: they never reach the Kubernetes
// API at all (state.go reads both through the manager's /state/* surface),
// so there is nothing here for this file to widen for them.

// podsKind is the one Kinds/InstallKinds entry this file treats specially.
const podsKind = "pods"

// podsAcrossNamespaces wraps the control-namespace Kube client every other
// kind already uses and adds a second, conversations-namespace one, used for
// "pods" alone. A single-namespace release builds both from the SAME
// namespace (main.go), so this is still exactly one list and one watch in
// substance — merging two identical sources — never a branch on whether the
// pair differs.
type podsAcrossNamespaces struct {
	control       *Kube
	conversations *Kube
}

// newPodsAcrossNamespaces builds the merged source. It implements apiSource
// (cache.go) and is handed to newCompositeSource in place of the bare Kube
// client this module used before the split.
func newPodsAcrossNamespaces(control, conversations *Kube) *podsAcrossNamespaces {
	return &podsAcrossNamespaces{control: control, conversations: conversations}
}

// List returns every kind's objects from the control namespace, except pods,
// which are the concatenation of both namespaces' lists.
func (p *podsAcrossNamespaces) List(ctx context.Context, kind string) ([]*Object, string, error) {
	if kind != podsKind {
		return p.control.List(ctx, kind)
	}
	controlPods, controlRV, err := p.control.List(ctx, kind)
	if err != nil {
		return nil, "", err
	}
	convPods, convRV, err := p.conversations.List(ctx, kind)
	if err != nil {
		return nil, "", err
	}
	// The resourceVersion returned here is the one BOTH inner watches below
	// resume from. etcd's mod-revision — what every Kubernetes resourceVersion
	// actually is — is a single counter for the whole cluster, so a value
	// from either namespace's list is a valid resume point for the other
	// too. The OLDER of the two is the safe choice: resuming a bit early only
	// redelivers an event the cache's apply() already treats as an
	// idempotent upsert, or — if the server has compacted past it —
	// produces ErrWatchExpired, which the caller (cache.go) already relists
	// on. Resuming from the NEWER one risks silently skipping an event the
	// other namespace's own list missed by the width of one HTTP round trip.
	return append(controlPods, convPods...), olderResourceVersion(controlRV, convRV), nil
}

// Get tries the control namespace first (where most install kinds, and most
// installs' pods, live), falling back to the conversations namespace on a
// definite 404 — the same "not found here, maybe found there" shape a
// cluster split across namespaces requires, with no namespace in the
// apiSource interface itself to choose from.
func (p *podsAcrossNamespaces) Get(ctx context.Context, kind, name string) (*Object, error) {
	if kind != podsKind {
		return p.control.Get(ctx, kind, name)
	}
	obj, err := p.control.Get(ctx, kind, name)
	if err == nil {
		return obj, nil
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) && apiErr.Code == 404 {
		return p.conversations.Get(ctx, kind, name)
	}
	return nil, err
}

// Watch runs both namespaces' pod watches concurrently from the SAME
// resourceVersion (see List's own comment for why that is safe), forwarding
// every event from either through fn untouched — the cache's store is keyed
// by name alone, and a runtime pod's name never collides with a control-
// plane one (agentops-conv-<conversation> against agentops-<component>-<hash>).
//
// Either side ending — cleanly, on a watch expiry, or on an error — ends the
// PAIR: the two must resume together, from one resourceVersion, or the next
// call would resume one namespace from a point the other has already moved
// past.
func (p *podsAcrossNamespaces) Watch(ctx context.Context, kind, resourceVersion string, fn func(eventType string, obj *Object)) error {
	if kind != podsKind {
		return p.control.Watch(ctx, kind, resourceVersion, fn)
	}
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan error, 2)
	run := func(k *Kube) {
		err := k.Watch(watchCtx, kind, resourceVersion, fn)
		cancel() // either side ending ends the pair
		results <- err
	}
	go run(p.control)
	go run(p.conversations)

	first, second := <-results, <-results

	// ErrWatchExpired wins over any other outcome: cache.go relists
	// specifically on it, and relisting is always safe for the side that did
	// not itself expire.
	for _, err := range [...]error{first, second} {
		if errors.Is(err, ErrWatchExpired) {
			return ErrWatchExpired
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if first != nil {
		return first
	}
	return second
}

// olderResourceVersion answers the smaller of two resourceVersions, read as
// decimal etcd mod-revisions — what every Kubernetes implementation emits.
// An unparseable or empty value loses to the other: "" already means "watch
// from now" to Kube.Watch, which neither List call here could have meant.
func olderResourceVersion(a, b string) string {
	an, aok := parseResourceVersion(a)
	bn, bok := parseResourceVersion(b)
	switch {
	case !aok:
		return b
	case !bok:
		return a
	case an <= bn:
		return a
	default:
		return b
	}
}

func parseResourceVersion(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}
