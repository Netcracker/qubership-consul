*** Variables ***
${CONSUL_DOES_NOT_EXIST_ALERT_NAME}      ConsulDoesNotExistAlarm
${CONSUL_IS_DEGRADED_ALERT_NAME}         ConsulIsDegradedAlarm
${CONSUL_IS_DOWN_ALERT_NAME}             ConsulIsDownAlarm
${ALERT_RETRY_TIME}                      5min
${ALERT_RETRY_INTERVAL}                  1s

*** Settings ***
Library  MonitoringLibrary  host=%{PROMETHEUS_URL}
...                         username=%{PROMETHEUS_USER}
...                         password=%{PROMETHEUS_PASSWORD}
Resource  ../../shared/keywords.robot

*** Keywords ***
Check That Prometheus Alert Is Active
    [Arguments]  ${alert_name}
    ${status}=  Get Alert Status  ${alert_name}  ${CONSUL_NAMESPACE}
    Should Be Equal As Strings  ${status}  pending

Check That Prometheus Alert Is Inactive
    [Arguments]  ${alert_name}
    ${status}=  Get Alert Status  ${alert_name}  ${CONSUL_NAMESPACE}
    Should Be Equal As Strings  ${status}  inactive

Get Leader IP
    ${leader}=  Get Leader
    ${resp} =  Delete Port  ${leader}
    [Return]  ${resp}

Check Servers Readiness
    ${replicas}=  Get Stateful Set Replicas Count  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}
    ${ready_replicas}=  Get Stateful Set Ready Replicas Count  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}
    Should Be Equal As Strings  ${replicas}  ${ready_replicas}

Check That Consul Servers Are Up
    [Arguments]  ${expected_replicas}
    ${replicas}=  Get Stateful Set Replicas Count  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}
    Run Keyword If  "${replicas}" != "${expected_replicas}"
    ...  Scale Up Stateful Sets By Service Name  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}  replicas=${expected_replicas}  with_check=True

Check Alerts Are Inactive
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Run Keywords
    ...  Check That Prometheus Alert Is Inactive  ${CONSUL_DOES_NOT_EXIST_ALERT_NAME}
    ...  AND  Check That Prometheus Alert Is Inactive  ${CONSUL_IS_DEGRADED_ALERT_NAME}
    ...  AND  Check That Prometheus Alert Is Inactive  ${CONSUL_IS_DOWN_ALERT_NAME}
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check Leader Using Request

Delete Server Pods
    ${server_ips}=  Get Server Ips List
    FOR  ${server_ip}  IN  @{server_ips}
        Delete Pod By Pod Ip  ${server_ip}  ${CONSUL_NAMESPACE}
    END

*** Test Cases ***
Consul Does Not Exist Alert
    [Tags]  alerts  consul_does_not_exist_alert
    Check That Prometheus Alert Is Inactive  ${CONSUL_DOES_NOT_EXIST_ALERT_NAME}
    ${replicas}=  Get Stateful Set Replicas Count  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}
    Pass Execution If  ${replicas} < 3  Consul cluster has less than 3 servers
    Set Replicas For Stateful Set  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}  0
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check That Prometheus Alert Is Active  ${CONSUL_DOES_NOT_EXIST_ALERT_NAME}
    Set Replicas For Stateful Set  ${CONSUL_HOST}  ${CONSUL_NAMESPACE}  ${replicas}
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check That Prometheus Alert Is Inactive  ${CONSUL_DOES_NOT_EXIST_ALERT_NAME}
    [Teardown]  Run Keywords  Check That Consul Servers Are Up  ${replicas}
                ...  AND  Check Alerts Are Inactive

Consul Is Degraded Alert
    [Tags]  alerts  consul_is_degraded_alert
    Check That Prometheus Alert Is Inactive  ${CONSUL_IS_DEGRADED_ALERT_NAME}
    Check Servers Readiness
    ${leader_ip}=  Get Leader IP
    Delete Pod By Pod Ip  ${leader_ip}  ${CONSUL_NAMESPACE}
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check That Prometheus Alert Is Active  ${CONSUL_IS_DEGRADED_ALERT_NAME}
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check That Prometheus Alert Is Inactive  ${CONSUL_IS_DEGRADED_ALERT_NAME}
    [Teardown]  Check Alerts Are Inactive

Consul Is Down Alert
    [Tags]  alerts  consul_is_down_alert
    Check That Prometheus Alert Is Inactive  ${CONSUL_IS_DOWN_ALERT_NAME}
    Check Servers Readiness
    Delete Server Pods
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check That Prometheus Alert Is Active  ${CONSUL_IS_DOWN_ALERT_NAME}
    Wait Until Keyword Succeeds  ${ALERT_RETRY_TIME}  ${ALERT_RETRY_INTERVAL}
    ...  Check That Prometheus Alert Is Inactive  ${CONSUL_IS_DOWN_ALERT_NAME}
    [Teardown]  Check Alerts Are Inactive
