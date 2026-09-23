package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// workReportServer is a manager holding one inflight conversation on the
// runtime `claude`, whose image is what every reported call leaves from.
func workReportServer(t *testing.T) (*Server, *activity.Log) {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "conv-a", "agent-ops"
	conv.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "claude"}
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", DispatchedAt: metav1.Now()}
	rt := &agentopsv1alpha1.AgentRuntime{}
	rt.Name, rt.Namespace = "claude", "agent-ops"
	rt.Spec.Image = "example/runtime-claude:1"

	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithObjects(conv, rt).WithStatusSubresource(conv).Build()
	log := activity.New(64)
	return &Server{Reader: c, Client: c, Namespace: "agent-ops", Activity: log}, log
}

func postWorkDone(t *testing.T, s *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	s.handleWorkDone(rec, httptest.NewRequest("POST", "/work/done", strings.NewReader(string(raw))))
	return rec
}

func eventsOfKind(log *activity.Log, kind string) []activity.Event {
	all, _ := log.Since("", 0)
	var out []activity.Event
	for _, e := range all {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func TestWorkResultTurnsAndToolCallsBecomeHops(t *testing.T) {
	s, log := workReportServer(t)
	rec := postWorkDone(t, s, map[string]any{
		"convo": "conv-a", "runId": "r1", "status": "succeeded", "result": "done",
		"turns": []map[string]any{
			{"model": "model-a", "tokensIn": 1200, "tokensOut": 80, "cacheReadTokens": 900, "stopReason": "tool_use"},
			{"model": "model-a", "tokensIn": 1400, "tokensOut": 40, "stopReason": "end_turn"},
		},
		"toolCalls": []map[string]any{
			{"tool": "mcp__kubernetes__pods_list", "server": "kubernetes", "durationMs": 320, "resultBytes": 4096},
			{"tool": "Bash", "durationMs": 15, "resultBytes": 12},
		},
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	models := eventsOfKind(log, activity.KindModelCall)
	if len(models) != 2 {
		t.Fatalf("want one model.call per turn, got %d", len(models))
	}
	m := models[0]
	if m.From == nil || m.From.Kind != activity.NodeRuntimeImage || m.From.Name != "example/runtime-claude:1" {
		t.Fatalf("a model call leaves from the runtime image: %+v", m.From)
	}
	if m.To == nil || m.To.Kind != activity.NodeModel || m.To.Name != "model-a" {
		t.Fatalf("a model call goes to the model: %+v", m.To)
	}
	if m.RunID != "r1" || m.Conversation != "conv-a" {
		t.Fatalf("a model call carries the run: %+v", m)
	}
	if m.Data["tokensIn"] != "1200" || m.Data["tokensOut"] != "80" ||
		m.Data["cacheReadTokens"] != "900" || m.Data["stopReason"] != "tool_use" {
		t.Fatalf("the turn's facts ride in data: %+v", m.Data)
	}
	if _, ok := models[1].Data["cacheReadTokens"]; ok {
		t.Fatalf("an unreported fact is omitted, never zero: %+v", models[1].Data)
	}

	tools := eventsOfKind(log, activity.KindToolCall)
	if len(tools) != 2 {
		t.Fatalf("want one tool.call per call, got %d", len(tools))
	}
	if tools[0].To == nil || tools[0].To.Kind != activity.NodeMCPServer || tools[0].To.Name != "kubernetes" {
		t.Fatalf("an MCP tool call goes to its server: %+v", tools[0].To)
	}
	if tools[0].LatencyMs != 320 || tools[0].Data["resultBytes"] != "4096" || tools[0].Data["tool"] != "mcp__kubernetes__pods_list" {
		t.Fatalf("the call's facts: %+v", tools[0])
	}
	if tools[1].To != nil {
		t.Fatalf("a built-in tool call carries no to: %+v", tools[1].To)
	}

	// The calls precede the completion, which is the order they happened in.
	all, _ := log.Since("", 0)
	if last := all[len(all)-1]; last.Kind != activity.KindRunCompleted {
		t.Fatalf("run.completed must follow the run's own calls, last was %s", last.Kind)
	}
}

func TestWorkResultWithoutTurnsDrawsNothingNew(t *testing.T) {
	s, log := workReportServer(t)
	rec := postWorkDone(t, s, map[string]any{"convo": "conv-a", "runId": "r1", "status": "succeeded"})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if n := len(eventsOfKind(log, activity.KindModelCall)) + len(eventsOfKind(log, activity.KindToolCall)); n != 0 {
		t.Fatalf("a silent runtime draws nothing new, got %d hops", n)
	}
	if len(eventsOfKind(log, activity.KindRunCompleted)) != 1 {
		t.Fatal("the completion is recorded as before")
	}
}

func TestOversizedWorkResultIsRefused(t *testing.T) {
	turns := make([]map[string]any, MaxReportedTurns+1)
	for i := range turns {
		turns[i] = map[string]any{"model": "m"}
	}
	calls := make([]map[string]any, MaxReportedToolCalls+1)
	for i := range calls {
		calls[i] = map[string]any{"tool": "Read"}
	}
	for name, extra := range map[string]map[string]any{
		"too many turns":      {"turns": turns},
		"too many tool calls": {"toolCalls": calls},
		"a long model name":   {"turns": []map[string]any{{"model": strings.Repeat("m", MaxReportedField+1)}}},
		"a long tool name":    {"toolCalls": []map[string]any{{"tool": strings.Repeat("t", MaxReportedField+1)}}},
		"a negative count":    {"turns": []map[string]any{{"model": "m", "tokensIn": -1}}},
	} {
		t.Run(name, func(t *testing.T) {
			s, log := workReportServer(t)
			body := map[string]any{"convo": "conv-a", "runId": "r1", "status": "succeeded"}
			for k, v := range extra {
				body[k] = v
			}
			rec := postWorkDone(t, s, body)
			if rec.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			if all, _ := log.Since("", 0); len(all) != 0 {
				t.Fatalf("a refused report records nothing, got %d events", len(all))
			}
		})
	}
}

func TestWorkResultFallsBackToTheRuntimeWhenNoImageResolves(t *testing.T) {
	s, log := workReportServer(t)
	conv := &agentopsv1alpha1.Conversation{}
	if err := s.Client.Get(t.Context(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, conv); err != nil {
		t.Fatal(err)
	}
	conv.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "gone"}
	if err := s.Client.Update(t.Context(), conv); err != nil {
		t.Fatal(err)
	}
	postWorkDone(t, s, map[string]any{"convo": "conv-a", "runId": "r1", "status": "succeeded",
		"turns": []map[string]any{{"model": "m"}}})
	models := eventsOfKind(log, activity.KindModelCall)
	if len(models) != 1 || models[0].From.Kind != activity.NodeRuntime || models[0].From.Name != "gone" {
		t.Fatalf("an unresolvable runtime names the runtime, never an invented image: %+v", models)
	}
}
