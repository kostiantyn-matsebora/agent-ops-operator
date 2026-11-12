// HandleConvTask is the "conv" queue's consumer.HandlerFunc (async-command-
// bus, task 5.2/5.3/5.5/5.6): the PROCESS half of every verb accept.go
// enqueues, and of the two verbs internal/controller's reconciler enqueues
// directly (task 5.4). Registered against a consumer.Pool from cmd/manager,
// it runs on whichever replica next claims the task — never gated on
// leader election, unlike the chat/signal reconcilers these verbs used to
// run inside.
//
// Every verb below is a thin call into a chat.Router/Server method that
// already existed before this change and is UNCHANGED by it (close, exit,
// reopen, reset-context, the cascades CloseConversation/
// AutoCloseConversation/CloseBudgetExceeded already perform live against
// causedBy): this file only moves WHEN they run, never WHAT they do. See
// accept.go's package doc for which verb gets which Key, and the change's
// own hand-back report for the two simplifications against design D-7 this
// package made under time: a plain close/auto-close/budget-exceeded
// cascade runs wholesale inside one task rather than one task per member
// (delete, task 5.10, is the one cascade decomposed per the letter of D-7).
package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// HandleConvTask decodes task.Payload (convtask.Decode) and dispatches on
// its Verb. An unreadable payload or an unknown verb is
// commandbus.CodeInvalidCommand (design D-6: straight to dead — a producer
// bug, never worth retrying).
func (s *Server) HandleConvTask(ctx context.Context, task commandbus.Task) error {
	p, err := convtask.Decode(task.Payload)
	if err != nil {
		return consumer.WithCode(commandbus.CodeInvalidCommand,
			fmt.Errorf("conv task %s: decode payload: %w", task.UID, err))
	}
	switch p.Verb {
	case convtask.VerbClose:
		return s.taskClose(ctx, &p)
	case convtask.VerbExit:
		return s.taskExit(ctx, &p)
	case convtask.VerbReopen:
		return s.taskReopen(ctx, &p)
	case convtask.VerbDelete:
		return s.taskDelete(ctx, &p)
	case convtask.VerbResetContext:
		return s.taskResetContext(ctx, &p)
	case convtask.VerbInboundMessage:
		return s.taskInboundMessage(ctx, &p)
	default:
		return consumer.WithCode(commandbus.CodeInvalidCommand,
			fmt.Errorf("conv task %s: unknown verb %q", task.UID, p.Verb))
	}
}

// closeBudgetExceeded is handleWorkDone's own budget-exceeded call site
// (design D-E's maxTurns edge, task 5.4): ENQUEUE a "conv" task when s.Bus
// is set, falling back to calling chat.Router directly, inline, when it is
// not — the exact pattern internal/controller's own closeBudgetExceeded
// follows for the deadline edge, kept in step rather than duplicated with
// drift. CommandID is stable per conversation, so a /work/done redelivery
// (same conversation, same digest text or not) resubmits a suppressed id
// rather than a second task.
func (s *Server) closeBudgetExceeded(ctx context.Context, conv *state.ConversationRecord, digest string) error {
	if s.Bus == nil {
		if s.Router == nil {
			return nil
		}
		return s.Router.CloseBudgetExceeded(ctx, conv, digest)
	}
	_, err := s.Bus.Submit(ctx, commandbus.SubmitRequest{
		Queue:     ConvQueue,
		CommandID: commandbus.CommandID("budget-exceeded:" + conv.Name),
		Key:       commandbus.Key(conv.Name),
		Payload: convtask.Payload{
			Verb: convtask.VerbClose, Conversation: conv.Name,
			CloseKind: convtask.CloseBudgetExceeded, Reason: digest,
		}.Encode(),
	})
	return err
}

// convByName re-reads a conversation by name, fresh — a task runs some time
// after accept, so it never trusts anything about the record beyond the
// name and whatever its own payload carries.
func (s *Server) convByName(ctx context.Context, name string) (*state.ConversationRecord, error) {
	conv, _, err := s.Conversations.Get(ctx, state.Key(name))
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// codeForRouterErr maps a chat.Router method's returned error onto the
// closed vocabulary (design D-6) for the ONE case these handlers can tell
// apart cheaply: the conversation this task names no longer exists.
// Everything else is left for Fail's own defensive ParseErrorCode fallback
// (CodeInternal, retried) — a Router method's internal errors are not typed
// finely enough here to do better without guessing.
func codeForRouterErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, state.ErrNotFound) {
		return consumer.WithCode(commandbus.CodeNotFound, err)
	}
	return err
}

func (s *Server) taskClose(ctx context.Context, p *convtask.Payload) error {
	conv, err := s.convByName(ctx, p.Conversation)
	if err != nil {
		return codeForRouterErr(err)
	}
	switch p.CloseKind {
	case convtask.CloseAuto:
		return codeForRouterErr(s.Router.AutoCloseConversation(ctx, conv, time.Duration(p.IdleForMS)*time.Millisecond))
	case convtask.CloseBudgetExceeded:
		return codeForRouterErr(s.Router.CloseBudgetExceeded(ctx, conv, p.Reason))
	default: // convtask.CloseManual, and the empty default — a plain human close.
		return codeForRouterErr(s.Router.CloseConversation(ctx, conv))
	}
}

