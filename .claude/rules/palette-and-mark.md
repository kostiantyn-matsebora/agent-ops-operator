---
paths:
  - "platform/console/ui/src/theme/**"
  - "platform/console/ui/src/components/Logo.tsx"
  - "docs/assets/css/**"
  - "docs/assets/js/**"
  - "docs/_includes/logo.svg"
  - "docs/assets/img/logos/**"
---

## The palette and the mark are COPIED, and the copy is one block

**The site's `--ao-*` palette is copied from `platform/console/ui/src/theme/theme.css`**
into the token blocks at the head of `docs/assets/css/agentops.css`. Changing a
token is a TWO-FILE change.

**The copy is deliberate and one-directional** — a Jekyll site must not need a
Node build to publish a paragraph.

**What makes it survivable is that no colour is stated literally anywhere else
in the site CSS**, so the sync is one block, not a hunt:

```sh
grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css
```

That must return hits only inside those blocks.

**The theme-choice semantics are copied on the same terms** —
`assets/js/theme.js` from `theme/useTheme.ts`.

**The MARK is copied on those terms across THREE files:**

| File | Is |
|---|---|
| `platform/console/ui/src/components/Logo.tsx` | the source |
| `docs/_includes/logo.svg` | the masthead's theme-driven copy |
| `docs/assets/img/logos/agent-ops.svg` | the standalone one an `<img>` can load |

The standalone one states its colours literally, because an `<img>` is its own
document and inherits no custom properties.

**Integration marks sit beside it**, committed unaltered from each project's own
source with their terms in that directory's README, and the PAGE names each
file. A vendor list in the stylesheet would be product knowledge in the theme.

### `--ao-*` is shared, `--wk-*` is a look's own

| Prefix | Owned by | Copied to the site |
|---|---|---|
| `--ao-*` | `platform/console/ui/src/theme/theme.css` | yes — `docs/assets/css/agentops.css` |
| `--wk-*` | `platform/console/ui/src/looks/workspace/workspace.css` | never |

**A `--wk-*` token belongs to the `workspace` look alone.** It is not
console-wide, so it never moves to `theme.css`, and it never gets a copy of
its own in the site CSS — not today, and not for a second look later.

**Verify the split by grep**, same spirit as the `#hex` check above:

```sh
grep -n '\-\-wk-' platform/console/ui/src/theme/theme.css docs/assets/css/agentops.css
```

That must return nothing. A hit means a look-owned token leaked into the
shared palette or the site's copy of it.
