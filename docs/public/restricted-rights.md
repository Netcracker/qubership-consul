# Pod Security Policies

Some Consul features require privileged rights for pods. For example, the Client pods need to open port `8502` on Kubernetes node.
For this you may need to enable `enablePodSecurityPolicies` which enables the Helm Chart to create the `PodSecurityPolicy` objects.

If you deploy Consul Service without `cluster-wide` rights, you need to create necessary pod security policies manually with privileged
user before installation via Helm Chart.

All necessary pod security policies can be automatically rendered with [Automatic Yaml Building](#automatic-yaml-building) section 
and then created manually.

Also, you need to specify names of corresponding created Pod Security Policies to Helm Chart configuration (`values.yaml`) 
with `podSecurityPolicy` parameter.

# Cluster Entities

If you deploy Consul Service without `cluster-wide` rights you need to create necessary Cluster Roles, Cluster Role Bindings 
and Security Context Constraints with privileged user before installation by Helm Chart. 
You need to create these resources only for services which you want to install with Helm Chart.

All necessary cluster roles, cluster role bindings and security context constrains can be automatically rendered with 
[Automatic Yaml Building](#automatic-yaml-building) section and then created manually.

# Automatic Yaml Building

Before you deploy Consul Service without `cluster-wide` rights, you need to create necessary cluster roles, cluster role bindings, 
security context constrains and pod security policies on your Kubernetes with privileged user.
You can build yaml files with necessary resources for your deployment using the `helm template` tool.

Specify the parameter values in the `values.yaml` file with necessary configurations for your deployment. 
Do not specify the values for the `podSecurityPolicy` parameters. Set value for `restrictedEnvironment` parameter to `false`.

Execute the template rendering for your configuration:

```sh
helm template ${CONSUL_RELEASE_NAME} ./ -n {CONSUL_NAMESPACE} --output-dir out
```

Where:

* `${CONSUL_RELEASE_NAME}` is the name of the Consul release. For example, `consul-service`.
* `${CONSUL_NAMESPACE}` is the namespace for Consul service.
* `./` is the path to [chart](/charts/helm/consul-service) folder.
* `out` is the directory where rendered templates are placed.

After execution of the command, the templates are available in the `out/consul/templates` folder. 
You can find the necessary templates in the folder.

For example, the following command allows you to get all Cluster Roles and Cluster Role Bindings which are generated according 
to your `values.yaml`:

```sh
cat out/consul/templates/*clusterrole* && cat out/consul/templates/*/*clusterrole* && cat out/consul/templates/*podsecuritypolicy* && cat out/consul/templates/*securitycontextconstraint*
```

To create resources from template, run the following command with privileged user:

```sh
kubectl apply -f ${TEMPLATE_PATH} -n ${CONSUL_NAMESPACE}
```

where:

* `${TEMPLATE_PATH}` is the path to corresponding template that needs to be created. 
* For example, `out/consul/templates/mesh-gateway-clusterrole.yaml`.
* `${CONSUL_NAMESPACE}` is the namespace for Consul service.

After creating necessary resources on cluster you need to specify names of created Pod Security Policies, 
if it is necessary, in your deployment configuration for each service, set the value for `restrictedEnvironment` parameter 
to `true` and run deployment.
For example, for `client`:

```yaml
...
client:
  enabled: true
  podSecurityPolicy: consul-client
...
```

Manually created resources are not managed by Helm and are not removed during uninstalling.
