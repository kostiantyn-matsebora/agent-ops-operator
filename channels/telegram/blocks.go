package main

// The block grammar, parsed HERE.
//
// The manager passes an agent's text through untouched, exactly as it passes
// markdown through untouched, and each adapter reads the grammar and renders it
// to what its transport has. See design D1 in the structured-agent-output
// change.
//
// It moved out of the manager because nothing there consumed it — the manager
// parsed, put the result on the wire, and every consumer was a renderer — and
// because the parse has to happen on the READ path too, which a manager
// composing outbound messages never runs.
//
// THIS FILE IS DUPLICATED, in TypeScript, in the console. That is the accepted
// cost of D1. The RECOGNITION RULES are stated once, in the capability spec, and
// both implementations are written against them: change one, change both, and
// keep the adversarial table in step.

import (
	"regexp"
	"strings"
)

// Role is what a block IS. Three values, and the vocabulary of SECTION NAMES is
// open on top of them: every agent has a different job, and a fixed tier set
// makes an ha-operator contort into an investigation report.
type Role string

const (
	// RoleTitle is the one-line heading, rendered FIRST wherever it was written.
	RoleTitle Role = "title"
	// RoleSection is above-the-fold content. Label is the agent's own name for
	// it, or empty for prose that carried no tag.
	RoleSection Role = "section"
	// RoleDetails is THE FOLD: collapsed by default on every surface.
	RoleDetails Role = "details"
)

// Block is one piece of a parsed message.
type Block struct {
	Role Role `json:"role"`
	// Label is the agent's own section name, present on named sections only.
	// Adapters render it GENERICALLY — an adapter carrying knowledge of any
	// particular agent's section names is the coupling this grammar avoids.
	Label string `json:"label,omitempty"`
	// Text is prose in the contract's markdown subset.
	Text string `json:"text"`
}

// reserved tag names, matched case-insensitively so `<Title>` is not a section
// called "Title" sitting where the heading should be.
const (
	tagTitle   = "title"
	tagDetails = "details"
)

// tagLine matches a line that is NOTHING BUT a tag.
//
// No leading whitespace is allowed and trailing whitespace is: indentation
// means the line is probably inside something (a list, a code sample), while
// trailing spaces are invisible and models emit them constantly. Rejecting a
// tag over a character nobody can see is the kind of strictness that reads as a
// bug.
var tagLine = regexp.MustCompile(`^(/?)([a-zA-Z][a-zA-Z0-9_-]*)>[ \t]*$`)

// parsedTag reports whether a line is a standalone block tag, and which.
func parsedTag(line string) (name string, closing bool, ok bool) {
	if !strings.HasPrefix(line, "<") {
		return "", false, false
	}
	m := tagLine.FindStringSubmatch(line[1:])
	if m == nil {
		return "", false, false
	}
	return m[2], m[1] == "/", true
}

