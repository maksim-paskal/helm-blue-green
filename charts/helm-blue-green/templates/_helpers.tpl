{{- define "helm-blue-green.versionSuffix" -}}
{{- $version := ternary .version "" .enabled }}
{{- ternary (printf "-%s" $version) "" .enabled }}
{{- end -}}