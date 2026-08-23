package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
)

// Rendering is the ADAPTER'S job, and this file is all of it for Telegram.
//
// The manager sends meaning — a signal, an answer, a relay, a notice — with
// prose in a small markdown subset (**bold**, *italic*, `code`, ```fenced```,
// [text](url)). Everything Telegram-shaped happens here: HTML composition,
// entity escaping, the 4096-character message limit, and the 128-character
// forum-topic limit. None of it is knowable by the manager, which serves
// surfaces that share none of those constraints.

// telegramMessageLimit is the Bot API cap on a sendMessage text. Exceeding it
// used to FAIL the op — a long alert payload simply never arrived — so splitting
// is the difference between delivery and silence, not a nicety.
const telegramMessageLimit = 4096

// telegramTopicLimit is the cap on a forum topic name.
const telegramTopicLimit = 128

// escape makes text safe inside Telegram HTML. Applied to EVERY interpolated
// value: an alert payload containing `<`, `>` or `&` broke parsing before, which
// meant the message with the actual incident in it was the one that failed.
func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

var (
	// The language is CAPTURED, not discarded: Telegram highlights a fenced
	// block tagged `<pre><code class="language-X">`, and the tag is the only
	// thing that turns a wall of JSON into something readable. The charset is
	// deliberately narrow so the captured value is safe in an attribute without
	// escaping — anything outside it is not a language name.
	fencedRe = regexp.MustCompile("(?s)```([a-zA-Z0-9_+#-]*)\n?(.*?)```")
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe = regexp.MustCompile(`(^|[^*\w])\*([^*\n]+)\*`)
	codeRe   = regexp.MustCompile("`([^`\n]+)`")
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	// A markdown list item: `- ` or `* ` at line start, with any indent kept so
	// a nested list still reads as nested.
	listRe = regexp.MustCompile(`(?m)^([ \t]*)[-*] `)
)

// markdownToHTML converts the contract's markdown subset to Telegram HTML.
//
// Order matters and is the whole trick: CODE IS LIFTED OUT FIRST — fenced AND
// inline — and restored last, so a `*` inside it is not read as emphasis. That
// is exactly where stars appear: glob patterns, tool allowlists, log lines.
// Escaping happens per segment, before any tag is introduced, so a `<` in the
// prose can never be confused with markup we generated.
//
// Inline code used to be converted in place, BEFORE emphasis rather than out of
// its way, and the emphasis regexes then ran over the tags it had just written.
// One star inside `*.md` and another inside `mcp__kubernetes__*` paired with
// each other across the prose between them, opening <i> inside one <code> and
// closing it inside the next. Telegram rejects the whole message for that
// ("Unmatched end tag ... expected </i>, found </code>"), so the answer never
// arrived — an agent describing its own tools could not report to a chat.
func markdownToHTML(md string) string {
	var blocks []string
	// stash swaps rendered HTML for a placeholder no markdown rule can match,
	// so everything after it sees prose only.
	stash := func(html string) string {
		blocks = append(blocks, html)
		return fmt.Sprintf("\x00%d\x00", len(blocks)-1)
	}

	withoutFences := fencedRe.ReplaceAllStringFunc(md, func(m string) string {
		g := fencedRe.FindStringSubmatch(m)
		lang, body := g[1], escape(strings.TrimRight(g[2], "\n"))
		if lang == "" {
			return stash("<pre>" + body + "</pre>")
		}
		// An unknown language is not an error: Telegram renders the block
		// unhighlighted, which is exactly what an untagged fence gets.
		return stash(`<pre><code class="language-` + strings.ToLower(lang) + `">` + body + "</code></pre>")
	})

	// TABLES BECOME A MONOSPACED BLOCK. Telegram has no table markup at all, so
	// the choice is a preformatted block with the columns padded, or thirty
	// lines of raw pipes — which is what a reader got before, and it is
	// unreadable on a phone.
	//
	// Stashed like a fence, for the same reason: nothing after this should see
	// the cell text as prose to re-mark-up.
	withoutFences = renderTables(withoutFences, stash)

	// MARKDOWN LISTS BECOME SPACED BULLETS. Telegram HTML has no list markup, so
	// the closest thing is a bullet glyph per line — and the agent must write a
	// real markdown list rather than typing the glyph itself, or a surface that
	// DOES have lists (the console) renders one running paragraph.
	withoutFences = bulletList(withoutFences)

	out := escape(withoutFences)
	out = codeRe.ReplaceAllStringFunc(out, func(m string) string {
		return stash("<code>" + codeRe.FindStringSubmatch(m)[1] + "</code>")
	})
	out = boldRe.ReplaceAllString(out, "<b>$1</b>")
	out = italicRe.ReplaceAllString(out, "$1<i>$2</i>")
	out = linkRe.ReplaceAllString(out, `<a href="$2">$1</a>`)

	for i, b := range blocks {
		out = strings.ReplaceAll(out, fmt.Sprintf("\x00%d\x00", i), b)
	}
	return out
}

