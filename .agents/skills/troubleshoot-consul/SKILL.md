---
name: troubleshoot-consul
description: Diagnose qubership-consul failures in Jira tickets, CI/CD pipelines, and project support. Use when the consul-service chart, Consul server or client pods, ACL init, backup daemon, or Consul monitoring shows ConsulDoesNotExistAlarm, ConsulIsDegradedAlarm, ConsulIsDownAlarm, ConsulCPULoadAlarm, ConsulMemoryUsageAlarm, No cluster leader, Election timeout reached, Error while renaming Node ID, HTTP 429, failed to get log, database is locked, or a failed restore from Velero.
---

# Troubleshoot Consul

Fast entrypoint for Consul assessment. Failure steps stay in `references/`. This file does not repeat them.

`references/troubleshooting.md` is the fact source for known failure modes. It is `docs/public/troubleshooting.md` from this repository. Do not read it in full for a single symptom.

1. Grep headers with line numbers first: `grep -n '^#' references/troubleshooting.md`
2. Match the symptom against the table below, or the raw headers if the table is stale.
3. Read from the matched header through the next header of the same or higher level. A `##` section includes its `###` children; stop at the next `##` or `#`. Never load the whole file for one lookup.

Read one more file only when the section already loaded points at it:

- CPU Limit → `references/cpu_overload.md`
- Memory Limit → `references/memory_limit.md`
- Data Is Out of Space → `references/disk_filled_on_all_nodes.md`

## Deployment

- Chart `charts/helm/consul-service` deploys the Consul server StatefulSet (`server.replicas` defaults to 3, `server.storage` defaults to 10Gi), Consul clients, Consul monitoring (Telegraf sidecar on servers), ACL init, and the backup daemon.
- Quorum needs a majority of servers: 2 of 3. Below that there is no leader and requests fail.
- A server pod that does not start has four known causes: Monitoring (`smDbHost`), incorrect nodes, client pod, sidecar container. Read the `###` children of `## Consul Server Pod Does Not Start`.

## Forbidden actions

- Do not run ACL reset steps unless the loaded section is `## Reset ACL`. It changes the ACL index.
- Do not delete server PVCs or data to free space unless the loaded section or `references/disk_filled_on_all_nodes.md` says so for that case.
- Do not restart all server pods at once unless the loaded section is `## All Servers Failure`. Restarting a majority breaks quorum.
- Do not treat a full backup storage as a full Consul server data volume, or the reverse.

## Failure scenarios

Match one row, then read that section. No resolution steps live in this table.

| Symptom | Signal | Section header |
| --- | --- | --- |
| Leader server crashed | Cluster size dropped on the dashboard | `## Leader Server Failure` |
| Every server failed | `Down` status | `## All Servers Failure` |
| CPU at the pod limit | `ConsulCPULoadAlarm` | `## CPU Limit` |
| Memory at the pod limit | `ConsulMemoryUsageAlarm` | `## Memory Limit` |
| Data volume full | High KV volume, server non-operational | `## Data Is Out of Space` |
| Quorum lost | `No cluster leader`, `Election timeout reached` | `## No Cluster Leader - Election Timeout Reached` |
| Client reuses a Node ID | `Error while renaming Node ID` | `## Consul Client Failed Renaming Node ID` |
| HTTP 429 from Consul | Too many connections per client IP | `## Consul Responses with 429 Code` |
| Raft log read error | `agent.server.raft: failed to get log` | `## Failed to get log` |
| Switchover fails on backup timeout | `BusyError: database is locked` in backup daemon | `## Backup Daemon SQLite Database Locked` |
| Server pod does not start | Deployment, pod stuck | `## Consul Server Pod Does Not Start` |
| Pods land on wrong nodes | Scheduling mismatch | `### Consul Starts on Incorrect Nodes` |
| Client pod does not start | Client pod failing | `### Consul Client Pod does not Start` |
| Sidecar container failed | Server pod with failing sidecar | `### Pod Failed with Consul Sidecar Container` |
| Consul on NFS misbehaves | NFS storage | `## Consul and NFS` |
| ACL lost or unresolvable | Lost bootstrap token, ACL init pod error | `## Reset ACL` |
| Full backup fails | Several datacenters configured | `## Multiple datacenters configuration` |
| Consul broken after restore | Restore from Velero | `## Consul Does Not Work After Restore From Velero Backup` |
| No Consul server pods exist | `ConsulDoesNotExistAlarm` | `## ConsulDoesNotExistAlarm` |
| Some servers failed | `ConsulIsDegradedAlarm` | `## ConsulIsDegradedAlarm` |
| All servers failed (alert) | `ConsulIsDownAlarm` | `## ConsulIsDownAlarm` |
| CPU alert details | `ConsulCPULoadAlarm` | `## ConsulCPULoadAlarm` |
| Memory alert details | `ConsulMemoryUsageAlarm` | `## ConsulMemoryUsageAlarm` |

When several rows match, read the more specific section first: start failure (`Consul Server Pod Does Not Start`), then disk or memory, then quorum (`No Cluster Leader`), then ACL or backup, then degraded or down.

## Assessment

Report the section header, the evidence that selected it, the impact stated in that section, and the resolution steps from that section only. Say which check the ticket, pipeline log, or cluster could not answer.
