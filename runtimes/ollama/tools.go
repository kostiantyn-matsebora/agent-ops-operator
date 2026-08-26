// Allowlist composition — a port of runtimes/claude/tools.js — plus the gate
// and the registry over both tool sources.
//
// The manager sends HALF the allowlist (the wiring's toolsets) and the mode;
// the other half is the `tools:` frontmatter of .claude/agents/<agent>.md in
// the checkout, which only this process can read.
//
// Deliberately NOT a YAML parser. It reads one field of one shape and treats
// everything it does not understand as "declares nothing" — an unreadable role
// file must never stop an agent from answering.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ModeMerge     = "merge"
	ModeOverwrite = "overwrite"
)

// splitList turns a comma-separated allowlist into trimmed entries.
func splitList(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func unquote(s string) string {
	t := strings.TrimSpace(s)
	if len(t) >= 2 && ((t[0] == '"' && t[len(t)-1] == '"') || (t[0] == '\'' && t[len(t)-1] == '\'')) {
		return strings.TrimSpace(t[1 : len(t)-1])
	}
	return t
}

var (
	reToolsKey  = regexp.MustCompile(`^tools:(.*)$`)
	reBlockItem = regexp.MustCompile(`^\s+-\s*(.*)$`)
)

// parseFrontmatterTools extracts `tools:` from an agent definition's YAML
// frontmatter. An absent key or absent frontmatter yields an empty list with a
// nil error; a file that was there but could not be understood returns an
// error so it can be LOGGED rather than pass silently.
func parseFrontmatterTools(text string) ([]string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return []string{}, nil
	}
	start, end := i+1, -1
	for j := start; j < len(lines); j++ {
		if t := strings.TrimSpace(lines[j]); t == "---" || t == "..." {
			end = j
			break
		}
	}
	if end < 0 {
		return nil, errors.New("frontmatter opened with --- but never closed")
	}
	for j := start; j < end; j++ {
		m := reToolsKey.FindStringSubmatch(lines[j])
		if m == nil {
			continue // top-level key only: an indented tools: belongs to something else
		}
		inline := strings.TrimSpace(m[1])
		if strings.HasPrefix(inline, "[") {
			if !strings.HasSuffix(inline, "]") {
				return nil, errors.New("tools: flow list is not closed on one line")
			}
			return unquoteAll(splitList(inline[1 : len(inline)-1])), nil
		}
		if inline != "" {
			return unquoteAll(splitList(inline)), nil
		}
		tools := []string{}
		for k := j + 1; k < end; k++ {
			if strings.TrimSpace(lines[k]) == "" {
				continue
			}
			item := reBlockItem.FindStringSubmatch(lines[k])
			if item == nil {
				break
			}
			if v := unquote(item[1]); v != "" {
				tools = append(tools, v)
			}
		}
		return tools, nil
	}
	return []string{}, nil
}

func unquoteAll(in []string) []string {
	out := []string{}
	for _, s := range in {
		if v := unquote(s); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// agentDeclaredTools reads .claude/agents/<agent>.md under workspace. Every
// failure yields an empty list; the run continues either way. logf receives a
// one-line reason when something was there but could not be used.
func agentDeclaredTools(workspace, agent string, logf func(string, ...any)) []string {
	if workspace == "" || agent == "" {
		return nil
	}
	text, err := os.ReadFile(filepath.Join(workspace, ".claude", "agents", agent+".md"))
	if err != nil {
		return nil
	}
	tools, perr := parseFrontmatterTools(string(text))
	if perr != nil {
		logf("[runtime] agent definition %s.md: %v — treating it as declaring no tools", agent, perr)
		return nil
	}
	return tools
}

// composeAllowedTools joins the two halves per mode. Any mode not recognised —
// including an absent one — is merge, because reading it as overwrite would
// silently strip what the agent declared.
func composeAllowedTools(agentTools []string, wiring string, mode string) []string {
	w := splitList(wiring)
	if mode == ModeOverwrite {
		return dedup(w)
	}
	return dedup(append(append([]string{}, agentTools...), w...))
}

func dedup(list []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range list {
		t := strings.TrimSpace(raw)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// patternMatches decides whether an allowlist entry grants a tool name.
//
// Exact name, a trailing `*` prefix, and therefore `mcp__server__*`. A
// claude-style narrowing specifier such as `Bash(kubectl:*)` grants NOTHING:
// reading it as bare Bash would widen a grant the operator wrote to narrow it.
// The caller logs those, once, so a binding that means nothing here is visible.
func patternMatches(pattern, name string) bool {
	if strings.Contains(pattern, "(") {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == name
}

// isNarrowing reports a specifier this runtime cannot honour.
func isNarrowing(pattern string) bool { return strings.Contains(pattern, "(") }

// Tool is one executable tool, from either source.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	// Call executes it. A tool ERROR is returned as (text, true, nil) so the
	// model can read and recover from it; a nil error with isError=false is a
	// result. A non-nil error is the tool layer itself failing.
	Call func(ctx context.Context, args json.RawMessage) (string, bool, error)
}

// Registry holds every tool this pod can provide, keyed by name.
type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

func (r *Registry) Add(t Tool) {
	if _, dup := r.tools[t.Name]; !dup {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) { t, ok := r.tools[name]; return t, ok }

// Gate applies the allowlist ONCE, before the request: the returned tools are
// the only ones advertised. It also reports what the allowlist named that
// nothing here provides — per the catalog rule, a runtime provides what it can
// and REPORTS what it cannot — and every narrowing specifier it ignored.
func (r *Registry) Gate(allowed []string) (granted []Tool, unavailable []string, ignored []string) {
	seen := map[string]bool{}
	for _, p := range allowed {
		if isNarrowing(p) {
			ignored = append(ignored, p)
			continue
		}
		matched := false
		for _, name := range r.order {
			if patternMatches(p, name) {
				matched = true
				if !seen[name] {
					seen[name] = true
					granted = append(granted, r.tools[name])
				}
			}
		}
		if !matched {
			unavailable = append(unavailable, p)
		}
	}
	return granted, unavailable, ignored
}

func toolDefs(tools []Tool) []ToolDef {
	out := make([]ToolDef, 0, len(tools))
	for _, t := range tools {
		out = append(out, newToolDef(t.Name, t.Description, t.Schema))
	}
	return out
}

// toolErr formats a readable error for the model.
func toolErr(format string, a ...any) (string, bool, error) {
	return "error: " + fmt.Sprintf(format, a...), true, nil
}
