package kube

import (
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
)

// CacheByObject is where the manager's cache keeps each kind this backend
// stores outside the control namespace, mirroring OpenStores' placement:
// Conversation and ConversationInput in ns.Conversations alone. Signals and
// the rest sit in ns.Control, the cache's default, so they need no entry.
//
// It lives here because this backend is the one package allowed to name a
// stored kind (internal/state/static_test.go).
func CacheByObject(ns namespace.Namespaces) map[client.Object]cache.ByObject {
	conversations := map[string]cache.Config{ns.Conversations: {}}
	return map[client.Object]cache.ByObject{
		&agentopsv1alpha1.Conversation{}:      {Namespaces: conversations},
		&agentopsv1alpha1.ConversationInput{}: {Namespaces: conversations},
	}
}
