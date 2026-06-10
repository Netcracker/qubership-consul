*** Settings ***
Resource  ../../shared/keywords.robot

*** Test Cases ***
Test Container Hardening
    [Tags]  consul_container_hardening
    ${part_of}=  Create List  consul-service
    ${consul_client}=  Remove String  ${CONSUL_HOST}  -server
    ${exclusions}=  Create Dictionary
    ...  _all=CH12
    ...  ${consul_client}=CH4
    ...  ${CONSUL_HOST}=CH4
    ...  ${CONSUL_HOST}-mesh-gateway=CH4
    Check Container Hardening    ${part_of}    ${CONSUL_NAMESPACE}    ${exclusions}
