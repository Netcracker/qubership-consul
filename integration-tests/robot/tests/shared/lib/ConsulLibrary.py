import os

import consul
import requests
from robot.libraries.BuiltIn import BuiltIn

CA_CERT_PATH = '/consul/tls/ca/tls.crt'


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
