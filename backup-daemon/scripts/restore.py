#!/usr/bin/python
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

import argparse
import ast
import base64
import logging
import os
import sys
import time

import requests

from library import KubernetesLibrary

REQUEST_HEADERS = {
    'Accept': 'application/json',
    'Content-type': 'application/json'
}

TLS_CRT_PATH = '/consul/tls/ca/tls.crt'
CA_CERT_PATH = '/consul/tls/ca/ca.crt'

loggingLevel = logging.DEBUG if os.getenv(
    'CONSUL_BACKUP_DAEMON_DEBUG') else logging.INFO
logging.basicConfig(level=loggingLevel,
                    format='[%(asctime)s,%(msecs)03d][%(levelname)s][category=Restore] %(message)s',
                    datefmt='%Y-%m-%dT%H:%M:%S')


class Restore:

    def __init__(self, storage_folder):
        consul_host = os.getenv("CONSUL_HOST")
        consul_port = os.getenv("CONSUL_PORT")
        consul_namespace = os.getenv("CONSUL_NAMESPACE")
        consul_scheme = os.getenv("CONSUL_SCHEME", "http")
        if not consul_host or not consul_port:
            logging.error("Consul service name or port isn't specified.")
            sys.exit(1)

        self._acl_enabled = True if os.getenv('CONSUL_HTTP_TOKEN') else False
        self._consul_fullname = os.getenv("CONSUL_FULLNAME")
        self._consul_url = f'{consul_scheme}://{consul_host}:{consul_port}'
        self._storage_folder = storage_folder
        self._consul_cafile = CA_CERT_PATH if os.path.exists(CA_CERT_PATH) \
            else TLS_CRT_PATH if os.path.exists(TLS_CRT_PATH) else None

        self.library = KubernetesLibrary(consul_namespace)

        if self._acl_enabled:
            secret = self.library.get_secret(f'{self._consul_fullname}-bootstrap-acl-token')
            data = secret.data.get("token")
            REQUEST_HEADERS['X-Consul-Token'] = base64.decodebytes(data.encode()).decode()

    def restore(self, folder, datacenters=None, skip_acl_recovery=False):
        snapshot_token_exists = os.path.exists(f'{folder}/.token')
        if not skip_acl_recovery:
            if self._acl_enabled and not snapshot_token_exists:
                logging.error('There is an attempt to restore Consul with ACL from snapshot without ACL')
                sys.exit(1)
            if not self._acl_enabled and snapshot_token_exists:
                logging.error('There is an attempt to restore Consul without ACL from snapshot with ACL')
                sys.exit(1)

        if not datacenters:
            logging.debug("Datacenters are not specified, full restore for each datacenter will be performed")
            datacenters = [f.name for f in os.scandir(folder) if f.is_dir()]

        logging.info(f'Perform restore of snapshot for datacenters {datacenters}')
        for datacenter in datacenters:
            restore_resp = requests.put(f'{self._consul_url}/v1/snapshot',
                                        params={'dc': datacenter},
                                        headers=REQUEST_HEADERS,
                                        data=open(f'{folder}/{datacenter}/snapshot.gz', 'rb'),
                                        verify=self._consul_cafile)
            if not restore_resp.ok:
                logging.error(
                    f'There is problem with restoring snapshot for datacenter: {datacenter}, details: {restore_resp.text}')
                sys.exit(1)
            logging.info(f'Snapshot for datacenter "{datacenter}" restored successfully.')

        if snapshot_token_exists and not skip_acl_recovery:
            token = self.restore_secret_data(f'{self._consul_fullname}-bootstrap-acl-token', 'token',
                                             f'{folder}/.token')
            self.recover_consul(token)
        logging.info(f'Snapshot for datacenters {datacenters} restored successfully.')

    def restore_secret_data(self, secret_name, secret_key, file_path_to_restore) -> str:
        secret = self.library.get_secret(secret_name)
        actual_data = secret.data.get(secret_key)
        with open(file_path_to_restore, 'r') as file_to_restore:
            data_to_restore = file_to_restore.read()
        REQUEST_HEADERS['X-Consul-Token'] = base64.decodebytes(data_to_restore.encode()).decode()
        if actual_data != data_to_restore:
            logging.info(f'Recovery of "{secret_name}" secret is needed because secret data has been changed')
            secret.data[secret_key] = data_to_restore
            self.library.patch_secret(secret_name, secret)
            return data_to_restore
        logging.info(f'Recovery of "{secret_name}" secret is not needed')
        return ''

    def recover_consul(self, token: str):
        logging.info('Consul recovery is started')
        if token:
            self.library.delete_pods_by_selector({'name': f'{self._consul_fullname}-server', 'component': 'server'})
            self.check_consul_leader()
        self.update_server_tokens()
        self.check_consul_leader()
        self.update_auth_methods()
        self.restart_incorrect_servers()
        self.check_consul_leader()
        self.library.delete_pods_by_selector({'restore-policy': 'restart'})

    def check_consul_leader(self, timeout=120):
        timeout_start = time.time()
        while time.time() < timeout_start + timeout:
            logging.info('Try to get Consul leader')
            time.sleep(5)
            try:
                response = requests.get(f'{self._consul_url}/v1/status/leader',
                                        headers=REQUEST_HEADERS,
                                        verify=self._consul_cafile,
                                        timeout=10)
            except Exception as e:
                logging.error(f'Consul has errors when receiving leader: {e}')
                continue
            if not response.ok:
                logging.info(f'Consul has problems when receiving leader: {response.status_code}, {response.content}')
                continue
            leader = response.content.decode()
            if leader and leader != "\"\"":
                logging.info(f'Consul leader is {leader}')
                return
        logging.error(f'Consul does not have a leader after {timeout} seconds')
        sys.exit(1)

    # Implement the same approach as in the Consul:
    # https://github.com/hashicorp/consul-k8s/blob/v1.2.2/control-plane/subcommand/server-acl-init/servers.go#L118
    def calculate_server_tokens(self, response: requests.Response) -> dict:
        tokens = response.json()
        server_tokens = {}
        for server in self.library.get_pods_by_selector(
                {'name': f'{self._consul_fullname}-server', 'component': 'server'}):
            secret_id = ""
            description = f'Server Token for {server.status.pod_ip}'
            for token in tokens:
                if token.get('Description') == description:
                    secret_id = token.get('SecretID')
                    logging.info(f'Token for "{description}" is found')
                    break
            if not secret_id:
                logging.info(f'Token for "{description}" is not found, creating new one')
                secret_id = self.create_server_token(description)
            server_tokens[server.metadata.name] = secret_id
        return server_tokens

    def update_server_tokens(self, timeout=180):
        logging.info('The update of Consul server tokens is started')
        timeout_start = time.time()
        while time.time() < timeout_start + timeout:
            time.sleep(5)
            try:
                response = requests.get(f'{self._consul_url}/v1/acl/tokens',
                                        headers=REQUEST_HEADERS,
                                        verify=self._consul_cafile,
                                        timeout=30)
            except Exception as e:
                logging.error(f'Consul has errors with receiving ACL tokens: {e}')
                continue
            if not response.ok:
                logging.info(f'Consul has problems with receiving tokens: {response.status_code}, {response.text}')
                continue
            server_tokens = self.calculate_server_tokens(response)
            for server, token in server_tokens.items():
                result, errors = self.library.execute_command_in_pod(
                    server, f'consul acl set-agent-token -token {REQUEST_HEADERS["X-Consul-Token"]} agent {token}',
                    'consul')
                if errors:
                    logging.error(f'Setting agent token: result - [{result}], errors - [{errors}]')
                    continue
                result, errors = self.library.execute_command_in_pod(
                    server, f'consul leave -token {REQUEST_HEADERS["X-Consul-Token"]}', 'consul')
                if errors:
                    logging.error(f'Leaving Consul: result - [{result}], errors - [{errors}]')
                    continue
                logging.info(f'Token for {server} is updated')
            return
        logging.error(f'There is no ability to update Consul server tokens after {timeout} seconds')
        sys.exit(1)

    def create_server_token(self, description: str, timeout=60):
        token = {
            'Description': description,
            'Policies': [{"Name": "agent-token"}]
        }
        timeout_start = time.time()
        while time.time() < timeout_start + timeout:
            try:
                response = requests.put(f'{self._consul_url}/v1/acl/token',
                                        headers=REQUEST_HEADERS,
                                        verify=self._consul_cafile,
                                        json=token)
            except Exception as e:
                logging.error(f'Unable to create "{description}" ACL token: {e}')
                continue
            if response.ok:
                logging.info(f'"{description}" token is created')
                return response.json().get('SecretID')
            logging.error(f'Consul has problems with "{description}" token creation: {response.status_code}, '
                          f'{response.content}')
            time.sleep(5)
        logging.error(f'Consul cannot create "{description}" token after {timeout} seconds')
        sys.exit(1)

    def find_service_account_secret(self, service_account_name: str) -> {}:
        sa_secrets = self.library.get_service_account_secrets(service_account_name)
        # Use the same approach the Consul uses in the 'server-acl-init' command:
        # https://github.com/hashicorp/consul-k8s/blob/v1.2.2/control-plane/subcommand/server-acl-init/connect_inject.go#L101
        secret_names = [service_account_name]
        for sa_secret in (sa_secrets or []):
            secret_names.append(sa_secret.name)
        logging.debug(f'Secrets are {secret_names}')
        sa_secret = None
        for secret_name in secret_names:
            secret = self.library.get_secret(secret_name)
            if secret and secret.type == 'kubernetes.io/service-account-token':
                sa_secret = secret
                break
        return sa_secret

    def update_auth_methods(self):
        logging.info('The update of Consul auth methods is started')
        service_account_name = f'{self._consul_fullname}-auth-method'
        sa_secret = self.find_service_account_secret(service_account_name)
        if not sa_secret:
            logging.error(f'found no secret of type "kubernetes.io/service-account-token" associated '
                          f'with the {service_account_name} service account')
            sys.exit(1)
        self.update_auth_method(f'{self._consul_fullname}-k8s-auth-method', sa_secret)
        self.update_auth_method(f'{self._consul_fullname}-k8s-component-auth-method', sa_secret)

    def restart_incorrect_servers(self):
        logging.info('The restart of incorrect Consul servers is started')
        response = requests.get(f'{self._consul_url}/v1/catalog/nodes',
                                headers=REQUEST_HEADERS,
                                verify=self._consul_cafile,
                                timeout=30)
        if not response.ok:
            logging.info(f'Consul has problems with receiving nodes: {response.status_code}, {response.text}')
            sys.exit(1)
        nodes = {d['Node']: d['Address'] for d in response.json()}
        for server_pod in self.library.get_pods_by_selector(
                {'name': f'{self._consul_fullname}-server', 'component': 'server'}):
            pod_name = server_pod.metadata.name
            pod_ip = server_pod.status.pod_ip
            if nodes.get(pod_name) != pod_ip:
                logging.info(
                    f'Server pod [{pod_name}:{pod_ip}] has incorrect registered IP [{nodes.get(pod_name)}],'
                    f' restarting it')
                self.library.delete_pods_by_selector({'statefulset.kubernetes.io/pod-name': pod_name})

    def update_auth_method(self, name: str, secret, timeout=60):
        auth_method = {
            'Name': name,
            'Description': 'Kubernetes Auth Method',
            'Type': 'kubernetes',
            'Config': {
                'Host': 'https://kubernetes.default.svc',
                'CACert': base64.decodebytes(secret.data['ca.crt'].encode()).decode(),
                'ServiceAccountJWT': base64.decodebytes(secret.data['token'].encode()).decode()
            }
        }
        timeout_start = time.time()
        while time.time() < timeout_start + timeout:
            time.sleep(5)
            try:
                response = requests.put(f'{self._consul_url}/v1/acl/auth-method/{name}',
                                        headers=REQUEST_HEADERS,
                                        json=auth_method,
                                        verify=self._consul_cafile)
            except Exception as e:
                logging.error(f'Consul has errors when upgrading "{name}" auth method: {e}')
                continue
            if not response.ok:
                logging.info(f'Consul has problems when upgrading "{name}" auth method: {response.status_code}, '
                             f'{response.content}')
                continue
            logging.info(f'{name} auth method configuration is updated')
            return
        logging.error(f'Consul can not upgrade "{name}" auth method after {timeout} seconds')
        sys.exit(1)


def str2bool(v: str):
    return v.lower() in ("yes", "true", "t", "1")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('folder')
    parser.add_argument('-d', '--datacenters')
    parser.add_argument('-skip_acl_recovery')
    args = parser.parse_args()
    folder = args.folder
    restore_instance = Restore(args.folder)

    datacenters = ast.literal_eval(args.datacenters) if args.datacenters else None
    skip_acl_recovery = str2bool(args.skip_acl_recovery) if args.skip_acl_recovery else False
    restore_instance.restore(folder, datacenters, skip_acl_recovery)
