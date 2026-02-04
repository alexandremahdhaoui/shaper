//go:build e2e

// Copyright 2024 Alexandre Mahdhaoui
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

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrPodNotFound indicates the pod was not found.
	ErrPodNotFound = errors.New("pod not found")
	// ErrPodLogsGet indicates a failure to get pod logs.
	ErrPodLogsGet = errors.New("failed to get pod logs")
	// ErrIPXERequestNotFound indicates no iPXE request was found in logs.
	ErrIPXERequestNotFound = errors.New("iPXE request not found in logs")
	// ErrTLSClientConnectedNotFound indicates no tls_client_connected log was found.
	ErrTLSClientConnectedNotFound = errors.New("tls_client_connected log not found")
	// ErrAssignmentSelectedNotFound indicates no assignment_selected log was found.
	ErrAssignmentSelectedNotFound = errors.New("assignment_selected log not found")
)

const (
	// ShaperAPILabelSelector is the label selector for shaper-api pods.
	ShaperAPILabelSelector = "app.kubernetes.io/name=shaper-api"
	// ShaperSystemNamespace is the namespace where shaper components are deployed.
	ShaperSystemNamespace = "shaper-system"
)

// IPXERequestLog represents a parsed iPXE boot request log entry.
// Note: The ipxe_boot_request entry from server.go contains client_ip, uuid, buildarch.
// The profile_name and assignment_name come from separate controller log entries
// (profile_matched and assignment_selected). Use FindIPXERequestWithProfile to get all fields.
type IPXERequestLog struct {
	// ClientIP is the IP address of the client making the request.
	ClientIP string `json:"client_ip"`
	// UUID is the machine UUID from the request.
	UUID string `json:"uuid"`
	// Buildarch is the build architecture from the request.
	Buildarch string `json:"buildarch"`
	// ProfileName is the name of the Profile being served.
	// Note: This comes from the profile_matched log entry, not ipxe_boot_request.
	ProfileName string `json:"profile_name"`
	// AssignmentName is the name of the matched Assignment.
	// Note: This comes from the assignment_selected log entry, not ipxe_boot_request.
	AssignmentName string `json:"assignment_name"`
	// Timestamp is when the request was received.
	Timestamp time.Time `json:"time"`
	// Message is the log message type (e.g., "ipxe_boot_request").
	Message string `json:"msg"`
	// Level is the log level.
	Level string `json:"level"`
}

// profileMatchedLog represents the profile_matched log entry from the controller.
type profileMatchedLog struct {
	ProfileName   string `json:"profile_name"`
	AssignmentRef string `json:"assignment"`
	Message       string `json:"msg"`
}

// assignmentSelectedLog represents the assignment_selected log entry from the controller.
type assignmentSelectedLog struct {
	AssignmentName string `json:"assignment_name"`
	Message        string `json:"msg"`
}

// TLSClientLog represents a parsed tls_client_connected log entry.
// This is logged when a client presents a valid certificate during mTLS handshake.
type TLSClientLog struct {
	// ClientCN is the Common Name from the client certificate.
	ClientCN string `json:"client_cn"`
	// ClientIssuer is the Common Name of the certificate issuer.
	ClientIssuer string `json:"client_issuer"`
	// ClientSerial is the serial number of the client certificate.
	ClientSerial string `json:"client_serial"`
	// Timestamp is when the connection was logged.
	Timestamp time.Time `json:"time"`
	// Message is the log message type (should be "tls_client_connected").
	Message string `json:"msg"`
	// Level is the log level.
	Level string `json:"level"`
}

// GetShaperAPIPodName finds the shaper-api pod name in the shaper-system namespace.
func GetShaperAPIPodName(ctx context.Context, c client.Client) (string, error) {
	pods := &corev1.PodList{}
	if err := c.List(ctx, pods,
		client.InNamespace(ShaperSystemNamespace),
		client.MatchingLabels{"app.kubernetes.io/name": "shaper-api"},
	); err != nil {
		return "", errors.Join(ErrPodNotFound, err)
	}

	// Find a running pod
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	// If no running pod, return first pod if any exist
	if len(pods.Items) > 0 {
		return pods.Items[0].Name, nil
	}

	return "", errors.Join(ErrPodNotFound, errors.New("no shaper-api pods found"))
}

