package integration

import (
	"sort"
	"strings"
	"testing"
)

// Chart-render assertions for the pulse subchart (pulse-integration-bundle):
// the Pulse webhook signal lane, the first-party mcp-pulse server with its
// two risk-split MCPConfig/MCPToolset pairs, the pulse-observer/pulse-operator
// AgentProfiles, and the bundle's own opt-in wiring in both Pipeline and
// Coordinator mode.
//
// Helpers (helmTemplate, helmTemplateErr, splitDocs, stripComments,
// labelledPipelines, findDocByKindAndName) live in charttemplate_test.go,
// same package.

func pulsePipelines(rendered string) map[string]string {
	return labelledPipelines(rendered, "agentops-pulse")
}

func pulsePipelineNames(rendered string) []string {
	var names []string
	for n := range pulsePipelines(rendered) {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// pulseEnabledArgs is the smallest enablement that renders every component:
// the ingest lane with a base URL, the deployed mcp-pulse server, and both
// MCP pairs' tokens.
func pulseEnabledArgs(extra ...string) []string {
	return append([]string{
		"--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org",
		"--set", "pulse.signal.credentials.token=ingest-tok",
		"--set", "pulse.mcpServer.enabled=true",
		"--set", "pulse.mcp.observer.credentials.token=observer-tok",
		"--set", "pulse.mcp.operator.credentials.token=operator-tok",
	}, extra...)
}

// The bundle consumes a Pulse instance no demo cluster runs, so demo mode
// must leave it alone — `pulse.active` has no demo branch precisely so this
// holds.
func TestPulseBundleIsOffByDefaultAndUnderDemo(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"default install", nil},
		{"demo mode", []string{"--set", "global.demo.enabled=true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmTemplate(t, tc.args...)
			for _, needle := range []string{
				"agentops-pulse", "pulse-observer", "pulse-operator",
				"pulse-observer-mcp", "pulse-operator-mcp",
				"pulse-observability", "pulse-admin",
				"agentops-mcp-pulse", "agentops-signal-pulse",
			} {
				if strings.Contains(out, needle) {
					t.Errorf("the bundle must render nothing here, found %q", needle)
				}
			}
		})
	}
}

// Enabling the bundle alone — no ingest lane, no MCP pairs, no wiring — must
// still render both AgentProfiles (identity only) and nothing else: no
// SignalAdapter claim, no Pipeline, no AgentCapability, no route
// ServiceAccount, no workload.
func TestPulseBundleEnabledAloneAddsNoRoute(t *testing.T) {
	out := helmTemplate(t, "--set", "pulse.enabled=true")
	if got := pulsePipelineNames(out); len(got) != 0 {
		t.Errorf("the bundle must render no Pipeline here, got %v", got)
	}
	for _, needle := range []string{"kind: AgentCapability", "kind: Deployment", "kind: MCPConfig"} {
		for _, doc := range splitDocs(out) {
			if strings.Contains(doc, "agentops-pulse") && strings.Contains(doc, needle) {
				t.Errorf("enabling the bundle alone must not render %q:\n%s", needle, doc)
			}
		}
	}
	findDocByKindAndName(t, out, "AgentProfile", "pulse-observer")
	findDocByKindAndName(t, out, "AgentProfile", "pulse-operator")
}

// Profiles carry IDENTITY ONLY. Capabilities are wiring's job, and unlike
// home-assistant's profiles, neither carries connectivity env either — the
// MCP credential lives on the MCPConfig, never on the profile.
func TestPulseProfilesCarryNoCapabilities(t *testing.T) {
	out := helmTemplate(t, "--set", "pulse.enabled=true")
	for _, name := range []string{"pulse-observer", "pulse-operator"} {
		doc := findDocByKindAndName(t, out, "AgentProfile", name)
		for _, forbidden := range []string{"allowedTools", "mcpConfigs", "toolsets", "env:"} {
			if strings.Contains(doc, forbidden) {
				t.Errorf("profile %s must carry no capabilities, found %q:\n%s", name, forbidden, doc)
			}
		}
	}
}

