# structured-agent-output

## Purpose

Agent output reaches chat as named blocks with a fold, so a reader gets the
conclusion first and the long tail only on request. The vocabulary of sections is
OPEN — each agent's purpose shapes its own — while the fold and the caps are
CLOSED, so every surface can present any agent's answer without knowing what that
agent does.

This capability owns the GRAMMAR and its recognition rules. Who gets the format
specification injected belongs to `profile-is-identity`. WHERE the grammar is
read belongs to `adapter-rendered-messages`, which states that a body is markdown
plus this grammar and that the adapter reads both.

## Requirements

### Requirement: Agent output carries structure the surface reads

An agent's reported output SHALL reach every surface AS THE AGENT WROTE IT. The
manager SHALL NOT parse it, rewrite it, or shorten it.

Each channel adapter SHALL parse the grammar and render it to what its transport
has. This is how the contract already treats markdown: it names a subset and
every adapter renders what it can, and the block grammar is an extension of the
same body language rather than a second representation beside it.

Parsing SHALL be total: every input yields blocks. Output containing no
recognized tag SHALL yield a single above-the-fold block holding the whole text,
which is how output written before this grammar — or by an agent that ignores it
— keeps rendering exactly as it does today.

Parsing SHALL follow AGENT-REPORTED text rather than the message kind that
carries it. A run that failed and explained itself is reported as a notice, and
that explanation SHALL be parsed exactly as an answer is — it is the longest
thing a failed investigation produces, and so the output the fold serves best.

A body the MANAGER composed — a listing, a refusal, a usage error — is its own
prose and carries no grammar to find. An adapter parses it anyway and gets one
block, which is the same result.

#### Scenario: Untagged output still renders

- **WHEN** an agent reports plain prose with no block tags
- **THEN** it becomes one above-the-fold block, and every surface renders it as
  it renders agent output today

#### Scenario: A failed run's explanation is folded too

- **WHEN** a run fails and its own explanation is what reaches the thread
- **THEN** that explanation is parsed into blocks and folded, the same as a
  successful run's answer

#### Scenario: A rehydrated transcript renders like a live one

- **WHEN** a viewer rebuilds a conversation from `status.runs[].result` after a
  restart
- **THEN** it parses the same characters a live message carried, and the reader
  sees the same title, sections and fold

#### Scenario: The manager returns the text unchanged

- **WHEN** an agent's output is recorded and delivered
- **THEN** what the adapter receives is byte-for-byte what the agent printed,
  and nothing in the manager has inspected the grammar

### Requirement: Two reserved tags, an open vocabulary

The grammar SHALL reserve exactly two block tags:

- `<title>` — at most one, rendered FIRST, a single line
- `<details>` — THE FOLD: its content is collapsed by default on every surface

Every other block tag SHALL be an agent-named section carrying that name as its
label. Named sections SHALL be rendered ABOVE the fold, in the order the agent
wrote them. The manager SHALL NOT reorder named sections, because with an open
vocabulary it cannot know which section is the conclusion.

An adapter SHALL render a named section generically from its label and content,
and SHALL NOT carry knowledge of any particular agent's section names.

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

### Requirement: Nothing shortens an agent's output

No component SHALL impose a length budget on above-the-fold content, and none
SHALL move content between blocks to satisfy one.

WHAT SITS ABOVE THE FOLD IS WHAT THE AGENT PUT THERE. Which part is the summary
is a judgement about meaning, and the agent already made it by choosing what
goes inside `<details>`. A length budget is a GUESS at that judgement, and with
an open section vocabulary there is nothing better to guess with.

Brevity is the PROMPT's responsibility. A specification can ask for short
sections and bulleted facts. It SHALL NOT be enforced by cutting the result up
afterwards.

#### Scenario: A table arrives whole

- **WHEN** an above-the-fold section holds a markdown table of twelve rows
- **THEN** every row renders above the fold, under its header, because trimming
  at a line boundary would leave the remainder as pipes with nothing to head them

#### Scenario: The action is never hidden

- **WHEN** an agent writes several sections and the last one is the fix
- **THEN** it renders above the fold like every other section, because written
  order is not importance and nothing may infer otherwise

#### Scenario: A verbose agent produces a long message

- **WHEN** an agent writes far more above the fold than a reader wants
- **THEN** the message is long, and the correction belongs in that agent's
  prompt — no surface silently rearranges what it said
