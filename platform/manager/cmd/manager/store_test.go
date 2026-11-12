package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func TestNewStateStoresRefusesAnUnknownBackend(t *testing.T) {
	if _, err := newStateStores("nosuch", nil, "agent-ops"); err == nil || !strings.Contains(err.Error(), `"nosuch"`) {
		t.Fatalf("want an error naming nosuch, got %v", err)
	}
}

func TestPodVersionReadsTheVersionEnv(t *testing.T) {
	pod := corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Name: "a", Env: []corev1.EnvVar{{Name: "OTHER", Value: "x"}}},
		{Name: "b", Env: []corev1.EnvVar{{Name: "VERSION", Value: "1.2.3"}}},
	}}}
	if got := podVersion(pod); got != "1.2.3" {
		t.Fatalf("got %q, want 1.2.3", got)
	}
	if got := podVersion(corev1.Pod{}); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func managerPod(name, version string, deleting bool) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "agent-ops",
			Labels: map[string]string{"app.kubernetes.io/name": "agentops-manager"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "m", Env: []corev1.EnvVar{{Name: "VERSION", Value: version}}},
		}},
	}
	if deleting {
		now := metav1.Now()
		p.DeletionTimestamp = &now
		p.Finalizers = []string{"test/hold"}
	}
	return p
}

func TestOldManagerReplicaPresent(t *testing.T) {
	for _, tc := range []struct {
		name string
		pods []client.Object
		want bool
	}{
		{"none", nil, false},
		{"same version", []client.Object{managerPod("a", "v2", false)}, false},
		{"older version", []client.Object{managerPod("a", "v2", false), managerPod("b", "v1", false)}, true},
		{"older but terminating", []client.Object{managerPod("b", "v1", true)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithObjects(tc.pods...).Build()
			got, err := oldManagerReplicaPresent(c, "agent-ops", "v2")(context.Background())
			if err != nil || got != tc.want {
				t.Fatalf("got (%v, %v), want %v", got, err, tc.want)
			}
		})
	}
}

func TestKubeSourceNamesListsSignalSourcesAndAdapterState(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	src := &agentopsv1alpha1.SignalSource{ObjectMeta: metav1.ObjectMeta{Name: "cron-a", Namespace: "agent-ops"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(src).Build()
	k := kubeSourceNames{c: c, namespace: "agent-ops"}

	names, err := k.SignalSourceNames(context.Background())
	if err != nil || len(names) != 1 || names[0] != "cron-a" {
		t.Fatalf("got (%v, %v), want [cron-a]", names, err)
	}
	if _, err := k.AdapterStateEntries(context.Background()); err != nil {
		t.Fatalf("AdapterStateEntries: %v", err)
	}
}

func TestStateBackendDefaultsToKube(t *testing.T) {
	for _, tc := range []struct{ name, val string }{
		{"unset", ""},
		{"blank", "  "},
		{"explicit", "kube"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(stateBackendEnv, tc.val)
			got, err := stateBackendFromEnv()
			if err != nil || got != backendKube {
				t.Fatalf("got (%q, %v), want the kube backend", got, err)
			}
		})
	}
}

func TestStateBackendRefusesAnUnknownNameAndNamesIt(t *testing.T) {
	t.Setenv(stateBackendEnv, "nosuch")
	got, err := stateBackendFromEnv()
	if err == nil || !strings.Contains(err.Error(), `"nosuch"`) {
		t.Fatalf("got (%q, %v), want an error naming nosuch", got, err)
	}
}

func TestStateBackendAcceptsNats(t *testing.T) {
	t.Setenv(stateBackendEnv, "nats")
	got, err := stateBackendFromEnv()
	if err != nil || got != backendNats {
		t.Fatalf("got (%q, %v), want the nats backend", got, err)
	}
}

// newNatsStores must fail fast, before connecting to anything, when the
// nats backend is selected with no NATS_URL — the same "stops before it
// connects" posture stateBackendFromEnv itself has.
func TestNewNatsStoresRequiresNatsURL(t *testing.T) {
	t.Setenv("NATS_URL", "")
	if _, err := newNatsStores(nil, "agent-ops"); err == nil {
		t.Fatal("want an error with NATS_URL unset")
	}
}

// runMainEnv switches TestMainExitsOnAnUnknownStateBackend's re-executed copy
// of the test binary into running main itself.
const runMainEnv = "AGENTOPS_TEST_RUN_MAIN"

// main must stop the process, not log and carry on, when the backend is one it
// does not carry. Proved on a real process: the test binary re-executes itself
// as the manager, with no cluster reachable, which also shows the check runs
// before anything connects to one.
func TestMainExitsOnAnUnknownStateBackend(t *testing.T) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExitsOnAnUnknownStateBackend$")
	cmd.Env = append(os.Environ(), runMainEnv+"=1", stateBackendEnv+"=nosuch")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("want a non-zero exit, got %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nosuch") {
		t.Fatalf("the refusal must name the value:\n%s", out)
	}
}
