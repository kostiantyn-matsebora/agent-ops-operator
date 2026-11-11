//go:build system

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	// StateURL and StateToken, once set, make Conversation and Conversations
	// read through the manager's /state surface. That surface is the one place
	// both backends serve alike: under the nats backend there is no
	// Conversation object in the Kubernetes API to read, so a helper that went
	// there would see nothing.
	StateURL   string
	StateToken string
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
			// Settled means the rollout is over: the newest spec observed, every
			// replica on it and available. One available old replica is not
			// enough, it keeps answering with the previous configuration while
			// a lane assumes the upgrade it just ran has taken effect.
			want := int32(1)
			if d.Spec.Replicas != nil {
				want = *d.Spec.Replicas
			}
			if d.Status.ObservedGeneration >= d.Generation && d.Status.AvailableReplicas >= 1 &&
				d.Status.UpdatedReplicas == want && d.Status.Replicas == want && d.Status.AvailableReplicas == want {
				return nil
			}
			last = fmt.Sprintf("available=%d ready=%d updated=%d replicas=%d want=%d", d.Status.AvailableReplicas,
				d.Status.ReadyReplicas, d.Status.UpdatedReplicas, d.Status.Replicas, want)
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

// RunCronJobOnce creates a one-off Job from a CronJob's own JobTemplate — the
// standard `kubectl create job --from=cronjob/NAME` behavior — and waits for
// it to reach a terminal state. Needed because 11.2's lane cannot wait for
// the real schedule: it proves the job's OWN run, not that cron ticks.
func (k *Kube) RunCronJobOnce(ctx context.Context, cronJobName string, timeout time.Duration) (*batchv1.Job, error) {
	var cj batchv1.CronJob
	if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: cronJobName}, &cj); err != nil {
		return nil, fmt.Errorf("getting cronjob %s: %w", cronJobName, err)
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJobName + "-manual-",
			Namespace:    k.Namespace,
			Labels:       cj.Spec.JobTemplate.Labels,
			Annotations:  cj.Spec.JobTemplate.Annotations,
		},
		Spec: cj.Spec.JobTemplate.Spec,
	}
	if err := k.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("creating a job from cronjob %s: %w", cronJobName, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var cur batchv1.Job
		if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: job.Name}, &cur); err != nil {
			return job, fmt.Errorf("getting job %s: %w", job.Name, err)
		}
		if cur.Status.Succeeded >= 1 {
			return &cur, nil
		}
		if cur.Status.Failed >= 1 {
			return &cur, fmt.Errorf("job %s failed", job.Name)
		}
		time.Sleep(2 * time.Second)
	}
	return job, fmt.Errorf("job %s did not complete within %s", job.Name, timeout)
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
	if k.StateURL != "" {
		var w stateConversation
		if err := k.stateGet(ctx, "/state/conversations/"+name, &w); err != nil {
			return nil, err
		}
		return w.object(k.Namespace), nil
	}
	var c agentopsv1alpha1.Conversation
	if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: name}, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Conversations lists them all.
