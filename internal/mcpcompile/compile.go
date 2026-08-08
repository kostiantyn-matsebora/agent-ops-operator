// Package mcpcompile renders a conversation's bound MCPConfigs into an
// mcp.json document plus the pod env vars that resolve its secrets.
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
	// JSON is the rendered mcp.json ("" when a raw config is mounted).
	JSON string
	// Env are pod env vars carrying the secret-backed values.
	Env []corev1.EnvVar
	// RawConfigMap / RawSecret request mounting the raw object as mcp.json.
	RawConfigMap string
	RawSecret    string
}

// RawExclusiveError reports a hand-written mcp.json bound alongside other
// configs. That document is opaque to the operator, so there is nothing to
// compose it with — surfaced rather than silently dropping one side.
type RawExclusiveError struct {
	// Name is the raw config; Others counts the configs bound alongside it.
	Name   string
	Others int
}

func (e *RawExclusiveError) Error() string {
	return fmt.Sprintf("MCPConfig %q mounts a hand-written mcp.json, which is opaque to the operator "+
		"and cannot be combined with the %d other config(s) bound alongside it — bind it alone, "+
		"or move its servers into the inline `servers` form", e.Name, e.Others)
}

// Compile renders the bound configs, in ref order, into one mcp.json: server
// keys are overlaid so a later config wins a collision. names parallels configs
// by index and is used only for error messages.
//
// A raw config (configMapRef/secretRef) is mounted as-is and must be the only
// one bound; anything else returns *RawExclusiveError. No bindings at all
// compiles to an empty document — a conversation whose wiring grants no MCP
// gets no servers, which is the point.
func Compile(configs []agentopsv1alpha1.MCPConfigSpec, names []string) (Result, error) {
	if len(configs) == 0 {
		return Result{JSON: `{"mcpServers":{}}`}, nil
	}
	for i := range configs {
		if !configs[i].IsRaw() {
			continue
		}
		if len(configs) > 1 {
			return Result{}, &RawExclusiveError{Name: nameAt(names, i), Others: len(configs) - 1}
		}
		if configs[i].ConfigMapRef != nil {
			return Result{RawConfigMap: configs[i].ConfigMapRef.Name}, nil
		}
		return Result{RawSecret: configs[i].SecretRef.Name}, nil
	}

	merged := map[string]agentopsv1alpha1.MCPServer{}
	for i := range configs {
		for name, srv := range configs[i].Servers {
			merged[name] = srv
		}
	}
	return render(merged)
}

func nameAt(names []string, i int) string {
	if i < len(names) {
		return names[i]
	}
	return "<unnamed>"
}

// render turns a resolved server map into mcp.json plus the env vars carrying
// its secret-backed values.
func render(merged map[string]agentopsv1alpha1.MCPServer) (Result, error) {
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
