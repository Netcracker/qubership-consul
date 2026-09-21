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
# Optional: only set (in the test-runner env) when the backup daemon is enabled.
# The backup daemon is a Deployment, so it is gated by pod readiness rather than
# by the StatefulSet replicas count used for the Consul servers.
backup_daemon = environ.get("CONSUL_BACKUP_DAEMON_HOST")
timeout = 500

if __name__ == '__main__':
    try:
        k8s_library = PlatformLibrary()
    except:
        exit(1)
    timeout_start = time.time()
    while time.time() < timeout_start + timeout:
        try:
            desired_pods = k8s_library.get_stateful_set_replicas_count(service, namespace)
            all_pods_in_project = k8s_library.get_pods(namespace)
            ready_pods = 0
            # When the backup daemon is disabled there is nothing to wait for.
            backup_daemon_ready = backup_daemon is None
            for pod in all_pods_in_project:
                name_label = pod.metadata.labels.get('name')
                if name_label == service and pod.status.container_statuses[0].ready:
                    ready_pods += 1
                elif backup_daemon and name_label == backup_daemon and pod.status.container_statuses[0].ready:
                    backup_daemon_ready = True
        except:
            time.sleep(10)
            continue
        if desired_pods == ready_pods and backup_daemon_ready:
            time.sleep(60)
            exit(0)
        time.sleep(10)
    exit(1)
