## 1. The move

- [x] 1.1 Record the current derived inventory — `.github/components.sh images | jq -r '.[].component' | sort` — so every later check compares against what is published today rather than against a hand-written list
- [x] 1.2 `git mv` the five signal adapters into `signals/`, keeping every leaf name, and verify `components.sh images` still lists the same five component names
- [x] 1.3 `git mv channel-telegram` into `channels/` and `gateway-telegram` into `gateways/`, and verify both component names are unchanged
- [x] 1.4 `git mv` `console`, `housekeeping`, `context-sync` and `egress-proxy` into `platform/`, and verify their component names are unchanged
- [x] 1.5 `git mv runtime-claude` into `runtimes/`, and verify its component name is unchanged

## 2. The manager

- [x] 2.1 `git mv` `api/`, `cmd/`, `config/`, `internal/`, `go.mod`, `go.sum`, `Dockerfile` and `.dockerignore` into `platform/manager/`, leaving the module path in `go.mod` unchanged — and verify `go build ./... && go vet ./...` passes from the new directory
- [x] 2.2 Verify the manager image builds from its own directory as the context (`docker build platform/manager/`), which is the property `COPY ../api` would have broken
- [x] 2.3 Verify the envtest suite still passes from the new location with `KUBEBUILDER_ASSETS` set, and that it still FAILS without it rather than skipping
- [x] 2.4 Verify `controller-gen` still regenerates deepcopy and CRDs into `chart/files/crds` from the new location, and that the regenerated files are identical to the committed ones

## 3. The derivations

- [x] 3.1 Delete the `if [ "$dir" = "." ]` branch in `.github/components.sh` so the component is always its directory's basename — and verify the derived image list equals the list recorded in 1.1, exactly
- [x] 3.2 Verify `components.sh modules` lists all eleven submodules plus `platform/manager` at their new paths, and that each entry is a directory that contains a `go.mod`
- [x] 3.3 Point `ci.yml`'s envtest job at `platform/manager`, and exclude that module from the per-module matrix so it is tested once, with assets — verify by reading the workflow's own assertion that assets were provisioned
- [x] 3.4 Rewrite the eleven `gomod` paths and the npm path in `.github/dependabot.yml`, and verify each named directory exists

## 4. The repository's own context

- [x] 4.1 Rewrite the repository map in `.claude/rules/structure.md` around the five buckets, keeping the counts derived from the tree rather than restated — and verify every path it names exists
- [x] 4.2 Update the `paths:` frontmatter in `.claude/rules/signal-rules.md` and `.claude/rules/palette-and-mark.md`, and verify each glob matches at least one file
- [x] 4.3 Update the scoped-rule routing table in `CLAUDE.md` to the new paths, and verify it matches the frontmatter it describes
- [x] 4.4 Update the build and image commands in `.claude/rules/build-test.md` — the module loop and the `buildx` list — and verify each command runs as written

## 5. Documentation and anchors

- [x] 5.1 Fix the relative links in `docs/contracts.md` to the reference implementations, and verify each resolves to a directory that exists
- [x] 5.2 Correct the module-path anchors in the seven specs under `openspec/specs/` that name one, changing nothing else about those requirements
- [x] 5.3 Update path references in the active changes under `openspec/changes/` so their tasks still match the tree, and leave everything under `openspec/changes/archive/` untouched — it records what was true
- [x] 5.4 Update the `docker buildx build ./<dir>/` comment line each Dockerfile carries, and verify the command in each one names its own directory
- [x] 5.5 Add the changelog entry, stating that no image, chart value or contract changed

## 6. Whole-change verification

- [x] 6.1 Verify every module builds, vets and tests from its new path, and the console UI's typecheck and suite pass
- [x] 6.2 Verify every image builds from its derived context — the same discovery CI uses — and that the set of images equals 1.1
- [x] 6.3 Verify the chart renders and its render tests pass, and that its only diff is the deliberate rename
- [x] 6.4 Verify a `<component>-v<semver>` tag would still resolve for every component, by checking each name against the derived list the release workflow validates against
- [x] 6.5 Verify no file outside `openspec/changes/archive/` still names an old module path
