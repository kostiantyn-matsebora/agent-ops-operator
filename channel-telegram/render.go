package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
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
	fencedRe = regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\n?(.*?)```")
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe = regexp.MustCompile(`(^|[^*\w])\*([^*\n]+)\*`)
	codeRe   = regexp.MustCompile("`([^`\n]+)`")
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

// markdownToHTML converts the contract's markdown subset to Telegram HTML.
//
// Order matters and is the whole trick: fenced blocks are lifted out FIRST and
// restored last, so a `*` or `_` inside a code block is not read as emphasis —
// which is exactly where they appear in log lines and shell snippets. Escaping
// happens per segment, before any tag is introduced, so a `<` in the prose can
// never be confused with markup we generated.
func markdownToHTML(md string) string {
	var blocks []string
	// lift fenced code out of the way
	withoutFences := fencedRe.ReplaceAllStringFunc(md, func(m string) string {
		inner := fencedRe.FindStringSubmatch(m)[1]
		blocks = append(blocks, "<pre>"+escape(strings.TrimRight(inner, "\n"))+"</pre>")
		return fmt.Sprintf("\x00%d\x00", len(blocks)-1)
	})

	out := escape(withoutFences)
	out = codeRe.ReplaceAllString(out, "<code>$1</code>")
	out = boldRe.ReplaceAllString(out, "<b>$1</b>")
	out = italicRe.ReplaceAllString(out, "$1<i>$2</i>")
	out = linkRe.ReplaceAllString(out, `<a href="$2">$1</a>`)

	for i, b := range blocks {
		out = strings.ReplaceAll(out, fmt.Sprintf("\x00%d\x00", i), b)
	}
	return out
}

// renderMessage turns one semantic message into Telegram HTML.
//
// An unknown kind renders its body rather than nothing: a manager that adds a
// fifth kind should degrade to plain prose here, not silently post an empty
// message.
func renderMessage(m Message) string {
	switch m.Kind {
	case MsgSignal:
		return renderSignal(m)
	case MsgRelay:
		who := m.Origin
		if m.Sender != "" {
			who = m.Origin + "/" + m.Sender
		}
		return "💬 <b>" + escape(who) + "</b>: " + markdownToHTML(m.Body)
	case MsgAnswer:
		return markdownToHTML(m.Body)
	default:
		return markdownToHTML(m.Body)
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
		b.WriteString("\n<pre>" + escape(body) + "</pre>")
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
	if len(s) <= telegramMessageLimit {
		return []string{s}
	}
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
