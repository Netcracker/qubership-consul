#!/bin/sh
# Provisions what the chart will later create from Helm values: the IdP JWT auth method,
# policies, roles and binding rules. Users of the dev realm:
#   alice  / alice  -> consul-readers -> idp_kv-reader (read KV, services, nodes)
#   bob    / bob    -> consul-writers -> idp_kv-writer (write KV)
#   nobody / nobody -> no role        -> 403 from the proxy
set -e

until consul members >/dev/null 2>&1; do echo "waiting for consul"; sleep 1; done
until wget -q -O /dev/null http://keycloak:8080/auth/realms/consul/protocol/openid-connect/certs; do
  echo "waiting for keycloak"; sleep 2
done

cat > /tmp/idp.json <<'EOF'
{
  "JWKSURL": "http://keycloak:8080/auth/realms/consul/protocol/openid-connect/certs",
  "BoundIssuer": "http://localhost:8081/auth/realms/consul",
  "BoundAudiences": ["netcracker"],
  "ClaimMappings": {"preferred_username": "username"},
  "ListClaimMappings": {"/realm_access/roles": "groups"}
}
EOF

if consul acl auth-method read -name=platform-idp >/dev/null 2>&1; then
  echo "auth method exists, nothing to do"
  exit 0
fi

consul acl auth-method create -type=jwt -name=platform-idp -max-token-ttl=5m -config=@/tmp/idp.json

consul acl policy create -name=idp_kv-reader -rules='
key_prefix "" { policy = "read" }
service_prefix "" { policy = "read" }
node_prefix "" { policy = "read" }
'
consul acl policy create -name=idp_kv-writer -rules='
key_prefix "" { policy = "write" }
service_prefix "" { policy = "read" }
node_prefix "" { policy = "read" }
'
consul acl role create -name=idp_kv-reader -policy-name=idp_kv-reader
consul acl role create -name=idp_kv-writer -policy-name=idp_kv-writer

consul acl binding-rule create -method=platform-idp -bind-type=role -bind-name=idp_kv-reader \
  -selector='"consul-readers" in list.groups'
consul acl binding-rule create -method=platform-idp -bind-type=role -bind-name=idp_kv-writer \
  -selector='"consul-writers" in list.groups'

consul kv put demo/hello world
echo "consul provisioned"