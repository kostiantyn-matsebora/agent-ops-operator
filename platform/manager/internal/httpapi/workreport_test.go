package httpapi

import (
	"encoding/json"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	objs := []client.Object{conv, rt}
	assignUIDs(objs)
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithObjects(objs...).WithStatusSubresource(conv).Build()
	log := activity.New(64)
	return &Server{Reader: c, Conversations: testConversations(t, c, "agent-ops"), Runs: testRuns(t, c, "agent-ops"), Client: c, Namespaces: namespace.Namespaces{Control: "agent-ops", Conversations: "agent-ops"}, Activity: log}, log
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
			{"model": "model-a", "tokensIn": 1200, "tokensOut": 80, "cacheReadTokens": 900, "cacheCreationTokens": 250, "stopReason": "tool_use"},
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
		m.Data["cacheReadTokens"] != "900" || m.Data["cacheCreationTokens"] != "250" ||
		m.Data["stopReason"] != "tool_use" {
		t.Fatalf("the turn's facts ride in data: %+v", m.Data)
	}
	if _, ok := models[1].Data["cacheReadTokens"]; ok {
		t.Fatalf("an unreported fact is omitted, never zero: %+v", models[1].Data)
	}
	if _, ok := models[1].Data["cacheCreationTokens"]; ok {
		t.Fatalf("an unreported cache write is omitted, never zero: %+v", models[1].Data)
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

// A run that was still in flight when /close landed reports its completion
// here — the report must be recorded (it is still a real answer) but phase
// must stay Closed: Closed is sticky until an explicit reopen. Measured live
// on the chart-v14.0.0 release smoke (TestConsoleLifecycle/close_then_delete):
// handleWorkDone's own phase write folded into the same unconditioned patch
// as the rest of the report, so it silently reverted phase to Idle while
// ClosedAt stayed stamped.
func TestWorkDoneNeverReopensAClosedConversation(t *testing.T) {
	s, _ := workReportServer(t)
	conv := &agentopsv1alpha1.Conversation{}
	if err := s.Client.Get(t.Context(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, conv); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.ClosedAt = &now
	if err := s.Client.Status().Update(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	rec := postWorkDone(t, s, map[string]any{"convo": "conv-a", "runId": "r1", "status": "succeeded", "result": "done"})
	if rec.Code != 200 {
		t.Fatalf("a belated report for a closed conversation is still accepted: %d %s", rec.Code, rec.Body.String())
	}

	got := &agentopsv1alpha1.Conversation{}
	if err := s.Client.Get(t.Context(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("a belated work report must not reopen a closed conversation, got phase %q", got.Status.Phase)
	}
	if got.Status.ClosedAt == nil {
		t.Fatalf("closedAt must survive the report")
	}
	if len(got.Status.Runs) != 1 || got.Status.Runs[0].Result != "done" {
		t.Fatalf("the run's result is still worth recording even after close: %+v", got.Status.Runs)
	}
}

// A pod still long-polling when /close landed must be handed nothing, even with
// an input queued before the close. Dispatching it writes Working over Closed,
// and Closed is sticky: the conversation never reads Closed again and its pod
// runs on until its idle timeout. Measured on the manager-v0.65.0 release smoke
// (TestReplicasTwoDeliversEveryConsoleThread): the pod ran a work unit one
// second after the close, and the conversation never closed.
func TestWorkIsNotServedToAClosedConversation(t *testing.T) {
	s, _ := workReportServer(t)
	conv := &agentopsv1alpha1.Conversation{}
	if err := s.Client.Get(t.Context(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, conv); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.ClosedAt = &now
	conv.Status.Inflight = nil
	if err := s.Client.Status().Update(t.Context(), conv); err != nil {
		t.Fatal(err)
	}

	unit, served, err := s.tryDispatch(t.Context(), "conv-a", "pod-a")
	if err != nil || served {
		t.Fatalf("a closed conversation serves no work, got served=%v err=%v unit=%+v", served, err, unit)
	}

	got := &agentopsv1alpha1.Conversation{}
	if err := s.Client.Get(t.Context(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed || got.Status.Inflight != nil {
		t.Fatalf("polling must leave a closed conversation closed with nothing in flight, got phase %q inflight %+v",
			got.Status.Phase, got.Status.Inflight)
	}
}

// A runtime pod is handed its CONVO_ID once, at creation, and polls /work
// with it for the pod's whole life (runtime-spec, podspec.go). If the
// conversation it names is deleted and a NEW, unrelated conversation is
// created under the same reused Name — AllocateName permits exactly this —
// the stale pod's own id must resolve to NOTHING, never to the new
// conversation's queue. Resolving by Name instead of ID (the bug conversation-
// identity Decision 1/7 fixes) would hand the old pod the new conversation's
// work.
func TestWorkDoesNotResolveAStalePodsIDToAConversationThatReusedItsName(t *testing.T) {
	s, _ := workReportServer(t)
	oldConv := &agentopsv1alpha1.Conversation{}
	if err := s.Client.Get(t.Context(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, oldConv); err != nil {
		t.Fatal(err)
	}
	staleID := string(oldConv.UID)
	if err := s.Client.Delete(t.Context(), oldConv); err != nil {
		t.Fatal(err)
	}

	reborn := &agentopsv1alpha1.Conversation{}
	reborn.Name, reborn.Namespace = "conv-a", "agent-ops"
	reborn.UID = types.UID("conv-a-reborn")
	reborn.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "claude"}
	if err := s.Client.Create(t.Context(), reborn); err != nil {
		t.Fatal(err)
	}

	if _, served, err := s.tryDispatch(t.Context(), staleID, "pod-a"); err == nil || served {
		t.Fatalf("a stale CONVO_ID must never resolve to a reborn conversation sharing its old name: served=%v err=%v", served, err)
	}
}

// The complement: an id naming no conversation at all — never deleted,
// simply unknown — is refused the same way, never silently served as if it
// named the first record List happened to see.
func TestWorkRefusesAnUnknownConvoID(t *testing.T) {
	s, _ := workReportServer(t)
	if _, served, err := s.tryDispatch(t.Context(), "never-existed", "pod-a"); err == nil || served {
		t.Fatalf("an unknown convo id must be refused, got served=%v err=%v", served, err)
	}
}
