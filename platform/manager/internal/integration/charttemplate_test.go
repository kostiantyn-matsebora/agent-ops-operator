package integration

import (
	"encoding/base64"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Chart-render assertions for the console bundle.
//
// The properties worth pinning are the ones a template edit can quietly break:
// that disabling it renders NOTHING, that the chart ships no workload objects
// (the reconciler owns them), and that the RBAC it does ship is read-only and
// scoped to agentops.dev. Skipped when helm is not installed — CI has it.

func helmTemplate(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "test", chartDir()}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	return string(out)
}

// helmTemplateErr renders expecting FAILURE and returns the message. Used for
// the guards whose whole value is that they fire.
func helmTemplateErr(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "test", chartDir()}, args...)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helm template unexpectedly succeeded:\n%s", out)
	}
	return string(out)
}

// The console is ON by default since chart 5.0.0, so the opt-out is what needs
// pinning: ONE value must remove every console object, or the "nothing about
// your install changes" promise in docs/CHANGELOG.md is not true.
func TestConsoleRendersNothingWhenDisabled(t *testing.T) {
	out := helmTemplate(t, "--set", "console.enabled=false")
	// console-specific names only: "kind: ChannelAdapter" appears in the CRD
	// definition itself, which ships regardless
	for _, needle := range []string{"agentops-console", "agentops-adapter-console", "app.kubernetes.io/name: agentops-console"} {
		if strings.Contains(out, needle) {
			t.Fatalf("console.enabled=false must render nothing, found %q", needle)
		}
	}
}

// ...and that the default really is on, since that is the breaking half of the
// major bump.
func TestConsoleIsEnabledByDefault(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "app.kubernetes.io/name: agentops-console") {
		t.Fatal("console.enabled must default to true (chart 5.0.0)")
	}
}

func TestConsoleBundleIsCRsAndRBACOnly(t *testing.T) {
	out := helmTemplate(t, "--set", "console.enabled=true")

	for _, needle := range []string{
		"kind: ChannelAdapter",
		"serviceAccountName: agentops-adapter-console",
		"singleton: true",
		"port: 8080",
		// A VIEWER, not a transport: it renders only what it is sent, so the
		// manager must deliver it its own users' messages. Without this the
		// console shows an answer with no question above it, which is the bug
		// the per-destination delivery rule was written for.
		"echoesOwnMessages: false",
		"kind: Channel",
		"adapter: console",
		"credentialsSecretRef",
		"uiToken",
		// the origination half: an externally-served SignalAdapter plus the
		// source it originates from
		"kind: SignalAdapter",
		"servedBy:",
		"kind: SignalSource",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("console bundle missing %q", needle)
		}
	}

	// the reconciler owns the workload and the Service — a chart-shipped one
	// would make the console deployable only by this chart
	for _, doc := range splitDocs(out) {
		if !strings.Contains(doc, "agentops-adapter-console") {
			continue
		}
		for _, forbidden := range []string{"kind: Deployment", "kind: Service\n"} {
			if strings.Contains(doc, forbidden) {
				t.Fatalf("chart must not ship %s for the console:\n%s", forbidden, doc)
			}
		}
	}
}

// TWO adapter identities, and the SignalAdapter must own NO workload — that is
// the whole point of servedBy, and a chart that gave it an image would quietly
// produce the second pod this design exists to prevent.
func TestConsoleSignalAdapterIsExternallyServed(t *testing.T) {
	out := helmTemplate(t, "--set", "console.enabled=true")
	var sa string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: SignalAdapter") && strings.Contains(doc, "name: console") {
			sa = doc
		}
	}
	if sa == "" {
		t.Fatal("console SignalAdapter not rendered")
	}
	rules := stripComments(sa)
	if strings.Contains(rules, "image:") {
		t.Fatalf("an externally-served SignalAdapter must declare no image:\n%s", rules)
	}
	if !strings.Contains(rules, "kind: ChannelAdapter") || !strings.Contains(rules, "name: console") {
		t.Fatalf("servedBy must name the serving ChannelAdapter:\n%s", rules)
	}
}

// The console's RBAC is read-only. It gained pods and deployments deliberately
// (install facts exist in no CR), so the check is on VERBS and on the group set
// being exactly the three it needs — not on the absence of pods.
func TestConsoleRoleIsReadOnly(t *testing.T) {
	out := helmTemplate(t, "--set", "console.enabled=true")
	var role string
	for _, doc := range splitDocs(out) {
		// "\nkind: Role\n" and not the RoleBinding's indented roleRef.kind
		if strings.Contains(doc, "\nkind: Role\n") && strings.Contains(doc, "name: agentops-adapter-console") {
			role = doc
		}
	}
	if role == "" {
		t.Fatal("console Role not rendered")
	}
	// assert on the RULES, not the prose above them: the template's own comment
	// says the words this check forbids
	rules := stripComments(role)

	// EVERY rule is get/list/watch. The console has no write path to the
	// Kubernetes API at all, so a write verb here would grant something no code
	// in that module can use.
	for _, line := range strings.Split(rules, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "verbs:") && !strings.HasPrefix(trimmed, "- verbs:") {
			continue
		}
		if !strings.Contains(trimmed, `["get", "list", "watch"]`) {
			t.Fatalf("console Role verbs changed:\n%s", rules)
		}
	}
	for _, forbidden := range []string{"create", "update", "patch", "delete", "secrets"} {
		if strings.Contains(rules, forbidden) {
			t.Fatalf("console Role must not grant %q:\n%s", forbidden, rules)
		}
	}
	// exactly three groups: agentops.dev, apps (deployments) and core (pods)
	if n := strings.Count(rules, "apiGroups:"); n != 3 {
		t.Fatalf("console Role should cover exactly 3 API groups, found %d:\n%s", n, rules)
	}
	for _, want := range []string{`apiGroups: ["agentops.dev"]`, `apiGroups: ["apps"]`, `apiGroups: [""]`} {
		if !strings.Contains(rules, want) {
			t.Fatalf("console Role missing %s:\n%s", want, rules)
		}
	}
}

// ---- console authentication --------------------------------------------------

// The default authenticates: a token is required, and the config says so. This
// is the half of the switch that must survive every edit to the other half.
func TestConsoleAuthenticatesByDefault(t *testing.T) {
	out := helmTemplate(t)
	// the Channel document specifically: `externalAuthenticator` also names a
	// property in the ChannelAdapter's configSchema, which is documentation
	ch := consoleChannel(t, out)
	if !strings.Contains(ch, "authEnabled: true") {
		t.Fatalf("the console Channel must declare authEnabled: true by default:\n%s", ch)
	}
	if strings.Contains(ch, "externalAuthenticator:") {
		t.Fatalf("no authenticator is named by default — the console is its own:\n%s", ch)
	}
	if !strings.Contains(out, "uiToken:") {
		t.Fatal("the default install must still render the browser token")
	}
}

// consoleChannel returns the rendered console Channel document.
func consoleChannel(t *testing.T, out string) string {
	t.Helper()
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: Channel\nmetadata:") && strings.Contains(doc, "adapter: console") {
			return stripComments(doc)
		}
	}
	t.Fatal("no console Channel rendered")
	return ""
}

// Disabling the only gate takes TWO deliberate statements. One alone is a
// configuration that cannot be right, and the chart refuses it rather than
// installing an open console.
func TestConsoleAuthDisabledRequiresAnAuthenticator(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "console.auth.enabled=false")
	for _, want := range []string{"console.auth.externalAuthenticator", "oauth2-proxy"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("the guard must name the value to set and an example, got:\n%s", msg)
		}
	}

	out := helmTemplate(t, "--set", "console.auth.enabled=false",
		"--set", "console.auth.externalAuthenticator=oauth2-proxy")
	ch := consoleChannel(t, out)
	if !strings.Contains(ch, "authEnabled: false") ||
		!strings.Contains(ch, `externalAuthenticator: "oauth2-proxy"`) {
		t.Fatalf("both settings must reach the pod through the Channel config:\n%s", ch)
	}
	// The Channel declares credentialsSecretRef and the reconciler projects it
	// with envFrom, so tidying the Secret away would turn "disable auth" into
	// "the console will not start" — with no obvious cause.
	if !strings.Contains(out, "uiToken:") {
		t.Fatal("the token Secret must survive the switch, or the adapter pod cannot start")
	}
}

// Console Ingress. Exposing the console is when its bearer token starts
// crossing a network, so both the positive shapes and the two guards that
// REFUSE a configuration are pinned here — a guard nobody tests is a guard that
// silently stops firing.

const (
	ingressOn   = "--set=console.ingress.enabled=true"
	ingressHost = "--set=console.ingress.host=console.example.com"
)

// consoleIngress returns the rendered Ingress document, failing if there is not
// exactly one.
func consoleIngress(t *testing.T, out string) string {
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

// No Ingress unless asked for: reaching the console means a port-forward or a
// deliberate decision.
func TestConsoleIngressIsOptIn(t *testing.T) {
	if strings.Contains(helmTemplate(t), "kind: Ingress") {
		t.Fatal("default install must render no Ingress")
	}
	// ingress values set while the console itself is off must still render
	// nothing — one value removes every console object, the Ingress included
	out := helmTemplate(t, "--set=console.enabled=false", ingressOn, ingressHost)
	if strings.Contains(out, "kind: Ingress") {
		t.Fatal("console.enabled=false must render no Ingress even with ingress values set")
	}
}

// The backend is the Service the ChannelAdapter reconciler owns. A chart that
// shipped its own would make the console deployable only by this chart.
func TestConsoleIngressTargetsTheReconcilersService(t *testing.T) {
	doc := consoleIngress(t, helmTemplate(t, ingressOn, ingressHost))
	for _, want := range []string{"name: agentops-adapter-console", "number: 8080"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("Ingress backend missing %q:\n%s", want, doc)
		}
	}
	// scoped to console documents: the chart ships a Service for the MANAGER,
	// which is its own and unrelated
	for _, d := range splitDocs(helmTemplate(t, ingressOn, ingressHost)) {
		if !strings.Contains(d, "agentops-adapter-console") {
			continue
		}
		// "kind: Service\n" with the newline: the console RoleBinding's subject
		// is `kind: ServiceAccount`, which a prefix match would flag
		for _, forbidden := range []string{"kind: Deployment", "kind: Service\n"} {
			if strings.Contains(d, forbidden) {
				t.Fatalf("chart must not ship %s for the console:\n%s", forbidden, d)
			}
		}
	}
}

// extraHosts serve the same console: one rule each, all to the same backend.
func TestConsoleIngressExtraHosts(t *testing.T) {
	doc := consoleIngress(t, helmTemplate(t, ingressOn, ingressHost,
		"--set=console.ingress.extraHosts={alt.example.com,legacy.example.com}"))
	for _, want := range []string{"console.example.com", "alt.example.com", "legacy.example.com"} {
		if !strings.Contains(doc, "host: \""+want+"\"") {
			t.Fatalf("missing rule for %s:\n%s", want, doc)
		}
	}
	if n := strings.Count(doc, "name: agentops-adapter-console"); n != 3 {
		t.Fatalf("expected 3 rules to the same backend, found %d:\n%s", n, doc)
	}
}

// TLS hosts are DERIVED from host+extraHosts, so a rule host and a certificate
// host cannot drift apart.
func TestConsoleIngressTLSDerivesItsHosts(t *testing.T) {
	doc := consoleIngress(t, helmTemplate(t, ingressOn, ingressHost,
		"--set=console.ingress.extraHosts={alt.example.com}",
		"--set=console.ingress.tls.secretName=my-cert"))
	if !strings.Contains(doc, "secretName: my-cert") {
		t.Fatalf("TLS entry missing the named secret:\n%s", doc)
	}
	tls := doc[strings.Index(doc, "tls:"):strings.Index(doc, "rules:")]
	for _, want := range []string{"console.example.com", "alt.example.com"} {
		if !strings.Contains(tls, want) {
			t.Fatalf("derived tls hosts missing %s:\n%s", want, tls)
		}
	}
}

// cert-manager in one value: the annotation plus a derived Secret name.
func TestConsoleIngressClusterIssuer(t *testing.T) {
	doc := consoleIngress(t, helmTemplate(t, ingressOn, ingressHost,
		"--set=console.ingress.tls.clusterIssuer=letsencrypt"))
	if !strings.Contains(doc, "cert-manager.io/cluster-issuer: letsencrypt") {
		t.Fatalf("missing issuer annotation:\n%s", doc)
	}
	if !strings.Contains(doc, "secretName: agentops-console-console-tls") {
		t.Fatalf("clusterIssuer must derive a secretName:\n%s", doc)
	}
}

