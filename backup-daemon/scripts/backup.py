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

        self._acl_enabled = True if os.getenv('CONSUL_HTTP_TOKEN') else False
        self._consul_fullname = os.getenv("CONSUL_FULLNAME")
        self._consul_url = f'{consul_scheme}://{consul_host}:{consul_port}'
        self._storage_folder = storage_folder
        self._consul_cafile = CA_CERT_PATH if os.path.exists(CA_CERT_PATH) else TLS_CRT_PATH if os.path.exists(TLS_CRT_PATH) else None

        self.library = KubernetesLibrary(consul_namespace)

        if self._acl_enabled:
            secret = self.library.get_secret(f'{self._consul_fullname}-bootstrap-acl-token')
            data = secret.data.get("token")
            REQUEST_HEADERS['X-Consul-Token'] = base64.decodebytes(data.encode()).decode()

    def __get_datacenters(self):
        dc_response = requests.get(f'{self._consul_url}/v1/catalog/datacenters', headers=REQUEST_HEADERS,
                                   verify=self._consul_cafile)
        if not dc_response.ok:
            logging.error(f'There is problem with getting datacenters from Consul server {self._consul_url}, '
                          f'details: {dc_response.text}')
            sys.exit(1)
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
            snapshot_resp = requests.get(f'{self._consul_url}/v1/snapshot', params={'dc': datacenter},
                                         headers=REQUEST_HEADERS, verify=self._consul_cafile)
            if not snapshot_resp.ok:
                logging.error(f'There is problem with getting snapshot from datacenter: {datacenter}, '
                              f'details: {snapshot_resp.text}')
                sys.exit(1)
            with open(f'{snapshot_folder}/snapshot.gz', 'wb') as snapshot_file:
                snapshot_file.write(snapshot_resp.content)
            logging.info(f'Snapshot for datacenter "{datacenter}" completed successfully.')

        if self._acl_enabled:
            self.backup_secret_data(f'{self._consul_fullname}-bootstrap-acl-token', 'token', f'{folder}/.token')
        logging.info(f'Snapshot for datacenters {datacenters} completed successfully.')

    def backup_secret_data(self, secret_name, secret_key, file_path):
        logging.info(f'Backup "{secret_name}" secret data')
        secret = self.library.get_secret(secret_name)
        data = secret.data.get(secret_key)
        with open(file_path, 'w') as secret_data_file:
            secret_data_file.write(data)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('folder')
    parser.add_argument('-d', '--datacenters')
    args = parser.parse_args()
    folder = args.folder
    datacenters = ast.literal_eval(args.datacenters) if args.datacenters else None

    backup_instance = Backup(args.folder)
    backup_instance.backup(folder, datacenters)