// tableRowRe matches one pipe-delimited row of a GFM table.
var tableRowRe = regexp.MustCompile(`^\s*\|.*\|\s*$`)

// tableDividerRe matches the |---|---| line that makes the rows above a HEADER.
var tableDividerRe = regexp.MustCompile(`^\s*\|[\s:|-]+\|\s*$`)

// renderTables replaces every GFM table with a preformatted block whose columns
// line up.
//
// Telegram supports no table markup, so this is the closest a chat surface
// gets: <pre> is the one place a proportional font is not used, which is what
// makes columns readable at all.
//
// Anything that is not a table is returned untouched, line for line.
func renderTables(md string, stash func(string) string) string {
	lines := strings.Split(md, "\n")
	var out []string
	for i := 0; i < len(lines); {
		// A table is two or more pipe rows whose SECOND line is a divider.
		if i+1 < len(lines) && tableRowRe.MatchString(lines[i]) && tableDividerRe.MatchString(lines[i+1]) {
			end := i + 2
			for end < len(lines) && tableRowRe.MatchString(lines[end]) {
				end++
			}
			rows := make([][]string, 0, end-i-1)
			for j := i; j < end; j++ {
				if j == i+1 {
					continue // the divider carries no data
				}
				rows = append(rows, splitRow(lines[j]))
			}
			out = append(out, stash("<pre>"+escape(alignColumns(rows))+"</pre>"))
			i = end
			continue
		}
		out = append(out, lines[i])
		i++
	}
	return strings.Join(out, "\n")
}

// splitRow turns `| a | b |` into the cells between the pipes.
func splitRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	cells := strings.Split(trimmed, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// alignColumns pads every cell to its column's widest, so the block reads as a
// table rather than as ragged text.
//
// Width is counted in RUNES, not bytes: a namespace with a non-ASCII character
// in it would otherwise pad short and skew every column after it.
func alignColumns(rows [][]string) string {
	widest := map[int]int{}
	for _, r := range rows {
		for i, c := range r {
			if n := len([]rune(c)); n > widest[i] {
				widest[i] = n
			}
		}
	}
	var b strings.Builder
	for ri, r := range rows {
		if ri > 0 {
			b.WriteByte('\n')
		}
		for i, c := range r {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(c)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widest[i]-len([]rune(c))))
			}
		}
	}
	return b.String()
}

// ---- blocks ----------------------------------------------------------------
//
// The manager parses agent output into blocks and this renders them. NO PARSING
// HAPPENS HERE, and none should ever be added: the grammar is meaning, which is
// the manager's half of the contract, and an adapter recognising tags would be
// the second parser this shape exists to prevent.
//
// What arrives is already structured, so the only Telegram-shaped decisions
// left are the ones this file exists for — which tag, how long, where to split.

