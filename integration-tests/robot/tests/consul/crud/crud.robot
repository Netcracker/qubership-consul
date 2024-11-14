*** Settings ***
Resource  ../../shared/keywords.robot
Suite Setup  Preparation


*** Keywords ***
Preparation
    ${random_id} =  Generate Random String  3  [LOWER]
    Set Suite Variable  ${test_key}  test_key_${random_id}
    Set Suite Variable  ${path_test_key}  path_test_key/test_data_${random_id}
    Set Suite Variable  ${test_value}  test_value_${random_id}


*** Test Cases ***
Test Add Data
    [Tags]  smoke  crud
    Add Test Data To Consul  ${test_key}  ${test_value}

Test Read Data
    [Tags]  smoke  crud
    Get And Check Test Data From Consul  ${test_key}  ${test_value}

Test Update Data
    [Tags]  smoke  crud
    Update Test Data In Consul  ${test_key}  update_${test_value}
    Get And Check Test Data From Consul  ${test_key}  update_${test_value}

Test Delete Data
    [Tags]  smoke  crud
    Delete Test Data From Consul  ${test_key}

Test Create Key Under Path
    [Tags]  smoke  crud
    Add Test Data To Consul  ${path_test_key}  path_${test_value}

Test Read Data Under Path
    [Tags]  smoke  crud
    Get And Check Test Data From Consul  ${path_test_key}  path_${test_value}

Test Update Data Under Path
    [Tags]  smoke  crud
    Update Test Data In Consul  ${path_test_key}  update_path_${test_value}
    Get And Check Test Data From Consul  ${path_test_key}  update_path_${test_value}

Test Delete Data Under Path
    [Tags]  smoke  crud
    Delete Test Data From Consul  ${path_test_key}


