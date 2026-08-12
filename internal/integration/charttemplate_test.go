package integration

import (
	"encoding/base64"
	"os/exec"
	"sort"
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
	cmd := exec.Command("helm", append([]string{"template", "test", "../../chart"}, args...)...)
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
	cmd := exec.Command("helm", append([]string{"template", "test", "../../chart"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helm template unexpectedly succeeded:\n%s", out)
	}
	return string(out)
}

// The console is ON by default since chart 5.0.0, so the opt-out is what needs
// pinning: ONE value must remove every console object, or the "nothing about
// your install changes" promise in CHANGELOG.md is not true.
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
		"kubernetesAccess: true",
		"singleton: true",
		"port: 8080",
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

// ---- k8s-bundle events lane -------------------------------------------------

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
			out := helmTemplate(t, "--set", "k8s-bundle.enabled=true",
				"--set", "k8s-bundle.eventsAdapter.rbac.clusterWide="+mode.flag)

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
	out := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
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
	out := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
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
	out := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
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
		"--set", "k8s-bundle.enabled=true",
		"--set", "k8s-bundle.eventsAdapter.source.route.timeIntervals[0].name=nightly",
		"--set", "k8s-bundle.eventsAdapter.source.route.timeIntervals[0].location=Europe/Kyiv",
		"--set", "k8s-bundle.eventsAdapter.source.route.timeIntervals[0].times[0].startTime=04:00",
		"--set", "k8s-bundle.eventsAdapter.source.route.timeIntervals[0].times[0].endTime=04:20",
		"--set", "k8s-bundle.eventsAdapter.source.route.muteTimeIntervals[0].name=nightly",
		"--set", `k8s-bundle.eventsAdapter.source.route.muteTimeIntervals[0].matchers[0]=reason="NodeNotReady"`,
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
		{"--set", "k8s-bundle.enabled=true"},
		{"--set", "telegram-bundle.enabled=true"},
		{"--set", "k8s-bundle.enabled=true", "--set", "telegram-bundle.enabled=true"},
	} {
		name := "defaults"
		if len(combo) > 0 {
			name = strings.Join(combo[1:], ",")
		}
		t.Run(name, func(t *testing.T) {
			out := helmTemplate(t, combo...)
			if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 1 {
				t.Errorf("want exactly 1 AgentRuntime, got %d", n)
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

// "Bring your own runtime": the component renders nothing, but the SA stays —
// the manager defaults every runtime pod onto it whoever wrote the CR.
func TestRuntimeDisabledRendersNoRuntimeObjects(t *testing.T) {
	out := helmTemplate(t, "--set", "runtime.enabled=false",
		"--set", "runtime.credentialsSecret.token=x")
	// anchored: the CRD document names the kind too, and it ships regardless
	if strings.Contains(out, "\nkind: AgentRuntime\n") {
		t.Error("runtime.enabled=false must render no AgentRuntime")
	}
	if strings.Contains(out, "name: agentops-claude") {
		t.Error("runtime.enabled=false must render no credential Secret")
	}
	if !strings.Contains(out, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n") {
		t.Error("the runtime ServiceAccount is not part of the component and must still render")
	}
}

// The release has ONE idle-TTL number. The field must be WRITTEN, not omitted:
// AgentRuntime.spec.idleTtlMinutes carries a CRD default of 10, so an omitted
// field is stored as 10, and the manager prefers any non-zero spec value over
// RUNTIME_IDLE_TTL_M — omitting it looks right in the manifest and silently
// ignores runtimeIdleTtlMinutes in the cluster.
func TestRuntimeIdleTTLFollowsTheReleaseDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "runtimeIdleTtlMinutes=7")
	if !strings.Contains(out, "idleTtlMinutes: 7") {
		t.Error("an empty runtime.idleTtlMinutes must follow runtimeIdleTtlMinutes")
	}
	out = helmTemplate(t, "--set", "runtimeIdleTtlMinutes=7", "--set", "runtime.idleTtlMinutes=30")
	if !strings.Contains(out, "idleTtlMinutes: 30") {
		t.Error("an explicit runtime.idleTtlMinutes must win")
	}
}

// Empty rbacMode grants NOTHING outside demo mode — defaulting it to readonly
// would silently bind cluster `view` on every upgrade. `full` is never inferred.
func TestRuntimeRbacModeResolution(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    []string
		notWant []string
	}{
		{"unset grants nothing", nil, nil,
			[]string{"agentops-runtime-view", "agentops-runtime-cluster-admin"}},
		{"demo is read-only", []string{"--set", "global.demo.enabled=true"},
			[]string{"agentops-runtime-view", "agentops-runtime-cluster-ro"},
			[]string{"agentops-runtime-cluster-admin"}},
		{"none", []string{"--set", "global.agentops.runtime.rbacMode=none", "--set", "global.demo.enabled=true"}, nil,
			[]string{"agentops-runtime-view", "agentops-runtime-cluster-admin"}},
		{"full", []string{"--set", "global.agentops.runtime.rbacMode=full"},
			[]string{"agentops-runtime-cluster-admin"}, nil},
		{"targeted grants compose with the mode",
			[]string{"--set", "global.agentops.runtime.rbacMode=readonly", "--set", "rbac.runtime.bindClusterRoles={edit}"},
			[]string{"agentops-runtime-view", "agentops-runtime-edit"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmTemplate(t, tc.args...)
			for _, needle := range tc.want {
				if !strings.Contains(out, needle) {
					t.Errorf("missing %q", needle)
				}
			}
			for _, needle := range tc.notWant {
				if strings.Contains(out, needle) {
					t.Errorf("must not render %q", needle)
				}
			}
		})
	}
}

// The old key would otherwise be read by nothing, running agents under an
// identity the operator did not choose.
func TestMovedRuntimeSAKeyFails(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "serviceAccounts.runtime=agentops-runtime-k8s")
	if !strings.Contains(msg, "global.agentops.runtime.serviceAccountName") {
		t.Fatalf("the failure must name the new key:\n%s", msg)
	}
}

// ---- k8s-bundle MCP ---------------------------------------------------------

// mcp and mcpServers flip together, so the config's URL always has a Service to
// default onto. The guard exists for the combination that is genuinely broken.
func TestMCPEndpointGuardStillBites(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "k8s-bundle.enabled=true",
		"--set", "k8s-bundle.mcpServers.enabled=false")
	if !strings.Contains(msg, "mcp.url is required") {
		t.Fatalf("the endpoint guard must name the missing URL:\n%s", msg)
	}
}

