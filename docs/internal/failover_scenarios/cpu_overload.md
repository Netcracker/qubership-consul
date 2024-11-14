# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- There is one running Consul server pod
- Consul monitoring is enabled

## Scenario

**Note**: `[-token ${TOKEN}]` is option to specify the bootstrap ACL token if Consul service is deployed
with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

1. Make sure that the Consul has a leader node and is ready to work. In single Consul server pod run
   the following command:

    ```sh
    $ consul operator raft list-peers [-token ${TOKEN}]
    Node                         ID                                    Address           State   Voter  RaftProtocol
    consul-helm-consul-server-0  297f7646-7043-acca-71aa-eac418ffd21a  10.130.2.97:8300  leader  true   3
    ```

   Then create test KV also by running the command inside the Consul server pod:

    ```sh
    $ consul kv put [-token ${TOKEN}] cpu_overload/0 0
    Success! Data written to: cpu_overload/0

    $ consul kv get [-token ${TOKEN}] cpu_overload/0
    0
    ```

2. Create a load on the Consul CPU by adding a huge quantity of KVs in several threads with a small
   value size. KVs can be created with the following command:

    ```sh
    curl [--header "Authorization: Bearer ${TOKEN}"] -s -XPUT "${CONSUL_URL}/v1/kv/cpu_overload/${INDEX}" --data ${INDEX} 1>/dev/null
    ```

   where:

      * `${CONSUL_URL}` is the external URL to the Consul server if you create KVs outside the
        Kubernetes/OpenShift, or `http://127.0.0.1:8500` if KVs are created in the Consul server pod.
        For example, `http://consul-consul-service.kube.example.com`.
      * `${INDEX}` is the serial KV's number in the `cpu_overload` folder. For example, `2`.
      * `[--header "Authorization: Bearer ${TOKEN}"]` is option to specify the bootstrap ACL token
        if Consul service is deployed with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

3. To create additional load on the Consul CPU, run the following command, that generates unlimited
   random data, inside the Consul server pod:

    ```sh
    cat /dev/urandom > /dev/null
    ```

4. During KVs generation you can watch the CPU change on the Consul monitoring dashboard.
   The `Consul Process CPU Usage` widget shows the CPU usage by the Consul process on all cores in
   milli-cores. The graph will increase until Consul CPU reaches the limit specified in the Consul
   server pod resources parameter.
   
    ![Consul Process CPU Usage](/docs/internal/failover_scenarios/pictures/cpu_overload.png)

   When it happens, Consul will continue to work, but new requests will be processed more slowly. It
   can be seen on the Consul monitoring dashboard, on widgets related to Raft, KVs.

5. Stop the generation of the load by KVs. To stop the additional load on the Consul CPU, just stop
   command execution or use the following command inside the Consul server pod:

    ```sh
    kill -9 $(pidof cat)
    ```

6. Make sure that Consul CPU characteristics returned to normal.

7. After completing case testing clear all KVs that were created during the load generation on
   the Consul using the following command:

    ```sh
    $ consul kv delete [-token ${TOKEN}] -recurse cpu_overload/
    Success! Deleted keys with prefix: cpu_overload/
    ```
