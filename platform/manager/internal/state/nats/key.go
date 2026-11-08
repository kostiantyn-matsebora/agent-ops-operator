package nats

import (
	"fmt"
	"strconv"
	"strings"
)

// wireKeyAllowed is the character set a KV key may hold unescaped (nats.go's
// own doc: "alphanumeric characters, dashes, underscores, equal signs, and
// dots"). '.' is excluded even though the server allows it: this backend
// reserves it as the Appended item separator (appended.go), and the escape
// introducer itself is excluded so escaping stays unambiguous to reverse.
func wireKeyAllowed(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '-' || b == '=':
		return true
	}
	return false
}

// escapeIntroducer starts a two-hex-digit escape. Reserving it (never passed
// through unescaped, even where the server would allow it) is what keeps
// wireKey exactly reversible: a '_' in a wire key always starts an escape.
const escapeIntroducer = '_'

// wireKey maps an opaque state.Key onto a valid NATS KV key: every byte
// outside wireKeyAllowed, including '.' and a composite index's NUL
// separator, becomes "_XX" (uppercase hex) of its own byte value. A plain
// name (a conversation, a signal, a source) passes through unchanged,
// keeping `nats kv get` readable (design D-3); only a composite key (an
// AdapterState key) is escaped at all. unwireKey is its exact inverse, so a
// delete event — which carries only the wire key, no envelope — still
// recovers the original state.Key.
func wireKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	for i := 0; i < len(key); i++ {
		c := key[i]
		if wireKeyAllowed(c) && c != escapeIntroducer {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "_%02X", c)
	}
	return b.String()
}

// unwireKey reverses wireKey. A malformed escape (this process never wrote
// one) is an error rather than a silent guess.
func unwireKey(wire string) (string, error) {
	var b strings.Builder
	b.Grow(len(wire))
	for i := 0; i < len(wire); i++ {
		c := wire[i]
		if c != escapeIntroducer {
			b.WriteByte(c)
			continue
		}
		if i+2 >= len(wire) {
			return "", fmt.Errorf("nats: wire key %q ends mid-escape", wire)
		}
		n, err := strconv.ParseUint(wire[i+1:i+3], 16, 8)
		if err != nil {
			return "", fmt.Errorf("nats: wire key %q: invalid escape at %d: %w", wire, i, err)
		}
		b.WriteByte(byte(n))
		i += 2
	}
	return b.String(), nil
}

// runsItemKey composes one item's own KV key under its parent (design D-5:
// "a run is one entry runs/<conversation>/<runId>"), '.' as the hierarchy
// separator a prefix listing (ListKeysFiltered) matches on. The suffix is a
// fresh, unique token per Append call, NEVER the item's own declared id:
// store.go's Append carries no promise that two appended items are never
// given the same id (kube's own Append is a plain list append with no such
// check, and the conformance bench relies on exactly that by appending one
// id repeatedly), so an id cannot double as this backend's storage key.
// Grow therefore finds its item by a scan over the parent's own entries
// (appended.go), not a direct lookup.
func runsItemKey(parent, uniqueToken string) string {
	return wireKey(parent) + "." + wireKey(uniqueToken)
}

// runsPrefixFilter is the ListKeysFiltered pattern matching every item under
// parent.
func runsPrefixFilter(parent string) string {
	return wireKey(parent) + ".>"
}

// parentOfItemKey reverses runsItemKey's own first half: the parent an
// item's wire key was composed under.
func parentOfItemKey(wire string) (string, error) {
	i := strings.IndexByte(wire, '.')
	if i < 0 {
		return "", fmt.Errorf("nats: runs key %q has no parent separator", wire)
	}
	return unwireKey(wire[:i])
}
