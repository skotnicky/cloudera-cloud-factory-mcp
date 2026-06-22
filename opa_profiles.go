package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func listOPAProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.OpaProfilesAPI.OpaprofilesList(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
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
	if args.ID > 0 {
		req = req.Id(args.ID)
	}
	if args.SearchID != "" {
		req = req.SearchId(args.SearchID)
	}
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list OPA profiles"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.OpaProfileListDto{}
	total := 0
	if result != nil {
		items = result.Data
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("opaProfiles", toOPAProfileSummaries(items), total, listMessage(total, "OPA profile", "OPA profiles")), nil
}

func createOPAProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateOpaProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.OpaProfilesAPI.OpaprofilesCreate(context.Background()).
		CreateOpaProfileCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create OPA profile", "OPA profile created successfully")
}

func updateOPAProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.OpaProfileUpdateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.OpaProfilesAPI.OpaprofilesUpdate(context.Background()).
		OpaProfileUpdateCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update OPA profile", "OPA profile updated successfully")
}

func deleteOPAProfile(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.OpaProfilesAPI.OpaprofilesDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete OPA profile", fmt.Sprintf("OPA profile %d deleted successfully", args.ID))
}

func dropdownOPAProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.OpaProfilesAPI.OpaprofilesDropdown(context.Background())
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
	if errorResp := checkResponse(httpResponse, "list OPA profile dropdown values"); errorResp != nil {
		return errorResp, nil
	}

	return createListResponse("opaProfiles", compactExtendedDropdownRefs(items), len(items), listMessage(len(items), "OPA profile", "OPA profiles")), nil
}

func lockOPAProfile(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewOpaProfileLockManagerCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.OpaProfilesAPI.OpaprofilesLockManager(context.Background()).
		OpaProfileLockManagerCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock OPA profile", fmt.Sprintf("OPA profile %d lock mode updated to %q", args.ID, args.Mode))
}

func syncOPAProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.OpaProfileSyncCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.OpaProfilesAPI.OpaprofilesSync(context.Background()).
		OpaProfileSyncCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "sync OPA profile", "OPA profile sync started successfully")
}

func makeOPAProfileDefault(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.OpaMakeDefaultCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.OpaProfilesAPI.OpaprofilesMakeDefault(context.Background()).
		OpaMakeDefaultCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "make OPA profile default", "OPA profile default updated successfully")
}
