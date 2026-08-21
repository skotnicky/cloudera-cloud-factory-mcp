package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/tidwall/gjson"
)

const (
	defaultInstallAppTimeoutSeconds = 600
	defaultInstallAppTTLMinutes     = 10
	installAppTimeoutHint           = "Larger applications may need a longer install timeout; set timeout explicitly when installs routinely take more than 10 minutes."
)

type AppParameter struct {
	Key   string `json:"key" jsonschema:"required,description=Parameter key"`
	Value string `json:"value" jsonschema:"required,description=Parameter value"`
}

type InstallAppArgs struct {
	Name                      string         `json:"name" jsonschema:"required,description=The name of the application instance"`
	Namespace                 string         `json:"namespace" jsonschema:"required,description=The namespace to install the application in"`
	ProjectID                 int32          `json:"projectId" jsonschema:"required,description=The project ID to install the application in"`
	CatalogAppID              int32          `json:"catalogAppId" jsonschema:"required,description=The catalog application ID to install"`
	ExtraValues               string         `json:"extraValues,omitempty" jsonschema:"description=Base64-encoded YAML extra values for the application (optional)"`
	AutoSync                  bool           `json:"autoSync,omitempty" jsonschema:"description=Enable automatic synchronization (default: false)"`
	TaikunLinkEnabled         bool           `json:"taikunLinkEnabled,omitempty" jsonschema:"description=Enable Cloudera Cloud Factory (Taikun) link integration (default: false)"`
	Timeout                   int32          `json:"timeout,omitempty" jsonschema:"description=Installation timeout in minutes (optional). Larger applications may need a bigger timeout."`
	TTL                       int32          `json:"ttl,omitempty" jsonschema:"description=Application TTL in minutes (default: 10, valid range: 10-200)"`
	Parameters                []AppParameter `json:"parameters,omitempty" jsonschema:"description=Application parameters as key-value pairs (optional)"`
	UseCatalogDefaults        *bool          `json:"useCatalogDefaults,omitempty" jsonschema:"description=Use catalog default parameters as a base (default: true)"`
	WaitForReady              bool           `json:"waitForReady,omitempty" jsonschema:"description=Wait for application to be ready before returning (default: false)"`
	WaitTimeout               int32          `json:"waitTimeout,omitempty" jsonschema:"description=Wait timeout in seconds (default: 600)"`
	ReadyStabilizationSeconds int32          `json:"readyStabilizationSeconds,omitempty" jsonschema:"description=Seconds the app must remain in Ready state before success (default: 30)"`
	RetrySyncOnFailure        bool           `json:"retrySyncOnFailure,omitempty" jsonschema:"description=If wait detects Failed status, run one sync retry and wait again (default: false)"`
}

type ListAppsArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=The project ID to list applications from"`
	Limit     int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset    int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search    string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type GetAppArgs struct {
	ProjectAppID int32 `json:"projectAppId" jsonschema:"required,description=The project application ID to get details for"`
}

type UpdateSyncAppArgs struct {
	ProjectAppID int32  `json:"projectAppId" jsonschema:"required,description=The project application ID to update/sync"`
	ExtraValues  string `json:"extraValues,omitempty" jsonschema:"description=Base64-encoded YAML extra values (optional - if not provided, will only sync)"`
	Timeout      int32  `json:"timeout,omitempty" jsonschema:"description=Operation timeout in minutes (optional)"`
	SyncOnly     bool   `json:"syncOnly,omitempty" jsonschema:"description=If true, only sync without updating values (default: false)"`
}

type UninstallAppArgs struct {
	ProjectAppID int32 `json:"projectAppId" jsonschema:"required,description=The project application ID to uninstall"`
}

