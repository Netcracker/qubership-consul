# Consul Helm Chart

This Helm chart is originally based o [HashiCorp Consul Helm chart](https://github.com/hashicorp/consul-k8s) and is distributed under `Mozilla Public License, version 2.0`.

## Prerequisites

Setting up Kubernetes and Helm and is outside the scope
of this README. Please refer to the Kubernetes and Helm documentation.

The versions required are:

* **Helm 3+** - This is the earliest version of Helm tested. It is possible
  it works with earlier versions but this chart is untested for those versions.
* **Kubernetes 1.29+** - This is the earliest version of Kubernetes tested.
  It is possible that this chart works with earlier versions but it is
  untested.

## Usage


Assuming this repository was unpacked into the directory `consul-service`, the chart can
then be installed directly:

    helm install ./ -f example.yaml

Please see the many options supported in the `values.yaml`
file. 
