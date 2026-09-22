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

sys.path.append('./tests/shared/lib')
from PlatformLibrary import PlatformLibrary

environ = os.environ
namespace = environ.get("CONSUL_NAMESPACE")
service = environ.get("CONSUL_HOST")
backup_daemon = environ.get("CONSUL_BACKUP_DAEMON_HOST")
timeout = 500


def stateful_set_rolled_out(stateful_set):
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
    status = deployment.status
    desired = deployment.spec.replicas or 0
    if desired == 0:
        return False
    if (status.observed_generation or 0) < (deployment.metadata.generation or 0):
        return False
    return (status.updated_replicas or 0) == desired and (status.ready_replicas or 0) == desired


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
            backup_daemon_ready = backup_daemon is None or deployment_rolled_out(
                k8s_library.get_deployment_entity(backup_daemon, namespace))
        except:
            time.sleep(10)
            continue
        if consul_ready and backup_daemon_ready:
            time.sleep(60)
            exit(0)
        time.sleep(10)
    exit(1)
