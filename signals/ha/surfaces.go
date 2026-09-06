package main

// The health surfaces beside the log: config entry state, repairs, fault
// sensors and pending updates. Each is read on its own cadence (main.go) and
// normalised HERE into a logRecord shaped for it, so that a condition takes
// the one consider path a log record takes — self-exclusion, scope, rules,
// inhibition, dwell — and no surface ends up with a policy of its own.
//
// A surface condition is a STATE, not an occurrence: a failed entry stands,
// where a log record happens. What is fed to consider is the condition's
// APPEARANCE — the standing set in main.go decides that — and its record
// carries the time it was first seen.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The `surface` label values.
const (
	surfaceLog         = "log"
	surfaceConfigEntry = "config-entry"
	surfaceRepair      = "repair"
	surfaceSensor      = "sensor"
	surfaceUpdate      = "update"
)

// The record NAME a surface condition carries in the logger slot. Hyphenated,
// which no Python logger name can be, so a surface record can never share a
// key with a log record and the dwell ladder can tell the two apart from the
// name alone.
const (
	nameConfigEntry = "config-entry"
	nameRepair      = "repair-issue"
	nameSensor      = "sensor-fault"
	nameUpdates     = "pending-updates"
)

// surfaceOfName maps a record name back to its surface, or "" for a log record.
func surfaceOfName(name string) string {
	switch name {
	case nameConfigEntry:
		return surfaceConfigEntry
	case nameRepair:
		return surfaceRepair
	case nameSensor:
		return surfaceSensor
	case nameUpdates:
		return surfaceUpdate
	}
	return ""
}

// configEntryStates is Home Assistant's ConfigEntryState vocabulary.
var configEntryStates = map[string]bool{
	"loaded": true, "setup_error": true, "migration_error": true, "setup_retry": true,
	"not_loaded": true, "failed_unload": true, "setup_in_progress": true,
}

// repairSeverities is Home Assistant's IssueSeverity vocabulary.
var repairSeverities = map[string]bool{"critical": true, "error": true, "warning": true}

// sensorFaultState is, per binary_sensor device class, the state that means a
// fault. Only classes with a fault polarity are here: a `door` being open is
// not a problem, so it cannot be watched, and the config says so.
var sensorFaultState = map[string]string{
	"problem": "on", "safety": "on", "tamper": "on", "smoke": "on", "gas": "on",
	"carbon_monoxide": "on", "moisture": "on", "heat": "on", "cold": "on", "battery": "on",
	"connectivity": "off",
}

// surfaceRecord shapes one condition as a record. id is the condition's
// identity within its surface and becomes the source-location slot, so
// Key() — name@id — is the fingerprint's tail and the standing set's key.
func surfaceRecord(name, id, surface, integration, level, message string, at time.Time, extra map[string]any) logRecord {
	return logRecord{
		Name:        name,
		Message:     []string{message},
		Level:       level,
		Source:      []json.RawMessage{json.RawMessage(strconv.Quote(id))},
		Timestamp:   float64(at.UnixNano()) / 1e9,
		Count:       1,
		Surface:     surface,
		Integration: integration,
		Extra:       extra,
	}
}

// configEntryRecords is the config-entry surface: one condition per entry in a
// counting state, keyed for the standing set.
func configEntryRecords(entries []configEntry, accept map[string]bool, now time.Time) map[string]logRecord {
	out := map[string]logRecord{}
	for _, e := range entries {
		if !accept[e.State] || e.Domain == "" {
			continue
		}
		title := e.Title
		if title == "" {
			title = e.Domain
		}
		msg := fmt.Sprintf("%s (%s) is in %s", title, e.Domain, e.State)
		if e.Reason != "" {
			msg += ": " + e.Reason
		}
		rec := surfaceRecord(nameConfigEntry, e.Domain+"/"+e.EntryID, surfaceConfigEntry, e.Domain, "ERROR", msg, now,
			map[string]any{"entryId": e.EntryID, "title": e.Title, "state": e.State, "reason": e.Reason})
		out[rec.Key()] = rec
	}
	return out
}

