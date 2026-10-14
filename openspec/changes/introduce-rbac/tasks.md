## 1. RoleGrant CRD and generated code — api-architect

- [ ] 1.1 Add `RoleGrant` type under `platform/manager/api/v1alpha1/` (`spec.channel`, `spec.key`, `spec.role` with the three-value enum, `status.lastModifiedBy`, `status.lastModifiedAt`), namespaced, per `design.md`'s "A new CRD, `RoleGrant`, not `RoleBinding`". Verify it compiles: `go build ./...` in `platform/manager`.
- [ ] 1.2 Add a name-derivation helper beside the type: `metadata.name` is `rolegrant-` plus the lowercase hex SHA-256 of `<channel>\n<key>` truncated to 40 characters, per `specs/access-control/spec.md`'s "One RoleGrant per identity pair". Verify with a unit test asserting determinism, that distinct pairs differ, and that the raw key never appears in the name.
- [ ] 1.3 Regenerate deepcopy and CRD YAML with `controller-gen` per `build-test.md`'s commands. Verify `chart/crds/` gained one new file and `go vet ./...` is clean.
- [ ] 1.4 Add the manager-side role+ownership decision function (hash the inbound `sender` with the existing `OriginReader`-stamping hash for ownership, use `sender` raw for the `RoleGrant` lookup, apply the "no `originReader` means no owner" rule from `design.md`). Verify with table-driven unit tests covering every `(role, ownership-present, ownership-match)` combination from `specs/access-control/spec.md`'s scenarios.

## 2. Manager enforcement — backend-developer

