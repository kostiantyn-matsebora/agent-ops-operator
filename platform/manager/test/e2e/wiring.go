//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The pack's wiring, created once and shared: a stub profile, task sources,
// and the Pipelines that claim the chart's own sources (console, tg-ops,
// cluster-events) plus the pack's. Everything a test starts goes through
// this wiring exactly as an adopter's would.
const (
	ProfileStub      = "e2e-stub"
	SourceTasks      = "e2e-tasks"     // task lane, console-bound
	SourceFanout     = "e2e-fanout"    // task lane, console + telegram
	SourceUnclaimed  = "e2e-unclaimed" // nobody claims it
	SourceAlerts     = "vm-alerts"     // the alertmanager adapter's webhook source
	PipelineConsole  = "e2e-console"   // claims e2e-tasks + console, delivers to console
	PipelineFanout   = "e2e-fanout"    // claims e2e-fanout, delivers to console + tg-ops
	PipelineChat     = "e2e-chat"      // claims tg-ops, delivers to tg-ops
	PipelineEvents   = "e2e-events"    // claims cluster-events + vm-alerts, delivers to console
	PipelineCron     = "e2e-cron"      // claims e2e-cron alone, so a test can pause that lane
	ChannelConsole   = "console"
	ChannelTelegram  = "tg-ops"
	SourceConsole    = "console"
	SourceTelegram   = "tg-ops"
	SourceEvents     = "cluster-events"
	AdapterCron      = "cron"
	SourceCron       = "e2e-cron"
	AdapterAlertmngr = "alertmanager"
)

func raw(v any) *runtime.RawExtension {
	b, _ := json.Marshal(v)
	return &runtime.RawExtension{Raw: b}
}

