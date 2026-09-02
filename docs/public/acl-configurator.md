# Introduction

This section describes a contract between a client service and Consul ACL Configurator.

# Consul ACL Configurator Contract

To create/update Consul ACL policy, role or rule binding a client service should implement "consulacls" Kubernetes custom resource
For example,

```yaml
apiVersion: netcracker.com/v1alpha1
kind: ConsulACL
metadata:
  name: example-consul-acl-config
  namespace: vault-service
spec:
  acl:
    name: consul-acls
    json: >
      {
         "policies":[
            {
               "ID":"",
               "Name":"vault_operator_policy",
               "Description":"policy for using vault",
               "Rules":"acl=\"write\"",
               "Datacenters":[
                  "dc1"
               ]
            }
         ],
         "roles":[
            {
               "ID":"",
               "Name":"vault_operator_role",
               "Description":"role for using vault",
               "policy_names":[
                  "vault_operator_policy"
               ]
            }
         ],
         "bind_rules":[
            {
               "BindName":"vault_operator_role",
               "ServiceAccountName":"vault-account"
            }
         ]
      }
```

There are some required yaml fields

- `apiVersion` (netcracker.com/v1alpha1),
- `kind` (ConsulACL),
- `metadata.name` (any name but it should be unique for namespace "consulacls" CRs),
- `spec.acl.name` (any name),
- `spec.acl.json` (Consul ACL configuration json which satisfied a contract which described below).

## Configuration json

A configuration json (`spec.acl.json` yaml field) contains a json with 3 first level inner fields (policies, roles, bind_rules). All of these
fields can be absent and each one contains array of JSONs.

`Policy inner json`:

