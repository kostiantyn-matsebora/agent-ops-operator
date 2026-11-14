package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// ConditionExpectedReferences names every Secret and ConfigMap a
// conversation's runtime pod will resolve in the CONVERSATIONS namespace
// (split-conversation-namespace decision 5): profile repository auth,
// profile and runtime env `valueFrom`, MCP config `valueFrom` and raw refs.
//
// NAMING A REFERENCE IS NOT READING ONE. The manager reads no Secret and
// holds no `secrets` verb, so every name here is derived from the
// referencing CRs alone — this condition is a pointer for an adopter to
// check against, never a verification that any of them exist. A missing one
// still fails at the pod, in the kubelet's own words; this just says where to
// look first.
//
// Set only when a conversation names at least one reference, the same
// restraint ConditionToolingResolved already takes — a conversation
// resolving nothing carries no condition rather than a standing "nothing to
// see here".
const ConditionExpectedReferences = "ExpectedReferences"

// ReasonReferencesNamed is the one reason this condition is ever set with.
const ReasonReferencesNamed = "Named"

// expectedReferences collects the Secret and ConfigMap names a conversation's
// pod will resolve, from the CRs alone:
//
//   - profile.Spec.Repository.Auth.SecretRef (the ssh key or the https token)
//   - profile.Spec.Env[].ValueFrom and runtimeEnv[].ValueFrom (the runtime's
//     own env, which is where a model credential arrives — there is no
//     dedicated field for it)
//   - mcp.Env[].ValueFrom (MCP server header/env valueFrom, already compiled
//     to env vars by internal/mcpcompile)
//   - mcp.RawConfigMap / mcp.RawSecret (a hand-written mcp.json mounted
//     directly)
//
// Image pull secrets are not named here: no CRD in this API carries a
// reference to one today, so there is nothing on the CRs for this function
// to read (see this change's hand-back report).
func expectedReferences(profile *agentopsv1alpha1.AgentProfile, runtimeEnv []corev1.EnvVar, mcp mcpcompile.Result) (secrets, configMaps []string) {
	secretSet := map[string]bool{}
	configMapSet := map[string]bool{}

	if profile.Spec.Repository.Auth != nil {
		secretSet[profile.Spec.Repository.Auth.SecretRef.Name] = true
	}
	for _, envVars := range [][]corev1.EnvVar{profile.Spec.Env, runtimeEnv, mcp.Env} {
		for _, e := range envVars {
			if e.ValueFrom == nil {
				continue
			}
			if ref := e.ValueFrom.SecretKeyRef; ref != nil && ref.Name != "" {
				secretSet[ref.Name] = true
			}
			if ref := e.ValueFrom.ConfigMapKeyRef; ref != nil && ref.Name != "" {
				configMapSet[ref.Name] = true
			}
		}
	}
	if mcp.RawConfigMap != "" {
		configMapSet[mcp.RawConfigMap] = true
	}
	if mcp.RawSecret != "" {
		secretSet[mcp.RawSecret] = true
	}
	delete(secretSet, "")
	delete(configMapSet, "")

	secrets = setToSortedSlice(secretSet)
	configMaps = setToSortedSlice(configMapSet)
	return secrets, configMaps
}

func setToSortedSlice(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// setExpectedReferencesCondition derives the expected references and writes
// ConditionExpectedReferences, naming the conversations namespace they are
// expected in. A conversation with nothing to resolve gets no condition.
func (r *ConversationReconciler) setExpectedReferencesCondition(ctx context.Context, conv *state.ConversationRecord,
	profile *agentopsv1alpha1.AgentProfile, mcpRes mcpcompile.Result, runtime runtimepod.Config) {

	secrets, configMaps := expectedReferences(profile, runtime.Env, mcpRes)
	if len(secrets) == 0 && len(configMaps) == 0 {
		return
	}
	var parts []string
	if len(secrets) > 0 {
		parts = append(parts, "secrets: "+strings.Join(secrets, ", "))
	}
	if len(configMaps) > 0 {
		parts = append(parts, "configmaps: "+strings.Join(configMaps, ", "))
	}
	message := fmt.Sprintf("expected in the conversations namespace %q (not read, not verified): %s",
		r.Namespaces.Conversations, strings.Join(parts, "; "))
	err := state.Patch(ctx, r.Conversations, conv, func(c *state.ConversationRecord) {
		c.SetCondition(state.Condition{
			Type: ConditionExpectedReferences, Status: state.ConditionTrue,
			Reason: ReasonReferencesNamed, Message: message,
		})
	})
	if err != nil {
		log.FromContext(ctx).Error(err, "patching ExpectedReferences condition", "conversation", conv.Name)
	}
}
