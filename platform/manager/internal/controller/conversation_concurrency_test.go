package controller

import "testing"

// The conversation controller runs several workers: one conversation's slow
// reconcile must not hold up every other conversation's lifecycle.
func TestConversationControllerRunsSeveralWorkers(t *testing.T) {
	if got := (&ConversationReconciler{}).concurrency(); got < 2 {
		t.Fatalf("default workers = %d, want at least 2", got)
	}
	if got := (&ConversationReconciler{Concurrency: 7}).concurrency(); got != 7 {
		t.Fatalf("configured workers = %d, want 7", got)
	}
}