// The raw escape hatch wins over the derived form — it exists for the cases
// derivation cannot express, so derivation must not fight it.
func TestConsoleIngressExistingTLSWins(t *testing.T) {
	doc := consoleIngress(t, helmTemplate(t, ingressOn, ingressHost,
		"--set=console.ingress.tls.secretName=derived",
		`--set-json=console.ingress.tls.existing=[{"secretName":"raw","hosts":["only.example.com"]}]`))
	if !strings.Contains(doc, "secretName: raw") || strings.Contains(doc, "secretName: derived") {
		t.Fatalf("tls.existing must render verbatim and suppress derivation:\n%s", doc)
	}
}

// The pre-6.x LIST form still renders. `helm upgrade --reuse-values` carries a
// previous release's console: map forward wholesale, so dropping this branch
// would fail the upgrade rather than the thing being upgraded.
func TestConsoleIngressLegacyTLSListStillRenders(t *testing.T) {
	doc := consoleIngress(t, helmTemplate(t, ingressOn, ingressHost,
		`--set-json=console.ingress.tls=[{"secretName":"legacy","hosts":["old.example.com"]}]`))
	if !strings.Contains(doc, "secretName: legacy") || !strings.Contains(doc, "old.example.com") {
		t.Fatalf("legacy list form of tls must render verbatim:\n%s", doc)
	}
}

// Two configurations that must FAIL rather than render something broken.
func TestConsoleIngressGuards(t *testing.T) {
	// a hostname cannot be guessed
	if msg := helmTemplateErr(t, ingressOn); !strings.Contains(msg, "console.ingress.host is required") {
		t.Fatalf("missing host must fail naming the value, got:\n%s", msg)
	}
	// the SPA is embedded with an absolute asset base, so a sub-path routes
	// correctly and then renders a blank page — refuse at render time rather
	// than at first page load
	msg := helmTemplateErr(t, ingressOn, ingressHost, "--set=console.ingress.path=/console")
	if !strings.Contains(msg, `console.ingress.path must be "/"`) {
		t.Fatalf("non-root path must fail with the root-hosting explanation, got:\n%s", msg)
	}
}

// The scrape templates are DEFAULT-DISABLED: neither VMServiceScrape nor
// ServiceMonitor is a built-in kind, and rendering one without its CRD fails the
// whole install.
func TestMetricsScrapeTemplatesAreOptIn(t *testing.T) {
	out := helmTemplate(t)
	for _, forbidden := range []string{"kind: VMServiceScrape", "kind: ServiceMonitor", "kind: VMRule"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("%s must not render by default", forbidden)
		}
	}
	on := helmTemplate(t,
		"--set", "metrics.vmServiceScrape.enabled=true",
		"--set", "metrics.serviceMonitor.enabled=true",
		"--set", "metrics.rules.enabled=true")
	for _, want := range []string{"kind: VMServiceScrape", "kind: ServiceMonitor", "kind: VMRule"} {
		if !strings.Contains(on, want) {
			t.Fatalf("%s did not render when enabled", want)
		}
	}
	// the scrape selects the manager Service by label, so that label must exist
	if !strings.Contains(on, "app.kubernetes.io/name: agentops-manager") {
		t.Fatal("the manager Service must carry the label the scrape selects")
	}
}

// ---- chart-generated credentials --------------------------------------------

// The two credentials the chart can generate, and the key each carries.
var generatedCredentials = []struct{ name, key string }{
	{"agentops-adapter-token", "token"},
	{"agentops-console-console", "uiToken"},
}

// secretDoc returns the rendered Secret of that name, or "" if none was emitted.
func secretDoc(rendered, name string) string {
	for _, doc := range splitDocs(rendered) {
		if strings.Contains(doc, "kind: Secret") && strings.Contains(doc, "name: "+name+"\n") {
			return doc
		}
	}
	return ""
}

// An explicitly configured token must WIN over whatever history the namespace
// holds. It used to lose: the existing-Secret branch was tested first, so on any
// install that already had a token the setting was accepted, documented and
// silently ignored — the worst shape a setting can have.
func TestExplicitTokensAreRendered(t *testing.T) {
	out := helmTemplate(t,
		"--set", "console.auth.uiToken=pinned-ui-token",
		"--set", "adapterAuth.token=pinned-master-token")
	for _, c := range []struct{ secret, key, want string }{
		{"agentops-console-console", "uiToken", "pinned-ui-token"},
		{"agentops-adapter-token", "token", "pinned-master-token"},
	} {
		doc := secretDoc(out, c.secret)
		if doc == "" {
			t.Fatalf("%s was not rendered", c.secret)
		}
		want := c.key + ": " + base64.StdEncoding.EncodeToString([]byte(c.want))
		if !strings.Contains(doc, want) {
			t.Fatalf("%s must carry the configured value (%s):\n%s", c.secret, want, doc)
		}
	}
}

// THE property this whole mechanism exists for: no renderer, cluster or not,
// ever proposes a fresh credential on the upgrade path. `lookup` returns empty
// wherever there is no cluster, so a template that could generate on upgrade
// does not merely SHOW a new token in a diff — piped to apply, it sets one,
// signing every browser out and invalidating every derived adapter token.
func TestUpgradeRendersNoGeneratedCredential(t *testing.T) {
	out := helmTemplate(t, "--is-upgrade")
	for _, c := range generatedCredentials {
		if doc := secretDoc(out, c.name); doc != "" {
			t.Fatalf("an upgrade with no explicit value must render no Secret, got:\n%s", doc)
		}
	}
}

// ...and the counterpart: an EXPLICIT value renders on upgrade too, because that
// is the path by which changing it takes effect. Install-only applies to the
// generated case alone.
func TestExplicitTokensRenderOnUpgrade(t *testing.T) {
	out := helmTemplate(t, "--is-upgrade",
		"--set", "console.auth.uiToken=rotated-ui",
		"--set", "adapterAuth.token=rotated-master")
	for _, c := range generatedCredentials {
		if secretDoc(out, c.name) == "" {
			t.Fatalf("%s must render on upgrade when configured explicitly", c.name)
		}
	}
}

// An install generates both, and each MUST carry the keep policy: not rendering
// on upgrade is only safe because the object survives leaving the manifest.
// Verified against helm — an unannotated resource dropped from the manifest is
// deleted, so this annotation is load-bearing rather than decorative.
func TestInstallGeneratesBothCredentialsAndKeepsThem(t *testing.T) {
	out := helmTemplate(t)
	for _, c := range generatedCredentials {
		doc := secretDoc(out, c.name)
		if doc == "" {
			t.Fatalf("%s must be generated on install", c.name)
		}
		if !strings.Contains(doc, "helm.sh/resource-policy: keep") {
			t.Fatalf("%s must carry the keep policy, or the first upgrade deletes it:\n%s", c.name, doc)
		}
		if !strings.Contains(doc, c.key+": ") {
			t.Fatalf("%s must carry key %q:\n%s", c.name, c.key, doc)
		}
	}
}

// Bringing a whole Secret still means the chart creates NONE — and the name is
// what the Channel references, so a rename cannot silently point the adapter at
// a Secret that is not there.
func TestExistingSecretIsReferencedAndNotCreated(t *testing.T) {
	out := helmTemplate(t,
		"--set", "console.auth.existingSecret=my-console-secret",
		"--set", "adapterAuth.existingSecret=my-adapter-secret")
	for _, c := range generatedCredentials {
		if doc := secretDoc(out, c.name); doc != "" {
			t.Fatalf("no Secret may be created when one is supplied, got:\n%s", doc)
		}
	}
	if secretDoc(out, "my-console-secret") != "" {
		t.Fatal("the supplied Secret must be referenced, never rendered")
	}
	var channel string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: Channel\n") && strings.Contains(doc, "adapter: console") {
			channel = doc
		}
	}
	if channel == "" {
		t.Fatal("console Channel not rendered")
	}
	if !strings.Contains(channel, "name: my-console-secret") {
		t.Fatalf("the Channel must reference the supplied Secret by name:\n%s", channel)
	}
}

func splitDocs(rendered string) []string {
	return strings.Split(rendered, "\n---\n")
}

func stripComments(doc string) string {
	var kept []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// ---- kubernetes events lane -------------------------------------------------

// The events adapter now reads pods and replicasets to resolve an event's
// workload and to re-check liveness. The operator grants adapters nothing, so
// the chart is the only place that grant can come from — and if it silently
// went missing the adapter would report Ready=False forever.
func TestEventsAdapterRBACCoversPodsAndReplicaSets(t *testing.T) {
	for _, mode := range []struct{ name, flag, kind string }{
		{"cluster-wide", "true", "ClusterRole"},
		{"namespaced", "false", "Role"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			out := helmTemplate(t, "--set", "kubernetes.enabled=true",
				"--set", "kubernetes.eventsAdapter.rbac.clusterWide="+mode.flag)

			// Anchor on the document's OWN kind line, which is followed by
			// metadata:. A binding names the same kind and the same name in
			// its roleRef, so both "kind: Role" and the name are ambiguous.
			var found string
			for _, doc := range splitDocs(out) {
				if strings.Contains(doc, "kind: "+mode.kind+"\nmetadata:") &&
					strings.Contains(doc, "name: agentops-signal-k8s-events-events") {
					found = stripComments(doc)
				}
			}
			if found == "" {
				t.Fatalf("no %s for the events adapter rendered", mode.kind)
			}
			for _, needle := range []string{`resources: ["events"]`, `resources: ["pods"]`, `resources: ["replicasets"]`} {
				if !strings.Contains(found, needle) {
					t.Errorf("%s missing %s\n%s", mode.kind, needle, found)
				}
			}
		})
	}
}

// Grouping by pod name is what produced hundreds of conversations: pod names
// are unique per replica and regenerated on every rollout, so the signature
// never repeated and window reuse could never fire.
func TestEventsSourceGroupsByWorkload(t *testing.T) {
	out := helmTemplate(t, "--set", "kubernetes.enabled=true")
	src := eventsSourceDoc(t, out)
	if !strings.Contains(src, "- workload") {
		t.Fatalf("the events source must group by workload:\n%s", src)
	}
	if strings.Contains(src, "- name\n") {
		t.Fatalf("per-pod grouping must be gone:\n%s", src)
	}
}

// The default rule set's SHAPE is what keeps it safe, and the shape is what a
// well-meaning edit breaks. Pin the invariants, not the tuning: the numbers
// should stay editable without anyone having to re-derive these properties.
func TestDefaultRulesShape(t *testing.T) {
	out := helmTemplate(t, "--set", "kubernetes.enabled=true")
	src := eventsSourceDoc(t, out)

	// Reasons describing something that ALREADY happened must never dwell: a
	// re-check would find the healthy replacement and erase the incident.
	//
	// Evicted is deliberately NOT in this list: it is dropped outright, not
	// given a dwell. The two are different failures — a dwell would erase the
	// incident silently, whereas the drop is justified below by the reasons
	// that report the same incident from the cause end and the consequence end.
	pastTense := []string{"OOMKilling", "SystemOOM", "BackoffLimitExceeded", "DeadlineExceeded"}
	for _, reason := range pastTense {
		line := ruleLineContaining(src, reason)
		if line == "" {
			t.Errorf("past-tense reason %q is not covered by any rule", reason)
			continue
		}
		if !strings.Contains(line, `for: "0"`) {
			t.Errorf("past-tense reason %q must carry for: \"0\", got rule:\n%s", reason, line)
		}
	}

	// Evicted is dropped, and the drop is only defensible while BOTH of its
	// substitutes survive. Pin them together: a later edit that re-tunes node
	// pressure or FailedScheduling must not silently leave eviction unreported
	// from every direction at once.
	evicted := ruleLineContaining(src, "Evicted")
	if evicted == "" {
		t.Error("Evicted is not covered by any rule")
	} else if !strings.Contains(evicted, "action: drop") {
		t.Errorf("Evicted must be dropped, not dwelled, got rule:\n%s", evicted)
	}
	for _, cause := range []string{"NodeHasMemoryPressure", "NodeHasDiskPressure"} {
		line := ruleLineContaining(src, cause)
		if line == "" || !strings.Contains(line, `for: "0"`) {
			t.Errorf("dropping Evicted requires %q to report the cause at for: \"0\", got rule:\n%s", cause, line)
		}
	}
	if line := ruleLineContaining(src, "FailedScheduling"); line == "" {
		t.Error("dropping Evicted requires FailedScheduling to report pods that fail to come back")
	}

	// The last rule must be a catch-all WITH a dwell, or an unanticipated
	// reason is silently discarded instead of verified.
	rules := ruleBlocks(src)
	if len(rules) == 0 {
		t.Fatal("no default rules rendered")
	}
	last := rules[len(rules)-1]
	if !strings.Contains(last, "matchers: []") {
		t.Fatalf("the last rule must be a catch-all:\n%s", last)
	}
	if strings.Contains(last, "action: drop") {
		t.Fatalf("the catch-all must never be a drop:\n%s", last)
	}
	if !strings.Contains(last, "for:") {
		t.Fatalf("the catch-all must carry a dwell:\n%s", last)
	}
}

