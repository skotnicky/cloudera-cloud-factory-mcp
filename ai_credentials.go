package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func listAICredentials(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.AiCredentialsAPI.AiCredentialList(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
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
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
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
	if errorResp := checkResponse(httpResponse, "list AI credentials"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.AiCredentialsListDto{}
	total := 0
	if result != nil {
		items = result.Data
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("aiCredentials", toAICredentialSummaries(items), total, listMessage(total, "AI credential", "AI credentials")), nil
}

func createAICredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateAiCredentialCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AiCredentialsAPI.AiCredentialCreate(context.Background()).
		CreateAiCredentialCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create AI credential", "AI credential created successfully")
}

func deleteAICredential(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.AiCredentialsAPI.AiCredentialDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete AI credential", fmt.Sprintf("AI credential %d deleted successfully", args.ID))
}

func dropdownAICredentials(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.AiCredentialsAPI.AiCredentialDropdown(context.Background())
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
	if errorResp := checkResponse(httpResponse, "list AI credential dropdown values"); errorResp != nil {
		return errorResp, nil
	}

	return createListResponse("aiCredentials", compactAICredentialDropdown(items), len(items), listMessage(len(items), "AI credential", "AI credentials")), nil
}
