# profile-inline-role

## Why

An `AgentProfile` gets its role from `.claude/agents/<agent>.md` in its
repository. A profile with NO repository — which `k8s-bundle` ships on purpose,
since a cluster agent needs no code checkout — can name no definition file,
because nothing is checked out to read.

The result is an agent with no role at all: a bare session whose only inputs are
the Pipeline's allowlist and the task text. It answers direct questions
adequately and arrives at a cluster event with no instructions whatsoever, which
is precisely the case where improvisation is least wanted.

`spec.prompt` does not solve this: it is a repo-relative template PATH, so it
needs the repository the profile does not have.

## What Changes

- **`AgentProfile.spec.systemPrompt`**: inline role text, optional.
- **The runtime APPENDS it** (`--append-system-prompt`), so its own system
  prompt survives. It is identity, never capability — the allowlist remains the
  sole permission authority, and nothing here widens it.
- **`k8s-bundle` ships a default role** for its repo-less profile, so the shipped
  agent is not personality-free out of the box.
- Profiles WITH a repository are unaffected and should keep carrying their role
  in the definition file, which is version-controlled and can declare `tools:`.

## Capabilities

### Modified Capabilities

- `profile-is-identity`: identity may now include an inline role, for profiles
  that cannot reference a definition file.

## Impact

- **API**: one optional string on `AgentProfileSpec`; CRD regenerated.
- **Manager**: `dispatch` copies it onto the work unit.
- **Runtime**: `runtime-claude` passes `--append-system-prompt` when present.
- **Chart**: `k8s-bundle.profile.systemPrompt`, defaulted.
- **Images**: manager and runtime both rebuilt — this is a vertical change, not
  a values-only one.
- Backward compatible: absent field, unchanged behavior.
