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

// isOwnedByOperator reports whether a custom resource is reconciled by the operator running in
// ownNamespace.
//
// A resource with operatorNamespace is owned only by the operator of that namespace, wherever
// the resource lives. A resource without operatorNamespace is owned:
//   - in strict mode, only by the operator running in the resource's own namespace;
//   - otherwise, by every operator that watches the resource's namespace.
func isOwnedByOperator(operatorNamespace, resourceNamespace, ownNamespace string, strict bool) bool {
	if operatorNamespace != "" {
		return operatorNamespace == ownNamespace
	}
	return !strict || resourceNamespace == ownNamespace
}
