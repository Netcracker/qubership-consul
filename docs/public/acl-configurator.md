# Consul ACL Configurator

## Introduction

This section describes a contract between a client service and Consul ACL Configurator.

## Contract

To create/update Consul ACL policy, role or rule binding a client service should implement the "consulacls"
Kubernetes custom resource.
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

### Configuration json

A configuration json (`spec.acl.json` yaml field) contains a json with 3 first level inner fields
(policies, roles, bind_rules). All of these fields can be absent and each one contains array of JSONs.

`Policy inner json`:

- `ID` - string, policy ID. Should be specified for "update" action, for "create" action can be absent.
- `Name` - string, policy unique name. A required field.
- `Description` - string, policy description. Can be absent.
- `Rules` - string which describe [Consul rule](https://www.consul.io/docs/acl/acl-rules). A required field.
  Note! you should escape inner quotes for example `acl=\"write\"`.
- `Datacenters` - array of strings which describes list of Consul data centers. Can be absent. Default value is
  `["dc1"]`.

`Role inner json`:

- `ID` - string, role ID. Should be specified for "update" action, for "create" action can be absent.
- `Name` - string, role unique name. A required field.
- `Description` - string, role description. Can be absent.
- `policy_names` - array of policy names which has been already specified in the `Policies` array. A required field.

`Rule Binding inner json`

- `BindName` - string, name of role. A required field.
- `ServiceAccountName` - string, name of Kubernetes service account of service which want to get token with
  binding rules. Used to build `Selector` when `Selector` is not set.
- `Description` - string, binding rule description. Can be absent.
- `AuthMethod` - string, Consul authentication method of the rule. Can be absent. By default the global
  JWT auth method `applications-k8s-m2m` is used (see [Authentication method](#authentication-method)).
- `Selector` - string, Consul selector of the rule. Can be absent. When it is set, it is passed to Consul as is and
  `ServiceAccountName` is not used for the selector.

`Rule Binding inner json explicit fields`
This fields will be set for any rule binding inner json.

- `BindType` - string, type of bind entity. Value is "role".
- `Selector` - when it is not set in the configuration, it is built from the namespace of the custom resource and
  `ServiceAccountName`. The form depends on the type of the auth method of the rule:
  - `jwt` (the default `applications-k8s-m2m`): `value.namespace == "<CR namespace>" and value.serviceaccount == "<ServiceAccountName>"`;
  - `kubernetes`: `serviceaccount.namespace == "<CR namespace>" and serviceaccount.name == "<ServiceAccountName>"`.

  If `ServiceAccountName` is a template such as `${value.serviceaccount}`, no selector is generated and the rule
  matches every login through its auth method.

### spec.acl.explicitName

By default the operator prefixes all entity names with `{crName}_{crNamespace}_` to avoid collisions between
CRs from different namespaces.
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

#### Shared entities

With `explicitName: true` several CRs can declare the same policy, role or binding rule. The operator records
the namespaces of the owning CRs at the end of the entity `Description`:

```text
my description
[consul-acl-owners: ns1, ns2]
```

When a CR is deleted, or an entity is removed from its configuration, the operator removes the CR namespace from
this list. The entity is deleted, and the tokens of a role revoked, only when no other owners remain. An entity
that is still declared by another CR of the same namespace is not changed. Entities created before the owner list
was introduced have no marker and are deleted together with the CR as before.

Do not edit the `[consul-acl-owners: ...]` line manually: the operator uses it to decide when an entity can be deleted.

### Operator ownership via spec.acl.operatorNamespace

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

For example, if `CONSUL_ADDRESS=http://consul-server.consul-service.svc.cluster.local:8500`,
`operatorNamespace` resolves to `consul-service`.

Only the operator deployed in that namespace will reconcile the CR. All other operators skip it at the informer
level — it never enters their reconcile queue.

**Rules:**

- `operatorNamespace` absent — the CR is processed only by the operator deployed in the same namespace as the CR.
  Operators in other namespaces ignore it, even if they watch the CR namespace. A CR created in a namespace without
  an operator is therefore not processed until `operatorNamespace` is set.
- `operatorNamespace` present — only the operator whose own namespace matches the value processes the CR, wherever
  the CR is created. The operator must watch the CR namespace (`consulAclConfigurator.namespaces`, `*` for all).

The same rules apply to `spec.kv.operatorNamespace` of [ConsulKV](#consulkv) resources.

## Authentication method

Binding rules are created under the global JWT auth method `applications-k8s-m2m`, unless a rule sets its own
`AuthMethod`. The operator creates the method at start, or updates it when its configuration differs, and retries
with exponential backoff while Consul or the JWKS proxy is not available. The method is not written when the
configuration is unchanged.

The method has the following configuration:

- `JWKSURL` - the JWKS proxy deployed by the chart, `http://<fullname>-acl-configurator-jwks-proxy:8080/openid/v1/jwks`.
  The proxy exposes only `/openid/v1/jwks` and `/.well-known/openid-configuration` of the Kubernetes API server
  and rejects all methods except `GET`.
- `ClaimMappings` - `/kubernetes.io/namespace` to `namespace` and `/kubernetes.io/serviceaccount/name` to
  `serviceaccount`, available in selectors as `value.namespace` and `value.serviceaccount`.
- `BoundIssuer` and `BoundAudiences` - see below.

Services log in with their service account token (a projected token with the cluster default audience works), for
example `consul login -method=applications-k8s-m2m -bearer-token-file=/var/run/secrets/kubernetes.io/serviceaccount/token`.

### BoundIssuer and BoundAudiences

By default the operator reads the `issuer` field of `/.well-known/openid-configuration` served by the JWKS proxy,
that is the `--service-account-issuer` of the cluster, and uses it as `BoundIssuer`. `BoundAudiences` defaults to the
same value, because the default `--api-audiences` of the API server is equal to the issuer.

When the audience of service account tokens differs from the issuer, or the issuer must be fixed, set
`consulAclConfigurator.boundIssuer` and `consulAclConfigurator.boundAudiences` in the deployment parameters.
Explicit values take precedence over detection; when only `boundAudiences` is set, the issuer is still detected.

To check the issuer of the cluster manually, run:

```bash
kubectl get --raw /.well-known/openid-configuration
```

If the issuer can not be read and `boundIssuer` is not set, the operator does not fall back to a fixed value: it keeps
retrying and logs `failed to ensure applications-k8s-m2m auth method, retrying`. Check that the JWKS proxy pods
are ready and, when `consulAclConfigurator.jwksProxy.networkPolicy.enabled` is `true`, that the network policy
allows the operator pod.

### Upgrade from the Kubernetes auth method

Earlier versions created binding rules under `<fullname>-k8s-auth-method`. After the upgrade the operator creates
and updates rules only under `applications-k8s-m2m`; rules under the old method are left untouched and are not
deleted together with the custom resources. Upgrade steps:

1. Upgrade the chart. The operator creates `applications-k8s-m2m` and, on the next reconcile of each custom
   resource, the binding rules under it.
2. Switch the client services to log in through `applications-k8s-m2m` with their service account token.
   Until then they keep using the old rules.
3. Remove the old rules that were created by the operator. They have `BindType` `role` and the selector built from
   `ServiceAccountName`, for example:

   ```bash
   consul acl binding-rule list -method=<fullname>-k8s-auth-method
   consul acl binding-rule delete -id=<rule ID>
   ```

   Do not remove the rules created by `server-acl-init` (`BindType` `service`), they are used by the service mesh.

The service mesh is not affected: `connect-inject` and the other Consul components keep logging in through
`<fullname>-k8s-auth-method` and `<fullname>-k8s-component-auth-method`, which are managed by `server-acl-init`.
After a restore from backup these Kubernetes auth methods are reconfigured by the backup daemon; `applications-k8s-m2m`
is brought in line with the cluster by the operator, which is restarted after the restore.

## Custom resource lifecycle

Consul ACL Configurator uses namespaced CRD it means each CR has unique Kubernetes Namespace and CR name pair.
After CR applied Consul ACL configurator receives it and tries to load Consul ACLs. The result of processing
will be stored in particular CR status.
Status field contains inner fields for policies, roles and rule binding statuses. If some error occurred during
a process, error message will be stored in the appropriate status field. For policy (role) the following flow
implemented: if policy (role) ID set - update action will be executed. If policy (role) ID is empty - Consul ACL
Configurator checks does mentioned policy (role) exist. If it exists - update action will be executed and create
action will be executed in another way.
Anyway new Rule Binding will be created (not updated) during each reconcile circle.

## Common reconcile REST endpoint

There is a way to start common reconcile process by change each existed "consulacls" custom resource. To do it
a service should send GET HTTP request to Consul ACL Configurator Reconcile Kubernetes/OpenShift service
`\reconcile` endpoint with Bearer token - current service account token. For example,

```sh
curl consul-acl-configurator-reconcile/reconcile -H "Authorization: Bearer {token}" -H "Accept: application/json"
```

If current service namespace belongs to the allowed list common reconcile will be occurred.
`ALLOWED_NAMESPACES` environment variable defines a list of namespaces which have permissions to execute common
reconcile. If this variable is empty all namespaces have necessary permissions. This is a service based behavior.
To start common reconcile manually we recommend scale down and then scale up Consul ACL Configurator deployment.

## ConsulKV

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

### KV ownership

The operator tracks ownership of KV keys using the Consul KV `Flags` field as a reference counter.
This allows multiple CRs to safely reference the same key — the key is only deleted when the last owner removes it.

**Ownership rules on apply:**

- Key does not exist → operator creates it with `Flags=1`. Key is **owned**; it will be deleted when the CR is
  removed.
- Key exists with `Flags=0` (created manually or by an external tool) → operator updates the value but does
  **not** claim ownership (`Flags` stays 0). The key will **not** be deleted when the CR is removed. The CR status
  for this entry will show `synced (not owned: pre-existing key)`.
- Key exists with `Flags>0` (owned by another CR) → operator increments `Flags`. Key is **co-owned**; it will
  only be deleted when all owning CRs are removed.

If the same key is declared more than once in one CR, the key is written once with the value of the last entry.
The earlier entries are reported in the CR status as `skipped (duplicate key)`; all other keys are written normally.

Keys are written and released in Consul transactions of at most 64 operations. Each transaction is atomic, the
operation as a whole is not: if a later transaction fails, the keys of the committed transactions are recorded in
the CR status as written (or released), only the keys of the failed transaction get an error status, and the
operator retries them after `consulAclConfigurator.reconcilePeriod`. The CR becomes `synced` only when all keys
are written.

### spec.kv.purgeOnDelete

If `spec.kv.purgeOnDelete: true` is set, deleting the CR removes each declared key and everything below it,
bypassing the ownership counter: also keys used by other CRs are removed. Use this only when the CR owns an
entire key namespace exclusively.

Each declared key is treated as a directory: the operator deletes the exact key and the tree `<key>/`. For example,
for `config/my-service` it deletes `config/my-service` and `config/my-service/...`, but not
`config/my-service-gateway/...`.

```yaml
spec:
  kv:
    purgeOnDelete: true
    entries:
      - key: "config/my-service/"
```