// GetPodLogs retrieves logs from a pod using kubectl.
// The since parameter filters logs to only those after the given time.
func GetPodLogs(ctx context.Context, kubeconfig, namespace, podName string, since time.Time) (string, error) {
	// Calculate the duration since the given time
	sinceSeconds := int(time.Since(since).Seconds()) + 1 // Add 1 second buffer

	args := []string{
		"logs",
		podName,
		"-n", namespace,
		"--kubeconfig", kubeconfig,
	}

	// Only add --since if we have a valid duration
	if sinceSeconds > 0 && sinceSeconds < 86400 { // Less than 24 hours
		args = append(args, "--since", (time.Duration(sinceSeconds) * time.Second).String())
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.Join(ErrPodLogsGet, errors.New(string(exitErr.Stderr)), err)
		}
		return "", errors.Join(ErrPodLogsGet, err)
	}

	return string(output), nil
}

// FindIPXERequest searches logs for an iPXE request from the given client IP.
// It parses JSON-formatted log entries and returns the first matching request.
// Note: This only returns client_ip, uuid, buildarch. Use FindIPXERequestWithProfile
// to also get profile_name and assignment_name.
func FindIPXERequest(logs, clientIP string) (*IPXERequestLog, error) {
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var logEntry IPXERequestLog
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			// Not JSON, skip
			continue
		}

		// Check if this is an iPXE boot request log entry
		if logEntry.Message != "ipxe_boot_request" {
			continue
		}

		// Check if the client IP matches
		if logEntry.ClientIP == clientIP {
			return &logEntry, nil
		}
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("no iPXE request found from client IP: "+clientIP))
}

// FindIPXERequestWithProfile searches logs for an iPXE request and enriches it
// with profile and assignment information from subsequent controller log entries.
// The logs should contain entries in this order:
// 1. ipxe_boot_request (with client_ip)
// 2. assignment_selected (with assignment_name)
// 3. profile_matched (with profile_name)
func FindIPXERequestWithProfile(logs, clientIP string) (*IPXERequestLog, error) {
	lines := strings.Split(logs, "\n")

	var result *IPXERequestLog
	foundRequest := false
	var lastAssignment string
	var lastProfile string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON to a generic map first
		var rawEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rawEntry); err != nil {
			continue
		}

		msg, ok := rawEntry["msg"].(string)
		if !ok {
			continue
		}

		switch msg {
		case "ipxe_boot_request":
			var logEntry IPXERequestLog
			if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
				continue
			}
			if logEntry.ClientIP == clientIP {
				result = &logEntry
				foundRequest = true
			}

		case "assignment_selected":
			if foundRequest && result.AssignmentName == "" {
				var asLog assignmentSelectedLog
				if err := json.Unmarshal([]byte(line), &asLog); err == nil {
					lastAssignment = asLog.AssignmentName
				}
			}

		case "profile_matched":
			if foundRequest && result.ProfileName == "" {
				var pmLog profileMatchedLog
				if err := json.Unmarshal([]byte(line), &pmLog); err == nil {
					lastProfile = pmLog.ProfileName
				}
			}
		}

		// If we found all the pieces, set them and return
		if foundRequest && lastAssignment != "" && lastProfile != "" {
			result.AssignmentName = lastAssignment
			result.ProfileName = lastProfile
			return result, nil
		}
	}

	if result != nil {
		// We found the request but maybe not all the profile info
		// Still fill in what we have
		if lastAssignment != "" {
			result.AssignmentName = lastAssignment
		}
		if lastProfile != "" {
			result.ProfileName = lastProfile
		}
		return result, nil
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("no iPXE request found from client IP: "+clientIP))
}

// FindIPXERequestWithUUID searches logs for an iPXE request matching both client IP and UUID.
func FindIPXERequestWithUUID(logs, clientIP, vmUUID string) (*IPXERequestLog, error) {
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var logEntry IPXERequestLog
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		// Check if this is an iPXE boot request
		if logEntry.Message != "ipxe_boot_request" {
			continue
		}

		// Check if both client IP and UUID match
		if logEntry.ClientIP == clientIP && logEntry.UUID == vmUUID {
			return &logEntry, nil
		}
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("no iPXE request found from client IP: "+clientIP+" with UUID: "+vmUUID))
}

