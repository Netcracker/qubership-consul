This section provides information about backup and recovery procedures using API for Consul Service.

For POST operations you must specify the username and password from `BACKUP_DAEMON_API_CREDENTIALS_USERNAME` and
`BACKUP_DAEMON_API_CREDENTIALS_PASSWORD` environment variables so that you can use REST API to run backup tasks.

# Backup

You can back up data for Consul Service per your requirement. You can select any one of the following options for backup:
* Full manual backup
* Granular backup
* Not Evictable Backup

## Full Manual Backup

To back up data of all Consul Datacenters, run the following command:

```
curl -u username:password -XPOST http://localhost:8080/backup
```

After executing the command, you receive a name of the folder where the backup is stored. For example, `20190321T080000`.

## Granular Backup

To back up data for specified datacenters, you can specify them in the `dbs` parameter. For example:

```
curl -u username:password -XPOST -v -H "Content-Type: application/json" -d '{"dbs":["dc1","dc2"]}'  http://localhost:8080/backup
```

## Not Evictable Backup

If you do not want the backup to be evicted automatically, you need to add the `allow_eviction` property
with value as `False` in the request body. For example:

```
curl -u username:password -XPOST -v -H "Content-Type: application/json" -d '{"allow_eviction":"False"}' http://localhost:8080/backup
```

## Backup Eviction by ID

To remove a specific backup, run the following command:

```
curl -u username:password -XPOST http://localhost:8080/evict/<backup_id>
```

where `backup_id` is the name of specific backup to be evicted, for example, `20190321T080000`.
If the operation is successful, the following text displays: `Backup <backup_id> successfully removed`.

## Backup Status

When the backup is in progress, you can check its status using the following command:

```
curl -XGET http://localhost:8080/jobstatus/<backup_id>
```

where `backup_id` is the backup name received at the backup execution step. The result is JSON with
the following information:

* `status` - Status of operation. The possible options are:
  * Successful
  * Queued
  * Processing
  * Failed
* `message` - Description of error (optional field)
* `vault` - The name of vault used in recovery
* `type` - The type of operation. The possible options are:
  * backup
  * restore
* `err` - None if no error, last 5 lines of log if `status=Failed`
* `task_id` - The identifier of the task

## Backup Information

To get the backup information, use the following command:

```
curl -XGET http://localhost:8080/listbackups/<backup_id>
```

where `backup_id` is the name of specific backup. The command returns JSON string with data about
specific backup:

* `ts` - The UNIX timestamp of backup.
* `spent_time` - The time spent on backup (in ms)
* `db_list` - The list of stored datacenters (only for granular backup)
* `id` - The name of backup
* `size` - The size of backup in bytes
* `evictable` - Whether backup is evictable, _true_ if backup is evictable and _false_ otherwise
* `locked` - Whether backup is locked. _true_ if backup is locked (either process is not finished, or it failed somehow)
* `exit_code` - The exit code of backup script
* `failed` - Whether backup failed or not. _true_ if backup failed and _false_ otherwise
* `valid`- Backup is valid or not. _true_ if backup is valid and _false_ otherwise

# Recovery

To recover data from a specific backup, you need to specify JSON body with information about a backup name (`vault`).

You also can start a recovery with specifying datacenters (`dbs`). In this case only snapshots for specified datacenters are restored.

Backup is restored with Consul boostrap ACL token if the token is present in backup and Consul has ACLs enabled. So, there are two scenarios when restore of backup is failed by default:

* Consul is deployed with ACLs but backup does not contain Consul boostrap ACL token
* Consul is deployed without ACLs but backup contains Consul boostrap ACL token

Moreover, if we know that only key-value storage is changed, we don't want to reconfigure tokens, auth-methods, etc. and restart all dependent services.

The described behaviour can be processed by `skip_acl_recovery` parameter. If `skip_acl_recovery` parameter is set to `true`, Consul bootstrap ACL token restoration and all additional Consul security recovery steps are skipped.

**Important**: Add `skip_acl_recovery` parameter only if you are sure that you need the behaviour described above.

```
curl -u username:password -XPOST -v -H "Content-Type: application/json" -d '{"vault":"20190321T080000", "dbs":["dc1","dc2"], "skip_acl_recovery":"false"}' http://localhost:8080/restore
```

As a response, you receive `task_id`, which can be used to check _Recovery Status_.

## Recovery Status

When the recovery is in progress, you can check its status using the following command:

```
curl -XGET http://localhost:8080/jobstatus/<task_id>
```

where `task_id` is task ID received at the recovery execution step.

# Backups List

To receive list of collected backups, use the following command:

```
curl -XGET http://localhost:8080/listbackups
```

It returns JSON with list of backup names.

# Backup Daemon Health

To know the state of Backup Daemon, use the following command:

```
curl -XGET http://localhost:8080/health
```

You receive JSON with the following information:

```
"status": status of backup daemon   
"backup_queue_size": backup daemon queue size (if > 0 then there are 1 or tasks waiting for execution)
 "storage": storage info:
  "total_space": total storage space in bytes
  "dump_count": number of backup
  "free_space": free space left in bytes
  "size": used space in bytes
  "total_inodes": total number of inodes on storage
  "free_inodes": free number of inodes on storage
  "used_inodes": used number of inodes on storage
  "last": last backup metrics
    "metrics['exit_code']": exit code of script 
    "metrics['exception']": python exception if backup failed
    "metrics['spent_time']": spent time
    "metrics['size']": backup size in bytes
    "failed": is failed or not
    "locked": is locked or not
    "id": vault name of backup
    "ts": timestamp of backup  
  "lastSuccessful": last successfull backup metrics
    "metrics['exit_code']": exit code of script 
    "metrics['spent_time']": spent time
    "metrics['size']": backup size in bytes
    "failed": is failed or not
    "locked": is locked or not
    "id": vault name of backup
    "ts": timestamp of backup
```