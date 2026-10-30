package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Watch delivers an at-least-once feed of state.Event over the namespace's
// Conversations, via the apiserver's own watch (store.go's Store.Watch
// contract).
//
// resume == "" means "from now" (store.go: "empty Revision starts from
// now"). To make that literal and deterministic — never replaying history —
// this does a plain List first, captures the list's own resourceVersion,
// discards every item without emitting it, then opens the apiserver watch
// starting strictly after that resourceVersion. A non-empty resume skips
// the List and watches from it directly.
func (s *Store) Watch(ctx context.Context, resume state.Revision) (<-chan state.Event, error) {
	startRV := string(resume)
	if startRV == "" {
		var list agentopsv1alpha1.ConversationList
		if err := s.client.List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
			return nil, fmt.Errorf("kube: list conversations to start watch: %w", err)
		}
		startRV = list.ResourceVersion
	}

	w, err := s.client.Watch(ctx, &agentopsv1alpha1.ConversationList{},
		client.InNamespace(s.namespace),
		&client.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: startRV}})
	if err != nil {
		return nil, fmt.Errorf("kube: watch conversations: %w", err)
	}

	events := make(chan state.Event)
	go runWatch(ctx, w, events)
	return events, nil
}

// runWatch drains w into events until ctx is done or w's own channel
// closes (an apiserver-side watch.Error is the usual reason — the
// apiserver closed the connection, nothing left to read). Either way it
// stops w and closes events, mirroring how an informer behaves on
// shutdown.
func runWatch(ctx context.Context, w watch.Interface, events chan<- state.Event) {
	defer close(events)
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.ResultChan():
			if !ok {
				return
			}
			if ev.Type == watch.Error {
				// The apiserver reported a terminal failure on this watch
				// (store.go's Watch: "a backend reports a terminal failure
				// by closing the channel early, with no error value
				// crossing this contract"). Returning here, rather than
				// waiting for ResultChan to close on its own, is what
				// makes that immediate rather than best-effort.
				return
			}
			out, forward := translateWatchEvent(ev)
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

// translateWatchEvent maps one apiserver watch.Event to a state.Event,
// trusting the raw watch payload directly — never re-reading the object.
// This forwards EVERY real write exactly once, in order, which is what the
// "no gap" half of the contract rests on: Create and Update each land as
// two apiserver calls (the main object, then the status subresource —
// store.go's Update comment), so a consumer may see a spec change with the
// PRIOR status still attached as its own, intermediate, genuinely-once-true
// state. That is an accurate history, not a defect — a design that instead
// re-read the object per notification was tried and reverted: under a
// burst of rapid writes it let a slow consumer's catch-up collapse many
// real intermediate states into repeats of whatever was CURRENT by the
// time each notification was finally processed, which is a real gap wearing
// the contract's own "duplicates permitted" sentence as cover. A caller
// that wants a stable basis for its next write reads a snapshot's
// Revision knowing a sibling call from the SAME logical operation may
// still land after it — exactly as CAS already requires it to tolerate any
// other writer.
func translateWatchEvent(ev watch.Event) (state.Event, bool) {
	switch ev.Type {
	case watch.Added, watch.Modified:
		conv, ok := ev.Object.(*agentopsv1alpha1.Conversation)
		if !ok {
			return state.Event{}, false
		}
		rev := state.Revision(conv.ResourceVersion)
		return state.Event{
			Type:     state.EventPut,
			Key:      state.Key(conv.Name),
			Revision: rev,
			Record:   toRecord(conv),
			Resume:   rev,
		}, true
	case watch.Deleted:
		conv, ok := ev.Object.(*agentopsv1alpha1.Conversation)
		if !ok {
			return state.Event{}, false
		}
		rev := state.Revision(conv.ResourceVersion)
		return state.Event{
			Type:   state.EventDelete,
			Key:    state.Key(conv.Name),
			Resume: rev,
		}, true
	default:
		// watch.Bookmark, watch.Error: see the function comment.
		return state.Event{}, false
	}
}
