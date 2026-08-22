// Package dispatch resolves a Conversation's pending inputs into the next
// work unit for its worker. Semantics ported from claude-runner v0.6:
// strictly serial per conversation, consecutive reply/recurrence inputs are
// batched into one resume, profile prompts win over built-in lane templates.
package dispatch

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

//go:embed templates/task.md
var taskTemplate string

//go:embed templates/investigate.md
var investigateTemplate string

//go:embed templates/reply.md
var replyTemplate string

//go:embed templates/format.md
var formatSpec string

// withFormat appends the mandatory message-format specification to a built-in
// lane template (repo-provided prompts are expected to carry their own).
func withFormat(tpl string) string {
	return tpl + "\n\n---\n\n" + formatSpec
}

// deliverySection is the delivery wording injected into every prompt. It is a
// constant on purpose: agents produce output, the operator routes it to the
// bound channels through their adapters. An agent never sends chat messages
// itself, so it never needs transport knowledge or channel credentials.
const deliverySection = "Your final printed answer IS the deliverable — it is captured by the runtime's completion report and delivered to every bound channel by the operator (it also lands in the Conversation status and pod logs). Do not attempt to send chat messages yourself."

// WorkUnit is what a worker receives from GET /work.
//
// AllowedTools and ToolsMode together are the WIRING'S HALF of the allowlist,
// not the final one: the runtime composes them with what the agent's own
// definition declares (see EffectiveAllowedTools). Agent names which definition
// that is — the runtime is the only component holding the repository, so it
// needs the name to find the file.
type WorkUnit struct {
	RunID    string  `json:"runId"`
	Convo    string  `json:"convo"`
	ThreadID *string `json:"threadId,omitempty"`
	// RuntimeContextID is the runtime's OWN handle for this conversation's
	// accumulated context, echoed back from the last run. Continue that context,
	// or report that you could not — where it is stored is the runtime's
	// business and the manager assumes nothing about it.
	//
	// Empty means "start fresh, nothing is being continued", which is also what
	// a deployment that cannot carry context sends every time, deliberately.
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	// ResumeSessionID is the former name of RuntimeContextID, sent alongside it
	// for ONE release so a runtime image can be upgraded independently of the
	// manager. Runtimes must read RuntimeContextID.
	//
	// DEPRECATED.
	ResumeSessionID string            `json:"resumeSessionId,omitempty"`
	PromptFile      string            `json:"promptFile,omitempty"` // repo-relative; worker renders PromptVars
	PromptText      string            `json:"promptText,omitempty"` // fully rendered by the manager
	PromptVars      map[string]string `json:"promptVars,omitempty"`
	Agent           string            `json:"agent,omitempty"`
	// SystemPrompt is inline role text the runtime APPENDS to the agent's
	// system prompt. Identity only — it never affects the allowlist.
	SystemPrompt string `json:"systemPrompt,omitempty"`
	AllowedTools string `json:"allowedTools,omitempty"`
	ToolsMode    string `json:"toolsMode,omitempty"`
	MaxTurns     int32  `json:"maxTurns,omitempty"`
}

// PayloadResolver returns the payload for an input (inline or via ConversationInput).
type PayloadResolver func(item agentopsv1alpha1.InputItem) (string, error)

