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

package main

import (
	"crypto/tls"
	"crypto/x509"
	"disaster-recovery/clients"
	"errors"
	"fmt"
	"github.com/Netcracker/qubership-disaster-recovery-daemon/api/entity"
	"github.com/Netcracker/qubership-disaster-recovery-daemon/config"
	"github.com/Netcracker/qubership-disaster-recovery-daemon/controller"
	"github.com/Netcracker/qubership-disaster-recovery-daemon/server"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	certificateFilePath = "/tls/backup/ca.crt"
	statusMode          = "status-mode"
	statusStatus        = "status-status"
)

var logger = logrus.WithFields(logrus.Fields{"name": "disasterrecovery"}).Logger

func main() {
	level := logrus.InfoLevel
	if _, found := os.LookupEnv("DEBUG"); found {
		level = logrus.DebugLevel
	}
	logger.SetLevel(level)
	// Make a config loader
	cfgLoader := config.GetDefaultEnvConfigLoader()

	// Build a config
	cfg, err := config.NewConfig(cfgLoader)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// Start DRD server
	go server.NewServer(cfg).Run()

	// Start DRD controller with external DR function
	controller.NewController(cfg).
		WithFunc(drFunction).
		Run()
	logger.Info("All procedures finished.")
}

func drFunction(controllerRequest entity.ControllerRequest) (entity.ControllerResponse, error) {
	var configMap corev1.ConfigMap
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(controllerRequest.Object, &configMap)
	if err != nil {
		return entity.ControllerResponse{}, err
	}

	if configMap.Data[statusMode] != "" && configMap.Data[statusMode] != controllerRequest.Mode ||
		configMap.Data[statusStatus] == entity.RUNNING || configMap.Data[statusStatus] == entity.FAILED {
		consulFullName := os.Getenv("CONSUL_FULLNAME")
		kubeClient := clients.NewKubernetesClient(configMap.Namespace, logger)
		err = kubeClient.CheckStatefulSetReadiness(fmt.Sprintf("%s-server", consulFullName))
		if err != nil {
			return entity.ControllerResponse{}, err
		}
		backupDaemonClient :=
			getBackupDaemonClient(consulFullName, configMap.Namespace, kubeClient, *controllerRequest.NoWait)
		backupDaemonName := fmt.Sprintf("%s-backup-daemon", consulFullName)
		backupDaemonLabels := getBackupDaemonLabels(backupDaemonName)
		if strings.ToLower(controllerRequest.Mode) == entity.ACTIVE {
			logger.Info("Scaling up backup daemon")
			err = kubeClient.ScaleDeploymentWithCheck(backupDaemonName, 1, backupDaemonLabels)
			if err != nil {
				return entity.ControllerResponse{}, err
			}
			logger.Info("Backup daemon is scaled up")

			if configMap.Data[statusMode] != entity.DISABLED {
				if err = startLastBackupRecovery(kubeClient, backupDaemonClient, consulFullName); err != nil {
					return entity.ControllerResponse{}, err
				}
			} else {
				logger.Info("Skip last backup restoring")
			}
		} else if strings.ToLower(controllerRequest.Mode) == entity.STANDBY &&
			strings.ToLower(configMap.Data["mode"]) != entity.DISABLED {
			isDeploymentScaledUp, err := kubeClient.IsDeploymentScaledUp(backupDaemonName)
			if err != nil {
				return entity.ControllerResponse{}, err
			}
			if isDeploymentScaledUp {
				logger.Info("Starting backup")
				backupID, err := backupDaemonClient.PerformBackup()
				if err != nil {
					return entity.ControllerResponse{}, err
				}

				logger.Infof("Backup is started: %s, checking status", backupID)
				if err = backupDaemonClient.CheckBackupStatus(backupID); err != nil {
					return entity.ControllerResponse{}, err
				}

				logger.Info("Scaling down backup daemon")
				if err = kubeClient.ScaleDeploymentWithCheck(backupDaemonName, 0, backupDaemonLabels); err != nil {
					return entity.ControllerResponse{}, err
				}
			}
			logger.Info("Backup daemon is scaled down")
		} else if strings.ToLower(controllerRequest.Mode) == entity.DISABLED {
			logger.Info("Scaling down backup daemon for disable mode")
			if err := kubeClient.ScaleDeploymentWithCheck(backupDaemonName, 0, backupDaemonLabels); err != nil {
				return entity.ControllerResponse{}, err
			}
			logger.Info("Backup daemon is scaled down for disable mode")
		}

	}

	return entity.ControllerResponse{
		SwitchoverState: entity.SwitchoverState{
			Mode:    controllerRequest.Mode,
			Status:  entity.DONE,
			Comment: "switchover successfully done",
		},
	}, nil
}

func startLastBackupRecovery(kubeClient clients.KubernetesClient, backupDaemonClient clients.BackupDaemonClient, consulFullName string) error {
	logger.Info("Starting last backup recovery")
	jobID, err := backupDaemonClient.RestoreLastFullBackup()
	if err != nil {
		return err
	}

	if jobID != "" {
		logger.Infof("Restore of backup is started: %s, checking status", jobID)
		if err = backupDaemonClient.CheckRestoreStatus(jobID); err != nil {
			return err
		}
		logger.Info("Checking Consul servers are ready")
		err = kubeClient.CheckStatefulSetReadiness(fmt.Sprintf("%s-server", consulFullName))
		if err != nil {
			return err
		}
		logger.Info("Consul servers are ready")
	}

	return nil
}

func getBackupDaemonClient(name string, namespace string, kubeClient clients.KubernetesClient, noWait bool) clients.BackupDaemonClient {
	url := getBackupDaemonUrl(name, namespace)
	secretName := fmt.Sprintf("%s-backup-daemon-secret", name)
	credentials := kubeClient.GetSecretCredentials(secretName, "username", "password")
	restClient := clients.NewRestClient(url, configureClient(), credentials)
	return clients.NewBackupDaemonClient(restClient, noWait, logger)
}

func getBackupDaemonLabels(backupDaemonName string) map[string]string {
	return map[string]string{
		"name":      backupDaemonName,
		"component": "backup-daemon",
	}
}

func getBackupDaemonUrl(name string, namespace string) string {
	protocol := "https"
	if _, err := os.Stat(certificateFilePath); errors.Is(err, os.ErrNotExist) {
		protocol = "http"
		return fmt.Sprintf("%s://%s-backup-daemon.%s:8080", protocol, name, namespace)
	}
	return fmt.Sprintf("%s://%s-backup-daemon.%s:8443", protocol, name, namespace)
}

func configureClient() http.Client {
	httpClient := http.Client{}
	if _, err := os.Stat(certificateFilePath); errors.Is(err, os.ErrNotExist) {
		return httpClient
	}
	caCert, err := os.ReadFile(certificateFilePath)
	if err != nil {
		logger.WithError(err).Errorf("Error occurred during reading certificate from %s path", certificateFilePath)
		return httpClient
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caCertPool,
		},
	}
	return httpClient
}
