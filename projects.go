package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

type ListProjectsArgs struct {
	Limit               int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset              int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search              string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
	HealthyOnly         bool   `json:"healthyOnly,omitempty" jsonschema:"description=Return only healthy projects (default: false)"`
	VirtualClustersOnly bool   `json:"virtualClustersOnly,omitempty" jsonschema:"description=Return only virtual cluster projects (default: false)"`
}

func emptyProjectListResponse(message string) *mcp_golang.ToolResponse {
	return createJSONResponse(ProjectListResponse{
		Projects: []ProjectSummary{},
		Total:    0,
		Message:  message,
	})
}

func listProjectsErrorResponse(httpResponse *http.Response, err error) *mcp_golang.ToolResponse {
	apiErr := apiErrorInfoFromResponse(httpResponse, err)
	if apiErr.isNotFound() {
		return emptyProjectListResponse("No projects found")
	}
	return apiErr.toolResponse()
}

func waitForProjectLookupErrorResponse(waitDeleted bool, projectID int32, httpResponse *http.Response, err error) *mcp_golang.ToolResponse {
	apiErr := apiErrorInfoFromResponse(httpResponse, err)
	if waitDeleted && apiErr.isNotFound() {
		return createJSONResponse(SuccessResponse{
			Message: fmt.Sprintf("Project %d has been successfully deleted", projectID),
			Success: true,
		})
	}
	return apiErr.toolResponse()
}

func deleteProjectErrorResponse(projectID int32, httpResponse *http.Response, err error) *mcp_golang.ToolResponse {
	apiErr := apiErrorInfoFromResponse(httpResponse, err)
	if apiErr.contains("You can not delete non empty project") {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Project %d cannot be deleted until all servers are removed", projectID),
			Details: apiErr.Message,
		})
	}
	return apiErr.toolResponse()
}

func listProjects(client *taikungoclient.Client, args ListProjectsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	const pageSize int32 = 100

	var filteredProjects []taikuncore.ProjectListDetailDto
	for pageOffset := int32(0); ; pageOffset += pageSize {
		req := client.Client.ProjectsAPI.ProjectsList(ctx).
			Limit(pageSize).
			Offset(pageOffset)

		if args.Search != "" {
			req = req.Search(args.Search)
		}
		if args.HealthyOnly {
			req = req.Healthy(true)
		}

		projectList, httpResponse, err := req.Execute()
		if err != nil {
			return listProjectsErrorResponse(httpResponse, err), nil
		}

		if errorResp := checkResponse(httpResponse, "list projects"); errorResp != nil {
			return errorResp, nil
		}

		if projectList == nil || len(projectList.Data) == 0 {
			break
		}

		for _, project := range projectList.Data {
			include := true

			if args.VirtualClustersOnly && !project.GetIsVirtualCluster() {
				include = false
			}

			if include {
				filteredProjects = append(filteredProjects, project)
			}
		}

		if int32(len(projectList.Data)) < pageSize || pageOffset+pageSize >= projectList.GetTotalCount() {
			break
		}
	}

	if len(filteredProjects) == 0 {
		return emptyProjectListResponse("No projects found matching the specified criteria"), nil
	}

	pagedProjects := applyOffsetLimit(filteredProjects, args.Offset, args.Limit)

	// Prepare the response data.
	var projects []ProjectSummary
	for _, project := range pagedProjects {
		projectSummary := ProjectSummary{
			ID:                     project.GetId(),
			Name:                   project.GetName(),
			Status:                 string(project.GetStatus()),
			Health:                 string(project.GetHealth()),
			Type:                   getProjectType(project),
			Cloud:                  string(project.GetCloudType()),
			Organization:           project.GetOrganizationName(),
			IsLocked:               project.GetIsLocked(),
			IsVirtualCluster:       project.GetIsVirtualCluster(),
			CreatedAt:              project.GetCreatedAt(),
			ServersCount:           project.GetTotalServersCount(),
			StandaloneVMsCount:     project.GetTotalStandaloneVmsCount(),
			HourlyCost:             project.GetTotalHourlyCost(),
			MonitoringEnabled:      project.GetIsMonitoringEnabled(),
			BackupEnabled:          project.GetIsBackupEnabled(),
			AlertsCount:            project.GetAlertsCount(),
			ReadyForVirtualCluster: isProjectReadyForVirtualCluster(project),
		}

		if project.GetIsVirtualCluster() && project.GetParentProjectId() > 0 {
			projectSummary.ParentProjectID = project.GetParentProjectId()
		}

		if !projectSummary.ReadyForVirtualCluster {
			projectSummary.VirtualClusterReason = getVirtualClusterReadinessReason(project)
		}

		projects = append(projects, projectSummary)
	}

	// Create response
	var filterType string
	var message string
	if args.VirtualClustersOnly {
		filterType = "virtual-clusters"
		message = fmt.Sprintf("Found %d virtual cluster projects", len(filteredProjects))
	} else {
		filterType = "all"
		message = fmt.Sprintf("Found %d projects", len(filteredProjects))
	}

	if len(projects) == 0 {
		message = fmt.Sprintf("No projects found on the requested page (total matches: %d)", len(filteredProjects))
	}

	response := ProjectListResponse{
		Projects:   projects,
		Total:      len(filteredProjects),
		FilterType: filterType,
		Message:    message,
	}

	return createJSONResponse(response), nil
}

