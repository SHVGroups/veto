{{- define "veto.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "veto.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "veto.secretName" -}}
{{- default (printf "%s-secrets" (include "veto.fullname" .)) .Values.secrets.existingSecret -}}
{{- end -}}

{{- define "veto.databaseSecretName" -}}
{{- default (include "veto.secretName" .) .Values.database.existingSecret -}}
{{- end -}}

{{- define "veto.databaseSecretKey" -}}
{{- if .Values.database.existingSecret -}}{{ .Values.database.existingSecretKey }}{{- else -}}database-url{{- end -}}
{{- end -}}

{{/*
Environment shared by the server and worker pods. Call with a dict holding
"root" (the chart context) and "workers" (whether these pods run jobs).
*/}}
{{- define "veto.serverEnv" -}}
{{- $root := .root -}}
{{- $v := .root.Values -}}
- { name: VETO_LISTEN_ADDR, value: ":8080" }
- { name: VETO_PUBLIC_URL, value: {{ $v.publicUrl | quote }} }
- { name: VETO_WORKERS, value: {{ .workers | toString | quote }} }
- { name: VETO_JEV_MODEL, value: {{ $v.jev.model | quote }} }
{{- with $v.jev.fallbacks }}
- { name: VETO_JEV_FALLBACKS, value: {{ . | quote }} }
{{- end }}
# Console pods forward requests; trust their forwarding headers.
- { name: VETO_TRUSTED_PROXIES, value: "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16" }
- name: VETO_DATABASE_URL
  valueFrom: { secretKeyRef: { name: {{ include "veto.databaseSecretName" $root }}, key: {{ include "veto.databaseSecretKey" $root }} } }
- name: VETO_MASTER_KEY
  valueFrom: { secretKeyRef: { name: {{ include "veto.secretName" $root }}, key: master-key } }
- name: VETO_MASTER_KEY_PREVIOUS
  valueFrom: { secretKeyRef: { name: {{ include "veto.secretName" $root }}, key: master-key-previous, optional: true } }
- name: VETO_JEV_API_KEY
  valueFrom: { secretKeyRef: { name: {{ include "veto.secretName" $root }}, key: jev-api-key, optional: true } }
- name: VETO_SETUP_TOKEN
  valueFrom: { secretKeyRef: { name: {{ include "veto.secretName" $root }}, key: setup-token, optional: true } }
- name: VETO_METRICS_TOKEN
  valueFrom: { secretKeyRef: { name: {{ include "veto.secretName" $root }}, key: metrics-token, optional: true } }
{{- range $name, $value := $v.server.env }}
- { name: {{ $name }}, value: {{ $value | quote }} }
{{- end }}
{{- end -}}
