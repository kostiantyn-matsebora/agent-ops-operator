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

// ---- tables --------------------------------------------------------------
//
// Telegram has no table markup. A GFM table arriving as raw pipes is thirty
// unreadable lines on a phone, so it becomes a preformatted block with the
// columns padded — the one place a chat client uses a monospaced font.

func TestTableBecomesAnAlignedPreBlock(t *testing.T) {
	md := "Here they are:\n\n| Name | Status | Age |\n|---|---|---|\n| agent-ops | Active | 40h |\n| b2-backup | Active | 188d |\n\ndone"
	out := renderMessage(newMenu(), Message{Kind: MsgAnswer, Body: md})

	if strings.Contains(out, "|---|") {
		t.Fatalf("the divider row reached the reader:\n%s", out)
	}
	if !strings.Contains(out, "<pre>") {
		t.Fatalf("table did not become a preformatted block:\n%s", out)
	}
	// Columns line up: the short name is padded to the width of the long one.
	if !strings.Contains(out, "agent-ops  Active") || !strings.Contains(out, "Name       Status") {
		t.Fatalf("columns are not aligned:\n%s", out)
	}
	// The prose around it is untouched.
	if !strings.Contains(out, "Here they are:") || !strings.Contains(out, "done") {
		t.Fatalf("surrounding prose lost:\n%s", out)
	}
}

// A lone pipe line is somebody's text, not a table.
func TestPipesWithoutADividerAreLeftAlone(t *testing.T) {
	out := renderMessage(newMenu(), Message{Kind: MsgAnswer, Body: "run a | b | c to see"})
	if strings.Contains(out, "<pre>") {
		t.Fatalf("ordinary pipes were treated as a table:\n%s", out)
	}
}

// Cell text is ESCAPED like everything else — a table is not a way to get
// markup past the renderer.
func TestTableCellsAreEscaped(t *testing.T) {
	md := "| Name | Note |\n|---|---|\n| a | <b>x</b> |"
	out := renderMessage(newMenu(), Message{Kind: MsgAnswer, Body: md})
	if strings.Contains(out, "<b>x</b>") {
		t.Fatalf("cell markup reached the transport:\n%s", out)
	}
}

// Width is counted in runes: a non-ASCII cell must not skew the columns after
// it.
func TestAlignmentCountsRunesNotBytes(t *testing.T) {
	md := "| Name | S |\n|---|---|\n| über | x |\n| ab | y |"
	out := renderMessage(newMenu(), Message{Kind: MsgAnswer, Body: md})
	if !strings.Contains(out, "über  x") {
		t.Fatalf("multi-byte cell padded wrong:\n%s", out)
	}
}

// ---- pointing at a new topic ---------------------------------------------
//
// Telegram cannot move somebody's client. The person who typed the command is
// left in the general surface with no sign anything happened, so a link is the
// closest a transport gets to taking them to the answer.

func TestTopicLink(t *testing.T) {
	for _, c := range []struct{ chat, want string }{
		{"-1001234567890", "https://t.me/c/1234567890/7"},
		// Not a supergroup, so there is no such link to build.
		{"123456", ""},
		{"-100", ""},
		{"", ""},
	} {
		if got := topicLink(c.chat, 7); got != c.want {
			t.Errorf("topicLink(%q) = %q, want %q", c.chat, got, c.want)
		}
	}
}

// ---- blocks ----------------------------------------------------------------

// blockMsg builds a message the way the manager now sends one: the agent's TEXT,
// tags and all. The adapter parses it — nothing upstream has.
func blockMsg(kind MessageKind, bs ...Block) Message {
	var b strings.Builder
	for _, blk := range bs {
		name := blk.Label
		switch blk.Role {
		case RoleTitle:
			name = "title"
		case RoleDetails:
			name = "details"
		}
		if name == "" {
			b.WriteString(blk.Text + "\n")
			continue
		}
		b.WriteString("<" + name + ">\n" + blk.Text + "\n</" + name + ">\n")
	}
	return Message{Kind: kind, Body: b.String()}
}

