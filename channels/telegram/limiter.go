package main

import (
	"context"
	"sync"
	"time"
)

// Outbound pacing for the Bot API.
//
// Telegram rejects rather than queues: on 2026-08-13 a 44-alert burst produced
// 105 createForumTopic and 74 sendMessage rejections in four minutes. Pacing is
// what turns that into a slower delivery instead of a lost one.
//
// Hand-rolled because this module has no dependencies and keeps none —
// golang.org/x/time/rate would be the first.

// Bot API budgets. Telegram documents these loosely; they are deliberately
// conservative, and `retry_after` remains the authority when we get one anyway.
const (
	// globalRate is the per-bot send ceiling, ~30 messages/second.
	globalRate  = 30
	globalBurst = 30
	// chatRate is the per-chat ceiling for a group or supergroup, 20/minute.
	//
	// THIS is the binding one for a forum: every topic in it shares one chat_id,
	// so cards, replies and topic creations for the whole channel contend for
	// the same budget. That is why one alert burst starved the entire channel.
	chatRate  = 20
	chatBurst = 20
)

// bucket is a token bucket that refills continuously.
//
// It hands out permission, never work: `wait` blocks until a token is free or
// the context ends. Nothing is queued here, because a queue inside the adapter
// would be a second record of what is owed — the manager's claim lease is the
// queue.
type bucket struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	// perSecond is the refill rate.
	perSecond float64
	last      time.Time
	// now and sleep are injectable so the tests can drive time without
	// spending it; nil means the real clock.
	now   func() time.Time
	sleep func(context.Context, time.Duration) bool
}

func newBucket(perSecond float64, capacity int) *bucket {
	return &bucket{tokens: float64(capacity), capacity: float64(capacity), perSecond: perSecond}
}

func (b *bucket) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// take removes one token if available, otherwise reports how long until one is.
func (b *bucket) take() (ok bool, wait time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock()
	if b.last.IsZero() {
		b.last = now
	}
	if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens += elapsed.Seconds() * b.perSecond
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// Round up: sleeping the exact deficit lands a hair early and spins.
	need := (1 - b.tokens) / b.perSecond
	return false, time.Duration(need*float64(time.Second)) + time.Millisecond
}

// wait blocks until a token is available. It returns false when the context
// ended first, which is how a shutdown drops out of pacing rather than holding
// a claim it cannot use.
func (b *bucket) wait(ctx context.Context) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		ok, d := b.take()
		if ok {
			return true
		}
		if !b.doSleep(ctx, d) {
			return false
		}
	}
}

func (b *bucket) doSleep(ctx context.Context, d time.Duration) bool {
	if b.sleep != nil {
		return b.sleep(ctx, d)
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// pacer holds the buckets one adapter sends through: one global, one per chat.
type pacer struct {
	global *bucket

	mu    sync.Mutex
	chats map[string]*bucket
}

func newPacer() *pacer {
	return &pacer{global: newBucket(globalRate, globalBurst), chats: map[string]*bucket{}}
}

func (p *pacer) forChat(chatID string) *bucket {
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.chats[chatID]
	if b == nil {
		b = newBucket(chatRate/60.0, chatBurst)
		p.chats[chatID] = b
	}
	return b
}

// wait spends one token from the global budget and one from every chat this
// adapter serves.
//
// EVERY chat, because pacing gates the CLAIM and a claim does not say which
// chat it is for — the op is not in hand yet. Charging all of them is the
// conservative reading: an adapter serving several chats paces to the slowest,
// which is right when the alternative is a rejection.
func (p *pacer) wait(ctx context.Context, chatIDs []string) bool {
	if !p.global.wait(ctx) {
		return false
	}
	for _, id := range chatIDs {
		if id == "" {
			continue
		}
		if !p.forChat(id).wait(ctx) {
			return false
		}
	}
	return true
}
