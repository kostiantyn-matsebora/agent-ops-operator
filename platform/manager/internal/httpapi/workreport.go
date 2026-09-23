package httpapi

import (
	"context"
	"fmt"
	"strconv"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// The work result's turns and tool calls: what the RUNTIME saw the run do. The
// manager never sees a model call or a tool call, so the runtime reports them
// and the manager records one activity hop each — telemetry only, never
// written to the Conversation.
//
// A runtime that reports neither stays conformant, and its runs draw exactly
// the hops they drew before. A fact the runtime cannot determine is OMITTED,
// never reported as zero: zero tokens is a real, different value.

// Bounds on the report. A report over any of them is REFUSED with a 400
// rather than trimmed, so a runtime that overflows hears it instead of drawing
// a silently shortened picture. Conformant runtimes trim to these bounds
// before posting, which keeps the refusal from ever costing a run its answer.
const (
	// MaxReportedTurns bounds turns[]. An agent loop runs tens of turns, not
	// hundreds; past this the per-turn picture has stopped being readable.
	MaxReportedTurns = 200
	// MaxReportedToolCalls bounds toolCalls[], on the same grounds.
	MaxReportedToolCalls = 200
	// MaxReportedField bounds every string in a turn or a tool call. These
	// are identifiers — a model name, a tool name, a server key, a stop
	// reason — never free text, so the bound is the event data map's own.
	MaxReportedField = activity.MaxDataValue
)

// Turn is one model call the run made.
type Turn struct {
	// Model is the model the call went to, as the runtime names it.
	Model string `json:"model,omitempty"`
	// TokensIn is every input token the call consumed, cache reads included,
	// as far as the vendor reports them.
	TokensIn *int64 `json:"tokensIn,omitempty"`
	// TokensOut is the output tokens the call produced.
	TokensOut *int64 `json:"tokensOut,omitempty"`
	// CacheReadTokens is the part of TokensIn served from the provider's
	// prompt cache. Omitted by a vendor that reports none.
	CacheReadTokens *int64 `json:"cacheReadTokens,omitempty"`
	// StopReason is why the model stopped: end of turn, tool use, max tokens.
	StopReason string `json:"stopReason,omitempty"`
}

// ToolCall is one tool call the run made. Its input and output are NEVER
// reported — a shell command or a file's contents is content, and content has
// no place in telemetry.
type ToolCall struct {
	// Tool is the tool's name in the agent-ops vocabulary: `Bash`, `Read`,
	// or `mcp__<server>__<tool>`.
	Tool string `json:"tool"`
	// Server is the MCP server the call reached. EMPTY for a built-in tool,
	// which then records a hop with no destination.
	Server string `json:"server,omitempty"`
	// DurationMs is the wall time from the call to its result.
	DurationMs *int64 `json:"durationMs,omitempty"`
	// ResultBytes is the size of the result the tool returned.
	ResultBytes *int64 `json:"resultBytes,omitempty"`
}

// validateReport refuses a report over the bounds, naming the bound it broke.
func validateReport(d *workDone) error {
	if n := len(d.Turns); n > MaxReportedTurns {
		return fmt.Errorf("turns: %d reported, at most %d allowed", n, MaxReportedTurns)
	}
	if n := len(d.ToolCalls); n > MaxReportedToolCalls {
		return fmt.Errorf("toolCalls: %d reported, at most %d allowed", n, MaxReportedToolCalls)
	}
	for i, t := range d.Turns {
		if err := fieldsWithin(fmt.Sprintf("turns[%d]", i), map[string]string{
			"model": t.Model, "stopReason": t.StopReason}); err != nil {
			return err
		}
		if err := nonNegative(fmt.Sprintf("turns[%d]", i), map[string]*int64{
			"tokensIn": t.TokensIn, "tokensOut": t.TokensOut, "cacheReadTokens": t.CacheReadTokens}); err != nil {
			return err
		}
	}
	for i, c := range d.ToolCalls {
		if err := fieldsWithin(fmt.Sprintf("toolCalls[%d]", i), map[string]string{
			"tool": c.Tool, "server": c.Server}); err != nil {
			return err
		}
		if err := nonNegative(fmt.Sprintf("toolCalls[%d]", i), map[string]*int64{
			"durationMs": c.DurationMs, "resultBytes": c.ResultBytes}); err != nil {
			return err
		}
	}
	return nil
}

func fieldsWithin(at string, fields map[string]string) error {
	for name, v := range fields {
		if len(v) > MaxReportedField {
			return fmt.Errorf("%s.%s: %d bytes, at most %d allowed", at, name, len(v), MaxReportedField)
		}
	}
	return nil
}

func nonNegative(at string, fields map[string]*int64) error {
	for name, v := range fields {
		if v != nil && *v < 0 {
			return fmt.Errorf("%s.%s: must not be negative", at, name)
		}
	}
	return nil
}

// runtimeImageNode names the node a run's own calls leave from: the image the
// conversation's pod runs, resolved by the same rule the pod builder uses. A
// runtime that no longer resolves, or a deployment with no image anywhere,
// falls back to the AgentRuntime itself — a hop from a coarser node is honest,
// one from an image nobody ran is not.
func (s *Server) runtimeImageNode(ctx context.Context, conv *agentopsv1alpha1.Conversation) *activity.NodeRef {
	if resolved, err := runtimepod.ResolveFor(ctx, s.Reader, s.Namespace, conv, s.Runtime); err == nil && resolved.Config.Image != "" {
		return activity.Node(activity.NodeRuntimeImage, resolved.Config.Image)
	}
	return activity.Node(activity.NodeRuntime, s.runtimeName(ctx, conv))
}

// emitRunCalls records one model.call per turn and one tool.call per tool
// call. A report with neither emits nothing, and resolves nothing.
func (s *Server) emitRunCalls(ctx context.Context, conv *agentopsv1alpha1.Conversation,
	pipeline, runID string, d *workDone) {

	if len(d.Turns) == 0 && len(d.ToolCalls) == 0 {
		return
	}
	from := s.runtimeImageNode(ctx, conv)
	for _, t := range d.Turns {
		e := activity.Event{
			Kind: activity.KindModelCall, From: from,
			Pipeline: pipeline, Conversation: conv.Name, RunID: runID,
			Detail: t.Model,
			Data: withCounts(map[string]string{"model": t.Model, "stopReason": t.StopReason},
				map[string]*int64{"tokensIn": t.TokensIn, "tokensOut": t.TokensOut, "cacheReadTokens": t.CacheReadTokens}),
		}
		if t.Model != "" {
			e.To = activity.Node(activity.NodeModel, t.Model)
		}
		s.Activity.Emit(e)
	}
	for _, c := range d.ToolCalls {
		e := activity.Event{
			Kind: activity.KindToolCall, From: from,
			Pipeline: pipeline, Conversation: conv.Name, RunID: runID,
			Detail: c.Tool,
			Data: withCounts(map[string]string{"tool": c.Tool, "server": c.Server},
				map[string]*int64{"resultBytes": c.ResultBytes}),
		}
		if c.Server != "" {
			e.To = activity.Node(activity.NodeMCPServer, c.Server)
		}
		if c.DurationMs != nil {
			e.LatencyMs = *c.DurationMs
		}
		s.Activity.Emit(e)
	}
}

// withCounts merges the present counts into the present strings, dropping
// every absent fact rather than writing it as empty or zero.
func withCounts(strs map[string]string, counts map[string]*int64) map[string]string {
	out := map[string]string{}
	for k, v := range strs {
		if v != "" {
			out[k] = v
		}
	}
	for k, v := range counts {
		if v != nil {
			out[k] = strconv.FormatInt(*v, 10)
		}
	}
	return out
}
