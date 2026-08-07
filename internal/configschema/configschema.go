// Package configschema applies an adapter-declared JSON Schema to a served
// CR's opaque spec.config.
//
// This is the ONLY place the manager touches config content, and it stays
// type-blind by construction: it compiles whatever schema document the adapter
// CR declares and reports violations mechanically. There is no per-type
// knowledge here and none anywhere else in the manager — swapping the
// validator means changing this file alone.
package configschema

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Schema is a compiled, reusable schema document.
type Schema struct {
	compiled *jsonschema.Schema
}

// Violation is one place the config disagrees with the schema.
type Violation struct {
	// Path is the JSON pointer into config ("" for the document root).
	Path string
	// Message is the validator's reason, e.g. "missing property 'chatId'".
	Message string
}

func (v Violation) String() string {
	if v.Path == "" {
		return v.Message
	}
	return v.Path + ": " + v.Message
}

// Compile parses a declared schema document. A nil/empty document yields no
// schema and no error — "nothing declared" is not a failure.
func Compile(raw []byte) (*Schema, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	const url = "adapter://configSchema"
	if err := c.AddResource(url, doc); err != nil {
		return nil, err
	}
	compiled, err := c.Compile(url)
	if err != nil {
		return nil, err
	}
	return &Schema{compiled: compiled}, nil
}

// Validate applies the schema to a config document. Absent config validates as
// an empty object, so a schema's `required` still bites when config is omitted
// entirely. A nil Schema yields no violations — callers report nothing rather
// than guessing.
func Validate(s *Schema, config []byte) []Violation {
	if s == nil || s.compiled == nil {
		return nil
	}
	if len(bytes.TrimSpace(config)) == 0 {
		config = []byte("{}")
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(config))
	if err != nil {
		return []Violation{{Message: fmt.Sprintf("config is not valid JSON: %v", err)}}
	}
	err = s.compiled.Validate(doc)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !asValidationError(err, &ve) {
		return []Violation{{Message: err.Error()}}
	}
	out := flatten(ve)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Summarize renders violations for a status condition message, bounded so a
// pathological schema cannot produce an unwritable condition.
func Summarize(vs []Violation, max int) string {
	if len(vs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, v.String())
	}
	msg := strings.Join(parts, "; ")
	if max > 0 && len(msg) > max {
		msg = msg[:max-3] + "..."
	}
	return msg
}

func asValidationError(err error, target **jsonschema.ValidationError) bool {
	ve, ok := err.(*jsonschema.ValidationError)
	if ok {
		*target = ve
	}
	return ok
}

// flatten walks to the leaf causes — those name the actual offending keyword,
// while parent nodes only say "doesn't validate with ...".
func flatten(ve *jsonschema.ValidationError) []Violation {
	if len(ve.Causes) == 0 {
		return []Violation{leafViolation(ve)}
	}
	var out []Violation
	for _, c := range ve.Causes {
		out = append(out, flatten(c)...)
	}
	return out
}

// leafViolation renders one leaf error. The library formats leaves as
// "at '<location>': <reason>" — reason text meant for humans, unlike the raw
// ErrorKind struct — so we split that apart to keep path and message separate.
func leafViolation(ve *jsonschema.ValidationError) Violation {
	s := ve.Error()
	if rest, found := strings.CutPrefix(s, "at '"); found {
		if loc, msg, ok := strings.Cut(rest, "': "); ok {
			return Violation{Path: loc, Message: msg}
		}
	}
	// unexpected shape: keep the library's text rather than invent one
	return Violation{Path: "/" + strings.Join(toStrings(ve.InstanceLocation), "/"), Message: s}
}

func toStrings(loc []string) []string {
	out := make([]string, 0, len(loc))
	for _, p := range loc {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
