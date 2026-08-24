*** Settings ***
Resource    ../../shared/keywords.robot
Library     PlatformLibrary  managed_by_operator=true
Library     Collections
Suite Setup       Create Override Auth Method
Suite Teardown    Delete Override Auth Method

*** Variables ***
${TEST_NAMESPACE}        %{CONSUL_NAMESPACE}
${AUTH_METHOD}           consul-k8s-auth-method
${RECONCILE_TIMEOUT}     60s
${RECONCILE_INTERVAL}    2s
${GROUP}                 netcracker.com
${VERSION}               v1alpha1
${OTHER_NAMESPACE}       other-namespace


*** Keywords ***
Apply ConsulACL CR
    [Arguments]    ${name}    ${body}
    ${result}    ${value}=    Run Keyword And Ignore Error
    ...    Get Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}
    IF    '${result}' == 'PASS'
        Delete Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}
        Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
        ...    CR Should Not Exist    ${name}
    END
    Create Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${body}

CR Should Not Exist
    [Arguments]    ${name}
    ${result}    ${value}=    Run Keyword And Ignore Error
    ...    Get Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}
    Should Be Equal    ${result}    FAIL

Delete ConsulACL CR
    [Arguments]    ${name}
    Delete Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}

Create Override Auth Method
    Create Auth Method    integration-override-auth-method    kubernetes    Auth method for per-rule override integration test

Delete Override Auth Method
    Delete Auth Method    integration-override-auth-method

Build ConsulACL Body
    [Arguments]    ${name}    ${explicit}    ${json}
    ${body}=    Evaluate
    ...    {'apiVersion': 'netcracker.com/v1alpha1', 'kind': 'ConsulACL', 'metadata': {'name': $name, 'namespace': $TEST_NAMESPACE}, 'spec': {'acl': {'name': $name, 'explicitName': bool($explicit), 'operatorNamespace': $TEST_NAMESPACE, 'json': $json}}}
    RETURN    ${body}

Build ConsulACL Body In Namespace
    [Arguments]    ${name}    ${namespace}    ${explicit}    ${operator_namespace}    ${json}
    ${body}=    Evaluate
    ...    {'apiVersion': 'netcracker.com/v1alpha1', 'kind': 'ConsulACL', 'metadata': {'name': $name, 'namespace': $namespace}, 'spec': {'acl': {'name': $name, 'explicitName': bool($explicit), 'operatorNamespace': $operator_namespace, 'json': $json}}}
    RETURN    ${body}

Apply ConsulACL CR In Namespace
    [Arguments]    ${name}    ${namespace}    ${body}
    ${result}    ${value}=    Run Keyword And Ignore Error
    ...    Get Namespaced Custom Object    ${GROUP}    ${VERSION}    ${namespace}    consulacls    ${name}
    IF    '${result}' == 'PASS'
        Delete Namespaced Custom Object    ${GROUP}    ${VERSION}    ${namespace}    consulacls    ${name}
        Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
        ...    CR Should Not Exist In Namespace    ${name}    ${namespace}
    END
    Create Namespaced Custom Object    ${GROUP}    ${VERSION}    ${namespace}    consulacls    ${body}

CR Should Not Exist In Namespace
    [Arguments]    ${name}    ${namespace}
    ${result}    ${value}=    Run Keyword And Ignore Error
    ...    Get Namespaced Custom Object    ${GROUP}    ${VERSION}    ${namespace}    consulacls    ${name}
    Should Be Equal    ${result}    FAIL

Delete ConsulACL CR In Namespace
    [Arguments]    ${name}    ${namespace}
    Delete Namespaced Custom Object    ${GROUP}    ${VERSION}    ${namespace}    consulacls    ${name}

Dicts To Json
    [Arguments]    ${policies}    ${roles}    ${bind_rules}
    ${json}=    Evaluate    __import__('json').dumps({'policies': $policies, 'roles': $roles, 'bind_rules': $bind_rules})
    RETURN    ${json}

