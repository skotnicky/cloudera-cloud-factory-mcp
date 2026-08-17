package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

const (
	spotToggleModeEnable  = "enable"
	spotToggleModeDisable = "disable"
)

var supportedSpotCloudTypes = map[string]struct{}{
	string(taikuncore.ECLOUDCREDENTIALTYPE_AWS):    {},
	string(taikuncore.ECLOUDCREDENTIALTYPE_AZURE):  {},
	string(taikuncore.ECLOUDCREDENTIALTYPE_GOOGLE): {},
}

type ProjectServiceToggleStatus struct {
	Enabled bool `json:"enabled"`
}

type ProjectAutoscalingServiceStatus struct {
	Enabled      bool     `json:"enabled"`
	SpotEnabled  bool     `json:"spotEnabled"`
	MinSize      *int32   `json:"minSize,omitempty"`
	MaxSize      *int32   `json:"maxSize,omitempty"`
	DiskSize     *float64 `json:"diskSize,omitempty"`
	Flavor       *string  `json:"flavor,omitempty"`
	MaxSpotPrice *float64 `json:"maxSpotPrice,omitempty"`
}

type ProjectAlertingServiceStatus struct {
	Enabled     bool   `json:"enabled"`
	ProfileID   *int32 `json:"profileId,omitempty"`
	ProfileName string `json:"profileName,omitempty"`
}

type ProjectBackupServiceStatus struct {
	Enabled      bool   `json:"enabled"`
	CredentialID *int32 `json:"credentialId,omitempty"`
}

type ProjectAIServiceStatus struct {
	Enabled      bool   `json:"enabled"`
	CredentialID *int32 `json:"credentialId,omitempty"`
}

type ProjectPolicyServiceStatus struct {
	Enabled     bool   `json:"enabled"`
	ProfileID   *int32 `json:"profileId,omitempty"`
	ProfileName string `json:"profileName,omitempty"`
}

type ProjectSpotServiceStatus struct {
	Available   bool `json:"available"`
	FullEnabled bool `json:"fullEnabled"`
	Workers     bool `json:"workersEnabled"`
	VMs         bool `json:"vmsEnabled"`
}

type ProjectServiceStatusResponse struct {
	ProjectID         int32                           `json:"projectId"`
	ProjectName       string                          `json:"projectName"`
	AccessIP          string                          `json:"accessIp,omitempty"`
	Status            string                          `json:"status"`
	Health            string                          `json:"health"`
	CloudType         string                          `json:"cloudType"`
	HasKubeconfigFile bool                            `json:"hasKubeconfigFile"`
	Autoscaling       ProjectAutoscalingServiceStatus `json:"autoscaling"`
	Alerting          ProjectAlertingServiceStatus    `json:"alerting"`
	AIAssistant       ProjectAIServiceStatus          `json:"aiAssistant"`
	Monitoring        ProjectServiceToggleStatus      `json:"monitoring"`
	Backup            ProjectBackupServiceStatus      `json:"backup"`
	Policy            ProjectPolicyServiceStatus      `json:"policy"`
	Spot              ProjectSpotServiceStatus        `json:"spot"`
	Success           bool                            `json:"success"`
	Message           string                          `json:"message"`
}

func nullableInt32Value(value *int32, ok bool) *int32 {
	if !ok || value == nil {
		return nil
	}
	v := *value
	return &v
}

func nullableFloat64Value(value *float64, ok bool) *float64 {
	if !ok || value == nil {
		return nil
	}
	v := *value
	return &v
}

func nullableStringValue(value *string, ok bool) *string {
	if !ok || value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	v := *value
	return &v
}

func isSpotAvailableForCloudType(cloudType string) bool {
	_, ok := supportedSpotCloudTypes[strings.ToUpper(strings.TrimSpace(cloudType))]
	return ok
}

func validateProjectSpotAvailability(projectID int32, cloudType string) *mcp_golang.ToolResponse {
	if isSpotAvailableForCloudType(cloudType) {
		return nil
	}

	return createJSONResponse(ErrorResponse{
		Error: fmt.Sprintf("Spot is available only for AWS, Azure, and GCP projects; project %d uses cloudType %q", projectID, cloudType),
	})
}

