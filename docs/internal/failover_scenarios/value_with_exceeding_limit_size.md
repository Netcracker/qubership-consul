# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- Consul service has at least one running Consul server pod
- Consul UI is enabled

## Scenario

**Note**: `[-token ${TOKEN}]` is option to specify the bootstrap ACL token if Consul service is deployed
with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

1. Check the Consul state by running the following command inside any Consul server pod:

   ```sh
   $ consul operator raft list-peers [-token ${TOKEN}]
   Node                                           ID                                    Address            State     Voter  RaftProtocol
   consul-service-consul-service-consul-server-0  2cd5000d-0dd5-37e9-6b13-db8cb9b74128  10.128.3.200:8300  leader    true   3
   consul-service-consul-service-consul-server-1  02bffdb1-2f9b-6040-af11-d947a8c19ae2  10.128.5.137:8300  follower  true   3
   consul-service-consul-service-consul-server-2  392f0ca7-e5e7-bb50-1916-aa16c6c7ab5e  10.128.3.201:8300  follower  true   3
   ```

2. In the Consul the limit on a key's value size is 512 kilobytes.
   Go to the Consul UI,
   on the `ACL` tab specify the bootstrap ACL token if Consul service is deployed with enabled ACLs
   (`global.acls.manageSystemACLs` parameter set to `true`) and create KV with a key named `exceeding_limit_size_value/0`
   and a value which size is larger than 512 kilobytes.
   For example, use value from the [extremely_big_value.txt](/docs/internal/failover_scenarios/resources/extremely_big_value.txt) file.

3. After several seconds of waiting the following error message should appear:

   ![Error message](/docs/internal/failover_scenarios/pictures/error_adding_key_message.png)

4. Make sure that KV is not created by running the following command in any Consul server pod:

    ```sh
    $ consul kv get [-token ${TOKEN}] exceeding_limit_size_value/0
    Error! No key exists at: exceeding_limit_size_value/0
    ```
