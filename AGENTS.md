# AGENTS.md

Qubership Consul is a HashiCorp Consul (community edition) distribution
for Kubernetes/OpenShift. It bundles a Helm chart, a Kubebuilder-based ACL
Configurator operator, several auxiliary services (Go and Python),
monitoring assets, and a Robot Framework integration test suite.

## Repository Layout

Monorepo. `go.mod` files live under
`acl-configurator/consul-acl-configurator-operator/`,
`acl-configurator/rest-acl-configurator/`, `disaster-recovery/` and
`remove-tokens/`. The `backup-daemon` service is Python.

- `/charts/helm/consul-service` — The only Helm chart. Installs Consul
  servers (StatefulSet), clients (DaemonSet), DNS, UI, TLS init, ACL init,
  and optional components (backup daemon, ACL Configurator, disaster
  recovery, remove tokens, pod scheduler, monitoring, status provisioner,
  integration tests; sync catalog, Connect inject and mesh gateway are
  disabled by default). The chart contains two sets of CRDs:
  - `crds/` — ACL Configurator CRDs (`ConsulACL`, `ConsulKV`), installed by
    Helm from the `crds/` directory.
  - `templates/crd-*.yaml` — Upstream HashiCorp Consul Connect CRDs
    (`consul.hashicorp.com`: `Mesh`, `ServiceDefaults`,
    `ServiceIntentions`, …), reconciled by Connect inject. Rendered only
    when `connectInject.enabled` is true and the restricted environment is
    off (`global.restrictedEnvironment`).

  Grafana dashboard in `monitoring/`, Prometheus rules in
  `templates/prometheus-rules.yaml`.
- `/acl-configurator/consul-acl-configurator-operator` — Kubebuilder v3
  operator (`projectName: consul-acl-configurator-operator`, domain
  `netcracker.com`). Reconciles `ConsulACL` (policies, roles, binding rules,
  auth methods) and `ConsulKV` CRs. API group is configurable through env
  `API_GROUP` (default `netcracker.com`).
- `/acl-configurator/rest-acl-configurator` — Go REST endpoint that
  re-applies all existing ACL CRs to Consul (used by privileged services,
  e.g. the backup daemon after restore).
- `/acl-configurator/sdk-tool` — `operator-sdk` image used for operator
  codegen.
- `/backup-daemon` — Python service for full and granular backup/restore
  of Consul snapshots and KV (PV or S3, Velero integration). Image is built
  on `qubership-backup-daemon-go`; config in `backup-daemon.conf`.
- `/disaster-recovery` — Go service implementing the DR site manager
  contract (active/standby switchover via backup daemon).
- `/remove-tokens` — Go job that cleans up stale Consul ACL tokens.
- `/docker-transfer` — `FROM scratch` image used to ship charts and docs.
- `/integration-tests` — Robot Framework suites at
  `robot/tests/consul/`, shared keywords and Python libs in
  `robot/tests/shared/`.
- `/demo` — Docker Compose stack (Consul + backup daemon). Not usable for
  running the integration tests.
- `/dev-kit` — `make_json_schema.sh` to regenerate `values.schema.json`.
- `/openspec` — OpenSpec change proposals and specs.
- `/agent-packages`, `apm.yml` — APM packages for AI agents (the
  `troubleshoot-consul` skill).
- `/docs/public` — User-facing documentation (architecture, installation,
  troubleshooting, ACL Configurator, backup daemon, security, TLS,
  disaster recovery, federation, etc.).

## Operator Code Organization

Paths below are relative to
`acl-configurator/consul-acl-configurator-operator/`.

- `api/v1`, `api/v1alpha1` — `ConsulACL` and `ConsulKV` types. Both
  versions are served; keep them in sync.
- `controllers/consulacl_controller.go` — `ConsulACL` reconciliation
  (policies, roles, binding rules, auth methods, finalizers, status).
- `controllers/consulkv_controller.go` — `ConsulKV` reconciliation.
- `controllers/acl_api_provider.go` — Consul ACL API client wrapper.
- `controllers/ownership.go` — Ownership tracking of Consul ACL entities
  created by CRs.
- `util/` — Env helpers and CR status updater.
- Operator configuration comes from env vars (`CONSUL_HOST`, `CONSUL_PORT`,
  `CONSUL_SCHEME`, `CONSUL_AUTH_METHOD_NAME`, `CONSUL_LEGACY_AUTH_METHODS`,
  `RECONCILE_PERIOD_SECONDS`, `WATCH_NAMESPACE`,
  `STRICT_NAMESPACE_VALIDATION`, `API_GROUP`, …) read in `main.go` and
  `controllers/consulacl_controller.go`.

## Compatibility Rules

All changes MUST be backward compatible.

### CRDs

- Add new functionality through new optional fields. Do not change the
  semantics or shape of existing fields.
- If a breaking change is unavoidable, stop and ask the user for explicit
  approval before adding a new API version.
- After editing anything under `api/`, run `make generate` and
  `make manifests`, and sync the chart CRDs in
  `charts/helm/consul-service/crds/` (`consul_acl_configurator_crd.yaml`,
  `consulkv_crd.yaml`). Use the dev-kit if `operator-sdk` is not installed
  locally (see below).

### Helm Charts

- Every new parameter MUST have a sensible default; never break existing
  values files.
- Document every new parameter in `docs/public/installation.md` and keep
  `values.schema.json` in sync (`dev-kit/make_json_schema.sh`).
- Reuse shared templates in `templates/_helpers.tpl` for labels,
  annotations, images and security contexts.
- Every change must be tested with `helm template`.

## Dev-Kit (Operator Code Generation)

Use the sdk-tool image when `operator-sdk` is not installed locally. It
runs `operator-sdk:v1.16` in Docker with the project mounted.

```sh
cd acl-configurator
docker build -t consul-sdk-tool sdk-tool
docker run --rm -it -v "$PWD/consul-acl-configurator-operator:/workspace" \
  -w /workspace consul-sdk-tool
make generate        # Regenerate DeepCopy code from api/
make manifests       # Regenerate CRDs in config/crd/bases
make test            # Run operator unit tests (envtest)
```

## Integration Tests

Robot Framework suites at `integration-tests/robot/tests/consul/`
(`crud`, `acl_configurator`, `backup`, `ha`, `alerts`, `hardening`,
`image_tests`). They run against a deployed Consul in Kubernetes: the
chart deploys them as a pod (`templates/integration_tests`, tags set via
`TAGS`), and CI runs them via `.github/workflows/run_tests.yaml`
(Netcracker/qubership-test-pipelines).

### Mandatory – every change must be tested

- Any change in docker folders where Dockerfiles are placed must be tested
  with Docker build.
- Any code change must be covered with unit tests.
- Any functional feature of components that can be tested without redeploy
  must be covered with Robot Framework tests.
- Before submitting any change to `backup-daemon`, `acl-configurator` or
  `integration-tests` folders, run **at minimum** the `smoke`, `backup`,
  `acl-configurator` and `kv-configurator` tagged suites.

```sh
cd acl-configurator/consul-acl-configurator-operator && make test
robot -i smokeORbackupORacl-configuratorORkv-configurator ./tests   # inside the integration-tests pod
```

See [integration-tests/README.md](integration-tests/README.md) for the
full environment reference (environment variables, tags, deployment).

## Troubleshooting

When diagnosing a Consul problem — a degraded or down cluster, no cluster
leader, CPU, memory, or full data volume, a server or client pod that does not
start, ACL reset, a backup daemon failure, or a failed Velero restore —
apply the `troubleshoot-consul` skill.