// waitForAppReady waits for an application to reach READY status or be deleted
func waitForAppReady(ctx context.Context, client *taikungoclient.Client, projectAppID int32, timeoutSeconds int32, waitDeleted bool, readyStabilizationSeconds int32) error {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeoutSeconds == 0 {
		timeout = 60 * time.Second // Default 60 seconds
		if waitDeleted {
			timeout = 30 * time.Second // Default 30 seconds
		}
	}

	readyStabilization := time.Duration(readyStabilizationSeconds) * time.Second
	if readyStabilizationSeconds <= 0 {
		readyStabilization = 30 * time.Second
	}

	var readySince time.Time
	start := time.Now()
	for {
		// Check if we've exceeded the timeout
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for application ID %d after %v", projectAppID, timeout)
		}

		status, found, response, err := fetchProjectAppStatus(ctx, client, projectAppID)
		if response != nil && (response.StatusCode < 200 || response.StatusCode >= 300) && response.StatusCode != http.StatusNotFound {
			return taikungoclient.CreateError(response, err)
		}
		if err != nil {
			return fmt.Errorf("error checking application status: %v", err)
		}

		if !found {
			if waitDeleted {
				return nil // App is gone, which is what we wanted
			}
			return fmt.Errorf("application ID %d not found in project", projectAppID)
		}

		if waitDeleted {
			logger.Printf("Application ID %d still exists - Status: %s", projectAppID, status)
		} else {
			logger.Printf("Application ID %d status: %s", projectAppID, status)

			// Mark ready only after a short stabilization window to avoid
			// transient "Ready" states where dependent resources fail moments later.
			if status == "Ready" {
				if readySince.IsZero() {
					readySince = time.Now()
					logger.Printf("Application ID %d reached Ready, starting stabilization window (%v)", projectAppID, readyStabilization)
				}
				if time.Since(readySince) >= readyStabilization {
					return nil // Success after stabilization
				}
			} else {
				readySince = time.Time{}
			}

			// Check for failure states
			if status == "Failed" {
				return fmt.Errorf("application ID %d installation failed - status: %s", projectAppID, status)
			}
		}

		// Wait before next poll, but stop promptly if the request is cancelled.
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait cancelled for application ID %d: %w", projectAppID, ctx.Err())
		case <-time.After(10 * time.Second):
		}
	}
}

func fetchProjectAppStatus(ctx context.Context, client *taikungoclient.Client, projectAppID int32) (string, bool, *http.Response, error) {
	appDetails, response, err := client.Client.ProjectAppsAPI.ProjectappDetails(ctx, projectAppID).Execute()
	if err == nil && appDetails != nil {
		return string(appDetails.GetStatus()), true, response, nil
	}

	if response != nil {
		if response.StatusCode == http.StatusNotFound {
			return "", false, response, nil
		}
		if response.StatusCode >= 300 {
			return "", false, response, err
		}
	}

	if response != nil && response.Body != nil {
		bodyBytes, readErr := io.ReadAll(response.Body)
		if readErr == nil {
			body := string(bodyBytes)
			if statusResult := gjson.Get(body, "status"); statusResult.Exists() {
				return statusResult.String(), true, response, nil
			}
			if statusResult := gjson.Get(body, "data.status"); statusResult.Exists() {
				return statusResult.String(), true, response, nil
			}
		}
	}

	return "", false, response, err
}

func syncProjectApp(ctx context.Context, client *taikungoclient.Client, projectAppID int32, timeout int32) error {
	syncCmd := taikuncore.NewSyncProjectAppCommand()
	syncCmd.SetProjectAppId(projectAppID)
	if timeout > 0 {
		syncCmd.SetTimeout(timeout)
	}

	response, err := client.Client.ProjectAppsAPI.ProjectappSync(ctx).
		SyncProjectAppCommand(*syncCmd).
		Execute()
	if err != nil {
		return taikungoclient.CreateError(response, err)
	}
	if response == nil {
		return fmt.Errorf("no response received while syncing application ID %d", projectAppID)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return taikungoclient.CreateError(response, fmt.Errorf("sync application returned status %d", response.StatusCode))
	}
	return nil
}

func resolveInstallAppTimeout(timeout int32) (int32, bool) {
	if timeout > 0 {
		return timeout, false
	}
	return defaultInstallAppTimeoutSeconds, true
}

func resolveInstallAppTTL(ttl int32) (int32, bool, string) {
	if ttl == 0 {
		return defaultInstallAppTTLMinutes, true, ""
	}
	if ttl < 10 || ttl > 200 {
		return 0, false, "ttl must be between 10 and 200 minutes"
	}
	return ttl, false, ""
}

