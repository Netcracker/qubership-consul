---
name: troubleshoot-consul
description: Diagnose and resolve HashiCorp Consul issues — cluster health, leader election, quorum loss, service discovery failures, health check flapping, ACL problems, resource exhaustion, Raft corruption, and Kubernetes deployment issues. Use when Consul servers fail or the cluster is down, logs show No cluster leader, Election timeout reached, failed to get log, no space left on device, Error while renaming Node ID, database is locked, or raft.db invalid argument, Consul responds with HTTP 429, a server, client, or connect-inject pod does not start, the ACL bootstrap token is lost, a full backup or switchover fails, Consul breaks after a Velero restore, or ConsulDoesNotExistAlarm, ConsulIsDegradedAlarm, ConsulIsDownAlarm, ConsulCPULoadAlarm, or ConsulMemoryUsageAlarm fires.
---

# Troubleshoot Consul

Use this skill when investigating issues related to:

- **Consul server cluster health** (leader election, quorum loss, leader server failure, all servers failure)
- **Service registration** and DNS resolution failures
- **Health checks** (flapping and misconfiguration)
- **ACL** token and policy issues, ACL reset after a lost bootstrap token
- **Kubernetes-deployed Consul** (consul-k8s, Helm chart), pods that do not start, NFS storage
- **Connect** (service mesh) sidecar proxy issues
- **Backup daemon** and disaster recovery operator, full backup with multiple datacenters, restore from Velero backup
- **Prometheus alerts** `ConsulDoesNotExistAlarm`, `ConsulIsDegradedAlarm`, `ConsulIsDownAlarm`, `ConsulCPULoadAlarm`, `ConsulMemoryUsageAlarm`

Failure-mode details and the remaining resolution steps live in `references/`; this file provides routing and deployment-configuration guardrails. Use only the bundled references.

## Component Overview

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

### Critical configuration paths

Configuration changes go through Helm chart values and a redeploy via the normal delivery channel. Key value paths:

#### Server resources

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

#### Extra agent configuration

Merged as JSON on top of operator defaults:

```yaml
server:
  extraConfig: '{"limits": {"http_max_conns_per_client": ...}}'

client:
  extraConfig: '{"disable_update_check": true, "disable_host_node_id": true}'
```

#### ACL management

```yaml
global:
  acls:
    manageSystemACLs: true
    bootstrapToken:
      secretName: ...
      secretKey: ...
```

#### Backup daemon

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

#### Disaster recovery

```yaml
global:
  disasterRecovery:
    mode: "active" # active | standby | disable
    region: ...
    timeout: 600
```

#### Connect inject (service mesh)

```yaml
connectInject:
  enabled: true
client:
  enabled: true
  grpc: true
```

Refer to the Consul installation guide for specific values per environment and cloud provider.

### Forbidden actions

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
- `rm /consul/data/raft.db` — never use this command.
- Deploying Consul on NFS storage — incompatible with Consul's file locking requirements.

## Common Failure Modes

### Leader election loop / quorum loss

**Symptom**: Logs show `No cluster leader` and `Election timeout reached, restarting election`. Cluster unable to process write requests.

**Cause**: Majority of server pods are unavailable, or clock skew between servers prevents Raft heartbeats.

**Fix**:

1. Check all server pod statuses and restart any failed pods.
2. If pods are healthy but election still fails, check for clock skew — sync NTP across nodes.
3. Last resort — StatefulSet reset:
   - Scale `consul-service-consul-server` StatefulSet to `0`; wait for full scale-down.
   - Scale back to original replica count.
   - **Do not do this during ACL initialization** — use host-path persistent volumes as workaround.

Guide section: `## No Cluster Leader - Election Timeout Reached`.

---

### Raft database corruption

**Symptom**: Server logs show `agent.server.raft: failed to get log`.

**Cause**: `raft.db` file corrupted, typically after unclean shutdown or disk error.

**Fix** (in order):

