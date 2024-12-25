This chapter describes the security audit logging for Consul.

The following topics are covered in this chapter:

<!-- TOC -->
* [Common Information](#common-information)
* [Configuration](#configuration)
  * [Example of Events](#example-of-events)
    * [Failed Login](#failed-login)
    * [Unauthorized event](#unauthorized-event)
<!-- TOC -->

# Common Information

Audit logs let you track key security events within Consul and are useful for compliance purposes or in the aftermath of a security breach.

# Configuration

Consul audit is Enterprise feature, refer to [Consul Audit](https://developer.hashicorp.com/consul/docs/enterprise/audit-logging).
Following audit logs are included in basic delivery and enabled by default.

## Example of Events

The audit log format for events are described further:

### Failed Login

```text
2024-08-14T11:03:11.196Z [ERROR] agent.grpc-api.acl.login: failed to validate login: request_id=16d08806-c1f4-48db-44ad-a368b0568d03 error=Unauthorized
```

### Unauthorized event

```text
2024-08-16T05:19:06.753Z [ERROR] agent.http: Request error: method=GET url=/v1/acl/tokens?dc=dc1 from=10.129.252.77:55280 error="Permission denied: anonymous token lacks permission 'acl:read'. The anonymous token is used implicitly when a request does not specify a token."
```
