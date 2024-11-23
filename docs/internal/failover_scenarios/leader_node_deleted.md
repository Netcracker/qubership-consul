# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- Consul service is deployed in HA scheme (there are at least three running Consul server pods)

## Scenario

**Note**: `[-token ${TOKEN}]` is option to specify the bootstrap ACL token if Consul service is deployed
with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

1. Determine which Consul server node is the leader. There are two ways to do this:

   * Run the following command in any Consul server pod:

     ```sh
     $ consul operator raft list-peers [-token ${TOKEN}]
     Node                            ID                                    Address            State     Voter  RaftProtocol
     consul-service-consul-server-2  eebcf56e-f149-0241-1e55-ce08ccc83e55  10.128.7.70:8300   follower  true   3
     consul-service-consul-server-1  be500fda-8512-700f-83ba-8dae6aaaba13  10.128.3.111:8300  leader    true   3
     consul-service-consul-server-0  1beac71a-af89-1c38-01e0-170ce870f097  10.128.5.131:8300  follower  true   3
     ``` 

     The result is presented as a table and allows to determine the name of leader node. For example,
     in this case it is the node with name `consul-service-consul-server-1`.

   * Go to the Consul UI.
     To receive information about Consul nodes, go to the `Nodes` tab. The leader node is marked with
     a `star` symbol.

2. Delete the leader Consul node and wait ten seconds.

3. Check that a new leader is elected by running command in the pod or using the Consul UI. For example,
   in Consul server pod you can see the following results:

    ```sh
    $ consul operator raft list-peers [-token ${TOKEN}]
    Node                            ID                                    Address            State     Voter  RaftProtocol
    consul-service-consul-server-2  eebcf56e-f149-0241-1e55-ce08ccc83e55  10.128.7.70:8300   leader    true   3
    consul-service-consul-server-0  1beac71a-af89-1c38-01e0-170ce870f097  10.128.5.131:8300  follower  true   3
    ```

4. Check the efficiency of the Consul service by performing CRUD (create, read, update, delete) operations
   inside `leader` Consul server pod.
    
    **Create**:

    ```sh
    $ consul kv put [-token ${TOKEN}] leader_node_deleted/0 0
    Success! Data written to: leader_node_deleted/0
    ```

   **Check that KV is created**:

    ```sh
    $ consul kv get [-token ${TOKEN}] leader_node_deleted/0
    0
    ```

   **Update**:

   ```sh
    $ consul kv put [-token ${TOKEN}] leader_node_deleted/0 5
    Success! Data written to: leader_node_deleted/0
    ```

   **Check that KV is updated**:

    ```sh
    $ consul kv get [-token ${TOKEN}] leader_node_deleted/0
    5
    ```

    **Delete**:

   ```sh
    $ consul kv delete [-token ${TOKEN}] -recurse leader_node_deleted/
    Success! Deleted keys with prefix: leader_node_deleted/
   ```

   **Check that KV is deleted**:

   ```sh
   $ consul kv get [-token ${TOKEN}] leader_node_deleted/0
   Error! No key exists at: leader_node_deleted/0
   ```
