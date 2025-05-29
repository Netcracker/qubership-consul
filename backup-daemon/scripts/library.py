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

import kubernetes
import urllib3
from kubernetes.client import ApiException
from kubernetes.stream import stream


def get_kubernetes_api_client(config_file=None, context=None, persist_config=True):
    try:
        kubernetes.config.load_incluster_config()
        return kubernetes.client.ApiClient()
    except kubernetes.config.ConfigException:
        return kubernetes.config.new_client_from_config(config_file=config_file,
                                                        context=context,
                                                        persist_config=persist_config)


class KubernetesLibrary(object):

    def __init__(self,
                 namespace: str,
                 config_file=None,
                 context=None,
                 persist_config=True):
        urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

        self.k8s_api_client = get_kubernetes_api_client(config_file=config_file,
                                                        context=context,
                                                        persist_config=persist_config)
        self.k8s_apps_v1_client = kubernetes.client.AppsV1Api(self.k8s_api_client)
        self.k8s_core_v1_client = kubernetes.client.CoreV1Api(self.k8s_api_client)
        self.namespace = namespace

    @staticmethod
    def _do_labels_satisfy_selector(labels: dict, selector: dict):
        selector_pairs = list(selector.items())
        label_pairs = list(labels.items())
        if len(selector_pairs) > len(label_pairs):
            return False
        for pair in selector_pairs:
            if pair not in label_pairs:
                return False
        return True

    def get_pods(self) -> list:
        return self.k8s_core_v1_client.list_namespaced_pod(self.namespace).items

    def get_pods_by_selector(self, selector: dict) -> list:
        label_selector = ",".join([f"{key}={value}" for key, value in selector.items()])
        return self.k8s_core_v1_client.list_namespaced_pod(self.namespace, label_selector=label_selector).items

    def delete_pod(self, name: str, grace_period=10):
        self.k8s_core_v1_client.delete_namespaced_pod(name, self.namespace, grace_period_seconds=grace_period)

    def delete_pods_by_selector(self, selector: dict):
        for pod in self.get_pods():
            if self._do_labels_satisfy_selector(pod.metadata.labels, selector):
                self.delete_pod(pod.metadata.name)

    def get_secret(self, name: str) -> {}:
        return self.k8s_core_v1_client.read_namespaced_secret(name, self.namespace)

    def patch_secret(self, name: str, body: {}):
        self.k8s_core_v1_client.patch_namespaced_secret(name, self.namespace, body)

    def get_service_account_secrets(self, name: str) -> list:
        service_account = self.k8s_core_v1_client.read_namespaced_service_account(name, self.namespace)
        return service_account.secrets

    def execute_command_in_pod(self, name: str, command: str, container: str):
        exec_cmd = ['/bin/sh', '-c', command]
        try:
            response = stream(self.k8s_core_v1_client.connect_get_namespaced_pod_exec,
                              name,
                              self.namespace,
                              container=container,
                              command=exec_cmd,
                              stderr=True,
                              stdin=False,
                              stdout=True,
                              tty=False,
                              _preload_content=False)
        except ApiException as e:
            return "", e.reason

        result = ""
        errors = ""
        while response.is_open():
            response.update(timeout=2)
            if response.peek_stdout():
                value = str(response.read_stdout())
                result += value
            if response.peek_stderr():
                error = response.read_stderr()
                errors += error
        return result.strip(), errors.strip()
