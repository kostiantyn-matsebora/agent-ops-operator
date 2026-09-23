package integration

import (
	"strings"
	"testing"
)

// Every adapter a bundle ships declares the systems it faces outside the
// install, and the console draws exactly what is declared. A bundle that stops
// declaring one draws an adapter facing nothing, which reads as a fact.

func allBundles(extra ...string) []string {
	return append([]string{
		"--set", "prometheus.enabled=true",
		"--set", "kubernetes.enabled=true",
		"--set", "telegram.enabled=true",
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.controlSecret=ha-control",
		"--set", "home-assistant.homeAssistant.credentials.operatorSecret=ha-operator",
	}, extra...)
}

// externalsOf returns the rendered externals block of one adapter CR.
func externalsOf(t *testing.T, rendered, kind, name string) string {
	t.Helper()
	doc := findDocByKindAndName(t, rendered, kind, name)
	_, after, found := strings.Cut(doc, "\n  externals:\n")
	if !found {
		t.Fatalf("%s %s declares no externals:\n%s", kind, name, doc)
	}
	var block []string
	for _, line := range strings.Split(after, "\n") {
		if !strings.HasPrefix(line, "    ") {
			break
		}
		block = append(block, strings.TrimSpace(line))
	}
	return strings.Join(block, "\n")
}

func TestEveryShippedAdapterDeclaresItsExternals(t *testing.T) {
	out := helmTemplate(t, allBundles("--set", "prometheus.alertmanager.registration.enabled=true")...)
	for _, tc := range []struct {
		kind, name string
		want       []string
	}{
		{"SignalAdapter", "alertmanager", []string{"- name: Alertmanager\nkind: sender", "- name: Kubernetes API\nkind: kubernetes"}},
		{"SignalAdapter", "k8s-events", []string{"- name: Kubernetes API\nkind: kubernetes"}},
		{"SignalAdapter", "home-assistant", []string{"- name: Home Assistant\nkind: api"}},
		{"ChannelAdapter", "telegram", []string{"- name: Telegram Bot API\nkind: api"}},
		{"ChannelAdapter", "console", []string{"- name: Browser\nkind: sender", "- name: Kubernetes API\nkind: kubernetes"}},
	} {
		got := externalsOf(t, out, tc.kind, tc.name)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s %s: missing %q in\n%s", tc.kind, tc.name, w, got)
			}
		}
	}
}

// Only what the adapter really faces: alertmanager reaches the Kubernetes API
// only to self-register, and the console reaches a metrics endpoint only when
// one is configured.
func TestConditionalExternalsFollowTheirValues(t *testing.T) {
	out := helmTemplate(t, allBundles()...)
	if got := externalsOf(t, out, "SignalAdapter", "alertmanager"); strings.Contains(got, "Kubernetes API") {
		t.Errorf("without registration the alertmanager adapter reaches no Kubernetes API:\n%s", got)
	}
	if got := externalsOf(t, out, "ChannelAdapter", "console"); strings.Contains(got, "Metrics endpoint") {
		t.Errorf("no metrics endpoint is configured:\n%s", got)
	}
	withMetrics := helmTemplate(t, "--set", "console.metrics.url=http://metrics.example.org:8428")
	if got := externalsOf(t, withMetrics, "ChannelAdapter", "console"); !strings.Contains(got, "- name: Metrics endpoint\nkind: api") {
		t.Errorf("a configured metrics endpoint is declared:\n%s", got)
	}
}

// The origination half of the telegram and console bundles faces nothing
// outside the install: the router and the console's own pod stand between.
func TestAdaptersFacingNothingDeclareNothing(t *testing.T) {
	out := helmTemplate(t, allBundles()...)
	for _, name := range []string{"telegram", "console"} {
		if doc := findDocByKindAndName(t, out, "SignalAdapter", name); strings.Contains(doc, "externals:") {
			t.Errorf("SignalAdapter %s faces nothing outside the install:\n%s", name, doc)
		}
	}
}
