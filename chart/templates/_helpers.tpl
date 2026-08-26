{{- /*
The substrate facts, resolved ONCE and readable from every subchart.

Every helper here reads `.Values.global` and nothing else, deliberately: a
subchart is rendered with its own context, so a helper reaching for a top-level
parent value would resolve to the subchart's values instead of failing —
silently disagreeing with the parent about the agent's identity or its power.
`global.` is the only scope both contexts share, which is why the canonical keys
live there.

NAMED TEMPLATES ARE GLOBAL IN HELM, AND THAT IS THE PART NOBODY REMEMBERS. The
`kubernetes` bundle CALLS `agentops.runtimeWriteRules` to build its MCP server's
RBAC, and inside that call only the SUBCHART's `.Values.global` resolves. So the
values these helpers read have to live under `global.` even when the parent is
the only thing that writes them.
*/ -}}

{{- /* THE FLOOR — the ServiceAccount a runtime pod runs as when neither its
Pipeline nor its AgentRuntime names one, and the account this chart ALWAYS
renders and NEVER binds anything to.

It is a CONSTANT rather than a value, because it is the one account whose
meaning is "holds nothing". A configurable floor is a floor somebody can point
at an account that holds something. */ -}}
{{- define "agentops.floorServiceAccount" -}}
agentops-runtime
{{- end -}}

{{- /* THE DEFAULT IDENTITY a runtime declares, and therefore what a Pipeline
naming no `serviceAccountName` inherits.

IT IS A REFERENCE THIS CHART DOES NOT CREATE — naming is not creating, the
posture adapters already have. Its default is the floor above, so an install
that says nothing grants nothing; point it at an account of your own and the
floor is still rendered, which is what keeps `agentops-runtime` available for
naming on one Pipeline to take that route back to nothing. */ -}}
{{- define "agentops.runtimeServiceAccount" -}}
{{- dig "agentops" "runtimeDefaults" "serviceAccountName" "" (.Values.global | default dict) | default (include "agentops.floorServiceAccount" .) -}}
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
CODE — which is the same thing as the ability to read a Secret.

READ FROM `global.agentops.runtimeDefaults`, AND THE READ DOES NOT MOVE. The
`kubernetes` bundle's MCP server role is built by a helper below that calls
this one from SUBCHART context, where only `.Values.global` resolves. Both walls
sit on one path — an agent reaches the cluster THROUGH that server — so they
move together or the hole opens one indirection along. */ -}}
{{- define "agentops.runtimePodExecutionAllowed" -}}
{{- dig "agentops" "runtimeDefaults" "allowPodExecution" false (.Values.global | default dict) | toString -}}
{{- end -}}

{{- /* DOES ANY DECLARED ACCOUNT GRANT ANYTHING AT ALL? Returns "true" or "".

Used only by NOTES.txt. It reads the DECLARED accounts, because that is the only
source of runtime permissions — the floor is bound to nothing in every
configuration, and there is no mode at either level.

IT NO LONGER ANSWERS "CAN AN AGENT ACT", AND THAT IS THE HONEST ANSWER RATHER
THAN A LOST FEATURE. It used to test `rbacMode == "full"`, which the chart could
read because the chart WROTE those verbs. An account now carries rules the
OPERATOR wrote, so summarising them would be this template guessing at YAML it
did not author — and a wrong summary in the one report an operator trusts about
their own grants is worse than no summary. NOTES.txt names the accounts and the
roles; `kubectl describe clusterrole` reads them. */ -}}
{{- define "agentops.anyGrantedAccount" -}}
{{- range $a := (dig "runtime" "serviceAccounts" list (.Values.rbac | default dict)) -}}
{{- if or $a.clusterRoles $a.bindClusterRoles $a.namespaced }}true{{ end -}}
{{- end -}}
{{- end -}}