// A maintenance window is opt-in. Muting is the one suppression axis that can
// silence a source without any event matching a rule, so a default that shipped
// one would be a cluster going quiet for reasons nobody configured.
func TestNoMuteWindowsByDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "kubernetes.enabled=true")
	src := stripComments(eventsSourceDoc(t, out))
	for _, needle := range []string{"timeIntervals", "muteTimeIntervals"} {
		if strings.Contains(src, needle) {
			t.Errorf("the default source must declare no %s:\n%s", needle, src)
		}
	}
}

// The chart passes `route` through verbatim, so this pins that the time axis
// actually REACHES the SignalSource — including the location, the field most
// likely to be dropped by a re-spelling and the one whose absence is invisible
// for six months until a daylight-saving change moves the window.
func TestConfiguredMuteWindowReachesTheSource(t *testing.T) {
	out := helmTemplate(t,
		"--set", "kubernetes.enabled=true",
		"--set", "kubernetes.eventsAdapter.source.route.timeIntervals[0].name=nightly",
		"--set", "kubernetes.eventsAdapter.source.route.timeIntervals[0].location=Europe/Kyiv",
		"--set", "kubernetes.eventsAdapter.source.route.timeIntervals[0].times[0].startTime=04:00",
		"--set", "kubernetes.eventsAdapter.source.route.timeIntervals[0].times[0].endTime=04:20",
		"--set", "kubernetes.eventsAdapter.source.route.muteTimeIntervals[0].name=nightly",
		"--set", `kubernetes.eventsAdapter.source.route.muteTimeIntervals[0].matchers[0]=reason="NodeNotReady"`,
	)
	src := stripComments(eventsSourceDoc(t, out))
	for _, needle := range []string{
		"timeIntervals:", "name: nightly", "location: Europe/Kyiv",
		"startTime: \"04:00\"", "endTime: \"04:20\"",
		"muteTimeIntervals:", "NodeNotReady",
	} {
		if !strings.Contains(src, needle) {
			t.Errorf("configured window missing %q from the rendered source:\n%s", needle, src)
		}
	}
}

// ---- runtime ownership ------------------------------------------------------

// The substrate is the PARENT's: one AgentRuntime, one runtime ServiceAccount,
// whatever bundles are on. The bundle used to ship its own — which is how two
// runtime identities came to exist, one of them granted everything.
func TestParentOwnsExactlyOneRuntime(t *testing.T) {
	for _, combo := range [][]string{
		nil,
		{"--set", "global.demo.enabled=true"},
		{"--set", "kubernetes.enabled=true"},
		{"--set", "telegram.enabled=true"},
		{"--set", "kubernetes.enabled=true", "--set", "telegram.enabled=true"},
	} {
		name := "defaults"
		if len(combo) > 0 {
			name = strings.Join(combo[1:], ",")
		}
		t.Run(name, func(t *testing.T) {
			out := helmTemplate(t, combo...)
			// the claude bundle's own CR plus the parent's `default` copy of it
			if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 2 {
				t.Errorf("want the claude runtime and its default copy, got %d", n)
			}
			var sas int
			for _, doc := range splitDocs(out) {
				if strings.Contains(doc, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n") {
					sas++
				}
			}
			if sas != 1 {
				t.Errorf("want exactly 1 runtime ServiceAccount, got %d", sas)
			}
			// the bundle-named identity must be gone everywhere, bindings included
			if strings.Contains(out, "agentops-runtime-k8s") {
				t.Error("the bundle-named runtime ServiceAccount must not render")
			}
		})
	}
}

// "Bring your own runtime": no runtime renders, but the FLOOR account stays —
// the manager defaults every runtime pod onto it whoever wrote the CR.
//
// Nothing resolves to `default` here, so the default-runtime guard stays quiet:
// it fails only where a route would have needed one.
func TestNoRuntimeDeclaredRendersNoRuntimeObjects(t *testing.T) {
	out := helmTemplate(t, "--set", "claude.enabled=false",
		"--set", "claude.credentialsSecret.token=x")
	// anchored: the CRD document names the kind too, and it ships regardless
	if strings.Contains(out, "\nkind: AgentRuntime\n") {
		t.Error("declaring no runtime must render no AgentRuntime")
	}
	if strings.Contains(out, "name: agentops-claude") {
		t.Error("declaring no runtime must render no credential Secret")
	}
	if !strings.Contains(out, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n") {
		t.Error("the FLOOR account is not part of any runtime and must still render")
	}
}

// The release has ONE idle-TTL number, and it is in the DEFAULTS block. The
// field must be WRITTEN, not omitted: AgentRuntime.spec.idleTtlMinutes carries
// a CRD default of 10, so an omitted field is stored as 10 and the manager
// prefers any non-zero spec value over its own bootstrap default — omitting it
// looks right in the manifest and silently ignores the release's setting.
//
// IT USED TO BE A TOP-LEVEL KEY, which a bundle-shipped runtime could not read,
// so it rendered EMPTY and the CRD default replaced it. That is the failure this
// pins from both ends.
func TestRuntimeIdleTTLComesFromTheDefaults(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.runtimeDefaults.idleTtlMinutes=7")
	if !strings.Contains(out, "idleTtlMinutes: 7") {
		t.Error("the default idleTtlMinutes must reach the runtime a bundle ships")
	}
	if !strings.Contains(out, `value: "7"`) {
		t.Error("the manager's own fallback must read the same number")
	}
	out = helmTemplate(t, "--set", "global.agentops.runtimeDefaults.idleTtlMinutes=7",
		"--set", "claude.idleTtlMinutes=30")
	if !strings.Contains(out, "idleTtlMinutes: 30") {
		t.Error("a runtime stating its own idleTtlMinutes must win")
	}
}

// THE DEFAULT IS NOTHING, AND NO SETTING WIDENS IT.
//
// This replaces a sweep over `global.agentops.runtime.rbacMode`, which rendered
// a named account from a release-wide value. That value is DELETED: it granted
// nothing until a route named the account, so its name described a state the
// runtime was in — the reading that caused the incident it was reverted for.
//
// The property that sweep protected is stronger now and is what this pins: with
// nothing declared, no account carrying any grant exists at all, in any mode,
// under demo or not.
func TestNothingIsGrantedUnlessAnAccountIsDeclared(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unset", nil},
		{"demo mode", []string{"--set", "global.demo.enabled=true"}},
		{"demo mode with the bundle acting", []string{
			"--set", "global.demo.enabled=true", "--set", "kubernetes.allowMutations=true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmTemplate(t, tc.args...)
			for _, doc := range strings.Split(out, "\n---") {
				if !strings.Contains(doc, "\nkind: ClusterRoleBinding\n") &&
					!strings.Contains(doc, "\nkind: RoleBinding\n") {
					continue
				}
				// The MCP server's and the adapters' own accounts are not the
				// agent's — an agent reaches the cluster THROUGH the server,
				// which is the point of the second identity.
				if strings.Contains(doc, "name: agentops-mcp-") ||
					strings.Contains(doc, "name: agentops-signal-") ||
					strings.Contains(doc, "name: agentops-adapter-") ||
					strings.Contains(doc, "name: agentops-manager") ||
					strings.Contains(doc, "name: agentops-housekeeping") {
					continue
				}
				if strings.Contains(doc, "name: agentops-runtime") {
					t.Errorf("no runtime identity may be bound to anything when none is "+
						"declared — silence must mean no power:\n%s", doc)
				}
			}
			if strings.Contains(out, "name: cluster-admin") {
				t.Error("cluster-admin must never be bound")
			}
		})
	}
}

// The old key would otherwise be read by nothing, running agents under an
// identity the operator did not choose.
func TestMovedRuntimeSAKeyFails(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "serviceAccounts.runtime=agentops-runtime-k8s")
	if !strings.Contains(msg, "global.agentops.runtimeDefaults.serviceAccountName") {
		t.Fatalf("the failure must name the new key:\n%s", msg)
	}
}

// ---- kubernetes MCP ---------------------------------------------------------

// mcp and mcpServers flip together, so the config's URL always has a Service to
// default onto. The guard exists for the combination that is genuinely broken.
func TestMCPEndpointGuardStillBites(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "kubernetes.enabled=true",
		"--set", "kubernetes.mcpServers.enabled=false")
	if !strings.Contains(msg, "mcp.url is required") {
		t.Fatalf("the endpoint guard must name the missing URL:\n%s", msg)
	}
}

// ONE STATED SETTING configures both walls coherently — an agent reaches the
// cluster THROUGH this server, so moving one and not the other leaves the hole
// one indirection along. An explicit readOnly must still recover the separation.
func TestMCPServerFollowsAllowMutations(t *testing.T) {
	readOnly := helmTemplate(t, "--set", "kubernetes.enabled=true")
	if !strings.Contains(readOnly, "- --read-only") {
		t.Error("default posture must be a read-only server")
	}
	if strings.Contains(readOnly, "name: k8s-admin") {
		t.Error("no mutating toolset without a server that registers those tools")
	}

	full := helmTemplate(t, "--set", "kubernetes.enabled=true",
		"--set", "kubernetes.allowMutations=true")
	if strings.Contains(full, "- --read-only") {
		t.Error("allowMutations must yield a write-capable server")
	}
	if !strings.Contains(full, "name: k8s-admin") {
		t.Error("allowMutations must render the mutating toolset with no other value set")
	}
	// The server's account gets the SAME split grant the runtime's does — an
	// agent reaches the cluster THROUGH this server, so leaving it on
	// cluster-admin would have kept the hole one indirection along. The
	// cluster-scoped half is a ClusterRole named for the account.
	if !strings.Contains(full, "name: agentops-mcp-k8s\n") {
		t.Error("allowMutations must yield an acting role for the server account")
	}
	if strings.Contains(full, "name: cluster-admin") {
		t.Error("the MCP server must not be bound to cluster-admin, ever")
	}

	recovered := helmTemplate(t, "--set", "kubernetes.enabled=true",
		"--set", "kubernetes.allowMutations=true",
		"--set", "kubernetes.mcpServers.readOnly=true")
	if !strings.Contains(recovered, "- --read-only") {
		t.Error("an explicit readOnly must win over the derivation")
	}
	if !strings.Contains(recovered, "name: k8s-admin") {
		t.Error("the toolset is a SIBLING of the server's flag, not a consequence — an explicit " +
			"readOnly must not un-render what allowMutations stated")
	}
}

// Collapsing the two identities removes the only thing this component adds
// over kubectl. The guard now compares against the release-wide SA.
func TestMCPServerRefusesTheRuntimeIdentity(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "kubernetes.enabled=true",
		"--set", "kubernetes.mcpServers.serviceAccountName=agentops-runtime")
	if !strings.Contains(msg, "global.agentops.runtime.serviceAccountName") {
		t.Fatalf("the guard must name the global key:\n%s", msg)
	}
}

// ---- kubernetes wiring ------------------------------------------------------

// bundlePipelines returns the Pipelines the BUNDLE rendered, by name. Anchored
// on the bundle label: an install-declared Pipeline carries
// app.kubernetes.io/name: agentops, and the CRD document names the kind too.
func bundlePipelines(rendered string) map[string]string {
	return labelledPipelines(rendered, "agentops-kubernetes")
}

// labelledPipelines returns the Pipelines a given BUNDLE rendered, keyed by
// name. Keying on the bundle label rather than on the kind alone is what keeps
// these assertions honest once more than one bundle ships wiring: a route the
// install declared under the parent's `pipelines:` must never be mistaken for
// one the bundle shipped, or "the bundle renders no Pipeline" passes for the
// wrong reason.
func labelledPipelines(rendered, label string) map[string]string {
	out := map[string]string{}
	for _, doc := range splitDocs(rendered) {
		if !strings.Contains(doc, "\nkind: Pipeline\n") ||
			!strings.Contains(doc, "app.kubernetes.io/name: "+label) {
			continue
		}
		i := strings.Index(doc, "metadata:\n  name: ")
		if i < 0 {
			continue
		}
		rest := doc[i+len("metadata:\n  name: "):]
		out[strings.TrimSpace(rest[:strings.Index(rest, "\n")])] = stripComments(doc)
	}
	return out
}

