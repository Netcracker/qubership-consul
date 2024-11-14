This section describes monitoring Grafana dashboards, metrics and their significance.

## Consul Monitoring

You can configure the following parameters on a Consul dashboard:

* Cloud
* Project
* Datacenter
* Consul fullname
* Sampling
* Interval time for metric display

For all graph panels, the mean metric value is used in the given interval. For all `singlestat` panels, the last metric value is used.

## Dashboard

An overview of the Consul dashboard is shown in the image below.

![Consul Dashboard](/docs/public/images/consul-monitoring_dashboard.png)

## Metrics

**Cluster Overview**

![Dashboard](/docs/public/images/consul-monitoring_cluster-overview.png)

`Consul Cluster Health` - Displays the overall health of the local server cluster. The following cluster states are displayed:

* `UP` - All servers are healthy.
* `DEGRADED` - Some servers are unhealthy.
* `DOWN` - Consul is unavailable.

`Leader Last Contact` - Measures the time since the leader was last able to contact the follower nodes when checking its leader lease.
If this metric is greater than 200ms, you should investigate possible network issues between the Consul servers. 
Another possible cause could be that the Consul servers are unable to keep up with the transaction load.
  
`Consul Servers` - Displays the health status of each server and information about which server is the leader.

`Servers Count` - Displays the current and expected number of servers in the Consul cluster. 

`Leadership Changes` - Displays the counters which are incremented when a Consul server starts an election process.
Normally this panel should be empty, because in a healthy environment, Consul Cluster should have a stable leader. 
There should not be any leadership changes unless you manually change leadership. For example, by taking a server out of the cluster 
or redeploy.
If there are unexpected elections or leadership changes, you should investigate possible network issues between the Consul servers. 
Another possible cause could be that the Consul servers are unable to keep up with the transaction load. 
So any activity on this panel means that leadership election is taking place.
   
`KV Store Timing` - Measures the time, in milliseconds, it takes to complete an update to the KV store.
This metric indicates how long it takes to complete write operations in Consul Key/Value Storage. 
Generally, it should remain reasonably consistent and no more than a few milliseconds. 
Sudden changes in the timing values could be due to unexpected load on the Consul servers 
or due to problems on the hosts themselves. Specifically, if any of these metrics deviate more than 50% from the baseline 
over the previous hour, this indicates an issue.

`Raft Transactions` - Displays the number of Raft transactions occurring over the interval.

`Raft Log Commit Time` - Measures the time it takes to commit a new entry to the Raft log on the leader.
These metrics indicate how long it takes to complete write operations in Raft from the Consul server. 
Generally, these values should remain reasonably consistent and no more than a few milliseconds each. 
Sudden changes in the timing values could be due to unexpected load on the Consul servers 
or due to problems on the hosts themselves. Specifically, if any of these metrics deviate more than 50% from the baseline 
over the previous hour, this indicates an issue.
  
**Note**: Raft is a consensus algorithm which is used by Consul to provide consistency. 
Raft is a complex protocol, for more information refer to the _Official Consul Documentation_.

**CPU Metrics**

![Dashboard](/docs/public/images/consul-monitoring_cpu-metrics.png)

`Server Container CPU Usage` - Displays the CPU used by server containers and limits that show the maximum number of CPUs
the container can use.
A spike in CPU usage could indicate too many operations taking place at the same time.
  
**Memory Metrics**

Consul keeps all of its data such as the KV store, the catalog and so on in memory. 
If Consul consumes all the available memory, it may crash. 
You should monitor total available RAM to make sure some RAM is available for other system processes.

![Dashboard](/docs/public/images/consul-monitoring_memory-metrics.png)

`Server Container Memory Usage` - Displays the memory used by server containers and limits that show the maximum number
of memory the container can use.
Consul servers are running low on memory if the value exceeds 90% of the limit.

`Consul Process Memory Usage` - Displays the number of bytes allocated by the Consul process.

**GC Metrics**

![Dashboard](/docs/public/images/consul-monitoring_gc-metrics.png)

`GC Pause` - Displays the number of nanoseconds consumed by stop-the-world garbage collection (GC) pauses since Consul started.
Garbage collection (GC) pauses are a "stop-the-world" event, all runtime threads are blocked until GC completes. 
In a healthy environment these pauses should only last a few nanoseconds. 
If memory usage is high, the Go runtime may start the GC process so frequently that it slows down the Consul. 
You might observe more frequent leader elections or longer write times.
If the value return is more than 2 seconds/minute, you should start investigating the cause. 
If it exceeds 5 seconds per minute, you should consider the cluster to be in a critical state.

`GC Runs Rate` - The number of stop-the-world garbage collection (GC) runs  since Consul started

`Runtime Heap Objects` - Consul runtime heap objects count.
  
**Disk Metrics**

![Dashboard](/docs/public/images/consul-monitoring_disk-metrics.png)

`Disk Volume` - Disk Volume stats.

`Raft Commit Rate` - Rate new entries to the Raft log on the leader.

`Raft Commit Time` - Measures the time it takes to commit a new entry to the Raft log on the leader.

`Container FS Reads Rate` - Container FS reads rate.

