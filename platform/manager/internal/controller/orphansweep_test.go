package controller

import (
	"context"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A runtime pod and its MCP ConfigMap must carry no owner reference when
// conversations are not Kubernetes objects. The garbage collector deletes a
// dependent whose owner it cannot find, and envtest runs no collector, so only
// asserting the absence catches it: this was a pod created and collected in a
// loop on a live cluster.
func TestOwnIsSkippedWhenNoKubernetesObjectStandsBehindTheRecord(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	conv := &state.ConversationRecord{}
	conv.Name, conv.ID = "chat-a", "MeIKbVF176UEABLfjcih43"

	for _, tc := range []struct {
		name      string
		skip      bool
		wantOwner bool
	}{
		{"nats backend", true, false},
		{"kube backend keeps the reference", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &ConversationReconciler{Scheme: scheme, Namespaces: namespace.Namespaces{Control: "ns-a", Conversations: "ns-a"}, NoOwnerReference: tc.skip}
			for _, obj := range []client.Object{
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns-a"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns-a"}},
			} {
				if err := r.own(conv, obj); err != nil {
					t.Fatalf("own(%T): %v", obj, err)
				}
				if got := len(obj.GetOwnerReferences()) > 0; got != tc.wantOwner {
					t.Fatalf("%T carries an owner reference = %v, want %v", obj, got, tc.wantOwner)
				}
			}
		})
	}
}

// The sweeper stands in for the garbage collector: it removes a pod and a
// ConfigMap whose record is gone, and leaves one whose record exists and one too
// young to judge.
func TestOrphanSweeperDeletesOnlyWorkloadsWhoseRecordIsGone(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	old := metav1.NewTime(time.Now().Add(-time.Hour))
	young := metav1.NewTime(time.Now())

	// The pod sweep resolves by the conversation-ID label now
	// (conversation-identity Decision 7), never by Name — "live"'s own
	// UID (assignUIDs' trivial convention, ID == Name) is what its pod's
	// label must carry for the sweep to recognize it as NOT orphaned.
	pod := func(conv string, created metav1.Time) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: runtimepod.PodName(conv), Namespace: claimTestNS, CreationTimestamp: created,
			Labels: map[string]string{runtimepod.LabelConversationID: conv},
		}}
	}
	cm := func(conv string, created metav1.Time) *corev1.ConfigMap {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: convMCPConfigMapName(conv), Namespace: claimTestNS, CreationTimestamp: created,
		}}
	}
	other := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: claimTestNS, CreationTimestamp: old}}

	live := claimTestConv("live")
	assignUIDs([]runtime.Object{live})
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithRuntimeObjects(live,
			pod("live", old), pod("gone", old), pod("fresh", young),
			cm("live", old), cm("gone", old), cm("fresh", young), other).Build()

	s := &OrphanSweeper{Client: c, Conversations: claimStore(c), Namespace: claimTestNS}
	n, err := s.Sweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n != 2 {
		t.Fatalf("deleted %d objects, want 2 (the pod and ConfigMap of the absent record)", n)
	}

	exists := func(obj client.Object, name string) bool {
		err := c.Get(context.Background(), client.ObjectKey{Namespace: claimTestNS, Name: name}, obj)
		if err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("get %s: %v", name, err)
		}
		return err == nil
	}
	for _, tc := range []struct {
		obj  client.Object
		name string
		want bool
	}{
		{&corev1.Pod{}, runtimepod.PodName("gone"), false},
		{&corev1.ConfigMap{}, convMCPConfigMapName("gone"), false},
		{&corev1.Pod{}, runtimepod.PodName("live"), true},
		{&corev1.ConfigMap{}, convMCPConfigMapName("live"), true},
		{&corev1.Pod{}, runtimepod.PodName("fresh"), true},
		{&corev1.ConfigMap{}, convMCPConfigMapName("fresh"), true},
		{&corev1.ConfigMap{}, "unrelated", true},
	} {
		if got := exists(tc.obj, tc.name); got != tc.want {
			t.Errorf("%s exists = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// An MCP ConfigMap left in the control namespace by a release from before the
// namespaces were split is swept too, once its record is gone. Nothing else in
// that namespace is touched, pods included.
func TestOrphanSweeperAlsoSweepsConfigMapsLeftInTheControlNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	old := metav1.NewTime(time.Now().Add(-time.Hour))
	const control = "control-ns"
	cm := func(conv string) *corev1.ConfigMap {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: convMCPConfigMapName(conv), Namespace: control, CreationTimestamp: old}}
	}
	unrelatedPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "manager", Namespace: control, CreationTimestamp: old,
		Labels: map[string]string{runtimepod.LabelConversation: "gone"}}}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithRuntimeObjects(claimTestConv("live"), cm("live"), cm("gone"), unrelatedPod).Build()

	s := &OrphanSweeper{Client: c, Conversations: claimStore(c), Namespace: claimTestNS,
		LegacyConfigMapNamespaces: []string{control}}
	if _, err := s.Sweep(context.Background(), time.Now()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	has := func(obj client.Object, name string) bool {
		err := c.Get(context.Background(), client.ObjectKey{Namespace: control, Name: name}, obj)
		return err == nil
	}
	if has(&corev1.ConfigMap{}, convMCPConfigMapName("gone")) {
		t.Error("an orphaned ConfigMap in the control namespace survives the sweep")
	}
	if !has(&corev1.ConfigMap{}, convMCPConfigMapName("live")) {
		t.Error("a ConfigMap whose record exists was swept")
	}
	if !has(&corev1.Pod{}, "manager") {
		t.Error("a pod in the control namespace was swept: only ConfigMaps are swept there")
	}
}
