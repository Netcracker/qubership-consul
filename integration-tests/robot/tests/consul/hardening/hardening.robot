*** Settings ***
Resource  ../../shared/keywords.robot

*** Test Cases ***
Test Container Hardening
    [Tags] hardening
    ${part_of}=  Create List  consul-service
    ${consul_client}=  Remove String  ${CONSUL_HOST}  -server
    ${exclusions}=  Create Dictionary  _all=CH12
    Check Container Hardening    ${part_of}    ${CONSUL_NAMESPACE}    ${exclusions}