1. Check backup daemon status — verify a recent successful backup exists before proceeding.
2. Remove `raft.db` from the affected pod or directly from the PV if pods are not running.
3. Restart Consul server pods — Consul will rebuild Raft state from peers.
4. Restore from last backup via backup daemon if data loss is unacceptable.
5. After recovery, verify business app authentication (e.g., key-manager, config-server) using credentials from `consul-acl-bootstrap-secret`; restart affected services.

**Note**: Some data loss is expected.

Guide section: `## Failed to get log`.

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

Guide section: `## Data Is Out of Space`; scenario: [Disk filled on all nodes](./references/disk_filled_on_all_nodes.md).

---

### CPU or memory resource limit hit

**Symptom**: Slow Consul responses, HTTP errors, pod OOMKilled, or CPU throttling visible in monitoring.

**Cause**: Server pod has reached its configured CPU or memory limit under high load.

**Fix**: Set `server.resources.requests` and `server.resources.limits` in Helm chart values and redeploy through the normal delivery channel.

Guide sections: `## CPU Limit`, `## Memory Limit`; scenarios: [CPU overload](./references/cpu_overload.md), [Memory limit](./references/memory_limit.md).

---

### HTTP 429 Too Many Requests

**Symptom**: Applications receive HTTP 429 responses from Consul. Default concurrent connection limit is 200 per client IP.

**Cause**: Concurrent TCP connections from a single client IP exceeded `http_max_conns_per_client`.

**Fix**: Set `server.extraConfig` in Helm chart values and redeploy:

```yaml
server:
  extraConfig: '{"limits": {"http_max_conns_per_client": <new_value>}}'
```

Refer to the Consul installation guide for recommended values.

Guide section: `## Consul Responses with 429 Code`.

---

### Client node ID conflict

**Symptom**: Client pod logs warn: `Node name <node> is reserved by node <different_id> with name <node>`. Services cannot register.

**Cause**: Manual cluster scaling bypassed the Rolling Update job, leaving stale node IDs.

**Fix**:

- Preferred: Set `client.extraConfig` in Helm values to `{"disable_update_check": true, "disable_host_node_id": true}` and redeploy.
- Alternative (emergency): Remove `/data/node-id` from the affected client pod and restart it.

**Never** manually scale client pods via `replicas` — always use the Rolling Update job.

Guide section: `## Consul Client Failed Renaming Node ID`.

---

### Backup daemon SQLite locked

**Symptom**: Switchover times out with a backup-timeout error. Backup daemon logs show `BusyError: database is locked`. Backups are present in S3 and completed successfully.

**Cause**: Backup daemon fails to update its internal SQLite status database; switchover logic never sees a recorded success.

**Fix**:

- Immediate: Restart the backup-daemon pod:
  ```bash
  kubectl rollout restart deployment/<consul-fullname>-backup-daemon -n <namespace>
  ```
- Permanent: Upgrade Consul to 0.13.0 or later.

Guide section: `## Backup Daemon SQLite Database Locked`.

---

### Server pod not starting

Four distinct sub-cases — check pod describe and logs to identify:

#### A. Monitoring sidecar failure

**Symptom**: Telegraf sidecar fails; pod stuck in init or error state.  
**Fix**: Verify `smDbHost` points to a valid time-series database.

#### B. Node affinity mismatch

**Symptom**: Pod assigned to a wrong node; StatefulSet cannot enforce hard node assignment.  
**Fix**: StatefulSet reset — scale to `0`, wait for full scale-down, scale back. Repeat until pods land on correct nodes. Do not do this during ACL initialization.

#### C. Port 8502 unavailable

**Symptom**: Consul client pod fails with bind error on gRPC port.  
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

Guide section: `## Consul Server Pod Does Not Start` (`### Monitoring`, `### Consul Starts on Incorrect Nodes`, `### Consul Client Pod does not Start`, `### Pod Failed with Consul Sidecar Container`).

## Using the Troubleshooting Guide and Scenarios

Never read `references/troubleshooting.md` in full; load only the section that matches the symptom.