{{- /* EVERY DECLARED ACCOUNT THAT GRANTS ANYTHING, as "name what-it-declares"
lines. The second field lists the KEYS the entry states, never a summary of the
verbs beneath them. */ -}}
{{- define "agentops.grantingAccounts" -}}
{{- range $a := (dig "runtime" "serviceAccounts" list (.Values.rbac | default dict)) -}}
{{- if or $a.clusterRoles $a.bindClusterRoles $a.namespaced }}
{{- $what := list -}}
{{- if $a.clusterRoles }}{{ $what = append $what (printf "%d ClusterRole(s) it renders" (len $a.clusterRoles)) }}{{ end -}}
{{- if $a.bindClusterRoles }}{{ $what = append $what (printf "%d existing ClusterRole(s)" (len $a.bindClusterRoles)) }}{{ end -}}
{{- if $a.namespaced }}{{ $what = append $what (printf "%d namespaced Role(s)" (len $a.namespaced)) }}{{ end -}}
{{ $a.name }}|{{ join ", " $what }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* ONE RUNTIME, RESOLVED — a declared entry laid over the release-wide
defaults, so an entry states only what DIFFERS.

Call with `(dict "root" $ "entry" <the entry>)`; returns YAML for `fromYaml`.

IT MERGES TWO LEVELS BY HAND RATHER THAN CALLING `mergeOverwrite`, and that is
not style. mergo skips ZERO values in the source, so a runtime declaring
`egressMediation: {enabled: false}` would silently keep the default `true` —
the one override the egress requirement exists to guarantee. `hasKey` semantics
are the only ones that let a runtime say `false`. */ -}}
{{- define "agentops.mergedRuntime" -}}
{{- $defaults := dig "agentops" "runtimeDefaults" dict (.root.Values.global | default dict) -}}
{{- $entry := .entry | default dict -}}
{{- $out := dict -}}
{{- range $k, $v := $defaults -}}
{{- $_ := set $out $k $v -}}
{{- end -}}
{{- range $k, $v := $entry -}}
{{- $dv := index $defaults $k -}}
{{- if and (kindIs "map" $v) (kindIs "map" $dv) -}}
{{- $sub := dict -}}
{{- range $sk, $sv := $dv -}}{{- $_ := set $sub $sk $sv -}}{{- end -}}
{{- range $sk, $sv := $v -}}{{- $_ := set $sub $sk $sv -}}{{- end -}}
{{- $_ := set $out $k $sub -}}
{{- else -}}
{{- $_ := set $out $k $v -}}
{{- end -}}
{{- end -}}
{{- toYaml $out -}}
{{- end -}}

{{- /* EVERY RUNTIME THIS RELEASE DECLARES, resolved, as a YAML list.

Two things need the whole release's answer rather than the parent's own list:
the manager's bootstrap env (context-sync and egress images are set only when
SOME runtime asks for them) and `agentops.defaultRuntimeGuard`.

A BUNDLE MAY SHIP A RUNTIME, so this cannot read `.Values.runtimes` alone. The
parent can see a subchart's values as `.Values.<subchart>`, which is the one
direction Helm allows, and each runtime-shipping bundle gets a line below.
Adding a vendor bundle means adding that line — there is no way to discover it,
and a bundle whose runtime this list missed would render a CR the guard could
not see. */ -}}
{{- /* THE OLLAMA BUNDLE'S VALUES AS A RUNTIME ENTRY. `endpoint`, `model`, `numCtx`
and `keepAlive` become the env the runtime reads; `env` is laid over them.

A PARENT helper, because two callers need the same answer: the bundle's own
template, and `agentops.declaredRuntimes` — from which the `default` copy is
rendered. When the mapping lived in the bundle's template, the copy carried the
bundle's VALUES and no env at all: a runtime named `default` pointed at nothing.
Call with the bundle's values. */ -}}
{{- define "agentops.ollamaRuntimeEntry" -}}
{{- $v := . -}}
{{- $endpoint := $v.endpoint | required "ollama.endpoint is required when the bundle is enabled — the URL of an Ollama server you already run, e.g. http://ollama.ollama.svc:11434; this bundle deploys none" -}}
{{- $model := $v.model | required "ollama.model is required when the bundle is enabled — a model already pulled on that server, e.g. qwen2.5:14b" -}}
{{- $env := list
  (dict "name" "OLLAMA_URL" "value" $endpoint)
  (dict "name" "OLLAMA_MODEL" "value" $model)
  (dict "name" "OLLAMA_NUM_CTX" "value" ($v.numCtx | default 8192 | toString))
  (dict "name" "OLLAMA_KEEP_ALIVE" "value" ($v.keepAlive | default "10m" | toString)) -}}
{{- range ($v.env | default list) }}{{ $env = append $env . }}{{ end -}}
{{- $entry := omit $v "enabled" "global" "endpoint" "model" "numCtx" "keepAlive" "env" -}}
{{- $_ := set $entry "env" $env -}}
{{- toYaml $entry -}}
{{- end -}}

