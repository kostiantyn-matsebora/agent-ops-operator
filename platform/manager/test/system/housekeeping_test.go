//go:build system

package system

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// 11.2 — the housekeeping lane. state-store-generic-contract moved
// housekeeping off a Kubernetes RBAC grant (a Role/RoleBinding, now deleted)
// onto the manager's own GET /state/conversations surface, authenticated by
// STATE_READ_TOKEN, with automountServiceAccountToken: false on its pod —
// nothing left for a ServiceAccount token to authenticate even if one were
// mounted. envtest can render the chart and prove the RBAC objects are gone;
// only a real cluster proves the pod ACTUALLY STARTS, actually reads through
// the new surface and actually finishes, under the real kubelet withholding
// the token. FULL TIER ONLY: `housekeeping.enabled` renders only under
// DefaultValues' full-tier install (install.go), so the smoke tier never
// builds or runs this lane.
func TestHousekeepingDryRunRetainsEveryConversation(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()

	// Never trust an earlier lane's leftovers for a non-vacuous count: seed
	// one conversation of our own, the way every other lane does.
	fp := "e2e-housekeeping-" + fmt.Sprint(time.Now().UnixNano())
	e.PostTask(t, SourceTasks, fp, "echo housekeeping")
	conv := e.ConversationFor(t, fp, time.Minute)
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})

	job, err := e.K.RunCronJobOnce(ctx, "agentops-housekeeping", 2*time.Minute)
	if err != nil {
		t.Fatalf("running the housekeeping job once: %v", err)
	}
	t.Cleanup(func() { _ = e.K.Delete(context.Background(), job) })
	if job.Status.Succeeded < 1 {
		t.Fatalf("a dry run must succeed: %+v", job.Status)
	}

	pods, err := e.K.Pods(ctx, "job-name="+job.Name)
	if err != nil || len(pods) == 0 {
		t.Fatalf("the job's own pod: %v %d", err, len(pods))
	}
	pod := pods[0]

	// The one visible proof this lane exercises the NEW state-surface path
	// rather than a stale image still doing RBAC reads: no automounted
	// ServiceAccount token, and the two env vars the new surface needs.
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("the housekeeping pod must not automount a ServiceAccount token: %v", pod.Spec.AutomountServiceAccountToken)
	}
	var sawManagerURL, sawStateReadTokenFromSecret bool
	for _, c := range pod.Spec.Containers {
		for _, ev := range c.Env {
			switch ev.Name {
			case "MANAGER_URL":
				sawManagerURL = ev.Value != ""
			case "STATE_READ_TOKEN":
				sawStateReadTokenFromSecret = ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil
			}
		}
	}
	if !sawManagerURL {
		t.Fatalf("the pod must carry MANAGER_URL: %+v", pod.Spec.Containers)
	}
	if !sawStateReadTokenFromSecret {
		t.Fatalf("the pod must read STATE_READ_TOKEN from a Secret (never a literal, never a Kubernetes read): %+v", pod.Spec.Containers)
	}

	logs, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "logs", pod.Name)
	if err != nil {
		t.Fatalf("reading the job's log: %v", err)
	}
	assertDryRunLogRemovesNothing(t, logs)
	// No conversation-count comparison: the full tier's other lanes create and
	// delete conversations in parallel, so a count before and after would
	// measure them, not this job. Housekeeping holds no Kubernetes grant at
	// all (the pod proofs above), so it cannot delete a Conversation; the
	// dry-run log above is the cross-check on what it does touch.
}

// reportLine matches Report.String() (platform/housekeeping/reclaim.go):
// "<kind>: scanned N, would remove N, retained N[, ...]" — "would remove" is
// the DryRun verb, the chart's own default for this CronJob.
var reportLine = regexp.MustCompile(`(workspaces|sessions): scanned (\d+), would remove (\d+), retained (\d+)`)

// assertDryRunLogRemovesNothing cross-checks the job's own reported counts:
// a dry run's "would remove" must be zero for every root it actually
// scanned (a root with no configured volume mount logs "no root configured,
// skipping" instead and never appears here).
func assertDryRunLogRemovesNothing(t *testing.T, logs string) {
	t.Helper()
	if !strings.Contains(logs, "DRY RUN") {
		t.Fatalf("the chart's own default is DRY_RUN=true: %s", logs)
	}
	if strings.Contains(strings.ToLower(logs), "is forbidden") {
		t.Fatalf("the job must hold no Kubernetes RBAC grant to fall back on: %s", logs)
	}
	matches := reportLine.FindAllStringSubmatch(logs, -1)
	if len(matches) == 0 {
		t.Fatalf("the job must log a report for at least one root (sessions is always mounted by this install's default persistence.context): %s", logs)
	}
	for _, m := range matches {
		removed, _ := strconv.Atoi(m[3])
		if removed != 0 {
			t.Fatalf("a dry run must remove nothing: %s", m[0])
		}
	}
}
