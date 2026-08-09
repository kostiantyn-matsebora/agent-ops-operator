// Manager introspection:
//
//	GET /status                     manager-internal runtime state
//	GET /pipelines/{name}/resolved  authoritative capability resolution
//
// THE BOUNDARY RULE: the manager exposes only what ONLY the manager knows. CR
// spec and status are read from the API server, with the API server's own RBAC,
// and are never proxied here — a manager that mirrors CRs becomes a second
// Kubernetes API with a second auth scope and its own staleness.
//
// What passes the rule, and why nothing else does:
//
//   - The OpQueue is in-memory BY DESIGN. Pending and claimed channel ops exist
//     in no Kubernetes object, so "is the queue backed up / is an adapter
//     claiming without completing" is unanswerable from outside this process.
//   - Runtime slots in use against the ceiling are counted from the live POD
//     list, not from status, for the same reason the admission gate is: a pod
//     stuck unschedulable or a lost status patch must not invent capacity.
//   - Cooldown state is per-source memory in this process.
//   - Capability resolution is manager LOGIC. A console that recomputed it
//     would eventually disagree with the one that runs, and the console's whole
//     claim is that it cannot disagree with the system.
package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/metrics"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

// LeaseName is the controller-runtime leader-election lease this manager holds
// — the LeaderElectionID from cmd/manager. Reading its holder answers "which
// replica is reconciling", a fact no CR of ours carries.
const LeaseName = "agentops-manager.agentops.dev"

// statusResponse is the manager's own state. Deliberately carries no CR spec or
// status: everything here is process memory or a count over pods.
type statusResponse struct {
	Version string `json:"version,omitempty"`
	// Leader is the lease holder identity, "" when the lease is unreadable
	// (the manager is granted no lease read in some installs) — reported as
	// unknown rather than guessed.
	Leader string `json:"leader,omitempty"`
	Now    string `json:"now"`

	RuntimeSlots runtimeSlots `json:"runtimeSlots"`
	// Queues are per-adapter op depths with the identity of the oldest stuck
	// item — the "which one" that a metric label must never carry.
	Queues    []chat.QueueStat `json:"queues"`
	Cooldowns []cooldownStat   `json:"cooldowns"`
}

type runtimeSlots struct {
	// InUse counts POD-BACKED conversations, from the live pod list — the same
	// definition the admission gate uses, so /status and admission cannot
	// disagree about whether there is room.
	InUse int `json:"inUse"`
	Max   int `json:"max"`
	// Waiting counts conversations that need a runtime pod and hold none.
	Waiting int `json:"waiting"`
}

