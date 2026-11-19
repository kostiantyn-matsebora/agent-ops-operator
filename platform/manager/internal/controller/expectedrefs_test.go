package controller

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
)

func valueFromSecret(name string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: "token",
	}}
}

func valueFromConfigMap(name string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: "value",
	}}
}

// TestExpectedReferencesNamesEveryReferenceKindWithNoRead pins
// split-conversation-namespace decision 5: every reference kind is derived
// from the CRs alone, with no Secret or ConfigMap object anywhere in reach of
// this function (it is never handed a client.Reader).
func TestExpectedReferencesNamesEveryReferenceKindWithNoRead(t *testing.T) {
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Spec.Repository.Auth = &agentopsv1alpha1.RepoAuth{
		Type:      agentopsv1alpha1.RepoAuthSSH,
		SecretRef: agentopsv1alpha1.ObjectRef{Name: "repo-key"},
	}
	profile.Spec.Env = []corev1.EnvVar{
		{Name: "PROFILE_TOKEN", ValueFrom: valueFromSecret("profile-secret")},
		{Name: "PROFILE_CFG", ValueFrom: valueFromConfigMap("profile-configmap")},
		{Name: "LITERAL", Value: "no-ref-here"},
	}
	runtimeEnv := []corev1.EnvVar{
		{Name: "MODEL_API_KEY", ValueFrom: valueFromSecret("model-credential")},
	}
	mcp := mcpcompile.Result{
		Env: []corev1.EnvVar{
			{Name: "MCP_HEADER", ValueFrom: valueFromSecret("mcp-secret")},
		},
		RawConfigMap: "raw-mcp-configmap",
	}

	secrets, configMaps := expectedReferences(profile, runtimeEnv, mcp)

	wantSecrets := []string{"mcp-secret", "model-credential", "profile-secret", "repo-key"}
	wantConfigMaps := []string{"profile-configmap", "raw-mcp-configmap"}
	if !reflect.DeepEqual(secrets, wantSecrets) {
		t.Fatalf("secrets = %v, want %v", secrets, wantSecrets)
	}
	if !reflect.DeepEqual(configMaps, wantConfigMaps) {
		t.Fatalf("configMaps = %v, want %v", configMaps, wantConfigMaps)
	}
}

// TestExpectedReferencesRawSecretIsNamedAsASecret confirms mcp.RawSecret (a
// hand-written mcp.json mounted from a Secret rather than a ConfigMap) is
// named in the secrets half, not the configmaps half.
func TestExpectedReferencesRawSecretIsNamedAsASecret(t *testing.T) {
	profile := &agentopsv1alpha1.AgentProfile{}
	mcp := mcpcompile.Result{RawSecret: "raw-mcp-secret"}

	secrets, configMaps := expectedReferences(profile, nil, mcp)

	if len(configMaps) != 0 {
		t.Fatalf("configMaps = %v, want none", configMaps)
	}
	if want := []string{"raw-mcp-secret"}; !reflect.DeepEqual(secrets, want) {
		t.Fatalf("secrets = %v, want %v", secrets, want)
	}
}

// TestExpectedReferencesNamesNothingForABareConversation mirrors
// setToolingCondition's own restraint: nothing to resolve, nothing to name.
func TestExpectedReferencesNamesNothingForABareConversation(t *testing.T) {
	secrets, configMaps := expectedReferences(&agentopsv1alpha1.AgentProfile{}, nil, mcpcompile.Result{})
	if len(secrets) != 0 || len(configMaps) != 0 {
		t.Fatalf("got secrets=%v configMaps=%v, want both empty", secrets, configMaps)
	}
}
