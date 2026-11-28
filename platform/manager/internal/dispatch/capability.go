package dispatch

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// CapabilityHolder is a wiring kind that carries an AgentCapability either
// INLINE or by a `capabilityRef`. Pipeline implements it; Coordinator will
// implement it the same way.
type CapabilityHolder interface {
	client.Object
	// CapabilityRef names the AgentCapability this holder references INSTEAD
	// of inlining a capability, or nil when it inlines one.
	CapabilityRef() *agentopsv1alpha1.ObjectRef
	// InlineCapability returns the holder's own embedded capability fields.
	// Meaningless when CapabilityRef is non-nil — the two are mutually
	// exclusive by CEL.
	InlineCapability() agentopsv1alpha1.AgentCapabilitySpec
}

// ResolveCapability is the ONE place a wiring kind's capability is read.
// Every reader of a Pipeline's (or a Coordinator's) profile, runtime,
// identity, tools, MCP servers or persistence calls this rather than the
// embedded fields directly — an inline capability and a referenced
// AgentCapability must resolve identically, and this is what makes that true
// in one place instead of in every caller.
func ResolveCapability(ctx context.Context, reader client.Reader, holder CapabilityHolder) (agentopsv1alpha1.AgentCapabilitySpec, error) {
	ref := holder.CapabilityRef()
	if ref == nil {
		return holder.InlineCapability(), nil
	}
	var capability agentopsv1alpha1.AgentCapability
	if err := reader.Get(ctx, types.NamespacedName{Namespace: holder.GetNamespace(), Name: ref.Name}, &capability); err != nil {
		return agentopsv1alpha1.AgentCapabilitySpec{}, err
	}
	return capability.Spec, nil
}

// ResolvedRepositories is what "a capability's list replaces the profile's
// list, never merging" (multi-repo-workspace) settles for one conversation:
// the repository list it checks out, where its agent definition lives, and
// which repository it starts in.
//
// AgentFile and WorkingDir travel with whichever list won — never resolved
// on their own, because both name entries of that list (design D1): a
// replaced list with a kept AgentFile would dangle by construction.
type ResolvedRepositories struct {
	Repositories []agentopsv1alpha1.RepositoryEntry
	AgentFile    *agentopsv1alpha1.AgentFileRef
	WorkingDir   string
}

// ResolveRepositories applies that rule to a resolved capability — the one
// ResolveCapability above already returned, whether it came from a
// Pipeline's inline fields or an AgentCapability it references.
//
// The capability's OWN Repositories wins outright the moment it is
// declared — even an explicitly empty list, which still counts as
// "declares a list" and still wins over a non-empty profile list (nil vs.
// an empty-but-present slice is exactly the distinction a declared field
// carries through JSON unmarshalling). Only when the capability names none
// at all does the profile's own list apply: its own Repositories, or — the
// deprecated one-repository shorthand — Repository as a one-entry list
// named `default`.
//
// The profile is read live, by the capability's ProfileRef, because this
// runs ONLY where re-resolving is still correct: at conversation creation
// (runtimepod.SnapshotFor's sibling for the workspace fields — every LATER
// pod build reads the conversation's own snapshot instead, never this) and
// from the Ready condition multi-repo-workspace's own validation
// requirement puts on Pipeline and Coordinator, which needs the resolved
// list to check a dangling agentFile.repo or workingDir against.
//
// A profile that cannot be read resolves as if it declared no list, rather
// than erroring: a dangling ProfileRef is already named on the holder's
// Ready condition by checkProfileRef, and duplicating that refusal here
// would be a second rule for the same fact.
func ResolveRepositories(ctx context.Context, reader client.Reader, namespace string,
	capability agentopsv1alpha1.AgentCapabilitySpec) ResolvedRepositories {

	if capability.Repositories != nil {
		return ResolvedRepositories{
			Repositories: capability.Repositories,
			AgentFile:    capability.AgentFile,
			WorkingDir:   capability.WorkingDir,
		}
	}
	profileName := capability.ProfileName()
	if profileName == "" {
		return ResolvedRepositories{}
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: profileName}, &profile); err != nil {
		return ResolvedRepositories{}
	}
	if profile.Spec.Repositories != nil {
		return ResolvedRepositories{
			Repositories: profile.Spec.Repositories,
			AgentFile:    profile.Spec.AgentFile,
			WorkingDir:   profile.Spec.WorkingDir,
		}
	}
	if profile.Spec.Repository.URL == "" {
		return ResolvedRepositories{}
	}
	return ResolvedRepositories{
		Repositories: []agentopsv1alpha1.RepositoryEntry{deprecatedRepositoryEntry(profile.Spec.Repository)},
		AgentFile:    profile.Spec.AgentFile,
		WorkingDir:   profile.Spec.WorkingDir,
	}
}

