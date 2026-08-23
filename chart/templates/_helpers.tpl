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
