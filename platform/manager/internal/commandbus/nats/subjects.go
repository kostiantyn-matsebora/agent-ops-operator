package nats

import (
	"fmt"
	"strings"
)

// wireKeyAllowed is the character set a subject token may hold unescaped.
// Restated here rather than imported from internal/state/nats/key.go —
// design D-1 keeps the two backends independent.
func wireKeyAllowed(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '-' || b == '=':
		return true
	}
	return false
}

// escapeIntroducer starts a two-hex-digit escape, reserved so a producer's
// own "_" can never be mistaken for one.
const escapeIntroducer = '_'

// emptySegment stands in for an empty string (KindDefault, an unscoped
// install): an empty token between two dots is not a valid subject. It is
// unambiguous against an escaped segment, which never emits a bare "_".
const emptySegment = "_"

// escapeToken maps an opaque producer-chosen string onto one valid subject
// token: every byte outside wireKeyAllowed, including the escape introducer
// and the "." that separates tokens, becomes "_XX" of its own byte value.
func escapeToken(s string) string {
	if s == "" {
		return emptySegment
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if wireKeyAllowed(c) && c != escapeIntroducer {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "_%02X", c)
	}
	return b.String()
}

// subjects names the subjects of one scope.
type subjects struct{ prefix string }

func newSubjects(scope string) subjects { return subjects{prefix: "cb." + escapeToken(scope)} }