// deprecatedRepositoryEntry converts AgentProfileSpec.Repository, the
// one-repository shorthand, into the one-entry list it resolves as: name
// `default` (inert, since there is only one entry), `keepGit` left nil so
// RepositoryEntry.KeepsGit() answers true — the "deprecated field still
// works" scenario's own "keepGit true".
func deprecatedRepositoryEntry(r agentopsv1alpha1.RepositorySpec) agentopsv1alpha1.RepositoryEntry {
	return agentopsv1alpha1.RepositoryEntry{Name: "default", URL: r.URL, Ref: r.Ref, Auth: r.Auth}
}

// RepositoriesOf is a capability's resolved repository list as a
// Conversation record snapshots it (design D2): copied field for field,
// Auth's secret reference carried by name only (state.RepoAuth, never the
// credential), and KeepGit RESOLVED to a plain bool — a record carries no
// CRD pointer-means-default semantics. Nil in, nil out: the zero-checkout
// case stays indistinguishable from "nothing resolved".
func RepositoriesOf(entries []agentopsv1alpha1.RepositoryEntry) []state.RepositoryEntry {
	if entries == nil {
		return nil
	}
	out := make([]state.RepositoryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, state.RepositoryEntry{
			Name: e.Name, URL: e.URL, Ref: e.Ref,
			Auth: repoAuthOf(e.Auth), KeepGit: e.KeepsGit(),
		})
	}
	return out
}

// repoAuthOf mirrors one RepositoryEntry.Auth onto its record shape. Nil
// in, nil out.
func repoAuthOf(a *agentopsv1alpha1.RepoAuth) *state.RepoAuth {
	if a == nil {
		return nil
	}
	return &state.RepoAuth{Type: string(a.Type), SecretRef: a.SecretRef.Name}
}

// AgentFileOf is a capability's resolved agent file location as a
// Conversation record snapshots it. Nil in, nil out.
func AgentFileOf(f *agentopsv1alpha1.AgentFileRef) *state.AgentFileRef {
	if f == nil {
		return nil
	}
	return &state.AgentFileRef{Repo: f.Repo, Path: f.Path}
}

// ToolsetBindingOf is a capability's toolset binding as a conversation record
// carries it: the mode and the toolset names, copied. Nil in, nil out.
func ToolsetBindingOf(b *agentopsv1alpha1.ToolsetBinding) *state.ToolsetBinding {
	if b == nil {
		return nil
	}
	return &state.ToolsetBinding{Mode: b.Mode, Refs: refNames(b.Refs)}
}

// ToolingBindingOf is a capability's MCPConfig binding as a conversation
// record carries it. Nil in, nil out.
func ToolingBindingOf(b *agentopsv1alpha1.ToolingBinding) *state.ToolingBinding {
	if b == nil {
		return nil
	}
	return &state.ToolingBinding{Refs: refNames(b.Refs)}
}

func refNames(refs []agentopsv1alpha1.ObjectRef) []state.Ref {
	out := make([]state.Ref, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}
