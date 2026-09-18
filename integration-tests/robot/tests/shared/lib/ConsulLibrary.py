import os
import time

import consul
import requests
from robot.libraries.BuiltIn import BuiltIn

CA_CERT_PATH = '/consul/tls/ca/tls.crt'
BACKUP_CA_CERT_PATH = '/consul/tls/backup/ca.crt'


class ConsulLibrary(object):
    def __init__(self, consul_namespace, consul_host, consul_port, consul_scheme="http", consul_token=None):
        self.consul_namespace = consul_namespace
        self.consul_host = consul_host
        self.consul_port = consul_port
        self.consul_scheme = consul_scheme
        self.consul_token = consul_token
        self.consul_cafile = CA_CERT_PATH if os.path.exists(CA_CERT_PATH) else None
        self.builtin = BuiltIn()
        self.connect = consul.Consul(self.consul_host,
                                     self.consul_port,
                                     token=self.consul_token,
                                     scheme=consul_scheme,
                                     verify=self.consul_cafile,
                                     timeout=10)

    def put_data(self, key, value):
        return self.connect.kv.put(key=key, value=value)

    def get_data(self, key):
        resp = self.connect.kv.get(key=key)
        data = resp[1]
        return data['Value']

    def delete_data(self, key, recurse=None):
        return self.connect.kv.delete(key=key, recurse=recurse)

    def get_leader(self):
        return self.connect.status.leader()

    def get_list_peers(self):
        return self.connect.status.peers()

    def delete_port(self, pod_ip):
        return pod_ip.replace(":8300", "")

    def is_leader_reelected(self, leader_new, leader_old, pod_list):
        for pod in pod_list:
            if pod == leader_new and pod != leader_old:
                return True
        return False

    def get_server_ips_list(self):
        return [self.delete_port(peer) for peer in self.get_list_peers()]

    def put_data_using_request(self, key, value):
        url = f'{self.consul_scheme}://{self.consul_host}:{self.consul_port}/v1/kv/{key}'
        headers = {'Authorization': 'Bearer ' + self.consul_token}
        response = requests.Response()
        # Handle OSError as large PUT request with enabled TLS produces SSLEOFError 
        try: 
            response = requests.put(url, data=value, headers=headers, verify=self.consul_cafile)
        except OSError:
            response.status_code = 413
            return response
        return response

    def check_leader_using_request(self):
        url = f'{self.consul_scheme}://{self.consul_host}:{self.consul_port}/v1/status/leader'
        leader_response = requests.get(url, verify=self.consul_cafile)
        return leader_response.status_code == 200 and str(leader_response.content) != ""

    def create_backup_with_retry(self, base_url, username, password, verify=None,
                                 attempts=3, backup_timeout=120, poll_interval=10):
        """Create a full backup, retrying the whole backup up to ``attempts`` times.

        A backup attempt is considered failed when the POST fails / is not 200,
        when the backup is reported as failed, or when it does not reach a
        successful state within ``backup_timeout`` seconds. On such a failure the
        backup is re-issued (a new POST /backup), up to ``attempts`` times.

        Backup completion is polled via ``/listbackups/<id>`` (the ``failed`` /
        ``valid`` fields) -- the backup counterpart of ``/jobstatus/<task_id>``
        used for restore. Returns the backup id on success; fails otherwise.
        """
        attempts = int(attempts)
        auth = (username, password)
        if verify is None:
            verify = BACKUP_CA_CERT_PATH if str(base_url).startswith('https') else True
        last_error = 'no attempt was made'
        for attempt in range(1, attempts + 1):
            try:
                response = requests.post(f'{base_url}/backup', auth=auth, verify=verify, timeout=30)
            except requests.exceptions.RequestException as e:
                last_error = f'POST /backup failed: {e}'
                self.builtin.log(f'Backup attempt {attempt}/{attempts}: {last_error}', 'WARN')
                continue
            if response.status_code != 200:
                last_error = f'POST /backup returned {response.status_code}: {response.text}'
                self.builtin.log(f'Backup attempt {attempt}/{attempts}: {last_error}', 'WARN')
                continue
            backup_id = response.text.strip()
            succeeded, status_error = self._wait_backup_completed(
                base_url, auth, verify, backup_id, backup_timeout, poll_interval)
            if succeeded:
                self.builtin.log(f'Backup {backup_id} succeeded on attempt {attempt}/{attempts}')
                return backup_id
            last_error = status_error
            self.builtin.log(
                f'Backup attempt {attempt}/{attempts} (id={backup_id}) not successful: {status_error}', 'WARN')
        raise AssertionError(f'Backup did not succeed after {attempts} attempts. Last error: {last_error}')

    def _wait_backup_completed(self, base_url, auth, verify, backup_id, timeout, interval):
        deadline = time.time() + float(timeout)
        last = 'no status received'
        while time.time() < deadline:
            try:
                response = requests.get(f'{base_url}/listbackups/{backup_id}',
                                        auth=auth, verify=verify, timeout=30)
            except requests.exceptions.RequestException as e:
                last = f'status request failed: {e}'
                time.sleep(float(interval))
                continue
            if response.status_code == 200:
                content = response.json()
                if content.get('failed') is True:
                    return False, f'backup {backup_id} reported failed=True: {content}'
                if content.get('failed') is False and content.get('valid') is True:
                    return True, ''
                last = f'not ready yet: {content}'
            else:
                # 404 while the backup is still running, or after a failed backup
                # that was never stored -- keep polling until timeout.
                last = f'HTTP {response.status_code}: {response.text}'
            time.sleep(float(interval))
        return False, f'backup {backup_id} not successful within {timeout}s ({last})'
