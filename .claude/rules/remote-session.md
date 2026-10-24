## Remote session (a change worked in the cloud)

**A RULE STATING A WORKSTATION PROPERTY AS A REPOSITORY PROPERTY IS WRONG
HERE.** This environment, named `agent-ops-operator`, runs true cloud-side
too.

Setup:
```sh
export CLAUDE_CODE_REMOTE=1
cd /home/user/agent-ops-operator && bash .github/scripts/cloud-bootstrap.sh
```

| Installs | Absent, breaks |
|---|---|
| helm | chart render tests SKIP |
| `@fission-ai/openspec` | no opsx command |
| pyyaml | `docs-generate.py`/`docs-task-guard.py` |
| envtest assets → `~/.envtest` | manager integration suite |
| serena | `.mcp.json` stdio server |
| Go (if below floor) | `platform/manager`, `runtimes/ollama` |

No-ops unless `CLAUDE_CODE_REMOTE=1/true`. Vars: `SONAR_ORG`,
`KUBEBUILDER_ASSETS`. A missing tool is named on stderr only. `--verify` is
the `SessionStart` hook.

**THE CLONE IS THE WORKING COPY. NEVER ADD A WORKTREE.**

```sh
git checkout -b change/<name> origin/master
```

Isolation already exists. A worktree beside it doubles the component
inventory. Pushes any unprotected branch with no foreign open PR/commits
(`allowed_push_branches: change/*`).

**A LABEL STARTS OR DRIVES ONE. THE SESSION PLACES NONE.**

| Label | On | Starts |
|---|---|---|
| `conveyor:implement` | issue | implement, open PR, stop |
| `conveyor:run` | issue | standing instruction, carried forward |
| `conveyor:fix` | PR | fixing loop |
| `conveyor:archive` | tracking issue of merged PR | archive station, `Closes #<n>` |
| `station:<x>`/`loop:<x>` | issue/PR | nothing — state only |
| `conveyor:keep-going` | capped PR | another round set |

Write access only, checked via the collaborators API — anyone else's label
is removed. The session opens its PR with NO LABEL and `Refs #<n>`, never
`Closes`.

- **A PROGRAM CARRIES A GRANT OR CONSUMES ONE. NEVER MINTS ONE** (#201: a
  self-labelled session was refused for having no write access). A workflow
  re-checks and carries the label instead, on `workflow_run` after `ci`.
- One session at a time. The platform's own auto-fix stays OFF.

**WORKSTATION-ONLY:** local cluster/`helmfile sync`/deploy, the visual
check, the system pack by hand. A remote session records these as not
performed, not done. Cluster tier is DISPATCHED
(`gh workflow run system-smoke.yml --ref change/<name>`), not carried.

**VARIABLES ARE PUBLIC, A SECRET IS AN API CREDENTIAL.** A credential goes
under API credentials instead. `SONAR_ORG` is a plain variable.
