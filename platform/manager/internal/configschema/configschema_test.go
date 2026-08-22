package configschema

import (
	"strings"
	"testing"
)

const telegramish = `{
  "type": "object",
  "properties": {
    "chatId": {"type": "string"},
    "feedThreadId": {"type": "integer"},
    "pollingEnabled": {"type": "boolean"}
  },
  "required": ["chatId"]
}`

func mustCompile(t *testing.T, doc string) *Schema {
	t.Helper()
	s, err := Compile([]byte(doc))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return s
}

// Nothing declared is not a failure — it means "no contract to check".
func TestCompileEmptyIsNoSchema(t *testing.T) {
	for _, doc := range []string{"", "   ", "\n"} {
		s, err := Compile([]byte(doc))
		if err != nil || s != nil {
			t.Fatalf("empty doc %q: want (nil, nil), got (%v, %v)", doc, s, err)
		}
	}
	if v := Validate(nil, []byte(`{"anything": true}`)); v != nil {
		t.Fatalf("nil schema must yield no violations: %+v", v)
	}
}

func TestCompileRejectsBrokenSchema(t *testing.T) {
	for name, doc := range map[string]string{
		"not json":     `{"type": "object"`,
		"bad keyword":  `{"type": "objekt"}`,
		"bad required": `{"required": "chatId"}`,
	} {
		if _, err := Compile([]byte(doc)); err == nil {
			t.Fatalf("%s: expected a compile error for %s", name, doc)
		}
	}
}

func TestValidateConformingConfig(t *testing.T) {
	s := mustCompile(t, telegramish)
	if v := Validate(s, []byte(`{"chatId": "-100123", "feedThreadId": 2}`)); v != nil {
		t.Fatalf("conforming config reported violations: %+v", v)
	}
}

// A required field must bite even when config is omitted entirely, otherwise
// the most common mistake — writing no config at all — would pass silently.
func TestValidateMissingRequiredField(t *testing.T) {
	s := mustCompile(t, telegramish)
	for _, cfg := range []string{`{"feedThreadId": 2}`, ``, `{}`} {
		v := Validate(s, []byte(cfg))
		if len(v) == 0 {
			t.Fatalf("config %q: expected a violation for the missing required field", cfg)
		}
		msg := Summarize(v, 0)
		if !strings.Contains(msg, "chatId") {
			t.Fatalf("config %q: violation should name chatId, got %q", cfg, msg)
		}
		// the message reaches a status condition a human reads, so it must be
		// prose — not Go's default struct rendering
		if !strings.Contains(msg, "missing property") || strings.Contains(msg, "&{") {
			t.Fatalf("config %q: violation must be human-readable, got %q", cfg, msg)
		}
	}
}

func TestValidateWrongTypeNamesThePath(t *testing.T) {
	s := mustCompile(t, telegramish)
	v := Validate(s, []byte(`{"chatId": "x", "feedThreadId": "two"}`))
	if len(v) == 0 {
		t.Fatal("expected a type violation")
	}
	msg := Summarize(v, 0)
	if !strings.Contains(msg, "feedThreadId") {
		t.Fatalf("violation should locate feedThreadId: %q", msg)
	}
	if !strings.Contains(msg, "want integer") || strings.Contains(msg, "&{") {
		t.Fatalf("violation must explain the mismatch in prose: %q", msg)
	}
}

func TestValidateRejectsUnparseableConfig(t *testing.T) {
	s := mustCompile(t, telegramish)
	v := Validate(s, []byte(`{"chatId":`))
	if len(v) != 1 || !strings.Contains(v[0].Message, "not valid JSON") {
		t.Fatalf("malformed config should report one clear violation: %+v", v)
	}
}

// Condition messages are size-bounded, so a pathological schema cannot produce
// an unwritable status.
func TestSummarizeTruncates(t *testing.T) {
	vs := []Violation{{Path: "/a", Message: strings.Repeat("x", 200)}}
	got := Summarize(vs, 50)
	if len(got) != 50 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncation: len=%d suffix=%q", len(got), got[max(0, len(got)-3):])
	}
	if Summarize(nil, 50) != "" {
		t.Fatal("no violations must summarize to empty")
	}
}
