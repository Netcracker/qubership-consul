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

package controllers

import "testing"

func TestIsOwnedByOperator(t *testing.T) {
	tests := []struct {
		name              string
		operatorNamespace string
		resourceNamespace string
		strict            bool
		want              bool
	}{
		{"own operatorNamespace, non-strict", "consul", "app", false, true},
		{"own operatorNamespace, strict", "consul", "app", true, true},
		{"foreign operatorNamespace, non-strict", "other", "consul", false, false},
		{"foreign operatorNamespace, strict", "other", "consul", true, false},
		{"no operatorNamespace in own namespace, non-strict", "", "consul", false, true},
		{"no operatorNamespace in own namespace, strict", "", "consul", true, true},
		{"no operatorNamespace in another namespace, non-strict", "", "app", false, true},
		{"no operatorNamespace in another namespace, strict", "", "app", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOwnedByOperator(tt.operatorNamespace, tt.resourceNamespace, "consul", tt.strict); got != tt.want {
				t.Errorf("isOwnedByOperator() = %v, want %v", got, tt.want)
			}
		})
	}
}