// WaitForIPXERequest polls logs until an iPXE request from the given IP is found.
func WaitForIPXERequest(
	ctx context.Context,
	kubeconfig, namespace, podName, clientIP string,
	since time.Time,
	timeout time.Duration,
) (*IPXERequestLog, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logs, err := GetPodLogs(ctx, kubeconfig, namespace, podName, since)
		if err != nil {
			// Log error but continue polling
			time.Sleep(pollInterval)
			continue
		}

		request, err := FindIPXERequest(logs, clientIP)
		if err == nil {
			return request, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("timeout waiting for iPXE request from: "+clientIP))
}

// FindIPXERequestByUUIDOnly searches logs for an iPXE request matching ONLY by UUID.
// This is useful when port-forward is used and client IP does not match the VM's actual IP.
func FindIPXERequestByUUIDOnly(logs, vmUUID string) (*IPXERequestLog, error) {
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var logEntry IPXERequestLog
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		// Check if this is an iPXE boot request
		if logEntry.Message != "ipxe_boot_request" {
			continue
		}

		// Check if UUID matches (case-insensitive comparison)
		if strings.EqualFold(logEntry.UUID, vmUUID) {
			return &logEntry, nil
		}
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("no iPXE request found with UUID: "+vmUUID))
}

// WaitForIPXERequestByUUID polls logs until an iPXE request with the given UUID is found.
// This is preferred over WaitForIPXERequest when using port-forward, as the client IP
// will appear as localhost instead of the VM's actual IP.
func WaitForIPXERequestByUUID(
	ctx context.Context,
	kubeconfig, namespace, podName, vmUUID string,
	since time.Time,
	timeout time.Duration,
) (*IPXERequestLog, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logs, err := GetPodLogs(ctx, kubeconfig, namespace, podName, since)
		if err != nil {
			// Log error but continue polling
			time.Sleep(pollInterval)
			continue
		}

		request, err := FindIPXERequestByUUIDOnly(logs, vmUUID)
		if err == nil {
			return request, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("timeout waiting for iPXE request with UUID: "+vmUUID))
}

// AssignmentSelectedLog represents a parsed assignment_selected log entry.
// This is logged when an assignment is selected for a boot request.
type AssignmentSelectedLog struct {
	// AssignmentName is the name of the selected assignment.
	AssignmentName string `json:"assignment_name"`
	// AssignmentNamespace is the namespace of the selected assignment.
	AssignmentNamespace string `json:"assignment_namespace"`
	// MatchedBy indicates how the assignment was matched (e.g., "uuid", "default").
	MatchedBy string `json:"matched_by"`
	// Timestamp is when the selection was logged.
	Timestamp time.Time `json:"time"`
	// Message is the log message type (should be "assignment_selected").
	Message string `json:"msg"`
	// Level is the log level.
	Level string `json:"level"`
}

// FindAssignmentSelectedByUUID searches logs for an assignment_selected log entry
// that follows an ipxe_boot_request with the given UUID.
// Returns the AssignmentSelectedLog with the matched_by field populated.
func FindAssignmentSelectedByUUID(logs, vmUUID string) (*AssignmentSelectedLog, error) {
	lines := strings.Split(logs, "\n")
	foundIPXERequest := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var rawEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rawEntry); err != nil {
			continue
		}

		msg, ok := rawEntry["msg"].(string)
		if !ok {
			continue
		}

		// First, find the ipxe_boot_request with matching UUID
		if msg == "ipxe_boot_request" {
			uuid, ok := rawEntry["uuid"].(string)
			if ok && strings.EqualFold(uuid, vmUUID) {
				foundIPXERequest = true
			}
			continue
		}

		// Then, find the assignment_selected that follows
		if msg == "assignment_selected" && foundIPXERequest {
			var logEntry AssignmentSelectedLog
			if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
				continue
			}
			return &logEntry, nil
		}
	}

	return nil, errors.Join(ErrAssignmentSelectedNotFound,
		errors.New("no assignment_selected log found for UUID: "+vmUUID))
}

