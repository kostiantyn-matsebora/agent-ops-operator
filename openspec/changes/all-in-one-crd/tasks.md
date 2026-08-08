## 1. API types

- [ ] 1.1 Add inline wrapper types to `api/v1alpha1/pipeline_types.go`:
      `InlineChannel`, `InlineSignalSource`, `InlineMCPConfig`, `InlineToolset`
      — each a `Name` plus the corresponding spec embedded with
      `json:",inline"`; `InlineProfile` with an optional `Name` (defaults to the
      pipeline name) plus `AgentProfileSpec`
- [ ] 1.2 Constrain `Name` on every inline type: `MinLength=1`, `MaxLength=200`,
      DNS-1123 pattern — so `<pipeline>-<entry>` is always a legal object name
- [ ] 1.3 Add the five spec fields with `+listType=map` / `+listMapKey=name` on
      the four list fields (duplicate rejection, SSA merge, and correlatable
      old values for the inherited `adapter` immutability rule)
- [ ] 1.4 Add `Inline []InlineToolset` / `Inline []InlineMCPConfig` to
      `ToolingBinding`, and relax its `Refs` from `MinItems=1` to optional with
      a CEL rule requiring at least one of `refs` or `inline`
- [ ] 1.5 Make `ProfileRef` optional and add the spec-level CEL rule
      `has(self.profileRef) != has(self.profile)`
- [ ] 1.6 Add `MaterializedRef{Kind,Name}` and `Status.Materialized []MaterializedRef`
- [ ] 1.7 Regenerate deepcopy + CRDs with controller-gen v0.16.5 and confirm the
      generated `channels[].adapter` property still carries the
      `self == oldSelf` XValidation rule inherited from `ChannelSpec`
- [ ] 1.8 Apply the CRD to an envtest/cluster and confirm existing Pipeline
      manifests (refs only) still validate and reconcile unchanged

## 2. RBAC

- [ ] 2.1 Widen `mcptoolsets` in `chart/templates/rbac.yaml` from
      `get,list,watch` to include `create,update,patch,delete`, with a comment
      stating the write scope is toolsets materialized from a Pipeline
- [ ] 2.2 Update `internal/integration/rbac_test.go` to pin the new verb set
      deliberately

## 3. Materialization

- [ ] 3.1 In `pipeline_controller.go`, add a `materialize` step running before
      reference validation: for each inline entry, build the child object named
      `<pipeline>-<entry>` with `ownerRef` to the Pipeline and label
      `agentops.dev/pipeline: <name>`
- [ ] 3.2 Create-or-update semantics: create when absent, patch the spec when it
      differs, and emit a Kubernetes Event on each corrective update so
      hand-edits to owned children are visible in `kubectl describe`
- [ ] 3.3 Name-conflict refusal: if the target exists and has no ownerRef to
      this Pipeline, make no write and set `Ready=False` reason `NameConflict`
      naming the object and its actual owner
- [ ] 3.4 Build the effective ref set (declared refs ∪ materialized names) and
      route the existing validation through it
- [ ] 3.5 Populate `status.materialized` and the `Materialized` condition
      (`False` naming the failing entry when a child cannot be created)
- [ ] 3.6 Add `Owns()` watches for Channel, SignalSource, AgentProfile,
      MCPToolset, MCPConfig so child drift requeues the owning Pipeline

## 4. Pruning and claiming

- [ ] 4.1 Prune step after create/update: list children by the management
      label, verify ownership from `ownerReferences` (never the label alone),
      and delete those no longer named in the spec
- [ ] 4.2 Verify `ownerRef` GC covers whole-Pipeline deletion, and that pruning
      covers single-block removal — these are two distinct paths
- [ ] 4.3 Extend `sourceConflicts` to count inline sources: a materialized
      source is claimed by its declaring Pipeline, so inlining cannot bypass
      one-pipeline-per-source

## 5. Graduation

- [ ] 5.1 Detect `agentops.dev/graduate: "true"` on an owned child: remove the
      ownerRef and management label, drop it from `status.materialized`, and
      stop managing it
- [ ] 5.2 While the inline block remains, set `Ready=False` reason
      `GraduationPending` with a message naming the exact edit that completes
      the swap (remove `spec.channels[<entry>]`, add
      `channelRefs: [{name: <pipeline>-<entry>}]`)
- [ ] 5.3 Ensure no duplicate is created during the handshake and the condition
      self-clears once the swap is applied

## 6. Tests

- [ ] 6.1 envtest: a Pipeline inlining channel + source + profile + toolset in
      an otherwise empty namespace materializes four CRs and reports `Ready`
- [ ] 6.2 envtest: editing an inline block patches the child in place; editing
      the child by hand is corrected and Evented
- [ ] 6.3 envtest: `channels[ops].adapter` change is REJECTED by the API server
      (the inherited CEL transition rule under `listType=map`) — assert
      directly, this is the least standard part of the design
- [ ] 6.4 envtest: removing a block deletes its child; deleting the Pipeline
      GCs all children; a forged management label on an unowned object is not
      deleted
- [ ] 6.5 envtest: name conflict with a hand-made CR leaves it untouched and
      reports `NameConflict`
- [ ] 6.6 envtest: graduation removes the ownerRef, reports `GraduationPending`
      with the remedy, creates no duplicate, and clears on swap completion
- [ ] 6.7 envtest: an inline source is claimed — a second Pipeline referencing
      the materialized source reports `SourceConflict=True`
- [ ] 6.8 envtest: mixing `channelRefs` with inline `channels` binds
      conversations to both
- [ ] 6.9 Assert the Pipeline reconciler performs zero Secret reads while
      reconciling inline blocks that name `credentialsSecretRef` and `valueFrom`
- [ ] 6.10 Full suite green: `KUBEBUILDER_ASSETS=… go test ./...`, plus
      `go build ./... && go vet ./...`

## 7. Docs and samples

- [ ] 7.1 Add a single-file all-in-one Pipeline to `config/samples/samples.yaml`
      next to the decomposed set
- [ ] 7.2 Amend the `CLAUDE.md` Pipeline terminology entry: the Pipeline may
      TEMPLATE content; templates materialize into real CRs which remain the
      only thing anything reads — replacing "all content stays in the referenced
      CRs" while keeping "no credentials, no runtime selection" intact
- [ ] 7.3 Amend the `CLAUDE.md` MCPToolset entry: "Manager RBAC on it is
      read-only" becomes read-only except toolsets it materializes
- [ ] 7.4 Note in `CLAUDE.md` that the Pipeline reconciler now owns child CRs
      (it previously created nothing) and that pruning is reconciler-side while
      GC covers Pipeline deletion
- [ ] 7.5 Document the collapsed spelling in the README / `docs/concepts.md`
      with the rule of thumb — inline what only this pipeline uses, reference
      what is shared — plus the graduation procedure
- [ ] 7.6 Bump the chart version; CRD changes ship as a chart upgrade with no
      manifest migration required
