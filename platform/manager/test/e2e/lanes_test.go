//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Section 10 — every inbound lane end to end, driven by fixtures, never by
// its upstream.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "test", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// forward opens (once) a port-forward to an adapter Service.
func (e *Env) forward(t *testing.T, service string, port int) *Forward {
	t.Helper()
	if f, ok := e.Adapter[service]; ok {
		return f
	}
	f, err := e.Cluster.Forward(Namespace, "svc/"+service, port)
	if err != nil {
		t.Fatal(err)
	}
	e.Adapter[service] = f
	return f
}

// 10.1 The alerting lane needs no alerting system: the Alertmanager-format
// fixture is POSTed to the adapter's webhook and opens a conversation through
// the claiming Pipeline.
func TestAlertmanagerLane(t *testing.T) {
	e := requireEnv(t)
	start := time.Now().Add(-5 * time.Second)
	am := e.forward(t, "agentops-signal-alertmanager", 8080)
	body := fixture(t, "alertmanager-webhook.json")
	// Make the fingerprints unique to this run so cooldown never folds them.
	stamp := fmt.Sprint(time.Now().UnixNano())
	body = []byte(strings.ReplaceAll(string(body), `"fp-`, `"fp-`+stamp+`-`))
	var code int
	var out string
	waitFor(t, "the webhook to accept the fixture", 2*time.Minute, func() (bool, error) {
		code, out = e.do(t, "POST", am.URL()+"/webhook/"+SourceAlerts, body, "")
		return code == 200, nil
	})
	if !strings.Contains(out, `"queued":2`) {
		t.Fatalf("two firing alerts must be queued: %d %s", code, out)
	}
	// The alert payload rides a payloadRef, so the conversation is matched by
	// its source and its age rather than by inline text.
	waitFor(t, "a conversation from the alert", 2*time.Minute, func() (bool, error) {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return false, err
		}
		for _, c := range items {
			if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == SourceAlerts &&
				c.CreationTimestamp.Time.After(start) {
				return true, nil
			}
		}
		return false, nil
	})
}

// 10.2 signal-cron fires on its own schedule and originates.
func TestCronLane(t *testing.T) {
	e := requireEnv(t)
	waitFor(t, "a conversation from the cron source", 3*time.Minute, func() (bool, error) {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return false, err
		}
		for _, c := range items {
			if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == SourceCron {
				return true, nil
			}
		}
		return false, nil
	})
}

// 10.3 signal-k8s-events: a genuinely failing workload's Warning event
// becomes a signal and opens a conversation. FULL TIER: the events source's
// default dwell re-checks the object for minutes before emitting.
func TestK8sEventsLane(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	ns := "e2e-broken-" + fmt.Sprint(time.Now().Unix()%100000)
	if out, err := e.Cluster.Kubectl(ctx, "create", "namespace", ns); err != nil {
		t.Fatalf("namespace: %v %s", err, out)
	}
	t.Cleanup(func() { _, _ = e.Cluster.Kubectl(context.Background(), "delete", "namespace", ns, "--wait=false") })
	createUnpullableBrokenDeployment(t, ctx, e, ns)
	waitFor(t, "a conversation about the broken workload", 12*time.Minute, func() (bool, error) {
		return conversationAboutBrokenWorkloadExists(ctx, e, ns)
	})
}

func createUnpullableBrokenDeployment(t *testing.T, ctx context.Context, e *Env, ns string) {
	t.Helper()
	one := int32(1)
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "broken"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "broken"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "broken"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "broken", Image: "agentops-e2e-image-that-does-not-exist:never",
					ImagePullPolicy: corev1.PullNever,
				}}},
			},
		},
	}
	if err := e.K.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
}

