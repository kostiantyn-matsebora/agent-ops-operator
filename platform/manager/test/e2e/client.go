//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Kube is a typed client over the project's own API types plus a clientset
// for SubjectAccessReview and the like.
type Kube struct {
	client.Client
	Clientset *kubernetes.Clientset
	Namespace string
	Scheme    *runtime.Scheme
}

// NewKube builds the client from a kubeconfig.
func NewKube(kubeconfig, namespace string) (*Kube, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentopsv1alpha1.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Kube{Client: c, Clientset: cs, Namespace: namespace, Scheme: scheme}, nil
}

// WaitDeploymentAvailable waits for a Deployment's Available condition.
func (k *Kube) WaitDeploymentAvailable(ctx context.Context, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		var d appsv1.Deployment
		err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: name}, &d)
		switch {
		case apierrors.IsNotFound(err):
			last = "not found"
		case err != nil:
			return err
		default:
			if d.Status.AvailableReplicas >= 1 {
				return nil
			}
			last = fmt.Sprintf("available=%d ready=%d", d.Status.AvailableReplicas, d.Status.ReadyReplicas)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("deployment %s not available after %s (%s)", name, timeout, last)
}

// Can asks the live authorizer whether a ServiceAccount may do something —
// the rendered Role is what envtest already checks and is not what enforces.
func (k *Kube) Can(ctx context.Context, sa, verb, group, resource, namespace string) (bool, error) {
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   "system:serviceaccount:" + k.Namespace + ":" + sa,
			Groups: []string{"system:serviceaccounts", "system:serviceaccounts:" + k.Namespace, "system:authenticated"},
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb: verb, Group: group, Resource: resource, Namespace: namespace,
			},
		},
	}
	out, err := k.Clientset.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return out.Status.Allowed, nil
}

// Pods lists pods by label selector.
func (k *Kube) Pods(ctx context.Context, selector string) ([]corev1.Pod, error) {
	list, err := k.Clientset.CoreV1().Pods(k.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// Conversation reads one conversation.
func (k *Kube) Conversation(ctx context.Context, name string) (*agentopsv1alpha1.Conversation, error) {
	var c agentopsv1alpha1.Conversation
	if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: name}, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Conversations lists them all.
func (k *Kube) Conversations(ctx context.Context) ([]agentopsv1alpha1.Conversation, error) {
	var list agentopsv1alpha1.ConversationList
	if err := k.List(ctx, &list, client.InNamespace(k.Namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// waitFor polls until cond holds, failing the test with `what` otherwise.
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := cond()
		if ok {
			return
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out after %s waiting for %s (last error: %v)", timeout, what, lastErr)
}

// mustCreate creates an object and deletes it when the test ends.
func mustCreate(t *testing.T, k *Kube, obj client.Object) {
	t.Helper()
	obj.SetNamespace(k.Namespace)
	if err := k.Create(context.Background(), obj); err != nil {
		t.Fatalf("creating %T %s: %v", obj, obj.GetName(), err)
	}
	t.Cleanup(func() {
		_ = k.Delete(context.Background(), obj)
	})
}

// ensure creates an object if it is absent and leaves it in place for the
// whole run — the pack's shared wiring.
func ensure(ctx context.Context, k *Kube, obj client.Object) error {
	obj.SetNamespace(k.Namespace)
	err := k.Create(ctx, obj)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
