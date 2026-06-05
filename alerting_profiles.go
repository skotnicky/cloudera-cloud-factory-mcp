package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func listAlertingProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.AlertingProfilesAPI.AlertingprofilesList(context.Background())
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
	if args.ID > 0 {
		req = req.Id(args.ID)
	}
	if args.SearchID != "" {
		req = req.SearchId(args.SearchID)
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
	if errorResp := checkResponse(httpResponse, "list alerting profiles"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.AlertingProfilesListDto{}
	total := 0
	if result != nil {
		items = result.Data
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("alertingProfiles", toAlertingProfileSummaries(items), total, listMessage(total, "alerting profile", "alerting profiles")), nil
}

func createAlertingProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateAlertingProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesCreate(context.Background()).
		CreateAlertingProfileCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create alerting profile", "Alerting profile created successfully")
}

func updateAlertingProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateAlertingProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesEdit(context.Background()).
		UpdateAlertingProfileCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "update alerting profile", "Alerting profile updated successfully")
}

func deleteAlertingProfile(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete alerting profile", fmt.Sprintf("Alerting profile %d deleted successfully", args.ID))
}

func dropdownAlertingProfiles(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.AlertingProfilesAPI.AlertingprofilesDropdown(context.Background())
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
	if errorResp := checkResponse(httpResponse, "list alerting profile dropdown values"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("alertingProfiles", compactDropdownRefs(items), len(items), listMessage(len(items), "alerting profile", "alerting profiles")), nil
}

func lockAlertingProfile(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewAlertingProfilesLockManagerCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesLockManager(context.Background()).
		AlertingProfilesLockManagerCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock alerting profile", fmt.Sprintf("Alerting profile %d lock mode updated to %q", args.ID, args.Mode))
}

func attachAlertingProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.AttachDetachAlertingProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesAttach(context.Background()).
		AttachDetachAlertingProfileCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "attach alerting profile", "Alerting profile attached successfully")
}

func detachAlertingProfile(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.AttachDetachAlertingProfileCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesDetach(context.Background()).
		AttachDetachAlertingProfileCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "detach alerting profile", "Alerting profile detached successfully")
}

func assignAlertingEmails(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[[]taikuncore.AlertingEmailDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesAssignEmail(context.Background(), args.ID).
		AlertingEmailDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "assign alerting emails", fmt.Sprintf("Updated emails for alerting profile %d", args.ID))
}

func assignAlertingWebhooks(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[[]taikuncore.AlertingWebhookDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesAssignWebhooks(context.Background(), args.ID).
		AlertingWebhookDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "assign alerting webhooks", fmt.Sprintf("Updated webhooks for alerting profile %d", args.ID))
}

func verifyAlertingWebhook(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.VerifyWebhookCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AlertingProfilesAPI.AlertingprofilesVerify(context.Background()).
		VerifyWebhookCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "verify alerting webhook", "Webhook verification request completed successfully")
}

func listAlertingIntegrations(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.AlertingIntegrationsAPI.AlertingintegrationsList(context.Background(), args.ID)
	if args.Payload != "" {
		type searchOnly struct {
			Search string `json:"search"`
		}
		payload, errorResp := decodePayload[searchOnly](args.Payload)
		if errorResp != nil {
			return errorResp, nil
		}
		if payload.Search != "" {
			req = req.Search(payload.Search)
		}
	}

	items, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list alerting integrations"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("alertingIntegrations", compactJSON(items), len(items), listMessage(len(items), "alerting integration", "alerting integrations")), nil
}

func createAlertingIntegration(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateAlertingIntegrationCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AlertingIntegrationsAPI.AlertingintegrationsCreate(context.Background()).
		CreateAlertingIntegrationCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create alerting integration", "Alerting integration created successfully")
}

func updateAlertingIntegration(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.EditAlertingIntegrationCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AlertingIntegrationsAPI.AlertingintegrationsEdit(context.Background()).
		EditAlertingIntegrationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update alerting integration", "Alerting integration updated successfully")
}

func deleteAlertingIntegration(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.AlertingIntegrationsAPI.AlertingintegrationsDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete alerting integration", fmt.Sprintf("Alerting integration %d deleted successfully", args.ID))
}
