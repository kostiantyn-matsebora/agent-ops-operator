// The ONLY file that knows Ollama's wire format. Everything else talks to the
// chatter interface, so an OpenAI-compatible sibling is one more file.
//
// Hand-rolled over net/http on purpose: github.com/ollama/ollama/api requires
// Go 1.26 and drags the whole server's dependency graph in for what is two
// request structs and two response structs.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is one entry of the conversation as the model sees it.
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolName names the tool a role=tool message answers for. Ollama accepts
	// it; older servers ignore it.
	ToolName string `json:"tool_name,omitempty"`
}

// ToolCall is the model asking for a tool. Arguments are kept RAW: a small
// model produces malformed ones often enough that decoding them is the tool
// layer's job, where the failure becomes a readable tool error.
type ToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// ToolDef is a tool as advertised to the model.
type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func newToolDef(name, description string, schema json.RawMessage) ToolDef {
	var d ToolDef
	d.Type = "function"
	d.Function.Name = name
	d.Function.Description = description
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	d.Function.Parameters = schema
	return d
}

// chatter is the seam between the loop and a vendor.
type chatter interface {
	// Chat produces the next assistant message. Streamed text is written to
	// out as it arrives; the returned message is the whole of it.
	Chat(ctx context.Context, messages []Message, tools []ToolDef, out io.Writer) (Message, error)
}

// Ollama speaks the native API.
type Ollama struct {
	URL       string
	Model     string
	NumCtx    int
	KeepAlive string
	HTTP      *http.Client
}

type chatRequest struct {
	Model     string         `json:"model"`
	Messages  []Message      `json:"messages"`
	Tools     []ToolDef      `json:"tools,omitempty"`
	Stream    bool           `json:"stream"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options"`
}

type chatChunk struct {
	Message    Message `json:"message"`
	Done       bool    `json:"done"`
	DoneReason string  `json:"done_reason,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// Chat streams /api/chat. options.num_ctx is ALWAYS set: the server default
// silently drops the front of an over-long prompt — where the system prompt and
// the signal payload sit — and a truncated prompt produces a confident wrong
// answer with nothing in any log to explain it.
func (o *Ollama) Chat(ctx context.Context, messages []Message, tools []ToolDef, out io.Writer) (Message, error) {
	req := chatRequest{
		Model:     o.Model,
		Messages:  messages,
		Tools:     tools,
		Stream:    true,
		KeepAlive: o.KeepAlive,
		Options:   map[string]any{"num_ctx": o.NumCtx},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Message{}, err
	}
	resp, err := o.post(ctx, "/api/chat", body)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Message{}, fmt.Errorf("%s /api/chat: %s: %s", o.URL, resp.Status, readErr(resp.Body))
	}
	msg := Message{Role: "assistant"}
	var text strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ch chatChunk
		if err := json.Unmarshal(line, &ch); err != nil {
			return Message{}, fmt.Errorf("%s /api/chat: decode: %w", o.URL, err)
		}
		if ch.Error != "" {
			return Message{}, fmt.Errorf("%s /api/chat: %s", o.URL, ch.Error)
		}
		if ch.Message.Content != "" {
			text.WriteString(ch.Message.Content)
			if out != nil {
				io.WriteString(out, ch.Message.Content)
			}
		}
		msg.ToolCalls = append(msg.ToolCalls, ch.Message.ToolCalls...)
		if ch.Done {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return Message{}, fmt.Errorf("%s /api/chat: stream: %w", o.URL, err)
	}
	msg.Content = text.String()
	return msg, nil
}

// ModelInfo is what /api/show tells us that matters.
type ModelInfo struct {
	Present bool
	Tools   bool
}

// Check verifies the endpoint (GET /api/tags) and the model (POST /api/show).
// Reachable-but-missing is reported as a present=false info, not an error, so
// the startup line can say exactly which of the three is wrong.
func (o *Ollama) Check(ctx context.Context) (ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.URL+"/api/tags", nil)
	if err != nil {
		return ModelInfo{}, err
	}
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("%s unreachable: %w", o.URL, err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ModelInfo{}, fmt.Errorf("%s /api/tags: %s", o.URL, resp.Status)
	}
	body, _ := json.Marshal(map[string]string{"model": o.Model})
	resp, err = o.post(ctx, "/api/show", body)
	if err != nil {
		return ModelInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ModelInfo{Present: false}, nil
	}
	if resp.StatusCode/100 != 2 {
		return ModelInfo{}, fmt.Errorf("%s /api/show: %s: %s", o.URL, resp.Status, readErr(resp.Body))
	}
	var show struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&show); err != nil {
		return ModelInfo{}, fmt.Errorf("%s /api/show: decode: %w", o.URL, err)
	}
	info := ModelInfo{Present: true}
	for _, c := range show.Capabilities {
		if c == "tools" {
			info.Tools = true
		}
	}
	return info, nil
}

func (o *Ollama) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s%s: %w", o.URL, path, err)
	}
	return resp, nil
}

func readErr(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 2048))
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(b))
}

// newHTTPClient is one place for the timeout; the chat call is long by nature.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}
