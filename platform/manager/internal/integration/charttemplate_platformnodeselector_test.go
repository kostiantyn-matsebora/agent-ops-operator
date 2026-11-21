// Chart render assertions for the platform node selector: ONE value places
// every platform pod (the ones that run outside the conversations namespace),
// and a component's own nodeSelector still wins over it. String checks on
// `helm template` output, like every other charttemplate_*_test.go file.
package integration

import (
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// every platform workload this chart renders in the release namespace, with the
// bundles that own them switched on. A component missing from this list is a
// component the global value can silently skip.
var platformWorkloadArgs = []string{
	"--set", "commandBus.backend=nats", "--set", "commandBus.nats.enabled=true",
	"--set", "kubernetes.enabled=true",
	"--set", "prometheus.enabled=true", "--set", "prometheus.mcpServers.enabled=true",
	"--set", "prometheus.mcpServers.backend=http://vm.example.svc:8429",
	"--set", "pulse.enabled=true", "--set", "pulse.mcpServer.enabled=true",
	"--set", "pulse.mcpServer.backend=https://pulse.example.org",
	"--set", "home-assistant.enabled=true", "--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
	"--set", "home-assistant.homeAssistant.credentials.operatorToken=x",
	"--set", "home-assistant.adminMcp.enabled=true", "--set", "home-assistant.adminMcpServer.enabled=true",
	"--set", "home-assistant.adminMcpServer.credentialsSecret=ha-admin",
	"--set", "telegram.enabled=true", "--set", "telegram.surface.enabled=true",
	"--set", "telegram.surface.chatId=-100123", "--set", "telegram.surface.credentials.botToken=x",
}

var platformWorkloads = []string{
	"agentops-manager", "agentops-nats", "agentops-commandbus", "agentops-gateway-telegram",
	"agentops-mcp-k8s", "agentops-mcp-prometheus", "agentops-mcp-pulse", "agentops-mcp-ha",
}

const amd64Only = `global.agentops.platformNodeSelector={"kubernetes.io/arch":"amd64"}`

// podNodeSelectors reads the nodeSelector of every Deployment and StatefulSet
// in a render, keyed by name.
func podNodeSelectors(t *testing.T, rendered string) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	for _, doc := range splitDocs(rendered) {
		var obj struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						NodeSelector map[string]string `json:"nodeSelector"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal([]byte(stripComments(doc)), &obj); err != nil || (obj.Kind != "Deployment" && obj.Kind != "StatefulSet") {
			continue
		}
		out[obj.Metadata.Name] = obj.Spec.Template.Spec.NodeSelector
	}
	return out
}

func TestPlatformNodeSelectorDefaultsToNothing(t *testing.T) {
	got := podNodeSelectors(t, helmTemplate(t, platformWorkloadArgs...))
	for _, name := range platformWorkloads {
		sel, rendered := got[name]
		if !rendered {
			t.Fatalf("%s did not render; the test's flags no longer reach it (have %v)", name, keysOfSelectors(got))
		}
		if len(sel) != 0 {
			t.Errorf("%s has nodeSelector %v with nothing set, want none", name, sel)
		}
	}
}

func TestPlatformNodeSelectorPlacesEveryPlatformWorkload(t *testing.T) {
	got := podNodeSelectors(t, helmTemplate(t, append(platformWorkloadArgs, "--set-json", amd64Only)...))
	want := map[string]string{"kubernetes.io/arch": "amd64"}
	for _, name := range platformWorkloads {
		sel, rendered := got[name]
		if !rendered {
			t.Fatalf("%s did not render (have %v)", name, keysOfSelectors(got))
		}
		if !reflect.DeepEqual(sel, want) {
			t.Errorf("%s nodeSelector = %v, want %v", name, sel, want)
		}
	}
}

// A component that names its own placement is not overridden by the global one,
// and the others are unaffected.
func TestAComponentsOwnNodeSelectorWinsOverThePlatformOne(t *testing.T) {
	args := append(append([]string{}, platformWorkloadArgs...), "--set-json", amd64Only,
		"--set", "nodeSelector.disk=ssd",
		"--set", "kubernetes.mcpServers.nodeSelector.disk=nvme")
	got := podNodeSelectors(t, helmTemplate(t, args...))
	if want := map[string]string{"disk": "ssd"}; !reflect.DeepEqual(got["agentops-manager"], want) {
		t.Errorf("manager nodeSelector = %v, want its own %v", got["agentops-manager"], want)
	}
	if want := map[string]string{"disk": "nvme"}; !reflect.DeepEqual(got["agentops-mcp-k8s"], want) {
		t.Errorf("mcp-k8s nodeSelector = %v, want its own %v", got["agentops-mcp-k8s"], want)
	}
	if want := map[string]string{"kubernetes.io/arch": "amd64"}; !reflect.DeepEqual(got["agentops-nats"], want) {
		t.Errorf("nats nodeSelector = %v, want the platform one %v", got["agentops-nats"], want)
	}
}

// The conversations namespace is not the platform's: housekeeping and the
// context probe run beside the runtime pods and are placed by the runtime's
// own settings, never by this value.
func TestPlatformNodeSelectorDoesNotReachTheConversationsNamespace(t *testing.T) {
	out := helmTemplate(t, append(platformWorkloadArgs, "--set-json", amd64Only)...)
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "namespace: agent-ops-conversations") && strings.Contains(doc, "kubernetes.io/arch") &&
			(strings.Contains(doc, "kind: CronJob") || strings.Contains(doc, "kind: Deployment")) {
			t.Errorf("a conversations-namespace workload picked up the platform nodeSelector:\n%s", stripComments(doc))
		}
	}
}

func keysOfSelectors(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
