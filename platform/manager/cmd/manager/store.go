package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
	natsstate "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/nats"
)

// stateBackendEnv names the variable selecting the state store backend. The
// chart renders it from stateStore.backend; unset means the kube backend
// (the chart's own default, task 3.5, selects nats explicitly — this binary
// still falls back to kube absent the variable, so a direct run or an older
// render keeps today's behaviour).
const stateBackendEnv = "STATE_BACKEND"

// backendKube is the legacy backend: the Conversation and ConversationInput
// custom resources, behind the state contract.
const backendKube = "kube"

// backendNats is the nats-state-store-default backend (design D-1): the
// closed inventory over JetStream KV and Object Store buckets.
const backendNats = "nats"

// stateBackendFromEnv resolves the selected backend name and refuses one this
// binary does not carry, naming it. Called before the manager connects to
// anything, so a bad value stops the process at once.
func stateBackendFromEnv() (string, error) {
	name := strings.TrimSpace(os.Getenv(stateBackendEnv))
	switch name {
	case "":
		return backendKube, nil
	case backendKube, backendNats:
		return name, nil
	}
	return "", fmt.Errorf("%s=%q is not a state backend this manager carries (known: %s, %s)",
		stateBackendEnv, name, backendKube, backendNats)
}

// newStateStores constructs the selected backend's state.Stores once, scoped to
// namespaces, and registers whatever a field must run beside the manager (the
// signals' expiry sweep, and — for nats — the leader-gated one-time import
// and the readiness check it gates, design D-10). The inventory is built here
// and nowhere else: the reconcilers, the HTTP API and chat each take the
// typed fields they use, so what a component may do with state is what its
// field types allow.
func newStateStores(backend string, mgr ctrl.Manager, namespaces namespace.Namespaces) (state.Stores, error) {
	switch backend {
	case backendKube:
		return newKubeStores(mgr, namespaces)
	case backendNats:
		return newNatsStores(mgr, namespaces)
	}
	return state.Stores{}, fmt.Errorf("%s=%q is not a state backend this manager carries", stateBackendEnv, backend)
}

// newKubeStores is unchanged from before this change beyond the namespace
// pair: an uncached client for the compare-and-swap writes, lists served from
// the informer cache (state-store design D-4, amended).
func newKubeStores(mgr ctrl.Manager, namespaces namespace.Namespaces) (state.Stores, error) {
	c, err := client.NewWithWatch(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return state.Stores{}, fmt.Errorf("building the uncached client for the kube state store: %w", err)
	}
	kubeStores, err := kube.OpenStores(c, namespaces, kube.WithCachedLists(mgr.GetCache()))
	if err != nil {
		return state.Stores{}, fmt.Errorf("opening the kube state store: %w", err)
	}
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		kubeStores.Run(ctx)
		return nil
	})); err != nil {
		return state.Stores{}, fmt.Errorf("registering the kube state store's expiry sweep: %w", err)
	}
	return kubeStores.Stores, nil
}

// newNatsStores opens the nats backend (task 2.8): NATS_URL, credentials and
// replicas from env (never a mounted file — invariants.md), reports not
// ready while the server is unreachable or the one-time import has not
// completed, and gates the import itself on this replica holding the leader
// Lease, repeating while an old-version replica's pod may still be writing
// (design D-10).
// natsOpenTimeout bounds how long startup waits for the state server to accept
// its buckets. Zero, the default, waits until it does: whether the manager has
// waited too long is the startup probe's decision, never the manager's. A
// variable so a test can bound it.
var natsOpenTimeout time.Duration

func newNatsStores(mgr ctrl.Manager, namespaces namespace.Namespaces) (state.Stores, error) {
	cfg, err := natsstate.ConfigFromEnv()
	if err != nil {
		return state.Stores{}, fmt.Errorf("nats state store: %w", err)
	}
	conn, err := natsstate.Connect(cfg)
	if err != nil {
		return state.Stores{}, fmt.Errorf("nats state store: %w", err)
	}

	// A cluster still forming its quorum cannot place a replicated bucket yet.
	// Wait for it instead of exiting into a crash loop.
	openCtx, cancelOpen := openWaitContext(natsOpenTimeout)
	defer cancelOpen()
	// Under nats a conversation is a record, not a CR (design D-1 of
	// split-conversation-namespace): the split moves no CR here, only
	// workloads and claims, so the backend keeps the single install-scope
	// string it always took — the CONTROL half, which is what today's
	// single-namespace value already was.
	backend, err := natsstate.OpenStoresWhenReady(openCtx, conn, namespaces.Control, natsstate.DefaultOpenRetryInterval)
	if err != nil {
		return state.Stores{}, fmt.Errorf("opening the nats state store: %w", err)
	}
	if len(backend.Retired) > 0 {
		ctrl.Log.WithName("setup").Info("removed buckets no code uses any more", "buckets", backend.Retired)
	}
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		backend.Run(ctx)
		return nil
	})); err != nil {
		return state.Stores{}, fmt.Errorf("registering the nats state store's expiry sweep: %w", err)
	}

	// The one-time import's kube SOURCE spans both namespaces, by kind (task
	// 3.2): conversation kinds read from Conversations, everything else from
	// Control — exactly kube.OpenStores' own split below.
	if err := registerNatsImport(mgr, conn, backend, namespaces); err != nil {
		return state.Stores{}, err
	}
	if err := mgr.AddReadyzCheck("state-store", natsReadinessCheck(conn, backend, mgr.Elected())); err != nil {
		return state.Stores{}, fmt.Errorf("registering the nats state store's readiness check: %w", err)
	}
	return backend.Stores, nil
}

