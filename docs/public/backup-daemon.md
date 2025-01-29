This document describes the information about Consul Backup Daemon features.

# Restore After Full Data Loss

There are two approaches of Consul recovery after full data loss, but both of them require presence of backup
and Consul bootstrap ACL token. In `automatic` approach Consul bootstrap ACL token is stored inside backup, in `manual`
it is stored outside backup.

**Pay attention**, Consul can't be restored after full data loss if you do not have backup or Consul bootstrap ACL token.

## Automatic Recovery

If you have at least one backup with stored Consul bootstrap ACL token inside, 
you can run `restore` procedure from Consul Backup Daemon with the following command:

```sh
curl -u USERNAME:PASSWORD -XPOST http://CONSUL_BACKUP_DAEMON:8080/restore -v -H "Content-Type: application/json" -d'{"vault":"BACKUP_ID"}'
```

where:

  * `USERNAME:PASSWORD` is the credentials of Consul Backup Daemon.
  * `CONSUL_BACKUP_DAEMON` is the host of Consul Backup Daemon. For example, `consul-backup-daemon`.
  * `BACKUP_ID` is the identifier of backup you want to restore Consul from. For example, `20190321T080000`.

If Consul Backup Daemon is deployed with TLS, it is also necessary to use `https` protocol instead of `http`
and add `--cacert /consul/tls/backup/tls.crt` to the command above.

If recovery does not end successfully, you need to define the step where it failed and continue the process manually using the 
[Manual Recovery](#manual-recovery) guide.

## Manual Recovery

If you have an old backup that does not contain Consul bootstrap ACL token inside and a Consul bootstrap ACL token for this 
backup stored separately, there is an ability to recover Consul after full data loss manually.
The following guide is going to help with that:

1. Install or upgrade Consul service with required parameters by Helm.

2. Restore Consul data from backup using the following command:

    ```sh
    curl -u USERNAME:PASSWORD -XPOST http://CONSUL_BACKUP_DAEMON:8080/restore -v -H "Content-Type: application/json" -d'{"vault":"BACKUP_ID"}'
    ```

   where:

    * `USERNAME:PASSWORD` is the credentials of Consul Backup Daemon.
    * `CONSUL_BACKUP_DAEMON` is the host of Consul Backup Daemon. For example, `consul-backup-daemon`.
    * `BACKUP_ID` is the identifier of backup you want to restore Consul from. For example, `20190321T080000`.

   If Consul Backup Daemon is deployed with TLS, it is also necessary to use `https` protocol instead of `http` and
   add `--cacert /consul/tls/backup/tls.crt` to the command above.

3. Find `CONSUL_NAME-bootstrap-acl-token` secret, where `CONSUL_NAME` is the full name of Consul,
   for example, `consul`. Replace data specified in `token` key of secret with stored Consul bootstrap ACL token.
   Don't forget to save changed data.

4. Restart all Consul servers. You can do it one by one or run the following command that removes all pods starting
   with `CONSUL_NAME-server`:

    ```sh
    kubectl get pods -n NAMESPACE --no-headers=true | awk '/CONSUL_NAME-server/{print $1}'| xargs kubectl delete pod -n NAMESPACE
    ```

   where:

    * `CONSUL_NAME` is the full name of Consul. For example, `consul`.
    * `NAMESPACE` is the namespace where Consul is deployed. For example, `consul-service`.

5. Before the further manipulations it is necessary to make sure that Consul servers are alive.
  You can check Consul UI or run the following command from any Consul server pod to find out the Consul leader:

    ```sh
    consul operator raft list-peers -token TOKEN
    ```

   where `TOKEN` is Consul bootstrap ACL token that was restored on 3 step.

6. For each Consul server it is necessary to recover ACL tokens needed to successfully join the datacenter.
    First of all, you have to find all tokens that are used in Consul with the following command in any Consul server pod:

    ```sh
    consul acl token list -token TOKEN
    ```

   where `TOKEN` is Consul bootstrap ACL token that was restored on 3 step.

   In the list of tokens you need to find those that contain `Server Token for consul-server-X.consul-server.consul-service.svc`
   description, where `X` is a server serial number. For example,

    ```text
    AccessorID:       0b7f04d1-fcfe-aa38-2497-1a1bc4e81f58
    SecretID:         d9af2b0b-9329-260d-8812-bda39fda0daa
    Description:      Server Token for consul-server-0.consul-server.consul-service.svc
    Local:            false
    Create Time:      2022-11-07 11:07:50.335915088 +0000 UTC
    Legacy:           false
    Policies:
       ae45e6f9-0719-2c9e-9006-e704ced79160 - agent-token
    ```

   Perform the following commands from each Consul server pod:

    ```sh
    consul acl set-agent-token -token TOKEN agent AGENT_TOKEN
    ```

    ```sh
    consul leave -token TOKEN
    ```

   where

    * `TOKEN` is Consul bootstrap ACL token that was restored on 3 step.
    * `AGENT_TOKEN` is the agent token that should be used for corresponding Consul server.
    For example, for `consul-server-0` we should specify value of `SecretID` field from corresponding token information
    (`d9af2b0b-9329-260d-8812-bda39fda0daa`).

7. Before the further manipulations it is necessary to make sure that Consul servers are alive.
   You can check Consul UI or run the following command from any Consul server pod to find out the Consul leader:

    ```sh
    consul operator raft list-peers -token TOKEN
    ```

   where `TOKEN` is Consul bootstrap ACL token that was restored on 3 step.

8. After full data loss Consul `auth methods` have non-existent JWT tokens and certificates in configuration.
  If Consul `auth methods` are configured incorrectly, none of the components will be able to connect to Consul and work with it.
  To recover `auth methods` you need to find secret with one of the names: `CONSUL_NAME-auth-method`, `CONSUL_NAME-auth-method-secret`,
  `CONSUL_NAME-auth-method-token-{5}`, where `CONSUL_NAME` is the full name of Consul, for example, `consul`, `{5}` is the random part
  of 5 alphanumeric characters. Take value of `ca.crt` key from the found secret and save it to `ca.crt` file on any Consul server pod.
  You can use the following command for that action:

    ```text
    cat > /consul/ca.crt << EOF
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
    EOF
    ```

   Then take value of `token` key from the found secret and perform the following commands on the Consul server pod with
   stored `ca.crt` certificate to actualize `auth methods` configuration:

    ```sh
    consul acl auth-method update -name consul-k8s-auth-method -token TOKEN -kubernetes-service-account-jwt JWT_TOKEN -kubernetes-ca-cert @/consul/ca.crt
    ```

    ```sh
    consul acl auth-method update -name consul-k8s-component-auth-method -token TOKEN -kubernetes-service-account-jwt JWT_TOKEN -kubernetes-ca-cert @/consul/ca.crt
    ```

   where

    * `TOKEN` is Consul bootstrap ACL token that was restored on 3 step.
    * `JWT_TOKEN` is the token taken from `token` key of the found secret.

   It is necessary to update both `auth methods` because they are used for different services interacting with Consul.

9. Restart all Consul components that have been installed with it.
   You can do it one by one or run the following commands that remove Consul backup daemon pod and all pods
   with `restore-policy=restart` selector:

    ```sh
    kubectl delete pod -l restore-policy=restart -n NAMESPACE
    ```

    ```sh
    kubectl delete pod -l name=CONSUL_NAME-backup-daemon -n NAMESPACE
    ```

   where:

    * `CONSUL_NAME` is the full name of Consul. For example, `consul`.
    * `NAMESPACE` is the namespace where Consul is deployed. For example, `consul-service`.
