*** Settings ***
Library  PlatformLibrary
Library  Collections

*** Variables ***
${CONSUL_NAMESPACE}  %{CONSUL_NAMESPACE}

*** Test Cases ***
Test Container Hardening
    [Tags]  hardening
    ${part_of}=  Create List  consul-service
    ${exclusions}=  Create Dictionary  _all=CH12
    Check Container Hardening  ${part_of}  ${CONSUL_NAMESPACE}  ${exclusions}
