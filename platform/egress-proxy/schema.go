package main

import "encoding/json"

// A PARAMETER WITH NO DECLARED TYPE IS A TOOL THE AGENT CANNOT CALL.
//
// Home Assistant advertises `GetLiveContext`'s `domain` filter like this:
//
//	"domain": {"anyOf": [{}, {"type": "array", "items": {"type": "string"}}],
//	           "description": "... Accepts a single domain or a list."}
//
// The first branch is an EMPTY SCHEMA — it constrains nothing, so the union as a
// whole constrains nothing, and a model told nothing writes a bare word:
//
//	{"domain": sensor, "name": "thermometer"}
//
// which is not valid JSON. The neighbouring `name`, typed `string`, comes out
// quoted in the same call. Measured on one install, 59 of 110 calls to that tool
// never executed — no server saw them, no allowlist refused them, nothing left
// the pod.
//
// The proxy already rewrites tool listings on their way back, so this is the
// one place that can repair the advertisement before the model ever sees it.
//
// IT INVENTS NOTHING. The only edit is to DROP a branch that says nothing, which
// is exactly the branch that makes the union unsatisfiable-by-guessing. What is
// left is the schema the server already published for the other case — for Home
// Assistant, an array of strings, which that tool accepts and which a model
// writes correctly.
//
// A property whose ONLY branch is empty is left alone: there would be nothing
// left to describe it with, and a silently emptied schema is worse than a
// useless one.

// repairSchemas normalises every tool's inputSchema in a listing, and reports
// whether anything changed.
func repairSchemas(tools []map[string]json.RawMessage) bool {
	changed := false
	for _, t := range tools {
		raw, ok := t["inputSchema"]
		if !ok {
			continue
		}
		var schema map[string]json.RawMessage
		if json.Unmarshal(raw, &schema) != nil {
			continue
		}
		propsRaw, ok := schema["properties"]
		if !ok {
			continue
		}
		var props map[string]json.RawMessage
		if json.Unmarshal(propsRaw, &props) != nil {
			continue
		}
		touched := false
		for name, propRaw := range props {
			repaired, ok := dropEmptyBranches(propRaw)
			if !ok {
				continue
			}
			props[name] = repaired
			touched = true
		}
		if !touched {
			continue
		}
		newProps, err := json.Marshal(props)
		if err != nil {
			continue
		}
		schema["properties"] = newProps
		newSchema, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		t["inputSchema"] = newSchema
		changed = true
	}
	return changed
}

// dropEmptyBranches removes `anyOf`/`oneOf` branches that constrain nothing.
//
// Returns the rewritten property and whether it was rewritten at all. A single
// surviving branch is INLINED, so what the model reads is an ordinary typed
// property rather than a union of one.
func dropEmptyBranches(propRaw json.RawMessage) (json.RawMessage, bool) {
	var prop map[string]json.RawMessage
	if json.Unmarshal(propRaw, &prop) != nil {
		return nil, false
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		raw, ok := prop[key]
		if !ok {
			continue
		}
		var branches []map[string]json.RawMessage
		if json.Unmarshal(raw, &branches) != nil {
			continue
		}
		kept := make([]map[string]json.RawMessage, 0, len(branches))
		for _, b := range branches {
			if len(b) == 0 {
				continue
			}
			kept = append(kept, b)
		}
		// Nothing empty, or nothing BUT empty: leave it exactly as published.
		if len(kept) == len(branches) || len(kept) == 0 {
			continue
		}
		delete(prop, key)
		if len(kept) == 1 {
			// Inline the survivor, keeping whatever the property said about
			// itself — the description is usually the only guidance a model has.
			for k, v := range kept[0] {
				if _, taken := prop[k]; !taken {
					prop[k] = v
				}
			}
		} else {
			rest, err := json.Marshal(kept)
			if err != nil {
				return nil, false
			}
			prop[key] = rest
		}
		out, err := json.Marshal(prop)
		if err != nil {
			return nil, false
		}
		return out, true
	}
	return nil, false
}
