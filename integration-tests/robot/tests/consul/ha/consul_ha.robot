*** Variables ***
${COUNT_OF_RETRY}            10x
${RETRY_INTERVAL}            5s
${FOLDER}                    test_folder
${SLEEP}                     15s

*** Settings ***
Library  OperatingSystem
Resource  ../../shared/keywords.robot
Suite Setup  Preparation

*** Keywords ***
Preparation
    ${random_id} =  Generate Random String  3  [LOWER]
    Set Suite Variable  ${test_key}  ha_key_${random_id}
    Set Suite Variable  ${test_value}  ha_value_${random_id}

Check CRUD Operations
    Add Test Data To Consul  ${test_key}  ${test_value}
    Get And Check Test Data From Consul  ${test_key}  ${test_value}
    Update Test Data In Consul  ${test_key}  update_${test_value}
    Get And Check Test Data From Consul  ${test_key}  update_${test_value}
    Delete Test Data From Consul  ${test_key}

Get Consul Leader
    ${response} =  Get Leader
    [Return]  ${response}

Get IP For Consul Leader
    [Arguments]  ${leader}
    ${resp} =  Delete Port  ${leader}
    [Return]  ${resp}

Delete Consul Leader Pod
    [Arguments]  ${leader_ip}
    Delete Pod By Pod IP  pod_ip=${leader_ip}  namespace=${CONSUL_NAMESPACE}

Check Leader Reelection
    [Arguments]  ${leader_old}
    ${list} =  Get List Peers
    Wait Until Keyword Succeeds  ${COUNT_OF_RETRY}  ${RETRY_INTERVAL}
    ...  Leader Should Be Presented
    ${leader_new} =  Get Consul Leader
    ${resp} =  Is Leader Reelected  ${leader_new}  ${leader_old}  ${list}
    Should Be True  ${resp}

Leader Should Be Presented
    ${resp} =  Get Consul Leader
    Should Not Be Empty  ${resp}

Get Leader Pod Name By IP
    [Arguments]  ${pod_ip}
    ${resp} =  Look Up Pod Name By Host IP  ${pod_ip}  ${CONSUL_NAMESPACE}
    [Return]  ${resp}

*** Test Cases ***
Test Value With Exceeding Limit Size
    [Tags]  ha  exceeding_limit_size
    ${text}=  Get File  ${CURDIR}/extremely_big_value.txt  UTF-8
    ${response}=  Put Data Using Request  ${FOLDER}/${test_key}  ${text}
    Should Be Equal As Strings  ${response.status_code}  413
    ${response}=  Put Data Using Request  ${FOLDER}/${test_key}  ${test_value}
    Should Be Equal As Strings  ${response.status_code}  200
    ${data}=  Get Data  ${FOLDER}/${test_key}
    Should Be Equal As Strings  ${data}  ${test_value}
    Delete Data  ${FOLDER}/${test_key}

Test Leader Node Deleted
    [Tags]  ha  leader_node_deleted
    ${replicas_counts} =  Get Stateful Set Replica Counts  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}
    Pass Execution If  ${replicas_counts} < 3
    ...  The test is skipped due to insufficient number of Consul servers for this test (minimum value is 3 replicas).
    ${resp} =  Get Consul Leader
    ${leader_ip} =  Get IP For Consul Leader  ${resp}
    Check CRUD Operations
    Delete Consul Leader Pod  ${leader_ip}
    Wait Until Keyword Succeeds  ${COUNT_OF_RETRY}  ${RETRY_INTERVAL}
    ...  Check Leader Reelection  ${resp}
    Sleep  ${SLEEP}
    Wait Until Keyword Succeeds  ${COUNT_OF_RETRY}  ${RETRY_INTERVAL}
    ...  Check CRUD Operations
    Check CRUD Operations
    Sleep  30s
