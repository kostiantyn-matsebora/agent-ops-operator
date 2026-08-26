// The agent loop: chat -> tool calls -> execute -> append -> repeat, bounded by
// the unit's maxTurns, streaming a readable transcript to stdout.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const baseSystemPrompt = `You are an operations agent running inside a Kubernetes cluster on behalf of agent-ops. ` +
	`Investigate with the tools you are given, then answer the person or the alert plainly in markdown. ` +
	`Call a tool only when its result changes your answer; when you have the answer, reply without calling tools.`

// Agent runs units.
type Agent struct {
	Chat              chatter
	Registry          *Registry
	Store             *ContextStore
	Workspace         string
	NumCtx            int
	ModelCanCallTools bool
	Model             string
	Out               io.Writer
	Logf              func(string, ...any)
}

const defaultMaxTurns = 60

// resolvePrompt renders the unit's prompt.
func (a *Agent) resolvePrompt(u WorkUnit) (string, error) {
	prompt := u.PromptText
	if prompt == "" && u.PromptFile != "" {
		data, err := os.ReadFile(filepath.Join(a.Workspace, u.PromptFile))
		if err != nil {
			return "", fmt.Errorf("prompt read: %w", err)
		}
		prompt = string(data)
		for k, v := range u.PromptVars {
			prompt = strings.ReplaceAll(prompt, "{{"+k+"}}", v)
		}
	}
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("empty prompt")
	}
	return prompt, nil
}

func failed(code int32, result string) RunResult {
	return RunResult{Status: "failed", ExitCode: code, Result: result}
}

