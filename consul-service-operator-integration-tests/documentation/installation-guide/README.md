- [Introduction](#introduction)
- [Prerequisites](#prerequisites)
- [Deployment](#deployment)
    - [Configuration](#configuration)
        - [Consul Service Integration Tests Parameters](#consul-service-integration-tests-parameters)
    - [Manual Deployment](#manual-deployment)
    - [Deployment via DP Deployer Job](#deployment-via-dp-deployer-job)
    - [Deployment via Groovy Deployer Job](#deployment-via-groovy-deployer-job)    

# Introduction

This guide covers the necessary steps to install and execute Consul service tests on OpenShift/Kubernetes using Helm.
The chart installs Consul Service Integration Tests service, pod and secret in OpenShift/Kubernetes.

# Prerequisites

* Kubernetes 1.11+ or OpenShift 3.11+
* `kubeclt` 1.11+ or `oc` 3.11+ CLI
* Helm 3.0+

# Deployment

Consul service integration tests installation is based on Helm Chart. Helm Chart is placed in [integration-tests](../../charts/helm/consul-integration-tests)
directory.

## Configuration

This section provides the list of parameters required for Consul Service Integration Tests installation and execution.

### Consul Service Integration Tests Parameters

The `service.name` parameter specifies the name of Consul integration tests service.

The `secret.aclToken` parameter specifies the ACL token for authentication in Consul.

The `secret.backupDaemon.username` parameter specifies the username of the Consul Backup Daemon API user. It can be
empty if authentication is disabled for Consul Backup Daemon.

The `secret.backupDaemon.password` parameter specifies the password of the Consul Backup Daemon API user. It can be
empty if authentication is disabled for Consul Backup Daemon.

The `secret.prometheus.user` parameter specifies the username for authentication on Prometheus/VictoriaMetrics secured endpoints.

The `secret.prometheus.password` parameter specifies the password for authentication on Prometheus/VictoriaMetrics secured endpoints.

The `secret.s3.keyId` parameter specifies the Access Key for authentication on S3 storage secured endpoints.

The `secret.s3.keySecret` parameter specifies the Secret Key for authentication on S3 storage secured endpoints.

The `tls.certManagerEnabled` parameter specifies whether cert manager for Consul services enabled. The default value is `false`.

The `tls.consul.secretName` parameter specifies the secret that contains TLS certificates for Consul. For example, `consul-ca-cert`.

The `tls.backupDaemon.secretName` parameter specifies the secret that contains TLS certificates for Consul Backup Daemon.

The `serviceAccount.create` parameter specifies whether service account for Consul Integration Tests is to be deployed or not.

The `serviceAccount.name` parameter specifies the name of the service account that is used to deploy Consul Integration Tests. If this
parameter is empty, the service account, the required role, role binding are
created automatically with default names (`consul-integration-tests`).

The `integrationTests.image` parameter specifies the Docker image of Consul Service.

The `integrationTests.tags` parameter specifies the tags combined with `AND`, `OR` and `NOT` operators that select test cases to run. You can use the following tags:

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

The `integrationTests.aclEnabled` parameter specifies whether access control lists are enabled for tested Consul or not. The default value is `false`.

The `integrationTests.consulFullName` parameter specifies the Consul service helm chart release name or overridden Consul full name. This parameter is
name prefix for all Consul Kubernetes entities. The parameter is used only if `aclEnabled` is `true`. The default value is `consul`. 

The `integrationTests.consulNamespace` parameter specifies the name of the Kubernetes namespace where Consul is located.

The `integrationTests.consulHost` parameter specifies the host name of Consul server. The default value is `consul-server`.

The `integrationTests.consulPort` parameter specifies the port of Consul. The default value is `8500`.

The `integrationTests.prometheusUrl` parameter specifies the URL (with schema and port) to Prometheus. For example, `http://prometheus.cloud.openshift.sdntest.example.com:80`. The parameter is empty by default. The parameter must be specified if you want to run integration tests with `alerts` tag. **Note:** This parameter could be used as VictoriaMetrics URL instead of Prometheus. For example, `http://vmauth-k8s.monitoring:8427`.

The `integrationTests.s3.enabled` If the Consul backup daemon uses S3 storage to store backups. The default value is `false`.

The `integrationTests.s3.url` parameter specifies the URL to the S3 storage. For example, https://s3.amazonaws.com.

The `integrationTests.s3.bucket` parameter specifies the existing bucket in the S3 storage that is used to store backups.

The `integrationTests.consulScheme` parameter specifies the scheme (`http` or `https`) of Consul. The default value is `http`.

The `integrationTests.consulBackupDaemonHost` parameter specifies the host name of Consul Backup Daemon. The default value is `consul-backup-daemon`.

The `integrationTests.datacenterName` parameter specifies the name of the datacenter.

The `integrationTests.statusWritingEnabled` parameter specifies whether status of Integration tests execution is to be writen to deployment or not.
The default value is `"true"`.

The `integrationTests.isShortStatusMessage` parameter specifies whether status message contains only first line of `result.txt` file or not.
The parameter does not matter if `statusWritingEnabled` is not `"true"`. The default value is `"true"`.

The `integrationTests.resources.requests.memory` parameter specifies the minimum amount of memory the container should use. 
The value can be specified with SI suffixes (E, P, T, G, M, K, m) or 
their power-of-two-equivalents (Ei, Pi, Ti, Gi, Mi, Ki). The default value is `256Mi.`

The `integrationTests.resources.requests.cpu` parameter specifies the minimum number of CPUs the container should use. 
The default value is `200m.`

The `integrationTests.resources.limits.memory` parameter specifies the maximum amount of memory the container can use. 
The value can be specified with SI suffixes (E, P, T, G, M, K, m) or 
their power-of-two-equivalents (Ei, Pi, Ti, Gi, Mi, Ki). The default value is `256Mi`.

The `integrationTests.resources.limits.cpu` parameter specifies the maximum number of CPUs the container can use. 
The default value is `400m.`

The `integrationTests.affinity` parameter specifies the affinity scheduling rules. The value should be specified in json format. The
parameter can be empty.

The `integrationTests.securityContext` parameter allows specifying pod security context for the Consul Service integration tests pod.

## Manual Deployment

### Installation

To deploy Consul service integration tests with Helm you need to customize the `values.yaml` file. For example:

```
service:
  name: consul-integration-tests-runner

secret:
  aclToken: "ACL_TOKEN"
  backupDaemon:
    username: ""
    password: ""
  prometheus:
    user: ""
    password: ""
  s3:
    keyId: ""
    keySecret: ""

serviceAccount:
  create: true
  name: "consul-integration-tests"

integrationTests:
  image: "artifactorycn.netcracker.com:17008/product/prod.platform.streaming_consul-service:master_latest"
  tags: "crud"
  consulNamespace: "consul-service"
  consulHost: "consul-server"
  consulPort: 8500
  consulBackupDaemonHost: "consul-backup-daemon"
  datacenterName: "dc1"
  s3:
    enabled: true
    url: "https://s3.amazonaws.com"
    bucket: "consul-s3-bucket"
  resources:
    requests:
      memory: 256Mi
      cpu: 200m
    limits:
      memory: 256Mi
      cpu: 400m
```

To deploy the service you need to execute the following command:

```
helm install ${RELEASE_NAME} ./consul-integration-tests -n ${NAMESPACE}
```

where:

* `${RELEASE_NAME}` is the Helm Chart release name and the name of the Consul service integration tests. 
For example, `consul-integration-tests`.
* `${NAMESPACE}` is the OpenShift/Kubernetes project/namespace to deploy Consul service integration tests. 
For example, `consul-service`.

You can monitor the deployment process in the OpenShift/Kubernetes dashboard or using `kubectl` in the command line:

```
kubectl get pods
```

### Uninstalling

To uninstall Consul service integration tests from OpenShift/Kubernetes you need to execute the following command:

```
helm delete ${RELEASE_NAME} -n ${NAMESPACE}
```

where:

* `${RELEASE_NAME}` is the Helm Chart release name and the name of the already deployed Consul service integration tests. 
For example, `consul-integration-tests`.
* `${NAMESPACE}` is the OpenShift/Kubernetes project/namespace to deploy Consul service integration tests. 
For example, `consul-service`.

The command uninstalls all the Kubernetes/OpenShift resources associated with the chart and deletes the release.

## Deployment via DP Deployer Job

Navigate to the Jenkins job `DP.Pub.Helm_deployer` and then click **Build with parameters**.

The job parameters are predefined and described as follows:

The `CLOUD_URL` parameter specifies the URL of the OpenShift/Kubernetes server. For example, `https://search.openshift.sdntest.example.com:8443`.

The `CLOUD_NAMESPACE` parameter specifies the name of the existing OpenShift project/Kubernetes namespace. For example,
`consul-service`.

The `CLOUD_USER` parameter specifies the name of the user on behalf of whom the deployment process in
OpenShift/Kubernetes starts. The parameter should be specified with `CLOUD_PASSWORD` parameter if `CLOUD_TOKEN` parameter
is not filled.

The `CLOUD_PASSWORD` parameter specifies the password for the user on behalf of whom the deployment process in
OpenShift/Kubernetes starts. The parameter should be specified with `CLOUD_USER` parameter if `CLOUD_TOKEN` parameter
is not filled.

The `CLOUD_TOKEN` parameter specifies the token for the user on behalf of whom the deployment process in
OpenShift/Kubernetes starts. The parameter should be specified if `CLOUD_USER` and `CLOUD_PASSWORD` parameters are not
filled.

The `DESCRIPTOR_URL` parameter specifies the link to the Consul Service Application Manifest.

The `DEPLOYMENT_PARAMETERS` parameter specifies the yaml that contains all parameters for installation. For example,

```
service:
  name: consul-integration-tests-runner

secret:
  aclToken: "ACL_TOKEN"
  backupDaemon:
    username: ""
    password: ""
  prometheus:
    user: ""
    password: ""
  s3:
    keyId: ""
    keySecret: ""

serviceAccount:
  create: true
  name: "consul-integration-tests"

integrationTests:
  tags: "crud"
  consulNamespace: "consul-service"
  consulHost: "consul-server"
  consulPort: 8500
  consulBackupDaemonHost: "consul-backup-daemon"
  datacenterName: "dc1"
  s3:
    enabled: true
    url: "https://s3.amazonaws.com"
    bucket: "consul-s3-bucket"
  resources:
    requests:
      memory: 256Mi
      cpu: 200m
    limits:
      memory: 256Mi
      cpu: 400m
```

The `DEPLOYMENT_MODE` parameter specifies the mode of the deployment. The possible values are `install` and `upgrade`.

The `ADDITIONAL_OPTIONS` parameter specifies the additional options for Helm install/upgrade commands. For example,
`--skip-crds` can be used in case of installation with restricted rights.

Click *Build*.

## Deployment via Groovy Deployer Job

Navigate to the Jenkins job `groovy.deploy.v3` and then click **Build with parameters**.

The job parameters are predefined and described as follows:

The `PROJECT` parameter specifies the name of the existing OpenShift project/Kubernetes namespace. For example,
`consul-service`.

The `OPENSHIFT_CREDENTIALS` parameter specifies the credentials of the user on behalf of whom the deployment process in
OpenShift/Kubernetes starts.

The `DEPLOY_MODE` parameter specifies the mode of the deployment. The possible values are `Clean Install` and
`Rolling Upgrade`. The `Clean Install` mode removes everything from the project before deployment.

The `ARTIFACT_DESCRIPTOR_VERSION` parameter specifies the version of maven artifact in the format `artifactId:artifactVersion`.
For example, `consul-service-integration-tests:consul_service_integration_tests_v01`.

The `CUSTOM_PARAMS` parameter specifies the list of parameters for Consul service installation. All parameters
should be divided by `;`. For example,

```
service.name=consul-integration-tests-runner;
secret.aclToken=<ACL_TOKEN>;
secret.backupDaemon.username=username;
secret.backupDaemon.password=password;
secret.prometheus.user=user;
secret.prometheus.password=password;
secret.s3.keyId=keyId;
secret.s3.keySecret=keySecret;

integrationTests.install=true;
integrationTests.tags=crud;
integrationTests.consulNamespace=consul-service;
integrationTests.consulHost=consul-server;
integrationTests.consulPort=8500;
integrationTests.consulBackupDaemonHost=consul-backup-daemon;
integrationTests.datacenterName=dc1;
integrationTests.s3.enabled=true;
integrationTests.s3.url=https://s3.amazonaws.com;
integrationTests.s3.bucket=consul-s3-bucket;
integrationTests.resources.requests.memory=256Mi;
integrationTests.resources.requests.cpu=200m;
integrationTests.resources.limits.memory=256Mi;
integrationTests.resources.limits.cpu=400m;
```

Click *Build*.