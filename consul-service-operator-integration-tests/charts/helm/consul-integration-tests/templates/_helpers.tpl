{{/*
Find a consul-integration-tests image in various places.
Image can be found from:
* SaaS/App deployer (or groovy.deploy.v3) from .Values.deployDescriptor "consul-integration-tests" "image"
* DP.Deployer from .Values.deployDescriptor.consulIntegrationTests.image
* or from default values .Values.integrationTests.image
*/}}
{{- define "consul-integration-tests.image" -}}
  {{- if .Values.deployDescriptor -}}
    {{- if index .Values.deployDescriptor "consul-integration-tests" -}}
      {{- printf "%s" (index .Values.deployDescriptor "consul-integration-tests" "image") -}}
    {{- else -}}
      {{- printf "%s" (index .Values.deployDescriptor.consulIntegrationTests.image) -}}
    {{- end -}}
  {{- else -}}
    {{- printf "%s" .Values.integrationTests.image -}}
  {{- end -}}
{{- end -}}

{{- define "consul.globalPodSecurityContext" -}}
runAsNonRoot: true
seccompProfile:
  type: "RuntimeDefault"
{{- end -}}

{{- define "consul.globalContainerSecurityContext" -}}
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
{{- end -}}