// EffectiveAllowedTools resolves THE WIRING'S CONTRIBUTION to a conversation's
// allowlist from the toolsets its wiring bound, whose tool lists arrive in ref
// order — concatenated with dedup, first occurrence keeping its position.
//
// This is NOT the final allowlist. The other contributor is the agent's own
// definition (`tools:` in .claude/agents/<agent>.md), which only the runtime
// can read, and the work unit's ToolsMode says how the two compose: merge
// unions them, overwrite passes this result alone. The manager computes one
// half and states which composition applies; the runtime decides the rest.
//
// The profile contributes nothing: capabilities live only on the Pipeline, so
// a conversation whose wiring binds no toolsets contributes an empty string
// here. That is the intended result, not a degradation — under overwrite it
// means the route grants nothing, the same way an unclaimed signal source has
// no route.
//
// Resolution happens per work unit, so editing a toolset takes effect on the
// next dispatch without restarting the runtime pod.
func EffectiveAllowedTools(byRef [][]string) string {
	var out []string
	seen := map[string]bool{}
	for _, tools := range byRef {
		for _, t := range tools {
			t = strings.TrimSpace(t)
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

// singleThread returns the thread id handed to the runtime as context: only
// single-channel conversations expose one, since a mirrored conversation has
// no single thread. The runtime never posts to it — the operator delivers.
func singleThread(c *agentopsv1alpha1.Conversation) *string {
	if len(c.Spec.ChannelRefs) == 1 {
		return c.ThreadFor(c.Spec.ChannelRefs[0].Name)
	}
	return nil
}

func render(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// PendingInputs returns spec inputs that are neither processed nor inflight.
func PendingInputs(c *agentopsv1alpha1.Conversation) []agentopsv1alpha1.InputItem {
	done := map[string]bool{}
	for _, id := range c.Status.ProcessedInputIDs {
		done[id] = true
	}
	if c.Status.Inflight != nil {
		for _, id := range c.Status.Inflight.InputIDs {
			done[id] = true
		}
	}
	var out []agentopsv1alpha1.InputItem
	for _, in := range c.Spec.Inputs {
		if !done[in.ID] {
			out = append(out, in)
		}
	}
	return out
}

// NeedsWorker reports whether a conversation has work that requires a runtime
// pod: something queued, or something already running.
//
// THE ONE DEFINITION OF BUSY. Two callers depend on it meaning the same thing
// in both directions: the reconciler creates a pod when it is true and evicts an
// idle one when it is false, and `/exit` releases a pod only when it is false.
// Restating it on either side would let a conversation an operator was told is
// releasable be one the manager refuses to release — a disagreement that
// surfaces as a bug report about the cap, far from either definition.
func NeedsWorker(c *agentopsv1alpha1.Conversation) bool {
	return len(PendingInputs(c)) > 0 || c.Status.Inflight != nil
}

// Tooling is the wiring's half of a work unit's tool access: the resolved
// contribution of the bound toolsets and the mode composing it with the agent
// definition's own declaration. The caller holds the client that reads the
// bound toolsets; see EffectiveAllowedTools.
type Tooling struct {
	AllowedTools string
	Mode         string
}

// ToolsModeOf returns the binding's mode, defaulting to merge. The CRD defaults
// it too, but a Conversation built in memory (tests, older stored objects) can
// still arrive without one, and an unset mode must never read as overwrite —
// that would silently strip what the agent declared.
func ToolsModeOf(b *agentopsv1alpha1.ToolsetBinding) string {
	if b == nil || b.Mode == "" {
		return agentopsv1alpha1.ToolsModeMerge
	}
	return b.Mode
}

// Next resolves the next work unit. Returns the unit and the consumed input
// ids, or ok=false when there is nothing to dispatch (inflight or empty).
// contextID is the handle to continue, or "" to start fresh. The CALLER decides
// it, because whether continuity is possible depends on the execution backend —
// which runtime, and whether this deployment gives it somewhere to keep context
// — and resolving that here would put runtime knowledge in the prompt builder.
func Next(c *agentopsv1alpha1.Conversation, profile *agentopsv1alpha1.AgentProfile, tools Tooling,
	resolve PayloadResolver, now time.Time, contextID string) (WorkUnit, []string, bool, error) {

	if c.Status.Inflight != nil {
		return WorkUnit{}, nil, false, nil
	}
	pending := PendingInputs(c)
	if len(pending) == 0 {
		return WorkUnit{}, nil, false, nil
	}
	first := pending[0]

	runID := now.UTC().Format("2006-01-02T15-04-05-000Z")
	unit := WorkUnit{
		Convo:        c.Name,
		ThreadID:     singleThread(c),
		AllowedTools: tools.AllowedTools,
		ToolsMode:    tools.Mode,
		MaxTurns:     profile.Spec.MaxTurns,
		SystemPrompt: profile.Spec.SystemPrompt,
	}
	agentName := profile.Spec.Agent
	// DEPRECATED DUAL-READ, one release only. Nothing writes InputItem.Agent
	// any more — the `/<pipeline>:<agent>` form is gone, because a caller
	// selecting its own agent reaches past the wiring that originated it. This
	// branch exists so an input QUEUED BEFORE the upgrade still dispatches to
	// the agent it was parsed with, rather than silently changing agent
	// mid-conversation on a manager restart. Delete it with the field.
	if first.Agent != "" {
		agentName = first.Agent
	}
	// The runtime resolves .claude/agents/<agent>.md from this — the same name
	// the lane templates put in front of the model, so the definition it reads
	// and the definition its tools come from are always the same file.
	unit.Agent = agentName

	switch first.Type {
	case agentopsv1alpha1.InputTask, agentopsv1alpha1.InputJob:
		payload, err := resolve(first)
		if err != nil {
			return WorkUnit{}, nil, false, err
		}
		unit.RunID = runID
		vars := map[string]string{"AGENT_NAME": agentName, "USER_TASK": payload, "DELIVERY_INSTRUCTIONS": deliverySection}
		if profile.Spec.Prompt != "" {
			unit.PromptFile = profile.Spec.Prompt
			unit.PromptVars = vars
		} else {
			unit.PromptText = render(withFormat(taskTemplate), vars)
		}
		return unit, []string{first.ID}, true, nil

	case agentopsv1alpha1.InputAlert:
		payload, err := resolve(first)
		if err != nil {
			return WorkUnit{}, nil, false, err
		}
		unit.RunID = runID
		vars := map[string]string{"AGENT_NAME": agentName, "SIGNAL_JSON": payload, "ALERTS_JSON": payload, "DELIVERY_INSTRUCTIONS": deliverySection}
		if profile.Spec.Prompt != "" {
			unit.PromptFile = profile.Spec.Prompt
			unit.PromptVars = vars
		} else {
			unit.PromptText = render(withFormat(investigateTemplate), vars)
		}
		return unit, []string{first.ID}, true, nil

	case agentopsv1alpha1.InputReply, agentopsv1alpha1.InputRecurrence:
		if contextID == "" {
			// nothing to continue — degrade to a task around the text (v0.6
			// behavior). Reached both before the first run has produced a handle
			// and in a deployment that cannot carry context at all, where the
			// handle is withheld deliberately.
			payload, err := resolve(first)
			if err != nil {
				return WorkUnit{}, nil, false, err
			}
			unit.RunID = runID
			unit.PromptText = render(withFormat(taskTemplate), map[string]string{"AGENT_NAME": agentName, "USER_TASK": payload, "DELIVERY_INSTRUCTIONS": deliverySection})
			return unit, []string{first.ID}, true, nil
		}
		// batch consecutive inputs of the same type into one resume
		ids := []string{first.ID}
		texts := []string{}
		p, err := resolve(first)
		if err != nil {
			return WorkUnit{}, nil, false, err
		}
		texts = append(texts, p)
		for _, in := range pending[1:] {
			if in.Type != first.Type {
				break
			}
			p, err := resolve(in)
			if err != nil {
				break
			}
			texts = append(texts, p)
			ids = append(ids, in.ID)
		}
		joined := strings.Join(texts, "\n---\n")
		if first.Type == agentopsv1alpha1.InputRecurrence {
			joined = "[automatic notification — this signal fired AGAIN; this is not a user reply]\n```json\n" +
				joined + "\n```\nRe-assess with your previous context: has anything changed? Update the diagnosis or proposed fix if needed; keep it short if nothing changed."
		}
		unit.RunID = runID + "-resume"
		// Both names for one release: the current one, and the retired spelling
		// so a runtime image upgrades independently of the manager.
		unit.RuntimeContextID = contextID
		unit.ResumeSessionID = contextID
		vars := map[string]string{"USER_REPLY": joined, "AGENT_NAME": agentName, "DELIVERY_INSTRUCTIONS": deliverySection}
		if profile.Spec.ReplyPrompt != "" {
			unit.PromptFile = profile.Spec.ReplyPrompt
			unit.PromptVars = vars
		} else {
			unit.PromptText = render(withFormat(replyTemplate), vars)
		}
		return unit, ids, true, nil
	}
	return WorkUnit{}, nil, false, fmt.Errorf("unknown input type %q", first.Type)
}
