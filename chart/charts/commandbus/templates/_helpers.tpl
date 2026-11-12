{{- /*
Every named template below is included ONLY from the PARENT chart's
templates/commandbus.yaml, never walked as a bare file here. `chart/charts/
commandbus/` carries no Chart.yaml dependency `condition:`, so Helm always
parses this file — but nothing in it emits YAML outside a `{{- define -}}`
block, so rendering this subchart natively (as Helm does regardless of
condition) produces nothing by itself. The PARENT decides whether the
`nats` command bus backend is selected AND whether its bundle (as opposed
to an external `commandBus.nats.url`) is the one rendering
(`agentops.commandBusBackend` / `agentops.commandBusNatsBundleEnabled`,
values this subchart cannot see on its own — chart.md: "A SUBCHART READS
NO PARENT SCOPE BUT `global.`") and calls these with an explicit
`(dict "root" $ "commandbus" .Values.commandBus.nats)`.

SAME SHAPE as chart/charts/nats/templates/_helpers.tpl on purpose (design
D-13: "same shape as the state bundle"), and deliberately NOT that file
instantiated a second time under an alias — task 8.1 found every object
name there a hardcoded literal (agentops-nats, agentops-nats-headless,
agentops-nats-client, agentops-nats-credentials, and agentops-nats's own
`app.kubernetes.io/name` label used as both the pod selector and the
NetworkPolicy's identity): 9 occurrences across statefulset/services/pdb/
credential/networkpolicy, none parameterized by anything the caller's dict
passes in. A second instantiation under an alias would collide on every
one of them — two StatefulSets, two headless Services and so on, all
named identically. So this is a renamed sibling instead, with its own
object names (agentops-commandbus and friends) throughout.
*/ -}}

{{- /*
A Secret named by `commandbus.auth.existingSecret` MUST carry the keys
`user` and `password`: the StatefulSet reads exactly those two keys from
it, and so does the manager's own COMMANDBUS_NATS_USER/PASSWORD (chart
root templates/deployment.yaml).
*/ -}}
{{- define "commandbus.credentialSecretName" -}}
{{- $auth := .commandbus.auth | default dict -}}
{{- $auth.existingSecret | default ($auth.secretName | default "agentops-commandbus-credentials") -}}
{{- end -}}

{{- /*
Builds the `--routes` value: every replica's own address on the headless
Service, including the caller's own — nats-server ignores a self-route
rather than erroring on one, and listing every peer (not peer-minus-self)
is what every shipped replica needs the SAME value for, so the StatefulSet
template stays one pod spec for all of them. Mirrors nats.routes exactly,
over the command bus's own headless Service name.
*/ -}}
{{- define "commandbus.routes" -}}
{{- $ns := .root.Release.Namespace -}}
{{- $replicas := int (.commandbus.replicas | default 3) -}}
{{- $routes := list -}}
{{- range $i := until $replicas -}}
{{- $routes = append $routes (printf "nats://agentops-commandbus-%d.agentops-commandbus-headless.%s.svc:6222" $i $ns) -}}
{{- end -}}
{{- join "," $routes -}}
{{- end -}}

