## Purpose

Agent output reaches chat as named blocks with a fold, so a reader gets the
conclusion first and the long tail only on request. The vocabulary of sections is
OPEN — each agent's purpose shapes its own — while the fold and the caps are
CLOSED, so every surface can present any agent's answer without knowing what that
agent does.

This capability owns the GRAMMAR, the parse and the cap. Who gets the format
specification injected belongs to `profile-is-identity`, and whether a signal
body is parsed at all belongs to `signal-adapter-contract` — each stated ONCE, in
the capability that owns it.

## ADDED Requirements

### Requirement: Agent output is parsed into blocks

An agent's reported output SHALL be parsed manager-side into an ordered list of
typed blocks. Parsing SHALL happen EXACTLY ONCE, in the manager, and the result
SHALL travel on the outbound message. No adapter SHALL parse the grammar.

Parsing SHALL be total: every input yields blocks. Output containing no
recognized tag SHALL yield a single above-the-fold block holding the whole text,
which is how output written before this grammar — or by an agent that ignores it
— keeps rendering exactly as it does today.

Parsing SHALL follow AGENT-REPORTED text rather than the message kind that
carries it. A run that failed and explained itself is reported as a notice, and
that explanation SHALL be parsed exactly as an answer is — it is the longest
thing a failed investigation produces, and so the output the fold serves best.

#### Scenario: Untagged output still renders

- **WHEN** an agent reports plain prose with no block tags
- **THEN** it becomes one above-the-fold block, and every surface renders it as
  it renders agent output today

#### Scenario: A failed run's explanation is folded too

- **WHEN** a run fails and its own explanation is what reaches the thread
- **THEN** that explanation is parsed into blocks and folded, the same as a
  successful run's answer

#### Scenario: Adapters receive blocks, never grammar

- **WHEN** any agent output is delivered to a bound thread
- **THEN** the outbound message carries parsed blocks, and no adapter is required
  to recognize a tag to render the message

### Requirement: Two reserved tags, an open vocabulary

The grammar SHALL reserve exactly two block tags:

- `<title>` — at most one, rendered FIRST, a single line
- `<details>` — THE FOLD: its content is collapsed by default on every surface

Every other block tag SHALL be an agent-named section carrying that name as its
label. Named sections SHALL be rendered ABOVE the fold, in the order the agent
wrote them. The manager SHALL NOT reorder named sections, because with an open
vocabulary it cannot know which section is the conclusion.

Adapters SHALL render a named section generically from its label and content.
An adapter SHALL NOT carry knowledge of any particular agent's section names.

#### Scenario: An agent names its own sections

- **WHEN** an agent emits `<root-cause>`, `<evidence>` and `<fix>` sections
- **THEN** all three render above the fold, labelled and in that order, on every
  surface — and no adapter contains those names

#### Scenario: The fold is honoured everywhere

- **WHEN** output carries a `<details>` block
- **THEN** every surface presents it collapsed, and the reader can expand it

#### Scenario: Title leads regardless of position

- **WHEN** an agent emits `<title>` after another section
- **THEN** the title is still rendered first

### Requirement: Tags are block-level and unambiguous

A tag SHALL be recognized only when ALL of the following hold:

- it stands alone on its own line, at the start of that line
- it forms a well-formed open/close pair
- it is not inside a fenced code block or an inline code span

Text failing any condition SHALL be literal text. Inline formatting SHALL remain
the existing markdown subset — the grammar SHALL define no inline tags.

This is what keeps an open vocabulary safe: agent output routinely contains `<`
in shell redirects, generics and code, and any of it that is not a well-formed
standalone block tag is prose.

#### Scenario: A less-than sign in prose is prose

- **WHEN** an agent writes `if x < y` or `Deployment<T>` mid-line
- **THEN** nothing is parsed as a block and the text renders as written

#### Scenario: A tag inside a code block is code

- **WHEN** a fenced block contains a line reading `<details>`
- **THEN** it renders as part of the code, not as a fold

#### Scenario: An unpaired tag is text

- **WHEN** an agent emits an opening tag with no matching close before the end of
  its output
- **THEN** that region is closed at end of output rather than discarding it, and
  no content is lost

### Requirement: The manager bounds what sits above the fold

The manager SHALL cap the total length of above-the-fold content. Content
exceeding the cap SHALL be DEMOTED into the fold, never dropped.

This is the guarantee the prompt alone cannot make: whatever an agent writes, a
reader who does not expand anything reads a bounded amount.

#### Scenario: An over-long section is demoted, not truncated

- **WHEN** an agent writes far more above-the-fold content than the cap allows
- **THEN** the overflow appears inside the fold, and expanding it recovers every
  word the agent wrote

#### Scenario: Nothing is lost

- **WHEN** any output is parsed, capped and rendered
- **THEN** the full text the agent reported remains reachable on the surface
