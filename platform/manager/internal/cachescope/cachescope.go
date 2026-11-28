// Package cachescope scopes the manager's informer cache to the namespaces
// its Roles grant, kind by kind. Its own package so the envtest suite starts
// a cache over exactly the options production does.
package cachescope

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

// Options scopes every informer to the namespaces the manager's Roles
// actually grant it, kind by kind.
//
// The DEFAULT is the control namespace alone: every wiring and identity kind,
// signals, adapter Deployments and Services live there, and the conversations
// Role grants none of them. A cache spanning both
// namespaces for every kind starts one informer per kind in each, and a
// forbidden list never syncs — the manager timed out waiting for its caches
// and crash-looped on the first split install.
//
// The overrides are the kinds the conversations Role grants:
//
//   - Conversation and ConversationInput live in conversations only, as
//     the kube state backend places them (kube.CacheByObject). So do PVCs.
//   - Pods and ConfigMaps are read in both: runtime pods and their MCP
//     ConfigMaps in conversations, the manager's own replicas (the leader
//     proxy, the old-replica check) and control-plane ConfigMaps in control.
//
// Equal namespaces collapse every map below to one key, which is today's
// single-namespace cache exactly — no branch on whether the pair differs.
func Options(ns namespace.Namespaces) cache.Options {
	control := map[string]cache.Config{ns.Control: {}}
	conversations := map[string]cache.Config{ns.Conversations: {}}
	both := map[string]cache.Config{ns.Control: {}, ns.Conversations: {}}
	byObject := kube.CacheByObject(ns)
	byObject[&corev1.PersistentVolumeClaim{}] = cache.ByObject{Namespaces: conversations}
	byObject[&corev1.Pod{}] = cache.ByObject{Namespaces: both}
	byObject[&corev1.ConfigMap{}] = cache.ByObject{Namespaces: both}
	return cache.Options{DefaultNamespaces: control, ByObject: byObject}
}
