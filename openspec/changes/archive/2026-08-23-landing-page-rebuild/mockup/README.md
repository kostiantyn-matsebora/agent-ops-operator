# Mockup

`landing.html` is the agreed composition, working: the presentation runs, the
tabs switch, both themes resolve.

**It is a mockup, not a source.** What ships is authored against the specs. This
exists so the shape the change was agreed on cannot be lost to a link, and so an
implementer can step the presentation beat by beat rather than infer it from
prose.

## Opening it

It loads the repository's own marks, recording and poster by relative path, so
it must be served from the repository root rather than opened from this
directory:

```sh
python3 -m http.server -d . 8000
# then open
# http://localhost:8000/openspec/changes/landing-page-rebuild/mockup/landing.html
```

```powershell
python3 -m http.server -d . 8000
# then open
# http://localhost:8000/openspec/changes/landing-page-rebuild/mockup/landing.html
```

Using the real assets is deliberate: a mockup carrying its own copies would
drift from `npm run demo`'s output the first time the console's UI changed.

## What is annotation, not design

The violet-ruled notes above each band say what that band replaces and why. They
are commentary for the reviewer and are not part of the page being proposed.
The closing table costs each band and is commentary too.
