package nats

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// newTestCluster starts three JetStream nodes in this process, routed to one
// another, and returns them with their client URLs.
func newTestCluster(t *testing.T) ([]*natsserver.Server, string) {
	t.Helper()
	const n = 3
	cluster := make([]int, n)
	client := make([]int, n)
	for i := range cluster {
		cluster[i], client[i] = freePort(t), freePort(t)
	}
	var routes []string
	for _, p := range cluster {
		routes = append(routes, fmt.Sprintf("nats://127.0.0.1:%d", p))
	}
	var servers []*natsserver.Server
	var urls []string
	for i := 0; i < n; i++ {
		dir, err := os.MkdirTemp("", "cb-cluster-*")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		opts := &natsserver.Options{
			ServerName: fmt.Sprintf("n%d", i), Host: "127.0.0.1", Port: client[i], JetStream: true, StoreDir: dir,
			NoLog: true, NoSigs: true, DisableJetStreamBanner: true,
			Cluster: natsserver.ClusterOpts{Name: "cbtest", Host: "127.0.0.1", Port: cluster[i]},
		}
		opts.Routes = natsserver.RoutesFromStr(strings.Join(routes, ","))
		srv, err := natsserver.NewServer(opts)
		if err != nil {
			t.Fatal(err)
		}
		srv.Start()
		t.Cleanup(srv.Shutdown)
		servers = append(servers, srv)
		urls = append(urls, srv.ClientURL())
	}
	for _, s := range servers {
		if !s.ReadyForConnections(20 * time.Second) {
			t.Fatal("a cluster node never became ready")
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if servers[0].JetStreamIsCurrent() && servers[1].JetStreamIsCurrent() && servers[2].JetStreamIsCurrent() {
			return servers, strings.Join(urls, ",")
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the cluster never formed a JetStream meta group")
	return nil, ""
}

// The bus works with two of three nodes: with the node that leads its stream
// stopped, work already accepted is still delivered, in order, and new work is
// still accepted.
func TestTheBusKeepsWorkingWhenTheNodeLeadingItsStreamStops(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a three-node cluster")
	}
	servers, url := newTestCluster(t)
	conn, err := Connect(Config{URL: url, Replicas: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bus, err := Open(ctx, conn, newScope(t), WithInventory(fastInventory()), WithLeaseDuration(2*time.Second), WithSweepInterval(time.Hour))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(bus.Close)

	submit := func(id, key string) {
		t.Helper()
		var last error
		for attempt := 0; attempt < 40 && ctx.Err() == nil; attempt++ {
			cctx, c := context.WithTimeout(ctx, 20*time.Second)
			_, last = bus.Submit(cctx, commandbus.SubmitRequest{Queue: "conv", CommandID: commandbus.CommandID(id), Key: commandbus.Key(key), Payload: []byte(id)})
			c()
			if last == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("Submit %s: %v", id, last)
	}
	for n := 0; n < 3; n++ {
		submit(fmt.Sprintf("a%d", n), "ka")
		submit(fmt.Sprintf("b%d", n), "kb")
	}

	// Stop the node that leads the stream.
	leader := ""
	if st, err := conn.js.Stream(ctx, bus.stream); err == nil {
		if info, err := st.Info(ctx); err == nil && info.Cluster != nil {
			leader = info.Cluster.Leader
		}
	}
	for _, srv := range servers {
		if srv.Name() == leader {
			srv.Shutdown()
		}
	}
	t.Logf("stopped the stream leader %q", leader)

	for n := 3; n < 5; n++ {
		submit(fmt.Sprintf("a%d", n), "ka")
		submit(fmt.Sprintf("b%d", n), "kb")
	}

	handled := map[commandbus.Key][]string{}
	total := 0
	for total < 10 && ctx.Err() == nil {
		cctx, c := context.WithTimeout(ctx, 15*time.Second)
		task, err := bus.Claim(cctx, "conv", "w")
		c()
		if err != nil {
			continue
		}
		handled[task.Key] = append(handled[task.Key], string(task.Payload))
		for ctx.Err() == nil {
			if err := bus.Settle(ctx, task.UID, task.Lease.Token); err == nil {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		total++
	}
	if total < 10 {
		dumpBus(t, bus, conn)
	}
	for _, k := range []commandbus.Key{"ka", "kb"} {
		var want []string
		prefix := string(k[1])
		for n := 0; n < 5; n++ {
			want = append(want, fmt.Sprintf("%s%d", prefix, n))
		}
		if fmt.Sprint(handled[k]) != fmt.Sprint(want) {
			t.Fatalf("%s handled %v, want %v", k, handled[k], want)
		}
	}
}

// dumpBus logs what the broker and the records say, for a failed run.
func dumpBus(t *testing.T, bus *Bus, conn *Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := conn.js.Stream(ctx, bus.stream)
	if err != nil {
		t.Logf("stream: %v", err)
		return
	}
	if info, err := st.Info(ctx); err == nil {
		t.Logf("stream msgs=%d first=%d last=%d cluster=%+v", info.State.Msgs, info.State.FirstSeq, info.State.LastSeq, info.Cluster)
	} else {
		t.Logf("stream info: %v", err)
	}
	cvs, err := bus.convoys.List(ctx)
	t.Logf("registry: %d entries, err=%v", len(cvs), err)
	for _, cv := range cvs {
		cons, err := st.Consumer(ctx, cv.Consumer)
		if err != nil {
			t.Logf("  %s/%s consumer: %v", cv.Queue, cv.Key, err)
			continue
		}
		info, err := cons.Info(ctx)
		if err != nil {
			t.Logf("  %s/%s info: %v", cv.Queue, cv.Key, err)
			continue
		}
		t.Logf("  %s/%s delivered=%+v ackfloor=%+v pending=%d ackpending=%d redelivered=%d waiting=%d cluster=%+v",
			cv.Queue, cv.Key, info.Delivered, info.AckFloor, info.NumPending, info.NumAckPending, info.NumRedelivered, info.NumWaiting, info.Cluster)
	}
	for _, d := range bus.mirror.all() {
		t.Logf("  task %s key=%s state=%s attempt=%d seq=%d", d.doc.Task.CommandID, d.doc.Task.Key, d.doc.Task.State, d.doc.Task.Attempt, d.doc.Seq)
	}
}
