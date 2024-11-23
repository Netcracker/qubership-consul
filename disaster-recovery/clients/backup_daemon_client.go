// Copyright 2024-2025 NetCracker Technology Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	fullBackup            = "full backup"
	region                = "region"
	backupInterval        = 10 * time.Second
	defaultBackupTimeout  = 180 * time.Second
	restoreInterval       = 10 * time.Second
	defaultRestoreTimeout = 240 * time.Second
	successful            = "successful"
)

type BackupInfo struct {
	IsGranular bool              `json:"is_granular"`
	DBList     interface{}       `json:"db_list"`
	Id         string            `json:"id"`
	Failed     bool              `json:"failed"`
	Locked     bool              `json:"locked"`
	TS         int               `json:"ts"`
	SpentTime  string            `json:"spent_time"`
	Valid      bool              `json:"valid"`
	Evictable  bool              `json:"evictable"`
	CustomVars map[string]string `json:"custom_vars,omitempty"`
}

type Status struct {
	Status string `json:"status"`
	Vault  string `json:"vault"`
	Type   string `json:"type"`
	Err    string `json:"err"`
	TaskId string `json:"task_id"`
}

type BackupDaemonClient struct {
	logger         *logrus.Logger
	noWait         bool
	region         string
	backupTimeout  time.Duration
	restoreTimeout time.Duration
	restClient     *RestClient
}

func NewBackupDaemonClient(restClient *RestClient, noWait bool, logger *logrus.Logger) BackupDaemonClient {
	backupTimeout, err := time.ParseDuration(os.Getenv("DISASTER_RECOVERY_BACKUP_TIMEOUT"))
	if err != nil {
		logger.Warnf("Backup timeout specified incorrectly, setting default value - %s. See error: %v", defaultBackupTimeout.String(), err)
		backupTimeout = defaultBackupTimeout
	}
	restoreTimeout, err := time.ParseDuration(os.Getenv("DISASTER_RECOVERY_RESTORE_TIMEOUT"))
	if err != nil {
		logger.Warnf("Restore timeout specified incorrectly, setting default value - %s. See error: %v", defaultRestoreTimeout.String(), err)
		restoreTimeout = defaultRestoreTimeout
	}
	return BackupDaemonClient{
		logger:         logger,
		noWait:         noWait,
		region:         os.Getenv("REGION"),
		backupTimeout:  backupTimeout,
		restoreTimeout: restoreTimeout,
		restClient:     restClient,
	}
}

func (bdc BackupDaemonClient) getBackupsList() ([]string, error) {
	var backupsList []string
	response, err := bdc.restClient.SendRequestWithStatusCodeCheck(http.MethodGet, "listbackups", nil)
	if err != nil {
		bdc.logger.WithError(err).Error("Error occurred during getting list of backups")
		return backupsList, err
	}
	err = json.Unmarshal(response, &backupsList)
	if err != nil {
		bdc.logger.WithError(err).Errorf("Error occurred during unmarshalling response: %v", response)
	}
	return backupsList, err
}

func (bdc BackupDaemonClient) getBackupInformation(backupID string) (BackupInfo, error) {
	var backupInfo BackupInfo
	response, err := bdc.restClient.SendRequestWithStatusCodeCheck(http.MethodGet,
		fmt.Sprintf("listbackups/%s", backupID), nil)
	if err != nil {
		bdc.logger.WithError(err).Errorf("Error occurred during getting %s backup information", backupID)
		return backupInfo, err
	}
	err = json.Unmarshal(response, &backupInfo)
	if err != nil {
		bdc.logger.WithError(err).Errorf("Error occurred during unmarshalling %s backup response: %v", backupID,
			response)
	}
	return backupInfo, err
}

func (bdc BackupDaemonClient) getStatus(taskID string) (Status, int, error) {
	var status Status
	statusCode, response, err := bdc.restClient.SendRequest(http.MethodGet, fmt.Sprintf("jobstatus/%s", taskID), nil)
	bdc.logger.Debugf("Status code is %d, response is %s, err is %v", statusCode, response, err)
	if err != nil {
		bdc.logger.WithError(err).Errorf("Error occurred during getting %s task status", taskID)
		return status, statusCode, err
	}
	err = json.Unmarshal(response, &status)
	if err != nil {
		bdc.logger.WithError(err).Errorf("Error occurred during unmarshalling response: %v", response)
	}
	return status, statusCode, err
}

