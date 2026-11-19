# Predefined Persistent Volumes

If you have prepared Persistent Volumes without storage class and dynamic provisioning,
you can specify Persistent Volumes names using the `server.persistentVolumes` parameter.

For example:

```yaml
persistentVolumes:
  - pv-default-consul-service-server-1
  - pv-default-consul-service-server-2
  - pv-default-consul-service-server-3
```

Persistent Volumes should be created on corresponding Kubernetes nodes and should be in the `Available` state.

Set appropriate UID and GID on hostPath directories and rule for SELinux:

```sh
chown -R 100:1000 /mnt/data/<pv-name>
```

You also need to specify node names via `server.nodes` parameter in the same order in which the Persistent Volumes are
specified so that Consul pods are assigned to these nodes.

According to the specified parameters, the `Pod Scheduler` distributes pods to the necessary Kubernetes nodes.
