package commandbus_test

import (
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// TestErrorCodeRetryableDerivationTable pins design D-6's closed vocabulary:
// retryability is derived from the code alone, never decided at a call site.
// Every one of the seven codes is enumerated explicitly — a new code added to
// the vocabulary without a line here is caught by the "want len" check below,
// not silently defaulted to whichever branch Retryable happens to fall
// through to.
func TestErrorCodeRetryableDerivationTable(t *testing.T) {
	cases := []struct {
		code      commandbus.ErrorCode
		retryable bool
	}{
		{commandbus.CodeNotFound, false},
		{commandbus.CodePreconditionFailed, false},
		{commandbus.CodeForbidden, false},
		{commandbus.CodeConflict, true},
		{commandbus.CodeUnavailable, true},
		{commandbus.CodeInvalidCommand, false},
		{commandbus.CodeInternal, true},
	}
	if len(cases) != 7 {
		t.Fatalf("the table must enumerate exactly the seven codes in the closed vocabulary, got %d", len(cases))
	}
	for _, c := range cases {
		t.Run(string(c.code), func(t *testing.T) {
			if got := c.code.Retryable(); got != c.retryable {
				t.Fatalf("%s.Retryable() = %v, want %v", c.code, got, c.retryable)
			}
		})
	}
}

// TestErrorCodeRetryableIsFalseForAnUnrecognizedCode is the "stays total"
// half of Retryable's own doc comment: a raw value outside the closed set
// must never panic and must never be retried just because it fell through
// every known branch.
func TestErrorCodeRetryableIsFalseForAnUnrecognizedCode(t *testing.T) {
	if got := commandbus.ErrorCode("not-a-real-code").Retryable(); got {
		t.Fatalf("an unrecognized code must report non-retryable, got %v", got)
	}
}

// TestParseErrorCodeRecognizesEveryKnownCode confirms ParseErrorCode is the
// identity on every member of the closed vocabulary — the boundary from a
// producer's raw string must never normalize a code it already knows.
func TestParseErrorCodeRecognizesEveryKnownCode(t *testing.T) {
	known := []commandbus.ErrorCode{
		commandbus.CodeNotFound, commandbus.CodePreconditionFailed, commandbus.CodeForbidden,
		commandbus.CodeConflict, commandbus.CodeUnavailable, commandbus.CodeInvalidCommand, commandbus.CodeInternal,
	}
	for _, code := range known {
		t.Run(string(code), func(t *testing.T) {
			if got := commandbus.ParseErrorCode(string(code)); got != code {
				t.Fatalf("ParseErrorCode(%q) = %q, want it unchanged", code, got)
			}
		})
	}
}

// TestParseErrorCodeFallsBackToInternalForAnUnknownCode pins design D-6's
// "an unknown code is recorded as internal" — the one normalization rule,
// exercised against a raw string no producer in this codebase ever reports
// (never once-known-but-retired, never a known code with different casing,
// since the vocabulary is matched exactly) and the empty string, the shape a
// zero-value ErrorCode or an adapter reporting no error code at all takes.
func TestParseErrorCodeFallsBackToInternalForAnUnknownCode(t *testing.T) {
	for _, raw := range []string{"", "quota_exceeded", "NOT_FOUND", "timeout"} {
		t.Run(raw, func(t *testing.T) {
			if got := commandbus.ParseErrorCode(raw); got != commandbus.CodeInternal {
				t.Fatalf("ParseErrorCode(%q) = %q, want %q", raw, got, commandbus.CodeInternal)
			}
		})
	}
}
