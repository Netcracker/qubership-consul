# Copyright 2024-2025 NetCracker Technology Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import sys
import time

import requests

sys.path.append('./tests/shared/lib')
from PlatformLibrary import PlatformLibrary

environ = os.environ
namespace = environ.get("CONSUL_NAMESPACE")
service = environ.get("CONSUL_HOST")
# Optional: only set (in the test-runner env) when the backup daemon is enabled.
# The backup daemon is a Deployment, so it is gated by its own rollout status
# rather than by the StatefulSet status used for the Consul servers.
backup_daemon = environ.get("CONSUL_BACKUP_DAEMON_HOST")
# CA bundle for talking to Consul over TLS (mounted only when TLS is enabled).
CONSUL_CA_CERT_PATH = '/consul/tls/ca/tls.crt'
timeout = 500


def stateful_set_rolled_out(stateful_set):
    """True when the StatefulSet is fully rolled out and every replica is Ready.

    Mirrors ``kubectl rollout status statefulset``: the controller has observed
    the current spec, the update has converged onto a single (latest) revision,
    and all desired replicas are both updated to that revision and Ready.

    This is what makes the check correct during a rolling upgrade. Counting Ready
    pods is not enough: the old-revision pods stay Ready while the new ones roll
    out, so a plain ``ready == desired`` comparison passes against a cluster that
    has not been upgraded yet. Requiring ``updatedReplicas`` and revision
    convergence closes that gap.

    It also gives us "Consul is ready" for free: the Consul server readiness probe
    only passes once the pod sees an elected leader (``/v1/status/leader``), so
    "all replicas Ready" means the cluster itself is serving.
    """
    status = stateful_set.status
    desired = stateful_set.spec.replicas or 0
    if desired == 0:
        return False
    if (status.observed_generation or 0) < (stateful_set.metadata.generation or 0):
        return False
    if status.update_revision and status.current_revision != status.update_revision:
        return False
    return (status.updated_replicas or 0) == desired and (status.ready_replicas or 0) == desired


def deployment_rolled_out(deployment):
    """True when a Deployment rollout is complete and every replica is Ready."""
    status = deployment.status
    desired = deployment.spec.replicas or 0
    if desired == 0:
        return False
    if (status.observed_generation or 0) < (deployment.metadata.generation or 0):
        return False
    return (status.updated_replicas or 0) == desired and (status.ready_replicas or 0) == desired


def consul_has_leader():
    """True when Consul reports an elected leader via /v1/status/leader.

    Explicit cluster-level confirmation on top of pod readiness: the endpoint
    returns the leader's address as a quoted string (e.g. "10.0.0.1:8300"), or an
    empty "" while there is no leader. No ACL token is required for this endpoint.
    Any transport error is treated as "not ready yet".
    """
    scheme = environ.get("CONSUL_SCHEME", "http")
    host = environ.get("CONSUL_HOST")
    port = environ.get("CONSUL_PORT")
    verify = True
    if scheme == "https":
        verify = CONSUL_CA_CERT_PATH if os.path.exists(CONSUL_CA_CERT_PATH) else False
    try:
        response = requests.get(f"{scheme}://{host}:{port}/v1/status/leader",
                                verify=verify, timeout=10)
    except requests.exceptions.RequestException:
        return False
    return response.status_code == 200 and response.text.strip().strip('"') != ""


if __name__ == '__main__':
    try:
        k8s_library = PlatformLibrary()
    except:
        exit(1)
    timeout_start = time.time()
    while time.time() < timeout_start + timeout:
        try:
            consul_ready = stateful_set_rolled_out(
                k8s_library.get_stateful_set(service, namespace))
            # The env var is only set when the backup daemon is enabled; when it
            # is disabled there is nothing to wait for.
            backup_daemon_ready = backup_daemon is None or deployment_rolled_out(
                k8s_library.get_deployment_entity(backup_daemon, namespace))
        except:
            time.sleep(10)
            continue
        # Only query Consul once the servers have rolled out and their pods are
        # Ready, so the service DNS name resolves and the call is meaningful.
        if consul_ready and backup_daemon_ready and consul_has_leader():
            time.sleep(60)
            exit(0)
        time.sleep(10)
    exit(1)
