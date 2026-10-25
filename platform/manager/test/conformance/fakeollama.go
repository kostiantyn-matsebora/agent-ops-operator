//go:build conformance

package conformance

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// FakeOllama stands in for OLLAMA_URL (runtimes/ollama/ollama.go's wire
// format) — an in-package httptest.Server, built and started like
// FakeManager itself (design.md: "neither applies here — nothing else dials
// a fake Ollama"). It is a PROTOCOL double, never a model: every response is
// fixed text, scripted by the FIRST WORD of the prompt it receives, the same
// convention test/stubruntime uses — scoped to exactly what the runtime
// conformance assertions need (design.md), not cloned in full:
//
//	(default)    a short, fixed completion, with usage counts and a <brief> tag
//	slow         holds the response for SlowDelay before answering
//	unparsable   answers successfully but with NO usage counts at all — a fact
//	             ollama could not determine, proving it is omitted rather than
//	             reported as a zero (API-7), and carries no <brief> tag either
//	calls        answers a tool call on the first turn, a final answer once a
//	             tool result comes back — drives one model+tool-call round trip
//
// A new directive is added only when an asserted requirement ID needs it —
// the same discipline test/stubruntime's own doc comment models.
type FakeOllama struct {
	Model string
	// SlowDelay is how long the "slow" directive holds its response. Exported
	// so a test can shrink or grow it; read without a lock, so set it before
	// the server sees its first request.
	SlowDelay time.Duration

	srv *httptest.Server

	mu    sync.Mutex
	chats []fakeOllamaChatRequest
}

// fakeOllamaMessage mirrors runtimes/ollama's Message wire shape.
type fakeOllamaMessage struct {
	Role      string               `json:"role"`
	Content   string               `json:"content"`
	ToolCalls []fakeOllamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string               `json:"tool_name,omitempty"`
}

type fakeOllamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type fakeOllamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []fakeOllamaMessage `json:"messages"`
	Tools    []json.RawMessage   `json:"tools,omitempty"`
	Stream   bool                `json:"stream"`
	Options  map[string]any      `json:"options"`
}

type fakeOllamaChatChunk struct {
	Model           string            `json:"model,omitempty"`
	Message         fakeOllamaMessage `json:"message"`
	Done            bool              `json:"done"`
	DoneReason      string            `json:"done_reason,omitempty"`
	PromptEvalCount *int64            `json:"prompt_eval_count,omitempty"`
	EvalCount       *int64            `json:"eval_count,omitempty"`
}

// NewFakeOllama starts the fake, one model ("fake-model") always listed and
// always present with tool-calling capability — so a conformance run never
// needs OLLAMA_MODEL set and is never blocked by agent.ModelCanCallTools.
func NewFakeOllama(t *testing.T) *FakeOllama {
	t.Helper()
	o := &FakeOllama{Model: "fake-model", SlowDelay: 2 * time.Second}
	t.Cleanup(o.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tags", o.handleTags)
	mux.HandleFunc("POST /api/show", o.handleShow)
	mux.HandleFunc("POST /api/chat", o.handleChat)
	o.srv = httptest.NewServer(mux)
	return o
}

// URL is the double's base address, for OLLAMA_URL.
func (o *FakeOllama) URL() string { return o.srv.URL }

// Close stops the fake.
func (o *FakeOllama) Close() { o.srv.Close() }

// Chats returns every POST /api/chat this fake has seen, decoded. Used to
// assert what the RUNTIME sent never carries a secret value (SEC assertions)
// and to inspect what tools were offered.
func (o *FakeOllama) Chats() []fakeOllamaChatRequest {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]fakeOllamaChatRequest{}, o.chats...)
}

func (o *FakeOllama) handleTags(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"models": []map[string]any{{"name": o.Model}}})
}

func (o *FakeOllama) handleShow(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"capabilities": []string{"completion", "tools"}})
}

func (o *FakeOllama) handleChat(w http.ResponseWriter, r *http.Request) {
	var req fakeOllamaChatRequest
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &req)
	o.mu.Lock()
	o.chats = append(o.chats, req)
	o.mu.Unlock()

	directive, rest := ollamaDirective(req.Messages)
	hasToolResult := false
	for _, m := range req.Messages {
		if m.Role == "tool" {
			hasToolResult = true
		}
	}

	switch directive {
	case "slow":
		select {
		case <-time.After(o.SlowDelay):
		case <-r.Context().Done():
			return
		}
		o.writeDone(w, "the slow directive finished", i64p(10), i64p(2), "stop")
	case "unparsable":
		// A response ollama could not determine any accounting for: valid
		// JSON, but no prompt_eval_count/eval_count at all, and no <brief>
		// tag — proving both are OMITTED rather than reported as a zero or
		// an empty string (API-7).
		o.writeDone(w, "an answer the vendor reported no accounting for", nil, nil, "stop")
	case "calls":
		if !hasToolResult {
			o.writeToolCall(w, "Bash", `{"command":"echo AGENTOPS_CONFORMANCE_TOOL_MARKER"}`)
		} else {
			o.writeDone(w, "calls scenario complete", i64p(5), i64p(5), "stop")
		}
	default:
		o.writeDone(w, "a short, fixed completion for "+rest+"\n\n<brief>\nFixed description for the conformance suite.\n</brief>\n",
			i64p(12), i64p(3), "stop")
	}
}

func (o *FakeOllama) writeDone(w http.ResponseWriter, content string, in, out *int64, reason string) {
	chunk := fakeOllamaChatChunk{Model: o.Model, Message: fakeOllamaMessage{Role: "assistant", Content: content},
		Done: true, DoneReason: reason, PromptEvalCount: in, EvalCount: out}
	b, _ := json.Marshal(chunk)
	w.Header().Set("Content-Type", "application/x-ndjson")
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

func (o *FakeOllama) writeToolCall(w http.ResponseWriter, name, argsJSON string) {
	var tc fakeOllamaToolCall
	tc.Function.Name = name
	tc.Function.Arguments = json.RawMessage(argsJSON)
	chunk := fakeOllamaChatChunk{Model: o.Model, Message: fakeOllamaMessage{Role: "assistant", ToolCalls: []fakeOllamaToolCall{tc}},
		Done: true, DoneReason: "tool_calls"}
	b, _ := json.Marshal(chunk)
	w.Header().Set("Content-Type", "application/x-ndjson")
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

// ollamaDirectives is the vocabulary this double recognizes. Anything else
// reads as the default completion — see the type's doc comment.
var ollamaDirectives = []string{"slow", "unparsable", "calls"}

// ollamaLastUserContent returns the LAST user-role message's content — the
// original prompt, which stays the same across every chat call of one run
// even once tool-result messages are appended ahead of it.
func ollamaLastUserContent(msgs []fakeOllamaMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

// ollamaDirective picks the directive from the first word of the prompt, the
// same convention test/stubruntime uses.
func ollamaDirective(msgs []fakeOllamaMessage) (directive, rest string) {
	text := strings.TrimSpace(ollamaLastUserContent(msgs))
	word, tail, _ := strings.Cut(text, " ")
	for _, d := range ollamaDirectives {
		if word == d {
			return d, strings.TrimSpace(tail)
		}
	}
	return "default", text
}

func i64p(n int64) *int64 { return &n }
