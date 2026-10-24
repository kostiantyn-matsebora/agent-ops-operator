## ADDED Requirements

### Requirement: Six verbs forward to the manager's /admin/* surface

The server SHALL expose `pause_signal_source(name)`,
`resume_signal_source(name)`, `pause_pipeline(kind, name)`,
`resume_pipeline(kind, name)`, `set_maintenance_mode()` and
`clear_maintenance_mode()`.

Each SHALL forward verbatim to the matching `/admin/*` verb
(`operational-pause-controls`), carrying the calling conversation's own
token exactly as the four coordination verbs already do.

The server decides nothing. It forwards the token and the args, and
reports back whatever the manager answered — success or an explicit
refusal.

**Authorization is not the Coordinator's `agents[]` bound.** `invoke`'s
bound restricts a caller to the members its own Coordinator lists. These
six verbs instead act on named `SignalSource`, `Pipeline`, `Coordinator`
or `MaintenanceMode` objects, none of which is ever a member of any
conversation's own subtree.

The manager instead authorizes from the calling conversation's own BOUND
TOOL ALLOWLIST — what it was actually given at dispatch. Declaring a tool
in a Pipeline's or Coordinator's `toolsets` is what grants it, the same
"reach bounded per caller" principle every other tool here already
follows.

A `channel-reader` token SHALL reach none of these six, exactly as it
reaches none of the four coordination verbs.

#### Scenario: A bound tool reaches the manager
- **WHEN** a coordinating agent whose Pipeline binds
  `mcp__aops__pause_signal_source` calls `pause_signal_source`
- **THEN** the server forwards the call with the caller's own token, and
  reports the manager's result

#### Scenario: An unbound tool is refused by the manager, not the server
- **WHEN** a coordinating agent whose Pipeline does NOT bind
  `mcp__aops__set_maintenance_mode` calls `set_maintenance_mode`
- **THEN** the server forwards the call exactly as it would any other, and
  the MANAGER refuses it naming the missing tool — the server itself makes
  no decision

#### Scenario: A channel-reader token cannot reach any of the six
- **WHEN** a caller holding a `channel-reader:<channel>` token calls any of
  the six
- **THEN** the manager refuses it, and the server has made no decision

#### Scenario: The target need not be in the caller's own subtree
- **WHEN** a coordinating agent calls `pause_pipeline` naming a Pipeline
  that is not any descendant of the caller's own conversation
- **THEN** the call is evaluated solely on whether the caller's own bound
  tools include `mcp__aops__pause_pipeline` — no subtree membership check
  applies, because the target is not a conversation at all