func pipelineNames(rendered string) []string {
	var names []string
	for n := range bundlePipelines(rendered) {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The default-off rule is the load-bearing half of "a bundle may ship wiring":
// enabling this bundle for its adapter and profile must never silently add a
// route beside the one the install declared under the parent's `pipelines:`.
func TestBundleShipsNoWiringUnlessAsked(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"default install", nil},
		{"bundle enabled directly", []string{"--set", "kubernetes.enabled=true"}},
		{"wiring declined under demo", []string{
			"--set", "global.demo.enabled=true",
			"--set", "kubernetes.pipelines.enabled=false"}},
		{"no profile, no route", []string{
			"--set", "global.demo.enabled=true",
			"--set", "kubernetes.profile.enabled=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pipelineNames(helmTemplate(t, tc.args...)); len(got) != 0 {
				t.Errorf("the bundle must render no Pipeline here, got %v", got)
			}
		})
	}
}

// Declining the route must cost nothing else — the opt-out in docs/CHANGELOG.md is
// one value, and it has to leave a bundle that still watches, profiles and tools.
func TestDecliningWiringLeavesTheRestOfTheBundle(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.pipelines.enabled=false")
	for _, needle := range []string{
		"kind: SignalAdapter", "name: cluster-events", "kind: AgentProfile",
		"name: k8s-engineer", "kind: MCPConfig", "name: k8s-observability",
	} {
		if !strings.Contains(out, needle) {
			t.Errorf("declining wiring must not remove %q", needle)
		}
	}
}

// Demo mode's whole promise is one flag and a working install. Before this it
// rendered an events lane, a profile and tooling that answered nothing.
func TestDemoModeWiresTheObservingRoute(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true")
	pipes := bundlePipelines(out)
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-observe" {
		t.Fatalf("demo mode must render exactly k8s-observe, got %v", got)
	}
	doc := pipes["k8s-observe"]
	for _, needle := range []string{
		"name: k8s-engineer",      // the bundle's profile
		"name: cluster-events",    // claims the bundle's source
		"name: agentops-observe",  // built-in reads
		"name: k8s-observability", // cluster reads
		"name: k8s-api",           // the MCPConfig
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("the observing route must bind %q:\n%s", needle, doc)
		}
	}
	if strings.Contains(doc, "name: k8s-admin") {
		t.Errorf("the observing route must NOT bind the mutating toolset:\n%s", doc)
	}
}

// ONE STATED SETTING, four consistent effects. `allowMutations` drops
// --read-only, widens the server SA and renders k8s-admin; the route that binds
// it has to move with them, or the setting grants a power no route can
// exercise. It is the bundle's OWN value now — the release-wide permission mode
// that used to drive all four named none of them.
func TestAllowMutationsPromotesTheRouteToActing(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.allowMutations=true")
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-operate" {
		t.Fatalf("allowMutations must render exactly k8s-operate, got %v", got)
	}
	doc := bundlePipelines(out)["k8s-operate"]
	for _, needle := range []string{"name: k8s-observability", "name: k8s-admin"} {
		if !strings.Contains(doc, needle) {
			t.Errorf("the acting route must bind %q:\n%s", needle, doc)
		}
	}
}

// Derivation is a default, never a ceiling or a floor: the explicit value
// decides in both directions, exactly as mcpServers.readOnly does.
func TestExplicitRouteValuesBeatTheDerivation(t *testing.T) {
	// acting route asked for under a read-only release
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.pipelines.admin.enabled=true",
		"--set", "kubernetes.pipelines.observe.enabled=false")
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-operate" {
		t.Fatalf("an explicit acting route must render under readonly, got %v", got)
	}
	// ...but it binds no toolset the read-only server never registered
	if doc := bundlePipelines(out)["k8s-operate"]; strings.Contains(doc, "name: k8s-admin") {
		t.Errorf("no ref to a toolset that was not rendered:\n%s", doc)
	}

	// observing route asked for under `full`
	out = helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.allowMutations=true",
		"--set", "kubernetes.pipelines.observe.enabled=true",
		"--set", "kubernetes.pipelines.admin.enabled=false")
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-observe" {
		t.Fatalf("an explicit observing route must win under allowMutations, got %v", got)
	}
}

// Two Ready Pipelines on one source is a SUPPORTED shape — sources are
// shareable and sourceConflicts was deleted. Failing the render here would be
// that guard returning one layer up.
func TestBothRoutesRenderWithoutConflict(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.pipelines.observe.enabled=true",
		"--set", "kubernetes.pipelines.admin.enabled=true")
	got := pipelineNames(out)
	if len(got) != 2 || got[0] != "k8s-observe" || got[1] != "k8s-operate" {
		t.Fatalf("both routes must render, got %v", got)
	}
	for _, name := range got {
		if !strings.Contains(bundlePipelines(out)[name], "name: cluster-events") {
			t.Errorf("%s must claim the shared source", name)
		}
	}
}

// A bundle may ship wiring only because every foreign name is values-supplied
// and omitted when unset, and every ref to its own components disappears with
// them. A ref to an object nobody rendered is how an allowlist rots.
func TestWiringNamesOnlyWhatWasRendered(t *testing.T) {
	// The console is deployed by default and the route claims it, so a turnkey
	// install can start a conversation in the surface it just installed.
	bare := bundlePipelines(helmTemplate(t, "--set", "global.demo.enabled=true"))["k8s-observe"]
	if !strings.Contains(bare, "channelRefs:\n    - name: console") {
		t.Errorf("the console must be bound as a channel by default:\n%s", bare)
	}
	if !strings.Contains(bare, "- name: console\n") || !strings.Contains(bare, "signalSourceRefs") {
		t.Errorf("the console's source must be claimed by default:\n%s", bare)
	}

	// A named channel joins the console rather than replacing it.
	named := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.pipelines.channels={home-ops}")
	if doc := bundlePipelines(named)["k8s-observe"]; !strings.Contains(doc, "- name: home-ops") ||
		!strings.Contains(doc, "- name: console") {
		t.Errorf("a named channel must join the console, not replace it:\n%s", doc)
	}

	// Every component the route would reference, turned off at once — INCLUDING
	// the console, whose names the parent must clear when it is not deployed.
	off := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "kubernetes.mcp.enabled=false",
		"--set", "kubernetes.mcpServers.enabled=false",
		"--set", "global.builtinToolsets.enabled=false",
		"--set", "kubernetes.eventsAdapter.source.create=false",
		"--set", "console.enabled=false",
		"--set", "global.agentops.console.signalSource=",
		"--set", "global.agentops.console.channel=")
	doc := bundlePipelines(off)["k8s-observe"]
	if doc == "" {
		t.Fatal("the route still renders — it is gated on the profile, not on tooling")
	}
	for _, dangling := range []string{"signalSourceRefs", "channelRefs", "toolsets", "mcpConfigs"} {
		if strings.Contains(doc, dangling) {
			t.Errorf("%s must be omitted when nothing rendered it:\n%s", dangling, doc)
		}
	}
}

// The console's identity is duplicated into `global.` so subcharts can read it,
// and Helm cannot derive one value from another — so the render FAILS when the
// two disagree rather than leaving a route claiming a source nothing renders.
func TestConsoleWiringGuard(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "disabled console still published",
			args: []string{"--set", "global.demo.enabled=true", "--set", "console.enabled=false"},
			want: "still names global.agentops.console.signalSource",
		},
		{
			name: "renamed source not republished",
			args: []string{"--set", "global.demo.enabled=true", "--set", "console.signalSourceName=console-k8s"},
			want: "global.agentops.console.signalSource",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmTemplateErr(t, tc.args...)
			if !strings.Contains(out, tc.want) {
				t.Errorf("failure must name the value to set, got:\n%s", out)
			}
		})
	}
}

func eventsSourceDoc(t *testing.T, rendered string) string {
	t.Helper()
	for _, doc := range splitDocs(rendered) {
		if strings.Contains(doc, "kind: SignalSource") && strings.Contains(doc, "name: cluster-events") {
			return stripComments(doc)
		}
	}
	t.Fatal("no cluster-events SignalSource rendered")
	return ""
}

// ruleBlocks splits the rendered `rules:` list into its entries.
func ruleBlocks(src string) []string {
	_, after, ok := strings.Cut(src, "\n    rules:\n")
	if !ok {
		return nil
	}
	body, _, _ := strings.Cut(after, "\n    severities:")
	var out []string
	for _, chunk := range strings.Split(body, "\n    - ") {
		if strings.TrimSpace(chunk) != "" {
			out = append(out, chunk)
		}
	}
	return out
}

func ruleLineContaining(src, reason string) string {
	for _, block := range ruleBlocks(src) {
		if strings.Contains(block, reason) {
			return block
		}
	}
	return ""
}

// runtimeDoc returns the rendered AgentRuntime document.
func runtimeDoc(t *testing.T, out string) string {
	t.Helper()
	for _, doc := range splitDocs(out) {
		// anchored: the CRD document names the kind too, and ships regardless
		if strings.Contains(doc, "\nkind: AgentRuntime\n") && !strings.Contains(doc, "CustomResourceDefinition") {
			return doc
		}
	}
	t.Fatal("no AgentRuntime rendered")
	return ""
}

// The storage defaults, and the asymmetry between them: the accumulated context
// persists out of the box because losing it silently costs conversational
// history, while a checkout is re-cloned because a stale shared one is worse
// than no cache.
func TestPersistenceDefaultsContextOnWorkspaceOff(t *testing.T) {
	out := helmTemplate(t)

	if !strings.Contains(out, "\n  name: agentops-context\n") {
		t.Error("the context claim must render by default (persistence.context.enabled: true)")
	}
	if strings.Contains(out, "agentops-workspace") {
		t.Error("workspace persistence must be OFF by default — a stale shared checkout is worse than a re-clone")
	}
	// THE RUNTIME DECLARES NEITHER VOLUME, and the release-wide claim reaches
	// conversations through the manager's bootstrap default instead. That is
	// what lets two routes on this one runtime persist to two volumes.
	rt := runtimeDoc(t, out)
	for _, key := range []string{"pvcRef", "\n  context:", "\n  workspace:"} {
		if strings.Contains(rt, key) {
			t.Errorf("the rendered AgentRuntime carries %q. Persistence is WIRING and it moved to the "+
				"Pipeline; a runtime declaring a volume is what made two routes on one runtime "+
				"impossible:\n%s", key, rt)
		}
	}
	if !strings.Contains(out, "name: CONTEXT_PVC") {
		t.Error("the manager's CONTEXT_PVC bootstrap default must follow the claim — it is the only " +
			"path the release-wide volume now takes to a conversation")
	}
	if strings.Contains(out, "name: WORKSPACE_PVC") {
		t.Error("WORKSPACE_PVC must not be set when no workspace claim exists")
	}
}

// The opt-out is the whole mitigation for a cluster with no RWX provisioner —
// it must remove the claim AND the reference, or runtime pods still wait on it.
func TestPersistenceOptOutRemovesEverything(t *testing.T) {
	out := helmTemplate(t, "--set", "persistence.context.enabled=false")

	// MATCHED AS A CLAIM, NOT AS A SUBSTRING. `agentops-context` is a prefix of
	// `agentops-context-sync`, the sidecar IMAGE — which the manager's bootstrap
	// env carries whenever any runtime declares context paths, and the reference
	// runtime now does by default. A bare Contains here failed on an install
	// with no claim at all, which is the opposite of what it guards.
	for _, ref := range []string{
		"claimName: agentops-context",
		"name: agentops-context\n",
		"agentops-context\"",
	} {
		if strings.Contains(out, ref) {
			t.Errorf("persistence.context.enabled=false must render no context claim "+
				"and no reference to one; found %q", ref)
		}
	}
	if strings.Contains(out, "name: CONTEXT_PVC") {
		t.Error("persistence.context.enabled=false must not set CONTEXT_PVC")
	}
	if rt := runtimeDoc(t, out); strings.Contains(rt, "pvcRef") {
		t.Errorf("the AgentRuntime must declare no volume at all:\n%s", rt)
	}
}

