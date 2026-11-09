//go:build system

package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// nats-state-store-default — the console's recovery is: list, keep the
// revision, open /state/conversations/stream from it. Across a real manager
// rollout that must still work: a revision taken BEFORE the rollout is
// accepted by the new pods (no resync), and a close made afterwards arrives
// on the stream as a put. A resync here is the console relisting for ever.
func TestStateStreamResumesFromAListedRevisionAcrossAManagerRollout(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	fp := "e2e-stream-rollout-" + stamp

	e.PostTask(t, SourceTasks, fp, "echo "+fp)
	conv := e.ConversationFor(t, fp, 2*time.Minute)
	e.WaitRun(t, conv.Name, 1, 3*time.Minute)

	auth := bearerPrefix + e.Values.AdapterToken
	revision := listedRevision(t, e, auth)

	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "rollout", "restart", "deployment/agentops-manager"); err != nil {
		t.Fatalf("rollout restart: %v\n%s", err, out)
	}
	if out, err := e.Cluster.Kubectl(ctx, "-n", Namespace, "rollout", "status", "deployment/agentops-manager", "--timeout=8m"); err != nil {
		t.Fatalf("rollout status: %v\n%s", err, out)
	}

	sctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	resp := openStateStream(t, sctx, e, auth, revision)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	body := []byte(`{"names":["` + conv.Name + `"]}`)
	if code, out := e.do(t, "POST", e.Console.URL()+"/api/conversations/close", body, bearerPrefix+e.Values.UIToken); code/100 != 2 {
		t.Fatalf("close: %d %s", code, out)
	}
	awaitClosePut(t, frames, conv.Name, revision)
}

// listedRevision waits for a /state/conversations listing and returns its revision.
func listedRevision(t *testing.T, e *Env, auth string) string {
	t.Helper()
	var listed struct {
		Revision string `json:"revision"`
	}
	waitFor(t, "a listing carrying a revision", time.Minute, func() (bool, error) {
		code, out := e.do(t, "GET", e.Manager.URL()+"/state/conversations", nil, auth)
		if code != 200 {
			return false, nil
		}
		_ = json.Unmarshal([]byte(out), &listed)
		return listed.Revision != "", nil
	})
	return listed.Revision
}

// openStateStream opens the conversation stream from a revision, retrying until accepted.
func openStateStream(t *testing.T, ctx context.Context, e *Env, auth, revision string) *http.Response {
	t.Helper()
	var resp *http.Response
	waitFor(t, "the stream to open from the pre-rollout revision", 2*time.Minute, func() (bool, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", e.Manager.URL()+"/state/conversations/stream?since="+revision, nil)
		req.Header.Set("Authorization", auth)
		r, err := http.DefaultClient.Do(req)
		if err != nil || r.StatusCode != 200 {
			if r != nil {
				r.Body.Close()
			}
			return false, nil
		}
		resp = r
		return true, nil
	})
	return resp
}

// streamFrames reads SSE frames off the response as "<event> <data>" strings.
func streamFrames(resp *http.Response) <-chan string {
	frames := make(chan string, 256)
	go func() {
		defer close(frames)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		event := ""
		for sc.Scan() {
			switch line := sc.Text(); {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				frames <- event + " " + strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return frames
}

// awaitClosePut requires a put frame showing the conversation Closed, and no resync before it.
func awaitClosePut(t *testing.T, frames <-chan string, name, revision string) {
	t.Helper()
	deadline := time.After(2 * time.Minute)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatal("the stream ended before delivering the close")
			}
			if strings.HasPrefix(f, "resync ") {
				t.Fatalf("the new manager refused the revision a listing returned before the rollout (%q): the console would relist for ever", revision)
			}
			if strings.HasPrefix(f, "put ") && strings.Contains(f, `"`+name+`"`) && strings.Contains(f, "Closed") {
				return
			}
		case <-deadline:
			t.Fatal("the close never arrived on a stream resumed after the rollout")
		}
	}
}
