package integration

import (
	"strings"
	"testing"
)

// Chart-render assertions for conversation retention and the housekeeping job.
//
// Both retention windows and the job are destructive in their own way, so the
// property worth pinning hardest is that a default install reaches none of them.

func TestRetentionAndHousekeepingAreOffByDefault(t *testing.T) {
	out := helmTemplate(t)
	for _, needle := range []string{
		"CONVERSATION_AUTOCLOSE_ENABLED",
		"CONVERSATION_AUTODELETE_ENABLED",
		"kind: CronJob",
		"agentops-housekeeping",
	} {
		if strings.Contains(out, needle) {
			t.Errorf("a default install must not reach %q", needle)
		}
	}
}

// Enabling one window must never enable the other: autoclose with autodelete
// off — a lane that tidies itself and keeps its record — is the common
// configuration.
func TestEachRetentionWindowIsIndependent(t *testing.T) {
	closeOnly := helmTemplate(t, "--set", "retention.autoclose.enabled=true")
	if !strings.Contains(closeOnly, "CONVERSATION_AUTOCLOSE_ENABLED") {
		t.Error("autoclose must reach the manager when enabled")
	}
	if strings.Contains(closeOnly, "CONVERSATION_AUTODELETE_ENABLED") {
		t.Error("enabling autoclose must NOT enable autodelete")
	}

	deleteOnly := helmTemplate(t, "--set", "retention.autodelete.enabled=true")
	if !strings.Contains(deleteOnly, "CONVERSATION_AUTODELETE_ENABLED") {
		t.Error("autodelete must reach the manager when enabled")
	}
	if strings.Contains(deleteOnly, "CONVERSATION_AUTOCLOSE_ENABLED") {
		t.Error("enabling autodelete must NOT enable autoclose")
	}
}

// A window with no duration would be an enabled timer that never fires — worse
// than off, because it reads as configured.
func TestAnEnabledWindowNeedsItsDuration(t *testing.T) {
	for _, tc := range []struct{ flag, age string }{
		{"retention.autoclose.enabled", "retention.autoclose.idleAge"},
		{"retention.autodelete.enabled", "retention.autodelete.closedAge"},
	} {
		out := helmTemplateErr(t, "--set", tc.flag+"=true", "--set", tc.age+"=")
		if !strings.Contains(out, tc.age) {
			t.Errorf("the failure must name %q:\n%s", tc.age, out)
		}
	}
}

// Mounting the claim ROOT is exactly the reach subPath isolation denies runtime
// pods. Sharing the runtime identity would hand every agent the ability to read
// and delete every other conversation's tree.
func TestHousekeepingRefusesTheRuntimeIdentity(t *testing.T) {
	out := helmTemplateErr(t,
		"--set", "housekeeping.enabled=true",
		"--set", "housekeeping.serviceAccountName=agentops-runtime")
	if !strings.Contains(out, "must differ from the floor account") {
		t.Errorf("the guard must fire and say why:\n%s", out)
	}
}

// A claim is mounted only when the persistence block that owns it is on: there
// is nothing to reclaim from a volume this release does not have, and an empty
// volumeMounts key is not valid output.
func TestHousekeepingMountsOnlyTheClaimsThatExist(t *testing.T) {
	// context only (workspace persistence is off by default)
	contextOnly := cronJobDoc(t, helmTemplate(t, "--set", "housekeeping.enabled=true"))
	if !strings.Contains(contextOnly, "claimName: agentops-context") {
		t.Error("the context claim must be mounted when persistence is on")
	}
	if strings.Contains(contextOnly, "agentops-workspace") || strings.Contains(contextOnly, "WORKSPACE_ROOT") {
		t.Error("the workspace claim must not be mounted when workspace persistence is off")
	}

	// both
	both := cronJobDoc(t, helmTemplate(t,
		"--set", "housekeeping.enabled=true", "--set", "persistence.workspace.enabled=true"))
	for _, needle := range []string{"claimName: agentops-context", "claimName: agentops-workspace",
		"WORKSPACE_ROOT", "SESSIONS_ROOT"} {
		if !strings.Contains(both, needle) {
			t.Errorf("with both claims on, the job needs %q", needle)
		}
	}

	// neither: no dangling volumeMounts/volumes keys
	neither := cronJobDoc(t, helmTemplate(t,
		"--set", "housekeeping.enabled=true", "--set", "persistence.context.enabled=false"))
	for _, needle := range []string{"volumeMounts:", "volumes:"} {
		if strings.Contains(neither, needle) {
			t.Errorf("with no claims mounted, %q must not render at all:\n%s", needle, neither)
		}
	}
}

