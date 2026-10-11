## Visual check (look at the UI yourself)

**IF A CHANGE IS VISIBLE, SCREENSHOT IT BEFORE SAYING IT WORKS.** Asking the
person to look is delegation, not verification.

### Against the LIVE install

```sh
kubectl -n agent-ops port-forward svc/agentops-adapter-console 18090:8080 &
cd platform/console/ui
npx playwright screenshot --viewport-size=1400,1100 --wait-for-timeout=5000 \
  --full-page "http://localhost:18090/conversations/<name>" /path/to/out.png
```

Then READ the PNG.

### Against UNRELEASED UI

```sh
kubectl -n agent-ops port-forward svc/agentops-adapter-console 8080:8080 &
cd platform/console/ui && npm run dev &          # :5173, proxy hardcoded to :8080
npx playwright screenshot --wait-for-timeout=6000 \
  "http://localhost:5173/conversations/<name>" /path/to/out.png
```

| Question | Use |
|---|---|
| does the DEPLOYED build render this correctly | the live install |
| does my WORKING TREE render this correctly | the dev server |
| does the PUBLISHED asset show it | `npm run screenshots` (`docs/CLAUDE.md`) |

- **`npm run screenshots`/`npm run demo` are NOT this check** — one curated
  fixture, can miss a real change.
- **Write the PNG to the scratchpad, never the repo.** Not build output.