// expandableQuotes is whether this Bot API supports `<blockquote expandable>`,
// added in Bot API 7.2. It LATCHES OFF the first time a send is refused for it.
//
// A version probe is not available — getMe reports nothing about the API
// version, and a self-hosted local Bot API server may be any age — so the only
// honest test is sending one and reading the refusal. The degraded form is a
// plain <blockquote> with a visible marker, which is Bot API 7.0, and below
// that the quote tags themselves would be refused and the same latch applies.
var expandableQuotes atomic.Bool

func init() { expandableQuotes.Store(true) }

// unsupportedQuote reports whether a send failed BECAUSE of the quote tag, as
// opposed to any other parse error — which must not silently disable a feature
// that was working.
func unsupportedQuote(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "can't parse entities") && !strings.Contains(msg, "unsupported") {
		return false
	}
	// NAMING `blockquote` IS NOT REQUIRED, and assuming it was is what let a
	// broken message retry in a loop: Telegram refused a `<pre>` nested in a
	// quote by complaining about the PRE, so the latch never fired. Any parse
	// refusal on a message we quoted is treated as the quote's fault — the
	// degraded form is always renderable, so a false positive costs a font and
	// a false negative costs the message.
	return strings.Contains(msg, "blockquote") ||
		strings.Contains(msg, "expandable") ||
		strings.Contains(msg, "start tag")
}