func (s *Server) taskExit(ctx context.Context, p *convtask.Payload) error {
	conv, err := s.convByName(ctx, p.Conversation)
	if err != nil {
		return codeForRouterErr(err)
	}
	return codeForRouterErr(s.Router.ReleaseRuntime(ctx, conv))
}

func (s *Server) taskReopen(ctx context.Context, p *convtask.Payload) error {
	conv, err := s.convByName(ctx, p.Conversation)
	if err != nil {
		return codeForRouterErr(err)
	}
	// ReopenConversation's own refusals (not Closed, a successor still open,
	// a dangling ref) are all precondition_failed by design D-6's own
	// example ("a refused /exit mid-run must not become a surprise release
	// minutes later") — terminal, never retried, since re-running the exact
	// same check against the exact same state would refuse identically.
	if err := s.Router.ReopenConversation(ctx, conv); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return consumer.WithCode(commandbus.CodeNotFound, err)
		}
		return consumer.WithCode(commandbus.CodePreconditionFailed, err)
	}
	return nil
}

// taskDelete deletes ONE conversation. design D-6/task 5.7: a conversation
// already gone is the outcome a delete wanted, so a not_found here is a
// SUCCESS, not a failure — this is what lets both a top-level delete and
// every cascade-member task it enqueues (handleConversationDelete,
// deepest-first) tolerate a descendant another path already removed.
func (s *Server) taskDelete(ctx context.Context, p *convtask.Payload) error {
	if err := s.Conversations.Delete(ctx, state.Key(p.Conversation)); err != nil && !errors.Is(err, state.ErrNotFound) {
		return err // unparsed: Fail's own ParseErrorCode falls back to internal, retried
	}
	return nil
}

func (s *Server) taskResetContext(ctx context.Context, p *convtask.Payload) error {
	conv, err := s.convByName(ctx, p.Conversation)
	if err != nil {
		return codeForRouterErr(err)
	}
	if conv.ContextID() == "" {
		return nil // nothing to clear — the same "cleared: false" success contextreset.go reported inline
	}
	if err := state.Patch(ctx, s.Conversations, conv, func(c *state.ConversationRecord) {
		c.RuntimeContextID = ""
		c.SessionID = ""
		c.ContextCheckpoint = nil
		c.SetCondition(state.Condition{
			Type: controller.ConditionContextContinuity, Status: state.ConditionFalse,
			Reason: "ResetByOperator",
			Message: "an operator reset this conversation's context; its previous memory is " +
				"gone and the next run starts fresh",
		})
	}); err != nil {
		return codeForRouterErr(err)
	}
	s.announceContextReset(ctx, conv)
	return nil
}

// taskInboundMessage replays HandleMessage WHOLESALE rather than redoing
// only "what's left" after accept's own self-input/closed checks
// (handleChannelInbound): HandleMessage already re-derives both from fresh
// state and is idempotent either way, so calling it unchanged is simpler
// and safer than forking its logic in two places that must then be kept in
// step. Accept's own checks exist to answer the COMMON case immediately,
// with no task at all — this is what runs on every message that cleared
// them.
func (s *Server) taskInboundMessage(ctx context.Context, p *convtask.Payload) error {
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: p.Channel}, &ch); err != nil {
		if apierrors.IsNotFound(err) {
			return consumer.WithCode(commandbus.CodeNotFound, err)
		}
		return err
	}
	if err := s.Router.HandleMessage(ctx, &ch, chat.InboundMessage{ThreadID: p.ThreadID, Text: p.Text, Sender: p.Sender}); err != nil {
		if errors.Is(err, chat.ErrSelfInput) {
			return consumer.WithCode(commandbus.CodeInvalidCommand, err)
		}
		return err
	}
	return nil
}

// inboundMessageCommandID is design D-9's inbound-dedup rule for a channel
// message: the caller's own Idempotency-Key when it presented one, else a
// key derived from channel, thread, text and sender — "no adapter has to
// change" (D-9), so this is computed manager-side, never required of the
// caller.
//
// Hashed rather than carried verbatim: a CommandID is compared for equality
// only (commandbus.CommandID's own doc), and the raw text can be arbitrarily
// long, carry newlines, or exceed whatever a backend is comfortable using as
// a map/bucket key.
func inboundMessageCommandID(idempotencyKey, channel, threadID, text, sender string) commandbus.CommandID {
	if idempotencyKey != "" {
		return commandbus.CommandID("inbound:" + idempotencyKey)
	}
	h := sha256.Sum256([]byte(channel + "\x00" + threadID + "\x00" + text + "\x00" + sender))
	return commandbus.CommandID("inbound:" + hex.EncodeToString(h[:]))
}