// The adapter renders what the manager parsed: title first, sections labelled
// and in order, the fold collapsed.
func TestRenderBlocks(t *testing.T) {
	expandableQuotes.Store(true)
	html := renderMessage(newMenu(), blockMsg(MsgAnswer,
		Block{Role: RoleTitle, Text: "Pod is looping"},
		Block{Role: RoleSection, Label: "root-cause", Text: "OOM at **512Mi**."},
		Block{Role: RoleSection, Label: "fix", Text: "Raise the limit."},
		Block{Role: RoleDetails, Text: "the long tail"},
	))
	for _, want := range []string{
		"<b>Pod is looping</b>",
		"<b>Root cause</b>",
		"<b>Fix</b>",
		"<blockquote expandable>the long tail</blockquote>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	// Order is the agent's and is never rearranged.
	if strings.Index(html, "Root cause") > strings.Index(html, "Fix") {
		t.Error("named sections must render in the order the agent wrote them")
	}
	if strings.Index(html, "Pod is looping") > strings.Index(html, "Root cause") {
		t.Error("the title leads")
	}
	// Inline markdown inside a block still renders.
	if !strings.Contains(html, "<b>512Mi</b>") {
		t.Errorf("inline markdown inside a block was not rendered:\n%s", html)
	}
}

// NO BLOCKS, NO CHANGE. This is the whole backward-compatibility story on this
// surface: a manager older than contract 3, or any manager-composed text.
func TestUnstructuredMessageRendersFromBody(t *testing.T) {
	m := Message{Kind: MsgAnswer, Body: "**done** — restarted `api`"}
	if got, want := renderMessage(newMenu(), m), markdownToHTML(m.Body); got != want {
		t.Fatalf("blockless answer changed:\n got  %q\n want %q", got, want)
	}
	notice := Message{Kind: MsgNotice, Body: "⚠️ nothing to do"}
	if !strings.Contains(renderMessage(newMenu(), notice), "nothing to do") {
		t.Fatal("a blockless notice must render its body")
	}
}

// A notice MAY carry blocks — a failed run that explained itself.
func TestNoticeWithBlocksIsFolded(t *testing.T) {
	expandableQuotes.Store(true)
	html := renderMessage(newMenu(), blockMsg(MsgNotice,
		Block{Role: RoleTitle, Text: "Could not continue"},
		Block{Role: RoleDetails, Text: "stack trace"},
	))
	if !strings.Contains(html, "<blockquote expandable>stack trace</blockquote>") {
		t.Fatalf("a failed run's explanation must fold like an answer:\n%s", html)
	}
}

// THE CONCLUSION LEADS THE FIRST CHUNK. Above-the-fold content is bounded by
// the manager, so it fits together and a reader who sees only one message still
// sees the answer.
func TestSplitPrefersBlockBoundaries(t *testing.T) {
	expandableQuotes.Store(true)
	chunks := renderChunks(newMenu(), blockMsg(MsgAnswer,
		Block{Role: RoleTitle, Text: "Big answer"},
		Block{Role: RoleSection, Label: "fix", Text: "restart it"},
		Block{Role: RoleDetails, Text: strings.Repeat("log line\n", 2000)},
	))
	if len(chunks) < 2 {
		t.Fatalf("a fold this long must split, got %d chunk(s)", len(chunks))
	}
	if !strings.Contains(chunks[0], "<b>Big answer</b>") || !strings.Contains(chunks[0], "<b>Fix</b>") {
		t.Fatalf("the first chunk must carry the title and the named sections:\n%s", chunks[0])
	}
	for i, c := range chunks {
		if len(c) > telegramMessageLimit {
			t.Fatalf("chunk %d is %d bytes, over the %d limit", i, len(c), telegramMessageLimit)
		}
		// Every piece of a split fold carries its own balanced quote tags, or
		// Telegram rejects the message outright.
		if got, want := strings.Count(c, "<blockquote"), strings.Count(c, "</blockquote>"); got != want {
			t.Fatalf("chunk %d has %d open and %d close quote tags", i, got, want)
		}
	}
}

// The degraded path is what a Bot API older than 7.2 gets. A plain blockquote
// is NOT collapsed, so the marker is what tells a reader the fold from the
// answer — without it the long tail simply arrives.
func TestQuoteDegradesWithAVisibleMarker(t *testing.T) {
	expandableQuotes.Store(false)
	defer expandableQuotes.Store(true)
	html := renderMessage(newMenu(), blockMsg(MsgAnswer, Block{Role: RoleDetails, Text: "tail"}))
	if strings.Contains(html, "expandable") {
		t.Fatalf("the latch did not take: %s", html)
	}
	if !strings.Contains(html, "<blockquote>") || !strings.Contains(html, "<b>Details</b>") {
		t.Fatalf("the degraded fold must still be marked:\n%s", html)
	}
}

// Only a refusal ABOUT the quote tag disables it. Any parse error disabling a
// working feature would be a silent downgrade nobody could explain later.
func TestOnlyQuoteErrorsLatchTheFeatureOff(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{fmt.Errorf(`telegram sendMessage: Bad Request: unsupported start tag "blockquote"`), true},
		{fmt.Errorf(`telegram sendMessage: Bad Request: can't parse entities: unsupported start tag "blockquote expandable"`), true},
		// The one that shipped broken: Telegram blamed the PRE, not the quote,
		// so a latch keyed on the word "blockquote" never fired and the message
		// retried forever.
		{fmt.Errorf(`telegram sendMessage: Bad Request: can't parse entities: Can't find end tag corresponding to start tag "pre"`), true},
		{fmt.Errorf(`telegram sendMessage: Too Many Requests`), false},
		{nil, false},
	} {
		if got := unsupportedQuote(tc.err); got != tc.want {
			t.Errorf("unsupportedQuote(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// THE BLOCK GRAMMAR IS NOT A WIRE VERSION.
//
// This adapter gained a parser and the contract did not move, because no field
// was added and none changed meaning: a body that was markdown is now markdown
// plus a grammar, read by the component that already read the markdown.
//
// A version bump would only mean something if the manager could serve the older
// shape, which would mean the manager parsing — the thing this design removed.
// Compatibility is `AgentProfile.spec.sharedOutputFormat`, which is off until an
// install whose adapters understand the grammar turns it on.
func TestTheGrammarIsNotAWireVersion(t *testing.T) {
	if ContractVersion != "2" {
		t.Fatalf("contract moved to %q — parsing a body needs no new version", ContractVersion)
	}
}

// THE ADAPTER PARSES. Nothing upstream did, and the body arrives with its tags.
func TestTheAdapterParsesTheBodyItself(t *testing.T) {
	raw := "<title>\nDisk filling\n</title>\n<fix>\nrotate the logs\n</fix>"
	got := renderMessage(newMenu(), Message{Kind: MsgAnswer, Body: raw})
	if !strings.Contains(got, "<b>Disk filling</b>") || !strings.Contains(got, "<b>Fix</b>") {
		t.Fatalf("the adapter did not parse the body it was given:\n%s", got)
	}
	if strings.Contains(got, "&lt;title&gt;") {
		t.Errorf("tags reached the transport as text:\n%s", got)
	}
}

// A RELAY IS SOMEBODY'S WORDS and is never parsed. A person asking why
// `<details>` will not render in their docs must see their own characters.
func TestARelayIsNotParsed(t *testing.T) {
	typed := "why won't\n<details>\nwork in my docs?\n</details>"
	got := renderMessage(newMenu(), Message{Kind: MsgRelay, Origin: "telegram", Sender: "kim", Body: typed})
	if !strings.Contains(got, "&lt;details&gt;") {
		t.Fatalf("a person's typed tags were consumed:\n%s", got)
	}
	if strings.Contains(got, "blockquote") {
		t.Error("a relay was folded")
	}
}

// A MARKDOWN LIST BECOMES BULLETS, because Telegram has no list markup.
//
// The agent writes `- item`, never the glyph: a surface that HAS lists renders
// a typed `•` as one running paragraph, which is exactly the wall of text the
// list was meant to prevent. Each adapter turns the real list into its own mark.
func TestMarkdownListsBecomeBullets(t *testing.T) {
	got := markdownToHTML("Findings:\n- Ready=True, restartCount 0\n- started at 07:40\n")
	for _, want := range []string{"• Ready=True, restartCount 0", "• started at 07:40"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// ITEMS ARE SEPARATED BY A BLANK LINE. Telegram sets messages tight, and
	// consecutive bullets run together into something that reads as a paragraph.
	if !strings.Contains(got, "• Ready=True, restartCount 0\n\n• started at 07:40") {
		t.Errorf("items are not separated by a blank line:\n%q", got)
	}
	// But the line INTRODUCING the list keeps its bullet tight beneath it —
	// a gap there detaches the list from what it belongs to.
	if strings.Contains(got, "Findings:\n\n• ") {
		t.Errorf("a gap was opened before the first item:\n%q", got)
	}
	if strings.Contains(got, "- Ready") {
		t.Errorf("the markdown marker survived into the output:\n%s", got)
	}
	// A hyphen that is not a list marker is left alone.
	if plain := markdownToHTML("read-only access, 3 - 1 = 2"); strings.Contains(plain, "•") {
		t.Errorf("a mid-line hyphen became a bullet: %s", plain)
	}
	// Indentation is kept, so a nested list still reads as nested.
	if nested := markdownToHTML("- top\n  - under"); !strings.Contains(nested, "  • under") {
		t.Errorf("nesting lost:\n%s", nested)
	}
}

// A TAGGED FENCE IS HIGHLIGHTED. Telegram renders
// `<pre><code class="language-X">` with syntax colouring, and the tag is the
// difference between a readable payload and a wall of JSON.
func TestFencedCodeCarriesItsLanguage(t *testing.T) {
	got := markdownToHTML("```json\n{\"a\": 1}\n```")
	if !strings.Contains(got, `<pre><code class="language-json">`) {
		t.Errorf("language lost:\n%s", got)
	}
	if !strings.Contains(got, "{&quot;a&quot;: 1}") && !strings.Contains(got, `{"a": 1}`) {
		t.Errorf("body mangled:\n%s", got)
	}
	// An UNTAGGED fence stays a plain pre — unchanged from before.
	if plain := markdownToHTML("```\nraw\n```"); plain != "<pre>raw</pre>" {
		t.Errorf("untagged fence changed: %q", plain)
	}
	// Case is normalised, so `JSON` and `json` are one thing.
	if up := markdownToHTML("```JSON\n{}\n```"); !strings.Contains(up, `class="language-json"`) {
		t.Errorf("language not lowercased:\n%s", up)
	}
	// A language Telegram does not know is still tagged — it renders the block
	// unhighlighted rather than failing.
	if odd := markdownToHTML("```c#\nvar x = 1;\n```"); !strings.Contains(odd, `class="language-c#"`) {
		t.Errorf("unusual language dropped:\n%s", odd)
	}
	// Escaping is unchanged: a payload full of angle brackets is still safe.
	if esc := markdownToHTML("```xml\n<a href=\"x\">&\n```"); strings.Contains(esc, "<a href=") {
		t.Errorf("fence body was not escaped:\n%s", esc)
	}
}

// A TALL SIGNAL PAYLOAD IS FOLDED. It is the evidence behind the card, and
// unfolded it is the tallest thing in the thread.
func TestSignalPayloadFoldsWhenTall(t *testing.T) {
	expandableQuotes.Store(true)
	tall := Message{Kind: MsgSignal, Title: "Unhealthy: share-manager",
		Body: strings.Repeat("\"field\": \"value\",\n", 12)}
	got := renderSignal(tall)
	// FOLDED, AND WITHOUT A NESTED `<pre>`. Telegram rejects that nesting with
	// `Can't find end tag corresponding to start tag "pre"` and drops the whole
	// message — it shipped, and retried in a loop.
	if !strings.Contains(got, "<blockquote expandable>") {
		t.Fatalf("a tall payload must be folded:\n%s", got)
	}
	if strings.Contains(got, "<blockquote expandable><pre>") {
		t.Fatalf("pre nested in a quote — the Bot API refuses this:\n%s", got)
	}
	// The card's own heading stays OUTSIDE the fold — folding the thing that
	// says what happened would leave a collapsed stub saying nothing.
	if strings.Index(got, "Unhealthy: share-manager") > strings.Index(got, "<blockquote") {
		t.Error("the title was folded with the payload")
	}

	// A SHORT one stays open: a tap to read three lines that would have fitted
	// is worse than showing them.
	short := Message{Kind: MsgSignal, Title: "t", Body: "reason: Unhealthy\ncount: 1"}
	if got := renderSignal(short); strings.Contains(got, "blockquote") {
		t.Errorf("a short payload must not be folded:\n%s", got)
	}
}

// A SIGNAL CARD'S FOLDED PAYLOAD MUST NOT BE CUT IN HALF.
//
// This is the failure that reached a live install: the Bot API answered
// `can't parse entities: Can't find end tag corresponding to start tag
// "blockquote"` and the op retried in a loop, because the quote latch only
// recognises refusals about an UNSUPPORTED tag, not an unbalanced one.
//
// `payloadBlock` folds a tall payload into `<blockquote expandable>`, which
// spans many lines — and `splitHTML` chose its cut at a newline on the stated
// assumption that "every tag we emit is opened and closed on the same line".
// That assumption was false for exactly this tag, on exactly the path that has
// no block splitter to rescue it: a signal is not agent output, so it never
// reaches `splitOversizeBlock`.
func TestSignalCardSplitsWithBalancedQuoteTags(t *testing.T) {
	expandableQuotes.Store(true)
	// Tall enough to fold, long enough to split, under documentThreshold so it
	// stays a message rather than becoming an upload.
	payload := strings.Repeat("2026-08-24T11:57:42Z coordinator timeout\n", 150)
	chunks := renderChunks(newMenu(), Message{
		Kind:   MsgSignal,
		Title:  "asusrouter — transient timeout",
		Source: "ha-logs",
		Body:   payload,
	})
	if len(chunks) < 2 {
		t.Fatalf("a payload this long must split, got %d chunk(s)", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > telegramMessageLimit {
			t.Errorf("chunk %d is %d bytes, over the %d limit", i, len(c), telegramMessageLimit)
		}
		if got, want := strings.Count(c, "<blockquote"), strings.Count(c, "</blockquote>"); got != want {
			t.Errorf("chunk %d has %d open and %d close quote tags — Telegram rejects the whole message:\n%.200s",
				i, got, want, c)
		}
	}
}

// The same cut, one kind over: a relay or an unknown kind goes through
// splitHTML too, and anything that folds there has the same tag spanning lines.
func TestSplitHTMLClosesAndReopensAQuoteItCuts(t *testing.T) {
	body := "<blockquote expandable>" + strings.Repeat("line\n", 2000) + "</blockquote>"
	chunks := splitHTML(body)
	if len(chunks) < 2 {
		t.Fatalf("expected a split, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > telegramMessageLimit {
			t.Errorf("chunk %d is %d bytes, over the %d limit", i, len(c), telegramMessageLimit)
		}
		if got, want := strings.Count(c, "<blockquote"), strings.Count(c, "</blockquote>"); got != want {
			t.Errorf("chunk %d has %d open and %d close quote tags", i, got, want)
		}
	}
}
