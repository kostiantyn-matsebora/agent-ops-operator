package conformance

// Bench, BenchAppended and BenchBlobs are the tier-1 numbers design.md's
// D-1 names for comparing backends: bytes written per record (a JSON-encoded
// size proxy — the contract gives no byte-count callback, so this is a lower
// bound true of every backend, never the authoritative figure for what
// crosses the wire), allocations per write, and records resident after a
// listing. BenchStores runs all three for every field of state.Stores, so
// `go test -bench . -benchmem ./internal/state/...` prints each one without
// a backend writing its own loop.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

const benchResidentScale = 50

// Bench reports Collection[T]'s own numbers against a backend open
// constructs once per invocation (the harness itself calls a Benchmark
// function several times with a growing b.N to calibrate; only the last
// invocation's reported numbers are the ones printed).
func Bench[T any](b *testing.B, open func() state.Collection[T], decl state.Decl[T], fx Fixture[T]) {
	b.Helper()
	ctx := context.Background()
	s := open()

	key := fx.Key("bench")
	if _, err := s.Create(ctx, key, fx.New()); err != nil {
		b.Fatalf("Create: %v", err)
	}
	recJSON, err := json.Marshal(fx.New())
	if err != nil {
		b.Fatalf("marshal: %v", err)
	}

	allocs := testing.AllocsPerRun(1, func() {
		if _, err := state.Mutate(ctx, s, key, func(rec *T) error { *rec = fx.Edit(*rec, 1); return nil }); err != nil {
			b.Fatalf("Mutate: %v", err)
		}
	})

	var resident int
	if len(decl.Indexes) > 0 {
		idx := decl.Indexes[0]
		probe := fx.New()
		if v, listed := idx.Value(&probe); listed {
			for i := 0; i < benchResidentScale; i++ {
				if _, err := s.Create(ctx, fx.Key(fmt.Sprintf("resident-%d", i)), fx.New()); err != nil {
					b.Fatalf("Create resident %d: %v", i, err)
				}
			}
			if got, err := s.List(ctx, idx, v); err == nil {
				resident = len(got)
			}
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.Mutate(ctx, s, key, func(rec *T) error { *rec = fx.Edit(*rec, i); return nil }); err != nil {
			b.Fatalf("Mutate: %v", err)
		}
	}

	b.ReportMetric(float64(len(recJSON)), "bytes/record(json-proxy)")
	b.ReportMetric(allocs, "allocs/write")
	b.ReportMetric(float64(resident), "resident_records")
}

// BenchAppended reports Appended[P, T]'s own numbers.
func BenchAppended[P, T any](b *testing.B, open func() AppendedOpen[P, T], fx AppendedFixture[P, T]) {
	b.Helper()
	ctx := context.Background()
	o := open()

	key := state.Key("bench-parent")
	if _, err := o.Parent.Create(ctx, key, fx.NewParent()); err != nil {
		b.Fatalf("Create parent: %v", err)
	}
	item := fx.NewItem("bench-item")
	itemJSON, err := json.Marshal(item)
	if err != nil {
		b.Fatalf("marshal: %v", err)
	}

	allocs := testing.AllocsPerRun(1, func() {
		if err := o.Items.Append(ctx, key, item); err != nil {
			b.Fatalf("Append: %v", err)
		}
	})

	var resident int
	if fx.AllIndex.Name() != "" {
		for i := 0; i < benchResidentScale; i++ {
			if _, err := o.Parent.Create(ctx, state.Key(fmt.Sprintf("bench-resident-%d", i)), fx.NewParent()); err != nil {
				b.Fatalf("Create resident %d: %v", i, err)
			}
		}
		if got, err := o.Items.ListWithItems(ctx, fx.AllIndex, fx.AllValue); err == nil {
			resident = len(got)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := o.Items.Append(ctx, key, item); err != nil {
			b.Fatalf("Append: %v", err)
		}
	}

	b.ReportMetric(float64(len(itemJSON)), "bytes/run(json-proxy)")
	b.ReportMetric(allocs, "allocs/write")
	b.ReportMetric(float64(resident), "resident_records")
}

// BenchBlobs reports Blobs' own numbers.
func BenchBlobs(b *testing.B, open func() state.Blobs) {
	b.Helper()
	ctx := context.Background()
	s := open()
	key := state.Key("bench-blob")
	blob := fixtureBlob()
	blobJSON, err := json.Marshal(blob)
	if err != nil {
		b.Fatalf("marshal: %v", err)
	}
	if err := s.Put(ctx, key, blob); err != nil {
		b.Fatalf("Put: %v", err)
	}

	allocs := testing.AllocsPerRun(1, func() {
		if err := s.Put(ctx, key, blob); err != nil {
			b.Fatalf("Put: %v", err)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.Put(ctx, key, blob); err != nil {
			b.Fatalf("Put: %v", err)
		}
	}

	b.ReportMetric(float64(len(blobJSON)), "bytes/record(json-proxy)")
	b.ReportMetric(allocs, "allocs/write")
}

// BenchStores runs Bench/BenchAppended/BenchBlobs for every field of
// state.Stores, so one `go test -bench . -benchmem` prints every number
// tasks.md 4.2 asks for.
func BenchStores(b *testing.B, open func() state.Stores) {
	b.Helper()
	b.Run("Conversations", func(b *testing.B) {
		Bench(b, func() state.Collection[state.ConversationRecord] { return open().Conversations }, state.StoreDeclarations.Conversations, conversationFixture)
	})
	b.Run("Runs", func(b *testing.B) {
		BenchAppended(b, func() AppendedOpen[state.ConversationRecord, state.Run] {
			s := open()
			return AppendedOpen[state.ConversationRecord, state.Run]{Parent: s.Conversations, Items: s.Runs}
		}, runFixture)
	})
	b.Run("Signals", func(b *testing.B) {
		Bench(b, func() state.Collection[state.SignalRecord] { return open().Signals }, state.StoreDeclarations.Signals, signalFixture)
	})
	b.Run("Cooldowns", func(b *testing.B) {
		Bench(b, func() state.Collection[state.CooldownWindow] { return open().Cooldowns }, state.StoreDeclarations.Cooldowns, cooldownFixture)
	})
	b.Run("Receipts", func(b *testing.B) {
		Bench(b, func() state.Collection[state.Received] { return open().Receipts }, state.StoreDeclarations.Receipts, receiptFixture)
	})
	b.Run("AdapterState", func(b *testing.B) {
		Bench(b, func() state.Collection[string] { return open().AdapterState }, state.StoreDeclarations.AdapterState, adapterStateFixture)
	})
	b.Run("Blobs", func(b *testing.B) {
		BenchBlobs(b, func() state.Blobs { return open().Blobs })
	})
}
