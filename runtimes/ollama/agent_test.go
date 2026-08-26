package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scripted answers one Chat call per entry, in order.
type scripted struct {
	replies []Message
	calls   [][]ToolDef
	sent    [][]Message
	err     error
}

func (s *scripted) Chat(_ context.Context, messages []Message, tools []ToolDef, out io.Writer) (Message, error) {
	s.sent = append(s.sent, messages)
	s.calls = append(s.calls, tools)
	if s.err != nil {
		return Message{}, s.err
	}
	if len(s.replies) == 0 {
		return Message{Role: "assistant", Content: "(out of script)"}, nil
	}
	m := s.replies[0]
	s.replies = s.replies[1:]
	io.WriteString(out, m.Content)
	return m, nil
}

func text(s string) Message { return Message{Role: "assistant", Content: s} }
func toolCall(name, args string) Message {
	var tc ToolCall
	tc.Function.Name = name
	tc.Function.Arguments = json.RawMessage(args)
	return Message{Role: "assistant", ToolCalls: []ToolCall{tc}}
}

func newAgent(t *testing.T, chat chatter) (*Agent, string) {
	t.Helper()
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "f.txt"), []byte("contents"), 0o644)
	r := NewRegistry()
	Builtins{Workspace: ws, OutputMax: 1024, BashTimeout: time.Second}.Register(r)
	return &Agent{Chat: chat, Registry: r, Store: newStore(t), Workspace: ws, NumCtx: 8192,
		ModelCanCallTools: true, Model: "m", Out: io.Discard, Logf: t.Logf}, ws
}

func TestTextOnly(t *testing.T) {
	s := &scripted{replies: []Message{text("the answer")}}
	a, _ := newAgent(t, s)
	res := a.Run(context.Background(), WorkUnit{RunID: "r1", Convo: "c", PromptText: "q"})
	if res.Status != "succeeded" || res.Result != "the answer" || res.Continuity != ContinuityNew || !validID(res.RuntimeContextID) {
		t.Errorf("%+v", res)
	}
	if len(s.calls[0]) != 0 {
		t.Error("an empty allowlist advertises no tools")
	}
	if s.sent[0][0].Role != "system" || s.sent[0][1].Content != "q" {
		t.Errorf("messages: %+v", s.sent[0])
	}
	// continue it
	s.replies = []Message{text("again")}
	res2 := a.Run(context.Background(), WorkUnit{RunID: "r2", Convo: "c", PromptText: "q2", RuntimeContextID: res.RuntimeContextID})
	if res2.Continuity != ContinuityContinued || res2.RuntimeContextID != res.RuntimeContextID {
		t.Errorf("%+v", res2)
	}
	if got := s.sent[1]; len(got) != 4 || got[1].Content != "q" || got[2].Content != "the answer" {
		t.Errorf("the earlier exchange must be sent: %+v", got)
	}
}

func TestOneToolCall(t *testing.T) {
	s := &scripted{replies: []Message{toolCall("Read", `{"path":"f.txt"}`), text("it says contents")}}
	a, _ := newAgent(t, s)
	res := a.Run(context.Background(), WorkUnit{PromptText: "q", AllowedTools: "Read"})
	if res.Status != "succeeded" {
		t.Fatalf("%+v", res)
	}
	if n := len(s.calls[0]); n != 1 || s.calls[0][0].Function.Name != "Read" {
		t.Errorf("advertised: %d", n)
	}
	last := s.sent[1][len(s.sent[1])-1]
	if last.Role != "tool" || last.Content != "contents" || last.ToolName != "Read" {
		t.Errorf("tool result message: %+v", last)
	}
}

func TestHallucinatedToolAndMalformedArgs(t *testing.T) {
	s := &scripted{replies: []Message{toolCall("Bash", `{"command":"id"}`), toolCall("Read", `{"path":`), text("ok")}}
	a, _ := newAgent(t, s)
	res := a.Run(context.Background(), WorkUnit{PromptText: "q", AllowedTools: "Read"})
	if res.Status != "succeeded" {
		t.Fatalf("%+v", res)
	}
	r1 := s.sent[1][len(s.sent[1])-1]
	if !strings.Contains(r1.Content, `tool "Bash" is not available`) || !strings.Contains(r1.Content, "Read") {
		t.Errorf("unadvertised tool must be refused readably: %q", r1.Content)
	}
	r2 := s.sent[2][len(s.sent[2])-1]
	if !strings.HasPrefix(r2.Content, "error: Read: bad arguments") {
		t.Errorf("malformed args: %q", r2.Content)
	}
}

