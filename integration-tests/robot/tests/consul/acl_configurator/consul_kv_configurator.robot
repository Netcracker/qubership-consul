*** Settings ***
Resource    ../../shared/keywords.robot
Library     PlatformLibrary  managed_by_operator=true

*** Variables ***
${TEST_NAMESPACE}        %{CONSUL_NAMESPACE}
${RECONCILE_TIMEOUT}     60s
${RECONCILE_INTERVAL}    2s


*** Keywords ***
Apply ConsulKV CR
    [Arguments]    ${cr_yaml}
    Apply Custom Resource    ${cr_yaml}    ${TEST_NAMESPACE}

Delete ConsulKV CR
    [Arguments]    ${name}
    Delete Custom Resource    consulkvs    ${name}    ${TEST_NAMESPACE}


*** Test Cases ***

# 18.4 — apply: verbatim keys exist; re-apply is idempotent
Test ConsulKV Apply Creates Keys Verbatim
    [Tags]    kv-configurator    apply
    Apply ConsulKV CR    ${EXECDIR}/resources/consulkv_apply.yaml
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Apply Keys Should Exist

Test ConsulKV Reapply Is Idempotent
    [Tags]    kv-configurator    apply
    Apply ConsulKV CR    ${EXECDIR}/resources/consulkv_apply.yaml
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
    Apply ConsulKV CR    ${EXECDIR}/resources/consulkv_delete.yaml
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
    Apply ConsulKV CR    ${EXECDIR}/resources/consulkv_partial_failure.yaml
    Sleep    ${RECONCILE_INTERVAL}
    Wait Until Keyword Succeeds    ${RECONCILE_TIMEOUT}    ${RECONCILE_INTERVAL}
    ...    ConsulKV Partial Failure Assertions
    [Teardown]    Delete ConsulKV CR    test-consulkv-partial

ConsulKV Partial Failure Assertions
    Kv Key Should Exist    data/integration/partial/valid1
    Kv Key Should Exist    data/integration/partial/valid2
    ${status}=    Get Custom Resource Status    consulkvs    test-consulkv-partial    ${TEST_NAMESPACE}
    ${entries}=    Get From Dictionary    ${status}    entries
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
