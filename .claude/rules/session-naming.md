## Session naming (say what this window is doing)

**NAME THE SESSION `<phase> <change>`, AND RENAME IT WHENEVER EITHER CHANGES.**
Several of these run at once, in several windows and over Remote Control, and a
row reading `agent-ops-operator-05` says nothing about which one is mid-migration
and which is idle on a docs tweak.

```sh
.claude/hooks/session-title.sh set 'opsx:apply discoverable-addressing'
```

That writes the title and paints it.

| Hook | Does |
|---|---|
| `UserPromptSubmit`, `Stop`, `SessionStart` | REPAINT at every turn boundary |
| `SessionEnd` | forgets the title |

- **The repaint is required, not belt-and-braces.** Claude Code writes the
  terminal title itself, so a one-off escape does not survive.
- **All four are wired in `.claude/settings.json`.**
- **The script is a silent no-op** with no title file, no terminal or no `jq`,
  so a hook never becomes an error.

**BOTH LIVE IN THIS REPO AND ARE COMMITTED**, so a clone gets the behaviour and
the rule that names it in one checkout. `$CLAUDE_PROJECT_DIR` is what keeps the
wiring path-independent.

- **Deliberately NOT `~/.claude`.** The rule is about THIS repo's changes and
  its opsx phases, and a title convention that only exists on the machine that
  wrote it is a rule nobody else follows.
- **The title file itself stays per-session under `$XDG_RUNTIME_DIR`**, never in
  the repo. It is scratch keyed by session id, not configuration.

- **The PHASE is the opsx verb driving the work** — `opsx:explore`,
  `opsx:propose`, `opsx:update`, `opsx:apply`, `opsx:archive`.
- **The CHANGE is its directory name** under `openspec/changes/`.
- **Set it at the START and again at every transition**, never once at the end.
  The title exists to be read while the work is still running.
- **Work with no change behind it says what it is**, in the same two-word shape:
  `review chart-values`, `debug telegram-409`, `docs installation`.
- **A title reading only `claude` is the failure this rule names.**

**The script moves the TERMINAL TITLE ONLY.** Two other names exist and neither
is reachable from inside a session:

| name | where it shows | who sets it |
|---|---|---|
| terminal title | the window or tab | this script, every turn |
| session display name | prompt box, `/resume` picker, terminal title | the USER: `/rename <name>`, or `claude -n "<name>"` at launch |
| peer name (`agent-ops-operator-05`) | `ListAgents`, Remote Control rows | nobody — derived from the directory |

- **When a window's name matters anywhere but its own title bar, ASK for
  `/rename`** rather than reporting a rename that did not happen.
- **`terminalTitleFromRename` governs the terminal-title half**, defaults ON,
  and therefore beats this script until the next repaint.
