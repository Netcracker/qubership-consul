*** Settings ***
Resource    ../../shared/keywords.robot
Library     PlatformLibrary  managed_by_operator=true

*** Variables ***
${TEST_NAMESPACE}        %{CONSUL_NAMESPACE}
${AUTH_METHOD}           %{CONSUL_AUTH_METHOD_NAME}
${RECONCILE_TIMEOUT}     60s
${RECONCILE_INTERVAL}    2s


*** Keywords ***
Apply ConsulACL CR
    [Arguments]    ${cr_yaml}
    Apply Custom Resource    ${cr_yaml}    ${TEST_NAMESPACE}

Delete ConsulACL CR
    [Arguments]    ${name}
    Delete Custom Resource    consulacls    ${name}    ${TEST_NAMESPACE}

Wait For ConsulACL Reconcile
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulACL Status Should Be Synced

ConsulACL Status Should Be Synced
    [Arguments]    ${name}=test-explicit-acl
    ${status}=    Get Custom Resource Status    consulacls    ${name}    ${TEST_NAMESPACE}
    Should Not Be Empty    ${status}


# ---- 18.1: ConsulACL with explicitName: true ----

Apply Explicit Name ACL CR With Both Entities
    Apply ConsulACL CR    ${EXECDIR}/resources/consulacl_explicit_full.yaml

Apply Explicit Name ACL CR With Role Removed
    Apply ConsulACL CR    ${EXECDIR}/resources/consulacl_explicit_no_role.yaml


*** Test Cases ***

# 18.1 — explicitName: true — verbatim names and stale entity removal on update
Test ConsulACL ExplicitName Verbatim Names
    [Tags]    acl-configurator    explicit-name
    Apply ConsulACL CR    ${EXECDIR}/resources/consulacl_explicit_full.yaml
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL Explicit Full Entities Should Exist

Test ConsulACL ExplicitName Removed Entity Deleted On Update
    [Tags]    acl-configurator    explicit-name
    Apply ConsulACL CR    ${EXECDIR}/resources/consulacl_explicit_no_role.yaml
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
    Apply ConsulACL CR    ${EXECDIR}/resources/consulacl_perrule_authmethod.yaml
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ACL PerRule AuthMethod Entities Should Exist
    [Teardown]    Delete ConsulACL CR    test-perrule-auth

ACL PerRule AuthMethod Entities Should Exist
    Acl Binding Rule Should Exist    integration_perrule_bind    integration-override-auth-method


# 18.3 — delete: all entities removed
Test ConsulACL Delete Removes All Entities
    [Tags]    acl-configurator    delete
    Apply ConsulACL CR    ${EXECDIR}/resources/consulacl_delete_test.yaml
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
