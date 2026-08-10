## Why

The chart generates two credentials — the console UI token and the adapter
master token — and both are meant to survive upgrades via `lookup`. On the
reference cluster they do: a no-op `helmfile apply` left
`agentops-console-console` untouched, resourceVersion and all.

But `lookup` returns nothing wherever the renderer has no cluster, so **every
`helmfile diff` reports both Secrets as changed**, on an install where nothing
changed:

```
agent-ops, agentops-adapter-token, Secret (v1) has changed:
-   token: '-------- # (32 bytes)'
+   token: '++++++++ # (32 bytes)'
agent-ops, agentops-console-console, Secret (v1) has changed:
-   uiToken: '-------- # (40 bytes)'
+   uiToken: '++++++++ # (40 bytes)'
```

Two costs, one cosmetic and one not. A diff that always reports a change trains
an operator to stop reading diffs. And any pipeline that renders without a
cluster — `helm template | kubectl apply`, CI, Argo CD, `--dry-run=client` —
does not merely *show* a new token, it **applies** one: everyone signed out of
the console, and every adapter's derived token invalidated at once, since
per-adapter tokens are HMACs of the master.

The documented escape hatch does not work either. `console.auth.uiToken` exists,
but the template checks the existing Secret **first**:

```
{{- if and $existing $existing.data (index $existing.data "uiToken") }}   ← existing wins
{{- else if $c.auth.uiToken }}                                            ← never reached
```

So pinning a token is a silent no-op on any install that already has one — the
value appears to be honoured and is ignored, which is the worst way for a
setting to fail. The adapter token has no explicit value at all; its only option
is bringing a whole Secret.

## What Changes

- **An explicit value wins.** Precedence becomes `explicit → existing →
  generate` for both credentials, so pinning a token takes effect on an install
  that already has one, and rotating means changing a value rather than deleting
  a Secret first.
- **`adapterAuth.token` gains parity** with `console.auth.uiToken` and with the
  `runtime.credentialsSecret.token` / telegram bot-token pattern the chart
  already uses: supply it and the credential is release-managed from your secret
  store; leave it empty and it is generated.
- **A generated credential is never re-rendered on upgrade.** When no explicit
  value is given, the Secret is rendered on **install only** and carries
  `helm.sh/resource-policy: keep`. An upgrade neither regenerates it nor reports
  it as changed — the churn disappears because there is no longer a random value
  produced on the upgrade path at all, rather than because a lookup happened to
  succeed.
- **`NOTES.txt` stops telling you to fetch a token you pinned**, and says which
  of the three sources is in effect.

**Not breaking.** An install that sets nothing keeps the token it already has;
the first upgrade after this change stops managing it rather than changing it.

## Capabilities

### New Capabilities

- `chart-managed-secrets`: how the chart provisions the credentials it generates
  — the three sources and their precedence, stability across upgrades and across
  renderers with no cluster access, and the rule that a rendered manifest must
  never contain a freshly generated value for a credential that already exists.

### Modified Capabilities

- `console-deployment`: the UI token Secret gains a defined source order and a
  stability guarantee; today the spec names the Secret as part of the bundle but
  says nothing about where its value comes from or that it must not rotate.

## Impact

- **Chart**: `templates/adapter-auth.yaml` and `templates/console.yaml`
  (precedence + install-only rendering + keep policy), `values.yaml`
  (`adapterAuth.token`, comments naming the trade-off), `NOTES.txt`.
- **Tests**: `internal/integration/charttemplate_test.go` — an explicit value is
  rendered even when one could exist, no random credential is emitted on an
  upgrade render, and the keep policy is present.
- **Docs**: `docs/console.md` (how to pin the UI token, and that it survives
  deploys), `CHANGELOG.md`.
- **Operational note**: a generated Secret becomes unmanaged after the first
  upgrade — the same trade the CRDs and the persistence claims already make.
  Deleting it by hand then requires an explicit value or a reinstall to restore,
  which the notes will say.
- **Out of scope**: rotating a credential on a schedule, and the runtime's Claude
  credential (already explicit-only — the chart never generates it).
