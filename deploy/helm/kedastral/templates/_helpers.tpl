{{/*
Expand the name of the chart.
*/}}
{{- define "kedastral.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "kedastral.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "kedastral.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kedastral.labels" -}}
helm.sh/chart: {{ include "kedastral.chart" . }}
{{ include "kedastral.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kedastral.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kedastral.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Forecaster labels
*/}}
{{- define "kedastral.forecaster.labels" -}}
helm.sh/chart: {{ include "kedastral.chart" . }}
{{ include "kedastral.forecaster.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: forecaster
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Forecaster selector labels
*/}}
{{- define "kedastral.forecaster.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kedastral.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: forecaster
{{- end }}

{{/*
Scaler labels
*/}}
{{- define "kedastral.scaler.labels" -}}
helm.sh/chart: {{ include "kedastral.chart" . }}
{{ include "kedastral.scaler.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: scaler
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Scaler selector labels
*/}}
{{- define "kedastral.scaler.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kedastral.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: scaler
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kedastral.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kedastral.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Forecaster image
*/}}
{{- define "kedastral.forecaster.image" -}}
{{- $registry := .Values.global.imageRegistry | default "" -}}
{{- $repository := .Values.forecaster.image.repository -}}
{{- $tag := .Values.forecaster.image.tag | default .Chart.AppVersion -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end }}

{{/*
Scaler image
*/}}
{{- define "kedastral.scaler.image" -}}
{{- $registry := .Values.global.imageRegistry | default "" -}}
{{- $repository := .Values.scaler.image.repository -}}
{{- $tag := .Values.scaler.image.tag | default .Chart.AppVersion -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end }}

{{/*
Forecaster service name
*/}}
{{- define "kedastral.forecaster.serviceName" -}}
{{- printf "%s-forecaster" (include "kedastral.fullname" .) }}
{{- end }}

{{/*
Scaler service name
*/}}
{{- define "kedastral.scaler.serviceName" -}}
{{- printf "%s-scaler" (include "kedastral.fullname" .) }}
{{- end }}