// natsReadinessCheck answers "not ready" while the server cannot be reached,
// and, on the elected replica, while the one-time import has not completed:
// task 2.8's own verify clause, and design D-10's "the manager serves nothing
// until it returns". A replica that does not hold the Lease runs no import and
// serves nothing itself, every route proxies to the leader, so it is ready
// once the server is reachable. Gating it on the import too would deadlock an
// upgrade from kube: the old replicas hold the Lease until the new ones are
// ready, and the new ones can never import without it.
func natsReadinessCheck(conn *natsstate.Conn, backend *natsstate.Backend, elected <-chan struct{}) healthz.Checker {
	return func(req *http.Request) error {
		ctx := req.Context()
		if err := conn.Reachable(ctx); err != nil {
			return err
		}
		select {
		case <-elected:
		default:
			return nil
		}
		imported, err := backend.Imported(ctx)
		if err != nil {
			return err
		}
		if !imported {
			return fmt.Errorf("the one-time import from kube has not completed")
		}
		return nil
	}
}

// registerNatsImport wires the leader-gated import Runnable: it starts only
// once this replica is elected (design D-10, step 1), reading the kube
// backend read-only and the SignalSource/Channel names and adapter-state
// entries the generic contract cannot enumerate on its own (import.go's own
// SourceNames).
func registerNatsImport(mgr ctrl.Manager, conn *natsstate.Conn, backend *natsstate.Backend, namespaces namespace.Namespaces) error {
	kubeClient, err := client.NewWithWatch(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return fmt.Errorf("building the read-only client for the import's kube source: %w", err)
	}
	kubeStores, err := kube.OpenStores(kubeClient, namespaces)
	if err != nil {
		return fmt.Errorf("opening the import's kube source: %w", err)
	}
	importer := &natsstate.Importer{
		Source: kubeStores.Stores,
		// SignalSource and adapter state are control-plane reads, same as
		// every other kube-store kind outside the conversation ones.
		SourceNames:       kubeSourceNames{c: kubeClient, namespace: namespaces.Control},
		Dest:              backend,
		OldReplicaPresent: oldManagerReplicaPresent(kubeClient, namespaces.Control, env("VERSION", "dev")),
	}
	return mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		return runWhenElected(ctx, mgr.Elected(), importer)
	}))
}

// runner is what runWhenElected runs: the import, in production.
type runner interface {
	Run(ctx context.Context) error
}

// runWhenElected waits for this replica to win the leader Lease, then runs r.
// A replica that never wins it, or whose context ends first, imports nothing:
// only one replica copies the old state across, and a standby must not race it.
func runWhenElected(ctx context.Context, elected <-chan struct{}, r runner) error {
	select {
	case <-elected:
	case <-ctx.Done():
		return ctx.Err()
	}
	return r.Run(ctx)
}

// kubeSourceNames implements natsstate.SourceNames over the kube client: the
// SignalSource/Channel listing is unguarded (neither type is a kind
// internal/state's own static check restricts), and the adapter-state read
// goes through internal/state/kube's one export built for this.
type kubeSourceNames struct {
	c         client.Reader
	namespace string
}

func (k kubeSourceNames) SignalSourceNames(ctx context.Context) ([]string, error) {
	var list agentopsv1alpha1.SignalSourceList
	if err := k.c.List(ctx, &list, client.InNamespace(k.namespace)); err != nil {
		return nil, fmt.Errorf("list SignalSources for the import: %w", err)
	}
	names := make([]string, len(list.Items))
	for i := range list.Items {
		names[i] = list.Items[i].Name
	}
	return names, nil
}

func (k kubeSourceNames) AdapterStateEntries(ctx context.Context) ([]natsstate.AdapterStateEntry, error) {
	entries, err := kube.EnumerateAdapterState(ctx, k.c, k.namespace)
	if err != nil {
		return nil, err
	}
	out := make([]natsstate.AdapterStateEntry, len(entries))
	for i, e := range entries {
		out[i] = natsstate.AdapterStateEntry{Key: e.Key, Value: e.Record}
	}
	return out, nil
}

// oldManagerReplicaPresent reports whether a manager pod carrying a
// different VERSION than thisVersion exists (design D-10, step 4): pods
// carry app.kubernetes.io/name=agentops-manager and a VERSION container env
// var (chart's deployment.yaml), both already true of every release this
// binary ships in.
func oldManagerReplicaPresent(c client.Reader, namespace, thisVersion string) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		var pods corev1.PodList
		if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{"app.kubernetes.io/name": "agentops-manager"}); err != nil {
			return false, fmt.Errorf("list manager pods: %w", err)
		}
		for _, pod := range pods.Items {
			if !pod.DeletionTimestamp.IsZero() {
				continue
			}
			if podVersion(pod) != "" && podVersion(pod) != thisVersion {
				return true, nil
			}
		}
		return false, nil
	}
}

// podVersion reads the VERSION container env var off a manager pod's own
// spec (never its live status, which this read-only client may not have
// RBAC to resolve beyond what it already lists).
func podVersion(pod corev1.Pod) string {
	for _, c := range pod.Spec.Containers {
		for _, e := range c.Env {
			if e.Name == "VERSION" {
				return e.Value
			}
		}
	}
	return ""
}

// openWaitContext is the context a start-up wait for a dependency runs under:
// bounded by timeout when one is set, otherwise until the process is stopped.
func openWaitContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithCancel(context.Background())
}
