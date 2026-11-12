package commandbus

import "errors"

// ErrNotFound is returned by GetTask for a UID no backend ever issued or
// that has since been swept out of retention.
var ErrNotFound = errors.New("commandbus: task not found")

// ErrLeaseLost is returned by Settle and Fail when leaseToken does not match
// the task's current lease: already settled, reclaimed as lapsed and handed
// to another consumer, or never leased at all. A duplicate, at-least-once
// completion of already-finished work is exactly this error, and a caller
// treats it as a no-op rather than a failure.
var ErrLeaseLost = errors.New("commandbus: lease lost")

// ErrUnknownQueue is returned by Submit and Claim for a (Queue, Kind) pair
// Resolve does not find in DefaultInventory.
var ErrUnknownQueue = errors.New("commandbus: unknown queue")

// ErrInvalidRequest is returned by Submit for a request missing a field the
// contract requires (CommandID, Key).
var ErrInvalidRequest = errors.New("commandbus: invalid request")

// ErrorCode is the closed vocabulary a Fail call records (design D-6).
// Retryability is derived from the code (Retryable) — nothing else decides
// it.
type ErrorCode string

const (
	CodeNotFound           ErrorCode = "not_found"
	CodePreconditionFailed ErrorCode = "precondition_failed"
	CodeForbidden          ErrorCode = "forbidden"
	CodeConflict           ErrorCode = "conflict"
	CodeUnavailable        ErrorCode = "unavailable"
	CodeInvalidCommand     ErrorCode = "invalid_command"
	CodeInternal           ErrorCode = "internal"
)

// knownCodes is the closed set ErrorCode may hold after normalize.
var knownCodes = map[ErrorCode]bool{
	CodeNotFound: true, CodePreconditionFailed: true, CodeForbidden: true,
	CodeConflict: true, CodeUnavailable: true, CodeInvalidCommand: true, CodeInternal: true,
}

// Retryable reports whether a failure of this code earns another attempt
// with backoff (design D-6: conflict, unavailable, internal) or goes
// straight to dead (not_found, precondition_failed, forbidden,
// invalid_command). A code outside the closed set reports false — Fail
// never persists one, normalizing it to CodeInternal first — but Retryable
// stays total so a caller holding a raw value cannot panic on it.
func (c ErrorCode) Retryable() bool {
	switch c {
	case CodeConflict, CodeUnavailable, CodeInternal:
		return true
	default:
		return false
	}
}

// ParseErrorCode is the one boundary where a producer's raw string becomes
// an ErrorCode (design D-6: "an unknown code is recorded as internal"). A
// consumer handler calls it on whatever a downstream failure reported —
// an adapter's error string, a Kubernetes error reason — before calling
// Fail; Fail itself also normalizes defensively, so a caller that
// constructs an ErrorCode by hand still cannot persist one outside the
// closed set.
func ParseErrorCode(raw string) ErrorCode {
	return normalizeCode(ErrorCode(raw))
}

// normalizeCode maps any value outside the closed set to CodeInternal.
func normalizeCode(c ErrorCode) ErrorCode {
	if knownCodes[c] {
		return c
	}
	return CodeInternal
}
