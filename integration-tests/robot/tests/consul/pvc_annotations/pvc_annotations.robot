*** Settings ***
Library  Collections
Resource  ../../shared/keywords.robot
Suite Setup  Parse Expected Annotations

*** Variables ***
${CONSUL_FULLNAME}              %{CONSUL_FULLNAME}
${CONSUL_BACKUP_DAEMON_HOST}    %{CONSUL_BACKUP_DAEMON_HOST=}
${PVC_METADATA_ANNOTATIONS}     %{PVC_METADATA_ANNOTATIONS={}}
# Canonical annotation key used in the unannotated-path checks
${ARGOCD_PRUNE_KEY}             argocd.argoproj.io/sync-options

*** Keywords ***
Parse Expected Annotations
    ${parsed}=  Parse Json Annotations  ${PVC_METADATA_ANNOTATIONS}
    Set Suite Variable  ${expected_annotations}  ${parsed}

Check PVC Has Expected Annotations
    [Arguments]  ${pvc_name}
    ${annotations}=  Get Pvc Annotations  ${pvc_name}
    FOR  ${key}  ${value}  IN  &{expected_annotations}
        Dictionary Should Contain Key  ${annotations}  ${key}
        ...  msg=PVC '${pvc_name}' is missing annotation '${key}'
        Should Be Equal As Strings  ${annotations}[${key}]  ${value}
        ...  msg=PVC '${pvc_name}': annotation '${key}' has wrong value
    END

Check PVC Has No Argocd Prune Annotation
    [Arguments]  ${pvc_name}
    ${annotations}=  Get Pvc Annotations  ${pvc_name}
    Dictionary Should Not Contain Key  ${annotations}  ${ARGOCD_PRUNE_KEY}
    ...  msg=PVC '${pvc_name}' unexpectedly contains '${ARGOCD_PRUNE_KEY}'

*** Test Cases ***
Test Server PVC Has Configured Annotations
    [Tags]  smoke  pvc_annotations
    ${annotation_count}=  Get Length  ${expected_annotations}
    Pass Execution If  ${annotation_count} == 0
    ...  Skipped: no custom PVC annotations configured (unannotated path)
    ${pvc_name}=  Set Variable  data-${CONSUL_NAMESPACE}-${CONSUL_FULLNAME}-server-0
    Check PVC Has Expected Annotations  ${pvc_name}

Test Server PVC Has No Custom Annotations When Not Configured
    [Tags]  smoke  pvc_annotations
    ${annotation_count}=  Get Length  ${expected_annotations}
    Pass Execution If  ${annotation_count} > 0
    ...  Skipped: custom PVC annotations are configured (annotated path)
    ${pvc_name}=  Set Variable  data-${CONSUL_NAMESPACE}-${CONSUL_FULLNAME}-server-0
    Check PVC Has No Argocd Prune Annotation  ${pvc_name}

Test Backup Daemon PVC Has Configured Annotations
    [Tags]  smoke  pvc_annotations  backup
    Pass Execution If  '${CONSUL_BACKUP_DAEMON_HOST}' == ''
    ...  Skipped: backup daemon is not configured
    ${pvc_name}=  Set Variable  data-${CONSUL_FULLNAME}-backup-daemon
    ${exists}=  Pvc Exists  ${pvc_name}
    Pass Execution If  not ${exists}
    ...  Skipped: backup daemon PVC does not exist (no persistent storage configured)
    ${annotation_count}=  Get Length  ${expected_annotations}
    Pass Execution If  ${annotation_count} == 0
    ...  Skipped: no custom PVC annotations configured (unannotated path)
    Check PVC Has Expected Annotations  ${pvc_name}

Test Backup Daemon PVC Has No Custom Annotations When Not Configured
    [Tags]  smoke  pvc_annotations  backup
    Pass Execution If  '${CONSUL_BACKUP_DAEMON_HOST}' == ''
    ...  Skipped: backup daemon is not configured
    ${pvc_name}=  Set Variable  data-${CONSUL_FULLNAME}-backup-daemon
    ${exists}=  Pvc Exists  ${pvc_name}
    Pass Execution If  not ${exists}
    ...  Skipped: backup daemon PVC does not exist (no persistent storage configured)
    ${annotation_count}=  Get Length  ${expected_annotations}
    Pass Execution If  ${annotation_count} > 0
    ...  Skipped: custom PVC annotations are configured (annotated path)
    Check PVC Has No Argocd Prune Annotation  ${pvc_name}