// The ingest lane renders without a claim when nothing — neither the wiring
// component nor the install — claims its source.
func TestPulseIngestLaneRendersWithoutAClaim(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org")
	findDocByKindAndName(t, out, "SignalAdapter", "pulse")
	findDocByKindAndName(t, out, "SignalSource", "pulse-alerts")
	if got := pulsePipelineNames(out); len(got) != 0 {
		t.Errorf("with wiring off, no Pipeline may claim the source, got %v", got)
	}
}

// Pulse commonly runs outside the cluster, so the webhook delivery URL
// registered WITH Pulse must be overridable rather than always the derived
// in-cluster Service DNS name — the adapter reads spec.config.webhookUrl
// (signals/pulse/register.go), and the chart's job is only to carry the
// value through, never to render an Ingress of its own.
func TestPulseWebhookURLOverrideRidesSourceConfig(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org",
		"--set", "pulse.signal.defaultSource.webhookUrl=https://pulse-adapter.example.org/webhook/pulse-alerts")
	doc := findDocByKindAndName(t, out, "SignalSource", "pulse-alerts")
	if !strings.Contains(doc, "webhookUrl: https://pulse-adapter.example.org/webhook/pulse-alerts") {
		t.Errorf("expected the override to ride spec.config.webhookUrl, got:\n%s", doc)
	}
}

// Without the override, spec.config carries no webhookUrl key at all — the
// adapter then derives the in-cluster Service URL itself, and a present but
// empty key would be a different, wrong signal to send it.
func TestPulseWebhookURLOverrideOmittedByDefault(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org")
	doc := findDocByKindAndName(t, out, "SignalSource", "pulse-alerts")
	if strings.Contains(doc, "webhookUrl") {
		t.Errorf("expected no webhookUrl key with no override set, got:\n%s", doc)
	}
}

// Two sources for one credential is ambiguous, so it fails the render rather
// than picking one — checked for all three independent credentials.
func TestPulseBothCredentialFormsIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"ingest", []string{
			"--set", "pulse.signal.credentials.existingSecret=s",
			"--set", "pulse.signal.credentials.token=t"}},
		{"observer", []string{
			"--set", "pulse.mcp.observer.credentials.existingSecret=s",
			"--set", "pulse.mcp.observer.credentials.token=t"}},
		{"operator", []string{
			"--set", "pulse.mcp.operator.credentials.existingSecret=s",
			"--set", "pulse.mcp.operator.credentials.token=t"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--set", "pulse.enabled=true"}, tc.args...)
			msg := helmTemplateErr(t, args...)
			if !strings.Contains(msg, "not both") {
				t.Fatalf("expected the ambiguity to be refused, got:\n%s", msg)
			}
		})
	}
}

// A raw token value makes the bundle create the Secret and derive BOTH keys
// the Secret needs for its own consumer: the ingest Secret carries only
// `token` (signals/pulse/register.go reads it directly off envFrom, never as
// a header), while each MCP pair's Secret ALSO derives `authorization`
// ("Bearer <token>") since a header value referencing a Secret is
// substituted whole.
func TestPulseTokenFormCreatesTheSecret(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		// The MCP component is off here: this test is about secret creation
		// alone, and with no mcp.url/mcpServer the MCPConfig would otherwise
		// fail the render for an unrelated reason (covered by
		// TestPulseMcpServerGuards).
		"--set", "pulse.mcp.observer.enabled=false",
		"--set", "pulse.mcp.operator.enabled=false",
		"--set", "pulse.signal.credentials.token=ITOK",
		"--set", "pulse.mcp.observer.credentials.token=OTOK",
		"--set", "pulse.mcp.operator.credentials.token=PTOK")

	ingest := findDocByKindAndName(t, out, "Secret", "agentops-pulse-ingest")
	if !strings.Contains(ingest, `token: "ITOK"`) {
		t.Errorf("the ingest secret must carry the raw token:\n%s", ingest)
	}
	if strings.Contains(ingest, "authorization:") {
		t.Errorf("the ingest secret is never sent as a header and must carry no derived authorization key:\n%s", ingest)
	}

	for _, tc := range []struct{ name, token string }{
		{"agentops-pulse-observer", "OTOK"},
		{"agentops-pulse-operator", "PTOK"},
	} {
		doc := findDocByKindAndName(t, out, "Secret", tc.name)
		if !strings.Contains(doc, `token: "`+tc.token+`"`) {
			t.Errorf("%s must carry the raw token:\n%s", tc.name, doc)
		}
		if !strings.Contains(doc, `authorization: "Bearer `+tc.token+`"`) {
			t.Errorf("%s must derive the complete header value:\n%s", tc.name, doc)
		}
	}
}

