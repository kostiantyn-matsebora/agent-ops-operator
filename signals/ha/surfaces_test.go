package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---- configuration ----------------------------------------------------------

func parseSurfacesFor(t *testing.T, extra map[string]any) surfacePolicies {
	t.Helper()
	cfg := map[string]any{"endpoint": "http://ha.example.org"}
	for k, v := range extra {
		cfg[k] = v
	}
	raw, _ := json.Marshal(cfg)
	f, err := parseConfig(raw)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return f.surfaces
}

func TestSurfacesDefaults(t *testing.T) {
	sp := parseSurfacesFor(t, nil)
	if !sp.configEntries.enabled || !sp.repairs.enabled || !sp.sensors.enabled || sp.updates.enabled {
		t.Fatalf("defaults: config entries, repairs and sensors on, updates off; got %+v", sp)
	}
	if !sp.configEntries.accept["setup_retry"] || sp.configEntries.accept["not_loaded"] {
		t.Fatalf("config entry states default to the failed ones, got %v", sp.configEntries.accept)
	}
	if !sp.repairs.accept["error"] || sp.repairs.accept["warning"] {
		t.Fatalf("repair severities default to critical and error, got %v", sp.repairs.accept)
	}
	if !sp.sensors.accept["problem"] || !sp.sensors.accept["connectivity"] || sp.sensors.accept["smoke"] {
		t.Fatalf("sensor classes default to problem and connectivity, got %v", sp.sensors.accept)
	}
}

func TestSurfacesSwitchesAndKnobs(t *testing.T) {
	sp := parseSurfacesFor(t, map[string]any{"surfaces": map[string]any{
		"sensors":       map[string]any{"enabled": false},
		"updates":       map[string]any{"enabled": true},
		"repairs":       map[string]any{"severities": []string{"warning"}},
		"configEntries": map[string]any{"states": []string{"setup_retry", "not_loaded"}},
	}})
	if sp.sensors.enabled || !sp.updates.enabled || !sp.repairs.enabled {
		t.Fatalf("switches: %+v", sp)
	}
	if !sp.repairs.accept["warning"] || sp.repairs.accept["error"] {
		t.Fatalf("severities knob replaces the default, got %v", sp.repairs.accept)
	}
	if !sp.configEntries.accept["not_loaded"] || sp.configEntries.accept["setup_error"] {
		t.Fatalf("states knob replaces the default, got %v", sp.configEntries.accept)
	}
}

// A misspelt surface, a knob on the wrong surface, or a value Home Assistant
// does not define is refused naming the mistake — never ignored, because for
// a switch "ignored" means a surface somebody believes is off.
func TestSurfacesRefuseUnknownKeysAndValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		block  map[string]any
		expect string
	}{
		{"unknown surface", map[string]any{"repair": map[string]any{}}, "surfaces"},
		{"unknown knob", map[string]any{"sensors": map[string]any{"threshold": 1}}, "surfaces"},
		{"knob on the wrong surface", map[string]any{"sensors": map[string]any{"states": []string{"loaded"}}}, "not a knob"},
		{"unknown state", map[string]any{"configEntries": map[string]any{"states": []string{"broken"}}}, "states"},
		{"unknown severity", map[string]any{"repairs": map[string]any{"severities": []string{"fatal"}}}, "severities"},
		{"class without a fault polarity", map[string]any{"sensors": map[string]any{"deviceClasses": []string{"door"}}}, "deviceClasses"},
	} {
		raw, _ := json.Marshal(map[string]any{"endpoint": "http://ha.example.org", "surfaces": tc.block})
		_, err := parseConfig(raw)
		if err == nil || !strings.Contains(err.Error(), tc.expect) {
			t.Fatalf("%s: expected an error naming %q, got %v", tc.name, tc.expect, err)
		}
	}
}

// ---- the surfaces, end to end against the fake --------------------------------

func surfaceAdapter(t *testing.T, ha *fakeHA, extra map[string]any) (*fakeManager, *adapter) {
	t.Helper()
	t.Setenv("AGENTOPS_CRED_HA_LOGS_token", "secret")
	fm := newFakeManager(t, sourceInfo("ha-logs", ha.URL, extra))
	a := newTestAdapter(fm)
	a.poll = 50 * time.Millisecond
	a.states = 50 * time.Millisecond
	return fm, a
}