* `ID` - string, policy ID. Should be specified for "update" action, for "create" action can be absent.
* `Name` - string, policy unique name. A required field.
* `Description` - string, policy description. Can be absent.
* `Rules` - string which describe [Consul rule](https://www.consul.io/docs/acl/acl-rules). A required field.
   Note! you should escape inner quotes for example `acl=\"write\"`.
* `Datacenters` - array of strings which describes list of Consul data centers. Can be absent. Default value is `["dc1"]`.

`Role inner json`:

* `ID` - string, role ID. Should be specified for "update" action, for "create" action can be absent.
* `Name` - string, role unique name. A required field.
* `Description` - string, role description. Can be absent.
* `policy_names` - array of policy names which has been already specified in the `Policies` array. A required field.

`Rule Binding inner json`

* `BindName` - string, name of role. A required field.
* `ServiceAccountName` - string, name of Kubernetes service account of service which want to get token with binding rules. A required field.

`Rule Binding inner json explicit fields`
This fields will be set for any rule binding inner json.

* `AuthMethod` - string, Consul authentication method name. By default `<Consul service account>-k8s-auth-method`.
* `BindType` - string, type of bind entity. Value is "role".
* `Namespace` - string, name of Kubernetes namespace (OpenShift project) of service which want to get token with binding rule.
* `Selector` - string, selector for service account namespace and service account name. This field will be built from `Namespace` and
  `ServiceAccountName` with equal condition like this `serviceaccount.namespace==\"<ServiceAccountName>\" and serviceaccount.name==\"<Namespace>\"`.

## spec.acl.explicitName

By default the operator prefixes all entity names with `{crName}_{crNamespace}_` to avoid collisions between CRs from different namespaces.
Set `spec.acl.explicitName: true` to use the literal names from the configuration JSON as-is.

```yaml
spec:
  acl:
    name: consul-acls
    explicitName: true
    json: >
      {
        "roles": [{"Name": "my-role", ...}],
        "bind_rules": [{"BindName": "my-role", ...}]
      }
```

With `explicitName: false` (default) the role above is created as `{crName}_{crNamespace}_my-role`.
With `explicitName: true` it is created as `my-role`.

## Operator ownership via spec.acl.operatorNamespace

When multiple Consul ACL Configurator operators are deployed in different namespaces and all watch the cluster
(`WATCH_NAMESPACE=*`), use `spec.acl.operatorNamespace` to pin a CR to a specific operator instance.

The field is typically populated in the client's Helm chart by extracting the namespace from the Consul URL:

```yaml
spec:
  acl:
    name: consul-acls
    operatorNamespace: {{ (index (splitList "." (first (splitList ":" (last (splitList "://" .Values.CONSUL_URL))))) 1) | quote }}  # yamllint disable-line rule:braces
    json: >
      { ... }
```

For example, if `CONSUL_ADDRESS=http://consul-service-server.alty1224-consul-service.svc.cluster.local:8500`,
`operatorNamespace` resolves to `alty1224-consul-service`.

Only the operator deployed in that namespace will reconcile the CR. All other operators skip it at the informer
level — it never enters their reconcile queue.

**Rules:**

- `operatorNamespace` absent — all operators that watch the CR's namespace process it (existing behaviour, no change).
- `operatorNamespace` present — only the operator whose own namespace matches the value processes the CR.

# Custom resource lifecycle

Consul ACL Configurator uses namespaced CRD it means each CR has unique Kubernetes Namespace and CR name pair. After CR applied Consul ACL
configurator receives it and tries to load Consul ACLs. The result of processing will be stored in particular CR status.
Status field contains inner fields for policies, roles and rule binding statuses. If some error occurred during a process,
error message will be stored in the appropriate status field. For policy (role) the following flow implemented:
if policy (role) ID set - update action will be executed. If policy (role) ID is empty - Consul ACL Configurator checks
does mentioned policy (role) exist. If it exists - update action will be executed and create action will be executed in another way.
Anyway new Rule Binding will be created (not updated) during each reconcile circle.

# Common reconcile REST endpoint

There is a way to start common reconcile process by change each existed "consulacls" custom resource. To do it a service should send
GET HTTP request to Consul ACL Configurator Reconcile Kubernetes/OpenShift service `\reconcile` endpoint with Bearer token - current
service account token. For example,

```sh
curl consul-acl-configurator-reconcile/reconcile -H "Authorization: Bearer {token}" -H "Accept: application/json"
```

If current service namespace belongs to the allowed list common reconcile will be occurred.
`ALLOWED_NAMESPACES` environment variable defines a list of namespaces which have permissions to execute common reconcile. If this variable is empty
all namespaces have necessary permissions. This is a service based behavior. To start common reconcile manually we recommend scale down and then
scale up Consul ACL Configurator deployment.

# ConsulKV

The `ConsulKV` custom resource allows provisioning Consul KV entries. For example:

```yaml
apiVersion: netcracker.com/v1alpha1
kind: ConsulKV
metadata:
  name: example-consulkv
  namespace: my-service
spec:
  kv:
    entries:
      - key: "config/my-service/application/"
      - key: "config/my-service/LOG_LEVEL"
        value: "INFO"
```

Keys are created exactly as defined in the spec — no automatic prefixes are applied.

## KV ownership

The operator tracks ownership of KV keys using the Consul KV `Flags` field as a reference counter.
This allows multiple CRs to safely reference the same key — the key is only deleted when the last owner removes it.

**Ownership rules on apply:**

- Key does not exist → operator creates it with `Flags=1`. Key is **owned**; it will be deleted when the CR is removed.
- Key exists with `Flags=0` (created manually or by an external tool) → operator updates the value but does **not** claim ownership (`Flags` stays 0). The key will **not** be deleted when the CR is removed. The CR status for this entry will show `synced (not owned: pre-existing key)`.
- Key exists with `Flags>0` (owned by another CR) → operator increments `Flags`. Key is **co-owned**; it will only be deleted when all owning CRs are removed.

## spec.kv.purgeOnDelete

If `spec.kv.purgeOnDelete: true` is set, deleting the CR will recursively delete all Consul keys under each entry's key as a prefix, bypassing the ownership counter. Use this only when the CR owns an entire key namespace exclusively.

```yaml
spec:
  kv:
    purgeOnDelete: true
    entries:
      - key: "config/my-service/"
```
