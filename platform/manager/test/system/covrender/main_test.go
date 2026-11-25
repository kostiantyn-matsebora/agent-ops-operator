package main

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

const manifest = `apiVersion: v1
kind: ConfigMap
metadata:
  name: agentops-manager-config
data:
  note: "mentions agentops-manager but is no deployment"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agentops-manager
spec:
  template:
    spec:
      containers:
        - name: manager
          image: m:e2e
          env:
            - name: A
              value: "1"
      volumes:
        - name: other
          emptyDir: {}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agentops-adapter-console
spec:
  template:
    spec:
      containers:
        - name: console
`

func TestOnlyTheManagersDeploymentIsInstrumented(t *testing.T) {
	out, err := render([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	docs := separator.Split(string(out), -1)
	if len(docs) != 3 {
		t.Fatalf("want 3 documents, got %d", len(docs))
	}
	if !strings.Contains(docs[0], "no deployment") || strings.Contains(docs[0], "GOCOVERDIR") {
		t.Errorf("the ConfigMap was touched:\n%s", docs[0])
	}
	if strings.Contains(docs[2], "GOCOVERDIR") || strings.Contains(docs[2], "covdata") {
		t.Errorf("another Deployment was touched:\n%s", docs[2])
	}
	var d map[string]any
	if err := yaml.Unmarshal([]byte(docs[1]), &d); err != nil {
		t.Fatal(err)
	}
	pod := d["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	c := pod["containers"].([]any)[0].(map[string]any)
	env := c["env"].([]any)
	if len(env) != 2 || env[1].(map[string]any)["name"] != "GOCOVERDIR" || env[0].(map[string]any)["name"] != "A" {
		t.Errorf("env: %v", env)
	}
	if m := c["volumeMounts"].([]any); len(m) != 1 || m[0].(map[string]any)["mountPath"] != dir {
		t.Errorf("mounts: %v", m)
	}
	vols := pod["volumes"].([]any)
	if len(vols) != 2 || vols[1].(map[string]any)["hostPath"].(map[string]any)["path"] != dir {
		t.Errorf("volumes: %v", vols)
	}
}

func TestAManifestWithoutTheManagerPassesThroughUnchanged(t *testing.T) {
	in := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n"
	out, err := render([]byte(in))
	if err != nil || string(out) != in {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestAManagerWithoutItsContainerIsRefused(t *testing.T) {
	_, err := render([]byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: agentops-manager\nspec:\n  template:\n    spec:\n      containers:\n        - name: other\n"))
	if err == nil {
		t.Fatal("a manager Deployment with no manager container must be an error, not silently uninstrumented")
	}
}

func TestInstrumentingIsAnnouncedOnlyForTheManager(t *testing.T) {
	if _, ok, _ := instrument([]byte("kind: Service\nmetadata:\n  name: agentops-manager\n")); ok {
		t.Fatal("a Service named like the manager is not instrumented")
	}
	if _, ok, _ := instrument([]byte("not: [valid")); ok {
		t.Fatal("an unreadable document passes through")
	}
}
