package main

import (
	"context"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// THE MENU IS THE ENTRY POINT.
//
// Registering commands makes Telegram render its own control in the composer —
// a permanent affordance that costs no message and does not scroll away — and
// complete what a person types. Without it a chat surface is a plain text box
// that says nothing about `/` existing at all.
//
// Everything Telegram can express is registered: the built-in commands AND the
// addressable Pipelines. A menu listing half of what may be typed teaches that
// the other half does not exist.

// telegramCommandName is Telegram's own rule for a command name: 1-32
// characters of lowercase letters, digits and underscores. It lives HERE and
// nowhere else — the manager publishes names unchanged, and a rule that suits
// one transport must never travel back into the contract.
var telegramCommandName = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// telegramMaxCommands is Telegram's cap on one scope's command list.
const telegramMaxCommands = 100

// telegramMaxDescription is Telegram's cap on a command description.
const telegramMaxDescription = 256

// spellForTelegram renders a published name as a Telegram command name, and
// reports whether Telegram can express it at all.
//
// INJECTIVE BY CONSTRUCTION. A Kubernetes object name is a DNS-1123 subdomain:
// lowercase alphanumerics, `-` and `.`, and NO underscore. So `-` -> `_`
// introduces a character no published name can contain, `_` -> `-` reverses it
// exactly, and two Pipelines can never share a spelling.
//
// That is why signal-telegram can reverse this with one line and no shared
// state — which it must, because the menu-completed command arrives THERE, and
// a signal adapter cannot read a channel endpoint at all.
//
// A name Telegram still cannot express (a dot, or over-long) is simply not
// registered. It stays typable, and nothing is refused, reported or conditioned
// on it.
func spellForTelegram(name string) (string, bool) {
	spelled := strings.ReplaceAll(name, "-", "_")
	if !telegramCommandName.MatchString(spelled) {
		return "", false
	}
	return spelled, true
}

// commandsFor renders the vocabulary as Telegram's command list.
//
// Pipelines first, by name, then the built-ins, bounded by Telegram's cap: an
// over-long list is truncated rather than rejected, and what falls off the end
// is still typable.
//
// POSITION IS NOT EXPRESSIBLE HERE. Telegram's finest command scope is a CHAT,
// and a forum's general surface and its topics share it, so the union is
// registered and the manager's own usage replies correct a command used in the
// wrong place. That division is deliberate: the menu offers, the replies
// correct.
func commandsFor(v Vocabulary) []BotCommand {
	var builtins, pipelines []BotCommand
	for _, e := range v.Entries {
		spelled, ok := spellForTelegram(e.Name)
		if !ok {
			continue
		}
		desc := e.Description
		if desc == "" {
			desc = e.Name
		}
		// The icon LEADS the description, because a Telegram command name takes
		// no emoji and the description is the rest of the row.
		if e.Icon != "" {
			desc = e.Icon + " " + desc
		}
		cmd := BotCommand{Command: spelled, Description: truncate(desc, telegramMaxDescription)}
		if e.Kind == "builtin" {
			builtins = append(builtins, cmd)
		} else {
			pipelines = append(pipelines, cmd)
		}
	}
	sort.Slice(pipelines, func(i, j int) bool { return pipelines[i].Command < pipelines[j].Command })
	// PIPELINES FIRST. Telegram lists commands in the order given, and typing
	// `/` is overwhelmingly somebody reaching for an agent — the manager's own
	// commands are the rarer errand and belong after them.
	//
	// Overflow therefore drops BUILT-INS first, which is the right half to
	// lose: they are four fixed words a person can learn, while a Pipeline they
	// cannot complete is one they may not know exists.
	out := append(pipelines, builtins...)
	if len(out) > telegramMaxCommands {
		out = out[:telegramMaxCommands]
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// menu tracks what this adapter last registered, so Telegram is called only
// when its own view actually changed.
type menu struct {
	mu sync.Mutex
	// revision is the manager's, last fetched.
	revision string
	// registered is the ADAPTED list last sent, per chat id.
	registered map[string]string
	// spelling maps a Telegram command name back to the published name, for
	// naming a Pipeline to a person in the form the menu completes.
	spelling map[string]string
}

func newMenu() *menu {
	return &menu{registered: map[string]string{}, spelling: map[string]string{}}
}

// stale reports whether the manager's revision differs from the one last
// SYNCED. It records nothing.
//
// Recording here is what broke this on a live cluster: the revision was
// consumed before the work, so a run that fetched nothing — a manager still
// rolling out, or a moment before this adapter had read its channels — marked
// that revision done and never tried again. The menu stayed empty and no error
// was logged, because nothing had failed. It just had not happened.
//
// A revision is recorded by `synced`, after the work.
func (m *menu) stale(revision string) bool {
	if m == nil || revision == "" {
		return false // a manager that predates the vocabulary
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.revision != revision
}

// synced records a revision as done. Called ONLY when the work completed: the
// vocabulary was read and every served chat was registered.
func (m *menu) synced(revision string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revision = revision
}

// publishedName maps a Telegram command name back to what the manager
// published. Unknown names pass through: a person may always type the real one.
func (m *menu) publishedName(spelled string) string {
	if m == nil {
		return strings.ReplaceAll(spelled, "_", "-")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if real, ok := m.spelling[spelled]; ok {
		return real
	}
	return strings.ReplaceAll(spelled, "_", "-")
}

// displayName renders a published name in the spelling the MENU completes, so
// what a listing prints and what the composer offers are one string.
func (m *menu) displayName(published string) string {
	if m == nil {
		return published
	}
	if spelled, ok := spellForTelegram(published); ok {
		return spelled
	}
	return published
}

// syncCommands re-registers the command menu for every served chat, but only
// where the ADAPTED list differs from what was last sent.
//
// Registration is rate-limited by Telegram, so a vocabulary change that leaves
// the adapted list identical must produce no call at all — a Pipeline whose
// name Telegram cannot express is exactly that case.
func (a *adapter) syncCommands(ctx context.Context, revision string) {
	if !a.menu.stale(revision) {
		return
	}
	v, err := a.mgr.Vocabulary(ctx)
	if err != nil {
		log.Printf("fetch vocabulary: %v", err)
		return // NOT recorded: the next poll must try again
	}
	cmds := commandsFor(v)

	a.menu.mu.Lock()
	a.menu.spelling = map[string]string{}
	for _, e := range v.Entries {
		if spelled, ok := spellForTelegram(e.Name); ok {
			a.menu.spelling[spelled] = e.Name
		}
	}
	a.menu.mu.Unlock()

	fingerprint := commandFingerprint(cmds)
	served := a.servedChannelList()
	if len(served) == 0 {
		// Nothing to register FOR yet — this adapter has not read its channels,
		// or serves none. Leaving the revision unrecorded is the whole point:
		// the menu appears once there is a chat to put it in.
		return
	}
	done := true
	for _, sc := range served {
		chatID := sc.cfg.ChatID
		if chatID == "" {
			continue
		}
		a.menu.mu.Lock()
		unchanged := a.menu.registered[chatID] == fingerprint
		a.menu.mu.Unlock()
		if unchanged {
			continue
		}
		if err := a.client(sc.token).SetCommands(ctx, chatID, cmds); err != nil {
			log.Printf("register commands for chat %s: %v", chatID, err)
			done = false // retry this revision on the next poll
			continue
		}
		a.menu.mu.Lock()
		a.menu.registered[chatID] = fingerprint
		a.menu.mu.Unlock()
	}
	if done {
		a.menu.synced(revision)
	}
}

// commandFingerprint renders a command list to a comparable string.
func commandFingerprint(cmds []BotCommand) string {
	var b strings.Builder
	for _, c := range cmds {
		b.WriteString(c.Command)
		b.WriteByte(0)
		b.WriteString(c.Description)
		b.WriteByte(0)
	}
	return b.String()
}

// ---- offered controls ----------------------------------------------------

// choicePrefix marks callback data this stack owns. The payload after it is the
// Pipeline's REAL name: callback data is never displayed, so spelling it for
// the transport would buy nothing and cost signal-telegram a lookup.
const choicePrefix = "p:"

// telegramMaxCallbackData is Telegram's cap on callback_data, in bytes.
const telegramMaxCallbackData = 64

// inlineKeyboard renders offered actions as controls attached to the message.
//
// INLINE, never a reply keyboard. A reply keyboard is shown to every member of
// a group and replaces their own composer — not acceptable on an operations
// chat several people read.
//
// A choice whose callback payload will not fit is left out of the keyboard. It
// is not lost: the prose the manager wrote names every choice and its addressed
// form, which is the same fallback a transport with no controls at all gets.
func inlineKeyboard(choices []Choice) any {
	if len(choices) == 0 {
		return nil
	}
	var rows [][]map[string]string
	for _, c := range choices {
		data := choicePrefix + strings.TrimPrefix(c.Command, "/")
		if len(data) > telegramMaxCallbackData {
			continue
		}
		label := c.Label
		if label == "" {
			label = c.Command
		}
		rows = append(rows, []map[string]string{{"text": label, "callback_data": data}})
	}
	if len(rows) == 0 {
		return nil
	}
	return map[string]any{"inline_keyboard": rows}
}

// commandMention matches a slash-command mention in rendered prose. It captures
// broadly and the LOOKUP narrows: only a name this adapter actually registered
// is rewritten, so a path, a date or a URL fragment passes through untouched.
var commandMention = regexp.MustCompile(`/([a-z0-9][a-z0-9.-]*)`)

// rewriteCommands renders every Pipeline the manager NAMED in the spelling the
// composer completes.
//
// The manager publishes `k8s-observe` and Telegram's menu completes
// `k8s_observe`. A listing that printed the first while the composer offered the
// second would show two strings for one thing, so the adapter picks one — the
// one that works when tapped or typed here.
//
// Only registered names are touched. Anything else is somebody's text.
func (m *menu) rewriteCommands(text string) string {
	// A NIL MENU IS INERT, never a panic: rendering must work for a manager
	// that publishes no vocabulary and for any caller that has not built one.
	if m == nil {
		return text
	}
	m.mu.Lock()
	published := make(map[string]string, len(m.spelling))
	for spelled, real := range m.spelling {
		if spelled != real {
			published[real] = spelled
		}
	}
	m.mu.Unlock()
	if len(published) == 0 {
		return text
	}
	return commandMention.ReplaceAllStringFunc(text, func(match string) string {
		if spelled, ok := published[match[1:]]; ok {
			return "/" + spelled
		}
		return match
	})
}
