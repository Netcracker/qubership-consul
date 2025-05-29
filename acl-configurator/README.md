Qubership Consul Access Control List Configurator
===========================

# Overview
Consul Access Control List Configurator aka Consul ACL Configurator is a service which allows create, update policies, roles 
and rule bindings for Consul. Get to know Consul ACL concept and Consul ACL entities you can [here](https://www.consul.io/docs/acl).

Basic flow is following - Consul ACL Configurator provides Kubernetes custom resource definition (crd) "consulacls", a client service
implements appropriate Kubernetes custom resource as yaml configuration file and applies it, Consul ACL Configurator read the configuration,
tries apply it for Consul and writes results to cr status. There is REST end point to retry install all exist ACL crs to Consul for 
privilege services (for example Consul backup daemon). The distribution consists of the two docker images and one helm chart.

# Documentation

- [Development Guide](../docs/public/acl-configurator)