// AllocateName against a real API server: the collision lookup, a genuine
// concurrent-creator race, and exhausting the bounded retry.
package integration

import (
	"context"
	"strings"
	"sync"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namewords"
)

// newBareConversation is the minimum Conversation AllocateName can create:
// ProfileRef is the one required spec field, and Name is left for
// AllocateName itself to set.
func newBareConversation() *agentopsv1alpha1.Conversation {
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = ns
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef: agentopsv1alpha1.ObjectRef{Name: "stub"},
	}
	return conv
}

func TestAllocateNameFirstAllocationHasNoSuffix(t *testing.T) {
	name, err := namewords.AllocateName(context.Background(), k8sClient, ns, "alert", "allocate-first-abc", newBareConversation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	if name != "alert-allocate-first-abc" {
		t.Errorf("name = %q, want the bare base name", name)
	}
}

func TestAllocateNameRecurringBaseGetsTheNextNumber(t *testing.T) {
	ctx := context.Background()
	const kind, words = "alert", "allocate-recur-abc"

	first, err := namewords.AllocateName(ctx, k8sClient, ns, kind, words, newBareConversation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, first) })
	if first != "alert-allocate-recur-abc" {
		t.Errorf("first = %q", first)
	}

	second, err := namewords.AllocateName(ctx, k8sClient, ns, kind, words, newBareConversation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, second) })
	if second != "alert-allocate-recur-abc-2" {
		t.Errorf("second = %q", second)
	}

	third, err := namewords.AllocateName(ctx, k8sClient, ns, kind, words, newBareConversation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, third) })
	if third != "alert-allocate-recur-abc-3" {
		t.Errorf("third = %q", third)
	}
}

// TestAllocateNameRetriesAGenuineCreateConflict races several concurrent
// creators for the SAME base name — the one retry case design.md describes
// as legitimate — and expects every one of them to land on a distinct name
// (base, base-2, base-3, ...) with no error, never a random fallback.
func TestAllocateNameRetriesAGenuineCreateConflict(t *testing.T) {
	ctx := context.Background()
	const kind, words = "member", "allocate-race-abc"
	const n = 3

	var wg sync.WaitGroup
	names := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			names[i], errs[i] = namewords.AllocateName(ctx, k8sClient, ns, kind, words, newBareConversation())
		}(i)
	}
	wg.Wait()

	base := kind + "-" + words
	want := map[string]bool{base: true, base + "-2": true, base + "-3": true}
	seen := map[string]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if seen[names[i]] {
			t.Fatalf("name %q allocated twice", names[i])
		}
		seen[names[i]] = true
		name := names[i]
		t.Cleanup(func() { cleanupConversation(t, name) })
		if !want[names[i]] {
			t.Errorf("unexpected name %q", names[i])
		}
	}
	if len(seen) != n {
		t.Errorf("got %d distinct names, want %d", len(seen), n)
	}
}

// TestAllocateNameExhaustsRetriesOnPersistentConflict simulates a conflict
// AllocateName's own List can never see clearing: a raw conversation
// already sits at the exact bare base name, with no name-base label, so
// every list-then-create attempt recomputes the same name and the same
// Create fails. Exhausting the bound returns the last conflict error,
// naming the base — never a random or hash-derived fallback.
func TestAllocateNameExhaustsRetriesOnPersistentConflict(t *testing.T) {
	ctx := context.Background()
	const kind, words = "member", "allocate-exhaust-abc"
	base := kind + "-" + words

	blocker := newBareConversation()
	blocker.Name = base
	if err := k8sClient.Create(ctx, blocker); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, blocker) })

	_, err := namewords.AllocateName(ctx, k8sClient, ns, kind, words, newBareConversation())
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !apierrors.IsAlreadyExists(err) {
		t.Errorf("want an AlreadyExists error, got %v", err)
	}
	if !strings.Contains(err.Error(), base) {
		t.Errorf("error %q does not name the base %q", err, base)
	}
}
