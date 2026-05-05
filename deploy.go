package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

var (
	projectServerAddLocks   = map[int32]*sync.Mutex{}
	projectServerAddLocksMu sync.Mutex

	projectPendingCommitChanges   = map[int32]pendingProjectCommitChanges{}
	projectPendingCommitChangesMu sync.Mutex
)

type pendingProjectCommitChanges struct {
	standaloneVMCreates int
	serverAdds          int
}

type projectCommitMode string

const (
	projectCommitModeAuto    projectCommitMode = "auto"
	projectCommitModeProject projectCommitMode = "project"
	projectCommitModeVM      projectCommitMode = "vm"

	minCommitSizingCPU int32   = 4
	minCommitSizingRAM float64 = 4
)

func getProjectServerAddLock(projectId int32) *sync.Mutex {
	projectServerAddLocksMu.Lock()
	defer projectServerAddLocksMu.Unlock()

	lock, ok := projectServerAddLocks[projectId]
	if !ok {
		lock = &sync.Mutex{}
		projectServerAddLocks[projectId] = lock
	}
	return lock
}

func recordPendingStandaloneVMCreate(projectID int32) {
	if projectID <= 0 {
		return
	}

	projectPendingCommitChangesMu.Lock()
	defer projectPendingCommitChangesMu.Unlock()

	state := projectPendingCommitChanges[projectID]
	state.standaloneVMCreates++
	projectPendingCommitChanges[projectID] = state
}

func recordPendingServerAdd(projectID int32) {
	if projectID <= 0 {
		return
	}

	projectPendingCommitChangesMu.Lock()
	defer projectPendingCommitChangesMu.Unlock()

	state := projectPendingCommitChanges[projectID]
	state.serverAdds++
	projectPendingCommitChanges[projectID] = state
}

func clearPendingProjectCommitChanges(projectID int32) {
	projectPendingCommitChangesMu.Lock()
	defer projectPendingCommitChangesMu.Unlock()

	delete(projectPendingCommitChanges, projectID)
}

func pendingProjectCommitMode(projectID int32) projectCommitMode {
	if projectID <= 0 {
		return projectCommitModeAuto
	}

	projectPendingCommitChangesMu.Lock()
	defer projectPendingCommitChangesMu.Unlock()

	state, ok := projectPendingCommitChanges[projectID]
	if !ok {
		return projectCommitModeAuto
	}
	if state.serverAdds > 0 {
		return projectCommitModeProject
	}
	if state.standaloneVMCreates > 0 {
		return projectCommitModeVM
	}
	return projectCommitModeAuto
}

func executeProjectCommitMode(projectID int32, mode projectCommitMode, projectCommit func() (projectCommitResult, *apiErrorInfo), fallbackCommit func() (projectCommitResult, *apiErrorInfo), vmCommit func() (projectCommitResult, *apiErrorInfo)) (projectCommitResult, *apiErrorInfo) {
	var (
		result    projectCommitResult
		errorInfo *apiErrorInfo
	)

	switch mode {
	case projectCommitModeVM:
		result, errorInfo = vmCommit()
	case projectCommitModeProject:
		result, errorInfo = projectCommit()
	default:
		result, errorInfo = fallbackCommit()
	}

	if errorInfo == nil {
		clearPendingProjectCommitChanges(projectID)
	}

	return result, errorInfo
}

func bindFlavorsToProject(client *taikungoclient.Client, args BindFlavorsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	command := taikuncore.NewBindFlavorToProjectCommand()
	command.SetProjectId(args.ProjectId)
	command.SetFlavors(args.Flavors)

	request := client.Client.FlavorsAPI.FlavorsBindToProject(ctx).
		BindFlavorToProjectCommand(*command)

	httpResponse, err := request.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "bind flavors to project"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]string{
		"message": fmt.Sprintf("Successfully bound %d flavors to project %d", len(args.Flavors), args.ProjectId),
	}), nil
}

