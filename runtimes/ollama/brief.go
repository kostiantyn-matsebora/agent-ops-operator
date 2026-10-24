// The `<brief>` contract field (design D-I, coordinated-agents task 2.14):
// one or two sentences of what a conversation is ABOUT, format.md asks the
// agent for and this runtime extracts the same way it extracts the context
// handle — never a displayed section, so it must not reach Result.
package main

import (
	"regexp"
	"strings"
)

// MaxBrief mirrors ConversationStatus.Brief's MaxLength — bounding here means
// a value this runtime reports never gets silently truncated by the manager
// mid-sentence.
const MaxBrief = 512

// briefTag matches a standalone `<brief>...</brief>` block, mirroring the
// block grammar's own recognition rule — the tag alone on its own line —
// closely enough for this one tag, without pulling in the full parser
// channels/telegram and the console each carry for the tags a reader
// actually sees.
var briefTag = regexp.MustCompile(`(?m)^<brief>[ \t]*\r?\n([\s\S]*?)\r?\n^</brief>[ \t]*(?:\r?\n)*`)

// extractBrief pulls the brief out of the agent's final text and returns the
// text with it removed. Absent means the agent wrote none this run — leave
// it unreported, never a placeholder.
func extractBrief(text string) (rest, brief string) {
	loc := briefTag.FindStringSubmatchIndex(text)
	if loc == nil {
		return text, ""
	}
	brief = strings.Join(strings.Fields(text[loc[2]:loc[3]]), " ")
	if len(brief) > MaxBrief {
		brief = brief[:MaxBrief]
	}
	return strings.TrimSpace(text[:loc[0]] + text[loc[1]:]), brief
}
