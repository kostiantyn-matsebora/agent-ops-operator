package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

// stateBackendEnv names the variable selecting the state store backend. The
// chart renders it from stateStore.backend; unset means the kube backend.
const stateBackendEnv = "STATE_BACKEND"

// backendKube is today's backend: the Conversation and ConversationInput custom
// resources, behind the state.Store contract.
const backendKube = "kube"

// stateBackendFromEnv resolves the selected backend name and refuses one this
// binary does not carry, naming it. Called before the manager connects to
// anything, so a bad value stops the process at once.
func stateBackendFromEnv() (string, error) {
	name := strings.TrimSpace(os.Getenv(stateBackendEnv))
	switch name {
	case "", backendKube:
		return backendKube, nil
	}
	return "", fmt.Errorf("%s=%q is not a state backend this manager carries (known: %s)",
		stateBackendEnv, name, backendKube)
}

// newStateStores constructs the selected backend's two stores scoped to
// namespace — the conversation Store and the SignalStore — and registers
// whatever each must run beside the manager.
//
// The kube backend reads through a client with no cache: its Get and Update
// carry the compare-and-swap, and a cached read would make that precondition
// a guess (state-store design D-4). List alone is served from the informer
// cache, since a lookup is not a precondition and a full list of a large
// install is slow (D-4, amended).
func newStateStores(backend string, mgr ctrl.Manager, namespace string) (state.Store, state.SignalStore, error) {
	switch backend {
	case backendKube:
		c, err := client.NewWithWatch(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
		if err != nil {
			return nil, nil, fmt.Errorf("building the uncached client for the kube state store: %w", err)
		}
		// Lists come from the informer cache, Get and the writes do not.
		store := kube.NewStoreWithSweepInterval(c, namespace, kube.DefaultSweepInterval).WithCachedLists(mgr.GetCache())
		signals := kube.NewSignalStoreWithSweepInterval(c, namespace, kube.DefaultSweepInterval).WithCachedLists(mgr.GetCache())
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			store.Run(ctx)
			return nil
		})); err != nil {
			return nil, nil, fmt.Errorf("registering the kube state store's expiry sweep: %w", err)
		}
		if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
			signals.Run(ctx)
			return nil
		})); err != nil {
			return nil, nil, fmt.Errorf("registering the kube signal store's expiry sweep: %w", err)
		}
		return store, signals, nil
	}
	return nil, nil, fmt.Errorf("%s=%q is not a state backend this manager carries", stateBackendEnv, backend)
}
