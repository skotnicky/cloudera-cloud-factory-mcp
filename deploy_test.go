package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
)

func TestCommitProjectNeedsVMEndpoint(t *testing.T) {
	if !commitProjectNeedsVMEndpoint(errors.New("Taikun Error: (TITLE Bad request) (DETAIL You need at least one worker, an odd number of master(s) and one bastion to commit changes.)")) {
		t.Fatal("expected VM commit fallback for Kubernetes layout validation error")
	}

	if commitProjectNeedsVMEndpoint(errors.New("some other error")) {
		t.Fatal("did not expect VM commit fallback for unrelated errors")
	}
}

func TestCommitProjectFallbackDecisionUsesResponseBody(t *testing.T) {
	httpResponse := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"title":"Bad request","detail":"You need at least one worker, an odd number of master(s) and one bastion to commit changes."}`)),
	}

	shouldUseVMCommit, errorInfo := commitProjectFallbackDecision(httpResponse, nil)
	if errorInfo.Message == "" {
		t.Fatal("expected error info for non-2xx response")
	}
	if !shouldUseVMCommit {
		t.Fatal("expected VM commit fallback when response body contains cluster layout validation error")
	}
}

func resetPendingProjectCommitChangesForTest(t *testing.T) {
	t.Helper()

	projectPendingCommitChangesMu.Lock()
	defer projectPendingCommitChangesMu.Unlock()

	projectPendingCommitChanges = map[int32]pendingProjectCommitChanges{}
}

func TestPendingProjectCommitModeStandaloneVMCreateOnly(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(101)
	recordPendingStandaloneVMCreate(projectID)

	if got := pendingProjectCommitMode(projectID); got != projectCommitModeVM {
		t.Fatalf("expected %q, got %q", projectCommitModeVM, got)
	}
}

func TestPendingProjectCommitModeServerAddOnly(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(102)
	recordPendingServerAdd(projectID)

	if got := pendingProjectCommitMode(projectID); got != projectCommitModeProject {
		t.Fatalf("expected %q, got %q", projectCommitModeProject, got)
	}
}

func TestPendingProjectCommitModeMixedChangesPreferProject(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(103)
	recordPendingStandaloneVMCreate(projectID)
	recordPendingServerAdd(projectID)

	if got := pendingProjectCommitMode(projectID); got != projectCommitModeProject {
		t.Fatalf("expected %q for mixed changes, got %q", projectCommitModeProject, got)
	}
}

func TestExecuteProjectCommitModeClearsPendingStateAfterSuccessfulVMCommit(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(104)
	recordPendingStandaloneVMCreate(projectID)

	called := ""
	result, errorInfo := executeProjectCommitMode(
		projectID,
		pendingProjectCommitMode(projectID),
		func() (projectCommitResult, *apiErrorInfo) {
			called = "project"
			return projectCommitResult{}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "fallback"
			return projectCommitResult{}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "vm"
			return projectCommitResult{Mode: "vm", Message: "ok"}, nil
		},
	)
	if errorInfo != nil {
		t.Fatalf("expected successful VM commit, got error %+v", errorInfo)
	}
	if called != "vm" {
		t.Fatalf("expected vm commit path, got %q", called)
	}
	if result.Mode != "vm" {
		t.Fatalf("expected vm result mode, got %q", result.Mode)
	}
	if got := pendingProjectCommitMode(projectID); got != projectCommitModeAuto {
		t.Fatalf("expected pending state to clear after success, got %q", got)
	}
}

func TestExecuteProjectCommitModeClearsPendingStateAfterSuccessfulProjectCommit(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(105)
	recordPendingStandaloneVMCreate(projectID)
	recordPendingServerAdd(projectID)

	called := ""
	result, errorInfo := executeProjectCommitMode(
		projectID,
		pendingProjectCommitMode(projectID),
		func() (projectCommitResult, *apiErrorInfo) {
			called = "project"
			return projectCommitResult{Mode: "project", Message: "ok"}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "fallback"
			return projectCommitResult{}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "vm"
			return projectCommitResult{}, nil
		},
	)
	if errorInfo != nil {
		t.Fatalf("expected successful project commit, got error %+v", errorInfo)
	}
	if called != "project" {
		t.Fatalf("expected project commit path, got %q", called)
	}
	if result.Mode != "project" {
		t.Fatalf("expected project result mode, got %q", result.Mode)
	}
	if got := pendingProjectCommitMode(projectID); got != projectCommitModeAuto {
		t.Fatalf("expected pending state to clear after success, got %q", got)
	}
}

func TestExecuteProjectCommitModePreservesPendingStateAfterFailedVMCommit(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(106)
	recordPendingStandaloneVMCreate(projectID)

	called := ""
	_, errorInfo := executeProjectCommitMode(
		projectID,
		pendingProjectCommitMode(projectID),
		func() (projectCommitResult, *apiErrorInfo) {
			called = "project"
			return projectCommitResult{}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "fallback"
			return projectCommitResult{}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "vm"
			return projectCommitResult{}, &apiErrorInfo{Message: "vm commit failed"}
		},
	)
	if errorInfo == nil {
		t.Fatal("expected VM commit failure")
	}
	if called != "vm" {
		t.Fatalf("expected vm commit path, got %q", called)
	}
	if got := pendingProjectCommitMode(projectID); got != projectCommitModeVM {
		t.Fatalf("expected pending VM state to remain after failure, got %q", got)
	}
}

func TestExecuteProjectCommitModeUsesFallbackWhenNoTrackedChanges(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(107)
	called := ""
	result, errorInfo := executeProjectCommitMode(
		projectID,
		pendingProjectCommitMode(projectID),
		func() (projectCommitResult, *apiErrorInfo) {
			called = "project"
			return projectCommitResult{}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "fallback"
			return projectCommitResult{Mode: "project", Message: "ok"}, nil
		},
		func() (projectCommitResult, *apiErrorInfo) {
			called = "vm"
			return projectCommitResult{}, nil
		},
	)
	if errorInfo != nil {
		t.Fatalf("expected successful fallback commit path, got error %+v", errorInfo)
	}
	if called != "fallback" {
		t.Fatalf("expected fallback commit path, got %q", called)
	}
	if result.Mode != "project" {
		t.Fatalf("expected project result mode from fallback path, got %q", result.Mode)
	}
}

func TestCommitProjectWithFallbackPreservesReactiveFallbackInTrackedProjectMode(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(108)
	recordPendingServerAdd(projectID)

	serversBody := mustMarshalJSONForDeploy(t, buildServersListForDetails(projectID, 77, false, []taikuncore.ServerListDto{
		buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-1", "m4"),
	}))
	flavorsBody := mustMarshalJSONForDeploy(t, buildAllFlavorsList([]taikuncore.FlavorsListDto{
		buildFlavor("m4", 4, 4),
	}))

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(callCount.Add(1))
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(serversBody))
		case 2:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(flavorsBody))
		case 3:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"title":"Bad request","detail":"You need at least one worker, an odd number of master(s) and one bastion to commit changes."}`))
		case 4:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected extra request %d to %s", n, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	cfg := taikuncore.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = strings.TrimPrefix(server.URL, "http://")
	client := &taikungoclient.Client{
		Client: taikuncore.NewAPIClient(cfg),
	}

	result, errorInfo := commitProjectWithFallback(client, projectID)
	if errorInfo != nil {
		t.Fatalf("expected reactive fallback to recover tracked project-mode commit, got error %+v", errorInfo)
	}
	if result.Mode != "vm" {
		t.Fatalf("expected VM commit result after fallback, got %q", result.Mode)
	}
	if callCount.Load() != 4 {
		t.Fatalf("expected preflight, commit, and VM fallback calls, got %d request(s)", callCount.Load())
	}
	if got := pendingProjectCommitMode(projectID); got != projectCommitModeAuto {
		t.Fatalf("expected pending state to clear after successful fallback, got %q", got)
	}
}