// An existing Secret is referenced, not duplicated — the bundle creates no
// Secret of its own when a name is supplied instead of a raw token.
func TestPulseExistingSecretIsReferencedNotDuplicated(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.mcp.observer.credentials.existingSecret=my-observer-secret",
		"--set", "pulse.mcp.observer.enabled=true",
		"--set", "pulse.mcpServer.enabled=true",
		"--set", "pulse.mcpServer.backend=https://pulse.example.org")
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: Secret\n") && strings.Contains(doc, "agentops-pulse-observer") {
			t.Fatalf("an existing secret must not be duplicated:\n%s", doc)
		}
	}
	cfg := findDocByKindAndName(t, out, "MCPConfig", "pulse-observer-mcp")
	if !strings.Contains(cfg, "name: my-observer-secret") {
		t.Errorf("the MCPConfig must bind the named secret:\n%s", cfg)
	}
}

// Enabling only the observer component renders only its pair — the operator
// pair and its toolset do not render, and no route can reference them.
func TestPulseEnablingOnlyObserverRendersOnlyItsPair(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.mcpServer.enabled=true",
		"--set", "pulse.mcpServer.backend=https://pulse.example.org",
		"--set", "pulse.mcp.observer.credentials.token=otok",
		"--set", "pulse.mcp.operator.enabled=false")
	findDocByKindAndName(t, out, "MCPConfig", "pulse-observer-mcp")
	findDocByKindAndName(t, out, "MCPToolset", "pulse-observability")
	for _, needle := range []string{"pulse-operator-mcp", "pulse-admin"} {
		if strings.Contains(out, needle) {
			t.Errorf("the operator pair must not render, found %q", needle)
		}
	}
}

// Without a credential at all, an MCP pair does not render — soft-gated
// rather than failed, mirroring home-assistant's own mcpEnabled: an install
// may enable the component ahead of supplying the token.
func TestPulseMcpPairNeedsACredential(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.mcpServer.enabled=true",
		"--set", "pulse.mcpServer.backend=https://pulse.example.org")
	for _, needle := range []string{"pulse-observer-mcp", "pulse-operator-mcp"} {
		if strings.Contains(out, needle) {
			t.Errorf("with no credential at all, no MCPConfig may render, found %q", needle)
		}
	}
}

// Tools are ENUMERATED by default, and a wildcard is refused at render time:
// mcp__pulse__* would span both the observability and admin tiers, since
// both pairs share one server key.
func TestPulseWildcardToolsAreRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"observer", []string{"--set", "pulse.mcp.observer.toolset.tools[0]=mcp__pulse__*"}},
		{"operator", []string{"--set", "pulse.mcp.operator.toolset.tools[0]=mcp__pulse__*"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{
				"--set", "pulse.enabled=true",
				"--set", "pulse.mcpServer.enabled=true",
				"--set", "pulse.mcpServer.backend=https://pulse.example.org",
				"--set", "pulse.mcp.observer.credentials.token=otok",
				"--set", "pulse.mcp.operator.credentials.token=ptok",
			}, tc.args...)
			msg := helmTemplateErr(t, args...)
			if !strings.Contains(msg, "must be enumerated, not wildcarded") {
				t.Fatalf("expected the wildcard to be refused, got:\n%s", msg)
			}
		})
	}
}

