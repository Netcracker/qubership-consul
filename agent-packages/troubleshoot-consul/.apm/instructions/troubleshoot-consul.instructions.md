---
description: >-
  Diagnostic playbook for Consul server, quorum, CPU, memory, disk, ACL, backup,
  and Velero restore failures. Never invoke automatically — only when explicitly
  requested by the user.
applyTo: "**/*"
---

When diagnosing a Consul problem — a degraded or down cluster, no cluster
leader, CPU, memory, or full data volume, a server or client pod that does not
start, ACL reset, a backup daemon failure, or a failed Velero restore —
apply the `troubleshoot-consul` skill.