- [references/troubleshooting.md](./references/troubleshooting.md) — known failure modes and Prometheus alerts, one `##` section each. It is `docs/public/troubleshooting.md` of this repository.

Detailed scenarios (open only the one matching the symptom):

- [references/cpu_overload.md](./references/cpu_overload.md)
- [references/memory_limit.md](./references/memory_limit.md)
- [references/disk_filled_on_all_nodes.md](./references/disk_filled_on_all_nodes.md)

Steps:

1. Match the symptom in `## Common Failure Modes` first, then in the table below. If the table looks stale, list headers with `grep -n '^## ' references/troubleshooting.md`.
2. Read from the matched `##` header to the next `##` header. A `##` section includes its `###` children.
3. Open the scenario linked in the matched row. Do not read unrelated scenarios.
4. Where the guide section and this file give different instructions for the same step, follow this file (for example, the guide's `consul-server-config` ConfigMap edit for HTTP 429 conflicts with `### Forbidden actions`).

## Failure scenarios

| Symptom in a ticket or pipeline | Signal in logs, monitoring, or alerts | Section header | Scenario |
| --- | --- | --- | --- |
| One server crashed; requests fail until a new leader is elected | Current cluster size goes down on the Consul monitoring dashboard | `## Leader Server Failure` | - |
| All servers failed | `Down` status on monitoring | `## All Servers Failure` | - |
| Requests slow or failing at the CPU limit | `ConsulCPULoadAlarm` | `## CPU Limit` | [CPU overload](./references/cpu_overload.md) |
| Requests slow or failing at the memory limit | `ConsulMemoryUsageAlarm` | `## Memory Limit` | [Memory limit](./references/memory_limit.md) |
| Consul non-operational, disk full | `raft: Failed to commit logs: no space left on device` | `## Data Is Out of Space` | [Disk filled on all nodes](./references/disk_filled_on_all_nodes.md) |
| Requests to Consul fail; quorum lost | `No cluster leader`, `Election timeout reached, restarting election` | `## No Cluster Leader - Election Timeout Reached` | - |
| Services cannot register | `Error while renaming Node ID`, `Node name ... is reserved by node ...` | `## Consul Client Failed Renaming Node ID` | - |
| Applications receive HTTP 429 | Concurrent connections per client IP over `http_max_conns_per_client` | `## Consul Responses with 429 Code` | - |
| Raft log read error | `agent.server.raft: failed to get log` | `## Failed to get log` | - |
| Switchover fails on backup timeout | `BusyError: database is locked`, `Unable to insert to jobs database` | `## Backup Daemon SQLite Database Locked` | - |
| Server pod does not start after deploy | Pod stuck; check its containers | `## Consul Server Pod Does Not Start` | - |
| Server pod fails on monitoring | Monitoring container logs, `smDbHost` | `### Monitoring` | - |
| Pods start on incorrect nodes | Predefined PVs with affinity; pods bound to wrong nodes | `### Consul Starts on Incorrect Nodes` | - |
| Client pod does not start | Port `8502` busy on the node | `### Consul Client Pod does not Start` | - |
| `connect-inject` pod fails | Errors in the sidecar container | `### Pod Failed with Consul Sidecar Container` | - |
| Consul pods fail to start on NFS | `Failed to start Raft: open /consul/data/raft/raft.db: invalid argument` | `## Consul and NFS` | - |
| ACL unresolvable or bootstrap token lost | `ACLs already bootstrapped but the ACL token was not written to a Kubernetes secret` | `## Reset ACL` | - |
| Full backup fails with several datacenters | `There is problem with getting snapshot from datacenter: dc2, details: ACL not found` | `## Multiple datacenters configuration` | - |
| Clients or applications cannot work with Consul after restore | Restore from Velero backup without backup daemon | `## Consul Does Not Work After Restore From Velero Backup` | - |
| No Consul server pods in namespace | `ConsulDoesNotExistAlarm` | `## ConsulDoesNotExistAlarm` | - |
| At least one server failed, cluster still works | `ConsulIsDegradedAlarm` | `## ConsulIsDegradedAlarm` | - |
| Cluster down, no available pods | `ConsulIsDownAlarm` | `## ConsulIsDownAlarm` | - |
| A server pod uses 95% of the CPU limit | `ConsulCPULoadAlarm` | `## ConsulCPULoadAlarm` | [CPU overload](./references/cpu_overload.md) |
| A server pod uses 95% of the memory limit | `ConsulMemoryUsageAlarm` | `## ConsulMemoryUsageAlarm` | [Memory limit](./references/memory_limit.md) |

If several rows match, prefer the most specific startup failure (`Consul Server Pod Does Not Start`), then disk or memory, then quorum (`No Cluster Leader`), then ACL or backup, and degraded or down last.

If no row matches, say the reference does not cover it and refer to the [Official Troubleshooting Guide](https://developer.hashicorp.com/consul/docs/troubleshoot).

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

## Log Analysis Tips

Run read-only commands only.

### Consul server logs

```bash
# Quorum and leader election
kubectl logs -n <namespace> <consul-server-pod> | grep -E "No cluster leader|Election timeout reached"

# Raft log, disk, and NFS
kubectl logs -n <namespace> <consul-server-pod> | grep -E "failed to get log|no space left on device|raft.db: invalid argument"

# Current leader (from any Consul node)
kubectl exec -n <namespace> <consul-server-pod> -- curl -s localhost:8500/v1/status/leader

# Data volume usage
kubectl exec -n <namespace> <consul-server-pod> -- df -h /consul/data
```

### Consul client and ACL init logs

```bash
kubectl logs -n <namespace> <consul-client-pod> | grep "Error while renaming Node ID"
kubectl logs -n <namespace> <acl-init-pod> | grep "ACLs already bootstrapped"
```

### Backup daemon

```bash
kubectl logs -n <namespace> deployment/<consul-fullname>-backup-daemon | grep -E "BusyError|Unable to insert to jobs database|ACL not found"

# Backup list and status
kubectl exec -n <namespace> deployment/<consul-fullname>-backup-daemon -- curl -s localhost:8080/listbackups
kubectl exec -n <namespace> deployment/<consul-fullname>-backup-daemon -- curl -s localhost:8080/listbackups/<ID>
```

## Safety

Commands in this file and in the references are recommendations for an authorized operator, not permission to run them. Before scaling the server StatefulSet to `0`, removing `raft.db` or snapshots, resetting ACL, removing `/data/node-id`, or creating auth methods after a Velero restore, get explicit user approval and confirm the namespace, the affected pods, and that a valid backup exists.

## Assessment

Report the matched failure mode or section header, the evidence that selected it, the impact stated there, and only the resolution steps that this file or the matched section supports. State what the available evidence could not answer. Do not invent a root cause.

### Recommendation discipline

- Phrase every configuration change as: _"Set `<helm-value>` in the Consul Helm chart values and redeploy through the normal delivery channel"_, with the YAML snippet underneath.
- If no documented Helm value path exists for a parameter, omit the recommendation. Do **not** fall back to direct CLI commands (`consul operator`, `consul kv`, `kubectl edit`).
- For HA membership changes (failover, switchover), scope the action to the operator's documented procedure — these are operational steps, not Helm configuration.

## When to Escalate

Escalate to component maintainers when:

- The Consul cluster is still unavailable after the StatefulSet reset in `## No Cluster Leader - Election Timeout Reached`.
- `failed to get log` remains after `raft.db` is removed, or the backup daemon cannot restore the last backup.
- ACL problems remain after `## Reset ACL`.
- Consul still does not work after the steps in `## Consul Does Not Work After Restore From Velero Backup`.

Include in the escalation:

- Logs from all Consul server pods and the affected client, ACL init, or backup daemon pods.
- `kubectl get pods,pvc -n <namespace> -o wide`.
- Output of `/v1/status/leader` and, for backup issues, `/listbackups`.
