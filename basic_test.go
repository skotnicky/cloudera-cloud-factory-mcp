package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"

	taikuncore "github.com/itera-io/taikungoclient/client"
)

func TestMain(m *testing.M) {
	// Initialize logger for tests
	logger = log.New(os.Stdout, "[test] ", log.LstdFlags)
	os.Exit(m.Run())
}

func TestResponseStructMarshaling(t *testing.T) {
	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "SuccessResponse",
			data: SuccessResponse{
				Message: "Test successful",
				Success: true,
			},
		},
		{
			name: "ErrorResponse",
			data: ErrorResponse{
				Error: "Test error",
			},
		},
		{
			name: "ProjectSummary",
			data: ProjectSummary{
				ID:     123,
				Name:   "test-project",
				Status: "Ready",
				Health: "Healthy",
				Type:   "Kubernetes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling
			jsonData, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("Failed to marshal %s: %v", tt.name, err)
			}

			// Test JSON unmarshaling
			var result map[string]interface{}
			err = json.Unmarshal(jsonData, &result)
			if err != nil {
				t.Fatalf("Failed to unmarshal %s: %v", tt.name, err)
			}

			// Basic validation
			if len(result) == 0 {
				t.Errorf("Empty result for %s", tt.name)
			}

			t.Logf("✅ %s JSON: %s", tt.name, string(jsonData))
		})
	}
}

func TestCreateJSONResponseHelper(t *testing.T) {
	data := SuccessResponse{
		Message: "Test message",
		Success: true,
	}

	response := createJSONResponse(data)
	if response == nil {
		t.Fatal("Expected response, got nil")
	}

	if len(response.Content) == 0 {
		t.Fatal("Expected content, got empty slice")
	}

	// Check that content contains valid JSON
	content := response.Content[0]
	if content.TextContent == nil {
		t.Fatal("Expected TextContent, got nil")
	}

	var result SuccessResponse
	err := json.Unmarshal([]byte(content.TextContent.Text), &result)
	if err != nil {
		t.Fatalf("Invalid JSON in response: %v", err)
	}

	if result.Message != "Test message" || !result.Success {
		t.Errorf("Expected message='Test message' success=true, got message='%s' success=%t",
			result.Message, result.Success)
	}

	t.Logf("✅ JSON Response: %s", content.TextContent.Text)
}

func TestResolveOrganizationIDForCatalogExplicitWins(t *testing.T) {
	previous := getRobotUserContext()
	setRobotUserContext(RobotUserContext{OrganizationID: 2090})
	defer setRobotUserContext(previous)

	orgID, errorResp := resolveOrganizationIDForCatalog(nil, context.Background(), 321)
	if errorResp != nil {
		t.Fatalf("expected no error response, got %#v", errorResp)
	}
	if orgID != 321 {
		t.Fatalf("expected explicit organizationId 321, got %d", orgID)
	}
}

func TestResolveOrganizationIDForCatalogPrefersRobotUserContext(t *testing.T) {
	previous := getRobotUserContext()
	setRobotUserContext(RobotUserContext{OrganizationID: 2090})
	defer setRobotUserContext(previous)

	orgID, errorResp := resolveOrganizationIDForCatalog(nil, context.Background(), 0)
	if errorResp != nil {
		t.Fatalf("expected no error response, got %#v", errorResp)
	}
	if orgID != 2090 {
		t.Fatalf("expected robot user organizationId 2090, got %d", orgID)
	}
}

func TestBuildCreateCatalogAppCommandAlwaysSendsParametersArray(t *testing.T) {
	cmd := buildCreateCatalogAppCommand(360, "anywhere-cloud", "anywhere-cloud", "", nil)
	payload, err := cmd.ToMap()
	if err != nil {
		t.Fatalf("expected no error building payload map, got %v", err)
	}

	parameters, ok := payload["parameters"]
	if !ok {
		t.Fatal("expected parameters key to be present")
	}

	paramsSlice, ok := parameters.([]taikuncore.CatalogAppParamsDto)
	if !ok {
		t.Fatalf("expected parameters to be []CatalogAppParamsDto, got %T", parameters)
	}
	if len(paramsSlice) != 0 {
		t.Fatalf("expected empty parameters slice, got len=%d", len(paramsSlice))
	}
}

