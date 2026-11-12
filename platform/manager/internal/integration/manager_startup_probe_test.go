package integration

import (
	"strings"
	"testing"
)

// The manager serves /healthz only once its state store has opened, and on the
// nats backend that waits for the cluster to elect a leader. A liveness probe
// alone kills it mid-wait and the pod never starts, so a startup probe must
// hold liveness off for longer than the open is allowed to take.
func TestManagerHasAStartupProbeThatOutlastsTheStoreOpen(t *testing.T) {
	deploy := findDocByKindAndMetaName(helmTemplate(t), "Deployment", "agentops-manager")
	if deploy == "" {
		t.Fatal("manager Deployment not rendered")
	}
	i := strings.Index(deploy, "startupProbe:")
	if i < 0 {
		t.Fatalf("manager Deployment has no startupProbe:\n%s", deploy)
	}
	block := deploy[i:]
	for _, want := range []string{"path: /healthz", "port: 8081", "failureThreshold: 30", "periodSeconds: 10"} {
		if !strings.Contains(block, want) {
			t.Errorf("startupProbe lacks %q:\n%s", want, block)
		}
	}
}
