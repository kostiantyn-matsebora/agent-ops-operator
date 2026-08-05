// Package chat is the channel-type-agnostic core of the conversation surface:
// the Provider contract for in-process (built-in) channel types, the Registry
// that resolves a Channel's type to one, the outbound operation queue external
// adapters consume over HTTP, and the transport-neutral Router that turns
// inbound messages into Conversations. Concrete transports (Telegram, Slack, …)
// live outside the manager as adapters; see channel-telegram/ for the
// reference implementation.
package chat

import (
	"context"
	"sync"
)

// Provider is an in-process channel implementation bound to one Channel.
// External adapters do not implement this — they consume the op queue instead.
type Provider interface {
	// EnsureTopic creates a conversation thread and returns its id (opaque
	// string in the channel type's own id space).
	EnsureTopic(ctx context.Context, title string) (string, error)
	// Send posts a message (chat HTML subset); nil threadID targets the
	// channel's general surface.
	Send(ctx context.Context, threadID *string, text string) error
}

// ProviderFactory builds a Provider for one Channel of a registered type.
type ProviderFactory func(ctx context.Context, channelName string) (Provider, error)

// Registry maps channel types to in-process provider factories. Types not
// registered here are served by external adapters via the op queue.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ProviderFactory
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]ProviderFactory{}}
}

// Register claims a channel type for an in-process implementation.
func (r *Registry) Register(channelType string, f ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[channelType] = f
}

// Resolve returns the factory for a type, or nil when the type is external.
func (r *Registry) Resolve(channelType string) ProviderFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.factories[channelType]
}
