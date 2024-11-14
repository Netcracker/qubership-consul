# Memory limit

This section describes the problem detection techniques for issues with memory limit.

## Metric

You can use information about used memory that is stored in `memory/usage` measurement in field
`value` and information about memory limit that is stored in `memory/limit` measurement in field
`value`.

`memory/usage` and `memory/limit` are Heapster measurements, so to get them Heapster should be
installed and configured for current environment.

These metrics show pod memory usage and limit in bytes. Constant high memory usage that is close
to memory limit may indicate a critical situation that service is overloaded or resource limits
are too low. It can potentially lead to the increase of response times or crashes.

## Grafana Dashboard

To retrieve the metric of memory usage the following query can be used:

```sql
SELECT max("value") FROM "memory/usage" WHERE ("namespace_name" =~ /^$project$/ AND "container_name" = 'consul' AND "pod_name" =~ /consul-server/) AND $timeFilter GROUP BY time($inter), "pod_name" fill(linear)
```

To retrieve the metric of memory limit the following query can be used:

```sql
SELECT max("value") FROM "memory/limit" WHERE ("namespace_name" =~ /^$project$/ AND "container_name" = 'consul' AND "pod_name" =~ /consul-server/) AND $timeFilter GROUP BY time($inter), "pod_name" fill(linear)
```

## Troubleshooting Procedure

If you see a high memory usage, you can either increase memory limit or scale out the cluster by adding more nodes.
