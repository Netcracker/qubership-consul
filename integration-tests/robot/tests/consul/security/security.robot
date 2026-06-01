*** Variables ***
${PART_OF}    %{PART_OF=consul-service}

*** Settings ***
Resource  ../../shared/keywords.robot

*** Test Cases ***
Test No Secrets Exposed As Environment Variables
    [Tags]  security  ch12
    [Documentation]    CH12: Verifies that no container in the namespace exposes Kubernetes
    ...                Secrets via env.valueFrom.secretKeyRef or envFrom.secretRef.
    ...                All secrets must be mounted as read-only files instead.
    ${part_of}=       Create List         ${PART_OF}
    ${exclusions}=    Create Dictionary   _all=CH1,CH2,CH3,CH4,CH5,CH6,CH7,CH8,CH9,CH10,CH11
    Check Container Hardening    ${part_of}    ${CONSUL_NAMESPACE}    ${exclusions}
