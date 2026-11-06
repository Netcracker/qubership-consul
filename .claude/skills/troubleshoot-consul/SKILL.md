---
name: troubleshoot-consul
description: Diagnose and resolve HashiCorp Consul issues — cluster health, leader election, quorum loss, service discovery failures, health check flapping, ACL problems, resource exhaustion, Raft corruption, and Kubernetes deployment issues. Also covers backup daemon failures, multiple datacenter full backup, NFS storage, restore from Velero backup, and the Prometheus alerts ConsulDoesNotExistAlarm, ConsulIsDegradedAlarm, ConsulIsDownAlarm, ConsulCPULoadAlarm, and ConsulMemoryUsageAlarm.
---

## Scope

This skill covers:

- Consul server cluster health (leader election, quorum loss, leader server failure, all servers failure)
- Service registration and DNS resolution failures
- Health check flapping and misconfiguration
- ACL token and policy issues (including ACL reset)
- Kubernetes-deployed Consul (consul-k8s, Helm chart)
- Connect (service mesh) sidecar proxy issues
- Backup daemon and disaster recovery operator (including multiple datacenters full backup)
- Consul on NFS storage
- Consul restore from Velero backup
- Prometheus alerts: `ConsulDoesNotExistAlarm`, `ConsulIsDegradedAlarm`, `ConsulIsDownAlarm`, `ConsulCPULoadAlarm`, `ConsulMemoryUsageAlarm`

