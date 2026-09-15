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

from library import KubernetesLibrary, acl_token_path, read_acl_token_from_file

REQUEST_HEADERS = {
    'Accept': 'application/json',
    'Content-type': 'application/json'
}

TLS_CRT_PATH = '/consul/tls/ca/tls.crt'
CA_CERT_PATH = '/consul/tls/ca/ca.crt'

# Consul can transiently drop RPC connections while the raft cluster is settling
# (leader election, follower catch-up, user-snapshot restore, rolling restart).
# Such errors are retryable; retrying a few times lets a single backup survive them
# instead of failing the whole run. Non-transient errors (ACL, bad config) are not
# retried. Attempts and backoff step (seconds) are overridable via env vars.
CONSUL_REQUEST_MAX_ATTEMPTS = int(os.getenv("CONSUL_REQUEST_MAX_ATTEMPTS", "5"))
CONSUL_REQUEST_RETRY_BACKOFF = int(os.getenv("CONSUL_REQUEST_RETRY_BACKOFF", "5"))
CONSUL_REQUEST_TIMEOUT = int(os.getenv("CONSUL_REQUEST_TIMEOUT", "120"))
_TRANSIENT_ERROR_MARKERS = (
    'connection reset',
    'connection refused',
    'connection aborted',
    'broken pipe',
    'eof',
    'timed out',
    'no cluster leader',
    'rpc error',
    'failed to decode response',
    'leadership lost',
    'stream closed',
)


def _is_transient_error(detail):
    detail = (detail or '').lower()
    return any(marker in detail for marker in _TRANSIENT_ERROR_MARKERS)

loggingLevel = logging.DEBUG if os.getenv(
    'CONSUL_BACKUP_DAEMON_DEBUG') else logging.INFO
logging.basicConfig(level=loggingLevel,
                    format='[%(asctime)s,%(msecs)03d][%(levelname)s][category=Backup] %(message)s',
                    datefmt='%Y-%m-%dT%H:%M:%S')


class Backup:

    def __init__(self, storage_folder):
        consul_host = os.getenv("CONSUL_HOST")
        consul_port = os.getenv("CONSUL_PORT")
        consul_namespace = os.getenv("CONSUL_NAMESPACE")
        consul_scheme = os.getenv("CONSUL_SCHEME", "http")
        if not consul_host or not consul_port:
            logging.error("Consul service name or port isn't specified.")
            sys.exit(1)

        self._acl_token = read_acl_token_from_file()
        self._acl_enabled = self._acl_token is not None
        self._consul_fullname = os.getenv("CONSUL_FULLNAME")
        self._consul_url = f'{consul_scheme}://{consul_host}:{consul_port}'
        self._storage_folder = storage_folder
        self._consul_cafile = CA_CERT_PATH if os.path.exists(CA_CERT_PATH) else TLS_CRT_PATH if os.path.exists(TLS_CRT_PATH) else None

        self.library = KubernetesLibrary(consul_namespace)

        if self._acl_enabled:
            REQUEST_HEADERS['X-Consul-Token'] = self._acl_token

    def __consul_get(self, url, error_context, params=None):
        """Perform a GET against Consul, retrying transient RPC/connection errors.

        Returns the successful response. On a non-transient error, or after the
        retries are exhausted, logs ``error_context`` with the last error detail
        and exits with code 1 (preserving the original failure semantics).
        """
        detail = None
        for attempt in range(1, CONSUL_REQUEST_MAX_ATTEMPTS + 1):
            try:
                response = requests.get(url, params=params, headers=REQUEST_HEADERS,
                                        verify=self._consul_cafile, timeout=CONSUL_REQUEST_TIMEOUT)
                if response.ok:
                    return response
                detail = response.text
                transient = _is_transient_error(response.text)
            except requests.exceptions.RequestException as e:
                detail = str(e)
                transient = True
            if attempt < CONSUL_REQUEST_MAX_ATTEMPTS and transient:
                delay = CONSUL_REQUEST_RETRY_BACKOFF * attempt
                logging.warning(f'{error_context} failed on attempt {attempt}/{CONSUL_REQUEST_MAX_ATTEMPTS} '
                                f'(transient), retrying in {delay}s. Details: {detail}')
                time.sleep(delay)
                continue
            break
        logging.error(f'{error_context}, details: {detail}')
        sys.exit(1)

    def __get_datacenters(self):
        dc_response = self.__consul_get(
            f'{self._consul_url}/v1/catalog/datacenters',
            f'There is problem with getting datacenters from Consul server {self._consul_url}')
        return dc_response.json()

    def backup(self, folder, datacenters=None):
        if not datacenters:
            logging.debug(
                "Datacenters are not specified, full backup for each datacenter will be performed")
            datacenters = self.__get_datacenters()

        logging.info(f'Perform backup for {datacenters} datacenters')
        for datacenter in datacenters:
            snapshot_folder = f'{folder}/{datacenter}'
            os.makedirs(snapshot_folder)
            snapshot_resp = self.__consul_get(
                f'{self._consul_url}/v1/snapshot',
                f'There is problem with getting snapshot from datacenter: {datacenter}',
                params={'dc': datacenter})
            with open(f'{snapshot_folder}/snapshot.gz', 'wb') as snapshot_file:
                snapshot_file.write(snapshot_resp.content)
            logging.info(f'Snapshot for datacenter "{datacenter}" completed successfully.')

        if self._acl_enabled:
            self.backup_acl_token(f'{folder}/.token')
        logging.info(f'Snapshot for datacenters {datacenters} completed successfully.')

    def backup_acl_token(self, file_path):
        logging.info(f'Backup ACL token from {acl_token_path()}')
        with open(file_path, 'w') as secret_data_file:
            secret_data_file.write(base64.b64encode(self._acl_token.encode()).decode())


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('folder')
    parser.add_argument('-d', '--datacenters')
    args = parser.parse_args()
    folder = args.folder
    datacenters = ast.literal_eval(args.datacenters) if args.datacenters else None

    backup_instance = Backup(args.folder)
    backup_instance.backup(folder, datacenters)
