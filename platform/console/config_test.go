package main

import (
	"os"
	"testing"
)

// config.go had NO test file at all: envOr, envBool, LoadConfig and
// CanOriginate were exercised nowhere, only ever constructed by hand with a
// literal Config{} elsewhere in the suite. These tests close that gap against
// the real os.Getenv-reading implementation, not a stand-in.

// clearConsoleEnv removes every var LoadConfig reads, so each subtest starts
// from a clean environment regardless of what the host or a parallel test set.
func clearConsoleEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"MANAGER_URL", "ADAPTER_NAME", "ADAPTER_TOKEN", "SIGNAL_ADAPTER_TOKEN",
		"SIGNAL_SOURCE_NAME", "LISTEN_ADDR", "POD_NAMESPACE", "UI_TOKEN",
		"AUTH_ENABLED", "EXTERNAL_AUTHENTICATOR", "WRITE_ENABLED", "METRICS_URL",
	} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestEnvOrFallsBackWhenUnset(t *testing.T) {
	clearConsoleEnv(t)
	if got := envOr("ADAPTER_NAME", "console"); got != "console" {
		t.Fatalf("want fallback, got %q", got)
	}
	os.Setenv("ADAPTER_NAME", "custom")
	if got := envOr("ADAPTER_NAME", "console"); got != "custom" {
		t.Fatalf("want set value, got %q", got)
	}
}

func TestEnvBoolParsesOrFallsBack(t *testing.T) {
	clearConsoleEnv(t)
	if !envBool("WRITE_ENABLED", true) {
		t.Fatal("unset must use the fallback")
	}
	os.Setenv("WRITE_ENABLED", "false")
	if envBool("WRITE_ENABLED", true) {
		t.Fatal("explicit false must win over the fallback")
	}
	os.Setenv("WRITE_ENABLED", "not-a-bool")
	if !envBool("WRITE_ENABLED", true) {
		t.Fatal("an unparseable value must fall back rather than error out silently")
	}
}

// LoadConfig must name the MISSING env var rather than starting half
// configured — three required fields, three distinct errors.
func TestLoadConfigRequiresManagerURL(t *testing.T) {
	clearConsoleEnv(t)
	if _, err := LoadConfig(); err == nil {
		t.Fatal("want an error with everything unset")
	}
}

func TestLoadConfigRequiresAdapterToken(t *testing.T) {
	clearConsoleEnv(t)
	os.Setenv("MANAGER_URL", "http://manager:8080")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("want an error with no ADAPTER_TOKEN")
	}
}

func TestLoadConfigRequiresNamespace(t *testing.T) {
	clearConsoleEnv(t)
	os.Setenv("MANAGER_URL", "http://manager:8080")
	os.Setenv("ADAPTER_TOKEN", "tok")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("want an error with no POD_NAMESPACE")
	}
}

// The success path, with every optional value left to its default — this is
// what a minimal chart-rendered install actually sets.
func TestLoadConfigSucceedsWithDefaults(t *testing.T) {
	clearConsoleEnv(t)
	os.Setenv("MANAGER_URL", "http://manager:8080")
	os.Setenv("ADAPTER_TOKEN", "tok")
	os.Setenv("POD_NAMESPACE", "agent-ops")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdapterName != "console" || cfg.SignalSourceName != "console" || cfg.ListenAddr != ":8080" {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if !cfg.AuthEnabled || !cfg.WriteEnabled {
		t.Fatalf("auth and writes must default on: %+v", cfg)
	}
	if cfg.CanOriginate() {
		t.Fatal("no signal identity was configured, so this console must not claim it can originate")
	}
}

// CanOriginate needs BOTH the token and the source name — either alone is a
// half-configured signal identity.
func TestCanOriginateNeedsBothFields(t *testing.T) {
	c := &Config{}
	if c.CanOriginate() {
		t.Fatal("neither field set")
	}
	c.SignalAdapterToken = "tok"
	if c.CanOriginate() {
		t.Fatal("token alone must not be enough")
	}
	c.SignalSourceName = "console"
	if !c.CanOriginate() {
		t.Fatal("both fields set must originate")
	}
}

// Overriding every env var, including the boolean ones set explicitly false,
// is the other half of LoadConfig nothing above exercises.
func TestLoadConfigHonorsEveryOverride(t *testing.T) {
	clearConsoleEnv(t)
	os.Setenv("MANAGER_URL", "http://manager:8080")
	os.Setenv("ADAPTER_TOKEN", "tok")
	os.Setenv("POD_NAMESPACE", "agent-ops")
	os.Setenv("ADAPTER_NAME", "my-console")
	os.Setenv("SIGNAL_ADAPTER_TOKEN", "sig-tok")
	os.Setenv("SIGNAL_SOURCE_NAME", "my-source")
	os.Setenv("LISTEN_ADDR", ":9090")
	os.Setenv("UI_TOKEN", "ui-tok")
	os.Setenv("AUTH_ENABLED", "false")
	os.Setenv("EXTERNAL_AUTHENTICATOR", "oauth2-proxy")
	os.Setenv("WRITE_ENABLED", "false")
	os.Setenv("METRICS_URL", "http://metrics:9090")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AdapterName != "my-console" || cfg.SignalAdapterToken != "sig-tok" ||
		cfg.SignalSourceName != "my-source" || cfg.ListenAddr != ":9090" ||
		cfg.UIToken != "ui-tok" || cfg.AuthEnabled || cfg.ExternalAuthenticator != "oauth2-proxy" ||
		cfg.WriteEnabled || cfg.MetricsURL != "http://metrics:9090" {
		t.Fatalf("overrides not honored: %+v", cfg)
	}
	if !cfg.CanOriginate() {
		t.Fatal("both signal fields were set, so this console can originate")
	}
}
