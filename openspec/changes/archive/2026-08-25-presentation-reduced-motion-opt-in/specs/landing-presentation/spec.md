## MODIFIED Requirements

### Requirement: Without scripting it is the story in order

**With no scripting available the presentation SHALL remain its own content** —
the beats as an ordinary numbered list, in order, nothing hidden and nothing
lost.

A reader who never runs the script SHALL still receive the whole explanation.
The presentation is therefore never the only carrier of a claim the page needs
to make.

#### Scenario: Scripting is unavailable

- **WHEN** the presentation is rendered without scripting
- **THEN** every beat is readable as an ordinary list, in order

#### Scenario: A reader has asked for reduced motion

- **WHEN** the reader's system requests reduced motion
- **THEN** nothing moves, the drawing is shown fully composed, and the beats are
  readable as a list
- **AND** nothing moves at any later time unless the reader asks for it

## ADDED Requirements

### Requirement: Reduced motion is a still the reader may start

A reduced-motion reader SHALL be offered a control that plays the presentation,
and it SHALL be the ONLY control offered while the presentation is still.

The preference says *do not move things at me*. It does not say *never let me
ask*, and a page that answers it by DELETING the control has decided on the
reader's behalf that the explanation is not for them. The beats are the landing
page's central account of the product, and on that machine they would otherwise
be reachable only in a form the page did not author.

**Nothing SHALL move until that control is used.** Engaging it is the reader's
own action, so what follows is the ordinary presentation with its full
transport, and it SHALL start at the first beat rather than resuming from
whatever the still was composed as.

While the presentation is still, the beat-level controls SHALL NOT be shown: the
beats are already on the page as the list, and a second copy of them beside it
is two things to read where the reader asked for less.

#### Scenario: A reduced-motion reader is offered the presentation

- **WHEN** the presentation is rendered for a reader whose system requests
  reduced motion
- **THEN** a play control is present and operable, and no other transport
  control is
- **AND** nothing has moved

#### Scenario: A reduced-motion reader starts it

- **WHEN** that reader uses the play control
- **THEN** the presentation plays from its first beat with its full transport,
  exactly as it does for a reader who asked for no such preference

#### Scenario: A reduced-motion reader pauses it again

- **WHEN** that reader stops the presentation after starting it
- **THEN** the transport stays, the current beat stays on the stage, and the
  reader may start it again or select any beat
