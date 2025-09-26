{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to
this (by the DNS naming spec). Supports the legacy fullnameOverride setting
as well as the global.name setting.
*/}}
{{- define "consul.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else if .Values.global.name -}}
{{- .Values.global.name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "consul.chart" -}}
{{- printf "%s-helm" .Chart.Name | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "consul.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Convert the passed IP parameter for IPv4 or IPv6 environment.
*/}}
{{- define "consul.convertIPFunction" -}}
{{- $ipv6 := index . 0 -}}
{{- $ip := index . 1 -}}
{{- if $ipv6 -}}
{{- printf "[%s]" $ip -}}
{{- else -}}
{{- $ip -}}
{{- end -}}
{{- end -}}

{{/*
Compute the maximum number of unavailable replicas for the PodDisruptionBudget.
This defaults to (n/2)-1 where n is the number of members of the server cluster.
Special case of replica equaling 3 and allowing a minor disruption of 1 otherwise
use the integer value
Add a special case for replicas=1, where it should default to 0 as well.
*/}}
{{- define "consul.pdb.maxUnavailable" -}}
{{- if eq (int (include "server.replicas" .)) 1 -}}
{{ 0 }}
{{- else if .Values.server.disruptionBudget.maxUnavailable -}}
{{ .Values.server.disruptionBudget.maxUnavailable -}}
{{- else -}}
{{- if eq (int (include "server.replicas" .)) 3 -}}
{{- 1 -}}
{{- else -}}
{{- sub (div (int (include "server.replicas" .)) 2) 1 -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "consul.pdb.connectInject.maxUnavailable" -}}
{{- if eq (int .Values.connectInject.replicas) 1 -}}
{{ 0 }}
{{- else if .Values.connectInject.disruptionBudget.maxUnavailable -}}
{{ .Values.connectInject.disruptionBudget.maxUnavailable -}}
{{- else -}}
{{- if eq (int .Values.connectInject.replicas) 3 -}}
{{- 1 -}}
{{- else -}}
{{- sub (div (int .Values.connectInject.replicas) 2) 1 -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Inject extra environment vars in the format key:value, if populated
*/}}
{{- define "consul.extraEnvironmentVars" -}}
{{- if .extraEnvironmentVars -}}
{{- range $key, $value := .extraEnvironmentVars }}
- name: {{ $key }}
  value: {{ $value | quote }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Consul server environment variables for consul-k8s commands.
*/}}
{{- define "consul.consulK8sConsulServerEnvVars" -}}
- name: CONSUL_ADDRESSES
  {{- if .Values.externalServers.enabled }}
  value: {{ .Values.externalServers.hosts | first }}
  {{- else }}
  value: {{ template "consul.fullname" . }}-server.{{ .Release.Namespace }}.svc
  {{- end }}
- name: CONSUL_GRPC_PORT
  {{- if .Values.externalServers.enabled }}
  value: "{{ .Values.externalServers.grpcPort }}"
  {{- else }}
  value: "8502"
  {{- end }}
- name: CONSUL_HTTP_PORT
  {{- if .Values.externalServers.enabled }}
  value: "{{ .Values.externalServers.httpsPort }}"
  {{- else if .Values.global.tls.enabled }}
  value: "8501"
  {{- else }}
  value: "8500"
  {{- end }}
- name: CONSUL_DATACENTER
  value: {{ .Values.global.datacenter }}
- name: CONSUL_API_TIMEOUT
  value: {{ .Values.global.consulAPITimeout }}
{{- if .Values.global.tls.enabled }}
- name: CONSUL_USE_TLS
  value: "true"
{{- if (not (and .Values.externalServers.enabled .Values.externalServers.useSystemRoots)) }}
- name: CONSUL_CACERT_FILE
  value: "/consul/tls/ca/tls.crt"
{{- end }}
{{- end }}
{{- if and .Values.externalServers.enabled .Values.externalServers.skipServerWatch }}
- name: CONSUL_SKIP_SERVER_WATCH
  value: "true"
{{- end }}
{{- if and .Values.externalServers.enabled .Values.externalServers.tlsServerName }}
- name: CONSUL_TLS_SERVER_NAME
  value: {{ .Values.externalServers.tlsServerName }}
{{- end }}
{{- end -}}

{{/*
Get Consul client CA to use when auto-encrypt is enabled.
This template is for an init container.
*/}}
{{- define "consul.getAutoEncryptClientCA" -}}
- name: get-auto-encrypt-client-ca
  image: {{ template "consul-k8s.image" . }}
  command:
    - "/bin/sh"
    - "-ec"
    - |
      consul-k8s get-consul-client-ca \
        -output-file=/consul/tls/client/ca/tls.crt \
        {{- if .Values.externalServers.enabled }}
        {{- if and .Values.externalServers.enabled (not .Values.externalServers.hosts) }}{{ fail "externalServers.hosts must be set if externalServers.enabled is true" }}{{ end -}}
        -server-addr={{ quote (first .Values.externalServers.hosts) }} \
        -server-port={{ .Values.externalServers.httpsPort }} \
        {{- if .Values.externalServers.tlsServerName }}
        -tls-server-name={{ .Values.externalServers.tlsServerName }} \
        {{- end }}
        {{- if not .Values.externalServers.useSystemRoots }}
        -ca-file=/consul/tls/ca/tls.crt
        {{- end }}
        {{- else }}
        -server-addr={{ template "consul.fullname" . }}-server \
        -server-port=8501 \
        -ca-file=/consul/tls/ca/tls.crt
        {{- end }}
  volumeMounts:
    {{- if not (and .Values.externalServers.enabled .Values.externalServers.useSystemRoots) }}
    - name: consul-ca-cert
      mountPath: /consul/tls/ca
    {{- end }}
    - name: consul-auto-encrypt-ca-cert
      mountPath: /consul/tls/client/ca
  resources:
    requests:
      memory: "50Mi"
      cpu: "50m"
    limits:
      memory: "50Mi"
      cpu: "50m"
{{- end -}}

{{/*
Sets up the extra-from-values config file passed to consul and then uses sed to do any necessary
substitution for HOST_IP/POD_IP/HOSTNAME. Useful for dogstats telemetry. The output file
is passed to consul as a -config-file param on command line.
*/}}
{{- define "consul.extraconfig" -}}
              cp /consul/tmp/extra-config/extra-from-values.json /consul/extra-config/extra-from-values.json
              [ -n "${HOST_IP}" ] && sed -Ei "s|HOST_IP|${HOST_IP?}|g" /consul/extra-config/extra-from-values.json
              [ -n "${POD_IP}" ] && sed -Ei "s|POD_IP|${POD_IP?}|g" /consul/extra-config/extra-from-values.json
              [ -n "${HOSTNAME}" ] && sed -Ei "s|HOSTNAME|${HOSTNAME?}|g" /consul/extra-config/extra-from-values.json
{{- end -}}

{{/*
Configure Consul service 'replicasForSingleService' property
*/}}
{{- define "consul.replicasForSingleService" -}}
{{- and (ne .Values.global.disasterRecovery.mode "standby") (ne .Values.global.disasterRecovery.mode "disabled") | ternary 1 0 -}}
{{- end -}}

{{- define "consul.globalPodSecurityContext" -}}
runAsNonRoot: true
seccompProfile:
  type: "RuntimeDefault"
{{- with .Values.global.securityContext }}
{{ toYaml . }}
{{- end -}}
{{- end -}}

{{- define "consul.globalContainerSecurityContext" -}}
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
{{- end -}}

{{/*
Calculates resources that should be monitored during deployment by Deployment Status Provisioner.
*/}}
{{- define "consul.monitoredResources" -}}
    {{- if (or (and (ne (.Values.backupDaemon.enabled | toString) "-") .Values.backupDaemon.enabled) (and (eq (.Values.backupDaemon.enabled | toString) "-") .Values.global.enabled)) }}
    {{- printf "Deployment %s-backup-daemon, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if eq (.Values.client.enabled | toString) "true" }}
    {{- printf "DaemonSet %s, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if (or (and (ne (.Values.connectInject.enabled | toString) "-") .Values.connectInject.enabled) (and (eq (.Values.connectInject.enabled | toString) "-") .Values.global.enabled)) }}
    {{- printf "Deployment %s-connect-injector-webhook-deployment, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if .Values.meshGateway.enabled }}
    {{- printf "Deployment %s-mesh-gateway, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if (or (and (ne (.Values.server.enabled | toString) "-") .Values.server.enabled) (and (eq (.Values.server.enabled | toString) "-") .Values.global.enabled)) }}
    {{- printf "StatefulSet %s-server, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if (or (and (ne (.Values.syncCatalog.enabled | toString) "-") .Values.syncCatalog.enabled) (and (eq (.Values.syncCatalog.enabled | toString) "-") .Values.global.enabled)) }}
    {{- printf "Deployment %s-sync-catalog, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if .Values.consulAclConfigurator.enabled }}
    {{- printf "Deployment %s-operator, " (include "consul-acl-configurator.name" .) -}}
    {{- end }}
    {{ if eq (include "consul.enableDisasterRecovery" .) "true" }}
    {{- printf "Deployment %s-disaster-recovery, " (include "consul.fullname" .) -}}
    {{- end }}
    {{ if eq (include "pod-scheduler.enabled" .) "true" }}
    {{- printf "Deployment %s-pod-scheduler, " (include "consul.fullname" .) -}}
    {{- end }}
    {{- if .Values.integrationTests.enabled }}
    {{- printf "Deployment %s, " (include "consul-integration-tests.name" .) -}}
    {{- end }}
{{- end -}}

{{/*
Whether Disaster Recovery is enabled for Consul
*/}}
{{- define "consul.enableDisasterRecovery" -}}
{{- or (eq .Values.global.disasterRecovery.mode "active") (eq .Values.global.disasterRecovery.mode "standby") (eq .Values.global.disasterRecovery.mode "disabled") -}}
{{- end -}}

{{/*
Whether TLS is enabled for Disaster Recovery in Consul
*/}}
{{- define "disaster-recovery.tlsEnabled" -}}
{{- and (eq (include "consul.enableDisasterRecovery" .) "true") .Values.global.tls.enabled .Values.global.disasterRecovery.tls.enabled -}}
{{- end -}}

{{/*
Cipher suites that can be used in Disaster Recovery
*/}}
{{- define "disaster-recovery.cipherSuites" -}}
{{- join "," (coalesce .Values.global.disasterRecovery.tls.cipherSuites .Values.global.tls.cipherSuites) -}}
{{- end -}}

{{/*
Disaster Recovery Port
*/}}
{{- define "disaster-recovery.port" -}}
  {{- if and .Values.global.tls.enabled .Values.global.disasterRecovery.tls.enabled -}}
    {{- "8443" -}}
  {{- else -}}
    {{- "8080" -}}
  {{- end -}}
{{- end -}}

{{/*
Whether DRD certificates are specified
*/}}
{{- define "disaster-recovery.certificatesSpecified" -}}
  {{- $filled := false -}}
  {{- range $key, $value := .Values.global.disasterRecovery.tls.certificates -}}
    {{- if $value -}}
        {{- $filled = true -}}
    {{- end -}}
  {{- end -}}
  {{- $filled -}}
{{- end -}}

{{/*
TLS secret name for Disaster Recovery
*/}}
{{- define "disaster-recovery.certSecretName" -}}
  {{- if and .Values.global.tls.enabled .Values.global.disasterRecovery.tls.enabled -}}
    {{- if .Values.global.disasterRecovery.tls.secretName }}
      {{- .Values.global.disasterRecovery.tls.secretName -}}
    {{- else }}
      {{- template "consul.fullname" . -}}-drd-tls-secret
    {{- end -}}
  {{- else }}
    {{- "" }}
  {{- end }}
{{- end -}}

{{/*
DNS names used to generate TLS certificate with "Subject Alternative Name" field for Disaster Recovery
*/}}
{{- define "disaster-recovery.certDnsNames" -}}
  {{- $dnsNames := list "localhost" (printf "%s-disaster-recovery" (include "consul.fullname" .)) (printf "%s-disaster-recovery.%s" (include "consul.fullname" .) .Release.Namespace) (printf "%s-disaster-recovery.%s.svc.cluster.local" (include "consul.fullname" .) .Release.Namespace) -}}
  {{- $dnsNames = concat $dnsNames .Values.global.disasterRecovery.tls.subjectAlternativeName.additionalDnsNames -}}
  {{- $dnsNames | toYaml -}}
{{- end -}}

{{/*
IP addresses used to generate TLS certificate with "Subject Alternative Name" field for Disaster Recovery
*/}}
{{- define "disaster-recovery.certIpAddresses" -}}
  {{- $ipAddresses := list "127.0.0.1" -}}
  {{- $ipAddresses = concat $ipAddresses .Values.global.disasterRecovery.tls.subjectAlternativeName.additionalIpAddresses -}}
  {{- $ipAddresses | toYaml -}}
{{- end -}}

{{/*
Generate certificates for Disaster Recovery
*/}}
{{- define "disaster-recovery.generateCerts" -}}
{{- $dnsNames := include "disaster-recovery.certDnsNames" . | fromYamlArray -}}
{{- $ipAddresses := include "disaster-recovery.certIpAddresses" . | fromYamlArray -}}
{{- $duration := default 365 .Values.global.tls.certManager.durationDays | int -}}
{{- $ca := genCA "consul-drd-ca" $duration -}}
{{- $drdName := "drd" -}}
{{- $cert := genSignedCert $drdName $ipAddresses $dnsNames $duration $ca -}}
tls.crt: {{ $cert.Cert | b64enc }}
tls.key: {{ $cert.Key | b64enc }}
ca.crt: {{ $ca.Cert | b64enc }}
{{- end -}}

{{/*
Specify the protocol for Disaster Recovery.
*/}}
{{- define "disaster-recovery.protocol" -}}
{{- eq (include "disaster-recovery.tlsEnabled" .) "true" | ternary "https" "http" -}}
{{- end -}}

{{/*
Specify the name for Consul ACL Configurator.
*/}}
{{- define "consul-acl-configurator.name" -}}
    {{- printf "%s-acl-configurator" (include "consul.fullname" .) -}}
{{- end -}}

{{/*
Specify the name for Consul integration tests runner.
*/}}
{{- define "consul-integration-tests.name" -}}
    {{- printf "%s-integration-tests-runner" (include "consul.fullname" .) -}}
{{- end -}}

{{/*
Collect all settings for Consul telemetry configuration
*/}}
{{- define "consul-telemetry.configuration" -}}
    {{- if (or (eq (include "monitoring.enabled" .) "true") (and (eq (.Values.monitoring.enabled | toString) "-") .Values.global.enabled)) -}}
    statsd_address = "localhost:8125", {{ end -}}
    {{- if (and .Values.global.metrics.enabled .Values.global.metrics.enableAgentMetrics) -}}
    prometheus_retention_time = "{{ .Values.global.metrics.agentMetricsRetentionTime }}", {{ end -}}
    disable_hostname = {{ .Values.global.metrics.disableHostname }}
{{- end -}}

{{/*
Find a consul image in various places.
*/}}
{{- define "consul.image" -}}
    {{- printf "%s" .Values.global.image -}}
{{- end -}}

{{/*
Find a consul-k8s image in various places.
*/}}
{{- define "consul-k8s.image" -}}
    {{- printf "%s" .Values.global.imageK8S -}}
{{- end -}}

{{/*
Find a Consul DataPlane image in various places.
*/}}
{{- define "consul-dataplane.image" -}}
    {{- printf "%s" .Values.global.imageConsulDataplane -}}
{{- end -}}

{{/*
Find a disaster recovery image in various places
*/}}
{{- define "disaster-recovery.image" -}}
    {{- printf "%s" .Values.global.disasterRecovery.image -}}
{{- end -}}

{{/*
Find a consul-backup-daemon image in various places.
*/}}
{{- define "consul-backup-daemon.image" -}}
    {{- printf "%s" .Values.backupDaemon.image -}}
{{- end -}}

{{/*
Find a consul-acl-configurator-operator image in various places.
*/}}
{{- define "consul-acl-configurator-operator.image" -}}
    {{- printf "%s" .Values.consulAclConfigurator.operatorImage -}}
{{- end -}}

{{/*
Find a consul-acl-configurator-rest-server image in various places.
*/}}
{{- define "consul-acl-configurator-rest-server.image" -}}
    {{- printf "%s" .Values.consulAclConfigurator.restServerImage -}}
{{- end -}}


{{- define "consul-integration-tests.image" -}}
    {{- printf "%s" .Values.integrationTests.image -}}
{{- end -}}

{{/*
Find a Deployment Status Provisioner image in various places.
*/}}
{{- define "deployment-status-provisioner.image" -}}
    {{- printf "%s" .Values.statusProvisioner.dockerImage -}}
{{- end -}}

{{- define "remove-tokens.image" -}}
    {{- printf "%s" .Values.consulAclConfigurator.removeTokens.dockerImage -}}
{{- end -}}

{{/*
Returns Consul port to communicate.
*/}}
{{- define "consul.port" -}}
  {{- if .Values.global.tls.enabled -}}
    {{- "8501" -}}
  {{- else -}}
    {{- "8500" -}}
  {{- end -}}
{{- end -}}

{{/*
Determine the HTTP port based on global, server and client setings.
*/}}
{{- define "consul.port.http" -}}
  {{- if .Values.server.ports.http -}}
    {{- .Values.server.ports.http -}}
  {{- else if .Values.global.ports.http -}}
    {{- .Values.global.ports.http -}}
  {{- else -}}
    {{- "8500" -}}
  {{- end -}}
{{- end -}}

{{/*
Determine the server HTTPS port.
*/}}
{{- define "consul.port.https" -}}
  {{- if .Values.server.ports.https -}}
    {{- .Values.server.ports.https -}}
  {{- else if .Values.global.ports.https -}}
    {{- .Values.global.ports.https -}}
  {{- else -}}
    {{- "8501" -}}
  {{- end -}}
{{- end -}}

{{/*
Determine the grpcPort based on global, server and client settings.
*/}}
{{- define "consul.port.grpc" -}}
  {{- if .Values.server.ports.grpc -}}
    {{- .Values.server.ports.grpc -}}
  {{- else if .Values.global.ports.grpc -}}
    {{- .Values.global.ports.grpc -}}
  {{- else -}}
    {{- 8502 -}}
  {{- end -}}
{{- end -}}

{{/*
Determine the client HTTP port.
*/}}
{{- define "consul.client.port.http" -}}
  {{- if .Values.client.ports.http -}}
    {{- .Values.client.ports.http -}}
  {{- else if .Values.global.ports.http -}}
    {{- .Values.global.ports.http -}}
  {{- else -}}
    {{- "8500" -}}
  {{- end -}}
{{- end -}}

{{/*
Determine the client HTTPS port.
*/}}
{{- define "consul.client.port.https" -}}
  {{- if .Values.client.ports.https -}}
    {{- .Values.client.ports.https -}}
  {{- else if .Values.global.ports.https -}}
    {{- .Values.global.ports.https -}}
  {{- else -}}
    {{- "8501" -}}
  {{- end -}}
{{- end -}}

{{/*
Determine the client grpc port.
*/}}
{{- define "consul.client.port.grpc" -}}
  {{- if .Values.client.ports.grpc -}}
    {{- .Values.client.ports.grpc -}}
  {{- else if .Values.global.ports.grpc -}}
    {{- .Values.global.ports.grpc -}}
  {{- else -}}
    {{- "8502" -}}
  {{- end -}}
{{- end -}}

{{/*
Returns Consul protocol scheme to communicate.
*/}}
{{- define "consul.scheme" -}}
  {{- if .Values.global.tls.enabled -}}
    {{- "https" -}}
  {{- else -}}
    {{- "http" -}}
  {{- end -}}
{{- end -}}

{{/*
Backup Daemon TLS enabled
*/}}
{{- define "consul.tlsEnabled" -}}
{{- .Values.global.tls.enabled -}}
{{- end -}}

{{/*
Whether Consul Server certificates are specified
*/}}
{{- define "server.certificatesSpecified" -}}
  {{- $filled := false -}}
  {{- range $key, $value := .Values.server.tls.certificates -}}
    {{- if $value -}}
        {{- $filled = true -}}
    {{- end -}}
  {{- end -}}
  {{- $filled -}}
{{ end }}

{{/*
Consul Server TLS secret name
*/}}
{{- define "server.tlsSecretName" -}}
  {{- if eq (include "server.certificatesSpecified" .) "true" -}}
    {{- template "consul.fullname" . }}-server-outer-cert
  {{- else -}}
    {{- template "consul.fullname" . }}-server-cert
  {{- end -}}
{{- end -}}

{{/*
Whether Consul Client certificates are specified
*/}}
{{- define "client.certificatesSpecified" -}}
  {{- $filled := false -}}
  {{- range $key, $value := .Values.client.tls.certificates -}}
    {{- if $value -}}
        {{- $filled = true -}}
    {{- end -}}
  {{- end -}}
  {{- $filled -}}
{{ end }}

{{/*
TLS Static Metric secret template
Arguments:
Dictionary with:
* "namespace" is a namespace of application
* "application" is name of application
* "service" is a name of service
* "enableTls" is TLS enabled for service
* "secret" is a name of tls secret for service
* "certProvider" is a type of tls certificates provider
* "certificate" is a name of CertManger's Certificate resource for service
Usage example:
{{template "global.tlsStaticMetric" (dict "namespace" .Release.Namespace "application" .Chart.Name "service" .global.name "enableTls" (include "global.tlsEnabled" .) "secret" (include "global.tlsSecretName" .) "certProvider" (include "services.certProvider" .) "certificate" (printf "%s-tls-certificate" (include "global.name")) }}
*/}}
{{- define "global.tlsStaticMetric" -}}
- expr: {{ ternary "1" "0" (eq .enableTls "true") }}
  labels:
    namespace: "{{ .namespace }}"
    application: "{{ .application }}"
    service: "{{ .service }}"
    {{ if eq .enableTls "true" }}
    secret: "{{ .secret }}"
    {{ if .certManager }}
    certificate: "{{ .certificate }}"
    {{ end }}
    {{ end }}
  record: service:tls_status:info
{{- end -}}

{{/*
Whether forced cleanup of previous consul-status-provisioner job is enabled
*/}}
{{- define "consul-status-provisioner.cleanupEnabled" -}}
  {{- if .Values.statusProvisioner.enabled -}}
    {{- $cleanupEnabled := .Values.statusProvisioner.cleanupEnabled | toString }}
    {{- if eq $cleanupEnabled "true" -}}
      {{- printf "true" }}
    {{- else if eq $cleanupEnabled "false" -}}
      {{- printf "false" -}}
    {{- else -}}
      {{- if or (gt .Capabilities.KubeVersion.Major "1") (ge .Capabilities.KubeVersion.Minor "21") -}}
        {{- printf "false" -}}
      {{- else -}}
        {{- printf "true" -}}
      {{- end -}}
    {{- end -}}
  {{- else -}}
    {{- printf "false" -}}
  {{- end -}}
{{- end -}}

{{/*
DNS names used to generate TLS certificate with "Subject Alternative Name" field
*/}}
{{- define "consul.certDnsNames" -}}
  {{- $consulServerName := printf "%s-server" (include "consul.fullname" .) -}}
  {{- $dnsNames := list "localhost" $consulServerName (printf "*.%s" $consulServerName) (printf "*.%s.%s" $consulServerName .Release.Namespace) (printf "%s.%s" $consulServerName .Release.Namespace) (printf "*.%s.%s.svc" $consulServerName .Release.Namespace) (printf "%s.%s.svc" $consulServerName .Release.Namespace) (printf "server.%s.%s" .Values.global.datacenter .Values.global.domain) (printf "*.server.%s.%s" .Values.global.datacenter .Values.global.domain) -}}
  {{- $dnsNames = concat $dnsNames .Values.global.tls.serverAdditionalDNSSANs -}}
  {{- $dnsNames | toYaml -}}
{{- end -}}

{{/*
IP addresses used to generate TLS certificate with "Subject Alternative Name" field
*/}}
{{- define "consul.certIpAddresses" -}}
  {{- $ipAddresses := list "127.0.0.1" -}}
  {{- $ipAddresses = concat $ipAddresses .Values.global.tls.serverAdditionalIPSANs -}}
  {{- $ipAddresses | toYaml -}}
{{- end -}}

{{/*
Consul CA cert secret name
*/}}
{{- define "consul.caCertSecretName" -}}
  {{- if .Values.global.tls.certManager.enabled }}
    {{- template "consul.fullname" . }}-ca-cert
  {{- else if eq (include "server.certificatesSpecified" .) "true" -}}
    {{- template "consul.fullname" . }}-outer-ca-cert
  {{- else  if .Values.global.tls.caCert.secretName }}
    {{- .Values.global.tls.caCert.secretName }}
  {{- else }}
    {{- template "consul.fullname" . }}-ca-cert
  {{- end }}
{{- end -}}

{{/*
Consul CA key secret name
*/}}
{{- define "consul.caCertKeySecretName" -}}
  {{- if .Values.global.tls.certManager.enabled -}}
    {{- printf "%s-ca-cert" (include "consul.fullname" . ) -}}
  {{- else -}}
    {{- printf "%s-ca-key" (include "consul.fullname" . ) -}}
  {{- end -}}
{{- end -}}

{{/*
Backup Daemon TLS enabled
*/}}
{{- define "backup-daemon.tlsEnabled" -}}
{{- and .Values.global.tls.enabled .Values.backupDaemon.tls.enabled -}}
{{- end -}}

{{/*
Whether Backup Daemon certificates are specified
*/}}
{{- define "backupDaemon.certificatesSpecified" -}}
  {{- $filled := false -}}
  {{- range $key, $value := .Values.backupDaemon.tls.certificates -}}
    {{- if $value -}}
        {{- $filled = true -}}
    {{- end -}}
  {{- end -}}
  {{- $filled -}}
{{ end }}

{{/*
Backup Daemon TLS secret name
*/}}
{{- define "backupDaemon.tlsSecretName" -}}
  {{- if and .Values.global.tls.enabled .Values.backupDaemon.tls.enabled -}}
    {{- if .Values.backupDaemon.tls.secretName -}}
      {{- .Values.backupDaemon.tls.secretName -}}
    {{- else -}}
      {{- printf "%s-backup-daemon-tls-secret" (include "consul.fullname" .) -}}
    {{- end -}}
  {{- else -}}
    {{- "" -}}
  {{- end -}}
{{- end -}}

{{/*
DNS names used to generate TLS certificate with "Subject Alternative Name" field for Backup Daemon
*/}}
{{- define "backupDaemon.certDnsNames" -}}
  {{- $backupDaemonNamespace := .Release.Namespace -}}
  {{- $dnsNames := list "localhost" (printf "%s-backup-daemon" (include "consul.fullname" .)) (printf "%s-backup-daemon.%s" (include "consul.fullname" .) $backupDaemonNamespace) (printf "%s-backup-daemon.%s.svc.cluster.local" (include "consul.fullname" .) $backupDaemonNamespace) -}}
  {{- $dnsNames = concat $dnsNames .Values.backupDaemon.tls.subjectAlternativeName.additionalDnsNames -}}
  {{- $dnsNames | toYaml -}}
{{- end -}}

{{/*
IP addresses used to generate TLS certificate with "Subject Alternative Name" field for Backup Daemon
*/}}
{{- define "backupDaemon.certIpAddresses" -}}
  {{- $ipAddresses := list "127.0.0.1" -}}
  {{- $ipAddresses = concat $ipAddresses .Values.backupDaemon.tls.subjectAlternativeName.additionalIpAddresses -}}
  {{- $ipAddresses | toYaml -}}
{{- end -}}

{{/*
Generate certificates for Backup Daemon
*/}}
{{- define "backupDaemon.generateCerts" -}}
  {{- $dnsNames := include "backupDaemon.certDnsNames" . | fromYamlArray -}}
  {{- $ipAddresses := include "backupDaemon.certIpAddresses" . | fromYamlArray -}}
  {{- $duration := default 365 .Values.global.tls.certManager.durationDays | int -}}
  {{- $ca := genCA "consul-backup-daemon-ca" $duration -}}
  {{- $backupDaemonName := "backupDaemon" -}}
  {{- $cert := genSignedCert $backupDaemonName $ipAddresses $dnsNames $duration $ca -}}
tls.crt: {{ $cert.Cert | b64enc }}
tls.key: {{ $cert.Key | b64enc }}
ca.crt: {{ $ca.Cert | b64enc }}
{{- end -}}

{{/*
Backup Daemon Protocol
*/}}
{{- define "backupDaemon.protocol" -}}
  {{- if and .Values.global.tls.enabled .Values.backupDaemon.tls.enabled -}}
    {{- "https" -}}
  {{- else -}}
    {{- "http" -}}
  {{- end -}}
{{- end -}}

{{/*
Backup Daemon Port
*/}}
{{- define "backupDaemon.port" -}}
  {{- if and .Values.global.tls.enabled .Values.backupDaemon.tls.enabled -}}
    {{- "8443" -}}
  {{- else -}}
    {{- "8080" -}}
  {{- end -}}
{{- end -}}

{{/*
Is scheduler enabled
*/}}
{{- define "pod-scheduler.enabled" -}}
  {{- if and .Values.podScheduler.enabled .Values.server.nodes }}
    {{- "true" -}}
  {{- else }}
    {{- "false" -}}
  {{- end -}}
{{- end -}}

{{- define "remove-tokens.enabled" -}}
  {{- if .Values.consulAclConfigurator.removeTokens.enabled}}
    {{- "true" -}}
  {{- else }}
    {{- "false" -}}
  {{- end -}}
{{- end -}}

{{/*
Find a kubectl image in various places.
*/}}
{{- define "kubectl.image" -}}
    {{- printf "%s" .Values.podScheduler.dockerImage -}}
{{- end -}}

{{/*
Define standard labels for frequently used metadata.
*/}}
{{- define "consul.labels" -}}
app: {{ template "consul.fullname" . }}
chart: {{ template "consul.chart" . }}
release: "{{ .Release.Name }}"
heritage: "{{ .Release.Service }}"
{{- end -}}

{{/*
The most common Consul operator chart related resources labels
*/}}
{{- define "consul-service.coreLabels" -}}
app.kubernetes.io/version: '{{ .Values.ARTIFACT_DESCRIPTOR_VERSION | trunc 63 | trimAll "-_." }}'
app.kubernetes.io/part-of: '{{ .Values.PART_OF }}'
{{- end -}}

{{/*
Core Consul operator chart related resources labels with backend component label
*/}}
{{- define "consul-service.defaultLabels" -}}
{{ include "consul-service.coreLabels" . }}
app.kubernetes.io/component: 'backend'
{{- end -}}

{{- define "consul.monitoredImages" -}}
  {{- if .Values.consulAclConfigurator.enabled -}}
    {{- printf "deployment %s-operator consul-acl-configurator-operator %s, " (include "consul-acl-configurator.name" .) (include "consul-acl-configurator-operator.image" .) -}}
    {{- printf "deployment %s-operator consul-acl-configurator-rest-server %s, " (include "consul-acl-configurator.name" .) (include "consul-acl-configurator-rest-server.image" .) -}}
  {{- end -}}
  {{- if .Values.server.enabled -}}
    {{- printf "statefulset %s-server consul %s, " (include "consul.fullname" .) (include "consul.image" .) -}}
  {{- end -}}
  {{- if .Values.client.enabled -}}
    {{- printf "daemonset %s consul %s, " (include "consul.fullname" .) (include "consul.image" .) -}}
  {{- end -}}
  {{- if eq (include "consul.enableDisasterRecovery" .) "true" -}}
    {{- printf "deployment %s-disaster-recovery consul-disaster-recovery %s, " (include "consul.fullname" .) (include "disaster-recovery.image" .) -}}
  {{- end -}}
  {{- if .Values.backupDaemon.enabled -}}
    {{- printf "deployment %s-backup-daemon consul-backup-daemon %s, " (include "consul.fullname" .) (include "consul-backup-daemon.image" .) -}}
  {{- end -}}
  {{- if .Values.integrationTests.enabled -}}
    {{- printf "deployment %s %s %s, " (include "consul-integration-tests.name" .) (include "consul-integration-tests.name" .) (include "consul-integration-tests.image" .) -}}
  {{- end -}}
{{- end -}}

{{/*
Is Openshift enabled.
*/}}
{{- define "openshift.enabled" -}}
  {{- if (ne (.Values.PAAS_PLATFORM_CUSTOM | toString) "<nil>") -}}
    {{- not (eq .Values.PAAS_PLATFORM_CUSTOM "KUBERNETES") }}
  {{- else -}}
    {{- if (ne (.Values.PAAS_PLATFORM | toString) "<nil>") -}}
      {{- not (eq .Values.PAAS_PLATFORM "KUBERNETES") }}
    {{- else -}}
      {{- .Values.global.openshift.enabled  }}
    {{- end -}}
  {{- end -}}
{{- end -}}

Enable Prometheus monitoring.
*/}}
{{- define "monitoring.enabled" -}}
  {{- if and (ne (.Values.MONITORING_ENABLED | toString) "<nil>") .Values.global.cloudIntegrationEnabled -}}
    {{- .Values.MONITORING_ENABLED }}
  {{- else -}}
    {{- and (ne (.Values.monitoring.enabled | toString) "-") .Values.monitoring.enabled -}}
  {{- end -}}
{{- end -}}

{{/*
Server storage class from various places.
*/}}
{{- define "server.storageClass" -}}
  {{- if and (ne (.Values.STORAGE_RWO_CLASS | toString) "<nil>") .Values.global.cloudIntegrationEnabled -}}
    {{- .Values.STORAGE_RWO_CLASS }}
  {{- else -}}
    {{- default "" .Values.server.storageClass -}}
  {{- end -}}
{{- end -}}

{{/*
Backup Daemon storage class from various places.
*/}}
{{- define "backupDaemon.storageClass" -}}
  {{- if and (ne (.Values.STORAGE_RWO_CLASS | toString) "<nil>") .Values.global.cloudIntegrationEnabled -}}
    {{- .Values.STORAGE_RWO_CLASS }}
  {{- else -}}
    {{- default "" .Values.backupDaemon.storageClass -}}
  {{- end -}}
{{- end -}}

{{/*
Server security context
*/}}
{{- define "server.securityContext" -}}
{{- include "consul.globalPodSecurityContext" . }}
{{ with .Values.server.securityContext }}
{{- toYaml . -}}
{{- end }}
{{- if and (ne (.Values.INFRA_CONSUL_FS_GROUP | toString) "<nil>") .Values.global.cloudIntegrationEnabled }}
fsGroup: {{ .Values.INFRA_CONSUL_FS_GROUP }}
{{- end }}
{{- end -}}

{{/*
Server replicas from various places.
*/}}
{{- define "server.replicas" -}}
  {{- if and (ne (.Values.INFRA_CONSUL_REPLICAS | toString) "<nil>") .Values.global.cloudIntegrationEnabled -}}
    {{- .Values.INFRA_CONSUL_REPLICAS }}
  {{- else -}}
    {{- .Values.server.replicas -}}
  {{- end -}}
{{- end -}}

{{/*
Restricted environment.
*/}}
{{- define "consul.restrictedEnvironment" -}}
  {{- if and (ne (.Values.INFRA_CONSUL_RESTRICTED_ENVIRONMENT | toString) "<nil>") .Values.global.cloudIntegrationEnabled -}}
    {{- .Values.INFRA_CONSUL_RESTRICTED_ENVIRONMENT }}
  {{- else -}}
    {{- .Values.global.restrictedEnvironment -}}
  {{- end -}}
{{- end -}}

{{/*
Whether ingress for Consul enabled
*/}}
{{- define "consul.ingressEnabled" -}}
  {{- if and (ne (.Values.PRODUCTION_MODE | toString) "<nil>") .Values.global.cloudIntegrationEnabled}}
    {{- and (eq .Values.PRODUCTION_MODE false) .Values.ui.ingress.enabled | ternary .Values.ui.ingress.enabled "" -}}
  {{- else -}}
    {{- default "" .Values.ui.ingress.enabled }}
  {{- end -}}
{{- end -}}


{{/*
Backup Daemon SSL secret name
*/}}
{{- define "backupDaemon.s3.tlsSecretName" -}}
  {{- if .Values.backupDaemon.s3.sslCert -}}
    {{- if .Values.backupDaemon.s3.sslSecretName -}}
      {{- .Values.backupDaemon.s3.sslSecretName -}}
    {{- else -}}
      {{- printf "consul-backup-daemon-s3-tls-secret" -}}
    {{- end -}}
  {{- else -}}
    {{- if .Values.backupDaemon.s3.sslSecretName -}}
      {{- .Values.backupDaemon.s3.sslSecretName -}}
    {{- else -}}
      {{- printf "" -}}
    {{- end -}}
  {{- end -}}
{{- end -}}

{{/*
Service Account for Site Manager depending on smSecureAuth
*/}}
{{- define "disasterRecovery.siteManagerServiceAccount" -}}
  {{- if .Values.global.disasterRecovery.httpAuth.smServiceAccountName -}}
    {{- .Values.global.disasterRecovery.httpAuth.smServiceAccountName -}}
  {{- else -}}
    {{- if .Values.global.disasterRecovery.httpAuth.smSecureAuth -}}
      {{- "site-manager-sa" -}}
    {{- else -}}
      {{- "sm-auth-sa" -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
