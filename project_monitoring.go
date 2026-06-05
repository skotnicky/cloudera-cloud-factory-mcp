package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

type ProjectMonitoringAlertsArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=The project ID to load Prometheus-style monitoring alerts for"`
}

type ProjectAlertsArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=The project ID to load project detail alerts/messages for"`
	Mode      string `json:"mode,omitempty" jsonschema:"description=Optional project mode filter: K8S or VM"`
}

type LokiLogFilterArgs struct {
	Operator string `json:"operator,omitempty" jsonschema:"description=Optional Loki filter operator"`
	Value    string `json:"value,omitempty" jsonschema:"description=Optional Loki filter value"`
}

type QueryProjectLokiLogsArgs struct {
	ProjectID  int32               `json:"projectId" jsonschema:"required,description=The project ID to query Loki logs for"`
	Parameters string              `json:"parameters,omitempty" jsonschema:"description=Optional LogQL query expression"`
	Filters    []LokiLogFilterArgs `json:"filters,omitempty" jsonschema:"description=Optional structured Loki filters"`
	StartDate  string              `json:"startDate,omitempty" jsonschema:"description=Optional RFC3339 start time"`
	EndDate    string              `json:"endDate,omitempty" jsonschema:"description=Optional RFC3339 end time"`
	Limit      int32               `json:"limit,omitempty" jsonschema:"description=Optional maximum number of log rows to return"`
	Direction  string              `json:"direction,omitempty" jsonschema:"description=Optional log direction such as forward or backward"`
}

type ExportProjectLokiLogsArgs struct {
	ProjectID  int32               `json:"projectId" jsonschema:"required,description=The project ID to export Loki logs for"`
	Parameters string              `json:"parameters,omitempty" jsonschema:"description=Optional LogQL query expression"`
	Filters    []LokiLogFilterArgs `json:"filters,omitempty" jsonschema:"description=Optional structured Loki filters"`
	StartDate  string              `json:"startDate,omitempty" jsonschema:"description=Optional RFC3339 start time"`
	EndDate    string              `json:"endDate,omitempty" jsonschema:"description=Optional RFC3339 end time"`
	Limit      int32               `json:"limit,omitempty" jsonschema:"description=Optional maximum number of log rows to export"`
	Direction  string              `json:"direction,omitempty" jsonschema:"description=Optional log direction such as forward or backward"`
}

type QueryProjectPrometheusMetricsArgs struct {
	ProjectID      int32  `json:"projectId" jsonschema:"required,description=The project ID to query Prometheus metrics for"`
	Parameters     string `json:"parameters" jsonschema:"required,description=PromQL query expression"`
	Time           string `json:"time,omitempty" jsonschema:"description=Optional RFC3339 evaluation time for instant queries"`
	Start          string `json:"start,omitempty" jsonschema:"description=Optional RFC3339 range query start time"`
	End            string `json:"end,omitempty" jsonschema:"description=Optional RFC3339 range query end time"`
	IsGraphEnabled *bool  `json:"isGraphEnabled,omitempty" jsonschema:"description=Optional graph mode flag for Prometheus query execution"`
	Step           string `json:"step,omitempty" jsonschema:"description=Optional Prometheus step value such as 30s or 5m"`
}

type ProjectPrometheusMetricsAutocompleteArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=The project ID to load Prometheus metric autocomplete suggestions for"`
}

type ProjectMonitoringAlertsResponse struct {
	ProjectID int32                 `json:"projectId"`
	Status    string                `json:"status,omitempty"`
	Data      *taikuncore.AlertData `json:"data,omitempty"`
	Success   bool                  `json:"success"`
	Message   string                `json:"message"`
}

type ProjectAlertsResponse struct {
	ProjectID int32                                   `json:"projectId"`
	Mode      string                                  `json:"mode,omitempty"`
	Alerts    []taikuncore.ProjectDetailsErrorListDto `json:"alerts"`
	Success   bool                                    `json:"success"`
	Message   string                                  `json:"message"`
}

