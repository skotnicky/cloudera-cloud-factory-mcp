package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

	minCommitSizingCPU        int32   = 4
	minCommitSizingRAM        float64 = 4
	minBastionCPU             int32   = 2
	minBastionRAM             float64 = 2
	defaultClusterWaitTimeout         = 1800
	// defaultCreateClusterDiskGB is used when diskSizeGb is omitted so cloud APIs
	// receive an explicit root volume size (some providers reject unset / too-small disks).
	defaultCreateClusterDiskGB int64 = 50
)

type clusterFlavorSelection struct {
	Bastion string `json:"bastion"`
	Master  string `json:"master"`
	Worker  string `json:"worker"`
}

// distinctCreateClusterFlavorNames returns unique flavor names in bastion, master, worker order.
func distinctCreateClusterFlavorNames(sel clusterFlavorSelection) []string {
	order := []string{sel.Bastion, sel.Master, sel.Worker}
	seen := make(map[string]struct{}, len(order))
	var out []string
	for _, name := range order {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

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

func createCluster(client *taikungoclient.Client, args CreateClusterArgs) (*mcp_golang.ToolResponse, error) {
	if strings.TrimSpace(args.Name) == "" {
		return createJSONResponse(ErrorResponse{Error: "Cluster name is required"}), nil
	}
	if args.CloudCredentialID <= 0 {
		return createJSONResponse(ErrorResponse{Error: "cloudCredentialId must be provided"}), nil
	}

	bastionCount := args.BastionCount
	if bastionCount <= 0 {
		bastionCount = 1
	}
	masterCount := args.MasterCount
	if masterCount <= 0 {
		masterCount = 1
	}
	if masterCount%2 == 0 {
		return createJSONResponse(ErrorResponse{
			Error:   "masterCount must be an odd number",
			Details: "Use 1, 3, 5, ... masters to satisfy cluster quorum requirements.",
		}), nil
	}
	workerCount := args.WorkerCount
	if workerCount <= 0 {
		workerCount = 1
	}

	projectArgs, projectError := resolveCreateClusterProjectArgs(client, args)
	if projectError != nil {
		return projectError, nil
	}

	createProjectResp, err := createProject(client, projectArgs)
	if err != nil {
		return nil, err
	}
	if !isToolResponseSuccess(createProjectResp) {
		return createProjectResp, nil
	}

	projectID, ok := projectIDFromCreateProjectResponse(createProjectResp)
	if !ok || projectID <= 0 {
		return createJSONResponse(ErrorResponse{
			Error:   "Project creation succeeded but project ID could not be parsed",
			Details: "Retry with create-project to inspect the raw response and confirm API payload format.",
		}), nil
	}

	if err := addCreatedProjectID(projectID); err != nil {
		logger.Printf("Failed to persist created project ID %d from create-cluster: %v", projectID, err)
	}

	flavors, flavorError := resolveCreateClusterFlavors(client, args)
	if flavorError != nil {
		return createClusterFailureResponse(projectID, "flavor-selection", flavorError), nil
	}

	flavorNames := distinctCreateClusterFlavorNames(flavors)
	if len(flavorNames) > 0 {
		bindResp, bindErr := bindFlavorsToProject(client, BindFlavorsArgs{
			ProjectId: projectID,
			Flavors:   flavorNames,
		})
		if bindErr != nil {
			return createClusterFailureResponse(projectID, "bind-flavors", createJSONResponse(ErrorResponse{
				Error:   "Failed to bind flavors to project",
				Details: bindErr.Error(),
			})), nil
		}
		if !isToolResponseSuccess(bindResp) {
			return createClusterFailureResponse(projectID, "bind-flavors", bindResp), nil
		}
	}

	diskGB := args.DiskSizeGB
	if diskGB <= 0 {
		diskGB = defaultCreateClusterDiskGB
	}

	verifyTimeout := args.VerifyTimeout
	if verifyTimeout <= 0 {
		verifyTimeout = 300
	}

	clusterName := strings.TrimSpace(args.Name)
	serverSteps := []AddServerArgs{
		{
			ProjectId:            projectID,
			Name:                 fmt.Sprintf("%s-bastion", clusterName),
			Role:                 "Bastion",
			Flavor:               flavors.Bastion,
			DiskSize:             diskGB,
			Count:                bastionCount,
			VerifyTimeoutSeconds: verifyTimeout,
		},
		{
			ProjectId:            projectID,
			Name:                 fmt.Sprintf("%s-master", clusterName),
			Role:                 "Kubemaster",
			Flavor:               flavors.Master,
			DiskSize:             diskGB,
			Count:                masterCount,
			VerifyTimeoutSeconds: verifyTimeout,
		},
		{
			ProjectId:            projectID,
			Name:                 fmt.Sprintf("%s-worker", clusterName),
			Role:                 "Kubeworker",
			Flavor:               flavors.Worker,
			DiskSize:             diskGB,
			Count:                workerCount,
			VerifyTimeoutSeconds: verifyTimeout,
		},
	}

	for _, step := range serverSteps {
		addResp, addErr := addServerToProject(client, step)
		if addErr != nil {
			return createClusterFailureResponse(projectID, "node-provisioning", createJSONResponse(ErrorResponse{
				Error:   fmt.Sprintf("Failed adding %s nodes", step.Role),
				Details: addErr.Error(),
			})), nil
		}
		if !isToolResponseSuccess(addResp) {
			return createClusterFailureResponse(projectID, "node-provisioning", addResp), nil
		}
	}

	commitResp, commitErr := commitProject(client, CommitProjectArgs{ProjectId: projectID})
	if commitErr != nil {
		return createClusterFailureResponse(projectID, "commit", createJSONResponse(ErrorResponse{
			Error:   "Failed to commit project",
			Details: commitErr.Error(),
		})), nil
	}
	if !isToolResponseSuccess(commitResp) {
		return createClusterFailureResponse(projectID, "commit", commitResp), nil
	}

	waitForCreation := true
	if args.WaitForCreation != nil {
		waitForCreation = *args.WaitForCreation
	}
	if waitForCreation {
		waitTimeout := args.Timeout
		if waitTimeout <= 0 {
			waitTimeout = defaultClusterWaitTimeout
		}
		waitResp, waitErr := waitForProject(client, WaitForProjectArgs{
			ProjectId: projectID,
			Timeout:   waitTimeout,
		})
		if waitErr != nil {
			return createClusterFailureResponse(projectID, "wait-for-ready", createJSONResponse(ErrorResponse{
				Error:   "Failed waiting for project readiness",
				Details: waitErr.Error(),
			})), nil
		}
		if !isToolResponseSuccess(waitResp) {
			return createClusterFailureResponse(projectID, "wait-for-ready", waitResp), nil
		}
	}

	return createJSONResponse(map[string]interface{}{
		"success":             true,
		"message":             fmt.Sprintf("Cluster %q provisioned successfully in project %d", args.Name, projectID),
		"projectId":           projectID,
		"projectCreated":      true,
		"flavors":             flavors,
		"kubernetesProfileId": projectArgs.KubernetesProfileID,
		"alertingProfileId":   projectArgs.AlertingProfileID,
		"monitoring":          projectArgs.Monitoring,
		"counts": map[string]int32{
			"bastion": bastionCount,
			"master":  masterCount,
			"worker":  workerCount,
		},
		"waitedForCreation": waitForCreation,
	}), nil
}

func resolveCreateClusterProjectArgs(client *taikungoclient.Client, args CreateClusterArgs) (CreateProjectArgs, *mcp_golang.ToolResponse) {
	projectArgs := CreateProjectArgs{
		Name:              args.Name,
		CloudCredentialID: args.CloudCredentialID,
		Monitoring:        args.Monitoring,
		KubernetesVersion: args.KubernetesVersion,
	}

	if args.KubernetesProfileID > 0 {
		projectArgs.KubernetesProfileID = args.KubernetesProfileID
	} else {
		resolvedProfileID, errorResp := resolveDeterministicKubernetesProfileID(client, args.CloudCredentialID)
		if errorResp != nil {
			return CreateProjectArgs{}, errorResp
		}
		projectArgs.KubernetesProfileID = resolvedProfileID
	}

	if args.AlertingProfileID > 0 {
		projectArgs.AlertingProfileID = args.AlertingProfileID
	} else if args.Monitoring {
		resolvedAlertingID, errorResp := resolveDeterministicAlertingProfileID(client)
		if errorResp != nil {
			return CreateProjectArgs{}, errorResp
		}
		projectArgs.AlertingProfileID = resolvedAlertingID
	}

	return projectArgs, nil
}

func resolveDeterministicKubernetesProfileID(client *taikungoclient.Client, cloudCredentialID int32) (int32, *mcp_golang.ToolResponse) {
	items, httpResponse, err := client.Client.KubernetesProfilesAPI.KubernetesprofilesDropdown(context.Background()).
		CloudId(cloudCredentialID).
		Limit(3).
		Execute()
	if err != nil {
		return 0, createError(httpResponse, err)
	}
	if errorResp := checkResponse(httpResponse, "resolve Kubernetes profile for create-cluster"); errorResp != nil {
		return 0, errorResp
	}

	ids := extractDistinctIDs(items)
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) == 0 {
		return 0, createJSONResponse(ErrorResponse{
			Error:   "Unable to auto-select Kubernetes profile",
			Details: fmt.Sprintf("No Kubernetes profile found for cloudCredentialId %d. Provide kubernetesProfileId explicitly.", cloudCredentialID),
		})
	}
	return 0, createJSONResponse(ErrorResponse{
		Error:   "Unable to auto-select Kubernetes profile",
		Details: fmt.Sprintf("Multiple Kubernetes profiles match cloudCredentialId %d (IDs: %v). Provide kubernetesProfileId explicitly.", cloudCredentialID, ids),
	})
}

func resolveDeterministicAlertingProfileID(client *taikungoclient.Client) (int32, *mcp_golang.ToolResponse) {
	items, httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesDropdown(context.Background()).
		Execute()
	if err != nil {
		return 0, createError(httpResponse, err)
	}
	if errorResp := checkResponse(httpResponse, "resolve alerting profile for create-cluster"); errorResp != nil {
		return 0, errorResp
	}

	ids := extractDistinctIDs(items)
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) == 0 {
		return 0, createJSONResponse(ErrorResponse{
			Error:   "Monitoring is enabled but no alerting profile could be auto-selected",
			Details: "No alerting profiles were found. Provide alertingProfileId explicitly or disable monitoring.",
		})
	}
	return 0, createJSONResponse(ErrorResponse{
		Error:   "Monitoring is enabled but alerting profile selection is ambiguous",
		Details: fmt.Sprintf("Multiple alerting profiles are available (IDs: %v). Provide alertingProfileId explicitly.", ids),
	})
}

