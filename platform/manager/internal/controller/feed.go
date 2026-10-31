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

// startFeed is the reconciler's event source (state-store design D-6): the
// store's Watch feed turned into reconcile requests, the way the informer on
// the Conversation CRD used to be. It runs under the controller's own start, so
// like every reconciler it works only while this process leads.
func (r *ConversationReconciler) startFeed(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	go r.pumpFeed(ctx, q)
	return nil
}

// pumpFeed keeps one Watch open for as long as ctx lives, resuming from the last
// revision it delivered. A watch with no resume point is followed by a relist of
// every record, started AFTER the watch so no change falls between the two —
// duplicates are harmless, a reconcile is idempotent. That relist is also the
// informer's own initial list.
func (r *ConversationReconciler) pumpFeed(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := log.FromContext(ctx).WithName("state-feed")
	var resume state.Revision
	for ctx.Err() == nil {
		started := time.Now()
		events, err := r.Store.Watch(ctx, resume)
		if err != nil {
			logger.Error(err, "opening the conversation feed")
			// A resume point the store rejects would otherwise be retried
			// once a second forever; start over and relist instead.
			resume = ""
		} else {
			if resume == "" {
				r.enqueueAll(ctx, q)
			}
			last, delivered := r.forward(ctx, events, q)
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
func (r *ConversationReconciler) forward(ctx context.Context, events <-chan state.Event,
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) (last state.Revision, delivered bool) {

	for ev := range events {
		if ev.Type != state.EventBookmark {
			q.Add(r.request(ev.Key))
		}
		last, delivered = ev.Resume, true
	}
	return last, delivered
}

// enqueueAll requests a reconcile of every record the store holds.
func (r *ConversationReconciler) enqueueAll(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	all, err := r.Store.List(ctx, state.ByAll, state.ListKey{})
	if err != nil {
		log.FromContext(ctx).Error(err, "listing conversations for the initial reconcile")
		return
	}
	for i := range all {
		q.Add(r.request(state.Key(all[i].Name)))
	}
}

func (r *ConversationReconciler) request(key state.Key) reconcile.Request {
	return reconcile.Request{NamespacedName: namespacedName(r.Namespace, string(key))}
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
