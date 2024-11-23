# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed
- Consul service is deployed in HA scheme (there are at least three running Consul server pods)

## Scenario

**Note**: `[-token ${TOKEN}]` is option to specify the bootstrap ACL token if Consul service is deployed
with enabled ACLs (`global.acls.manageSystemACLs` parameter set to `true`).

1. Check the list of Consul nodes by running the following command inside any Consul server pod:

    ```sh
    $ consul operator raft list-peers [-token ${TOKEN}]
    Node                            ID                                    Address            State     Voter  RaftProtocol
    consul-service-consul-server-0  627d109d-3aa6-b690-3c50-f8de2d87d635  10.130.2.83:8300   leader    true   3
    consul-service-consul-server-1  138101d9-d407-e9b1-c263-02d891ffb390  10.131.6.102:8300  follower  true   3
    consul-service-consul-server-2  addee6a5-8b2e-2f27-3923-23dd12aaef3b  10.131.3.103:8300  follower  false  3
    ```

2. Check the information about Consul pods by running the following command using OpenShift/Kubernetes client:

    ```sh
    $ kubectl get pods -o wide
    NAME                                                READY     STATUS    RESTARTS   AGE       IP             NODE
    consul-service-consul-backup-daemon-c4c889d-hclgm   1/1       Running   0          4d        10.130.3.220   dr311dev-node-left-1
    consul-service-consul-server-0                      1/1       Running   0          4m        10.130.2.83    dr311dev-node-left-1
    consul-service-consul-server-1                      1/1       Running   0          3m        10.131.6.102   dr311dev-node-left-3
    consul-service-consul-server-2                      1/1       Running   0          2m        10.131.3.103   dr311dev-node-left-2
    ```

   Find the OpenShift/Kubernetes node where the leader Consul node (`consul-service-consul-server-0`)
   is located. For example, in this case it is `dr311dev-node-left-1`.

3. Connect to the selected OpenShift/Kubernetes node (`dr311dev-node-left-1`) using the command:

    ```sh
    sudo ssh -i ${RSA_KEY} ${USER}@${IP_ADDRESS}
    ```

   where:

      * `${USER}` is the user to connect to the node. For example, `openshift`.
      * `${RSA_KEY}` is the file that contains the RSA private key. For example, `rsa_key`.
      * `${IP_ADDRESS}` is the IP address of necessary OpenShift/Kubernetes node. For example, `10.102.1.208`.

4. Restart the OpenShift/Kubernetes node by running the following command:

   ```sh
   sudo shutdown -r
   ```

5. While the OpenShift/Kubernetes node is restarting, make sure that the Consul server pod from this
   node (`dr311dev-node-left-1`) does not have `Running` status.

   ```sh
   $ oc get pods -o wide
   NAME                                                READY     STATUS    RESTARTS   AGE       IP             NODE
   consul-service-consul-backup-daemon-c4c889d-l89n4   1/1       Running   0          2m        10.131.3.106   dr311dev-node-left-2
   consul-service-consul-server-0                      1/1       Unknown   0          27m       10.130.2.83    dr311dev-node-left-1
   consul-service-consul-server-1                      1/1       Running   0          26m       10.131.6.102   dr311dev-node-left-3
   consul-service-consul-server-2                      1/1       Running   0          26m       10.131.3.103   dr311dev-node-left-2
   ```

6. Make sure that the Consul service has chosen the new leader.

   ```sh
   $ consul operator raft list-peers [-token ${TOKEN}]
   Node                            ID                                    Address            State     Voter  RaftProtocol
   consul-service-consul-server-0  627d109d-3aa6-b690-3c50-f8de2d87d635  10.130.2.83:8300   follower  true   3
   consul-service-consul-server-1  138101d9-d407-e9b1-c263-02d891ffb390  10.131.6.102:8300  leader    true   3
   consul-service-consul-server-2  addee6a5-8b2e-2f27-3923-23dd12aaef3b  10.131.3.103:8300  follower  true   3
   ```

7. Check the efficiency of the Consul service by performing CRUD (create, read, update, delete) operations
   inside `leader` Consul server pod.

   **Create**:

    ```sh
    $ consul kv put [-token ${TOKEN}] kubernetes_node_failed/0 0
    Success! Data written to: kubernetes_node_failed/0
    ```
   
   **Check that KV is created**:

    ```sh
    $ consul kv get [-token ${TOKEN}] kubernetes_node_failed/0
    0
    ```

   **Update**:

    ```sh
    $ consul kv put [-token ${TOKEN}] kubernetes_node_failed/0 5
    Success! Data written to: kubernetes_node_failed/0
    ```

   **Check that KV is updated**:

    ```sh
    $ consul kv get [-token ${TOKEN}] kubernetes_node_failed/0
    5
    ```

   **Delete**:

    ```sh
    $ consul kv delete [-token ${TOKEN}] -recurse kubernetes_node_failed/
    Success! Deleted keys with prefix: kubernetes_node_failed/
    ```

   **Check that KV is deleted**:

    ```sh
    $ consul kv get [-token ${TOKEN}] kubernetes_node_failed/0
    Error! No key exists at: kubernetes_node_failed/0
    ```
