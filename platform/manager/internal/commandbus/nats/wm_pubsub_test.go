package nats

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type pubsubEnv struct {
	url string
	cfg func(t *testing.T) pubsubConfig // a fresh connection, as another replica has
}

func newPubsubEnv(t *testing.T) pubsubEnv {
	t.Helper()
	srv, err := newTestServer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Shutdown)
	subj := newSubjects("test")
	boot, _ := natsgo.Connect(srv.ClientURL())
	t.Cleanup(boot.Close)
	js, _ := jetstream.New(boot)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := ensurePubSubStream(ctx, js, "cb_test", subj, 0, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "cbconvoys_test", History: 1}); err != nil {
		t.Fatal(err)
	}
	return pubsubEnv{url: srv.ClientURL(), cfg: func(t *testing.T) pubsubConfig {
		nc, err := natsgo.Connect(srv.ClientURL())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(nc.Close)
		js, _ := jetstream.New(nc)
		kv, err := js.KeyValue(context.Background(), "cbconvoys_test")
		if err != nil {
			t.Fatal(err)
		}
		return pubsubConfig{
			js: js, stream: "cb_test", subj: subj, convoys: newConvoyController(kv, subj),
			routing: func(topic string) Mode {
				if topic == "conv" {
					return ModeConvoy
				}
				return ModeRegular
			},
		}
	}}
}

func msg(id, key, body string) *message.Message {
	m := message.NewMessage(id, []byte(body))
	if key != "" {
		m.Metadata.Set(KeyMetadata, key)
	}
	return m
}

// run handles every message of a topic with h, concurrently, answering ack on
// nil and nack on an error.
func run(t *testing.T, sub *Subscriber, topic string, h func(*message.Message) error) {
	t.Helper()
	ch, err := sub.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for m := range ch {
			go func() {
				if h(m) == nil {
					m.Ack()
				} else {
					m.Nack()
				}
			}()
		}
	}()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRegularQueueDeliversEachMessageOnceToCompetingConsumers(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	var mu sync.Mutex
	seen := map[string]int{}
	h := func(m *message.Message) error { mu.Lock(); seen[m.UUID]++; mu.Unlock(); return nil }
	for i := 0; i < 2; i++ { // two replicas
		s := newSubscriber(env.cfg(t), 4)
		t.Cleanup(func() { s.Close() })
		run(t, s, "in.signal", h)
	}
	for i := 0; i < 30; i++ {
		if err := pub.Publish("in.signal", msg(fmt.Sprintf("m%d", i), "", "x")); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "30 deliveries", func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) == 30 })
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("%s delivered %d times", id, n)
		}
	}
}

// The sequential convoy: per key one at a time and in order, across two
// replicas, while different keys run in parallel.
func TestConvoyQueueKeepsOrderPerKeyAndRunsKeysInParallel(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	var mu sync.Mutex
	order := map[string][]string{}
	inflight := map[string]int{}
	var maxInflightPerKey, parallelKeys, maxParallelKeys atomic.Int32
	h := func(m *message.Message) error {
		key := m.Metadata.Get(KeyMetadata)
		mu.Lock()
		inflight[key]++
		if int32(inflight[key]) > maxInflightPerKey.Load() {
			maxInflightPerKey.Store(int32(inflight[key]))
		}
		order[key] = append(order[key], string(m.Payload))
		mu.Unlock()
		if n := parallelKeys.Add(1); n > maxParallelKeys.Load() {
			maxParallelKeys.Store(n)
		}
		time.Sleep(40 * time.Millisecond)
		parallelKeys.Add(-1)
		mu.Lock()
		inflight[key]--
		mu.Unlock()
		return nil
	}
	for i := 0; i < 2; i++ {
		s := newSubscriber(env.cfg(t), 4)
		t.Cleanup(func() { s.Close() })
		run(t, s, "conv", h)
	}
	keys := []string{"a", "b", "c"}
	for n := 0; n < 5; n++ {
		for _, k := range keys {
			if err := pub.Publish("conv", msg(fmt.Sprintf("%s-%d", k, n), k, fmt.Sprint(n))); err != nil {
				t.Fatal(err)
			}
		}
	}
	waitFor(t, "15 deliveries", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order["a"])+len(order["b"])+len(order["c"]) == 15
	})
	mu.Lock()
	defer mu.Unlock()
	for _, k := range keys {
		if fmt.Sprint(order[k]) != "[0 1 2 3 4]" {
			t.Fatalf("key %s out of order: %v", k, order[k])
		}
	}
	if maxInflightPerKey.Load() != 1 {
		t.Fatalf("a key had %d messages in flight at once", maxInflightPerKey.Load())
	}
	if maxParallelKeys.Load() < 2 {
		t.Fatalf("keys did not run in parallel (max %d)", maxParallelKeys.Load())
	}
}

