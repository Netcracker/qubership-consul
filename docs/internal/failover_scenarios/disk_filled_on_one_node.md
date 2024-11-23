# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- Consul service is deployed in HA scheme (there are at least three running Consul server pods)
- Persistent volumes are used

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

2. Fill up disk space on Consul leader server pod. For example, you can use the following command
   inside it:

    ```sh
    $ dd if=/dev/zero of=/consul/data/busy_space bs={BLOCK_SIZE} count={NUMBER_OF_BLOCKS}
    dd: /consul/data/busy_space: No space left on device
    ```

   where

      * `{BLOCK_SIZE}` is the size of the block. For example, `5M`.
      * `{NUMBER_OF_BLOCKS}` is the number of blocks of `{BLOCK_SIZE}` size that should be copied.
        For example, if the disk size is `1024M`, you need `205` blocks of `5M` size to fill it.

3. Make sure the disk is full.

    ```sh
    $ df -h /consul/data
    Filesystem                Size      Used Available Use% Mounted on
    kube06nc-nfs.example.com:/export/pvc-cbc1dc0b-e10c-4a70-bf69-3dd4dd3e8101
                              1.0G      1.0G         0 100% /consul/data
    ```

4. Consul can reserve some disk space, so it needs to be filled as well. For this purpose generate
   KVs with large values (for example, from [big_value.txt](/docs/internal/failover_scenarios/resources/big_value.txt) file). Stop
   creating KVs, when the following error will appear in the leader Consul server pod:

    ```text
    2020/08/05 12:53:14 [ERROR] raft: Failed to append to logs: no space left on device
    ```

5. After the disk on the leader Consul server is full, the Consul re-elects leader:

    ```sh
    $ consul operator raft list-peers [-token ${TOKEN}]
    Node                                           ID                                    Address            State     Voter  RaftProtocol
    consul-service-consul-service-consul-server-1  02bffdb1-2f9b-6040-af11-d947a8c19ae2  10.128.5.137:8300  follower  true   3
    consul-service-consul-service-consul-server-0  2cd5000d-0dd5-37e9-6b13-db8cb9b74128  10.128.3.200:8300  leader    true   3
    consul-service-consul-service-consul-server-2  392f0ca7-e5e7-bb50-1916-aa16c6c7ab5e  10.128.3.201:8300  follower  true   3
    ```

6. Check that Consul works correctly by creating several KVs.

    ```sh
    $ consul kv put [-token ${TOKEN}] disk_filled_on_one_node/0 0
    Success! Data written to: disk_filled_on_one_node/0
    $ consul kv put [-token ${TOKEN}] disk_filled_on_one_node/1 0
    Success! Data written to: disk_filled_on_one_node/1
    $ consul kv put [-token ${TOKEN}] disk_filled_on_one_node/2 0
    Success! Data written to: disk_filled_on_one_node/2
    $ consul kv put [-token ${TOKEN}] disk_filled_on_one_node/3 0
    Success! Data written to: disk_filled_on_one_node/3
    ```

7. To clear disk space use the following command:

    ```sh
    rm /consul/data/busy_space
    ```

   Also do not forget to remove created KVs by the following command:

    ```sh
    $ consul kv delete [-token ${TOKEN}] -recurse disk_filled_on_one_node/
    Success! Deleted keys with prefix: disk_filled_on_one_node/
    ```
