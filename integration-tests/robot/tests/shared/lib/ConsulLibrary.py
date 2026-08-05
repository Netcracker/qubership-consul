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

    # --- ACL helpers ---

    def _acl_get(self, path):
        url = f'{self.consul_scheme}://{self.consul_host}:{self.consul_port}/v1/acl/{path}'
        headers = {'X-Consul-Token': self.consul_token} if self.consul_token else {}
        response = requests.get(url, headers=headers, verify=self.consul_cafile)
        response.raise_for_status()
        return response.json()

    def _acl_delete(self, path):
        url = f'{self.consul_scheme}://{self.consul_host}:{self.consul_port}/v1/acl/{path}'
        headers = {'X-Consul-Token': self.consul_token} if self.consul_token else {}
        response = requests.delete(url, headers=headers, verify=self.consul_cafile)
        response.raise_for_status()

    def get_acl_policy_by_name(self, name):
        """Return the policy object with the given name, or None if not found."""
        try:
            return self._acl_get(f'policy/name/{name}')
        except requests.HTTPError as e:
            if e.response.status_code == 403:
                return None
            raise

    def get_acl_role_by_name(self, name):
        """Return the role object with the given name, or None if not found."""
        try:
            return self._acl_get(f'role/name/{name}')
        except requests.HTTPError as e:
            if e.response.status_code == 403:
                return None
            raise

    def acl_policy_should_exist(self, name):
        """Fail if no ACL policy with the given name exists in Consul."""
        result = self.get_acl_policy_by_name(name)
        assert result is not None, f'ACL policy "{name}" not found in Consul'

    def acl_policy_should_not_exist(self, name):
        """Fail if an ACL policy with the given name exists in Consul."""
        result = self.get_acl_policy_by_name(name)
        assert result is None, f'ACL policy "{name}" should not exist in Consul but was found'

    def acl_role_should_exist(self, name):
        """Fail if no ACL role with the given name exists in Consul."""
        result = self.get_acl_role_by_name(name)
        assert result is not None, f'ACL role "{name}" not found in Consul'

    def acl_role_should_not_exist(self, name):
        """Fail if an ACL role with the given name exists in Consul."""
        result = self.get_acl_role_by_name(name)
        assert result is None, f'ACL role "{name}" should not exist in Consul but was found'

    def create_auth_method(self, name, auth_type="kubernetes", description=""):
        """Create a Consul ACL auth method. Idempotent: updates if already exists."""
        url = f'{self.consul_scheme}://{self.consul_host}:{self.consul_port}/v1/acl/auth-method'
        headers = {'X-Consul-Token': self.consul_token} if self.consul_token else {}
        payload = {"Name": name, "Type": auth_type, "Description": description,
                   "Config": {"Host": "https://kubernetes.default.svc", "CACert": "", "ServiceAccountJWT": ""}}
        response = requests.put(url, json=payload, headers=headers, verify=self.consul_cafile)
        response.raise_for_status()
        return response.json()

    def delete_auth_method(self, name):
        """Delete a Consul ACL auth method. No-op if not found."""
        try:
            self._acl_delete(f'auth-method/{name}')
        except requests.HTTPError as e:
            if e.response.status_code == 404:
                return
            raise

    def list_acl_binding_rules(self, auth_method):
        """Return list of binding rules for the given auth method."""
        try:
            return self._acl_get(f'binding-rules?authmethod={auth_method}')
        except requests.HTTPError as e:
            if e.response.status_code == 403:
                return []
            raise

    def acl_binding_rule_should_exist(self, bind_name, auth_method):
        """Fail if no binding rule with bind_name exists under auth_method."""
        rules = self.list_acl_binding_rules(auth_method)
        for rule in rules:
            if rule.get('BindName') == bind_name:
                return
        raise AssertionError(
            f'Binding rule with BindName "{bind_name}" under auth method "{auth_method}" not found in Consul'
        )

    def acl_binding_rule_should_not_exist(self, bind_name, auth_method):
        """Fail if a binding rule with bind_name exists under auth_method."""
        rules = self.list_acl_binding_rules(auth_method)
        for rule in rules:
            if rule.get('BindName') == bind_name:
                raise AssertionError(
                    f'Binding rule with BindName "{bind_name}" under auth method "{auth_method}" should not exist but was found'
                )

    def kv_key_should_exist(self, key):
        """Fail if the KV key does not exist in Consul."""
        index, data = self.connect.kv.get(key)
        assert data is not None, f'KV key "{key}" not found in Consul'

    def kv_key_should_not_exist(self, key):
        """Fail if the KV key exists in Consul."""
        index, data = self.connect.kv.get(key)
        assert data is None, f'KV key "{key}" should not exist in Consul but was found'
