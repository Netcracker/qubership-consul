*** Variables ***
${CONSUL_BACKUP_DAEMON_HOST}      %{CONSUL_BACKUP_DAEMON_HOST}
${CONSUL_BACKUP_DAEMON_PORT}      %{CONSUL_BACKUP_DAEMON_PORT}
${CONSUL_BACKUP_DAEMON_USERNAME}  %{CONSUL_BACKUP_DAEMON_USERNAME=}
${CONSUL_BACKUP_DAEMON_PASSWORD}  %{CONSUL_BACKUP_DAEMON_PASSWORD=}
${CONSUL_BACKUP_DAEMON_PROTOCOL}  %{CONSUL_BACKUP_DAEMON_PROTOCOL}
${DATACENTER_NAME}                %{DATACENTER_NAME}
${BACKUP_TIMEOUT}                 2min
${BACKUP_TIME_INTERVAL}           10s
${RESTORE_TIMEOUT}                2min
${RESTORE_TIME_INTERVAL}          10s

*** Settings ***
Library  String
Library   Collections
Library   RequestsLibrary
Resource  ../../shared/keywords.robot
Suite Setup  Preparation

*** Keywords ***
Preparation
    ${auth}  Create List  ${CONSUL_BACKUP_DAEMON_USERNAME}  ${CONSUL_BACKUP_DAEMON_PASSWORD}
    ${headers}  Create Dictionary  Content-Type=application/json  Accept=application/json
    Set Suite Variable  ${headers}
    ${verify}=  Set Variable If  '${CONSUL_BACKUP_DAEMON_PROTOCOL}' == 'https'  /consul/tls/backup/ca.crt  ${True}
    Create Session  backupsession  ${CONSUL_BACKUP_DAEMON_PROTOCOL}://${CONSUL_BACKUP_DAEMON_HOST}:${CONSUL_BACKUP_DAEMON_PORT}  auth=${auth}  verify=${verify}
    Create Unique Key And Value

Create Unique Key And Value
    ${random_id} =  Generate Random String  3  [LOWER]
    Set Suite Variable  ${test_key}  test_key_${random_id}
    Set Suite Variable  ${test_value}  test_value_${random_id}

Convert Json ${json} To Type
    ${json_dictionary}  Evaluate  json.loads('''${json}''')  json
    [Return]  ${json_dictionary}

Create Test Data
    Add Test Data To Consul  ${test_key}  ${test_value}
    Get And Check Test Data From Consul  ${test_key}  ${test_value}

Full Backup
    ${response}=  Post Request  backupsession  /backup
    Should Be Equal As Strings  ${response.status_code}  200
    ${backup_id}=  Set Variable  ${response.content}
    Wait Until Keyword Succeeds  ${BACKUP_TIMEOUT}  ${BACKUP_TIME_INTERVAL}
    ...  Check Backup Status  ${backup_id}  ${False}
    [Return]  ${backup_id}

Check Backup Status
    [Arguments]  ${backup_id}  ${is_granular}
    ${status}=  Get Request  backupsession  /listbackups/${backup_id}
    ${content}=  Set Variable  ${status.json()}
    Should Be Equal As Strings  ${content['failed']}  False
    Should Be Equal As Strings  ${content['valid']}  True
    Should Be Equal As Strings  ${content['is_granular']}  ${is_granular}

Delete Test Data
    Delete Test Data From Consul  ${test_key}

Full Restore
    [Arguments]  ${backup_id}
    ${restore_data}=  Set Variable  {"vault":"${backup_id}","dbs":["${DATACENTER_NAME}"],"skip_acl_recovery":"true"}
    ${response}=  Post Request  backupsession  /restore  data=${restore_data}  headers=${headers}
    Should Be Equal As Strings  ${response.status_code}   200
    Wait Until Keyword Succeeds  ${RESTORE_TIMEOUT}  ${RESTORE_TIME_INTERVAL}
    ...  Check Restore Status  ${response.content}

Check Restore Status
    [Arguments]  ${task_id}
    ${status}=  Get Request  backupsession  /jobstatus/${task_id}
    Should Be Equal As Strings  ${status.json()['status']}  Successful

Check Data In Key
    Get And Check Test Data From Consul  ${test_key}  ${test_value}

Granular Backup
    ${data}=  Set Variable  {"dbs":["${DATACENTER_NAME}"]}
    ${response}=  Post Request  backupsession  /backup  data=${data}  headers=${headers}
    ${backup_id}=  Set Variable  ${response.content}
    Wait Until Keyword Succeeds  ${BACKUP_TIMEOUT}  ${BACKUP_TIME_INTERVAL}
    ...  Check Backup Status  ${backup_id}  ${True}
    [Return]  ${backup_id}

Delete Backup From Backup Daemon
    [Arguments]  ${backup_id}
    ${resp_delete}=  Post Request  backupsession  /evict/${backup_id}
    Should Be Equal As Strings  ${resp_delete.status_code}   200
    ${list_backups} =  Get Request  backupsession  /listbackups
    Should Not Contain  ${list_backups.content}  ${backup_id}

*** Test Cases ***
Test Full Backup And Restore
    [Tags]  backup  full_backup
    Create Test Data
    ${backup_id} =  Full Backup
    Set Suite Variable  ${backup_id}
    Delete Test Data
    Full Restore  ${backup_id}
    Check Data In Key
    [Teardown]  Delete Test Data

Test Granular Backup And Restore
    [Tags]  backup  granular_backup
    Create Unique Key And Value
    Create Test Data
    ${granular_backup_id} =  Granular Backup
    Set Suite Variable  ${granular_backup_id}
    Delete Test Data
    Full Restore  ${granular_backup_id}
    Check Data In Key
    [Teardown]  Delete Test Data

Test Evict Backup By Id
    [Tags]  backup  backup_eviction
    ${backup_id} =  Full Backup
    Delete Backup From Backup Daemon  ${backup_id}

Test Unauthorized Access
    [Tags]  backup  unauthorized_access
    ${verify}=  Set Variable If  '${CONSUL_BACKUP_DAEMON_PROTOCOL}' == 'https'  /consul/tls/backup/ca.crt  ${True}
    Create Session  backupsession_unauthorized  ${CONSUL_BACKUP_DAEMON_PROTOCOL}://${CONSUL_BACKUP_DAEMON_HOST}:${CONSUL_BACKUP_DAEMON_PORT}  verify=${verify}
    ${resp_backup}=  Post Request  backupsession_unauthorized  /backup
    Should Be Equal As Strings  ${resp_backup.status_code}  401
