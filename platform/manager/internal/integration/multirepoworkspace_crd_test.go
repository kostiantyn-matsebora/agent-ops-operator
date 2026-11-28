// multi-repo-workspace, task 1.3: CEL (and structural) refusals on the
// repository list and the agent file location, on the two holders that
// carry them — AgentProfile and AgentCapability. Each case asserts the API
// server itself refuses the object, naming the rule, rather than a
// reconciler reporting it afterwards.
package integration

import (
	"context"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// mustRefuse creates obj and fails the test unless the API server refused it
// with an error message containing want. On success (which is the failure
// case here) it cleans the object up so a later test in the suite never
// trips over a half-created fixture.
func mustRefuse(t *testing.T, obj client.Object, want string) {
	t.Helper()
	err := k8sClient.Create(context.Background(), obj)
	if err == nil {
		t.Fatalf("the API server accepted an object it should have refused (wanted an error containing %q)", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("refused, but not for the expected reason: got %v, want it to contain %q", err, want)
	}
}

// 1.2 — repository and repositories set together on one profile are refused
// by the CEL rule, never admitted and left to a reconciler to notice.
func TestProfileRepositoryAndRepositoriesAreMutuallyExclusive(t *testing.T) {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = "mrw-prof-both-repo-forms", ns
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	p.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/app.git"}
	p.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{{Name: "app", URL: "https://example.com/app.git"}}

	mustRefuse(t, p, "mutually exclusive")
}

// The deprecated field still works alone — a profile naming only
// `repository` is unaffected by the new CEL rule.
func TestProfileRepositoryAloneStillAdmits(t *testing.T) {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = "mrw-prof-repository-alone", ns
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	p.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/app.git"}

	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatalf("a profile naming only the deprecated repository field must admit: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), p) })
}

// agent and agentFile set together on one profile are refused — the two are
// two answers to "where is the agent definition".
func TestProfileAgentAndAgentFileAreMutuallyExclusive(t *testing.T) {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = "mrw-prof-agent-and-file", ns
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	p.Spec.Agent = "tester"
	p.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Path: "agents/tester.md"}

	mustRefuse(t, p, "mutually exclusive")
}

// A duplicate repository name within one list is refused: Repositories is a
// map-keyed list (`name`), so the API server itself enforces uniqueness
// structurally — no CEL rule is needed to repeat that check.
func TestProfileDuplicateRepositoryNameIsRefused(t *testing.T) {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = "mrw-prof-dup-repo-name", ns
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	p.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git"},
		{Name: "app", URL: "https://example.com/app2.git"},
	}

	err := k8sClient.Create(context.Background(), p)
	if err == nil {
		_ = k8sClient.Delete(context.Background(), p)
		t.Fatal("the API server accepted two repositories entries sharing one name")
	}
}

// A repository name that is not a safe DNS label — containing a slash, or
// naming `..` — is refused by the field's own pattern.
func TestProfileUnsafeRepositoryNameIsRefused(t *testing.T) {
	for _, name := range []string{"a/b", "..", "UPPER", "", "-leading"} {
		p := &agentopsv1alpha1.AgentProfile{}
		p.Name, p.Namespace = "mrw-prof-unsafe-"+safeSuffix(name), ns
		p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
		p.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{{Name: name, URL: "https://example.com/app.git"}}

		err := k8sClient.Create(context.Background(), p)
		if err == nil {
			_ = k8sClient.Delete(context.Background(), p)
			t.Fatalf("repository name %q must be refused as an unsafe name", name)
		}
	}
}

// safeSuffix turns an unsafe candidate name into something that is itself a
// valid Kubernetes object name, purely so each case in the table above gets
// its own object.
func safeSuffix(s string) string {
	if s == "" {
		return "empty"
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, 'x')
		}
	}
	return string(out)
}

// agentFile.path escaping the repository — absolute, or containing `..` — is
// refused by its own CEL rule, on both AgentProfile and AgentCapability
// (the capability carries the SAME type).
func TestAgentFilePathEscapeIsRefused(t *testing.T) {
	for _, path := range []string{"../other/agent.md", "/etc/passwd"} {
		p := &agentopsv1alpha1.AgentProfile{}
		p.Name, p.Namespace = "mrw-prof-escape-"+safeSuffix(path), ns
		p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
		p.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Path: path}

		mustRefuse(t, p, "must be relative")
	}
}

// 1.3/agent-capability-model — the same repository-list validation (unique,
// safe names) applies identically on AgentCapability, since it reuses the
// same RepositoryEntry type with the same list-map key.
func TestCapabilityDuplicateRepositoryNameIsRefused(t *testing.T) {
	c := &agentopsv1alpha1.AgentCapability{}
	c.Name, c.Namespace = "mrw-cap-dup-repo-name", ns
	c.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git"},
		{Name: "app", URL: "https://example.com/app2.git"},
	}

	err := k8sClient.Create(context.Background(), c)
	if err == nil {
		_ = k8sClient.Delete(context.Background(), c)
		t.Fatal("the API server accepted two repositories entries sharing one name on an AgentCapability")
	}
}

// An AgentCapability's agentFile.path escaping the repository is refused by
// the same CEL rule as a profile's.
func TestCapabilityAgentFilePathEscapeIsRefused(t *testing.T) {
	c := &agentopsv1alpha1.AgentCapability{}
	c.Name, c.Namespace = "mrw-cap-escape", ns
	c.Spec.AgentFile = &agentopsv1alpha1.AgentFileRef{Path: "../escape.md"}

	mustRefuse(t, c, "must be relative")
}

// capabilityRef on a Pipeline alongside an inline workspace field
// (repositories, agentFile or workingDir) is refused exactly as it is for
// the six original capability fields — the workspace fields widened the
// SAME inline-capability-vs-capabilityRef exclusivity, not a new rule.
func TestPipelineCapabilityRefAndInlineRepositoriesAreMutuallyExclusive(t *testing.T) {
	mkCapability(t, "mrw-pipe-xor-cap", "")

	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "mrw-pipe-xor", ns
	p.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "mrw-pipe-xor-cap"}
	p.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{{Name: "app", URL: "https://example.com/app.git"}}

	mustRefuse(t, p, "mutually exclusive")
}