ACL Explicit Full Entities Should Exist
    Acl Policy Should Exist    test-explicit-acl_${TEST_NAMESPACE}_integration_explicit_policy
    Acl Role Should Exist    integration_explicit_role
    Acl Binding Rule Should Exist    integration_explicit_bind    ${AUTH_METHOD}

ACL Explicit Role Should Be Gone
    Acl Policy Should Exist    test-explicit-acl_${TEST_NAMESPACE}_integration_explicit_policy
    Acl Role Should Not Exist    integration_explicit_role

ACL PerRule AuthMethod Entities Should Exist
    Acl Binding Rule Should Exist    integration_perrule_bind    integration-override-auth-method

ACL Delete Test Entities Should Exist
    Acl Policy Should Exist    test-delete-acl_${TEST_NAMESPACE}_delete_policy
    Acl Role Should Exist    test-delete-acl_${TEST_NAMESPACE}_delete_role
    Acl Binding Rule Should Exist    test-delete-acl_${TEST_NAMESPACE}_delete_bind    ${AUTH_METHOD}

ACL Delete Test Entities Should Not Exist
    Acl Policy Should Not Exist    test-delete-acl_${TEST_NAMESPACE}_delete_policy
    Acl Role Should Not Exist    test-delete-acl_${TEST_NAMESPACE}_delete_role
    Acl Binding Rule Should Not Exist    test-delete-acl_${TEST_NAMESPACE}_delete_bind    ${AUTH_METHOD}


*** Test Cases ***

# 18.1 — explicitName: true — verbatim names and stale entity removal on update
Test ConsulACL ExplicitName Verbatim Names
    [Tags]    acl-configurator    explicit-name
    ${policies}=    Evaluate    [{'Name': 'integration_explicit_policy', 'Description': 'Integration test explicit policy', 'Rules': 'key_prefix "integration/" { policy = "read" }'}]
    ${roles}=       Evaluate    [{'Name': 'integration_explicit_role', 'Description': 'Integration test explicit role', 'policy_names': ['integration_explicit_policy']}]
    ${bind_rules}=  Evaluate    [{'BindName': 'integration_explicit_bind', 'ServiceAccountName': 'integration-sa'}]
    ${json}=    Dicts To Json    ${policies}    ${roles}    ${bind_rules}
    &{body}=    Build ConsulACL Body    test-explicit-acl    ${TRUE}    ${json}
    Apply ConsulACL CR    test-explicit-acl    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL Explicit Full Entities Should Exist

Test ConsulACL ExplicitName Removed Entity Deleted On Update
    [Tags]    acl-configurator    explicit-name
    ${policies}=    Evaluate    [{'Name': 'integration_explicit_policy', 'Description': 'Integration test explicit policy', 'Rules': 'key_prefix "integration/" { policy = "read" }'}]
    ${empty}=       Evaluate    []
    ${json}=    Dicts To Json    ${policies}    ${empty}    ${empty}
    &{body}=    Build ConsulACL Body    test-explicit-acl    ${TRUE}    ${json}
    Apply ConsulACL CR    test-explicit-acl    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL Explicit Role Should Be Gone
    [Teardown]    Delete ConsulACL CR    test-explicit-acl

# 18.2 — per-rule AuthMethod override
Test ConsulACL PerRule AuthMethod Binding Rule Under Override Method
    [Tags]    acl-configurator    per-rule-auth-method
    ${bind_rules}=  Evaluate    [{'BindName': 'integration_perrule_bind', 'AuthMethod': 'integration-override-auth-method', 'ServiceAccountName': 'integration-sa'}]
    ${empty}=       Evaluate    []
    ${json}=    Dicts To Json    ${empty}    ${empty}    ${bind_rules}
    &{body}=    Build ConsulACL Body    test-perrule-auth    ${TRUE}    ${json}
    Apply ConsulACL CR    test-perrule-auth    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL PerRule AuthMethod Entities Should Exist
    [Teardown]    Delete ConsulACL CR    test-perrule-auth