func TestARepeatedMessageIDIsStoredAndDeliveredOnce(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	var n atomic.Int32
	s := newSubscriber(env.cfg(t), 4)
	t.Cleanup(func() { s.Close() })
	run(t, s, "in.signal", func(*message.Message) error { n.Add(1); return nil })
	for i := 0; i < 3; i++ {
		if err := pub.Publish("in.signal", msg("same", "", "x")); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "a delivery", func() bool { return n.Load() >= 1 })
	time.Sleep(300 * time.Millisecond)
	if n.Load() != 1 {
		t.Fatalf("delivered %d times", n.Load())
	}
}

func TestANackedMessageIsDeliveredAgain(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	var n atomic.Int32
	s := newSubscriber(env.cfg(t), 4)
	s.nackDelay = 100 * time.Millisecond
	t.Cleanup(func() { s.Close() })
	run(t, s, "in.signal", func(*message.Message) error {
		if n.Add(1) < 3 {
			return fmt.Errorf("not yet")
		}
		return nil
	})
	pub.Publish("in.signal", msg("retry", "", "x"))
	waitFor(t, "three deliveries", func() bool { return n.Load() == 3 })
}

// A handler running longer than the ack-wait keeps its message: no second
// delivery while the subscriber renews it.
func TestASlowHandlerIsNotRedeliveredWhileItRuns(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	var n atomic.Int32
	s := newSubscriber(env.cfg(t), 4)
	s.ackWait = 600 * time.Millisecond
	t.Cleanup(func() { s.Close() })
	run(t, s, "conv", func(*message.Message) error { n.Add(1); time.Sleep(2 * time.Second); return nil })
	pub.Publish("conv", msg("slow", "k", "x"))
	time.Sleep(2500 * time.Millisecond)
	if n.Load() != 1 {
		t.Fatalf("a slow handler's message was delivered %d times", n.Load())
	}
}

// A replica that stops without answering hands its message to another after
// the ack-wait, and the next message of the key still waits behind it.
func TestAConvoyMessageHeldByAStoppedReplicaMovesToAnother(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	dead := newSubscriber(env.cfg(t), 4)
	dead.ackWait = 600 * time.Millisecond
	got := make(chan struct{}, 1)
	ch, err := dead.Subscribe(context.Background(), "conv")
	if err != nil {
		t.Fatal(err)
	}
	pub.Publish("conv", msg("first", "k", "1"))
	pub.Publish("conv", msg("second", "k", "2"))
	go func() { <-ch; got <- struct{}{} }() // takes the first and never answers
	<-got
	dead.Close()

	var mu sync.Mutex
	var order []string
	live := newSubscriber(env.cfg(t), 4)
	live.ackWait = 600 * time.Millisecond
	t.Cleanup(func() { live.Close() })
	run(t, live, "conv", func(m *message.Message) error { mu.Lock(); order = append(order, m.UUID); mu.Unlock(); return nil })
	waitFor(t, "both messages", func() bool { mu.Lock(); defer mu.Unlock(); return len(order) == 2 })
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(order) != "[first second]" {
		t.Fatalf("order after failover: %v", order)
	}
}

// An attach that fails, as one does during a leader change, is tried again: the
// convoy queue is not left unconsumed because the registry says nothing more
// about it.
func TestAConvoyQueueThatFailsToAttachIsAttachedAgain(t *testing.T) {
	env := newPubsubEnv(t)
	pub := newPublisher(env.cfg(t))
	var fails, got atomic.Int32
	s := newSubscriber(env.cfg(t), 4)
	s.attachHook = func(Convoy) error {
		if fails.Add(1) <= 2 {
			return fmt.Errorf("the consumer's leader is changing")
		}
		return nil
	}
	t.Cleanup(func() { s.Close() })
	run(t, s, "conv", func(*message.Message) error { got.Add(1); return nil })
	if err := pub.Publish("conv", msg("m", "k", "x")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the message after the failed attaches", func() bool { return got.Load() == 1 })
	if fails.Load() < 3 {
		t.Fatalf("the attach was tried %d times, want a retry after each failure", fails.Load())
	}
}
