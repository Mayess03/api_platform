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
package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/wso2-extensions/apim-gw-connectors/common-agent/config"
	logger "github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/loggers"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/tlsutils"
)

const (
	// AdminAdvancedThrottlingPoliciesRelativePath is the relative admin API path to list advanced throttling policies
	AdminAdvancedThrottlingPoliciesRelativePath = "api/am/admin/v4/throttling/policies/advanced"
)

var (
	adminAdvancedPoliciesURL string
)

func init() {
	// Read configurations and derive the control plane base URL
	conf, errReadConfig := config.ReadConfigs()
	if errReadConfig != nil {
		logger.LoggerUtils.Errorf("Error reading configs: %v", errReadConfig)
	}
	cpURL := conf.ControlPlane.ServiceURL
	if strings.HasSuffix(cpURL, "/") {
		adminAdvancedPoliciesURL = cpURL + AdminAdvancedThrottlingPoliciesRelativePath
	} else {
		adminAdvancedPoliciesURL = cpURL + "/" + AdminAdvancedThrottlingPoliciesRelativePath
	}
}

// ThrottlingPoliciesResponse represents the response body for listing advanced throttling policies
type ThrottlingPoliciesResponse struct {
	Count int                      `json:"count"`
	List  []map[string]interface{} `json:"list"`
}

// AdvancedThrottlePolicyInfo represents the payload to create an advanced throttling policy
type AdvancedThrottlePolicyInfo struct {
	PolicyName   string        `json:"policyName"`
	DisplayName  string        `json:"displayName,omitempty"`
	Description  string        `json:"description,omitempty"`
	IsDeployed   bool          `json:"isDeployed,omitempty"`
	Type         string        `json:"type"`
	DefaultLimit ThrottleLimit `json:"defaultLimit"`
}

// ThrottleLimit is a union type container; provide appropriate sub-object based on Type
type ThrottleLimit struct {
	Type         string             `json:"type"` // REQUESTCOUNTLIMIT | BANDWIDTHLIMIT | EVENTCOUNTLIMIT | AIAPIQUOTALIMIT
	RequestCount *RequestCountLimit `json:"requestCount,omitempty"`
	Bandwidth    *BandwidthLimit    `json:"bandwidth,omitempty"`
	EventCount   *EventCountLimit   `json:"eventCount,omitempty"`
	AIAPIQuota   *AIAPIQuotaLimit   `json:"aiApiQuota,omitempty"`
}

// RequestCountLimit for REQUESTCOUNTLIMIT
type RequestCountLimit struct {
	TimeUnit     string `json:"timeUnit"` // sec|min|hour|day
	UnitTime     int    `json:"unitTime"`
	RequestCount int64  `json:"requestCount"`
}

// BandwidthLimit for BANDWIDTHLIMIT
type BandwidthLimit struct {
	TimeUnit   string `json:"timeUnit"` // sec|min|hour|day
	UnitTime   int    `json:"unitTime"`
	DataAmount int64  `json:"dataAmount"`
	DataUnit   string `json:"dataUnit"` // KB|MB|GB
}

// EventCountLimit for EVENTCOUNTLIMIT
type EventCountLimit struct {
	TimeUnit   string `json:"timeUnit"` // sec|min|hour|day
	UnitTime   int    `json:"unitTime"`
	EventCount int64  `json:"eventCount"`
}

// AIAPIQuotaLimit for AIAPIQUOTALIMIT
type AIAPIQuotaLimit struct {
	TimeUnit             string `json:"timeUnit"` // sec|min|hour|day
	UnitTime             int    `json:"unitTime"`
	RequestCount         int64  `json:"requestCount"`
	TotalTokenCount      int64  `json:"totalTokenCount,omitempty"`
	PromptTokenCount     int64  `json:"promptTokenCount,omitempty"`
	CompletionTokenCount int64  `json:"completionTokenCount,omitempty"`
}

// GetAllAdvancedThrottlingPolicies retrieves all existing advanced throttling policies from Admin REST API.
// Returns the policies (if any), a retryable flag indicating whether the caller should retry on error, and an error.
func GetAllAdvancedThrottlingPolicies() (*ThrottlingPoliciesResponse, bool, error) {
	// Use admin scope for Admin REST API
	authHeaderVal, err := GetSuitableAuthHeadervalue([]string{string(AdminScope)})
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequest("GET", adminAdvancedPoliciesURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", authHeaderVal)
	req.Header.Set("Accept", "application/json")

	resp, err := tlsutils.InvokeControlPlane(req, skipSSL)
	if err != nil {
		logger.LoggerTLSUtils.Errorf("Error occurred while sending list advanced throttling policies request. Error: %+v", err)
		// Transport/TLS/DNS/timeouts are typically retryable
		return nil, true, err
	}
	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	logger.LoggerTLSUtils.Infof("For the Admin advanced throttling policies list request we received response status %s, code: %+v, body: %+v", resp.Status, resp.StatusCode, string(respBody))

	switch resp.StatusCode {
	case http.StatusOK: // 200
		// proceed
	case http.StatusNotAcceptable: // 406
		return nil, false, fmt.Errorf("requested media type is not acceptable (status %d)", resp.StatusCode)
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusTooManyRequests:
		return nil, true, fmt.Errorf("list advanced throttling policies returned retryable status %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	default:
		return nil, false, fmt.Errorf("unexpected response status %d: %s", resp.StatusCode, string(respBody))
	}

	var policies ThrottlingPoliciesResponse
	if err := json.Unmarshal(respBody, &policies); err != nil {
		return nil, false, err
	}
	return &policies, false, nil
}

// AddAdvancedThrottlingPolicy creates a new advanced throttling policy via Admin REST API.
// Returns the created policy object (as a generic map to accommodate server-added fields),
// a retryable flag indicating whether the caller should retry on error, and an error.
func AddAdvancedThrottlingPolicy(policy AdvancedThrottlePolicyInfo) (map[string]interface{}, bool, error) {
	authHeaderVal, err := GetSuitableAuthHeadervalue([]string{string(AdminScope)})
	if err != nil {
		return nil, false, err
	}

	payload, err := json.Marshal(policy)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal policy payload: %w", err)
	}

	req, err := http.NewRequest("POST", adminAdvancedPoliciesURL, bytes.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", authHeaderVal)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := tlsutils.InvokeControlPlane(req, skipSSL)
	if err != nil {
		logger.LoggerTLSUtils.Errorf("Error occurred while sending add advanced throttling policy request. Error: %+v", err)
		// Transport/TLS/DNS/timeouts are typically retryable
		return nil, true, err
	}
	defer resp.Body.Close()
	respBody, _ := ioutil.ReadAll(resp.Body)
	logger.LoggerTLSUtils.Infof("For the Admin add advanced throttling policy request we received response status %s, code: %+v, body: %+v", resp.Status, resp.StatusCode, string(respBody))

	switch resp.StatusCode {
	case http.StatusCreated: // 201
		// proceed
	case http.StatusBadRequest: // 400
		return nil, false, fmt.Errorf("invalid request or validation error (status %d): %s", resp.StatusCode, string(respBody))
	case http.StatusUnsupportedMediaType: // 415
		return nil, false, fmt.Errorf("unsupported media type (status %d): %s", resp.StatusCode, string(respBody))
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusTooManyRequests:
		return nil, true, fmt.Errorf("add advanced throttling policy returned retryable status %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	default:
		return nil, false, fmt.Errorf("unexpected response status %d: %s", resp.StatusCode, string(respBody))
	}

	var created map[string]interface{}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, false, fmt.Errorf("failed to parse create policy response: %w", err)
	}
	return created, false, nil
}