// mcp-pulse needs its own backend, with no in-cluster endpoint to default
// onto further than the ingest lane's own base URL — an MCPConfig pointing
// nowhere costs an agent its tools silently.
func TestPulseMcpServerGuards(t *testing.T) {
	on := []string{"--set", "pulse.enabled=true"}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"server deployed with no backend",
			[]string{"--set", "pulse.mcpServer.enabled=true"},
			"mcpServer.backend is required"},
		{"server wearing the runtime identity",
			[]string{
				"--set", "pulse.mcpServer.enabled=true",
				"--set", "pulse.mcpServer.backend=https://pulse.example.org",
				"--set", "pulse.mcpServer.serviceAccountName=agentops-runtime"},
			"must NOT be the runtime's ServiceAccount"},
		{"mcp pair enabled with no server and no url",
			[]string{"--set", "pulse.mcp.observer.credentials.token=otok"},
			"mcp.url is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out := helmTemplateErr(t, append(on, tc.args...)...); !strings.Contains(out, tc.want) {
				t.Errorf("the guard must say %q:\n%s", tc.want, out)
			}
		})
	}
}

// mcp-pulse's backend defaults onto the ingest lane's own base URL — the
// common case of one Pulse instance serving both lanes — and the deployed
// server mounts no ServiceAccount token when none is named.
func TestPulseMcpServerBackendDefaultsOntoIngestBaseUrl(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org",
		"--set", "pulse.mcpServer.enabled=true")
	dep := findDocByKindAndName(t, out, "Deployment", "agentops-mcp-pulse")
	if !strings.Contains(dep, `value: "https://pulse.example.org"`) {
		t.Errorf("PULSE_URL must default onto the ingest lane's base URL:\n%s", dep)
	}
	if strings.Contains(dep, "serviceAccountName:") {
		t.Errorf("a pod that mounts no token gains nothing from naming an account:\n%s", dep)
	}
	if !strings.Contains(dep, "automountServiceAccountToken: false") {
		t.Errorf("the server must mount no token:\n%s", dep)
	}
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: ServiceAccount\n") && strings.Contains(doc, "agentops-mcp-pulse") {
			t.Fatalf("no account is rendered for a workload that never authenticates:\n%s", doc)
		}
	}
}

// Both pairs name the SAME server key and the SAME deployed Service URL —
// they differ only in which Secret-backed header each supplies.
func TestPulseBothPairsShareOneServerAndDiffer(t *testing.T) {
	out := helmTemplate(t, pulseEnabledArgs()...)
	observer := findDocByKindAndName(t, out, "MCPConfig", "pulse-observer-mcp")
	operator := findDocByKindAndName(t, out, "MCPConfig", "pulse-operator-mcp")
	for _, doc := range []string{observer, operator} {
		if !strings.Contains(doc, "\n    pulse:\n") {
			t.Errorf("the server key must be pulse:\n%s", doc)
		}
		if !strings.Contains(doc, "http://agentops-mcp-pulse.") {
			t.Errorf("both pairs must point at the deployed Service:\n%s", doc)
		}
	}
	if !strings.Contains(observer, "name: agentops-pulse-observer") {
		t.Errorf("the observer pair must bind the observer secret:\n%s", observer)
	}
	if !strings.Contains(operator, "name: agentops-pulse-operator") {
		t.Errorf("the operator pair must bind the operator secret:\n%s", operator)
	}
}

