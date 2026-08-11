{{- /*
The substrate facts, resolved ONCE and readable from every subchart.

Both helpers read `.Values.global` and nothing else, deliberately: a subchart is
rendered with its own context, so a helper reaching for a top-level parent value
would resolve to the subchart's values instead of failing — silently disagreeing
with the parent about the agent's identity or its power. `global.` is the only
scope both contexts share, which is why the canonical keys live there.
*/ -}}

{{- /* The ServiceAccount every runtime pod runs as. Exactly one per release —
its RBAC IS the agent's power, whichever bundle originated the conversation. */ -}}
{{- define "agentops.runtimeServiceAccount" -}}
{{- dig "agentops" "runtime" "serviceAccountName" "" (.Values.global | default dict) | default "agentops-runtime" -}}
{{- end -}}

{{- /* The EFFECTIVE RBAC mode: never empty, always one of none|readonly|full.

Empty means readonly under demo mode and none otherwise. Defaulting it to
readonly outright would silently bind cluster `view` to the runtime SA of every
existing install on upgrade; defaulting it to none would break the demo promise
that one flag yields a working, cluster-reading agent. `full` is never inferred.

The parent's bindings and k8s-bundle's MCP derivation both call this, so they
cannot drift apart. */ -}}
{{- define "agentops.runtimeRbacMode" -}}
{{- $g := .Values.global | default dict -}}
{{- $mode := dig "agentops" "runtime" "rbacMode" "" $g -}}
{{- if not $mode -}}
{{- ternary "readonly" "none" (dig "demo" "enabled" false $g | toString | eq "true") -}}
{{- else if has $mode (list "none" "readonly" "full") -}}
{{- $mode -}}
{{- else -}}
{{- fail (printf "global.agentops.runtime.rbacMode must be \"none\", \"readonly\" or \"full\" (empty = readonly under demo, else none), got %q" $mode) -}}
{{- end -}}
{{- end -}}

{{- /*
THE ONE-TIME MIGRATION GUARD for a chart-generated credential.

A generated Secret now leaves the release manifest on upgrade, retained by
`helm.sh/resource-policy: keep`. Helm reads that annotation off the LIVE object,
not off the manifest that is dropping it — so a Secret created by an earlier
chart, which carries no annotation, is DELETED by the first upgrade that stops
rendering it. Verified against helm v4: unannotated + dropped = gone.

Deleting the console token signs every browser out; deleting the adapter master
token 401s every adapter at once. So the upgrade is REFUSED, naming the one
command that makes it safe, rather than silently destroying the credential this
whole mechanism exists to hold still.

Fires at most once per install, and only where it can act: on a real upgrade,
where `lookup` sees the cluster. A cluster-less renderer prunes nothing, so a
silent guard there is correct.
*/ -}}
{{- define "agentops.generatedSecretGuard" -}}
{{- $root := .root -}}
{{- if and (not .explicit) (not $root.Release.IsInstall) .existingVal -}}
{{- $ann := (.existing.metadata).annotations | default dict -}}
{{- if ne (index $ann "helm.sh/resource-policy" | default "") "keep" -}}
{{- fail (printf "Secret %q in namespace %q holds a chart-generated credential that this chart version no longer renders on upgrade. Helm reads helm.sh/resource-policy off the live object, so dropping it from the manifest DELETES it — and with it every session or adapter credential derived from key %q. Annotate it once, then upgrade again:\n\n  kubectl -n %s annotate secret %s helm.sh/resource-policy=keep\n\nOr take the credential under release management instead by setting %s to its current value." .name $root.Release.Namespace .key $root.Release.Namespace .name .setting) -}}
{{- end -}}
{{- end -}}
{{- end -}}