func addServerToProject(client *taikungoclient.Client, args AddServerArgs) (*mcp_golang.ToolResponse, error) {
	lock := getProjectServerAddLock(args.ProjectId)
	lock.Lock()
	defer lock.Unlock()

	ctx := context.Background()

	serverDto := taikuncore.NewServerForCreateDto()
	serverDto.SetName(args.Name)

	role, err := taikuncore.NewCloudRoleFromValue(args.Role)
	if err != nil {
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("Invalid role: %v", err),
		}), nil
	}
	serverDto.SetRole(*role)

	serverDto.SetProjectId(args.ProjectId)
	serverDto.SetFlavor(args.Flavor)

	if args.DiskSize > 0 {
		serverDto.SetDiskSize(args.DiskSize * 1024 * 1024 * 1024)
	}

	count := args.Count
	if count <= 0 {
		count = 1
	}
	serverDto.SetCount(count)

	request := client.Client.ServersAPI.ServersCreate(ctx).
		ServerForCreateDto(*serverDto)

	_, httpResponse, err := request.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "add server to project"); errorResp != nil {
		return errorResp, nil
	}

	type AddServerResponse struct {
		Message  string          `json:"message"`
		Success  bool            `json:"success"`
		Verified bool            `json:"verified"`
		Expected int32           `json:"expected"`
		Found    int32           `json:"found"`
		Servers  []ServerSummary `json:"servers,omitempty"`
	}

	verifyTimeout := args.VerifyTimeoutSeconds
	if verifyTimeout <= 0 {
		verifyTimeout = 300
	}
	verifyDeadline := time.Now().Add(time.Duration(verifyTimeout) * time.Second)
	var matched []ServerSummary
	for {
		serversResp, listHTTPResponse, listErr := client.Client.ServersAPI.ServersDetails(ctx, args.ProjectId).Execute()
		if listErr != nil {
			return createError(listHTTPResponse, listErr), nil
		}
		if listHTTPResponse == nil {
			return createJSONResponse(ErrorResponse{
				Error: "Failed to verify server creation: no response received",
			}), nil
		}
		if listHTTPResponse.StatusCode < 200 || listHTTPResponse.StatusCode >= 300 {
			return createError(listHTTPResponse, fmt.Errorf("failed to verify server creation")), nil
		}

		matched = matched[:0]
		if serversResp != nil {
			for _, server := range serversResp.Data {
				serverName := server.GetName()
				if args.Name != "" {
					if count == 1 && serverName != args.Name {
						continue
					}
					if count > 1 && !strings.HasPrefix(serverName, args.Name) {
						continue
					}
				}
				if args.Role != "" && string(server.GetRole()) != args.Role {
					continue
				}
				if args.Flavor != "" && server.GetFlavor() != args.Flavor {
					continue
				}

				matched = append(matched, ServerSummary{
					ID:        server.GetId(),
					Name:      serverName,
					Role:      string(server.GetRole()),
					Status:    server.GetStatus(),
					IPAddress: server.GetIpAddress(),
					Flavor:    server.GetFlavor(),
				})
			}
		}

		if int32(len(matched)) >= count {
			recordPendingServerAdd(args.ProjectId)
			return createJSONResponse(AddServerResponse{
				Message:  fmt.Sprintf("Successfully added %d server(s) of type %s with flavor %s to project %d", count, args.Role, args.Flavor, args.ProjectId),
				Success:  true,
				Verified: true,
				Expected: count,
				Found:    int32(len(matched)),
				Servers:  matched,
			}), nil
		}

		if time.Now().After(verifyDeadline) {
			recordPendingServerAdd(args.ProjectId)
			return createJSONResponse(AddServerResponse{
				Message:  fmt.Sprintf("Server creation request accepted but not verified within timeout (expected %d)", count),
				Success:  false,
				Verified: false,
				Expected: count,
				Found:    int32(len(matched)),
				Servers:  matched,
			}), nil
		}

		time.Sleep(5 * time.Second)
	}
}

