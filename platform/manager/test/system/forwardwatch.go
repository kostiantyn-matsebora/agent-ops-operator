package system

import (
	"net/http"
	"time"
)

// watchForward closes the gap a process supervisor cannot see: `kubectl port-forward svc/<name>` pins itself
// to ONE backing pod, and when that pod is replaced (a manager restart lane) the kubectl process stays alive
// while every connection through it resets or hangs. Waiting for the process to exit never relaunches it, and
// every later request fails to the end of the run (measured: 17 lanes of one pack, the stale port 127.0.0.1:34641).
//
// It requests url every interval. A transport error twice in a row (a reset, an EOF, a timeout) calls kill, which
// ends the process so the supervisor relaunches it and re-resolves the Service. Any HTTP answer, a 404 included,
// proves the forward carries traffic and resets the count. It returns when stopped reports true.
func watchForward(url string, interval, timeout time.Duration, stopped func() bool, kill func()) {
	client := &http.Client{Timeout: timeout}
	failures := 0
	for {
		time.Sleep(interval)
		if stopped() {
			return
		}
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			failures = 0
			continue
		}
		if failures++; failures >= 2 {
			failures = 0
			kill()
		}
	}
}