// THE THREE STORAGE-CLASS STATES, on BOTH volumes.
//
// The `-` case is the one that was previously inexpressible: an ABSENT
// storageClassName is filled in by admission from the cluster's default class,
// which provisions a second volume and leaves the operator's pre-created one
// untouched. An explicit empty string is what declines that, and nothing
// rendered one.
func TestStorageClassConventionOnBothVolumes(t *testing.T) {
	for _, vol := range []struct{ key, claim string }{
		{"context", "agentops-context"},
		{"workspace", "agentops-workspace"},
	} {
		t.Run(vol.key, func(t *testing.T) {
			on := []string{"--set", "persistence." + vol.key + ".enabled=true"}

			// undefined/empty: no field at all, the default provisioner.
			doc := claimDoc(t, helmTemplate(t, on...), vol.claim)
			if strings.Contains(doc, "storageClassName") {
				t.Errorf("an empty class must omit the field entirely:\n%s", doc)
			}

			// a name: that class.
			doc = claimDoc(t, helmTemplate(t, append(append([]string{}, on...),
				"--set", "persistence."+vol.key+".storageClassName=fast-rwx")...), vol.claim)
			if !strings.Contains(doc, `storageClassName: "fast-rwx"`) {
				t.Errorf("a named class must render:\n%s", doc)
			}

			// "-": an EXPLICIT empty string, never an omitted field.
			doc = claimDoc(t, helmTemplate(t, append(append([]string{}, on...),
				"--set", "persistence."+vol.key+`.storageClassName=-`)...), vol.claim)
			if !strings.Contains(doc, `storageClassName: ""`) {
				t.Errorf(`"-" must render an EXPLICIT empty storage class, or admission injects the default class and provisions a second volume:`+"\n%s", doc)
			}
		})
	}
}

// The previously-broken combination: a pre-created volume named on a claim
// whose storage class was left at the shipped default. It bound nothing,
// because there was no spelling of "no storage class" at all.
func TestPreCreatedVolumeIsBindableOnBothVolumes(t *testing.T) {
	for _, vol := range []struct{ key, claim string }{
		{"context", "agentops-context"},
		{"workspace", "agentops-workspace"},
	} {
		t.Run("byName/"+vol.key, func(t *testing.T) {
			doc := claimDoc(t, helmTemplate(t,
				"--set", "persistence."+vol.key+".enabled=true",
				"--set", "persistence."+vol.key+".volumeName=pv-"+vol.key,
				"--set", "persistence."+vol.key+`.storageClassName=-`), vol.claim)

			if !strings.Contains(doc, "volumeName: pv-"+vol.key) {
				t.Errorf("the claim must name the pre-created volume:\n%s", doc)
			}
			if !strings.Contains(doc, `storageClassName: ""`) {
				t.Errorf("the working form must be expressible: name a volume AND decline provisioning:\n%s", doc)
			}
		})

		t.Run("byLabel/"+vol.key, func(t *testing.T) {
			doc := claimDoc(t, helmTemplate(t,
				"--set", "persistence."+vol.key+".enabled=true",
				"--set", `persistence.`+vol.key+`.selector.matchLabels.agentops\.dev/volume=`+vol.key,
				"--set", "persistence."+vol.key+`.storageClassName=-`), vol.claim)

			if !strings.Contains(doc, "selector:") || !strings.Contains(doc, "agentops.dev/volume: "+vol.key) {
				t.Errorf("the claim must carry the selector, for the fleet case naming one volume cannot serve:\n%s", doc)
			}
		})
	}

	// A pre-created volume is by definition not the release's to create.
	out := helmTemplate(t,
		"--set", "persistence.context.volumeName=pv-context",
		"--set", `persistence.context.storageClassName=-`)
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: PersistentVolume\n") {
			t.Fatalf("the chart must render NO PersistentVolume:\n%s", doc)
		}
	}
}

// The values block moved wholesale. Helm never reports an unread values key, so
// a flat `persistence.enabled: false` written for a cluster with no RWX
// provisioner would be silently ignored and provision the claim it declined.
func TestRetiredPersistenceKeysFailTheRender(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "persistence.enabled=false")

	if !strings.Contains(msg, "persistence.enabled") || !strings.Contains(msg, "persistence.context") {
		t.Errorf("the failure must name the retired key and where it moved to:\n%s", msg)
	}
}

// The runtime keys are GONE, not renamed, so a values file that still points
// the runtime at a claim must be told rather than ignored.
//
// The quiet case is the expensive one: an operator who deliberately named a
// claim the chart did not create keeps every signal of success while the
// release-wide claim is used instead, and every conversation on that install
// answers out of the wrong volume.
func TestRetiredRuntimeVolumeKeysFailTheRender(t *testing.T) {
	for _, key := range []string{"contextPvcRef", "homePvcRef", "workspacePvcRef"} {
		t.Run(key, func(t *testing.T) {
			msg := helmTemplateErr(t, "--set", "global.agentops.runtimeDefaults."+key+"=byo-claim")

			if !strings.Contains(msg, key) {
				t.Errorf("the failure must NAME the retired key:\n%s", msg)
			}
			if !strings.Contains(msg, "persistence.context") || !strings.Contains(msg, "pipelines[].persistence") {
				t.Errorf("the failure must name BOTH places the declaration moved to — release-wide "+
					"and per route:\n%s", msg)
			}
		})
	}
}

// A route the chart ships can bind a pre-created volume, and the chart renders
// the claim for it — under the SAME derived name the manager would use, since
// both must spell it identically or the route gets two claims and mounts the
// wrong one.
func TestAChartShippedRouteCanBindItsOwnVolume(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pipelines[0].name=k8s-ops",
		"--set", "pipelines[0].profile=k8s-engineer",
		"--set", "pipelines[0].contextVolume=pv-ops-context")

	var pipeline string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: Pipeline") && strings.Contains(doc, "\n  name: k8s-ops\n") {
			pipeline = doc
		}
	}
	if !strings.Contains(pipeline, "persistence:") || !strings.Contains(pipeline, "volumeName: pv-ops-context") {
		t.Fatalf("the route must carry its own binding:\n%s", pipeline)
	}

	doc := claimDoc(t, out, "agentops-k8s-ops-context")
	if !strings.Contains(doc, "volumeName: pv-ops-context") {
		t.Errorf("the claim must name the pre-created volume:\n%s", doc)
	}
	if !strings.Contains(doc, `storageClassName: ""`) {
		t.Errorf("the claim must decline dynamic provisioning with an EXPLICIT empty class, or "+
			"admission provisions a second volume beside the operator's:\n%s", doc)
	}
	// A route's storage outlives the route, and certainly the release.
	if !strings.Contains(doc, "helm.sh/resource-policy: keep") {
		t.Errorf("the per-route claim must carry the keep policy:\n%s", doc)
	}
	// A pre-created volume is by definition not the release's to create — the
	// per-route path is no exception to that.
	for _, d := range splitDocs(out) {
		if strings.Contains(d, "kind: PersistentVolume\n") {
			t.Fatalf("the chart must render NO PersistentVolume:\n%s", d)
		}
	}
}

// A route naming a claim that already exists creates nothing, at either level.
func TestAChartShippedRouteNamingAClaimRendersNoClaim(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pipelines[0].name=ha-ops",
		"--set", "pipelines[0].profile=ha-engineer",
		"--set", "pipelines[0].contextClaim=team-ha-context")

	if strings.Contains(out, "name: agentops-ha-ops-context") {
		t.Error("naming an EXISTING claim must render nothing — that is what naming it means")
	}
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: Pipeline") && strings.Contains(doc, "\n  name: ha-ops\n") {
			if !strings.Contains(doc, "claimName: team-ha-context") {
				t.Errorf("the route must carry the claim it named:\n%s", doc)
			}
		}
	}
}

// claimDoc returns the PersistentVolumeClaim document named `name`.
func claimDoc(t *testing.T, out, name string) string {
	t.Helper()
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: PersistentVolumeClaim") &&
			strings.Contains(doc, "\n  name: "+name+"\n") {
			return doc
		}
	}
	t.Fatalf("no PersistentVolumeClaim named %q rendered", name)
	return ""
}

// Enabling workspace persistence takes ONE value: the claim name is never
// restated by the operator, exactly as context.pvcRef already works.
func TestWorkspacePersistenceIsWiredFromOneValue(t *testing.T) {
	out := helmTemplate(t, "--set", "persistence.workspace.enabled=true")

	if !strings.Contains(out, "\n  name: agentops-workspace\n") {
		t.Error("the workspace claim must render when enabled")
	}
	// Uninstall must never destroy uncommitted agent work.
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "name: agentops-workspace") && strings.Contains(doc, "kind: PersistentVolumeClaim") {
			if !strings.Contains(doc, "helm.sh/resource-policy: keep") {
				t.Error("the workspace claim must carry the keep policy, like the context claim")
			}
		}
	}
	if rt := runtimeDoc(t, out); strings.Contains(rt, "pvcRef") {
		t.Errorf("the AgentRuntime declares no volume, for the workspace as for the context:\n%s", rt)
	}
	if !strings.Contains(out, "name: WORKSPACE_PVC") {
		t.Error("the manager's WORKSPACE_PVC bootstrap default must follow the claim")
	}

	// An existing claim is honored and provisions nothing.
	out = helmTemplate(t, "--set", "persistence.workspace.enabled=true",
		"--set", "persistence.workspace.existingClaim=byo-checkouts")
	if strings.Contains(out, "kind: PersistentVolumeClaim\nmetadata:\n  name: agentops-workspace") {
		t.Error("an existingClaim must provision nothing")
	}
	// The existing claim reaches conversations the one way any release-wide
	// claim does now: the manager's bootstrap default.
	if !strings.Contains(out, "value: \"byo-checkouts\"") {
		t.Error("WORKSPACE_PVC must carry the existing claim")
	}
}

