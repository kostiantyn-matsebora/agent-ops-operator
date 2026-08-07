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

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
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
type WorkUnit struct {
	RunID           string            `json:"runId"`
	Convo           string            `json:"convo"`
	ThreadID        *string           `json:"threadId,omitempty"`
	ResumeSessionID string            `json:"resumeSessionId,omitempty"`
	PromptFile      string            `json:"promptFile,omitempty"` // repo-relative; worker renders PromptVars
	PromptText      string            `json:"promptText,omitempty"` // fully rendered by the manager
	PromptVars      map[string]string `json:"promptVars,omitempty"`
	AllowedTools    string            `json:"allowedTools,omitempty"`
	MaxTurns        int32             `json:"maxTurns,omitempty"`
}

// PayloadResolver returns the payload for an input (inline or via ConversationInput).
type PayloadResolver func(item agentopsv1alpha1.InputItem) (string, error)

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

// Next resolves the next work unit. Returns the unit and the consumed input
// ids, or ok=false when there is nothing to dispatch (inflight or empty).
func Next(c *agentopsv1alpha1.Conversation, profile *agentopsv1alpha1.AgentProfile,
	resolve PayloadResolver, now time.Time) (WorkUnit, []string, bool, error) {

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
		AllowedTools: profile.Spec.AllowedTools,
		MaxTurns:     profile.Spec.MaxTurns,
	}
	agentName := profile.Spec.Agent
	if first.Agent != "" {
		agentName = first.Agent
	}

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
		if c.Status.SessionID == "" {
			// no session yet — degrade to a task around the text (v0.6 behavior)
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
		unit.ResumeSessionID = c.Status.SessionID
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
