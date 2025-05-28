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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ACL struct {
	Json            string `json:"json"`
	Name            string `json:"name"`
	CommonReconcile string `json:"commonReconcile,omitempty"`
}

// ConsulACLSpec defines the desired state of ConsulACL
type ConsulACLSpec struct {
	ACL *ACL `json:"acl"`
}

// ConsulACLStatus defines the observed state of ConsulACL
type ConsulACLStatus struct {
	PoliciesStatus  string `json:"policiesStatus"`
	RolesStatus     string `json:"rolesStatus,omitempty"`
	BindRulesStatus string `json:"bindRulesStatus,omitempty"`
	GeneralStatus   string `json:"generalStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+genclient:nonNamespaced

// ConsulACL is the Schema for the consulacls API
type ConsulACL struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsulACLSpec   `json:"spec,omitempty"`
	Status ConsulACLStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConsulACLList contains a list of ConsulACL
type ConsulACLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConsulACL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConsulACL{}, &ConsulACLList{})
}
