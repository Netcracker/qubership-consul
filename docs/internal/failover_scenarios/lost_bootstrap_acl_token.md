# Scenario Testing

## Prerequisites

- [bash](https://en.wikipedia.org/wiki/Bash_(Unix_shell)) is available
- [oc](https://github.com/openshift/origin/releases) (openshift-origin-client-tools) or
  [kubectl](https://github.com/kubernetes/kubernetes/releases) is installed

## Scenario

1. Install Consul service with storage class or persistent volumes and enabled ACL (`global.acls.manageSystemACLs`
   parameter set to `true`) using job, or the following Helm command:

    ```sh
    helm install ${RELEASE_NAME} .\charts\helm\consul-service\ -n ${NAMESPACE}
    ```

   where:

      * `${RELEASE_NAME}` is the name of Helm Chart release and name of the Consul service.
        For example, `consul-cluster`.
      * `${NAMESPACE}` is the Kubernetes namespace to deploy Consul. For example, `consul-service`.

2. Check the Consul state by running the following command inside any Consul server pod:

    ```sh
    $ consul operator raft list-peers -token ${TOKEN}
    Node                            ID                                    Address             State     Voter  RaftProtocol
    consul-cluster-consul-server-0  0ef45409-8e20-e4b1-a80f-ba0bb13633d5  10.131.60.152:8300  leader    true   3
    consul-cluster-consul-server-2  0f02d8de-0d9f-6324-e973-eb97247c3b16  10.131.60.141:8300  follower  true   3
    consul-cluster-consul-server-1  4f9d4be8-5be7-7979-d2f9-d8c3dbdcbd8b  10.131.60.160:8300  follower  true   3
    ```

   where `${TOKEN}` is the bootstrap ACL token that can be found in the corresponding secret.

3. Uninstall current Consul service by running the following command:

    ```sh
    helm delete ${RELEASE_NAME} -n ${NAMESPACE}
    ```

   Do not forget to remove ACL secret with name `${RELEASE_NAME}-consul-bootstrap-acl-token`
   manually by dint of Kubernetes UI or the following command:

    ```sh
    kubectl delete secret ${RELEASE_NAME}-consul-bootstrap-acl-token
    ```

   Persistent volume claims should remain.

4. Install the Consul service with the same configuration in the same namespace using job, or the
   following Helm command:

    ```sh
    helm install ${RELEASE_NAME} .\charts\helm\consul-service\ -n ${NAMESPACE}
    ```

5. Make sure that ACL init pod failed with the following error:

    ```text
    2020-08-03T16:18:08.308Z [ERROR] ACLs already bootstrapped but the ACL token was not written to a Kubernetes secret. We can't proceed because the bootstrap token is lost. You must reset ACLs.
    ```

6. To find the leader Consul server, run the following command inside any Consul server pod:

    ```sh
    $ curl localhost:8500/v1/status/leader
    "10.131.60.187:8300"
    ```

   ACL reset must be performed on the leader Consul server.

7. Get the reset index by running the following bootstrap command on the leader Consul server:

    ```sh
    $ consul acl bootstrap
    Failed ACL bootstrapping: Unexpected response code: 403 (Permission denied: rpc error making call: ACL bootstrap no longer allowed (reset index: 6))
    ```

8. Use the reset index from the previous step to write it into the bootstrap reset file:

    ```sh
    echo 6 >> /consul/data/acl-bootstrap-reset
    ```

9. Wait a minute and check that Consul service is alive. Verify this by running the following
   command inside any Consul server pod:

    ```sh
    $ consul operator raft list-peers -token ${TOKEN}
    Node                            ID                                    Address             State     Voter  RaftProtocol
    consul-cluster-consul-server-0  0ef45409-8e20-e4b1-a80f-ba0bb13633d5  10.131.60.147:8300  follower  true   3
    consul-cluster-consul-server-2  0f02d8de-0d9f-6324-e973-eb97247c3b16  10.131.60.187:8300  leader    true   3
    consul-cluster-consul-server-1  4f9d4be8-5be7-7979-d2f9-d8c3dbdcbd8b  10.131.60.137:8300  follower  true   3
    ```

   Note, that `${TOKEN}` should differ from the token from the `2` step.
