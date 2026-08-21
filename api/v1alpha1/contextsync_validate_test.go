package v1alpha1

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContextSyncValidate(t *testing.T) {
	dur := func(s string) *metav1.Duration {
		d, err := time.ParseDuration(s)
		if err != nil {
			t.Fatal(err)
		}
		return &metav1.Duration{Duration: d}
	}
	cases := []struct {
		name    string
		in      *ContextSync
		wantErr string
	}{
		{"absent is valid — it means today's behaviour", nil, ""},
		{
			"a normal declaration",
			&ContextSync{Paths: []string{".claude/projects/-data-workspace/**"},
				Exclude: []string{"**/*.lock"}, Interval: dur("2m"), Retain: 3},
			"",
		},
		{
			"work-boundary only is a legitimate setting",
			&ContextSync{Paths: []string{".claude/**"}, Interval: dur("0s")},
			"",
		},
		{
			"no paths persists nothing while looking configured",
			&ContextSync{Paths: nil},
			"at least one path",
		},
		{
			"an absolute path is not the agent's own context",
			&ContextSync{Paths: []string{"/etc/passwd"}},
			"must be relative",
		},
		{
			"escaping home would copy what the agent is denied",
			&ContextSync{Paths: []string{"../../secrets"}},
			"must not escape",
		},
		{
			"escaping mid-path is caught too",
			&ContextSync{Paths: []string{".claude/../../secrets"}},
			"must not escape",
		},
		{
			"excludes are held to the same rule",
			&ContextSync{Paths: []string{".claude/**"}, Exclude: []string{"/var/log"}},
			"must be relative",
		},
		{
			"an empty path is a typo, not a wildcard",
			&ContextSync{Paths: []string{"  "}},
			"empty path",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q, got none", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestSyncIntervalOffWhenAbsent(t *testing.T) {
	var nilSync *ContextSync
	if _, on := nilSync.SyncInterval(); on {
		t.Fatal("a nil ContextSync must report periodic checkpointing OFF")
	}
	c := &ContextSync{Paths: []string{"a/**"}}
	if _, on := c.SyncInterval(); on {
		t.Fatal("no interval means no timer")
	}
	c.Interval = &metav1.Duration{Duration: 0}
	if _, on := c.SyncInterval(); on {
		t.Fatal("zero means work-boundary checkpoints only, not a busy loop")
	}
	c.Interval = &metav1.Duration{Duration: 2 * time.Minute}
	if d, on := c.SyncInterval(); !on || d != 2*time.Minute {
		t.Fatalf("SyncInterval = %v,%v; want 2m,true", d, on)
	}
}
