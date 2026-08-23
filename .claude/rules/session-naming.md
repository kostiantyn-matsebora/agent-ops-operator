## Session naming (say what this window is doing)

**NAME THE SESSION `<phase> <change>`, AND RENAME IT WHENEVER EITHER CHANGES.**
Several of these run at once, in several windows and over Remote Control, and a
row reading `agent-ops-operator-05` says nothing about which one is mid-migration
and which is idle on a docs tweak.

- **The PHASE is the opsx verb driving the work** — `opsx:explore`,
  `opsx:propose`, `opsx:update`, `opsx:apply`, `opsx:archive`.
- **The CHANGE is its directory name** under `openspec/changes/`.
- **Work with no change behind it says what it is**, in the same two-word shape:
  `review chart-values`, `debug telegram-409`, `docs installation`.

### THE DISPLAY NAME IS THE ONLY HANDLE. SET IT AT LAUNCH

```sh
claude -n "opsx:apply discoverable-addressing"
```

`--name` is documented as setting the name shown in "the picker, and terminal
title" — so ONE flag reaches every surface at once: the terminal title, the
`/resume` picker, the prompt box, the claude.ai row and Remote Control.

**Mid-session, when the phase changes, ASK THE USER for `/rename`:**

```
/rename opsx:apply discoverable-addressing
```

- **A session cannot rename itself.** This is a REQUEST, never something to
  report as done.
- **An unnamed session shows ACTIVITY TEXT** in those rows, so a row reading
  `Interrupted request` means nobody named it — not that anything is broken.
- **A title reading only `claude` is the same failure**, one surface over.

### DO NOT REBUILD THE TITLE HOOK. IT WAS DELETED FOR CAUSE

This repo carried `.claude/hooks/session-title.sh` — an OSC title painter wired
to `UserPromptSubmit`, `Stop`, `SessionStart` and `SessionEnd`. It is GONE, and
the next person to think "a hook could just paint the title" is who this section
is for.

**CLAUDE CODE OWNS THE TERMINAL TITLE AND REPAINTS IT FROM THE SESSION'S OWN
STATE, CONTINUOUSLY WHILE THE SESSION IS ACTIVE.** Nothing painting from outside
can hold it:

| Attempt | Result |
|---|---|
| paint at every turn boundary | overwritten moments later — the title visibly snaps back |
| one detached repaint 1.5s after the boundary | lost too: it lands mid-burst while the session is still transitioning |
| a burst of repaints over ~16s | the tab ALTERNATES between two names. Worse than losing |
| `terminalTitleFromRename: false` in `~/.claude/settings.json` | does not stop it. Verified across a session restart |

**The fix is not to out-write Claude Code, it is to tell it what to write** —
which is the display name, and `-n` / `/rename` are the only things that set it.

**Four sessions were spent on this, and the script was healthy in all four.** It
fired, resolved the tty, and painted correctly every time. What made it look
broken were two failures downstream that are invisible from inside a terminal:

1. **The VS Code tab template** discarded the escape — see below. That one is
   real, and its fix is KEPT.
2. **Claude Code's repaint**, above.

**A HOOK THAT NEVER RAN AND A TITLE THAT NEVER DISPLAYED ARE
INDISTINGUISHABLE.** Anything that paints a title again must log its
invocations, or the next four sessions go the same way.

### A PAINTED TITLE IS NOT A DISPLAYED ONE

Still true, and still worth knowing — Claude Code's own title reaches the tab
only if the tab is willing to show it.

**VS Code's integrated terminal DISCARDS the OSC title sequence by default.** It
captures it as `${sequence}`, but the default tab label is
`terminal.integrated.tabs.title: "${process}"`, which never names it. Every tab
then reads `bash` forever and neither side reports a failure.

```json
"terminal.integrated.tabs.title": "${process}${separator}${sequence}"
```

- **`${process}` stays FIRST**, so a terminal with no title still reads `bash`.
  VS Code elides the separator along with the empty sequence.
- **THE SETTING IS WINDOW-SCOPED**, so where it must live depends on what VS
  Code has open, and both wrong places look identical from inside the terminal:

  | What is open | Where the setting must live |
  |---|---|
  | this repo as the folder ROOT | its own `.vscode/settings.json` |
  | a `.code-workspace` file | that file's own `settings` — a folder cannot supply it |
  | a PARENT folder of this repo | that folder's `.vscode/settings.json`, or user settings |

- **On the machine this repo is developed on it is the second row**, a
  `.code-workspace` over `Projects/` — so the repo's committed
  `.vscode/settings.json` is inert twice over.
- **ASK VS CODE WHAT IT HAS OPEN before editing any settings file:**

  ```sh
  python3 -c "import json,os;print(json.load(open(os.path.expanduser(
    '~/.config/Code/User/globalStorage/storage.json')))['windowsState'])"
  ```

  A `configURIPath` means a workspace FILE holds the setting. A `folderUri`
  means the FOLDER it names does.

### The three names, and who can set each

| name | where it shows | who sets it |
|---|---|---|
| session display name | terminal title, prompt box, `/resume`, claude.ai, Remote Control | the USER: `claude -n "<name>"` at launch, or `/rename <name>` |
| terminal title | the window or tab | CLAUDE CODE, from the display name and session state |
| peer name (`agent-ops-operator-05`) | `ListAgents`, Remote Control rows | nobody — derived from the directory |

**Only the first row is settable, and only by the USER.** That is why naming a
window is a request rather than an action, and why a peer row naming the work is
impossible without a display name behind it.