`Container FS Writes Rate` - Container FS writes rate.

`Container FS Reads Bytes Rate` - Container FS reads bytes rate.

`Container FS Writes Bytes Rate` - Container FS writes bytes rate.

**KV**

![Dashboard](/docs/public/images/consul-monitoring-kv-metrics.png)

`KV Applies Rate` - Shows the rate of KV operations in interval buckets. 

`KV Apply Time` - This measures the time it takes to complete an update to the KV store.

`Transaction Apply Time` - Measures the time spent applying transaction operations.

`KV entries` - KV entries.

**ACL**

![Dashboard](/docs/public/images/consul-monitoring-acl-metrics1.png)

![Dashboard](/docs/public/images/consul-monitoring-acl-metrics2.png)

![Dashboard](/docs/public/images/consul-monitoring-acl-metrics3.png)

`ACL Login Rate` - Counts the number of times we log in with ACL token by 5m. 

`ACL Logout Rate` - Counts the number of times we log out with ACL token by 5m.

`ACL Token Resolves Rate` - Counts the number of times we've resolves an ACL token by 5m.

`ACL Token Resolves` - Counts the number of times we've resolves an ACL token.

`ACL Token Upserts Rate` - Counts the number of times we've upsert an ACL token by 5m.

`ACL Token Upserts` - Counts the number of times we've upsert an ACL token.

`ACL Authmethod Upsets Rate` - Counts the number of times we've upsets an ACL auth method by 5m.

`ACL Authmethod Upsets` - Counts the number of times we've upsets an ACL auth method.

`ACL Bindingrule Upsets Rate` - Counts the number of times we've upsets an ACL binding rule by 5m.

`ACL Bindingrule Upsets` - Counts the number of times we've upsets an ACL binding rule.

`ACL Role Upsets Rate` - Counts the number of times we've upsets an ACL role by 5m.

`ACL Role Upsets` - Counts the number of times we've upsets an ACL role.

`ACL Policy Upsets Rate` - Counts the number of times we've upsets an ACL policy by 5m.

`ACL Policy Upsets` - Counts the number of times we've upsets an ACL policy.

**API HTTP**

![Dashboard](/docs/public/images/consul-monitoring-api-http-metrics.png)

`Consul API HTTP Rate By Pod` - Consul API HTTP rate by pod.

`Consul API HTTP Rate By Path` - Consul API HTTP rate by path.

**Catalog**

![Dashboard](/docs/public/images/consul-monitoring-catalog-metrics.png)

`Catalog Operations Rate` - This shows the rate of increase for catalog register and deregister operations.

`Catalog Operation Time` - Measures the time it takes to complete catalog register or deregister operations.

### Monitoring Alarms Description

This section describes Prometheus monitoring alarms.

| Name                    | Summary                                                       | For | Severity | Expression Example                                                                                                                                                                                                                                                                    | Description                                    | Troubleshooting Link                                                               |
|-------------------------|---------------------------------------------------------------|-----|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------|------------------------------------------------------------------------------------|
| ConsulDoesNotExistAlarm | There are no Consul server pods in namespace                  | 3m  | high     | `absent(kube_pod_status_ready{exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) > 0`                                                                                                                                                                         | There are no Consul server pods in namespace   | [ConsulDoesNotExistAlarm](/docs/public/troubleshooting.md#consuldoesnotexistalarm) |
| ConsulIsDegradedAlarm   | Some of Consul server pods are down                           | 3m  | high     | `sum(kube_pod_status_ready{condition="false",exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) / sum(kube_pod_status_ready{exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) > 0`                                                   | Consul is Degraded                             | [ConsulIsDegradedAlarm](/docs/public/troubleshooting.md#consulisdegradedalarm)     |
| ConsulIsDownAlarm       | All of the Consul server nodes have failed                    | 3m  | critical | `sum(kube_pod_status_ready{condition="false",exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) / sum(kube_pod_status_ready{exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) == 1`                                                  | Consul is Down                                 | [ConsulIsDownAlarm](/docs/public/troubleshooting.md#consulisdownalarm)             |
| ConsulCPULoadAlarm      | Some of Consul server pods load CPU higher than 95 percents   | 3m  | high     | `max(rate(container_cpu_usage_seconds_total{container="consul",namespace="consul-service",pod=~"consul-server-[0-9]+"}[1m])) / max(kube_pod_container_resource_limits_cpu_cores{container="consul",exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) > 0.95` | Consul CPU load is higher than 95 percents     | [ConsulCPULoadAlarm](/docs/public/troubleshooting.md#consulcpuloadalarm)           |
| ConsulMemoryUsageAlarm  | Some of Consul server pods use memory higher than 95 percents | 3m  | high     | `max(container_memory_working_set_bytes{container="consul",namespace="consul-service",pod=~"consul-server-[0-9]+"}) / max(kube_pod_container_resource_limits_memory_bytes{container="consul",exported_namespace="consul-service",exported_pod=~"consul-server-[0-9]+"}) > 0.95`       | Consul memory usage is higher than 95 percents | [ConsulMemoryUsageAlarm](/docs/public/troubleshooting.md#consulmemoryusagealarm)   |