// sectionLabel turns an agent's own tag name into something a person reads.
// GENERIC, and deliberately so: this adapter carries no knowledge of any
// particular agent's section names, only of how to present a name it is given.
func sectionLabel(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// renderBlock is one block as Telegram HTML.
func renderBlock(b Block) string {
	switch b.Role {
	case RoleTitle:
		if b.Text == "" {
			return ""
		}
		return "<b>" + markdownToHTML(b.Text) + "</b>"
	case RoleDetails:
		if b.Text == "" {
			return ""
		}
		return quote(markdownToHTML(b.Text))
	default:
		body := markdownToHTML(b.Text)
		if lbl := sectionLabel(b.Label); lbl != "" {
			if body == "" {
				return "<b>" + escape(lbl) + "</b>"
			}
			return "<b>" + escape(lbl) + "</b>\n" + body
		}
		return body
	}
}

// quote wraps the fold in the transport's own collapsed presentation.
//
// The marker on the degraded path is not decoration: a plain <blockquote> is
// NOT collapsed, so without it a reader has no way to tell the fold from the
// answer and the long tail simply arrives, which is the state this whole change
// exists to end.
func quote(inner string) string {
	if expandableQuotes.Load() {
		return "<blockquote expandable>" + inner + "</blockquote>"
	}
	return "<blockquote><b>Details</b>\n" + inner + "</blockquote>"
}

// agentBlocks parses a message's body, for the kinds whose body an AGENT wrote.
//
// ANSWER AND NOTICE ONLY, and the exclusions are the point:
//
//   - a RELAY is somebody's typed words. Parsing those would consume characters
//     a person deliberately wrote — somebody asking why `<details>` will not
//     render in their docs must see their own text arrive.
//   - a SIGNAL is not prose at all. Its structured fields are the message and
//     renderSignal builds a card from them.
func agentBlocks(m Message) []Block {
	switch m.Kind {
	case MsgAnswer, MsgNotice:
		return Parse(m.Body)
	}
	return nil
}

// renderBlocks renders a message's blocks in order, returning ONE STRING PER
// BLOCK so the splitter can prefer block boundaries.
func renderBlocks(bs []Block) []string {
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		if r := renderBlock(b); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// splitBlocks packs rendered blocks into sendable chunks, breaking at BLOCK
// boundaries wherever it can.
//
// THE FIRST CHUNK CARRIES THE ABOVE-FOLD CONTENT. That is what makes a split
// message still readable: the manager bounds title plus named sections well
// under one message, so they fit together and a reader who sees only the first
// message still sees the conclusion.
//
// A single block too large for one message is split inside itself — and if it
// is the fold, each piece is re-wrapped in its own quote, because a chunk
// boundary inside <blockquote> would send unbalanced tags.
func splitBlocks(rendered []string) []string {
	var out []string
	var cur string
	flush := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	for _, b := range rendered {
		if len(b) > telegramMessageLimit {
			flush()
			out = append(out, splitOversizeBlock(b)...)
			continue
		}
		switch {
		case cur == "":
			cur = b
		case len(cur)+2+len(b) <= telegramMessageLimit:
			cur += "\n\n" + b
		default:
			flush()
			cur = b
		}
	}
	flush()
	if len(out) == 0 {
		return nil
	}
	return out
}

// splitOversizeBlock splits one block that cannot fit in a single message,
// preserving the quote wrapper across the pieces when it has one.
func splitOversizeBlock(b string) []string {
	const open, openAlt, close = "<blockquote expandable>", "<blockquote>", "</blockquote>"
	prefix := ""
	switch {
	case strings.HasPrefix(b, open) && strings.HasSuffix(b, close):
		prefix = open
	case strings.HasPrefix(b, openAlt) && strings.HasSuffix(b, close):
		prefix = openAlt
	}
	if prefix == "" {
		return splitHTML(b)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(b, prefix), close)
	// Room for the wrapper on every piece, or the re-wrapped chunk overflows.
	pieces := splitHTMLTo(inner, telegramMessageLimit-len(prefix)-len(close))
	out := make([]string, 0, len(pieces))
	for _, p := range pieces {
		out = append(out, prefix+p+close)
	}
	return out
}

// renderMessage turns one semantic message into Telegram HTML.
//
// An unknown kind renders its body rather than nothing: a manager that adds a
// fifth kind should degrade to plain prose here, not silently post an empty
// message.
func renderMessage(mn *menu, m Message) string {
	switch m.Kind {
	case MsgSignal:
		return renderSignal(m)
	case MsgRelay:
		who := m.Origin
		if m.Sender != "" {
			who = m.Origin + "/" + m.Sender
		}
		// A relayed message is somebody's OWN words. Never rewritten.
		return "💬 <b>" + escape(who) + "</b>: " + markdownToHTML(m.Body)
	case MsgAnswer:
		if r := renderBlocks(agentBlocks(m)); len(r) > 0 {
			return strings.Join(r, "\n\n")
		}
		// No blocks: a message from a manager that predates them, or output
		// with nothing to structure. Renders exactly as it always has.
		return markdownToHTML(m.Body)
	default:
		// A NOTICE MAY CARRY THE GRAMMAR — a failed run that explained itself
		// leaves as one, and that explanation is the longest thing an agent
		// produces. Manager-composed prose parses to one block, same result.
		if r := renderBlocks(agentBlocks(m)); len(r) > 0 {
			return mn.rewriteCommands(strings.Join(r, "\n\n"))
		}
		// ONE SPELLING PER SURFACE. The manager names a Pipeline as it is
		// published; the composer completes the spelling Telegram accepts.
		// Showing both would be two strings for one thing, so what the manager
		// says is rendered in the form the menu offers.
		return mn.rewriteCommands(markdownToHTML(m.Body))
	}
}

// renderSignal composes the event card: a heading naming the route, the labels
// as a compact block, then the payload in a code block because it is a machine
// document and proportional text mangles it.
func renderSignal(m Message) string {
	var b strings.Builder
	head := m.Title
	if head == "" {
		head = "Signal"
	}
	b.WriteString("📣 <b>" + escape(head) + "</b>")

	// Attribution is omitted when absent rather than rendered as "unknown": the
	// pipeline is INFERRED by the manager and blank means "not determinable",
	// which is a fact about the wiring, not something to announce in chat.
	var from []string
	if m.Source != "" {
		from = append(from, "source "+escape(m.Source))
	}
	if m.Pipeline != "" {
		from = append(from, "via "+escape(m.Pipeline))
	}
	if len(from) > 0 {
		b.WriteString("\n<i>" + strings.Join(from, " · ") + "</i>")
	}
	if len(m.Labels) > 0 {
		keys := make([]string, 0, len(m.Labels))
		for k := range m.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys) // stable order: a card that reshuffles reads as new
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, escape(k)+"="+escape(m.Labels[k]))
		}
		b.WriteString("\n<code>" + strings.Join(parts, "  ") + "</code>")
	}
	if body := strings.TrimSpace(m.Body); body != "" {
		b.WriteString("\n" + payloadBlock(body))
	}
	if m.InputRef != "" {
		b.WriteString("\n<i>full event: conversationinput/" + escape(m.InputRef) + "</i>")
	}
	return b.String()
}

