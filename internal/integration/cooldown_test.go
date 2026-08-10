package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// Cooldown used to live only in the manager's memory, with a comment accepting
// that "restart resets the window, which only risks one duplicate
// investigation". During an incident — a flapping alert and a rolling manager —
// that is not one duplicate. These tests pin the record on the SignalSource.
//
// A "restart" is a fresh httpapi.Server: same objects, empty cooldown map.

func loadSource(t *testing.T, name string) *agentopsv1alpha1.SignalSource {
	t.Helper()
	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: name}, &src); err != nil {
		t.Fatal(err)
	}
	return &src
}

func countConversations(t *testing.T, profile string) int {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == profile {
			n++
			name := list.Items[i].Name
			t.Cleanup(func() { cleanupConversation(t, name) })
		}
	}
	return n
}

func TestCooldownIsRecordedAndSurvivesARestart(t *testing.T) {
	mkProfile(t, "prof-cool")
	mkSignalSource(t, "src-cool", "am-cool", "")
	mkPipeline(t, "cool-pipe", []string{"src-cool"}, nil, "prof-cool")
	reconcilePipeline(t, "cool-pipe")

	before := apiServer()
	rec := postSignal(t, before.Handler(), testMasterToken, "src-cool", []map[string]any{{
		"fingerprint": "cool-1", "labels": map[string]string{"alertname": "Flapping", "namespace": "prod"},
		"payload": "first sighting",
	}})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	if n := countConversations(t, "prof-cool"); n != 1 {
		t.Fatalf("want one conversation, got %d", n)
	}

	src := loadSource(t, "src-cool")
	if len(src.Status.Cooldown) != 1 || src.Status.Cooldown[0].Fingerprint != "cool-1" {
		t.Fatalf("an admitted fingerprint must be recorded on the source, got %+v", src.Status.Cooldown)
	}
	if src.Status.Cooldown[0].At.IsZero() {
		t.Fatal("a recorded suppression needs its timestamp — the window runs from it")
	}

	// A suppressed re-delivery must not write: this is the high-volume path,
	// and the whole design rests on it staying free.
	recorded := loadSource(t, "src-cool").ResourceVersion
	rec = postSignal(t, before.Handler(), testMasterToken, "src-cool", []map[string]any{{
		"fingerprint": "cool-1", "payload": "same alert again",
	}})
	if rec.Code != 200 {
		t.Fatalf("re-delivery: %d %s", rec.Code, rec.Body.String())
	}
	if got := loadSource(t, "src-cool").ResourceVersion; got != recorded {
		t.Fatalf("a suppressed signal wrote to the source (%s -> %s)", recorded, got)
	}

	// Restart: the new process rebuilds the window by reading the source.
	after := apiServer()
	rec = postSignal(t, after.Handler(), testMasterToken, "src-cool", []map[string]any{{
		"fingerprint": "cool-1", "payload": "still firing after the restart",
	}})
	if rec.Code != 200 {
		t.Fatalf("post-restart delivery: %d %s", rec.Code, rec.Body.String())
	}
	if n := countConversations(t, "prof-cool"); n != 1 {
		t.Fatalf("a restart must not re-open a suppressed conversation, got %d", n)
	}
}
