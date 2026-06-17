This document presents the security hardening recommendations for the Consul service.

## Exposed Ports

List of ports used by Consul and other Services. 

| Port  | Service                  | Description                                                                                                                                                                                       |
|-------|--------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 8500  | Consul                   | The default Consul server port for http.                                                                                                                                                          |
| 8501  | Consul                   | The consul server port for https.                                                                                                                                                                 |
| 8502  | Consul                   | Port used by clients for gRPC to connect with client services.                                                                                                                                    |
| 8300  | Consul                   | Port used for Consul server communication.                                                                                                                                                        |
| 8096  | Consul                   | Port used for Prometheus client communication.                                                                                                                                                    |
| 9445  | Consul                   | Port used to check if the application is started, alive and ready to serve requests.                                                                                                              |
| 20100 | Consul                   | The port at which the Consul sidecar listens to return combined metrics. This port only needs to be changed if it conflicts with the application's ports.                                         |
| 20200 | Consul                   | The port Prometheus scrapes metrics from, configured via the pod annotation prometheus.io/port and the corresponding listener in the Consul DataPlane sidecar.                                    |
| 8080  | Consul                   | Port used by the webhook server in the Consul sidecar injector.                                                                                                                                   |
| 8301  | Consul                   | UDP and TCP port for Serf LAN communication.                                                                                                                                                      |
| 8302  | Consul                   | UDP and TCP port for Serf WAN communication.                                                                                                                                                      |
| 8600  | Consul                   | UDP and TCP port for DNS service.                                                                                                                                                                 |
| 8088  | Consul Acl Configurator  | Port used by the Consul ACL Configurator REST server for handling configuration requests.                                                                                                         |
| 8500  | Consul Client Monitoring | The port used to monitor the health and status of Consul clients.                                                                                                                                 |
| 8501  | Consul Client Monitoring | The port used to monitor the health and status of Consul clients when TLS is enabled.                                                                                                             |
| 80    | Consul UI                | The port used to access Consul UI if `ui.service.enabled` is true.                                                                                                                                |
| 443   | Consul UI                | The port used to access Consul UI if `ui.service.enabled` is true and if TLS is enabled.                                                                                                          |
| 8080  | Consul Integration-tests | Exposes the container's port to the network. It allows access to the application running in the container.                                                                                        |
| 8443  | Backup Daemon            | Port is used for secure communication with the backup daemon service when TLS is enabled. This ensures encrypted and secure data transmission.                                                    |
| 8080  | Backup Daemon            | Port used to manage and execute backup and restoration tasks to ensure data integrity and availability.                                                                                           |
| 8443  | DRD                      | If TLS for Disaster Recovery is enabled the HTTPS protocol and port 8443 is used for API requests to ensure secure communication.                                                                 |
| 8080  | DRD                      | Port used for SiteManager endpoints.                                                                                                                                                              |
| 31565 | MeshGateway              | Port used by the mesh gateway service when enabled, with type `ClusterIP`.                                                                                                                        |
| 443   | MeshGateway              | Port that the service is exposed on. The targetPort is set to `meshGateway.containerPort` and also port 443 is registered for WAN traffic if `meshGateway.wanAddress.source` is set to `Service`. |
| 8443  | MeshGateway              | Port that the gateway runs on inside the container.                                                                                                                                               |
| 53    | DNS                      | Port used for DNS service over TCP and UDP.                                                                                                                                                       |

## User Accounts

List of user accounts used for Consul and other Services.

| Service | OOB accounts        | Deployment parameter  | Is Break Glass account | Can be blocked | Can be deleted | Comment                                                                                                        |
|---------|---------------------|-----------------------|------------------------|----------------|----------------|----------------------------------------------------------------------------------------------------------------|
| Consul  | client              | backupDaemon.username | no                     | yes            | yes            | The Consul backup daemon API user. There is no default value, the name must be specified during deploy.        |
| Consul  | Bootstrap ACL Token | N/A                   | yes                    | no             | no             | Consul Bootstrap ACL Token: This token cannot be specified during deployment. It must be manually regenerated. |

## Disabling User Accounts

Consul does not support disabling user accounts.

## Password Policies

Since Consul does not employ password-based authentication, password policies are not applicable. 

## Changing password guide

Since Consul does not employ password-based authentication, there are no password changing procedures. 

# Logging

Security events and critical operations should be logged for audit purposes. 
You can find more details about available audit logging in [Audit Guide](/docs/public/audit.md).

# Refresh Bootstrap ACL Token

