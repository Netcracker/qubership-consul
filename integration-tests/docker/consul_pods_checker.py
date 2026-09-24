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
deployments = [name for name in (environ.get("CONSUL_BACKUP_DAEMON_HOST"),
                                 environ.get("CONSUL_ACL_CONFIGURATOR_HOST")) if name]
timeout = 500


def components_ready(k8s_library):
    if not k8s_library.is_stateful_set_rolled_out(service, namespace):
        return False
    for name in deployments:
        if not k8s_library.is_deployment_rolled_out(name, namespace, "app.kubernetes.io/name"):
            return False
    return True


if __name__ == '__main__':
    try:
        k8s_library = PlatformLibrary()
    except:
        exit(1)
    timeout_start = time.time()
    while time.time() < timeout_start + timeout:
        try:
            ready = components_ready(k8s_library)
        except:
            time.sleep(10)
            continue
        if ready:
            exit(0)
        time.sleep(10)
    exit(1)
