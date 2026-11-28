package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
)

// The four workspace-layout causes multi-repo-workspace's "An inconsistent
// declaration is refused at resolution" names — each its own stable Reason,
// so a Ready condition failing on one of these NAMES which, rather than
// sharing "MissingReferences" with every dangling ref.
const (
	// ReasonAgentFileRepoUnresolved: agentFile.repo, or a prompt/replyPrompt
	// qualifier, names a repository not in the resolved list.
	ReasonAgentFileRepoUnresolved = "AgentFileRepoUnresolved"
	// ReasonAgentFileRepoRequired: agentFile is declared with no repo while
	// two or more repositories resolve — nothing to disambiguate with.
	ReasonAgentFileRepoRequired = "AgentFileRepoRequired"
	// ReasonWorkingDirUnresolved: workingDir names a repository not in the
	// resolved list.
	ReasonWorkingDirUnresolved = "WorkingDirUnresolved"
	// ReasonWorkingDirNeedsRepositories: workingDir is set with fewer than
	// two repositories resolved.
	ReasonWorkingDirNeedsRepositories = "WorkingDirNeedsRepositories"
)

// workspaceLayoutProblem reports the FIRST workspace-layout cause a resolved
// capability's repositories/agentFile/workingDir/prompts hold — multi-repo-
// workspace's "An inconsistent declaration is refused at resolution" — or
// ok=false when none applies.
//
// Checked HERE, on the HOLDER (Pipeline or Coordinator), never on
// AgentProfile/AgentCapability admission (design D8): the cross-object
// cases need the RESOLVED list, which exists only once a holder settles
// which one — the capability's or the profile's — wins. CEL on the two CRDs
// already covers what one object alone can express.
//
// The caller checks this ONLY once every dangling ref is already resolved
// (len(missing) == 0): a profile or capability that cannot even be read is
// reported there, under "MissingReferences", and asking this question of an
// unresolved capability would just repeat that refusal under a different
// name.
func workspaceLayoutProblem(ctx context.Context, c client.Reader, namespace string,
	capability agentopsv1alpha1.AgentCapabilitySpec) (reason, message string, ok bool) {

	resolved := dispatch.ResolveRepositories(ctx, c, namespace, capability)
	names := repoNameSet(resolved.Repositories)

	if resolved.AgentFile != nil {
		switch {
		case resolved.AgentFile.Repo == "" && len(resolved.Repositories) >= 2:
			return ReasonAgentFileRepoRequired,
				fmt.Sprintf("agentFile.repo is required to disambiguate the resolved repositories %s",
					formatRepoNames(names)), true
		case resolved.AgentFile.Repo != "" && !names[resolved.AgentFile.Repo]:
			return ReasonAgentFileRepoUnresolved,
				fmt.Sprintf("agentFile.repo %q is not in the resolved repositories %s",
					resolved.AgentFile.Repo, formatRepoNames(names)), true
		}
	}

	if resolved.WorkingDir != "" {
		switch {
		case len(resolved.Repositories) < 2:
			return ReasonWorkingDirNeedsRepositories,
				fmt.Sprintf("workingDir %q needs two or more resolved repositories, got %d",
					resolved.WorkingDir, len(resolved.Repositories)), true
		case !names[resolved.WorkingDir]:
			return ReasonWorkingDirUnresolved,
				fmt.Sprintf("workingDir %q is not in the resolved repositories %s",
					resolved.WorkingDir, formatRepoNames(names)), true
		}
	}

	prompt, replyPrompt := promptsOf(ctx, c, namespace, capability)
	if reason, message, ok := promptQualifierProblem(names, "prompt", prompt); ok {
		return reason, message, true
	}
	if reason, message, ok := promptQualifierProblem(names, "replyPrompt", replyPrompt); ok {
		return reason, message, true
	}
	return "", "", false
}

// promptsOf reads both prompt fields off the capability's named profile —
// the ONLY place either lives (AgentCapabilitySpec carries neither). A
// profile that cannot be read, or no ProfileRef at all, answers "","": its
// dangling ref is already named under "MissingReferences", and a holder
// resolving to no profile has no prompt to qualify.
func promptsOf(ctx context.Context, c client.Reader, namespace string,
	capability agentopsv1alpha1.AgentCapabilitySpec) (prompt, replyPrompt string) {

	profileName := capability.ProfileName()
	if profileName == "" {
		return "", ""
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: profileName}, &profile); err != nil {
		return "", ""
	}
	return profile.Spec.Prompt, profile.Spec.ReplyPrompt
}

// promptRepoQualifier extracts the `<repo>` name from a prompt path's
// optional `<repo>:<path>` qualifier (design D7: "A repo name is a DNS
// label and holds no `:`, so the first colon splits unambiguously"). This
// is enough to VALIDATE the name against the resolved list — the actual
// split used to load the file at dispatch time is a separate task (4.3 of
// this change), not reimplemented here.
func promptRepoQualifier(path string) (repo string, ok bool) {
	i := strings.IndexByte(path, ':')
	if i < 0 {
		return "", false
	}
	return path[:i], true
}

// promptQualifierProblem reports the ReasonAgentFileRepoUnresolved cause —
// the same one a dangling agentFile.repo reports, since both are "a repo
// name this holder's resolved list does not contain" — for a dangling
// prompt/replyPrompt qualifier. label names which field in the message.
func promptQualifierProblem(names map[string]bool, label, path string) (reason, message string, ok bool) {
	repo, qualified := promptRepoQualifier(path)
	if !qualified || names[repo] {
		return "", "", false
	}
	return ReasonAgentFileRepoUnresolved,
		fmt.Sprintf("%s's %q qualifier is not in the resolved repositories %s", label, repo, formatRepoNames(names)), true
}

// repoNameSet is the resolved list's names, for a membership check.
func repoNameSet(repos []agentopsv1alpha1.RepositoryEntry) map[string]bool {
	set := make(map[string]bool, len(repos))
	for _, r := range repos {
		set[r.Name] = true
	}
	return set
}

// formatRepoNames renders a resolved list's names sorted, for a stable
// message.
func formatRepoNames(names map[string]bool) string {
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return "[" + strings.Join(out, ", ") + "]"
}
