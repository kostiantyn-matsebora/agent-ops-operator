package kube

import (
	"context"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Backend is the kube backend as the manager wires it: the closed inventory,
// with every field opened over the same client and namespace, and the expiry
// sweeps the expiring collections need running beside the manager.
type Backend struct {
	// Stores is the inventory, every field implemented by this backend.
	Stores state.Stores

	sweeps []interface{ Run(context.Context) }
}

// OpenStores opens every field of state.Stores, scoped to the namespace pair
// (split-conversation-namespace): Conversations, Runs and Blobs open in
// ns.Conversations — conversation-scoped state, under the placement rule.
// Signals, Cooldowns, Receipts and AdapterState open in ns.Control, beside
// every other control-plane kind. The sweeps are not started: Run does that,
// once the caller has somewhere to run them.
//
// A single-namespace caller passes a pair of two equal strings, which is
// exactly today's behaviour: every field opens in the one namespace it
// always did.
func OpenStores(c client.WithWatch, ns namespace.Namespaces, opts ...Option) (*Backend, error) {
	conversations, err := Open(c, ns.Conversations, ConversationLayout(), opts...)
	if err != nil {
		return nil, err
	}
	runs, err := OpenAppended(c, ns.Conversations, RunsLayout(), opts...)
	if err != nil {
		return nil, err
	}
	signals, err := OpenExpiring(c, ns.Control, SignalLayout(), opts...)
	if err != nil {
		return nil, err
	}
	cooldowns, err := Open(c, ns.Control, CooldownLayout(), opts...)
	if err != nil {
		return nil, err
	}
	receipts, err := Open(c, ns.Control, ReceiptLayout(), opts...)
	if err != nil {
		return nil, err
	}
	adapterState, err := OpenAdapterState(c, ns.Control, opts...)
	if err != nil {
		return nil, err
	}
	return &Backend{
		Stores: state.Stores{
			Conversations: conversations,
			Runs:          runs,
			Signals:       signals,
			Cooldowns:     cooldowns,
			Receipts:      receipts,
			AdapterState:  adapterState,
			Blobs:         OpenBlobs(c, ns.Conversations, opts...),
		},
		sweeps: []interface{ Run(context.Context) }{signals},
	}, nil
}

// Run runs every expiry sweep until ctx is done.
func (b *Backend) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, s := range b.sweeps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Run(ctx)
		}()
	}
	wg.Wait()
}