func commitProject(client *taikungoclient.Client, args CommitProjectArgs) (*mcp_golang.ToolResponse, error) {
	result, errorInfo := commitProjectWithFallback(client, args.ProjectId)
	if errorInfo != nil {
		return errorInfo.toolResponse(), nil
	}

	return createJSONResponse(map[string]interface{}{
		"message":    result.Message,
		"success":    true,
		"commitMode": result.Mode,
	}), nil
}

type projectCommitResult struct {
	Mode    string
	Message string
}

func commitProjectWithFallback(client *taikungoclient.Client, projectID int32) (projectCommitResult, *apiErrorInfo) {
	mode := pendingProjectCommitMode(projectID)
	if mode != projectCommitModeVM {
		if errorInfo := validateProjectSizingForCommit(client, projectID); errorInfo != nil {
			return projectCommitResult{}, errorInfo
		}
	}

	return executeProjectCommitMode(
		projectID,
		mode,
		func() (projectCommitResult, *apiErrorInfo) {
			return commitProjectWithReactiveFallback(client, projectID)
		},
		func() (projectCommitResult, *apiErrorInfo) {
			return commitProjectWithReactiveFallback(client, projectID)
		},
		func() (projectCommitResult, *apiErrorInfo) {
			return commitProjectVMChanges(client, projectID)
		},
	)
}

func validateProjectSizingForCommit(client *taikungoclient.Client, projectID int32) *apiErrorInfo {
	ctx := context.Background()

	serversResult, serversResponse, err := client.Client.ServersAPI.ServersDetails(ctx, projectID).Execute()
	if err != nil {
		errorInfo := apiErrorInfoFromResponse(serversResponse, err)
		return &errorInfo
	}
	if serversResponse == nil || serversResponse.StatusCode < http.StatusOK || serversResponse.StatusCode >= http.StatusMultipleChoices {
		errorInfo := apiErrorInfoFromResponse(serversResponse, fmt.Errorf("failed to load project %d servers for commit sizing validation", projectID))
		return &errorInfo
	}
	if serversResult == nil {
		return &apiErrorInfo{
			Message: fmt.Sprintf("Unable to validate project %d sizing because no server details were returned", projectID),
		}
	}

	projectDetails := serversResult.GetProject()
	cloudID := projectDetails.GetCloudId()
	if cloudID <= 0 {
		return &apiErrorInfo{
			Message: fmt.Sprintf("Unable to validate project %d sizing because cloud metadata is missing", projectID),
		}
	}

	workerCount := 0
	qualifyingWorkerCount := 0
	for _, server := range serversResult.GetData() {
		role := strings.ToLower(strings.TrimSpace(string(server.GetRole())))
		if role != "kubemaster" && role != "kubeworker" {
			continue
		}

		flavorName := strings.TrimSpace(server.GetFlavor())
		normalizedFlavorName := strings.ToLower(flavorName)

		flavorsResult, flavorsResponse, err := client.Client.CloudCredentialAPI.CloudcredentialsAllFlavors(ctx, cloudID).Search(flavorName).Execute()
		if err != nil {
			errorInfo := apiErrorInfoFromResponse(flavorsResponse, err)
			return &errorInfo
		}
		if flavorsResponse == nil || flavorsResponse.StatusCode < http.StatusOK || flavorsResponse.StatusCode >= http.StatusMultipleChoices {
			errorInfo := apiErrorInfoFromResponse(flavorsResponse, fmt.Errorf("failed to load flavors for cloud credential %d while validating project %d", cloudID, projectID))
			return &errorInfo
		}

		var flavor FlavorSummary
		found := false
		if flavorsResult != nil {
			for _, f := range flavorsResult.GetData() {
				if strings.ToLower(strings.TrimSpace(f.GetName())) == normalizedFlavorName {
					flavor = FlavorSummary{
						Name: f.GetName(),
						CPU:  f.GetCpu(),
						RAM:  f.GetRam(),
					}
					found = true
					break
				}
			}
		}
		if !found {
			return &apiErrorInfo{
				Message: fmt.Sprintf("Cannot validate sizing for server %q (%s) because flavor %q is missing from cloud credential %d metadata", server.GetName(), server.GetRole(), flavorName, cloudID),
			}
		}

		if role == "kubemaster" && !flavorMeetsMinimum(flavor, minCommitSizingCPU, minCommitSizingRAM) {
			return &apiErrorInfo{
				Message: fmt.Sprintf("commit-project requires every Kubemaster to use at least %d CPU / %.0f GB RAM; server %q uses flavor %q (%d CPU / %.1f GB RAM)", minCommitSizingCPU, minCommitSizingRAM, server.GetName(), flavor.Name, flavor.CPU, flavor.RAM),
			}
		}

		if role == "kubeworker" {
			workerCount++
			if flavorMeetsMinimum(flavor, minCommitSizingCPU, minCommitSizingRAM) {
				qualifyingWorkerCount++
			}
		}
	}

	if projectDetails.GetIsMonitoringEnabled() && qualifyingWorkerCount == 0 {
		if workerCount == 0 {
			return &apiErrorInfo{
				Message: fmt.Sprintf("commit-project requires at least one Kubeworker with %d CPU / %.0f GB RAM when monitoring is enabled for project %d", minCommitSizingCPU, minCommitSizingRAM, projectID),
			}
		}
		return &apiErrorInfo{
			Message: fmt.Sprintf("commit-project requires at least one Kubeworker with %d CPU / %.0f GB RAM when monitoring is enabled for project %d; none of the %d worker nodes meet this minimum", minCommitSizingCPU, minCommitSizingRAM, projectID, workerCount),
		}
	}

	return nil
}