// The default enablement is the smallest that renders every component — this
// test would pass vacuously if it rendered less than it claims to.
func TestPulseEnabledArgsRenderEveryComponent(t *testing.T) {
	out := helmTemplate(t, pulseEnabledArgs()...)
	for _, needle := range []string{
		"kind: SignalAdapter", "kind: SignalSource",
		"kind: AgentProfile", "kind: MCPConfig", "kind: MCPToolset",
		"kind: Deployment", "kind: Service",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("%s is not in the render — this test would pass vacuously", needle)
		}
	}
}

// Pipelines-mode wiring claims the shared source TWICE — sources are
// shareable, so both routes render without conflict — and each binds its
// own MCP pair.
func TestPulsePipelinesModeClaimsTheSharedSourceTwice(t *testing.T) {
	out := helmTemplate(t, pulseEnabledArgs("--set", "pulse.pipelines.enabled=true")...)
	got := pulsePipelineNames(out)
	sort.Strings(got)
	if want := []string{"pulse-observer", "pulse-operator"}; !equalStrings(got, want) {
		t.Fatalf("wiring must render exactly %v, got %v", want, got)
	}
	docs := pulsePipelines(out)
	for name, want := range map[string][]string{
		"pulse-observer": {"name: pulse-alerts", "name: pulse-observability", "name: pulse-observer-mcp"},
		"pulse-operator": {"name: pulse-alerts", "name: pulse-admin", "name: pulse-operator-mcp"},
	} {
		doc := docs[name]
		for _, needle := range want {
			if !strings.Contains(doc, needle) {
				t.Errorf("route %s must bind %q:\n%s", name, needle, doc)
			}
		}
	}
}

// A ref to an object nobody rendered is how a route rots into fiction — with
// the MCP components off, neither route may name a toolset or an MCPConfig,
// and an empty channel list omits the key entirely.
func TestPulseWiringNamesOnlyWhatWasRendered(t *testing.T) {
	base := []string{
		"--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org",
		"--set", "pulse.pipelines.enabled=true",
	}
	bare := pulsePipelines(helmTemplate(t, base...))["pulse-observer"]
	for _, needle := range []string{"pulse-observability", "pulse-observer-mcp", "mcpConfigs"} {
		if strings.Contains(bare, needle) {
			t.Errorf("with the MCP component off nothing may name %q:\n%s", needle, bare)
		}
	}
	if strings.Contains(bare, "channelRefs") {
		t.Errorf("an empty channel list must omit the key entirely:\n%s", bare)
	}
	named := helmTemplate(t, append(base, "--set", "pulse.pipelines.channels={console}")...)
	if doc := pulsePipelines(named)["pulse-observer"]; !strings.Contains(doc, "channelRefs:\n    - name: console") {
		t.Errorf("a named channel must reach the route:\n%s", doc)
	}
}

// Each route is independently toggleable.
func TestPulseRoutesAreIndependentlyToggleable(t *testing.T) {
	out := helmTemplate(t, pulseEnabledArgs(
		"--set", "pulse.pipelines.enabled=true",
		"--set", "pulse.pipelines.operator.enabled=false")...)
	got := pulsePipelineNames(out)
	if want := []string{"pulse-observer"}; !equalStrings(got, want) {
		t.Fatalf("disabling the operator route must leave exactly %v, got %v", want, got)
	}
}

// Coordinator-mode wiring contributes BOTH capabilities to the chart-rendered
// Coordinator's agents[] — never a Pipeline — described well enough that a
// person's question matches one as readily as an alert does.
func TestPulseCoordinatorModeContributesBoth(t *testing.T) {
	out := helmTemplate(t, pulseEnabledArgs(
		"--set", "global.agentops.wiringMode=coordinator",
		"--set", "pulse.pipelines.enabled=true")...)

	if got := pulsePipelineNames(out); len(got) != 0 {
		t.Fatalf("coordinator mode must render no Pipeline, got %v", got)
	}
	findDocByKindAndName(t, out, "AgentCapability", "pulse-observer")
	findDocByKindAndName(t, out, "AgentCapability", "pulse-operator")

	coord := findDocByKindAndName(t, out, "Coordinator", "agentops-coordinator")
	for _, needle := range []string{"name: pulse-alerts", "name: pulse-observer", "name: pulse-operator"} {
		if !strings.Contains(coord, needle) {
			t.Errorf("the Coordinator must contribute %q:\n%s", needle, coord)
		}
	}
}

