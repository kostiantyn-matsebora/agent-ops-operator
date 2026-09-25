# Prototype — the conversations view

Static HTML, one file per board. Open any of them in a browser.

| File | Shows |
|---|---|
| `C-rail.html` | the chosen direction: icon rail, inbox column, list, thread, splitters |
| `C-collapsed.html` | the same with the inbox collapsed to icons and a pane mid-resize |
| `D-incident.html` | a coordination: the tree in the list, the incident timeline in the thread |
| `States.html` | the unread rule by message kind, row states, thread cues, selection mode, the row menu |
| `A-inbox.html` | not chosen: two-pane inbox |
| `B-peek.html` | not chosen: table with a peek panel and open tabs |

**Composition is settled here, behaviour in `../specs/`.** Geometry, timings,
spacing and wording are read from these files and written nowhere else.
`design.md` names where the prototype is deliberately not the target.
