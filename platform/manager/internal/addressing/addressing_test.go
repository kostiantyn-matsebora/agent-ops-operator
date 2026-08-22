package addressing

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in       string
		ok       bool
		pipeline string
		rest     string
	}{
		{"/ha-engineer check the vacuum", true, "ha-engineer", "check the vacuum"},
		{"/pipelines", true, "pipelines", ""},
		{"/ha-watch@ExampleOpsBot", true, "ha-watch", ""},
		{"plain message", false, "", ""},
		// A colon is ORDINARY TASK TEXT. The retired second segment used to eat
		// it, so `/devops:node-doctor drain agent-2` addressed `devops` and
		// silently overrode the agent; now the whole remainder is the task.
		{"/devops:node-doctor drain agent-2", true, "devops", ":node-doctor drain agent-2"},
		{"/k8s-observe check:the pods", true, "k8s-observe", "check:the pods"},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if ok != c.ok {
			t.Fatalf("%q ok=%v want %v", c.in, ok, c.ok)
		}
		if !ok {
			continue
		}
		if got.Pipeline != c.pipeline || got.Rest != c.rest {
			t.Fatalf("%q -> %+v, want pipeline=%q rest=%q", c.in, got, c.pipeline, c.rest)
		}
	}
}
