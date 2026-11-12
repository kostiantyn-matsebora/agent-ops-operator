package nats

import (
	"fmt"
	"strings"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
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

// logFilter is every subject of the queue log; deadFilter every one of the
// dead log.
func (s subjects) logFilter() string  { return s.prefix + ".log.>" }
func (s subjects) deadFilter() string { return s.prefix + ".dead.>" }

func (s subjects) keyBase(q commandbus.Queue, k commandbus.Key) string {
	return s.prefix + ".log." + escapeToken(string(q)) + "." + escapeToken(string(k))
}

// forEntry is the subject an entry is appended to: the key's .msg for a
// message, the key's .ev for everything that happens to one.
func (s subjects) forEntry(e entrylog.Entry) string {
	if e.Type == entrylog.TypeMsg {
		return s.keyBase(e.Queue, e.Key) + ".msg"
	}
	return s.keyBase(e.Queue, e.Key) + ".ev"
}

func (s subjects) forDead(q commandbus.Queue, uid string) string {
	return s.prefix + ".dead." + escapeToken(string(q)) + "." + escapeToken(uid)
}

// msgID is the dedup id of an entry, so a retried publish of the SAME entry
// within the stream's duplicate window is not appended twice. A claim carries
// its own lease token: two replicas claiming one attempt must be two different
// ids, or the server would answer the loser's append with a duplicate ack
// before ever checking its condition, and both would believe they hold the
// lease.
func msgID(e entrylog.Entry) string {
	return fmt.Sprintf("%s.%s.%d.%s", e.UID, e.Type, e.Attempt, e.Token)
}
