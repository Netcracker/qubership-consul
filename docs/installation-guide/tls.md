# SSL Configuration

You can enable TLS-based encryption for communication with Consul.

Consul uses Transport Layer Security (TLS) encryption across the cluster to verify authenticity of the servers and
clients that connect. Consul provides additional HTTPS (TLS) port `8501`, while HTTP port is `8500`.

**Important**: If you upgrade Consul service with changed approach for certificates generation, manually restart all pods
to apply the changes.

## SSL Configuration using Parameters with Manually Generated Certificates

You can automatically create TLS-based secrets using Helm by specifying certificates in deployment parameters. 
The certificates should be generated manually and provided in BASE64 format:

```yaml
ca.crt: ${ROOT_CA_CERTIFICATE}
tls.crt: ${CERTIFICATE}
tls.key: ${PRIVATE_KEY}
```

Where:

   * ${ROOT_CA_CERTIFICATE} is the root CA in BASE64 format.
   * ${CERTIFICATE} is the certificate in BASE64 format.
   * ${PRIVATE_KEY} is the private key in BASE64 format.

**Important**: Before generating certificates, read the [Manual Certificates Generation](#manual-certificates-generation) section.

The example of installation parameters to deploy Consul with prepared TLS certificates in parameters:

```yaml
global:
  enabled: true
  tls:
    enabled: true
  acls:
    manageSystemACLs: true
server:
  enabled: true
  replicas: 3
  tls:
    certificates:
      crt: "LS0bbbCRUdJTiBDRVJ..."
      key: "LS0tbbbCRUdJTiBSU0..."
      ca: "LS0tLbbbUdJTiBDRVJ..."
client:
  enabled: true
  tls:
    certificates:
      crt: "LS0xxxCRUdJTiBDRVJU..."
      key: "LS0fffCRUdJTiBSU0EgUF..."
      ca: "LS0tvvvCRUdJTiBDRVJUSUZJQ0..."
ui:
  enabled: true
monitoring:
  enabled: true
backupDaemon:
  enabled: true
  tls:
    certificates:
      crt: "LS0tyyyUdJTiBDRVJ..."
      key: "LS0tyyyRUdJTiBSU0E..."
      ca: "LS0tLyyyRUdJTiBDRVJU..."
```

**Important**: If you upgrade Consul service with new certificates, manually restart all pods to apply the changes.

**Note:** TLS-based secrets using Helm are only supported for `server`, `client`, `backupDaemon`, `global.disasterRecovery`.

## SSL Configuration using CertManager

The example of deploy parameters to deploy Consul with enabled TLS and `CertManager` certificate generation:

```yaml
...
global:
  enabled: true
  enablePodSecurityPolicies: true

  tls:
    enabled: true
    httpsOnly: false
    certManager:
      enabled: true
      durationDays: 730
      clusterIssuerName: "example-tls-issuer"
...
```

Minimal parameters to enable TLS are:

```yaml
global.tls.enabled: true
global.tls.certManager.enabled: true
global.tls.certManager.clusterIssuerName: "example-tls-issuer"
...
```

Where `global.tls.certManager.clusterIssuerName` is the real name of CertManager cluster issuer.

## Manual Certificates Generation

If you need to generate Consul certificates manually, pay attention, `server` and `client` certificates must be generated with
the same Root CA.

Consul `server` certificate should have the following addresses in `Subject Alternative Names` field:

* IP:
  * `127.0.0.1`
* DNS:
  * `localhost`
  * `consul-server`
  * `*.consul-server`
  * `*.consul-server.${NAMESPACE}`
  * `consul-server.${NAMESPACE}`
  * `*.consul-server.${NAMESPACE}.svc`
  * `consul-server.${NAMESPACE}.svc`
  * `*.server.dc1.consul`
  * `server.dc1.consul`

Where:

* `${NAMESPACE}` is the namespace where Consul service is located. For example, `consul-service`.

Consul `client` certificate should have the following addresses in `Subject Alternative Names` field:

* IP:
  * `127.0.0.1`
  * IP addresses of all Kubernetes nodes for current environment
* DNS:
    * `localhost`
    * `client.dc1.consul`

## Certificate Renewal

CertManager automatically renews Certificates. 
It calculates when to renew a Certificate based on the issued X.509 certificate's duration and a `renewBefore` value 
which specifies how long before expiry a certificate should be renewed.
By default, the value of `renewBefore` parameter is 2/3 through the X.509 certificate's `duration`. 
More info in [Cert Manager Renewal](https://cert-manager.io/docs/usage/certificate/#renewal).

After certificate renewed by CertManager the secret contains new certificate, but running applications store previous certificate in pods. 
As CertManager generates new certificates before old expired the both certificates are valid for some time (`renewBefore`).

Consul service does not have any handlers for certificates secret changes, so you need to manually restart **all** 
Consul service pods until the time when old certificate is expired.

## Re-encrypt Route In Openshift Without NGINX Ingress Controller 

Automatic re-encrypt Route creation is not supported out of box, need to perform the following steps:

1. Disable Ingress in deployment parameters: `ui.ingress.enabled: false`.
   
   Deploy with enabled UI Ingress leads to incorrect Ingress and Route configuration.

2. Create Route manually. You can use the following template as an example:

   ```yaml
   kind: Route
   apiVersion: route.openshift.io/v1
   metadata:
     annotations:
       route.openshift.io/termination: reencrypt
     name: <specify-uniq-route-name>
     namespace: <specify-namespace-where-consul-service-is-installed>
   spec:
     host: <specify-your-target-host-here>
     to:
       kind: Service
       name: consul-ui 
       weight: 100
     port:
       targetPort: https
     tls:
       termination: reencrypt
       destinationCACertificate: <place-CA-certificate-here-from-consul-server-TLS-secret>
       insecureEdgeTerminationPolicy: Redirect
   ```

**NOTE**: If you can't access the Consul host after Route creation because of "too many redirects" error, then one of the possible root
causes is there is HTTP traffic between balancers and the cluster. To resolve that issue it's necessary to add the Route name to 
the exception list at the balancers.
