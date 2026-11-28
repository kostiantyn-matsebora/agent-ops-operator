package integration

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/cachescope"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
)

// The manager's cache must SYNC under the Roles the chart actually grants it,
// in both layouts. A cache spanning both namespaces for every kind once
// started a Pipeline informer (and every other wiring kind's) in the
// conversations namespace, where the conversations Role grants none — the
// list was forbidden, the cache never synced, and the manager crash-looped
// on the first split install. envtest enforces RBAC, so rendering the
// chart's Roles and impersonating the manager's account reproduces exactly
// what a cluster does.

func TestManagerCacheSyncsUnderTheChartsRolesSplit(t *testing.T) {
	assertManagerCacheSyncs(t, createNamespace(t), createNamespace(t))
}

func TestManagerCacheSyncsUnderTheChartsRolesSingleNamespace(t *testing.T) {
	ctl := createNamespace(t)
	assertManagerCacheSyncs(t, ctl, ctl)
}

// cachedKinds is every kind the manager may read through its cache: every
// agentops.dev kind, and every core/apps kind a reconciler watches or reads.
func cachedKinds() []client.Object {
	var objs []client.Object
	for gvk := range scheme.AllKnownTypes() {
		if gvk.Group != agentopsv1alpha1.GroupVersion.Group || strings.HasSuffix(gvk.Kind, "List") {
			continue
		}
		obj, err := scheme.New(gvk)
		if err != nil {
			continue
		}
		if o, ok := obj.(client.Object); ok {
			objs = append(objs, o)
		}
	}
	return append(objs,
		// NOT ServiceAccount: no reconciler watches or reads it (least-privilege-manager),
		// and the chart grants the manager no verb on it — an informer here would never sync.
		&corev1.Pod{}, &corev1.ConfigMap{}, &corev1.PersistentVolumeClaim{},
		&corev1.Service{}, &appsv1.Deployment{})
}

func assertManagerCacheSyncs(t *testing.T, ctl, conv string) {
	t.Helper()
	applyManagerRoles(t, ctl, conv)
	cfg := impersonate(managerUser(ctl))

	// RBAC really is enforced here, or a green result below proves nothing.
	if ctl != conv {
		assertPipelineListForbidden(t, cfg, conv)
	}

	opts := cachescope.Options(namespace.Namespaces{Control: ctl, Conversations: conv})
	opts.Scheme = scheme
	c, err := cache.New(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	informers := map[string]cache.Informer{}
	for _, obj := range cachedKinds() {
		inf, err := c.GetInformer(ctx, obj)
		if err != nil {
			t.Fatalf("informer for %T: %v", obj, err)
		}
		informers[typeName(obj)] = inf
	}
	go func() { _ = c.Start(ctx) }()
	waitInformersSynced(t, informers, ctl, conv)
}

func assertPipelineListForbidden(t *testing.T, cfg *rest.Config, conv string) {
	t.Helper()
	probe, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	err = probe.List(context.Background(), &agentopsv1alpha1.PipelineList{}, client.InNamespace(conv))
	if !apierrors.IsForbidden(err) {
		t.Fatalf("listing Pipelines in the conversations namespace must be forbidden to the manager, got %v", err)
	}
}

func waitInformersSynced(t *testing.T, informers map[string]cache.Informer, ctl, conv string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var unsynced []string
		for name, inf := range informers {
			if !inf.HasSynced() {
				unsynced = append(unsynced, name)
			}
		}
		if len(unsynced) == 0 {
			return
		}
		if time.Now().After(deadline) {
			sort.Strings(unsynced)
			t.Fatalf("the manager's cache never synced under the chart's Roles (control %q, conversations %q): %s",
				ctl, conv, strings.Join(unsynced, ", "))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// applyManagerRoles renders the chart's rbac.yaml for this layout and creates
// every Role and RoleBinding it names in either namespace.
func applyManagerRoles(t *testing.T, ctl, conv string) {
	t.Helper()
	out := helmTemplate(t, "--namespace", ctl,
		"--set", "global.agentops.conversationNamespace="+conv,
		"--show-only", "templates/rbac.yaml")
	dec := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(out), 4096)
	applied := 0
	for {
		var u unstructured.Unstructured
		err := dec.Decode(&u.Object)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode rendered rbac.yaml: %v", err)
		}
		gvk := u.GroupVersionKind()
		if gvk.Group != "rbac.authorization.k8s.io" || (gvk.Kind != "Role" && gvk.Kind != "RoleBinding") {
			continue
		}
		if ns := u.GetNamespace(); ns != ctl && ns != conv {
			continue
		}
		if err := k8sClient.Create(context.Background(), &u); err != nil {
			t.Fatalf("create %s %s/%s: %v", gvk.Kind, u.GetNamespace(), u.GetName(), err)
		}
		applied++
	}
	if applied == 0 {
		t.Fatal("rbac.yaml rendered no manager Role or RoleBinding")
	}
}

// managerUser is the identity the manager's pod authenticates as.
func managerUser(ctl string) rest.ImpersonationConfig {
	return rest.ImpersonationConfig{
		UserName: "system:serviceaccount:" + ctl + ":agentops-manager",
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:" + ctl, "system:authenticated"},
	}
}

func impersonate(as rest.ImpersonationConfig) *rest.Config {
	cfg := rest.CopyConfig(testEnv.Config)
	cfg.Impersonate = as
	return cfg
}

func typeName(obj client.Object) string {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		return "unknown"
	}
	return gvks[0].Kind
}