func waitCalls(t *testing.T, ha *fakeHA, typ string, more int) {
	t.Helper()
	target := ha.Calls(typ) + more
	waitFor(t, typ+" reads", func() bool { return ha.Calls(typ) >= target })
}

// The case that started this: an integration that cannot reach its device
// sits in setup_retry and logs nothing about it after startup.
func TestConfigEntryFailureIsReportedOnceWithItsReason(t *testing.T) {
	ha := newFakeHA(t, "secret")
	failing := configEntry{EntryID: "e1", Domain: "fully_kiosk", Title: "Fire Tablet", State: "setup_retry",
		Reason: "Cannot connect to host 192.0.2.10:2323"}
	ha.SetEntries(failing)
	fm, a := surfaceAdapter(t, ha, nil)
	runAdapter(t, a)

	waitFor(t, "the config-entry signal", func() bool { return len(fm.Posted()) == 1 })
	// The cursor is seeded to "now" on a first start; nothing below may move it.
	seeded := fm.State("ha-logs", "last-record")
	sig := fm.Posted()[0]
	if sig.Labels["surface"] != surfaceConfigEntry || sig.Labels["integration"] != "fully_kiosk" {
		t.Fatalf("labels: %v", sig.Labels)
	}
	if sig.Fingerprint != "ha-logs@config-entry@fully_kiosk/e1" {
		t.Fatalf("fingerprint = %q", sig.Fingerprint)
	}
	if !strings.Contains(sig.Payload, "Cannot connect to host") || !strings.Contains(sig.Title, "fully_kiosk") {
		t.Fatalf("the entry's own reason must be the message: title=%q payload=%s", sig.Title, sig.Payload)
	}
	// Standing: several more sweeps, no second report.
	waitCalls(t, ha, "config_entries/get", 4)
	if n := len(fm.Posted()); n != 1 {
		t.Fatalf("a standing condition was reported %d times", n)
	}
	// Recovered, then failed again: a new report.
	loaded := failing
	loaded.State, loaded.Reason = "loaded", ""
	ha.SetEntries(loaded)
	waitCalls(t, ha, "config_entries/get", 3)
	ha.SetEntries(failing)
	waitFor(t, "the second report after recovery", func() bool { return len(fm.Posted()) == 2 })
	if got := fm.State("ha-logs", "last-record"); got != seeded {
		t.Fatalf("a surface record must not move the log cursor: %q -> %q", seeded, got)
	}
}

func TestRepairIsReportedWithWhatItNamesAndWarningsAreNot(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetIssues(
		repairIssue{Domain: "automation", IssueID: "ring_popup_service_not_found", Severity: "error", IsFixable: true,
			TranslationKey: "service_not_found",
			Placeholders:   map[string]string{"service": "browser_mod.more_info", "name": "ring_show_camera_popup"}},
		repairIssue{Domain: "hacs", IssueID: "deprecated_yaml", Severity: "warning"},
	)
	fm, a := surfaceAdapter(t, ha, nil)
	runAdapter(t, a)

	waitFor(t, "the repair signal", func() bool { return len(fm.Posted()) >= 1 })
	waitCalls(t, ha, "repairs/list_issues", 3)
	posted := fm.Posted()
	if len(posted) != 1 {
		t.Fatalf("expected the error-severity repair alone, got %d: %+v", len(posted), posted)
	}
	sig := posted[0]
	if sig.Labels["surface"] != surfaceRepair || sig.Labels["integration"] != "automation" || sig.Labels["level"] != "ERROR" {
		t.Fatalf("labels: %v", sig.Labels)
	}
	if sig.Fingerprint != "ha-logs@repair-issue@automation/ring_popup_service_not_found" {
		t.Fatalf("fingerprint = %q", sig.Fingerprint)
	}
	if !strings.Contains(sig.Payload, "browser_mod.more_info") || !strings.Contains(sig.Title, "service_not_found") {
		t.Fatalf("the placeholders must reach the reader: title=%q payload=%s", sig.Title, sig.Payload)
	}
}

