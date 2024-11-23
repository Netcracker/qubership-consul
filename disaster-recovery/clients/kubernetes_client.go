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
	"github.com/Netcracker/qubership-disaster-recovery-daemon/client"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"time"
)

const (
	interval = 10 * time.Second
	timeout  = 120 * time.Second
)

type KubernetesClient struct {
	client    *kubernetes.Clientset
	logger    *logrus.Logger
	namespace string
}

func NewKubernetesClient(namespace string, logger *logrus.Logger) KubernetesClient {
	return KubernetesClient{
		client:    client.MakeKubeClientSet(),
		logger:    logger,
		namespace: namespace,
	}
}

func (kc KubernetesClient) findSecret(name string) (*corev1.Secret, error) {
	kc.logger.Infof("Trying to find [%s] secret", name)
	return kc.client.CoreV1().Secrets(kc.namespace).Get(context.TODO(), name, v1.GetOptions{})
}

func (kc KubernetesClient) GetSecretCredentials(name string, keys ...string) []string {
	var credentials []string
	secret, err := kc.findSecret(name)
	if err != nil {
		kc.logger.Infof("Problem occurred during getting credentials of %s secret: %v", name, err)
		return credentials
	}
	for _, key := range keys {
		credentials = append(credentials, string(secret.Data[key]))
	}
	return credentials
}

func (kc KubernetesClient) findDeploymentList(deploymentLabels map[string]string) (*appsv1.DeploymentList, error) {
	return kc.client.AppsV1().Deployments(kc.namespace).List(context.TODO(), v1.ListOptions{
		LabelSelector: labels.SelectorFromSet(deploymentLabels).String(),
	})
}

func (kc KubernetesClient) IsDeploymentScaledUp(name string) (bool, error) {
	scale, err := kc.client.AppsV1().Deployments(kc.namespace).GetScale(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return false, err
	}
	return scale.Spec.Replicas != 0, nil
}

func (kc KubernetesClient) areDeploymentsReady(labels map[string]string) bool {
	deployments, err := kc.findDeploymentList(labels)
	if err != nil {
		kc.logger.WithError(err).Error("Can't find deployments")
		return false
	}
	for _, deployment := range deployments.Items {
		if !kc.isDeploymentReady(deployment) {
			kc.logger.Infof("%s deployment is not ready yet", deployment.Name)
			return false
		}
	}
	return true
}

func (kc KubernetesClient) isDeploymentReady(deployment appsv1.Deployment) bool {
	kc.logger.Debugf("Spec.Replicas = %d, ReadyReplicas = %d, UpdatedReplicas = %d", *deployment.Spec.Replicas,
		deployment.Status.ReadyReplicas, deployment.Status.UpdatedReplicas)
	return *deployment.Spec.Replicas == deployment.Status.ReadyReplicas &&
		*deployment.Spec.Replicas == deployment.Status.UpdatedReplicas
}

func (kc KubernetesClient) scaleDeployment(name string, replicas int32) error {
	scale, err := kc.client.AppsV1().Deployments(kc.namespace).GetScale(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		return err
	}
	scale.Spec.Replicas = replicas
	_, err = kc.client.AppsV1().Deployments(kc.namespace).UpdateScale(context.TODO(), name, scale, v1.UpdateOptions{})
	return err
}

func (kc KubernetesClient) ScaleDeploymentWithCheck(name string, replicas int32, labels map[string]string) error {
	err := kc.scaleDeployment(name, replicas)
	if err != nil {
		return err
	}
	return kc.CheckDeploymentReadiness(labels)
}

func (kc KubernetesClient) CheckDeploymentReadiness(labels map[string]string) error {
	return wait.PollUntilContextTimeout(context.Background(), interval, timeout, false, func(_ context.Context) (done bool, err error) {
		return kc.areDeploymentsReady(labels), nil
	})
}

func (kc KubernetesClient) isStatefulSetReady(name string) bool {
	statefulSet, err := kc.client.AppsV1().StatefulSets(kc.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if err != nil {
		kc.logger.WithError(err).Errorf("Can't find '%s' StatefulSet", name)
		return false
	}
	return *statefulSet.Spec.Replicas == statefulSet.Status.ReadyReplicas &&
		*statefulSet.Spec.Replicas == statefulSet.Status.UpdatedReplicas
}

func (kc KubernetesClient) CheckStatefulSetReadiness(name string) error {
	return wait.PollUntilContextTimeout(context.Background(), interval, timeout, true, func(_ context.Context) (done bool, err error) {
		if kc.isStatefulSetReady(name) {
			return true, nil
		}
		kc.logger.Infof("%s StatefulSet is not ready yet", name)
		return false, nil
	})
}