# 18.3 — delete: all entities removed
Test ConsulACL Delete Removes All Entities
    [Tags]    acl-configurator    delete
    ${policies}=    Evaluate    [{'Name': 'delete_policy', 'Description': 'Policy to be deleted', 'Rules': 'key_prefix "delete/" { policy = "read" }'}]
    ${roles}=       Evaluate    [{'Name': 'delete_role', 'Description': 'Role to be deleted', 'policy_names': ['delete_policy']}]
    ${bind_rules}=  Evaluate    [{'BindName': 'delete_bind', 'ServiceAccountName': 'integration-sa'}]
    ${json}=    Dicts To Json    ${policies}    ${roles}    ${bind_rules}
    &{body}=    Build ConsulACL Body    test-delete-acl    ${FALSE}    ${json}
    Apply ConsulACL CR    test-delete-acl    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL Delete Test Entities Should Exist
    Delete ConsulACL CR    test-delete-acl
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL Delete Test Entities Should Not Exist

# 18.4 — operatorNamespace: CR from another namespace processed by correct operator
Test ConsulACL OperatorNamespace Cross Namespace Routing
    [Tags]    acl-configurator    operator-namespace
    ${policies}=    Evaluate    [{'Name': 'cross_ns_policy', 'Description': 'Cross-namespace test policy', 'Rules': 'key_prefix "config/cross-ns/" { policy = "read" }'}]
    ${roles}=       Evaluate    [{'Name': 'cross_ns_role', 'Description': 'Cross-namespace test role', 'policy_names': ['cross_ns_policy']}]
    ${bind_rules}=  Evaluate    [{'BindName': 'cross_ns_bind', 'ServiceAccountName': 'cross-ns-sa'}]
    ${json}=    Dicts To Json    ${policies}    ${roles}    ${bind_rules}
    &{body}=    Build ConsulACL Body In Namespace    test-cross-ns-acl    ${OTHER_NAMESPACE}    ${TRUE}    ${TEST_NAMESPACE}    ${json}
    Apply ConsulACL CR In Namespace    test-cross-ns-acl    ${OTHER_NAMESPACE}    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    Acl Role Should Exist    cross_ns_role
    [Teardown]    Delete ConsulACL CR In Namespace    test-cross-ns-acl    ${OTHER_NAMESPACE}

# 18.5 — operatorNamespace: CR with wrong operatorNamespace is ignored
Test ConsulACL OperatorNamespace Wrong Namespace Ignored
    [Tags]    acl-configurator    operator-namespace
    ${policies}=    Evaluate    [{'Name': 'ignored_policy', 'Rules': 'key_prefix "ignored/" { policy = "read" }'}]
    ${empty}=       Evaluate    []
    ${json}=    Dicts To Json    ${policies}    ${empty}    ${empty}
    &{body}=    Build ConsulACL Body In Namespace    test-ignored-acl    ${TEST_NAMESPACE}    ${TRUE}    wrong-namespace    ${json}
    Apply ConsulACL CR In Namespace    test-ignored-acl    ${TEST_NAMESPACE}    ${body}
    Sleep    10s
    Acl Policy Should Not Exist    ignored_policy
    [Teardown]    Delete ConsulACL CR In Namespace    test-ignored-acl    ${TEST_NAMESPACE}

# 18.6 — idempotent binding rules: re-apply does not create duplicate
Test ConsulACL Idempotent BindingRule No Duplicate On Reapply
    [Tags]    acl-configurator    idempotent
    ${bind_rules}=  Evaluate    [{'BindName': 'idempotent_bind', 'ServiceAccountName': 'integration-sa'}]
    ${empty}=       Evaluate    []
    ${json}=    Dicts To Json    ${empty}    ${empty}    ${bind_rules}
    &{body}=    Build ConsulACL Body    test-idempotent-acl    ${TRUE}    ${json}
    Apply ConsulACL CR    test-idempotent-acl    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    Acl Binding Rule Should Exist    idempotent_bind    ${AUTH_METHOD}
    Apply ConsulACL CR    test-idempotent-acl    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    Acl Binding Rule Count Should Be    idempotent_bind    ${AUTH_METHOD}    1
    [Teardown]    Delete ConsulACL CR    test-idempotent-acl
