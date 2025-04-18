The following topics are covered in this chapter:

<!-- TOC -->
* [Common Problems](#common-problems)
  * [Leader Server Failure](#leader-server-failure)
  * [All Servers Failure](#all-servers-failure)
  * [CPU Limit](#cpu-limit)
  * [Memory Limit](#memory-limit)
  * [Data Is Out of Space](#data-is-out-of-space)
  * [No Cluster Leader - Election Timeout Reached](#no-cluster-leader---election-timeout-reached)
  * [Consul Client Failed Renaming Node ID](#consul-client-failed-renaming-node-id)
  * [Consul Responses with 429 Code](#consul-responses-with-429-code)
* [Deployment Problem](#deployment-problem)
  * [Consul Server Pod Does Not Start](#consul-server-pod-does-not-start)
    * [Monitoring](#monitoring)
    * [Consul Starts on Incorrect Nodes](#consul-starts-on-incorrect-nodes)
    * [Consul Client Pod does not Start](#consul-client-pod-does-not-start)
    * [Pod Failed with Consul Sidecar Container](#pod-failed-with-consul-sidecar-container)
  * [Consul and NFS](#consul-and-nfs)
* [ACL Issues](#acl-issues)
  * [Reset ACL](#reset-acl)
* [Full backup](#full-backup)
  * [Multiple datacenters configuration](#multiple-datacenters-configuration)
* [Velero](#velero)
  * [Consul Does Not Work After Restore From Velero Backup](#consul-does-not-work-after-restore-from-velero-backup)
* [Prometheus Alerts](#prometheus-alerts)
  * [ConsulDoesNotExistAlarm](#consuldoesnotexistalarm)
    * [Description](#description)
    * [Possible Causes](#possible-causes)
    * [Impact](#impact)
    * [Actions for Investigation](#actions-for-investigation)
    * [Recommended Actions to Resolve Issue](#recommended-actions-to-resolve-issue)
  * [ConsulIsDegradedAlarm](#consulisdegradedalarm)
    * [Description](#description-1)
    * [Possible Causes](#possible-causes-1)
    * [Impact](#impact-1)
    * [Actions for Investigation](#actions-for-investigation-1)
    * [Recommended Actions to Resolve Issue](#recommended-actions-to-resolve-issue-1)
  * [ConsulIsDownAlarm](#consulisdownalarm)
    * [Description](#description-2)
    * [Possible Causes](#possible-causes-2)
    * [Impact](#impact-2)
    * [Actions for Investigation](#actions-for-investigation-2)
    * [Recommended Actions to Resolve Issue](#recommended-actions-to-resolve-issue-2)
  * [ConsulCPULoadAlarm](#consulcpuloadalarm)
    * [Description](#description-3)
    * [Possible Causes](#possible-causes-3)
    * [Impact](#impact-3)
    * [Actions for Investigation](#actions-for-investigation-3)
    * [Recommended Actions to Resolve Issue](#recommended-actions-to-resolve-issue-3)
  * [ConsulMemoryUsageAlarm](#consulmemoryusagealarm)
    * [Description](#description-4)
    * [Possible Causes](#possible-causes-4)
    * [Impact](#impact-4)
    * [Actions for Investigation](#actions-for-investigation-4)
    * [Recommended Actions to Resolve Issue](#recommended-actions-to-resolve-issue-4)
<!-- TOC -->

This section provides detailed troubleshooting procedures for the Consul Service.

If you face any problem with Consul Service refer to the 
[Official Troubleshooting Guide](https://learn.hashicorp.com/consul/day-2-operations/troubleshooting).

# Common Problems

## Leader Server Failure

This problem can be detected on the Consul monitoring dashboard. If the current cluster's size
goes down, then it means that one of the servers most likely has crashed.

Consul cluster will be temporary unable to process requests until new leader is elected. Leader
election is performed automatically by Consul cluster. More information about leader election in
Consul you can find at [Consensus](https://www.consul.io/docs/internals/consensus.html) article.

As a solution, reboot failed Consul server pod.

## All Servers Failure

A `Down` status on monitoring indicates that all the Consul servers have failed.

As a solution, restart all Consul server pods.

## CPU Limit

Consul request processing may be impacted up to potential Consul server failure when CPU consumption
reaches resource limit for particular Consul server.

As a solution, increase CPU requests and limits for Consul server.

For more information, see [Consul CPU Overload](/docs/public/troubleshooting-scenarios/cpu_overload.md).

## Memory Limit

Consul request processing may be impacted up to potential Consul server failure when memory
consumption reaches resource limit for particular Consul server.

As a solution, increase memory requests and limits or scale out the cluster.

For more information, see [Consul Memory Limit](/docs/public/troubleshooting-scenarios/memory_limit.md).

## Data Is Out of Space

Consul becomes non-operational when disk capacity on a server runs out due to high volume of KVs.

For more information, see [Consul Disk Filled on All Nodes](/docs/public/troubleshooting-scenarios/disk_filled_on_all_nodes.md).

## No Cluster Leader - Election Timeout Reached

The Consul cluster is in an invalid state, you are not able to send requests to Consul and logs of
the Consul server(s) have the following errors:

```text
2020/08/13 07:16:58 [ERR] agent: failed to sync remote state: No cluster leader
2020/08/13 07:17:04 [WARN]  raft: Election timeout reached, restarting election
2020/08/13 07:17:04 [INFO]  raft: Node at 10.130.169.164:8300 [Candidate] entering Candidate state in term 169
2020/08/13 07:17:07 [ERR] http: Request GET /v1/operator/autopilot/health, error: No cluster leader from=127.0.0.1:43164
2020/08/13 07:17:07 [ERR] agent: Coordinate update error: No cluster leader 
```

It means that Consul cannot form a quorum to elect a single node to be the leader. A quorum is
a majority of members from peer set: for a set of size `n`, quorum requires at least
`(n+1)/2` members. For example, if there are 3 members in the peer set, we would need 2 nodes to form
a quorum.

The reason is that most of the Consul server nodes are unavailable. The possible reasons, that the
Consul server failed, are:

* The disk is out of space.
* The Consul server is out of cluster due to long absence. For example, because of OpenShift/Kubernetes
  node failure.
* etc.

To solve the problem, look through logs from all Consul server pods and identify the failed ones
(errors different from `No cluster leader` and `Election timeout reached, restarting election`). Fix
identified problems using the other articles of this guide.

If the Consul cluster is unavailable after fixing all problems, restart the Consul: edit the
StatefulSet `consul-service-consul-server`, set `replicas` to `0` and wait until all pods are scaled down.
Then return `replicas` to the previous value, for example `3`.

## Consul Client Failed Renaming Node ID

If the Consul Client pod starts with Node ID already gathered by another Client, it shows warning:

```text
[WARN] agent.fsm: EnsureRegistration failed: error="failed inserting node: Error while renaming Node ID: "1f0a9dbd-655c-e7d8-c934-6f4d5be42491": Node name node-1 is reserved by node c11fbfa2-3144-01fb-44b1-b4ded071e4a7 with name node-1"
```

In this case services can not register in Consul. It can happen in case of manual cluster scaling
(it should be done via Rolling Update job) or after failed de-registration.

To resolve such issue it is recommended to apply `--disable-host-node-id=true` flag to Consul Client
DaemonSet. It can be specified in `client.extraConfig` parameter:

```yaml
extra-from-values.json: '{"disable_update_check":true, "disable_host_node_id":true}'
```

Also, the stored Node ID can be discarded by removing the `/data/node-id` file manually in Client pod terminal.
In both cases, restart the affected Client pods to apply changes and confirm that unique node id is applied
for every Consul Client.

## Consul Responses with 429 Code

The problem occurs because of limit on concurrent TCP connections a single client IP address is allowed to open to the agent's 
HTTP(S) server. The default value for this parameter is `200`. 
For more details, refer to [Agents Configuration File Reference](https://developer.hashicorp.com/consul/docs/agent/config/config-files#http_max_conns_per_client).

There are two ways to increase the limit value:

* In properties during upgrade by Deployer:

  ```yaml
  server:
    extraConfig: {
      "limits": {
        "http_max_conns_per_client": <new_value>
      }
    }
  ```

  where `<new_value>` is an increased value of limit. For example, `500`.

* In `consul-server-config` `configMap` in Consul namespace add to `extra-from-values.json` key `http_max_conns_per_client` parameter.
  For example,

  ```yaml
  extra-from-values.json: '{"disable_update_check":true,"limits":{"http_max_conns_per_client":<new_value>}}'
  ```

  Don't forget to restart Consul servers to apply made changes.

## Failed to get log

Problem with error log "agent.server.raft: failed to get log" allows any action:
  
  1. Fetch backups for checking, via routes "/listbackups", "/listbackups/ID. If status equal "Successful" and if last backup is old, that means that we
  are loss the data.
  2. Removing raft.db, you can follow one of the next actions:
    1) Removing from pod directly: connect to a pod and remove file.
    2) Removing file from PV.(this action better, if you have any pods, and you will not have a time after raload)

  3. Then you need restart pods. Consul is reload, but part of data will be loss.
  4. Trying restore last backup through consul backup daemon. If consul backup daemon don't work, then reload.
  5. Check business applications - key-manager and config-server. If they are failure on authentication to consul, then change secrets to correct from consul-acl-bootstral-secret. Then reboot.


# Deployment Problem

This section provides information about the issues you may face during deployment of Consul Service.

## Consul Server Pod Does Not Start

When you deploy Consul Service, the Consul Server pod does not start. The following are the list of causes:

* Monitoring
* Consul starts on incorrect nodes
* Consul client pod does not start
* Pod failed with Consul sidecar container

### Monitoring

Consul Monitoring is a Telegraf Agent deployed as a sidecar container on Consul Servers. If the Consul Servers does not start, 
the problem can be with Consul Monitoring and you need to check the logs of its container within a Consul Server pod.

The most common cause is the incorrect value in the `smDbHost` parameter. Its value should be a valid address of a data series database.

### Consul Starts on Incorrect Nodes

If you use predefined Persistent Volumes and specify affinity to bind Consul's pods with nodes you may face issues with Consul pods
starting on incorrect nodes.
There are many causes, but most commonly it happens because Consul uses the StatefulSet to deploy pods which cannot guarantee 
Consul pods assign to specific nodes, 
you can only deploy via preferred affinity. To avoid issues with incorrectly assigned nodes, you must split deployment of Consul Server and 
other components.
For more information, refer to the [Predefined Persistent Volumes](/docs/public/installation.md#predefined-persistent-volumes) 
section in the _Consul Service Installation Procedure_.

Sometimes the Consul Server also starts on incorrect nodes with correct affinity. 
It can depend on the state of Kubernetes nodes and cluster.
For example one of Consul's pod could get stuck and skip its turn to bind to node. 
You need to edit the StatefulSet `consul-service-consul-server` and to set `replicas` to `0` and wait until all pods are scaled down.
Then you need to return `replicas` to previous value, for example `3`. 
Perform the above steps until the Consul's pods are assigned to the right nodes.

In case, you deploy Consul with ACL you cannot change the `replicas` number during installation.
You need to re-install the Consul until it is assigned on right PV.
You can use a workaround and create `host-path` folders for Consul's pods on each Kubernetes node.

### Consul Client Pod does not Start

If you deploy Consul Service with enabled `client`, you need to make sure your Kubernetes nodes have free port `8502`.
Clients use this port for `grpc` to connect with client services.

### Pod Failed with Consul Sidecar Container

Some pods with enabled `connect-inject` cannot start with errors in sidecar container.
Check if is there any problem with Consul Client on the Kubernetes node. 
If there is an issue, resolve it and then re-deploy your service.
Also, you need to check the deployment parameters. Verify the `connect-inject` has the following parameters:

```yaml
connectInject:
  enabled: true

client:
  enabled: true
  grpc: true
```

## Consul and NFS

Not all types of NFS storage are compatible with Consul.

Using NFS can lead to several problems, and also it may be the cause of performance degradation.

If you have the following problem when Consul pods start,
it indicates that the NFS works in "On demand" mode and does not allow to read file until operating system scans it.

```text
==> Error starting agent: Failed to start Consul server: Failed to start Raft: open /consul/data/raft/raft.db: invalid argument
```

You need to resolve the issue with NFS configuration or start using local storage instead of NFS.

# ACL Issues

The following section describe the ACL issues and its solutions.

## Reset ACL

If you encounter issues that are unresolvable, or misplace (lose) the bootstrap token, you can reset the ACL system by updating the index. 

You can see this issue with error on ACL init pod:

```text
2020-04-01T19:54:47.778Z [ERROR] ACLs already bootstrapped but the ACL token was not written to a Kubernetes secret. We can't proceed because the bootstrap token is lost. You must reset ACLs.
``` 

Find the leader using `/v1/status/leader` endpoint on any node of Consul in terminal. ACL reset must be performed on the leader.

```sh
$ curl localhost:8500/v1/status/leader
"172.17.0.3:8300"%
```

In the above example, you can see that the leader is at IP `172.17.0.3`. Run the following commands on that server.

Re-run the bootstrap command to get the index number.

```sh
$ consul acl bootstrap
Failed ACL bootstrapping: Unexpected response code: 403 (Permission denied: ACL bootstrap no longer allowed (reset index: 13))
```

Write the reset index into the bootstrap reset file. For example, here the reset index is 13.

```sh
echo 13 >> /consul/data/acl-bootstrap-reset
```

After resetting the ACL system, you can recreate the bootstrap token or re-install Consul Cluster.

# Full backup

## Multiple datacenters configuration

For example, you have already installed Consul in 2 datacenters and try to perform full backup, but you see the following error in response
to collecting a Consul backup:

```json
{
    "is_granular": false,
    "db_list": "full backup",
    "id": "20201211T152323",
    "failed": true,
    "locked": false,
    "sharded": false,
    "ts": 1607700203000,
    "exit_code": 1,
    "spent_time": "2525ms",
    "size": "9566b",
    "exception": "Traceback (most recent call last):\n\n  File \"/opt/backup/backup-daemon.py\", line 213, in __perform_backup\n    raise BackupProcessException(msg)\n\nBackupProcessException: Last 5 lines of logfile: b'[2020-12-11T15:23:26,339][INFO][category=Backup] Snapshot for datacenter \"dc1\" completed successfully.\\n [2020-12-11T15:23:26,357][ERROR][category=Backup] There is problem with getting snapshot from datacenter: dc2, details: ACL not found\\n'\n",
    "valid": false,
    "evictable": true
}
```

It means that the Consul datacenter from the error ("dc2") is not configured to back up data.

As a solution, upgrade problematic Consul with the following parameter:

```yaml
backupDaemon:
  enabled: true
```

# Velero

## Consul Does Not Work After Restore From Velero Backup

After recovery from a Velero backup, Consul clients or other applications can't work with Consul.

To properly restore Consul from a Velero backup, Consul backup daemon should be installed.
If it is not installed, you need to perform the following steps manually to recover Consul:

1. Find service account secret starting with `<CONSUL_FULLNAME>-auth-method`. Save `ca.crt` and `token` from this secret in any text editor.
2. Transform found `ca.crt` locally from multi-line to one-line string to use it in the next step. For example, the following certificate:

   ```text
   -----BEGIN CERTIFICATE-----
   MIICyDCCAbCgAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl
   cm5ldGVzMB4XDTIxMDIwODIyMzEwOVoXDTMxMDIwNjIyMzEwOVowFTETMBEGA1UE
   AxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAJm4
   iV3cacrBc4OMw55fFVATaXioGHZF65iEFfiQz5rni3xVeeKmCJMLuScUEsqny8io
   HyE7ESxt9/hdXz1smRiNXtMGCVTIYKv2RTtiRP/b1R9DBZUIcdVIMSm5h19lhrL7
   sAIMPx1KLQcWRWwaYmFNJBIJi5ZPJVE+UR84g67W8HGIv9EQUZZOrVSd4C0ybgx5
   k7Rt99FXCzzEPLh8iq/yzvwV95ctag2Hr1gWbELJcCSt6im8P2X7uQ7mc8izF5xf
   aXoJtxWOIgQb1BE5okv5IerKUAWbCmsac0nsawfBLWj5CCjssGWTYQj6GK4hgZIm
   5KuJccaJippJ1x/+5LkCAwEAAaMjMCEwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB
   /wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAEZGx+VFVni0bD1gnbIMdbwX3grC
   4vlTrj/suvzwlZ6+ff2ygEMb3pjmCprLUXeWC7rVzqxNEmVr0xH3hAGCB57qolck
   nzxOA8dtIwUotG1oM/8bRvNSqhnxNlQeptiagHd+Zyrux9vV5ZogM76NwPAbbT48
   OooOMshWjxG7RHWqKNEG5c8mc7cEBYpM+NdGLbzAcDnYzOL7QlQUrH7dqtFeLb0A
   u8EF80PYejvrtBdNYEteJkBZSkNAVC1e3HjYO6eA6enyEW3d/6d5HzcOuZyWx7OE
   Q6SiRG7FfqFgfAmUN9P1+1B1soT7+SxknhebwITr0gkppY2eXyZ7l7Wox8U=
   -----END CERTIFICATE-----
   ```
   
   is to be transformed to the following structure:

   ```text
   -----BEGIN CERTIFICATE-----\nMIICyDCCAbCgAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl\ncm5ldGVzMB4XDTIxMDIwODIyMzEwOVoXDTMxMDIwNjIyMzEwOVowFTETMBEGA1UE\nAxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAJm4\niV3cacrBc4OMw55fFVATaXioGHZF65iEFfiQz5rni3xVeeKmCJMLuScUEsqny8io\nHyE7ESxt9/hdXz1smRiNXtMGCVTIYKv2RTtiRP/b1R9DBZUIcdVIMSm5h19lhrL7\nsAIMPx1KLQcWRWwaYmFNJBIJi5ZPJVE+UR84g67W8HGIv9EQUZZOrVSd4C0ybgx5\nk7Rt99FXCzzEPLh8iq/yzvwV95ctag2Hr1gWbELJcCSt6im8P2X7uQ7mc8izF5xf\naXoJtxWOIgQb1BE5okv5IerKUAWbCmsac0nsawfBLWj5CCjssGWTYQj6GK4hgZIm\n5KuJccaJippJ1x/+5LkCAwEAAaMjMCEwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB\n/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAEZGx+VFVni0bD1gnbIMdbwX3grC\n4vlTrj/suvzwlZ6+ff2ygEMb3pjmCprLUXeWC7rVzqxNEmVr0xH3hAGCB57qolck\nnzxOA8dtIwUotG1oM/8bRvNSqhnxNlQeptiagHd+Zyrux9vV5ZogM76NwPAbbT48\nOooOMshWjxG7RHWqKNEG5c8mc7cEBYpM+NdGLbzAcDnYzOL7QlQUrH7dqtFeLb0A\nu8EF80PYejvrtBdNYEteJkBZSkNAVC1e3HjYO6eA6enyEW3d/6d5HzcOuZyWx7OE\nQ6SiRG7FfqFgfAmUN9P1+1B1soT7+SxknhebwITr0gkppY2eXyZ7l7Wox8U=\n-----END CERTIFICATE-----
   ```

3. Run the following commands to create necessary auth methods from any Consul server pod:

   ```bash
   curl -XPUT -H "X-Consul-Token:<CONSUL_BOOTSTRAP_ACL_TOKEN>" -k -H "Accept:application/json" -H  "Content-Type:application/json" "<CONSUL_URL>/v1/acl/auth-method/<CONSUL_FULLNAME>-k8s-auth-method" -d'
   {
     "Name": "<CONSUL_FULLNAME>-k8s-auth-method",
     "Description": "Kubernetes Auth Method",
     "Type": "kubernetes",
     "Config": {
       "Host": "https://kubernetes.default.svc",
       "CACert": "<SA_SECRET_CA_CRT>",
       "ServiceAccountJWT": "<SA_SECRET_TOKEN>"
     }
   }'
   ```

   ```bash
   curl -XPUT -H "X-Consul-Token:<CONSUL_BOOTSTRAP_ACL_TOKEN>" -k -H "Accept:application/json" -H  "Content-Type:application/json" "<CONSUL_URL>/v1/acl/auth-method/<CONSUL_FULLNAME>-k8s-component-auth-method" -d'
   {
     "Name": "<CONSUL_FULLNAME>-k8s-component-auth-method",
     "Description": "Kubernetes Auth Method",
     "Type": "kubernetes",
     "Config": {
       "Host": "https://kubernetes.default.svc",
       "CACert": "<SA_SECRET_CA_CRT>",
       "ServiceAccountJWT": "<SA_SECRET_TOKEN>"
     }
   }'
   ```

Where:

  * `<CONSUL_FULLNAME>` is the fullname of Consul. For example, `consul`.
  * `<CONSUL_BOOTSTRAP_ACL_TOKEN>` is the Consul bootstrap ACL token that can be found in corresponding secret.
  * `<CONSUL_URL>` is the URL for Consul with protocol and port. For example, `http://consul-server:8500`.
  * `<SA_SECRET_CA_CRT>` is the `ca.crt` certificate transformed to one-line string from 2 step.
  * `<SA_SECRET_TOKEN>` is the `token` from 1 step.

# Prometheus Alerts

## ConsulDoesNotExistAlarm

### Description

There are no Consul server pods in namespace.

### Possible Causes

- Consul Server pod failures or unavailability.
- Resource constraints impacting Consul Server pod performance.
- Consul Server stateful set is scaled down to 0 intentionally or due to incorrect installation.

### Impact

- Complete unavailability of the Consul cluster.

### Actions for Investigation

1. Check if the Consul Server pods exist.
2. Check if the Consul Server stateful set exists and has at least one desired pod.
3. Verify resource utilization of Consul Server pods (CPU, memory).

### Recommended Actions to Resolve Issue

1. Scale-in or redeploy Consul Service pods if stateful set is in failed state.
2. Investigate and address any resource constraints affecting the Consul Server pod performance.

## ConsulIsDegradedAlarm

### Description

Consul cluster is degraded, it means that at least one of the nodes have failed, but cluster is able to work.

For more information refer to [Leader Server Failure](#leader-server-failure).

### Possible Causes

- Consul Server pod failures or unavailability.
- Resource constraints impacting Consul Server pod performance.

### Impact

- Reduced or disrupted functionality of the Consul cluster.
- Potential impact on processes relying on the Consul.

### Actions for Investigation

1. Check the status of Consul Server pods.
2. Review logs for Consul Server pods for any errors or issues.
3. Verify resource utilization of Consul Server pods (CPU, memory).

### Recommended Actions to Resolve Issue

1. Restart or redeploy Consul Server pods if they are in a failed state.
2. Investigate and address any resource constraints affecting the Consul Server pod performance.

## ConsulIsDownAlarm

### Description

Consul cluster is down, and there are no available pods.

For more information refer to [All Servers Failure](#all-servers-failure).

### Possible Causes

- Network issues affecting the Consul Server pod communication.
- Consul Server's storage is corrupted.
- Internal error blocks Consul Server cluster working.

### Impact

- Complete unavailability of the Consul cluster.
- Other processes relying on the Consul cluster will fail.

### Actions for Investigation

1. Check the status of Consul Server pods.
2. Review logs for Consul Server pods for any errors or issues.
3. Verify resource utilization of Consul Server pods (CPU, memory).

### Recommended Actions to Resolve Issue

1. Restart or redeploy Consul Server pods if they are in a failed state.
2. Investigate and address any resource constraints affecting the Consul Server pod performance.

## ConsulCPULoadAlarm

### Description

One of Consul Server pods uses 95% of the CPU limit.

For more information refer to [CPU Limit](#cpu-limit).

### Possible Causes

- Insufficient CPU resources allocated to Consul Server pods.
- The service is overloaded.

### Impact

- Increased response time and potential slowdown of Consul requests.
- Degraded performance of services used the Consul.
- Potential Consul server failure when CPU consumption reaches resource limit for particular Consul server.

### Actions for Investigation

1. Monitor the CPU usage trends in Consul Monitoring dashboard.
2. Review Consul Server logs for any performance related issues.

### Recommended Actions to Resolve Issue

1. Try to increase CPU request and CPU limit for Consul Server.
2. Scale up Consul cluster as needed.

## ConsulMemoryUsageAlarm

### Description

One of Consul Server pods uses 95% of the memory limit.

For more information refer to [Memory Limit](#memory-limit).

### Possible Causes

- Insufficient memory resources allocated to Consul Server pods.
- Service is overloaded.

### Impact

- Potentially lead to the increase of response times or crashes.
- Degraded performance of services used the Consul.

### Actions for Investigation

1. Monitor the memory usage trends in Consul Monitoring dashboard.
2. Review Consul Server logs for memory related errors.

### Recommended Actions to Resolve Issue

1. Try to increase Memory request and Memory limit for Consul Server.
2. Scale up Consul cluster as needed.
