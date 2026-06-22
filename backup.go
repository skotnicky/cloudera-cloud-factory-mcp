package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func projectBackupReadErrorResponse(projectID int32, httpResponse *http.Response, err error) *mcp_golang.ToolResponse {
	apiErr := apiErrorInfoFromResponse(httpResponse, err)
	if apiErr.contains("There is no kubeconfig file for this project") {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Backup operations are unavailable for project %d until the project is deployed and a kubeconfig is available", projectID),
			Details: apiErr.Message,
		})
	}
	return apiErr.toolResponse()
}

func listBackupCredentials(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.S3CredentialsAPI.S3credentialsList(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
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
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list backup credentials"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.BackupCredentialsListDto{}
	total := 0
	if result != nil {
		items = result.Data
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("backupCredentials", compactJSON(items), total, listMessage(total, "backup credential", "backup credentials")), nil
}

func createBackupCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.BackupCredentialsCreateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.S3CredentialsAPI.S3credentialsCreate(context.Background()).
		BackupCredentialsCreateCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create backup credential", "Backup credential created successfully")
}

func updateBackupCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.BackupCredentialsUpdateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.S3CredentialsAPI.S3credentialsUpdate(context.Background()).
		BackupCredentialsUpdateCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update backup credential", "Backup credential updated successfully")
}

func deleteBackupCredential(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.S3CredentialsAPI.S3credentialsDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete backup credential", fmt.Sprintf("Backup credential %d deleted successfully", args.ID))
}

func dropdownBackupCredentials(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.S3CredentialsAPI.S3credentialsDropdown(context.Background())
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
	if errorResp := checkResponse(httpResponse, "list backup credential dropdown values"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("backupCredentials", compactJSON(items), len(items), listMessage(len(items), "backup credential", "backup credentials")), nil
}

func makeBackupCredentialDefault(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewBackupMakeDefaultCommand()
	command.SetId(args.ID)

	httpResponse, err := client.Client.S3CredentialsAPI.S3credentialsMakeDefault(context.Background()).
		BackupMakeDefaultCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "make backup credential default", fmt.Sprintf("Backup credential %d set as default successfully", args.ID))
}

func lockBackupCredential(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewBackupLockManagerCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.S3CredentialsAPI.S3credentialsLockManagement(context.Background()).
		BackupLockManagerCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock backup credential", fmt.Sprintf("Backup credential %d lock mode updated to %q", args.ID, args.Mode))
}

func createBackupPolicy(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateBackupPolicyCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupCreate(context.Background()).
		CreateBackupPolicyCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "create backup policy", "Backup policy created successfully")
}

func getBackupByName(client *taikungoclient.Client, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.BackupPolicyAPI.BackupByName(context.Background(), args.ProjectID, args.Name).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get backup by name"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"backup":  compactJSON(result),
		"message": fmt.Sprintf("Loaded backup %q for project %d", args.Name, args.ProjectID),
		"success": true,
	}), nil
}

func listProjectBackups(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.BackupPolicyAPI.BackupListAllBackups(context.Background(), args.ProjectID).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list backups"); errorResp != nil {
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
	return createListResponse("backups", compactJSON(items), total, listMessage(total, "backup", "backups")), nil
}

func listProjectRestoreRequests(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.BackupPolicyAPI.BackupListAllRestores(context.Background(), args.ProjectID).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list restore requests"); errorResp != nil {
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
	return createListResponse("restores", compactJSON(items), total, listMessage(total, "restore", "restores")), nil
}

func listProjectBackupSchedules(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.BackupPolicyAPI.BackupListAllSchedules(context.Background(), args.ProjectID).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list backup schedules"); errorResp != nil {
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
	return createListResponse("schedules", compactJSON(items), total, listMessage(total, "schedule", "schedules")), nil
}

func listProjectBackupStorageLocations(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.BackupPolicyAPI.BackupListAllBackupStorages(context.Background(), args.ProjectID).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list backup storage locations"); errorResp != nil {
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
	return createListResponse("storageLocations", compactJSON(items), total, listMessage(total, "storage location", "storage locations")), nil
}

func listProjectBackupDeleteRequests(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.BackupPolicyAPI.BackupListAllDeleteBackupRequests(context.Background(), args.ProjectID).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list backup delete requests"); errorResp != nil {
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
	return createListResponse("deleteRequests", compactJSON(items), total, listMessage(total, "delete request", "delete requests")), nil
}

func describeBackup(client *taikungoclient.Client, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
	content, httpResponse, err := client.Client.BackupPolicyAPI.BackupDescribeBackup(context.Background(), args.ProjectID, args.Name).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "describe backup"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"projectId": args.ProjectID,
		"name":      args.Name,
		"content":   content,
		"message":   fmt.Sprintf("Loaded backup description for %q", args.Name),
	}), nil
}

func describeRestore(client *taikungoclient.Client, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
	content, httpResponse, err := client.Client.BackupPolicyAPI.BackupDescribeRestore(context.Background(), args.ProjectID, args.Name).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "describe restore"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"projectId": args.ProjectID,
		"name":      args.Name,
		"content":   content,
		"message":   fmt.Sprintf("Loaded restore description for %q", args.Name),
	}), nil
}

func describeSchedule(client *taikungoclient.Client, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
	content, httpResponse, err := client.Client.BackupPolicyAPI.BackupDescribeSchedule(context.Background(), args.ProjectID, args.Name).Execute()
	if err != nil {
		return projectBackupReadErrorResponse(args.ProjectID, httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "describe schedule"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"projectId": args.ProjectID,
		"name":      args.Name,
		"content":   content,
		"message":   fmt.Sprintf("Loaded schedule description for %q", args.Name),
	}), nil
}

func deleteBackup(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteBackupCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupDeleteBackup(context.Background()).
		DeleteBackupCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "delete backup", "Backup deletion requested successfully")
}

func deleteBackupStorageLocation(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteBackupStorageLocationCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupDeleteBackupLocation(context.Background()).
		DeleteBackupStorageLocationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "delete backup storage location", "Backup storage location deleted successfully")
}

func deleteRestore(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteRestoreCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupDeleteRestore(context.Background()).
		DeleteRestoreCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "delete restore", "Restore deletion requested successfully")
}

func deleteSchedule(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteScheduleCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupDeleteSchedule(context.Background()).
		DeleteScheduleCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "delete schedule", "Backup schedule deleted successfully")
}

func importBackupStorageLocation(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.ImportBackupStorageLocationCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupImportBackupStorage(context.Background()).
		ImportBackupStorageLocationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "import backup storage location", "Backup storage location imported successfully")
}

func restoreBackup(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.RestoreBackupCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.BackupPolicyAPI.BackupRestoreBackup(context.Background()).
		RestoreBackupCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "restore backup", "Backup restore requested successfully")
}
