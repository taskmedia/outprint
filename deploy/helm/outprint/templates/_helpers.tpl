{{- define "outprint.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}{{ .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else -}}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}{{- end -}}
{{- end -}}

{{- define "outprint.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: outprint
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/* call with (dict "root" . "component" "worker") */}}
{{- define "outprint.selector" -}}
app.kubernetes.io/name: outprint
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "outprint.image" -}}
{{ .repository }}:{{ .tag | default .appVersion }}
{{- end -}}

{{- define "outprint.tokenSecretName" -}}
{{- default (printf "%s-worker-token" (include "outprint.fullname" .)) .Values.workerToken.existingSecret -}}
{{- end -}}

{{- define "outprint.tokenEnv" -}}
{{- if .Values.workerToken.enabled }}
- name: OUTPRINT_WORKER_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ include "outprint.tokenSecretName" . }}
      key: token
{{- end }}
{{- end -}}

{{- define "outprint.extraEnv" -}}
{{- range $k, $v := . }}
- name: {{ $k }}
  value: {{ $v | quote }}
{{- end }}
{{- end -}}

{{- define "outprint.containerSecurity" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: [ALL]
{{- end -}}

{{- define "outprint.podSecurity" -}}
runAsNonRoot: true
runAsUser: {{ .uid }}
runAsGroup: {{ .uid }}
seccompProfile:
  type: RuntimeDefault
{{- end -}}
