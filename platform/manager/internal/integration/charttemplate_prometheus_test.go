package integration

import (
	"sort"
	"strings"
	"testing"
)

// Chart-render assertions for the prometheus-bundle subchart.
//
// The bundle was renamed from vm-bundle, dropped its logs component, gained an
// alert-investigator profile and its own default-off wiring. None of that was
// covered by a chart test before — which is how two stale claims (a
// `defaultSource.profileRef` and "the Pipeline claiming it") survived in the
// bundle's spec and its documentation page while no template rendered either.
//
// Helpers (helmTemplate, helmTemplateErr, helmNotes, labelledPipelines,
// splitDocs) live in charttemplate_test.go, same package.

func promPipelines(rendered string) map[string]string {
	return labelledPipelines(rendered, "agentops-prometheus-bundle")
}

func promPipelineNames(rendered string) []string {
	var names []string
	for n := range promPipelines(rendered) {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The bundle consumes an Alertmanager and a metrics endpoint that no demo
// cluster has, so demo mode must leave it alone. `prometheus-bundle.active` has
// no demo branch precisely so this holds.
func TestPrometheusBundleIsOffUntilAskedFor(t *testing.T) {
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
				"agentops-prometheus-bundle", "alert-investigator",
				"prometheus-api", "prometheus-observability",
				"agentops-mcp-prometheus", "agentops-signal-alertmanager",
			} {
				if strings.Contains(out, needle) {
					t.Errorf("the bundle must render nothing here, found %q", needle)
				}
			}
		})
	}
}

// Helm never reports an unread values key, so without this guard the rename
// would present as a successful upgrade that rendered nothing at all. The
// message has to name the NEW key, or it sends nobody anywhere.
func TestRetiredVMBundleKeyFailsTheRender(t *testing.T) {
	out := helmTemplateErr(t, "--set", "vm-bundle.enabled=true")
	for _, needle := range []string{"vm-bundle", "prometheus-bundle"} {
		if !strings.Contains(out, needle) {
			t.Errorf("the guard must name %q:\n%s", needle, out)
		}
	}
}

// The same default-off rule k8s-bundle follows: turning the bundle on for its
// ingest lane must never silently add a route beside the install's own.
func TestPrometheusBundleShipsNoWiringUnlessAsked(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"bundle enabled", []string{"--set", "prometheus-bundle.enabled=true"}},
		{"ingest lane enabled", []string{
			"--set", "prometheus-bundle.enabled=true",
			"--set", "prometheus-bundle.alertmanager.defaultSource.enabled=true"}},
		// A Pipeline with no profile has no agent to run.
		{"wiring without a profile", []string{
			"--set", "prometheus-bundle.enabled=true",
			"--set", "prometheus-bundle.pipelines.enabled=true",
			"--set", "prometheus-bundle.profile.enabled=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := promPipelineNames(helmTemplate(t, tc.args...)); len(got) != 0 {
				t.Errorf("the bundle must render no Pipeline here, got %v", got)
			}
		})
	}
}

// The wiring flag must yield an install that ANSWERS: one route, claiming the
// bundle's own source with its own profile and BOTH halves of its tooling — a
// server without the toolset gives an agent tools it may not call.
func TestPrometheusWiringClaimsItsOwnLane(t *testing.T) {
	out := helmTemplate(t,
		"--set", "prometheus-bundle.enabled=true",
		"--set", "prometheus-bundle.alertmanager.defaultSource.enabled=true",
		"--set", "prometheus-bundle.mcp.enabled=true",
		"--set", "prometheus-bundle.mcp.url=http://mcp.example/mcp",
		"--set", "prometheus-bundle.pipelines.enabled=true")
	got := promPipelineNames(out)
	if len(got) != 1 || got[0] != "alert-triage" {
		t.Fatalf("wiring must render exactly alert-triage, got %v", got)
	}
	doc := promPipelines(out)["alert-triage"]
	for _, needle := range []string{
		"name: alert-investigator",       // the bundle's profile
		"name: alerts",                   // claims the bundle's source
		"name: prometheus-observability", // the toolset
		"name: prometheus-api",           // the MCPConfig
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("the route must bind %q:\n%s", needle, doc)
		}
	}
}