Consul does not offer a built-in mechanism to refresh the bootstrap ACL token.
However, in certain security-related scenarios, it may be necessary to refresh this token due to its extensive
administrative privileges.

To refresh the bootstrap ACL token, implement the following steps:

1. Open the `consul-bootstrap-acl-token` secret and make a note of the value of the current bootstrap ACL token,
   as it is needed for the next steps.
   The name of the secret can differ according to the `CUSTOM_RESOURCE_NAME` parameter.

   **Note**: Ensure that you decode the value using base64. For example, `bfc8dad9-f567-02c7-1872-332ef62a43f4`.

2. Identify the leader using the `/v1/status/leader` endpoint in a terminal on any Consul Server pod. The ACL reset
   should be performed on the leader.

    ```sh
    $ curl localhost:8500/v1/status/leader
    "172.17.0.3:8300"%
    ```

   In the above example, the leader's IP is `172.17.0.3`. Locate the Consul Server pod with this IP to execute the
   following commands.

3. Run the bootstrap command on the leader pod to obtain the bootstrap reset index number.

    ```sh
    $ consul acl bootstrap
    Failed ACL bootstrapping: Unexpected response code: 403 (Permission denied: ACL bootstrap no longer allowed (reset index: 13))
    ```

4. Write the reset index into the bootstrap reset file. For instance, if the reset index is `13`, execute the following
   command.

    ```sh
    echo 13 >> /consul/data/acl-bootstrap-reset
    ```

5. Generate a new bootstrap token using the following command:

    ```bash
    consul acl bootstrap
    ```
   
   Example of output:

    ```text
    AccessorID: edcaacda-b6d0-1954-5939-b5aceaca7c9a
    SecretID: 4411f091-a4c9-48e6-0884-1fcb092da1c8
    Description: Bootstrap Token (Global Management)
    Local: false
    Create Time: 2018-12-06 18:03:23.742699239 +0000 UTC
    Policies:
    00000000-0000-0000-0000-000000000001 - global-management
    ```

   The value in the `SecretID` field represents the newly generated bootstrap ACL token.

   **Important**: This command does not refresh the existing token, it creates a new one with general management
   privileges.
   The previous token remains active but is deactivated in the next steps.

6. Write the value of the `SecretID` field received in previous step as a `base64` encoded value to the `consul-bootstrap-acl-token` secret.

7. Restart the `Consul Backup Daemon` and `Consul ACL Configurator` pods if they are deployed.

8. Update the new token in all microservices that interact Consul, including the `CMDB` configuration. Restart them if necessary.

9. After delivering the new bootstrap ACL token to all configurations, remove the previous token using a terminal on any Consul pod.

   First, add the environment variable with the current bootstrap ACL token for authorization of further commands.
   
   ```bash
   export CONSUL_HTTP_TOKEN=4411f091-a4c9-48e6-0884-1fcb092da1c8
   ```
   
   Here, `4411f091-a4c9-48e6-0884-1fcb092da1c8` corresponds to the generated Consul bootstrap ACL token from step `5`.

   Identify the `ID` of the previous token.

   ```bash
   consul acl token list --format json | jq 'map(select(.SecretID == "bfc8dad9-f567-02c7-1872-332ef62a43f4"))'
   ```

   Here, `bfc8dad9-f567-02c7-1872-332ef62a43f4` corresponds to the previous Consul bootstrap ACL token from the first step.

   Example of output:

   ```json
   [
     {
       "CreateIndex": 1246469,
       "ModifyIndex": 1246469,
       "AccessorID": "86452c23-87b3-f4a7-a1ec-e1b428ca4602",
       "SecretID": "bfc8dad9-f567-02c7-1872-332ef62a43f4",
       "Description": "Bootstrap Token (Global Management)",
       "Policies": [
         {
           "ID": "00000000-0000-0000-0000-000000000001",
           "Name": "global-management"
         }
       ],
       "Local": false,
       "CreateTime": "2023-05-11T06:23:16.381782249Z",
       "Hash": "X2AgaFhnQGRhSSF/h0m6qpX1wj/HJWbyXcxkEM/5GrY=",
       "Legacy": false
     }
   ]
   ```

   Copy the value of the `AccessorID` field and use it to remove that token with the following command.

   ```bash
   consul acl token delete -id 86452c23-87b3-f4a7-a1ec-e1b428ca4602
   ```

10. The bootstrap token has been refreshed. You can verify this by accessing the Consul UI with both the new and old tokens.
