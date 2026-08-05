// Package mcpcompile renders an AgentProfile's tri-form MCP configuration into
// an mcp.json document plus the pod env vars that resolve its secrets.
//
// The manager NEVER reads secret material: header values with valueFrom become
// "${ENV_NAME}" placeholders in mcp.json, and the matching env var (with the
// original valueFrom source) is added to the runtime pod spec — resolution
// happens in the kubelet.
package mcpcompile

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

var envSafe = regexp.MustCompile(`[^A-Z0-9_]`)

func envName(server, header string) string {
	s := strings.ToUpper(server + "_" + header)
	return "MCP_" + envSafe.ReplaceAllString(s, "_")
}

// Result of a compilation.
type Result struct {
	// JSON is the rendered mcp.json ("" when the profile mounts a raw ref).
	JSON string
	// Env are pod env vars carrying the secret-backed values.
	Env []corev1.EnvVar
	// RawConfigMap / RawSecret request mounting the raw object as mcp.json.
	RawConfigMap string
	RawSecret    string
}

// Compile merges the tri-form spec. Merge order: configRefs (in order) then
// inline servers override by name. A raw ConfigMap/Secret ref is exclusive —
// it wins entirely and is mounted as-is (escape hatch).
func Compile(spec *agentopsv1alpha1.MCPSpec, refs map[string]agentopsv1alpha1.MCPConfigSpec) (Result, error) {
	if spec == nil {
		return Result{JSON: `{"mcpServers":{}}`}, nil
	}
	if spec.ConfigMapRef != nil {
		return Result{RawConfigMap: spec.ConfigMapRef.Name}, nil
	}
	if spec.SecretRef != nil {
		return Result{RawSecret: spec.SecretRef.Name}, nil
	}

	merged := map[string]agentopsv1alpha1.MCPServer{}
	for _, ref := range spec.ConfigRefs {
		cfg, ok := refs[ref.Name]
		if !ok {
			return Result{}, fmt.Errorf("MCPConfig %q not found", ref.Name)
		}
		for name, srv := range cfg.Servers {
			merged[name] = srv
		}
	}
	for name, srv := range spec.Servers {
		merged[name] = srv
	}

	type jsonServer struct {
		Type    string            `json:"type,omitempty"`
		URL     string            `json:"url,omitempty"`
		Command string            `json:"command,omitempty"`
		Args    []string          `json:"args,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}
	out := map[string]jsonServer{}
	var env []corev1.EnvVar

	for name, srv := range merged {
		js := jsonServer{Type: srv.Type, URL: srv.URL, Command: srv.Command, Args: srv.Args}
		if len(srv.Headers) > 0 {
			js.Headers = map[string]string{}
			for _, h := range srv.Headers {
				if h.ValueFrom != nil {
					en := envName(name, h.Name)
					js.Headers[h.Name] = "${" + en + "}"
					env = append(env, corev1.EnvVar{Name: en, ValueFrom: h.ValueFrom})
				} else {
					js.Headers[h.Name] = h.Value
				}
			}
		}
		if len(srv.Env) > 0 {
			js.Env = map[string]string{}
			for _, e := range srv.Env {
				if e.ValueFrom != nil {
					en := envName(name, e.Name)
					js.Env[e.Name] = "${" + en + "}"
					env = append(env, corev1.EnvVar{Name: en, ValueFrom: e.ValueFrom})
				} else {
					js.Env[e.Name] = e.Value
				}
			}
		}
		out[name] = js
	}

	b, err := json.MarshalIndent(map[string]any{"mcpServers": out}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	return Result{JSON: string(b), Env: env}, nil
}
