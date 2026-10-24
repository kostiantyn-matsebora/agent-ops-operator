Every build and test below runs INSIDE the worktree (`docker exec -w "$PWD"`
from `../agent-ops-worktrees/message-addressing`), and every deploy uses
`--state-values-set chartPath=` naming this worktree's `chart/` — the
defaults resolve master and report success against it. The change depends on
`coordinated-agents` phase 3 being on master first.

## 1. Component (design D-A, D-B)

- [ ] 1.1 `platform/analyzer/`: new standard-library module on the shared
      recipe; `.github/components.sh` derives `analyzer` with no duplicate;
      `go build ./... && go vet ./...` green in the container.
- [ ] 1.2 `POST /utterance` handler: request/response types per the spec's
      table; a malformed request is 400 naming the field; a test posts each
      decision kind through a fake model and asserts the shape.
- [ ] 1.3 `mcp-aops` client forwarding the caller's reader token for
      `list_conversations` and the pipeline listing; test asserts the analyzer
      sends no token of its own and refuses a request without one.

## 2. Decision loop (design D-C, D-D, D-E)

- [ ] 2.1 `chatter` interface with `ollama.go` and `openai.go` backends,
      selected by `ANALYZER_LLM_BACKEND`; a unit test per backend against a
      recorded response.
- [ ] 2.2 Prompt builder: claimants, brief projection, pending intent,
      utterance, language; asks for one JSON decision; golden-file test.
- [ ] 2.3 Validation: every name in the model's answer must be in the listed
      set, else `ask`; test with a forged name.
- [ ] 2.4 Pending intents keyed `(surface, speaker)`, TTL `ANALYZER_ASK_TTL`
      default 120 s; tests: an answer resolves the intent, expiry drops it, a
      new question replaces the old.
- [ ] 2.5 Fixture set of utterances (named per the role they exercise, no
      real names) with expected decisions, run against a fake model and, in a
      skippable integration test, against a local Ollama.

## 3. Chart and wall

- [ ] 3.1 `chart/`: Deployment, Service, `analyzer.enabled` (default false),
      `analyzer.llm.{backend,endpoint,model}`, `ANALYZER_ASK_TTL`; NetworkPolicy
      admitting only wired adapters and allowing egress to `mcp-aops` and the
      LLM endpoint; `helm template` with and without; `serviceaccount-guard.py`
      passes (the floor account, no token needed → none mounted).
- [ ] 3.2 Reader-token rendering: a helper that renders
      `channel-reader:<channel>` for a calling adapter, used by no adapter yet
      in this change; render test.
- [ ] 3.3 Smoke from the worktree chart against the local install: post a
      fixture utterance with curl and a rendered reader token; observe
      `originate` naming a Ready Pipeline. Record the verdict, not the text.
- [ ] 3.4 Measure decision latency on the reference VM with two candidate
      models; record the numbers in design.md's Open Questions and pick the
      default.

## 4. Rules and verification

- [ ] 4.1 `.claude/rules/structure.md`: `platform/analyzer`;
      `terminology.md`: utterance, decision, "the analyzer decides, the
      adapter delivers"; `invariants.md`: the analyzer holds no credential
      and delivers nothing.
- [ ] 4.2 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass; record the verdict only.

## 5. Documentation — THE LAST TASK, and it is not optional

### 5a. Reference docs

- [ ] 5a.1 `docs/contracts.md`: the utterance contract, the five decision
      shapes, the forwarded reader token.
- [ ] 5a.2 `docs/concepts.md`: addressing by words beside addressing by form.
- [ ] 5a.3 `docs/security.md`: the component, what it reads, what it cannot
      do; re-run `python3 docs/diagrams/threat-model.py`.
- [ ] 5a.4 `docs/installation.md`: `analyzer.*` values; `docs/CHANGELOG.md`.
- [ ] 5a.5 `python3 .github/scripts/docs-generate.py --check` passes.

### 5b. Adopter site

- [ ] 5b.1 `docs/introduction.md`: the third way to reach a Pipeline.
- [ ] 5b.2 `docs/installation.md`: the component list.
- [ ] 5b.3 `README.md`: the seams line; `wc -l README.md` ≤ 215.
