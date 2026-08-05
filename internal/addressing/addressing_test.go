package addressing

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		ok      bool
		profile string
		agent   string
		rest    string
	}{
		{"/ha-engineer check the vacuum", true, "ha-engineer", "", "check the vacuum"},
		{"/devops:node-doctor drain agent-2", true, "devops", "node-doctor", "drain agent-2"},
		{"/agents", true, "agents", "", ""},
		{"/ha-watch@HomeOpsBot", true, "ha-watch", "", ""},
		{"plain message", false, "", "", ""},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if ok != c.ok {
			t.Fatalf("%q ok=%v want %v", c.in, ok, c.ok)
		}
		if !ok {
			continue
		}
		if got.Profile != c.profile || got.Agent != c.agent || got.Rest != c.rest {
			t.Fatalf("%q -> %+v", c.in, got)
		}
	}
}