func (k *Kube) Conversations(ctx context.Context) ([]agentopsv1alpha1.Conversation, error) {
	if k.StateURL != "" {
		var out struct {
			Items []stateConversation `json:"items"`
		}
		if err := k.stateGet(ctx, "/state/conversations", &out); err != nil {
			return nil, err
		}
		items := make([]agentopsv1alpha1.Conversation, 0, len(out.Items))
		for i := range out.Items {
			items = append(items, *out.Items[i].object(k.Namespace))
		}
		return items, nil
	}
	var list agentopsv1alpha1.ConversationList
	if err := k.List(ctx, &list, client.InNamespace(k.Namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

// Delete overrides the embedded client's. A Conversation object is removed
// the ordinary way where one exists. Where none does, because the manager's
// store holds it, the conversation is closed and deleted through the manager's
// own channel verbs, as a channel adapter does. Every other kind is the
// embedded client's.
func (k *Kube) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	conv, ok := obj.(*agentopsv1alpha1.Conversation)
	if !ok || k.StateURL == "" {
		return k.Client.Delete(ctx, obj, opts...)
	}
	err := k.Client.Delete(ctx, obj, opts...)
	if err == nil || !apierrors.IsNotFound(err) {
		return err
	}
	return k.deleteStored(ctx, []string{conv.Name})
}

// DeleteAllConversations removes every conversation, whichever store holds it.
func (k *Kube) DeleteAllConversations(ctx context.Context) error {
	if k.StateURL != "" {
		items, err := k.Conversations(ctx)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(items))
		for i := range items {
			names = append(names, items[i].Name)
		}
		if len(names) > 0 {
			if err := k.deleteStored(ctx, names); err != nil {
				return err
			}
		}
	}
	var list agentopsv1alpha1.ConversationList
	if err := k.List(ctx, &list, client.InNamespace(k.Namespace)); err != nil {
		return err
	}
	for i := range list.Items {
		if err := k.Client.Delete(ctx, &list.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// deleteStored closes, then deletes, the named conversations through the
// manager. A name already gone is not
// an error: a cleanup that races another is still a cleanup. A conversation
// with no thread has nothing to post a close on, so only its pod is removed.
func (k *Kube) deleteStored(ctx context.Context, names []string) error {
	var live, open []string
	channels := map[string]string{}
	for _, n := range names {
		c, err := k.Conversation(ctx, n)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		live = append(live, n)
		channel, thread := conversationChannel(c)
		channels[n] = channel
		if c.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		if thread == "" {
			// No channel holds it, so no manager verb can close it: a thread is
			// what a close is posted on. What a caller clearing the admission
			// pool needs is the slot, and a slot is a pod. The record stays,
			// Idle and holding nothing.
			live = live[:len(live)-1]
			if err := k.deleteConversationPods(ctx, n); err != nil {
				return err
			}
			continue
		}
		if err := k.managerPost(ctx, "/channel/inbound", map[string]any{
			"channel": channel, "threadId": thread, "sender": "system-pack", "text": "/close"}); err != nil {
			return fmt.Errorf("close %s: %w", n, err)
		}
		open = append(open, n)
	}
	if len(live) == 0 {
		return nil
	}
	if len(open) > 0 {
		if err := k.waitConversations(ctx, open, func(c *agentopsv1alpha1.Conversation, _ bool) bool {
			return c.Status.Phase == agentopsv1alpha1.ConversationClosed
		}); err != nil {
			return fmt.Errorf("waiting for %v to close: %w", open, err)
		}
	}
	for _, n := range live {
		if err := k.managerPost(ctx, "/channel/conversations/"+url.PathEscape(n)+"/delete", map[string]any{"channel": channels[n]}); err != nil {
			return fmt.Errorf("delete %s: %w", n, err)
		}
	}
	// Returns once the deletes are accepted, as `kubectl delete --wait=false`
	// did: a deleted conversation lingers under its close-topics finalizer for
	// up to two minutes, and what a caller needs is the pod and the slot, which
	// the close already released.
	return nil
}

// deleteConversationPods removes the runtime pods of one conversation.
func (k *Kube) deleteConversationPods(ctx context.Context, name string) error {
	pods, err := k.Pods(ctx, "agentops.dev/conversation="+name)
	if err != nil {
		return err
	}
	for i := range pods {
		if err := k.Client.Delete(ctx, &pods[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// conversationChannel names the channel a conversation is bound to and the
// thread it holds there. The thread is empty until the topic exists.
func conversationChannel(c *agentopsv1alpha1.Conversation) (channel, thread string) {
	for _, t := range c.Status.Threads {
		if t.ThreadID != "" {
			return t.Channel, t.ThreadID
		}
	}
	if len(c.Spec.ChannelRefs) > 0 {
		return c.Spec.ChannelRefs[0].Name, ""
	}
	return "", ""
}

// managerPost sends a verb to the manager with the master adapter token,
// retrying a transport error: a port-forward whose pod just rolled answers EOF
// once and recovers.
func (k *Kube) managerPost(ctx context.Context, path string, payload map[string]any) error {
	body, _ := json.Marshal(payload)
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.StateURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+k.StateToken)
		resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
		if err != nil {
			last = err
			time.Sleep(2 * time.Second)
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil
	}
	return last
}

// waitConversations polls until done holds for every name, or ctx ends or two
// minutes pass.
func (k *Kube) waitConversations(ctx context.Context, names []string, done func(c *agentopsv1alpha1.Conversation, gone bool) bool) error {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		pending := 0
		for _, n := range names {
			c, err := k.Conversation(ctx, n)
			switch {
			case apierrors.IsNotFound(err):
				continue
			case err != nil:
				return err
			case !done(c, false):
				pending++
			}
		}
		if pending == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d of %d still not done after 2m", pending, len(names))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Signal reads one Signal — the persist-then-ack record signal-persistence
// adds, mirrored off Conversation's own shape above. With a state URL it reads
// through the manager, which is the only place a signal exists on a backend
// that keeps no Signal object.
func (k *Kube) Signal(ctx context.Context, name string) (*agentopsv1alpha1.Signal, error) {
	if k.StateURL != "" {
		items, err := k.Signals(ctx)
		if err != nil {
			return nil, err
		}
		for i := range items {
			if items[i].Name == name {
				return &items[i], nil
			}
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "agentops.dev", Resource: "signals"}, name)
	}
	var s agentopsv1alpha1.Signal
	if err := k.Get(ctx, types.NamespacedName{Namespace: k.Namespace, Name: name}, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Signals lists them all.
func (k *Kube) Signals(ctx context.Context) ([]agentopsv1alpha1.Signal, error) {
	if k.StateURL != "" {
		var out struct {
			Items []stateSignal `json:"items"`
		}
		if err := k.stateGet(ctx, "/state/signals", &out); err != nil {
			return nil, err
		}
		items := make([]agentopsv1alpha1.Signal, 0, len(out.Items))
		for i := range out.Items {
			items = append(items, *out.Items[i].object(k.Namespace))
		}
		return items, nil
	}
	var list agentopsv1alpha1.SignalList
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
	err := k.Create(context.Background(), obj)
	if apierrors.IsAlreadyExists(err) {
		// Left behind by a run interrupted before its cleanup. Test-owned
		// objects carry the e2e- prefix, so reclaiming one is safe.
		_ = k.Delete(context.Background(), obj)
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			obj.SetResourceVersion("")
			if err = k.Create(context.Background(), obj); !apierrors.IsAlreadyExists(err) {
				break
			}
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
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

// stateConversation is the manager's /state wire shape of a conversation. Its
// spec and status are the API types themselves, so a reader written against
// the Kubernetes object reads this unchanged.
type stateConversation struct {
	Name      string                              `json:"name"`
	ID        string                              `json:"id"`
	CreatedAt time.Time                           `json:"createdAt"`
	Spec      agentopsv1alpha1.ConversationSpec   `json:"spec"`
	Status    agentopsv1alpha1.ConversationStatus `json:"status"`
}

func (w stateConversation) object(namespace string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{Spec: w.Spec, Status: w.Status}
	c.Name, c.Namespace, c.UID = w.Name, namespace, types.UID(w.ID)
	c.CreationTimestamp = metav1.NewTime(w.CreatedAt)
	return c
}

// stateSignal is a /state/signals item. Spec and status are the API types
// themselves, as for a conversation, so a reader written against the
// Kubernetes object reads this unchanged.
type stateSignal struct {
	Name      string                        `json:"name"`
	ID        string                        `json:"id"`
	CreatedAt time.Time                     `json:"createdAt"`
	Spec      agentopsv1alpha1.SignalSpec   `json:"spec"`
	Status    agentopsv1alpha1.SignalStatus `json:"status"`
}

func (w stateSignal) object(namespace string) *agentopsv1alpha1.Signal {
	s := &agentopsv1alpha1.Signal{Spec: w.Spec, Status: w.Status}
	s.Name, s.Namespace, s.UID = w.Name, namespace, types.UID(w.ID)
	s.CreationTimestamp = metav1.NewTime(w.CreatedAt)
	return s
}

// stateGet reads one /state path into out. A 404 comes back as the same
// NotFound error the Kubernetes client gives, so callers need not know which
// source answered.
func (k *Kube) stateGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.StateURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+k.StateToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return apierrors.NewNotFound(schema.GroupResource{Group: "agentops.dev", Resource: "conversations"}, path)
	case resp.StatusCode/100 != 2:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