// One knob configures both identities coherently: with derivation, rbacMode
// full must render the mutating toolset and a write-capable server with no
// other value set — and an explicit readOnly must still recover the separation.
func TestMCPServerDerivesFromRuntimeRbacMode(t *testing.T) {
	readOnly := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
	if !strings.Contains(readOnly, "- --read-only") {
		t.Error("default posture must be a read-only server")
	}
	if strings.Contains(readOnly, "name: k8s-admin") {
		t.Error("no mutating toolset without a server that registers those tools")
	}

	full := helmTemplate(t, "--set", "k8s-bundle.enabled=true",
		"--set", "global.agentops.runtime.rbacMode=full")
	if strings.Contains(full, "- --read-only") {
		t.Error("rbacMode=full must yield a write-capable server")
	}
	if !strings.Contains(full, "name: k8s-admin") {
		t.Error("rbacMode=full must render the mutating toolset with no other value set")
	}
	if !strings.Contains(full, "name: agentops-mcp-k8s-cluster-admin") {
		t.Error("rbacMode=full must yield a full server ServiceAccount")
	}

	recovered := helmTemplate(t, "--set", "k8s-bundle.enabled=true",
		"--set", "global.agentops.runtime.rbacMode=full",
		"--set", "k8s-bundle.mcpServers.readOnly=true")
	if !strings.Contains(recovered, "- --read-only") {
		t.Error("an explicit readOnly must win over the derivation")
	}
	if strings.Contains(recovered, "name: k8s-admin") {
		t.Error("a read-only server must not render the mutating toolset")
	}
}

// Collapsing the two identities removes the only thing this component adds
// over kubectl. The guard now compares against the release-wide SA.
func TestMCPServerRefusesTheRuntimeIdentity(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "k8s-bundle.enabled=true",
		"--set", "k8s-bundle.mcpServers.serviceAccountName=agentops-runtime")
	if !strings.Contains(msg, "global.agentops.runtime.serviceAccountName") {
		t.Fatalf("the guard must name the global key:\n%s", msg)
	}
}

// ---- k8s-bundle wiring ------------------------------------------------------

// bundlePipelines returns the Pipelines the BUNDLE rendered, by name. Anchored
// on the bundle label: an install-declared Pipeline carries
// app.kubernetes.io/name: agentops, and the CRD document names the kind too.
func bundlePipelines(rendered string) map[string]string {
	out := map[string]string{}
	for _, doc := range splitDocs(rendered) {
		if !strings.Contains(doc, "\nkind: Pipeline\n") ||
			!strings.Contains(doc, "app.kubernetes.io/name: agentops-k8s-bundle") {
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
		{"bundle enabled directly", []string{"--set", "k8s-bundle.enabled=true"}},
		{"wiring declined under demo", []string{
			"--set", "global.demo.enabled=true",
			"--set", "k8s-bundle.pipelines.enabled=false"}},
		{"no profile, no route", []string{
			"--set", "global.demo.enabled=true",
			"--set", "k8s-bundle.profile.enabled=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pipelineNames(helmTemplate(t, tc.args...)); len(got) != 0 {
				t.Errorf("the bundle must render no Pipeline here, got %v", got)
			}
		})
	}
}

