## REMOVED Requirements

### Requirement: The incident view is rooted at the uncaused conversation, and nests

**Reason**: Described a dedicated incident page standing apart from the
conversation list. Both are deleted (`pages/Conversations.tsx`,
`pages/Conversation.tsx`), replaced by one view under `pages/chat/`.

**Migration**: Nesting a coordination's whole tree, at any depth, in one
readable place is now `console-conversation-tree`'s "The list groups a
coordination under its uncaused root" and "The uncaused root's thread is
the incident timeline" requirements, inside the view that replaces both
former pages.

### Requirement: A conversation is reached from its ancestors, and its ancestors from it

**Reason**: Described the conversation list's group/flatten toggle (flat by
default) and the old transcript's single parent-name link, both on pages
this change deletes.

**Migration**: `console-conversation-tree`'s "The list groups a
coordination under its uncaused root" requirement groups by default now,
with a control to flatten — the toggle's default inverted, not lost.

Its "A member names its place in the tree" requirement names the WHOLE
chain from the uncaused root, each step navigable, where the old
transcript named only the immediate parent.

### Requirement: Un-escalated closures are shown, with their reason

**Reason**: Described the conversation list page this change deletes.

**Migration**: `console-conversation-tree`'s "An incident nobody was told
about is visible" requirement shows the same marker and `closeReason` in
the tree view's list, unchanged in substance.