type queuedHTTPResponse struct {
	statusCode int
	body       string
}

func newQueuedResponseClient(t *testing.T, responses []queuedHTTPResponse) (*taikungoclient.Client, *atomic.Int32, func()) {
	t.Helper()

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(callCount.Add(1)) - 1
		if idx >= len(responses) {
			t.Errorf("unexpected request %d to %s", idx+1, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		next := responses[idx]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(next.statusCode)
		_, _ = w.Write([]byte(next.body))
	}))

	cfg := taikuncore.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = strings.TrimPrefix(server.URL, "http://")
	client := &taikungoclient.Client{
		Client: taikuncore.NewAPIClient(cfg),
	}

	return client, &callCount, server.Close
}

func mustMarshalJSONForDeploy(t *testing.T, value interface{}) string {
	t.Helper()

	bytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal json: %v", err)
	}
	return string(bytes)
}

func buildServersListForDetails(projectID, cloudID int32, monitoringEnabled bool, servers []taikuncore.ServerListDto) taikuncore.ServersListForDetails {
	project := taikuncore.ProjectDetailsForServersDto{
		Id:                  projectID,
		Name:                "project-under-test",
		Status:              taikuncore.PROJECTSTATUS_READY,
		Health:              taikuncore.PROJECTHEALTH_HEALTHY,
		CloudType:           taikuncore.ECLOUDCREDENTIALTYPE_AWS,
		ProxmoxStorage:      taikuncore.PROXMOXSTORAGE_NFS,
		CloudId:             cloudID,
		IsMonitoringEnabled: monitoringEnabled,
	}

	return taikuncore.ServersListForDetails{
		Data:    servers,
		Project: project,
	}
}