func buildProjectServiceStatusResponse(project taikuncore.ProjectDetailsForServersDto) ProjectServiceStatusResponse {
	alertingProfileID, hasAlertingProfileID := project.GetAlertingProfileIdOk()
	backupCredentialID, hasBackupCredentialID := project.GetS3CredentialIdOk()
	aiCredentialID, hasAICredentialID := project.GetAiCredentialIdOk()
	policyProfileID, hasPolicyProfileID := project.GetOpaProfileIdOk()
	minSize, hasMinSize := project.GetMinSizeOk()
	maxSize, hasMaxSize := project.GetMaxSizeOk()
	diskSize, hasDiskSize := project.GetDiskSizeOk()
	flavor, hasFlavor := project.GetFlavorOk()
	maxSpotPrice, hasMaxSpotPrice := project.GetMaxSpotPriceOk()

	return ProjectServiceStatusResponse{
		ProjectID:         project.GetId(),
		ProjectName:       project.GetName(),
		AccessIP:          strings.TrimSpace(project.GetAccessIp()),
		Status:            string(project.GetStatus()),
		Health:            string(project.GetHealth()),
		CloudType:         string(project.GetCloudType()),
		HasKubeconfigFile: project.GetHasKubeConfigFile(),
		Autoscaling: ProjectAutoscalingServiceStatus{
			Enabled:      project.GetIsAutoscalingEnabled(),
			SpotEnabled:  project.GetIsAutoscalingSpotEnabled(),
			MinSize:      nullableInt32Value(minSize, hasMinSize),
			MaxSize:      nullableInt32Value(maxSize, hasMaxSize),
			DiskSize:     nullableFloat64Value(diskSize, hasDiskSize),
			Flavor:       nullableStringValue(flavor, hasFlavor),
			MaxSpotPrice: nullableFloat64Value(maxSpotPrice, hasMaxSpotPrice),
		},
		Alerting: ProjectAlertingServiceStatus{
			Enabled:     hasAlertingProfileID && alertingProfileID != nil,
			ProfileID:   nullableInt32Value(alertingProfileID, hasAlertingProfileID),
			ProfileName: project.GetAlertingProfileName(),
		},
		AIAssistant: ProjectAIServiceStatus{
			Enabled:      project.GetAiEnabled(),
			CredentialID: nullableInt32Value(aiCredentialID, hasAICredentialID),
		},
		Monitoring: ProjectServiceToggleStatus{
			Enabled: project.GetIsMonitoringEnabled(),
		},
		Backup: ProjectBackupServiceStatus{
			Enabled:      project.GetIsBackupEnabled(),
			CredentialID: nullableInt32Value(backupCredentialID, hasBackupCredentialID),
		},
		Policy: ProjectPolicyServiceStatus{
			Enabled:     project.GetIsOpaEnabled(),
			ProfileID:   nullableInt32Value(policyProfileID, hasPolicyProfileID),
			ProfileName: project.GetOpaProfileName(),
		},
		Spot: ProjectSpotServiceStatus{
			Available:   isSpotAvailableForCloudType(string(project.GetCloudType())),
			FullEnabled: project.GetAllowFullSpotKubernetes(),
			Workers:     project.GetAllowSpotWorkers(),
			VMs:         project.GetAllowSpotVMs(),
		},
		Success: true,
		Message: fmt.Sprintf("Loaded project service status for project %d", project.GetId()),
	}
}

func getProjectServiceStatus(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.ServersAPI.ServersDetails(context.Background(), args.ProjectID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get project service status"); errorResp != nil {
		return errorResp, nil
	}
	if result == nil {
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("project %d not found", args.ProjectID),
		}), nil
	}

	return createJSONResponse(buildProjectServiceStatusResponse(result.GetProject())), nil
}

func enableProjectAIAssistant(client *taikungoclient.Client, args ProjectAICredentialArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentEnableAiCommand()
	command.SetProjectId(args.ProjectID)
	command.SetAiCredentialId(args.AICredentialID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentEnableAi(context.Background()).
		DeploymentEnableAiCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "enable project AI assistant", fmt.Sprintf("AI Assistant enabled for project %d", args.ProjectID))
}

func disableProjectAIAssistant(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentDisableAiCommand()
	command.SetProjectId(args.ProjectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentDisableAi(context.Background()).
		DeploymentDisableAiCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "disable project AI assistant", fmt.Sprintf("AI Assistant disabled for project %d", args.ProjectID))
}

func enableProjectBackup(client *taikungoclient.Client, args ProjectBackupCredentialArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentEnableBackupCommand()
	command.SetProjectId(args.ProjectID)
	command.SetS3CredentialId(args.BackupCredentialID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentEnableBackup(context.Background()).
		DeploymentEnableBackupCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "enable project backup", fmt.Sprintf("Backup enabled for project %d", args.ProjectID))
}

func disableProjectBackup(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentDisableBackupCommand()
	command.SetProjectId(args.ProjectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentDisableBackup(context.Background()).
		DeploymentDisableBackupCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "disable project backup", fmt.Sprintf("Backup disabled for project %d", args.ProjectID))
}

