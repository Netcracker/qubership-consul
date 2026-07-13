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
import os
import ssl
import urllib3
from kubernetes.client import ApiException
from kubernetes.stream import stream

SERVICE_ACCOUNT_CA = '/var/run/secrets/kubernetes.io/serviceaccount/ca.crt'

POD_SECRETS_DIR = '/etc/secrets/backup-daemon-pod-secrets'
ACL_TOKEN_FILE = 'CONSUL_HTTP_TOKEN'


def get_secret_value(key: str):
    secrets_dir = os.getenv('BACKUP_DAEMON_SECRETS_DIR', POD_SECRETS_DIR)
    if secrets_dir:
        path = os.path.join(secrets_dir, key)
        if os.path.isfile(path):
            with open(path, encoding='utf-8') as handle:
                value = handle.read().strip()
                if value:
                    return value
    return os.getenv(key)


def acl_token_path():
    return os.path.join(os.getenv('BACKUP_DAEMON_SECRETS_DIR', POD_SECRETS_DIR), ACL_TOKEN_FILE)


def read_acl_token_from_file():
    token = get_secret_value(ACL_TOKEN_FILE)
    return token or None


def _create_ssl_context(configuration):
    if not configuration.verify_ssl or not hasattr(ssl, 'VERIFY_X509_STRICT'):
        return None
    ca_file = configuration.ssl_ca_cert
    if not ca_file or not os.path.isfile(ca_file):
        ca_file = SERVICE_ACCOUNT_CA if os.path.isfile(SERVICE_ACCOUNT_CA) else None
    if not ca_file:
        return None
    ssl_context = ssl.create_default_context(cafile=ca_file)
    ssl_context.verify_flags &= ~ssl.VERIFY_X509_STRICT
    if configuration.cert_file and configuration.key_file:
        ssl_context.load_cert_chain(configuration.cert_file, configuration.key_file)
    return ssl_context


def _patch_api_client_ssl(api_client):
    configuration = api_client.configuration
    ssl_context = _create_ssl_context(configuration)
    if ssl_context is None:
        return api_client

    pool_kwargs = {
        'cert_reqs': ssl.CERT_REQUIRED,
        'ssl_context': ssl_context,
    }
    retries = getattr(configuration, 'retries', None)
    if retries is not None:
        pool_kwargs['retries'] = retries
    assert_hostname = getattr(configuration, 'assert_hostname', None)
    if assert_hostname is not None:
        pool_kwargs['assert_hostname'] = assert_hostname
    tls_server_name = getattr(configuration, 'tls_server_name', None)
    if tls_server_name:
        pool_kwargs['server_hostname'] = tls_server_name

    api_client.rest_client.pool_manager = urllib3.PoolManager(**pool_kwargs)
    return api_client


def get_kubernetes_api_client(config_file=None, context=None, persist_config=True):
    try:
        kubernetes.config.load_incluster_config()
        return _patch_api_client_ssl(kubernetes.client.ApiClient())
    except kubernetes.config.ConfigException:
        return _patch_api_client_ssl(kubernetes.config.new_client_from_config(
            config_file=config_file,
            context=context,
            persist_config=persist_config))


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
