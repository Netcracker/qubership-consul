# Qubership Consul

Qubership Consul is a comprehensive solution for deploying HashiCorp Consul in Kubernetes with High Availability (HA), Disaster Recovery (DR), and Multi-AZ setups. 
Includes tools for backup management, monitoring, ACL configuration, and integration testing to ensure reliable operation and security. 
Designed for creating resilient and secure Consul clusters in a cloud-native environment.

## Repository structure

* `./charts` - directory with main HELM chart for Consul and integration tests.
* `./disasterrecovery` - directory with disaster-recovery microservice source code.
* `./docs` - directory with actual documentation for the component.
* `./integration-tests` - directory with Robot Framework test cases for Consul.

## Useful links

* [Installation guide](/docs/public/installation.md).
* [Troubleshooting guide](/docs/public/troubleshooting.md).

## License

* Main part is distributed under `Apache License, Version 2.0`.
* Folder `charts/helm/consul-service` is distributed under `Mozilla Public License, Version 2.0`.
