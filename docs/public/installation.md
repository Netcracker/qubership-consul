The following topics are covered in this chapter:

<!-- TOC -->
* [Prerequisites](#prerequisites)
  * [Common](#common)
    * [Custom Resource Definitions](#custom-resource-definitions)
    * [Deployment Permissions](#deployment-permissions)
    * [Velero](#velero)
    * [Multiple Availability Zone](#multiple-availability-zone)
    * [Storage Types](#storage-types)
      * [Dynamic Persistent Volume Provisioning](#dynamic-persistent-volume-provisioning)
      * [Predefined Persistent Volumes](#predefined-persistent-volumes)
  * [Kubernetes](#kubernetes)
    * [Kubernetes 1.25](#kubernetes-125)
      * [Migration to Kubernetes 1.25](#migration-to-kubernetes-125)
  * [OpenShift](#openshift)
  * [Google Cloud](#google-cloud)
  * [AWS](#aws)
* [Best Practices and Recommendations](#best-practices-and-recommendations)
  * [HWE](#hwe)
    * [Tiny](#tiny)
    * [Small](#small)
    * [Medium](#medium)
    * [Large](#large)
    * [Additional components](#additional-components)
* [Parameters](#parameters)
  * [Cloud Integration Parameters](#cloud-integration-parameters)
  * [Global](#global)
    * [TLS](#tls)
    * [ACLs](#acls)
    * [Federation](#federation)
    * [Disaster Recovery](#disaster-recovery)
  * [Servers](#servers)
  * [External Servers](#external-servers)
  * [Clients](#clients)
  * [DNS](#dns)
  * [UI](#ui)
  * [Sync Catalog](#sync-catalog)
  * [Connect Injector](#connect-injector)
  * [Mesh Gateway](#mesh-gateway)
  * [Pod Scheduler](#pod-scheduler)
  * [Monitoring](#monitoring)
  * [Backup Daemon](#backup-daemon)
  * [ACL Configurator](#acl-configurator)
  * [Deployment Status Provisioner](#deployment-status-provisioner)
  * [Update Resources Job](#update-resources-job)
  * [Integration Tests](#integration-tests)
    * [Tags description](#tags-description)
* [Installation](#installation)
  * [Before You Begin](#before-you-begin)
    * [Helm](#helm)
  * [On-Prem Examples](#on-prem-examples)
    * [HA Scheme](#ha-scheme)
    * [DR Scheme](#dr-scheme)
  * [Google Cloud Examples](#google-cloud-examples)
    * [HA Scheme](#ha-scheme-1)
    * [DR Scheme](#dr-scheme-1)
  * [AWS Examples](#aws-examples)
    * [HA Scheme](#ha-scheme-2)
    * [DR Scheme](#dr-scheme-2)
  * [Azure Examples](#azure-examples)
    * [HA Scheme](#ha-scheme-3)
    * [DR Scheme](#dr-scheme-3)
* [Upgrade](#upgrade)
  * [Common](#common-1)
  * [Rolling Upgrade](#rolling-upgrade)
  * [CRD Upgrade](#crd-upgrade)
    * [Automatic CRD Upgrade](#automatic-crd-upgrade)
  * [Migration From DVM to Helm](#migration-from-dvm-to-helm)
  * [Rollback](#rollback)
* [Additional Features](#additional-features)
  * [Multiple Availability Zone Deployment](#multiple-availability-zone-deployment)
    * [Affinity](#affinity)
      * [Replicas Fewer Than Availability Zones](#replicas-fewer-than-availability-zones)
      * [Replicas More Than Availability Zones](#replicas-more-than-availability-zones)
  * [Consul Authentication Method](#consul-authentication-method)
  * [Multiple Datacenters](#multiple-datacenters)
    * [Federate Multiple Datacenters Via Mesh Gateways](#federate-multiple-datacenters-via-mesh-gateways)
    * [Federate Multiple Datacenters Using WAN Gossip](#federate-multiple-datacenters-using-wan-gossip)
      * [Create Multi-DC Configuration Manually](#create-multi-dc-configuration-manually)
<!-- TOC -->

# Prerequisites

## Common

Before you start the installation and configuration of a Consul cluster, ensure the following requirements are met:

* Kubernetes 1.21+ or OpenShift 4.10+
* `kubectl` 1.21+ or `oc` 4.10+ CLI
* Helm 3.0+
* All required CRDs are installed

Note the following terms:

* `DEPLOY_W_HELM` means installation is performed with `helm install/upgrade` commands, not `helm template + kubectl apply`.

### Custom Resource Definitions

The following Custom Resource Definitions should be installed to the cloud before the installation of Consul:

* `ConsulACL` - When you deploy with restricted rights or the CRDs' creation is disabled by the Deployer job.
  For more information, see [Automatic CRD Upgrade](#automatic-crd-upgrade).
* `GrafanaDashboard`, `PrometheusRule`, and `ServiceMonitor` - They should be installed when you deploy Consul monitoring with
  `monitoring.enabled=true` and `monitoring.monitoringType=prometheus`.
  You need to install the Monitoring Operator service before the Consul installation.
* `SiteManager` - It is installed when you deploy Consul with Disaster Recovery support (`global.disasterRecovery.mode`).
  You have to install the SiteManager service before the Consul installation.

**Important**: To create CRDs, you must have cloud rights for `CustomResourceDefinitions`.
If the deployment user does not have the necessary rights, you need to perform the steps described in
the [Deployment Permissions](#deployment-permissions) section before the installation.

### Deployment Permissions

To avoid using `cluster-wide` rights during the deployment, the following conditions are required:

* The cloud administrator creates the namespace/project in advance.
* The following grants should be provided for the `Role` of deployment user:

    ```yaml
    rules:
      - apiGroups:
          - qubership.org
        resources:
          - "*"
        verbs:
          - create
          - get
          - list
          - patch
          - update
          - watch
          - delete
      - apiGroups:
          - ""
        resources:
          - pods
          - services
          - endpoints
          - persistentvolumeclaims
          - configmaps
          - secrets
          - pods/exec
          - pods/portforward
          - pods/attach
          - pods/binding
          - serviceaccounts
        verbs:
          - create
          - get
          - list
          - patch
          - update
          - watch
          - delete
      - apiGroups:
          - apps
        resources:
          - deployments
          - deployments/scale
          - deployments/status
        verbs:
          - create
          - get
          - list
          - patch
          - update
          - watch
          - delete
          - deletecollection
      - apiGroups:
          - batch
        resources:
          - jobs
          - jobs/status
        verbs:
          - create
          - get
          - list
          - patch
          - update
          - watch
          - delete
      - apiGroups:
          - ""
        resources:
          - events
        verbs:
          - create
      - apiGroups:
          - apps
        resources:
          - statefulsets
          - statefulsets/scale
          - statefulsets/status
        verbs:
          - create
          - delete
          - get
          - list
          - patch
          - update
      - apiGroups:
          - networking.k8s.io
        resources:
          - ingresses
        verbs:
          - create
          - delete
          - get
          - list
          - patch
          - update
      - apiGroups:
          - rbac.authorization.k8s.io
        resources:
          - roles
          - rolebindings
        verbs:
          - create
          - delete
          - get
          - list
          - patch
          - update
      - apiGroups:
          - integreatly.org
        resources:
          - grafanadashboards
        verbs:
          - create
          - delete
          - get
          - list
          - patch
          - update
      - apiGroups:
          - monitoring.coreos.com
        resources:
          - servicemonitors
          - prometheusrules
        verbs:
          - create
          - delete
          - get
          - list
          - patch
          - update
      - apiGroups:
          - apps
        resources:
          - daemonsets
          - daemonsets/status
          - daemonsets/scale
        verbs:
          - create
          - get
          - list
          - patch
          - update
          - watch
          - delete
          - deletecollection
      - apiGroups:
          - policy
        resources:
          - poddisruptionbudgets
        verbs:
          - create
          - get
          - patch
    ```

* Pod security policies are created before installation if `enablePodSecurityPolicies` parameter value is set to `true`.
  For more information, refer to [Pod Security Policies](/docs/public/restricted-rights.md#pod-security-policies)
  or [Automatic YAML Building](/docs/public/restricted-rights.md#automatic-yaml-building).
* Cluster roles, cluster role bindings and security context constrains are created before installation.
  For more information, refer to [Cluster Entities](/docs/public/restricted-rights.md#cluster-entities)
  or [Automatic YAML Building](/docs/public/restricted-rights.md#automatic-yaml-building).

### Velero

* It is required to have the Consul backup daemon installed to properly restore Consul from a Velero backup.

### Multiple Availability Zone

If Kubernetes cluster has several availability zones, it is more reliable to start Consul server pods in different availability zones.
For more information, refer to [Multiple Availability Zone Deployment](#multiple-availability-zone-deployment).

### Storage Types

The following are a few approaches of storage management used in the Consul Service solution deployment:

* Dynamic Persistent Volume Provisioning
* Predefined Persistent Volumes

#### Dynamic Persistent Volume Provisioning

Consul Helm installation supports specifying storage class for server Persistent Volume Claims.

If you are setting up the persistent volumes' resource in Kubernetes, you need to map the Consul server to the volume
using the `server.storageClass` parameter.

#### Predefined Persistent Volumes

If you have prepared Persistent Volumes without storage class and dynamic provisioning,
you can specify Persistent Volumes names using the `server.persistentVolumes` parameter.

For example:

```yaml
persistentVolumes:
  - pv-default-consul-service-server-1
  - pv-default-consul-service-server-2
  - pv-default-consul-service-server-3
```

Persistent Volumes should be created on corresponding Kubernetes nodes and should be in the `Available` state.

Set appropriate UID and GID on hostPath directories and rule for SELinux:

```sh
chown -R 100:1000 /mnt/data/<pv-name>
```

You also need to specify node names via `server.nodes` parameter in the same order in which the Persistent Volumes are specified
so that Consul pods are assigned to these nodes.

According to the specified parameters, the `Pod Scheduler` distributes pods to the necessary Kubernetes nodes.
For more information, refer to [Pod Scheduler](#pod-scheduler) section.

## Kubernetes

* It is required to upgrade the component before upgrading Kubernetes.
* Follow the information in tags regarding Kubernetes certified versions.

### Kubernetes 1.25

Kubernetes 1.25+ does not contain Pod Security Policies in its API. It is replaced with Pod Security Standards.
In most cases it is enough to disable PSP in deployment parameters (`global.enablePodSecurityPolicies: false`)
to allow installation to 1.25 version, but Consul Client (`client.enabled: true`) requires `hostPort` access which is not covered by
OOB Pod Admission Control and `baseline` [Pod Security Standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/).
To be able to deploy Consul cluster with enabled `clients` you need to provide `privileged` policy to Consul namespace as prerequisite
step.

It can be performed with the following command:

```bash
kubectl label --overwrite ns "$CONSUL_NAMESPACE" pod-security.kubernetes.io/enforce=privileged
```

This command can be executed automatically with property `ENABLE_PRIVILEGED_PSS: true` in deployment parameters.
It requires the following cluster rights for deployment user:

```yaml
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["patch"]
    resourceNames:
    - $CONSUL_NAMESPACE
```

#### Migration to Kubernetes 1.25

When you have Consul with enabled `clients` and `global.enablePodSecurityPolicies: true` installed to Kubernetes 1.23+ version with
enabled PSP admission control you need to prepare Consul service **before** Kubernetes 1.25 upgrade with the following steps:

1. Upgrade Consul to `0.2.0+` version with enabled `privileged` PSS for namespace.
   Refer to [Deployment to Kubernetes 1.25](#kubernetes-125) guide.
2. Enable PSS for Kubernetes `rbac.admission: pss`.
   [KubeMarine:RBAC Admission](https://github.com/Netcracker/KubeMarine/blob/main/documentation/Installation.md#configuring-default-profiles)
  .
3. Upgrade Consul with disabled PSP `global.enablePodSecurityPolicies: false`.
4. Upgrade Kubernetes to 1.25.

## OpenShift

* It is required to upgrade the component before upgrading OpenShift. Follow the information in tags regarding OpenShift certified
  versions.
* `global.openshift.enabled` parameter should be set to `true`.
* The following annotations should be specified for the project:

  ```sh
  oc annotate --overwrite ns ${OS_PROJECT} openshift.io/sa.scc.supplemental-groups="1000/1000"
  oc annotate --overwrite ns ${OS_PROJECT} openshift.io/sa.scc.uid-range="100/1000"
  ```

## Google Cloud

The `Google Cloud Storage` bucket is created if a backup is necessary.

## AWS

The `AWS S3` bucket is created if a backup is necessary.

# Best Practices and Recommendations

## HWE

The provided values do not guarantee that these values are correct for all cases. It is a general recommendation
. Resources should be calculated and estimated for each project case with test load on the SVT stand, especially the HDD size.

The Hashicorp recommends starting resources configuration from 
[System Requirements](https://developer.hashicorp.com/consul/tutorials/production-deploy/reference-architecture#system-requirements) guide.

### Tiny

It is recommended for single environment development purposes, PoC and demos. Disk throughput is about 10 MB/s.

| Module                  | CPU   | RAM, Gi | Storage, Gb |
|-------------------------|-------|---------|-------------|
| Consul Server (x3)      | 0.5   | 2       | 50          |
| Consul Client (xN)      | 0.2   | 0.3     | 0           |
| Consul Backup Daemon    | 0.1   | 0.2     | 50          |
| Consul ACL Configurator | 0.1   | 0.2     | 0           |
| Disaster Recovery       | 0.1   | 0.1     | 0           |
| Pod Scheduler           | 0.1   | 0.1     | 0           |
| Status Provisioner      | 0.1   | 0.2     | 0           |
| ACL init job            | 0.1   | 0.1     | 0           |
| ACL init cleanup job    | 0.1   | 0.1     | 0           |
| TLS init job            | 0.1   | 0.1     | 0           |
| TLS init cleanup job    | 0.1   | 0.1     | 0           |
| **Total (Rounded)**     | **2** | **8**   | **200**     |

<details>
<summary>Click to expand YAML</summary>

```yaml
global:
  disasterRecovery:
    resources:
      requests:
        cpu: 25m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 128Mi
  tls:
    init:
      resources:
        requests:
          memory: 50Mi
          cpu: 50m
        limits:
          memory: 50Mi
          cpu: 50m
server:
  resources:
    requests:
      cpu: 0.1
      memory: 2Gi
    limits:
      cpu: 0.5
      memory: 2Gi
  aclInit:
    resources:
      requests:
        memory: 100Mi
        cpu: 50m
      limits:
        memory: 100Mi
        cpu: 50m
client:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
backupDaemon:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
consulAclConfigurator:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
podScheduler:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
statusProvisioner:
  resources:
    requests:
      memory: 50Mi
      cpu: 50m
    limits:
      memory: 100Mi
      cpu: 100m
```

</details>

### Small

It is recommended for development purposes, PoC, demos and not heavy loaded productions. Disk throughput is about 30 MB/s.

| Module                  | CPU   | RAM, Gi | Storage, Gb |
|-------------------------|-------|---------|-------------|
| Consul Server (x3)      | 1     | 4       | 50          |
| Consul Client (xN)      | 0.3   | 0.3     | 0           |
| Consul Backup Daemon    | 0.2   | 0.2     | 50          |
| Consul ACL Configurator | 0.2   | 0.2     | 0           |
| Disaster Recovery       | 0.1   | 0.1     | 0           |
| Pod Scheduler           | 0.1   | 0.1     | 0           |
| Status Provisioner      | 0.2   | 0.2     | 0           |
| ACL init job            | 0.1   | 0.1     | 0           |
| ACL init cleanup job    | 0.1   | 0.1     | 0           |
| TLS init job            | 0.1   | 0.1     | 0           |
| TLS init cleanup job    | 0.1   | 0.1     | 0           |
| **Total (Rounded)**     | **6** | **15**  | **200**     |

<details>
<summary>Click to expand YAML</summary>

```yaml
global:
  disasterRecovery:
    resources:
      requests:
        cpu: 25m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 128Mi
  tls:
    init:
      resources:
        requests:
          memory: 50Mi
          cpu: 50m
        limits:
          memory: 50Mi
          cpu: 50m
server:
  resources:
    requests:
      cpu: 0.5
      memory: 4Gi
    limits:
      cpu: 1
      memory: 4Gi
  aclInit:
    resources:
      requests:
        memory: 100Mi
        cpu: 50m
      limits:
        memory: 100Mi
        cpu: 50m
client:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
backupDaemon:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
consulAclConfigurator:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
podScheduler:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
statusProvisioner:
  resources:
    requests:
      memory: 50Mi
      cpu: 50m
    limits:
      memory: 100Mi
      cpu: 100m
```

</details>

### Medium

It is recommended for deployments with average load. Disk throughput is about 75 MB/s.

| Module                  | CPU   | RAM, Gi | Storage, Gb |
|-------------------------|-------|---------|-------------|
| Consul Server (x3)      | 2     | 8       | 100         |
| Consul Client (xN)      | 0.3   | 0.3     | 0           |
| Consul Backup Daemon    | 0.2   | 0.2     | 100         |
| Consul ACL Configurator | 0.2   | 0.2     | 0           |
| Disaster Recovery       | 0.1   | 0.1     | 0           |
| Pod Scheduler           | 0.1   | 0.1     | 0           |
| Status Provisioner      | 0.2   | 0.2     | 0           |
| ACL init job            | 0.1   | 0.1     | 0           |
| ACL init cleanup job    | 0.1   | 0.1     | 0           |
| TLS init job            | 0.1   | 0.1     | 0           |
| TLS init cleanup job    | 0.1   | 0.1     | 0           |
| **Total (Rounded)**     | **9** | **27**  | **400**     |

<details>
<summary>Click to expand YAML</summary>

```yaml
global:
  disasterRecovery:
    resources:
      requests:
        cpu: 25m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 128Mi
  tls:
    init:
      resources:
        requests:
          memory: 50Mi
          cpu: 50m
        limits:
          memory: 50Mi
          cpu: 50m
server:
  resources:
    requests:
      cpu: 2
      memory: 8Gi
    limits:
      cpu: 2
      memory: 8Gi
  aclInit:
    resources:
      requests:
        memory: 100Mi
        cpu: 50m
      limits:
        memory: 100Mi
        cpu: 50m
client:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
backupDaemon:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
consulAclConfigurator:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
podScheduler:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
statusProvisioner:
  resources:
    requests:
      memory: 50Mi
      cpu: 50m
    limits:
      memory: 100Mi
      cpu: 100m
```

</details>

### Large

It is recommended for deployments with high workload and large amount of data. Disk throughput is about 250 MB/s.

| Module                  | CPU    | RAM, Gi | Storage, Gb |
|-------------------------|--------|---------|-------------|
| Consul Server (x3)      | 8      | 32      | 200         |
| Consul Client (xN)      | 0.3    | 0.3     | 0           |
| Consul Backup Daemon    | 0.2    | 0.2     | 200         |
| Consul ACL Configurator | 0.2    | 0.2     | 0           |
| Disaster Recovery       | 0.1    | 0.1     | 0           |
| Pod Scheduler           | 0.1    | 0.1     | 0           |
| Status Provisioner      | 0.2    | 0.2     | 0           |
| ACL init job            | 0.1    | 0.1     | 0           |
| ACL init cleanup job    | 0.1    | 0.1     | 0           |
| TLS init job            | 0.1    | 0.1     | 0           |
| TLS init cleanup job    | 0.1    | 0.1     | 0           |
| **Total (Rounded)**     | **27** | **99**  | **800**     |

<details>
<summary>Click to expand YAML</summary>

```yaml
global:
  disasterRecovery:
    resources:
      requests:
        cpu: 25m
        memory: 32Mi
      limits:
        cpu: 100m
        memory: 128Mi
  tls:
    init:
      resources:
        requests:
          memory: 50Mi
          cpu: 50m
        limits:
          memory: 50Mi
          cpu: 50m
server:
  resources:
    requests:
      cpu: 8
      memory: 32Gi
    limits:
      cpu: 8
      memory: 32Gi
  aclInit:
    resources:
      requests:
        memory: 100Mi
        cpu: 50m
      limits:
        memory: 100Mi
        cpu: 50m
client:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
backupDaemon:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
consulAclConfigurator:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
podScheduler:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 100m
      memory: 128Mi
statusProvisioner:
  resources:
    requests:
      memory: 50Mi
      cpu: 50m
    limits:
      memory: 100Mi
      cpu: 100m
```

</details>

### Additional components

| Module                  | CPU   | RAM, Gi | Storage, Gb |
|-------------------------|-------|---------|-------------|
| Federation Secret job   | 0.1   | 0.1     | 0           |
| Consul Sync Catalog     | 0.1   | 0.1     | 0           |
| Consul Connect Injector | 0.1   | 0.1     | 0           |
| Consul Webhook Manager  | 0.1   | 0.1     | 0           |
| Consul Mesh Gateway     | 0.5   | 0.4     | 0           |
| **Total (Rounded)**     | **1** | **1**   | **0**       |

<details>
<summary>Click to expand YAML</summary>

```yaml
global:
  acls:
    init:
      resources:
        requests:
          cpu: 50m
          memory: 50Mi
        limits:
          cpu: 50m
          memory: 50Mi
syncCatalog:
  resources:
    requests:
      cpu: 50m
      memory: 50Mi
    limits:
      cpu: 50m
      memory: 50Mi
connectInject:
  resources:
    requests:
      cpu: 50m
      memory: 50Mi
    limits:
      cpu: 50m
      memory: 50Mi
meshGateway:
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 400m
      memory: 256Mi
  initServiceInitContainer:
    resources:
      requests:
        cpu: 50m
        memory: 50Mi
      limits:
        cpu: 50m
        memory: 150Mi
```

</details>

# Parameters

The section lists the configurable parameters of the Consul chart and their default values.

## Cloud Integration Parameters

| Parameter                           | Type    | Mandatory | Default value | Description                                                                       |
|-------------------------------------|---------|-----------|---------------|-----------------------------------------------------------------------------------|
| MONITORING_ENABLED                  | boolean | no        | `false`       | Specifies whether Consul Monitoring component is to be deployed or not.           |
| STORAGE_RWO_CLASS                   | string  | yes       | `""`          | Storage class name used to dynamically provide volumes.                           |
| INFRA_CONSUL_FS_GROUP               | integer | no        |               | Specifies group ID used inside Consul pods.                                       |
| INFRA_CONSUL_REPLICAS               | integer | no        |               | Specifies Consul replicas count.                                                  |
| INFRA_CONSUL_RESTRICTED_ENVIRONMENT | boolean | no        | `false`       | Specifies whether the Consul service is to be deployed in restricted environment. |

## Global

The global values affect all the other parameters in the chart. To enable all the Consul components in the Helm chart, set
`global.enabled` to `true`. This installs the servers, clients, Consul DNS, and the Consul UI with their defaults.

You should also set the global parameters based on your specific environment requirements.

| Parameter                                  | Type    | Mandatory | Default value            | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
|--------------------------------------------|---------|-----------|--------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `global.enabled`                           | boolean | no        | true                     | Whether all the components within this chart are to be enabled by default. Each component can be overridden using the component-specific `enabled` value.                                                                                                                                                                                                                                                                                                                          |
| `global.extraLabels`                       | object  | no        | {}                       | The custom labels for all pods which are related to the Consul service. These labels can be overridden by local custom labels.                                                                                                                                                                                                                                                                                                                                                     |
| `global.name`                              | string  | no        | consul                   | The prefix used for all resources in the Helm chart. If it is not set, the prefix is "{{ .Release.Name }}-consul". This value must be unique for all Consul clusters installed to one Kubernetes environment. It is not recommended to place multiple Consul clusters in the same Kubernetes environment but if you want to deploy Consul to Kubernetes where another Consul is already installed (e.g., for `development` purposes), you need to change this value.               |
| `global.domain`                            | string  | no        | consul                   | The domain to register the Consul DNS server to listen for.                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `global.ipv6`                              | boolean | no        | false                    | Whether the Consul services REST API is to be started on `IPv6` interface. Set the parameter to `true` if you deploy Consul in Kubernetes environment with IPv6 network interfaces.                                                                                                                                                                                                                                                                                                |
| `global.restrictedEnvironment`             | boolean | no        | false                    | Whether the Consul service is to be deployed in restricted environment. If it is set to `true`, necessary cluster entities (`Cluster Role`, `Cluster Role Binding`, `Pod Security Policy`) are not created automatically.                                                                                                                                                                                                                                                          |
| `global.image`                             | string  | no        | Calculates automatically | The image of the Consul docker image for clients and servers.                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `global.imagePullSecrets`                  | list    | no        | []                       | The list of references to secrets in the same namespace to use for pulling any of the images used by any container.                                                                                                                                                                                                                                                                                                                                                                |
| `global.imageK8S`                          | string  | no        | Calculates automatically | The image of the `consul-k8s` docker image that is used for functionality such as the catalog sync.                                                                                                                                                                                                                                                                                                                                                                                |
| `global.imageConsulDataplane`              | string  | no        | Calculates automatically | The image of the Consul DataPlane docker image that is used for ingress and terminating gateways.                                                                                                                                                                                                                                                                                                                                                                                  |
| `global.datacenter`                        | string  | no        | dc1                      | The name of the datacenter that the agents should register. It should not be changed after the Consul cluster is up and running since Consul does not support an automatic way to change this value. For more information, refer to [https://github.com/hashicorp/consul/issues/1858](https://github.com/hashicorp/consul/issues/1858).                                                                                                                                            |
| `global.enablePodSecurityPolicies`         | boolean | no        | false                    | Whether pod security policies are to be created for the Consul components created by the chart. For more information, refer to [https://kubernetes.io/docs/concepts/policy/pod-security-policy/](https://kubernetes.io/docs/concepts/policy/pod-security-policy/). **Note:** Pod security policies are not available starting from Kubernetes `1.25`. For more information, refer to [Kubernetes 1.25](#kubernetes-125).                                                           |
| `global.gossipEncryption.secretName`       | string  | no        | ""                       | The name of the Kubernetes secret that holds the gossip encryption key. The secret must be in the same namespace that Consul is installed into.                                                                                                                                                                                                                                                                                                                                    |
| `global.gossipEncryption.secretKey`        | string  | no        | ""                       | The key within the Kubernetes secret that holds the gossip encryption key.                                                                                                                                                                                                                                                                                                                                                                                                         |
| `global.recursors`                         | list    | no        | []                       | The list of addresses of upstream DNS servers that are used to recursively resolve DNS queries. If this is an empty array (the default), then Consul DNS only resolves queries for the Consul top level domain (by default `.consul`).                                                                                                                                                                                                                                             |
| `global.metrics.enabled`                   | boolean | no        | true                     | Whether the components to expose Prometheus metrics for the Consul service mesh are to be configured. By default, it includes gateway metrics and sidecar metrics.                                                                                                                                                                                                                                                                                                                 |
| `global.metrics.enableAgentMetrics`        | boolean | no        | true                     | Whether the metrics for Consul agent are to be configured. It is applicable only if `global.metrics.enabled` is set to `true`.                                                                                                                                                                                                                                                                                                                                                     |
| `global.metrics.agentMetricsRetentionTime` | string  | no        | 24h                      | The retention time for metrics in Consul clients and servers. It must be greater than 0 for Consul clients and servers to expose any metrics at all. It is applicable only if `global.metrics.enabled` is set to `true`.                                                                                                                                                                                                                                                           |
| `global.metrics.enableGatewayMetrics`      | boolean | no        | true                     | Whether the `mesh`, `terminating`, and `ingress` gateways are to expose their Consul DataPlane metrics on port `20200` at the `/metrics` path and all gateway pods are to have Prometheus scrape annotations. It is applicable only if `global.metrics.enabled` is set to `true`.                                                                                                                                                                                                  |
| `global.metrics.disableHostname`           | boolean | no        | true                     | Whether the runtime telemetry with the machine hostname is to be prepended.                                                                                                                                                                                                                                                                                                                                                                                                        |
| `global.logLevel`                          | string  | no        | info                     | The default log level to apply to all components which do not override this setting. It is recommended to generally not set this below "info" unless actively debugging due to logging verbosity. The possible values are `debug`, `info`, `warn`, `error`.                                                                                                                                                                                                                        |
| `global.logJSON`                           | boolean | no        | false                    | Whether the output in JSON format for all component logs is to be enabled.                                                                                                                                                                                                                                                                                                                                                                                                         |
| `global.openshift.enabled`                 | boolean | no        | false                    | Whether the necessary configuration for running Consul components on OpenShift is to be created.                                                                                                                                                                                                                                                                                                                                                                                   |
| `global.consulAPITimeout`                  | string  | no        | 5s                       | The time in seconds that the Consul API client waits for a response from the API before cancelling the request.                                                                                                                                                                                                                                                                                                                                                                    |
| `global.securityContext`                   | object  | no        | {}                       | The pod-level security attributes and common container settings for all pods that are related to the Consul service. This security context can be overridden by component `securityContext` parameter.                                                                                                                                                                                                                                                                             |
| `global.velero.postHookRestoreEnabled`     | boolean | no        | true                     | Whether Velero restore post-hook with Auth Method restore command is to be enabled. If parameter is set to `true`, Consul Backup Daemon initiates update of Consul Auth Methods with actual tokens after Velero restore procedure. For more information about Velero restore hooks, see [Restore Hooks](https://velero.io/docs/v1.9/restore-hooks/). **NOTE**: Backup Daemon should be enabled for Velero integration (`backupDaemon.enabled: true`).                              |
| `global.cloudIntegrationEnabled`           | boolean | no        | true                     | This parameter specifies whether to apply [Cloud Integration Parameters](#cloud-integration-parameters) instead of parameters described in Consul. If it is set to false or global parameter is absent, corresponding parameter from Consul is applied.                                                                                                                                                                                                                            |
| `global.ports.https`                       | string  | no        | ""                       | This parameter specifies custom global https port providing the ability to deploy two Consul instances with enabled Clients to one cluster. This parameter affects both servers and clients unless the corresponding values are overridden in the server or client configuration sections.                                                                                                                                                                                         |
| `global.ports.http`                        | string  | no        | ""                       | This parameter specifies custom global http port providing the ability to deploy two Consul instances with enabled Clients to one cluster. This parameter affects both servers and clients unless the corresponding values are overridden in the server or client configuration sections.                                                                                                                                                                                          |
| `global.ports.grpc`                        | string  | no        | ""                       | This parameter specifies custom global grpc port providing the ability to deploy two Consul instances with enabled Clients to one cluster. This parameter affects both servers and clients unless the corresponding values are overridden in the server or client configuration sections.                                                                                                                                                                                          |

### TLS

Consul uses Transport Layer Security (TLS) encryption across the cluster to verify authenticity of the servers and clients that connect.
HTTPS (TLS) port is `8501`, while HTTP port is `8500`.
You can find additional information regarding TLS certificates and examples of deployment in [Encrypted Access](/docs/public/tls.md) guide.

| Parameter                                   | Type    | Mandatory | Default value | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|---------------------------------------------|---------|-----------|---------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `global.tls.enabled`                        | boolean | no        | false         | Whether TLS is to be enabled for all Consul components. For more information about TLS, see [Encrypted Access](/docs/public/tls.md).                                                                                                                                                                                                                                                                                                                                                                                                                |
| `global.tls.enableAutoEncrypt`              | boolean | no        | false         | Whether the `auto-encrypt` feature is to enabled on clients and servers. It also switches `consul-k8s` components to retrieve the Certificate Authority (CA) from the servers via the API.                                                                                                                                                                                                                                                                                                                                                          |
| `global.tls.cipherSuites`                   | list    | no        | []            | The list of cipher suites that are used to negotiate the security settings for a network connection using TLS or SSL network protocol. By default, all the available cipher suites are supported.                                                                                                                                                                                                                                                                                                                                                   |
| `global.tls.certManager.enabled`            | boolean | no        | false         | Whether TLS certificates are to be generated and managed by Cert Manager. This parameter is taken into account only if `global.tls.enabled` parameter is set to `true`. When it is set to `false` the Consul deployment procedure generates self-signed certificates during installation.                                                                                                                                                                                                                                                           |
| `global.tls.certManager.clusterIssuerName`  | string  | no        | ""            | The name of the `ClusterIssuer` resource. If the parameter is not set or empty, the `Issuer` resource is created in the current Kubernetes namespace.                                                                                                                                                                                                                                                                                                                                                                                               |
| `global.tls.certManager.durationDays`       | integer | no        | 730           | The TLS certificates validity period in days.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `global.tls.serverAdditionalDNSSANs`        | list    | no        | []            | The list of additional DNS names to set as Subject Alternative Names (SANs) in the server certificate. This is useful when you need to access the Consul server(s) externally, for example, if you're using the UI.                                                                                                                                                                                                                                                                                                                                 |
| `global.tls.serverAdditionalIPSANs`         | list    | no        | []            | The list of additional IP addresses names to set as Subject Alternative Names (SANs) in the server certificate. This is useful when you need to access the Consul server(s) externally, for example, if you're using the UI.                                                                                                                                                                                                                                                                                                                        |
| `global.tls.verify`                         | boolean | no        | true          | Whether servers and client configuration is to be verified. If the value is `true`, `verify_outgoing`, `verify_server_hostname`, and `verify_incoming_rpc` are set to `true` for Consul servers and clients. Set this to `false` to incrementally roll out TLS on an existing Consul cluster. For more information, refer to [https://learn.hashicorp.com/tutorials/consul/tls-encryption-secure](https://learn.hashicorp.com/tutorials/consul/tls-encryption-secure). **Note**: remember to switch it back to `true` once the rollout is complete. |
| `global.tls.httpsOnly`                      | boolean | no        | false         | Whether the HTTP port on both clients and servers is to be disabled and only HTTPS connections are to be accepted. **Note**: The most part of services which work with Consul do not support `https` (TLS) mode to connect with Consul. Remember that when you disable `http` access.                                                                                                                                                                                                                                                               |
| `global.tls.caCert.secretName`              | string  | no        | null          | The name of the Kubernetes secret containing the certificate of the CA to use for TLS communication within the Consul cluster. If it is not specified, Consul generates the CA automatically.                                                                                                                                                                                                                                                                                                                                                       |
| `global.tls.caCert.secretKey`               | string  | no        | tls.crt       | The key of the Kubernetes secret containing the certificate of the CA to use for TLS communication within the Consul cluster.                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `global.tls.caKey.secretName`               | string  | no        | null          | The name of the Kubernetes secret containing the private key of the CA to use for TLS communication within the Consul cluster. If it is not specified, Consul generates the CA automatically.                                                                                                                                                                                                                                                                                                                                                       |
| `global.tls.caKey.secretKey`                | string  | no        | tls.key       | The key of the Kubernetes secret containing the private key of the CA to use for TLS communication within the Consul cluster.                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `global.tls.init.resources.requests.cpu`    | string  | no        | 50m           | The minimum number of CPUs the TLS init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `global.tls.init.resources.requests.memory` | string  | no        | 50Mi          | The minimum amount of memory the TLS init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `global.tls.init.resources.limits.cpu`      | string  | no        | 50m           | The maximum number of CPUs the TLS init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `global.tls.init.resources.limits.memory`   | string  | no        | 50Mi          | The maximum amount of memory the TLS init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |

### ACLs

Consul uses Access Control Lists (ACLs) to secure the UI, API, CLI, service communications, and agent communications.
When securing your cluster, you should configure the ACLs first.
ACLs operate by grouping rules into policies and then associating one or more policies with a token.

| Parameter                                    | Type    | Mandatory | Default value | Description                                                                                                                                                                                                                                                                                                                                                                  |
|----------------------------------------------|---------|-----------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `global.acls.manageSystemACLs`               | boolean | no        | true          | Whether ACL tokens and policies for all Consul and `consul-k8s` components are to be managed.                                                                                                                                                                                                                                                                                |
| `global.acls.createAuthMethod`               | boolean | no        | true          | Whether Consul authentication method is to be created. If ACL is not enabled, the authentication method is not created.                                                                                                                                                                                                                                                      |
| `global.acls.bootstrapToken.secretName`      | string  | no        | null          | The name of the Kubernetes secret containing the bootstrap token to use for creating policies and tokens for all Consul and `consul-k8s` components. If it is set, ACL bootstrapping of the servers is skipped and only ACLs for the Consul clients and `consul-k8s` system components are initialized.                                                                      |
| `global.acls.bootstrapToken.secretKey`       | string  | no        | null          | The key of the Kubernetes secret containing the bootstrap token to use for creating policies and tokens for all Consul and `consul-k8s` components. If it is set, ACL bootstrapping of the servers is skipped and only ACLs for the Consul clients and `consul-k8s` system components are initialized.                                                                       |
| `global.acls.createReplicationToken`         | boolean | no        | false         | Whether ACL token that can be used in secondary datacenters for replication is to be created. It should only be set to `true` in the primary datacenter since the replication token must be created from that datacenter. In secondary datacenters, the secret needs to be imported from the primary datacenter and referenced via `global.acls.replicationToken` parameter. |
| `global.acls.replicationToken.secretName`    | string  | no        | null          | The name of the Kubernetes secret containing the replication ACL token. This token is used by secondary datacenters to perform ACL replication and create ACL tokens and policies. This value is ignored if `global.acls.bootstrapToken` settings are also set.                                                                                                              |
| `global.acls.replicationToken.secretKey`     | string  | no        | null          | The key of the Kubernetes secret containing the replication ACL token. This token is used by secondary datacenters to perform ACL replication and create ACL tokens and policies. This value is ignored if `global.acls.bootstrapToken` settings are also set.                                                                                                               |
| `global.acls.init.resources.requests.cpu`    | string  | no        | 50m           | The minimum number of CPUs the ACLs init job container should use.                                                                                                                                                                                                                                                                                                           |
| `global.acls.init.resources.requests.memory` | string  | no        | 50Mi          | The minimum amount of memory the ACLs init job container should use.                                                                                                                                                                                                                                                                                                         |
| `global.acls.init.resources.limits.cpu`      | string  | no        | 50m           | The maximum number of CPUs the ACLs init job container should use.                                                                                                                                                                                                                                                                                                           |
| `global.acls.init.resources.limits.memory`   | string  | no        | 50Mi          | The maximum amount of memory the ACLs init job container should use.                                                                                                                                                                                                                                                                                                         |

### Federation

Consul provides ability to federate with another Consul datacenter.

**Important**: If during `upgrade` process you change parameters for [Mesh Gateway](#mesh-gateway), 
you need to wait until all services go to the `ready` status and run the `upgrade` again with the same parameters for 
the correct formation of the `federation` secret.

| Parameter                                  | Type    | Mandatory | Default value | Description                                                                                                                                                                                                                                                                                                                  |
|--------------------------------------------|---------|-----------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `global.federation.enabled`                | boolean | no        | false         | Whether the datacenter is to be federation-capable. Only federation via mesh gateways is supported. Mesh gateways and servers should be configured to allow federation. It requires `global.tls.enabled`, `meshGateway.enabled` and `connectInject.enabled` to be set to `true`.                                             |
| `global.federation.createFederationSecret` | boolean | no        | false         | Whether the Kubernetes secret that can be imported into secondary datacenters, so they can federate with this datacenter, is to be created. The secret contains all the information secondary datacenters need to contact and authenticate with this datacenter. This should only be set to true in your primary datacenter. |
| `global.federation.primaryDatacenter`      | string  | no        | null          | The name of the primary datacenter. It should be filled only on secondary datacenters.                                                                                                                                                                                                                                       |
| `global.federation.primaryGateways`        | list    | no        | []            | The list of addresses of the primary mesh gateways in the form `<ip>:<port>`. For example, `["1.1.1.1:443", "2.3.4.5:443"]`.                                                                                                                                                                                                 |
| `global.federation.k8sAuthMethodHost`      | string  | no        | null          | The address of the Kubernetes API server. It should be filled only on secondary datacenters if `global.federation.enabled` parameter is set to `true`. This address must be reachable from the Consul servers. For example, `https://k8s-2.openshift.sdntest.example.com:6443`.                                              |
| `global.federation.securityContext`        | object  | no        | {}            | The pod-level security attributes and common container settings for Federation job pod.                                                                                                                                                                                                                                      |

The federation secret is automatically generated if `createFederationSecret` parameter is set to `true`. 
It contains the following information:

* `Server certificate authority certificate` (`caCert`) is the certificate authority used to sign Consul server-to-server communication.
  This is required by `secondary` clusters because they must communicate with the Consul servers in the `primary` cluster.
* `Server certificate authority key` (`caKey`) is the signing key for the server certificate authority.
  This is required by `secondary` clusters because they need to create server certificates for each Consul server
  using the same certificate authority as the `primary`.
* `Consul server config` (`serverConfigJSON`) is a JSON snippet that must be used as part of the server config for `secondary` datacenters.
  It sets:
  * `primary_datacenter` to the name of the `primary` datacenter.
  * `primary_gateways` to an array of IPs or hostnames for the mesh gateways in the `primary` datacenter.
    These are the addresses that Consul servers in `secondary` clusters use to communicate with the primary datacenter.
    Even if there are multiple `secondary` datacenters, only the primary gateways need to be configured.
    Upon first connection with a `primary` datacenter, the addresses for other `secondary` datacenters are discovered.
* `ACL replication token` (`replicationToken`) is an ACL token in order to authenticate with the `primary` datacenter required for
  `secondary` datacenters if ACLs are enabled.
  This ACL token is also used to replicate ACLs from the `primary` datacenter so that components in each datacenter
  can authenticate with one another.
* `Gossip encryption key` (`gossipEncryptionKey`) is the gossip encryption key in order to be part of the gossip pool required for
  `secondary` datacenters if gossip encryption is enabled. Gossip is the method by which Consul discovers the addresses and
  health of other nodes.

### Disaster Recovery

The Disaster Recovery mode implies two Consul services, one of which is in an `active` state and the other is in a `standby` state.
They are installed on separate Kubernetes/OpenShift clusters.

| Parameter                                                                  | Type    | Mandatory | Default value            | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|----------------------------------------------------------------------------|---------|-----------|--------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `global.disasterRecovery.image`                                            | string  | no        | Calculates automatically | The image of Consul Disaster Recovery container.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `global.disasterRecovery.tls.enabled`                                      | boolean | no        | true                     | Whether TLS is to be enabled for Disaster Recovery Daemon. This parameter is taken into account only if `global.tls.enabled` parameter is set to `true`. For more information about TLS, see [Encrypted Access](/docs/public/tls.md).                                                                                                                                                                                                                                                                                                                               |
| `global.disasterRecovery.tls.certificates.crt`                             | string  | no        | ""                       | The certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                  |
| `global.disasterRecovery.tls.certificates.key`                             | string  | no        | ""                       | The private key in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                  |
| `global.disasterRecovery.tls.certificates.ca`                              | string  | no        | ""                       | The root CA certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                          |
| `global.disasterRecovery.tls.secretName`                                   | string  | no        | ""                       | The secret that contains TLS certificates. It is required if TLS for Disaster Recovery Daemon is enabled and certificates generation is disabled.                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `global.disasterRecovery.tls.cipherSuites`                                 | list    | no        | []                       | The list of cipher suites that are used to negotiate the security settings for a network connection using TLS or SSL network protocol. If this parameter is not specified, cipher suites are taken from `global.tls.cipherSuites` parameter.                                                                                                                                                                                                                                                                                                                        |
| `global.disasterRecovery.tls.subjectAlternativeName.additionalDnsNames`    | list    | no        | []                       | The list of additional DNS names to be added to the `Subject Alternative Name` field of TLS certificate.                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `global.disasterRecovery.tls.subjectAlternativeName.additionalIpAddresses` | list    | no        | []                       | The list of additional IP addresses to be added to the `Subject Alternative Name` field of TLS certificate.                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `global.disasterRecovery.httpAuth.enabled`                                 | boolean | no        | false                    | Whether site manager authentication is to be enabled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `global.disasterRecovery.httpAuth.smSecureAuth`                            | boolean | no        | false                    | Whether the `smSecureAuth` mode is enabled for Site Manager or not.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `global.disasterRecovery.httpAuth.smNamespace`                             | string  | no        | site-manager             | The name of Kubernetes namespace where site manager is located.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `global.disasterRecovery.httpAuth.smServiceAccountName`                    | string  | no        | ""                       | The name of Kubernetes service account that site manager is used.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `global.disasterRecovery.httpAuth.customAudience`                          | string  | no        | sm-services              | The name of custom audience for rest API token, that is used to connect with services. It is necessary if Site Manager installed with `smSecureAuth=true` and has applied custom audience (`sm-services` by default). It is considered if `global.disasterRecovery.httpAuth.smSecureAuth` parameter is set to `true`                                                                                                                                                                                                                                                |
| `global.disasterRecovery.mode`                                             | string  | no        | ""                       | The mode of Consul disaster recovery installation. If you do not specify this parameter, the service is deployed in a regular mode, not Disaster Recovery mode. The possible values are `active`, `standby`, `disable`. The Disaster Recovery mode requires `backupDaemon.enabled` parameter set to `true`. **Note**: You need to set this parameter during primary initialization via `clean install` or `reinstall`. Do not change it with `upgrade` process. To change the mode use the `SiteManager` functionality or Consul disaster recovery REST server API. |
| `global.disasterRecovery.region`                                           | string  | no        | ""                       | The region of cloud where current instance of Consul service is installed. For example, `us-central`. This parameter is mandatory if Consul is being deployed in Disaster Recovery mode.                                                                                                                                                                                                                                                                                                                                                                            |
| `global.disasterRecovery.siteManagerEnabled`                               | boolean | no        | true                     | Whether creation of a Kubernetes Custom Resource for `SiteManager` is to be enabled. This property is used for inner developers purposes.                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `global.disasterRecovery.timeout`                                          | integer | no        | 600                      | The timeout for switchover process.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `global.disasterRecovery.backupTimeout`                                    | string  | no        | "180s"                   | The timeout for backup procedure. The value is a sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".                                                                                                                                                                                                                                                                                                                                    |
| `global.disasterRecovery.restoreTimeout`                                   | string  | no        | "240s"                   | The timeout for restore procedure. The value is a sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms", "1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".                                                                                                                                                                                                                                                                                                                                   |
| `global.disasterRecovery.afterServices`                                    | list    | no        | []                       | The list of `SiteManager` names for services after which the Consul service switchover is to be run.                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `global.disasterRecovery.resources.requests.cpu`                           | string  | no        | 25m                      | The minimum number of CPUs the disaster recovery daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `global.disasterRecovery.resources.requests.memory`                        | string  | no        | 32Mi                     | The minimum amount of memory the disaster recovery daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `global.disasterRecovery.resources.limits.cpu`                             | string  | no        | 100m                     | The maximum number of CPUs the disaster recovery daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `global.disasterRecovery.resources.limits.memory`                          | string  | no        | 128Mi                    | The maximum amount of memory the disaster recovery daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `global.disasterRecovery.extraLabels`                                      | object  | no        | {}                       | The custom labels for Consul Disaster Recovery pod.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `global.disasterRecovery.securityContext`                                  | object  | no        | {}                       | The pod-level security attributes and common container settings for Disaster Recovery pod.                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

For more information, see [Consul Disaster Recovery](/docs/public/disaster-recovery.md) section in 
the _Cloud Platform Disaster Recovery Guide_.

## Servers

For production deployments, you need to deploy 3 or 5 Consul servers for quorum and failure tolerance. 
For most deployments 3 servers are adequate.

In the server section, set `replicas` to 3. This deploys three servers and can cause Consul to wait to perform leader election 
until all 3 are healthy. The resources depend on your environment.

| Parameter                                  | Type    | Mandatory | Default value                                                                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
|--------------------------------------------|---------|-----------|-------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `server.enabled`                           | boolean | no        | true                                                                          | Whether the servers are to be configured to run. You need to disable this parameter if you plan on connecting to a Consul cluster external to the Kubernetes cluster.                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `server.tls.certificates.crt`              | string  | no        | ""                                                                            | The certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                                         |
| `server.tls.certificates.key`              | string  | no        | ""                                                                            | The private key in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                                         |
| `server.tls.certificates.ca`               | string  | no        | ""                                                                            | The root CA certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                                 |
| `server.replicas`                          | integer | no        | 3                                                                             | The number of Consul server nodes. It determines the fault tolerance of the cluster. For more information, see [Deployment Table](https://consul.io/docs/internals/consensus#deployment-table).                                                                                                                                                                                                                                                                                                                                                                                                            |
| `server.bootstrapExpect`                   | integer | no        | null                                                                          | The number of servers that are expected to be running. The default value is equal to `server.replicas` parameter. In most cases the default value should be used, however if there are more servers in this datacenter than `server.replicas` it might make sense to override the default. This would be the case if two Kubernetes clusters were joined into the same datacenter and each cluster ran a certain number of servers.                                                                                                                                                                        |
| `server.exposeGossipAndRPCPorts`           | boolean | no        | false                                                                         | Whether the servers' gossip and RPC ports as hostPorts are to be exposed. To enable a client agent outside the Kubernetes cluster to join the datacenter, you would need to enable `server.exposeGossipAndRPCPorts`, `client.exposeGossipPorts`, and set `server.ports.serflan.port` to a port not being used on the host. Since `client.exposeGossipPorts` uses the hostPort 8301, `server.ports.serflan.port` must be set to something other than 8301.                                                                                                                                                  |
| `server.ports.serflan.port`                | string  | no        | 8301                                                                          | The LAN gossip port for the Consul servers. If you choose to enable `server.exposeGossipAndRPCPorts` and `client.exposeGossipPorts`, that configures the LAN gossip ports on the servers and clients to be `hostPorts`, so if you are running clients and servers on the same node the ports conflict if they are both `8301`. When you enable `server.exposeGossipAndRPCPorts` and `client.exposeGossipPorts`, you must change this from the default to an unused port on the host, e.g. `9301`. By default, the LAN gossip port is `8301` and configured as a `containerPort` on the Consul server pods. |
| `server.ports.https`                       | string  | no        | ""                                                                            | Customize the server's https consul port providing the ability to deploy two Consul instances with enabled Clients to one cluster. If specified, the server will use this port for HTTPS instead of the globally defined one.                                                                                                                                                                                                                                                                                                                                                                              |
| `server.ports.http`                        | string  | no        | ""                                                                            | Customize the server's http consul port providing the ability to deploy two Consul instances with enabled Clients to one cluster. If specified, the server will use this port for HTTP instead of the globally defined one.                                                                                                                                                                                                                                                                                                                                                                                |
| `server.ports.grpc`                        | string  | no        | ""                                                                            | Customize the server's grpc consul port providing the ability to deploy two Consul instances with enabled Clients to one cluster. If specified, the server will use this port for GRPC instead of the globally defined one.                                                                                                                                                                                                                                                                                                                                                                                |
| `server.storage`                           | string  | no        | 10Gi                                                                          | The disk size for configuring the servers' StatefulSet storage. For dynamically provisioned storage classes, this is the desired size. For manually defined persistent volumes, this should be set to the disk size of the attached volume.                                                                                                                                                                                                                                                                                                                                                                |
| `server.storageClass`                      | string  | yes       | null                                                                          | The class of storage to use for the servers' StatefulSet storage. It must be able to be dynamically provisioned if you want the storage to be automatically created. For example, to use localStorage classes, the PersistentVolumeClaims would need to be manually created.                                                                                                                                                                                                                                                                                                                               |
| `server.persistentVolumes`                 | list    | no        | []                                                                            | The list of predefined Persistent Volumes for the Consul servers. Consul nodes take name of these Persistent Volumes by order. If you deploy Consul server with [Predefined Persistent Volumes](#predefined-persistent-volumes), you need to install the Consul server and then use `upgrade` procedure to enable other components. **Note**: If `storageClass` and `persistentVolumes` are not specified, the Consul server is deployed with `emptyDir`.                                                                                                                                                  |
| `server.nodes`                             | list    | no        | []                                                                            | The list of Kubernetes node names to assign Consul server nodes. The number of nodes should be equal to `server.replicas` parameter. It should not be used with `storageClass` pod assignment.                                                                                                                                                                                                                                                                                                                                                                                                             |
| `server.connect`                           | boolean | no        | true                                                                          | Whether `connect` on all the servers is to be enabled, initializing a CA for Connect-related connections. You can do other customizations using the `extraConfig` setting.                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `server.serviceAccount.annotations`        | object  | no        | null                                                                          | The additional annotations for the server service account.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `server.resources.requests.cpu`            | string  | no        | 50m                                                                           | The minimum number of CPUs the Consul server container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `server.resources.requests.memory`         | string  | no        | 128Mi                                                                         | The minimum amount of memory the Consul server container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `server.resources.limits.cpu`              | string  | no        | 400m                                                                          | The maximum number of CPUs the Consul server container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `server.resources.limits.memory`           | string  | no        | 1024Mi                                                                        | The maximum amount of memory the Consul server container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `server.securityContext`                   | object  | no        | {"runAsNonRoot": true, "runAsGroup": 1000, "runAsUser": 100, "fsGroup": 1000} | The pod-level security attributes and common container settings for Consul server pod. **Note**: if it is running on OpenShift, this setting is ignored because the user and group are set automatically by the OpenShift platform.                                                                                                                                                                                                                                                                                                                                                                        |
| `server.updatePartition`                   | integer | no        | 0                                                                             | The partition to perform a rolling update. It is used to carefully control a rolling update of Consul server agents.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `server.disruptionBudget.enabled`          | boolean | no        | true                                                                          | Whether the creation of a `PodDisruptionBudget` to prevent voluntary degrading of the Consul server cluster is to be created.                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `server.disruptionBudget.maxUnavailable`   | integer | no        | (<server.replicas>/2)-1                                                       | The maximum number of unavailable pods.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `server.extraConfig`                       | object  | no        | {"disable_update_check": true}                                                | The extra configuration to set with the server. It should be JSON. More info about extra configs in [Configuration Files](https://www.consul.io/docs/agent/options.html).                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `server.extraVolumes`                      | list    | no        | []                                                                            | The list of extra volumes to mount. They are exposed to Consul in the path `/consul/userconfig/<name>/`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `server.affinity`                          | object  | no        | <anti_affinity_rule>                                                          | The affinity scheduling rules in JSON format. To allow deployment to single node services such as Minikube, you can comment out or set the affinity variable as empty.                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `server.tolerations`                       | object  | no        | {}                                                                            | The list of toleration policies for Consul servers in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `server.topologySpreadConstraints`         | string  | no        | ""                                                                            | The Pod topology spread constraints for server pods.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `server.nodeSelector`                      | object  | no        | {}                                                                            | The labels for server pod assignment, formatted as a JSON string. For more information, refer to [https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector).                                                                                                                                                                                                                                                                                                                                       |
| `server.priorityClassName`                 | string  | no        | ""                                                                            | The priority class to be used to assign priority to Consul server pods. Priority class should be created beforehand. For more information, refer to [https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                                                                                                                                                                                              |
| `server.extraLabels`                       | object  | no        | null                                                                          | The extra labels to attach to the server pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `server.annotations`                       | object  | no        | {}                                                                            | The list of extra annotations to set to the Consul stateful set.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `server.service.annotations`               | object  | no        | {}                                                                            | The list of extra annotations to set to the Consul server service.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `server.extraEnvironmentVars`              | object  | no        | {}                                                                            | The list of extra environment variables to set to the Consul stateful set. You can use these environment variables to include proxy settings required for cloud auto-join feature, in case Kubernetes cluster is behind egress HTTP proxies. Additionally, it could be used to configure custom Consul parameters.                                                                                                                                                                                                                                                                                         |
| `server.aclInit.resources.requests.cpu`    | string  | no        | 50m                                                                           | The minimum number of CPUs the ACL init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `server.aclInit.resources.requests.memory` | string  | no        | 100Mi                                                                         | The minimum amount of memory the ACL init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `server.aclInit.resources.limits.cpu`      | string  | no        | 50m                                                                           | The maximum number of CPUs the ACL init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `server.aclInit.resources.limits.memory`   | string  | no        | 100Mi                                                                         | The maximum amount of memory the ACL init job container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `server.serverLocalityEnabled`             | boolean | no        | false                                                                         | Enables Consul server to set locality From its topology region label.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `server.logLevel`                          | string  | no        | ""                                                                            | Enables The log verbosity level. It is recommended to generally not set this below "info" unless actively debugging due to logging verbosity. The possible values are `debug`, `info`, `warn`, `error`.                                                                                                                                                                                                                                                                                                                                                                                                    |
| `server.enableAgentDebug`                  | boolean | no        | false                                                                         | Enables Consul to report additional debugging information, including runtime profiling (pprof) data.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `server.limits.requestLimits.mode`         | string  | no        | "disabled"                                                                    | Enables or disables rate limiting.  If not disabled, it enforces the action that will occur when RequestLimitsReadRate or RequestLimitsWriteRate is exceeded.                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `server.limits.requestLimits.readRate`     | integer | no        | -1                                                                            | Sets how frequently RPC, gRPC, and HTTP queries are allowed to happen.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `server.limits.requestLimits.writeRate`    | integer | no        | -1                                                                            | Sets how frequently RPC, gRPC, and HTTP writes are allowed to happen.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `server.raftSnapshotThreshold`             | integer | no        | 500                                                                           | Sets the minimum number of raft commit entries between snapshots that are saved to disk.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

Where:

* <anti_affinity_rule> is as follows:

  ```yaml
    affinity: {
      "podAntiAffinity": {
        "requiredDuringSchedulingIgnoredDuringExecution": [
          {
            "labelSelector": {
              "matchLabels": {
                "app": "{{ template \"consul.name\" . }}",
                "release": "{{ .Release.Name }}",
                "component": "server"
              }
            },
            "topologyKey": "kubernetes.io/hostname"
          }
        ]
      }
    }
  ```

## External Servers

This section describes configuration for Consul servers when the servers are running outside of Kubernetes.

| Parameter                           | Type    | Mandatory | Default value | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|-------------------------------------|---------|-----------|---------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `externalServers.enabled`           | boolean | no        | false         | Whether the Helm chart is to be configured to talk to the external servers. If this parameter is set to `true`, you must also set `server.enabled` to `false`.                                                                                                                                                                                                                                                                                                                              |
| `externalServers.hosts`             | list    | no        | []            | The list of external Consul server hosts that are used to make HTTPS connections from the components in this Helm chart. Valid values include IPs, DNS names, or Cloud auto-join string. The port must be provided separately below. **Note**: `client.join` must also be set to the hosts that should be used to join the cluster. In most cases, the `client.join` values should be the same, however, they may be different if you wish to use separate hosts for the HTTPS connections. |
| `externalServers.httpsPort`         | string  | no        | 8501          | The HTTPS port of the Consul servers.                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `externalServers.tlsServerName`     | string  | no        | null          | The server name to use as the SNI host header when connecting with HTTPS.                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `externalServers.useSystemRoots`    | boolean | no        | false         | Whether `consul-k8s` components are to ignore the CA set in `global.tls.caCert` when making HTTPS calls to Consul servers and instead use the `consul-k8s` image's system CAs for TLS verification. **Note**: This does not affect Consul's internal RPC communication which always uses `global.tls.caCert`.                                                                                                                                                                               |
| `externalServers.k8sAuthMethodHost` | string  | no        | null          | The address of the Kubernetes API server. This parameter should be filled if `global.acls.manageSystemACLs` and `connectInject.enabled` are set to `true`. For more information, refer to [https://www.consul.io/docs/security/acl/auth-methods/kubernetes](https://www.consul.io/docs/security/acl/auth-methods/kubernetes) article.                                                                                                                                                       |
| `externalServers.skipServerWatch`   | boolean | no        | false         | Whether the `consul-dataplane` and `consul-k8s` components are to watch the Consul servers for changes. This is useful for situations where Consul servers are behind a load balancer.                                                                                                                                                                                                                                                                                                      |

## Clients

A Consul client is deployed on every Kubernetes node, so you do not need to specify the number of clients for your deployments. 
You need to specify resources and enable `gRPC`. 
For most production scenarios, the Consul clients are designed for horizontal scalability. 
Enabling `gRPC` enables the gRPC listener on port `8502` and exposes it to the host. 
It is required when you use Consul Connect. 
This port is opened on Kubernetes node, so you need to have corresponding RBAC and security policies.

If your security policy denies opening such ports, you need to set the `enablePodSecurityPolicies` parameter to `true` for creating 
necessary pod security policies. For Kubernetes 1.25+ you need to follow the [Kubernetes 1.25](#kubernetes-125) guide.

| Parameter                           | Type    | Mandatory | Default value                                                                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
|-------------------------------------|---------|-----------|-------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `client.enabled`                    | boolean | no        | false                                                                         | Whether Consul clients are to be run on every node within the Kubernetes cluster. The current deployment model follows a traditional DC where a single agent is deployed per node. If you deploy Consul server with [Predefined Persistent Volumes](#predefined-persistent-volumes), you need to install the Consul server and then use `upgrade` procedure to enable the client. **NOTE:** Consul Client module requires privileged access for Kubernetes to open `hostPorts` on nodes. It requires `global.enablePodSecurityPolicies: true` for Kubernetes prior 1.25 or `global.openshift.enabled: true` for OpenShift. For Kubernetes 1.25+ you need follow the [Kubernetes 1.25](#kubernetes-125) guide. |
| `client.tls.certificates.crt`       | string  | no        | ""                                                                            | The certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `client.tls.certificates.key`       | string  | no        | ""                                                                            | The private key in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `client.tls.certificates.ca`        | string  | no        | ""                                                                            | The root CA certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `client.join`                       | list    | no        | null                                                                          | The list of valid `-retry-join` values. If it is `null`, the clients attempt to automatically join the server cluster running within Kubernetes. This means that with `server.enabled` set to `true`, clients automatically join that cluster. If `server.enabled` is not `true`, a value must be specified, so the clients can join a valid cluster.                                                                                                                                                                                                                                                                                                                                                         |
| `client.podSecurityPolicy`          | string  | no        | null                                                                          | The predefined pod security policy for Consul clients. If this value is specified, Helm does not create pod security policy during installation. It is useful when the `enablePodSecurityPolicies` parameter is `true` and the user has no rights to create pod security policies.                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `client.securityContextConstraint`  | string  | no        | null                                                                          | The predefined security context constraint for Consul client. If this value is specified, Helm does not security context constraint during installation. It is useful when the `openshift.enabled` parameter is `true` and the user has no rights to create security context constraint by installation user. For example, `consul-client`.                                                                                                                                                                                                                                                                                                                                                                   |
| `client.dataDirectoryHostPath`      | string  | no        | null                                                                          | The absolute path to a directory on the host machine to use as the Consul client data directory. If set to the empty string or `null`, the Consul agent stores its data in the Pod's local filesystem which is lost if the Pod is deleted. **Security Warning**: If setting this, Pod Security Policies _must_ be enabled on your cluster and in this Helm chart (via the `global.enablePodSecurityPolicies` setting) to prevent other Pods from mounting the same host path and gaining access to all of Consul's data. Consul's data is not encrypted at rest.                                                                                                                                              |
| `client.grpc`                       | boolean | no        | true                                                                          | Whether the gRPC listener is to be enabled. It should be set to `true` if `connectInject` or `meshGateway` is enabled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `client.nodeMeta`                   | object  | no        | {"pod-name": "${HOSTNAME}", "host-ip": "${HOST_IP}"}                          | The arbitrary metadata key-value pair to associate with the node.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `client.exposeGossipPorts`          | boolean | no        | false                                                                         | Whether the clients' gossip ports as `hostPorts` are to be exposed. This is only necessary if pod IPs in the k8s cluster are not directly routable, and the Consul servers are outside the k8s cluster. This also changes the clients' advertised IP to the `hostIP` rather than `podIP`.                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `client.serviceAccount.annotations` | object  | no        | {}                                                                            | The additional annotations for the client service account.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `client.resources.requests.cpu`     | string  | no        | 25m                                                                           | The minimum number of CPUs the Consul client container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `client.resources.requests.memory`  | string  | no        | 64Mi                                                                          | The minimum amount of memory the Consul client container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `client.resources.limits.cpu`       | string  | no        | 200m                                                                          | The maximum number of CPUs the Consul client container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `client.resources.limits.memory`    | string  | no        | 256Mi                                                                         | The maximum amount of memory the Consul client container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `client.securityContext`            | object  | no        | {"runAsNonRoot": true, "runAsGroup": 1000, "runAsUser": 100, "fsGroup": 1000} | The pod-level security attributes and common container settings for Consul client pod. **Note**: if it is running on OpenShift, this setting is ignored because the user and group are set automatically by the OpenShift platform.                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `client.extraConfig`                | object  | no        | {"disable_update_check": true}                                                | The extra configuration to set to the client. It should be JSON. More info about extra configs in [Configuration Files](https://www.consul.io/docs/agent/options.html).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `client.extraVolumes`               | list    | no        | []                                                                            | The list of extra volumes to mount. They are exposed to Consul in the path `/consul/userconfig/<name>/`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `client.tolerations`                | object  | no        | {}                                                                            | The list of toleration policies for Consul clients in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `client.nodeSelector`               | object  | no        | {}                                                                            | The labels for client pod assignment, formatted as a JSON string. For more information, refer to [https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector).                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `client.affinity`                   | object  | no        | {}                                                                            | The affinity scheduling rules in JSON format. To allow deployment to single node services such as Minikube, you can comment out or set the affinity variable as empty.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `client.priorityClassName`          | string  | no        | ""                                                                            | The priority class to be used to assign priority to Consul client pods. Priority class should be created beforehand. For more information, refer to [https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                                                                                                                                                                                                                                                                                                 |
| `client.annotations`                | object  | no        | {}                                                                            | The list of extra annotations to set to the Consul daemon set.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `client.extraLabels`                | object  | no        | null                                                                          | The extra labels to attach to the client pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `client.extraEnvironmentVars`       | object  | no        | {}                                                                            | The list of extra environment variables to set to the Consul daemon set. You can use these environment variables to include proxy settings required for cloud auto-join feature, in case Kubernetes cluster is behind egress HTTP proxies. Additionally, it could be used to configure custom Consul parameters.                                                                                                                                                                                                                                                                                                                                                                                              |
| `client.dnsPolicy`                  | string  | no        | null                                                                          | The [Pod DNS policy](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy) for client pods to use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `client.hostNetwork`                | boolean | no        | false                                                                         | Whether host networking is to be used instead of hostPort in the event that a CNI plugin doesn't support hostPort. This has security implications and is not recommended as doing so gives the Consul client unnecessary access to all network traffic on the host. In most cases, pod network and host network are on different networks so this should be combined with `client.dnsPolicy: ClusterFirstWithHostNet`.                                                                                                                                                                                                                                                                                        |
| `client.updateStrategy`             | object  | no        | {}                                                                            | The [Update Strategy](https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/#daemonset-update-strategy) for the client DaemonSet.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `client.logLevel`                   | string  | no        | ""                                                                            | Enables The log verbosity level. It is recommended to generally not set this below "info" unless actively debugging due to logging verbosity. The possible values are `debug`, `info`, `warn`, `error`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `client.ports.https`                | string  | no        | ""                                                                            | Customize the client's https consul port providing the ability to deploy two Consul instances to one cluster. This parameter takes precedence over the global parameter when defined.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `client.ports.http`                 | string  | no        | ""                                                                            | Customize the client's http consul port providing the ability to deploy two Consul instances to one cluster. This parameter takes precedence over the global parameter when defined.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `client.ports.grpc`                 | string  | no        | ""                                                                            | Customize the client's grpc consul port providing the ability to deploy two Consul instances to one cluster. This parameter takes precedence over the global parameter when defined.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |

## DNS

You can create configuration for DNS within the Kubernetes cluster. This creates a service that routes to all agents, client or server,
for serving DNS requests.
It does not automatically configure kube-dns, you must manually configure a `stubDomain` with kube-dns for this to have an effect.
For more information, refer to 
[Configuration of Stub-domain and upstream nameserver using CoreDNS](https://kubernetes.io/docs/tasks/administer-cluster/dns-custom-nameservers/#configuration-of-stub-domain-and-upstream-nameserver-using-coredns)
.

| Parameter            | Type    | Mandatory | Default value | Description                                                                                                                                          |
|----------------------|---------|-----------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dns.enabled`        | boolean | no        | true          | Whether the service that routes to all agents is to be created.                                                                                      |
| `dns.type`           | string  | no        | ClusterIP     | The type of service created. For example, setting this to `LoadBalancer` creates an external load balancer (for supported Kubernetes installations). |
| `dns.clusterIP`      | string  | no        | null          | The predefined cluster IP for the DNS service. This is useful if you need to reference the DNS service's IP address in CoreDNS config.               |
| `dns.annotations`    | object  | no        | {}            | The extra annotations to attach to the DNS service. It should be a JSON string of annotations to apply to the DNS Service.                           |
| `dns.additionalSpec` | string  | no        | null          | The additional `ServiceSpec` values. It should be a JSON string mapping directly to a Kubernetes `ServiceSpec` object.                               |

## UI

To enable the Consul web UI, update the `ui` section to the values file and set the `enabled` parameter value to `true`.

| Parameter                   | Type    | Mandatory | Default value              | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
|-----------------------------|---------|-----------|----------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ui.enabled`                | boolean | no        | false                      | Whether Consul UI is to be enabled. Specify the value as `true` if you want to enable the Consul UI. The UI runs only on the server nodes. This makes UI access via the service (if enabled) predictable rather than "any node" if you are running Consul clients as well.                                                                                                                                                                                                                                               |
| `ui.service.enabled`        | boolean | no        | true                       | Whether the service entry for the Consul UI is to be created.                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `ui.service.type`           | string  | no        | null                       | The type of service created for the Consul UI. If this parameter value is set to `LoadBalancer`, an external load balancer for supported K8S installations to access the UI is to be created.                                                                                                                                                                                                                                                                                                                            |
| `ui.service.nodePort.http`  | string  | no        | null                       | The HTTP node port of the UI service if a `NodePort` service is used.                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `ui.service.nodePort.https` | string  | no        | null                       | The HTTPS node port of the UI service if a `NodePort` service is used.                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `ui.service.annotations`    | object  | no        | {}                         | The annotations to apply to the UI service.                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `ui.service.additionalSpec` | object  | no        | {}                         | The additional `ServiceSpec` values. It should be a JSON string mapping directly to a Kubernetes `ServiceSpec` object.                                                                                                                                                                                                                                                                                                                                                                                                   |
| `ui.ingress.enabled`        | boolean | no        | false                      | Whether `Ingress` resource to access Consul UI outside the cloud is to be created.                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `ui.ingress.hosts`          | list    | no        | []                         | The list of external hostname, which the Consul UI should be available on, and paths to create Ingress rules. The hostname must be complex and unique enough not to intersect with other possible external hostnames. For example, to generate hostname value for this parameter you can use the OpenShift/Kubernetes host: if URL to OpenShift/Kubernetes is ```https://search.example.com:8443``` and the namespace is `consul-service`, the hostname for Consul UI can be `consul-consul-service.search.example.com`. |
| `ui.ingress.tls`            | list    | no        | []                         | The list of hosts and secret name in an Ingress which tells the Ingress controller to secure the channel.                                                                                                                                                                                                                                                                                                                                                                                                                |
| `ui.ingress.annotations`    | object  | no        | null                       | The annotations to apply to the UI ingress.                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `ui.metrics.enabled`        | boolean | no        | true                       | Whether metrics are to be displayed in the UI.                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `ui.metrics.provider`       | string  | no        | prometheus                 | The provider for metrics. This value is used only if `ui.enabled` is set to `true`.                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `ui.metrics.annotations`    | string  | no        | <http://prometheus-server> | The URL of the prometheus server, usually the service URL. This value is used only if `ui.enabled` is set to `true`.                                                                                                                                                                                                                                                                                                                                                                                                     |

## Sync Catalog

Sync Catalog runs the catalog sync process to sync K8S with Consul services.
This can run bidirectional (default) or unidirectional (Consul to K8S or K8S to Consul only).
This process assumes that a Consul agent is available on the host IP.
This is done automatically if clients are enabled.
If clients are not enabled then set the node selection to choose a node with a Consul agent.

| Parameter                                | Type    | Mandatory | Default value                  | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|------------------------------------------|---------|-----------|--------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `syncCatalog.enabled`                    | boolean | no        | false                          | Whether the catalog sync process to sync K8S with Consul services is to be enabled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `syncCatalog.default`                    | boolean | no        | true                           | Whether the sync is enabled by default, otherwise it requires annotation.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `syncCatalog.priorityClassName`          | string  | no        | ""                             | The priority class to be used to assign priority to Sync Catalog pods. Priority class should be created beforehand. For more information, refer to [Pod Priority and Preemption](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                                                                                                                                                                                               |
| `syncCatalog.toConsul`                   | boolean | no        | true                           | Whether syncing is to be enabled to Consul as a destination.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `syncCatalog.toK8S`                      | boolean | no        | true                           | Whether syncing is to be enabled to K8S as a destination.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `syncCatalog.k8sPrefix`                  | string  | no        | null                           | The service prefix to prepend it to services before registering with Kubernetes. For example, `consul-` registers all services prepended with `consul-`.                                                                                                                                                                                                                                                                                                                                                                                                    |
| `syncCatalog.k8sAllowNamespaces`         | list    | no        | ["*"]                          | The list of Kubernetes namespaces to sync the Kubernetes services from. If a Kubernetes namespace is not included in this list or is listed in `syncCatalog.k8sDenyNamespaces`, services in that Kubernetes namespace are not synced even if they are explicitly annotated. Use `["*"]` to automatically allow all Kubernetes namespaces. For example, `["namespace1", "namespace2"]` only allows services in the Kubernetes namespaces `namespace1` and `namespace2` to be synced and registered with Consul. All other Kubernetes namespaces are ignored. |
| `syncCatalog.k8sDenyNamespaces`          | list    | no        | ["kube-system", "kube-public"] | The list of Kubernetes namespaces that should not have their services synced. This list takes precedence over `syncCatalog.k8sAllowNamespaces`. `*` is not supported because then nothing would be allowed to sync. For example, if `syncCatalog.k8sAllowNamespaces` is `["*"]` and `syncCatalog.k8sDenyNamespaces` is `["namespace1", "namespace2"]`, then all Kubernetes namespaces besides `namespace1` and `namespace2` are synced.                                                                                                                     |
| `syncCatalog.k8sSourceNamespace`         | string  | no        | null                           | The Kubernetes namespace to watch for service changes and sync to Consul. If this is not set then it will default to all namespaces. **[DEPRECATED] Use `syncCatalog.k8sAllowNamespaces` and `syncCatalog.k8sDenyNamespaces` instead.**                                                                                                                                                                                                                                                                                                                     |
| `syncCatalog.addK8SNamespaceSuffix`      | boolean | no        | true                           | Whether sync catalog is to append Kubernetes namespace suffix to each service name synced to Consul, separated by a dash. For example, for a service `foo` in the `default` namespace, the sync process creates a Consul service named `foo-default`. Set this flag to `true` to avoid registering services with the same name but in different namespaces as instances for the same Consul service. Namespace suffix is not added if `annotationServiceName` is provided.                                                                                  |
| `syncCatalog.consulPrefix`               | string  | no        | null                           | The service prefix which prepends itself to Kubernetes services registered within Consul. For example, `k8s-` registers all services prepended with `k8s-`.                                                                                                                                                                                                                                                                                                                                                                                                 |
| `syncCatalog.k8sTag`                     | string  | no        | null                           | The optional tag that is applied to all the Kubernetes services that are synced into Consul.                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `syncCatalog.consulNodeName`             | string  | no        | k8s-sync                       | The Consul synthetic node that all services will be registered to. **Note**: Changing the node name and upgrading the Helm chart leave all the previously synced services registered with Consul and register them again under the new Consul node name. The out-of-date registrations need to be explicitly removed.                                                                                                                                                                                                                                       |
| `syncCatalog.syncClusterIPServices`      | boolean | no        | true                           | Whether services of the ClusterIP type are to be synced. They may or may not be broadly accessible depending on your Kubernetes cluster. Set this value to `false` to skip syncing ClusterIP services.                                                                                                                                                                                                                                                                                                                                                      |
| `syncCatalog.nodePortSyncType`           | string  | no        | ExternalFirst                  | The type of syncing that happens for NodePort services. The possible values are `ExternalOnly` (uses a node's external IP address for the sync), `InternalOnly` (uses the node's internal IP address) and `ExternalFirst` (preferentially uses the node's external IP address, but if it does not exist, it uses the node's internal IP address instead).                                                                                                                                                                                                   |
| `syncCatalog.aclSyncToken.secretName`    | string  | no        | null                           | The name of Kubernetes secret that contains an ACL token for your Consul cluster which allows the sync to process the correct permissions. This is only needed if ACLs are enabled on the Consul cluster.                                                                                                                                                                                                                                                                                                                                                   |
| `syncCatalog.aclSyncToken.secretKey`     | string  | no        | null                           | The key of Kubernetes secret that contains an ACL token for your Consul cluster which allows the sync to process the correct permissions. This is only needed if ACLs are enabled on the Consul cluster.                                                                                                                                                                                                                                                                                                                                                    |
| `syncCatalog.nodeSelector`               | object  | no        | null                           | The labels for Sync Catalog pod assignment, formatted as a JSON string. For more information, refer to [NodeSelector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector).                                                                                                                                                                                                                                                                                                                                                     |
| `syncCatalog.affinity`                   | object  | no        | {}                             | The affinity scheduling rules in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `syncCatalog.tolerations`                | object  | no        | {}                             | The list of toleration policies for Sync Catalog pods in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `syncCatalog.securityContext`            | object  | no        | {}                             | The pod-level security attributes and common container settings for Sync Catalog pods.                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `syncCatalog.serviceAccount.annotations` | object  | no        | null                           | The additional annotations for the Sync Catalog service account.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `syncCatalog.resources.requests.cpu`     | string  | no        | 50m                            | The minimum number of CPUs the Sync Catalog container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `syncCatalog.resources.requests.memory`  | string  | no        | 50Mi                           | The minimum amount of memory the Sync Catalog container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `syncCatalog.resources.limits.cpu`       | string  | no        | 50m                            | The maximum number of CPUs the Sync Catalog container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `syncCatalog.resources.limits.memory`    | string  | no        | 50Mi                           | The maximum amount of memory the Sync Catalog container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `syncCatalog.logLevel`                   | string  | no        | info                           | The log verbosity level. It is recommended to generally not set this below "info" unless actively debugging due to logging verbosity. The possible values are `debug`, `info`, `warn`, `error`.                                                                                                                                                                                                                                                                                                                                                             |
| `syncCatalog.consulWriteInterval`        | string  | no        | null                           | The interval to perform syncing operations creating Consul services.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `syncCatalog.extraLabels`                | object  | no        | null                           | The extra labels to attach to the Sync Catalog pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

## Connect Injector

After you enable Consul server communication over Connect in the server section, you also need to enable `connectInject`
by setting the `enabled` parameter value to `true`. You can also configure security features.
When you enable the `default` parameter, it allows the injector to automatically inject the Connect sidecar into all pods.
If you prefer to manually annotate which pods to inject, you can set this value to `false`.

Also, you need to have rights for creating `MutatingWebhookConfiguration` if you enable `connectInject` parameter.

| Parameter                                               | Type    | Mandatory | Default value                  | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|---------------------------------------------------------|---------|-----------|--------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `connectInject.enabled`                                 | boolean | no        | false                          | Whether the automatic Connect sidecar injector is to be enabled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `connectInject.replicas`                                | integer | no        | 1                              | The number of Connect Inject deployment replicas.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `connectInject.extraLabels`                             | object  | no        | null                           | The extra labels to attach to the Connect Injector pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `connectInject.default`                                 | boolean | no        | false                          | Whether the inject is enabled by default, otherwise it requires annotation. **Note**: It is highly recommended enabling TLS with this feature because it requires making calls to Consul clients across the cluster. Without TLS enabled, these calls could leak ACL tokens should the cluster network become compromised.                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `connectInject.transparentProxy.defaultEnabled`         | boolean | no        | true                           | Whether all Consul Service mesh are to run with transparent proxy enabled by default, i.e. we enforce that all traffic within the pod goes through the proxy. This value is overridable via the `consul.hashicorp.com/transparent-proxy` pod annotation.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `connectInject.transparentProxy.defaultOverwriteProbes` | boolean | no        | true                           | Whether Kubernetes HTTP probes of the pod are to be overridden to point to the Consul DataPlane proxy instead. This setting is recommended because with traffic being enforced to go through the Consul DataPlane proxy, the probes on the pod fail because kube-proxy doesn't have the right certificates to talk to Consul DataPlane. This value is also overridable via the `consul.hashicorp.com/transparent-proxy-overwrite-probes` annotation. **Note**: This value has no effect if transparent proxy is disabled on the pod.                                                                                                                                                                                                                |
| `connectInject.disruptionBudget.enabled`                | boolean | no        | true                           | Whether the creation of a `PodDisruptionBudget` to prevent voluntary degrading of the Connect inject pods is to be created.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `connectInject.disruptionBudget.maxUnavailable`         | integer | no        | (<connectInject.replicas>/2)-1 | The maximum number of unavailable pods.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `connectInject.disruptionBudget.minAvailable`           | integer | no        | null                           | The minimum number of available pods. It takes precedence over `connectInject.disruptionBudget.maxUnavailable` parameter if it is set.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `connectInject.metrics.defaultEnabled`                  | boolean | no        | true                           | Whether connect-injector is to add prometheus annotations to connect-injected pods automatically. It also adds a listener on the Consul DataPlane sidecar to expose metrics. The exposed metrics depend on whether metrics merging is enabled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `connectInject.metrics.defaultEnableMerging`            | boolean | no        | false                          | Whether the Consul sidecar is to run a merged metrics server to combine and serve both Consul DataPlane and Connect service metrics.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `connectInject.metrics.defaultMergedMetricsPort`        | string  | no        | 20100                          | The port at which the Consul sidecar listens on to return combined metrics. This port only needs to be changed if it conflicts with the application's ports.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `connectInject.metrics.defaultPrometheusScrapePort`     | string  | no        | 20200                          | The port Prometheus scrapes metrics from, by configuring the pod annotation `prometheus.io/port` and the corresponding listener in the Consul DataPlane sidecar. **Note**: This is **not** the port that your application exposes metrics on. That can be configured with the `consul.hashicorp.com/service-metrics-port` annotation.                                                                                                                                                                                                                                                                                                                                                                                                               |
| `connectInject.metrics.defaultPrometheusScrapePath`     | string  | no        | /metrics                       | The path Prometheus scrapes metrics from, by configuring the pod annotation `prometheus.io/path` and the corresponding handler in the Consul DataPlane sidecar. **Note**: This is **not** the path that your application exposes metrics on. That can be configured with the `consul.hashicorp.com/service-metrics-path` annotation.                                                                                                                                                                                                                                                                                                                                                                                                                |
| `connectInject.priorityClassName`                       | string  | no        | ""                             | The priority class to be used to assign priority to Connect Injector pods. Priority class should be created beforehand. For more information, refer to [Pod Priority and Preemption](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `connectInject.logLevel`                                | string  | no        | info                           | The log verbosity level. It is recommended to generally not set this below "info" unless actively debugging due to logging verbosity. The possible values are `debug`, `info`, `warn`, `error`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `connectInject.serviceAccount.annotations`              | object  | no        | {}                             | The additional annotations for the Connect Injector service account.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `connectInject.resources.requests.cpu`                  | string  | no        | 50m                            | The minimum number of CPUs the Connect Injector container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `connectInject.resources.requests.memory`               | string  | no        | 50Mi                           | The minimum amount of memory the Connect Injector container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `connectInject.resources.limits.cpu`                    | string  | no        | 50m                            | The maximum number of CPUs the Connect Injector container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `connectInject.resources.limits.memory`                 | string  | no        | 50Mi                           | The maximum amount of memory the Connect Injector container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `connectInject.namespaceSelector`                       | object  | no        | {}                             | The selector for restricting the webhook to only specific namespaces. It should be set to a JSON string.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `connectInject.k8sAllowNamespaces`                      | list    | no        | ["*"]                          | The list of Kubernetes namespaces to allow `Connect` sidecar injection in. If a Kubernetes namespace is not included in this list or is listed in `connectInject.k8sDenyNamespaces`, pods in that Kubernetes namespace are not injected even if they are explicitly annotated. Use `["*"]` to automatically allow all Kubernetes namespaces. For example, `["namespace1", "namespace2"]` only allows pods in the Kubernetes namespaces `namespace1` and `namespace2` to have `Connect` sidecars injected and registered with Consul. All other Kubernetes namespaces are ignored. **Note**: `connectInject.k8sDenyNamespaces` takes precedence over values defined here. `kube-system` and `kube-public` are never injected, even if included here. |
| `connectInject.k8sDenyNamespaces`                       | list    | no        | []                             | The list of Kubernetes namespaces that should not allow `Connect` sidecar injection. This list takes precedence over `connectInject.k8sAllowNamespaces`. `*` is not supported because then nothing would be allowed to be injected. For example, if `connectInject.k8sAllowNamespaces` is `["*"]` and `connectInject.k8sDenyNamespaces` is `["namespace1", "namespace2"]`, then all Kubernetes namespaces besides `namespace1` and `namespace2` are injected. **Note**: `kube-system` and `kube-public` are never injected.                                                                                                                                                                                                                         |
| `connectInject.certs.secretName`                        | string  | no        | null                           | The name of the secret that has the TLS certificate and private key to serve the injector webhook. If this value is `null`, the injector uses the default automatic management mode that assigns a service account to the injector to generate its own certificates.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `connectInject.certs.caBundle`                          | string  | no        | ""                             | The base64-encoded PEM-encoded certificate bundle for the CA that signed the TLS certificate that the webhook serves. This value must be set if `connectInject.certs.secretName` is not null.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `connectInject.certs.certName`                          | string  | no        | tls.crt                        | The name of the files within the secret for the TLS certificate.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `connectInject.certs.keyName`                           | string  | no        | tls.key                        | The name of the files within the secret for the TLS private key.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `connectInject.nodeSelector`                            | object  | no        | null                           | The labels for Connect Injector pod assignment, formatted as a JSON string. For more information, refer to [NodeSelector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `connectInject.affinity`                                | object  | no        | {}                             | The affinity scheduling rules in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `connectInject.tolerations`                             | object  | no        | {}                             | The list of toleration policies for Connect Injector pods in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `connectInject.securityContext`                         | object  | no        | {}                             | The pod-level security attributes and common container settings for Connect Injector pods.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `connectInject.aclBindingRuleSelector`                  | string  | no        | serviceaccount.name!=default   | The query that defines which service accounts can authenticate to Consul and receive an ACL token during Connect injection. The default setting, `serviceaccount.name!=default`, prevents the 'default' service account from logging in. If the value is set to an empty string, all service accounts can log in. This only has effect if ACLs are enabled.                                                                                                                                                                                                                                                                                                                                                                                         |
| `connectInject.overrideAuthMethodName`                  | string  | no        | ""                             | The auth method for Connect inject. Set the value to the name of your auth method.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `connectInject.aclInjectToken.secretName`               | string  | no        | null                           | The name of the Kubernetes secret that contains an ACL token for your Consul cluster which allows the Connect injector the correct permissions. This is only needed if ACLs are enabled on the Consul cluster, and you are not setting `global.acls.manageSystemACLs` to `true`. This token needs to have `operator = "write"` privileges to be able to create Consul namespaces.                                                                                                                                                                                                                                                                                                                                                                   |
| `connectInject.aclInjectToken.secretKey`                | string  | no        | null                           | The key of the Kubernetes secret that contains an ACL token for your Consul cluster which allows the Connect injector the correct permissions. This is only needed if ACLs are enabled on the Consul cluster, and you are not setting `global.acls.manageSystemACLs` to `true`. This token needs to have `operator = "write"` privileges to be able to create Consul namespaces.                                                                                                                                                                                                                                                                                                                                                                    |
| `connectInject.sidecarProxy.resources.requests.cpu`     | string  | no        | null                           | The minimum number of CPUs the Consul DataPlane sidecar proxy container injected into each Connect pod should use. The recommended value is `100m`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `connectInject.sidecarProxy.resources.requests.memory`  | string  | no        | null                           | The minimum amount of memory the Consul DataPlane sidecar proxy container injected into each Connect pod should use. The recommended value is `100Mi`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `connectInject.sidecarProxy.resources.limits.cpu`       | string  | no        | null                           | The maximum number of CPUs the Consul DataPlane sidecar proxy container injected into each Connect pod should use. The recommended value is `100m`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `connectInject.sidecarProxy.resources.limits.memory`    | string  | no        | null                           | The maximum amount of memory the Consul DataPlane sidecar proxy container injected into each Connect pod should use. The recommended value is `100Mi`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `connectInject.initContainer.resources.requests.cpu`    | string  | no        | 50m                            | The minimum number of CPUs the Connect injected init container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `connectInject.initContainer.resources.requests.memory` | string  | no        | 25Mi                           | The minimum amount of memory the Connect injected init container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `connectInject.initContainer.resources.limits.cpu`      | string  | no        | 50m                            | The maximum number of CPUs the Connect injected init container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `connectInject.initContainer.resources.limits.memory`   | string  | no        | 150Mi                          | The maximum amount of memory the Connect injected init container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

## Mesh Gateway

Mesh Gateways enables Consul Connect to work across Consul datacenters.

| Parameter                                                        | Type    | Mandatory | Default value        | Description                                                                                                                                                                                                                                                                                                                                                                      |
|------------------------------------------------------------------|---------|-----------|----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `meshGateway.enabled`                                            | boolean | no        | false                | Whether Consul service mesh is to be configured to use gateways. This setting is required for cluster peering.                                                                                                                                                                                                                                                                   |
| `meshGateway.replicas`                                           | integer | no        | 1                    | The number of Mesh Gateway deployment replicas.                                                                                                                                                                                                                                                                                                                                  |
| `meshGateway.extraLabels`                                        | object  | no        | {}                   | The extra labels to attach to the Mesh Gateway pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                    |
| `meshGateway.wanAddress.source`                                  | string  | no        | Service              | The source where the WAN address (and possibly port) for the mesh gateway is retrieved from. The possible values are `Service`, `NodeIP`, `NodeName` or `Static`. For more information, refer to [WAN Address](https://developer.hashicorp.com/consul/docs/k8s/helm#v-meshgateway-wanaddress).                                                                                   |
| `meshGateway.wanAddress.port`                                    | string  | no        | 443                  | The port that gets registered for WAN traffic. If `meshGateway.wanAddress.source` is set to `Service`, this setting has no effect.                                                                                                                                                                                                                                               |
| `meshGateway.wanAddress.static`                                  | string  | no        | ""                   | The WAN address of the mesh gateways if `meshGateway.wanAddress.source` is set to `Static`. It is useful if you've configured a DNS entry to point to your mesh gateways.                                                                                                                                                                                                        |
| `meshGateway.service.type`                                       | string  | no        | ClusterIP            | The type of Mesh Gateway service. For example `LoadBalancer`, `ClusterIP`, etc.                                                                                                                                                                                                                                                                                                  |
| `meshGateway.service.port`                                       | string  | no        | 443                  | The port that the service is exposed on. The `targetPort` is set to `meshGateway.containerPort`.                                                                                                                                                                                                                                                                                 |
| `meshGateway.service.nodePort`                                   | string  | no        | null                 | The port of the node that the service is exposed on. Optionally hardcode the `nodePort` of the service if using a `NodePort` service. If it is not set and a `NodePort` service is used, Kubernetes assigns a port automatically.                                                                                                                                                |
| `meshGateway.service.annotations`                                | object  | no        | {}                   | The annotations to apply to the Mesh Gateway service.                                                                                                                                                                                                                                                                                                                            |
| `meshGateway.service.additionalSpec`                             | object  | no        | {}                   | The additional `ServiceSpec` values. It should be a JSON string mapping directly to a Kubernetes `ServiceSpec` object.                                                                                                                                                                                                                                                           |
| `meshGateway.hostNetwork`                                        | boolean | no        | false                | Whether gateway Pods is to run on the host network.                                                                                                                                                                                                                                                                                                                              |
| `meshGateway.dnsPolicy`                                          | string  | no        | null                 | The DNS policy to use.                                                                                                                                                                                                                                                                                                                                                           |
| `meshGateway.consulServiceName`                                  | string  | no        | mesh-gateway         | The Consul service name for the mesh gateways. It cannot be set to anything other than `mesh-gateway` if `global.acls.manageSystemACLs` is `true` since the ACL token is generated only for the name `mesh-gateway`.                                                                                                                                                             |
| `meshGateway.containerPort`                                      | string  | no        | 8443                 | The port that the gateway runs on inside the container.                                                                                                                                                                                                                                                                                                                          |
| `meshGateway.hostPort`                                           | string  | no        | null                 | The optional `hostPort` for the gateway to be exposed on. It can be used with `meshGateway.wanAddress.port` to expose the gateways directly from the node. If `meshGateway.hostNetwork` is set to `true`, it must be `null` or set to the same port as `meshGateway.containerPort`. **Note**: You cannot set it to 8500 or 8502 because those are reserved for the Consul agent. |
| `meshGateway.serviceAccount.annotations`                         | object  | no        | {}                   | The additional annotations for the Mesh Gateway service account.                                                                                                                                                                                                                                                                                                                 |
| `meshGateway.resources.requests.cpu`                             | string  | no        | 50m                  | The minimum number of CPUs the Mesh Gateway container should use.                                                                                                                                                                                                                                                                                                                |
| `meshGateway.resources.requests.memory`                          | string  | no        | 128Mi                | The minimum amount of memory the Mesh Gateway container should use.                                                                                                                                                                                                                                                                                                              |
| `meshGateway.resources.limits.cpu`                               | string  | no        | 400m                 | The maximum number of CPUs the Mesh Gateway container should use.                                                                                                                                                                                                                                                                                                                |
| `meshGateway.resources.limits.memory`                            | string  | no        | 256Mi                | The maximum amount of memory the Mesh Gateway container should use.                                                                                                                                                                                                                                                                                                              |
| `meshGateway.initServiceInitContainer.resources.requests.cpu`    | string  | no        | 50m                  | The minimum number of CPUs the Mesh Gateway `copy-consul-bin` init container should use.                                                                                                                                                                                                                                                                                         |
| `meshGateway.initServiceInitContainer.resources.requests.memory` | string  | no        | 50Mi                 | The minimum amount of memory the Mesh Gateway `copy-consul-bin` init container should use.                                                                                                                                                                                                                                                                                       |
| `meshGateway.initServiceInitContainer.resources.limits.cpu`      | string  | no        | 50m                  | The maximum number of CPUs the Mesh Gateway `copy-consul-bin` init container should use.                                                                                                                                                                                                                                                                                         |
| `meshGateway.initServiceInitContainer.resources.limits.memory`   | string  | no        | 150Mi                | The maximum amount of memory the Mesh Gateway `copy-consul-bin` init container should use.                                                                                                                                                                                                                                                                                       |
| `meshGateway.affinity`                                           | object  | no        | <anti_affinity_rule> | The affinity scheduling rules in JSON format. **Note**: Gateways require that Consul client agents are also running on the nodes alongside each gateway Pod.                                                                                                                                                                                                                     |
| `meshGateway.tolerations`                                        | object  | no        | {}                   | The list of toleration policies for Mesh Gateway pods in JSON format.                                                                                                                                                                                                                                                                                                            |
| `meshGateway.nodeSelector`                                       | object  | no        | {}                   | The labels for Mesh Gateway pod assignment, formatted as a JSON string. For more information, refer to [NodeSelector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector).                                                                                                                                                                          |
| `meshGateway.priorityClassName`                                  | string  | no        | ""                   | The priority class to be used to assign priority to Mesh Gateway pods. Priority class should be created beforehand. For more information, refer to [Pod Priority and Preemption](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                    |
| `meshGateway.annotations`                                        | object  | no        | {}                   | The additional annotations for Mesh Gateway deployment.                                                                                                                                                                                                                                                                                                                          |
| `meshGateway.securityContext`                                    | object  | no        | {}                   | The pod-level security attributes and common container settings for Mesh Gateway pods.                                                                                                                                                                                                                                                                                           |

Where:

* <anti_affinity_rule> is as follows:

  ```yaml
  affinity: {
    "podAntiAffinity": {
      "requiredDuringSchedulingIgnoredDuringExecution": [
        {
          "labelSelector": {
            "matchLabels": {
              "app": "{{ template \"consul.name\" . }}",
              "release": "{{ .Release.Name }}",
              "component": "mesh-gateway"
            }
          },
          "topologyKey": "kubernetes.io/hostname"
        }
      ]
    }
  }
  ```

## Pod Scheduler

| Parameter                                | Type    | Mandatory | Default value            | Description                                                                                                                                                                                                                              |
|------------------------------------------|---------|-----------|--------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `podScheduler.enabled`                   | boolean | no        | true                     | Whether custom Kubernetes pod scheduler pod is to be deployed to assign Consul pods to nodes with `hostPath` persistent volumes. It must be enabled if `persistentVolumes` and `nodes` are specified for `master` or `data` persistence. |
| `podScheduler.dockerImage`               | string  | no        | Calculates automatically | The docker image for Pod Scheduler.                                                                                                                                                                                                      |
| `podScheduler.affinity`                  | object  | no        | {}                       | The affinity scheduling rules in `JSON` format.                                                                                                                                                                                          |
| `podScheduler.nodeSelector`              | object  | no        | {}                       | The selector that defines the nodes where the Pod Scheduler pods are to be scheduled on.                                                                                                                                                 |
| `podScheduler.resources.requests.cpu`    | string  | no        | 15m                      | The minimum number of CPUs the Pod Scheduler container should use.                                                                                                                                                                       |
| `podScheduler.resources.requests.memory` | string  | no        | 128Mi                    | The minimum number of memory the Pod Scheduler container should use.                                                                                                                                                                     |
| `podScheduler.resources.limits.cpu`      | string  | no        | 50m                      | The maximum number of CPUs the Pod Scheduler container should use.                                                                                                                                                                       |
| `podScheduler.resources.limits.memory`   | string  | no        | 128Mi                    | The maximum number of memory the Pod Scheduler container should use.                                                                                                                                                                     |
| `podScheduler.securityContext`           | object  | no        | {}                       | The pod-level security attributes and common container settings for the Pod Scheduler pods. It should be filled as `runAsUser: 1000` for non-root privileges environments.                                                               |
| `podScheduler.customLabels`              | object  | no        | {}                       | The custom labels for the Consul scheduler pod.                                                                                                                                                                                          |

## Monitoring

Monitoring is a Consul telemetry exported from client and server pods for Prometheus.

For more information regarding metrics, refer to _[Cloud Platform Monitoring Guide](/docs/public/monitoring.md)_.

You can enable Prometheus export of Consul metrics independently by setting the `global.metrics.enabled`
and `global.metrics.enableAgentMetrics` parameters values to `true`.
Fetching the metrics using Prometheus can then be performed using the `/v1/agent/metrics?format=prometheus` endpoint.
The format is natively compatible with Prometheus.

| Parameter                     | Type    | Mandatory | Default value | Description                                                            |
|-------------------------------|---------|-----------|---------------|------------------------------------------------------------------------|
| `monitoring.enabled`          | boolean | no        | true          | Whether the installation of Consul monitoring is to be enabled.        |
| `monitoring.installDashboard` | boolean | no        | true          | Whether the installation of Consul Grafana dashboard is to be enabled. |

## Backup Daemon

Consul Backup Daemon is a service to manage Consul snapshots.

When you enable Consul Backup Daemon in deployment parameters, the separated service is deployed with Consul server.
Backup Daemon is available via Kubernetes service and port `8080` (`8443` for HTTPS) and allows you to collect and
restore snapshots using a schedule or via REST API. Consul Backup Daemon can work with snapshot of all datacenters of Consul cluster.

| Parameter                                                       | Type    | Mandatory | Default value                                                                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
|-----------------------------------------------------------------|---------|-----------|-------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `backupDaemon.enabled`                                          | boolean | no        | false                                                                         | Whether the installation of Consul backup daemon is to be enabled.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `backupDaemon.image`                                            | string  | no        | Calculates automatically                                                      | The docker image of Consul backup daemon.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `backupDaemon.tls.enabled`                                      | boolean | no        | true                                                                          | Whether TLS is to be enabled for Consul backup daemon. This parameter is taken into account only if `global.tls.enabled` parameter is set to `true`. For more information about TLS, see [Encrypted Access](/docs/public/tls.md).                                                                                                                                                                                                                                                                                                                                                 |
| `backupDaemon.tls.certificates.crt`                             | string  | no        | ""                                                                            | The certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                |
| `backupDaemon.tls.certificates.key`                             | string  | no        | ""                                                                            | The private key in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                                |
| `backupDaemon.tls.certificates.ca`                              | string  | no        | ""                                                                            | The root CA certificate in base64 format. It can be specified if `global.tls.enabled` parameter is set to `true`, `global.tls.certManager.enabled` parameter is set to `false`, and you have pre-created TLS certificates.                                                                                                                                                                                                                                                                                                                                                        |
| `backupDaemon.tls.secretName`                                   | string  | no        | ""                                                                            | The name of the secret that contains TLS certificates of Consul backup daemon. It is required if TLS for Consul backup daemon is enabled and certificates generation is disabled.                                                                                                                                                                                                                                                                                                                                                                                                 |
| `backupDaemon.tls.subjectAlternativeName.additionalDnsNames`    | list    | no        | []                                                                            | The list of additional DNS names to be added to the `Subject Alternative Name` field of Consul backup daemon TLS certificate.                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `backupDaemon.tls.subjectAlternativeName.additionalIpAddresses` | list    | no        | []                                                                            | The list of additional IP addresses to be added to the `Subject Alternative Name` field of Consul backup daemon TLS certificate.                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `backupDaemon.storage`                                          | string  | no        | 1Gi                                                                           | The disk size of the attached volume for the Consul backup daemon.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `backupDaemon.storageClass`                                     | string  | no        | null                                                                          | The name of storage class for the Consul backup daemon. **Note**: If `backupDaemon.storageClass` and `backupDaemon.persistentVolume` parameters are not specified the Consul Backup Daemon is deployed with `emptyDir`.                                                                                                                                                                                                                                                                                                                                                           |
| `backupDaemon.persistentVolume`                                 | string  | no        | null                                                                          | The predefined persistent volume for the Consul backup daemon.  **Note**: If `backupDaemon.storageClass` and `backupDaemon.persistentVolume` parameters are not specified the Consul Backup Daemon is deployed with `emptyDir`.                                                                                                                                                                                                                                                                                                                                                   |
| `backupDaemon.s3.enabled`                                       | boolean | no        | false                                                                         | Whether Consul backups are to be stored in S3 storage. Consul supports the following S3 providers: AWS S3, GCS, MinIO, etc. A clipboard storage is needed to be mounted to the Consul backup daemon, it can be an `emptyDir` volume. As soon as backup is uploaded to S3, it is removed from the clipboard storage. A restore procedure works the same way: a backup is downloaded from S3 to the clipboard and restored from it, then it is removed from the clipboard but stays on S3. Eviction procedure removes backups directly from S3.                                     |
| `backupDaemon.s3.sslVerify`                                     | boolean | no        | true                                                                          | This parameter specifies whether or not to verify SSL certificates for S3 connections.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `backupDaemon.s3.sslSecretName`                                 | string  | no        | `""`                                                                          | This parameter specifies name of the secret with CA certificate for S3 connections. If secret not exists and parameter `backupDaemon.s3.sslCert` is specified secret will be created, else boto3 certificates will be used.                                                                                                                                                                                                                                                                                                                                                       |
| `backupDaemon.s3.sslCert`                                       | string  | no        | `""`                                                                          | The root CA certificate in base64 format. It is required if pre-created secret with certificates not exists and default boto3 certificates will not be used.                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `backupDaemon.s3.url`                                           | string  | no        | ""                                                                            | The URL to the S3 storage. For example, `https://s3.amazonaws.com`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `backupDaemon.s3.bucket`                                        | string  | no        | ""                                                                            | The existing bucket in the S3 storage that is used to store backups.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `backupDaemon.s3.keyId`                                         | string  | no        | ""                                                                            | The key ID for the S3 storage. The user must have access to the bucket.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `backupDaemon.s3.keySecret`                                     | string  | no        | ""                                                                            | The key secret for the S3 storage. The user must have access to the bucket.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `backupDaemon.extraLabels`                                      | object  | no        | {}                                                                            | The extra labels to attach to the Consul backup daemon pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `backupDaemon.username`                                         | string  | no        | ""                                                                            | The name of the Consul backup daemon API user. This parameter enables Consul backup daemon authentication. If the parameter is empty, Consul backup daemon is deployed with disabled authentication.                                                                                                                                                                                                                                                                                                                                                                              |
| `backupDaemon.password`                                         | string  | no        | ""                                                                            | The password of the Consul backup daemon API user. This parameter enables Consul backup daemon authentication. If the parameter is empty, Consul backup daemon is deployed with disabled authentication.                                                                                                                                                                                                                                                                                                                                                                          |
| `backupDaemon.backupSchedule`                                   | string  | no        | 0 0 * * *                                                                     | The schedule time in cron format (value must be within quotes). If this parameter is empty, the default schedule (`"0 0 * * *"`), defined in Consul backup daemon configuration, is used. The value `0 0 * * *` means that snapshots are created everyday at 0:00.                                                                                                                                                                                                                                                                                                                |
| `backupDaemon.evictionPolicy`                                   | string  | no        | 1h/1d,7d/delete                                                               | The eviction policy for snapshots. It is a comma-separated string of policies written as `$start_time/$interval`. This policy splits all backups older then `$start_time` to numerous time intervals `$interval` time long. Then it deletes all backups in every interval except the newest one. For example, `1d/7d` policy means "take all backups older then one day, split them in groups by 7-days interval, and leave only the newest". If this parameter is empty, the default eviction policy (`"0/1d,7d/delete"`) defined in Consul backup daemon configuration is used. |
| `backupDaemon.resources.requests.cpu`                           | string  | no        | 25m                                                                           | The minimum number of CPUs the Consul backup daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `backupDaemon.resources.requests.memory`                        | string  | no        | 64Mi                                                                          | The minimum amount of memory the Consul backup daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `backupDaemon.resources.limits.cpu`                             | string  | no        | 200m                                                                          | The maximum number of CPUs the Consul backup daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `backupDaemon.resources.limits.memory`                          | string  | no        | 256Mi                                                                         | The maximum amount of memory the Consul backup daemon container should use.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `backupDaemon.affinity`                                         | object  | no        | {}                                                                            | The affinity scheduling rules in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `backupDaemon.tolerations`                                      | object  | no        | {}                                                                            | The list of toleration policies for Consul backup daemon pods in JSON format.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `backupDaemon.nodeSelector`                                     | object  | no        | {}                                                                            | The labels for Consul backup daemon pod assignment, formatted as a JSON string. For more information, refer to [NodeSelector](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#nodeselector).                                                                                                                                                                                                                                                                                                                                                                   |
| `backupDaemon.priorityClassName`                                | string  | no        | ""                                                                            | The priority class to be used to assign priority to Consul backup daemon pods. Priority class should be created beforehand. For more information, refer to [Pod Priority and Preemption](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                                                                                                                                                                                                             |
| `backupDaemon.securityContext`                                  | object  | no        | {"runAsNonRoot": true, "runAsGroup": 1000, "runAsUser": 100, "fsGroup": 1000} | The pod-level security attributes and common container settings for Consul backup daemon pod. **Note**: if it is running on OpenShift, this setting is ignored because the user and group are set automatically by the OpenShift platform.                                                                                                                                                                                                                                                                                                                                        |

## ACL Configurator

Consul ACL Configurator is a service to manage Consul ACLs.

When you enable Consul ACL Configurator in deployment parameters, the separate service is deployed with Consul server.
Consul ACL Configurator provides an operator with appropriate Kubernetes CRD to collect and process CRs which contain
Consul ACLs configuration. Also, Consul ACL Configurator has a separate docker container which is an HTTP server on `8088` port to
execute common reconcile process (reload each configuration from all CRs) via REST API.

| Parameter                                         | Type    | Mandatory | Default value                     | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
|---------------------------------------------------|---------|-----------|-----------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `consulAclConfigurator.enabled`                   | boolean | no        | true                              | Whether Consul ACL Configurator is to be installed. Makes no sense enable Consul ACL Configurator if ACLs are not enabled on Consul. Also, Consul ACL Configurator implies client services use Consul authentication method which is installed in the `connectInject` section. So do not set this property to `true` if `connectInject.enabled` property is `false` or `connectInject.overrideAuthMethodName` is empty or `global.acls.manageSystemACLs` is `false`. |
| `consulAclConfigurator.operatorImage`             | string  | no        | Calculates automatically          | The image of Consul ACL Configurator Kubernetes operator which implements collecting processing Consul ACL configuration custom resources.                                                                                                                                                                                                                                                                                                                           |
| `consulAclConfigurator.restServerImage`           | string  | no        | Calculates automatically          | The the image of Consul ACL Configurator HTTP server which allows executing common reconciliation for Consul ACL configuration custom resources by HTTP request from any Kubernetes service account.                                                                                                                                                                                                                                                                 |
| `consulAclConfigurator.resources.requests.cpu`    | string  | no        | 25m                               | The minimum number of CPUs the Consul ACL Configurator containers should use.                                                                                                                                                                                                                                                                                                                                                                                        |
| `consulAclConfigurator.resources.requests.memory` | string  | no        | 128Mi                             | The minimum amount of memory the Consul ACL Configurator containers should use.                                                                                                                                                                                                                                                                                                                                                                                      |
| `consulAclConfigurator.resources.limits.cpu`      | string  | no        | 100m                              | The maximum number of CPUs the Consul ACL Configurator containers should use.                                                                                                                                                                                                                                                                                                                                                                                        |
| `consulAclConfigurator.resources.limits.memory`   | string  | no        | 128Mi                             | The maximum amount of memory the Consul ACL Configurator containers should use.                                                                                                                                                                                                                                                                                                                                                                                      |
| `consulAclConfigurator.reconcilePeriod`           | integer | no        | 100                               | The delay period for repeated a Custom Resource reconciliation in seconds.                                                                                                                                                                                                                                                                                                                                                                                           |
| `consulAclConfigurator.namespaces`                | string  | no        | ""                                | The list of Kubernetes namespaces which watched by Consul ACL Configurator operator. If this parameter is empty, all namespaces are watched.                                                                                                                                                                                                                                                                                                                         |
| `consulAclConfigurator.serviceName`               | string  | no        | consul-acl-configurator-reconcile | The name of Kubernetes service for Consul ACL Configurator HTTP server.                                                                                                                                                                                                                                                                                                                                                                                              |
| `consulAclConfigurator.tolerations`               | object  | no        | {}                                | The list of toleration policies for Consul ACL Configurator pods in JSON format.                                                                                                                                                                                                                                                                                                                                                                                     |
| `consulAclConfigurator.extraLabels`               | object  | no        | {}                                | The extra labels to attach to the Consul ACL Configurator pods. It should be a YAML map.                                                                                                                                                                                                                                                                                                                                                                             |
| `consulAclConfigurator.securityContext`           | object  | no        | {}                                | The pod-level security attributes and common container settings for Consul ACL Configurator pod. **Note**: if it is running on OpenShift, this setting is ignored because the user and group are set automatically by the OpenShift platform.                                                                                                                                                                                                                        |
| `consulAclConfigurator.priorityClassName`         | string  | no        | ""                                | The priority class to be used to assign priority to Consul ACL Configurator pods. Priority class should be created beforehand. For more information, refer to [Pod Priority and Preemption](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/).                                                                                                                                                                                             |
| `consulAclConfigurator.consul.port`               | string  | no        | ""                                | The Consul server port. By default, it is equal to `8500` for non-TLS Consul and `8501` for TLS Consul.                                                                                                                                                                                                                                                                                                                                                              |
| `consulAclConfigurator.allowedNamespaces`         | string  | no        | ""                                | The list of Kubernetes namespaces. If current service account belongs to one of mentioned namespaces it has permissions to send request for common reconciliation to Consul ACL Configurator REST server. If this parameter is empty, all namespaces are allowed.                                                                                                                                                                                                    |

## Deployment Status Provisioner

Deployment Status Provisioner is a component to provide overall Consul service status.

If Deployment Status Provisioner is enabled, the separate job is created during the deployment. 
This job waits until all monitored resources are ready or completed. 
If integration tests are running, this job also waits for the integration tests to complete and writes the final result to the job status.

For more information, refer to the 
[Deployment Status Provisioner](https://github.com/Netcracker/qubership-deployment-status-provisioner/blob/main/Readme.md).

| Parameter                                     | Type    | Mandatory | Default value            | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|-----------------------------------------------|---------|-----------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `statusProvisioner.dockerImage`               | string  | no        | Calculates automatically | The image for Deployment Status Provisioner pod.                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `statusProvisioner.cleanupEnabled`            | boolean | no        |                          | Whether forced cleanup of previous Status Provisioner job is enabled. If the parameter is set to `false` and Kubernetes version is less than `1.21`, then the previous Status Provisioner job must be manually removed before deployment. If the parameter is not defined, then its value is calculated automatically according to the following rules: `false` if Kubernetes version is greater than or equal to `1.21`, `true` if Kubernetes version is less than `1.21`. |
| `statusProvisioner.lifetimeAfterCompletion`   | integer | no        | 600                      | The number of seconds that the job remains alive after its completion. This functionality works only since `1.21` Kubernetes version.                                                                                                                                                                                                                                                                                                                                       |
| `statusProvisioner.podReadinessTimeout`       | integer | no        | 300                      | The timeout in seconds that the job waits for the monitored resources to be ready or completed.                                                                                                                                                                                                                                                                                                                                                                             |
| `statusProvisioner.integrationTestsTimeout`   | integer | no        | 300                      | The timeout in seconds that the job waits for the integration tests to complete.                                                                                                                                                                                                                                                                                                                                                                                            |
| `statusProvisioner.resources.requests.cpu`    | string  | no        | 50m                      | The minimum number of CPUs the container should use.                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `statusProvisioner.resources.requests.memory` | string  | no        | 50Mi                     | The minimum amount of memory the container should use.                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `statusProvisioner.resources.limits.cpu`      | string  | no        | 100m                     | The maximum number of CPUs the container should use.                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `statusProvisioner.resources.limits.memory`   | string  | no        | 100Mi                    | The maximum amount of memory the container should use.                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `statusProvisioner.securityContext`           | object  | no        | {}                       | The pod-level security attributes and common container settings for the Status Provisioner pod. The parameter is empty by default.                                                                                                                                                                                                                                                                                                                                          |

## Update Resources Job

Update resources job is intended to update resource parameters values on post-install/post-upgrade stage.

| Parameter                                      | Type   | Mandatory | Default value | Description                                                                                   |
|------------------------------------------------|--------|-----------|---------------|-----------------------------------------------------------------------------------------------|
| `updateResourcesJob.resources.requests.cpu`    | string | no        | 75m           | The minimum number of CPUs the container should use.                                          |
| `updateResourcesJob.resources.requests.memory` | string | no        | 75Mi          | The minimum amount of memory the container should use.                                        |
| `updateResourcesJob.resources.limits.cpu`      | string | no        | 150m          | The maximum number of CPUs the container should use.                                          |
| `updateResourcesJob.resources.limits.memory`   | string | no        | 150Mi         | The maximum amount of memory the container should use.                                        |
| `updateResourcesJob.securityContext`           | object | no        | {}            | The pod-level security attributes and common container settings for the Update Resources pod. |

## Integration Tests

| Parameter                                     | Type    | Mandatory | Default value            | Description                                                                                                                                                                                                                                                                                                                                                |
|-----------------------------------------------|---------|-----------|--------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `integrationTests.enabled`                    | boolean | no        | false                    | Whether the installation of Consul integration tests is to be enabled.                                                                                                                                                                                                                                                                                     |
| `integrationTests.dockerImage`                | string  | no        | Calculates automatically | The docker image of Consul integration tests.                                                                                                                                                                                                                                                                                                              |
| `integrationTests.secret.aclToken`            | string  | no        | ""                       | The ACL token for authentication in Consul. If the parameter value is not specified, but Consul ACL is enabled, ACL token is taken from Consul secret with bootstrap ACL token (`<name>-bootstrap-acl-token`, where `<name>` is the value of `global.name` parameter).                                                                                     |
| `integrationTests.secret.prometheus.user`     | string  | no        | ""                       | The username for authentication on Prometheus/VictoriaMetrics secured endpoints.                                                                                                                                                                                                                                                                           |
| `integrationTests.secret.prometheus.password` | string  | no        | ""                       | The password for authentication on Prometheus/VictoriaMetrics secured endpoints.                                                                                                                                                                                                                                                                           |
| `integrationTests.affinity`                   | object  | no        | <affinity_rule>          | The affinity scheduling rules in JSON format.                                                                                                                                                                                                                                                                                                              |
| `integrationTests.tags`                       | string  | no        | crud                     | The tags combined with `AND`, `OR` and `NOT` operators that select test cases to run. Information about available tags can be found in the [Integration test tags description](#tags-description) article.                                                                                                                                                 |
| `integrationTests.statusWritingEnabled`       | boolean | no        | true                     | Whether the status of Consul integration tests execution is to be written to deployment.                                                                                                                                                                                                                                                                   |
| `integrationTests.isShortStatusMessage`       | boolean | no        | true                     | Whether the status message is to contain only first line of `result.txt` file. The parameter makes sense only if `integrationTests.statusWritingEnabled` parameter is set to `true`.                                                                                                                                                                       |
| `integrationTests.consulPort`                 | string  | no        | ""                       | The port of the Consul server. By default, it is equal to `8500` for non-TLS Consul and `8501` for TLS Consul.                                                                                                                                                                                                                                             |
| `integrationTests.prometheusUrl`              | string  | no        | ""                       | The URL (with schema and port) to Prometheus. For example, `http://prometheus.cloud.openshift.sdntest.example.com:80`. This parameter must be specified if you want to run integration tests with `prometheus` tag. **Note:** This parameter could be used as VictoriaMetrics URL instead of Prometheus. For example, `http://vmauth-k8s.monitoring:8427`. |
| `integrationTests.resources.requests.cpu`     | string  | no        | 50m                      | The minimum number of CPUs the container should use.                                                                                                                                                                                                                                                                                                       |
| `integrationTests.resources.requests.memory`  | string  | no        | 256Mi                    | The minimum amount of memory the container should use.                                                                                                                                                                                                                                                                                                     |
| `integrationTests.resources.limits.cpu`       | string  | no        | 400m                     | The maximum number of CPUs the container should use.                                                                                                                                                                                                                                                                                                       |
| `integrationTests.resources.limits.memory`    | string  | no        | 256Mi                    | The maximum amount of memory the container should use.                                                                                                                                                                                                                                                                                                     |
| `integrationTests.extraLabels`                | object  | no        | {}                       | The custom labels for the Consul integration tests pod.                                                                                                                                                                                                                                                                                                    |
| `integrationTests.securityContext`            | object  | no        | {}                       | The pod-level security attributes and common container settings for the Consul integration tests pod.                                                                                                                                                                                                                                                      |

* <affinity_rule> is as follows:

  ```yaml
  affinity: {
    "podAffinity": {
      "preferredDuringSchedulingIgnoredDuringExecution": [
        {
          "podAffinityTerm": {
            "labelSelector": {
              "matchExpressions": [
                {
                  "key": "component",
                  "operator": "In",
                  "values": [
                    "consul-server"
                  ]
                }
              ]
            },
            "topologyKey": "kubernetes.io/hostname"
          },
          "weight": 100
        }
      ]
    }
  }
  ```

### Tags description

This section contains information about integration test tags that can be used in order to test Consul service.
You can use the following tags:

* `alerts` tag runs all tests for Prometheus alert cases:
  * `consul_does_not_exist_alert` tag runs `Consul Does Not Exist Alert` test.
  * `consul_is_degraded_alert` tag runs `Consul Is Degraded Alert` test.
  * `consul_is_down_alert` tag runs `Consul Is Down Alert` test.
* `backup` tag runs all tests for backup cases:
  * `full_backup` tag runs `Test Full Backup And Restore` and `Test Full Backup And Restore On S3 Storage` tests.
  * `granular_backup` tag runs `Test Granular Backup And Restore` and `Test Granular Backup And Restore On S3 Storage` tests.
  * `full_backup_s3` tag runs `Test Full Backup And Restore On S3 Storage` test.
  * `granular_backup_s3` tag runs `Test Granular Backup And Restore On S3 Storage` test.
  * `backup_eviction` tag runs `Test Evict Backup By Id` test.
  * `unauthorized_access` tag runs `Test Unauthorized Access` test.
* `crud` tag runs all tests for creating, reading, updating and removing Consul data.
* `ha` tag runs all tests connected to HA scenarios:
  * `exceeding_limit_size` tag runs `Test Value With Exceeding Limit Size` test.
  * `leader_node_deleted` tag runs `Test Leader Node Deleted` test.
* `smoke` tag runs tests to reveal simple failures: it includes tests with `crud` tag.
* `consul_images` tag runs `Test Hardcoded Images` test.

# Installation

## Before You Begin

* Make sure the environment corresponds the requirements in the [Prerequisites](#prerequisites) section.
* Make sure you review the [Upgrade](#upgrade) section.
* Before doing major upgrade, it is recommended to make a backup.
* Check if the application is already installed and find its previous deployments' parameters to make changes.

### Helm

To deploy via Helm you need to prepare YAML file with custom deploy parameters and run the following
command in [Consul Chart](/charts/helm/consul-service):

```bash
helm install [release-name] ./ -f [parameters-yaml] -n [namespace]
```

If you need to use resource profile then you can use the following command:

```bash
helm install [release-name] ./ -f ./resource-profiles/[profile-name-yaml] -f [parameters-yaml] -n [namespace]
```

**Warning**: pure Helm deployment does not support the automatic CRD upgrade procedure, so you need to perform it manually.

```bash
kubectl replace -f ./crds/crd.yaml
```

## On-Prem Examples

### HA Scheme

The minimal template for HA scheme is as follows:

```yaml
global:
  enabled: true
  name: consul
  enablePodSecurityPolicies: false
  acls:
    manageSystemACLs: true
server:
  replicas: 3
  storage: 10Gi
  storageClass: {applicable_to_env_storage_class}
  resources:
    requests:
      memory: "128Mi"
      cpu: "50m"
    limits:
      memory: "1024Mi"
      cpu: "400m"
client:
  enabled: false
ui:
  enabled: true
  service:
    enabled: true
  ingress:
    enabled: true
    hosts:
      - host: consul-{namespace}.{url_to_kubernetes}
monitoring:
  enabled: true
backupDaemon:
  enabled: true
  storage: 1Gi
  storageClass: {applicable_to_env_storage_class}
  username: admin
  password: admin
  resources:
    requests:
      memory: "64Mi"
      cpu: "25m"
    limits:
      memory: "256Mi"
      cpu: "200m"
consulAclConfigurator:
  enabled: true
  resources:
    requests:
      memory: 128Mi
      cpu: 25m
    limits:
      memory: 128Mi
      cpu: 100m
integrationTests:
  enabled: false
DEPLOY_W_HELM: true
ESCAPE_SEQUENCE: true
```

### DR Scheme

See [Consul Disaster Recovery](/docs/public/disaster-recovery.md) guide.

## Google Cloud Examples

### HA Scheme

<details>
<summary>Click to expand YAML</summary>

```yaml
global:
  enabled: true
  name: consul
  enablePodSecurityPolicies: false
  acls:
    manageSystemACLs: true
server:
  replicas: 3
  storage: 10Gi
  storageClass: {applicable_to_env_storage_class}
  resources:
    requests:
      memory: "128Mi"
      cpu: "50m"
    limits:
      memory: "1024Mi"
      cpu: "400m"
client:
  enabled: false
ui:
  enabled: true
  service:
    enabled: true
  ingress:
    enabled: true
    hosts:
      - host: consul-{namespace}.{url_to_kubernetes}
        paths:
          - /
monitoring:
  enabled: true
backupDaemon:
  enabled: true
  s3:
    enabled: true
    url: "https://storage.googleapis.com"
    bucket: {google_cloud_storage_bucket}
    keyId: {google_cloud_storage_key_id}
    keySecret: {google_cloud_storage_secret}
  username: "admin"
  password: "admin"
  resources:
    requests:
      memory: "64Mi"
      cpu: "25m"
    limits:
      memory: "256Mi"
      cpu: "200m"
consulAclConfigurator:
  enabled: true
  resources:
    requests:
      memory: 128Mi
      cpu: 25m
    limits:
      memory: 128Mi
      cpu: 100m
integrationTests:
  enabled: false
DEPLOY_W_HELM: true
ESCAPE_SEQUENCE: true
```

</details>

### DR Scheme

See [Consul Disaster Recovery](/docs/public/disaster-recovery.md) guide.

## AWS Examples

### HA Scheme

The same as [On-Prem Examples HA Scheme](#on-prem-examples).

### DR Scheme

Not applicable

## Azure Examples

### HA Scheme

The same as [On-Prem Examples HA Scheme](#on-prem-examples).

### DR Scheme

The same as [On-Prem Examples DR Scheme](#on-prem-examples).

# Upgrade

## Common

In the common way, the upgrade procedure is the same as the initial deployment. You need to follow `Release Notes` and 
`Breaking Changes` in the version you install to find details.
If you upgrade to a version which has several major diff changes from the installed version (e.g. `0.3.1` over `0.1.4`),
you need to check `Release Notes` and `Breaking Changes` sections for `0.2.0` and `0.3.0` versions.

## Rolling Upgrade

Consul supports rolling upgrade feature with near-zero downtime.

## CRD Upgrade

Custom resource definition `ConsulACL` should be upgraded before the installation if there are any changes.
<!-- #GFCFilterMarkerStart# -->
The CRD for this version is stored in [consul_acl_configurator_crd.yaml](/charts/helm/consul-service/crds/consul_acl_configurator_crd.yaml)
and can be applied with the following command:

```sh
kubectl replace -f consul_acl_configurator_crd.yaml
```

<!-- #GFCFilterMarkerEnd# -->
It can be done automatically during the upgrade with [Automatic CRD Upgrade](#automatic-crd-upgrade) feature.

### Automatic CRD Upgrade

It is possible to upgrade CRD automatically on the environment to the latest one which is presented with the installing version.
This feature is enabled by default if the `DISABLE_CRD` parameter is not `true`.

Automatic CRD upgrade requires the following cluster rights for the deployment user:

```yaml
  - apiGroups: [ "apiextensions.k8s.io" ]
    resources: [ "customresourcedefinitions" ]
    verbs: [ "get", "create", "patch" ]
```

## Migration From DVM to Helm

Not applicable

## Rollback

Consul does not support rollback with downgrade of a version. In this case, you need to do the following steps:

1. Deploy the previous version according to the steps described above[Installation](#installation).
2. Restore the data from backup.

# Additional Features

## Multiple Availability Zone Deployment

When deploying to a cluster with several availability zones, it is important that Consul server pods start in different availability zones.

### Affinity

You can manage pods' distribution using `affinity` rules to prevent Kubernetes from running Consul server pods on nodes of the same
availability zone.

**Note**: This section describes deployment only for `storage class` persistent volumes (PV) type because with predefined PV,
the Consul server pods are started on the nodes that are specified explicitly with persistent volumes.
In that way, it is necessary to take care of creating PVs on nodes belonging to different availability zones in advance.

#### Replicas Fewer Than Availability Zones

For cases when the number of Consul server pods (value of the `server.replicas` parameter) is equal to or less than the number of
availability zones, you need to restrict the start of pods to one pod per availability zone. You can also specify additional node
affinity rule to start pods on allowed Kubernetes nodes.

For this, you can use the following affinity rules:

<details>
<summary>Click to expand YAML</summary>

```yaml
server:
  affinity: {
    "podAntiAffinity": {
      "requiredDuringSchedulingIgnoredDuringExecution": [
        {
          "labelSelector": {
            "matchLabels": [
              "app": "{{ template \"consul.name\" . }}",
              "release": "{{ .Release.Name }}",
              "component": "server"
            ]
          },
          "topologyKey": "topology.kubernetes.io/zone"
        }
      ]
    },
    "nodeAffinity": {
      "requiredDuringSchedulingIgnoredDuringExecution": {
        "nodeSelectorTerms": [
          {
            "matchExpressions": [
              {
                "key": "role",
                "operator": "In",
                "values": [
                  "compute"
                ]
              }
            ]
          }
        ]
      }
    }
  }
```

</details>

Where:

* `topology.kubernetes.io/zone` is the name of the label that defines the availability zone. This is the default name for Kubernetes 1.17+.
  Earlier, `failure-domain.beta.kubernetes.io/zone` was used.
* `role` and `compute` are the sample name and value of label that defines the region to run Consul server pods.

#### Replicas More Than Availability Zones

For cases when the number of Consul server pods (value of the `server.replicas` parameter) is greater than 
the number of availability zones, you need to restrict the start of pods to one pod per node and specify the preferred rule to 
start on different availability zones. You can also specify an additional node affinity rule to start the pods on allowed Kubernetes nodes.

For this, you can use the following affinity rules:

<details>
<summary>Click to expand YAML</summary>

```yaml
server:
  affinity: {
    "podAntiAffinity": {
      "requiredDuringSchedulingIgnoredDuringExecution": [
        {
          "labelSelector": {
            "matchLabels": [
              "app": "{{ template \"consul.name\" . }}",
              "release": "{{ .Release.Name }}",
              "component": "server"
            ]
          },
          "topologyKey": "kubernetes.io/hostname"
        }
      ],
      "preferredDuringSchedulingIgnoredDuringExecution": [
        {
          "weight": 100,
          "podAffinityTerm": {
            "labelSelector": {
              "matchLabels": [
                "app": "{{ template \"consul.name\" . }}",
                "release": "{{ .Release.Name }}",
                "component": "server"
              ]
            },
            "topologyKey": "topology.kubernetes.io/zone"
          }
        }
      ]
    },
    "nodeAffinity": {
      "requiredDuringSchedulingIgnoredDuringExecution": {
        "nodeSelectorTerms": [
          {
            "matchExpressions": [
              {
                "key": "role",
                "operator": "In",
                "values": [
                  "compute"
                ]
              }
            ]
          }
        ]
      }
    }
  }
```

</details>

Where:

* `kubernetes.io/hostname` is the name of the label that defines the Kubernetes node. This is a standard name for Kubernetes.
* `topology.kubernetes.io/zone` is the name of the label that defines the availability zone. This is a standard name for Kubernetes 1.17+.
   Earlier, `failure-domain.beta.kubernetes.io/zone` was used.
* `role` and `compute` are the sample name and value of the label that defines the region to run Consul server pods.

## Consul Authentication Method

To communicate with secured Consul (ACLs enabled) API each client service should log in to Consul via Consul Authentication method.
In practice, it means that client service should know Consul auth method name.
This name is `<prifix>-k8s-auth-method` where `prefix` is `global.name` installation parameter value if it is not `null`.
Otherwise, it is helm chart release name plus "-consul" suffix.
In production version we totally recommend not override default `global.name` value - `consul`.
In this way Consul authentication method name is `consul-k8s-auth-method`.

## Multiple Datacenters

### Federate Multiple Datacenters Via Mesh Gateways

For more information, refer to [Federation Between Kubernetes Clusters](/docs/public/federation-between-datacenters.md#federation-between-kubernetes-clusters).

### Federate Multiple Datacenters Using WAN Gossip

Consul Service supports multiple datacenters. To get started, you need to install two separate datacenters and 
join them via single WAN gossip pool. You can do this manually during installation process.

#### Create Multi-DC Configuration Manually

For more information, refer to [Datacenter Federation with WAN Gossip](https://developer.hashicorp.com/consul/docs/deploy/server/vm/bootstrap).
