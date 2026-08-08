package mcpcompile

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// baseProfileSpec is a profile MCP in the mergeable (non-raw) forms: one
// server via a configRef, one inline.
func baseProfileSpec() (*agentopsv1alpha1.MCPSpec, map[string]agentopsv1alpha1.MCPConfigSpec) {
	refs := map[string]agentopsv1alpha1.MCPConfigSpec{
		"profile-cfg": {Servers: map[string]agentopsv1alpha1.MCPServer{
			"fromref": {Type: "sse", URL: "http://ref"},
		}},
	}
	spec := &agentopsv1alpha1.MCPSpec{
		ConfigRefs: []agentopsv1alpha1.ObjectRef{{Name: "profile-cfg"}},
		Servers: map[string]agentopsv1alpha1.MCPServer{
			"inline":  {Type: "sse", URL: "http://inline"},
			"clashes": {Type: "sse", URL: "http://profile-wins-without-overlay"},
		},
	}
	return spec, refs
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

func TestOverlayMergeExtendsProfile(t *testing.T) {
	spec, refs := baseProfileSpec()
	overlays := []agentopsv1alpha1.MCPConfigSpec{
		{Servers: map[string]agentopsv1alpha1.MCPServer{"vmlogs": {Type: "sse", URL: "http://vl"}}},
	}
	res, err := CompileOverlaid(spec, refs, overlays, agentopsv1alpha1.ToolingMerge)
	if err != nil {
		t.Fatal(err)
	}
	got := servers(t, res)
	for _, want := range []string{"fromref", "inline", "clashes", "vmlogs"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("merge must keep the profile's servers and add the bound ones, missing %q: %v", want, got)
		}
	}
}

// The bound config wins on a server-key collision — same direction as the
// profile's own "inline overrides configRefs" precedence.
func TestOverlayMergeLetsBoundConfigWinCollisions(t *testing.T) {
	spec, refs := baseProfileSpec()
	overlays := []agentopsv1alpha1.MCPConfigSpec{
		{Servers: map[string]agentopsv1alpha1.MCPServer{"clashes": {Type: "sse", URL: "http://first-overlay"}}},
		{Servers: map[string]agentopsv1alpha1.MCPServer{"clashes": {Type: "sse", URL: "http://last-overlay"}}},
	}
	res, err := CompileOverlaid(spec, refs, overlays, agentopsv1alpha1.ToolingMerge)
	if err != nil {
		t.Fatal(err)
	}
	if url := servers(t, res)["clashes"]["url"]; url != "http://last-overlay" {
		t.Fatalf("later overlay must win: %v", url)
	}
}

func TestOverlayOverwriteDropsProfileServers(t *testing.T) {
	spec, refs := baseProfileSpec()
	overlays := []agentopsv1alpha1.MCPConfigSpec{
		{Servers: map[string]agentopsv1alpha1.MCPServer{"vmlogs": {Type: "sse", URL: "http://vl"}}},
	}
	res, err := CompileOverlaid(spec, refs, overlays, agentopsv1alpha1.ToolingOverwrite)
	if err != nil {
		t.Fatal(err)
	}
	got := servers(t, res)
	if len(got) != 1 || got["vmlogs"] == nil {
		t.Fatalf("overwrite must yield only the bound servers: %v", got)
	}
}

// A raw profile MCP is an opaque document — merging onto it is refused rather
// than half-applied, and the message says which form is in the way.
func TestOverlayRawFormRefusesMerge(t *testing.T) {
	spec := &agentopsv1alpha1.MCPSpec{ConfigMapRef: &agentopsv1alpha1.ObjectRef{Name: "raw-mcp"}}
	_, err := CompileOverlaid(spec, nil, []agentopsv1alpha1.MCPConfigSpec{{}}, agentopsv1alpha1.ToolingMerge)
	var rawErr *RawMergeError
	if !errors.As(err, &rawErr) {
		t.Fatalf("want *RawMergeError, got %v", err)
	}
	if rawErr.Kind != "configMapRef" || rawErr.Name != "raw-mcp" || !strings.Contains(err.Error(), "raw-mcp") {
		t.Fatalf("error must name the incompatible form: %+v / %v", rawErr, err)
	}

	spec = &agentopsv1alpha1.MCPSpec{SecretRef: &agentopsv1alpha1.ObjectRef{Name: "raw-secret"}}
	_, err = CompileOverlaid(spec, nil, nil, agentopsv1alpha1.ToolingMerge)
	if !errors.As(err, &rawErr) || rawErr.Kind != "secretRef" {
		t.Fatalf("secretRef form must be refused too: %v", err)
	}
}

// Overwrite ignores the profile's MCP entirely, so the raw form is no obstacle.
func TestOverlayOverwriteWorksOverRawForm(t *testing.T) {
	spec := &agentopsv1alpha1.MCPSpec{ConfigMapRef: &agentopsv1alpha1.ObjectRef{Name: "raw-mcp"}}
	overlays := []agentopsv1alpha1.MCPConfigSpec{
		{Servers: map[string]agentopsv1alpha1.MCPServer{"vmlogs": {Type: "sse", URL: "http://vl"}}},
	}
	res, err := CompileOverlaid(spec, nil, overlays, agentopsv1alpha1.ToolingOverwrite)
	if err != nil {
		t.Fatal(err)
	}
	if res.RawConfigMap != "" || servers(t, res)["vmlogs"] == nil {
		t.Fatalf("overwrite must render the bound servers, not the raw ref: %+v", res)
	}
}

// Secret-backed headers on a BOUND config keep compiling to env placeholders:
// the manager still reads no Secrets on the wiring path either.
func TestOverlayPreservesValueFrom(t *testing.T) {
	overlays := []agentopsv1alpha1.MCPConfigSpec{{Servers: map[string]agentopsv1alpha1.MCPServer{
		"vmmetrics": {Type: "sse", URL: "http://vm", Headers: []agentopsv1alpha1.NamedValue{
			{Name: "Authorization", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "vm"}, Key: "token",
				},
			}},
		}},
	}}}
	res, err := CompileOverlaid(nil, nil, overlays, agentopsv1alpha1.ToolingMerge)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.JSON, "${MCP_VMMETRICS_AUTHORIZATION}") {
		t.Fatalf("placeholder missing:\n%s", res.JSON)
	}
	if strings.Contains(res.JSON, "token") && strings.Contains(res.JSON, "secretKeyRef") {
		t.Fatalf("secret reference leaked into mcp.json:\n%s", res.JSON)
	}
	if len(res.Env) != 1 || res.Env[0].ValueFrom.SecretKeyRef.Key != "token" {
		t.Fatalf("env valueFrom missing: %+v", res.Env)
	}
}

// The compile path with no binding at all must stay byte-identical to Compile.
func TestOverlayWithoutOverlaysMatchesCompile(t *testing.T) {
	spec, refs := baseProfileSpec()
	want, err := Compile(spec, refs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := CompileOverlaid(spec, refs, nil, agentopsv1alpha1.ToolingMerge)
	if err != nil {
		t.Fatal(err)
	}
	if got.JSON != want.JSON {
		t.Fatalf("merge with no overlays must equal a plain compile:\n%s\n---\n%s", got.JSON, want.JSON)
	}
}