func TestTurnLimit(t *testing.T) {
	var replies []Message
	for i := 0; i < 10; i++ {
		replies = append(replies, toolCall("Read", `{"path":"f.txt"}`))
	}
	a, _ := newAgent(t, &scripted{replies: replies})
	res := a.Run(context.Background(), WorkUnit{PromptText: "q", AllowedTools: "Read", MaxTurns: 3})
	if res.Status != "failed" || !strings.Contains(res.Result, "turn limit reached: 3") || res.RuntimeContextID == "" {
		t.Errorf("%+v", res)
	}
}

func TestModelWithoutToolsOnAToolRoute(t *testing.T) {
	a, _ := newAgent(t, &scripted{replies: []Message{text("x")}})
	a.ModelCanCallTools = false
	res := a.Run(context.Background(), WorkUnit{PromptText: "q", AllowedTools: "Read"})
	if res.Status != "failed" || !strings.Contains(res.Result, `model "m" cannot call tools`) {
		t.Errorf("%+v", res)
	}
	// a text-only route on a text-only model is legitimate
	if res := a.Run(context.Background(), WorkUnit{PromptText: "q"}); res.Status != "succeeded" {
		t.Errorf("%+v", res)
	}
}

func TestMissingContextFails(t *testing.T) {
	a, _ := newAgent(t, &scripted{replies: []Message{text("x")}})
	os.MkdirAll(a.Store.Dir, 0o755)
	res := a.Run(context.Background(), WorkUnit{PromptText: "q", RuntimeContextID: "oc-deadbeefdead"})
	if res.Status != "failed" || res.Continuity != ContinuityUnavailable || res.ContinuityReason == "" || res.Result == "" ||
		!strings.Contains(res.Result, "cannot be continued") {
		t.Errorf("%+v", res)
	}
}

func TestNoModelFailsTheRunReadably(t *testing.T) {
	a, _ := newAgent(t, &scripted{replies: []Message{text("x")}})
	a.Model = ""
	if res := a.Run(context.Background(), WorkUnit{PromptText: "q"}); res.Status != "failed" || !strings.Contains(res.Result, "ollama.model") {
		t.Errorf("%+v", res)
	}
}

func TestEmptyPromptAndInferenceFailure(t *testing.T) {
	a, _ := newAgent(t, &scripted{})
	if res := a.Run(context.Background(), WorkUnit{}); res.Status != "failed" || res.Result != "empty prompt" {
		t.Errorf("%+v", res)
	}
	a.Chat = &scripted{err: io.ErrUnexpectedEOF}
	if res := a.Run(context.Background(), WorkUnit{PromptText: "q"}); res.Status != "failed" || !strings.Contains(res.Result, "inference failed") || res.RuntimeContextID == "" {
		t.Errorf("the handle is surrendered even on failure: %+v", res)
	}
}

func TestPromptFileWithVars(t *testing.T) {
	s := &scripted{replies: []Message{text("ok")}}
	a, ws := newAgent(t, s)
	os.MkdirAll(filepath.Join(ws, "prompts"), 0o755)
	os.WriteFile(filepath.Join(ws, "prompts", "p.md"), []byte("hello {{name}}"), 0o644)
	a.Run(context.Background(), WorkUnit{PromptFile: "prompts/p.md", PromptVars: map[string]string{"name": "there"}})
	if got := s.sent[0][1].Content; got != "hello there" {
		t.Errorf("%q", got)
	}
}

func TestEmptyAnswerIsNudgedOnce(t *testing.T) {
	s := &scripted{replies: []Message{text(""), text("recovered")}}
	a, _ := newAgent(t, s)
	res := a.Run(context.Background(), WorkUnit{PromptText: "q"})
	if res.Status != "succeeded" || res.Result != "recovered" || len(s.sent) != 2 {
		t.Errorf("%+v sent=%d", res, len(s.sent))
	}
	if last := s.sent[1][len(s.sent[1])-1]; last.Role != "user" || !strings.Contains(last.Content, "empty") {
		t.Errorf("the nudge must be a user message: %+v", last)
	}
	a, _ = newAgent(t, &scripted{replies: []Message{text(""), text("")}})
	if res := a.Run(context.Background(), WorkUnit{PromptText: "q"}); res.Status != "failed" || !strings.Contains(res.Result, "twice") {
		t.Errorf("a second empty answer fails: %+v", res)
	}
}
