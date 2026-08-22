package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// THE MANAGER SAYS WHAT MAY BE TYPED; THE ADAPTER DECIDES WHAT IT CAN EXPRESS.
//
// A surface can only offer what it can see, and a channel adapter is granted no
// Kubernetes access at all — channel-telegram cannot read a Pipeline and never
// will. So the only component that can tell a surface what is addressable is
// this one.
//
// It publishes the vocabulary UNFILTERED. Which entries a transport can express
// is transport knowledge: Telegram's command names admit no hyphen, Telegram's
// list caps at 100, and neither fact belongs here any more than parse_mode or a
// 4096-character limit does. Nothing in this file may grow a rule that exists to
// suit one transport.

// EntryKind distinguishes a manager command from an addressable Pipeline. An
// adapter filters on properties, not on which array a thing arrived in, so both
// travel in ONE list.
type EntryKind string

const (
	// KindBuiltin is a command the manager itself intercepts.
	KindBuiltin EntryKind = "builtin"
	// KindPipeline is an addressable Pipeline.
	KindPipeline EntryKind = "pipeline"
)

// Position is WHERE an entry is valid. The two positions take disjoint sets:
// addressing a Pipeline works only on a general surface (inside a thread the
// same text is input for the agent), and the commands that end or release a
// conversation work only inside a thread (on a general surface there is no
// conversation to act on).
//
// A surface that can express the distinction offers only what applies where the
// person is typing. One that cannot — Telegram's finest command scope is a
// CHAT, and a forum's general surface and its topics share it — offers the
// union, and the manager's existing usage replies remain the correction.
type Position string

const (
	// PositionGeneral is the surface a conversation starts from.
	PositionGeneral Position = "general"
	// PositionThread is inside an existing conversation's thread.
	PositionThread Position = "thread"
)

// Entry is one thing a person may type.
type Entry struct {
	Kind        EntryKind `json:"kind"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Position    Position  `json:"position"`
	// Icon is how this entry is RECOGNISED in a list — an emoji, or nothing.
	// Published as declared and interpreted no further: whether a surface can
	// draw one, and where it puts it, is that adapter's business.
	Icon string `json:"icon,omitempty"`
	// Profile is the profile answering for a Pipeline entry, empty on a builtin.
	// It is DERIVED from what the Pipeline already declares — no CRD field was
	// added to carry prose here, because a second place to write a name is a
	// second place for it to be wrong.
	Profile string `json:"profile,omitempty"`
}

// Vocabulary is the whole of what may be typed, plus the revision identifying
// it.
type Vocabulary struct {
	Revision string  `json:"revision"`
	Entries  []Entry `json:"entries"`
}

// builtins are the manager's own commands, in the order a listing shows them.
//
// The retired listing name is absent ON PURPOSE: it still WORKS (see
// RetiredListCommand), but publishing it here would register it into a
// transport's command menu and teach the wrong noun to everyone who reads one.
func builtins() []Entry {
	return []Entry{
		{Kind: KindBuiltin, Name: ListCommand, Position: PositionGeneral,
			Description: "List the pipelines you can address"},
		{Kind: KindBuiltin, Name: "help", Position: PositionGeneral,
			Description: "Show the pipelines and how to address them"},
		{Kind: KindBuiltin, Name: ExitCommand, Position: PositionThread,
			Description: "Release this conversation's runtime, keep the conversation"},
		{Kind: KindBuiltin, Name: CloseCommand, Position: PositionThread,
			Description: "End this conversation and archive its thread"},
	}
}

// isListCommand reports whether a command name asks for the Pipeline listing.
// The retired name is accepted here and nowhere else.
func isListCommand(name string) bool {
	return name == ListCommand || name == RetiredListCommand || name == "help" || name == "start"
}

// ReservedCommands are the names a Pipeline cannot be reached by: interception
// precedes the Pipeline lookup, which is what makes the commands reliable.
func ReservedCommands() []string {
	return []string{ListCommand, RetiredListCommand, "help", "start", ExitCommand, CloseCommand}
}

// readyPipelines returns the addressable Pipelines as vocabulary entries,
// sorted by name. Returns nil on a list error so a caller can tell it apart
// from a namespace with none.
//
// READY ONLY: an unready Pipeline names wiring that does not resolve, and
// offering it invites a request nothing can serve. The listing command and the
// vocabulary must never disagree about this, which is why both come through
// here.
func (r *Router) readyPipelines(ctx context.Context) []Entry {
	var list agentopsv1alpha1.PipelineList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
		return nil
	}
	out := make([]Entry, 0, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
		if !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
			continue
		}
		profile := p.Spec.ProfileRef.Name
		out = append(out, Entry{
			Kind: KindPipeline, Name: p.Name, Position: PositionGeneral,
			Description: profile, Profile: profile, Icon: p.Spec.Icon,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Vocabulary builds the published vocabulary and its revision.
//
// NAMESPACE-WIDE, not per-channel, because addressing is: a command naming a
// Pipeline is resolved by name with no claim check, and the originating surface
// is folded into the resulting conversation's bindings. Every Ready Pipeline is
// therefore reachable from every wired surface, and a per-channel vocabulary
// would understate what a person may type.
func (r *Router) Vocabulary(ctx context.Context) Vocabulary {
	entries := append(builtins(), r.readyPipelines(ctx)...)
	return Vocabulary{Revision: revisionOf(entries), Entries: entries}
}

// revisionOf derives a revision from the entries themselves.
//
// DERIVED, NEVER STORED: two managers serving the same entries report the same
// revision and a restart changes nothing, so this adds no row to the
// state-durability matrix — it is "derivable from Kubernetes objects".
//
// Hashed over the PUBLISHED FIELDS ONLY. No timestamps, no resourceVersions, no
// conditions beyond the Ready filter that already decided membership: anything
// volatile in here would wake every adapter in the install on an unrelated edit.
func revisionOf(entries []Entry) string {
	h := sha256.New()
	for _, e := range entries {
		for _, f := range []string{string(e.Kind), e.Name, e.Description, string(e.Position), e.Icon} {
			h.Write([]byte(f))
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
