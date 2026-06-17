# CPU Overload

This section describes the problem detection techniques for issues with CPU overload.

## Metric

You can use information about used CPU that is stored in `cpu/usage_rate` measurement in field
`value` and information about CPU limit that is stored in `cpu/limit` measurement in field
`value`.

`cpu/usage_rate` and `cpu/limit` are Heapster measurements, so to get them Heapster should be
installed and configured for current environment.

These metrics show pod CPU usage and limit in bytes. Constant high CPU usage that is close
to CPU limit may indicate a critical situation that service is overloaded or resource limits
are too low. It can potentially lead to the increase of response times or crashes.

## Grafana Dashboard

To retrieve the metric of CPU usage the following query can be used:

```sql
SELECT max("value") FROM "cpu/usage_rate" WHERE ("namespace_name" =~ /^$project$/ AND "container_name" = 'consul' AND "pod_name" =~ /consul-server/) AND $timeFilter GROUP BY time($inter), "pod_name" fill(linear)
```

To retrieve the metric of CPU limit the following query can be used:

```sql
SELECT max("value") FROM "cpu/limit" WHERE ("namespace_name" =~ /^$project$/ AND "container_name" = 'consul' AND "pod_name" =~ /consul-server/) AND $timeFilter GROUP BY time($inter), "pod_name" fill(linear)
```

## Troubleshooting Procedure

If you see an increase in CPU usage, this is usually caused by high load on Consul. In this case you
can either increase CPU limit or add more Consul servers to redistribute the load. Also set up
a notification to find out if your Consul servers CPU usage is consistently increasing.
