package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
)

func newTestTaikunClient(baseURL string) *taikungoclient.Client {
	cfg := taikuncore.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = strings.TrimPrefix(baseURL, "http://")
	return &taikungoclient.Client{
		Client: taikuncore.NewAPIClient(cfg),
	}
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to marshal test value: %v", err)
	}
	return string(data)
}

func testProjectsListJSON(t *testing.T, projectID int32, monitoringEnabled bool) string {
	t.Helper()

	project := taikuncore.NewProjectListDetailDtoWithDefaults()
	project.Id = projectID
	project.Name = "observability-demo"
	project.IsMonitoringEnabled = monitoringEnabled
	project.Status = taikuncore.PROJECTSTATUS_READY
	project.Health = taikuncore.PROJECTHEALTH_HEALTHY
	project.CloudType = taikuncore.ECLOUDCREDENTIALTYPE_AWS
	project.ImportClusterType = taikuncore.IMPORTCLUSTERTYPE_NONE

	return mustMarshalJSON(t, taikuncore.NewProjectsList([]taikuncore.ProjectListDetailDto{*project}, 1))
}

func newMonitoringStatusServer(t *testing.T, projectID int32, monitoringEnabled bool) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			t.Fatalf("unexpected downstream path while checking monitoring precondition: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("Id"); got != strconv.Itoa(int(projectID)) {
			t.Fatalf("expected Id query %d, got %q", projectID, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testProjectsListJSON(t, projectID, monitoringEnabled)))
	}))
}

func TestNormalizeProjectAlertsModeCaseInsensitive(t *testing.T) {
	mode, response := normalizeProjectAlertsMode("vm")
	if response != nil {
		t.Fatalf("expected VM mode to normalize, got error response %+v", response)
	}
	if mode == nil || *mode != taikuncore.PROJECTTYPE_VM {
		t.Fatalf("expected VM mode, got %+v", mode)
	}
}

func TestNormalizeProjectAlertsModeRejectsInvalidValue(t *testing.T) {
	mode, response := normalizeProjectAlertsMode("cluster")
	if mode != nil {
		t.Fatalf("expected nil mode for invalid input, got %+v", mode)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Details, "K8S and VM") {
		t.Fatalf("expected allowed mode guidance, got %+v", result)
	}
}

func TestBuildLokiLogsQuerySetsOptionalFields(t *testing.T) {
	query, response := buildLokiLogsQuery(
		42,
		`{namespace="prod"}`,
		[]LokiLogFilterArgs{{Operator: "=", Value: "prod"}},
		"2026-04-16T12:00:00Z",
		"2026-04-16T13:00:00Z",
		200,
		"backward",
	)
	if response != nil {
		t.Fatalf("expected Loki query to build, got error response %+v", response)
	}
	if query.GetProjectId() != 42 {
		t.Fatalf("expected project ID 42, got %d", query.GetProjectId())
	}
	if query.GetParameters() != `e25hbWVzcGFjZT0icHJvZCJ9` {
		t.Fatalf("expected parameters to be preserved, got %q", query.GetParameters())
	}
	filters := query.GetFilters()
	if len(filters) != 1 || filters[0].GetOperator() != "=" || filters[0].GetValue() != "prod" {
		t.Fatalf("expected single structured filter, got %+v", filters)
	}
	if query.GetLimit() != 200 {
		t.Fatalf("expected limit 200, got %d", query.GetLimit())
	}
	if query.GetDirection() != "backward" {
		t.Fatalf("expected backward direction, got %q", query.GetDirection())
	}
}

func TestBuildLokiLogsQueryRejectsInvalidTime(t *testing.T) {
	query, response := buildLokiLogsQuery(42, "", nil, "not-a-time", "", 0, "")
	if query != nil {
		t.Fatalf("expected invalid timestamp to fail, got %+v", query)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Invalid startDate value") {
		t.Fatalf("expected clear startDate validation error, got %+v", result)
	}
}

func TestBuildLokiLogsQueryRejectsInvalidLogQL(t *testing.T) {
	query, response := buildLokiLogsQuery(42, `{namespace="prod"`, nil, "", "", 0, "")
	if query != nil {
		t.Fatalf("expected invalid LogQL to fail, got %+v", query)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Invalid LogQL query") {
		t.Fatalf("expected clear LogQL validation error, got %+v", result)
	}
	if !strings.Contains(result.Details, "LogQL expression") {
		t.Fatalf("expected LogQL guidance, got %+v", result)
	}
}

func TestRequireProjectMonitoringEnabledRejectsDisabledProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("Id"); got != "77" {
			t.Fatalf("expected Id query 77, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testProjectsListJSON(t, 77, false)))
	}))
	defer server.Close()

	project, response := requireProjectMonitoringEnabled(newTestTaikunClient(server.URL), 77)
	if project != nil {
		t.Fatalf("expected nil project when monitoring is disabled, got %+v", project)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Monitoring is not enabled for project 77") {
		t.Fatalf("expected monitoring-disabled error, got %+v", result)
	}
	if !strings.Contains(result.Details, "enable-project-monitoring") {
		t.Fatalf("expected enable hint in details, got %+v", result)
	}
}

