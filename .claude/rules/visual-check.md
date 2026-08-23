## Visual check (look at the UI yourself)

**IF A CHANGE IS VISIBLE, SCREENSHOT IT BEFORE SAYING IT WORKS.** Playwright is
already a dev dependency of `platform/console/ui`, and one command produces a
PNG that can be read directly.

**Asking the person to look is not verification, it is delegation.** A source
chip was reported missing five times while the code was correct — the field it
renders was simply empty on every conversation that predated it. One screenshot
would have shown an empty chip row instead of a broken component, and the
diagnosis would have started at the DATA.

### Against the LIVE install

```sh
# 1. reach the console directly. This bypasses oauth2-proxy, which is why it
#    needs no credential — the port-forward IS the authentication.
kubectl -n agent-ops port-forward svc/agentops-adapter-console 18090:8080 &

# 2. shoot. --wait-for-timeout is not optional: it is an SPA, and without it the
#    capture is an empty shell.
cd platform/console/ui
npx playwright screenshot --viewport-size=1400,1100 --wait-for-timeout=5000 \
  --full-page "http://localhost:18090/conversations/<name>" /path/to/out.png
```

Then READ the PNG. That is the whole point — the file is an image the model can
open, so a rendering claim is checked rather than asserted.

### Against UNRELEASED UI

The live console serves the image's embedded bundle, so a working-tree change is
invisible there until it is built and deployed. Use the dev server, which
proxies its API to a port-forwarded console:

```sh
kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080 &
cd platform/console/ui && npm run dev &          # :5173, proxy is hardcoded to :8080
npx playwright screenshot --wait-for-timeout=6000 \
  "http://localhost:5173/conversations/<name>" /path/to/out.png
```

- **The proxy target is `localhost:8080` and is hardcoded** in `vite.config.ts`,
  so the port-forward must be on 8080. Any other port silently yields a console
  with no data.

### Both, and they answer different questions

| Question | Use |
|---|---|
| does the DEPLOYED build render this correctly | the live install |
| does my WORKING TREE render this correctly | the dev server |
| does the PUBLISHED asset show it | `npm run screenshots` — see below |

**`npm run screenshots` and `npm run demo` are NOT this.** They render one
curated fixture for the site and are build output. A change that renders
correctly against real data can still be absent from them, because the fixture
does not exercise it — the fixture is a separate thing to update, and
`docs/CLAUDE.md` owns that.

### Write the PNG to the scratchpad, never the repo

A verification capture is not build output and must not be committed. The site's
assets come from `npm run screenshots` alone.
