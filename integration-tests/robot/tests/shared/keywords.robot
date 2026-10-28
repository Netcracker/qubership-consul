*** Variables ***
${CONSUL_NAMESPACE}                 %{CONSUL_NAMESPACE}
${CONSUL_HOST}                      %{CONSUL_HOST}
${CONSUL_PORT}                      %{CONSUL_PORT}
${CONSUL_SCHEME}                    %{CONSUL_SCHEME}
${MANAGED_BY_OPERATOR}              true
${CONSUL_API_RETRY_TIMEOUT}          1 min
${CONSUL_API_RETRY_INTERVAL}         5 s


*** Settings ***
Library  String
Variables  %{ROBOT_HOME}/SecretData.py
Library  PlatformLibrary  managed_by_operator=${MANAGED_BY_OPERATOR}
Library  lib/ConsulLibrary.py  consul_namespace=${CONSUL_NAMESPACE}
...                                         consul_host=${CONSUL_HOST}
...                                         consul_port=${CONSUL_PORT}
...                                         consul_scheme=${CONSUL_SCHEME}
...                                         consul_token=${CONSUL_TOKEN}


*** Keywords ***
Add Test Data To Consul
    [Arguments]  ${key}  ${value}
    ${response} =  Wait Until Keyword Succeeds  ${CONSUL_API_RETRY_TIMEOUT}  ${CONSUL_API_RETRY_INTERVAL}
    ...  Put Data  ${key}  ${value}
    Should Be True  ${response}

Get And Check Test Data From Consul
    [Arguments]  ${key}  ${value}
    ${response} =  Wait Until Keyword Succeeds  ${CONSUL_API_RETRY_TIMEOUT}  ${CONSUL_API_RETRY_INTERVAL}
    ...  Get Data  ${key}
    Should Be Equal As Strings  ${response}  ${value}

Update Test Data In Consul
    [Arguments]  ${key}  ${value}
    ${response} =  Wait Until Keyword Succeeds  ${CONSUL_API_RETRY_TIMEOUT}  ${CONSUL_API_RETRY_INTERVAL}
    ...  Put Data  ${key}  ${value}
    Should Be True  ${response}

Delete Test Data From Consul
    [Arguments]  ${key}
    ${response} =  Wait Until Keyword Succeeds  ${CONSUL_API_RETRY_TIMEOUT}  ${CONSUL_API_RETRY_INTERVAL}
    ...  Delete Data  ${key}
    Should Be True  ${response}
