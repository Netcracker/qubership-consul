#Introduction

This section describes a contract between a client service and Consul ACL Configurator.

#Consul ACL Configurator Contract

To create/update Consul ACL policy, role or rule binding a client service should implement "consulacls" Kubernetes custom resource
For example,
```yaml
apiVersion: qubership.org/v1alpha1
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
- `apiVersion` (qubership.org/v1alpha1), 
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

#Custom resource lifecycle

Consul ACL Configurator uses namespaced CRD it means each CR has unique Kubernetes Namespace and CR name pair. After CR applied Consul ACL 
configurator receives it and tries to load Consul ACLs. The result of processing will be stored in particular CR status. 
Status field contains inner fields for policies, roles and rule binding statuses. If some error occurred during a process, 
error message will be stored in the appropriate status field. For policy (role) the following flow implemented:
if policy (role) ID set - update action will be executed. If policy (role) ID is empty - Consul ACL Configurator checks 
does mentioned policy (role) exist. If it exists - update action will be executed and create action will be executed in another way. 
Anyway new Rule Binding will be created (not updated) during each reconcile circle.       

#Common reconcile REST endpoint

There is a way to start common reconcile process by change each existed "consulacls" custom resource. To do it a service should send
GET HTTP request to Consul ACL Configurator Reconcile Kubernetes/OpenShift service `\reconcile` endpoint with Bearer token - current 
service account token. For example,
```
curl consul-acl-configurator-reconcile/reconcile -H "Authorization: Bearer {token}" -H "Accept: application/json"
``` 
If current service namespace belongs to the allowed list common reconcile will be occurred. 
`ALLOWED_NAMESPACES` environment variable defines a list of namespaces which have permissions to execute common reconcile. If this variable is empty
all namespaces have necessary permissions. This is a service based behavior. To start common reconcile manually we recommend scale down and then 
scale up Consul ACL Configurator deployment. 