func getProjectType(project taikuncore.ProjectListDetailDto) string {
	if project.GetIsVirtualCluster() {
		return "VirtualCluster"
	}
	return "Project"
}

func isProjectReadyForVirtualCluster(project taikuncore.ProjectListDetailDto) bool {
	return project.GetIsKubernetes() &&
		project.GetStatus() == taikuncore.PROJECTSTATUS_READY &&
		project.GetHealth() == taikuncore.PROJECTHEALTH_HEALTHY &&
		!project.GetIsLocked() &&
		!project.GetIsVirtualCluster()
}

func getVirtualClusterReadinessReason(project taikuncore.ProjectListDetailDto) string {
	if !project.GetIsKubernetes() {
		return "Not a Kubernetes project"
	}
	if project.GetStatus() != taikuncore.PROJECTSTATUS_READY {
		return fmt.Sprintf("Status is %s (must be Ready)", project.GetStatus())
	}
	if project.GetHealth() != taikuncore.PROJECTHEALTH_HEALTHY {
		return fmt.Sprintf("Health is %s (must be Healthy)", project.GetHealth())
	}
	if project.GetIsLocked() {
		return "Project is locked (read-only)"
	}
	if project.GetIsVirtualCluster() {
		return "Virtual clusters cannot host other virtual clusters"
	}
	return "Unknown reason"
}

func createProject(client *taikungoclient.Client, args CreateProjectArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	// Create the project command
	createCmd := taikuncore.NewCreateProjectCommand()
	createCmd.SetName(args.Name)
	createCmd.SetCloudCredentialId(args.CloudCredentialID)
	createCmd.SetIsKubernetes(true)

	if args.KubernetesProfileID != 0 {
		createCmd.SetKubernetesProfileId(args.KubernetesProfileID)
	}
	if args.KubernetesVersion != "" {
		createCmd.SetKubernetesVersion(args.KubernetesVersion)
	}
	if args.AlertingProfileID != 0 {
		createCmd.SetAlertingProfileId(args.AlertingProfileID)
	}

	createCmd.SetIsMonitoringEnabled(args.Monitoring)

	// Execute the API call
	projectResponse, httpResponse, err := client.Client.ProjectsAPI.ProjectsCreate(ctx).
		CreateProjectCommand(*createCmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "create project"); errorResp != nil {
		return errorResp, nil
	}

	// Prepare success response with project details
	type ProjectCreationResponse struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		CloudCredentialID int32  `json:"cloudCredentialId"`
		IsKubernetes      bool   `json:"isKubernetes"`
		MonitoringEnabled bool   `json:"monitoringEnabled"`
		Message           string `json:"message"`
		Success           bool   `json:"success"`
	}

	var projectID string
	if projectResponse != nil && projectResponse.Id.IsSet() && projectResponse.Id.Get() != nil {
		projectID = *projectResponse.Id.Get()
	}

	response := ProjectCreationResponse{
		ID:                projectID,
		Name:              args.Name,
		CloudCredentialID: args.CloudCredentialID,
		IsKubernetes:      true,
		MonitoringEnabled: args.Monitoring,
		Message:           fmt.Sprintf("Project '%s' created successfully with ID %s. This creates project metadata only; add cluster nodes and run commit-project, or use create-cluster for end-to-end provisioning.", args.Name, projectID),
		Success:           true,
	}

	return createJSONResponse(response), nil
}