{{- define "agentops.declaredRuntimes" -}}
{{- $root := . -}}
{{- $out := list -}}
{{- range $e := ($root.Values.runtimes | default list) -}}
{{- $out = append $out (fromYaml (include "agentops.mergedRuntime" (dict "root" $root "entry" $e))) -}}
{{- end -}}
{{- /* The runtime-shipping bundles. One line each, and there is no way to
discover them: a bundle whose runtime this list missed would render a CR the
default-runtime guard could not see, and the manager's bootstrap env would miss
the sidecar and proxy images that runtime asked for. `ollama` proved the point:
its first render passed every test but this guard, which reported "(none)". */ -}}
{{- range $key := list "claude" "ollama" -}}
{{- $bv := index $root.Values $key -}}
{{- if and $bv $bv.enabled -}}
{{- $entry := omit $bv "enabled" "global" -}}
{{- if eq $key "ollama" -}}{{- $entry = fromYaml (include "agentops.ollamaRuntimeEntry" $bv) -}}{{- end -}}
{{- $out = append $out (fromYaml (include "agentops.mergedRuntime" (dict "root" $root "entry" $entry))) -}}
{{- end -}}
{{- end -}}
{{- toYaml $out -}}
{{- end -}}

{{- /* WHICH RUNTIME ANSWERS TO `default`.

Every runtime renders under its OWN name — `claude`, `ollama`, whatever a
`runtimes:` entry says — and the parent renders ONE MORE CR named `default`,
a copy of the runtime this helper picks: the one flagged `default: true`, or,
with none flagged, the FIRST configured (top-level `runtimes:` in order, then
the bundles in the order `agentops.declaredRuntimes` lists them). One runtime
alone is therefore always the default. Two flagged is refused by the guard.

EVERY RUNTIME IS OPTIONAL, and this is what makes it so: turning the reference
bundle off and another on needs no rename, and the CR a route resolves to is
still one named `default`, which is what keeps the manager out of it. Returns
the chosen entry as YAML, or nothing when no runtime is declared. */ -}}
{{- define "agentops.defaultRuntimeEntry" -}}
{{- $declared := fromYamlArray (include "agentops.declaredRuntimes" .) -}}
{{- $pick := dict -}}
{{- range $rt := $declared -}}{{- if and $rt.default (not $pick) -}}{{- $pick = $rt -}}{{- end -}}{{- end -}}
{{- if and (not $pick) $declared -}}{{- $pick = first $declared -}}{{- end -}}
{{- if $pick -}}{{- toYaml $pick -}}{{- end -}}
{{- end -}}

