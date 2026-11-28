package dispatch

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

const capTestNS = "agent-ops"

func capScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

func repoEntry(name string) agentopsv1alpha1.RepositoryEntry {
	return agentopsv1alpha1.RepositoryEntry{Name: name, URL: "https://example.com/" + name + ".git"}
}

// TestResolveRepositoriesPrecedence pins every case of "a capability's list
// replaces the profile's list, never merging" (multi-repo-workspace) — the
// rule ResolveRepositories exists to apply in one place, instead of in
// every caller that would otherwise have to remember it.
func TestResolveRepositoriesPrecedence(t *testing.T) {
	ctx := context.Background()

	capList := []agentopsv1alpha1.RepositoryEntry{repoEntry("docs")}
	capAgentFile := &agentopsv1alpha1.AgentFileRef{Path: "agents/docs.md"}
	profList := []agentopsv1alpha1.RepositoryEntry{repoEntry("app"), repoEntry("infra")}
	profAgentFile := &agentopsv1alpha1.AgentFileRef{Repo: "app", Path: "agents/app.md"}

	profileWithList := &agentopsv1alpha1.AgentProfile{}
	profileWithList.Namespace, profileWithList.Name = capTestNS, "prof-list"
	profileWithList.Spec.Repositories = profList
	profileWithList.Spec.AgentFile = profAgentFile
	profileWithList.Spec.WorkingDir = "infra"

	profileDeprecated := &agentopsv1alpha1.AgentProfile{}
	profileDeprecated.Namespace, profileDeprecated.Name = capTestNS, "prof-deprecated"
	profileDeprecated.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/legacy.git", Ref: "main"}

	profileEmpty := &agentopsv1alpha1.AgentProfile{}
	profileEmpty.Namespace, profileEmpty.Name = capTestNS, "prof-empty"

	c := fake.NewClientBuilder().WithScheme(capScheme(t)).
		WithObjects(profileWithList, profileDeprecated, profileEmpty).Build()

	cases := []struct {
		name       string
		capability agentopsv1alpha1.AgentCapabilitySpec
		want       ResolvedRepositories
	}{
		{
			name: "capability list only",
			capability: agentopsv1alpha1.AgentCapabilitySpec{
				Repositories: capList, AgentFile: capAgentFile, WorkingDir: "docs",
			},
			want: ResolvedRepositories{Repositories: capList, AgentFile: capAgentFile, WorkingDir: "docs"},
		},
		{
			name: "profile list only",
			capability: agentopsv1alpha1.AgentCapabilitySpec{
				ProfileRef: &agentopsv1alpha1.ObjectRef{Name: "prof-list"},
			},
			want: ResolvedRepositories{Repositories: profList, AgentFile: profAgentFile, WorkingDir: "infra"},
		},
		{
			name: "profile's deprecated repository only",
			capability: agentopsv1alpha1.AgentCapabilitySpec{
				ProfileRef: &agentopsv1alpha1.ObjectRef{Name: "prof-deprecated"},
			},
			want: ResolvedRepositories{Repositories: []agentopsv1alpha1.RepositoryEntry{
				{Name: "default", URL: "https://example.com/legacy.git", Ref: "main"},
			}},
		},
		{
			name: "both declared: the capability wins",
			capability: agentopsv1alpha1.AgentCapabilitySpec{
				Repositories: capList, AgentFile: capAgentFile, WorkingDir: "docs",
				ProfileRef: &agentopsv1alpha1.ObjectRef{Name: "prof-list"},
			},
			want: ResolvedRepositories{Repositories: capList, AgentFile: capAgentFile, WorkingDir: "docs"},
		},
		{
			name:       "neither declared: no checkout",
			capability: agentopsv1alpha1.AgentCapabilitySpec{ProfileRef: &agentopsv1alpha1.ObjectRef{Name: "prof-empty"}},
			want:       ResolvedRepositories{},
		},
		{
			name:       "no profile ref and no capability list: no checkout",
			capability: agentopsv1alpha1.AgentCapabilitySpec{},
			want:       ResolvedRepositories{},
		},
		{
			// "declares a list" means the field is PRESENT, not that it is
			// non-empty — an explicit empty list still replaces a non-empty
			// profile list outright, exactly as a non-empty one would.
			name: "capability declares an explicit empty list: still wins",
			capability: agentopsv1alpha1.AgentCapabilitySpec{
				Repositories: []agentopsv1alpha1.RepositoryEntry{},
				ProfileRef:   &agentopsv1alpha1.ObjectRef{Name: "prof-list"},
			},
			want: ResolvedRepositories{Repositories: []agentopsv1alpha1.RepositoryEntry{}},
		},
		{
			name: "a dangling profileRef resolves as no list, not an error",
			capability: agentopsv1alpha1.AgentCapabilitySpec{
				ProfileRef: &agentopsv1alpha1.ObjectRef{Name: "does-not-exist"},
			},
			want: ResolvedRepositories{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveRepositories(ctx, c, capTestNS, tc.capability)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// AgentCapabilitySpec carries no deprecated singular `repository` field of
// its own (api/v1alpha1/agentcapability_types.go) — only AgentProfileSpec
// does. The task list names "capability's deprecated repository only" as a
// precedence case, but there is no such field to declare it with; this test
// documents that absence rather than silently skipping the case.
func TestCapabilityHasNoDeprecatedSingularRepositoryField(t *testing.T) {
	spec := agentopsv1alpha1.AgentCapabilitySpec{}
	v := reflect.ValueOf(spec)
	if f := v.FieldByName("Repository"); f.IsValid() {
		t.Fatal("AgentCapabilitySpec grew a deprecated singular Repository field — " +
			"add the precedence case this test documents as missing")
	}
}

func TestRepositoriesOfAndAgentFileOf(t *testing.T) {
	if got := RepositoriesOf(nil); got != nil {
		t.Errorf("nil in must give nil out, got %+v", got)
	}
	if got := AgentFileOf(nil); got != nil {
		t.Errorf("nil in must give nil out, got %+v", got)
	}

	keepFalse := false
	entries := []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git", Ref: "main"},
		{
			Name: "infra", URL: "https://example.com/infra.git", Ref: "release",
			Auth:    &agentopsv1alpha1.RepoAuth{Type: agentopsv1alpha1.RepoAuthHTTPS, SecretRef: agentopsv1alpha1.ObjectRef{Name: "infra-token"}},
			KeepGit: &keepFalse,
		},
	}
	got := RepositoriesOf(entries)
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0].Name != "app" || !got[0].KeepGit {
		t.Errorf("app entry: %+v (KeepGit absent must resolve true)", got[0])
	}
	if got[1].Auth == nil || got[1].Auth.Type != "https" || got[1].Auth.SecretRef != "infra-token" {
		t.Errorf("infra entry's auth must carry the secret name, never the credential: %+v", got[1].Auth)
	}
	if got[1].KeepGit {
		t.Errorf("infra entry's keepGit=false must resolve false, got true")
	}

	file := AgentFileOf(&agentopsv1alpha1.AgentFileRef{Repo: "infra", Path: "agents/ops.md"})
	if file == nil || file.Repo != "infra" || file.Path != "agents/ops.md" {
		t.Errorf("got %+v", file)
	}
}
