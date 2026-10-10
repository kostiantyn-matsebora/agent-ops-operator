//go:build system

package system

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// Coordinator-mode wiring: hand-authored Coordinator and AgentCapability
// objects, created directly through the Kube client exactly as an operator's
// own CRs would be (wiring-mode: "An operator MAY hand-write both a Pipeline
// and a Coordinator ... The API server SHALL accept both").
//
// What the CHART renders under `global.agentops.wiringMode: coordinator` —
// the Coordinator, each bundle's AgentCapability, the reaper's own objects —
// is a RENDERING fact, already pinned by the envtest permutation matrix
// (charttemplate_test.go): docs/testing.md gives rendering to the
// unit/envtest tier. What only a live cluster decides is the SUBSTRATE this
// file exercises instead: a real kubelet starting a member conversation's
// pod, a real cron-adapter pod firing a real admitted signal, and the live
// deployed manager's own HTTP `/coordinate/*` surface reached over the
// network with a derived bearer token — the exact path `platform/mcp-aops`
// forwards through, never envtest's in-process handler call.

// agentCapability is a standalone AgentCapability naming a profile and
// nothing else — the ordinary shape `coordinator-model` describes for a
// capability meant to be referenced from a Coordinator's `agents[]`.
func agentCapability(name, profile string) *agentopsv1alpha1.AgentCapability {
	c := &agentopsv1alpha1.AgentCapability{ObjectMeta: metav1.ObjectMeta{Name: name}}
	c.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile}
	return c
}

// capabilityEntry is one agents[] entry invoking an ordinary AgentCapability
// member — never a nested Coordinator.
func capabilityEntry(name, capability, description string) agentopsv1alpha1.CoordinatorAgentEntry {
	return agentopsv1alpha1.CoordinatorAgentEntry{
		Name:          name,
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: capability},
		Description:   description,
	}
}

// coordinatorObj is a Coordinator naming its own inline coordinating
// capability (profile only, mirroring the pack's stub-driven pipelines), the
// sources it claims, and the agents[] entries its own conversation may
// invoke.
func coordinatorObj(name, profile string, sources []string, agents []agentopsv1alpha1.CoordinatorAgentEntry) *agentopsv1alpha1.Coordinator {
	co := &agentopsv1alpha1.Coordinator{ObjectMeta: metav1.ObjectMeta{Name: name}}
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile}
	for _, s := range sources {
		co.Spec.SignalSourceRefs = append(co.Spec.SignalSourceRefs, agentopsv1alpha1.ObjectRef{Name: s})
	}
	co.Spec.Agents = agents
	return co
}

// coordinatorObjWithChannels is coordinatorObj plus a declared `channelRefs`
// set — the bindings a conversation this Coordinator opens now binds at
// CREATION, unconditionally (coordinator-unconditional-channels), exactly as
// a Pipeline's own channelRefs always have.
func coordinatorObjWithChannels(name, profile string, sources, channels []string, agents []agentopsv1alpha1.CoordinatorAgentEntry) *agentopsv1alpha1.Coordinator {
	co := coordinatorObj(name, profile, sources, agents)
	for _, c := range channels {
		co.Spec.ChannelRefs = append(co.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	return co
}

// waitCoordinatorReady mirrors waitPipelineReady for the Coordinator kind —
// a reconciler that cannot watch Coordinators never writes the condition, so
// this is itself an informer-liveness fact, exactly as the Pipeline wait is.
func waitCoordinatorReady(ctx context.Context, k *Kube, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		var co agentopsv1alpha1.Coordinator
		if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: name}, &co); err == nil {
			for _, c := range co.Status.Conditions {
				if c.Type == "Ready" {
					if c.Status == metav1.ConditionTrue {
						return nil
					}
					last = c.Reason + ": " + c.Message
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("coordinator %s not Ready after %s (%s)", name, timeout, last)
}

// coordinatorToken derives the per-conversation `coordinator:<name>:<conv>`
// bearer token the manager re-derives on `/coordinate/*`
// (aops-mcp-server, chat.DeriveCoordinatorToken) — the SAME derivation a
// coordinated conversation's own pod carries as AOPS_MCP_TOKEN. Computed here
// from the master key the pack's own install already holds
// (e.Values.AdapterToken) so a test can speak the mcp-aops forwarding
// contract directly against the live manager, without deploying that server.
func coordinatorToken(e *Env, coordinatorName, conversationName string) string {
	return chat.DeriveCoordinatorToken(e.Values.AdapterToken, coordinatorName, conversationName)
}

// coordinateCall POSTs to the manager's `/coordinate/*` surface with a bearer
// token, exactly as platform/mcp-aops forwards one tool call — against the
// LIVE deployed manager reached through its Service, never an in-process
// handler the way the envtest suite calls it.
func (e *Env) coordinateCall(t *testing.T, path, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	code, out := e.do(t, "POST", e.Manager.URL()+path, b, bearerPrefix+token)
	var parsed map[string]any
	_ = json.Unmarshal([]byte(out), &parsed)
	return code, parsed
}

// invokeMember calls /coordinate/invoke as the named caller conversation,
// returning the created/attached member's name. Fails the test on anything
// but 200 — every lane in this file that calls it is asserting the ordinary,
// already-working invoke path (the caller is always an uncaused root here).
func (e *Env) invokeMember(t *testing.T, callerCoordinator, caller, agent, task string) string {
	t.Helper()
	token := coordinatorToken(e, callerCoordinator, caller)
	code, out := e.coordinateCall(t, "/coordinate/invoke", token,
		map[string]any{"conversation": caller, "agent": agent, "task": task})
	if code != 200 {
		t.Fatalf("invoke(%s) from %s: %d %v", agent, caller, code, out)
	}
	member, _ := out["member"].(string)
	if member == "" {
		t.Fatalf("invoke(%s) from %s: no member name in response %v", agent, caller, out)
	}
	return member
}

// waitForCoordinatorRoot polls for the UNCAUSED conversation a Coordinator's
// own claim or addressed command opened after `after` — the shape an
// addressed chat command produces, since it is created under
// `GenerateName: "task-"` and its eventual name is not known in advance the
// way a signal-opened conversation's fingerprint-matched one is (see
// ConversationFor).
func waitForCoordinatorRoot(ctx context.Context, t *testing.T, e *Env, coordName string, after time.Time, timeout time.Duration) *agentopsv1alpha1.Conversation {
	t.Helper()
	var found *agentopsv1alpha1.Conversation
	waitFor(t, "a root conversation from coordinator "+coordName, timeout, func() (bool, error) {
		items, err := e.K.Conversations(ctx)
		if err != nil {
			return false, err
		}
		for i := range items {
			c := &items[i]
			if c.Spec.CoordinatorRef != nil && c.Spec.CoordinatorRef.Name == coordName &&
				c.Spec.CausedBy == nil && c.CreationTimestamp.Time.After(after) {
				found = c
				return true, nil
			}
		}
		return false, nil
	})
	return found
}

// waitForConversation polls until a conversation with this name exists.
func (e *Env) waitForConversation(t *testing.T, name string, timeout time.Duration) *agentopsv1alpha1.Conversation {
	t.Helper()
	ctx := context.Background()
	var found *agentopsv1alpha1.Conversation
	waitFor(t, "conversation "+name+" to exist", timeout, func() (bool, error) {
		c, err := e.K.Conversation(ctx, name)
		if err != nil {
			return false, nil // not found yet is not an error worth surfacing
		}
		found = c
		return true, nil
	})
	return found
}