func buildServer(role taikuncore.CloudRole, name, flavor string) taikuncore.ServerListDto {
	flavorValue := taikuncore.NewNullableString(&flavor)
	return taikuncore.ServerListDto{
		Name:        name,
		Role:        role,
		CloudType:   taikuncore.CLOUDTYPE_AWS,
		ProxmoxRole: taikuncore.PROXMOXROLE_NONE,
		Flavor:      *flavorValue,
	}
}

func buildFlavor(name string, cpu int32, ram float64) taikuncore.FlavorsListDto {
	return taikuncore.FlavorsListDto{
		Name:        name,
		Cpu:         cpu,
		Ram:         ram,
		Description: "",
	}
}

func buildAllFlavorsList(flavors []taikuncore.FlavorsListDto) taikuncore.AllFlavorsList {
	return taikuncore.AllFlavorsList{
		Data:       flavors,
		TotalCount: int32(len(flavors)),
	}
}

func TestValidateProjectSizingForCommit(t *testing.T) {
	projectID := int32(500)
	cloudID := int32(77)

	tests := []struct {
		name            string
		monitoring      bool
		servers         []taikuncore.ServerListDto
		// one entry per kubemaster/kubeworker in servers order; each entry is the flavors the search returns
		flavorResponses [][]taikuncore.FlavorsListDto
		wantErrContains string
	}{
		{
			name:       "valid master sizing passes",
			monitoring: false,
			servers: []taikuncore.ServerListDto{
				buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-1", "m4"),
			},
			flavorResponses: [][]taikuncore.FlavorsListDto{
				{buildFlavor("m4", 4, 4)},
			},
		},
		{
			name:       "undersized master fails",
			monitoring: false,
			servers: []taikuncore.ServerListDto{
				buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-small", "m2"),
			},
			flavorResponses: [][]taikuncore.FlavorsListDto{
				{buildFlavor("m2", 2, 2)},
			},
			wantErrContains: "every Kubemaster",
		},
		{
			name:       "monitoring disabled no worker requirement",
			monitoring: false,
			servers: []taikuncore.ServerListDto{
				buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-1", "m4"),
			},
			flavorResponses: [][]taikuncore.FlavorsListDto{
				{buildFlavor("m4", 4, 4)},
			},
		},
		{
			name:       "monitoring enabled with qualifying worker passes",
			monitoring: true,
			servers: []taikuncore.ServerListDto{
				buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-1", "m4"),
				buildServer(taikuncore.CLOUDROLE_KUBEWORKER, "worker-1", "w4"),
			},
			flavorResponses: [][]taikuncore.FlavorsListDto{
				{buildFlavor("m4", 4, 4)},
				{buildFlavor("w4", 4, 4)},
			},
		},
		{
			name:       "monitoring enabled without qualifying worker fails",
			monitoring: true,
			servers: []taikuncore.ServerListDto{
				buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-1", "m4"),
				buildServer(taikuncore.CLOUDROLE_KUBEWORKER, "worker-small", "w2"),
			},
			flavorResponses: [][]taikuncore.FlavorsListDto{
				{buildFlavor("m4", 4, 4)},
				{buildFlavor("w2", 2, 2)},
			},
			wantErrContains: "at least one Kubeworker",
		},
		{
			name:       "unknown flavor metadata returns clear error",
			monitoring: true,
			servers: []taikuncore.ServerListDto{
				buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-1", "m4"),
				buildServer(taikuncore.CLOUDROLE_KUBEWORKER, "worker-unknown", "unknown"),
			},
			flavorResponses: [][]taikuncore.FlavorsListDto{
				{buildFlavor("m4", 4, 4)},
				{}, // search for "unknown" returns no results
			},
			wantErrContains: "missing from cloud credential",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			serversBody := mustMarshalJSONForDeploy(t, buildServersListForDetails(projectID, cloudID, testCase.monitoring, testCase.servers))

			queued := []queuedHTTPResponse{
				{statusCode: http.StatusOK, body: serversBody},
			}
			for _, flavorSet := range testCase.flavorResponses {
				queued = append(queued, queuedHTTPResponse{
					statusCode: http.StatusOK,
					body:       mustMarshalJSONForDeploy(t, buildAllFlavorsList(flavorSet)),
				})
			}

			client, callCount, cleanup := newQueuedResponseClient(t, queued)
			defer cleanup()

			errInfo := validateProjectSizingForCommit(client, projectID)

			if testCase.wantErrContains == "" {
				if errInfo != nil {
					t.Fatalf("expected no validation error, got %q", errInfo.Message)
				}
			} else {
				if errInfo == nil {
					t.Fatalf("expected validation error containing %q, got nil", testCase.wantErrContains)
				}
				if !strings.Contains(errInfo.Message, testCase.wantErrContains) {
					t.Fatalf("expected error containing %q, got %q", testCase.wantErrContains, errInfo.Message)
				}
			}

			wantCalls := int32(1 + len(testCase.flavorResponses))
			if callCount.Load() != wantCalls {
				t.Fatalf("expected %d preflight calls, got %d", wantCalls, callCount.Load())
			}
		})
	}
}