func resolveCreateClusterFlavors(client *taikungoclient.Client, args CreateClusterArgs) (clusterFlavorSelection, *mcp_golang.ToolResponse) {
	flavorsResp, httpResponse, err := client.Client.CloudCredentialAPI.CloudcredentialsAllFlavors(context.Background(), args.CloudCredentialID).Execute()
	if err != nil {
		return clusterFlavorSelection{}, createError(httpResponse, err)
	}
	if errorResp := checkResponse(httpResponse, "resolve flavors for create-cluster"); errorResp != nil {
		return clusterFlavorSelection{}, errorResp
	}

	available := make([]FlavorSummary, 0)
	if flavorsResp != nil {
		for _, flavor := range flavorsResp.GetData() {
			available = append(available, FlavorSummary{
				Name: flavor.GetName(),
				CPU:  flavor.GetCpu(),
				RAM:  flavor.GetRam(),
			})
		}
	}
	if len(available) == 0 {
		return clusterFlavorSelection{}, createJSONResponse(ErrorResponse{
			Error:   "No flavors available for cloud credential",
			Details: fmt.Sprintf("cloudCredentialId %d returned an empty flavor list; provide explicit node flavors after binding valid flavors.", args.CloudCredentialID),
		})
	}

	sort.Slice(available, func(i, j int) bool {
		if available[i].CPU != available[j].CPU {
			return available[i].CPU < available[j].CPU
		}
		if available[i].RAM != available[j].RAM {
			return available[i].RAM < available[j].RAM
		}
		return strings.ToLower(available[i].Name) < strings.ToLower(available[j].Name)
	})

	// Workers use the same minimum as Kubemaster/commit preflight (4 CPU / 4 GB RAM)
	// so auto-selected workers are not smaller than control-plane sizing when monitoring is off.
	workerMinCPU := minCommitSizingCPU
	workerMinRAM := minCommitSizingRAM

	bastion, ok := selectFlavorForMinimum(available, args.BastionFlavor, minBastionCPU, minBastionRAM)
	if !ok {
		return clusterFlavorSelection{}, createJSONResponse(ErrorResponse{
			Error:   "Unable to determine bastion flavor",
			Details: fmt.Sprintf("Provide bastionFlavor explicitly or ensure a flavor with at least %d CPU / %.0f GB RAM is available.", minBastionCPU, minBastionRAM),
		})
	}
	master, ok := selectFlavorForMinimum(available, args.MasterFlavor, minCommitSizingCPU, minCommitSizingRAM)
	if !ok {
		return clusterFlavorSelection{}, createJSONResponse(ErrorResponse{
			Error:   "Unable to determine master flavor",
			Details: fmt.Sprintf("Provide masterFlavor explicitly or ensure a flavor with at least %d CPU / %.0f GB RAM is available.", minCommitSizingCPU, minCommitSizingRAM),
		})
	}
	worker, ok := selectFlavorForMinimum(available, args.WorkerFlavor, workerMinCPU, workerMinRAM)
	if !ok {
		return clusterFlavorSelection{}, createJSONResponse(ErrorResponse{
			Error:   "Unable to determine worker flavor",
			Details: fmt.Sprintf("Provide workerFlavor explicitly or ensure a flavor with at least %d CPU / %.0f GB RAM is available.", workerMinCPU, workerMinRAM),
		})
	}

	return clusterFlavorSelection{
		Bastion: bastion,
		Master:  master,
		Worker:  worker,
	}, nil
}

