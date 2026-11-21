// Package consumer runs a pool of goroutines claiming tasks off one
// commandbus.Queue and dispatching each to a registered HandlerFunc,
// settling or failing it depending on the result (async-command-bus design
// D-1/D-2, task 4.2 of that change's tasks.md).
//
// One Pool runs on EVERY replica, never gated behind the leader-election
// Lease the chat/signal reconcilers still use — that is the whole point of
// moving work off the leader-only OpQueue (proposal.md's "Why"): the lease
// commandbus.Bus.Claim hands back, not which process holds a Kubernetes
// Lease, is what makes a claim safe to run anywhere.
//
// This package registers no handler of its own. A later change (tasks.md
// sections 5/6) builds the real ones — ensure-topic, send, close-topic,
// delete-conversation, signal intake — and wires them in from cmd/manager.
package consumer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// HandlerFunc processes one claimed task. A nil return Settles it; a
// non-nil one Fails it, with the code codeFor derives (WithCode's explicit
// code, or commandbus.ParseErrorCode(err.Error()) otherwise — design D-6:
// "an unknown code is recorded as internal").
type HandlerFunc func(ctx context.Context, task commandbus.Task) error

// CodedError lets a handler say "this is commandbus.CodeNotFound" rather
// than always falling through to CodeInternal (design D-6). Built by
// WithCode, never directly — a caller needs no field access, only
// errors.As to recognise one it did not create itself (a handler composing
// another's returned error).
type CodedError struct {
	code commandbus.ErrorCode
	err  error
}

// WithCode wraps err so the pool's Fail call records code instead of
// deriving one from err's own message. Returns nil for a nil err — nothing
// to wrap, and a handler that returns WithCode(code, err) unconditionally
// (not only on a non-nil err) must still Settle on success.
func WithCode(code commandbus.ErrorCode, err error) error {
	if err == nil {
		return nil
	}
	return &CodedError{code: code, err: err}
}

func (e *CodedError) Error() string { return e.err.Error() }
func (e *CodedError) Unwrap() error { return e.err }

// codeFor derives the commandbus.ErrorCode a Fail call records for err:
// the code a CodedError carries, normalized exactly as Fail itself
// normalizes defensively, or — for a handler that did not call WithCode —
// ParseErrorCode on the error's own text.
func codeFor(err error) commandbus.ErrorCode {
	var coded *CodedError
	if errors.As(err, &coded) {
		return commandbus.ParseErrorCode(string(coded.code))
	}
	return commandbus.ParseErrorCode(err.Error())
}

// claimRetryInterval bounds how long the pool waits before asking Claim
// again after it returned an error that was NOT the context ending — a
// transient bus failure (e.g. the nats backend's own CAS conflict surfaced
// as an error rather than retried internally). Short: Claim's own internal
// loop already backs off waiting for a task to become claimable: this is
// only the fallback for the bus itself misbehaving.
const claimRetryInterval = time.Second

// DefaultHandlerTimeout bounds one message's processing when Config sets none.
// It is the bound the bus's lease has always been, so a handler that runs
// longer than it was never going to keep its message anyway.
const DefaultHandlerTimeout = 5 * time.Minute

// Config configures one Pool. Every field is required except ConsumerID.
type Config struct {
	// HandlerTimeout is the longest one message may be processed. At the
	// deadline the handler's context is cancelled and the attempt is failed with
	// a retryable code, so a message is never processed forever. Zero is
	// DefaultHandlerTimeout.
	HandlerTimeout time.Duration
	// Bus is claimed from and settled/failed against.
	Bus commandbus.Bus
	// Queue is the one FIFO lane this pool claims from.
	Queue commandbus.Queue
	// Handler processes a claimed task. Called with this Pool's Run
	// context — a handler that outlives it is a leak this pool makes no
	// attempt to bound.
	Handler HandlerFunc
	// Concurrency is how many goroutines claim from Queue at once. Must be
	// positive — New refuses zero or negative rather than silently running
	// no consumer at all.
	Concurrency int
	// ConsumerID names this replica's claims for a human reading a Lease's
	// Holder back (/state/tasks) — never consulted by Settle or Fail.
	// Defaults to the queue's own name when empty; one pool's goroutines
	// are distinguished from each other by an appended index, never by a
	// caller-supplied value.
	ConsumerID string
}

// Pool runs Config.Concurrency goroutines, each in its own claim/handle/
// settle-or-fail loop, until its Run context ends.
type Pool struct {
	cfg Config
}