// Declining the route must cost nothing else — the opt-out in CHANGELOG.md is
// one value, and it has to leave a bundle that still watches, profiles and tools.
func TestDecliningWiringLeavesTheRestOfTheBundle(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "k8s-bundle.pipelines.enabled=false")
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

// One posture, four consistent effects. Widening to `full` already drops
// --read-only, widens the server SA and renders k8s-admin; the route that binds
// it has to move with them, or `full` grants a power no route can exercise.
func TestFullModePromotesTheRouteToActing(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "global.agentops.runtime.rbacMode=full")
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-operate" {
		t.Fatalf("rbacMode=full must render exactly k8s-operate, got %v", got)
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
		"--set", "k8s-bundle.pipelines.admin.enabled=true",
		"--set", "k8s-bundle.pipelines.observe.enabled=false")
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-operate" {
		t.Fatalf("an explicit acting route must render under readonly, got %v", got)
	}
	// ...but it binds no toolset the read-only server never registered
	if doc := bundlePipelines(out)["k8s-operate"]; strings.Contains(doc, "name: k8s-admin") {
		t.Errorf("no ref to a toolset that was not rendered:\n%s", doc)
	}

	// observing route asked for under `full`
	out = helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "global.agentops.runtime.rbacMode=full",
		"--set", "k8s-bundle.pipelines.observe.enabled=true",
		"--set", "k8s-bundle.pipelines.admin.enabled=false")
	if got := pipelineNames(out); len(got) != 1 || got[0] != "k8s-observe" {
		t.Fatalf("an explicit observing route must win under full, got %v", got)
	}
}

