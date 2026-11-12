package httpapi

import "testing"

// isChannelOpDonePath is the leader-proxy exemption's own match for POST
// /channel/ops/{id}/done (async-command-bus design D-11) — a prefix/suffix
// check standing in for the mux's own pattern match, since leaderProxyMiddleware
// runs OUTSIDE the mux and sees only the raw path.
func TestIsChannelOpDonePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/channel/ops/abc123/done", true},
		{"/channel/ops/topic:conv-1:id:ch/done", true},
		{"/channel/ops", false},                // the poll itself, matched separately
		{"/channel/ops/done", false},           // no {id} segment at all
		{"/channel/ops//done", false},          // an empty {id} is not a real completion
		{"/channel/opsX/abc/done", false},      // must not match a path merely starting the same
		{"/channel/ops/abc/doneX", false},      // must not match a path merely ending the same
		{"/channel/ops/abc/done/extra", false}, // {id} ends at the LAST "/done", not before
	}
	for _, c := range cases {
		if got := isChannelOpDonePath(c.path); got != c.want {
			t.Errorf("isChannelOpDonePath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
