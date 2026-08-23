package main

import "testing"

// The manager and the chart upgrade at different moments. A bootstrap default
// that vanished for one reconcile would build runtime pods with no context
// volume — the exact silent-emptying this rename is guarded against elsewhere.
func TestContextPVCDualRead(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		want       string
		deprecated bool
	}{
		{
			name: "the current spelling wins where both are set",
			env:  map[string]string{"CONTEXT_PVC": "agentops-context", "HOME_PVC": "agentops-home"},
			want: "agentops-context",
		},
		{
			name:       "a chart from before the rename is honoured",
			env:        map[string]string{"HOME_PVC": "agentops-home"},
			want:       "agentops-home",
			deprecated: true,
		},
		{
			name: "the current spelling alone",
			env:  map[string]string{"CONTEXT_PVC": "agentops-context"},
			want: "agentops-context",
		},
		{
			name: "neither set leaves the volume ephemeral",
			env:  map[string]string{},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"CONTEXT_PVC", "HOME_PVC"} {
				t.Setenv(k, tc.env[k])
			}
			got, deprecated := contextPVC()
			if got != tc.want {
				t.Fatalf("contextPVC() = %q, want %q", got, tc.want)
			}
			if deprecated != tc.deprecated {
				t.Fatalf("deprecated = %v, want %v", deprecated, tc.deprecated)
			}
		})
	}
}