{{- define "commandbus.statefulset" -}}
{{- $root := .root -}}
{{- $v := .commandbus -}}
{{- $ns := $root.Release.Namespace -}}
{{- $cred := include "commandbus.credentialSecretName" . -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: agentops-commandbus
  namespace: {{ $ns }}
  labels:
    app.kubernetes.io/name: agentops-commandbus
spec:
  serviceName: agentops-commandbus-headless
  replicas: {{ int ($v.replicas | default 3) }}
  # Every replica writes the SAME pod spec, so there is nothing to order —
  # starting them in parallel is what lets a 3-replica install form its
  # cluster instead of waiting on a readiness probe none of them can pass
  # alone (quorum needs two).
  podManagementPolicy: Parallel
  selector:
    matchLabels:
      app.kubernetes.io/name: agentops-commandbus
  template:
    metadata:
      labels:
        app.kubernetes.io/name: agentops-commandbus
    spec:
      # SOFT: a one-node cluster (replicas: 1, the install most likely to
      # run that on purpose) still schedules. A required rule would leave
      # two Pending on exactly that cluster.
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    app.kubernetes.io/name: agentops-commandbus
                topologyKey: kubernetes.io/hostname
      {{- with $v.nodeSelector }}
      nodeSelector:
{{ toYaml . | indent 8 }}
      {{- end }}
      containers:
        - name: commandbus
          image: {{ if $v.image.digest }}{{ printf "%s:%s@%s" $v.image.repository $v.image.tag $v.image.digest | quote }}{{ else }}{{ printf "%s:%s" $v.image.repository $v.image.tag | quote }}{{ end }}
          args:
            - -js
            - -sd
            - /data
            - -p
            - "4222"
            - -m
            - "8222"
            - --cluster
            - nats://0.0.0.0:6222
            - --cluster_name
            - agentops-commandbus
            # A JetStream cluster refuses to start without a server name,
            # and each replica needs its OWN: the pod name, which the
            # StatefulSet keeps stable across restarts (a changed name is a
            # different peer to the Raft group).
            - --server_name
            - $(POD_NAME)
            - --routes
            - {{ include "commandbus.routes" . | quote }}
            # Kubernetes expands $(VAR) against THIS container's own env —
            # a kubelet mechanism, not nats-server's — so the password
            # never appears as a literal in the pod spec the API server
            # stores, only as a reference the kubelet resolves
            # (invariants.md: "Manager reads NO secrets" applies here too —
            # this process is not the manager, but the same rule the
            # manager itself follows is why the credential is never typed
            # into a template as a value).
            - --user
            - $(POD_COMMANDBUS_USER)
            - --pass
            - $(POD_COMMANDBUS_PASSWORD)
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_COMMANDBUS_USER
              valueFrom:
                secretKeyRef:
                  name: {{ $cred }}
                  key: user
            - name: POD_COMMANDBUS_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: {{ $cred }}
                  key: password
          ports:
            - name: client
              containerPort: 4222
            - name: cluster
              containerPort: 6222
            - name: monitor
              containerPort: 8222
          volumeMounts:
            - name: data
              mountPath: /data
          # /healthz answers before authorization is checked (confirmed
          # against nats-server's own monitor.go, same as the state-store
          # bundle's own probe), so the probe needs no credential and
          # cannot be mistaken for the auth wall being down.
          readinessProbe:
            httpGet:
              path: /healthz
              port: monitor
            initialDelaySeconds: {{ ($v.probes | default dict).initialDelaySeconds | default 5 }}
            periodSeconds: {{ ($v.probes | default dict).periodSeconds | default 10 }}
          # Liveness asks only whether JetStream is enabled. The full /healthz
          # also fails while quorum is lost, and restarting the one server
          # that is still up would turn a survivable outage into a longer one.
          livenessProbe:
            httpGet:
              path: /healthz?js-enabled-only=true
              port: monitor
            initialDelaySeconds: {{ ($v.probes | default dict).initialDelaySeconds | default 5 }}
            periodSeconds: {{ ($v.probes | default dict).periodSeconds | default 10 }}
          {{- with $v.resources }}
          resources:
{{ toYaml . | indent 12 }}
          {{- end }}
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        # ALWAYS ReadWriteOnce: one claim per replica, never shared, so
        # `agentops.accessModes` (RWX-by-default, for a volume many runtime
        # pods mount at once) does not apply to a per-pod store directory.
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: {{ $v.persistence.size | default "10Gi" | quote }}
{{ include "agentops.storageClassName" $v.persistence | indent 8 }}
{{- end -}}

{{- define "commandbus.services" -}}
{{- $root := .root -}}
{{- $v := .commandbus -}}
{{- $ns := $root.Release.Namespace -}}
apiVersion: v1
kind: Service
metadata:
  name: agentops-commandbus-headless
  namespace: {{ $ns }}
  labels:
    app.kubernetes.io/name: agentops-commandbus
spec:
  clusterIP: None
  # A replica's peer DNS record must resolve before it reports Ready (the
  # readiness probe needs the OTHER replicas to have formed quorum first
  # under podManagementPolicy: Parallel), so this cannot wait on Ready
  # itself the way an ordinary headless Service would.
  publishNotReadyAddresses: true
  selector:
    app.kubernetes.io/name: agentops-commandbus
  ports:
    - name: client
      port: 4222
    - name: cluster
      port: 6222
    - name: monitor
      port: 8222
---
apiVersion: v1
kind: Service
metadata:
  name: agentops-commandbus-client
  namespace: {{ $ns }}
  labels:
    app.kubernetes.io/name: agentops-commandbus
spec:
  selector:
    app.kubernetes.io/name: agentops-commandbus
  ports:
    - name: client
      port: 4222
{{- end -}}

{{- define "commandbus.pdb" -}}
{{- $root := .root -}}
{{- $v := .commandbus -}}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: agentops-commandbus
  namespace: {{ $root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-commandbus
spec:
  # Quorum needs two of three: allowing one down keeps the server
  # available through a voluntary eviction. Meaningless at replicas: 1 —
  # rendered anyway, since a template that special-cased it would be a
  # second place this number could drift from the StatefulSet's own.
  maxUnavailable: {{ ($v.podDisruptionBudget | default dict).maxUnavailable | default 1 }}
  selector:
    matchLabels:
      app.kubernetes.io/name: agentops-commandbus
{{- end -}}

{{- /*
The chart-generated credential, following nats.credential /
adapter-auth.yaml / control-plane-auth.yaml / state-read-auth.yaml's rule
exactly: explicit value, then an existing Secret, then generation on
install only — gated the same way, on `agentops.clusterConnected` rather
than `.Release.IsInstall` alone, so a cluster-less render (helm template,
CI, a GitOps repo-server) can never tell "missing" from "unknown" and
mints nothing. Only `password` is ever generated; `user` defaults to the
literal `commandbus` so a render with no explicit or existing value still
needs no second secret key to exist.

Rendered whenever `commandBus.backend` is `nats`, REGARDLESS of whether
the bundle itself renders (`agentops.commandBusNatsBundleEnabled`) — an
external `commandBus.nats.url` still needs this Secret, since the manager
reads its credential only through `valueFrom` (invariants.md: "Manager
reads NO secrets") and never a literal value. The guard
(`agentops.commandBusGuard`) already refuses a URL with no credential of
its own before this template is ever reached, so the generate-on-install
branch below is only ever live for the rendered-bundle case in practice.
*/ -}}
{{- define "commandbus.credential" -}}
{{- $root := .root -}}
{{- $v := .commandbus -}}
{{- $auth := $v.auth | default dict -}}
{{- if not $auth.existingSecret }}
{{- $name := $auth.secretName | default "agentops-commandbus-credentials" -}}
{{- $explicitUser := $auth.user | default "" -}}
{{- $explicitPassword := $auth.password | default "" -}}
{{- $existing := lookup "v1" "Secret" $root.Release.Namespace $name -}}
{{- $existingUser := "" -}}
{{- $existingPassword := "" -}}
{{- if and $existing $existing.data }}
{{- $existingUser = index $existing.data "user" | default "" -}}
{{- $existingPassword = index $existing.data "password" | default "" -}}
{{- end }}
{{- include "agentops.generatedSecretGuard" (dict "root" $root "name" $name "explicit" $explicitPassword "existing" $existing "existingVal" $existingPassword "key" "password" "setting" "commandBus.nats.auth.password") -}}
{{- $connected := eq (include "agentops.clusterConnected" $root) "true" -}}
{{- if and $connected (not $root.Release.IsInstall) (not $explicitPassword) (not $existingPassword) }}
{{- fail (printf "Secret %q does not exist, or exists without a non-empty password key, and neither commandBus.nats.auth.password nor commandBus.nats.auth.existingSecret is set. Set one of them before upgrading with the nats command bus backend." $name) }}
{{- end }}
{{- if or $explicitPassword $existingPassword $root.Release.IsInstall $connected }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ $name }}
  namespace: {{ $root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-commandbus
  annotations:
    # a render with no cluster (connected=false, not install, not
    # explicit) omits this object entirely, same as every other
    # chart-generated credential — `keep` is what stops THAT renderer's
    # absence of it from reading as a delete once a connected renderer has
    # created it for real
    helm.sh/resource-policy: keep
type: Opaque
data:
  user: {{ if $explicitUser }}{{ $explicitUser | b64enc }}{{ else if $existingUser }}{{ $existingUser }}{{ else }}{{ "commandbus" | b64enc }}{{ end }}
  password: {{ if $explicitPassword }}{{ $explicitPassword | b64enc }}{{ else if $existingPassword }}{{ $existingPassword }}{{ else }}{{ randAlphaNum 32 | b64enc }}{{ end }}
{{- end }}
{{- end }}
{{- end -}}

{{- /*
The client port admits the manager alone, the peer (cluster) port admits
only the server's own pods. Mirrors nats.networkpolicy exactly — every
other component reads work through the manager, never this bus directly.
*/ -}}
{{- define "commandbus.networkpolicy" -}}
{{- $np := (.root.Values.global.agentops).networkPolicy | default dict -}}
{{- if $np.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agentops-commandbus
  namespace: {{ .root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-commandbus
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: agentops-commandbus
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: agentops-manager
      ports:
        - protocol: TCP
          port: 4222
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: agentops-commandbus
      ports:
        - protocol: TCP
          port: 6222
    # THE KUBELET'S PROBE, from the node — matching the manager's own
    # policy's note: no podSelector can express a node, and the monitor
    # port serves /healthz only, so leaving it open to the pod network
    # costs nothing an attacker can use. Omitting it crash-loops the
    # server behind the very policy meant to protect it.
    - ports:
        - protocol: TCP
          port: 8222
{{- end }}
{{- end -}}
