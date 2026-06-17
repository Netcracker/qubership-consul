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

package util

import (
	"os"
	"strings"
)

func Contains(value string, list []string) bool {
	for _, element := range list {
		if value == element {
			return true
		}
	}
	return false
}

func getEnv(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}

// GetSecretFromFileOrEnv reads secret from file first, then falls back to envKey.
func GetSecretFromFileOrEnv(filePath string, envKey string) string {
	secretBytes, err := os.ReadFile(filePath)
	if err == nil {
		return strings.TrimSpace(string(secretBytes))
	}
	return getEnv(envKey, "")
}
