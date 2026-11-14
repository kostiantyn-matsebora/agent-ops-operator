// Package namespace carries the control/conversations namespace pair that
// replaces the manager's single NAMESPACE value (split-conversation-
// namespace). It is deliberately a leaf: a value type with no dependencies,
// so every package that places an object — a reconciler, the HTTP API, the
// state store — can import it with no risk of an import cycle.
package namespace

// Namespaces is the pair every namespace-scoped component now carries, in
// place of one Namespace string.
//
// Control is where the control plane lives: the manager, every wiring and
// identity CR, the state store outside its conversation-scoped kinds, NATS,
// adapters, the gateway and the console.
//
// Conversations is where a runtime pod runs, what it runs as, mounts or
// resolves, and conversation-scoped state — the placement rule design.md
// decision 1 states in full.
//
// A single-namespace install is the two fields holding one equal value, and
// every caller threads BOTH rather than branching on whether they differ:
// that is the one-code-path rule this pair exists to keep.
type Namespaces struct {
	Control       string
	Conversations string
}

// Resolve builds the pair from the two values a caller already read from its
// own environment: control, and a conversations value that may be empty.
// Empty defaults to control — the default that makes a single-namespace
// install two equal strings, never a branch on whether it was set.
func Resolve(control, conversations string) Namespaces {
	if conversations == "" {
		conversations = control
	}
	return Namespaces{Control: control, Conversations: conversations}
}

// Distinct reports whether the pair names two different namespaces. It
// exists to size the controller-runtime cache's namespace set — never to
// gate behaviour, which must take one code path whatever this answers.
func (n Namespaces) Distinct() bool {
	return n.Control != n.Conversations
}
