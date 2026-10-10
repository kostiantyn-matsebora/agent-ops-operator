// Package namewords builds the deterministic word-chain used for a
// Conversation's object name (readable-conversation-names), and allocates
// that name against collisions without ever falling back to a random or
// hash-derived suffix.
package namewords

import (
	"regexp"
	"strings"
)

// stopwords are dropped before picking the first three significant words.
// A small fixed list — not configuration, since nothing tunes it per
// install.
var stopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "of": {}, "on": {}, "in": {}, "to": {},
	"and": {}, "is": {}, "do": {}, "please": {},
}

// MaxWordChainLength bounds the joined word-chain. Chosen against the
// LONGEST derived name, not the bare 63-character DNS label ceiling: the
// 18-character "agentops-mcp-conv-" prefix, the 7-character "member-" kind
// prefix and a numeric suffix of up to 4 characters ("-999") leave the
// remaining 14 characters of the 63-character limit for a suffix past
// 999, so a long-recurring base name keeps allocating. A future consumer
// that copies a conversation's name into another label value has one
// number here to check against.
const MaxWordChainLength = 24

var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var nonWord = regexp.MustCompile(`[^A-Za-z0-9]+`)

// Words extracts the word-chain built from text: split on punctuation and
// on CamelCase boundaries, drop stopwords, keep the first three remaining
// words, lowercase, join with "-". Where the joined chain would exceed
// MaxWordChainLength, whole trailing words are dropped until it fits —
// never a word cut in half.
func Words(text string) string {
	spaced := camelBoundary.ReplaceAllString(text, "$1 $2")
	var words []string
	for _, token := range nonWord.Split(spaced, -1) {
		if token == "" {
			continue
		}
		w := strings.ToLower(token)
		if _, stop := stopwords[w]; stop {
			continue
		}
		words = append(words, w)
		if len(words) == 3 {
			break
		}
	}
	for len(words) > 0 {
		chain := strings.Join(words, "-")
		if len(chain) <= MaxWordChainLength {
			return chain
		}
		words = words[:len(words)-1]
	}
	return ""
}