// Run executes one unit end to end. It never panics on a bad unit and never
// returns an empty result on failure.
func (a *Agent) Run(ctx context.Context, u WorkUnit) RunResult {
	prompt, err := a.resolvePrompt(u)
	if err != nil {
		return failed(-1, err.Error())
	}
	if a.Model == "" {
		return failed(1, "no model: OLLAMA_MODEL is unset and the Ollama server does not have exactly one model pulled — "+
			"set ollama.model to the one this route should use")
	}

	// The allowlist: the wiring's half from the unit, the agent definition's
	// half from the checkout, composed per mode — then the gate, ONCE.
	declared := agentDeclaredTools(a.Workspace, u.Agent, a.Logf)
	allowed := composeAllowedTools(declared, u.AllowedTools, u.ToolsMode)
	granted, unavailable, ignored := a.Registry.Gate(allowed)
	a.Logf("[runtime] tools agent=%s declared=%d wiring=%d mode=%s -> %s",
		orDash(u.Agent), len(declared), len(splitList(u.AllowedTools)), orDefault(u.ToolsMode, ModeMerge), toolNames(granted))
	for _, p := range unavailable {
		a.Logf("[runtime] allowlist entry %q: no built-in and no connected MCP server provides it — unavailable on this runtime", p)
	}
	for _, p := range ignored {
		a.Logf("[runtime] allowlist entry %q: a narrowing specifier this runtime cannot honour — grants nothing", p)
	}
	if len(allowed) > 0 && !a.ModelCanCallTools {
		return failed(1, fmt.Sprintf("this route grants tools (%s) but model %q cannot call tools — "+
			"choose a model whose capabilities include tools, or route this Pipeline elsewhere",
			strings.Join(allowed, ","), a.Model))
	}

	// Context: continue the handle or start a new transcript.
	var t *Transcript
	continuity := ContinuityNew
	if id := u.ContextID(); id != "" {
		t, err = a.Store.Load(id)
		switch {
		case err == nil:
			continuity = ContinuityContinued
		case errors.Is(err, ErrContextMissing):
			reason := fmt.Sprintf("the stored context for this conversation could not be reached — no transcript %s under %s "+
				"(is /data/context backed by a durable volume? without one it dies with the pod)", id, a.Store.Dir)
			a.Logf("[runtime] %s — failing rather than answering without it", reason)
			return RunResult{Status: "failed", ExitCode: 1, Continuity: ContinuityUnavailable, ContinuityReason: reason,
				Result: "⚠️ **This conversation cannot be continued.**\n\n" +
					"Its stored context is no longer available, so answering now would mean answering with no memory of " +
					"what came before — including anything already done. Start a new conversation to continue.\n\n" +
					"Reason: " + reason}
		default:
			// The STORE did not answer. An outage, not a loss: fail this run
			// without touching the handle so the next one can try again.
			return RunResult{Status: "failed", ExitCode: 1, RuntimeContextID: id, Continuity: ContinuityContinued,
				Result: "context store unavailable: " + err.Error()}
		}
	} else {
		t = a.Store.New(u.Convo)
	}
	a.Logf("[runtime] run %s continuity=%s context=%s thread=%s", u.RunID, continuity, t.ID, threadOf(u))

	system := Message{Role: "system", Content: baseSystemPrompt}
	if u.SystemPrompt != "" {
		system.Content += "\n\n" + u.SystemPrompt
	}
	current := Message{Role: "user", Content: prompt}
	maxTurns := int(u.MaxTurns)
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}
	tools := toolDefs(granted)

	// The turn's own messages accumulate here and are appended to the
	// transcript at the end, so a run that fails mid-way still records what
	// it did.
	turn := []Message{current}
	save := func() {
		t.Messages = append(t.Messages, turn...)
		if err := a.Store.Save(t); err != nil {
			a.Logf("[runtime] context save: %v", err)
		}
	}
	// Reserve a quarter of the window for the answer.
	budget := a.NumCtx * 3 / 4
	nudged := false
	for i := 1; i <= maxTurns; i++ {
		history := append(append([]Message{}, t.Messages...), turn[:len(turn)-1]...)
		messages, dropped := Trim(system, history, turn[len(turn)-1], budget)
		if dropped > 0 {
			a.Logf("[runtime] context window: dropped %d oldest message(s) to fit num_ctx=%d", dropped, a.NumCtx)
		}
		fmt.Fprintf(a.Out, "[model] ")
		resp, err := a.Chat.Chat(ctx, messages, tools, a.Out)
		fmt.Fprintln(a.Out)
		if err != nil {
			save()
			return RunResult{Status: "failed", ExitCode: 1, RuntimeContextID: t.ID, Continuity: continuity,
				Result: "inference failed: " + err.Error()}
		}
		turn = append(turn, resp)
		if len(resp.ToolCalls) == 0 {
			result := strings.TrimSpace(resp.Content)
			if result == "" && !nudged {
				// A small model sometimes ends a turn with nothing — usually a
				// tool call it could not form, which the server drops unseen.
				// One nudge costs a turn and recovers most of them; a second
				// empty answer is reported as what it is.
				nudged = true
				a.Logf("[runtime] the model returned an empty answer — asking once for a text answer")
				turn = append(turn, Message{Role: "user", Content: "Your last message was empty. Answer the request now, in text, using only what you already know or the tools you were given."})
				continue
			}
			save()
			if result == "" {
				return RunResult{Status: "failed", ExitCode: 1, RuntimeContextID: t.ID, Continuity: continuity,
					Result: "the model returned an empty answer twice"}
			}
			return RunResult{Status: "succeeded", ExitCode: 0, RuntimeContextID: t.ID, Continuity: continuity, Result: result}
		}
		for _, call := range resp.ToolCalls {
			name := call.Function.Name
			args := string(call.Function.Arguments)
			if len(args) > 160 {
				args = args[:160] + "…"
			}
			fmt.Fprintf(a.Out, "[tool] %s %s\n", name, args)
			text, isErr := a.execute(ctx, granted, call)
			outcome := "ok"
			if isErr {
				outcome = "error"
			}
			fmt.Fprintf(a.Out, "[tool] %s -> %s, %d bytes\n", name, outcome, len(text))
			turn = append(turn, Message{Role: "tool", ToolName: name, Content: text})
		}
	}
	save()
	return RunResult{Status: "failed", ExitCode: 1, RuntimeContextID: t.ID, Continuity: continuity,
		Result: fmt.Sprintf("turn limit reached: %d turns without a final answer", maxTurns)}
}

// execute runs one call against the GRANTED set. A name outside it — the gate
// was applied to what was advertised, so this is the model inventing one —
// gets a readable error, never an execution.
func (a *Agent) execute(ctx context.Context, granted []Tool, call ToolCall) (string, bool) {
	for _, t := range granted {
		if t.Name != call.Function.Name {
			continue
		}
		text, isErr, err := t.Call(ctx, call.Function.Arguments)
		if err != nil {
			return "error: " + err.Error(), true
		}
		return text, isErr
	}
	return fmt.Sprintf("error: tool %q is not available; the tools you may call are: %s", call.Function.Name, toolNames(granted)), true
}

func toolNames(tools []Tool) string {
	if len(tools) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}

func orDash(s string) string { return orDefault(s, "-") }
func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
func threadOf(u WorkUnit) string {
	if u.ThreadID == nil {
		return "general"
	}
	return *u.ThreadID
}