For any other problem refer to the [Official Troubleshooting Guide](https://developer.hashicorp.com/consul/docs/troubleshoot).

## Troubleshooting guide

`references/troubleshooting.md` is `docs/public/troubleshooting.md`. Do not read it in full. Grep section headers first, then read only the matched section:

```bash
grep -n '^## ' references/troubleshooting.md
```

## Deployment

### Application structure

Consul is delivered as a single Helm chart. Key components:

| Component                    | Kind        | Purpose                                       |
| ---------------------------- | ----------- | --------------------------------------------- |
| `consul-server`              | StatefulSet | Raft consensus cluster                        |
| `consul-client`              | DaemonSet   | Local agent (deprecated, disabled by default) |
| `backup-daemon`              | Deployment  | Snapshot backup and restore                   |
| `consul-acl-configurator`    | Deployment  | ACL CRD reconciler                            |
| `status-provisioner`         | Job         | Post-deploy health validation                 |
| `disaster-recovery-operator` | Deployment  | Switchover and failover orchestration         |

### CRD install ordering

The following CRDs must be pre-installed **before** Helm install when auto-creation is disabled:

| CRD                                                    | When required                                                                 |
| ------------------------------------------------------ | ----------------------------------------------------------------------------- |
| `ConsulACL`                                            | Always, when `global.restrictedEnvironment: true` or CRD creation is disabled |
| `GrafanaDashboard`, `PrometheusRule`, `ServiceMonitor` | When `monitoring.enabled: true`                                               |
| `SiteManager`                                          | When `global.disasterRecovery.mode` is set                                    |

Auto-upgrade is enabled by default unless `DISABLE_CRD=true`. Requires cluster rights on `apiextensions.k8s.io/customresourcedefinitions`.

### Restricted environments

When the deployer lacks cluster-wide rights, set `global.restrictedEnvironment: true`. Cluster Role, Cluster Role Binding, and Pod Security Policy must be pre-created by a cluster administrator before installation.

## Forbidden actions

Never recommend these — the operator reverts them or they risk data loss:

- `consul force-leave <node>` on a live server — can drop quorum; only after node is confirmed permanently gone.
- `consul operator raft remove-peer` — permanently removes a peer; operator may re-add it on next reconcile.
- `consul kv delete` on operator-managed key prefixes — operator overwrites KV on next reconcile.
- Manual edits to `consul.hcl` or any config file on a pod/node — operator regenerates config on restart.
- `kubectl edit cm <consul-configmap>` — Helm/operator state is source of truth; edits are overwritten on next reconcile.
- `consul snapshot restore` — overwrites entire cluster KV and ACL state; never run without an explicit runbook and change window.
- `consul acl token delete` — may silently break all services that depend on the token.
- Manual cluster scaling via the `replicas` field — use the Rolling Update job instead.
- Editing StatefulSet replicas during ACL initialization — breaks bootstrap.
- `rm /consul/data/raft.db` without first verifying a valid backup exists.
- Deploying Consul on NFS storage — incompatible with Consul's file locking requirements.

## Helm configuration

Configuration changes go through Helm chart values and a redeploy via the normal delivery channel. Key value paths:

### Server resources

```yaml
server:
  resources:
    requests:
      cpu: ...
      memory: ...
    limits:
      cpu: ...
      memory: ...
```

### Extra agent configuration

Merged as JSON on top of operator defaults:

```yaml
server:
  extraConfig: '{"limits": {"http_max_conns_per_client": ...}}'

client:
  extraConfig: '{"disable_update_check": true, "disable_host_node_id": true}'
```

### ACL management

```yaml
global:
  acls:
    manageSystemACLs: true
    bootstrapToken:
      secretName: ...
      secretKey: ...
```

### Backup daemon

```yaml
backupDaemon:
  enabled: true
  backupSchedule: "0 0 * * *"
  evictionPolicy: "1h/1d,7d/delete"
  s3:
    enabled: true
    url: ...
    bucket: ...
    keyId: ...
    keySecret: ...
```

### Disaster recovery

```yaml
global:
  disasterRecovery:
    mode: "active" # active | standby | disable
    region: ...
    timeout: 600
```

### Connect inject (service mesh)

```yaml
connectInject:
  enabled: true
client:
  enabled: true
  grpc: true
```

Refer to the Consul installation guide for specific values per environment and cloud provider.

## Common failure modes

### Leader server failure

**Symptom**: The current cluster size goes down on the Consul monitoring dashboard — one of the servers most likely has crashed.

**Cause**: A Consul server pod failed. The cluster is temporarily unable to process requests until a new leader is elected. Leader election is performed automatically by the Consul cluster (see [Consensus](https://www.consul.io/docs/internals/consensus.html)).

**Fix**: Reboot the failed Consul server pod.

---

### All servers failure

**Symptom**: A `Down` status on monitoring — all the Consul servers have failed.

**Fix**: Restart all Consul server pods.

---

### Leader election loop / quorum loss

**Symptom**: Logs show `No cluster leader` and `Election timeout reached, restarting election`. Cluster unable to process write requests.

```text
[ERR] agent: failed to sync remote state: No cluster leader
[WARN]  raft: Election timeout reached, restarting election
[INFO]  raft: Node at 10.130.169.164:8300 [Candidate] entering Candidate state in term 169
[ERR] http: Request GET /v1/operator/autopilot/health, error: No cluster leader from=127.0.0.1:43164
```

**Cause**: Majority of server pods are unavailable, or clock skew between servers prevents Raft heartbeats. Consul cannot form a quorum: for a peer set of size `n`, quorum requires at least `(n+1)/2` members (2 of 3). A server may fail because the disk is out of space, or it dropped out of the cluster after a long absence (for example, a Kubernetes node failure).

**Fix**:

1. Check all server pod statuses and restart any failed pods. Look through logs from all Consul server pods and identify the failed ones (errors different from `No cluster leader` and `Election timeout reached, restarting election`); fix them using the other failure modes in this skill.
2. If pods are healthy but election still fails, check for clock skew — sync NTP across nodes.
3. Last resort — StatefulSet reset:
   - Scale `consul-service-consul-server` StatefulSet to `0`; wait for full scale-down.
   - Scale back to original replica count.
   - **Do not do this during ACL initialization** — use host-path persistent volumes as workaround.

---

### Raft database corruption

**Symptom**: Server logs show `agent.server.raft: failed to get log`.

**Cause**: `raft.db` file corrupted, typically after unclean shutdown or disk error.

**Fix** (in order):

1. Check backup daemon status — verify a recent successful backup exists before proceeding. Fetch backups via `/listbackups` and `/listbackups/<ID>`: if the status is `Successful` but the last backup is old, data will be lost.
2. Remove `raft.db` from the affected pod or directly from the PV if pods are not running.
3. Restart Consul server pods — Consul will rebuild Raft state from peers.
4. Restore from last backup via backup daemon if data loss is unacceptable. If the backup daemon does not work, reload it.
5. After recovery, verify business app authentication (e.g., key-manager, config-server) using credentials from `consul-acl-bootstrap-secret`; restart affected services.

**Note**: Some data loss is expected.

---

### Disk full on server

**Symptom**: Raft logs show `[ERROR] raft: Failed to commit logs: no space left on device`. Consul becomes non-operational.

**Cause**: High KV volume or accumulated snapshots exhausting disk on the server PVC.

**Fix**:

1. Remove outdated snapshots from inside the pod:
   ```bash
   rm -r /consul/data/raft/snapshots/<snapshot_id>
   ```
2. Restart Consul server.
3. Long-term: increase PVC size via `server.storage` Helm value, or configure `backupDaemon.evictionPolicy`.

For detection and metrics, see `references/disk_filled_on_all_nodes.md`.

---

### CPU or memory resource limit hit

**Symptom**: Slow Consul responses, HTTP errors, pod OOMKilled, or CPU throttling visible in monitoring. Consul request processing may be impacted up to potential Consul server failure.

**Cause**: Server pod has reached its configured CPU or memory limit under high load.

**Fix**: Set `server.resources.requests` and `server.resources.limits` in Helm chart values and redeploy through the normal delivery channel. For memory, you can also scale out the cluster.

For detection and metrics, see `references/cpu_overload.md` and `references/memory_limit.md`.

---

### HTTP 429 Too Many Requests

**Symptom**: Applications receive HTTP 429 responses from Consul. Default concurrent connection limit is 200 per client IP.

**Cause**: Concurrent TCP connections from a single client IP exceeded `http_max_conns_per_client` (see [Agents Configuration File Reference](https://developer.hashicorp.com/consul/docs/agent/config/config-files#http_max_conns_per_client)).

**Fix**: Set `server.extraConfig` in Helm chart values and redeploy:

```yaml
server:
  extraConfig: '{"limits": {"http_max_conns_per_client": <new_value>}}'
```

where `<new_value>` is an increased limit, for example `500`.

Refer to the Consul installation guide for recommended values.

---

### Client node ID conflict

**Symptom**: Client pod logs warn: `Node name <node> is reserved by node <different_id> with name <node>`. Services cannot register.

**Cause**: Manual cluster scaling bypassed the Rolling Update job, leaving stale node IDs, or a de-registration failed.

**Fix**:

- Preferred: Set `client.extraConfig` in Helm values to `{"disable_update_check": true, "disable_host_node_id": true}` and redeploy.
- Alternative (emergency): Remove `/data/node-id` from the affected client pod and restart it.

In both cases, restart the affected client pods and confirm that a unique node ID is applied for every Consul client.

**Never** manually scale client pods via `replicas` — always use the Rolling Update job.

---

### Backup daemon SQLite locked

**Symptom**: Switchover times out with a backup-timeout error. Backup daemon logs show `BusyError: database is locked` and `Unable to insert to jobs database`. Backups are present in S3 and completed successfully.

**Cause**: Backup daemon fails to update its internal SQLite status database; switchover logic never sees a recorded success.

**Fix**:

- Immediate: Restart the backup-daemon pod:
  ```bash
  kubectl rollout restart deployment/<consul-fullname>-backup-daemon -n <namespace>
  ```
- Permanent: Upgrade Consul to 0.13.0 or later (new Backup Daemon DB feature).

---

### Server pod not starting

Four distinct sub-cases — check pod describe and logs to identify:

#### A. Monitoring sidecar failure

**Symptom**: Telegraf sidecar fails; pod stuck in init or error state.  
**Fix**: Check the logs of the monitoring container within the Consul server pod. Verify `smDbHost` points to a valid time-series database.

#### B. Node affinity mismatch

**Symptom**: Pod assigned to a wrong node; StatefulSet cannot enforce hard node assignment.  
**Cause**: Predefined Persistent Volumes with affinity to nodes — a StatefulSet allows only preferred affinity. Sometimes the pods start on incorrect nodes even with correct affinity, depending on the state of Kubernetes nodes and cluster (a pod gets stuck and skips its turn to bind to a node).  
**Fix**: Split deployment of Consul server and other components (see [Predefined Persistent Volumes](/docs/public/installation.md#predefined-persistent-volumes)). StatefulSet reset — scale to `0`, wait for full scale-down, scale back. Repeat until pods land on correct nodes. Do not do this during ACL initialization: re-install Consul until it is assigned to the right PV, or create `host-path` folders for Consul pods on each Kubernetes node.

#### C. Port 8502 unavailable

**Symptom**: Consul client pod (deployed with enabled `client`) fails with bind error on gRPC port.  
**Fix**: Free port 8502 on the target Kubernetes node (used for client gRPC services).

#### D. Connect-inject sidecar failure

**Symptom**: Application pod with `connect-inject` annotation fails to start; sidecar errors.  
**Fix**:

1. Verify Consul client is healthy on the node.
2. Confirm Helm values include:
   ```yaml
   connectInject:
     enabled: true
   client:
     enabled: true
     grpc: true
   ```
3. Redeploy the application after fixing.

---

### Consul on NFS

**Symptom**: Consul pods fail to start with:

```text
==> Error starting agent: Failed to start Consul server: Failed to start Raft: open /consul/data/raft/raft.db: invalid argument
```

**Cause**: Not all types of NFS storage are compatible with Consul. The NFS works in "On demand" mode and does not allow reading a file until the operating system scans it. NFS can also cause performance degradation.

**Fix**: Resolve the NFS configuration, or use local storage instead of NFS.

---

### Reset ACL

**Symptom**: ACL issues are unresolvable, or the bootstrap token is lost. ACL init pod shows:

```text
[ERROR] ACLs already bootstrapped but the ACL token was not written to a Kubernetes secret. We can't proceed because the bootstrap token is lost. You must reset ACLs.
```

**Fix**: Reset the ACL system by updating the index. Run on the leader:

1. Find the leader from any Consul node:
   ```sh
   curl localhost:8500/v1/status/leader
   ```
2. On the leader, re-run bootstrap to get the reset index:
   ```sh
   consul acl bootstrap
   # Failed ACL bootstrapping: ... (reset index: 13)
   ```
3. Write the reset index into the bootstrap reset file:
   ```sh
   echo 13 >> /consul/data/acl-bootstrap-reset
   ```
4. Recreate the bootstrap token or re-install the Consul cluster.

---

### Multiple datacenters full backup

**Symptom**: Full backup fails with `There is problem with getting snapshot from datacenter: dc2, details: ACL not found` in the backup exception.

**Cause**: The Consul datacenter from the error is not configured to back up data.

**Fix**: Upgrade the problematic Consul with:

```yaml
backupDaemon:
  enabled: true
```

---

### Consul does not work after restore from Velero backup

**Symptom**: After recovery from a Velero backup, Consul clients or other applications cannot work with Consul.

**Cause**: Consul backup daemon is not installed. It is required to properly restore Consul from a Velero backup.

**Fix** (only when backup daemon is not installed):

1. Find the service account secret starting with `<CONSUL_FULLNAME>-auth-method`. Save `ca.crt` and `token`.
2. Transform `ca.crt` from multi-line to a one-line string with `\n` separators.
3. From any Consul server pod, create the auth methods `<CONSUL_FULLNAME>-k8s-auth-method` and `<CONSUL_FULLNAME>-k8s-component-auth-method`:
   ```bash
   curl -XPUT -H "X-Consul-Token:<CONSUL_BOOTSTRAP_ACL_TOKEN>" -k -H "Accept:application/json" -H "Content-Type:application/json" "<CONSUL_URL>/v1/acl/auth-method/<AUTH_METHOD_NAME>" -d'
   {
     "Name": "<AUTH_METHOD_NAME>",
     "Description": "Kubernetes Auth Method",
     "Type": "kubernetes",
     "Config": {
       "Host": "https://kubernetes.default.svc",
       "CACert": "<SA_SECRET_CA_CRT>",
       "ServiceAccountJWT": "<SA_SECRET_TOKEN>"
     }
   }'
   ```

Where `<CONSUL_URL>` is the Consul URL with protocol and port (for example, `http://consul-server:8500`), `<CONSUL_BOOTSTRAP_ACL_TOKEN>` is from the corresponding secret, `<SA_SECRET_CA_CRT>` is the one-line `ca.crt`, and `<SA_SECRET_TOKEN>` is the `token`.

---

## Prometheus alerts

| Alert                    | Description                                                    | Possible causes                                                                                       | Impact                                                                    | Investigation                                                                                      | Resolution                                                                                         |
| ------------------------ | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| `ConsulDoesNotExistAlarm` | No Consul server pods in namespace                            | Server pod failures; resource constraints; stateful set scaled to 0 intentionally or by incorrect installation | Complete unavailability of the Consul cluster                             | Check server pods exist; check stateful set exists with at least one desired pod; check CPU/memory | Scale-in or redeploy Consul pods if stateful set failed; address resource constraints              |
| `ConsulIsDegradedAlarm`  | At least one node failed, cluster still works (see Leader server failure) | Server pod failures; resource constraints                                                  | Reduced functionality; impact on processes relying on Consul             | Check server pods status and logs; check CPU/memory                                                | Restart or redeploy failed server pods; address resource constraints                               |
| `ConsulIsDownAlarm`      | Cluster down, no available pods (see All servers failure)      | Network issues between server pods; corrupted server storage; internal error                          | Complete unavailability; processes relying on Consul fail                | Check server pods status and logs; check CPU/memory                                                | Restart or redeploy failed server pods; address resource constraints                               |
| `ConsulCPULoadAlarm`     | A server pod uses 95% of CPU limit (see CPU or memory resource limit hit) | Insufficient CPU; service overloaded                                                       | Increased response time; degraded dependent services; potential server failure | CPU trends on Consul Monitoring dashboard; server logs                                       | Increase CPU request and limit; scale up the cluster as needed                                     |
| `ConsulMemoryUsageAlarm` | A server pod uses 95% of memory limit (see CPU or memory resource limit hit) | Insufficient memory; service overloaded                                                 | Increased response times or crashes; degraded dependent services         | Memory trends on Consul Monitoring dashboard; server logs                                          | Increase memory request and limit; scale up the cluster as needed                                  |

## Recommendation discipline

- Phrase every configuration change as: _"Set `<helm-value>` in the Consul Helm chart values and redeploy through the normal delivery channel"_, with the YAML snippet underneath.
- If no documented Helm value path exists for a parameter, omit the recommendation. Do **not** fall back to direct CLI commands (`consul operator`, `consul kv`, `kubectl edit`).
- For HA membership changes (failover, switchover), scope the action to the operator's documented procedure — these are operational steps, not Helm configuration.

## Common pitfalls

| Symptom                                      | Likely cause                                          | Fix                                                             |
| -------------------------------------------- | ----------------------------------------------------- | --------------------------------------------------------------- |
| Leader election loop                         | Clock skew or network partition between servers       | Sync clocks (NTP); check network between server pods            |
| Service resolves in Consul UI but not in app | App uses wrong DNS port or address                    | Point app DNS to Consul agent on port 8600                      |
| Health check always critical after deploy    | Check interval too short for slow startup             | Add `deregister_critical_service_after`; increase `interval`    |
| ACL token works in curl but not in app       | Token not passed in `X-Consul-Token` header           | Verify SDK/library ACL token config                             |
| Consul agent not joining cluster             | Wrong `retry_join` addresses or firewall on port 8301 | Check serf LAN port reachability between nodes                  |
| All pods healthy but writes fail             | Quorum lost — majority of servers unavailable         | Restore failed pods; StatefulSet reset if pods will not recover |
| Switchover fails with timeout                | Backup daemon SQLite lock — see failure mode above    | Restart backup-daemon pod                                       |
| Pod stuck Pending after deploy               | Node affinity mismatch or port 8502 occupied          | StatefulSet reset or free port on node                          |
