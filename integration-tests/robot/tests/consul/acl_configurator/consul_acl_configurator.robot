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


*** Keywords ***
Apply ConsulACL CR
    [Arguments]    ${name}    ${body}
    ${result}    ${value}=    Run Keyword And Ignore Error
    ...    Get Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}
    IF    '${result}' == 'PASS'
        Patch Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}    ${body}
    ELSE
        Create Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${body}
    END

Delete ConsulACL CR
    [Arguments]    ${name}
    Delete Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulacls    ${name}

Create Override Auth Method
    Create Auth Method    integration-override-auth-method    kubernetes    Auth method for per-rule override integration test

Delete Override Auth Method
    Delete Auth Method    integration-override-auth-method

Build ConsulACL Body
    [Arguments]    ${name}    ${explicit}    ${json}
    &{metadata}=    Create Dictionary    name=${name}    namespace=${TEST_NAMESPACE}
    &{acl}=         Create Dictionary    name=${name}    explicitName=${explicit}    json=${json}
    &{spec}=        Create Dictionary    acl=&{acl}
    &{body}=        Create Dictionary    apiVersion=netcracker.com/v1alpha1    kind=ConsulACL    metadata=&{metadata}    spec=&{spec}
    RETURN    &{body}

Dicts To Json
    [Arguments]    ${policies}    ${roles}    ${bind_rules}
    ${json}=    Evaluate    __import__('json').dumps({'policies': $policies, 'roles': $roles, 'bind_rules': $bind_rules})
    RETURN    ${json}


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
    ${json}=    Dicts To Json    ${policies}    ${[]}    ${[]}
    &{body}=    Build ConsulACL Body    test-explicit-acl    ${TRUE}    ${json}
    Apply ConsulACL CR    test-explicit-acl    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL Explicit Role Should Be Gone
    [Teardown]    Delete ConsulACL CR    test-explicit-acl

ACL Explicit Full Entities Should Exist
    Acl Policy Should Exist    integration_explicit_policy
    Acl Role Should Exist    integration_explicit_role
    Acl Binding Rule Should Exist    integration_explicit_bind    ${AUTH_METHOD}

ACL Explicit Role Should Be Gone
    Acl Policy Should Exist    integration_explicit_policy
    Acl Role Should Not Exist    integration_explicit_role


# 18.2 — per-rule AuthMethod override
Test ConsulACL PerRule AuthMethod Binding Rule Under Override Method
    [Tags]    acl-configurator    per-rule-auth-method
    ${bind_rules}=  Evaluate    [{'BindName': 'integration_perrule_bind', 'AuthMethod': 'integration-override-auth-method', 'ServiceAccountName': 'integration-sa'}]
    ${json}=    Dicts To Json    ${[]}    ${[]}    ${bind_rules}
    &{body}=    Build ConsulACL Body    test-perrule-auth    ${TRUE}    ${json}
    Apply ConsulACL CR    test-perrule-auth    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL PerRule AuthMethod Entities Should Exist
    [Teardown]    Delete ConsulACL CR    test-perrule-auth

ACL PerRule AuthMethod Entities Should Exist
    Acl Binding Rule Should Exist    integration_perrule_bind    integration-override-auth-method


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

ACL Delete Test Entities Should Exist
    Acl Policy Should Exist    test-delete-acl_${TEST_NAMESPACE}_delete_policy
    Acl Role Should Exist    test-delete-acl_${TEST_NAMESPACE}_delete_role
    Acl Binding Rule Should Exist    test-delete-acl_${TEST_NAMESPACE}_delete_bind    ${AUTH_METHOD}

ACL Delete Test Entities Should Not Exist
    Acl Policy Should Not Exist    test-delete-acl_${TEST_NAMESPACE}_delete_policy
    Acl Role Should Not Exist    test-delete-acl_${TEST_NAMESPACE}_delete_role
    Acl Binding Rule Should Not Exist    test-delete-acl_${TEST_NAMESPACE}_delete_bind    ${AUTH_METHOD}