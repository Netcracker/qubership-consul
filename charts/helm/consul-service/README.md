# Consul Helm Chart

This Helm chart is originally based o [HashiCorp Consul Helm chart](https://github.com/hashicorp/consul-k8s) and is distributed under `Mozilla Public License, version 2.0`.

## Prerequisites

To use the charts here, [Helm](https://helm.sh/) must be installed in your
Kubernetes cluster. Setting up Kubernetes and Helm and is outside the scope
of this README. Please refer to the Kubernetes and Helm documentation.

The versions required are:

  * **Helm 2.10+** - This is the earliest version of Helm tested. It is possible
    it works with earlier versions but this chart is untested for those versions.
  * **Kubernetes 1.29+** - This is the earliest version of Kubernetes tested.
    It is possible that this chart works with earlier versions but it is
    untested. Other versions verified are Kubernetes 1.10, 1.11.

## Usage

For now, we do not host a chart repository. To use the charts, you must
download this repository and unpack it into a directory. Either
[download a tagged release](https://github.com/hashicorp/consul-helm/releases) or
use `git checkout` to a tagged release.

Before installing, you may need to create docker credentials secret for `ghcr.io` registry.
```bash
kubectl create secret docker-registry github-registry --docker-server=ghcr.io --docker-username=<user_name> --docker-password=<token> --docker-email=<user_email> -n <namespace>
```

Assuming this repository was unpacked into the directory `consul-helm`, the chart can
then be installed directly:

    helm install ./ -f example.yaml

Please see the many options supported in the `values.yaml`
file. These are also fully documented directly on the
[Consul website](https://www.consul.io/docs/platform/k8s/helm.html).