// repairRecords is the repair surface: one condition per issue of a counting
// severity. The message is the translation key and the placeholders, which is
// what a person would read on the Repairs page — the automation, the missing
// service, the edit link.
func repairRecords(issues []repairIssue, accept map[string]bool, now time.Time) map[string]logRecord {
	out := map[string]logRecord{}
	for _, i := range issues {
		if !accept[strings.ToLower(i.Severity)] || i.Domain == "" {
			continue
		}
		keys := make([]string, 0, len(i.Placeholders))
		for k := range i.Placeholders {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+i.Placeholders[k])
		}
		msg := i.TranslationKey
		if msg == "" {
			msg = i.IssueID
		}
		if len(parts) > 0 {
			msg += ": " + strings.Join(parts, ", ")
		}
		rec := surfaceRecord(nameRepair, i.Domain+"/"+i.IssueID, surfaceRepair, i.Domain, strings.ToUpper(i.Severity), msg, now,
			map[string]any{
				"issueId": i.IssueID, "severity": i.Severity, "fixable": i.IsFixable, "created": i.Created,
				"translationKey": i.TranslationKey, "placeholders": i.Placeholders,
				"learnMoreUrl": i.LearnMoreURL, "breaksInVersion": i.BreaksIn,
			})
		out[rec.Key()] = rec
	}
	return out
}

// sensorRecords is the sensor surface: one condition per binary_sensor of a
// watched device class in its fault state. The integration is the platform
// the registry names; an entity the registry does not list is labelled by
// its own domain rather than dropped.
func sensorRecords(states []entityState, registry map[string]string, accept map[string]bool, now time.Time) map[string]logRecord {
	out := map[string]logRecord{}
	for i := range states {
		s := &states[i]
		if !strings.HasPrefix(s.EntityID, "binary_sensor.") {
			continue
		}
		class := s.attr("device_class")
		if !accept[class] || sensorFaultState[class] != s.State {
			continue
		}
		integration := registry[s.EntityID]
		if integration == "" {
			integration = "binary_sensor"
		}
		name := s.attr("friendly_name")
		if name == "" {
			name = s.EntityID
		}
		msg := fmt.Sprintf("%s (%s) reports %s: %s", name, s.EntityID, class, s.State)
		rec := surfaceRecord(nameSensor, s.EntityID, surfaceSensor, integration, "ERROR", msg, now,
			map[string]any{"entityId": s.EntityID, "deviceClass": class, "state": s.State,
				"friendlyName": s.attr("friendly_name"), "lastChanged": s.LastChanged})
		out[rec.Key()] = rec
	}
	return out
}

// pendingUpdate is one update.* entity with a newer version.
type pendingUpdate struct {
	EntityID  string `json:"entityId"`
	Title     string `json:"title,omitempty"`
	Installed string `json:"installedVersion"`
	Latest    string `json:"latestVersion"`
}

// pendingUpdates lists every update.* entity in state on, sorted by entity id.
func pendingUpdates(states []entityState) []pendingUpdate {
	var out []pendingUpdate
	for i := range states {
		s := &states[i]
		if !strings.HasPrefix(s.EntityID, "update.") || s.State != "on" {
			continue
		}
		out = append(out, pendingUpdate{
			EntityID: s.EntityID, Title: s.attr("title"),
			Installed: s.attr("installed_version"), Latest: s.attr("latest_version"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntityID < out[j].EntityID })
	return out
}

// updateDigest is the update surface: ONE record per source listing every
// pending update. It is fed when the set grows (main.go), never per entity — a
// list is the natural unit, and seven conversations about seven packages is
// the shape this exists to avoid. Labelled `updates` so it groups with
// nothing else.
func updateDigest(pending []pendingUpdate, now time.Time) logRecord {
	lines := make([]string, 0, len(pending))
	for _, u := range pending {
		label := u.Title
		if label == "" {
			label = strings.TrimPrefix(u.EntityID, "update.")
		}
		lines = append(lines, fmt.Sprintf("%s %s → %s", label, u.Installed, u.Latest))
	}
	msg := fmt.Sprintf("%d pending update(s): %s", len(pending), strings.Join(lines, "; "))
	return surfaceRecord(nameUpdates, "pending", surfaceUpdate, "updates", "INFO", msg, now,
		map[string]any{"pending": pending})
}
