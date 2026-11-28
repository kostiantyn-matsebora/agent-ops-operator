// The expected-references condition at envtest level (split-conversation-
// namespace decision 5): internal/controller/expectedrefs_test.go already
// pins expectedReferences() as a pure function with no client in reach at
// all. This file is the different claim — that a REAL reconcile, against a
// REAL Conversation object, sets the condition naming every reference kind
// while never once reading a Secret through the API server it has every
// permission to read one from.
package integration

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// secretForbiddenClient wraps k8sClient and fails the test the instant
// anything asks it for a Secret — the strict double standing in for the
// manager's own missing `secrets` RBAC verb, which envtest's own client
// (full admin credentials) would otherwise silently allow.
type secretForbiddenClient struct {
	client.Client
	t *testing.T
}

func (c *secretForbiddenClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.Secret); ok {
		c.t.Fatalf("unexpected Secret GET for %s/%s — the manager must read no Secret "+
			"(split-conversation-namespace decision 5)", key.Namespace, key.Name)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *secretForbiddenClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.SecretList); ok {
		c.t.Fatalf("unexpected Secret LIST — the manager must read no Secret "+
			"(split-conversation-namespace decision 5)")
	}
	return c.Client.List(ctx, list, opts...)
}

// erValueFromSecret and erValueFromConfigMap build an env source by Secret
// or ConfigMap key — this package's own copy of internal/controller/
// expectedrefs_test.go's valueFromSecret/valueFromConfigMap, unexported
// there and so not reachable from here.
func erValueFromSecret(name string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: "token",
	}}
}

func erValueFromConfigMap(name string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: "value",
	}}
}

// mkExpectedRefsProfile is mkProfile plus the two reference kinds a profile
// alone can carry: repo auth (SSH) and env valueFrom (Secret and
// ConfigMap).
func mkExpectedRefsProfile(t *testing.T, name string) {
	t.Helper()
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, ns
	p.Spec.Repository = &agentopsv1alpha1.RepositorySpec{URL: "https://example.com/repo.git", Ref: "main"}
	p.Spec.Repository.Auth = &agentopsv1alpha1.RepoAuth{
		Type: agentopsv1alpha1.RepoAuthSSH, SecretRef: agentopsv1alpha1.ObjectRef{Name: "er-repo-key"},
	}
	p.Spec.Agent = "tester"
	p.Spec.MaxTurns = 5
	p.Spec.OutputFormat = agentopsv1alpha1.OutputFormatNone
	p.Spec.Env = []corev1.EnvVar{
		{Name: "PROFILE_TOKEN", ValueFrom: erValueFromSecret("er-profile-secret")},
		{Name: "PROFILE_CFG", ValueFrom: erValueFromConfigMap("er-profile-configmap")},
	}
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "er-runtime"}
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

// ensureERRuntime creates the one AgentRuntime both tests in this file
// bind, tolerating AlreadyExists so either test may run first (and both may
// run in the same `go test` invocation) without racing a Create.
func ensureERRuntime(t *testing.T) {
	t.Helper()
	rt := &agentopsv1alpha1.AgentRuntime{}
	rt.Name, rt.Namespace = "er-runtime", ns
	rt.Spec.Image = "busybox:stub"
	rt.Spec.Env = []corev1.EnvVar{{Name: "MODEL_API_KEY", ValueFrom: erValueFromSecret("er-model-credential")}}
	if err := k8sClient.Create(context.Background(), rt); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
}

// TestExpectedReferencesConditionNamesEveryKindWithNoSecretRead pins
// split-conversation-namespace decision 5 end to end: a real reconcile
// against a real Conversation sets ConditionExpectedReferences naming the
// profile's repo-auth Secret, its env Secret and ConfigMap, the runtime's
// own env Secret (the model credential) and the bound MCP config's header
// Secret — and never reads a Secret doing it.
func TestExpectedReferencesConditionNamesEveryKindWithNoSecretRead(t *testing.T) {
	ctx := context.Background()
	ensureERRuntime(t)
	mkExpectedRefsProfile(t, "er-profile")

	mc := &agentopsv1alpha1.MCPConfig{}
	mc.Name, mc.Namespace = "er-mcp", ns
	mc.Spec.Servers = map[string]agentopsv1alpha1.MCPServer{
		"logs": {Type: "sse", URL: "http://logs.example/sse", Headers: []agentopsv1alpha1.NamedValue{
			{Name: "Authorization", ValueFrom: erValueFromSecret("er-mcp-secret")},
		}},
	}
	if err := k8sClient.Create(ctx, mc); err != nil {
		t.Fatal(err)
	}

	conv := mkConv(t, "er-conv", "er-profile", nil, bind("er-mcp"))

	rc := reconcilerWithOps(nil)
	rc.Client = &secretForbiddenClient{Client: k8sClient, t: t}
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}

	var got agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "er-conv"}, &got); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(got.Status.Conditions, controller.ConditionExpectedReferences)
	if cond == nil || cond.Status != "True" {
		t.Fatalf("ConditionExpectedReferences expected True: %+v", got.Status.Conditions)
	}
	for _, name := range []string{"er-repo-key", "er-profile-secret", "er-model-credential", "er-mcp-secret"} {
		if !strings.Contains(cond.Message, name) {
			t.Errorf("message does not name secret %q: %q", name, cond.Message)
		}
	}
	if !strings.Contains(cond.Message, "er-profile-configmap") {
		t.Errorf("message does not name configmap %q: %q", "er-profile-configmap", cond.Message)
	}
	// decision 1/5: the condition names the CONVERSATIONS namespace, never
	// the control one, even though this suite runs them equal — the message
	// is built from r.Namespaces.Conversations, not a literal "ns".
	if !strings.Contains(cond.Message, ns) {
		t.Errorf("message does not name the conversations namespace %q: %q", ns, cond.Message)
	}
}

// TestExpectedReferencesConditionNamesARawMCPConfigMapRef confirms the raw
// forms (a hand-written mcp.json mounted from a ConfigMap or a Secret) are
// named too, each in its own half, with the same no-Secret-read guarantee.
func TestExpectedReferencesConditionNamesARawMCPConfigMapRef(t *testing.T) {
	ctx := context.Background()
	ensureERRuntime(t)
	mkExpectedRefsProfile(t, "er-raw-profile")
	mkRawMCPConfig(t, "er-raw-cm-mcp", "er-raw-hand-written")

	conv := mkConv(t, "er-raw-conv", "er-raw-profile", nil, bind("er-raw-cm-mcp"))

	rc := reconcilerWithOps(nil)
	rc.Client = &secretForbiddenClient{Client: k8sClient, t: t}
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: conv.Name}}); err != nil {
		t.Fatal(err)
	}

	var got agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "er-raw-conv"}, &got); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(got.Status.Conditions, controller.ConditionExpectedReferences)
	if cond == nil || cond.Status != "True" {
		t.Fatalf("ConditionExpectedReferences expected True: %+v", got.Status.Conditions)
	}
	if !strings.Contains(cond.Message, "er-raw-hand-written") {
		t.Errorf("message does not name the raw configmap %q: %q", "er-raw-hand-written", cond.Message)
	}
	if !strings.Contains(cond.Message, "er-repo-key") {
		t.Errorf("message does not still name the profile's repo-auth secret: %q", cond.Message)
	}
}