func kubeAppProjectPrerequisiteError(projectID int32, status string, health string, hasKubeconfig bool, operation string) *ErrorResponse {
	if strings.TrimSpace(status) != "Ready" {
		return &ErrorResponse{
			Error:   fmt.Sprintf("Project %d is not ready for %s", projectID, operation),
			Details: fmt.Sprintf("Project status is %q. Wait for project Ready and platform kubeconfig availability before installing apps or binding catalogs.", status),
		}
	}
	normalizedHealth := strings.TrimSpace(health)
	if normalizedHealth != "Healthy" && normalizedHealth != "Warning" {
		return &ErrorResponse{
			Error:   fmt.Sprintf("Project %d health is not ready for %s", projectID, operation),
			Details: fmt.Sprintf("Project health is %q. Resolve cluster health before installing apps or binding catalogs.", health),
		}
	}
	if !hasKubeconfig {
		return &ErrorResponse{
			Error:   fmt.Sprintf("Project %d has no platform kubeconfig for %s", projectID, operation),
			Details: "Commit/provision the project first, then poll wait-for-project or preflight-project until kubeconfig is available.",
		}
	}
	return nil
}

func ensureProjectReadyForKubeApp(ctx context.Context, client *taikungoclient.Client, projectID int32, operation string) *mcp_golang.ToolResponse {
	serversResult, response, err := client.Client.ServersAPI.ServersDetails(ctx, projectID).Execute()
	if err != nil {
		return apiErrorInfoFromResponse(response, err).toolResponse()
	}
	if errorResp := checkResponse(response, operation+" preflight"); errorResp != nil {
		return errorResp
	}
	if serversResult == nil {
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("No project details returned for project %d", projectID),
		})
	}

	project := serversResult.GetProject()
	if prereq := kubeAppProjectPrerequisiteError(
		projectID,
		strings.TrimSpace(string(project.GetStatus())),
		strings.TrimSpace(string(project.GetHealth())),
		project.GetHasKubeConfigFile(),
		operation); prereq != nil {
		return createJSONResponse(*prereq)
	}

	return nil
}

