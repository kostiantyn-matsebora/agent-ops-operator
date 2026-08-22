package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// The two failures this renderer exists to fix, pinned:
//
//  1. A message over 4096 characters used to FAIL the op outright — the long
//     payloads worth reading were exactly the ones that never arrived.
//  2. A payload containing `<`, `>` or `&` broke HTML parsing, so an alert
//     about a `<none>` image or an `a && b` command lost its own text.

func TestLongBodyIsSplitAndEveryPartFits(t *testing.T) {
	body := strings.TrimSpace(strings.Repeat("line of investigation output\n", 400)) // ~11k
	parts := splitHTML(renderMessage(newMenu(), Message{Kind: MsgAnswer, Body: body}))
	if len(parts) < 2 {
		t.Fatalf("a 10k body must split, got %d part(s)", len(parts))
	}
	joined := 0
	for i, p := range parts {
		if len(p) > telegramMessageLimit {
			t.Fatalf("part %d is %d bytes, over the Bot API limit", i, len(p))
		}
		if p == "" {
			t.Fatalf("part %d is empty — an empty sendMessage is rejected", i)
		}
		joined += strings.Count(p, "line of investigation output")
	}
	if joined != 400 {
		t.Fatalf("splitting lost content: %d of 400 lines survived", joined)
	}
}

func TestPayloadWithMarkupCharactersSurvives(t *testing.T) {
	payload := `image: <none>, cmd: a && b, cond: x > 1 && y < 2`
	out := renderMessage(newMenu(), Message{Kind: MsgSignal, Title: "Broken pod", Body: payload})
	if strings.Contains(out, "<none>") {
		t.Fatalf("unescaped payload would break HTML parsing: %q", out)
	}
	for _, want := range []string{"&lt;none&gt;", "a &amp;&amp; b", "x &gt; 1", "y &lt; 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("payload content lost or mis-escaped, want %q in %q", want, out)
		}
	}
}

func TestMarkdownSubsetRenders(t *testing.T) {
	out := markdownToHTML("**bold** and *italic* and `code` and [docs](https://x.test)")
	for _, want := range []string{"<b>bold</b>", "<i>italic</i>", "<code>code</code>",
		`<a href="https://x.test">docs</a>`} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in %q", want, out)
		}
	}
}

// A fenced block is lifted out BEFORE emphasis is applied, because `*` and `_`
// inside a shell snippet or a log line are not emphasis and turning them into
// tags corrupts the very thing being quoted.
func TestFencedCodeIsNotTreatedAsEmphasis(t *testing.T) {
	out := markdownToHTML("before\n```\nrm -rf *.log && echo *done*\n```\nafter")
	if strings.Contains(out, "<i>") || strings.Contains(out, "<b>") {
		t.Fatalf("emphasis applied inside a code fence: %q", out)
	}
	if !strings.Contains(out, "<pre>") || !strings.Contains(out, "rm -rf *.log &amp;&amp; echo *done*") {
		t.Fatalf("fenced block not preserved verbatim: %q", out)
	}
}

// INLINE code is lifted out for the same reason as a fence, and this is the
// message that proved it: an agent describing its own tools writes `*.md` and
// `mcp__kubernetes__*`, and those two stars used to pair with each other ACROSS
// the prose between them. That opened <i> inside one <code> and closed it inside
// the next, Telegram rejected the message whole, and the answer never arrived.
//
// The property is nesting, not the absence of emphasis: tags must close in the
// order they opened, so the check is that no <i> is opened inside code at all.
func TestStarsInsideInlineCodeDoNotPairAcrossIt(t *testing.T) {
	out := markdownToHTML("No agent file (`.claude/agents/*.md`) was found, " +
		"so I am limited to the `mcp__kubernetes__*` allowlist.")
	if strings.Contains(out, "<i>") {
		t.Fatalf("stars inside inline code became emphasis: %q", out)
	}
	for _, want := range []string{"<code>.claude/agents/*.md</code>", "<code>mcp__kubernetes__*</code>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inline code not preserved verbatim, want %q in %q", want, out)
		}
	}
	if err := wellNested(out); err != nil {
		t.Fatalf("%v in %q", err, out)
	}
}

