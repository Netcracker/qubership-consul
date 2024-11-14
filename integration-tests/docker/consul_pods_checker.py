import os
import sys
import time

sys.path.append('./tests/shared/lib')
from PlatformLibrary import PlatformLibrary

environ = os.environ
namespace = environ.get("CONSUL_NAMESPACE")
service = environ.get("CONSUL_HOST")
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
            for pod in all_pods_in_project:
                if pod.metadata.labels.get('name') == service and pod.status.container_statuses[0].ready:
                    ready_pods += 1
        except:
            time.sleep(10)
            continue
        if desired_pods == ready_pods:
            time.sleep(60)
            exit(0)
        time.sleep(10)
    exit(1)