func TestSensorFaultIsReportedWithItsIntegration(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetRegistry(registryEntry{EntityID: "binary_sensor.cabinet_warnout", Platform: "tion"})
	ha.SetStates(
		entityState{EntityID: "binary_sensor.cabinet_warnout", State: "on",
			Attributes: map[string]any{"device_class": "problem", "friendly_name": "Cabinet warn-out"}},
		entityState{EntityID: "binary_sensor.bridge_link", State: "off",
			Attributes: map[string]any{"device_class": "connectivity"}},
		entityState{EntityID: "binary_sensor.other_problem", State: "off",
			Attributes: map[string]any{"device_class": "problem"}},
		entityState{EntityID: "binary_sensor.front_door", State: "on",
			Attributes: map[string]any{"device_class": "door"}},
	)
	fm, a := surfaceAdapter(t, ha, nil)
	runAdapter(t, a)

	waitFor(t, "two sensor signals", func() bool { return len(fm.Posted()) == 2 })
	waitCalls(t, ha, "get_states", 3)
	posted := fm.Posted()
	if len(posted) != 2 {
		t.Fatalf("expected exactly the two faulting sensors, got %d: %+v", len(posted), posted)
	}
	byEntity := map[string]Signal{}
	for _, s := range posted {
		byEntity[s.Labels["location"]] = s
	}
	if got := byEntity["binary_sensor.cabinet_warnout"].Labels["integration"]; got != "tion" {
		t.Fatalf("a registered entity groups by its platform, got %q", got)
	}
	if got := byEntity["binary_sensor.bridge_link"].Labels["integration"]; got != "binary_sensor" {
		t.Fatalf("an unregistered entity groups by its domain, got %q", got)
	}
	if !strings.Contains(byEntity["binary_sensor.cabinet_warnout"].Title, "Cabinet warn-out") {
		t.Fatalf("title: %q", byEntity["binary_sensor.cabinet_warnout"].Title)
	}
}

func updateState(id, installed, latest string) entityState {
	return entityState{EntityID: id, State: "on",
		Attributes: map[string]any{"installed_version": installed, "latest_version": latest}}
}

func TestUpdatesAreOffByDefault(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetStates(updateState("update.mushroom_update", "v5.2.2", "v5.2.3"), updateState("update.kiosk_mode_update", "v14.0.2", "v14.1.0"))
	fm, a := surfaceAdapter(t, ha, nil)
	runAdapter(t, a)

	waitCalls(t, ha, "get_states", 3) // the sensor surface reads states; the digest must not ride on it
	if n := len(fm.Posted()); n != 0 {
		t.Fatalf("updates are off by default, got %d posts", n)
	}
}

func TestUpdatesAreOneDigestRepostedOnGrowth(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetStates(updateState("update.mushroom_update", "v5.2.2", "v5.2.3"), updateState("update.kiosk_mode_update", "v14.0.2", "v14.1.0"))
	fm, a := surfaceAdapter(t, ha, map[string]any{"surfaces": map[string]any{"updates": map[string]any{"enabled": true}}})
	runAdapter(t, a)

	waitFor(t, "the digest", func() bool { return len(fm.Posted()) == 1 })
	sig := fm.Posted()[0]
	if sig.Labels["surface"] != surfaceUpdate || sig.Labels["integration"] != "updates" || sig.Fingerprint != "ha-logs@pending-updates@pending" {
		t.Fatalf("digest identity: %q %v", sig.Fingerprint, sig.Labels)
	}
	if !strings.Contains(sig.Payload, "update.mushroom_update") || !strings.Contains(sig.Payload, "update.kiosk_mode_update") ||
		!strings.Contains(sig.Title, "2 pending") {
		t.Fatalf("the digest carries the whole set: title=%q payload=%s", sig.Title, sig.Payload)
	}
	waitCalls(t, ha, "get_states", 3)
	if n := len(fm.Posted()); n != 1 {
		t.Fatalf("an unchanged set was re-posted: %d", n)
	}
	// One applied: the set shrank, not news.
	ha.SetStates(updateState("update.kiosk_mode_update", "v14.0.2", "v14.1.0"))
	waitCalls(t, ha, "get_states", 3)
	if n := len(fm.Posted()); n != 1 {
		t.Fatalf("a shrinking set was re-posted: %d", n)
	}
	// A new one: the set grew, re-posted with everything pending.
	ha.SetStates(updateState("update.kiosk_mode_update", "v14.0.2", "v14.1.0"), updateState("update.timeflow_card_update", "v3.4", "v3.5.1"))
	waitFor(t, "the grown digest", func() bool { return len(fm.Posted()) == 2 })
	if p := fm.Posted()[1].Payload; !strings.Contains(p, "timeflow_card") || !strings.Contains(p, "kiosk_mode") {
		t.Fatalf("the re-posted digest must carry the whole set: %s", p)
	}
}