// renderTopicName names a forum topic from the descriptor, within Telegram's
// 128-character limit.
//
// The lane prefix and the source come first because they are what makes a list
// of topics scannable; the title is what gets cut when something must be. Plain
// text, never HTML — Telegram takes no markup in a topic name.
func renderTopicName(t TopicDescriptor) string {
	name := strings.TrimSpace(t.Title)
	if t.Source != "" {
		name = "[" + t.Source + "] " + name
	}
	if name == "" {
		name = t.Conversation
	}
	return truncateRunes(name, telegramTopicLimit)
}

// truncateRunes cuts to n RUNES, not bytes — a byte cut would split a
// multi-byte character and Telegram rejects the result.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// splitHTML breaks rendered HTML into sendable chunks under the Bot API limit.
//
// Splitting prefers a paragraph break, then a line break, then a hard cut, so a
// long investigation arrives as readable pieces instead of failing whole. Tags
// are not balanced across a split — a chunk boundary is chosen at a newline,
// which in this renderer never falls inside a tag, because every tag we emit is
// opened and closed on the same line except <pre>, handled by keeping its
// content intact where it fits.
func splitHTML(s string) []string {
	return splitHTMLTo(s, telegramMessageLimit)
}

// splitHTMLTo is splitHTML against a caller-supplied budget, which the block
// splitter needs because a re-wrapped fold chunk must leave room for its own
// quote tags.
func splitHTMLTo(s string, limit int) []string {
	if limit <= 0 {
		return []string{s}
	}
	if len(s) <= limit {
		return []string{s}
	}
	telegramMessageLimit := limit // shadowed so the body below reads unchanged
	var out []string
	rest := s
	for len(rest) > telegramMessageLimit {
		cut := strings.LastIndex(rest[:telegramMessageLimit], "\n\n")
		if cut <= 0 {
			cut = strings.LastIndex(rest[:telegramMessageLimit], "\n")
		}
		if cut <= 0 {
			// no break to honour: cut on a rune boundary rather than mid-character
			cut = telegramMessageLimit
			for cut > 0 && !utf8Boundary(rest, cut) {
				cut--
			}
		}
		out = append(out, strings.TrimRight(rest[:cut], "\n"))
		rest = strings.TrimLeft(rest[cut:], "\n")
	}
	if rest != "" {
		out = append(out, rest)
	}
	return out
}

// utf8Boundary reports whether i starts a rune in s.
func utf8Boundary(s string, i int) bool {
	return i >= len(s) || s[i]&0xC0 != 0x80
}

// documentThreshold is when a signal payload stops being readable as chat.
// Deliberately well above the message limit rather than at it: two or three
// chunks still read fine in a thread, a dozen does not.
const documentThreshold = 3 * telegramMessageLimit

// asDocument decides whether a signal's payload should be attached instead of
// posted, and returns the caption (the card WITHOUT the payload) plus the raw
// payload to upload.
//
// Only `signal` qualifies. An agent's answer is prose written to be read in the
// thread — attaching it would hide the deliverable behind a download — while a
// signal payload is a machine document that is already being quoted verbatim.
func asDocument(m Message) (caption, content string, ok bool) {
	if m.Kind != MsgSignal || len(m.Body) <= documentThreshold {
		return "", "", false
	}
	head := m
	head.Body = "" // the card, minus the payload it is about to carry as a file
	caption = renderSignal(head)
	if len(caption) > 1024 { // Telegram's caption limit
		caption = truncateRunes(caption, 1024)
	}
	return caption, m.Body, true
}

