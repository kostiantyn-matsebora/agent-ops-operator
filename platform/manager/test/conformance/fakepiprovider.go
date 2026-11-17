//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// FakePiProvider stands in for PI_BASE_URL — an OpenAI-compatible
// chat-completions endpoint, the shape runtimes/pi/test/fake-provider.js
// already proved live against the real SDK (design.md's S7: "a fake that
// returns one plain JSON body is rejected... the real fake answers
// Server-Sent-Events chunks"). This is the SAME wire shape, ported to Go so
// the Go-side conformance suite can drive it without a second process: SSE
// `data: {...}\n\n` chunks matching the OpenAI `chat.completion.chunk`
// streaming shape, terminated with `data: [DONE]\n\n`.
//
// Like FakeOllama, it is a PROTOCOL double, never a model: every response is
// scripted by the FIRST WORD of the latest user-role message, the same
// convention FakeOllama and test/stubruntime use — scoped to exactly what
// the pi runtime conformance assertions need, not a general OpenAI mock.
//
//	(default)    a short, fixed completion, with usage counts and a <brief> tag
//	slow         holds the response for SlowDelay before answering
//	unparsable   answers successfully but with NO usage object at all, and
//	             carries no <brief> tag either — see
//	             runtimePiAssertAPI7OmitsWhatPiCannotDetermine for what this
//	             actually proves live (narrower than "every count is
//	             omitted": measured against the real SDK, only
//	             reasoningTokens stays genuinely absent)
//	calls        answers a "bash" tool call on the first turn (pi's own
//	             built-in, vocabulary.js's BUILTIN.Bash mapping — the ONLY
//	             built-in this lane ever grants), a final answer once the
//	             tool's real result comes back on the next turn
//
// The path is never checked, exactly as runtimes/pi/test/fake-provider.js's
// own bare http.createServer never checks it — the real pi SDK's own OpenAI
// client posts to "<baseUrl>/chat/completions" (confirmed against the
// vendored dist's own openai-completions provider file), but this double's
// whole point is standing in for "some OpenAI-compatible endpoint", not
// pinning an SDK-internal path nothing here owns.
type FakePiProvider struct {
	// SlowDelay is how long the "slow" directive holds its response before
	// answering, read without a lock, so set it before the first request.
	SlowDelay time.Duration

	srv *httptest.Server

	mu   sync.Mutex
	reqs []map[string]any
}

// NewFakePiProvider starts the fake.
func NewFakePiProvider(t *testing.T) *FakePiProvider {
	t.Helper()
	p := &FakePiProvider{SlowDelay: 2 * time.Second}
	t.Cleanup(p.Close)
	p.srv = httptest.NewServer(http.HandlerFunc(p.handle))
	return p
}

// URL is the double's base address, for PI_BASE_URL.
func (p *FakePiProvider) URL() string { return p.srv.URL }

// Close stops the fake.
func (p *FakePiProvider) Close() { p.srv.Close() }

// Requests returns every completions call this fake has seen, decoded.
func (p *FakePiProvider) Requests() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]any{}, p.reqs...)
}

func (p *FakePiProvider) handle(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(raw, &req)
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()

	directive, rest := piDirective(req)
	hasToolResult := piHasToolResult(req)

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	send := func(delta map[string]any, finishReason any, extra map[string]any) {
		chunk := map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion.chunk", "created": 0, "model": "fake-model",
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finishReason}},
		}
		for k, v := range extra {
			chunk[k] = v
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	send(map[string]any{"role": "assistant", "content": ""}, nil, nil)
	switch directive {
	case "slow":
		select {
		case <-time.After(p.SlowDelay):
		case <-r.Context().Done():
			return
		}
		send(map[string]any{"content": "the slow directive finished"}, "stop",
			map[string]any{"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12}})
	case "unparsable":
		// A response with no usage object at all, and no <brief> tag — see
		// runtimePiAssertAPI7OmitsWhatPiCannotDetermine for what this
		// double-side absence actually proves once pi's own SDK has
		// normalized it.
		send(map[string]any{"content": "an answer the vendor reported no accounting for"}, "stop", nil)
	case "calls":
		if !hasToolResult {
			send(map[string]any{"tool_calls": []map[string]any{{"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": "bash", "arguments": ""}}}}, nil, nil)
			send(map[string]any{"tool_calls": []map[string]any{{"index": 0,
				"function": map[string]any{"arguments": `{"command":"echo AGENTOPS_CONFORMANCE_TOOL_MARKER"}`}}}}, nil, nil)
			send(map[string]any{}, "tool_calls", nil)
		} else {
			send(map[string]any{"content": "calls scenario complete"}, "stop",
				map[string]any{"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
		}
	default:
		send(map[string]any{"content": "a short, fixed completion for " + rest +
			"\n\n<brief>\nFixed description for the conformance suite.\n</brief>\n"}, "stop",
			map[string]any{"usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 3, "total_tokens": 15}})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

// piDirectives is the vocabulary this double recognizes. Anything else reads
// as the default completion — see the type's doc comment.
var piDirectives = []string{"slow", "unparsable", "calls"}

// piLastUserContent returns the last user-role message's text — the
// original prompt, stable across every call of one run even once
// tool-result messages are appended ahead of it. pi's own OpenAI-completions
// client sends a plain string for an ordinary text turn, confirmed live
// against the real SDK by this very test — but a content-PARTS array
// (`[{type:"text", text:"..."}]`, the shape every OpenAI-compatible wire
// format also accepts) is read too, rather than assumed never to appear.
func piLastUserContent(messages []any) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m, _ := messages[i].(map[string]any)
		if m == nil {
			continue
		}
		if role, _ := m["role"].(string); role == "user" {
			return piMessageText(m["content"])
		}
	}
	return ""
}

// piMessageText reads a message's `content` as either a plain string or a
// parts array, concatenating every text part — see piLastUserContent.
func piMessageText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, raw := range parts {
		part, _ := raw.(map[string]any)
		if part == nil {
			continue
		}
		if t, _ := part["type"].(string); t == "text" {
			if text, _ := part["text"].(string); text != "" {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

func piDirective(req map[string]any) (directive, rest string) {
	messages, _ := req["messages"].([]any)
	text := strings.TrimSpace(piLastUserContent(messages))
	word, tail, _ := strings.Cut(text, " ")
	for _, d := range piDirectives {
		if word == d {
			return d, strings.TrimSpace(tail)
		}
	}
	return "default", text
}

// piHasToolResult reports whether the request already carries a tool-role
// message — the signal this double uses (like FakeOllama's own) to tell a
// round trip's SECOND call from its first.
func piHasToolResult(req map[string]any) bool {
	messages, _ := req["messages"].([]any)
	for _, raw := range messages {
		m, _ := raw.(map[string]any)
		if role, _ := m["role"].(string); role == "tool" {
			return true
		}
	}
	return false
}