// mcp-pulse's own ServiceAccount must differ from the runtime's — sharing
// one collapses two walls into one. Covered standalone (beyond
// TestPulseMcpServerGuards above) because it is the SPECIFIC scenario
// pulse-mcp-tooling's spec names by name.
func TestPulseMcpServerEqualToRuntimeServiceAccountFailsRender(t *testing.T) {
	msg := helmTemplateErr(t,
		"--set", "pulse.enabled=true",
		"--set", "pulse.mcpServer.enabled=true",
		"--set", "pulse.mcpServer.backend=https://pulse.example.org",
		"--set", "pulse.mcpServer.serviceAccountName=agentops-runtime",
		"--set", "global.agentops.runtimeDefaults.serviceAccountName=agentops-runtime")
	if !strings.Contains(msg, "must NOT be the runtime's ServiceAccount") {
		t.Fatalf("sharing the runtime identity collapses the two walls, got:\n%s", msg)
	}
}

// Neither route needs Kubernetes RBAC by default — Pulse is reached entirely
// over HTTP/MCP — so no route ServiceAccount renders unless a grant is
// declared.
func TestPulseRoutesRenderNoIdentityByDefault(t *testing.T) {
	out := helmTemplate(t, pulseEnabledArgs("--set", "pulse.pipelines.enabled=true")...)
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: ServiceAccount\n") &&
			strings.Contains(doc, "agentops-pulse") &&
			(strings.Contains(doc, "agentops-pulse-observer") || strings.Contains(doc, "agentops-pulse-operator")) &&
			!strings.Contains(doc, "kind: Secret") {
			t.Fatalf("neither route needs Kubernetes RBAC, no ServiceAccount should render:\n%s", doc)
		}
	}
}

