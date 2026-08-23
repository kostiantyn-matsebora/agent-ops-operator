## ADDED Requirements

### Requirement: A newline in a message is a line break on every surface

The message subset SHALL define a single newline in a free-text field as a **line
break**, not as whitespace. Every renderer SHALL honour it — external channel
adapters and in-process providers alike — so one message has ONE shape wherever
it is read.

**A renderer whose markdown engine follows CommonMark SHALL enable that
behaviour explicitly.** CommonMark treats a soft break as a space, which is
correct for prose written to be re-wrapped and wrong for a chat message written
line by line. The engine's default SHALL NOT decide what the contract means.

**Fenced code blocks and tables keep their own rules.** A break inside them means
what that construct means, and enabling line breaks in prose SHALL NOT alter
their content.

**The corollary binds the AUTHOR, and the mandatory format specification SHALL
state it: prose SHALL NOT be hard-wrapped.** A newline an agent types is one the
reader sees, so wrapping a sentence to keep source lines short is a formatting
decision rather than tidiness. Wrapping is the surface's job.

This exists because the subset was silent and two renderers read the silence
differently: the same answer arrived as written on one surface and as a run-on
paragraph on the other, in exactly the shape the format specification exists to
prevent.

#### Scenario: A templated answer is read on any surface

- **WHEN** an agent writes a section label on one line and its content on the
  next, as the format specification instructs
- **THEN** every surface renders them as two lines

#### Scenario: One message, two surfaces

- **WHEN** the same message is delivered to two bound channels with different
  renderers
- **THEN** its line structure is the same on both, and neither surface invents or
  removes a break

#### Scenario: A renderer's engine defaults to soft breaks

- **WHEN** a renderer is built on a markdown engine that treats a single newline
  as a space
- **THEN** the renderer configures it to break, rather than inheriting a default
  that contradicts the contract

#### Scenario: A fenced block is rendered

- **WHEN** a message carries a fenced code block or a table
- **THEN** its content is unchanged by the line-break rule, and no break is
  inserted into the code

#### Scenario: An author hard-wraps a sentence

- **WHEN** an agent wraps prose across source lines for tidiness
- **THEN** the reader sees those breaks, and the format specification tells the
  agent not to do it