func installApp(ctx context.Context, client *taikungoclient.Client, args InstallAppArgs) (*mcp_golang.ToolResponse, error) {
	installTimeout, timeoutDefaulted := resolveInstallAppTimeout(args.Timeout)
	installTTL, ttlDefaulted, ttlValidationError := resolveInstallAppTTL(args.TTL)
	if ttlValidationError != "" {
		return createJSONResponse(ErrorResponse{
			Error: ttlValidationError,
		}), nil
	}
	if errorResp := ensureProjectReadyForKubeApp(ctx, client, args.ProjectID, "app install"); errorResp != nil {
		return errorResp, nil
	}

	createCmd := taikuncore.NewCreateProjectAppCommand()
	createCmd.SetName(args.Name)
	createCmd.SetNamespace(args.Namespace)
	createCmd.SetProjectId(args.ProjectID)
	createCmd.SetCatalogAppId(args.CatalogAppID)
	createCmd.SetAutoSync(args.AutoSync)
	createCmd.SetTaikunLinkEnabled(args.TaikunLinkEnabled)
	createCmd.SetTimeout(installTimeout)
	if createCmd.AdditionalProperties == nil {
		createCmd.AdditionalProperties = map[string]interface{}{}
	}
	createCmd.AdditionalProperties["ttl"] = installTTL

	if args.ExtraValues != "" {
		createCmd.SetExtraValues(args.ExtraValues)
	}

	useCatalogDefaults := true
	if args.UseCatalogDefaults != nil {
		useCatalogDefaults = *args.UseCatalogDefaults
	}

	paramMap := map[string]string{}
	if useCatalogDefaults {
		defaults, response, err := client.Client.CatalogAppAPI.CatalogAppParamDetails(ctx, args.CatalogAppID).Execute()
		if err != nil {
			return createError(response, err), nil
		}
		if errorResp := checkResponse(response, "get catalog app default parameters"); errorResp != nil {
			return errorResp, nil
		}

		for _, param := range defaults {
			if key, ok := param.GetKeyOk(); ok && key != nil {
				if value, ok := param.GetValueOk(); ok && value != nil {
					paramMap[*key] = *value
				}
			}
		}
	}

	for _, param := range args.Parameters {
		paramMap[param.Key] = param.Value
	}

	if len(paramMap) > 0 {
		keys := make([]string, 0, len(paramMap))
		for key := range paramMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		params := make([]taikuncore.ProjectAppParamsDto, 0, len(keys))
		for _, key := range keys {
			p := taikuncore.NewProjectAppParamsDto()
			p.SetKey(key)
			p.SetValue(paramMap[key])
			params = append(params, *p)
		}
		createCmd.SetParameters(params)
	}

	response, httpResponse, err := client.Client.ProjectAppsAPI.ProjectappInstall(ctx).
		CreateProjectAppCommand(*createCmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "install application"); errorResp != nil {
		return errorResp, nil
	}

	// Get the application ID by searching for the newly created app
	var projectAppID int32
	if args.WaitForReady {
		// Find the newly created application by searching
		appList, _, err := client.Client.ProjectAppsAPI.ProjectappList(ctx).
			ProjectId(args.ProjectID).
			Search(args.Name).
			Execute()

		if err == nil && appList != nil && len(appList.Data) > 0 {
			// Find the app with matching name and namespace
			for _, app := range appList.Data {
				if app.GetName() == args.Name && app.GetNamespace() == args.Namespace {
					projectAppID = app.GetId()
					logger.Printf("Found application '%s' with ID: %d", args.Name, projectAppID)
					break
				}
			}
		}
	}

	var resultMsg string
	if args.WaitForReady && projectAppID > 0 {
		logger.Printf("Waiting for application '%s' (ID: %d) to be ready...", args.Name, projectAppID)
		waitTimeout := args.WaitTimeout
		if waitTimeout == 0 {
			waitTimeout = 60 // Default 60 seconds
		}

		err := waitForAppReady(ctx, client, projectAppID, waitTimeout, false, args.ReadyStabilizationSeconds)
		if err != nil {
			if args.RetrySyncOnFailure {
				details, detailsResponse, detailsErr := client.Client.ProjectAppsAPI.ProjectappDetails(ctx, projectAppID).Execute()
				if detailsErr == nil && details != nil && string(details.GetStatus()) == "Failed" {
					logger.Printf("Application '%s' (ID: %d) failed, attempting one sync retry", args.Name, projectAppID)
					syncErr := syncProjectApp(ctx, client, projectAppID, waitTimeout)
					if syncErr == nil {
						retryWaitErr := waitForAppReady(ctx, client, projectAppID, waitTimeout, false, args.ReadyStabilizationSeconds)
						if retryWaitErr == nil {
							resultMsg = fmt.Sprintf("Application '%s' (ID: %d) installed successfully after sync retry in namespace '%s'", args.Name, projectAppID, args.Namespace)
							logger.Printf("Application '%s' (ID: %d) is now ready after sync retry", args.Name, projectAppID)
						} else {
							errorResp := ErrorResponse{
								Error: fmt.Sprintf("Application '%s' failed initial wait, sync retry executed, but app is still not ready: %v", args.Name, retryWaitErr),
							}
							return createJSONResponse(errorResp), nil
						}
					} else {
						errorResp := ErrorResponse{
							Error: fmt.Sprintf("Application '%s' failed initial wait and sync retry failed: %v", args.Name, syncErr),
						}
						return createJSONResponse(errorResp), nil
					}
				} else {
					if detailsErr != nil {
						logger.Printf("Failed to inspect app details for retry decision (projectAppId %d): %v", projectAppID, detailsErr)
						if detailsResponse != nil && (detailsResponse.StatusCode < 200 || detailsResponse.StatusCode >= 300) {
							errorResp := ErrorResponse{
								Error: fmt.Sprintf("Application '%s' installation failed during wait and details lookup failed: %v", args.Name, taikungoclient.CreateError(detailsResponse, detailsErr)),
							}
							return createJSONResponse(errorResp), nil
						}
					}
					errorResp := ErrorResponse{
						Error: fmt.Sprintf("Application '%s' installation initiated but failed during wait: %v", args.Name, err),
					}
					return createJSONResponse(errorResp), nil
				}
			} else {
				errorResp := ErrorResponse{
					Error: fmt.Sprintf("Application '%s' installation initiated but failed during wait: %v", args.Name, err),
				}
				return createJSONResponse(errorResp), nil
			}
		}
		if resultMsg == "" {
			resultMsg = fmt.Sprintf("Application '%s' (ID: %d) installed successfully and is ready in namespace '%s'", args.Name, projectAppID, args.Namespace)
			logger.Printf("Application '%s' (ID: %d) is now ready", args.Name, projectAppID)
		}
	} else {
		if response != nil && response.GetMessage() != "" {
			resultMsg = fmt.Sprintf("Application '%s' installation initiated successfully. Message: %s", args.Name, response.GetMessage())
		} else {
			resultMsg = fmt.Sprintf("Application '%s' installation initiated successfully in namespace '%s'", args.Name, args.Namespace)
		}
		if projectAppID > 0 {
			resultMsg += fmt.Sprintf(" (ID: %d)", projectAppID)
		}
	}

	type InstallAppResponse struct {
		Message            string         `json:"message"`
		Success            bool           `json:"success"`
		ProjectAppID       int32          `json:"projectAppId,omitempty"`
		Name               string         `json:"name"`
		Namespace          string         `json:"namespace"`
		Status             string         `json:"status"`
		TimeoutSeconds     int32          `json:"timeoutSeconds"`
		TimeoutDefaulted   bool           `json:"timeoutDefaulted"`
		TTLMinutes         int32          `json:"ttlMinutes"`
		TTLDefaulted       bool           `json:"ttlDefaulted"`
		Hint               string         `json:"hint,omitempty"`
		UseCatalogDefaults bool           `json:"useCatalogDefaults"`
		ParametersApplied  []AppParameter `json:"parametersApplied,omitempty"`
	}

	var appliedParams []AppParameter
	if len(paramMap) > 0 {
		keys := make([]string, 0, len(paramMap))
		for key := range paramMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			appliedParams = append(appliedParams, AppParameter{
				Key:   key,
				Value: paramMap[key],
			})
		}
	}

	responseData := InstallAppResponse{
		Message:            resultMsg,
		Success:            true,
		ProjectAppID:       projectAppID,
		Name:               args.Name,
		Namespace:          args.Namespace,
		Status:             "initiated",
		TimeoutSeconds:     installTimeout,
		TimeoutDefaulted:   timeoutDefaulted,
		TTLMinutes:         installTTL,
		TTLDefaulted:       ttlDefaulted,
		UseCatalogDefaults: useCatalogDefaults,
		ParametersApplied:  appliedParams,
	}

	if args.WaitForReady {
		responseData.Status = "ready"
	}
	if timeoutDefaulted {
		responseData.Hint = installAppTimeoutHint
	}

	return createJSONResponse(responseData), nil
}

