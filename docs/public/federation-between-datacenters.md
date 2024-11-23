This guide provides information about how to federate multiple Kubernetes clusters.

For more information, refer to the 
[Federation Between Kubernetes Clusters](https://www.consul.io/docs/k8s/installation/multi-cluster/kubernetes).

Topics covered in this section:

- [Federation Between Kubernetes Clusters](#federation-between-kubernetes-clusters)
    - [Primary Datacenter](#primary-datacenter)
    - [Federation Secret](#federation-secret)
    - [Secondary Datacenter(s)](#secondary-datacenters)
- [Installation In One Kubernetes](#installation-in-one-kubernetes)
- [Installation In Multiple Kubernetes](#installation-in-multiple-kubernetes)

# Federation Between Kubernetes Clusters

There are several important steps to configure the multiple Consul datacenters with federation between them:

- Install `primary` Consul datacenter with necessary security options (TLS, ACL, gossip encryption).
- Import federation secret to Kubernetes namespace(s) where `secondary` datacenter(s) is(are) going to be installed.
- Install `secondary` Consul datacenter(s) with the same security options as the `primary` datacenter.

**Important**: Installation in IPv6 environment is currently not supported.

Below we are going to talk about these steps in more detail.

## Primary Datacenter

Consul treats each Kubernetes cluster as a separate Consul datacenter. In order to federate clusters, one cluster must
be designated the `primary` datacenter. This datacenter will be responsible for creating the certificate authority
that signs the TLS certificates Connect uses to encrypt and authorize traffic. It also handles validating global ACL
tokens. All other clusters that are federated are considered `secondaries`.

You need to use the following configuration for your primary cluster, with the possible modifications listed below.

```yaml
global:
  datacenter: dc1

  # TLS configures whether Consul components use TLS.
  tls:
    # TLS must be enabled for federation in Kubernetes.
    enabled: true

  acls:
    manageSystemACLs: true
    # If ACLs are enabled, we must create a token for secondary
    # datacenters to replicate ACLs.
    createReplicationToken: true

  federation:
    enabled: true
    # This will cause a Kubernetes secret to be created that
    # can be imported by secondary datacenters to configure them
    # for federation.
    createFederationSecret: true

  # Gossip encryption secures the protocol Consul uses to quickly
  # discover new nodes and detect failure.
  gossipEncryption:
    secretName: consul-gossip-encryption-key
    secretKey: key

connectInject:
  # Consul Connect service mesh must be enabled for federation.
  enabled: true

meshGateway:
  # Mesh gateways are gateways between datacenters. They must be enabled
  # for federation in Kubernetes since the communication between datacenters
  # goes through the mesh gateways.
  enabled: true
```

Possible modifications are as follows:

1. The Consul datacenter name is `dc1`. The datacenter name in each federated cluster **must be unique**.

2. ACLs are enabled in the above config file. They can be disabled by setting:

    ```yaml
    global:
      acls:
        manageSystemACLs: false
        createReplicationToken: false
    ```

   ACLs secure Consul by requiring every API call to present an ACL token that is validated to ensure it has the proper
   permissions. This is not required only when testing Consul.

3. Gossip encryption is enabled in the above config file. To disable it, comment out or delete the `gossipEncryption` key:

    ```yaml
    global:
      # gossipEncryption:
      #   secretName: consul-gossip-encryption-key
      #   secretKey: key
    ```

   Gossip encryption encrypts the communication layer used to discover other nodes in the cluster and report on failure.
   If you are only testing Consul, this is not required.

   **Note:** This config assumes you've already created a Kubernetes secret called `consul-gossip-encryption-key`.
   See `gossipEncryption` parameter description in the [Global Values](/docs/public/installation.md#global) section 
  for more information on how to create this secret.

4. The default mesh gateway configuration creates a Kubernetes `ClusterIP` service. It is used for federation within one
   Kubernetes. If you wish to install Consul in multiple Kubernetes, you need to use a `NodePort` or `LoadBalancer` service
   type with `meshGateway.wanAddress.source` set to `Service` or `Static` depending on your environment. For example,
   for `NodePort` service you can use the following configuration:

    ```yaml
    meshGateway:
      enabled: true
      wanAddress:
        source: Service
      service:
        enabled: true
        type: NodePort
        nodePort: 31565
    ```

   For more information, see the [Mesh Gateway](/docs/public/installation.md#mesh-gateway).

When you have completed the configuration for the `primary` datacenter, go to the 
[Installation](/docs/public/installation.md#installation) section to install Consul on your `primary` cluster.

## Federation Secret

The federation secret is a Kubernetes secret containing information needed for `secondary` datacenters/clusters
to federate with the primary. This secret is created automatically by the following setting:

```yaml
global:
  federation:
    createFederationSecret: true
```

After the installation of the `primary` datacenter you need to export this secret using the following command:

```sh
kubectl get secret ${CONSUL_FEDERATION_SECRET} -n ${NAMESPACE} -o yaml > consul-federation-secret.yaml
```

where:

  * `${CONSUL_FEDERATION_SECRET}` is the name of the federation secret. For example, `consul-cluster-consul-federation`.
  * `${NAMESPACE}` is the Kubernetes namespace where `primary` datacenter of Consul is deployed. For example, `consul-cluster`.

**Note**: Do not forget to remove all `metadata` fields in the stored secret except `name` so that it looks like this:

```yaml
apiVersion: v1
data:
  caCert: <ca_cert>
  caKey: <ca_key>
  gossipEncryptionKey: <gossip_encryption_key>
  replicationToken: <replication_token>
  serverConfigJSON: <server_config_json>
kind: Secret
metadata:
  name: consul-cluster-consul-federation
type: Opaque
```

For more information about the content of the federation secret, see the
[Consul Federation](/docs/public/installation.md#federation) section.

**Important**: If during `upgrade` process you change parameters for mesh gateways, you need to wait until all services
go to the `ready` status and run the `upgrade` again with the same parameters for the correct formation of the `federation`
secret.

**Security note**: The federation secret makes it possible to gain full admin privileges in Consul. This secret must be
kept securely, i.e., it should be deleted from your filesystem after importing it to your `secondary` cluster. You
should use RBAC permissions to ensure only administrators can read the secret from Kubernetes.

Now you can import the secret into your `secondary` cluster(s).

Switch `kubectl` context to your `secondary` Kubernetes cluster and import the secret:

```sh
$ kubectl apply -f consul-federation-secret.yaml -n ${NAMESPACE}
secret/consul-cluster-consul-federation created
```

where `${NAMESPACE}` is the Kubernetes namespace to deploy `secondary` datacenter of Consul. For example, `consul-service`.

## Secondary Datacenter(s)

With the `primary` cluster up and running, and the federation secret imported into the `secondary` cluster, you can
install Consul into the `secondary` cluster.

You need to use the following configuration for your `secondary` cluster(s), with the possible modifications listed below.

```yaml
global:
  datacenter: dc2

  tls:
    enabled: true

    # Here we're using the shared certificate authority from the primary
    # datacenter that was exported via the federation secret.
    caCert:
      secretName: consul-federation
      secretKey: caCert
    caKey:
      secretName: consul-federation
      secretKey: caKey

  acls:
    manageSystemACLs: true

    # Here we're importing the replication token that was
    # exported from the primary via the federation secret.
    replicationToken:
      secretName: consul-federation
      secretKey: replicationToken

  federation:
    enabled: true
    primaryDatacenter: dc1
    k8sAuthMethodHost: https://k8s-2.openshift.sdntest.example.com:6443

  gossipEncryption:
    secretName: consul-federation
    secretKey: gossipEncryptionKey

server:
  # Here we're including the server config exported from the primary
  # via the federation secret. This config includes the addresses of
  # the primary datacenter's mesh gateways so Consul can begin federation.
  extraVolumes:
    - type: secret
      name: consul-federation
      items:
        - key: serverConfigJSON
          path: config.json
      load: true

connectInject:
  enabled: true

meshGateway:
  enabled: true
```

Possible modifications are as follows:

1. If ACLs are enabled, change the value of `global.federation.k8sAuthMethodHost` to the full URL (including `https://`)
   of the secondary cluster's Kubernetes API.

2. `global.federation.primaryDatacenter` must be set to the name of the primary datacenter.

3. The Consul datacenter name is `dc2`. The `primary` datacenter's name was `dc1`. The datacenter name in each federated
cluster **must be unique**.

4. ACLs are enabled in the above config file. They can be disabled by removing the whole ACLs block:

    ```yaml
    acls:
      manageSystemACLs: false
      replicationToken:
        secretName: consul-federation
        secretKey: replicationToken
    ```

   If ACLs are enabled in one datacenter, they must be enabled in all datacenters because in order to communicate with
   that one datacenter ACL tokens are required.

5. Gossip encryption is enabled in the above config file. To disable it, don't set the `gossipEncryption` key:

    ```yaml
    global:
      # gossipEncryption:
      # secretName: consul-federation
      # secretKey: gossipEncryptionKey
    ```

   If gossip encryption is enabled in one datacenter, it must be enabled in all datacenters because in order to
   communicate with that one datacenter the encryption key is required.

6. The default mesh gateway configuration creates a Kubernetes `ClusterIP` service. It is used for federation within one
   Kubernetes. If you wish to install Consul in multiple Kubernetes, you need to use a `NodePort` or `LoadBalancer` service
   type with `meshGateway.wanAddress.source` set to `Service` or `Static` depending on your environment. For example,
   for `NodePort` service you can use the following configuration:

    ```yaml
    meshGateway:
      enabled: true
      wanAddress:
        source: Service
      service:
        enabled: true
        type: NodePort
        nodePort: 31565
    ```

   For more information, see the [Mesh Gateway](/docs/public/installation.md#mesh-gateway).

When you have completed the configuration for the `secondary` datacenter(s), go to the 
[Installation](/docs/public/installation.md#installation) section to install Consul on your `secondary` cluster(s).

# Installation In One Kubernetes

The installation of multiple Consul datacenters in one Kubernetes has several distinctive features:

* There is no need to use external addresses for `Mesh Gateway`, because they are all in one Kubernetes.
* It is necessary to pay attention to `global.name` parameter value that is used in generating names for all Consul
  resources. It must be unique, because some Consul resources are created for the entire cloud, not specific namespace.
* It is required to separate Kubernetes nodes that are used by clients of different datacenters.

To install `primary` datacenter with ACLs, use the following configuration:

```yaml
global:
  enabled: true
  name: consul-primary
  domain: consul
  datacenter: dc1
  gossipEncryption:
    secretName: consul-gossip-encryption-key
    secretKey: key
  tls:
    enabled: true
  acls:
    manageSystemACLs: true
    createReplicationToken: true
  federation:
    enabled: true
    createFederationSecret: true

server:
  enabled: "-"
  replicas: 3
  storage: 1Gi
  storageClass: standard
  connect: true
  nodeSelector: {
    "node-role.kubernetes.io/compute": "true"
    "site": "left"
  }
  resources:
    requests:
      memory: "128Mi"
      cpu: "50m"
    limits:
      memory: "1024Mi"
      cpu: "400m"

dns:
  enabled: "-"

ui:
  enabled: true
  ingress:
    enabled: true
    hosts:
      - host: consul-consul-cluster.search.example.com
  service:
    enabled: true

connectInject:
  enabled: true
  nodeSelector: {
    "node-role.kubernetes.io/compute": "true"
    "site": "left"
  }

meshGateway:
  enabled: true
  wanAddress:
    source: Service
  service:
    enabled: true
    type: ClusterIP
    port: 31565
  nodeSelector: {
    "node-role.kubernetes.io/compute": "true"
    "site": "left"
  }

monitoring:
  enabled: true
  resources:
    requests:
      memory: "64Mi"
      cpu: "15m"
    limits:
      memory: "128Mi"
      cpu: "100m"
  consulExecPluginInterval: "30s"
  consulExecPluginTimeout: "20s"
  monitoringType: "prometheus"
  installDashboard: true
  consulScriptDebug: ""

consulAclConfigurator:
  enabled: true
  reconcilePeriod: 100
  namespaces: ""
  serviceName: "consul-acl-configurator-reconcile"
  consul:
    port: 8500
```

After completing the installation of `primary` Consul datacenter you can find the federation secret in its namespace
by `<global.name>-federation` name. For example, with the above configuration the secret name of federation is
`consul-primary-federation`.

To install `secondary` datacenter, use the following configuration:

```yaml
global:
  enabled: true
  name: consul-dc2
  domain: consul
  datacenter: dc2
  gossipEncryption:
    secretName: consul-primary-federation
    secretKey: gossipEncryptionKey
  tls:
    enabled: true
    caCert:
      secretName: consul-primary-federation
      secretKey: caCert
    caKey:
      secretName: consul-primary-federation
      secretKey: caKey
  acls:
    manageSystemACLs: true
    replicationToken:
      secretName: consul-primary-federation
      secretKey: replicationToken
  federation:
    enabled: true
    primaryDatacenter: dc1
    k8sAuthMethodHost: https://k8s-2.openshift.sdntest.example.com:6443

server:
  enabled: "-"
  replicas: 3
  storage: 1Gi
  storageClass: standard
  connect: true
  extraVolumes: [
    {
      "type": "secret",
      "name": "consul-primary-federation",
      "items": [
        {
          "key": "serverConfigJSON",
          "path": "config.json"
        }
      ],
      "load": true
    }
  ]
  nodeSelector: {
    "node-role.kubernetes.io/compute": "true"
    "site": "right"
  }
  resources:
    requests:
      memory: "128Mi"
      cpu: "50m"
    limits:
      memory: "1024Mi"
      cpu: "400m"

dns:
  enabled: "-"

ui:
  enabled: true
  ingress:
    enabled: true
    hosts:
      - host: consul-consul-service.search.example.com
  service:
    enabled: true

connectInject:
  enabled: true
  nodeSelector: {
    "node-role.kubernetes.io/compute": "true"
    "site": "right"
  }

meshGateway:
  enabled: true
  wanAddress:
    source: Service
  service:
    enabled: true
    type: ClusterIP
    port: 31565
  nodeSelector: {
    "node-role.kubernetes.io/compute": "true"
    "site": "right"
  }

monitoring:
  enabled: true
  resources:
    requests:
      memory: "64Mi"
      cpu: "15m"
    limits:
      memory: "128Mi"
      cpu: "100m"
  consulExecPluginInterval: "30s"
  consulExecPluginTimeout: "20s"
  monitoringType: "prometheus"
  installDashboard: true
  consulScriptDebug: ""

consulAclConfigurator:
  enabled: true
  reconcilePeriod: 100
  namespaces: ""
  serviceName: "consul-acl-configurator-reconcile"
  consul:
    port: 8500
```

When creating several `secondary` Consul datacenter do not forget to change `global.name`, `global.datacenter` and
`nodeSelector` parameters in the above configuration.
More information about `primary` and `secondary` datacenters you can find in 
[Federation Between Kubernetes Clusters](#federation-between-kubernetes-clusters) section.

For more details about deploying Consul on Kubernetes, see the [Installation](/docs/public/installation.md#installation) section.

# Installation In Multiple Kubernetes

The installation of multiple Consul datacenters in multiple Kubernetes has several distinctive features:

* It is required to use external addresses for `Mesh Gateways`, because they should communicate from different
  OpenShift/Kubernetes. If you use OpenShift/Kubernetes node IPs for external access, it is necessary to make sure, that
  node IPs presented in OpenShift/Kubernetes UI are external. To check that, run the following command outside the
  OpenShift/Kubernetes:

  ```sh
  ping <node_ip>
  ```

  where `<node_ip>` is the IP address of the OpenShift/Kubernetes node.

  If the result of performing the above command is successful (no packets are lost), the IP address of OpenShift/Kubernetes
  node is external, and you can use the following configuration for `Mesh Gateway`:

  ```yaml
  meshGateway:
  enabled: true
  wanAddress:
    source: Service
  service:
    enabled: true
    type: NodePort
    nodePort: 31565
  ```

  Otherwise, the IP address of the OpenShift/Kubernetes node is not external. So, you need to find out the public IP
  of the node and use the following configuration for `Mesh Gateway`:

  ```yaml
  meshGateway:
    enabled: true
    replicas: 1
    wanAddress:
      source: Static
      port: 31565
      static: <public_node_ip>
    service:
      enabled: true
      type: NodePort
      nodePort: 31565
    nodeSelector:
      <node_label_key>: <node_label_value>
  ```

  where:

  * `<public_node_ip>` is the public (external) IP of the specific OpenShift/Kubernetes node.
  * `<node_label_key>` and `<node_label_value>` are key and value of the label that uniquely identifies the node which IP
    is specified in `<public_node_ip>` parameter. For example, for label `kubernetes.io/hostname=node-1`
    `<node_label_key>` is `kubernetes.io/hostname`, `<node_label_value>` is `node-1`.

  The configurations below contain examples of `Mesh Gateways` with `NodePort` services.

* The `global.name` parameter value can be any, because Consul datacenters are located in different environments.
  By default, `global.name` parameter is set to `consul`.
* There is no need to worry about used Kubernetes nodes, but you can still specify (limit) which of them are used by clients.

To install `primary` datacenter with ACLs, use the following configuration:

```yaml
global:
  enabled: true
  domain: consul
  datacenter: dc1
  gossipEncryption:
    secretName: consul-gossip-encryption-key
    secretKey: key
  tls:
    enabled: true
  acls:
    manageSystemACLs: true
    createReplicationToken: true
  federation:
    enabled: true
    createFederationSecret: true

server:
  enabled: "-"
  replicas: 3
  storage: 1Gi
  storageClass: standard
  connect: true
  resources:
    requests:
      memory: "128Mi"
      cpu: "50m"
    limits:
      memory: "1024Mi"
      cpu: "400m"

dns:
  enabled: "-"

ui:
  enabled: true
  ingress:
    enabled: true
    hosts:
      - host: consul-consul-cluster.search.example.com
  service:
    enabled: true

connectInject:
  enabled: true

meshGateway:
  enabled: true
  wanAddress:
    source: Service
  service:
    enabled: true
    type: NodePort
    nodePort: 31565

monitoring:
  enabled: true
  resources:
    requests:
      memory: "64Mi"
      cpu: "15m"
    limits:
      memory: "128Mi"
      cpu: "100m"
  consulExecPluginInterval: "30s"
  consulExecPluginTimeout: "20s"
  monitoringType: "prometheus"
  installDashboard: true
  consulScriptDebug: ""

consulAclConfigurator:
  enabled: true
  reconcilePeriod: 100
  namespaces: ""
  serviceName: "consul-acl-configurator-reconcile"
  consul:
    port: 8500
```

After completing the installation of `primary` Consul datacenter you can find the federation secret in its namespace
by `<global.name>-federation` name. For example, with the above configuration the secret name of federation is
`consul-federation`.

To install `secondary` datacenter, use the following configuration:

```yaml
global:
  enabled: true
  domain: consul
  datacenter: dc2
  gossipEncryption:
    secretName: consul-federation
    secretKey: gossipEncryptionKey
  tls:
    enabled: true
    caCert:
      secretName: consul-federation
      secretKey: caCert
    caKey:
      secretName: consul-federation
      secretKey: caKey
  acls:
    manageSystemACLs: true
    replicationToken:
      secretName: consul-federation
      secretKey: replicationToken
  federation:
    enabled: true
    primaryDatacenter: dc1
    k8sAuthMethodHost: https://k8s-2.openshift.sdntest.example.com:6443

server:
  enabled: "-"
  replicas: 3
  storage: 1Gi
  storageClass: standard
  connect: true
  extraVolumes: [
    {
      "type": "secret",
      "name": "consul-federation",
      "items": [
        {
          "key": "serverConfigJSON",
          "path": "config.json"
        }
      ],
      "load": true
    }
  ]
  resources:
    requests:
      memory: "128Mi"
      cpu: "50m"
    limits:
      memory: "1024Mi"
      cpu: "400m"

dns:
  enabled: "-"

ui:
  enabled: true
  ingress:
    enabled: true
    hosts:
      - host: consul-consul-service.search.example.com
  service:
    enabled: true

connectInject:
  enabled: true

meshGateway:
  enabled: true
  wanAddress:
    source: Service
  service:
    enabled: true
    type: NodePort
    nodePort: 31565

monitoring:
  enabled: true
  resources:
    requests:
      memory: "64Mi"
      cpu: "15m"
    limits:
      memory: "128Mi"
      cpu: "100m"
  consulExecPluginInterval: "30s"
  consulExecPluginTimeout: "20s"
  monitoringType: "prometheus"
  installDashboard: true
  consulScriptDebug: ""

consulAclConfigurator:
  enabled: true
  reconcilePeriod: 100
  namespaces: ""
  serviceName: "consul-acl-configurator-reconcile"
  consul:
    port: 8500
```

When creating several `secondary` Consul datacenters do not forget to change `global.datacenter` in the above configuration.
More information about `primary` and `secondary` datacenters you can find in 
[Federation Between Kubernetes Clusters](#federation-between-kubernetes-clusters) section.

For more details about deploying Consul on Kubernetes, see the [Installation](/docs/public/installation.md#installation) section.
