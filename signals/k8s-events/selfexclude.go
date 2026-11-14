package main

import (
	"os"
	"strings"
)

// Self-exclusion: agent-ops never ingests its own machinery as a signal.
//
// This is the signal-lane twin of the no-relay-loops rule for channels — the
// system must never re-ingest its own output as input. Without it, a
// Conversation whose runtime pod cannot start emits a Warning event on that
// pod; the event becomes a signal; the signal opens a Conversation; that
// Conversation creates another runtime pod under a NEW name, and the cycle
// repeats without bound. Nothing downstream catches it: the initiationId is
// fresh (new pod name), the workload is fresh (the owner is the Conversation
// CR), and even a correct liveness re-check passes it through because the pod
// really is still broken. The conversation cap bounds concurrent pods and the
// queued-conversation bound throttles the backlog, but neither BREAKS the
// cycle — the runaway still fills etcd, only more slowly.
//
// agent-ops' own health is STATUS, not SIGNAL: the reconciler already holds the
// pod's failure. Routing that knowledge back through the ingest pipeline to
// wake an agent is the architectural error, not merely a noisy one.
//
// FOUR INDEPENDENT mechanisms, because any one of them can be blind — numbered
// to match docs/contracts/signal-self-exclusion's own spec, not evaluation
// order (Excludes still checks the name prefix FIRST, since that is the one
// that must hold before any cache is warm):
//
//	1. owner/label                   — precise, but needs the pod cache
//	                                   (see enrich.go)
//	2. name prefix                   — needs no object read, so it holds
//	                                   before any cache is warm; that is
//	                                   exactly when a mass pod-creation
//	                                   failure is most likely to be in flight
//	3. release-namespace exclusion    — coarse; the ONLY one of the four that
//	                                   is configurable, for an install that
//	                                   co-locates its own workloads
//	4. conversations-namespace exclusion — every event in CONVERSATION_NAMESPACE
//	                                   is dropped whenever it is set and
//	                                   differs from POD_NAMESPACE; that
//	                                   namespace holds only agent-ops
//	                                   machinery by construction (split-
//	                                   conversation-namespace), so, like
//	                                   mechanism 2, it needs no object read
//
// Mechanisms 1, 2 and 4 are deliberately NOT configurable. A deny-list is
// editable, and an editable loop breaker means a values typo can take out a
// cluster. Mechanism 4 cannot be the same deny-list risk: it is a single
// namespace name the manager itself resolved at startup, never a pattern an
// adopter edits.

// ownedNamePrefixes are the workload names agent-ops gives its own pods:
// runtime pods (runtimepod.PodName), their MCP ConfigMaps
// (ConversationReconciler.MCPConfigMapName), channel adapter workloads,
// signal adapter workloads, and the housekeeping CronJob's pods. Matching on
// the name alone requires no API read.
//
// The housekeeping entry matters more than its size suggests: a CronJob fails
// on a SCHEDULE, so without it every failed cleanup run emits a Warning event,
// that event becomes a signal, and the signal wakes an agent about agent-ops'
// own maintenance — repeatedly. agent-ops' own health is STATUS, not SIGNAL.
//
// agentops-mcp-conv- (split-conversation-namespace task 5.3) is what excludes
// an MCP ConfigMap under the nats backend, where it carries no owner
// reference back to a Conversation for mechanism 1 to follow — mechanism 2
// is the only one of the four that catches it there.
var ownedNamePrefixes = []string{
	"agentops-conv-",
	"agentops-mcp-conv-",
	"agentops-adapter-",
	"agentops-signal-",
	"agentops-housekeeping-",
}

// ownedAppLabels are the `app.kubernetes.io/name` values agent-ops stamps on
// its own workloads.
var ownedAppLabels = map[string]bool{
	"agentops-runtime":        true,
	"agentops-manager":        true,
	"agentops-adapter":        true,
	"agentops-signal-adapter": true,
}

// ownLabelPrefix marks any object carrying an agent-ops label key
// (agentops.dev/conversation, agentops.dev/adapter, agentops.dev/signal-adapter).
// Prefix-matching the KEY rather than enumerating keys keeps a future label
// from silently escaping the rule.
const ownLabelPrefix = "agentops.dev/"

