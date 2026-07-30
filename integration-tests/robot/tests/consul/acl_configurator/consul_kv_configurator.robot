*** Settings ***
Resource    ../../shared/keywords.robot
Library     PlatformLibrary  managed_by_operator=true
Library     Collections

*** Variables ***
${TEST_NAMESPACE}        %{CONSUL_NAMESPACE}
${RECONCILE_TIMEOUT}     60s
${RECONCILE_INTERVAL}    2s
${GROUP}                 netcracker.com
${VERSION}               v1alpha1


*** Keywords ***
Apply ConsulKV CR
    [Arguments]    ${name}    ${body}
    ${result}    ${value}=    Run Keyword And Ignore Error
    ...    Get Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulkvs    ${name}
    IF    '${result}' == 'PASS'
        Replace Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulkvs    ${name}    ${body}
    ELSE
        Create Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulkvs    ${body}
    END

Delete ConsulKV CR
    [Arguments]    ${name}
    Delete Namespaced Custom Object    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulkvs    ${name}

Build KV Entry
    [Arguments]    ${key}    ${value}=${EMPTY}
    IF    '${value}' != '${EMPTY}'
        &{entry}=    Create Dictionary    key=${key}    value=${value}
    ELSE
        &{entry}=    Create Dictionary    key=${key}
    END
    RETURN    &{entry}

Build ConsulKV Body
    [Arguments]    ${name}    @{entries}
    &{metadata}=    Create Dictionary    name=${name}    namespace=${TEST_NAMESPACE}
    &{kv}=          Create Dictionary    entries=@{entries}
    &{spec}=        Create Dictionary    kv=&{kv}
    &{body}=        Create Dictionary    apiVersion=netcracker.com/v1alpha1    kind=ConsulKV    metadata=&{metadata}    spec=&{spec}
    RETURN    &{body}


*** Test Cases ***

# 18.4 — apply: verbatim keys exist; re-apply is idempotent
Test ConsulKV Apply Creates Keys Verbatim
    [Tags]    kv-configurator    apply
    &{e1}=    Build KV Entry    config/integration/test-app/
    &{e2}=    Build KV Entry    logging/integration/test-app/LOG_LEVEL    INFO
    &{body}=    Build ConsulKV Body    test-consulkv-apply    ${e1}    ${e2}
    Apply ConsulKV CR    test-consulkv-apply    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Apply Keys Should Exist

Test ConsulKV Reapply Is Idempotent
    [Tags]    kv-configurator    apply
    &{e1}=    Build KV Entry    config/integration/test-app/
    &{e2}=    Build KV Entry    logging/integration/test-app/LOG_LEVEL    INFO
    &{body}=    Build ConsulKV Body    test-consulkv-apply    ${e1}    ${e2}
    Apply ConsulKV CR    test-consulkv-apply    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Apply Keys Should Exist
    [Teardown]    Delete ConsulKV CR    test-consulkv-apply

ConsulKV Apply Keys Should Exist
    Kv Key Should Exist    config/integration/test-app/
    Kv Key Should Exist    logging/integration/test-app/LOG_LEVEL


# 18.5 — delete: all keys removed
Test ConsulKV Delete Removes All Keys
    [Tags]    kv-configurator    delete
    &{e1}=    Build KV Entry    data/integration/delete-test/key1    val1
    &{e2}=    Build KV Entry    data/integration/delete-test/key2    val2
    &{body}=    Build ConsulKV Body    test-consulkv-delete    ${e1}    ${e2}
    Apply ConsulKV CR    test-consulkv-delete    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Delete Keys Should Exist
    Delete ConsulKV CR    test-consulkv-delete
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Delete Keys Should Not Exist

ConsulKV Delete Keys Should Exist
    Kv Key Should Exist    data/integration/delete-test/key1
    Kv Key Should Exist    data/integration/delete-test/key2

ConsulKV Delete Keys Should Not Exist
    Kv Key Should Not Exist    data/integration/delete-test/key1
    Kv Key Should Not Exist    data/integration/delete-test/key2


# 18.6 — partial failure: valid keys written, empty-key error in .status
Test ConsulKV Partial Failure Valid Keys Written Error Status Recorded
    [Tags]    kv-configurator    partial-failure
    &{e_empty}=    Build KV Entry    ${EMPTY}    this entry has an empty key and must produce an error status
    &{e1}=         Build KV Entry    data/integration/partial/valid1    v1
    &{e2}=         Build KV Entry    data/integration/partial/valid2    v2
    &{body}=    Build ConsulKV Body    test-consulkv-partial    ${e_empty}    ${e1}    ${e2}
    Apply ConsulKV CR    test-consulkv-partial    ${body}
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Partial Failure Assertions
    [Teardown]    Delete ConsulKV CR    test-consulkv-partial

ConsulKV Partial Failure Assertions
    Kv Key Should Exist    data/integration/partial/valid1
    Kv Key Should Exist    data/integration/partial/valid2
    ${cr}=          Get Namespaced Custom Object Status    ${GROUP}    ${VERSION}    ${TEST_NAMESPACE}    consulkvs    test-consulkv-partial
    ${cr_status}=   Get From Dictionary    ${cr}    status
    ${entries}=     Get From Dictionary    ${cr_status}    entries
    ${error_found}=    Set Variable    ${FALSE}
    FOR    ${entry}    IN    @{entries}
        ${key}=    Get From Dictionary    ${entry}    key
        ${s}=      Get From Dictionary    ${entry}    status
        IF    '${key}' == ''
            Should Contain    ${s}    error
            ${error_found}=    Set Variable    ${TRUE}
        END
    END
    Should Be True    ${error_found}    msg=Empty-key entry error should be recorded in .status