// A bundle may ship wiring only because every foreign name is values-supplied
// and omitted when unset, and every ref to its own components disappears with
// them. A ref to an object nobody rendered is how a route rots into fiction.
func TestPrometheusWiringNamesOnlyWhatWasRendered(t *testing.T) {
	base := []string{
		"--set", "prometheus-bundle.enabled=true",
		"--set", "prometheus-bundle.alertmanager.defaultSource.enabled=true",
		"--set", "prometheus-bundle.pipelines.enabled=true",
	}
	bare := promPipelines(helmTemplate(t, base...))["alert-triage"]
	for _, needle := range []string{"prometheus-observability", "prometheus-api", "mcpConfigs"} {
		if strings.Contains(bare, needle) {
			t.Errorf("with the metrics component off nothing may name %q:\n%s", needle, bare)
		}
	}
	if strings.Contains(bare, "channelRefs") {
		t.Errorf("an empty channel list must omit the key entirely:\n%s", bare)
	}
	named := helmTemplate(t, append(base, "--set", "prometheus-bundle.pipelines.channels={console}")...)
	if doc := promPipelines(named)["alert-triage"]; !strings.Contains(doc, "channelRefs:\n    - name: console") {
		t.Errorf("a named channel must reach the route:\n%s", doc)
	}
}

// Every guard whose whole value is that it fires. An MCPConfig pointing
// nowhere, a server with no backend, or a server wearing the agent's identity
// all render happily and fail later, somewhere that does not name them.
func TestPrometheusBundleGuards(t *testing.T) {
	on := []string{"--set", "prometheus-bundle.enabled=true"}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"mcp enabled with no server and no url",
			[]string{"--set", "prometheus-bundle.mcp.enabled=true"},
			"mcp.url is required"},
		{"server deployed with no backend",
			[]string{"--set", "prometheus-bundle.mcpServers.enabled=true"},
			"mcpServers.backend is required"},
		{"server wearing the runtime identity",
			[]string{
				"--set", "prometheus-bundle.mcpServers.enabled=true",
				"--set", "prometheus-bundle.mcpServers.backend=http://vm:8429",
				"--set", "prometheus-bundle.mcpServers.serviceAccountName=agentops-runtime"},
			"must differ from global.agentops.runtime.serviceAccountName"},
		{"transport the server cannot speak",
			[]string{
				"--set", "prometheus-bundle.mcp.enabled=true",
				"--set", "prometheus-bundle.mcp.url=http://mcp.example/mcp",
				"--set", "prometheus-bundle.mcp.transport=grpc"},
			"mcp.transport must be"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out := helmTemplateErr(t, append(on, tc.args...)...); !strings.Contains(out, tc.want) {
				t.Errorf("the guard must say %q:\n%s", tc.want, out)
			}
		})
	}
}

// The deployed server defaults to STDIO, and a stdio process in a pod starts,
// prints a banner and exits — a Completed pod behind a Service that never
// answers. The variable that prevents it is PROMETHEUS_MCP_SERVER_TRANSPORT;
// the shorter PROMETHEUS_MCP_TRANSPORT is silently ignored, which is how the
// first version of this bundle shipped a crash-looping server through a passing
// render test. The transport must also MATCH the path the MCPConfig advertises,
// because this server speaks one transport per process.
func TestMCPServerIsStartedInTheAdvertisedTransport(t *testing.T) {
	for _, tc := range []struct{ transport, path string }{
		{"http", "/mcp"},
		{"sse", "/sse"},
	} {
		t.Run(tc.transport, func(t *testing.T) {
			out := helmTemplate(t,
				"--set", "prometheus-bundle.enabled=true",
				"--set", "prometheus-bundle.mcp.enabled=true",
				"--set", "prometheus-bundle.mcp.transport="+tc.transport,
				"--set", "prometheus-bundle.mcpServers.enabled=true",
				"--set", "prometheus-bundle.mcpServers.backend=http://vm:8429")
			// The NAME and its VALUE asserted as one adjacent pair — checking
			// them separately is how the first version of this test passed on a
			// value that belonged to a different variable entirely.
			want := "- name: PROMETHEUS_MCP_SERVER_TRANSPORT\n              value: \"" + tc.transport + "\""
			if !strings.Contains(out, want) {
				t.Errorf("the workload must be started as %q; the short PROMETHEUS_MCP_TRANSPORT is ignored and the server falls back to stdio", want)
			}
			if !strings.Contains(out, "svc:8080"+tc.path) {
				t.Errorf("the MCPConfig must advertise %q for transport %q", tc.path, tc.transport)
			}
		})
	}
}

