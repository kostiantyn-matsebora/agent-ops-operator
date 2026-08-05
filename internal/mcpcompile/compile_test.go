package mcpcompile

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

func TestCompileMergeAndSecrets(t *testing.T) {
	refs := map[string]agentopsv1alpha1.MCPConfigSpec{
		"obs": {Servers: map[string]agentopsv1alpha1.MCPServer{
			"victorialogs": {Type: "sse", URL: "http://vl/sse"},
			"override-me":  {Type: "sse", URL: "http://old"},
		}},
	}
	spec := &agentopsv1alpha1.MCPSpec{
		ConfigRefs: []agentopsv1alpha1.ObjectRef{{Name: "obs"}},
		Servers: map[string]agentopsv1alpha1.MCPServer{
			"override-me": {Type: "sse", URL: "http://new"},
			"homeassistant": {Type: "sse", URL: "http://ha/sse", Headers: []agentopsv1alpha1.NamedValue{
				{Name: "Authorization", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "ha"}, Key: "bearer",
					},
				}},
			}},
		},
	}
	res, err := Compile(spec, refs)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(res.JSON), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.McpServers) != 3 {
		t.Fatalf("want 3 servers, got %d", len(doc.McpServers))
	}
	if doc.McpServers["override-me"]["url"] != "http://new" {
		t.Fatalf("inline must override ref: %v", doc.McpServers["override-me"])
	}
	// secret header became a placeholder + env var, no secret material in JSON
	if !strings.Contains(res.JSON, "${MCP_HOMEASSISTANT_AUTHORIZATION}") {
		t.Fatalf("placeholder missing:\n%s", res.JSON)
	}
	if len(res.Env) != 1 || res.Env[0].ValueFrom == nil || res.Env[0].ValueFrom.SecretKeyRef == nil {
		t.Fatalf("env valueFrom missing: %+v", res.Env)
	}
}

func TestCompileRawRefWins(t *testing.T) {
	spec := &agentopsv1alpha1.MCPSpec{
		ConfigMapRef: &agentopsv1alpha1.ObjectRef{Name: "raw-mcp"},
		Servers:      map[string]agentopsv1alpha1.MCPServer{"x": {Type: "sse", URL: "u"}},
	}
	res, err := Compile(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RawConfigMap != "raw-mcp" || res.JSON != "" {
		t.Fatalf("raw ref must win: %+v", res)
	}
}

func TestCompileNil(t *testing.T) {
	res, err := Compile(nil, nil)
	if err != nil || !strings.Contains(res.JSON, "mcpServers") {
		t.Fatalf("nil spec: %+v %v", res, err)
	}
}
