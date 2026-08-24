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

{{- /* MAY AN ACTING AGENT CAUSE A POD TO EXIST, OR ENTER ONE?

DEFAULT FALSE, and this is the flag that decides whether "no runtime identity
may read Secrets" is TRUE or merely WRITTEN DOWN.

THE RBAC RULES REFUSING `secrets` DO NOT SURVIVE POD EXECUTION. The KUBELET
resolves a Secret when it builds a pod, so an agent that can create a pod
mounting one — or exec into a pod that already has one — reads the value having
never asked the API server for a Secret. `secrets: get` is never evaluated.
Demonstrated on a live cluster against this very role: pod created, pod log
read, secret value returned, every one of the seven `secrets` verbs denied
throughout.

SO THE GATE IS "MAY A POD BE PRODUCED OR ENTERED", NOT "MAY A POD BE CREATED".
Gating `pods: create` alone closes nothing: `create` on a Job, Deployment,
StatefulSet or DaemonSet produces a pod spec that can reference a Secret, and
`update`/`patch` on an existing one edits the pod template to add the mount.
Every one of those is the same path with more steps.

WHAT AN AGENT KEEPS WITH THIS OFF, which is still a real operator: it reads
everything it is scoped to, scales workloads, deletes and evicts pods, cordons
nodes, deletes workloads, and creates or edits ConfigMaps, Services, Ingresses,
NetworkPolicies, PDBs, HPAs and PVCs. What it loses is the ability to RUN NEW
CODE — which is the same thing as the ability to read a Secret. */ -}}
{{- define "agentops.runtimePodExecutionAllowed" -}}
{{- $g := .Values.global | default dict -}}
{{- dig "agentops" "runtime" "allowPodExecution" false $g | toString -}}
{{- end -}}

{{- /* THE ACCOUNT A MODE RENDERS, named for the posture it holds.

`global.agentops.runtime.serviceAccountName` is the FLOOR — it holds no RBAC in
any mode, and it is what a Pipeline naming nothing runs as. So a mode cannot
bind to it: that would make SILENCE MEAN MAXIMUM, which is the defect this
whole change exists to remove, one level up from `cluster-admin`.

A mode therefore renders its OWN account and a route opts in BY NAME. The name
states the posture, so `serviceAccountName: agentops-runtime-acting` on a
Pipeline is readable without resolving anything.

Empty for mode `none`: there is no posture to name. */ -}}
{{- define "agentops.runtimeModeServiceAccount" -}}
{{- $mode := include "agentops.runtimeRbacMode" . -}}
{{- if eq $mode "none" -}}
{{- else -}}
{{- printf "%s-%s" (include "agentops.runtimeServiceAccount" .) (ternary "acting" "readonly" (eq $mode "full")) -}}
{{- end -}}
{{- end -}}

