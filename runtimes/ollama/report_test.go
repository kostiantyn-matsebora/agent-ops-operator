package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func i64(n int64) *int64 { return &n }

func withUsage(m Message, model string, in, out int64, stop string) Message {
	m.Usage = &Usage{Model: model, TokensIn: i64(in), TokensOut: i64(out), StopReason: stop}
	return m
}

func TestChatReadsTheDoneChunksAccounting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"qwen3:8b","message":{"role":"assistant","content":"hi"},"done":false}`+"\n")
		io.WriteString(w, `{"model":"qwen3:8b","message":{"role":"assistant","content":""},"done":true,`+
			`"done_reason":"stop","prompt_eval_count":812,"eval_count":24}`+"\n")
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", NumCtx: 4096, HTTP: srv.Client()}
	msg, err := o.Chat(context.Background(), []Message{{Role: "user", Content: "q"}}, nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	u := msg.Usage
	if u == nil || u.Model != "qwen3:8b" || *u.TokensIn != 812 || *u.TokensOut != 24 || u.StopReason != "stop" {
		t.Fatalf("usage: %+v", u)
	}
	if b, _ := json.Marshal(msg); strings.Contains(string(b), "812") {
		t.Fatalf("usage must never enter the transcript: %s", b)
	}
}

func TestChatWithoutCountsLeavesThemUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"message":{"role":"assistant","content":"hi"},"done":true}`+"\n")
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", NumCtx: 4096, HTTP: srv.Client()}
	msg, _ := o.Chat(context.Background(), nil, nil, io.Discard)
	if msg.Usage == nil || msg.Usage.Model != "m" || msg.Usage.TokensIn != nil || msg.Usage.TokensOut != nil {
		t.Fatalf("an absent count is omitted, never zero: %+v", msg.Usage)
	}
}

func TestRunReportsItsTurnsAndToolCalls(t *testing.T) {
	s := &scripted{replies: []Message{
		withUsage(toolCall("Read", `{"path":"f.txt"}`), "m", 100, 10, "stop"),
		withUsage(text("it says contents"), "m", 140, 6, "stop"),
	}}
	a, _ := newAgent(t, s)
	res := a.Run(context.Background(), WorkUnit{RunID: "r1", Convo: "c", PromptText: "q", AllowedTools: "Read"})
	if res.Status != "succeeded" {
		t.Fatalf("%+v", res)
	}
	if len(res.Turns) != 2 || *res.Turns[0].TokensIn != 100 || *res.Turns[1].TokensOut != 6 {
		t.Fatalf("turns: %+v", res.Turns)
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("tool calls: %+v", res.ToolCalls)
	}
	c := res.ToolCalls[0]
	if c.Tool != "Read" || c.Server != "" || c.DurationMs == nil || *c.ResultBytes != int64(len("contents")) {
		t.Fatalf("a built-in call names no server and measures its result: %+v", c)
	}
	body, _ := json.Marshal(res)
	if strings.Contains(string(body), "f.txt") {
		t.Fatalf("a tool's arguments are content and never reported: %s", body)
	}
}

func TestSilentRunReportsNothing(t *testing.T) {
	a, _ := newAgent(t, &scripted{replies: []Message{text("the answer")}})
	res := a.Run(context.Background(), WorkUnit{RunID: "r1", Convo: "c", PromptText: "q"})
	body, _ := json.Marshal(res)
	if strings.Contains(string(body), "turns") || strings.Contains(string(body), "toolCalls") {
		t.Fatalf("a chatter reporting no usage adds nothing: %s", body)
	}
}

func TestReportIsTrimmedToTheManagersBounds(t *testing.T) {
	l := &callLog{}
	for i := 0; i < maxReportedTurns+3; i++ {
		l.turn(&Usage{Model: strings.Repeat("m", maxReportedField+9)})
		l.tool("Read", 0, 1)
	}
	turns, tools := l.report()
	if len(turns) != maxReportedTurns || len(tools) != maxReportedToolCalls || len(turns[0].Model) != maxReportedField {
		t.Fatalf("turns=%d tools=%d model=%d", len(turns), len(tools), len(turns[0].Model))
	}
}

func TestMCPServerOf(t *testing.T) {
	for name, want := range map[string]string{
		"mcp__kubernetes__pods_list": "kubernetes", "Read": "", "mcp__broken": "",
	} {
		if got := mcpServerOf(name); got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
}
