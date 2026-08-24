{{- /*
The bundle renders when enabled. Unlike kubernetes there is no demo-mode OR, so
the parent chart can use a plain Helm `condition:` — but every template still
self-gates, so the subchart behaves the same however it is invoked.
*/ -}}
{{- define "telegram.active" -}}
{{- if .Values.enabled -}}true{{- end -}}
{{- end -}}

{{- /*
The chat surface is opt-in and EXPLICIT: surface.enabled says "I want a
Channel and a chat source", and everything that cannot be guessed is then
required. Asking outright beats inferring intent from which fields happen to be
filled — a typo'd key reads as "no surface wanted" under inference, and you get
a silent half-install instead of an error.
*/ -}}
{{- define "telegram.surfaceEnabled" -}}
{{- if .Values.surface.enabled -}}
{{- include "telegram.validateSurface" . -}}
true
{{- end -}}
{{- end -}}

{{- /* Everything surface.enabled makes mandatory, checked in one place. */ -}}
{{- define "telegram.validateSurface" -}}
{{- $s := .Values.surface -}}
{{- $c := $s.credentials -}}
{{- if not $s.chatId -}}
{{- fail "telegram: surface.enabled is true but surface.chatId is not set — the Telegram chat id (a forum supergroup) cannot be guessed. Set it, or set surface.enabled=false to ship the adapters only." -}}
{{- end -}}
{{- if and $c.existingSecret $c.botToken -}}
{{- fail (printf "telegram: set EITHER surface.credentials.existingSecret (%v) OR surface.credentials.botToken, not both — two sources for one bot token is ambiguous." $c.existingSecret) -}}
{{- end -}}
{{- if not (or $c.existingSecret $c.botToken) -}}
{{- fail "telegram: surface.enabled is true but no bot credential is set — give surface.credentials.existingSecret (a Secret with key botToken) or surface.credentials.botToken (the bundle then creates the Secret). Both the Channel and the router need it." -}}
{{- end -}}
{{- end -}}

{{- /* The Secret name the Channel and the router both reference: an existing
one, or the one this bundle creates from a supplied token. */ -}}
{{- define "telegram.secretName" -}}
{{- $c := .Values.surface.credentials -}}
{{- if $c.existingSecret -}}
{{- $c.existingSecret -}}
{{- else -}}
{{- $c.secretName | default (printf "%s-telegram" .Values.surface.name) -}}
{{- end -}}
{{- end -}}

{{- /* Does this bundle own the Secret (created from a supplied token)? */ -}}
{{- define "telegram.createSecret" -}}
{{- if and .Values.surface.credentials.botToken (not .Values.surface.credentials.existingSecret) -}}
true
{{- end -}}
{{- end -}}

{{- /* Chat SignalSource name: explicit, else the surface name itself. The
Channel, the chat SignalSource and the Pipeline claiming it are all one surface,
so they share one name — different kinds never collide, and "home-ops" then
means the whole thing rather than three near-misses. */ -}}
{{- define "telegram.sourceName" -}}
{{- .Values.surface.source.name | default .Values.surface.name -}}
{{- end -}}

{{- /* In-cluster base URLs of the two receiving adapters. The reconcilers own
these Services under deterministic names, and the chart injects both into the
router's Deployment as env — the router reads no CR. */ -}}
{{- define "telegram.signalTarget" -}}
http://agentops-signal-{{ .Values.signalAdapter.name }}.{{ .Release.Namespace }}.svc:{{ .Values.signalAdapter.port }}
{{- end -}}

{{- define "telegram.channelTarget" -}}
http://agentops-adapter-{{ .Values.channelAdapter.name }}.{{ .Release.Namespace }}.svc:{{ .Values.channelAdapter.port }}
{{- end -}}