// helmNotes renders the post-install notes, which `helm template` omits.
func helmNotes(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	// NOTES render ONLY through `helm install --dry-run`, and helm insists on
	// reaching a cluster for it: `helm template` omits them, and `--show-only`
	// cannot address a non-manifest. So this skips without one rather than
	// failing — the same posture as the missing-helm skip above.
	//
	// The CRDs are not rendered at all now — they live in the chart's crds/
	// directory, which `helm template` and `--dry-run` do not touch. This
	// dry-run install otherwise trips the ownership check against CRDs a real
	// release already owns. The notes do not depend on them.
	cmd := exec.Command("helm", append([]string{"install", "notes-test", chartDir(), "--dry-run"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "cluster unreachable") {
			t.Skip("no cluster reachable; NOTES cannot be rendered")
		}
		t.Fatalf("helm install --dry-run failed: %v\n%s", err, out)
	}
	return string(out)
}

// Durable context must not require distributed storage — but a single-attach
// claim with unpinned runtime pods works until a SECOND conversation schedules
// elsewhere, then fails to attach, far from the setting that caused it. That is
// a note, not a render failure: on a single-node cluster pinning is pointless.
func TestSingleAttachWithoutPinningIsCalledOut(t *testing.T) {
	out := helmNotes(t, "--set", "persistence.context.accessModes={ReadWriteOnce}")
	if !strings.Contains(out, "attached by ONE node") {
		t.Fatal("an unpinned single-attach claim must be called out in the notes")
	}

	// Pinned: the operator has said where runtime pods go, so there is nothing
	// to warn about.
	pinned := helmNotes(t, "--set", "persistence.context.accessModes={ReadWriteOnce}",
		"--set", `global.agentops.runtimeDefaults.nodeSelector.kubernetes\.io/hostname=node-1`)
	if strings.Contains(pinned, "attached by ONE node") {
		t.Fatal("pinning runtime pods resolves it — the warning must go")
	}

	// RWX: many nodes may attach, so the whole concern is absent.
	if strings.Contains(helmNotes(t), "attached by ONE node") {
		t.Fatal("the default RWX claim must not warn")
	}
}

// The notes speak about the bundle's route only where they have something true
// to say. The failure worth pinning is the middle row: an install that declared
// its own claim under the parent's `pipelines:` is fully wired, and telling it
// "nothing answers cluster events yet" sends someone to fix what is not broken.
func TestWiringNotesReadTheActualClaims(t *testing.T) {
	claimsIt := []string{
		"--set", "pipelines[0].name=k8s-ops",
		"--set", "pipelines[0].profile=k8s-engineer",
		"--set", "pipelines[0].signalSources={cluster-events}",
	}
	// A dry-run INSTALL adopts nothing, but helm still refuses cluster-scoped
	// objects another release owns — and these cases are the first to turn the
	// bundle on, whose RBAC is cluster-scoped by default. Nothing in the notes
	// reads it, so switch it off rather than requiring a cluster with no
	// agent-ops release on it.
	noClusterRBAC := []string{
		"--set", "kubernetes.eventsAdapter.rbac.create=false",
		"--set", "kubernetes.mcpServers.rbac.create=false",
	}
	// surface.name defaults to k8s-ops, and the chat SignalSource takes it too —
	// that name is what a claiming pipeline has to list.
	chatSurface := []string{
		"--set", "telegram.enabled=true",
		"--set", "telegram.surface.enabled=true",
		"--set", "telegram.surface.chatId=-100",
		"--set", "telegram.surface.credentials.botToken=x",
	}
	for _, tc := range []struct {
		name          string
		args          []string
		want, notWant []string
	}{
		{"bundle on, nobody claims", []string{"--set", "kubernetes.enabled=true"},
			[]string{"ONE STEP LEFT — nothing answers cluster events"}, nil},
		{"bundle on, the install claims",
			append([]string{"--set", "kubernetes.enabled=true"}, claimsIt...),
			nil, []string{"ONE STEP LEFT — nothing answers cluster events", "claimed TWICE"}},
		{"demo wires it", []string{"--set", "global.demo.enabled=true"},
			[]string{"this release WIRED it", "k8s-observe"},
			[]string{"ONE STEP LEFT — nothing answers cluster events", "claimed TWICE"}},
		{"both claim it — a note, never a failure",
			append([]string{"--set", "global.demo.enabled=true"}, claimsIt...),
			[]string{"claimed TWICE", "k8s-ops"}, nil},
		// The same rule one lane over. telegram genuinely ships no
		// Pipeline, so its prompt stays — but only while nobody has answered it.
		{"chat surface nobody claims", chatSurface,
			[]string{"ONE STEP LEFT — nothing answers yet"}, nil},
		{"chat surface the install claims",
			append(append([]string{}, chatSurface...),
				"--set", "pipelines[0].name=chat",
				"--set", "pipelines[0].profile=k8s-engineer",
				"--set", "pipelines[0].signalSources={k8s-ops}"),
			nil, []string{"ONE STEP LEFT — nothing answers yet"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmNotes(t, append(tc.args, noClusterRBAC...)...)
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

// An install that cannot carry context says so plainly, rather than letting
// every follow-up discover it.
func TestEphemeralInstallSaysConversationsCannotContinue(t *testing.T) {
	out := helmNotes(t, "--set", "persistence.context.enabled=false")
	if !strings.Contains(out, "CANNOT BE CONTINUED") {
		t.Fatal("an install with no durable context volume must say conversations cannot be continued")
	}
	// ...and it names the way to have it without distributed storage.
	if !strings.Contains(out, "single-node") {
		t.Fatal("the notes must name the single-node topology as the remedy")
	}
	if strings.Contains(helmNotes(t), "CANNOT BE CONTINUED") {
		t.Fatal("the default install CAN continue conversations")
	}
}

// ---- home-assistant --------------------------------------------------------------

// haArgs is the smallest enablement that renders every home-assistant component.
func haArgs(extra ...string) []string {
	return append([]string{
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.controlSecret=ha-control",
		"--set", "home-assistant.homeAssistant.credentials.operatorSecret=ha-operator",
	}, extra...)
}

func haDoc(t *testing.T, rendered, kind, name string) string {
	t.Helper()
	for _, doc := range splitDocs(rendered) {
		if strings.Contains(doc, "\nkind: "+kind+"\n") &&
			strings.Contains(doc, "\n  name: "+name+"\n") &&
			strings.Contains(doc, "agentops-home-assistant") {
			return doc
		}
	}
	t.Fatalf("no %s/%s rendered by home-assistant", kind, name)
	return ""
}

// The bundle is off by default and demo mode must never turn it on: every
// component needs an endpoint and a token no demo cluster has.
func TestHaBundleIsOffByDefaultAndUnderDemo(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"default", nil},
		{"demo", []string{"--set", "global.demo.enabled=true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmTemplate(t, tc.args...)
			if strings.Contains(out, "agentops-home-assistant") {
				t.Fatal("home-assistant must render nothing here")
			}
		})
	}
}

// The wiring flag DEFAULTS OFF. That default is the load-bearing half of the
// rule that lets a subchart ship wiring at all: enabling the bundle for its
// parts must never silently acquire routes beside the ones the install declared.
func TestHaWiringDefaultsOff(t *testing.T) {
	out := helmTemplate(t, haArgs()...)
	// Checked per DOCUMENT, not by substring: the bundle's Secret names contain
	// the route names, so a grep would find itself.
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: Pipeline\n") && strings.Contains(doc, "agentops-home-assistant") {
			t.Fatalf("home-assistant.pipelines.enabled defaults false:\n%s", doc)
		}
	}
	// ...and the rest of the bundle still renders.
	haDoc(t, out, "AgentProfile", "ha-user")
	haDoc(t, out, "SignalSource", "ha-logs")
}

// BOTH routes claim the chat sources. Wiring is many-to-many, so a surface
// serving both agents is the ordinary shape — and the claim is what puts each
// agent in the list an unaddressed message is answered with.
func TestHaBothRoutesClaimTheChatSources(t *testing.T) {
	out := helmTemplate(t, haArgs(
		"--set", "home-assistant.pipelines.enabled=true",
		"--set", "home-assistant.pipelines.chatSources={console-ha,home-ops}",
	)...)

	control := stripComments(haDoc(t, out, "Pipeline", "ha-control"))
	ops := stripComments(haDoc(t, out, "Pipeline", "ha-ops"))
	for _, doc := range []string{control, ops} {
		for _, src := range []string{"- name: console-ha", "- name: home-ops"} {
			if !strings.Contains(doc, src) {
				t.Fatalf("both routes claim every chat source, missing %q:\n%s", src, doc)
			}
		}
	}
	// The log source is the ONLY asymmetry between them.
	if !strings.Contains(ops, "- name: ha-logs") {
		t.Fatalf("the ops route must claim the log source:\n%s", ops)
	}
	if strings.Contains(control, "- name: ha-logs") {
		t.Fatalf("the control route answers people, not the log:\n%s", control)
	}

	// The split is USE versus FIX, so BOTH bind the service-call toolset. What
	// separates them is the REST path: configuration is what the ops agent
	// repairs, and no Assist intent reaches configuration.
	for _, doc := range []string{control, ops} {
		if !strings.Contains(doc, "- name: ha-actions") {
			t.Fatalf("both routes bind the service-call toolset:\n%s", doc)
		}
	}
	if !strings.Contains(ops, "- name: agentops-shell") {
		t.Fatalf("the ops route must bind the shell toolset — REST is how it reconfigures:\n%s", ops)
	}
	if strings.Contains(control, "- name: agentops-shell") {
		t.Fatalf("the control route must NOT bind a shell: it works through the intents:\n%s", control)
	}
}

// The OPERATOR credential gates the fixing half. With none, a house that can be
// used but not repaired is a configuration rather than a half-install: no ops
// profile and no ops route.
func TestHaWithoutOperatorCredentialShipsNoFixingLane(t *testing.T) {
	out := helmTemplate(t,
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.controlSecret=ha-control",
		"--set", "home-assistant.pipelines.enabled=true")
	for _, doc := range splitDocs(out) {
		if !strings.Contains(doc, "agentops-home-assistant") {
			continue
		}
		if strings.Contains(doc, "\nkind: AgentProfile\n") && strings.Contains(doc, "name: ha-operator") {
			t.Fatal("no operator credential must render no operator profile")
		}
		if strings.Contains(doc, "\nkind: Pipeline\n") && strings.Contains(doc, "name: ha-ops") {
			t.Fatal("no operator credential must render no fixing route")
		}
	}
	haDoc(t, out, "Pipeline", "ha-control")
}

// A credential supplied as a TOKEN makes the bundle create the Secret, deriving
// BOTH keys from one value — which is what lets a secret manager hand this chart
// a reference instead of anyone creating a Secret by hand.
func TestHaTokenFormCreatesTheSecret(t *testing.T) {
	out := helmTemplate(t,
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.controlToken=CTOK",
		"--set", "home-assistant.homeAssistant.credentials.operatorToken=OTOK")
	for _, tc := range []struct{ name, token string }{
		{"agentops-ha-control", "CTOK"},
		{"agentops-ha-operator", "OTOK"},
	} {
		doc := haDoc(t, out, "Secret", tc.name)
		if !strings.Contains(doc, "token: "+strconv.Quote(tc.token)) {
			t.Errorf("%s must carry the raw token:\n%s", tc.name, doc)
		}
		// The header value is substituted WHOLE, so the scheme lives inside the
		// Secret rather than in front of it.
		if !strings.Contains(doc, "authorization: "+strconv.Quote("Bearer "+tc.token)) {
			t.Errorf("%s must derive the complete header value:\n%s", tc.name, doc)
		}
	}
	// The MCP path authenticates as CONTROL: every Assist intent is within that
	// user's rights, so defaulting to the operator would widen the shared path
	// and buy no capability.
	cfg := haDoc(t, out, "MCPConfig", "ha-api")
	if !strings.Contains(cfg, "name: agentops-ha-control") {
		t.Fatalf("the MCP path must authenticate as the control user:\n%s", cfg)
	}
}

// Two sources for one token is ambiguous, so it fails the render rather than
// picking one.
func TestHaBothCredentialFormsIsRefused(t *testing.T) {
	msg := helmTemplateErr(t,
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.controlSecret=ha-control",
		"--set", "home-assistant.homeAssistant.credentials.controlToken=CTOK")
	if !strings.Contains(msg, "not both") {
		t.Fatalf("expected the ambiguity to be refused, got:\n%s", msg)
	}
}

// The MCP server key is FIXED. It IS the mcp__homeassistant__* namespace named
// in every allowlist, so a values path here would let a rename silently strip an
// agent's tools instead of failing.
func TestHaMCPServerKeyIsFixed(t *testing.T) {
	out := helmTemplate(t, haArgs()...)
	cfg := stripComments(haDoc(t, out, "MCPConfig", "ha-api"))
	if !strings.Contains(cfg, "\n    homeassistant:\n") {
		t.Fatalf("the MCP server key must be homeassistant:\n%s", cfg)
	}
	if !strings.Contains(cfg, "/mcp_server/sse") {
		t.Fatalf("an empty url must default onto Home Assistant's own MCP endpoint:\n%s", cfg)
	}
}

// Tools are ENUMERATED, never wildcarded. A server-wide wildcard spans both
// halves of the risk split and defeats it — which is exactly what the split
// replaces.
func TestHaToolsetsAreEnumerated(t *testing.T) {
	out := helmTemplate(t, haArgs()...)
	for _, name := range []string{"ha-observability", "ha-actions"} {
		doc := stripComments(haDoc(t, out, "MCPToolset", name))
		if strings.Contains(doc, "*") {
			t.Fatalf("toolset %s must enumerate its tools, found a wildcard:\n%s", name, doc)
		}
		if !strings.Contains(doc, "mcp__homeassistant__") {
			t.Fatalf("toolset %s names no Home Assistant tools:\n%s", name, doc)
		}
	}
}

// The default rule set's SHAPE, pinned for the same reasons as the cluster
// events one: the tuning stays editable, the two properties that make it safe do
// not.
func TestHaDefaultRulesShape(t *testing.T) {
	out := helmTemplate(t, haArgs()...)
	src := stripComments(haDoc(t, out, "SignalSource", "ha-logs"))

	blocks := haRuleBlocks(src)
	if len(blocks) == 0 {
		t.Fatal("no default rules rendered")
	}

	// Anything describing something that ALREADY happened must never dwell: a
	// re-check would find the recovered house and erase the incident.
	for _, pastTense := range []string{"Error executing script", `level="CRITICAL"`} {
		var line string
		for _, b := range blocks {
			if strings.Contains(b, pastTense) {
				line = b
				break
			}
		}
		if line == "" {
			t.Errorf("past-tense condition %q is not covered by any rule", pastTense)
			continue
		}
		if !strings.Contains(line, `for: "0"`) {
			t.Errorf("past-tense condition %q must carry for: \"0\", got rule:\n%s", pastTense, line)
		}
	}

	last := blocks[len(blocks)-1]
	if !strings.Contains(last, "matchers: []") {
		t.Fatalf("the last rule must be a catch-all:\n%s", last)
	}
	if strings.Contains(last, "action: drop") {
		t.Fatalf("the catch-all must never be a drop:\n%s", last)
	}
	if !strings.Contains(last, "for:") {
		t.Fatalf("the catch-all must carry a dwell:\n%s", last)
	}

	// Grouping by integration, never per record: one broken hub logs from
	// several code paths and is one conversation.
	if !strings.Contains(src, "- integration") {
		t.Fatalf("the log source must group by integration:\n%s", src)
	}
	// This adapter implements no time axis, so a window here would be a key it
	// rejects rather than one that silently never fires.
	for _, absent := range []string{"timeIntervals", "muteTimeIntervals"} {
		if strings.Contains(src, absent) {
			t.Errorf("the log source must declare no %s:\n%s", absent, src)
		}
	}
}

// The REST path is per ROUTE, and an explicit value still moves both. Its
// default asymmetry is the design: configuration is what the ops agent repairs,
// and no Assist intent reaches configuration.
func TestHaRestAccessIsPerRoute(t *testing.T) {
	shell := "- name: agentops-shell"

	derived := helmTemplate(t, haArgs("--set", "home-assistant.pipelines.enabled=true")...)
	if !strings.Contains(stripComments(haDoc(t, derived, "Pipeline", "ha-ops")), shell) {
		t.Error("the ops route needs the REST path to reconfigure anything")
	}
	if strings.Contains(stripComments(haDoc(t, derived, "Pipeline", "ha-control")), shell) {
		t.Error("the everyday route must not get its token's whole surface by default")
	}

	off := helmTemplate(t, haArgs(
		"--set", "home-assistant.pipelines.enabled=true",
		"--set", "home-assistant.pipelines.restAccess=false")...)
	if strings.Contains(stripComments(haDoc(t, off, "Pipeline", "ha-ops")), shell) {
		t.Error("an explicit false must take the REST path away from the ops route too")
	}
}

// Profiles carry identity ONLY. allowedTools and mcp were deleted from this CRD
// once already, and re-adding them here is the mistake to catch at render time.
func TestHaProfilesCarryNoCapabilities(t *testing.T) {
	out := helmTemplate(t, haArgs()...)
	for _, name := range []string{"ha-user", "ha-operator"} {
		doc := stripComments(haDoc(t, out, "AgentProfile", name))
		for _, forbidden := range []string{"allowedTools", "mcpConfigs", "toolsets"} {
			if strings.Contains(doc, forbidden) {
				t.Errorf("profile %s must carry no capabilities, found %q:\n%s", name, forbidden, doc)
			}
		}
		if !strings.Contains(doc, "HA_URL") {
			t.Errorf("profile %s must carry its connectivity env:\n%s", name, doc)
		}
	}
}

// The adapter's data source is the house, not the cluster, so it names no
// account and inherits the floor — an identity denied every verb. Naming one
// would be claiming a grant it has no use for.
func TestHaAdapterNamesNoIdentity(t *testing.T) {
	out := helmTemplate(t, haArgs()...)
	doc := stripComments(haDoc(t, out, "SignalAdapter", "home-assistant"))
	if strings.Contains(doc, "serviceAccountName:") {
		t.Fatalf("the ha adapter reaches no Kubernetes API and must name no account:\n%s", doc)
	}
	if !strings.Contains(doc, "singleton: true") {
		t.Fatalf("two sessions would post every record twice:\n%s", doc)
	}
	// credentialKeys entries key on `key`. Spelling it `name` renders, passes
	// every template assertion, and is REJECTED by the API server on apply —
	// which is how it was found.
	if !strings.Contains(doc, "- key: token") {
		t.Fatalf("credentialKeys entries must use `key`, not `name`:\n%s", doc)
	}
}

// haRuleBlocks splits the rendered `rules:` list into its entries.
func haRuleBlocks(src string) []string {
	_, after, ok := strings.Cut(src, "\n    rules:\n")
	if !ok {
		return nil
	}
	body, _, _ := strings.Cut(after, "\n    route:")
	var out []string
	for _, chunk := range strings.Split(body, "\n    - ") {
		if strings.TrimSpace(chunk) != "" {
			out = append(out, chunk)
		}
	}
	return out
}

// The ADMIN MCP path exists because Home Assistant's built-in server exposes
// Assist intents only: it cannot read a log, reload an integration, or disable
// an entity. Those live in registries served over the WebSocket API, and an ops
// agent without this component hands the job back.
func TestHaAdminMcpIsOffByDefault(t *testing.T) {
	out := helmTemplate(t, haArgs("--set", "home-assistant.pipelines.enabled=true")...)
	for _, absent := range []string{"ha-admin-api", "agentops-mcp-ha"} {
		if strings.Contains(out, absent) {
			t.Fatalf("the admin MCP path is opt-in, found %q", absent)
		}
	}
}

// Two servers, split by lane. The control route reaches ONE server that cannot
// touch configuration, so the split is a wall rather than an allowlist.
func TestHaAdminMcpIsBoundToTheOpsRouteOnly(t *testing.T) {
	out := helmTemplate(t, haArgs(
		"--set", "home-assistant.pipelines.enabled=true",
		"--set", "home-assistant.adminMcp.enabled=true",
		"--set", "home-assistant.adminMcpServer.enabled=true")...)

	control := stripComments(haDoc(t, out, "Pipeline", "ha-control"))
	ops := stripComments(haDoc(t, out, "Pipeline", "ha-ops"))
	for _, admin := range []string{"ha-admin-api", "- name: ha-admin"} {
		if strings.Contains(control, admin) {
			t.Fatalf("the control route must not reach the admin server, found %q:\n%s", admin, control)
		}
		if !strings.Contains(ops, admin) {
			t.Fatalf("the ops route must bind %q:\n%s", admin, ops)
		}
	}

	// Distinct server keys: two servers with two tool vocabularies, so one key
	// for both would make an allowlist entry mean whichever happened to bind.
	cfg := stripComments(haDoc(t, out, "MCPConfig", "ha-admin-api"))
	if !strings.Contains(cfg, "\n    homeassistant_admin:\n") {
		t.Fatalf("the admin server key must be homeassistant_admin:\n%s", cfg)
	}
}

// The tools are enumerated from the server's real list. A wildcard would grant
// restarting Home Assistant, deleting registry objects and installing HACS
// packages in one line.
func TestHaAdminToolsetIsEnumeratedAndWithholdsTheDestructive(t *testing.T) {
	out := helmTemplate(t, haArgs(
		"--set", "home-assistant.adminMcp.enabled=true",
		"--set", "home-assistant.adminMcpServer.enabled=true")...)
	doc := stripComments(haDoc(t, out, "MCPToolset", "ha-admin"))

	if strings.Contains(doc, "*") {
		t.Fatalf("the admin toolset must enumerate:\n%s", doc)
	}
	// The tools that answer the failures this component exists for.
	for _, want := range []string{"ha_set_entity", "ha_set_integration", "ha_get_logs", "ha_reload_core"} {
		if !strings.Contains(doc, "mcp__homeassistant_admin__"+want) {
			t.Errorf("the admin toolset must grant %s — it is why this component exists", want)
		}
	}
	// Withheld by default: these restart Home Assistant, delete registry
	// objects, or install software. Adding one is a values decision.
	for _, absent := range []string{"ha_restart", "ha_manage_backup", "ha_remove_entity", "ha_manage_hacs"} {
		if strings.Contains(doc, "mcp__homeassistant_admin__"+absent) {
			t.Errorf("%s must not ship in the default allowlist", absent)
		}
	}
}

// Either the bundle deploys a server or it points at one. Enabled with neither
// renders an MCPConfig aimed at nothing, which costs the agent its tools and
// looks installed — so it fails instead.
func TestHaAdminMcpNeedsAServer(t *testing.T) {
	msg := helmTemplateErr(t, haArgs("--set", "home-assistant.adminMcp.enabled=true")...)
	if !strings.Contains(msg, "no server to reach") {
		t.Fatalf("expected the missing-server guard, got:\n%s", msg)
	}
}

// Pointing at a server you already run — a HACS MCP integration inside Home
// Assistant, say — renders the CRs and no workload.
func TestHaAdminMcpCanUseAnExistingServer(t *testing.T) {
	out := helmTemplate(t, haArgs(
		"--set", "home-assistant.adminMcp.enabled=true",
		"--set", "home-assistant.adminMcp.url=https://ha.example.org/api/mcp")...)
	cfg := stripComments(haDoc(t, out, "MCPConfig", "ha-admin-api"))
	if !strings.Contains(cfg, "https://ha.example.org/api/mcp") {
		t.Fatalf("the configured url must win:\n%s", cfg)
	}
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: Deployment\n") && strings.Contains(doc, "agentops-mcp-ha") {
			t.Fatal("pointing at an existing server must render no workload")
		}
	}
}

