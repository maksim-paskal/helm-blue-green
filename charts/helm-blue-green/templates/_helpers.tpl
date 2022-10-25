{{- define "common.names.fullname" -}}
{{- printf "%s-%s" .Release.Name "helm-blue-green" | trunc 63 | trimSuffix "-" -}}
{{- end -}}