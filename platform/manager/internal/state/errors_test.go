package state

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrConflictDistinguishableFromAnyOtherError(t *testing.T) {
	wrapped := fmt.Errorf("update %q: %w", "conv-1", ErrConflict)
	if !errors.Is(wrapped, ErrConflict) {
		t.Fatalf("errors.Is(%v, ErrConflict) = false, want true", wrapped)
	}

	other := errors.New("boom")
	if errors.Is(other, ErrConflict) {
		t.Fatalf("errors.Is(%v, ErrConflict) = true, want false", other)
	}
	if errors.Is(ErrNotFound, ErrConflict) {
		t.Fatal("ErrNotFound must not be mistaken for ErrConflict")
	}
	if errors.Is(ErrAlreadyExists, ErrConflict) {
		t.Fatal("ErrAlreadyExists must not be mistaken for ErrConflict: a create names no revision to lose a race over")
	}
}
