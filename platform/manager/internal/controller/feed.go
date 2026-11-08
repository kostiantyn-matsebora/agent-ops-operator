package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// feedRetryDelay is how long the feed waits before asking the store for a new
// watch after the last one ended.
const feedRetryDelay = time.Second

// feedResumeGrace is how short a watch may live, having delivered nothing,
// before its resume point is presumed unusable and the feed relists instead.
const feedResumeGrace = time.Second

// feed turns one store's Watch into reconcile requests (state-store design
// D-6, D-14): the Conversation and the Signal reconcilers run the same loop
// over different event types, so the loop is written once. E is the store's
// event type.
type feed[E any] struct {
	// what names the records, for the log lines.
	what string
	// open starts a Watch from a resume point.
	open func(ctx context.Context, resume state.Revision) (<-chan E, error)
	// describe reads an event's key, its resume point, and whether it is a
	// bookmark (a resume point only, naming no record).
	describe func(E) (key state.Key, resume state.Revision, bookmark bool)
	// list returns the key of every record the store holds.
	list func(ctx context.Context) ([]state.Key, error)
	// request builds the reconcile request of a key.
	request func(state.Key) reconcile.Request
}

// pump keeps one Watch open for as long as ctx lives, resuming from the last
// revision it delivered. A watch with no resume point is followed by a relist of
// every record, started AFTER the watch so no change falls between the two —
// duplicates are harmless, a reconcile is idempotent. That relist is also the
// informer's own initial list.
func (f feed[E]) pump(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx).WithName("state-feed")
	var resume state.Revision
	for ctx.Err() == nil {
		started := time.Now()
		events, err := f.open(ctx, resume)
		if err != nil {
			logger.Error(err, "opening the "+f.what+" feed")
			// A resume point the store rejects would otherwise be retried
			// once a second forever; start over and relist instead.
			resume = ""
		} else {
			if resume == "" {
				f.enqueueAll(ctx, q)
			}
			last, delivered := f.forward(events, q)
			if delivered {
				resume = last
			} else if resume != "" && time.Since(started) < feedResumeGrace {
				resume = "" // the resume point was refused: start over and relist
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(feedRetryDelay):
		}
	}
}

// forward enqueues the key of every event until the feed closes, and reports
// the last resume point it saw.
func (f feed[E]) forward(events <-chan E, q workqueue.TypedRateLimitingInterface[reconcile.Request]) (last state.Revision, delivered bool) {
	for ev := range events {
		key, resume, bookmark := f.describe(ev)
		if !bookmark {
			q.Add(f.request(key))
		}
		last, delivered = resume, true
	}
	return last, delivered
}

// enqueueAll requests a reconcile of every record the store holds. The list is
// the only thing that reconciles a record already present when the watch
// opens, and a failed one is never repeated by the watch, so it is retried at
// the feed's retry delay until it succeeds or ctx ends.
func (f feed[E]) enqueueAll(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	for {
		keys, err := f.list(ctx)
		if err == nil {
			for _, key := range keys {
				q.Add(f.request(key))
			}
			return
		}
		log.FromContext(ctx).Error(err, "listing "+f.what+"s for the initial reconcile; retrying")
		select {
		case <-ctx.Done():
			return
		case <-time.After(feedRetryDelay):
		}
	}
}

// startFeed is the reconciler's event source (state-store design D-6): the
// store's Watch feed turned into reconcile requests, the way the informer on
// the Conversation CRD used to be. It runs under the controller's own start, so
// like every reconciler it works only while this process leads.
func (r *ConversationReconciler) startFeed(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	go r.pumpFeed(ctx, q)
	return nil
}

func (r *ConversationReconciler) pumpFeed(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	feed[state.Event[state.ConversationRecord]]{
		what: "conversation",
		open: r.Conversations.Watch,
		describe: func(ev state.Event[state.ConversationRecord]) (state.Key, state.Revision, bool) {
			return ev.Key, ev.Resume, ev.Type == state.EventBookmark
		},
		list: func(ctx context.Context) ([]state.Key, error) {
			all, err := r.Conversations.List(ctx, state.ByAll, "")
			keys := make([]state.Key, 0, len(all))
			for i := range all {
				keys = append(keys, state.Key(all[i].Name))
			}
			return keys, err
		},
		request: r.request,
	}.pump(ctx, q)
}

func (r *ConversationReconciler) request(key state.Key) reconcile.Request {
	return reconcile.Request{NamespacedName: namespacedName(r.Namespace, string(key))}
}

// startFeed is the Signal reconciler's event source (design D-14), the same
// loop over the SignalStore's Watch.
func (r *SignalReconciler) startFeed(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	go feed[state.Event[state.SignalRecord]]{
		what: "signal",
		open: r.Signals.Watch,
		describe: func(ev state.Event[state.SignalRecord]) (state.Key, state.Revision, bool) {
			return ev.Key, ev.Resume, ev.Type == state.EventBookmark
		},
		list: func(ctx context.Context) ([]state.Key, error) {
			all, err := r.Signals.List(ctx, state.SignalAll, "")
			keys := make([]state.Key, 0, len(all))
			for i := range all {
				keys = append(keys, state.Key(all[i].Name))
			}
			return keys, err
		},
		request: func(key state.Key) reconcile.Request {
			return reconcile.Request{NamespacedName: namespacedName(r.Namespace, string(key))}
		},
	}.pump(ctx, q)
	return nil
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
