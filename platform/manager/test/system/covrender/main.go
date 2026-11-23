// Command covrender is a Helm post-renderer for the system pack's COVERAGE run.
//
// An operator built with `go build -cover` writes its coverage counters when the
// process exits, into the directory GOCOVERDIR names. Inside a pod that directory
// dies with the pod, so this adds three things to the manager's Deployment and
// touches nothing else:
//
//   - the env var GOCOVERDIR=/covdata
//   - a hostPath volume on the node's /covdata, which the harness created world
//     writable beforehand (the manager runs as a non-root user)
//   - the mount of it
//
// The harness then stops the manager gracefully, copies the directory off the node
// and turns it into a profile. A post-renderer rather than a chart value because
// this is test machinery: the chart gains no knob a user could mistake for a
// supported one, and the change survives every `helm upgrade` the pack makes.
//
// It reads the rendered manifest on stdin and writes it on stdout.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"

	"sigs.k8s.io/yaml"
)

const (
	deployment = "agentops-manager"
	container  = "manager"
	volume     = "covdata"
	dir        = "/covdata"
)

var separator = regexp.MustCompile(`(?m)^---[ \t]*$`)

func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "covrender:", err)
		os.Exit(1)
	}
	out, err := render(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "covrender:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		os.Exit(1)
	}
}

// render rewrites the manager's Deployment in a multi-document manifest.
func render(manifest []byte) ([]byte, error) {
	docs := separator.Split(string(manifest), -1)
	for i, doc := range docs {
		if !bytes.Contains([]byte(doc), []byte(deployment)) {
			continue
		}
		changed, ok, err := instrument([]byte(doc))
		if err != nil {
			return nil, err
		}
		if ok {
			docs[i] = "\n" + string(changed)
		}
	}
	var b bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			b.WriteString("---")
		}
		b.WriteString(doc)
	}
	return b.Bytes(), nil
}

// instrument adds the coverage wiring to one document, if it is the manager's
// Deployment. The second result says whether it was.
func instrument(doc []byte) ([]byte, bool, error) {
	var obj map[string]any
	if err := yaml.Unmarshal(doc, &obj); err != nil || obj == nil {
		return nil, false, nil // not a document this program owns: pass it through
	}
	meta, _ := obj["metadata"].(map[string]any)
	if obj["kind"] != "Deployment" || meta["name"] != deployment {
		return nil, false, nil
	}
	pod, err := path(obj, "spec", "template", "spec")
	if err != nil {
		return nil, false, err
	}
	containers, _ := pod["containers"].([]any)
	found := false
	for _, c := range containers {
		cm, _ := c.(map[string]any)
		if cm["name"] != container {
			continue
		}
		found = true
		cm["env"] = append(list(cm["env"]), map[string]any{"name": "GOCOVERDIR", "value": dir})
		cm["volumeMounts"] = append(list(cm["volumeMounts"]), map[string]any{"name": volume, "mountPath": dir})
	}
	if !found {
		return nil, false, fmt.Errorf("deployment %s has no container %q", deployment, container)
	}
	pod["volumes"] = append(list(pod["volumes"]), map[string]any{
		"name":     volume,
		"hostPath": map[string]any{"path": dir, "type": "Directory"},
	})
	out, err := yaml.Marshal(obj)
	return out, err == nil, err
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func path(obj map[string]any, keys ...string) (map[string]any, error) {
	cur := obj
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("manifest has no %s", k)
		}
		cur = next
	}
	return cur, nil
}
