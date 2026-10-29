package ingest

import (
	"sync"
	"testing"
	"time"
)

func TestCooldownBookBuildsOncePerSourceAndReportsSorted(t *testing.T) {
	b := NewCooldownBook()
	built := 0
	build := func() *Cooldown { built++; return NewCooldown(time.Hour) }
	if b.For("b", build) != b.For("b", build) || built != 1 {
		t.Fatal("one Cooldown per source")
	}
	b.For("a", build).Fresh([]string{"x"})
	b.For("a", build).Fresh([]string{"x"}) // suppressed, not counted twice
	stats := b.Stats()
	if len(stats) != 2 || stats[0].Source != "a" || stats[0].Suppressed != 1 || stats[0].Window != time.Hour {
		t.Fatalf("stats: %+v", stats)
	}
	var nilBook *CooldownBook
	if nilBook.Stats() != nil {
		t.Fatal("a nil book reports nothing")
	}
}

func TestCooldownBookLockSerializesOneSource(t *testing.T) {
	b := NewCooldownBook()
	var wg sync.WaitGroup
	inside, max := 0, 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := b.Lock("s")
			mu.Lock()
			inside++
			if inside > max {
				max = inside
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			inside--
			mu.Unlock()
			unlock()
		}()
	}
	wg.Wait()
	if max != 1 {
		t.Fatalf("%d holders at once, want 1", max)
	}
	// a different source is independent
	u1 := b.Lock("a")
	done := make(chan struct{})
	go func() { b.Lock("b")(); close(done) }()
	<-done
	u1()
}

func TestCooldownReleaseForgetsAnAdmission(t *testing.T) {
	c := NewCooldown(time.Hour)
	if len(c.Fresh([]string{"x"})) != 1 || len(c.Fresh([]string{"x"})) != 0 {
		t.Fatal("setup")
	}
	c.Release("x")
	if len(c.Fresh([]string{"x"})) != 1 {
		t.Fatal("a released id is fresh again")
	}
}