func deleteProject(client *taikungoclient.Client, args DeleteProjectArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	// Create the delete command
	deleteCmd := taikuncore.NewDeleteProjectCommand()
	deleteCmd.SetProjectId(args.ProjectID)

	// Execute the API call to delete the project
	httpResponse, err := client.Client.ProjectsAPI.ProjectsDelete(ctx).
		DeleteProjectCommand(*deleteCmd).
		Execute()

	if err != nil {
		return deleteProjectErrorResponse(args.ProjectID, httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "delete project"); errorResp != nil {
		return errorResp, nil
	}

	// Prepare success response
	successResp := SuccessResponse{
		Message: fmt.Sprintf("Project ID %d deleted successfully", args.ProjectID),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func waitForProject(client *taikungoclient.Client, args WaitForProjectArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()
	timeout := 600 // Default 10 minutes for creation
	if args.WaitDeleted {
		timeout = 300 // Default 5 minutes for deletion
	}
	if args.Timeout > 0 {
		timeout = int(args.Timeout)
	}

	if args.WaitDeleted {
		logger.Printf("Waiting for project %d to be deleted (timeout: %d seconds)", args.ProjectId, timeout)
	} else {
		logger.Printf("Waiting for project %d to be ready (timeout: %d seconds)", args.ProjectId, timeout)
	}

	// Poll every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	timeoutChan := time.After(time.Duration(timeout) * time.Second)

	for {
		select {
		case <-timeoutChan:
			return createJSONResponse(ErrorResponse{
				Error: fmt.Sprintf("Timeout waiting for project %d after %d seconds", args.ProjectId, timeout),
			}), nil
		case <-ticker.C:
			// Check project status
			request := client.Client.ProjectsAPI.ProjectsList(ctx).Id(args.ProjectId)
			result, httpResponse, err := request.Execute()
			if err != nil {
				return waitForProjectLookupErrorResponse(args.WaitDeleted, args.ProjectId, httpResponse, err), nil
			}

			if errorResp := checkResponse(httpResponse, "check project status"); errorResp != nil {
				return errorResp, nil
			}

			if len(result.Data) == 0 {
				if args.WaitDeleted {
					return createJSONResponse(SuccessResponse{
						Message: fmt.Sprintf("Project %d has been successfully deleted", args.ProjectId),
						Success: true,
					}), nil
				}
				return createJSONResponse(ErrorResponse{
					Error: fmt.Sprintf("Project %d not found", args.ProjectId),
				}), nil
			}

			if args.WaitDeleted {
				project := result.Data[0]
				logger.Printf("Project %d still exists - Status: %s", args.ProjectId, project.GetStatus())
				continue
			}

			project := result.Data[0]
			status := project.GetStatus()
			health := project.GetHealth()

			logger.Printf("Project %d status: %s, health: %s", args.ProjectId, status, health)

			if status == taikuncore.PROJECTSTATUS_READY && health == taikuncore.PROJECTHEALTH_HEALTHY {
				return createJSONResponse(SuccessResponse{
					Message: fmt.Sprintf("Project %d is now ready and healthy. This confirms project state only; it does not imply Kubernetes nodes were added unless add-server-to-project/commit-project or create-cluster was used.", args.ProjectId),
					Success: true,
				}), nil
			}

			if status == taikuncore.PROJECTSTATUS_FAILURE || health == taikuncore.PROJECTHEALTH_UNHEALTHY {
				return createJSONResponse(ErrorResponse{
					Error: fmt.Sprintf("Project %d reached a failure state - Status: %s, Health: %s", args.ProjectId, status, health),
				}), nil
			}
		}
	}
}
