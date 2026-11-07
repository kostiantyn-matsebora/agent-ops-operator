package chat

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

// newTestStore is the unit tier's state.Store: the kube backend over the
// test's fake client, so a test that seeds Conversations into the fake and
// reads them back through it is exercising the same mapping the manager runs.
func newTestStore(c client.WithWatch) state.Store { return kube.NewStore(c, testNS) }

// recOf reads the record a test seeded under name, failing the test when the
// store does not hold it.
func recOf(t *testing.T, s state.Store, name string) *state.ConversationRecord {
	t.Helper()
	rec, _, err := s.Get(t.Context(), state.Key(name))
	if err != nil {
		t.Fatalf("read record %q: %v", name, err)
	}
	return &rec
}

// testRec is a record with no client behind it, for the tests that only hand
// one to the op queue's id derivation.
func testRec(name string) *state.ConversationRecord {
	return &state.ConversationRecord{Name: name, ID: "uid-" + name, Title: "t: " + name}
}

// crRec is the record view of a Conversation CR a test built in memory, for
// the tests that hand one straight to the op queue or the router without a
// client behind it. It carries what those callers read: identity, title,
// provenance of the correlation id, phase, the bound channels and threads.
func crRec(c *agentopsv1alpha1.Conversation) *state.ConversationRecord {
	if c == nil {
		return nil
	}
	rec := &state.ConversationRecord{
		Name: c.Name, ID: string(c.UID), Title: c.Spec.Title, ConversationID: c.Spec.ConversationID,
		Phase: string(c.Status.Phase), Reopens: c.Status.Reopens,
	}
	if c.Spec.Signal != nil {
		rec.Signal = &state.SignalProvenance{Labels: c.Spec.Signal.Labels}
	}
	for _, ref := range c.Spec.ChannelRefs {
		rec.ChannelRefs = append(rec.ChannelRefs, ref.Name)
	}
	for _, in := range c.Spec.Inputs {
		item := state.InputItem{ID: in.ID, Type: string(in.Type), Payload: in.Payload, ReceivedAt: in.ReceivedAt.Time}
		if in.PayloadRef != nil {
			item.PayloadRef = state.Key(in.PayloadRef.Name)
		}
		if o := in.Origin; o != nil {
			item.Origin = &state.InputOrigin{Kind: string(o.Kind), Name: o.Name, Entry: o.Entry, SignalKind: o.SignalKind, Sender: o.Sender}
		}
		rec.Inputs = append(rec.Inputs, item)
	}
	for _, th := range c.Status.Threads {
		rec.Threads = append(rec.Threads, state.ThreadBinding{Channel: th.Channel, ThreadID: th.ThreadID})
	}
	return rec
}