func flavorMeetsMinimum(flavor FlavorSummary, minCPU int32, minRAM float64) bool {
	return flavor.CPU >= minCPU && flavor.RAM >= minRAM
}

func commitProjectChanges(client *taikungoclient.Client, projectID int32) (projectCommitResult, *apiErrorInfo) {
	ctx := context.Background()

	command := taikuncore.NewProjectDeploymentCommitCommand()
	command.SetProjectId(projectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentCommit(ctx).
		ProjectDeploymentCommitCommand(*command).
		Execute()

	if _, errorInfo := commitProjectFallbackDecision(httpResponse, err); errorInfo.Message != "" {
		return projectCommitResult{}, &errorInfo
	}

	return projectCommitResult{
		Mode:    "project",
		Message: fmt.Sprintf("Successfully committed project %d deployment. Provisioning standalone VMs or other changes may take several minutes; an initial full Kubernetes cluster deploy often takes 10 to 30 minutes.", projectID),
	}, nil
}

func commitProjectWithReactiveFallback(client *taikungoclient.Client, projectID int32) (projectCommitResult, *apiErrorInfo) {
	result, errorInfo := commitProjectChanges(client, projectID)
	if errorInfo == nil {
		return result, nil
	}
	if commitProjectNeedsVMMessage(errorInfo.Message) {
		return commitProjectVMChanges(client, projectID)
	}
	return projectCommitResult{}, errorInfo
}

func commitProjectVMChanges(client *taikungoclient.Client, projectID int32) (projectCommitResult, *apiErrorInfo) {
	ctx := context.Background()

	vmCommand := taikuncore.NewDeploymentCommitVmCommand()
	vmCommand.SetProjectId(projectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentCommitVm(ctx).
		DeploymentCommitVmCommand(*vmCommand).
		Execute()

	if _, errorInfo := commitProjectFallbackDecision(httpResponse, err); errorInfo.Message != "" {
		return projectCommitResult{}, &errorInfo
	}

	return projectCommitResult{
		Mode:    "vm",
		Message: fmt.Sprintf("Successfully committed VM changes for project %d. Provisioning standalone VMs may take several minutes.", projectID),
	}, nil
}

func commitProjectFallbackDecision(httpResponse *http.Response, err error) (bool, apiErrorInfo) {
	if err == nil && httpResponse != nil && httpResponse.StatusCode >= 200 && httpResponse.StatusCode < 300 {
		return false, apiErrorInfo{}
	}

	errorInfo := apiErrorInfoFromResponse(httpResponse, err)
	return commitProjectNeedsVMMessage(errorInfo.Message), errorInfo
}

func commitProjectNeedsVMEndpoint(err error) bool {
	if err == nil {
		return false
	}

	return commitProjectNeedsVMMessage(err.Error())
}

func commitProjectNeedsVMMessage(message string) bool {
	msg := strings.ToLower(message)
	return strings.Contains(msg, "at least one worker") &&
		strings.Contains(msg, "one bastion") &&
		strings.Contains(msg, "master")
}

func getProjectDetails(client *taikungoclient.Client, args GetProjectDetailsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	// Using ProjectsList because it contains status and health info
	request := client.Client.ProjectsAPI.ProjectsList(ctx).
		Id(args.ProjectId)

	result, httpResponse, err := request.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "get project details"); errorResp != nil {
		return errorResp, nil
	}

	if len(result.Data) == 0 {
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("Project with ID %d not found", args.ProjectId),
		}), nil
	}

	project := result.Data[0]
	response := ProjectStatusResponse{
		ID:        project.GetId(),
		Name:      project.GetName(),
		Status:    string(project.GetStatus()),
		Health:    string(project.GetHealth()),
		CloudType: string(project.GetCloudType()),
	}

	return createJSONResponse(response), nil
}

