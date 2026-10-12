package namewords

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// MaxAllocateAttempts bounds the retry on a genuine create conflict between
// several concurrent creators racing for the same name-base — measured live
// against ten concurrent console starts sharing one base, which exhausted
// the original bound of 5: every loser's List saw the same stale snapshot as
// every other loser, so they recomputed the identical "next free" slot and
// collided again on the very next attempt. 20 carries margin above that, and
// retryBackoff below is what actually breaks the lockstep.
const MaxAllocateAttempts = 20

// AllocateName sets obj's Name and its agentops.dev/name-base label to the
// next free name for "<kind>-<words>" and creates it: one List of every
// conversation sharing that exact label value, then one Create, never
// sequential name-guessing and never a random or hash-derived suffix. A
// create conflict from a genuine race between two concurrent creators
// retries the same list-then-create step, bounded to MaxAllocateAttempts;
// exhausting it returns the last conflict error, naming the base name.
func AllocateName(ctx context.Context, c client.Client, namespace, kind, words string, obj *agentopsv1alpha1.Conversation) (string, error) {
	base := kind + "-" + words
	var lastErr error
	for attempt := 0; attempt < MaxAllocateAttempts; attempt++ {
		if attempt > 0 {
			// Desynchronizes this attempt's List from every other loser's:
			// with no delay, every concurrent creator that lost the last
			// Create recomputes the SAME "next free" slot from the SAME
			// stale list and collides again, attempt after attempt.
			if err := sleepCtx(ctx, retryBackoff(attempt)); err != nil {
				return "", err
			}
		}
		var list agentopsv1alpha1.ConversationList
		if err := c.List(ctx, &list, client.InNamespace(namespace), client.MatchingLabels{agentopsv1alpha1.LabelNameBase: base}); err != nil {
			return "", fmt.Errorf("name-base %s: list: %w", base, err)
		}
		name := nameForSuffix(base, nextSuffix(base, list.Items))

		obj.Name = name
		obj.GenerateName = ""
		if obj.Labels == nil {
			obj.Labels = map[string]string{}
		}
		obj.Labels[agentopsv1alpha1.LabelNameBase] = base

		err := c.Create(ctx, obj)
		if err == nil {
			return name, nil
		}
		if !apierrors.IsAlreadyExists(err) {
			return "", err
		}
		lastErr = err
		// A genuine race: another creator took this exact name between our
		// List and our Create. Relist and try the next free name — the
		// object was never persisted, so it is safe to retry as-is.
		obj.ResourceVersion = ""
	}
	return "", fmt.Errorf("name-base %s: exhausted %d attempts: %w", base, MaxAllocateAttempts, lastErr)
}

// nextSuffix reads the highest numeric suffix already in use among
// conversations sharing base, and returns the next free one.
func nextSuffix(base string, items []agentopsv1alpha1.Conversation) int {
	highest := 0
	for _, it := range items {
		if it.Name == base {
			if highest < 1 {
				highest = 1
			}
			continue
		}
		suffix := strings.TrimPrefix(it.Name, base+"-")
		if suffix == it.Name {
			continue
		}
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 1 {
			continue
		}
		if n > highest {
			highest = n
		}
	}
	return highest + 1
}

// nameForSuffix renders the bare base name for suffix 1, or base-N
// otherwise — suffix 1 means "no prior conversation holds this base at
// all", so the first one gets the readable, unsuffixed name.
func nameForSuffix(base string, suffix int) string {
	if suffix <= 1 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, suffix)
}

// retryBackoff grows with the attempt and carries full jitter (0 to the
// cap, not a narrow band around it), so a wide field of concurrent losers
// spreads across the whole window instead of clustering near one value and
// re-colliding in smaller lockstep groups. Capped at 200ms: ten concurrent
// creators exhausting every attempt would otherwise add seconds to a path
// nothing else on the signal-to-answer route waits this long for.
func retryBackoff(attempt int) time.Duration {
	ceiling := 10 * attempt
	if ceiling > 200 {
		ceiling = 200
	}
	return time.Duration(rand.Intn(ceiling+1)) * time.Millisecond
}

// sleepCtx waits for d or ctx's end, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