// The server holds a credential that can change the whole house, so it runs
// under its OWN identity — the same two-wall argument kubernetes's server makes.
func TestHaAdminMcpServerHasItsOwnIdentity(t *testing.T) {
	out := helmTemplate(t, haArgs(
		"--set", "home-assistant.adminMcp.enabled=true",
		"--set", "home-assistant.adminMcpServer.enabled=true")...)
	dep := haDoc(t, out, "Deployment", "agentops-mcp-ha")
	// IT RENDERS NO ACCOUNT AND NAMES NONE. This server talks to Home Assistant
	// over HTTP and mounts no token, so the identity it runs as is never
	// presented to the API server — an account here would be a name, not a
	// wall. Its two walls are the Home Assistant TOKEN and the bound toolset.
	if strings.Contains(dep, "serviceAccountName:") {
		t.Fatalf("a pod that mounts no token gains nothing from naming an account:\n%s", dep)
	}
	if !strings.Contains(dep, "automountServiceAccountToken: false") {
		t.Fatalf("the server must mount no token:\n%s", dep)
	}
	for _, doc := range strings.Split(out, "\n---") {
		if strings.Contains(doc, "\nkind: ServiceAccount\n") && strings.Contains(doc, "agentops-mcp-ha") {
			t.Fatalf("no account is rendered for a workload that never authenticates:\n%s", doc)
		}
	}
	// Env var NAMES are read off the image, not its documentation — the docs
	// describe the Home Assistant ADD-ON, a different deployment.
	for _, env := range []string{"HOMEASSISTANT_URL", "HOMEASSISTANT_TOKEN", "MCP_PORT", "MCP_SECRET_PATH"} {
		if !strings.Contains(dep, "name: "+env) {
			t.Errorf("the server needs %s", env)
		}
	}
	if !strings.Contains(dep, `command: ["ha-mcp-web"]`) {
		t.Errorf("ha-mcp-web is the env-driven HTTP entry point:\n%s", dep)
	}

	msg := helmTemplateErr(t, haArgs(
		"--set", "home-assistant.adminMcp.enabled=true",
		"--set", "home-assistant.adminMcpServer.enabled=true",
		"--set", "home-assistant.adminMcpServer.serviceAccountName=shared",
		"--set", "global.agentops.runtimeDefaults.serviceAccountName=shared")...)
	if !strings.Contains(msg, "must NOT be the runtime") {
		t.Fatalf("sharing the runtime identity collapses the two walls, got:\n%s", msg)
	}
}

// EVERY CHART-SHIPPED PROFILE DECLARES ITS OUTPUT CONTRACT.
//
// The field is REQUIRED on the CR with no default, so a template that forgets
// it renders a profile the API refuses. That failure is at apply time, on an
// install, which is exactly where a render test is cheaper.
func TestEveryShippedProfileDeclaresItsOutputFormat(t *testing.T) {
	out := helmTemplate(t,
		"--set", "kubernetes.enabled=true",
		"--set", "prometheus.enabled=true",
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.operatorToken=t",
	)
	profiles := []string{"k8s-engineer", "alert-investigator", "ha-user", "ha-operator"}
	for _, name := range profiles {
		if !strings.Contains(out, "name: "+name) {
			t.Fatalf("%s is not in the render — this test would pass vacuously", name)
		}
	}
	// One declaration per profile, and all of them `blocks`: the shipped agents
	// are written against the shared specification.
	if got := strings.Count(out, "outputFormat: blocks"); got != len(profiles) {
		t.Errorf("got %d outputFormat declarations, want %d", got, len(profiles))
	}
}

// An install may decline it per profile, which is what `none` is for.
func TestOutputFormatCanBeDeclined(t *testing.T) {
	out := helmTemplate(t,
		"--set", "kubernetes.enabled=true",
		"--set", "kubernetes.profile.outputFormat=none",
	)
	if !strings.Contains(out, "outputFormat: none") {
		t.Fatal("an install must be able to decline the shared format")
	}
}

// RWX IS THE SHIPPED DEFAULT ON BOTH VOLUMES, AND NOTHING CHECKED IT.
//
// It is the mode a MULTI-NODE install needs: concurrent conversations mount one
// claim at the same time, and ReadWriteOnce binds a volume to ONE node, so the
// second conversation to schedule elsewhere fails to attach at the moment of
// concurrency — far from the setting that caused it.
//
// The reason this went unpinned is the reason it needed pinning: the local demo
// OVERRIDES both volumes to ReadWriteOnce, because k3s local-path offers
// nothing else. So the shipped default was exercised by no test and no install
// anyone ran, and a template edit could have quietly dropped it to RWO for
// everybody.
//
// This pins the DEFAULT. It cannot prove RWX attaches — only a multi-node
// cluster with an RWX provisioner does that.
func TestBothVolumesDefaultToReadWriteMany(t *testing.T) {
	out := helmTemplate(t, "--set", "persistence.workspace.enabled=true")

	for _, claim := range []string{"agentops-context", "agentops-workspace"} {
		doc := claimDoc(t, out, claim)
		if !strings.Contains(doc, "- ReadWriteMany") {
			t.Errorf("%s does not default to ReadWriteMany. Concurrent conversations mount one claim "+
				"at once, so ReadWriteOnce fails the SECOND one to land on another node:\n%s", claim, doc)
		}
		if strings.Contains(doc, "ReadWriteOnce") {
			t.Errorf("%s ships ReadWriteOnce by default:\n%s", claim, doc)
		}
	}
}