- [ ] 2.1 Add a read-only watch/cache of `RoleGrant`, keyed by `{channel, key}`, wired into `cmd/manager/main.go` alongside the manager's existing informers. Verify with an envtest case that a created/updated `RoleGrant` is observable through the lookup within the existing watch latency this manager already tolerates elsewhere.
- [ ] 2.2 Enforce the decision function from 1.4 on `POST /channel/inbound` in `internal/httpapi`. Verify with the scenarios in `specs/channel-adapter-contract/spec.md`'s `## MODIFIED Requirements`: a `user`-role sender refused on a foreign conversation, accepted on an ownerless one, and never refused for `responder`/`administrator`.
- [ ] 2.3 Apply the same check when addressing an EXISTING conversation in `internal/chat` (`router.go`'s `HandleCommand`), leaving addressing a Pipeline fresh ungated, per `specs/chat-addressing-discovery/spec.md`. Verify with an integration test asserting a `user`-role sender's address into someone else's thread is refused while starting a new one is not.
- [ ] 2.4 Add manager RBAC (`get`/`list`/`watch` only, never write) on `RoleGrant` to the manager's chart-rendered role. Verify `kubectl auth can-i create rolegrants --as=system:serviceaccount:<ns>:agentops-manager` reports `no` against a rendered chart.

## 3. Console backend: control-plane route and siloed reads — backend-developer

- [ ] 3.1 Add the `/api/control-plane/...` route namespace in `platform/console`, gated to `administrator` only, with role assignment (list/create/update `RoleGrant`) as its first handler. Verify with a handler test asserting `responder` and `user` both get 403, and another asserting a create for an identity pair that already has a `RoleGrant` is refused and the existing object is updated instead.
- [ ] 3.2 Resolve the requester's role in `auth.go` alongside the existing `Identity(r)` resolution, defaulting to `user` when no `RoleGrant` matches. Return the resolved role in the `GET /api/session` response (`handleSession` in `auth.go`) so the UI can read it. Verify with a unit test covering the token-identity-is-administrator bootstrap and the external-auth chart-seeded admins path.
- [ ] 3.3 Filter conversation list and detail (`conversations.go`, `convapi.go`) to the requester's own `{channel: "console", key}` for `user`, unfiltered for `responder`/`administrator`. Verify with the scenarios in `specs/console-application/spec.md`'s `## ADDED Requirements`.
- [ ] 3.4 Hide (never anonymize) conversation nodes a `user` cannot see from Topology's Model view, and omit their pods from the Infrastructure view (`topology.go`). Verify with the scenarios in `specs/console-topology/spec.md`.
- [ ] 3.5 Extend the console's write-action log line (`console write: action=%s identity=%s path=%s`) to cover role assignment, and stamp `status.lastModifiedBy`/`.lastModifiedAt` on every `RoleGrant` write. Verify with a test asserting both are present after a write.
- [ ] 3.6 Enforce the role server-side on the console's bulk-close route (`responder` or `administrator`) and bulk-delete route (`administrator`), refusing with 403 naming the required role, per `specs/access-control/spec.md`'s bulk requirement. Verify with a handler test asserting `user` is refused bulk-close and `user` and `responder` are refused bulk-delete.
- [ ] 3.7 Keep the existing `a.write()` gate on `handleSend` etc. as a fail-fast check (now also consulting role for ownership), documented in-code as a courtesy ahead of the manager's authoritative check from 2.2. Verify existing write-gate tests still pass unmodified in intent.

## 4. Chart and RBAC — deployment-engineer

- [ ] 4.1 Add `rolegrants` to the console's existing read-only (`get`/`list`/`watch`) resource list, and add `create`/`update` on `rolegrants` and `update` on `rolegrants/status` as a separate rule, in the console's namespaced `Role` in `chart/templates/console.yaml`, adding no verb to any other kind. Verify with `helm template` diffing only the expected block.
- [ ] 4.2 Add a chart value for bootstrap administrators under external authentication (e.g. `console.rbac.bootstrapAdmins`), read once, documented in its own values comment. Verify with a chart render test asserting the value reaches the console's config as designed.

## 5. Console UI: role assignment — frontend-developer

- [ ] 5.1 Add an `administrator`-only role-assignment page in `platform/console/ui` calling the `/api/control-plane/...` routes from 3.1. Verify by screenshot against the dev server per `visual-check.md`, showing the role list and a working promote/demote action.
- [ ] 5.2 Hide bulk-delete and role-assignment affordances from `user` and `responder` sessions, and bulk-close from `user` alone, reading the role from `GET /api/session`, which 3.2 extends. Verify by screenshot for each of the three roles.
- [ ] 5.3 Confirm the Conversations list and Topology pages render correctly for a `user` session seeing only their own conversations, with no foreign nodes in Topology. Verify by screenshot per `visual-check.md`.

## 6. Unit tests

- [ ] 6.1 Run `go test ./...` in `platform/manager`, covering the new type, the decision function, the watch/cache, and the httpapi/chat enforcement changes.
- [ ] 6.2 Run `go test ./...` in `platform/console`, covering the new route namespace, role resolution, and the list/detail/topology filtering.
- [ ] 6.3 Run the chart's render tests covering the new RBAC verbs and the new chart value.
- [ ] 6.4 Run `npm run test` (or the UI's existing test command) in `platform/console/ui` covering the new role-assignment page and the per-role affordance hiding.

## 7. System tests

- [ ] 7.1 Extend a lane in `platform/manager/test/system/` asserting the console's ServiceAccount, under the real chart-rendered namespaced `Role`, can `get`/`list`/`watch`/`create`/`update` `RoleGrant` against a live API server and is refused `delete` and every verb on every other kind it did not already hold. This is an RBAC question a real cluster decides, per `change-tests.md`.
- [ ] 7.2 Extend the same or a sibling lane asserting the manager's informer picks up a `RoleGrant` created or updated live, and that a `user`-role sender's reply to a foreign conversation is refused end to end (adapter to manager to refusal), covering the case this change's own write path uniquely exercises.

## 8. Documentation

### Reference docs
- [ ] 8.1 Update `docs/concepts.md` with the `RoleGrant` CRD and the three roles.
- [ ] 8.2 Update `docs/contracts.md` with the `POST /channel/inbound` refusal case and the `/api/control-plane/...` route namespace.
- [ ] 8.3 Update `docs/configuration.md` with the bootstrap-admins chart value, since it is a reversible parent chart value.
- [ ] 8.4 Re-run `python3 .github/scripts/docs-generate.py` since this change adds a CRD field and a chart value, and verify it reports clean.
- [ ] 8.5 Record in `docs/installation.md` and `docs/CHANGELOG.md` that `kubectl apply -f chart/crds/` is required on upgrade for the new CRD, per the Migration Plan in `design.md`. `chart/templates/NOTES.txt` gets no entry, since it carries no per-CRD upgrade notes.

### Adopter site
- [ ] 8.6 Update `docs/console-guide.md` with a role-assignment section alongside the existing authentication-decision section.
- [ ] 8.7 Correct `docs/console.md`'s "there is no write path to the Kubernetes API anywhere in the module" claim, as a stated reversal naming what changed and why, not a silent edit.
- [ ] 8.8 Update `docs/security.md` with the new trust-boundary entry: the console can now mutate who is an administrator, and what bounds that, per `design.md`'s Risks section. Re-run `threat-model.py`, since this moves a trust boundary.
- [ ] 8.9 Re-run `npm run screenshots` and `npm run demo` in `platform/console/ui`, since this change alters the console UI.