// selfExcluder decides whether an event's involved object belongs to agent-ops.
//
// It holds no state of its own beyond the operator's namespace; the label and
// owner rules are answered by the object cache, which is nil until one is
// wired in (mechanism 1 still works without it — that is its entire point).
type selfExcluder struct {
	// ownNamespace is the namespace this adapter runs in, injected as
	// POD_NAMESPACE (a downward-API field) by the SignalAdapter reconciler on
	// every adapter workload — it is not a permission, and does not depend on
	// spec.serviceAccountName.
	ownNamespace string
	// conversationsNamespace is CONVERSATION_NAMESPACE (split-conversation-
	// namespace), a LITERAL value the SignalAdapter reconciler injects
	// beside POD_NAMESPACE — never a downward-API field, since it names a
	// namespace other than the one this pod runs in. Empty, or equal to
	// ownNamespace, means a single-namespace release: mechanism 4 then
	// never applies, and mechanisms 1-3 govern alone.
	conversationsNamespace string
	// cache answers the owner/label rule. Nil is valid and means only the
	// name-prefix and namespace rules apply.
	cache objectLookup
}

// objectLookup is the slice of the object cache self-exclusion needs. Keeping
// it an interface lets mechanism 1 be tested and shipped with no cache at all.
type objectLookup interface {
	// OwnedByAgentOps reports whether the named object carries an agent-ops
	// label or has an owner chain reaching a Conversation. The second return
	// is false when the object is unknown to the cache, so callers can tell
	// "not ours" from "cannot say".
	OwnedByAgentOps(namespace, kind, name string) (owned bool, known bool)
}

func newSelfExcluder() *selfExcluder {
	return &selfExcluder{
		ownNamespace:           os.Getenv("POD_NAMESPACE"),
		conversationsNamespace: os.Getenv("CONVERSATION_NAMESPACE"),
	}
}

// withCache returns the excluder wired to an object cache, enabling mechanism 2.
func (s *selfExcluder) withCache(c objectLookup) *selfExcluder {
	s.cache = c
	return s
}

// Excludes reports whether this event is about agent-ops' own machinery, and
// why. includeOwnNamespace disables mechanism 3 ONLY — 1, 2 and 4 always
// apply, mechanism 4 whenever it is set and differs from ownNamespace.
//
// A nil receiver still applies mechanism 2 (the name-prefix rule). That is
// deliberate: the failure mode this invariant guards against must not be
// reachable by forgetting to wire the excluder up, so an unconfigured
// excluder degrades to the rule that needs no configuration rather than to
// no rule at all.
func (s *selfExcluder) Excludes(ev *Event, includeOwnNamespace bool) (bool, string) {
	name := ev.InvolvedObject.Name

	// 2. name prefix — no read, works with a cold cache or no excluder at
	// all, so it is checked FIRST even though the spec numbers it second.
	for _, p := range ownedNamePrefixes {
		if strings.HasPrefix(name, p) {
			return true, "agent-ops workload name prefix " + p
		}
	}
	if s == nil {
		return false, ""
	}

	// 4. conversations-namespace exclusion — also no read, and NOT gated by
	// includeOwnNamespace: that flag names an installation co-locating its
	// OWN workloads beside the operator, and the conversations namespace
	// never holds anything but agent-ops machinery.
	if s.conversationsNamespace != "" && s.conversationsNamespace != s.ownNamespace && ev.Namespace() == s.conversationsNamespace {
		return true, "event is in the conversations namespace " + s.conversationsNamespace
	}

	// 1. owner/label — precise, needs the cache
	if s.cache != nil {
		if owned, known := s.cache.OwnedByAgentOps(ev.Namespace(), ev.InvolvedObject.Kind, name); known && owned {
			return true, "object is labelled or owned by agent-ops"
		}
	}

	// 3. release-namespace exclusion — the configurable one
	if !includeOwnNamespace && s.ownNamespace != "" && ev.Namespace() == s.ownNamespace {
		return true, "event is in the operator's own namespace " + s.ownNamespace
	}

	return false, ""
}

// isOwnedLabels reports whether a label set marks an agent-ops workload. Shared
// by the object cache so the vocabulary lives in one place.
func isOwnedLabels(labels map[string]string) bool {
	if ownedAppLabels[labels["app.kubernetes.io/name"]] {
		return true
	}
	for k := range labels {
		if strings.HasPrefix(k, ownLabelPrefix) {
			return true
		}
	}
	return false
}