// documentName names the attachment after what it is about, so a thread full of
// them stays navigable.
func documentName(m Message) string {
	base := m.Source
	if base == "" {
		base = "signal"
	}
	return base + ".txt"
}

// renderChunks is the ONE entry point for turning a message into sendable
// pieces: blocks split at block boundaries, everything else at a paragraph.
//
// One function because the two used to be one call to splitHTML, and a second
// call site that forgot the block path would post an answer as a wall again.
func renderChunks(mn *menu, m Message) []string {
	if r := renderBlocks(agentBlocks(m)); len(r) > 0 {
		if m.Kind == MsgNotice {
			for i := range r {
				r[i] = mn.rewriteCommands(r[i])
			}
		}
		if chunks := splitBlocks(r); len(chunks) > 0 {
			return chunks
		}
	}
	return splitHTML(renderMessage(mn, m))
}

// degradeQuotes rewrites an already-rendered message for a Bot API with no
// expandable quote, so the retry after the latch does not need the message
// composed a second time.
func degradeQuotes(html string) string {
	return strings.ReplaceAll(html, "<blockquote expandable>", "<blockquote><b>Details</b>\n")
}

// bulletList turns markdown list markers into bullets and SEPARATES THE ITEMS
// WITH A BLANK LINE.
//
// Telegram sets messages tight, so consecutive bullet lines run together into a
// grey block that reads as a paragraph — the thing a list exists to avoid. A
// blank line between items is the only spacing control the transport offers.
//
// Fenced code is already stashed behind placeholders by the time this runs, so
// a list-looking line inside a code sample is not reachable here.
func bulletList(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	prevItem := false
	for _, l := range lines {
		m := listRe.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			prevItem = false
			continue
		}
		// Between two items only: never before the first, which would open the
		// list with a gap under whatever introduced it.
		if prevItem {
			out = append(out, "")
		}
		out = append(out, m[1]+"• "+l[len(m[0]):])
		prevItem = true
	}
	return strings.Join(out, "\n")
}

// foldPayloadOver is when a signal payload stops being something to read at a
// glance and becomes something to open. Counted in LINES rather than
// characters: a payload is a machine document laid out one field per line, and
// what makes it dominate a thread is its height.
const foldPayloadOver = 6

// payloadBlock renders a signal's payload, FOLDED once it is tall enough.
//
// An event card exists to say what happened. The payload is the evidence behind
// it, and unfolded it is the tallest thing in the thread — the same argument
// `<details>` makes for an agent's answer, one message kind over.
//
// SHORT PAYLOADS STAY OPEN. A three-line event behind a control costs a tap to
// read something that would have fitted, and a thread of collapsed stubs tells
// a reader nothing.
func payloadBlock(body string) string {
	if strings.Count(body, "\n") < foldPayloadOver {
		// Short enough to read at a glance: monospaced, and open.
		return "<pre>" + escape(body) + "</pre>"
	}
	// TALL PAYLOADS FOLD, AND LOSE THE MONOSPACE TO DO IT.
	//
	// `<pre>` INSIDE `<blockquote>` IS REJECTED by the Bot API — it answers
	// `can't parse entities: Can't find end tag corresponding to start tag
	// "pre"`, and the whole message fails. That shipped, and it retried in a
	// loop because the quote latch only recognises refusals naming
	// `blockquote`.
	//
	// So the fold wins over the font. A thirty-line event document collapsed
	// into one line a reader can open is worth more than proportional-vs-
	// monospaced, and the alternative is the tallest thing in the thread staying
	// open forever.
	return quote(escape(body))
}
