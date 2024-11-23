# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- One Consul server pod is running
- Consul monitoring is enabled

## Scenario

**Note**: `[-token ${TOKEN}]` is option to specify the bootstrap ACL token if Consul service is deployed
with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

1. Make sure that Consul has leader and is ready to work. In single Consul server pod run the following command:

    ```sh
    $ consul operator raft list-peers [-token ${TOKEN}]
    Node                         ID                                    Address           State   Voter  RaftProtocol
    consul-helm-consul-server-0  297f7646-7043-acca-71aa-eac418ffd21a  10.130.2.97:8300  leader  true   3
    ```

   Then create test KV also by running the command inside the Consul server pod:

    ```sh
    $ consul kv put [-token ${TOKEN}] memory_limit/0 0
    Success! Data written to: memory_limit/0

    $ consul kv get [-token ${TOKEN}] memory_limit/0
    0
    ```

2. Create a load on the Consul memory by adding a large quantity of KVs with a large value size
   (close to Consul value size limit - 512 kilobytes). KVs can be created with the following command:

    ```sh
    curl [--header "Authorization: Bearer ${TOKEN}"] -s -XPUT "${CONSUL_URL}/v1/kv/memory_limit/${INDEX}" --data-binary @${FILE_PATH} 1>/dev/null
    ```

   where:

      * `${CONSUL_URL}` is the external URL to the Consul server if you create KVs outside the
        Kubernetes/OpenShift, or `http://127.0.0.1:8500` if KVs are created in the Consul server pod.
        For example, `http://consul-consul-service.kube.example.com`.
      * `${INDEX}` is the serial KV number in the `memory_limit` folder. For example, `2`.
      * `${FILE_PATH}` is the path to a file that contains large value.
        For example, [value.txt](/docs/internal/failover_scenarios/resources/big_value.txt).
      * `[--header "Authorization: Bearer ${TOKEN}"]` is option to specify the bootstrap ACL token
        if Consul service is deployed with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

3. During KVs generation you can watch the memory change on the Consul monitoring dashboard.
   The `Consul Process Memory Usage` widget shows the number of bytes allocated by the Consul process.
   The graph will increase until Consul memory reaches the limit specified in the Consul server pod
   resources parameter.
   
    ![Consul Process Memory Usage](/docs/internal/failover_scenarios/pictures/memory_limit.png)
   
   When it happens, Consul server pod will restart with `OutOfMemory` (OOMKilled) error, and
   the following error will appear in pod logs:

    ```text
    2020/07/28 12:09:32 [ERR] http: Request PUT /v1/kv/memory_limit/143, error: read tcp 10.130.2.97:8500->10.129.10.1:56032: read: bad address from=10.129.10.1:56032
    ```

   Further generation of the load should be stopped. From the error log you can see that KV with
   the `memory_limit/143` key failed to create due to lack of memory.

4. After completing case testing clear all KVs that were created during the load generation on
   the Consul using the following command:

    ```sh
    $ consul kv delete [-token ${TOKEN}] -recurse memory_limit/
    Success! Deleted keys with prefix: memory_limit/
    ```