// New validates cfg and returns a Pool ready to Run. Validating here,
// rather than inside Run, is what lets cmd/manager fail at startup — the
// same "stop before anything runs" posture stateBackendFromEnv and
// commandBusBackendFromEnv already hold.
func New(cfg Config) (*Pool, error) {
	if cfg.Bus == nil {
		return nil, fmt.Errorf("commandbus/consumer: Config.Bus is nil")
	}
	if cfg.Queue == "" {
		return nil, fmt.Errorf("commandbus/consumer: Config.Queue is empty")
	}
	if cfg.Handler == nil {
		return nil, fmt.Errorf("commandbus/consumer: Config.Handler is nil")
	}
	if cfg.Concurrency <= 0 {
		return nil, fmt.Errorf("commandbus/consumer: Config.Concurrency must be positive, got %d", cfg.Concurrency)
	}
	if cfg.ConsumerID == "" {
		cfg.ConsumerID = string(cfg.Queue)
	}
	return &Pool{cfg: cfg}, nil
}

// Run blocks, running Config.Concurrency claim loops until ctx is done,
// then waits for every goroutine's current claim to finish settling or
// failing before returning — never abandoning a task mid-handle just
// because the context ended under it. Matches the shape every other
// manager.RunnableFunc in cmd/manager already has (e.g. the state store's
// own expiry sweep).
func (p *Pool) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for i := 0; i < p.cfg.Concurrency; i++ {
		consumerID := fmt.Sprintf("%s-%d", p.cfg.ConsumerID, i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.loop(ctx, consumerID)
		}()
	}
	wg.Wait()
	return nil
}

// loop claims, handles and settles or fails, one task at a time, until ctx
// ends. A Claim error that is not ctx ending is the bus itself failing —
// logged and retried after claimRetryInterval rather than returning, since
// one goroutine giving up would silently shrink this pool's own configured
// concurrency for good.
func (p *Pool) loop(ctx context.Context, consumerID string) {
	logger := log.FromContext(ctx).WithValues("queue", p.cfg.Queue, "consumer", consumerID)
	for {
		task, err := p.cfg.Bus.Claim(ctx, p.cfg.Queue, consumerID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error(err, "claim failed, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(claimRetryInterval):
			}
			continue
		}
		p.settleOrFail(ctx, logger, task)
	}
}

// settleOrFail runs Handler on task and records its outcome. Errors from
// Settle/Fail themselves are logged, never returned — ErrLeaseLost is an
// expected at-least-once outcome (the task was already reclaimed or
// completed by another consumer), and any other error leaves the task for
// the bus's own reaping to resolve rather than crashing a goroutine that a
// transient store write must not be allowed to take down.
func (p *Pool) settleOrFail(ctx context.Context, logger logr.Logger, task commandbus.Task) {
	leaseToken := ""
	if task.Lease != nil {
		leaseToken = task.Lease.Token
	}
	err := p.handle(ctx, logger, task, leaseToken)
	if err == nil {
		if sErr := p.cfg.Bus.Settle(ctx, task.UID, leaseToken); sErr != nil && !errors.Is(sErr, commandbus.ErrLeaseLost) {
			logger.Error(sErr, "settle failed", "task", task.UID)
		}
		return
	}
	if fErr := p.cfg.Bus.Fail(ctx, task.UID, leaseToken, codeFor(err), err.Error()); fErr != nil && !errors.Is(fErr, commandbus.ErrLeaseLost) {
		logger.Error(fErr, "fail failed", "task", task.UID)
	}
}

// handle runs the handler under its deadline, keeping the lease alive while it
// works when the bus asks for that. A handler that overruns, or whose lease was
// taken from it, has its context cancelled; one that ignores the context is
// left behind and the attempt is failed without waiting for it.
func (p *Pool) handle(ctx context.Context, logger logr.Logger, task commandbus.Task, leaseToken string) error {
	timeout := p.cfg.HandlerTimeout
	if timeout <= 0 {
		timeout = DefaultHandlerTimeout
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if r, ok := p.cfg.Bus.(commandbus.Renewer); ok && task.Lease != nil {
		if every := time.Until(task.Lease.Until) / 3; every > 0 {
			go renewWhile(hctx, cancel, r, task.UID, leaseToken, every, logger)
		}
	}
	done := make(chan error, 1)
	go func() { done <- p.cfg.Handler(hctx, task) }()
	select {
	case err := <-done:
		if err != nil && errors.Is(hctx.Err(), context.DeadlineExceeded) {
			return WithCode(commandbus.CodeUnavailable, fmt.Errorf("handler timed out after %s: %w", timeout, err))
		}
		return err
	case <-hctx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		logger.Info("handler did not stop at its deadline, failing the attempt", "task", task.UID, "timeout", timeout.String())
		return WithCode(commandbus.CodeUnavailable, fmt.Errorf("handler timed out after %s", timeout))
	}
}

// renewWhile extends the lease every interval until ctx ends. A lease that is
// lost cancels the handler: another consumer may now hold the message.
func renewWhile(ctx context.Context, cancel context.CancelFunc, r commandbus.Renewer, uid, token string, every time.Duration, logger logr.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.Renew(ctx, uid, token); err != nil {
				if errors.Is(err, commandbus.ErrLeaseLost) {
					cancel()
					return
				}
				logger.V(1).Info("lease renewal failed, will retry", "task", uid, "error", err.Error())
			}
		}
	}
}
