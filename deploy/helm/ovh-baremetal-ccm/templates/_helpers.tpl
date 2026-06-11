{{/*
Chart name, truncated to 63 chars.
*/}}
{{- define "ovh-baremetal-ccm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name, truncated to 63 chars.
*/}}
{{- define "ovh-baremetal-ccm.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Chart label value.
*/}}
{{- define "ovh-baremetal-ccm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "ovh-baremetal-ccm.labels" -}}
helm.sh/chart: {{ include "ovh-baremetal-ccm.chart" . }}
{{ include "ovh-baremetal-ccm.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels used in matchLabels.
*/}}
{{- define "ovh-baremetal-ccm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ovh-baremetal-ccm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "ovh-baremetal-ccm.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ovh-baremetal-ccm.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Secret name — uses existingSecret if set, otherwise the chart-managed secret.
*/}}
{{- define "ovh-baremetal-ccm.secretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- include "ovh-baremetal-ccm.fullname" . }}
{{- end }}
{{- end }}

{{/*
Image tag — defaults to Chart appVersion.
*/}}
{{- define "ovh-baremetal-ccm.imageTag" -}}
{{- default .Chart.AppVersion .Values.image.tag }}
{{- end }}