// Both profiles' inline roles frame Pulse's governed-action flow as Pro-gated
// and out of reach, and the operator role never claims to execute a fix.
func TestPulseRolesFrameCommunityEdition(t *testing.T) {
	out := helmTemplate(t, "--set", "pulse.enabled=true")
	observer := findDocByKindAndName(t, out, "AgentProfile", "pulse-observer")
	if !strings.Contains(observer, "Pro") {
		t.Errorf("the observer role must frame the governed-action flow as Pro-gated:\n%s", observer)
	}
	operator := findDocByKindAndName(t, out, "AgentProfile", "pulse-operator")
	for _, needle := range []string{"cannot plan, approve or execute", "maintenance window", "criticality"} {
		if !strings.Contains(operator, needle) {
			t.Errorf("the operator role must say %q:\n%s", needle, operator)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- webhook exposure (Pulse outside the cluster) --------------------------

// pulseIngress returns the rendered Ingress document, failing if there is not
// exactly one.
func pulseIngress(t *testing.T, out string) string {
	t.Helper()
	var found []string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: Ingress") {
			found = append(found, doc)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 Ingress, found %d:\n%s", len(found), out)
	}
	return found[0]
}

// No Ingress unless asked for — Pulse in-cluster needs neither it nor
// webhookUrl, and the default install must stay silent about the whole
// question.
func TestPulseIngressIsOptIn(t *testing.T) {
	out := helmTemplate(t, "--set", "pulse.enabled=true",
		"--set", "pulse.signal.defaultSource.enabled=true",
		"--set", "pulse.signal.defaultSource.baseUrl=https://pulse.example.org")
	if strings.Contains(out, "kind: Ingress") {
		t.Fatal("ingress.enabled=false must render no Ingress")
	}
	// disabling the whole bundle must still render nothing, even with
	// ingress values set
	out = helmTemplate(t, "--set", "pulse.enabled=false",
		"--set", "pulse.signal.ingress.enabled=true",
		"--set", "pulse.signal.ingress.host=pulse-adapter.example.org")
	if strings.Contains(out, "kind: Ingress") {
		t.Fatal("pulse.enabled=false must render no Ingress even with ingress values set")
	}
}

// The backend is the Service the SignalAdapter reconciler owns — a Service
// this chart renders of its own would be a second, competing Service for the
// same pods.
func TestPulseIngressTargetsTheReconcilersService(t *testing.T) {
	doc := pulseIngress(t, helmTemplate(t, "--set", "pulse.enabled=true",
		"--set", "pulse.signal.ingress.enabled=true",
		"--set", "pulse.signal.ingress.host=pulse-adapter.example.org"))
	for _, want := range []string{"name: agentops-signal-pulse", "number: 8080"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("Ingress backend missing %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "kind: Service") {
		t.Fatalf("this chart must render no Service of its own for the adapter:\n%s", doc)
	}
}

// Scoped to the webhook path by default — the adapter's only other route
// (/healthz) gains nothing from external exposure.
func TestPulseIngressDefaultsToTheWebhookPath(t *testing.T) {
	doc := pulseIngress(t, helmTemplate(t, "--set", "pulse.enabled=true",
		"--set", "pulse.signal.ingress.enabled=true",
		"--set", "pulse.signal.ingress.host=pulse-adapter.example.org"))
	if !strings.Contains(doc, "path: /webhook") {
		t.Fatalf("expected the default path to be /webhook:\n%s", doc)
	}
}

// extraHosts serve the same adapter: one rule each, all to the same backend.
func TestPulseIngressExtraHosts(t *testing.T) {
	doc := pulseIngress(t, helmTemplate(t, "--set", "pulse.enabled=true",
		"--set", "pulse.signal.ingress.enabled=true",
		"--set", "pulse.signal.ingress.host=pulse-adapter.example.org",
		"--set", "pulse.signal.ingress.extraHosts={alt.example.org}"))
	for _, want := range []string{"pulse-adapter.example.org", "alt.example.org"} {
		if !strings.Contains(doc, `host: "`+want+`"`) {
			t.Fatalf("missing rule for %s:\n%s", want, doc)
		}
	}
	// 1 occurrence from metadata.name, plus one per rule's backend (2 hosts)
	if n := strings.Count(doc, "name: agentops-signal-pulse"); n != 3 {
		t.Fatalf("expected 1 metadata name + 2 backend refs, found %d:\n%s", n, doc)
	}
}

// cert-manager in one value: the annotation plus a derived Secret name.
func TestPulseIngressClusterIssuer(t *testing.T) {
	doc := pulseIngress(t, helmTemplate(t, "--set", "pulse.enabled=true",
		"--set", "pulse.signal.ingress.enabled=true",
		"--set", "pulse.signal.ingress.host=pulse-adapter.example.org",
		"--set", "pulse.signal.ingress.tls.clusterIssuer=letsencrypt"))
	if !strings.Contains(doc, "cert-manager.io/cluster-issuer: letsencrypt") {
		t.Fatalf("missing issuer annotation:\n%s", doc)
	}
	if !strings.Contains(doc, "secretName: agentops-signal-pulse-tls") {
		t.Fatalf("clusterIssuer must derive a secretName:\n%s", doc)
	}
}

// Missing host fails loudly rather than rendering an Ingress with no rule a
// hostname-less controller could ever route.
func TestPulseIngressMissingHostFailsRender(t *testing.T) {
	out := helmTemplateErr(t, "--set", "pulse.enabled=true",
		"--set", "pulse.signal.ingress.enabled=true")
	if !strings.Contains(out, "pulse.signal.ingress.host is required") {
		t.Fatalf("expected the missing-host message, got:\n%s", out)
	}
}