// The adapter CR name is the ROUTING KEY. Its default changed from
// vm-alertmanager to alertmanager in the rename, and an install restores the old
// one with a single value rather than editing every hand-written source — so the
// default source has to FOLLOW it, or the bundle ships a source pointed at an
// implementation that does not exist.
func TestSourceFollowsTheAdapterName(t *testing.T) {
	out := helmTemplate(t,
		"--set", "prometheus-bundle.enabled=true",
		"--set", "prometheus-bundle.alertmanager.defaultSource.enabled=true")
	if !strings.Contains(out, "adapter: alertmanager") {
		t.Error("the default source must name the default adapter")
	}
	out = helmTemplate(t,
		"--set", "prometheus-bundle.enabled=true",
		"--set", "prometheus-bundle.alertmanager.name=vm-alertmanager",
		"--set", "prometheus-bundle.alertmanager.defaultSource.enabled=true")
	if !strings.Contains(out, "adapter: vm-alertmanager") {
		t.Error("an overridden adapter name must reach the source's spec.adapter")
	}
}

// The notes. Vanilla Alertmanager has no object for the adapter to write, so
// the printed receiver IS the integration path — and `send_resolved: false` is
// load-bearing: the adapter drops non-firing alerts, so a sender left at its
// default posts resolutions that are silently discarded, which reads as an
// ingest fault from the sender's side.
func TestPrometheusBundleNotes(t *testing.T) {
	on := []string{
		"--set", "prometheus-bundle.enabled=true",
		"--set", "prometheus-bundle.alertmanager.defaultSource.enabled=true",
	}
	// A dry-run install adopts nothing, but helm still refuses cluster-scoped
	// objects another release owns. Nothing in these notes reads them.
	noClusterRBAC := []string{
		"--set", "k8s-bundle.eventsAdapter.rbac.create=false",
		"--set", "k8s-bundle.mcpServers.rbac.create=false",
	}
	claimsIt := []string{
		"--set", "pipelines[0].name=my-alerts",
		"--set", "pipelines[0].profile=alert-investigator",
		"--set", "pipelines[0].signalSources={alerts}",
	}
	wired := []string{"--set", "prometheus-bundle.pipelines.enabled=true"}
	for _, tc := range []struct {
		name          string
		args          []string
		want, notWant []string
	}{
		{"receiver stanza for a vanilla sender", nil,
			[]string{"webhook_configs:", "send_resolved: false", "/webhook/alerts"}, nil},
		{"a credentialed source prints its auth form",
			[]string{"--set", "prometheus-bundle.alertmanager.defaultSource.credentialsSecretRef=alert-token"},
			[]string{"authorization:", "type: Bearer", "alert-token"}, nil},
		{"registration says it is VictoriaMetrics-only",
			[]string{"--set", "prometheus-bundle.alertmanager.registration.enabled=true"},
			[]string{"VICTORIAMETRICS-ONLY", "VMAlertmanagerConfig"},
			[]string{"webhook_configs:"}},
		{"nobody claims", nil,
			[]string{"ONE STEP LEFT — nothing answers alerts yet"}, nil},
		{"the install claims", claimsIt,
			nil, []string{"ONE STEP LEFT — nothing answers alerts yet", "claimed TWICE"}},
		{"the bundle wires itself", wired,
			[]string{"this release WIRED it", "alert-triage"},
			[]string{"ONE STEP LEFT — nothing answers alerts yet", "claimed TWICE"}},
		{"both claim it — a note, never a failure",
			append(append([]string{}, wired...), claimsIt...),
			[]string{"claimed TWICE", "my-alerts"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append(append([]string{}, on...), noClusterRBAC...), tc.args...)
			out := helmNotes(t, args...)
			for _, needle := range tc.want {
				if !strings.Contains(out, needle) {
					t.Errorf("the notes must say %q", needle)
				}
			}
			for _, needle := range tc.notWant {
				if strings.Contains(out, needle) {
					t.Errorf("the notes must NOT say %q", needle)
				}
			}
		})
	}
}
