package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

const (
	defaultStandaloneVMVolumeSizeGiB int64 = 10
	windowsStandaloneVMVolumeHintGiB int64 = 50
)

type StandaloneWindowsPasswordArgs struct {
	ID         int32  `json:"id" jsonschema:"required,description=Standalone VM ID"`
	Key        string `json:"key,omitempty" jsonschema:"description=Private key content or passphrase when required by the API"`
	ConfigPath string `json:"configPath,omitempty" jsonschema:"description=Optional path to a config file used by the API to recover the Windows password"`
}

func listStandaloneVMs(client *taikungoclient.Client, args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.StandaloneAPI.StandaloneList(context.Background())
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.ProjectID > 0 {
		req = req.ProjectId(args.ProjectID)
	}
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.ID > 0 {
		req = req.Id(args.ID)
	}
	if args.FilterBy != "" {
		req = req.FilterBy(args.FilterBy)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list standalone VMs"); errorResp != nil {
		return errorResp, nil
	}

	total := 0
	items := interface{}([]interface{}{})
	if result != nil {
		items = result.GetData()
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(result.GetData())
		}
	}
	return createListResponse("standaloneVMs", compactJSON(items), total, listMessage(total, "standalone VM", "standalone VMs")), nil
}

func getStandaloneVMDetails(client *taikungoclient.Client, args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.StandaloneAPI.StandaloneDetails(context.Background(), args.ProjectID)
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.ID > 0 {
		req = req.Id(args.ID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get standalone VM details"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"standaloneVMs": compactJSON(result),
		"message":       fmt.Sprintf("Loaded standalone VM details for project %d", args.ProjectID),
		"success":       true,
	}), nil
}

func createStandaloneVM(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateStandAloneVmCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	volumeSizeDefaulted, windowsVolumeHint := applyCreateStandaloneVMDefaults(command)

	apiResp, httpResponse, err := client.Client.StandaloneAPI.StandaloneCreate(context.Background()).
		CreateStandAloneVmCommand(*command).
		Execute()
	if err != nil {
		return apiErrorInfoFromResponse(httpResponse, err).toolResponse(), nil
	}
	if errorResp := checkResponse(httpResponse, "create standalone VM"); errorResp != nil {
		return errorResp, nil
	}

	message := "Standalone VM created successfully"
	if apiResp != nil && apiResp.GetMessage() != "" {
		message = apiResp.GetMessage()
	}
	if projectID, ok := command.GetProjectIdOk(); ok {
		recordPendingStandaloneVMCreate(*projectID)
	}

	resp := map[string]interface{}{
		"message":         message,
		"success":         true,
		"volumeSizeGiB":   command.GetVolumeSize(),
		"volumeDefaulted": volumeSizeDefaulted,
	}
	if apiResp != nil && apiResp.GetId() != "" {
		resp["id"] = apiResp.GetId()
	}
	if apiResp != nil && apiResp.Result != nil {
		resp["result"] = apiResp.Result
	}
	if windowsVolumeHint != "" {
		resp["hint"] = windowsVolumeHint
	}

	return createJSONResponse(resp), nil
}

func applyCreateStandaloneVMDefaults(command *taikuncore.CreateStandAloneVmCommand) (bool, string) {
	if command == nil {
		return false, ""
	}

	volumeSizeDefaulted := false
	if command.VolumeSize == nil || *command.VolumeSize <= 0 {
		command.SetVolumeSize(defaultStandaloneVMVolumeSizeGiB)
		volumeSizeDefaulted = true
	}

	image, ok := command.GetImageOk()
	if ok && strings.Contains(strings.ToLower(*image), "windows") {
		return volumeSizeDefaulted, fmt.Sprintf("Windows images typically need a %d GiB root volume; consider setting volumeSize explicitly.", windowsStandaloneVMVolumeHintGiB)
	}

	return volumeSizeDefaulted, ""
}

func deleteStandaloneVM(client *taikungoclient.Client, args DeleteStandaloneVMArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewProjectDeploymentDeleteVmsCommand()
	command.SetProjectId(args.ProjectID)
	command.SetVmIds([]int32{args.VMID})

	httpResponse, err := client.Client.ProjectDeploymentAPI.ProjectDeploymentDeleteVms(context.Background()).
		ProjectDeploymentDeleteVmsCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "delete standalone VM"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]interface{}{
		"message":   fmt.Sprintf("Standalone VM %d marked for deletion from project %d. Call commit-project to apply the deletion.", args.VMID, args.ProjectID),
		"success":   true,
		"projectId": args.ProjectID,
		"vmId":      args.VMID,
	}), nil
}

func updateStandaloneVMFlavor(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateStandAloneVmFlavorCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneAPI.StandaloneUpdateFlavor(context.Background()).
		UpdateStandAloneVmFlavorCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update standalone VM flavor", "Standalone VM flavor updated successfully")
}

func manageStandaloneVMIP(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.StandAloneVmIpManagementCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneAPI.StandaloneIpManagement(context.Background()).
		StandAloneVmIpManagementCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "manage standalone VM IP", "Standalone VM IP management action completed successfully")
}

func resetStandaloneVMStatus(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.ResetStandAloneVmStatusCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneAPI.StandaloneReset(context.Background()).
		ResetStandAloneVmStatusCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "reset standalone VM status", "Standalone VM status reset successfully")
}

