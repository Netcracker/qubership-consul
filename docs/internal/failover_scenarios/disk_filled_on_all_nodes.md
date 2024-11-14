# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- Consul service is deployed in HA scheme (there are at least three running Consul server pods)
- Persistent volumes are used
- Consul UI is enabled

## Scenario

**Note**: `[-token ${TOKEN}]` is option to specify the bootstrap ACL token if Consul service is deployed
with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

1. Check the list of Consul nodes by running the following command inside any Consul server pod:

    ```sh
    $ consul operator raft list-peers [-token ${TOKEN}]
    Node                                           ID                                    Address            State     Voter  RaftProtocol
    consul-service-consul-service-consul-server-1  02bffdb1-2f9b-6040-af11-d947a8c19ae2  10.128.5.137:8300  leader    true   3
    consul-service-consul-service-consul-server-0  2cd5000d-0dd5-37e9-6b13-db8cb9b74128  10.128.3.200:8300  follower  true   3
    consul-service-consul-service-consul-server-2  392f0ca7-e5e7-bb50-1916-aa16c6c7ab5e  10.128.3.201:8300  follower  true   3
    ```

2. Go to the Consul UI 
   on the `ACL` tab specify the bootstrap ACL token if Consul service is deployed with enabled ACLs
   (`global.acls.manageSystemACLs` parameter set to `true`) and create KV with a key named `disk_filled_on_all_nodes/0`
   and a value from the [big_value.txt](/docs/internal/failover_scenarios/resources/big_value.txt) file. KV is created successfully,
   the following message is displayed:

    ![Successful message](/docs/internal/failover_scenarios/pictures/successful_adding_key.png)

3. Sequentially fill up the disk space on all Consul nodes by using the following command:

    ```sh
    $ dd if=/dev/zero of=/consul/data/busy_space bs=${BLOCK_SIZE} count=${NUMBER_OF_BLOCKS}
    dd: /consul/data/busy_space: No space left on device
    ```

   where:

      * `${BLOCK_SIZE}` is the size of the block. For example, `5M`.
      * `${NUMBER_OF_BLOCKS}` is the number of blocks of `${BLOCK_SIZE}` size that should be copied.
        For example, if the disk size is `1024M`, you need `205` blocks of `5M` size to fill it.

   It is also enough to fill the disk space on all but one Consul nodes. In that case the last
   remaining Consul server node with empty disk space becomes the leader, but the Consul leader server
   does not create new KVs if all followers are unavailable, because it can't replicate data.

4. Consul can reserve some disk space, so it needs to be filled as well. For this purpose generate
   KVs with large values (for example, from [big_value.txt](/docs/internal/failover_scenarios/resources/big_value.txt) file). Stop creating KVs,
   when the following error will appear in all Consul server pods:

    ```text
    2020/08/05 12:53:14 [ERROR] raft: Failed to append to logs: no space left on device
    ```

5. In Consul UI, create KV with a key named `disk_filled_on_all_nodes/1000` and a value from the
   [big_value.txt](/docs/internal/failover_scenarios/resources/big_value.txt) file.
   Consul do not create the KV because the disk space is full, the following message is displayed:

    ![Error message](/docs/internal/failover_scenarios/pictures/error_adding_key_message.png)

6. To clear disk space use the following command:

    ```sh
    rm /consul/data/busy_space
    ```

   Also do not forget to remove created KVs by the following command:

    ```sh
    $ consul kv delete [-token ${TOKEN}] -recurse disk_filled_on_all_nodes/
    Success! Deleted keys with prefix: disk_filled_on_all_nodes/
    ```