type cooldownStat struct {
	Source string `json:"source"`
	// Suppressed is how many fingerprints are currently within their cooldown
	// window. A suppressed lane looks exactly like an idle one on a graph;
	// this is what tells them apart.
	Suppressed int     `json:"suppressed"`
	WindowSecs float64 `json:"windowSeconds"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slots, err := s.runtimeSlots(ctx)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := statusResponse{
		Version:      s.Version,
		Leader:       s.leaderIdentity(ctx),
		Now:          time.Now().UTC().Format(time.RFC3339Nano),
		RuntimeSlots: slots,
		Queues:       []chat.QueueStat{},
		Cooldowns:    []cooldownStat{},
	}
	if s.Ops != nil {
		out.Queues = s.Ops.Stats()
	}
	for name, cd := range s.cooldowns {
		n, window := cd.Stats()
		out.Cooldowns = append(out.Cooldowns, cooldownStat{Source: name, Suppressed: n, WindowSecs: window.Seconds()})
	}
	writeJSON(w, 200, out)
}

// leaderIdentity reads the leader-election lease holder. Best effort: the
// manager may not be granted leases in every install, and an unknown leader is
// reported as empty rather than as this replica.
func (s *Server) leaderIdentity(ctx context.Context) string {
	var lease coordinationv1.Lease
	// APIReader, not the cache: leader-election RBAC grants get/create/update on
	// leases, not list/watch, so a cached read would start an informer that is
	// forbidden and never syncs.
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: LeaseName}, &lease); err != nil {
		return ""
	}
	if lease.Spec.HolderIdentity == nil {
		return ""
	}
	return *lease.Spec.HolderIdentity
}

// runtimeSlots counts capacity the way admission does: live pods against the
// ceiling, and conversations that want a pod and have none.
func (s *Server) runtimeSlots(ctx context.Context) (runtimeSlots, error) {
	var pods corev1.PodList
	if err := s.Client.List(ctx, &pods, client.InNamespace(s.Namespace),
		client.MatchingLabels{runtimepod.LabelApp: runtimepod.LabelAppValue}); err != nil {
		return runtimeSlots{}, err
	}
	hasPod := map[string]bool{}
	inUse := 0
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Status.Phase != corev1.PodRunning && p.Status.Phase != corev1.PodPending {
			continue
		}
		inUse++
		hasPod[p.Labels[runtimepod.LabelConversation]] = true
	}
	var convs agentopsv1alpha1.ConversationList
	if err := s.Client.List(ctx, &convs, client.InNamespace(s.Namespace)); err != nil {
		return runtimeSlots{}, err
	}
	waiting := 0
	for i := range convs.Items {
		c := &convs.Items[i]
		if !c.DeletionTimestamp.IsZero() || hasPod[c.Name] {
			continue
		}
		if len(dispatch.PendingInputs(c)) > 0 || c.Status.Inflight != nil {
			waiting++
		}
	}
	return runtimeSlots{InUse: inUse, Max: s.MaxActiveConversations, Waiting: waiting}, nil
}

// MetricsSample renders the same in-memory state /status reports into the
// gauge shape the Prometheus collector needs. ONE source, two renderings: a
// second sampler would be the second implementation this design exists to
// avoid.
func (s *Server) MetricsSample() metrics.Sample {
	ctx := context.Background()
	out := metrics.Sample{
		ConversationsInflight: map[string]int{},
		CooldownsActive:       map[string]int{},
	}
	if s.Ops != nil {
		for _, q := range s.Ops.Stats() {
			out.Queues = append(out.Queues, metrics.QueueSample{
				Adapter: q.Adapter, Queued: q.Queued, Claimed: q.Claimed,
				OldestQueuedAge: q.OldestQueuedAgeSecs, OldestClaimedAge: q.OldestClaimedAgeSecs,
			})
		}
	}
	if slots, err := s.runtimeSlots(ctx); err == nil {
		out.RuntimeSlotsInUse, out.RuntimeSlotsMax = slots.InUse, slots.Max
	}
	for name, cd := range s.cooldowns {
		n, _ := cd.Stats()
		out.CooldownsActive[name] = n
	}
	// Pipelines are listed ONCE and matched per conversation: attributing each
	// conversation independently would list the pipeline set per row, on every
	// scrape.
	pipelines := chat.ReadyPipelines(ctx, s.Client, s.Namespace)
	var convs agentopsv1alpha1.ConversationList
	if err := s.Client.List(ctx, &convs, client.InNamespace(s.Namespace)); err == nil {
		for i := range convs.Items {
			c := &convs.Items[i]
			if c.Status.Inflight == nil {
				continue
			}
			name := ""
			if p := chat.MatchPipeline(pipelines, c); p != nil {
				name = p.Name
			}
			out.ConversationsInflight[name]++
		}
	}
	return out
}

// ---- resolved capabilities ---------------------------------------------------

// resolvedResponse is what an agent routed by this pipeline may actually reach.
//
// AllowedTools is the WIRING's half of the allowlist. The runtime composes it
// with the agent definition's own `tools:` per Mode — that composition needs the
// repository checked out, which only the runtime has — so this is reported for
// what it is rather than as a final answer the manager cannot produce.
type resolvedResponse struct {
	Pipeline string `json:"pipeline"`
	Profile  string `json:"profile"`
	Runtime  string `json:"runtime,omitempty"`

	// AllowedTools is composed in ref order, concatenated with dedup. An empty
	// binding reports an EMPTY list, never a fallback: a pipeline that grants no
	// tools is a configuration, and inventing a default here would silently
	// widen it.
	AllowedTools []string `json:"allowedTools"`
	ToolsMode    string   `json:"toolsMode"`
	Toolsets     []string `json:"toolsets"`

	MCPConfigs []string `json:"mcpConfigs"`
	// MCPServers are the server keys after the later-wins overlay.
	MCPServers []string `json:"mcpServers"`

	// Unresolved names refs that resolve to nothing. Reported rather than
	// silently skipped: a missing toolset is why an agent cannot do the thing
	// its wiring says it can.
	Unresolved []string `json:"unresolved,omitempty"`
}

func (s *Server) handlePipelineResolved(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	var p agentopsv1alpha1.Pipeline
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, &p); err != nil {
		writeJSON(w, 404, map[string]string{"error": "unknown pipeline " + name})
		return
	}
	out := resolvedResponse{
		Pipeline:     p.Name,
		Profile:      p.Spec.ProfileRef.Name,
		ToolsMode:    dispatch.ToolsModeOf(p.Spec.Toolsets),
		AllowedTools: []string{},
		Toolsets:     []string{},
		MCPConfigs:   []string{},
		MCPServers:   []string{},
	}

	var profile agentopsv1alpha1.AgentProfile
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: p.Spec.ProfileRef.Name}, &profile); err != nil {
		out.Unresolved = append(out.Unresolved, "AgentProfile/"+p.Spec.ProfileRef.Name)
	} else {
		out.Runtime = s.runtimeName(ctx, &profile)
	}

	// Composed through the SAME function dispatch uses, over the same inputs.
	// Reimplementing composition here is the one thing this endpoint exists to
	// prevent — the console asks precisely so a second implementation cannot
	// drift from the one that runs.
	var byRef [][]string
	if p.Spec.Toolsets != nil {
		for _, ref := range p.Spec.Toolsets.Refs {
			out.Toolsets = append(out.Toolsets, ref.Name)
			var ts agentopsv1alpha1.MCPToolset
			if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref.Name}, &ts); err != nil {
				out.Unresolved = append(out.Unresolved, "MCPToolset/"+ref.Name)
				continue
			}
			byRef = append(byRef, ts.Spec.Tools)
		}
	}
	// dispatch composes it; this endpoint only splits the wire form back into a
	// list for the console. An empty binding stays an EMPTY list — a pipeline
	// granting no tools is a configuration, not a defect to paper over.
	if tools := dispatch.EffectiveAllowedTools(byRef); tools != "" {
		out.AllowedTools = strings.Split(tools, ",")
	}

	if p.Spec.MCPConfigs != nil {
		seen := map[string]bool{}
		for _, ref := range p.Spec.MCPConfigs.Refs {
			out.MCPConfigs = append(out.MCPConfigs, ref.Name)
			var cfg agentopsv1alpha1.MCPConfig
			if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref.Name}, &cfg); err != nil {
				out.Unresolved = append(out.Unresolved, "MCPConfig/"+ref.Name)
				continue
			}
			for key := range cfg.Spec.Servers {
				if !seen[key] {
					seen[key] = true
					out.MCPServers = append(out.MCPServers, key)
				}
			}
		}
		sort.Strings(out.MCPServers)
	}
	writeJSON(w, 200, out)
}