func selectFlavorForMinimum(available []FlavorSummary, override string, minCPU int32, minRAM float64) (string, bool) {
	override = strings.TrimSpace(override)
	if override != "" {
		for _, flavor := range available {
			if strings.EqualFold(flavor.Name, override) && flavorMeetsMinimum(flavor, minCPU, minRAM) {
				return flavor.Name, true
			}
		}
		return "", false
	}

	for _, flavor := range available {
		if flavorMeetsMinimum(flavor, minCPU, minRAM) {
			return flavor.Name, true
		}
	}
	return "", false
}

func extractDistinctIDs(value interface{}) []int32 {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}

	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}

	collected := []int32{}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch typed := v.(type) {
		case map[string]interface{}:
			for key, nested := range typed {
				if normalizeMCPLockKey(key) == "id" {
					if id, ok := toMCPLockInt32(nested); ok && id > 0 {
						collected = append(collected, id)
					}
				}
				walk(nested)
			}
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(decoded)
	return normalizeMCPLockIDs(collected)
}

func parseToolResponsePayload(response *mcp_golang.ToolResponse) (map[string]interface{}, bool) {
	if response == nil || len(response.Content) == 0 || response.Content[0].TextContent == nil {
		return nil, false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(response.Content[0].TextContent.Text), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func isToolResponseSuccess(response *mcp_golang.ToolResponse) bool {
	payload, ok := parseToolResponsePayload(response)
	if !ok {
		return false
	}

	if errorMessage, hasError := payload["error"].(string); hasError && strings.TrimSpace(errorMessage) != "" {
		return false
	}
	if success, hasSuccess := payload["success"].(bool); hasSuccess {
		return success
	}
	return true
}

func createClusterFailureResponse(projectID int32, stage string, base *mcp_golang.ToolResponse) *mcp_golang.ToolResponse {
	payload, ok := parseToolResponsePayload(base)
	if !ok {
		return createJSONResponse(map[string]interface{}{
			"success":        false,
			"error":          fmt.Sprintf("create-cluster failed during %s", stage),
			"details":        fmt.Sprintf("Project %d was already created before this failure.", projectID),
			"projectId":      projectID,
			"projectCreated": true,
			"stage":          stage,
		})
	}

	errorMessage, hasError := payload["error"].(string)
	if !hasError || strings.TrimSpace(errorMessage) == "" {
		payload["error"] = fmt.Sprintf("create-cluster failed during %s", stage)
	}
	existingDetails, _ := payload["details"].(string)
	if strings.TrimSpace(existingDetails) == "" {
		payload["details"] = fmt.Sprintf("Project %d was already created before this failure.", projectID)
	} else {
		payload["details"] = fmt.Sprintf("%s Project %d was already created before this failure.", existingDetails, projectID)
	}
	payload["success"] = false
	payload["projectId"] = projectID
	payload["projectCreated"] = true
	payload["stage"] = stage
	return createJSONResponse(payload)
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
