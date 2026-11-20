# consul-idp-proxy (MVP)

Stateless reverse proxy in front of the Consul API and UI. Users log in at the platform IdP
(identity-provider, Keycloak); the proxy exchanges the IdP JWT for a Consul ACL token through the Consul
JWT auth method (`/v1/acl/login`) and injects `X-Consul-Token`. The IdP session, the refresh token and the
Consul token are kept in AES-GCM encrypted cookies — nothing is stored in the proxy memory.

Design, configuration and test instructions (in Russian): `openspec/changes/consul-idp-proxy/overview.ru.md`.

```bash
go test ./...
cd dev && docker compose up -d --build   # Keycloak + Consul + proxy on http://localhost:8080
```
