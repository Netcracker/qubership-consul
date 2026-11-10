# Disk filled on all nodes

This section describes the problem detection techniques for issues with filled disks.

## Metric

You can use information about mounted volumes that is stored in `t_disk` measurement with fields:

* `used`
* `used_percent`

`t_disk` is a Heapster measurement, so to get it Heapster should be installed and configured for
current environment.

To retrieve information about how much disk space Consul is using, run the following command
inside Consul server pod:

```sh
df -h /consul/data
```

Possible output is as follows:

```text
Filesystem                Size      Used Available Use% Mounted on
kube06nc-nfs.example.com:/export/pvc-cbc1dc0b-e10c-4a70-bf69-3dd4dd3e8101
                          1.0G     19.0M   1005.0M   2% /consul/data
```

## Grafana Dashboard

To retrieve the metric of disk usage the following query can be used:

```sql
SELECT max("used_percent") FROM "t_disk" WHERE ("path" =~ /consul/) AND $timeFilter GROUP BY time($inter), "path" fill(null)
```

## Logging

When the problem occurs, you can see the following exception in the console logs:

```text
2020/07/29 16:03:46 [ERROR] raft: Failed to commit logs: no space left on device
```

## Troubleshooting Procedure

Because Consul persists all data to disk, it is necessary to monitor the amount of free disk space
available to Consul.
When the problem occurs, manually remove the outdated snapshots with the following command:

```sh
rm -r /consul/data/raft/snapshots/<snapshot_id>
```

where `<snapshot_id>` is the name of outdated snapshot. For example, `2-131111-1596611474407`.
Then you can clean up redundant data from Consul using Consul UI or Consul server pod console. Do not
forget to restart Consul service after all manipulations.
