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
// repeats without bound. Nothing downstream catches it: the fingerprint is
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
// Three INDEPENDENT mechanisms, because any one of them can be blind:
//
//	1. name prefix     — needs no object read, so it holds before any cache is
//	                     warm; that is exactly when a mass pod-creation failure
//	                     is most likely to be in flight
//	2. owner/label     — precise, but needs the pod cache (see enrich.go)
//	3. own namespace   — coarse; the ONLY one of the three that is configurable
//
// Mechanisms 1 and 2 are deliberately NOT configurable. A deny-list is
// editable, and an editable loop breaker means a values typo can take out a
// cluster.

// ownedNamePrefixes are the workload names agent-ops gives its own pods:
// runtime pods (runtimepod.PodName), channel adapter workloads, signal adapter
// workloads, and the housekeeping CronJob's pods. Matching on the name alone
// requires no API read.
//
// The housekeeping entry matters more than its size suggests: a CronJob fails
// on a SCHEDULE, so without it every failed cleanup run emits a Warning event,
// that event becomes a signal, and the signal wakes an agent about agent-ops'
// own maintenance — repeatedly. agent-ops' own health is STATUS, not SIGNAL.
var ownedNamePrefixes = []string{
	"agentops-conv-",
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
	// POD_NAMESPACE by ChannelAdapter/SignalAdapter kubernetesAccess.
	ownNamespace string
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
	return &selfExcluder{ownNamespace: os.Getenv("POD_NAMESPACE")}
}

// withCache returns the excluder wired to an object cache, enabling mechanism 2.
func (s *selfExcluder) withCache(c objectLookup) *selfExcluder {
	s.cache = c
	return s
}

// Excludes reports whether this event is about agent-ops' own machinery, and
// why. includeOwnNamespace disables mechanism 3 ONLY — 1 and 2 always apply.
//
// A nil receiver still applies mechanism 1. That is deliberate: the failure
// mode this invariant guards against must not be reachable by forgetting to
// wire the excluder up, so an unconfigured excluder degrades to the rule that
// needs no configuration rather than to no rule at all.
func (s *selfExcluder) Excludes(ev *Event, includeOwnNamespace bool) (bool, string) {
	name := ev.InvolvedObject.Name

	// 1. name prefix — no read, works with a cold cache or no excluder at all
	for _, p := range ownedNamePrefixes {
		if strings.HasPrefix(name, p) {
			return true, "agent-ops workload name prefix " + p
		}
	}
	if s == nil {
		return false, ""
	}

	// 2. owner/label — precise, needs the cache
	if s.cache != nil {
		if owned, known := s.cache.OwnedByAgentOps(ev.Namespace(), ev.InvolvedObject.Kind, name); known && owned {
			return true, "object is labelled or owned by agent-ops"
		}
	}

	// 3. own namespace — the configurable one
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