func listApps(client *taikungoclient.Client, args ListAppsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	req := client.Client.ProjectAppsAPI.ProjectappList(ctx).ProjectId(args.ProjectID)

	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}

	appList, httpResponse, err := req.Execute()

	// Prepare response data
	type AppSummary struct {
		ID         int32  `json:"id"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Status     string `json:"status"`
		Version    string `json:"version"`
		CatalogApp string `json:"catalogApp"`
		AutoSync   bool   `json:"autoSync"`
		Created    string `json:"created,omitempty"`
		CreatedBy  string `json:"createdBy,omitempty"`
	}

	var applications []AppSummary

	if err != nil {
		if httpResponse == nil || httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
			return createError(httpResponse, err), nil
		}

		// If unmarshaling failed, try to parse the raw body
		if httpResponse != nil && httpResponse.Body != nil {
			bodyBytes, readErr := readResponseBodyPreservingBody(httpResponse)
			if readErr == nil {
				body := string(bodyBytes)
				// Use gjson to extract applications
				data := gjson.Get(body, "data")
				if data.IsArray() {
					for _, app := range data.Array() {
						applications = append(applications, AppSummary{
							ID:         int32(app.Get("id").Int()),
							Name:       app.Get("name").String(),
							Namespace:  app.Get("namespace").String(),
							Status:     app.Get("status").String(),
							Version:    app.Get("version").String(),
							CatalogApp: app.Get("catalogAppName").String(),
							AutoSync:   app.Get("autoSync").Bool(),
							Created:    app.Get("created").String(),
							CreatedBy:  app.Get("createdBy").String(),
						})
					}
				}
			}
		}
		if len(applications) == 0 {
			return createError(httpResponse, err), nil
		}
	} else {
		if errorResp := checkResponse(httpResponse, "list applications"); errorResp != nil {
			return errorResp, nil
		}

		if appList != nil && len(appList.Data) > 0 {
			for _, app := range appList.Data {
				appSummary := AppSummary{
					ID:         app.GetId(),
					Name:       app.GetName(),
					Namespace:  app.GetNamespace(),
					Status:     string(app.GetStatus()),
					Version:    app.GetVersion(),
					CatalogApp: app.GetCatalogAppName(),
					AutoSync:   app.GetAutoSync(),
				}

				if app.GetCreated() != "" {
					appSummary.Created = app.GetCreated()
				}
				if app.GetCreatedBy() != "" {
					appSummary.CreatedBy = app.GetCreatedBy()
				}

				applications = append(applications, appSummary)
			}
		}
	}

	message := fmt.Sprintf("Found %d applications in project %d", len(applications), args.ProjectID)
	if len(applications) == 0 {
		message = fmt.Sprintf("No applications found in project %d", args.ProjectID)
	}

	listResp := struct {
		Applications []AppSummary `json:"applications"`
		Total        int          `json:"total"`
		ProjectID    int32        `json:"projectId"`
		Message      string       `json:"message"`
	}{
		Applications: applications,
		Total:        len(applications),
		ProjectID:    args.ProjectID,
		Message:      message,
	}

	return createJSONResponse(listResp), nil
}

func getApp(client *taikungoclient.Client, args GetAppArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	appDetails, httpResponse, err := client.Client.ProjectAppsAPI.ProjectappDetails(ctx, args.ProjectAppID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "get application details"); errorResp != nil {
		return errorResp, nil
	}

	if appDetails == nil {
		errorResp := ErrorResponse{
			Error: fmt.Sprintf("Application with ID %d not found", args.ProjectAppID),
		}
		return createJSONResponse(errorResp), nil
	}

	// Prepare response data
	type AppParameter struct {
		Key                         string `json:"key"`
		Value                       string `json:"value"`
		IsMandatory                 bool   `json:"isMandatory"`
		IsEditableAfterInstallation bool   `json:"isEditableAfterInstallation"`
	}

	type AppDetails struct {
		ID                int32          `json:"id"`
		Name              string         `json:"name"`
		Namespace         string         `json:"namespace"`
		Status            string         `json:"status"`
		Version           string         `json:"version"`
		ProjectName       string         `json:"projectName"`
		ProjectID         int32          `json:"projectId"`
		CatalogName       string         `json:"catalogName"`
		CatalogID         int32          `json:"catalogId"`
		CatalogAppName    string         `json:"catalogAppName"`
		CatalogAppID      int32          `json:"catalogAppId"`
		PackageID         string         `json:"packageId"`
		AutoSync          bool           `json:"autoSync"`
		HasJsonSchema     bool           `json:"hasJsonSchema"`
		TaikunLinkEnabled bool           `json:"taikunLinkEnabled"`
		TaikunLinkUrl     string         `json:"taikunLinkUrl,omitempty"`
		Logo              string         `json:"logo,omitempty"`
		Parameters        []AppParameter `json:"parameters,omitempty"`
		Values            string         `json:"values,omitempty"`
		TTL               int32          `json:"ttl"`
	}

	appDetail := AppDetails{
		ID:                args.ProjectAppID,
		Name:              appDetails.GetName(),
		Namespace:         appDetails.GetNamespace(),
		Status:            string(appDetails.GetStatus()),
		Version:           appDetails.GetVersion(),
		ProjectName:       appDetails.GetProjectName(),
		ProjectID:         appDetails.GetProjectId(),
		CatalogName:       appDetails.GetCatalogName(),
		CatalogID:         appDetails.GetCatalogId(),
		CatalogAppName:    appDetails.GetCatalogAppName(),
		CatalogAppID:      appDetails.GetCatalogAppId(),
		PackageID:         appDetails.GetPackageId(),
		AutoSync:          appDetails.GetAutoSync(),
		HasJsonSchema:     appDetails.GetHasJsonSchema(),
		TaikunLinkEnabled: appDetails.GetTaikunLinkEnabled(),
	}

	if appDetails.GetTaikunLinkUrl() != "" {
		appDetail.TaikunLinkUrl = appDetails.GetTaikunLinkUrl()
	}

	if appDetails.GetLogo() != "" {
		appDetail.Logo = appDetails.GetLogo()
	}

	if len(appDetails.ProjectAppParams) > 0 {
		for _, param := range appDetails.ProjectAppParams {
			appDetail.Parameters = append(appDetail.Parameters, AppParameter{
				Key:                         param.GetKey(),
				Value:                       param.GetValue(),
				IsMandatory:                 param.GetIsMandatory(),
				IsEditableAfterInstallation: param.GetIsEditableAfterInstallation(),
			})
		}
	}

	if appDetails.GetValues() != "" {
		appDetail.Values = appDetails.GetValues()
	}

	// helmResult, logs and releaseNotes are large/opaque (releaseNotes is
	// base64) and are intentionally omitted here to keep the response compact.

	appDetail.TTL = appDetails.GetTtl()

	return createJSONResponse(appDetail), nil
}

func updateSyncApp(ctx context.Context, client *taikungoclient.Client, args UpdateSyncAppArgs) (*mcp_golang.ToolResponse, error) {
	var resultMsg string

	// First, get the app details to check autosync status
	appDetails, httpResponse, err := client.Client.ProjectAppsAPI.ProjectappDetails(ctx, args.ProjectAppID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "get application details"); errorResp != nil {
		return errorResp, nil
	}

	if appDetails == nil {
		errorResp := ErrorResponse{
			Error: fmt.Sprintf("Application with ID %d not found", args.ProjectAppID),
		}
		return createJSONResponse(errorResp), nil
	}

	hasAutoSync := appDetails.GetAutoSync()
	logger.Printf("Application ID %d has autosync enabled: %t", args.ProjectAppID, hasAutoSync)

	if !args.SyncOnly && args.ExtraValues != "" {
		// Update the values
		updateCmd := taikuncore.NewEditProjectAppExtraValuesCommand()
		updateCmd.SetProjectAppId(args.ProjectAppID)
		updateCmd.SetExtraValues(args.ExtraValues)

		if args.Timeout > 0 {
			updateCmd.SetTimeout(args.Timeout)
		}

		_, httpResponse, err := client.Client.ProjectAppsAPI.ProjectappUpdateExtraValues(ctx).
			EditProjectAppExtraValuesCommand(*updateCmd).
			Execute()

		if err != nil {
			return createError(httpResponse, err), nil
		}

		if errorResp := checkResponse(httpResponse, "update application values"); errorResp != nil {
			return errorResp, nil
		}

		if hasAutoSync {
			// App has autosync enabled, no need to manually sync
			resultMsg = fmt.Sprintf("Application ID %d values updated successfully (autosync enabled)", args.ProjectAppID)
		} else {
			// App doesn't have autosync, need to manually sync
			syncCmd := taikuncore.NewSyncProjectAppCommand()
			syncCmd.SetProjectAppId(args.ProjectAppID)

			if args.Timeout > 0 {
				syncCmd.SetTimeout(args.Timeout)
			}

			httpResponse, err := client.Client.ProjectAppsAPI.ProjectappSync(ctx).
				SyncProjectAppCommand(*syncCmd).
				Execute()

			if err != nil {
				return createError(httpResponse, err), nil
			}

			if errorResp := checkResponse(httpResponse, "sync application"); errorResp != nil {
				return errorResp, nil
			}

			resultMsg = fmt.Sprintf("Application ID %d values updated and synced successfully", args.ProjectAppID)
		}
	} else {
		// Only sync (no value update)
		if hasAutoSync {
			resultMsg = fmt.Sprintf("Application ID %d has autosync enabled, no manual sync needed", args.ProjectAppID)
		} else {
			syncCmd := taikuncore.NewSyncProjectAppCommand()
			syncCmd.SetProjectAppId(args.ProjectAppID)

			if args.Timeout > 0 {
				syncCmd.SetTimeout(args.Timeout)
			}

			httpResponse, err := client.Client.ProjectAppsAPI.ProjectappSync(ctx).
				SyncProjectAppCommand(*syncCmd).
				Execute()

			if err != nil {
				return createError(httpResponse, err), nil
			}

			if errorResp := checkResponse(httpResponse, "sync application"); errorResp != nil {
				return errorResp, nil
			}

			resultMsg = fmt.Sprintf("Application ID %d synced successfully", args.ProjectAppID)
		}
	}

	successResp := SuccessResponse{
		Message: resultMsg,
		Success: true,
	}
	return createJSONResponse(successResp), nil
}

func uninstallApp(client *taikungoclient.Client, args UninstallAppArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	uninstallResponse, httpResponse, err := client.Client.ProjectAppsAPI.ProjectappDelete(ctx, args.ProjectAppID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "uninstall application"); errorResp != nil {
		return errorResp, nil
	}

	var resultMsg string
	if uninstallResponse != nil && uninstallResponse.GetIsJobSkipped() {
		resultMsg = fmt.Sprintf("Application ID %d uninstall skipped (already uninstalled or not found)", args.ProjectAppID)
	} else {
		resultMsg = fmt.Sprintf("Application ID %d uninstall initiated successfully", args.ProjectAppID)
	}
	successResp := SuccessResponse{
		Message: resultMsg,
		Success: true,
	}
	return createJSONResponse(successResp), nil
}

func waitForApp(ctx context.Context, client *taikungoclient.Client, args WaitForAppArgs) (*mcp_golang.ToolResponse, error) {
	timeout := args.Timeout
	if timeout == 0 {
		timeout = 60 // Default 60s for creation
		if args.WaitDeleted {
			timeout = 30 // Default 30s for deletion
		}
	}
	cappedTimeout, capped := effectiveWaitTimeout(int(timeout))
	timeout = int32(cappedTimeout)

	if args.WaitDeleted {
		logger.Printf("Waiting for application ID %d to be deleted (timeout: %d seconds)", args.ProjectAppId, timeout)
	} else {
		logger.Printf("Waiting for application ID %d to be ready (timeout: %d seconds)", args.ProjectAppId, timeout)
	}

	err := waitForAppReady(ctx, client, args.ProjectAppId, timeout, args.WaitDeleted, args.ReadyStabilizationSeconds)
	if err != nil {
		if capped && strings.Contains(err.Error(), "timeout waiting for application") {
			return createJSONResponse(map[string]interface{}{
				"success":      false,
				"status":       "pending",
				"projectAppId": args.ProjectAppId,
				"message":      fmt.Sprintf("Application %d is not ready yet after %d seconds. HTTP waits are capped at %d seconds to avoid long-held requests; call wait-for-app again or poll get-app.", args.ProjectAppId, timeout, maxHTTPWaitSeconds),
			}), nil
		}
		return createJSONResponse(ErrorResponse{
			Error: err.Error(),
		}), nil
	}

	message := fmt.Sprintf("Application ID %d is now ready", args.ProjectAppId)
	if args.WaitDeleted {
		message = fmt.Sprintf("Application ID %d has been successfully deleted", args.ProjectAppId)
	}

	return createJSONResponse(SuccessResponse{
		Message: message,
		Success: true,
	}), nil
}

type UpdateAppAutoSyncArgs struct {
	ProjectAppID int32  `json:"projectAppId" jsonschema:"required,description=The project application ID to update autosync for"`
	Mode         string `json:"mode,omitempty" jsonschema:"description=The TTL mode (e.g., 'Seconds', 'Minutes', 'Hours')"`
	TTL          int32  `json:"ttl,omitempty" jsonschema:"description=The TTL value"`
}

func updateAppAutoSync(client *taikungoclient.Client, args UpdateAppAutoSyncArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	cmd := taikuncore.NewAutoSyncManagementCommand()
	cmd.SetId(args.ProjectAppID)
	if args.Mode != "" {
		cmd.SetMode(args.Mode)
	}
	if args.TTL > 0 {
		cmd.SetTtl(args.TTL)
	}

	httpResponse, err := client.Client.ProjectAppsAPI.ProjectappAutosync(ctx).
		AutoSyncManagementCommand(*cmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "update application autosync"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Application ID %d autosync configuration updated successfully", args.ProjectAppID),
		Success: true,
	}
	return createJSONResponse(successResp), nil
}