// The job reads conversations and writes NOTHING: both etcd-side stages of the
// lifecycle belong to the manager.
func TestHousekeepingRoleIsReadOnly(t *testing.T) {
	out := helmTemplate(t, "--set", "housekeeping.enabled=true")
	var role string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: Role\n") && strings.Contains(doc, "name: agentops-housekeeping") {
			role = stripComments(doc)
		}
	}
	if role == "" {
		t.Fatal("housekeeping Role not rendered")
	}
	if !strings.Contains(role, `verbs: ["get", "list"]`) {
		t.Errorf("the job's Role must be get/list only:\n%s", role)
	}
	for _, forbidden := range []string{"delete", "create", "update", "patch", "watch"} {
		if strings.Contains(role, forbidden) {
			t.Errorf("the job must not be granted %q:\n%s", forbidden, role)
		}
	}
}

// The workload name is what signal-k8s-events' name-prefix self-exclusion
// matches. A CronJob fails on a schedule, so getting this wrong wakes an agent
// about agent-ops' own maintenance over and over.
func TestHousekeepingWorkloadIsNamedForSelfExclusion(t *testing.T) {
	doc := cronJobDoc(t, helmTemplate(t, "--set", "housekeeping.enabled=true"))
	if !strings.Contains(doc, "name: agentops-housekeeping") {
		t.Errorf("the CronJob must carry the self-excluded name prefix:\n%s", doc)
	}
}

// Safe by default: the first run on an established install is the dangerous one.
func TestHousekeepingDefaultsToDryRunAndABoundedRun(t *testing.T) {
	doc := cronJobDoc(t, helmTemplate(t, "--set", "housekeeping.enabled=true"))
	if !strings.Contains(doc, "name: DRY_RUN") || !strings.Contains(doc, `value: "true"`) {
		t.Errorf("the job must default to a dry run:\n%s", doc)
	}
	if !strings.Contains(doc, "name: MAX_DELETIONS") {
		t.Errorf("every run must be bounded:\n%s", doc)
	}
}

func cronJobDoc(t *testing.T, rendered string) string {
	t.Helper()
	for _, doc := range splitDocs(rendered) {
		if strings.Contains(doc, "\nkind: CronJob\n") {
			return doc
		}
	}
	t.Fatal("no CronJob rendered")
	return ""
}

// The Telegram surface's opt-out of keeping a deleted conversation's topic.
//
// It lives on the CHANNEL, not the ChannelAdapter: the adapter CR carries
// implementation only, and whether a group's threads should outlive their
// conversations is a property of that group.
func TestTelegramTopicDeletionIsOptInAndPerChannel(t *testing.T) {
	surface := []string{
		"--set", "telegram.enabled=true",
		"--set", "telegram.surface.enabled=true",
		"--set", "telegram.surface.chatId=-100",
		"--set", "telegram.surface.credentials.botToken=x",
	}
	// default: the key is absent, so the transcript survives
	if out := helmTemplate(t, surface...); strings.Contains(out, "deleteTopicOnConversationDelete: true") {
		t.Error("topic deletion must be off by default — upgrading may not destroy a transcript")
	}
	// opted in: it reaches the CHANNEL's config
	on := helmTemplate(t, append(surface, "--set", "telegram.surface.deleteTopicOnDelete=true")...)
	var channel string
	for _, doc := range splitDocs(on) {
		if strings.Contains(doc, "kind: Channel\n") && strings.Contains(doc, "adapter: telegram") {
			channel = doc
		}
	}
	if channel == "" {
		t.Fatal("no telegram Channel rendered")
	}
	if !strings.Contains(channel, "deleteTopicOnConversationDelete: true") {
		t.Errorf("the flag must land in the Channel's config:\n%s", channel)
	}
	// ...and NOT on the ChannelAdapter, which carries no configuration
	for _, doc := range splitDocs(on) {
		if strings.Contains(doc, "kind: ChannelAdapter\n") &&
			strings.Contains(doc, "deleteTopicOnConversationDelete: true") {
			t.Errorf("configuration must not land on the adapter CR:\n%s", doc)
		}
	}
	// the declared schema names it, so a misspelling is reported rather than
	// silently reading as false
	if !strings.Contains(on, "deleteTopicOnConversationDelete:\n        type: boolean") {
		t.Error("the adapter's configSchema must declare the key")
	}
}