// Emphasis SPANNING inline code is legitimate and must still nest correctly —
// the fix lifts code out of the way, it does not forbid a span around it.
func TestEmphasisAroundInlineCodeStaysNested(t *testing.T) {
	out := markdownToHTML("*use `kubectl get pods` first*")
	if !strings.Contains(out, "<i>use <code>kubectl get pods</code> first</i>") {
		t.Fatalf("emphasis around code lost its nesting: %q", out)
	}
	if err := wellNested(out); err != nil {
		t.Fatalf("%v in %q", err, out)
	}
}

// wellNested reports the first tag that closes out of order — the exact
// complaint Telegram makes ("expected </i>, found </code>") and the one thing a
// "contains the right substring" assertion cannot see.
func wellNested(html string) error {
	var stack []string
	tagRe := regexp.MustCompile(`</?([a-z]+)[^>]*>`)
	for _, m := range tagRe.FindAllStringSubmatch(html, -1) {
		name := m[1]
		if strings.HasPrefix(m[0], "</") {
			if len(stack) == 0 {
				return fmt.Errorf("closing </%s> with nothing open", name)
			}
			if top := stack[len(stack)-1]; top != name {
				return fmt.Errorf("expected </%s>, found </%s>", top, name)
			}
			stack = stack[:len(stack)-1]
			continue
		}
		if name == "br" {
			continue
		}
		stack = append(stack, name)
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed <%s>", stack[len(stack)-1])
	}
	return nil
}

func TestTopicNameFitsTelegramsLimit(t *testing.T) {
	long := TopicDescriptor{
		Conversation: "alert-abcde",
		Source:       "vm-alerts",
		Title:        strings.Repeat("HighMemoryUsage on a very chatty workload ", 8),
	}
	name := renderTopicName(long)
	if n := utf8.RuneCountInString(name); n > telegramTopicLimit {
		t.Fatalf("topic name is %d runes, over Telegram's %d", n, telegramTopicLimit)
	}
	if !strings.HasPrefix(name, "[vm-alerts] ") {
		t.Fatalf("the source is what makes a topic list scannable: %q", name)
	}

	// multi-byte input must not be cut mid-character — Telegram rejects that
	cyrillic := renderTopicName(TopicDescriptor{Title: strings.Repeat("почему", 60)})
	if !utf8.ValidString(cyrillic) {
		t.Fatalf("topic name cut mid-character: %q", cyrillic)
	}

	// an empty descriptor still names something addressable
	if got := renderTopicName(TopicDescriptor{Conversation: "conv-1"}); got != "conv-1" {
		t.Fatalf("fallback topic name: %q", got)
	}
}

// The pipeline is INFERRED by the manager and blank when ambiguous. A card must
// omit it rather than print an empty "via".
func TestSignalCardOmitsWhatItDoesNotKnow(t *testing.T) {
	out := renderSignal(Message{Kind: MsgSignal, Title: "DiskFull", Source: "vm-alerts", Body: "boom"})
	if strings.Contains(out, "via") {
		t.Fatalf("no pipeline was inferred, so none may be shown: %q", out)
	}
	if !strings.Contains(out, "source vm-alerts") {
		t.Fatalf("the source is known and must be shown: %q", out)
	}
	with := renderSignal(Message{Kind: MsgSignal, Source: "vm-alerts", Pipeline: "prod-oncall"})
	if !strings.Contains(with, "via prod-oncall") {
		t.Fatalf("an inferred pipeline must be shown: %q", with)
	}
}

// Labels render in a stable order: a card that reshuffles its own labels
// between renders reads as a different card.
func TestLabelsRenderInStableOrder(t *testing.T) {
	m := Message{Kind: MsgSignal, Labels: map[string]string{"namespace": "prod", "alertname": "DiskFull"}}
	first := renderSignal(m)
	for i := 0; i < 20; i++ {
		if renderSignal(m) != first {
			t.Fatal("label order is not stable across renders")
		}
	}
	if !strings.Contains(first, "alertname=DiskFull  namespace=prod") {
		t.Fatalf("labels: %q", first)
	}
}

// An unknown kind renders its prose rather than nothing: a manager that adds a
// fifth kind must degrade to plain text, not to an empty message.
func TestUnknownKindStillRendersItsBody(t *testing.T) {
	if got := renderMessage(newMenu(), Message{Kind: "something-new", Body: "still says this"}); got != "still says this" {
		t.Fatalf("unknown kind rendered %q", got)
	}
}
