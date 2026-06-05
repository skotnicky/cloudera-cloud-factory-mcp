package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func listAccessProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	req := client.Client.AccessProfilesAPI.AccessprofilesList(ctx)
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.DomainID > 0 {
		req = req.DomainId(args.DomainID)
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
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list access profiles"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.AccessProfilesListDto{}
	total := 0
	if result != nil {
		items = result.Data
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("accessProfiles", toAccessProfileSummaries(items), total, listMessage(total, "access profile", "access profiles")), nil
}

func createAccessProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateAccessProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AccessProfilesAPI.AccessprofilesCreate(context.Background()).
		CreateAccessProfileCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create access profile", "Access profile created successfully")
}

func updateAccessProfile(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateAccessProfileDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AccessProfilesAPI.AccessprofilesUpdate(context.Background(), args.ID).
		UpdateAccessProfileDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update access profile", fmt.Sprintf("Access profile %d updated successfully", args.ID))
}

func deleteAccessProfile(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.AccessProfilesAPI.AccessprofilesDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete access profile", fmt.Sprintf("Access profile %d deleted successfully", args.ID))
}

func dropdownAccessProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.AccessProfilesAPI.AccessprofilesDropdown(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}

	items, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list access profile dropdown values"); errorResp != nil {
		return errorResp, nil
	}

	return createListResponse("accessProfiles", compactDropdownRefs(items), len(items), listMessage(len(items), "access profile", "access profiles")), nil
}

func lockAccessProfile(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewAccessProfilesLockManagementCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.AccessProfilesAPI.AccessprofilesLockManager(context.Background()).
		AccessProfilesLockManagementCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock access profile", fmt.Sprintf("Access profile %d lock mode updated to %q", args.ID, args.Mode))
}
