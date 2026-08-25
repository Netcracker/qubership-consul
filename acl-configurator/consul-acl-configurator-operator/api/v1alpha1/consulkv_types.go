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

type ConsulKVEntry struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

type ConsulKVConfig struct {
	Entries []ConsulKVEntry `json:"entries"`
}

type ConsulKVSpec struct {
	KV ConsulKVConfig `json:"kv"`
}

type ConsulKVEntryStatus struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Info   string `json:"info,omitempty"`
}

type ConsulKVStatus struct {
	Entries       []ConsulKVEntryStatus `json:"entries,omitempty"`
	GeneralStatus string                `json:"generalStatus,omitempty"`
	ManagedBy     string                `json:"managedBy,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ConsulKV is the Schema for the consulkvs API
type ConsulKV struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConsulKVSpec   `json:"spec,omitempty"`
	Status ConsulKVStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConsulKVList contains a list of ConsulKV
type ConsulKVList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConsulKV `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConsulKV{}, &ConsulKVList{})
}
