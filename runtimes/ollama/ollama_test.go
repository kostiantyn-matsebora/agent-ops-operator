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

func TestChatAlwaysSetsNumCtxAndStreams(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		for _, line := range []string{
			`{"message":{"role":"assistant","content":"Hel"},"done":false}`,
			`{"message":{"role":"assistant","content":"lo"},"done":false}`,
			`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"Read","arguments":{"path":"x"}}}]},"done":true}`,
		} {
			io.WriteString(w, line+"\n")
		}
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", NumCtx: 4096, KeepAlive: "5m", HTTP: srv.Client()}
	var out strings.Builder
	msg, err := o.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := got.Options["num_ctx"]; !ok || v != float64(4096) {
		t.Errorf("num_ctx must be on EVERY request, got %v", got.Options)
	}
	if got.KeepAlive != "5m" || !got.Stream {
		t.Errorf("keep_alive/stream: %+v", got)
	}
	if msg.Content != "Hello" || out.String() != "Hello" {
		t.Errorf("stream assembly: %q / %q", msg.Content, out.String())
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "Read" {
		t.Errorf("tool calls: %+v", msg.ToolCalls)
	}
}

func TestChatFailuresNameTheEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":"model not loaded"}`)
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", NumCtx: 1, HTTP: srv.Client()}
	_, err := o.Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), srv.URL) || !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("want endpoint and reason, got %v", err)
	}
	srv.Close()
	if _, err := o.Chat(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("transport failure must name the endpoint: %v", err)
	}
}

func TestCheckPicksTheOnlyModelWhenNoneIsConfigured(t *testing.T) {
	models := `{"models":[{"name":"qwen2.5:7b"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, models)
		case "/api/show":
			io.WriteString(w, `{"capabilities":["completion","tools"]}`)
		}
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, HTTP: srv.Client()}
	if info, err := o.Check(context.Background()); err != nil || o.Model != "qwen2.5:7b" || !info.Present {
		t.Errorf("the only model must be chosen: model=%q %+v %v", o.Model, info, err)
	}
	models = `{"models":[{"name":"a"},{"name":"b"}]}`
	o = &Ollama{URL: srv.URL, HTTP: srv.Client()}
	if info, err := o.Check(context.Background()); err != nil || o.Model != "" || len(info.Models) != 2 {
		t.Errorf("several models and none configured must leave the choice open, naming them: %q %+v %v", o.Model, info, err)
	}
	models = `{"models":[]}`
	o = &Ollama{URL: srv.URL, HTTP: srv.Client()}
	if _, err := o.Check(context.Background()); err != nil || o.Model != "" {
		t.Errorf("no model pulled: %q %v", o.Model, err)
	}
}

func TestCheck(t *testing.T) {
	present := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[]}`)
		case "/api/show":
			if !present {
				w.WriteHeader(404)
				return
			}
			io.WriteString(w, `{"capabilities":["completion","tools"]}`)
		}
	}))
	defer srv.Close()
	o := &Ollama{URL: srv.URL, Model: "m", HTTP: srv.Client()}
	info, err := o.Check(context.Background())
	if err != nil || !info.Present || !info.Tools {
		t.Errorf("present+tools: %+v %v", info, err)
	}
	present = false
	if info, err := o.Check(context.Background()); err != nil || info.Present {
		t.Errorf("missing model is reported, not errored: %+v %v", info, err)
	}
}
