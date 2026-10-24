# test-stub-runtime Specification

## Purpose
TBD - created by archiving change e2e-testing. Update Purpose after archive.

## Requirements

### Requirement: A stub runtime exists for mechanisms an agent cannot be asked to exhibit
A stub agent runtime SHALL exist as a test-only image that conforms to the
`/work` contract — long-poll for a work unit, report through `/work/done`, exit
after the idle TTL. It is selected through the existing `AgentRuntime` image
field, so no chart template logic and no manager change is required to use it.

Its purpose SHALL NOT be stated as cost avoidance. The real runtime is the
primary oracle for everything an agent can be asked to demonstrate; the stub
exists for the disjoint set of behaviors that are **manager and runtime
mechanisms rather than agent behavior**, which no prompt makes a working agent
exhibit on cue. A stub test that could have been written against the real
runtime SHALL be written against the real runtime instead.

#### Scenario: The stub is a conforming runtime, not a mock
- **WHEN** the stub is deployed as the runtime for a conversation
- **THEN** it polls `/work`, reports through `/work/done`, and honours the idle TTL exactly as the contract specifies, with no manager-side special case for it

#### Scenario: An agent-observable behavior is not tested with the stub
- **WHEN** a candidate test asserts something the real agent could be asked to demonstrate, such as answer correctness or context recall
- **THEN** it is written against the real runtime, not the stub

### Requirement: Stub behavior is scripted by the input, deterministically
The stub SHALL select its behavior from the text of the work unit's input,
producing identical output for identical input on every run. It SHALL support at
minimum the following behaviors, each of which guards a named mechanism:

| Input directive | Stub behavior | Mechanism guarded |
|---|---|---|
| echo | returns the input as the result | dispatch and delivery of a successful run |
| fail | reports a failed run | a run that fails fails its conversation |
| stale-context | returns a `runtimeContextId` that names nothing | latest-wins handles; a lost promised context fails the run |
| no-context | omits `runtimeContextId` entirely | absence is not the same as loss |
| die | exits without reporting | inflight clearing, and FIFO promotion on pod DELETE |
| stall | holds the work unit past the idle TTL | the serial-per-conversation rule and idle eviction |
| storage-outage | reports a context-storage failure | the manager-side breaker HOLDS work rather than failing it |

#### Scenario: The same input always produces the same run
- **WHEN** the stub receives the same input twice
- **THEN** the reported result is identical, so a failure is a defect rather than variance

#### Scenario: A stale handle fails the run rather than being carried forward
- **WHEN** the stub returns a `runtimeContextId` that names nothing and a further input arrives
- **THEN** the run fails rather than repeating a doomed continuation, and the failure is visible on the conversation

#### Scenario: A storage outage holds work rather than destroying context
- **WHEN** the stub reports a context-storage failure
- **THEN** work is held by the breaker, and active conversations do not have their context declared lost

### Requirement: The stub does not ship
The stub SHALL be built and published only for testing, SHALL NOT be referenced
by any chart default, sample or documented install path, and SHALL be named so
that its presence in a running deployment is obviously wrong.

#### Scenario: No shipped default points at the stub
- **WHEN** the chart is rendered with default values, or a sample CR is applied
- **THEN** no `AgentRuntime` references the stub image

#### Scenario: The stub is not discovered as a component
- **WHEN** `.github/components.sh images` runs
- **THEN** neither the stub runtime nor the fake Bot API is listed, so no release tag publishes them and no CI matrix grows
