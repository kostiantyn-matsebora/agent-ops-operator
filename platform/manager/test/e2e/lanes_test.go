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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// 10.3.1 signal-k8s-events drain awareness: the SUBSTRATE fact envtest cannot
// decide is whether a REAL `kubectl cordon` actually suppresses. The rules
// engine and the drain predicate are unit-tested exhaustively in
// signals/k8s-events/; what only a live cluster proves is that the adapter's
// node watch reflects a cordon and reaches the same decision against it.
//
// Uses OOMKilling (a `for: "0"` reason) rather than the default lane's
// genuinely-broken workload so the positive half reports within seconds
// instead of riding out a multi-minute dwell — this lane belongs in the
// SMOKE tier, gating every release, not the nightly full pack.
func TestK8sEventsDrainAwareness(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	ns := "e2e-drain-" + fmt.Sprint(time.Now().Unix()%100000)
	if out, err := e.Cluster.Kubectl(ctx, "create", "namespace", ns); err != nil {
		t.Fatalf("namespace: %v %s", err, out)
	}
	t.Cleanup(func() { _, _ = e.Cluster.Kubectl(context.Background(), "delete", "namespace", ns, "--wait=false") })

	// Scheduling assigns a node whether or not the image will ever pull —
	// exactly why the broken-workload lane above uses the same image.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "e2e-drain-subject"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "subject", Image: "agentops-e2e-image-that-does-not-exist:never",
			ImagePullPolicy: corev1.PullNever,
		}}},
	}
	if err := e.K.Create(ctx, pod); err != nil {
		t.Fatalf("creating subject pod: %v", err)
	}
	var subject corev1.Pod
	waitFor(t, "the subject pod scheduled onto a node", 2*time.Minute, func() (bool, error) {
		if err := e.K.Get(ctx, client.ObjectKeyFromObject(pod), &subject); err != nil {
			return false, err
		}
		return subject.Spec.NodeName != "", nil
	})
	node := subject.Spec.NodeName

	if out, err := e.Cluster.Kubectl(ctx, "cordon", node); err != nil {
		t.Fatalf("cordon %s: %v %s", node, err, out)
	}
	t.Cleanup(func() { _, _ = e.Cluster.Kubectl(context.Background(), "uncordon", node) })
	// Settle: the adapter's own node WATCH (a separate pod) needs a moment to
	// see the cordon before a synthetic event posted against it means
	// anything — racing the two once cost an entire run (the uncordon half
	// below raced the same way and lost, taking the event through the
	// ordinary catch-all instead of proving drain awareness released it).
	time.Sleep(10 * time.Second)

	stamp := fmt.Sprint(time.Now().UnixNano())
	postSyntheticEvent(t, ctx, e, &subject, "OOMKilling", "suppressed-"+stamp)

	// A negative assertion, so the wait has to be long enough that a
	// regression reintroducing the old unconditional for:"0" firing would be
	// caught — not merely "hasn't happened yet". `for: "0"` reports within
	// one poll of the event; 45s is generous room past that.
	time.Sleep(45 * time.Second)
	if found, err := conversationMentioning(ctx, e, "suppressed-"+stamp); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("an event on a draining node's pod must not open a conversation")
	}

	if out, err := e.Cluster.Kubectl(ctx, "uncordon", node); err != nil {
		t.Fatalf("uncordon %s: %v %s", node, err, out)
	}
	time.Sleep(10 * time.Second) // the same settle, the other direction
	postSyntheticEvent(t, ctx, e, &subject, "OOMKilling", "reported-"+stamp)
	waitFor(t, "a conversation about the uncordoned pod's event", 3*time.Minute, func() (bool, error) {
		return conversationMentioning(ctx, e, "reported-"+stamp)
	})
}

// postSyntheticEvent creates a core/v1 Event as the node lifecycle controller
// would for a pod on a draining node — this test does not need a REAL failure
// to prove suppression, only an event the adapter's watch actually sees.
func postSyntheticEvent(t *testing.T, ctx context.Context, e *Env, subject *corev1.Pod, reason, marker string) {
	t.Helper()
	now := metav1.Now()
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "e2e-drain-",
			Namespace:    subject.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Namespace: subject.Namespace, Name: subject.Name, UID: subject.UID,
		},
		Reason:         reason,
		Message:        "e2e drain-awareness marker " + marker,
		Type:           corev1.EventTypeWarning,
		Count:          1,
		FirstTimestamp: now,
		LastTimestamp:  now,
	}
	if err := e.K.Create(ctx, ev); err != nil {
		t.Fatalf("posting synthetic event: %v", err)
	}
}

// conversationMentioning reports whether any events-lane conversation carries
// the marker. It has to check THREE places: `spec.inputs[]` is the work
// queue, pruned the moment a run processes it, so a fast stub-runtime answer
// can move the very evidence being searched for into `status.runs[].inputs[]`
// (the durable record, `.Text` rather than `.Payload`) between one poll and
// the next.
func conversationMentioning(ctx context.Context, e *Env, marker string) (bool, error) {
	items, err := e.K.Conversations(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range items {
		if c.Spec.Signal == nil || c.Spec.Signal.SourceRef == nil || c.Spec.Signal.SourceRef.Name != SourceEvents {
			continue
		}
		if strings.Contains(c.Spec.Title+c.Spec.Signature, marker) {
			return true, nil
		}
		for _, in := range c.Spec.Inputs {
			if strings.Contains(in.Payload, marker) {
				return true, nil
			}
		}
		for _, run := range c.Status.Runs {
			for _, in := range run.Inputs {
				if strings.Contains(in.Text, marker) {
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
