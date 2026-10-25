//go:build conformance

package conformance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A bare http.Client speaking the double's wire shape directly — no runtime
// binary involved. runtime_test.go drives the real binary against it.

func fakeOllamaChat(t *testing.T, o *FakeOllama, prompt string) fakeOllamaChatChunk {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model":    o.Model,
		"messages": []map[string]any{{"role": "user", "content": prompt}},
		"stream":   true,
	})
	resp, err := http.Post(o.URL()+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("POST /api/chat: %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	var chunk fakeOllamaChatChunk
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			t.Fatalf("decoding chunk: %v: %s", err, line)
		}
	}
	return chunk
}

func TestFakeOllamaTagsAndShowAdvertiseExactlyOneToolCapableModel(t *testing.T) {
	o := NewFakeOllama(t)
	resp, err := http.Get(o.URL() + "/api/tags")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct{ Name string } `json:"models"`
	}
	json.NewDecoder(resp.Body).Decode(&tags)
	if len(tags.Models) != 1 || tags.Models[0].Name != o.Model {
		t.Fatalf("exactly one model must be listed, named %q: %+v", o.Model, tags.Models)
	}

	resp, err = http.Post(o.URL()+"/api/show", "application/json", strings.NewReader(`{"model":"fake-model"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var show struct {
		Capabilities []string `json:"capabilities"`
	}
	json.NewDecoder(resp.Body).Decode(&show)
	found := false
	for _, c := range show.Capabilities {
		if c == "tools" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the model must advertise tool-calling, got %v", show.Capabilities)
	}
}

func TestFakeOllamaDefaultDirectiveReturnsAFixedCompletionWithUsageAndBrief(t *testing.T) {
	o := NewFakeOllama(t)
	chunk := fakeOllamaChat(t, o, "hello there")
	if !strings.Contains(chunk.Message.Content, "hello there") {
		t.Fatalf("the default completion must echo the prompt, got %q", chunk.Message.Content)
	}
	if !strings.Contains(chunk.Message.Content, "<brief>") {
		t.Fatalf("the default completion must carry a <brief> tag, got %q", chunk.Message.Content)
	}
	if chunk.PromptEvalCount == nil || chunk.EvalCount == nil {
		t.Fatalf("the default completion must report usage counts, got %+v", chunk)
	}
	if !chunk.Done {
		t.Fatalf("the default completion must be done in one chunk")
	}
}

func TestFakeOllamaSlowDirectiveHoldsPastOnePollCycle(t *testing.T) {
	o := NewFakeOllama(t)
	o.SlowDelay = 300 * time.Millisecond
	start := time.Now()
	chunk := fakeOllamaChat(t, o, "slow task")
	if elapsed := time.Since(start); elapsed < o.SlowDelay {
		t.Fatalf("the slow directive must hold its response for at least %s, took %s", o.SlowDelay, elapsed)
	}
	if !chunk.Done || chunk.Message.Content == "" {
		t.Fatalf("the slow directive still answers eventually: %+v", chunk)
	}
}

func TestFakeOllamaUnparsableDirectiveOmitsUsageAndBrief(t *testing.T) {
	o := NewFakeOllama(t)
	chunk := fakeOllamaChat(t, o, "unparsable task")
	if chunk.PromptEvalCount != nil || chunk.EvalCount != nil {
		t.Fatalf("a fact the double could not determine must be OMITTED, never a zero: %+v", chunk)
	}
	if strings.Contains(chunk.Message.Content, "<brief>") {
		t.Fatalf("the unparsable directive must carry no <brief> tag, got %q", chunk.Message.Content)
	}
}

func TestFakeOllamaCallsDirectiveAsksForATollCallThenFinishes(t *testing.T) {
	o := NewFakeOllama(t)
	first := fakeOllamaChat(t, o, "calls now")
	if len(first.Message.ToolCalls) != 1 || first.Message.ToolCalls[0].Function.Name != "Bash" {
		t.Fatalf("the first turn must ask for a Bash call: %+v", first.Message)
	}

	body, _ := json.Marshal(map[string]any{
		"model": o.Model,
		"messages": []map[string]any{
			{"role": "user", "content": "calls now"},
			{"role": "assistant", "tool_calls": first.Message.ToolCalls},
			{"role": "tool", "tool_name": "Bash", "content": "AGENTOPS_CONFORMANCE_TOOL_MARKER\n"},
		},
		"stream": true,
	})
	resp, err := http.Post(o.URL()+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var second fakeOllamaChatChunk
	json.NewDecoder(resp.Body).Decode(&second)
	if len(second.Message.ToolCalls) != 0 || second.Message.Content == "" {
		t.Fatalf("the second turn, with a tool result present, must give a final text answer: %+v", second.Message)
	}
}