func conversationAboutBrokenWorkloadExists(ctx context.Context, e *Env, ns string) (bool, error) {
	items, err := e.K.Conversations(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range items {
		if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == SourceEvents && strings.Contains(c.Spec.Title+c.Spec.Signature, ns) {
			return true, nil
		}
		if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == SourceEvents {
			for _, in := range c.Spec.Inputs {
				if strings.Contains(in.Payload, ns) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// 10.4 + 10.5 The Telegram lane against the fake: the router classifies and
// forwards verbatim, ingest and channel adapters handle it, and the outbound
// send is observed as a recorded call; exactly ONE getUpdates consumer exists.
func TestTelegramLane(t *testing.T) {
	e := requireEnv(t)
	stamp := fmt.Sprint(time.Now().UnixNano())
	update := fixture(t, "telegram-update-message.json")
	// A fresh update_id and a run-specific text, so the router's offset and
	// the manager's fingerprint cooldown never fold it with an earlier run.
	var u map[string]any
	_ = json.Unmarshal(update, &u)
	u["update_id"] = time.Now().Unix() % 1000000000
	msg := u["message"].(map[string]any)
	msg["text"] = "echo telegram " + stamp
	fed, _ := json.Marshal(u)
	e.BotFeed(t, fed)
	// The reply reaches the general surface first (the manager's ack or the
	// answer in a topic); either way a sendMessage carrying the stub's echo.
	waitFor(t, "the answer sent back through the fake Bot API", 5*time.Minute, func() (bool, error) {
		for _, c := range e.BotCalls(t, "sendMessage") {
			b, _ := json.Marshal(c["body"])
			if strings.Contains(string(b), "telegram "+stamp) && strings.Contains(string(b), "[stub]") {
				return true, nil
			}
		}
		return false, nil
	})
	if len(e.BotCalls(t, "createForumTopic")) == 0 {
		t.Fatalf("a conversation on a forum supergroup opens a topic")
	}
	// 10.5 Exactly one consumer, ever.
	_, out := e.do(t, "GET", e.BotAPI.URL()+"/control/consumers", nil, "")
	var consumers map[string]int
	_ = json.Unmarshal([]byte(out), &consumers)
	if consumers["maxConcurrent"] != 1 || consumers["conflicts"] != 0 {
		t.Fatalf("exactly one getUpdates consumer must exist: %v", consumers)
	}
}

// 10.6 An unclaimed source drops its signal with Wired=False, and a chat
// source's reason reaches the surface the person typed on.
func TestUnclaimedSourceDrops(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	res := e.PostTask(t, SourceUnclaimed, "e2e-unclaimed-"+fmt.Sprint(time.Now().UnixNano()), "echo nobody")
	if fmt.Sprint(res["queued"]) != "0" || !strings.Contains(fmt.Sprint(res["reason"]), "Wired=False") {
		t.Fatalf("an unclaimed source must drop with Wired=False: %v", res)
	}
	waitFor(t, "Wired=False on the source", time.Minute, func() (bool, error) {
		var src struct {
			Status struct {
				Conditions []metav1.Condition `json:"conditions"`
			}
		}
		out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "get", "signalsource", SourceUnclaimed, "-o", "json")
		if err != nil {
			return false, err
		}
		_ = json.Unmarshal([]byte(out), &src)
		for _, c := range src.Status.Conditions {
			if c.Type == "Wired" && c.Status == metav1.ConditionFalse {
				return true, nil
			}
		}
		return false, nil
	})
	// A chat source nobody claims: the reason goes back to the surface.
	chat := source("e2e-unclaimed-chat", "e2e", nil)
	chat.Namespace = Namespace
	if err := e.K.Create(ctx, chat); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.K.Delete(context.Background(), chat) })
	stamp := fmt.Sprint(time.Now().UnixNano())
	e.PostSignal(t, chat.Name, map[string]any{"fingerprint": "chat-" + stamp, "kind": "chat", "payload": "hello " + stamp,
		"labels": map[string]string{"agentops.dev/channel": ChannelTelegram, "agentops.dev/sender": "operator"}})
	waitFor(t, "the Wired=False notice on the Telegram surface", 3*time.Minute, func() (bool, error) {
		for _, c := range e.BotCalls(t, "sendMessage") {
			b, _ := json.Marshal(c["body"])
			if strings.Contains(string(b), "Nothing here is wired") && strings.Contains(string(b), chat.Name) {
				return true, nil
			}
		}
		return false, nil
	})
}
