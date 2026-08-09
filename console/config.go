package main

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the console's whole environment contract, read once at startup so
// a missing grant is named at boot rather than discovered as an empty page.
//
// The console holds TWO adapter identities in one pod. ADAPTER_TOKEN carries
// the channel identity (carry conversations, reply in a thread);
// SIGNAL_ADAPTER_TOKEN carries the signal identity (originate). The second pair
// is injected by the ChannelAdapter reconciler when a SignalAdapter declares
// `servedBy` pointing at this workload — see docs/concepts.md. Both are
// optional in the sense that the console still runs without the signal half; it
// simply cannot start conversations, and says so rather than offering a button
// that fails.
type Config struct {
	ManagerURL   string
	AdapterName  string
	AdapterToken string

	// SignalAdapterToken / SignalSourceName are the origination identity. Empty
	// means "this console cannot originate" — a state the UI reports with its
	// reason instead of hiding.
	SignalAdapterToken string
	SignalSourceName   string

	ListenAddr string
	Namespace  string

	// UIToken is the fallback browser token for channels declaring no
	// credentials. An UNCONFIGURED token authorizes nobody and is
	// indistinguishable from a wrong one — see api auth.
	UIToken string

	// WriteEnabled gates BOTH write paths (origination and replying). Default
	// true: a default-enabled component whose headline capabilities are
	// default-disabled is a confusing way to ship nothing.
	WriteEnabled bool

	// MetricsURL optionally points at a Prometheus/VictoriaMetrics query API for
	// windows longer than the manager's ring buffer. Unset is fully functional —
	// long windows are reported as unavailable rather than rendered empty.
	MetricsURL string
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envBool reads a boolean env var, defaulting when unset or unparseable.
func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// LoadConfig reads the environment and reports what is missing rather than
// starting half-configured.
func LoadConfig() (*Config, error) {
	c := &Config{
		ManagerURL:         os.Getenv("MANAGER_URL"),
		AdapterName:        envOr("ADAPTER_NAME", "console"),
		AdapterToken:       os.Getenv("ADAPTER_TOKEN"),
		SignalAdapterToken: os.Getenv("SIGNAL_ADAPTER_TOKEN"),
		SignalSourceName:   envOr("SIGNAL_SOURCE_NAME", "console"),
		ListenAddr:         envOr("LISTEN_ADDR", ":8080"),
		Namespace:          os.Getenv("POD_NAMESPACE"),
		UIToken:            os.Getenv("UI_TOKEN"),
		WriteEnabled:       envBool("WRITE_ENABLED", true),
		MetricsURL:         os.Getenv("METRICS_URL"),
	}
	if c.ManagerURL == "" {
		return nil, fmt.Errorf("missing required env MANAGER_URL")
	}
	if c.AdapterToken == "" {
		return nil, fmt.Errorf("missing required env ADAPTER_TOKEN (injected by the ChannelAdapter reconciler)")
	}
	if c.Namespace == "" {
		return nil, fmt.Errorf("missing required env POD_NAMESPACE (injected by ChannelAdapter spec.kubernetesAccess)")
	}
	return c, nil
}

// CanOriginate reports whether the console holds a signal identity. Kept
// separate from WriteEnabled: "not wired to originate" and "writes turned off"
// are different answers and the UI shows different things for them.
func (c *Config) CanOriginate() bool {
	return c.SignalAdapterToken != "" && c.SignalSourceName != ""
}