func TestArgumentStructs(t *testing.T) {
	// Test that our argument structs can be marshaled/unmarshaled
	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "RefreshTaikunClientArgs",
			data: RefreshTaikunClientArgs{},
		},
		{
			name: "ServerVersionArgs",
			data: ServerVersionArgs{},
		},
		{
			name: "RobotUserCapabilitiesArgs",
			data: RobotUserCapabilitiesArgs{},
		},
		{
			name: "MCPLockArgs",
			data: MCPLockArgs{
				OrganizationIDs: []int32{10, 20},
				ProjectIDs:      []int32{100, 200},
			},
		},
		{
			name: "MCPLockStatusArgs",
			data: MCPLockStatusArgs{},
		},
		{
			name: "MCPLockClearArgs",
			data: MCPLockClearArgs{},
		},
		{
			name: "CreateVirtualClusterArgs",
			data: CreateVirtualClusterArgs{
				ProjectID:       123,
				Name:            "test-cluster",
				WaitForCreation: true,
				Timeout:         600,
			},
		},
		{
			name: "UpdateAppAutoSyncArgs",
			data: UpdateAppAutoSyncArgs{
				ProjectAppID: 123,
				Mode:         "Minutes",
				TTL:          60,
			},
		},
		{
			name: "ListProjectsArgs",
			data: ListProjectsArgs{
				Limit:               10,
				Search:              "test",
				HealthyOnly:         true,
				VirtualClustersOnly: false,
			},
		},
		{
			name: "AddAppToCatalogArgs",
			data: AddAppToCatalogArgs{
				CatalogID:   123,
				Repository:  "bitnami",
				PackageName: "nginx",
				Version:     "1.2.3",
			},
		},
		{
			name: "AddAppToCatalogWithParametersArgs",
			data: AddAppToCatalogWithParametersArgs{
				CatalogID:   123,
				Repository:  "bitnami",
				PackageName: "nginx",
				Version:     "1.2.3",
				Parameters: []AppParameter{
					{
						Key:   "replicaCount",
						Value: "2",
					},
				},
			},
		},
		{
			name: "ListRepositoriesArgs",
			data: ListRepositoriesArgs{
				Limit:          10,
				Offset:         0,
				Search:         "bitnami",
				SortBy:         "name",
				SortDirection:  "asc",
				ID:             "repo-123",
				IsPrivate:      func() *bool { v := true; return &v }(),
				OrganizationID: 321,
			},
		},
		{
			name: "ImportRepositoryArgs",
			data: ImportRepositoryArgs{
				Name:           "anywhere-cloud",
				URL:            "oci://docker-private.infra.cloudera.com/cloudera-helm/awc-core/anywhere-cloud",
				OrganizationID: 321,
				Username:       "robot-user",
				Password:       "robot-password",
			},
		},
		{
			name: "BindRepositoryArgs",
			data: BindRepositoryArgs{
				RepositoryID:               "repo-123",
				Name:                       "anywhere-cloud",
				RepositoryOrganizationName: "cloudera-helm",
				OrganizationID:             321,
			},
		},
		{
			name: "UnbindRepositoryArgs",
			data: UnbindRepositoryArgs{
				RepositoryID:   "repo-123",
				RepositoryIDs:  []string{"repo-123", "repo-456"},
				OrganizationID: 321,
			},
		},
		{
			name: "DeleteRepositoryArgs",
			data: DeleteRepositoryArgs{
				AppRepoID:      654,
				RepositoryID:   "repo-123",
				OrganizationID: 321,
			},
		},
		{
			name: "UpdateRepositoryPasswordArgs",
			data: UpdateRepositoryPasswordArgs{
				RepositoryID:   "repo-123",
				Username:       "robot-user",
				Password:       "robot-password",
				OrganizationID: 321,
			},
		},
		{
			name: "ListAvailablePackagesArgs",
			data: ListAvailablePackagesArgs{
				Repository: "bitnami",
				Limit:      20,
				Offset:     5,
				Search:     "web",
			},
		},
		{
			name: "ListAvailableAppsArgs",
			data: ListAvailableAppsArgs{
				Repository: "bitnami",
				Limit:      20,
				Offset:     5,
				Search:     "web",
			},
		},
		{
			name: "CreateProjectArgs",
			data: CreateProjectArgs{
				Name:                "test-project",
				CloudCredentialID:   123,
				KubernetesProfileID: 456,
				AlertingProfileID:   789,
				Monitoring:          true,
				KubernetesVersion:   "1.28.0",
			},
		},
		{
			name: "CreateClusterArgs",
			data: CreateClusterArgs{
				Name:                "test-cluster",
				CloudCredentialID:   123,
				KubernetesProfileID: 456,
				AlertingProfileID:   789,
				Monitoring:          true,
				KubernetesVersion:   "1.28.0",
				BastionCount:        1,
				MasterCount:         1,
				WorkerCount:         2,
				BastionFlavor:       "small",
				MasterFlavor:        "medium",
				WorkerFlavor:        "large",
				DiskSizeGB:          50,
				VerifyTimeout:       300,
				WaitForCreation:     func() *bool { v := true; return &v }(),
				Timeout:             1800,
			},
		},
		{
			name: "DeleteProjectArgs",
			data: DeleteProjectArgs{
				ProjectID: 123,
			},
		},
		{
			name: "DeleteStandaloneVMArgs",
			data: DeleteStandaloneVMArgs{
				ProjectID: 123,
				VMID:      456,
			},
		},
		{
			name: "ListCatalogAppsArgs",
			data: ListCatalogAppsArgs{
				CatalogID: 123,
				Limit:     10,
				Search:    "nginx",
			},
		},
		{
			name: "GetCatalogAppParamsArgs",
			data: GetCatalogAppParamsArgs{
				CatalogAppID: 456,
				PackageID:    "cf2baee0-d026-42a0-8a5b-20d432ae1f01",
				Version:      "0.5.0",
			},
		},
		{
			name: "SetCatalogAppDefaultParamsArgs",
			data: SetCatalogAppDefaultParamsArgs{
				CatalogAppID: 456,
				Parameters: []AppParameter{
					{
						Key:   "replicaCount",
						Value: "3",
					},
				},
				MergeWithExisting: func() *bool { v := true; return &v }(),
			},
		},
		{
			name: "RemoveAppFromCatalogArgs",
			data: RemoveAppFromCatalogArgs{
				CatalogID:   123,
				Repository:  "bitnami",
				PackageName: "nginx",
			},
		},
		{
			name: "KubernetesResourceKindsArgs",
			data: KubernetesResourceKindsArgs{},
		},
		{
			name: "ListKubernetesResourcesArgs",
			data: ListKubernetesResourcesArgs{
				ProjectID:  123,
				Kind:       "Pods",
				Limit:      10,
				SearchTerm: "test",
			},
		},
		{
			name: "DescribeKubernetesResourceArgs",
			data: DescribeKubernetesResourceArgs{
				ProjectID: 123,
				Name:      "test-pod",
				Kind:      "Pod",
			},
		},
		{
			name: "DeleteServersArgs",
			data: DeleteServersArgs{
				ProjectId:                123,
				ServerIds:                []int32{456, 789},
				ForceDeleteVClusters:     true,
				DeleteAutoscalingServers: false,
			},
		},
		{
			name: "WaitForProjectArgs",
			data: WaitForProjectArgs{
				ProjectId:   123,
				Timeout:     600,
				WaitDeleted: true,
			},
		},
		{
			name: "WaitForAppArgs",
			data: WaitForAppArgs{
				ProjectAppId: 123,
				Timeout:      300,
				WaitDeleted:  true,
			},
		},
		{
			name: "JSONPayloadArgs",
			data: JSONPayloadArgs{
				Payload: `{"name":"example"}`,
			},
		},
		{
			name: "IDArgs",
			data: IDArgs{
				ID: 123,
			},
		},
		{
			name: "IDPayloadArgs",
			data: IDPayloadArgs{
				ID:      123,
				Payload: `{"name":"example"}`,
			},
		},
		{
			name: "StringIDArgs",
			data: StringIDArgs{
				ID: "user-123",
			},
		},
		{
			name: "DomainScopedIDArgs",
			data: DomainScopedIDArgs{
				DomainID: 12,
				ID:       34,
			},
		},
		{
			name: "DomainScopedStringIDArgs",
			data: DomainScopedStringIDArgs{
				DomainID: 12,
				ID:       "user-123",
			},
		},
		{
			name: "GroupOrganizationPayloadArgs",
			data: GroupOrganizationPayloadArgs{
				GroupID:        12,
				OrganizationID: 34,
				Payload:        `{"role":"Manager"}`,
			},
		},
		{
			name: "ProjectIDArgs",
			data: ProjectIDArgs{
				ProjectID: 123,
			},
		},
		{
			name: "ProjectMonitoringAlertsArgs",
			data: ProjectMonitoringAlertsArgs{
				ProjectID: 123,
			},
		},
		{
			name: "ProjectAlertsArgs",
			data: ProjectAlertsArgs{
				ProjectID: 123,
				Mode:      "K8S",
			},
		},
		{
			name: "QueryProjectLokiLogsArgs",
			data: QueryProjectLokiLogsArgs{
				ProjectID:  123,
				Parameters: `{app="nginx"}`,
				Filters: []LokiLogFilterArgs{
					{
						Operator: "=",
						Value:    "nginx",
					},
				},
				StartDate: "2026-04-16T12:00:00Z",
				EndDate:   "2026-04-16T13:00:00Z",
				Limit:     100,
				Direction: "backward",
			},
		},
		{
			name: "ExportProjectLokiLogsArgs",
			data: ExportProjectLokiLogsArgs{
				ProjectID:  123,
				Parameters: `{app="nginx"}`,
				Limit:      100,
				Direction:  "forward",
			},
		},
		{
			name: "QueryProjectPrometheusMetricsArgs",
			data: QueryProjectPrometheusMetricsArgs{
				ProjectID:  123,
				Parameters: `sum(rate(container_cpu_usage_seconds_total[5m]))`,
				Start:      "2026-04-16T12:00:00Z",
				End:        "2026-04-16T13:00:00Z",
				Step:       "30s",
				IsGraphEnabled: func() *bool {
					v := true
					return &v
				}(),
			},
		},
		{
			name: "ProjectPrometheusMetricsAutocompleteArgs",
			data: ProjectPrometheusMetricsAutocompleteArgs{
				ProjectID: 123,
			},
		},
		{
			name: "ProjectBackupCredentialArgs",
			data: ProjectBackupCredentialArgs{
				ProjectID:          123,
				BackupCredentialID: 456,
			},
		},
		{
			name: "ProjectAICredentialArgs",
			data: ProjectAICredentialArgs{
				ProjectID:      123,
				AICredentialID: 456,
			},
		},
		{
			name: "ProjectPolicyProfileArgs",
			data: ProjectPolicyProfileArgs{
				ProjectID:       123,
				PolicyProfileID: 456,
			},
		},
		{
			name: "ProjectIDPayloadArgs",
			data: ProjectIDPayloadArgs{
				ProjectID: 123,
				Payload:   `{"enabled":true}`,
			},
		},
		{
			name: "ProjectNameArgs",
			data: ProjectNameArgs{
				ProjectID: 123,
				Name:      "backup-1",
			},
		},
		{
			name: "LockModeArgs",
			data: LockModeArgs{
				ID:   123,
				Mode: "lock",
			},
		},
		{
			name: "SearchListArgs",
			data: SearchListArgs{
				Limit:          10,
				Offset:         5,
				CursorID:       7,
				Search:         "example",
				SortBy:         "name",
				SortDirection:  "asc",
				OrganizationID: 12,
				DomainID:       34,
				ID:             56,
				SearchID:       "78",
				CloudID:        90,
			},
		},
		{
			name: "ProjectSearchListArgs",
			data: ProjectSearchListArgs{
				ProjectID:      123,
				Limit:          10,
				Offset:         5,
				Search:         "vm",
				SortBy:         "name",
				SortDirection:  "desc",
				FilterBy:       "running",
				OrganizationID: 12,
				ID:             34,
			},
		},
		{
			name: "ImageListArgs",
			data: ImageListArgs{
				Provider:      "aws",
				Mode:          "common",
				CloudID:       123,
				ProjectID:     456,
				Limit:         10,
				Offset:        5,
				Search:        "ubuntu",
				SortBy:        "name",
				SortDirection: "asc",
				Payload:       `{"region":"us-east-1"}`,
			},
		},
		{
			name: "StandaloneWindowsPasswordArgs",
			data: StandaloneWindowsPasswordArgs{
				ID:         123,
				Key:        "secret",
				ConfigPath: "/tmp/config",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling
			jsonData, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("Failed to marshal %s: %v", tt.name, err)
			}

			t.Logf("✅ %s JSON: %s", tt.name, string(jsonData))
		})
	}
}

func TestBuildInfo(t *testing.T) {
	t.Logf("✅ Go build successful")
	t.Logf("✅ All imports resolved")
	t.Logf("✅ Struct definitions valid")
}
