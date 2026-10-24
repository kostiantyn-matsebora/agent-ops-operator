// The run's turns and tool calls, reported with the work result so the
// manager can draw them. The manager never sees a model or a tool call; this
// loop makes every one, so it is the only place either can be counted.
//
// FACTS, never content: a tool's name, how long it ran and how much it
// returned — never its arguments or its output.
package main

import (
	"strings"
	"time"
	"unicode/utf8"
)

// The manager's bounds. Over any of them it refuses the whole report, so the
// runtime trims to them first.
const (
	maxReportedTurns     = 200
	maxReportedToolCalls = 200
	maxReportedField     = 200
)

// TurnReport is one model call. A count the server did not send is omitted.
type TurnReport struct {
	Model      string `json:"model,omitempty"`
	TokensIn   *int64 `json:"tokensIn,omitempty"`
	TokensOut  *int64 `json:"tokensOut,omitempty"`
	StopReason string `json:"stopReason,omitempty"`
}

// ToolCallReport is one tool call. Server is empty for a built-in tool.
type ToolCallReport struct {
	Tool        string `json:"tool"`
	Server      string `json:"server,omitempty"`
	DurationMs  *int64 `json:"durationMs,omitempty"`
	ResultBytes *int64 `json:"resultBytes,omitempty"`
}

type callLog struct {
	turns []TurnReport
	tools []ToolCallReport
}

func (l *callLog) turn(u *Usage) {
	if u == nil {
		return
	}
	l.turns = append(l.turns, TurnReport{Model: clip(u.Model), TokensIn: u.TokensIn,
		TokensOut: u.TokensOut, StopReason: clip(u.StopReason)})
}

func (l *callLog) tool(name string, took time.Duration, resultBytes int) {
	ms, size := took.Milliseconds(), int64(resultBytes)
	l.tools = append(l.tools, ToolCallReport{Tool: clip(name), Server: clip(mcpServerOf(name)),
		DurationMs: &ms, ResultBytes: &size})
}

func (l *callLog) report() ([]TurnReport, []ToolCallReport) {
	turns, tools := l.turns, l.tools
	if len(turns) > maxReportedTurns {
		turns = turns[:maxReportedTurns]
	}
	if len(tools) > maxReportedToolCalls {
		tools = tools[:maxReportedToolCalls]
	}
	return turns, tools
}

// mcpServerOf names the server of an `mcp__<server>__<tool>` name, or "".
func mcpServerOf(name string) string {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return ""
	}
	server, _, found := strings.Cut(rest, "__")
	if !found {
		return ""
	}
	return server
}

func clip(s string) string {
	if len(s) <= maxReportedField {
		return s
	}
	n := maxReportedField
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