// ...and the single-node escape hatch stays available, because the spec makes it
// a SUPPORTED configuration rather than a downgrade: "a ReadWriteOnce claim...
// with runtime pods pinned to that node SHALL be a documented, supported way"
// to have durable context.
//
// What must never be silent is RWO with NOTHING PINNED, which is the shape that
// works until concurrency and then fails on attachment. The chart reports that
// in its notes — pinned elsewhere in this file.
func TestReadWriteOnceRemainsExpressible(t *testing.T) {
	doc := claimDoc(t, helmTemplate(t,
		"--set", "persistence.context.accessModes={ReadWriteOnce}"), "agentops-context")

	if !strings.Contains(doc, "- ReadWriteOnce") {
		t.Errorf("a single-node install must still be able to ask for ReadWriteOnce:\n%s", doc)
	}
}

// DEMO MODE ASKS FOR ReadWriteOnce, AND THAT IS WHAT MAKES THE ONE-FLAG DEMO
// WORK ON THE CLUSTERS IT IS FOR.
//
// `local-path` is the only storage class rancher-desktop, k3d, kind and
// minikube ship, and it refuses an RWX claim outright — so the demo's context
// claim sat Pending, no runtime pod was created, and the conversation waited
// forever. The documented workaround turned persistence OFF, buying a working
// demo by removing the thing being demonstrated.
//
// The default is EMPTY rather than a mode because an empty list is the one
// thing a chart can tell apart from a value somebody typed, which is what keeps
// an explicit ReadWriteMany working under demo mode too.
func TestDemoModeAsksForReadWriteOnce(t *testing.T) {
	demo := claimDoc(t, helmTemplate(t, "--set", "global.demo.enabled=true"), "agentops-context")
	if !strings.Contains(demo, "- ReadWriteOnce") {
		t.Errorf("demo mode must render a ReadWriteOnce context claim:\n%s", demo)
	}

	ordinary := claimDoc(t, helmTemplate(t), "agentops-context")
	if !strings.Contains(ordinary, "- ReadWriteMany") {
		t.Errorf("an ordinary install must be unchanged at ReadWriteMany:\n%s", ordinary)
	}

	typed := claimDoc(t, helmTemplate(t,
		"--set", "global.demo.enabled=true",
		"--set", "persistence.context.accessModes={ReadWriteMany}"), "agentops-context")
	if !strings.Contains(typed, "- ReadWriteMany") {
		t.Errorf("an explicit mode must win under demo mode:\n%s", typed)
	}
}

// The ollama bundle is the SECOND vendor runtime, in the claude bundle's exact
// shape: off by default, one AgentRuntime through the parent's renderer, no
// substrate. Enabled, it inherits the defaults and carries only what names the
// vendor — the image, the endpoint and model as env, and its own sync paths.
func TestOllamaBundleRendersOneRuntimeAndNoSubstrate(t *testing.T) {
	out := helmTemplate(t, "--set", "ollama.enabled=true",
		"--set", "ollama.endpoint=http://ollama.ollama.svc:11434",
		"--set", "ollama.model=qwen2.5:14b")
	if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 3 {
		t.Fatalf("want claude, ollama and the default copy, got %d", n)
	}
	var rt string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: AgentRuntime\n") && strings.Contains(doc, "\n  name: ollama\n") {
			rt = doc
		}
	}
	if rt == "" {
		t.Fatal("no AgentRuntime named ollama rendered")
	}
	for _, want := range []string{
		`image: "ghcr.io/kostiantyn-matsebora/agentops-runtime-ollama:`,
		"serviceAccountName: agentops-runtime\n", // the floor, inherited
		"contextStorage: volume\n",
		"idleTtlMinutes: 1\n", // the release default, not the CRD's 10
		"- .agentops/contexts/**\n",
		"name: OLLAMA_URL\n      value: http://ollama.ollama.svc:11434\n",
		"name: OLLAMA_MODEL\n      value: qwen2.5:14b\n",
		"name: OLLAMA_NUM_CTX\n      value: \"8192\"\n",
	} {
		if !strings.Contains(rt, want) {
			t.Errorf("ollama runtime lacks %q:\n%s", want, rt)
		}
	}
	if strings.Contains(rt, ".claude/projects") {
		t.Error("the ollama runtime must not inherit claude-code's sync paths")
	}
	// no substrate: the ServiceAccount count is unchanged from the default render
	if got, want := strings.Count(out, "kind: ServiceAccount\n"), strings.Count(helmTemplate(t), "kind: ServiceAccount\n"); got != want {
		t.Errorf("the bundle must render no ServiceAccount: %d vs %d", got, want)
	}
}

// Enabled without an endpoint, the render FAILS naming the key. A runtime
// pointed at nothing starts fine and fails every run, which reads as a broken
// model rather than a missing value. The MODEL is optional: unset, the runtime
// uses the server's only pulled model and fails its runs naming the choices
// when there are several.
func TestOllamaBundleRequiresEndpoint(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--set", "ollama.enabled=true"}, "ollama.endpoint is required"},
	} {
		if out := helmTemplateErr(t, tc.args...); !strings.Contains(out, tc.want) {
			t.Errorf("%v: want %q, got %s", tc.args, tc.want, out)
		}
	}
}

// WHICH RUNTIME IS `default` IS A FLAG, OR THE FIRST CONFIGURED. Every runtime
// renders under its own name and the parent renders one more CR named
// `default`, a copy of the flagged one, annotated with its source. One runtime
// alone is therefore always the default; the claude bundle off and the ollama
// one on needs no rename; two flags is refused.
func TestDefaultRuntimeIsTheFlaggedOrFirstConfigured(t *testing.T) {
	ollama := []string{"--set", "ollama.enabled=true",
		"--set", "ollama.endpoint=http://ollama.ollama.svc:11434",
		"--set", "ollama.model=qwen2.5:14b",
		"--set", "global.demo.enabled=true"} // a route naming no runtimeRef
	defaultOf := func(out string) (string, string) {
		for _, doc := range splitDocs(out) {
			if strings.Contains(doc, "kind: AgentRuntime\n") && strings.Contains(doc, "\n  name: default\n") {
				src := ""
				for _, line := range strings.Split(doc, "\n") {
					if strings.Contains(line, "agentops.dev/default-of:") {
						src = strings.Trim(strings.TrimSpace(strings.SplitN(line, ":", 2)[1]), `"`)
					}
				}
				return doc, src
			}
		}
		return "", ""
	}
	// ollama alone: it is the default, under its own name plus the copy
	out := helmTemplate(t, append(ollama, "--set", "claude.enabled=false")...)
	if doc, src := defaultOf(out); src != "ollama" || !strings.Contains(doc, "name: OLLAMA_URL") {
		t.Errorf("ollama alone must be copied as default, got source %q", src)
	}
	if strings.Count(out, "\nkind: AgentRuntime\n") != 2 {
		t.Error("ollama alone renders exactly ollama and default")
	}
	// both, none flagged: the first configured — claude — is the default
	if _, src := defaultOf(helmTemplate(t, ollama...)); src != "claude" {
		t.Errorf("with both and no flag the first configured is default, got %q", src)
	}
	// both, ollama flagged: the flag wins
	if doc, src := defaultOf(helmTemplate(t, append(ollama, "--set", "ollama.default=true")...)); src != "ollama" || strings.Contains(doc, "kind: Secret") {
		t.Errorf("the flag must move the default, got %q", src)
	}
	// the copy carries no Secret of its own, even when the source has a token
	if out := helmTemplate(t, "--set", "claude.credentialsSecret.token=x"); strings.Count(out, "kind: Secret\nmetadata:\n  name: agentops-claude\n") != 1 {
		t.Error("the default copy must not render a second credential Secret")
	}
	// two flags: refused by name
	two := append(ollama, "--set", "ollama.default=true", "--set", "claude.default=true")
	if out := helmTemplateErr(t, two...); !strings.Contains(out, "2 runtimes are flagged") {
		t.Errorf("two flagged defaults must fail naming both, got %s", out)
	}
	// nothing declared and a route needing default: still refused
	if out := helmTemplateErr(t, "--set", "claude.enabled=false", "--set", "global.demo.enabled=true"); !strings.Contains(out, "Declared runtimes: (none)") {
		t.Errorf("no runtime at all must fail, got %s", out)
	}
}

// The copilot bundle is the THIRD vendor runtime, in the ollama bundle's exact
// shape: off by default, one AgentRuntime through the parent's renderer, no
// substrate. Enabled, it inherits the defaults and carries only what names the
// vendor — the image, the credential as env, the model and credit ceiling as
// env, and its own sync paths.
func TestCopilotBundleRendersOneRuntimeAndNoSubstrate(t *testing.T) {
	out := helmTemplate(t, "--set", "copilot.enabled=true",
		"--set", "copilot.credentialsSecret.token=ghp_placeholder",
		"--set", "copilot.model=gpt-5",
		"--set", "copilot.maxAiCredits=50")
	if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 3 {
		t.Fatalf("want claude, copilot and the default copy, got %d", n)
	}
	var rt, secret string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: AgentRuntime\n") && strings.Contains(doc, "\n  name: copilot\n") {
			rt = doc
		}
		if strings.Contains(doc, "kind: Secret\n") && strings.Contains(doc, "\n  name: agentops-copilot\n") {
			secret = doc
		}
	}
	if rt == "" {
		t.Fatal("no AgentRuntime named copilot rendered")
	}
	for _, want := range []string{
		`image: "ghcr.io/kostiantyn-matsebora/agentops-runtime-copilot:`,
		"serviceAccountName: agentops-runtime\n", // the floor, inherited
		"contextStorage: volume\n",
		"idleTtlMinutes: 1\n", // the release default, not the CRD's 10
		"- .copilot/session-state/**\n",
		"name: COPILOT_MODEL\n      value: gpt-5\n",
		"name: COPILOT_MAX_AI_CREDITS\n      value: \"50\"\n",
		"name: COPILOT_GITHUB_TOKEN\n      valueFrom:\n        secretKeyRef:\n          key: githubToken\n          name: agentops-copilot\n",
	} {
		if !strings.Contains(rt, want) {
			t.Errorf("copilot runtime lacks %q:\n%s", want, rt)
		}
	}
	if strings.Contains(rt, ".claude/projects") {
		t.Error("the copilot runtime must not inherit claude-code's sync paths")
	}
	if secret == "" || !strings.Contains(secret, "githubToken: \"ghp_placeholder\"") {
		t.Errorf("a supplied token must render the credential Secret:\n%s", secret)
	}
	// no substrate: the ServiceAccount count is unchanged from the default render
	if got, want := strings.Count(out, "kind: ServiceAccount\n"), strings.Count(helmTemplate(t), "kind: ServiceAccount\n"); got != want {
		t.Errorf("the bundle must render no ServiceAccount: %d vs %d", got, want)
	}
}

// Without a token the runtime references the Secret by name and creates
// nothing; without a model or a credit ceiling neither env entry renders, so
// the runtime's own defaults apply. The reference bundle off and this one on
// makes it the default with no rename.
func TestCopilotBundleDefaultsAndBecomesDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "copilot.enabled=true")
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: Secret\n") && strings.Contains(doc, "agentops-copilot") {
			t.Error("no token supplied, so no Secret may render")
		}
		if strings.Contains(doc, "kind: AgentRuntime\n") && strings.Contains(doc, "\n  name: copilot\n") {
			for _, absent := range []string{"COPILOT_MODEL", "COPILOT_MAX_AI_CREDITS"} {
				if strings.Contains(doc, absent) {
					t.Errorf("unset %s must not render", absent)
				}
			}
			if !strings.Contains(doc, "name: agentops-copilot\n") {
				t.Error("the runtime must reference the credential Secret by name")
			}
		}
	}
	alone := helmTemplate(t, "--set", "copilot.enabled=true", "--set", "claude.enabled=false")
	if doc, src := defaultOf(alone); src != "copilot" || !strings.Contains(doc, "COPILOT_GITHUB_TOKEN") {
		t.Errorf("copilot alone must be copied as default with its env, got source %q", src)
	}
	if strings.Count(alone, "\nkind: AgentRuntime\n") != 2 {
		t.Error("copilot alone renders exactly copilot and default")
	}
}
