package mcpcompile

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func cfg(servers map[string]agentopsv1alpha1.MCPServer) agentopsv1alpha1.MCPConfigSpec {
	return agentopsv1alpha1.MCPConfigSpec{Servers: servers}
}

func servers(t *testing.T, res Result) map[string]map[string]any {
	t.Helper()
	var doc struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(res.JSON), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", res.JSON, err)
	}
	return doc.McpServers
}

func TestCompileMergesConfigsInRefOrder(t *testing.T) {
	res, err := Compile([]agentopsv1alpha1.MCPConfigSpec{
		cfg(map[string]agentopsv1alpha1.MCPServer{
			"victorialogs": {Type: "sse", URL: "http://vl"},
			"clashes":      {Type: "sse", URL: "http://first"},
		}),
		cfg(map[string]agentopsv1alpha1.MCPServer{
			"victoriametrics": {Type: "sse", URL: "http://vm"},
			"clashes":         {Type: "sse", URL: "http://last"},
		}),
	}, []string{"vm-logs", "vm-metrics"})
	if err != nil {
		t.Fatal(err)
	}
	got := servers(t, res)
	if len(got) != 3 {
		t.Fatalf("want 3 servers, got %d: %v", len(got), got)
	}
	if got["clashes"]["url"] != "http://last" {
		t.Fatalf("later ref must win a server-key collision: %v", got["clashes"])
	}
}

// A conversation whose wiring binds no MCP gets an empty document — not an
// error, and not a mount of somebody else's servers.
func TestCompileNoConfigsIsEmptyDocument(t *testing.T) {
	res, err := Compile(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers(t, res)) != 0 {
		t.Fatalf("no bindings must compile to no servers: %s", res.JSON)
	}
}

// Secret-backed headers never appear in mcp.json: they become env placeholders
// the kubelet resolves, so the manager reads no Secrets.
func TestCompilePreservesValueFrom(t *testing.T) {
	res, err := Compile([]agentopsv1alpha1.MCPConfigSpec{
		cfg(map[string]agentopsv1alpha1.MCPServer{
			"vmmetrics": {Type: "sse", URL: "http://vm", Headers: []agentopsv1alpha1.NamedValue{
				{Name: "Authorization", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "vm"}, Key: "token",
					},
				}},
			}},
		}),
	}, []string{"vm-metrics"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.JSON, "${MCP_VMMETRICS_AUTHORIZATION}") {
		t.Fatalf("placeholder missing:\n%s", res.JSON)
	}
	if strings.Contains(res.JSON, "secretKeyRef") {
		t.Fatalf("secret reference leaked into mcp.json:\n%s", res.JSON)
	}
	if len(res.Env) != 1 || res.Env[0].ValueFrom.SecretKeyRef.Key != "token" {
		t.Fatalf("env valueFrom missing: %+v", res.Env)
	}
}

// The raw escape hatch: a hand-written mcp.json is mounted as-is when bound
// alone.
func TestCompileRawConfigAlone(t *testing.T) {
	res, err := Compile([]agentopsv1alpha1.MCPConfigSpec{
		{ConfigMapRef: &agentopsv1alpha1.ObjectRef{Name: "raw-mcp"}},
	}, []string{"hand-written"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RawConfigMap != "raw-mcp" || res.JSON != "" {
		t.Fatalf("raw config must be mounted as-is: %+v", res)
	}

	res, err = Compile([]agentopsv1alpha1.MCPConfigSpec{
		{SecretRef: &agentopsv1alpha1.ObjectRef{Name: "raw-secret"}},
	}, []string{"hand-written"})
	if err != nil || res.RawSecret != "raw-secret" {
		t.Fatalf("raw secret form: %+v %v", res, err)
	}
}

// ...but it cannot be composed, because its content is opaque to us. Refusing
// beats mounting one side and dropping the other.
func TestCompileRawConfigIsExclusive(t *testing.T) {
	_, err := Compile([]agentopsv1alpha1.MCPConfigSpec{
		{ConfigMapRef: &agentopsv1alpha1.ObjectRef{Name: "raw-mcp"}},
		cfg(map[string]agentopsv1alpha1.MCPServer{"vmlogs": {Type: "sse", URL: "http://vl"}}),
	}, []string{"hand-written", "vm-logs"})

	var exclusive *RawExclusiveError
	if !errors.As(err, &exclusive) {
		t.Fatalf("want *RawExclusiveError, got %v", err)
	}
	if exclusive.Name != "hand-written" || exclusive.Others != 1 {
		t.Fatalf("error must name the raw config and count the others: %+v", exclusive)
	}
	if !strings.Contains(err.Error(), "hand-written") {
		t.Fatalf("message must name the offender: %v", err)
	}
}
