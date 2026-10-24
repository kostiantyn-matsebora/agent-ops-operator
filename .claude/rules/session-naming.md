## Session naming (say what this window is doing)

**NAME THE SESSION `<phase> <change>`, RENAME WHENEVER EITHER CHANGES.**
Phase is the opsx verb (`opsx:explore/propose/update/apply/archive`), change
is its directory under `openspec/changes/`. No change behind it: say what it
is (`review chart-values`, `debug telegram-409`).

**THE NAME AND THE WORKTREE ARE ONE FACT.** A session named `<phase>
<change>` OWNS that change's worktree, at `../agent-ops-worktrees/<change>/`
on branch `change/<change>`. No change named: the session works the MAIN
checkout. See `worktree-delivery.md`.

**SET THE DISPLAY NAME AT LAUNCH, `-n`/`/rename` ONLY** — it is the one
handle reaching every surface (terminal title, `/resume`, prompt box,
claude.ai, Remote Control):

```sh
claude -n "opsx:apply discoverable-addressing"
```

Mid-session, ASK THE USER for `/rename <phase> <change>` — a session cannot
rename itself.

**CLAUDE CODE OWNS THE TERMINAL TITLE, REPAINTING IT CONTINUOUSLY.** Nothing
external can hold it, so don't rebuild a title-painting hook.

**VS CODE'S INTEGRATED TERMINAL DISCARDS THE TITLE BY DEFAULT.** Fix:

```json
"terminal.integrated.tabs.title": "${process}${separator}${sequence}"
```

placed wherever VS Code says it has the workspace open (check first):

```sh
python3 -c "import json,os;print(json.load(open(os.path.expanduser(
  '~/.config/Code/User/globalStorage/storage.json')))['windowsState'])"
```

### The three names, and who sets each

| name | where it shows | who sets it |
|---|---|---|
| session display name | title, prompt, `/resume`, claude.ai, Remote Control | the USER (`-n`/`/rename`) |
| terminal title | the window/tab | CLAUDE CODE, from the display name |
| peer name (`agent-ops-operator-05`) | `ListAgents` | nobody — derived from the directory |
