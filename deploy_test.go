package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		switch callCount {
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
			t.Fatalf("unexpected extra request %d to %s", callCount, r.URL.Path)
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
	if callCount != 4 {
		t.Fatalf("expected preflight, commit, and VM fallback calls, got %d request(s)", callCount)
	}
	if got := pendingProjectCommitMode(projectID); got != projectCommitModeAuto {
		t.Fatalf("expected pending state to clear after successful fallback, got %q", got)
	}
}

type queuedHTTPResponse struct {
	statusCode int
	body       string
}

func newQueuedResponseClient(t *testing.T, responses []queuedHTTPResponse) (*taikungoclient.Client, *int, func()) {
	t.Helper()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callCount >= len(responses) {
			t.Fatalf("unexpected request %d to %s", callCount+1, r.URL.Path)
		}

		next := responses[callCount]
		callCount++
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

			wantCalls := 1 + len(testCase.flavorResponses)
			if *callCount != wantCalls {
				t.Fatalf("expected %d preflight calls, got %d", wantCalls, *callCount)
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
	if *callCount != 2 {
		t.Fatalf("expected only preflight calls before failure, got %d", *callCount)
	}
}

func TestCommitProjectWithFallbackSkipsSizingValidationForTrackedVMMode(t *testing.T) {
	resetPendingProjectCommitChangesForTest(t)

	projectID := int32(502)
	recordPendingStandaloneVMCreate(projectID)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if strings.Contains(r.URL.Path, "/servers/") {
			t.Fatalf("unexpected sizing validation call in VM mode: %s", r.URL.Path)
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
	if callCount != 1 {
		t.Fatalf("expected only VM commit endpoint call, got %d calls", callCount)
	}
}
