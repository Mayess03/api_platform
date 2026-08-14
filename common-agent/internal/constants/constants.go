/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package constants

import (
	"os"
	"strconv"
	"time"
)

// APIM Mediation constants
const (
	InterceptorService      = "CallInterceptorService"
	BackendJWT              = "backEndJWT"
	AddHeader               = "apkAddHeader"
	RemoveHeader            = "apkRemoveHeader"
	MirrorRequest           = "apkMirrorRequest"
	RedirectRequest         = "apkRedirectRequest"
	ModelWeightedRoundRobin = "modelWeightedRoundRobin"
	ModelRoundRobin         = "modelRoundRobin"
	LuaInterceptorService   = "apkLuaInterceptorService"
	WASMInterceptorService  = "apkWASMInterceptorService"

	// Version constants
	V1 = "v1"
	V2 = "v2"

	// Policy Types
	CommonType = "common"

	DefaultKongAgentName  = "Kong"
	DefaultEnvoyAgentName = "EG"

	// Default delay (in seconds) before re-queuing retryable events when processing CR events
	DefaultEventRetryDelaySeconds = 5
)

// GetEnvoyAgentName returns the Envoy agent name, allowing override via APIM_AGENT_NAME env var.
// This keeps consistency across modules when comparing agent names in events.
func GetEnvoyAgentName() string {
	if v := os.Getenv("APIM_AGENT_NAME"); v != "" {
		return v
	}
	return DefaultEnvoyAgentName
}

// GetEventRetryDelay returns the delay to wait before re-queuing a retryable CR event.
// It reads APIM_EVENT_RETRY_DELAY_SECONDS from the environment, falling back to a sane default.
func GetEventRetryDelay() time.Duration {
	if v := os.Getenv("APIM_EVENT_RETRY_DELAY_SECONDS"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			return time.Duration(s) * time.Second
		}
	}
	return time.Duration(DefaultEventRetryDelaySeconds) * time.Second
}

// DisableRetryFeature returns the delay to wait before re-queuing a retryable CR event.
func DisableRetryFeature() bool {
	if v := os.Getenv("APIM_DISABLE_RETRY"); v != "" {
		disabled, err := strconv.ParseBool(v)
		if err == nil {
			return disabled
		}
	}
	return false
}
