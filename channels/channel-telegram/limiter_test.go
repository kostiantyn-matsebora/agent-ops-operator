package main

import (
	"context"
	"testing"
	"time"
)

// A bucket driven by a fake clock: the tests assert rate and burst without
// spending the wall-clock time they describe.
func fakeBucket(perSecond float64, capacity int) (*bucket, *time.Time, *[]time.Duration) {
	now := time.Unix(0, 0)
	var slept []time.Duration
	b := newBucket(perSecond, capacity)
	b.now = func() time.Time { return now }
	b.sleep = func(ctx context.Context, d time.Duration) bool {
		if ctx.Err() != nil {
			return false
		}
		slept = append(slept, d)
		now = now.Add(d) // the clock only moves because we waited
		return true
	}
	return b, &now, &slept
}

func TestBucketSpendsItsBurstThenPaces(t *testing.T) {
	b, _, slept := fakeBucket(20/60.0, 20) // the per-chat budget: 20/minute

	for i := 0; i < 20; i++ {
		if ok, _ := b.take(); !ok {
			t.Fatalf("burst token %d refused: a full bucket must not pace", i+1)
		}
	}
	if ok, wait := b.take(); ok || wait <= 0 {
		t.Fatalf("the 21st call must wait, got ok=%v wait=%v", ok, wait)
	}
	// ...and waiting for it costs about one refill interval (3s at 20/min)
	if !b.wait(context.Background()) {
		t.Fatal("wait must succeed once the clock advances")
	}
	if len(*slept) != 1 {
		t.Fatalf("want one sleep, got %v", *slept)
	}
	if d := (*slept)[0]; d < 2900*time.Millisecond || d > 3200*time.Millisecond {
		t.Fatalf("a 20/min bucket refills every ~3s, slept %v", d)
	}
}

func TestBucketRefillsOverTime(t *testing.T) {
	b, now, _ := fakeBucket(20/60.0, 20)
	for i := 0; i < 20; i++ {
		b.take()
	}
	*now = now.Add(30 * time.Second) // half a minute at 20/min = 10 tokens
	got := 0
	for {
		ok, _ := b.take()
		if !ok {
			break
		}
		got++
	}
	if got < 9 || got > 11 {
		t.Fatalf("30s at 20/min should refill ~10 tokens, got %d", got)
	}
}

func TestBucketNeverExceedsItsCapacity(t *testing.T) {
	b, now, _ := fakeBucket(20/60.0, 20)
	*now = now.Add(time.Hour) // long idle
	got := 0
	for {
		ok, _ := b.take()
		if !ok {
			break
		}
		got++
	}
	if got != 20 {
		t.Fatalf("an idle hour must not bank more than the burst: got %d", got)
	}
}

// A shutdown drops out of pacing rather than holding a claim it cannot use.
func TestBucketWaitRespectsCancellation(t *testing.T) {
	b := newBucket(20/60.0, 1)
	if ok, _ := b.take(); !ok {
		t.Fatal("the first token must be free")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b.wait(ctx) {
		t.Fatal("wait must report failure on a cancelled context rather than sleeping")
	}
}

// THE POINT OF PACING BEFORE THE CLAIM: an exhausted budget must not take work
// out of the manager, where it is still derivable and survives a restart.
func TestPacerGatesTheClaimNotTheSend(t *testing.T) {
	p := newPacer()
	ctx := context.Background()
	// spend the whole per-chat burst
	for i := 0; i < chatBurst; i++ {
		if !p.wait(ctx, []string{"-100"}) {
			t.Fatalf("burst call %d refused", i+1)
		}
	}
	// the next one must block; a cancelled context proves it did not pass
	done, cancel := context.WithCancel(ctx)
	cancel()
	claimed := false
	if p.wait(done, []string{"-100"}) {
		claimed = true
	}
	if claimed {
		t.Fatal("an exhausted budget must not admit another claim")
	}
}

// Two chats are paced independently, so a burst on one does not starve another.
func TestPacerIsPerChat(t *testing.T) {
	p := newPacer()
	ctx := context.Background()
	for i := 0; i < chatBurst; i++ {
		p.wait(ctx, []string{"-100"})
	}
	if ok, _ := p.forChat("-200").take(); !ok {
		t.Fatal("a second chat must keep its own budget")
	}
}
