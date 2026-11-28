// multi-repo-workspace, task 2.2: Build reads the Conversation's OWN
// snapshotted repository list, never the live AgentProfile — an edit to the
// profile after creation must change nothing a rebuilt pod does (design D2).
package runtimepod

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// envVar finds the worker container's env var by name, "" when absent.
func envVar(pod *corev1.Pod, name string) string {
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// envVarSet reports whether the worker container carries an env var by this
// name at all — distinct from envVar's "" for an empty-valued one.
func envVarSet(pod *corev1.Pod, name string) bool {
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == name {
			return true
		}
	}
	return false
}

// buildWithSnapshot is the test fixture's own Build call: a conversation
// carrying a snapshot, a profile that may disagree with it, and no other
// resolved config.
func buildWithSnapshot(conv *state.ConversationRecord, profile *agentopsv1alpha1.AgentProfile) *corev1.Pod {
	return Build(conv, profile, mcpcompile.Result{}, "mcp-cm", Resolved{Config: Config{Image: "img"}}, "master-key")
}

// TestBuildReadsRepositoryFromTheConversationSnapshot pins the whole
// requirement: REPO_URL/REPO_REF/GIT_* come from conv.Repositories, and a
// profile naming a DIFFERENT repository is simply never read for it.
func TestBuildReadsRepositoryFromTheConversationSnapshot(t *testing.T) {
	conv := &state.ConversationRecord{
		Name: "conv-a", ID: "id-a",
		Repositories: []state.RepositoryEntry{
			{Name: "app", URL: "https://example.com/app.git", Ref: "release-1",
				Auth: &state.RepoAuth{Type: "https", SecretRef: "app-token"}},
		},
	}
	// A profile that, if it were read, would leak a completely different
	// repository into the pod.
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/STALE.git", Ref: "main"}
	profile.Spec.Repository.Auth = &agentopsv1alpha1.RepoAuth{
		Type: agentopsv1alpha1.RepoAuthSSH, SecretRef: agentopsv1alpha1.ObjectRef{Name: "stale-key"},
	}

	pod := buildWithSnapshot(conv, profile)

	if got := envVar(pod, "REPO_URL"); got != "https://example.com/app.git" {
		t.Errorf("REPO_URL = %q, want the SNAPSHOT's url, never the profile's", got)
	}
	if got := envVar(pod, "REPO_REF"); got != "release-1" {
		t.Errorf("REPO_REF = %q, want the snapshot's ref", got)
	}
	if got := envVar(pod, "GIT_AUTH_TYPE"); got != "https" {
		t.Errorf("GIT_AUTH_TYPE = %q, want the snapshot's own auth type (https), never the profile's (ssh)", got)
	}
	if envVarSet(pod, "GIT_SSH_KEY") {
		t.Error("GIT_SSH_KEY must not appear — the snapshot's auth is https, the profile's stale ssh key must never leak in")
	}
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == "GIT_TOKEN" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			if got := e.ValueFrom.SecretKeyRef.Name; got != "app-token" {
				t.Errorf("GIT_TOKEN secret = %q, want the snapshot's own (app-token), never the profile's stale-key", got)
			}
		}
	}
}

// TestBuildWithNoSnapshottedRepositoriesLeavesRepoEnvEmpty is the zero-
// checkout case: an edit to a live profile that still names a repository
// must not leak into a conversation whose OWN snapshot resolved none.
func TestBuildWithNoSnapshottedRepositoriesLeavesRepoEnvEmpty(t *testing.T) {
	conv := &state.ConversationRecord{Name: "conv-b", ID: "id-b"}
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/not-mine.git"}

	pod := buildWithSnapshot(conv, profile)

	if got := envVar(pod, "REPO_URL"); got != "" {
		t.Errorf("REPO_URL = %q, want empty — this conversation resolved no repositories", got)
	}
	if envVarSet(pod, "GIT_AUTH_TYPE") {
		t.Error("no auth env should appear for a conversation with no snapshotted repository")
	}
}

// TestBuildIgnoresASecondRepositoryEditedInAfterCreation is "a later edit
// does not re-wire a running conversation" (multi-repo-workspace), proven
// at the pod-builder boundary: a profile that grew a SECOND, different
// repository after this conversation's own single-entry snapshot was
// written must still only ever see the one it was created with.
func TestBuildIgnoresASecondRepositoryEditedInAfterCreation(t *testing.T) {
	conv := &state.ConversationRecord{
		Name: "conv-c", ID: "id-c",
		Repositories: []state.RepositoryEntry{{Name: "app", URL: "https://example.com/app.git", Ref: "main"}},
	}
	editedProfile := &agentopsv1alpha1.AgentProfile{}
	editedProfile.Spec.Repositories = []agentopsv1alpha1.RepositoryEntry{
		{Name: "app", URL: "https://example.com/app.git", Ref: "main"},
		{Name: "infra", URL: "https://example.com/infra.git", Ref: "main"},
	}

	pod := buildWithSnapshot(conv, editedProfile)

	if got := envVar(pod, "REPO_URL"); got != "https://example.com/app.git" {
		t.Errorf("REPO_URL = %q, want the conversation's own frozen entry", got)
	}
}