{{- /* THE ACTING GRANT — cluster-scoped, in two lists.

  agentops.runtimeReadRules   -> what an agent may see
  agentops.runtimeWriteRules  -> what it may change, gated by allowPodExecution

BOTH ARE CLUSTER-WIDE, INCLUDING THIS RELEASE'S OWN NAMESPACE. That is a
deliberate choice, and what makes it defensible is what is NOT here:

  * `agentops.dev` IS NEVER GRANTED, in any rule, so Conversations, Pipelines
    and profiles are unreadable everywhere. RBAC is deny-by-default and nothing
    below names that group.
  * `secrets` is never granted either.
  * No component in this release logs message content, so the pod logs an agent
    can read carry no conversation text.

What an agent therefore sees in the operator's own namespace is pod names and
specs (credentials are `valueFrom` references, not values), Services, events and
the compiled MCP ConfigMaps. None of it is sensitive.

WHAT IT COSTS, AND IT IS REAL: an agent can delete or patch workloads in this
namespace too — the manager, the adapters, the MCP servers. It can restart its
own supervisor. NOTES.txt says so at install time.

THE ALTERNATIVE WAS 224 OBJECTS. Namespaced Roles were tried: RBAC cannot say
"everywhere except", so bounding reads meant an allow-list, one binding per
namespace per account, and a new namespace invisible to the agent until someone
edited values and redeployed. The maintenance was worse than the exposure.

FOUR RULES THE READ LIST MUST KEEP, each of which is how such a role usually
fails:

  1. NO VERB ON `secrets`. Not get, not list, not watch. The manager itself
     holds none — everything secret-shaped compiles to `valueFrom` and the
     kubelet resolves it — and the component running untrusted model output must
     not out-rank the component orchestrating it.
  2. NO `*` in `resources` or `apiGroups`. A wildcard reaches Secrets without
     naming them, which is how a role passes review and fails its purpose.
  3. NO `escalate` or `bind` on rbac.authorization.k8s.io, or the role widens
     itself and rule 1 becomes advisory.
  4. NO write on rbac or CRDs, and NO read of `clusterroles` — that listing is a
     map of every identity in the install and which one is worth attacking.
     Namespaced `roles` stay readable so an agent can explain a Forbidden.

The WRITE list mirrors what the `k8s-admin` toolset advertises, so the allowlist
and the grant agree. A toolset promising a tool while RBAC refuses it is worse
than either wall alone: the agent reports a Forbidden for something it was told
it could do, and the operator debugs the wrong layer. That shipped once — four
of k8s-admin's six tools 403'd on a live cluster.

Shared with k8s-bundle's MCP server, exactly as agentops.runtimeRbacMode is, so
the two walls cannot drift into disagreeing about what `full` means. */ -}}
{{- define "agentops.runtimeReadRules" -}}
- apiGroups: [""]
  resources: ["pods", "pods/log", "pods/status", "services", "endpoints", "configmaps",
              "persistentvolumeclaims", "persistentvolumes", "namespaces", "nodes",
              "nodes/status", "events", "serviceaccounts", "replicationcontrollers",
              "limitranges", "resourcequotas"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets", "controllerrevisions"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses", "networkpolicies", "ingressclasses"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses", "volumeattachments", "csidrivers", "csinodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["discovery.k8s.io"]
  resources: ["endpointslices"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["events.k8s.io"]
  resources: ["events"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["nodes", "pods"]
  verbs: ["get", "list"]
# THE KUBELET's own endpoints, reached through the API server. `nodes_log` and
# `nodes_stats_summary` need this and nothing else provides it — they worked
# before only because this account held cluster-admin. Broader than the reads
# around it, and named so that is a decision rather than an accident.
- apiGroups: [""]
  resources: ["nodes/proxy"]
  verbs: ["get"]
# NAMESPACED RBAC ONLY, so an agent can explain a Forbidden. `clusterroles` is
# deliberately absent: it maps every identity in the install.
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings"]
  verbs: ["get", "list", "watch"]
{{- end -}}

{{- define "agentops.runtimeWriteRules" -}}
{{- $podExec := eq (include "agentops.runtimePodExecutionAllowed" .) "true" -}}
# Restart a pod by deleting it, evict one the way a drain does, and cordon a
# node. None of these reads anything, so all are unconditional.
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["delete"]
- apiGroups: [""]
  resources: ["pods/eviction"]
  verbs: ["create"]
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["patch"]
{{- if $podExec }}
# GATED — global.agentops.runtime.allowPodExecution. `pods_run` and `pods_exec`,
# and `patch` on a pod, are each a way to read any Secret in the namespace
# without ever asking the API server. See the helper above.
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["create", "patch"]
- apiGroups: [""]
  resources: ["pods/exec"]
  verbs: ["create"]
{{- end }}
# Delete a workload unconditionally — it produces no pod spec and reads nothing.
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
  verbs: ["delete"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["delete"]
{{- if $podExec }}
# GATED for the same reason: creating or editing a workload writes a POD SPEC,
# and a pod spec can mount any Secret in its namespace. Gating `pods: create`
# and leaving this open would close nothing.
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
  verbs: ["create", "update", "patch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["create", "update", "patch"]
- apiGroups: [""]
  resources: ["serviceaccounts", "replicationcontrollers"]
  verbs: ["create", "update", "patch", "delete"]
{{- end }}
# Scale. The subresource is named on its own so that scaling does not require
# the broader update above on installs that trim this list.
- apiGroups: ["apps"]
  resources: ["deployments/scale", "statefulsets/scale", "replicasets/scale"]
  verbs: ["get", "update", "patch"]
# These produce no pod and mount no Secret, so they are ungated: fixing a
# ConfigMap, a Service or an Ingress is most of what an operator is asked for.
- apiGroups: [""]
  resources: ["configmaps", "services", "persistentvolumeclaims",
              "limitranges", "resourcequotas"]
  verbs: ["create", "update", "patch", "delete"]
- apiGroups: ["networking.k8s.io"]
  resources: ["ingresses", "networkpolicies"]
  verbs: ["create", "update", "patch", "delete"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["create", "update", "patch", "delete"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["create", "update", "patch", "delete"]
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

{{- /*
THE STORAGE CLASS CONVENTION, implemented once for both claims.

| Value | Renders |
|---|---|
| undefined or empty | no field — the cluster's default provisioner |
| `-` | `storageClassName: ""` — no class, bind to a pre-created volume |
| a name | that class |

The `-` case is the whole reason this helper exists. Binding a claim to a
pre-created PersistentVolume requires an EXPLICIT empty storage class: an ABSENT
field is filled in by the admission plugin from the cluster's default
StorageClass, which then provisions a second volume and leaves the operator's
untouched. Before this there was no spelling of "no storage class" at all.

`-` is the magic string prometheus-community, Bitnami and most charts already
use, taken deliberately rather than invented: an operator arriving from any of
them already knows it. It is ADDITIVE — empty keeps meaning "default
provisioner", exactly as this chart's own shipped `storageClassName: ""` always
has, so no existing install changes behaviour.

Call with the volume's values block; emits nothing at all when there is no
class to state.
*/ -}}
{{- define "agentops.storageClassName" -}}
{{- $sc := .storageClassName | default "" -}}
{{- if eq $sc "-" }}
  storageClassName: ""
{{- else if $sc }}
  storageClassName: {{ $sc | quote }}
{{- end -}}
{{- end -}}

{{- /*
THE CONTEXT CLAIM RENAME GUARD.

`agentops-home` became `agentops-context` with the volume's name, and NOTHING
COPIES A VOLUME. An upgrade that adopted the new default unremarked would
provision a second, empty claim and every conversation in the install would
answer without its context while every signal reported success — which is worse
than a failed upgrade in the one way that matters: a failed upgrade is
recoverable.

So the render is REFUSED, naming the one values line that fixes it. Same
technique as `agentops.generatedSecretGuard`, against the same class of problem.

THE LIMITATION, NAMED: `lookup` returns empty on any renderer without a cluster
— `helm template`, CI, a GitOps controller — so an Argo install upgrades
straight past this. `docs/CHANGELOG.md` is written as if it is the only warning
that arrives, because for those installs it is.
*/ -}}
{{- define "agentops.contextClaimRenameGuard" -}}
{{- $root := .root -}}
{{- $ctx := $root.Values.persistence.context -}}
{{- $resolved := $ctx.existingClaim | default $ctx.name -}}
{{- if and (not $root.Release.IsInstall) $ctx.enabled (ne $resolved "agentops-home") -}}
{{- if lookup "v1" "PersistentVolumeClaim" $root.Release.Namespace "agentops-home" -}}
{{- if not (lookup "v1" "PersistentVolumeClaim" $root.Release.Namespace $resolved) -}}
{{- fail (printf "PersistentVolumeClaim \"agentops-home\" exists in namespace %q and this chart version no longer renders it: the context volume was renamed, and its default claim is now %q. Nothing copies a volume, so upgrading as-is would provision a second, EMPTY claim and every conversation in this install would answer without its accumulated context while every signal reported success. Keep the volume you have — this moves no data and is one line:\n\n  persistence:\n    context:\n      existingClaim: agentops-home\n\nOr, better, REBIND the volume under the new name — retain the PV, clear its claimRef, delete the old claim, and set persistence.context.volumeName to the PV. MATCH ITS STORAGE CLASS: a PV that was dynamically provisioned KEEPS its class, and a claim requesting a different one is refused with VolumeMismatch, so set persistence.context.storageClassName to that class. Only a statically created PV with no class takes \"-\"." $root.Release.Namespace $resolved) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /*
THE RETIRED PERSISTENCE KEYS GUARD.

The `persistence` block moved wholesale under `persistence.context`. A values
file still supplying the flat keys would be SILENTLY IGNORED — and the worst
case is the quiet one: `persistence.enabled: false`, written by an operator
whose cluster has no RWX provisioner, becomes a provisioned claim that sits
Pending and no runtime pod ever schedules behind.

This one needs no cluster, so unlike the rename guard it fires under
`helm template`, in CI and under a GitOps controller too.
*/ -}}
{{/*
agentops.retiredRuntimeVolumeKeysGuard — the runtime no longer names a volume,
and a values file that still does gets told rather than ignored.

SAME CLASS AS THE BLOCK GUARD BELOW, AND FOR THE SAME REASON: Helm reports no
unread values key, so the alternative is silence. The quiet case here is the
expensive one — an operator who deliberately pointed the runtime at a claim the
chart did not create keeps every signal of success while the release-wide claim
is used instead, and every conversation on that install answers out of the wrong
volume.

It needs NO CLUSTER, so unlike the claim-rename guard it also protects a GitOps
install.
*/}}
{{- define "agentops.retiredRuntimeVolumeKeysGuard" -}}
{{- $rt := .Values.runtime -}}
{{- $retired := list -}}
{{- range $k := list "contextPvcRef" "homePvcRef" "workspacePvcRef" -}}
{{- if hasKey $rt $k -}}
{{- $retired = append $retired (printf "runtime.%s" $k) -}}
{{- end -}}
{{- end -}}
{{- if $retired -}}
{{- fail (printf "These values are GONE, not renamed: %s. An AgentRuntime declares no volume at all — persistence is WIRING, and it moved to the Pipeline. Set it release-wide under persistence.context / persistence.workspace, which reach every conversation with nothing restated; or per route under pipelines[].persistence.context.claimName (or .volumeName) for a route that keeps its state somewhere of its own. A runtime CR still carrying the retired field contributes no volume after this upgrade." (join ", " $retired)) -}}
{{- end -}}
{{- end -}}

{{- define "agentops.retiredPersistenceKeysGuard" -}}
{{- $p := .Values.persistence -}}
{{- $retired := list -}}
{{- range $k := list "enabled" "name" "size" "storageClassName" "accessModes" "volumeName" "existingClaim" -}}
{{- if hasKey $p $k -}}
{{- $retired = append $retired (printf "persistence.%s" $k) -}}
{{- end -}}
{{- end -}}
{{- if $retired -}}
{{- fail (printf "These values moved under persistence.context and are no longer read: %s. Rewrite them there — for example persistence.enabled becomes persistence.context.enabled. If this install already holds an agentops-home claim, set persistence.context.existingClaim: agentops-home as well, which keeps the volume you have and copies nothing." (join ", " $retired)) -}}
{{- end -}}
{{- end -}}