func listFlavors(client *taikungoclient.Client, args ListFlavorsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	request := client.Client.CloudCredentialAPI.CloudcredentialsAllFlavors(ctx, args.CloudCredentialId)
	if args.Limit > 0 {
		request = request.Limit(args.Limit)
	}
	if args.Offset > 0 {
		request = request.Offset(args.Offset)
	}
	if args.Search != "" {
		request = request.Search(args.Search)
	}

	result, httpResponse, err := request.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "list flavors"); errorResp != nil {
		return errorResp, nil
	}

	var flavors []FlavorSummary
	if result != nil && result.Data != nil {
		for _, f := range result.Data {
			flavors = append(flavors, FlavorSummary{
				Name: f.GetName(),
				CPU:  f.GetCpu(),
				RAM:  f.GetRam(),
			})
		}
	}

	response := FlavorListResponse{
		Flavors: flavors,
		Total:   int32(len(flavors)),
		Message: fmt.Sprintf("Found %d flavors", len(flavors)),
	}

	return createJSONResponse(response), nil
}

func listServers(client *taikungoclient.Client, args ListServersArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	request := client.Client.ServersAPI.ServersDetails(ctx, args.ProjectId)

	result, httpResponse, err := request.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "list servers"); errorResp != nil {
		return errorResp, nil
	}

	var servers []ServerSummary
	if result != nil {
		for _, s := range result.Data {
			servers = append(servers, ServerSummary{
				ID:        s.GetId(),
				Name:      s.GetName(),
				Role:      string(s.GetRole()),
				Status:    s.GetStatus(),
				IPAddress: s.GetIpAddress(),
				Flavor:    s.GetFlavor(),
			})
		}
	}

	response := ServerListResponse{
		Servers: servers,
		Total:   int32(len(servers)),
		Message: fmt.Sprintf("Found %d servers", len(servers)),
	}

	return createJSONResponse(response), nil
}

func deleteServersFromProject(client *taikungoclient.Client, args DeleteServersArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	command := taikuncore.NewProjectDeploymentDeleteServersCommand()
	command.SetProjectId(args.ProjectId)
	command.SetServerIds(args.ServerIds)
	command.SetForceDeleteVClusters(args.ForceDeleteVClusters)
	command.SetDeleteAutoscalingServers(args.DeleteAutoscalingServers)

	request := client.Client.ProjectDeploymentAPI.ProjectDeploymentDelete(ctx).
		ProjectDeploymentDeleteServersCommand(*command)

	httpResponse, err := request.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "delete servers from project"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]interface{}{
		"message":   fmt.Sprintf("Successfully deleted %d server(s) from project %d", len(args.ServerIds), args.ProjectId),
		"serverIds": args.ServerIds,
		"success":   true,
	}), nil
}
