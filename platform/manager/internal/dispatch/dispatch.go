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

//go:embed templates/coordinate.md
var coordinateTemplate string

//go:embed templates/format.md
var formatSpec string

// appendFormatSpec adds the shared output-format specification to a work unit
// WHEN THE PROFILE ASKED FOR IT — the block grammar, the fold, the markdown
// subset and a default section set.
//
// ON THE SYSTEM PROMPT, AND IN ONE PLACE, because there are two prompt lanes
// and only one of them is a string the manager holds. A profile with its own
// `prompt` file sends the runtime a PATH plus variables, so nothing here can
// append to it — and the previous shape, which appended to the built-in lane
// templates only, made "is this agent told how to format" a property of whether
// a profile happened to set `prompt`. It is now a property of the profile
// saying so, on both lanes.
//
// It is also where the spec belongs: how an agent SPEAKS is identity, which is
// what SystemPrompt already carries.
//
// `none` APPENDS NOTHING, and that is a declaration rather than a default — the
// field is required, so a profile reaching here has stated its contract.
//
// THE DECLARATION GATES THE PROMPT AND NOTHING ELSE. Output is parsed into blocks
// either way, so a profile that declines this and teaches its own agent the
// grammar still gets folded output, and one that declines everything gets a
// single block — exactly today's rendering.
func appendFormatSpec(unit *WorkUnit, profile *agentopsv1alpha1.AgentProfile) {
	if profile == nil || profile.Spec.OutputFormat != agentopsv1alpha1.OutputFormatBlocks {
		return
	}
	if unit.SystemPrompt == "" {
		unit.SystemPrompt = formatSpec
		return
	}
	unit.SystemPrompt += "\n\n---\n\n" + formatSpec
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
	// RunID is a unique id for this unit, echoed back in the completion
	// report.
	RunID string `json:"runId"`
	// Convo is the Conversation this unit belongs to.
	Convo string `json:"convo"`
	// ThreadID is the bound channel's thread id, present only for a
	// conversation bound to exactly one channel. The runtime never posts to
	// it -- the manager delivers to every bound channel after the report.
	ThreadID *string `json:"threadId,omitempty"`
	// RuntimeContextID is the runtime's OWN handle for this conversation's
	// accumulated context, echoed back from the last run. Continue that context,
	// or report that you could not — where it is stored is the runtime's
	// business and the manager assumes nothing about it.
	//
	// Empty means "start fresh, nothing is being continued", which is also what
	// a deployment that cannot carry context sends every time, deliberately.
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	// ResumeSessionID is the former name of `runtimeContextId`, sent alongside
	// it for ONE release so a runtime image can be upgraded independently of
	// the manager. Runtimes must read `runtimeContextId`.
	//
	// DEPRECATED.
	ResumeSessionID string `json:"resumeSessionId,omitempty"`
	// PromptFile is a repo-relative path to render, when the profile
	// supplies its own prompt file. The runtime renders PromptVars into it.
	PromptFile string `json:"promptFile,omitempty"`
	// PromptText is the fully rendered prompt, when the manager rendered it
	// from a built-in template. Exactly one of PromptFile/PromptText is set.
	PromptText string `json:"promptText,omitempty"`
	// PromptVars is the set of template variables rendered into `promptFile`.
	PromptVars map[string]string `json:"promptVars,omitempty"`
	// Agent is the agent definition's name, for the runtime to resolve from
	// the checked-out repository.
	Agent string `json:"agent,omitempty"`
	// SystemPrompt is inline role text the runtime APPENDS to the agent's
	// system prompt. Identity only — it never affects the allowlist.
	SystemPrompt string `json:"systemPrompt,omitempty"`
	// AllowedTools is the wiring's half of the tool allowlist,
	// comma-separated.
	AllowedTools string `json:"allowedTools,omitempty"`
	// ToolsMode is "merge" or "overwrite", directing how `allowedTools`
	// composes with the agent definition's own declared tools.
	ToolsMode string `json:"toolsMode,omitempty"`
	// MaxTurns is the profile's turn budget for this run, when set.
	MaxTurns int32 `json:"maxTurns,omitempty"`
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

// isRoot reports whether a conversation is a Coordinator's own — the one
// framed as coordination rather than as a repository agent's task. A member
// (`causedBy` alone) is a domain agent and keeps the lane templates: it may
// have a repository, and its framing says so.
func isRoot(c *agentopsv1alpha1.Conversation) bool {
	return c.Spec.CoordinatorRef != nil
}

// framed is one work unit's prompt before the lane is chosen: the built-in
// template a member or plain conversation renders, the variables both lanes
// share, and the inputs the unit consumes — whose origins a root is told about.
type framed struct {
	// promptFile is the profile's own prompt file, "" for the built-in lane.
	promptFile string
	// template is the built-in lane template for a non-root conversation.
	template string
	vars     map[string]string
	inputs   []agentopsv1alpha1.InputItem
	// inputBlock is the input as coordinate.md shows it, verbatim: a quoted
	// block for text, a json fence for a signal payload.
	inputBlock string
}

// into fills the unit's prompt lane. The profile's prompt file wins on either
// kind of conversation — an adopter's file is their framing — and gets the same
// variables. Otherwise a Coordinator root renders coordinate.md for EVERY input
// type, in place of the three lane templates, so that a corrective input from
// the manager reads exactly like a member's result or a person's reply. The
// profile's systemPrompt still carries the role; nothing here does.
func (f framed) into(unit *WorkUnit, root bool) {
	if root {
		f.vars["FROM"] = fromLine(f.inputs)
	}
	if f.promptFile != "" {
		unit.PromptFile = f.promptFile
		unit.PromptVars = f.vars
		return
	}
	tpl := f.template
	if root {
		tpl = coordinateTemplate
		f.vars["INPUT"] = f.inputBlock
	}
	unit.PromptText = render(tpl, f.vars)
}

// fromLine is coordinate.md's FROM: one origin phrase per consumed input, in
// input order, joined for a batched resume.
func fromLine(inputs []agentopsv1alpha1.InputItem) string {
	phrases := make([]string, 0, len(inputs))
	for _, in := range inputs {
		phrases = append(phrases, originPhrase(in.Origin))
	}
	return strings.Join(phrases, "; then ")
}

// originPhrase says who one input came from, in the words the root template
// puts in front of the model. Attribution only: the model is told the door the
// input came through, never handed anything to act on by it.
func originPhrase(o *agentopsv1alpha1.InputOrigin) string {
	if o == nil {
		return "an input with no recorded origin"
	}
	switch o.Kind {
	case agentopsv1alpha1.OriginSignal:
		return fmt.Sprintf("a signal on source %s", o.Name)
	case agentopsv1alpha1.OriginChannel:
		if o.Sender != "" {
			return fmt.Sprintf("a person on channel %s (%s)", o.Name, o.Sender)
		}
		return fmt.Sprintf("a person on channel %s", o.Name)
	case agentopsv1alpha1.OriginMember:
		return fmt.Sprintf("the member invoked as %s (conversation %s)", o.Entry, o.Name)
	case agentopsv1alpha1.OriginManager:
		return fmt.Sprintf("the manager, under its rule %s", o.Name)
	}
	return fmt.Sprintf("an input of unrecognized origin kind %q", o.Kind)
}

// quoteBlock renders text verbatim as a markdown blockquote, every line
// prefixed, so a multi-line input stays one block and a fence inside it
// (a recurrence's payload) nests rather than closing the quote.
func quoteBlock(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}

// jsonFence renders a signal payload as the json block the lane templates use.
func jsonFence(payload string) string {
	return "```json\n" + payload + "\n```"
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
	// ONE call, before any lane branches: both prompt lanes get the same
	// treatment, and a new lane cannot forget it.
	appendFormatSpec(&unit, profile)

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
		framed{
			promptFile: profile.Spec.Prompt,
			template:   taskTemplate,
			vars:       map[string]string{"AGENT_NAME": agentName, "USER_TASK": payload, "DELIVERY_INSTRUCTIONS": deliverySection},
			inputs:     []agentopsv1alpha1.InputItem{first},
			inputBlock: quoteBlock(payload),
		}.into(&unit, isRoot(c))
		return unit, []string{first.ID}, true, nil

	case agentopsv1alpha1.InputAlert:
		payload, err := resolve(first)
		if err != nil {
			return WorkUnit{}, nil, false, err
		}
		unit.RunID = runID
		framed{
			promptFile: profile.Spec.Prompt,
			template:   investigateTemplate,
			vars:       map[string]string{"AGENT_NAME": agentName, "SIGNAL_JSON": payload, "ALERTS_JSON": payload, "DELIVERY_INSTRUCTIONS": deliverySection},
			inputs:     []agentopsv1alpha1.InputItem{first},
			inputBlock: jsonFence(payload),
		}.into(&unit, isRoot(c))
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
			framed{
				template:   taskTemplate,
				vars:       map[string]string{"AGENT_NAME": agentName, "USER_TASK": payload, "DELIVERY_INSTRUCTIONS": deliverySection},
				inputs:     []agentopsv1alpha1.InputItem{first},
				inputBlock: quoteBlock(payload),
			}.into(&unit, isRoot(c))
			return unit, []string{first.ID}, true, nil
		}
		batch, texts, err := batchSameType(pending, resolve)
		if err != nil {
			return WorkUnit{}, nil, false, err
		}
		ids := make([]string, 0, len(batch))
		for _, in := range batch {
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
		framed{
			promptFile: profile.Spec.ReplyPrompt,
			template:   replyTemplate,
			vars:       map[string]string{"USER_REPLY": joined, "AGENT_NAME": agentName, "DELIVERY_INSTRUCTIONS": deliverySection},
			inputs:     batch,
			inputBlock: quoteBlock(joined),
		}.into(&unit, isRoot(c))
		return unit, ids, true, nil
	}
	return WorkUnit{}, nil, false, fmt.Errorf("unknown input type %q", first.Type)
}

// batchSameType collects the consecutive pending inputs sharing the first one's
// type into one resume, with their resolved payloads in the same order. The
// first input's payload failing is the unit's error; a later one failing ends
// the batch there, leaving that input pending for the next dispatch.
func batchSameType(pending []agentopsv1alpha1.InputItem, resolve PayloadResolver) ([]agentopsv1alpha1.InputItem, []string, error) {
	first := pending[0]
	p, err := resolve(first)
	if err != nil {
		return nil, nil, err
	}
	batch, texts := []agentopsv1alpha1.InputItem{first}, []string{p}
	for _, in := range pending[1:] {
		if in.Type != first.Type {
			break
		}
		p, err := resolve(in)
		if err != nil {
			break
		}
		batch = append(batch, in)
		texts = append(texts, p)
	}
	return batch, texts, nil
}
