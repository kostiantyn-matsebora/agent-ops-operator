{{- /*
Every named template below is included ONLY from the PARENT chart's
templates/nats.yaml, never walked as a bare file here. `chart/charts/nats/`
carries no Chart.yaml dependency `condition:`, so Helm always parses this
file — but nothing in it emits YAML outside a `{{- define -}}` block, so
rendering this subchart natively (as Helm does regardless of condition)
produces nothing by itself. The PARENT decides whether `nats` is the
selected backend (`agentops.stateBackend`, a value this subchart cannot see
on its own — chart.md: "A SUBCHART READS NO PARENT SCOPE BUT `global.`") and
calls these with an explicit `(dict "root" $ "nats" .Values.nats)`.
*/ -}}

{{- /*
A Secret named by `nats.auth.existingSecret` MUST carry the keys `user` and
`password`: the StatefulSet reads exactly those two keys from it.
*/ -}}
{{- define "nats.credentialSecretName" -}}
{{- $auth := .nats.auth | default dict -}}
{{- $auth.existingSecret | default ($auth.secretName | default "agentops-nats-credentials") -}}
{{- end -}}

{{- /*
Builds the `--routes` value: every replica's own address on the headless
Service, including the caller's own — nats-server ignores a self-route
rather than erroring on one, and listing every peer (not peer-minus-self)
is what every shipped replica needs the SAME value for, so the StatefulSet
template stays one pod spec for all of them.
*/ -}}
{{- define "nats.routes" -}}
{{- $ns := .root.Release.Namespace -}}
{{- $replicas := int (.nats.replicas | default 3) -}}
{{- $routes := list -}}
{{- range $i := until $replicas -}}
{{- $routes = append $routes (printf "nats://agentops-nats-%d.agentops-nats-headless.%s.svc:6222" $i $ns) -}}
{{- end -}}
{{- join "," $routes -}}
{{- end -}}