func TestCommitProjectWithFallbackFailsBeforeCommitWhenSizingValidationFails(t *testing.T) {
	projectID := int32(501)
	cloudID := int32(88)

	serversBody := mustMarshalJSONForDeploy(t, buildServersListForDetails(projectID, cloudID, false, []taikuncore.ServerListDto{
		buildServer(taikuncore.CLOUDROLE_KUBEMASTER, "master-small", "m2"),
	}))
	flavorsBody := mustMarshalJSONForDeploy(t, buildAllFlavorsList([]taikuncore.FlavorsListDto{
		buildFlavor("m2", 2, 2),
	}))

	client, callCount, cleanup := newQueuedResponseClient(t, []queuedHTTPResponse{
		{statusCode: http.StatusOK, body: serversBody},
		{statusCode: http.StatusOK, body: flavorsBody},
	})
	defer cleanup()

	_, errInfo := commitProjectWithFallback(client, projectID)
	if errInfo == nil {
		t.Fatal("expected commit preflight to fail for undersized Kubemaster")
	}
	if !strings.Contains(errInfo.Message, "every Kubemaster") {
		t.Fatalf("expected Kubemaster sizing error, got %q", errInfo.Message)
	}
	if callCount.Load() != 2 {
		t.Fatalf("expected only preflight calls before failure, got %d", callCount.Load())
	}
}