// Two Ready Pipelines on one source is a SUPPORTED shape — sources are
// shareable and sourceConflicts was deleted. Failing the render here would be
// that guard returning one layer up.
func TestBothRoutesRenderWithoutConflict(t *testing.T) {
	out := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "k8s-bundle.pipelines.observe.enabled=true",
		"--set", "k8s-bundle.pipelines.admin.enabled=true")
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
	// no channel named: the key is ABSENT, not empty-valued
	bare := bundlePipelines(helmTemplate(t, "--set", "global.demo.enabled=true"))["k8s-observe"]
	if strings.Contains(bare, "channelRefs") {
		t.Errorf("an empty channel list must omit the key entirely:\n%s", bare)
	}

	named := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "k8s-bundle.pipelines.channels={console}")
	if doc := bundlePipelines(named)["k8s-observe"]; !strings.Contains(doc, "channelRefs:\n    - name: console") {
		t.Errorf("a named channel must reach the Pipeline:\n%s", doc)
	}

	// every component the route would reference, turned off at once
	off := helmTemplate(t, "--set", "global.demo.enabled=true",
		"--set", "k8s-bundle.mcp.enabled=false",
		"--set", "k8s-bundle.mcpServers.enabled=false",
		"--set", "global.builtinToolsets.enabled=false",
		"--set", "k8s-bundle.eventsAdapter.source.create=false")
	doc := bundlePipelines(off)["k8s-observe"]
	if doc == "" {
		t.Fatal("the route still renders — it is gated on the profile, not on tooling")
	}
	for _, dangling := range []string{"signalSourceRefs", "toolsets", "mcpConfigs"} {
		if strings.Contains(doc, dangling) {
			t.Errorf("%s must be omitted when nothing rendered it:\n%s", dangling, doc)
		}
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

// The storage defaults, and the asymmetry between them: sessions persist out of
// the box because losing them silently costs conversational history, while a
// checkout is re-cloned because a stale shared one is worse than no cache.
func TestPersistenceDefaultsHomeOnWorkspaceOff(t *testing.T) {
	out := helmTemplate(t)

	if !strings.Contains(out, "\n  name: agentops-home\n") {
		t.Error("the home claim must render by default (persistence.enabled: true)")
	}
	if strings.Contains(out, "agentops-workspace") {
		t.Error("workspace persistence must be OFF by default — a stale shared checkout is worse than a re-clone")
	}
	rt := runtimeDoc(t, out)
	if !strings.Contains(rt, "home:\n    pvcRef:\n      name: agentops-home") {
		t.Errorf("home.pvcRef must be wired from the chart's own persistence block:\n%s", rt)
	}
	if strings.Contains(rt, "workspace:") {
		t.Error("no workspace claim means the AgentRuntime declares no workspace volume")
	}
	if !strings.Contains(out, "name: HOME_PVC") {
		t.Error("the manager's HOME_PVC bootstrap default must follow the claim")
	}
	if strings.Contains(out, "name: WORKSPACE_PVC") {
		t.Error("WORKSPACE_PVC must not be set when no workspace claim exists")
	}
}

// The opt-out is the whole mitigation for a cluster with no RWX provisioner —
// it must remove the claim AND the reference, or runtime pods still wait on it.
func TestPersistenceOptOutRemovesEverything(t *testing.T) {
	out := helmTemplate(t, "--set", "persistence.enabled=false")

	if strings.Contains(out, "agentops-home") {
		t.Error("persistence.enabled=false must render no home claim and no reference to one")
	}
	if strings.Contains(out, "name: HOME_PVC") {
		t.Error("persistence.enabled=false must not set HOME_PVC")
	}
	if rt := runtimeDoc(t, out); strings.Contains(rt, "home:") {
		t.Errorf("the AgentRuntime must declare no home volume:\n%s", rt)
	}
}

// Enabling workspace persistence takes ONE value: the claim name is never
// restated by the operator, exactly as home.pvcRef already works.
func TestWorkspacePersistenceIsWiredFromOneValue(t *testing.T) {
	out := helmTemplate(t, "--set", "persistence.workspace.enabled=true")

	if !strings.Contains(out, "\n  name: agentops-workspace\n") {
		t.Error("the workspace claim must render when enabled")
	}
	// Uninstall must never destroy uncommitted agent work.
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "name: agentops-workspace") && strings.Contains(doc, "kind: PersistentVolumeClaim") {
			if !strings.Contains(doc, "helm.sh/resource-policy: keep") {
				t.Error("the workspace claim must carry the keep policy, like the home claim")
			}
		}
	}
	rt := runtimeDoc(t, out)
	if !strings.Contains(rt, "workspace:\n    pvcRef:\n      name: agentops-workspace") {
		t.Errorf("workspace.pvcRef must be wired from the chart's own values:\n%s", rt)
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
	if !strings.Contains(runtimeDoc(t, out), "name: byo-checkouts") {
		t.Error("the AgentRuntime must reference the existing claim")
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
	// crds.enabled=false because this chart ships CRDs as gated TEMPLATES, so a
	// dry-run install otherwise trips the ownership check against CRDs a real
	// release already owns. The notes do not depend on them.
	cmd := exec.Command("helm", append([]string{"install", "notes-test", "../../chart", "--dry-run", "--set", "crds.enabled=false"}, args...)...)
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
	out := helmNotes(t, "--set", "persistence.accessModes={ReadWriteOnce}")
	if !strings.Contains(out, "attached by ONE node") {
		t.Fatal("an unpinned single-attach claim must be called out in the notes")
	}

	// Pinned: the operator has said where runtime pods go, so there is nothing
	// to warn about.
	pinned := helmNotes(t, "--set", "persistence.accessModes={ReadWriteOnce}",
		"--set", `runtime.nodeSelector.kubernetes\.io/hostname=node-1`)
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
		"--set", "k8s-bundle.eventsAdapter.rbac.create=false",
		"--set", "k8s-bundle.mcpServers.rbac.create=false",
	}
	// surface.name defaults to k8s-ops, and the chat SignalSource takes it too —
	// that name is what a claiming pipeline has to list.
	chatSurface := []string{
		"--set", "telegram-bundle.enabled=true",
		"--set", "telegram-bundle.surface.enabled=true",
		"--set", "telegram-bundle.surface.chatId=-100",
		"--set", "telegram-bundle.surface.credentials.botToken=x",
	}
	for _, tc := range []struct {
		name          string
		args          []string
		want, notWant []string
	}{
		{"bundle on, nobody claims", []string{"--set", "k8s-bundle.enabled=true"},
			[]string{"ONE STEP LEFT — nothing answers cluster events"}, nil},
		{"bundle on, the install claims",
			append([]string{"--set", "k8s-bundle.enabled=true"}, claimsIt...),
			nil, []string{"ONE STEP LEFT — nothing answers cluster events", "claimed TWICE"}},
		{"demo wires it", []string{"--set", "global.demo.enabled=true"},
			[]string{"this release WIRED it", "k8s-observe"},
			[]string{"ONE STEP LEFT — nothing answers cluster events", "claimed TWICE"}},
		{"both claim it — a note, never a failure",
			append([]string{"--set", "global.demo.enabled=true"}, claimsIt...),
			[]string{"claimed TWICE", "k8s-ops"}, nil},
		// The same rule one lane over. telegram-bundle genuinely ships no
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
	out := helmNotes(t, "--set", "persistence.enabled=false")
	if !strings.Contains(out, "CANNOT BE CONTINUED") {
		t.Fatal("an install with no durable home must say conversations cannot be continued")
	}
	// ...and it names the way to have it without distributed storage.
	if !strings.Contains(out, "single-node") {
		t.Fatal("the notes must name the single-node topology as the remedy")
	}
	if strings.Contains(helmNotes(t), "CANNOT BE CONTINUED") {
		t.Fatal("the default install CAN continue conversations")
	}
}