func enableProjectMonitoring(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentEnableMonitoringCommand()
	command.SetProjectId(args.ProjectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentEnableMonitoring(context.Background()).
		DeploymentEnableMonitoringCommand(*command).
		Execute()
	invalidateCachedMonitoringStatus(args.ProjectID)
	return finalizeAction(httpResponse, err, "enable project monitoring", fmt.Sprintf("Monitoring enabled for project %d", args.ProjectID))
}

func disableProjectMonitoring(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentDisableMonitoringCommand()
	command.SetProjectId(args.ProjectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentDisableMonitoring(context.Background()).
		DeploymentDisableMonitoringCommand(*command).
		Execute()
	invalidateCachedMonitoringStatus(args.ProjectID)
	return finalizeAction(httpResponse, err, "disable project monitoring", fmt.Sprintf("Monitoring disabled for project %d", args.ProjectID))
}

func enableProjectPolicy(client *taikungoclient.Client, args ProjectPolicyProfileArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentOpaEnableCommand()
	command.SetProjectId(args.ProjectID)
	command.SetOpaCredentialId(args.PolicyProfileID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentEnableOpa(context.Background()).
		DeploymentOpaEnableCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "enable project policy", fmt.Sprintf("Policy enabled for project %d", args.ProjectID))
}

func disableProjectPolicy(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeploymentDisableOpaCommand()
	command.SetProjectId(args.ProjectID)

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentDisableOpa(context.Background()).
		DeploymentDisableOpaCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "disable project policy", fmt.Sprintf("Policy disabled for project %d", args.ProjectID))
}

func enableProjectFullSpot(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	return toggleProjectFullSpot(client, args.ProjectID, spotToggleModeEnable)
}

func disableProjectFullSpot(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	return toggleProjectFullSpot(client, args.ProjectID, spotToggleModeDisable)
}

func enableProjectSpotWorkers(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	return toggleProjectSpotWorkers(client, args.ProjectID, spotToggleModeEnable)
}

func disableProjectSpotWorkers(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	return toggleProjectSpotWorkers(client, args.ProjectID, spotToggleModeDisable)
}

func enableProjectSpotVMs(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	return toggleProjectSpotVMs(client, args.ProjectID, spotToggleModeEnable)
}

func disableProjectSpotVMs(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	return toggleProjectSpotVMs(client, args.ProjectID, spotToggleModeDisable)
}

func toggleProjectFullSpot(client *taikungoclient.Client, projectID int32, mode string) (*mcp_golang.ToolResponse, error) {
	projectCloudType, errorResp := resolveProjectCloudType(client, projectID)
	if errorResp != nil {
		return errorResp, nil
	}
	if errorResp := validateProjectSpotAvailability(projectID, projectCloudType); errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewFullSpotOperationCommand()
	command.SetId(projectID)
	command.SetMode(mode)

	httpResponse, err := client.Client.ProjectsAPI.ProjectsToggleFullSpot(context.Background()).
		FullSpotOperationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "toggle project full spot", fmt.Sprintf("Full spot %sd for project %d", mode, projectID))
}

func toggleProjectSpotWorkers(client *taikungoclient.Client, projectID int32, mode string) (*mcp_golang.ToolResponse, error) {
	projectCloudType, errorResp := resolveProjectCloudType(client, projectID)
	if errorResp != nil {
		return errorResp, nil
	}
	if errorResp := validateProjectSpotAvailability(projectID, projectCloudType); errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewSpotWorkerOperationCommand()
	command.SetId(projectID)
	command.SetMode(mode)

	httpResponse, err := client.Client.ProjectsAPI.ProjectsToggleSpotWorkers(context.Background()).
		SpotWorkerOperationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "toggle project spot workers", fmt.Sprintf("Spot workers %sd for project %d", mode, projectID))
}

func toggleProjectSpotVMs(client *taikungoclient.Client, projectID int32, mode string) (*mcp_golang.ToolResponse, error) {
	projectCloudType, errorResp := resolveProjectCloudType(client, projectID)
	if errorResp != nil {
		return errorResp, nil
	}
	if errorResp := validateProjectSpotAvailability(projectID, projectCloudType); errorResp != nil {
		return errorResp, nil
	}

	command := taikuncore.NewSpotVmOperationCommand()
	command.SetId(projectID)
	command.SetMode(mode)

	httpResponse, err := client.Client.ProjectsAPI.ProjectsToggleSpotVms(context.Background()).
		SpotVmOperationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "toggle project spot VMs", fmt.Sprintf("Spot VMs %sd for project %d", mode, projectID))
}