func getStandaloneVMConsole(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.VmConsoleScreenshotCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	result, httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsConsole(context.Background()).
		VmConsoleScreenshotCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get standalone VM console"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"console": result,
		"message": "Loaded standalone VM console output",
		"success": true,
	}), nil
}

func downloadStandaloneVMRDP(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsDownloadRdp(context.Background(), args.ID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "download standalone VM RDP"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"rdp":     result,
		"message": fmt.Sprintf("Downloaded RDP metadata for standalone VM %d", args.ID),
		"success": true,
	}), nil
}

func rebootStandaloneVM(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.RebootStandAloneVmCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsReboot(context.Background()).
		RebootStandAloneVmCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "reboot standalone VM", "Standalone VM reboot requested successfully")
}

func shelveStandaloneVM(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.ShelveStandAloneVmCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsShelve(context.Background()).
		ShelveStandAloneVmCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "shelve standalone VM", "Standalone VM shelve requested successfully")
}

func startStandaloneVM(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.StartStandaloneVmCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsStart(context.Background()).
		StartStandaloneVmCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "start standalone VM", "Standalone VM start requested successfully")
}

func getStandaloneVMStatus(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsStatus(context.Background(), args.ID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get standalone VM status"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"status":  compactJSON(result),
		"message": fmt.Sprintf("Loaded status for standalone VM %d", args.ID),
		"success": true,
	}), nil
}

func stopStandaloneVM(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.StopStandaloneVmCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsStop(context.Background()).
		StopStandaloneVmCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "stop standalone VM", "Standalone VM stop requested successfully")
}

func unshelveStandaloneVM(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UnshelveStandaloneVmCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneActionsAPI.StandaloneactionsUnshelve(context.Background()).
		UnshelveStandaloneVmCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "unshelve standalone VM", "Standalone VM unshelve requested successfully")
}

func getStandaloneVMWindowsPassword(client *taikungoclient.Client, args StandaloneWindowsPasswordArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.StandaloneActionsAPI.StandaloneactionsWindowsInstancePassword(context.Background()).
		Id(args.ID)
	if args.Key != "" {
		req = req.Key(args.Key)
	}
	if args.ConfigPath != "" {
		file, err := os.Open(args.ConfigPath)
		if err != nil {
			return createJSONResponse(ErrorResponse{
				Error:   "failed to open config file",
				Details: err.Error(),
			}), nil
		}
		defer func() {
			if err := file.Close(); err != nil {
				logger.Printf("Failed to close standalone VM config file %q: %v", args.ConfigPath, err)
			}
		}()
		req = req.Config(file)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get standalone VM Windows password"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"password": result,
		"message":  "Loaded Windows instance password",
		"success":  true,
	}), nil
}

func createStandaloneVMDisk(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateStandAloneDiskCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.StandaloneVMDisksAPI.StandalonevmdisksCreate(context.Background()).
		CreateStandAloneDiskCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create standalone VM disk", "Standalone VM disk created successfully")
}

func resizeStandaloneVMDisk(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateStandaloneVmDiskSizeCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneVMDisksAPI.StandalonevmdisksUpdateSize(context.Background()).
		UpdateStandaloneVmDiskSizeCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "resize standalone VM disk", "Standalone VM disk resized successfully")
}

func listStandaloneProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.StandaloneProfileAPI.StandaloneprofileList(context.Background())
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.SearchID != "" {
		req = req.SearchId(args.SearchID)
	}
	if args.ID > 0 {
		req = req.Id(args.ID)
	}
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list standalone profiles"); errorResp != nil {
		return errorResp, nil
	}
	total := 0
	items := interface{}([]interface{}{})
	if result != nil {
		items = result.GetData()
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(result.GetData())
		}
	}
	return createListResponse("standaloneProfiles", compactJSON(items), total, listMessage(total, "standalone profile", "standalone profiles")), nil
}

func createStandaloneProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.StandAloneProfileCreateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.StandaloneProfileAPI.StandaloneprofileCreate(context.Background()).
		StandAloneProfileCreateCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create standalone profile", "Standalone profile created successfully")
}

func updateStandaloneProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.StandAloneProfileUpdateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.StandaloneProfileAPI.StandaloneprofileEdit(context.Background()).
		StandAloneProfileUpdateCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update standalone profile", "Standalone profile updated successfully")
}

func deleteStandaloneProfile(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewDeleteStandAloneProfileCommand()
	command.SetId(args.ID)

	httpResponse, err := client.Client.StandaloneProfileAPI.StandaloneprofileDelete(context.Background()).
		DeleteStandAloneProfileCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "delete standalone profile", fmt.Sprintf("Standalone profile %d deleted successfully", args.ID))
}

func dropdownStandaloneProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.StandaloneProfileAPI.StandaloneprofileDropdown(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}

	items, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list standalone profile dropdown values"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("standaloneProfiles", compactJSON(items), len(items), listMessage(len(items), "standalone profile", "standalone profiles")), nil
}

func lockStandaloneProfile(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewStandAloneProfileLockManagementCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.StandaloneProfileAPI.StandaloneprofileLockManagement(context.Background()).
		StandAloneProfileLockManagementCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock standalone profile", fmt.Sprintf("Standalone profile %d lock mode updated to %q", args.ID, args.Mode))
}