func TestCommitProjectWithFallbackSkipsSizingValidationForTrackedVMMode(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(502)
	recordPendingStandaloneVMCreate(projectID)

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if strings.Contains(r.URL.Path, "/servers/") {
			t.Errorf("unexpected sizing validation call in VM mode: %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	cfg := taikuncore.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = strings.TrimPrefix(server.URL, "http://")
	client := &taikungoclient.Client{
		Client: taikuncore.NewAPIClient(cfg),
	}

	result, errInfo := commitProjectWithFallback(client, projectID)
	if errInfo != nil {
		t.Fatalf("expected VM-only commit to skip sizing validation, got error %+v", errInfo)
	}
	if result.Mode != "vm" {
		t.Fatalf("expected vm commit mode, got %q", result.Mode)
	}
	if callCount.Load() != 1 {
		t.Fatalf("expected only VM commit endpoint call, got %d calls", callCount.Load())
	}
}

func TestResolveCreateClusterProjectArgsRejectsAmbiguousKubernetesProfiles(t *testing.T) {
	client, _, cleanup := newQueuedResponseClient(t, []queuedHTTPResponse{
		{statusCode: http.StatusOK, body: `[{"id":1001,"name":"kp-a"},{"id":1002,"name":"kp-b"}]`},
	})
	defer cleanup()

	_, errorResp := resolveCreateClusterProjectArgs(client, CreateClusterArgs{
		Name:              "cluster-a",
		CloudCredentialID: 300,
	})
	if errorResp == nil {
		t.Fatal("expected ambiguous Kubernetes profile selection error")
	}
	payload, ok := parseToolResponsePayload(errorResp)
	if !ok {
		t.Fatal("expected JSON error payload")
	}
	if !strings.Contains(payload["error"].(string), "Unable to auto-select Kubernetes profile") {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestResolveCreateClusterProjectArgsUsesExplicitProfiles(t *testing.T) {
	projectArgs, errorResp := resolveCreateClusterProjectArgs(nil, CreateClusterArgs{
		Name:                "cluster-explicit",
		CloudCredentialID:   301,
		KubernetesProfileID: 11,
		AlertingProfileID:   22,
		Monitoring:          true,
	})
	if errorResp != nil {
		t.Fatalf("did not expect error response, got %+v", errorResp)
	}
	if projectArgs.KubernetesProfileID != 11 || projectArgs.AlertingProfileID != 22 {
		t.Fatalf("expected explicit profile IDs to pass through, got %+v", projectArgs)
	}
}

func TestCreateClusterOrchestratesProjectNodesAndCommit(t *testing.T) {
	originalStatePath := mcpLockStateFilePath
	mcpLockStateFilePath = ""
	t.Cleanup(func() { mcpLockStateFilePath = originalStatePath })

	projectID := int32(9001)
	cloudID := int32(77)
	clusterName := "e2e-cluster"

	serversBody := mustMarshalJSONForDeploy(t, buildServersListForDetails(projectID, cloudID, false, []taikuncore.ServerListDto{
		buildServer(taikuncore.CLOUDROLE_BASTION, clusterName+"-bastion", "small"),
		buildServer(taikuncore.CLOUDROLE_KUBEMASTER, clusterName+"-master", "medium"),
		buildServer(taikuncore.CLOUDROLE_KUBEWORKER, clusterName+"-worker", "medium"),
	}))
	flavorsBody := mustMarshalJSONForDeploy(t, buildAllFlavorsList([]taikuncore.FlavorsListDto{
		buildFlavor("small", 2, 2),
		buildFlavor("medium", 4, 4),
	}))

	client, callCount, cleanup := newQueuedResponseClient(t, []queuedHTTPResponse{
		{statusCode: http.StatusOK, body: `{"id":"9001"}`}, // create-project
		{statusCode: http.StatusOK, body: flavorsBody},     // select flavors
		{statusCode: http.StatusOK, body: `{}`},            // bind flavors to project
		{statusCode: http.StatusOK, body: `{}`},            // add bastion
		{statusCode: http.StatusOK, body: serversBody},     // verify bastion
		{statusCode: http.StatusOK, body: `{}`},            // add master
		{statusCode: http.StatusOK, body: serversBody},     // verify master
		{statusCode: http.StatusOK, body: `{}`},            // add worker
		{statusCode: http.StatusOK, body: serversBody},     // verify worker
		{statusCode: http.StatusOK, body: serversBody},     // commit preflight servers
		{statusCode: http.StatusOK, body: flavorsBody},     // commit preflight flavors (master)
		{statusCode: http.StatusOK, body: flavorsBody},     // commit preflight flavors (worker)
		{statusCode: http.StatusOK, body: `{}`},            // commit
	})
	defer cleanup()

	wait := false
	resp, err := createCluster(client, CreateClusterArgs{
		Name:                clusterName,
		CloudCredentialID:   cloudID,
		KubernetesProfileID: 5001,
		BastionCount:        1,
		MasterCount:         1,
		WorkerCount:         1,
		WorkerFlavor:        "medium",
		WaitForCreation:     &wait,
	})
	if err != nil {
		t.Fatalf("createCluster returned error: %v", err)
	}
	if !isToolResponseSuccess(resp) {
		payload, _ := parseToolResponsePayload(resp)
		t.Fatalf("expected success response, got %+v", payload)
	}
	payload, ok := parseToolResponsePayload(resp)
	if !ok {
		t.Fatal("expected create-cluster response payload")
	}
	if payload["projectId"] == nil {
		t.Fatalf("expected projectId in response, got %+v", payload)
	}
	if callCount.Load() != 13 {
		t.Fatalf("expected 13 API calls, got %d", callCount.Load())
	}
}

func TestCreateClusterFlavorFailureReportsCreatedProjectContext(t *testing.T) {
	originalStatePath := mcpLockStateFilePath
	mcpLockStateFilePath = ""
	t.Cleanup(func() { mcpLockStateFilePath = originalStatePath })

	projectID := int32(9010)
	cloudID := int32(77)

	client, callCount, cleanup := newQueuedResponseClient(t, []queuedHTTPResponse{
		{statusCode: http.StatusOK, body: `{"id":"9010"}`},
		{statusCode: http.StatusOK, body: `{"data":[],"totalCount":0}`},
	})
	defer cleanup()

	wait := false
	resp, err := createCluster(client, CreateClusterArgs{
		Name:                "cluster-flavor-failure",
		CloudCredentialID:   cloudID,
		KubernetesProfileID: 5001,
		WaitForCreation:     &wait,
	})
	if err != nil {
		t.Fatalf("createCluster returned error: %v", err)
	}
	if isToolResponseSuccess(resp) {
		payload, _ := parseToolResponsePayload(resp)
		t.Fatalf("expected failure response, got %+v", payload)
	}

	payload, ok := parseToolResponsePayload(resp)
	if !ok {
		t.Fatal("expected JSON payload in failure response")
	}
	if payload["projectId"] != float64(projectID) {
		t.Fatalf("expected projectId %d in error response, got %+v", projectID, payload["projectId"])
	}
	if payload["projectCreated"] != true {
		t.Fatalf("expected projectCreated=true in error response, got %+v", payload["projectCreated"])
	}
	details, _ := payload["details"].(string)
	if !strings.Contains(details, "already created") {
		t.Fatalf("expected created-project guidance in details, got %q", details)
	}
	if callCount.Load() != 2 {
		t.Fatalf("expected 2 API calls (create project + flavors), got %d", callCount.Load())
	}
}

func TestResolveCreateClusterFlavorsWorkerUsesCommitMinimumWhenMonitoringDisabled(t *testing.T) {
	flavorsBody := mustMarshalJSONForDeploy(t, buildAllFlavorsList([]taikuncore.FlavorsListDto{
		buildFlavor("small", 2, 2),
		buildFlavor("medium", 4, 4),
	}))
	client, _, cleanup := newQueuedResponseClient(t, []queuedHTTPResponse{
		{statusCode: http.StatusOK, body: flavorsBody},
	})
	defer cleanup()

	sel, errResp := resolveCreateClusterFlavors(client, CreateClusterArgs{
		CloudCredentialID: 1,
		Monitoring:        false,
	})
	if errResp != nil {
		t.Fatalf("expected flavor resolution to succeed, got %+v", errResp)
	}
	if sel.Bastion != "small" || sel.Master != "medium" {
		t.Fatalf("unexpected bastion/master flavors: bastion=%q master=%q", sel.Bastion, sel.Master)
	}
	if sel.Worker != "medium" {
		t.Fatalf("expected worker flavor to match commit minimum (medium / 4 CPU), got %q", sel.Worker)
	}
}

func TestResolveCreateClusterFlavorsFailsWhenWorkerOverrideBelowCommitMinimum(t *testing.T) {
	flavorsBody := mustMarshalJSONForDeploy(t, buildAllFlavorsList([]taikuncore.FlavorsListDto{
		buildFlavor("small", 2, 2),
		buildFlavor("medium", 4, 4),
	}))
	client, _, cleanup := newQueuedResponseClient(t, []queuedHTTPResponse{
		{statusCode: http.StatusOK, body: flavorsBody},
	})
	defer cleanup()

	_, errResp := resolveCreateClusterFlavors(client, CreateClusterArgs{
		CloudCredentialID: 1,
		Monitoring:        false,
		WorkerFlavor:      "small",
	})
	if errResp == nil {
		t.Fatal("expected worker flavor override to be rejected when below commit minimum")
	}
	payload, ok := parseToolResponsePayload(errResp)
	if !ok {
		t.Fatal("expected JSON error payload")
	}
	if !strings.Contains(payload["error"].(string), "Unable to determine worker flavor") {
		t.Fatalf("unexpected error: %+v", payload)
	}
}