{{- define "nats.statefulset" -}}
{{- $root := .root -}}
{{- $v := .nats -}}
{{- $ns := $root.Release.Namespace -}}
{{- $cred := include "nats.credentialSecretName" . -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: agentops-nats
  namespace: {{ $ns }}
  labels:
    app.kubernetes.io/name: agentops-nats
spec:
  serviceName: agentops-nats-headless
  replicas: {{ int ($v.replicas | default 3) }}
  # Every replica writes the SAME pod spec, so there is nothing to order —
  # starting them in parallel is what lets a 3-replica install form its
  # cluster instead of waiting on a readiness probe none of them can pass
  # alone (quorum needs two).
  podManagementPolicy: Parallel
  selector:
    matchLabels:
      app.kubernetes.io/name: agentops-nats
  template:
    metadata:
      labels:
        app.kubernetes.io/name: agentops-nats
    spec:
      # SOFT (D-8): a one-node cluster — the installs most likely to run
      # `nats.replicas: 1` on purpose — still schedules every replica. A
      # required rule would leave two Pending on exactly that cluster.
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    app.kubernetes.io/name: agentops-nats
                topologyKey: kubernetes.io/hostname
      {{- with $v.nodeSelector }}
      nodeSelector:
{{ toYaml . | indent 8 }}
      {{- end }}
      containers:
        - name: nats
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
            - agentops-nats
            # A JetStream cluster refuses to start without a server name,
            # and each replica needs its OWN: the pod name, which the
            # StatefulSet keeps stable across restarts (a changed name is a
            # different peer to the Raft group).
            - --server_name
            - $(POD_NAME)
            - --routes
            - {{ include "nats.routes" . | quote }}
            # Kubernetes expands $(VAR) against THIS container's own env —
            # a kubelet mechanism, not nats-server's — so the password
            # never appears as a literal in the pod spec the API server
            # stores, only as a reference the kubelet resolves
            # (invariants.md: "Manager reads NO secrets" applies here too —
            # this process is not the manager, but the same rule the
            # manager itself follows is why the credential is never typed
            # into a template as a value).
            - --user
            - $(POD_NATS_USER)
            - --pass
            - $(POD_NATS_PASSWORD)
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NATS_USER
              valueFrom:
                secretKeyRef:
                  name: {{ $cred }}
                  key: user
            - name: POD_NATS_PASSWORD
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
          # against nats-server's own monitor.go), so the probe needs no
          # credential and cannot be mistaken for the auth wall being down.
          readinessProbe:
            httpGet:
              path: /healthz
              port: monitor
            initialDelaySeconds: {{ ($v.probes | default dict).initialDelaySeconds | default 5 }}
            periodSeconds: {{ ($v.probes | default dict).periodSeconds | default 10 }}
          livenessProbe:
            httpGet:
              path: /healthz
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

{{- define "nats.services" -}}
{{- $root := .root -}}
{{- $v := .nats -}}
{{- $ns := $root.Release.Namespace -}}
apiVersion: v1
kind: Service
metadata:
  name: agentops-nats-headless
  namespace: {{ $ns }}
  labels:
    app.kubernetes.io/name: agentops-nats
spec:
  clusterIP: None
  # A replica's peer DNS record must resolve before it reports Ready (the
  # readiness probe needs the OTHER replicas to have formed quorum first
  # under podManagementPolicy: Parallel), so this cannot wait on Ready
  # itself the way an ordinary headless Service would.
  publishNotReadyAddresses: true
  selector:
    app.kubernetes.io/name: agentops-nats
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
  name: agentops-nats-client
  namespace: {{ $ns }}
  labels:
    app.kubernetes.io/name: agentops-nats
spec:
  selector:
    app.kubernetes.io/name: agentops-nats
  ports:
    - name: client
      port: 4222
{{- end -}}

{{- define "nats.pdb" -}}
{{- $root := .root -}}
{{- $v := .nats -}}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: agentops-nats
  namespace: {{ $root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-nats
spec:
  # Quorum needs two of three (D-8): allowing one down keeps the server
  # available through a voluntary eviction. Meaningless at replicas: 1 —
  # rendered anyway, since a template that special-cased it would be a
  # second place this number could drift from the StatefulSet's own.
  maxUnavailable: {{ ($v.podDisruptionBudget | default dict).maxUnavailable | default 1 }}
  selector:
    matchLabels:
      app.kubernetes.io/name: agentops-nats
{{- end -}}

{{- /*
The chart-generated credential (design D-8, spec chart-managed-secrets),
following adapter-auth.yaml / control-plane-auth.yaml / state-read-auth.yaml's
rule exactly: explicit value, then an existing Secret, then generation on
install only — gated the same way, on `agentops.clusterConnected` rather
than `.Release.IsInstall` alone, so a cluster-less render (helm template,
CI, a GitOps repo-server) can never tell "missing" from "unknown" and
mints nothing. Only `password` is ever generated; `user` defaults to the
literal `nats` so a render with no explicit or existing value still needs
no second secret key to exist.
*/ -}}
{{- define "nats.credential" -}}
{{- $root := .root -}}
{{- $v := .nats -}}
{{- $auth := $v.auth | default dict -}}
{{- if not $auth.existingSecret }}
{{- $name := $auth.secretName | default "agentops-nats-credentials" -}}
{{- $explicitUser := $auth.user | default "" -}}
{{- $explicitPassword := $auth.password | default "" -}}
{{- $existing := lookup "v1" "Secret" $root.Release.Namespace $name -}}
{{- $existingUser := "" -}}
{{- $existingPassword := "" -}}
{{- if and $existing $existing.data }}
{{- $existingUser = index $existing.data "user" | default "" -}}
{{- $existingPassword = index $existing.data "password" | default "" -}}
{{- end }}
{{- include "agentops.generatedSecretGuard" (dict "root" $root "name" $name "explicit" $explicitPassword "existing" $existing "existingVal" $existingPassword "key" "password" "setting" "nats.auth.password") -}}
{{- $connected := eq (include "agentops.clusterConnected" $root) "true" -}}
{{- /*
Generation SHALL happen on install only (spec chart-managed-secrets,
"The state server credential is generated once"): unlike
adapter-auth.yaml/control-plane-auth.yaml/state-read-auth.yaml, which mint on
the first CONNECTED upgrade too, an upgrade that finds no existing Secret and
no explicit value for THIS credential fails loudly instead, naming the Secret
and the value — design D-8's own text and the spec's own scenario "An upgrade
that finds no Secret fails loudly". A render with no cluster access still
can't tell "missing" from "unknown" and renders nothing either way.
*/ -}}
{{- if and $connected (not $root.Release.IsInstall) (not $explicitPassword) (not $existingPassword) }}
{{- fail (printf "Secret %q does not exist, or exists without a non-empty password key, and neither nats.auth.password nor nats.auth.existingSecret is set. Set one of them before upgrading to the nats state-store backend." $name) }}
{{- end }}
{{- if or $explicitPassword $existingPassword $root.Release.IsInstall $connected }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ $name }}
  namespace: {{ $root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-nats
  annotations:
    # a render with no cluster (connected=false, not install, not
    # explicit) omits this object entirely, same as every other
    # chart-generated credential — `keep` is what stops THAT renderer's
    # absence of it from reading as a delete once a connected renderer has
    # created it for real
    helm.sh/resource-policy: keep
type: Opaque
data:
  user: {{ if $explicitUser }}{{ $explicitUser | b64enc }}{{ else if $existingUser }}{{ $existingUser }}{{ else }}{{ "nats" | b64enc }}{{ end }}
  password: {{ if $explicitPassword }}{{ $explicitPassword | b64enc }}{{ else if $existingPassword }}{{ $existingPassword }}{{ else }}{{ randAlphaNum 32 | b64enc }}{{ end }}
{{- end }}
{{- end }}
{{- end -}}

{{- /*
D-12: the client port admits the manager alone, the peer (cluster) port
admits only the server's own pods. Every other component reads state
through the manager's /state/* surface and needs no route here.
*/ -}}
{{- define "nats.networkpolicy" -}}
{{- $np := (.root.Values.global.agentops).networkPolicy | default dict -}}
{{- if $np.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agentops-nats
  namespace: {{ .root.Release.Namespace }}
  labels:
    app.kubernetes.io/name: agentops-nats
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: agentops-nats
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
              app.kubernetes.io/name: agentops-nats
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
