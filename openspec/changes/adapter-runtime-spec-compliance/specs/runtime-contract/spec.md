# runtime-contract — delta

## MODIFIED Requirements

### Requirement: The specification is a versioned reference page

The runtime specification SHALL live at `docs/contracts/runtime.md` as a
published reference page. It SHALL carry the site's standard front matter —
title, permalink, description — which the site's own layout rules own.

Front matter is PRESENTATION, never a normative statement. A front-matter
change with no normative statement touched moves no version.

It SHALL carry a semver version, starting at `1.0.0`, and its own changelog
recording every version with what changed in it.

It SHALL replace the work-contract narrative in `docs/contracts.md` as the
normative statement of the contract. The narrative SHALL NOT survive as a
second statement beside it.

#### Scenario: A runtime author needs the contract

- **WHEN** an author sets out to build an `AgentRuntime` image
- **THEN** `docs/contracts/runtime.md` states every obligation of both
  parties
- **AND** the author needs no shipped runtime's source to build against it

#### Scenario: A normative statement changes

- **WHEN** a normative statement in the specification is added, changed or
  removed
- **THEN** the version moves as the compatibility rules direct, and the
  changelog records the change under the new version

#### Scenario: Front matter moves no version

- **WHEN** the page's front matter is edited and no normative statement changes
- **THEN** the specification version is unchanged

## ADDED Requirements

### Requirement: A startup configuration failure is a crash, not a lifecycle exit

The idle TTL SHALL remain the runtime's only LIFECYCLE exit: exit `0` after
`RUNTIME_IDLE_TTL_M` minutes without work.

Missing or unusable required configuration, detected before the first
`GET /work` poll, SHALL end the process with a nonzero exit naming what is
missing. That is a CRASH the kubelet surfaces and restarts, not a lifecycle
exit, and it does not breach the single-exit rule.

Honouring the pod's own termination signal is likewise not a lifecycle exit.

The specification SHALL state this scoping, so a conformance reading of the
exit rule never condemns failing fast at startup.

#### Scenario: Missing required configuration fails fast

- **WHEN** a runtime starts without a required variable such as `CONTROL_URL`
- **THEN** it exits nonzero before its first poll, naming the missing variable
  in its log

#### Scenario: The lifecycle exit rule still holds after startup

- **WHEN** a runtime has answered its first poll and runs out of work
- **THEN** exiting `0` at the idle TTL remains its one lifecycle exit

### Requirement: A definitive report rejection is not retried

The specification SHALL state the delivery rule for `POST /work/done`:

| Response | Runtime behaviour |
|---|---|
| transport error, timeout, `408`, `429`, any `5xx` | bounded retry, then log the loss and resume polling |
| any other `4xx` | NO retry — log the status and body, resume polling |
| `2xx` | done |

A definitive rejection means the manager refused the report's content, and
resending the same bytes cannot change the verdict. Retrying it only blocks the
poll loop.

Every undelivered report SHALL leave a log line carrying the response status
and body, so a refused report is diagnosable from the pod log. A report SHALL
never be dropped silently.

#### Scenario: A 400 is not retried

- **WHEN** the manager refuses a report with `400` naming an over-bound field
- **THEN** the runtime logs the status and body, does not resend, and returns
  to polling

#### Scenario: A transient failure keeps its bounded retry

- **WHEN** the manager is unreachable while a report is due
- **THEN** the runtime retries within its bounded budget, and logs the loss if
  the budget exhausts