func (bdc BackupDaemonClient) getLastFullBackup() (string, error) {
	backupsList, err := bdc.getBackupsList()
	if err != nil {
		return "", err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(backupsList)))
	for _, backup := range backupsList {
		backupInfo, err := bdc.getBackupInformation(backup)
		if err != nil {
			return "", err
		}
		if dbList, ok := backupInfo.DBList.(string); ok && dbList == fullBackup && backupInfo.Valid &&
			backupInfo.CustomVars != nil && len(backupInfo.CustomVars) != 0 && backupInfo.CustomVars[region] != bdc.region {
			return backup, nil
		}
	}
	return "", nil
}

func (bdc BackupDaemonClient) PerformBackup() (string, error) {
	var vaultID string
	err := wait.PollUntilContextTimeout(context.Background(), backupInterval, bdc.backupTimeout, true, func(_ context.Context) (done bool, err error) {
		response, err := bdc.restClient.SendRequestWithStatusCodeCheck(http.MethodPost, "backup", nil)
		if err != nil {
			bdc.logger.WithError(err).Error("Can't perform backup. Trying again")
			return false, nil
		}
		vaultID = string(response)
		return true, nil
	})
	return vaultID, err
}

func (bdc BackupDaemonClient) CheckBackupStatus(backupID string) error {
	err := wait.PollUntilContextTimeout(context.Background(), backupInterval, bdc.backupTimeout, true, func(_ context.Context) (done bool, err error) {
		status, _, err := bdc.getStatus(backupID)
		if err != nil {
			bdc.logger.WithError(err).Errorf("Can't get %s backup status. Trying again", backupID)
			return false, nil
		}
		if strings.ToLower(status.Status) != successful {
			bdc.logger.Info("Backup is not completed yet")
			return false, nil
		}
		bdc.logger.Info("Backup is completed")
		return true, nil
	})
	return err
}

func (bdc BackupDaemonClient) RestoreLastFullBackup() (string, error) {
	var jobID string
	err := wait.PollUntilContextTimeout(context.Background(), restoreInterval, bdc.restoreTimeout, true, func(_ context.Context) (done bool, err error) {
		backupID, err := bdc.getLastFullBackup()
		if err != nil {
			bdc.logger.WithError(err).Error("Can't get last full backup. Trying again")
			return false, nil
		}
		if backupID == "" {
			if bdc.noWait {
				bdc.logger.Error("There is no backup to restore. Switchover process will be continued because 'noWait' parameter is set to 'true'")
				return true, nil
			} else {
				bdc.logger.Error("There is no backup to restore, attempt failed")
				return false, nil
			}
		}
		bdc.logger.Infof("The last full backup to restore is %s", backupID)
		jobID, err = bdc.performRestore(backupID)
		if err != nil {
			bdc.logger.WithError(err).Errorf("Can't perform restore of %s backup. Trying again", backupID)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		bdc.logger.WithError(err).Error("Error occurred during restoring last full backup")
	}
	return jobID, err
}

func (bdc BackupDaemonClient) performRestore(backupID string) (string, error) {
	requestBody := fmt.Sprintf(`{"vault": "%s"}`, backupID)
	response, err :=
		bdc.restClient.SendRequestWithStatusCodeCheck(http.MethodPost, "restore", strings.NewReader(requestBody))
	if err != nil {
		bdc.logger.WithError(err).Errorf("Error occurred during restoring %s backup", backupID)
		return "", err
	}
	return string(response), nil
}

func (bdc BackupDaemonClient) CheckRestoreStatus(jobID string) error {
	err := wait.PollUntilContextTimeout(context.Background(), restoreInterval, bdc.restoreTimeout, true, func(_ context.Context) (done bool, err error) {
		status, statusCode, err := bdc.getStatus(jobID)
		if err != nil {
			bdc.logger.WithError(err).Errorf("Can't get %s restore status. Trying again", jobID)
			return false, nil
		}
		if strings.ToLower(status.Status) != successful && statusCode != http.StatusNotFound {
			bdc.logger.Info("Restore of backup is not completed yet")
			return false, nil
		}
		bdc.logger.Info("Restore of backup is completed")
		return true, nil
	})
	return err
}
