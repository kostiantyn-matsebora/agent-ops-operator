//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The runtime's model and tool calls cross the work contract into a real
// manager and come out as hops. A cluster decides this: the report is made by
// a runtime pod the kubelet started, over the network the chart wired, and the
// manager resolves the image that pod runs from the AgentRuntime it names.
func TestRuntimeCallsBecomeHops(t *testing.T) {
	e := requireEnv(t)
	fp := "e2e-calls-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, SourceTasks, fp, "calls")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	conv = e.WaitRun(t, conv.Name, 1, 4*time.Minute)
	runID := conv.Status.Runs[len(conv.Status.Runs)-1].RunID

	type hop struct {
		Kind  string                       `json:"kind"`
		From  *struct{ Kind, Name string } `json:"from"`
		To    *struct{ Kind, Name string } `json:"to"`
		RunID string                       `json:"runId"`
		Data  map[string]string            `json:"data"`
	}
	var models, tools []hop
	waitFor(t, "the run's model and tool hops", time.Minute, func() (bool, error) {
		code, out := e.do(t, "GET", e.Manager.URL()+"/activity?limit=10000", nil, bearerPrefix+e.Values.AdapterToken)
		if code != 200 {
			return false, fmt.Errorf("GET /activity: %d %s", code, out)
		}
		var feed struct {
			Events []hop `json:"events"`
		}
		if err := json.Unmarshal([]byte(out), &feed); err != nil {
			return false, err
		}
		models, tools = nil, nil
		for _, h := range feed.Events {
			if h.RunID != runID {
				continue
			}
			switch h.Kind {
			case "model.call":
				models = append(models, h)
			case "tool.call":
				tools = append(tools, h)
			}
		}
		return len(models) == 2 && len(tools) == 1, nil
	})

	for _, m := range models {
		if m.From == nil || m.From.Kind != "runtime-image" || m.From.Name == "" {
			t.Fatalf("a model call leaves from the image the pod ran: %+v", m.From)
		}
		if m.To == nil || m.To.Kind != "model" || m.To.Name != "stub-model" || m.Data["tokensIn"] == "" {
			t.Fatalf("a model call reaches its model with its tokens: %+v", m)
		}
	}
	if to := tools[0].To; to == nil || to.Kind != "mcp-server" || to.Name != "stub" || tools[0].Data["tool"] != "mcp__stub__lookup" {
		t.Fatalf("a tool call reaches its MCP server: %+v", tools[0])
	}
}