func TestGetProjectMonitoringAlertsReturnsWrappedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/projects":
			_, _ = w.Write([]byte(testProjectsListJSON(t, 88, true)))
		case "/api/v1/projects/monitoringalerts":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST for monitoring alerts, got %s", r.Method)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"groups":[]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	response, err := getProjectMonitoringAlerts(newTestTaikunClient(server.URL), ProjectMonitoringAlertsArgs{ProjectID: 88})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ProjectMonitoringAlertsResponse](t, response)
	if !result.Success || result.ProjectID != 88 {
		t.Fatalf("expected successful wrapped response, got %+v", result)
	}
	if result.Status != "success" {
		t.Fatalf("expected status to be preserved, got %+v", result)
	}
	if result.Data == nil || len(result.Data.GetGroups()) != 0 {
		t.Fatalf("expected empty alert groups payload, got %+v", result)
	}
}

func TestGetProjectMonitoringAlertsRejectsDisabledProject(t *testing.T) {
	server := newMonitoringStatusServer(t, 89, false)
	defer server.Close()

	response, err := getProjectMonitoringAlerts(newTestTaikunClient(server.URL), ProjectMonitoringAlertsArgs{ProjectID: 89})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Monitoring is not enabled for project 89") {
		t.Fatalf("expected monitoring-disabled error, got %+v", result)
	}
}

func TestQueryProjectPrometheusMetricsRequiresParameters(t *testing.T) {
	server := newMonitoringStatusServer(t, 123, true)
	defer server.Close()

	response, err := queryProjectPrometheusMetrics(newTestTaikunClient(server.URL), QueryProjectPrometheusMetricsArgs{
		ProjectID: 123,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if result.Error != "parameters is required for Prometheus metrics queries" {
		t.Fatalf("expected missing-parameters validation, got %+v", result)
	}
}

func TestQueryProjectLokiLogsRejectsDisabledProject(t *testing.T) {
	server := newMonitoringStatusServer(t, 90, false)
	defer server.Close()

	response, err := queryProjectLokiLogs(newTestTaikunClient(server.URL), QueryProjectLokiLogsArgs{
		ProjectID:  90,
		Parameters: `{namespace="prod"}`,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Monitoring is not enabled for project 90") {
		t.Fatalf("expected monitoring-disabled error, got %+v", result)
	}
}

func TestExportProjectLokiLogsRejectsDisabledProject(t *testing.T) {
	server := newMonitoringStatusServer(t, 91, false)
	defer server.Close()

	response, err := exportProjectLokiLogs(newTestTaikunClient(server.URL), ExportProjectLokiLogsArgs{
		ProjectID:  91,
		Parameters: `{namespace="prod"}`,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Monitoring is not enabled for project 91") {
		t.Fatalf("expected monitoring-disabled error, got %+v", result)
	}
}

func TestEncodeQueryParametersUsesBase64(t *testing.T) {
	encoded := encodeQueryParameters(`up`)
	if encoded != "dXA=" {
		t.Fatalf("expected base64 encoding for query parameters, got %q", encoded)
	}
}

func TestQueryProjectPrometheusMetricsRejectsInvalidPromQL(t *testing.T) {
	server := newMonitoringStatusServer(t, 123, true)
	defer server.Close()

	response, err := queryProjectPrometheusMetrics(newTestTaikunClient(server.URL), QueryProjectPrometheusMetricsArgs{
		ProjectID:  123,
		Parameters: `sum(rate(container_cpu_usage_seconds_total[5m])`,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Invalid PromQL query") {
		t.Fatalf("expected PromQL validation error, got %+v", result)
	}
	if !strings.Contains(result.Details, "PromQL expression") {
		t.Fatalf("expected PromQL guidance, got %+v", result)
	}
}

func TestQueryProjectPrometheusMetricsRejectsDisabledProject(t *testing.T) {
	server := newMonitoringStatusServer(t, 92, false)
	defer server.Close()

	response, err := queryProjectPrometheusMetrics(newTestTaikunClient(server.URL), QueryProjectPrometheusMetricsArgs{
		ProjectID:  92,
		Parameters: `up`,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Monitoring is not enabled for project 92") {
		t.Fatalf("expected monitoring-disabled error, got %+v", result)
	}
}

func TestAutocompleteProjectPrometheusMetricsRejectsDisabledProject(t *testing.T) {
	server := newMonitoringStatusServer(t, 93, false)
	defer server.Close()

	response, err := autocompleteProjectPrometheusMetrics(newTestTaikunClient(server.URL), ProjectPrometheusMetricsAutocompleteArgs{
		ProjectID: 93,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Monitoring is not enabled for project 93") {
		t.Fatalf("expected monitoring-disabled error, got %+v", result)
	}
}