func pipeline(name, profile string, sources, channels []string) *agentopsv1alpha1.Pipeline {
	p := &agentopsv1alpha1.Pipeline{ObjectMeta: metav1.ObjectMeta{Name: name}}
	p.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	for _, s := range sources {
		p.Spec.SignalSourceRefs = append(p.Spec.SignalSourceRefs, agentopsv1alpha1.ObjectRef{Name: s})
	}
	for _, c := range channels {
		p.Spec.ChannelRefs = append(p.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	return p
}

func source(name, adapter string, config any) *agentopsv1alpha1.SignalSource {
	s := &agentopsv1alpha1.SignalSource{ObjectMeta: metav1.ObjectMeta{Name: name}}
	s.Spec.Adapter = adapter
	if config != nil {
		s.Spec.Config = raw(config)
	}
	return s
}

// SetupWiring creates the shared objects and waits for every Pipeline to be
// Ready — which is itself the first informer-liveness fact: a reconciler that
// cannot watch Pipelines never writes the condition.
func SetupWiring(ctx context.Context, k *Kube) error {
	profile := &agentopsv1alpha1.AgentProfile{ObjectMeta: metav1.ObjectMeta{Name: ProfileStub}}
	profile.Spec.Agent = "stub"
	profile.Spec.OutputFormat = agentopsv1alpha1.OutputFormat("none")
	profile.Spec.SystemPrompt = "You are the e2e stub."
	cron := &agentopsv1alpha1.SignalAdapter{ObjectMeta: metav1.ObjectMeta{Name: AdapterCron}}
	cron.Spec.Image = "agentops-signal-cron:e2e"
	singleton := true
	cron.Spec.Singleton = &singleton

	for _, obj := range []client.Object{
		profile, cron,
		source(SourceTasks, "e2e", nil),
		source(SourceFanout, "e2e", nil),
		source(SourceUnclaimed, "e2e", nil),
		source(SourceAlerts, AdapterAlertmngr, map[string]any{}),
		source(SourceCron, AdapterCron, map[string]any{"schedule": "* * * * *", "input": "echo cron tick", "title": "e2e cron"}),
		pipeline(PipelineConsole, ProfileStub, []string{SourceTasks, SourceConsole}, []string{ChannelConsole}),
		pipeline(PipelineCron, ProfileStub, []string{SourceCron}, []string{ChannelConsole}),
		pipeline(PipelineFanout, ProfileStub, []string{SourceFanout}, []string{ChannelConsole, ChannelTelegram}),
		pipeline(PipelineChat, ProfileStub, []string{SourceTelegram}, []string{ChannelTelegram}),
		pipeline(PipelineEvents, ProfileStub, []string{SourceEvents, SourceAlerts}, []string{ChannelConsole}),
	} {
		if err := ensure(ctx, k, obj); err != nil {
			return fmt.Errorf("creating %s: %w", obj.GetName(), err)
		}
	}
	for _, name := range []string{PipelineConsole, PipelineFanout, PipelineChat, PipelineEvents, PipelineCron} {
		if err := waitPipelineReady(ctx, k, name, 2*time.Minute); err != nil {
			return err
		}
	}
	return k.WaitDeploymentAvailable(ctx, "agentops-signal-"+AdapterCron, 3*time.Minute)
}

func waitPipelineReady(ctx context.Context, k *Kube, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		var p agentopsv1alpha1.Pipeline
		if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: name}, &p); err == nil {
			for _, c := range p.Status.Conditions {
				if c.Type == "Ready" {
					if c.Status == metav1.ConditionTrue {
						return nil
					}
					last = c.Reason + ": " + c.Message
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("pipeline %s not Ready after %s (%s)", name, timeout, last)
}

// ---- driving the install ----------------------------------------------------

// PostTask posts a task-lane signal to a source through the manager's own
// ingest endpoint — an ordinary signal to a source a Ready Pipeline claims;
// there is no route that names a Pipeline. Returns the response body.
func (e *Env) PostTask(t *testing.T, source, fingerprint, payload string) map[string]any {
	t.Helper()
	return e.PostSignal(t, source, map[string]any{"fingerprint": fingerprint, "kind": "task", "payload": payload, "title": fingerprint})
}

// PostSignal posts one normalized signal.
func (e *Env) PostSignal(t *testing.T, source string, signal map[string]any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"source": source, "signals": []any{signal}})
	code, out := e.do(t, "POST", e.Manager.URL()+"/signal/inbound", body, "Bearer "+e.Values.AdapterToken)
	if code != 200 {
		t.Fatalf("POST /signal/inbound: %d %s", code, out)
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	return res
}

// ConversationFor finds the conversation a task with this title opened.
func (e *Env) ConversationFor(t *testing.T, fingerprint string, timeout time.Duration) *agentopsv1alpha1.Conversation {
	t.Helper()
	var found *agentopsv1alpha1.Conversation
	waitFor(t, "a conversation for "+fingerprint, timeout, func() (bool, error) {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return false, err
		}
		for i := range items {
			c := &items[i]
			if c.Spec.Title == fingerprint || c.Spec.Signature == fingerprint {
				found = c
				return true, nil
			}
		}
		return false, nil
	})
	return found
}

// WaitRun waits until the conversation has at least n finished runs, and
// returns the conversation.
func (e *Env) WaitRun(t *testing.T, name string, n int, timeout time.Duration) *agentopsv1alpha1.Conversation {
	t.Helper()
	var conv *agentopsv1alpha1.Conversation
	waitFor(t, fmt.Sprintf("run %d of %s", n, name), timeout, func() (bool, error) {
		c, err := e.K.Conversation(context.Background(), name)
		if err != nil {
			return false, err
		}
		conv = c
		finished := 0
		for _, r := range c.Status.Runs {
			if r.FinishedAt != nil {
				finished++
			}
		}
		return finished >= n, nil
	})
	return conv
}

// ConsoleSend continues a conversation through the console's write path.
func (e *Env) ConsoleSend(t *testing.T, name, text string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	return e.do(t, "POST", e.Console.URL()+"/api/conversations/"+name+"/messages", body, "Bearer "+e.Values.UIToken)
}

// ConsoleStart starts a conversation through the console.
func (e *Env) ConsoleStart(t *testing.T, task string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"task": task})
	return e.do(t, "POST", e.Console.URL()+"/api/conversations", body, "Bearer "+e.Values.UIToken)
}

// ConsoleTranscript reads a conversation's transcript as JSON text.
func (e *Env) ConsoleTranscript(t *testing.T, name string) string {
	t.Helper()
	_, out := e.do(t, "GET", e.Console.URL()+"/api/conversations/"+name, nil, "Bearer "+e.Values.UIToken)
	return out
}

// BotCalls reads the fake Bot API's recorded calls.
func (e *Env) BotCalls(t *testing.T, method string) []map[string]any {
	t.Helper()
	_, out := e.do(t, "GET", e.BotAPI.URL()+"/control/calls?method="+method, nil, "")
	var calls []map[string]any
	_ = json.Unmarshal([]byte(out), &calls)
	return calls
}

// BotFeed queues an Update on the fake Bot API.
func (e *Env) BotFeed(t *testing.T, update []byte) {
	t.Helper()
	if code, out := e.do(t, "POST", e.BotAPI.URL()+"/control/updates", update, ""); code != 200 {
		t.Fatalf("feeding the fake bot api: %d %s", code, out)
	}
}

func (e *Env) do(t *testing.T, method, url string, body []byte, auth string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	var resp *http.Response
	var err error
	// One retry on a transport error: a port-forward whose pod just rolled
	// answers EOF once and is fine again a moment later.
	for attempt := 0; attempt < 3; attempt++ {
		req.Body = io.NopCloser(bytes.NewReader(body))
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