// WaitForAssignmentSelectedByUUID polls logs until an assignment_selected log
// following an ipxe_boot_request with the given UUID is found.
func WaitForAssignmentSelectedByUUID(
	ctx context.Context,
	kubeconfig, namespace, podName, vmUUID string,
	since time.Time,
	timeout time.Duration,
) (*AssignmentSelectedLog, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logs, err := GetPodLogs(ctx, kubeconfig, namespace, podName, since)
		if err != nil {
			// Log error but continue polling
			time.Sleep(pollInterval)
			continue
		}

		result, err := FindAssignmentSelectedByUUID(logs, vmUUID)
		if err == nil {
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, errors.Join(ErrAssignmentSelectedNotFound,
		errors.New("timeout waiting for assignment_selected with UUID: "+vmUUID))
}

// FindProfileMatchedLog searches logs for a profile_matched log entry with the given profile name.
func FindProfileMatchedLog(logs, profileName string) (*IPXERequestLog, error) {
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var rawEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rawEntry); err != nil {
			continue
		}

		msg, ok := rawEntry["msg"].(string)
		if !ok || msg != "profile_matched" {
			continue
		}

		// Check if profile name matches
		pName, ok := rawEntry["profile_name"].(string)
		if !ok || pName != profileName {
			continue
		}

		// Build the result with available info
		result := &IPXERequestLog{
			ProfileName: pName,
			Message:     msg,
		}

		if assignmentName, ok := rawEntry["assignment"].(string); ok {
			result.AssignmentName = assignmentName
		}

		return result, nil
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("no profile_matched log found for profile: "+profileName))
}

// FindIPXERequestByBuildarch searches logs for an iPXE request matching the given buildarch.
// This is useful for default assignment tests where UUID may not be available.
func FindIPXERequestByBuildarch(logs, buildarch string) (*IPXERequestLog, error) {
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var logEntry IPXERequestLog
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		// Check if this is an iPXE boot request
		if logEntry.Message != "ipxe_boot_request" {
			continue
		}

		// Check if buildarch matches
		if logEntry.Buildarch == buildarch {
			return &logEntry, nil
		}
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("no iPXE request found with buildarch: "+buildarch))
}

// WaitForProfileMatched polls logs until a profile_matched log with the given profile name is found.
// This verifies that the correct profile was served to a boot request.
func WaitForProfileMatched(
	ctx context.Context,
	kubeconfig, namespace, podName, profileName string,
	since time.Time,
	timeout time.Duration,
) (*IPXERequestLog, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logs, err := GetPodLogs(ctx, kubeconfig, namespace, podName, since)
		if err != nil {
			// Log error but continue polling
			time.Sleep(pollInterval)
			continue
		}

		result, err := FindProfileMatchedLog(logs, profileName)
		if err == nil {
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("timeout waiting for profile_matched with profile: "+profileName))
}

// WaitForIPXERequestByBuildarch polls logs until an iPXE request with the given buildarch is found.
func WaitForIPXERequestByBuildarch(
	ctx context.Context,
	kubeconfig, namespace, podName, buildarch string,
	since time.Time,
	timeout time.Duration,
) (*IPXERequestLog, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logs, err := GetPodLogs(ctx, kubeconfig, namespace, podName, since)
		if err != nil {
			// Log error but continue polling
			time.Sleep(pollInterval)
			continue
		}

		request, err := FindIPXERequestByBuildarch(logs, buildarch)
		if err == nil {
			return request, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, errors.Join(ErrIPXERequestNotFound,
		errors.New("timeout waiting for iPXE request with buildarch: "+buildarch))
}

// FindTLSClientConnectedLog searches logs for a tls_client_connected log entry with the given client CN.
// This verifies that a client with the expected certificate CN successfully completed mTLS handshake.
func FindTLSClientConnectedLog(logs, clientCN string) (*TLSClientLog, error) {
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var logEntry TLSClientLog
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		// Check if this is a tls_client_connected log entry
		if logEntry.Message != "tls_client_connected" {
			continue
		}

		// Check if the client CN matches
		if logEntry.ClientCN == clientCN {
			return &logEntry, nil
		}
	}

	return nil, errors.Join(ErrTLSClientConnectedNotFound,
		errors.New("no tls_client_connected log found for client CN: "+clientCN))
}

// WaitForTLSClientConnected polls logs until a tls_client_connected log with the given client CN is found.
// This verifies that a client with the expected certificate successfully completed mTLS handshake.
func WaitForTLSClientConnected(
	ctx context.Context,
	kubeconfig, namespace, podName, clientCN string,
	since time.Time,
	timeout time.Duration,
) (*TLSClientLog, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logs, err := GetPodLogs(ctx, kubeconfig, namespace, podName, since)
		if err != nil {
			// Log error but continue polling
			time.Sleep(pollInterval)
			continue
		}

		result, err := FindTLSClientConnectedLog(logs, clientCN)
		if err == nil {
			return result, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, errors.Join(ErrTLSClientConnectedNotFound,
		errors.New("timeout waiting for tls_client_connected with client CN: "+clientCN))
}