// Parse turns an agent's reported output into blocks. It is TOTAL: every input
// yields blocks, and no input loses a character.
//
// Recognition requires all three of:
//
//   - the tag stands alone on its own line, at line start
//   - it forms a well-formed open/close pair
//   - it is outside fenced code
//
// Anything else is literal text, which is what keeps an OPEN vocabulary safe:
// `if x < y`, `Deployment<T>` and a shell redirect all fail the first condition,
// and a fenced block explaining this very grammar fails the third.
func Parse(s string) []Block {
	lines := strings.Split(s, "\n")
	fenced := fencedLines(lines)

	var (
		out      []Block
		prose    []string
		region   []string
		curName  string
		inRegion bool
	)

	// flushProse closes off untagged text as an unlabelled above-fold section.
	flushProse := func() {
		if t := strings.TrimSpace(strings.Join(prose, "\n")); t != "" {
			out = append(out, Block{Role: RoleSection, Text: t})
		}
		prose = nil
	}
	// closeRegion ends the open region, whatever ended it.
	closeRegion := func() {
		text := strings.TrimSpace(strings.Join(region, "\n"))
		switch {
		case strings.EqualFold(curName, tagTitle):
			out = append(out, Block{Role: RoleTitle, Text: oneLine(text)})
		case strings.EqualFold(curName, tagDetails):
			out = append(out, Block{Role: RoleDetails, Text: text})
		default:
			out = append(out, Block{Role: RoleSection, Label: curName, Text: text})
		}
		region, curName, inRegion = nil, "", false
	}

	for i, line := range lines {
		name, closing, isTag := parsedTag(line)
		if fenced[i] || !isTag {
			if inRegion {
				region = append(region, line)
			} else {
				prose = append(prose, line)
			}
			continue
		}
		switch {
		case inRegion && closing && strings.EqualFold(name, curName):
			closeRegion()
		case inRegion:
			// A TAG INSIDE AN OPEN REGION IS LITERAL. The model is flat, and a
			// model that forgot a close tag is far commoner than one nesting
			// deliberately — so the content stays where it was written rather
			// than being re-parented by a guess.
			region = append(region, line)
		case closing:
			// A close with no open never formed a pair: literal text.
			prose = append(prose, line)
		case hasCloser(lines, fenced, i+1, name):
			flushProse()
			curName, inRegion = name, true
		default:
			// UNPAIRED OPEN. The region runs to end of output rather than being
			// discarded — losing an agent's words to a grammar slip is the worst
			// failure available here.
			flushProse()
			curName, inRegion = name, true
		}
	}
	if inRegion {
		closeRegion()
	}
	flushProse()

	out = normalize(out)
	if len(out) == 0 {
		// Total means total: empty in, one empty block out, so every caller
		// downstream can assume at least one block rather than branching.
		out = []Block{{Role: RoleSection, Text: strings.TrimSpace(s)}}
	}
	return out
}

// hasCloser reports whether a matching close tag appears later, outside fences.
// Only TOP-LEVEL closers count, which is the same flatness the parse assumes.
func hasCloser(lines []string, fenced []bool, from int, name string) bool {
	for i := from; i < len(lines); i++ {
		if fenced[i] {
			continue
		}
		n, closing, ok := parsedTag(lines[i])
		if ok && closing && strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// fencedLines marks every line inside a ``` fence, INCLUDING the fence lines
// themselves. A fenced block is a machine document quoted verbatim, and a
// sample showing `<details>` must survive being about this grammar.
func fencedLines(lines []string) []bool {
	out := make([]bool, len(lines))
	open := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			out[i] = true
			open = !open
			continue
		}
		out[i] = open
	}
	return out
}

// oneLine collapses a title to a single line. A heading that wraps to three
// lines is not a heading, and every surface would have to cut it differently.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalize applies the ordering rules: the title FIRST wherever it was
// written, at most one, and the fold LAST. Named sections keep the order the
// agent wrote them in — with an open vocabulary the manager cannot tell which
// section is the conclusion, so reordering them would be a guess.
func normalize(in []Block) []Block {
	var (
		title  *Block
		mid    []Block
		folded []string
	)
	for _, b := range in {
		switch b.Role {
		case RoleTitle:
			if title == nil {
				t := b
				title = &t
				continue
			}
			// A SECOND TITLE IS A SECTION. Keeping its text under its own name
			// preserves every word without inventing a second heading no
			// surface knows where to put.
			b.Role, b.Label = RoleSection, tagTitle
			mid = append(mid, b)
		case RoleDetails:
			if b.Text != "" {
				folded = append(folded, b.Text)
			}
		default:
			if b.Text != "" || b.Label != "" {
				mid = append(mid, b)
			}
		}
	}
	var out []Block
	if title != nil {
		out = append(out, *title)
	}
	out = append(out, mid...)
	if len(folded) > 0 {
		// ONE FOLD. Several `<details>` regions are one folded region joined in
		// written order — "the fold" is singular on every surface, and two
		// disclosure controls in one message is a presentation nobody asked for.
		out = append(out, Block{Role: RoleDetails, Text: strings.Join(folded, "\n\n")})
	}
	return out
}
