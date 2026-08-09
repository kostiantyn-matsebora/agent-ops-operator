package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A small YAML encoder for the JSON-shaped values the console holds.
//
// Why not sigs.k8s.io/yaml: this module is dependency-free, like every other
// adapter, and pulling a YAML library in for one read-only view would trade that
// for output nobody diffs against anyway. The proposal narrows the
// dependency-free rule for the FRONTEND (npm inside console/ and its image), not
// for the Go half.
//
// What it must get right is exactly what a `kubectl get -o yaml` reader needs:
// stable key order, correct quoting of strings that would otherwise parse as
// something else, block scalars for multi-line text, and empty collections
// rendered as {} / [] rather than silently dropped.

// encodeYAML renders a decoded-JSON value as YAML.
func encodeYAML(v any) string {
	var b strings.Builder
	writeYAMLValue(&b, v, 0, false)
	return b.String()
}

// writeYAMLValue writes v at the given indent. inline reports that the caller
// already emitted the key and a space, so a scalar continues on that line while
// a collection starts on the next.
func writeYAMLValue(b *strings.Builder, v any, indent int, inline bool) {
	pad := strings.Repeat("  ", indent)
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			finishScalar(b, "{}", inline)
			return
		}
		if inline {
			b.WriteString("\n")
		}
		for _, k := range sortedMapKeys(t) {
			b.WriteString(pad)
			b.WriteString(yamlKey(k))
			b.WriteString(":")
			if isCollection(t[k]) && !isEmptyCollection(t[k]) {
				writeYAMLValue(b, t[k], indent+1, true)
				continue
			}
			b.WriteString(" ")
			writeYAMLValue(b, t[k], indent+1, true)
		}
	case []any:
		if len(t) == 0 {
			finishScalar(b, "[]", inline)
			return
		}
		if inline {
			b.WriteString("\n")
		}
		for _, item := range t {
			b.WriteString(pad)
			b.WriteString("-")
			if m, ok := item.(map[string]any); ok && len(m) > 0 {
				// a mapping inside a sequence starts on the SAME line as the
				// dash, which is what makes nested lists readable
				writeSequenceMapping(b, m, indent+1)
				continue
			}
			b.WriteString(" ")
			writeYAMLValue(b, item, indent+1, true)
		}
	default:
		finishScalar(b, yamlScalar(v, indent), inline)
	}
}

// writeSequenceMapping renders "- key: value" with the remaining keys aligned
// under it.
func writeSequenceMapping(b *strings.Builder, m map[string]any, indent int) {
	pad := strings.Repeat("  ", indent)
	keys := sortedMapKeys(m)
	for i, k := range keys {
		if i == 0 {
			b.WriteString(" ")
		} else {
			b.WriteString(pad)
		}
		b.WriteString(yamlKey(k))
		b.WriteString(":")
		if isCollection(m[k]) && !isEmptyCollection(m[k]) {
			writeYAMLValue(b, m[k], indent+1, true)
			continue
		}
		b.WriteString(" ")
		writeYAMLValue(b, m[k], indent+1, true)
	}
}

func finishScalar(b *strings.Builder, s string, inline bool) {
	b.WriteString(s)
	if inline {
		b.WriteString("\n")
	}
}

func isCollection(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func isEmptyCollection(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		return len(t) == 0
	case []any:
		return len(t) == 0
	}
	return false
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// metadata/spec/status read better in the order kubectl prints them, and
	// apiVersion/kind belong at the top — everything else stays alphabetical.
	priority := map[string]int{"apiVersion": 0, "kind": 1, "metadata": 2, "spec": 3, "status": 4}
	sort.SliceStable(keys, func(i, j int) bool {
		pi, oki := priority[keys[i]]
		pj, okj := priority[keys[j]]
		if oki && okj {
			return pi < pj
		}
		if oki != okj {
			return oki
		}
		return keys[i] < keys[j]
	})
	return keys
}

func yamlKey(k string) string {
	if needsQuoting(k) {
		return strconv.Quote(k)
	}
	return k
}

// yamlScalar renders a leaf. Multi-line strings become block scalars — an agent
// result or a condition message is often several lines, and folding them into
// one quoted string is what makes a YAML view unreadable.
func yamlScalar(v any, indent int) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case string:
		if strings.Contains(t, "\n") {
			pad := strings.Repeat("  ", indent)
			lines := strings.Split(strings.TrimRight(t, "\n"), "\n")
			var b strings.Builder
			b.WriteString("|-")
			for _, line := range lines {
				b.WriteString("\n")
				b.WriteString(pad)
				b.WriteString(line)
			}
			return b.String()
		}
		if needsQuoting(t) {
			return strconv.Quote(t)
		}
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

// needsQuoting reports whether a plain string would parse as something else —
// a number, a bool, null, or YAML structure. Over-quoting is harmless here;
// under-quoting changes the value's type.
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	switch strings.ToLower(s) {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	if strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`,\n\t") {
		return true
	}
	return strings.TrimSpace(s) != s || strings.HasPrefix(s, "-")
}
