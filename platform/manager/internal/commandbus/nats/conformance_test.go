package nats

import (
	"context"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/conformance"
)

// testLease is how long a claim runs before the broker hands it on.
const testLease = 600 * time.Millisecond

// fastInventory is DefaultInventory with durations short enough to wait out in
// real time: a real broker's timers cannot be moved by a fake clock, so the
// suite runs against this one and Advance waits.
func fastInventory() []commandbus.Declaration {
	inv := commandbus.DefaultInventory()
	for i := range inv {
		p := &inv[i].Policy
		p.Backoff = commandbus.Backoff{Min: 40 * time.Millisecond, Max: 80 * time.Millisecond}
		if p.Attempts == 1 {
			p.Backoff = commandbus.Backoff{}
		}
		scale := func(d time.Duration) time.Duration { return min(d/100, 450*time.Millisecond) }
		p.DedupWindow = scale(p.DedupWindow)
		p.ParkCoolOff = scale(p.ParkCoolOff)
		p.TTL = scale(p.TTL)
		p.DeliveryTTL = scale(p.DeliveryTTL)
	}
	return inv
}

type natsBacking struct {
	t     *testing.T
	scope string
}

func (n natsBacking) Bus() commandbus.Bus {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	bus, err := Open(ctx, testConn, n.scope, WithInventory(fastInventory()), WithLeaseDuration(testLease),
		WithSweepInterval(time.Hour))
	if err != nil {
		n.t.Fatalf("Open: %v", err)
	}
	n.t.Cleanup(bus.Close)
	return bus
}

// Advance waits. The suite asks for hours; every duration of fastInventory is
// under half a second and the lease 600ms, so waiting 1.1s passes them all.
func (natsBacking) Advance(d time.Duration)      { time.Sleep(min(max(d, 0), 1100*time.Millisecond)) }
func (natsBacking) LeaseDuration() time.Duration { return testLease }

func TestConformance(t *testing.T) {
	conformance.RunInventory(t, func() conformance.Backing {
		return natsBacking{t: t, scope: newScope(t)}
	}, fastInventory())
}
