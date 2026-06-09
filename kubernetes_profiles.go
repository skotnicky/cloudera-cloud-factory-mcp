package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func normalizeCreateKubernetesProfileCommand(command *taikuncore.CreateKubernetesProfileCommand) *mcp_golang.ToolResponse {
	if !command.HasOctaviaEnabled() && !command.HasTaikunLBEnabled() {
		command.SetOctaviaEnabled(true)
		command.SetTaikunLBEnabled(false)
		return nil
	}

	if command.HasOctaviaEnabled() && command.HasTaikunLBEnabled() &&
		command.GetOctaviaEnabled() && command.GetTaikunLBEnabled() {
		return createJSONResponse(ErrorResponse{
			Error:   "octaviaEnabled and taikunLBEnabled cannot both be true",
			Details: "Set only one load balancer mode, or omit both fields to use the safe defaults octaviaEnabled=true and taikunLBEnabled=false.",
		})
	}

	return nil
}

func listKubernetesProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.KubernetesProfilesAPI.KubernetesprofilesList(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
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
	if args.SearchID != "" {
		req = req.SearchId(args.SearchID)
	}
	if args.ID > 0 {
		req = req.Id(args.ID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list Kubernetes profiles"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.KubernetesProfilesListDto{}
	total := 0
	if result != nil {
		items = result.Data
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("kubernetesProfiles", toKubernetesProfileSummaries(items), total, listMessage(total, "Kubernetes profile", "Kubernetes profiles")), nil
}

func createKubernetesProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateKubernetesProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	if errorResp := normalizeCreateKubernetesProfileCommand(command); errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.KubernetesProfilesAPI.KubernetesprofilesCreate(context.Background()).
		CreateKubernetesProfileCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create Kubernetes profile", "Kubernetes profile created successfully")
}

func deleteKubernetesProfile(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.KubernetesProfilesAPI.KubernetesprofilesDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete Kubernetes profile", fmt.Sprintf("Kubernetes profile %d deleted successfully", args.ID))
}

func dropdownKubernetesProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.KubernetesProfilesAPI.KubernetesprofilesDropdown(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.CloudID > 0 {
		req = req.CloudId(args.CloudID)
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
	if errorResp := checkResponse(httpResponse, "list Kubernetes profile dropdown values"); errorResp != nil {
		return errorResp, nil
	}

	return createListResponse("kubernetesProfiles", compactKubernetesProfileDropdown(items), len(items), listMessage(len(items), "Kubernetes profile", "Kubernetes profiles")), nil
}

func lockKubernetesProfile(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewKubernetesProfilesLockManagerCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.KubernetesProfilesAPI.KubernetesprofilesLockManager(context.Background()).
		KubernetesProfilesLockManagerCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock Kubernetes profile", fmt.Sprintf("Kubernetes profile %d lock mode updated to %q", args.ID, args.Mode))
}
