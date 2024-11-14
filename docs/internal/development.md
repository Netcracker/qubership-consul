# Development Guide

## Consul Commands (CLI)

Consul is controlled via a very easy to use command-line interface (CLI). Consul is only a single command-line application: `consul`. 
This application then takes a subcommand such as "agent", "kv" or "members" and provides abilities to configure your Consul cluster
and to get access to data.

For example, the following commands allow you to write and read data in Consul KV storage:

```bash
$ consul kv put redis/config/connections 5
Success! Data written to: redis/config/connections

$ consul kv get redis/config/connections
5
```

When authentication is enabled you need to set environment variable `CONSUL_HTTP_TOKEN` before using CLI tool.

To find full documentation regarding Consul CLI refer to [Consul Commands (CLI) Guide](https://www.consul.io/docs/commands/index.html).

## Consul Rest Api

The main interface to Consul is a REST HTTP API. 
The API can perform basic CRUD operations on nodes, services, checks, configuration, and more.

For example, the following requests allow you to write and read data in Consul KV storage:

```bash
$ curl \
    --request PUT \
    --data 'hello consul' \
    http://127.0.0.1:8500/v1/kv/my-key
```

Response:

```text
true
```

```bash
curl http://127.0.0.1:8500/v1/kv/my-key
```

Response:

```json
[
  {
    "CreateIndex": 100,
    "ModifyIndex": 200,
    "LockIndex": 200,
    "Key": "zip",
    "Flags": 0,
    "Value": "dGVzdA==",
    "Session": "adf4238a-882b-9ddc-4a9d-5b6758e4159e"
  }
]
```

When authentication is enabled, a Consul token should be provided to API requests using the `X-Consul-Token` header or with the `Bearer`
scheme in the authorization header. 

Below is an example using curl with `X-Consul-Token`.

```sh
$ curl \
    --header "X-Consul-Token: <consul token>" \
    http://127.0.0.1:8500/v1/agent/members
```
    
Below is an example using curl with `Bearer` scheme.

```sh
$ curl \
    --header "Authorization: Bearer <consul token>" \
    http://127.0.0.1:8500/v1/agent/members
```

To find full documentation regarding Consul HTTP API refer to [HTTP API Structure](https://www.consul.io/api/index.html)

## Registration Services

Components which work with Consul can be registered in Consul as services. 
The `services register` CLI command registers a service with the local agent. 
This command returns after registration and must be paired with explicit service de-registration. 
This command simplifies service registration from scripts, in dev mode, etc.

This is just one method of service registration. Services can also be registered by placing a service definition in
the Consul agent configuration directory and issuing a restart. 
This approach is easiest for configuration management systems that other systems that have access to the configuration directory. 
You can find documentation for service register in [Consul Agent Service Registration](https://www.consul.io/api/agent/service.html).

Clients may also use the [HTTP API](https://www.consul.io/api/agent/service.html) directly.

## ACL

Consul provides an optional Access Control List (ACL) system which can be used to control access to data and APIs. 
The ACL is Capability-based, relying on tokens which are associated with policies to determine which fine-grained rules can be applied. 
Consul's capability based ACL system is very similar to the design of AWS IAM.

The ACL system is designed to be easy to use and fast to enforce while providing administrative insight. At the highest level,
there are two major components to the ACL system:

* ACL Policies - Policies allow the grouping of a set of rules into a logical unit that can be reused and linked with many tokens.

* ACL Roles - Roles allow for the grouping of a set of policies and service identities into a reusable higher-level entity that
  can be applied to many tokens.

* ACL Tokens - Requests to Consul are authorized by using bearer token. Each ACL token has a public Accessor ID which is used to
  name a token, and a Secret ID which is used as the bearer token used to make requests to Consul.

* ACL Binding Rules - Binding rules allow an operator to express a systematic way of automatically linking roles and service identities to
  newly created tokens without operator intervention.

The main guide about ACL configuration is [ACL System](https://www.consul.io/docs/acl/acl-system.html).

When Consul ACL is enabled on the cluster you have to create policies for your services to access the Consul's features. 
Services also have to obtain token with corresponding rights (set of policies) for Consul to work with API. 

There are few ways to create policy and obtain token and the sections below describe how to do it using Consul CLI.

### Bootstrap Token

When you deploy Consul with enabled ACL the secret with consul admin bootstrap token is created in Consul server namespace.
For example, `consul-consul-bootstrap-acl-token`.

You can use value of this secret to access to restricted Consul API. If you use Consul CLI you need to set environment variable with
token as the follow:

```sh
export CONSUL_HTTP_TOKEN=xxxx-xxxx-xxx-xxxx-xxx
```

### Policies and Roles

An ACL policy is a named set of rules and is composed of the following elements:

* ID - The policy's auto-generated public identifier.
* Name - A unique meaningful name for the policy.
* Description - A human-readable description of the policy. (Optional)
* Rules - Set of rules granting or denying permissions. See the Rule Specification documentation for more details.
* Datacenters - A list of datacenters the policy is valid within.

Rules are composed of a resource, a segment (for some resource areas) and a policy disposition. The general structure of a rule is:

```text
<resource> "<segment>" {
  policy = "<policy disposition>"
}
```

Policy dispositions can have several control levels:

* `read`: allow the resource to be read but not modified.
* `write`: allow the resource to be read and modified.
* `deny`: do not allow the resource to be read or modified.
* `list`: allows access to all the keys under a segment in the Consul KV. 

To find more detailed information about rules please refer to [ACL Rules Guide](https://www.consul.io/docs/acl/acl-rules.html).

The below is example of Consul ACL policy for service Vault in JSON format:

```json
{
  "key_prefix": {
    "vault/": {
      "policy": "write"
    }
  },
  "node_prefix": {
    "": {
      "policy": "write"
    }
  },
  "service": {
    "vault": {
      "policy": "write"
    }
  },
  "agent_prefix": {
    "": {
      "policy": "write"
    }
  },
  "session_prefix": {
    "": {
      "policy": "write"
    }
  }
}
```

To create this policy on Consul you can use UI or Consul CLI. 
For CLI, you need to save your policy as file `rules.hcl` and to perform the following command:

```sh
$ consul acl policy create -name "new-policy" \
                         -description "This is an example policy" \
                         -datacenter "dc1" \
                         -rules @rules.hcl

06acc965
```

Roles allow for the grouping of a set of policies and service identities into a 
reusable higher-level entity that can be applied to many tokens. 

This is simple example how to create role for one policy:

```sh
$ consul acl role create -name "new-role" \
                       -description "This is an example role" \
                       -policy-id 06acc965
```

### Tokens

Requests to Consul are authorized by using bearer token. 
Each ACL token has a public Accessor ID which is used to name a token, and a Secret ID which is used as the bearer token used
to make requests to Consul.

In context of Consul tokens are permanent credentials which are used by service to access Consul features.

You can create token using Consul UI, HTTP API or Consul CLI.

This is simple example how to create token for policy:

```sh
  consul acl token create \
             -description "This is an example token" \
             -policy-id 06acc965
```

Then obtained token can be used for Consul CLI (via exporting `CONSUL_HTTP_TOKEN` variable) or in HTTP requests (via `X-Consul-Token` header).

### Service Login

Services which work with Consul should use their own token. Token can be created manually by operator how it is described above,
or by service using `login` API.

The `login` command will exchange the provided third party credentials with the requested auth method for a newly minted Consul ACL token. 
In Kubernetes environment services can log in to Consul with Kubernetes token which contains all necessary information about Service Account
and Consul can validate it.

This is an example how to log in to Consul using Kubernetes token:

```sh
$ consul login -method 'kubernetes' \
    -bearer-token-file '/run/secrets/kubernetes.io/serviceaccount/token' \
    -token-sink-file 'consul.token'

$ cat consul.token
36103ae4-6731-e719-f53a-d35188cfa41d
```

### ACL Bindings

Binding rules allow an operator to express a systematic way of automatically linking roles and service identities to newly created tokens
without operator intervention.

Successful authentication with an auth method returns a set of trusted identity attributes corresponding to the authenticated identity. 

Each binding rule is composed of two portions:

* Selector - A logical query that must match the trusted identity attributes for the binding rule to be applicable
  to a given login attempt. 
  The syntax uses [github.com/hashicorp/go-bexpr](http://github.com/hashicorp/go-bexpr) which is shared with the API filtering feature. 
  For example: "serviceaccount.namespace==default and serviceaccount.name!=vault"

* Bind Type and Name - A binding rule can bind a token to a role or to a service identity by name.
  The name can be specified with a plain string or the bind name can be lightly templated using HIL syntax to interpolate the same values that
  are usable by the Selector syntax. 
  For example: "dev-${serviceaccount.name}"

This is simple example how to create binding-rule for role and service account:

```sh
  consul acl binding-rule create \
        -method=kubernetes \
        -bind-type=role \
        -bind-name='new-role' \
        -selector='serviceaccount.namespace==vault-service and serviceaccount.name==vault'
```

To find more detailed information about rules please refer to [Binding Rules](https://www.consul.io/docs/acl/acl-auth-methods.html#binding-rules).
