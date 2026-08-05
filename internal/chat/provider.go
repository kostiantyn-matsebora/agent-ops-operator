// Package chat abstracts the conversation surface. v1 ships Telegram
// (supergroup with Topics); Slack/Discord are future implementations of the
// same interface.
package chat

import "context"

// Provider is a chat surface bound to one channel (e.g. one Telegram supergroup).
type Provider interface {
	// EnsureTopic creates a forum topic and returns its thread id.
	EnsureTopic(ctx context.Context, title string) (int64, error)
	// Send posts an HTML message; nil threadID targets the general topic.
	Send(ctx context.Context, threadID *int64, html string) error
}
