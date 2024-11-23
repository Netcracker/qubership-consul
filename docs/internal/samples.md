This section provides samples of Helm Chart configurations for different Consul Deployments.

# Consul Minimal Viable Config

This section describes the deployment procedure of Consul with **the minimal set of parameters** by App Deployer with CMDB Plugin.

## Prerequisites

Ensure the prerequisites for OpenShift and node labels are met as mentioned in the sections below.

### OpenShift

The Consul project annotations should have the value `1000/1000` for the `openshift.io/sa.scc.supplemental-groups` and `100/1000`
for the `openshift.io/sa.scc.uid-range` parameters.

This is required because the Consul Service container starts under a user with `100` identifier.

### Node Labels

Choose labels for Kubernetes nodes where you can run Consul. For example:

* `"region": "database"` - For Consul servers

## Deployment with Cluster Wide Rights

1. Open CMDB Plugin for App Deployer and specify the parameters and their values. For example:

   ```yaml
   global:
     enabled: true
     
   server:
     enabled: true
     replicas: 3
     resources:
       requests:
         memory: "128Mi"
         cpu: "50m"
       limits:
         memory: "1024Mi"
         cpu: "400m"
     nodeSelector: {
       "region": "database"
     }
     storage: 1Gi
     storageClass: standard
     
   ui:
     enabled: true
     ingress:
       enabled: true
       hosts:
         - host: consul-consul-service.kubernetes.example.com
    
   ```

   For OpenShift installations you need to add the following parameters:

   ```yaml
   global:
     openshift:
       enabled: true
   ```

   **Where you need to change the following parameters**:

   * `server.nodeSelector` - The node selector to run Consul Server pods. By default, Consul has node anti-affinity to distribute across
     nodes.
   * `server.storageClass` - The storage class for Consul Server.
   * `ui.ingress.hosts` - The ingress hosts for Consul UI.

2. Open the deployment job, specify the latest version,
   and click **Build**.

## Deployment without Cluster Wide Rights

1. Create cluster-specific entities such as Custom Resource Definition, Pod Security Policies, Cluster Roles and Cluster Role Bindings.
   For more information see, [Deployment with Restricted Rights](/docs/public/restricted-rights.md).

   For example, 
   * You can use the following resources [restricted_consul.yaml](/docs/public/configs/restricted_consul.yaml:

   ```sh
   kubectl create -f restricted_consul.yaml
   ```

   * To create cluster specific entities on OpenShift use following resources [restricted_consul_openshift.yaml](/docs/public/configs/restricted_consul_openshift.yaml):

   ```sh
   kubectl create -f restricted_consul_openshift.yaml
   ```

   **NOTE:**  Pay attention on the namespace name and name of cluster entities.
   For example, `consul-acl-configurator-operator-consul-service` is built as `{global.name}-acl-configurator-operator-{namespace}`.
   If you use default `global.name` and deploy to namespace `consul-service` you can leave parameters as-is.

   Apply CRD [consul_acl_configurator_crd.yaml](/charts/helm/consul-service/crds/consul_acl_configurator_crd.yaml):
   
   ```sh
   kubectl replace -f consul_acl_configurator_crd.yaml
   ```

2. Open CMDB Plugin for App Deployer and specify the parameters and their values. You can use the example from the 
   [Deployment with Cluster Wide Rights](#deployment-with-cluster-wide-rights) section with few additional parameters:

   ```yaml
   DISABLE_CRD: true
   
   global:
     restrictedEnvironment: true
   ```

   **NOTE:** If you used `restricted_consul.yaml` or `restricted_consul_openshift.yaml` 
   when applying cluster resources, just leave the names as is.

3. Open the deployment job, specify the latest version and click **Build**.