{{- /* THE DEFAULT-RUNTIME GUARD.

`default` is the name a Pipeline declaring no `runtimeRef` resolves to. WHERE
NOTHING ANSWERS TO IT AND A ROUTE STILL RESOLVES TO IT, THE RENDER FAILS,
naming the missing runtime and the routes that needed it.

THIS REPLACES THE RULE THAT THE PARENT ALWAYS RENDERS `default`. That rule was
what guaranteed a bundle-free install could execute, and it cannot survive the
runtime shipping in a bundle an operator may turn off. Failing is the honest
replacement: the alternative is conversations reaching `Pending` forever with
the reason in the manager's log and nowhere an operator looks.

IT READS NO CLUSTER, so it protects a GitOps render exactly as it protects an
interactive one — unlike the claim-rename guard, whose `lookup` is blind
without one.

Routes naming their own runtime need no default, so the check is conditional on
something actually resolving to that name. */ -}}
{{- define "agentops.defaultRuntimeGuard" -}}
{{- $root := . -}}
{{- $declared := fromYamlArray (include "agentops.declaredRuntimes" .) -}}
{{- $names := list -}}
{{- range $rt := $declared -}}
{{- $names = append $names ($rt.name | default "") -}}
{{- end -}}
{{- if $declared -}}{{- $names = append $names "default" -}}{{- end -}}
{{- /* WHICH RUNTIME IS `default` IS DECIDED BY agentops.defaultRuntimeEntry, and the
parent renders a CR of that name from it. So `default` is missing only when NOTHING
is declared at all — or when the flag was set twice, which is refused first. */ -}}
{{- $flagged := list -}}
{{- range $rt := $declared -}}{{- if $rt.default -}}{{- $flagged = append $flagged $rt.name -}}{{- end -}}{{- end -}}
{{- if gt (len $flagged) 1 -}}
{{- fail (printf "%d runtimes are flagged `default: true` (%s), and only one can answer to that name. Flag one, or none — with none flagged the first configured runtime is the default." (len $flagged) (join ", " $flagged)) -}}
{{- end -}}
{{- if and (not (has "default" $names)) (eq (len $names) 0) -}}
{{- $needing := list -}}
{{- range $p := (.Values.pipelines | default list) -}}
{{- if not $p.runtimeRef -}}
{{- $needing = append $needing (printf "pipelines[%s]" ($p.name | default "<unnamed>")) -}}
{{- end -}}
{{- end -}}
{{- /* BUNDLE-SHIPPED ROUTES RESOLVE TO `default` TOO, and they are the case
this guard exists for: demo mode ships one, and an install that turns off the
runtime bundle would otherwise get a working-looking render whose conversations
never execute.

A bundle helper needs a SUBCHART context, which is the dict below — the
parent's globals plus that bundle's values. Same technique NOTES.txt uses to
report the wiring, and for the same reason: re-deriving through the bundle's own
helper is what stops this drifting from what actually rendered.

A route is counted only where it ACTUALLY RENDERS — re-derived through the same
helper the bundle uses — and where BOTH its bundle-level `pipelines.runtimeRef`
and its own are empty. An install that names a runtime on every route is not
failed for a default it does not use, and a route that does not render is not
named in a failure about routes. */ -}}
{{- /* `gated` marks a bundle whose Chart.yaml `condition:` is `<key>.enabled`.
Helm does not PARSE a disabled subchart's templates, so its named helpers do not
exist and calling one is a render error rather than a false answer. The
Kubernetes bundle carries no condition — it turns on by `enabled` OR demo mode,
which a Helm condition cannot express — so its helpers are always loaded. */ -}}
{{- range $b := list
      (dict "key" "kubernetes" "helper" "kubernetes.wiringActive" "gated" false "routes" (dict "observe" "kubernetes.observePipelineEnabled" "admin" "kubernetes.adminPipelineEnabled"))
      (dict "key" "home-assistant" "helper" "home-assistant.wiringActive" "gated" true "routes" (dict "control" "home-assistant.userProfileEnabled" "ops" "home-assistant.opsProfileEnabled"))
      (dict "key" "prometheus" "helper" "prometheus.wiringActive" "gated" true "routes" dict) -}}
{{- $bv := index $root.Values $b.key -}}
{{- if and $bv (or (not $b.gated) $bv.enabled) -}}
{{- $ctx := dict "Values" (merge (deepCopy $bv) (dict "global" $root.Values.global)) "Release" $root.Release "Chart" $root.Chart -}}
{{- if include $b.helper $ctx -}}
{{- $bundleRef := dig "pipelines" "runtimeRef" "" $bv -}}
{{- if not $bundleRef -}}
{{- if $b.routes -}}
{{- range $r, $rh := $b.routes -}}
{{- if and (include $rh $ctx) (not (dig "pipelines" $r "runtimeRef" "" $bv)) -}}
{{- $needing = append $needing (printf "%s.pipelines.%s" $b.key $r) -}}
{{- end -}}
{{- end -}}
{{- else -}}
{{- $needing = append $needing (printf "%s.pipelines" $b.key) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if $needing -}}
{{- fail (printf "No AgentRuntime named \"default\" is declared in this release, and %d route(s) resolve to it: %s. A Pipeline naming no runtimeRef falls back to the runtime named `default`, so these routes would create conversations that never execute — Pending forever, with the reason in the manager's log and nowhere you look. Declare one under the top-level `runtimes:` (an entry naming only its `name` inherits every value from global.agentops.runtimeDefaults), or enable a runtime bundle — the first one configured becomes `default`, or the one flagged `default: true`. Or give each route above its own `runtimeRef`. Declared runtimes: %s" (len $needing) (join ", " $needing) (ternary "(none)" (join ", " $names) (eq (len $names) 0))) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* ONE RUNTIME'S OBJECTS — its credential Secret, where a token was supplied,
and its AgentRuntime CR.

SHARED, because a bundle may ship a runtime and two renderers would be two
places for the CR's shape to drift. Call with `(dict "root" $ "entry" <entry>)`.

IT RENDERS NO VOLUME. Persistence is WIRING and lives on the Pipeline; the
release-wide claims reach conversations through the manager's bootstrap
configuration, not through this CR. */ -}}
{{- define "agentops.renderRuntime" -}}
{{- $root := .root -}}
{{- $rt := fromYaml (include "agentops.mergedRuntime" .) -}}
{{- $name := $rt.name | required "every entry in `runtimes:` needs a name — it is what a Pipeline's spec.runtimeRef refers to, and `default` is the one a route naming none resolves to" -}}
{{- $cred := $rt.credentialsSecret | default dict -}}
{{- if and $cred.token (not $rt.defaultOf) }}
---
# Created because this runtime supplied a credential token, so the agent's
# credential is provisioned with the release instead of being a prerequisite
# someone has to remember. The AgentRuntime below references it by NAME only —
# the manager reads no Secrets, the kubelet resolves it.
apiVersion: v1
kind: Secret
metadata:
  name: {{ $cred.name }}
  namespace: {{ $root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-runtime
type: Opaque
stringData:
  {{ $cred.key }}: {{ $cred.token | quote }}
{{- end }}
---
apiVersion: agentops.dev/v1alpha1
kind: AgentRuntime
metadata:
  name: {{ $name }}
  namespace: {{ $root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-runtime
  {{- with $rt.defaultOf }}
  # THE DEFAULT: a copy of the runtime named below, rendered by the parent so a
  # route naming no runtimeRef has a CR of this name to resolve to. Which one is
  # copied is `default: true` on that runtime, or the first configured.
  annotations:
    agentops.dev/default-of: {{ . | quote }}
  {{- end }}
spec:
  image: {{ $rt.image | required (printf "runtime %q declares no image, and global.agentops.runtimeDefaults.image is empty — a runtime with no image is a CR the API server refuses" $name) | quote }}
  # The agent's security identity — its RBAC IS its power. A REFERENCE: this
  # chart renders only its own floor account, never the one named here.
  serviceAccountName: {{ $rt.serviceAccountName | default (include "agentops.floorServiceAccount" $root) }}
  # BACKEND SHAPE, never placement — whether this runtime's backend keeps
  # context on a disk at all, which is what lets the manager say up front
  # whether continuity is possible rather than failing a follow-up for a volume
  # this runtime never needed.
  contextStorage: {{ $rt.contextStorage | default "volume" }}
  # WRITTEN OUT, never omitted: the CRD carries a structural default of 10, so
  # an omitted field is not "unset" — the API server stores 10 and the manager,
  # which prefers any non-zero spec value over its own bootstrap default, would
  # silently ignore what the release configured. Omitting it looked correct in
  # the rendered manifest and was wrong in the stored object.
  idleTtlMinutes: {{ $rt.idleTtlMinutes | default 10 }}
  {{- with $rt.nodeSelector }}
  nodeSelector:
{{ toYaml . | indent 4 }}
  {{- end }}
  # THIS CR DECLARES NO VOLUME, and that is the shape rather than an omission.
  # A runtime is an ENGINE — an image and its pod-level defaults. WHERE a
  # route's conversations keep their state is the ROUTE's decision, declared on
  # `Pipeline.spec.persistence` beside the tools it grants and the identity it
  # executes under; a route binding neither takes the release-wide claims, which
  # reach the manager as its bootstrap configuration (deployment.yaml's
  # CONTEXT_PVC / WORKSPACE_PVC). Nothing restates a claim name.
  {{- with (($rt.contextSync | default dict).paths) }}
  # CONTEXT SYNC: declared by the RUNTIME, because only it knows where its
  # backend keeps context. Absent means the context volume is mounted directly and
  # no sidecar is built — today's behaviour, so an existing install upgrades
  # with no migration.
  contextSync:
    paths:
{{ toYaml . | indent 6 }}
    {{- with $rt.contextSync.exclude }}
    exclude:
{{ toYaml . | indent 6 }}
    {{- end }}
    interval: {{ $rt.contextSync.interval | quote }}
    retain: {{ $rt.contextSync.retain }}
  {{- end }}
  {{- if (($rt.egressMediation | default dict).enabled) }}
  # EGRESS MEDIATION: declared by the RUNTIME, because enabling it changes what
  # the pod may do at startup — it adds a privileged init container, which a
  # namespace under `restricted` Pod Security admission will refuse. ON by
  # default; a runtime declares `egressMediation.enabled: false` to decline it,
  # and then its pods carry nothing extra at all.
  egressMediation:
    # ALWAYS rendered, never an empty mapping: an empty stanza is null to the
    # API server, and null means ABSENT — which would read as "mediation off"
    # on an install that just asked for it, silently.
    port: {{ $rt.egressMediation.port | default 15001 }}
    {{- with $rt.egressMediation.excludePorts }}
    # Every port here is reachable by the agent UNMEDIATED.
    excludePorts:
{{ toYaml . | indent 6 }}
    {{- end }}
    {{- with $rt.egressMediation.resources }}
    resources:
{{ toYaml . | indent 6 }}
    {{- end }}
  {{- end }}
  {{- with $rt.resources }}
  resources:
{{ toYaml . | indent 4 }}
  {{- end }}
  {{- with $rt.env }}
  env:
{{ toYaml . | indent 4 }}
  {{- end }}
  {{- if $cred.name }}
  {{- if $rt.env }}
  {{- fail (printf "runtime %q sets both `env` and `credentialsSecret`; state the credential as one more `env` entry instead" $name) }}
  {{- end }}
  env:
    - name: {{ $cred.envName }}
      valueFrom:
        secretKeyRef:
          name: {{ $cred.name }}
          key: {{ $cred.key }}
  {{- end }}
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

Shared with kubernetes's MCP server, exactly as agentops.runtimeRbacMode is, so
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
{{- /*
THE ACCESS MODE, AND WHY DEMO MODE ANSWERS IT DIFFERENTLY.

`ReadWriteMany` is right for a real install: runtime pods land wherever the
scheduler puts them, and every one of them mounts the context volume, so a claim
only one node can attach pins the whole install to that node.

**IT IS WRONG FOR THE ONE-FLAG DEMO, AND THAT COST AN ADOPTER THE PRODUCT.**
`local-path` — which rancher-desktop, k3d, kind and minikube all ship as the
ONLY class — supports RWO and RWOP alone and REFUSES an RWX claim outright:

  failed to provision volume with StorageClass "local-path":
  NodePath only supports ReadWriteOnce and ReadWriteOncePod (1.22+) access modes

The claim sits `Pending`, no runtime pod is ever created, and the conversation
waits forever. Getting started documented the workaround
(`persistence.context.enabled=false`) — which trades the demo's memory away for
a storage detail the reader has no reason to have read first.

- **It is not about node COUNT.** Any cluster whose only provisioner is RWO-only
  hits it, and a single-node cluster with an RWX provisioner never does.
- **RWO is CORRECT under demo rather than merely tolerable.** Kubernetes lets
  many pods share an RWO volume on ONE node, and the volume's node affinity is
  what puts them there — so the default cap of five concurrent conversations
  still holds, on a laptop and on a demo running against a real cluster alike.
- **PERSISTENCE STAYS ON.** The fix a reader would otherwise apply turns it off,
  which is the opposite of what a demo should show.

**AN EXPLICIT VALUE ALWAYS WINS**, which is why the shipped default is EMPTY
rather than a mode: an empty list is the one thing a chart can tell apart from a
typed one, so "the chart decides" is expressible and an operator who typed
`ReadWriteMany` under demo mode still gets it.

Call with the volume's values block plus the root.
*/ -}}
{{- define "agentops.accessModes" -}}
{{- $modes := .volume.accessModes | default (list) -}}
{{- if $modes -}}
{{ toYaml $modes }}
{{- else if .root.Values.global.demo.enabled -}}
- ReadWriteOnce
{{- else -}}
- ReadWriteMany
{{- end -}}
{{- end -}}

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
agentops.retiredRuntimeKeysGuard — every values key this restructure deleted,
failing the render and NAMING its replacement.

HELM REPORTS NO UNREAD VALUES KEY, so the alternative is silence, and the quiet
outcomes here are the expensive ones:

  * `runtime.image` left in a values file renders the DEFAULT image while every
    signal reports success.
  * `global.agentops.runtime.rbacMode: full` left behind grants nothing at all
    now, so an install that believed it had an acting agent has an inert one.
    The per-account `rbacMode` is the same story and is refused in
    `runtime-rbac.yaml`, where the entry is read.
  * `kubernetes.enabled: true` left behind renders NOTHING, indistinguishable
    from an operator who meant to leave the bundle off.

It needs NO CLUSTER, so unlike the claim-rename guard it also protects a GitOps
install and a CI render.
*/}}
{{- define "agentops.retiredRuntimeKeysGuard" -}}
{{- $v := .Values -}}
{{- $g := dig "agentops" dict (.Values.global | default dict) -}}
{{- $bad := list -}}
{{- if hasKey $v "runtimeIdleTtlMinutes" -}}
{{- $bad = append $bad "runtimeIdleTtlMinutes -> global.agentops.runtimeDefaults.idleTtlMinutes. It moved for the same reason `resources` did: a bundle-shipped runtime cannot read a parent-scope value, so it rendered an EMPTY field and the CRD's structural default of 10 silently replaced the release's setting" -}}
{{- end -}}
{{- if hasKey $v "runtime" -}}
{{- $bad = append $bad "runtime.* -> the block SPLIT IN TWO: what every runtime inherits is global.agentops.runtimeDefaults, and the runtimes that EXIST are the top-level `runtimes:` list, each stating only what differs. `runtime.enabled: false` becomes `runtimes: []`" -}}
{{- end -}}
{{- if hasKey $g "runtime" -}}
{{- $r := index $g "runtime" -}}
{{- if hasKey $r "rbacMode" -}}
{{- $bad = append $bad "global.agentops.runtime.rbacMode -> DELETED, with no replacement and no alias. There is no preset posture at ANY level: the default is NO permissions, and an install wanting more declares an account under rbac.runtime.serviceAccounts STATING ITS RULES (clusterRoles / bindClusterRoles / namespaced) and NAMES it on the routes that need it. There is no per-account `rbacMode` either — that was the same preset one level down. Start from agentops.runtimeReadRules / runtimeWriteRules in chart/templates/_helpers.tpl and paste what you want" -}}
{{- end -}}
{{- if hasKey $r "serviceAccountName" -}}
{{- $bad = append $bad "global.agentops.runtime.serviceAccountName -> global.agentops.runtimeDefaults.serviceAccountName. It is now a REFERENCE this chart does not create, and its default is the floor account the chart always renders" -}}
{{- end -}}
{{- if hasKey $r "allowPodExecution" -}}
{{- $bad = append $bad "global.agentops.runtime.allowPodExecution -> global.agentops.runtimeDefaults.allowPodExecution" -}}
{{- end -}}
{{- end -}}
{{- /* THE VENDOR'S KEYS LEFT IN THE RELEASE-WIDE DEFAULTS. Not silently
ignored — WORSE: they still merge into EVERY runtime, so one vendor's image
reference and one vendor's environment variable reach a backend that is not that
vendor. That is the exact condition extracting the `claude` bundle was meant to
end, and it looks like a working install right up to the first run. */ -}}
{{- $rdef := dig "agentops" "runtimeDefaults" dict (.Values.global | default dict) -}}
{{- if hasKey $rdef "image" -}}
{{- $bad = append $bad "global.agentops.runtimeDefaults.image -> the bundle or entry that ships that VENDOR. The reference runtime's is `claude.image`; another backend states its own on its `runtimes:` entry. The release-wide defaults carry no image, because an image cannot be stated without naming a vendor and every runtime inherits this block" -}}
{{- end -}}
{{- if hasKey $rdef "credentialsSecret" -}}
{{- $bad = append $bad "global.agentops.runtimeDefaults.credentialsSecret -> the bundle or entry that ships that VENDOR. The reference runtime's is `claude.credentialsSecret` (set `.token` and the Secret is created with the release). A key and an env var name one vendor, and left here they reach every runtime — including one that reads neither" -}}
{{- end -}}
{{- /* THE ADAPTER FIELD THAT BECAME AN IDENTITY. Silently dropping a
`kubernetesAccess: true` would UNMOUNT a token the adapter's own code depends on,
and that surfaces as an error inside the adapter rather than at the values file
that caused it. */ -}}
{{- range $k := list "signalAdapters" "channelAdapters" -}}
{{- range $e := (index $v $k | default list) -}}
{{- if hasKey $e "kubernetesAccess" -}}
{{- $bad = append $bad (printf "%s[].kubernetesAccess -> serviceAccountName. An adapter NAMES the account it runs as, and naming one is what mounts its token — the two were always one decision. The chart that grants an adapter renders that account beside the grant; naming none means the release floor, denied every verb" $k) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $rr := dig "runtime" dict (.Values.rbac | default dict) -}}
{{- range $k := list "clusterRoles" "bindClusterRoles" "namespaced" -}}
{{- if hasKey $rr $k -}}
{{- $bad = append $bad (printf "rbac.runtime.%s -> moved ONTO an account. These added to the account the deleted release-wide mode rendered, and that account is gone; declare the account under rbac.runtime.serviceAccounts and put `%s` on the entry, where it is now the ONLY way an account is granted anything" $k $k) -}}
{{- end -}}
{{- end -}}
{{- range $pair := list (list "k8s-bundle" "kubernetes") (list "ha-bundle" "home-assistant") (list "prometheus-bundle" "prometheus") (list "telegram-bundle" "telegram") (list "vm-bundle" "prometheus") -}}
{{- if hasKey $v (index $pair 0) -}}
{{- $bad = append $bad (printf "`%s:` -> `%s:`. Every subchart is now named for the SYSTEM it integrates, with no suffix. Restate every value under the new key" (index $pair 0) (index $pair 1)) -}}
{{- end -}}
{{- end -}}
{{- if $bad -}}
{{- fail (printf "These values keys are RETIRED and are no longer read:\n\n  - %s\n\nSee docs/CHANGELOG.md. Helm never reports an unread values key, which is why this fails the render rather than letting the install succeed while doing something else." (join "\n  - " $bad)) -}}
{{- end -}}
{{- end -}}

{{/*
agentops.retiredRuntimeVolumeKeysGuard — kept as its own guard because the
message it carries is about the VOLUME concept moving to the Pipeline, which is
a different migration from the values restructure above.

An AgentRuntime declares no volume at all, and an operator who deliberately
pointed one at a claim the chart did not create would otherwise keep every
signal of success while the release-wide claim was used instead — every
conversation on that install answering out of the wrong volume.
*/}}
{{- define "agentops.retiredRuntimeVolumeKeysGuard" -}}
{{- $root := . -}}
{{- $retired := list -}}
{{- range $i, $rt := (.Values.runtimes | default list) -}}
{{- range $k := list "contextPvcRef" "homePvcRef" "workspacePvcRef" -}}
{{- if hasKey $rt $k -}}
{{- $retired = append $retired (printf "runtimes[%d].%s" $i $k) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- range $k := list "contextPvcRef" "homePvcRef" "workspacePvcRef" -}}
{{- if hasKey (dig "agentops" "runtimeDefaults" dict ($root.Values.global | default dict)) $k -}}
{{- $retired = append $retired (printf "global.agentops.runtimeDefaults.%s" $k) -}}
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
