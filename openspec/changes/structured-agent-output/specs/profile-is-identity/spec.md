## ADDED Requirements

### Requirement: A profile DECLARES its output contract, and the field is required

`AgentProfile.spec.outputFormat` SHALL be REQUIRED, with exactly two values:

| Value | Means |
|---|---|
| `blocks` | the shared output-format specification is appended to the agent's prompt — the block grammar, the fold, the markdown subset and a default section set |
| `none` | NOTHING is appended, and the profile's own prompt owns formatting entirely |

THERE SHALL BE NO DEFAULT, because both candidate defaults are wrong: `none`
leaves output unformatted unless the author wrote a format into the prompt, and
`blocks` shapes output by something the author never asked for. The author
declares it.

Making it required SHALL break `kubectl apply` on a profile that omits it. That
is intended: the failure names the valid values, and a profile whose output
contract is unstated is the condition this field exists to end.

This is IDENTITY, never capability — it shapes how the agent SPEAKS, not what it
may call. It SHALL NOT affect the allowlist or the MCP servers, which remain
exclusively the originating Pipeline's.

THE DECLARATION SHALL GATE THE PROMPT ONLY. Whether output is parsed into blocks
is not a profile decision — an adapter parses whatever it is given, so a profile
declaring `none` whose agent emits tags anyway is still rendered as blocks.

It SHALL NOT gate the operator's unconditional prompt content. Text stating how
the system handles a reply — that the printed answer IS the deliverable, and
that the agent never posts to a transport itself — is a fact about the system
and SHALL be injected whatever this field says.

#### Scenario: A profile without the field is refused

- **WHEN** an `AgentProfile` is applied with no `outputFormat`
- **THEN** the API rejects it, naming `blocks` and `none`

#### Scenario: Declaring none leaves formatting to the profile

- **WHEN** a profile declares `none` and its own prompt describes an output
  structure
- **THEN** no shared specification is injected, and the agent follows its profile

#### Scenario: The declaration grants nothing

- **WHEN** a profile declares `blocks`
- **THEN** the conversation's tools and MCP servers are identical to the same
  profile declaring `none`

#### Scenario: Declining the spec does not decline the parse

- **WHEN** a profile declares `none` and its agent emits block tags anyway
- **THEN** the surface renders them as blocks, because parsing follows the TEXT