type ProjectLokiLogsResponse struct {
	ProjectID int32                   `json:"projectId"`
	Results   []taikuncore.LokiResult `json:"results"`
	Success   bool                    `json:"success"`
	Message   string                  `json:"message"`
}

type ProjectLokiLogsExportResponse struct {
	ProjectID   int32  `json:"projectId"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
	Success     bool   `json:"success"`
	Message     string `json:"message"`
}

type ProjectPrometheusMetricsResponse struct {
	ProjectID int32                  `json:"projectId"`
	Status    string                 `json:"status,omitempty"`
	Data      *taikuncore.MetricData `json:"data,omitempty"`
	Success   bool                   `json:"success"`
	Message   string                 `json:"message"`
}

type ProjectPrometheusMetricsAutocompleteResponse struct {
	ProjectID   int32    `json:"projectId"`
	Suggestions []string `json:"suggestions"`
	Success     bool     `json:"success"`
	Message     string   `json:"message"`
}

type monitoringProjectStatus struct {
	ID                int32
	Name              string
	MonitoringEnabled bool
	Status            string
}

func getMonitoringProjectStatus(client *taikungoclient.Client, projectID int32) (*monitoringProjectStatus, *mcp_golang.ToolResponse) {
	ctx := context.Background()
	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsList(ctx).
		Id(projectID).
		Execute()
	if err != nil {
		return nil, createError(httpResponse, err)
	}
	if errorResp := checkResponse(httpResponse, "get project monitoring status"); errorResp != nil {
		return nil, errorResp
	}
	if result == nil || len(result.Data) == 0 {
		return nil, createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("Project with ID %d not found", projectID),
		})
	}

	project := result.Data[0]
	return &monitoringProjectStatus{
		ID:                project.GetId(),
		Name:              project.GetName(),
		MonitoringEnabled: project.GetIsMonitoringEnabled(),
		Status:            string(project.GetStatus()),
	}, nil
}

// Short-TTL cache of per-project "monitoring enabled" so the monitoring/log/
// metric tools do not perform an extra ProjectsList round-trip on every call.
// Project IDs are globally unique in CCF, so the project ID is a sufficient key.
type cachedMonitoringStatus struct {
	enabled   bool
	expiresAt time.Time
}

const (
	monitoringStatusCacheTTL     = 60 * time.Second
	monitoringStatusCacheMaxSize = 4096
)

var (
	monitoringStatusCacheMu sync.Mutex
	monitoringStatusCache   = make(map[int32]cachedMonitoringStatus)
)

func getCachedMonitoringEnabled(projectID int32) (bool, bool) {
	monitoringStatusCacheMu.Lock()
	defer monitoringStatusCacheMu.Unlock()
	entry, ok := monitoringStatusCache[projectID]
	if !ok {
		return false, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(monitoringStatusCache, projectID)
		return false, false
	}
	return entry.enabled, true
}

func setCachedMonitoringEnabled(projectID int32, enabled bool) {
	monitoringStatusCacheMu.Lock()
	defer monitoringStatusCacheMu.Unlock()
	if len(monitoringStatusCache) >= monitoringStatusCacheMaxSize {
		now := time.Now()
		for id, entry := range monitoringStatusCache {
			if now.After(entry.expiresAt) {
				delete(monitoringStatusCache, id)
			}
		}
		if len(monitoringStatusCache) >= monitoringStatusCacheMaxSize {
			monitoringStatusCache = make(map[int32]cachedMonitoringStatus)
		}
	}
	monitoringStatusCache[projectID] = cachedMonitoringStatus{
		enabled:   enabled,
		expiresAt: time.Now().Add(monitoringStatusCacheTTL),
	}
}

// invalidateCachedMonitoringStatus clears the cached state for a project so an
// enable/disable change is observed immediately by later monitoring queries.
func invalidateCachedMonitoringStatus(projectID int32) {
	monitoringStatusCacheMu.Lock()
	delete(monitoringStatusCache, projectID)
	monitoringStatusCacheMu.Unlock()
}

func monitoringNotEnabledResponse(projectID int32) *mcp_golang.ToolResponse {
	return createJSONResponse(ErrorResponse{
		Error:   fmt.Sprintf("Monitoring is not enabled for project %d", projectID),
		Details: "Enable monitoring on the project first with enable-project-monitoring before querying alerts, logs, or metrics.",
	})
}

func requireProjectMonitoringEnabled(client *taikungoclient.Client, projectID int32) (*monitoringProjectStatus, *mcp_golang.ToolResponse) {
	if enabled, ok := getCachedMonitoringEnabled(projectID); ok {
		if !enabled {
			return nil, monitoringNotEnabledResponse(projectID)
		}
		return &monitoringProjectStatus{ID: projectID, MonitoringEnabled: true}, nil
	}

	project, errorResp := getMonitoringProjectStatus(client, projectID)
	if errorResp != nil {
		return nil, errorResp
	}
	setCachedMonitoringEnabled(projectID, project.MonitoringEnabled)
	if !project.MonitoringEnabled {
		return nil, monitoringNotEnabledResponse(projectID)
	}
	return project, nil
}

func parseOptionalRFC3339(value string, fieldName string) (*time.Time, *mcp_golang.ToolResponse) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Invalid %s value: %s", fieldName, value),
			Details: fmt.Sprintf("%s must be a valid RFC3339 timestamp", fieldName),
		})
	}
	return &parsed, nil
}

// checkBalancedDelimiters performs a lightweight syntactic sanity check on a
// query expression. It intentionally avoids importing the full LogQL/PromQL
// parsers (which drag in very large transitive dependency trees); the
// monitoring backend performs authoritative validation and returns precise
// errors, so a cheap balanced-delimiter check is enough to catch obvious typos.
func checkBalancedDelimiters(expr string) error {
	closers := map[rune]rune{')': '(', ']': '[', '}': '{'}
	openers := map[rune]bool{'(': true, '[': true, '{': true}
	var stack []rune
	for _, r := range expr {
		switch {
		case openers[r]:
			stack = append(stack, r)
		case closers[r] != 0:
			if len(stack) == 0 || stack[len(stack)-1] != closers[r] {
				return fmt.Errorf("unbalanced %q", string(r))
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed %q", string(stack[len(stack)-1]))
	}
	return nil
}

func validateLogQLExpression(parameters string) *mcp_golang.ToolResponse {
	trimmed := strings.TrimSpace(parameters)
	if trimmed == "" {
		return nil
	}
	if err := checkBalancedDelimiters(trimmed); err != nil {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Invalid LogQL query: %v", err),
			Details: "parameters must be a syntactically balanced LogQL expression; the monitoring backend performs full validation.",
		})
	}
	return nil
}

func validatePromQLExpression(parameters string) *mcp_golang.ToolResponse {
	trimmed := strings.TrimSpace(parameters)
	if trimmed == "" {
		return createJSONResponse(ErrorResponse{
			Error: "parameters is required for Prometheus metrics queries",
		})
	}
	if err := checkBalancedDelimiters(trimmed); err != nil {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Invalid PromQL query: %v", err),
			Details: "parameters must be a syntactically balanced PromQL expression; the monitoring backend performs full validation.",
		})
	}
	return nil
}

func encodeQueryParameters(parameters string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(parameters)))
}

func normalizeProjectAlertsMode(mode string) (*taikuncore.ProjectType, *mcp_golang.ToolResponse) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return nil, nil
	}
	for _, allowed := range taikuncore.AllowedProjectTypeEnumValues {
		if strings.EqualFold(string(allowed), trimmed) {
			normalized := allowed
			return &normalized, nil
		}
	}
	return nil, createJSONResponse(ErrorResponse{
		Error:   fmt.Sprintf("Invalid project alert mode: %s", mode),
		Details: "Allowed mode values are K8S and VM.",
	})
}

func buildLokiLogsQuery(projectID int32, parameters string, filters []LokiLogFilterArgs, startDate string, endDate string, limit int32, direction string) (*taikuncore.LokiLogsQuery, *mcp_golang.ToolResponse) {
	query := taikuncore.NewLokiLogsQuery()
	query.SetProjectId(projectID)

	if strings.TrimSpace(parameters) != "" {
		if errorResp := validateLogQLExpression(parameters); errorResp != nil {
			return nil, errorResp
		}
		query.SetParameters(encodeQueryParameters(parameters))
	}
	if len(filters) > 0 {
		items := make([]taikuncore.Filter, 0, len(filters))
		for _, filter := range filters {
			item := taikuncore.NewFilter()
			if strings.TrimSpace(filter.Operator) != "" {
				item.SetOperator(filter.Operator)
			}
			if strings.TrimSpace(filter.Value) != "" {
				item.SetValue(filter.Value)
			}
			items = append(items, *item)
		}
		query.SetFilters(items)
	}

	startTime, errorResp := parseOptionalRFC3339(startDate, "startDate")
	if errorResp != nil {
		return nil, errorResp
	}
	if startTime != nil {
		query.SetStartDate(*startTime)
	}

	endTime, errorResp := parseOptionalRFC3339(endDate, "endDate")
	if errorResp != nil {
		return nil, errorResp
	}
	if endTime != nil {
		query.SetEndDate(*endTime)
	}

	if limit > 0 {
		query.SetLimit(limit)
	}
	if strings.TrimSpace(direction) != "" {
		query.SetDirection(direction)
	}
	return query, nil
}

func getProjectMonitoringAlerts(client *taikungoclient.Client, args ProjectMonitoringAlertsArgs) (*mcp_golang.ToolResponse, error) {
	if _, errorResp := requireProjectMonitoringEnabled(client, args.ProjectID); errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewProjectsMonitoringAlertsCommand()
	command.SetProjectId(args.ProjectID)

	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsMonitoringAlerts(context.Background()).
		ProjectsMonitoringAlertsCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get project monitoring alerts"); errorResp != nil {
		return errorResp, nil
	}

	response := ProjectMonitoringAlertsResponse{
		ProjectID: args.ProjectID,
		Success:   true,
		Message:   fmt.Sprintf("Loaded monitoring alerts for project %d", args.ProjectID),
	}
	if result != nil {
		response.Status = result.GetStatus()
		if data, ok := result.GetDataOk(); ok && data != nil {
			response.Data = data
		}
	}
	return createJSONResponse(response), nil
}

func listProjectAlerts(client *taikungoclient.Client, args ProjectAlertsArgs) (*mcp_golang.ToolResponse, error) {
	if _, errorResp := requireProjectMonitoringEnabled(client, args.ProjectID); errorResp != nil {
		return errorResp, nil
	}

	query := taikuncore.NewProjectAlertsQuery()
	query.SetProjectId(args.ProjectID)
	mode, errorResp := normalizeProjectAlertsMode(args.Mode)
	if errorResp != nil {
		return errorResp, nil
	}
	if mode != nil {
		query.SetMode(*mode)
	}

	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsAlerts(context.Background()).
		ProjectAlertsQuery(*query).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list project alerts"); errorResp != nil {
		return errorResp, nil
	}

	response := ProjectAlertsResponse{
		ProjectID: args.ProjectID,
		Alerts:    result,
		Success:   true,
		Message:   fmt.Sprintf("Loaded %d project alert entries for project %d", len(result), args.ProjectID),
	}
	if mode != nil {
		response.Mode = string(*mode)
	}
	return createJSONResponse(response), nil
}

func queryProjectLokiLogs(client *taikungoclient.Client, args QueryProjectLokiLogsArgs) (*mcp_golang.ToolResponse, error) {
	if _, errorResp := requireProjectMonitoringEnabled(client, args.ProjectID); errorResp != nil {
		return errorResp, nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 100
	}
	query, errorResp := buildLokiLogsQuery(args.ProjectID, args.Parameters, args.Filters, args.StartDate, args.EndDate, limit, args.Direction)
	if errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewProjectsLogsCommand()
	command.SetQuery(*query)

	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsLokiLogs(context.Background()).
		ProjectsLogsCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "query project loki logs"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(ProjectLokiLogsResponse{
		ProjectID: args.ProjectID,
		Results:   result,
		Success:   true,
		Message:   fmt.Sprintf("Loaded %d Loki log streams for project %d", len(result), args.ProjectID),
	}), nil
}

func exportProjectLokiLogs(client *taikungoclient.Client, args ExportProjectLokiLogsArgs) (*mcp_golang.ToolResponse, error) {
	if _, errorResp := requireProjectMonitoringEnabled(client, args.ProjectID); errorResp != nil {
		return errorResp, nil
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 1000
	}
	query, errorResp := buildLokiLogsQuery(args.ProjectID, args.Parameters, args.Filters, args.StartDate, args.EndDate, limit, args.Direction)
	if errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewExportLokiLogsCommand()
	command.SetQuery(*query)

	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsExportLokiLogs(context.Background()).
		ExportLokiLogsCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "export project loki logs"); errorResp != nil {
		return errorResp, nil
	}
	if result == nil {
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("No Loki log export was returned for project %d", args.ProjectID),
		}), nil
	}

	return createJSONResponse(ProjectLokiLogsExportResponse{
		ProjectID:   args.ProjectID,
		FileName:    result.GetFileName(),
		ContentType: result.GetContentType(),
		Content:     result.GetContent(),
		Success:     true,
		Message:     fmt.Sprintf("Exported Loki logs for project %d", args.ProjectID),
	}), nil
}

func queryProjectPrometheusMetrics(client *taikungoclient.Client, args QueryProjectPrometheusMetricsArgs) (*mcp_golang.ToolResponse, error) {
	if _, errorResp := requireProjectMonitoringEnabled(client, args.ProjectID); errorResp != nil {
		return errorResp, nil
	}
	if errorResp := validatePromQLExpression(args.Parameters); errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewPrometheusMetricsCommand()
	command.SetProjectId(args.ProjectID)
	command.SetParameters(encodeQueryParameters(args.Parameters))

	queryTime, errorResp := parseOptionalRFC3339(args.Time, "time")
	if errorResp != nil {
		return errorResp, nil
	}
	if queryTime != nil {
		command.SetTime(*queryTime)
	}

	startTime, errorResp := parseOptionalRFC3339(args.Start, "start")
	if errorResp != nil {
		return errorResp, nil
	}
	if startTime != nil {
		command.SetStart(*startTime)
	}

	endTime, errorResp := parseOptionalRFC3339(args.End, "end")
	if errorResp != nil {
		return errorResp, nil
	}
	if endTime != nil {
		command.SetEnd(*endTime)
	}

	if args.IsGraphEnabled != nil {
		command.SetIsGraphEnabled(*args.IsGraphEnabled)
	}
	if strings.TrimSpace(args.Step) != "" {
		command.SetStep(args.Step)
	}

	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsPrometheusMetrics(context.Background()).
		PrometheusMetricsCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "query project prometheus metrics"); errorResp != nil {
		return errorResp, nil
	}

	response := ProjectPrometheusMetricsResponse{
		ProjectID: args.ProjectID,
		Success:   true,
		Message:   fmt.Sprintf("Loaded Prometheus metrics for project %d", args.ProjectID),
	}
	if result != nil {
		response.Status = result.GetStatus()
		if data, ok := result.GetDataOk(); ok && data != nil {
			response.Data = data
		}
	}
	return createJSONResponse(response), nil
}

func autocompleteProjectPrometheusMetrics(client *taikungoclient.Client, args ProjectPrometheusMetricsAutocompleteArgs) (*mcp_golang.ToolResponse, error) {
	if _, errorResp := requireProjectMonitoringEnabled(client, args.ProjectID); errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewPrometheusMetricsAutocompleteCommand()
	command.SetProjectId(args.ProjectID)

	result, httpResponse, err := client.Client.ProjectsAPI.ProjectsPrometheusMetricsAutocomplete(context.Background()).
		PrometheusMetricsAutocompleteCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "autocomplete project prometheus metrics"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(ProjectPrometheusMetricsAutocompleteResponse{
		ProjectID:   args.ProjectID,
		Suggestions: result,
		Success:     true,
		Message:     fmt.Sprintf("Loaded %d Prometheus metric suggestions for project %d", len(result), args.ProjectID),
	}), nil
}
