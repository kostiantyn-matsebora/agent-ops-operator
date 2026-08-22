## MODIFIED Requirements

### Requirement: Discovery reads what the surface already knows
The listing SHALL be built from what a surface can already obtain. A surface
that observes Pipelines for itself SHALL use that view. A surface that cannot —
a channel adapter is granted NO Kubernetes access, so it can never read a
Pipeline — SHALL be supplied by the manager over the adapter contract it already
speaks.

Discovery SHALL therefore add no adapter PERMISSION and no CRD field. What a
surface cannot see for itself the manager states, because the alternative is
granting an adapter cluster access to render a menu.

#### Scenario: No new access
- **WHEN** the discovery listing is served to a surface that observes Pipelines
- **THEN** it uses that existing read-only view and adds no permission or field

#### Scenario: A surface with no cluster view is supplied
- **WHEN** an adapter that holds no Kubernetes access offers a listing
- **THEN** it is built from the manager-published vocabulary, and the adapter
  gains no Kubernetes permission

### Requirement: Every surface keeps a discovery path that needs no client support
The Pipeline-listing command SHALL remain available on every chat surface and
SHALL report the same Ready Pipelines the typeahead would offer. A surface whose
client cannot present a listing SHALL therefore still have a way to find out
what it can address.

The command SHALL be named for what it lists. A Pipeline is what a message
addresses, and "agent" already names a definition inside a profile's repository
— one word for two things, with the more visible use being the wrong one. The
former name SHALL keep working, because it is published in installs already, and
SHALL NOT be offered, registered or printed anywhere.

An ambiguous bare message SHALL itself name the available Pipelines, so a person
who does not know the command is told at the moment it matters.

#### Scenario: Command works where typeahead cannot
- **WHEN** a person sends the Pipeline-listing command on a surface with no input assistance
- **THEN** the same Ready Pipelines are posted to that surface

#### Scenario: The listing is named for Pipelines
- **WHEN** a surface offers, registers or prints the listing command
- **THEN** it uses the Pipeline-named form, and the former name appears nowhere

#### Scenario: The former name still works
- **WHEN** a person sends the former name
- **THEN** the same listing is posted, unchanged

#### Scenario: Ambiguity teaches the form
- **WHEN** a bare message is refused as ambiguous
- **THEN** the reply names the Pipelines that serve the surface and shows the addressed form

## ADDED Requirements

### Requirement: A surface shows that addressing exists before anyone types
A surface SHALL make the address prefix discoverable WITHOUT requiring a person
to have already typed it, sent a message, or read documentation. A person who
has never used the system SHALL be able to see, from the composer alone, that
commands exist.

Where the transport provides a standing affordance for its own command
vocabulary, registering with it SHALL be preferred over posting a message,
because it costs no message and does not expire from the scrollback. Everything
the transport can express SHALL be registered — the built-in commands and the
addressable Pipelines alike — so completion covers what a person actually types.
Where the transport provides no such affordance, the composer SHALL carry a
visible hint.

The reactive paths SHALL remain: the entry point tells a person that addressing
exists, and the existing replies still teach the form at the moment it is
needed.

#### Scenario: The composer advertises commands
- **WHEN** a person opens a chat surface for the first time and sends nothing
- **THEN** the surface already shows that a command prefix is available, without
  a message having been posted to teach it

#### Scenario: The transport's own affordance is used where it exists
- **WHEN** a transport can be told its command vocabulary and renders its own
  control for it
- **THEN** the adapter registers everything that transport can express, built-in
  commands and Pipelines alike, so the control appears and completes both

#### Scenario: Reactive teaching is unchanged
- **WHEN** a bare message is refused as ambiguous
- **THEN** the reply still names the Pipelines and shows the addressed form

### Requirement: A conversation's own composer offers the commands that act on it
A composer attached to an existing conversation SHALL offer the commands valid
there — releasing the conversation's runtime and ending the conversation — and
SHALL NOT offer Pipeline addressing, which inside a thread is input for the
agent rather than a command.

The two commands SHALL be presented TOGETHER with the difference between them
stated, because they are one word apart and only one of them ends the
conversation.

#### Scenario: Thread commands are offered where they work
- **WHEN** a person types the address prefix in a composer attached to a
  conversation
- **THEN** the release and end commands are offered, each with what it does

#### Scenario: Pipelines are not offered inside a thread
- **WHEN** a person types the address prefix inside a conversation's thread
- **THEN** no Pipeline is offered, because addressing one there does not start
  anything

#### Scenario: The pair is never shown alone
- **WHEN** either thread command is presented
- **THEN** the other is presented beside it with the distinction stated

### Requirement: Offered choices are a discovery path where a transport has them
Where the manager offers a set of actions, a surface whose transport supports
selectable controls SHALL present them as controls rather than requiring the
text to be retyped.

Selecting a choice offered in answer to a message a person already wrote SHALL
carry that message to the chosen Pipeline. Making somebody retype what they just
typed is the cost this removes, and it is the whole value of the affordance at
the moment they are stuck.

A surface whose transport has no such controls SHALL present the same choices as
a list, and SHALL remain fully conformant.

#### Scenario: Ambiguity becomes one action
- **WHEN** a bare message is refused because several Pipelines serve the surface
- **THEN** the reply offers each Pipeline as a control, and selecting one sends
  the person's original message to that Pipeline without it being retyped

#### Scenario: A transport without controls loses nothing
- **WHEN** the same refusal reaches a surface that cannot render controls
- **THEN** the choices are presented as a list naming each Pipeline and the
  addressed form