func TestDisabledSurfacesIssueNoReads(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetIssues(repairIssue{Domain: "automation", IssueID: "x", Severity: "error"})
	ha.SetStates(entityState{EntityID: "binary_sensor.p", State: "on", Attributes: map[string]any{"device_class": "problem"}})
	off := map[string]any{"enabled": false}
	fm, a := surfaceAdapter(t, ha, map[string]any{"surfaces": map[string]any{
		"configEntries": off, "repairs": off, "sensors": off, "updates": off,
	}})
	runAdapter(t, a)

	waitFor(t, "several sweeps", func() bool { return ha.ListCalls() >= 4 })
	for _, typ := range []string{"repairs/list_issues", "get_states", "config/entity_registry/list"} {
		if n := ha.Calls(typ); n != 0 {
			t.Fatalf("%s was read %d times with its surface off", typ, n)
		}
	}
	if n := len(fm.Posted()); n != 0 {
		t.Fatalf("a disabled surface posted %d signals", n)
	}
}

// A Home Assistant restart passes entries through setup_retry and loads them a
// moment later. The rule that ships dwells, and the re-check asks the surface
// itself: an entry loaded before the close is churn, one still failing is the
// incident — with the dwell's evidence attached.
func TestConfigEntryDwellAsksTheSurfaceAtTheClose(t *testing.T) {
	ha := newFakeHA(t, "secret")
	stuck := configEntry{EntryID: "h1", Domain: "hue", State: "setup_retry", Reason: "bridge unreachable"}
	churn := configEntry{EntryID: "t1", Domain: "tuya", State: "setup_retry", Reason: "cloud slow"}
	ha.SetEntries(stuck, churn)
	fm, a := surfaceAdapter(t, ha, map[string]any{"rules": []map[string]any{
		{"matchers": []string{`surface="config-entry"`}, "for": "300ms"},
	}})
	ctx := runAdapter(t, a)
	go a.runDwellFlusher(ctx)

	waitCalls(t, ha, "config_entries/get", 2)
	churn.State, churn.Reason = "loaded", ""
	ha.SetEntries(stuck, churn)

	// dwellTick is 5s in production code, so the flusher's second tick is what
	// decides these entries; give it generous room.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && len(fm.Posted()) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	posted := fm.Posted()
	if len(posted) != 1 || posted[0].Labels["integration"] != "hue" {
		t.Fatalf("expected the still-failing entry alone, got %+v", posted)
	}
	if !strings.Contains(posted[0].Payload, "failing for") {
		t.Fatalf("expected the dwell evidence in the payload, got %q", posted[0].Payload)
	}
}

// Self-exclusion holds on every surface: a repair whose text names agent-ops is
// about agent-ops' own machinery, whatever the configuration says.
func TestRepairNamingAgentOpsIsExcluded(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetIssues(
		repairIssue{Domain: "automation", IssueID: "own", Severity: "error", TranslationKey: "service_not_found",
			Placeholders: map[string]string{"name": "agentops_notify"}},
		repairIssue{Domain: "zwave_js", IssueID: "other", Severity: "error", TranslationKey: "device_unsupported"},
	)
	fm, a := surfaceAdapter(t, ha, nil)
	runAdapter(t, a)

	waitFor(t, "the ordinary repair", func() bool { return len(fm.Posted()) >= 1 })
	waitCalls(t, ha, "repairs/list_issues", 3)
	posted := fm.Posted()
	if len(posted) != 1 || posted[0].Labels["integration"] != "zwave_js" {
		t.Fatalf("expected the zwave_js repair alone, got %+v", posted)
	}
}
