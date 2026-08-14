package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/itera-io/taikungoclient"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

// CreateGoogleCloudCredentialArgs drives create-google-cloud-credential. Google
// credentials cannot be created from a JSON body: the API expects a multipart
// upload of the GCP service-account JSON key file plus scalar form fields, so
// this tool takes a local file path to that key file instead of a payload.
type CreateGoogleCloudCredentialArgs struct {
	ConfigFilePath   string `json:"configFilePath" jsonschema:"required,description=Path to the GCP service-account JSON key file on the local filesystem"`
	Name             string `json:"name" jsonschema:"required,description=Name for the new Google cloud credential"`
	Region           string `json:"region,omitempty" jsonschema:"description=GCP region (discover with list-google-regions)"`
	BillingAccountID string `json:"billingAccountId,omitempty" jsonschema:"description=GCP billing account ID (discover with list-google-billing-accounts); required when importProject is false"`
	FolderID         string `json:"folderId,omitempty" jsonschema:"description=GCP folder ID to create the new project under (used when importProject is false)"`
	ImportProject    bool   `json:"importProject,omitempty" jsonschema:"description=Import the existing GCP project from the service-account key instead of creating a new project (default: false)"`
	AzCount          int32  `json:"azCount,omitempty" jsonschema:"description=Number of availability zones to use"`
	OrganizationID   int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID that should own the credential"`
}

// GoogleConfigArgs is used by lookups that only need the service-account key.
type GoogleConfigArgs struct {
	ConfigFilePath string `json:"configFilePath" jsonschema:"required,description=Path to the GCP service-account JSON key file on the local filesystem"`
}

// GoogleZoneListArgs is used by list-google-zones.
type GoogleZoneListArgs struct {
	ConfigFilePath string `json:"configFilePath" jsonschema:"required,description=Path to the GCP service-account JSON key file on the local filesystem"`
	Region         string `json:"region" jsonschema:"required,description=GCP region to list zones for (discover with list-google-regions)"`
	CloudID        int32  `json:"cloudId,omitempty" jsonschema:"description=Existing Google cloud credential ID to scope the zone lookup when supported"`
}

func openGoogleConfigFile(path string) (*os.File, *mcp_golang.ToolResponse) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, createJSONResponse(ErrorResponse{Error: "configFilePath is required"})
	}
	file, err := os.Open(trimmed)
	if err != nil {
		return nil, createJSONResponse(ErrorResponse{
			Error:   "unable to open GCP service-account key file",
			Details: err.Error(),
		})
	}
	return file, nil
}

func createGoogleCloudCredential(client *taikungoclient.Client, args CreateGoogleCloudCredentialArgs) (*mcp_golang.ToolResponse, error) {
	if strings.TrimSpace(args.Name) == "" {
		return createJSONResponse(ErrorResponse{Error: "name is required"}), nil
	}

	file, errorResp := openGoogleConfigFile(args.ConfigFilePath)
	if errorResp != nil {
		return errorResp, nil
	}
	defer file.Close()

	req := client.Client.GoogleAPI.GooglecloudCreate(context.Background()).
		Config(file).
		Name(args.Name).
		ImportProject(args.ImportProject)

	if region := strings.TrimSpace(args.Region); region != "" {
		req = req.Region(region)
	}
	if billing := strings.TrimSpace(args.BillingAccountID); billing != "" {
		req = req.BillingAccountId(billing)
	}
	if folder := strings.TrimSpace(args.FolderID); folder != "" {
		req = req.FolderId(folder)
	}
	if args.AzCount > 0 {
		req = req.AzCount(args.AzCount)
	}
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}

	apiResp, httpResponse, err := req.Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create Google cloud credential", "Google cloud credential created successfully")
}

func listGoogleRegions(client *taikungoclient.Client, args GoogleConfigArgs) (*mcp_golang.ToolResponse, error) {
	file, errorResp := openGoogleConfigFile(args.ConfigFilePath)
	if errorResp != nil {
		return errorResp, nil
	}
	defer file.Close()

	regions, httpResponse, err := client.Client.GoogleAPI.GooglecloudRegionList(context.Background()).
		Config(file).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list Google regions"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("regions", regions, len(regions), listMessage(len(regions), "Google region", "Google regions")), nil
}

func listGoogleZones(client *taikungoclient.Client, args GoogleZoneListArgs) (*mcp_golang.ToolResponse, error) {
	if strings.TrimSpace(args.Region) == "" {
		return createJSONResponse(ErrorResponse{Error: "region is required"}), nil
	}

	file, errorResp := openGoogleConfigFile(args.ConfigFilePath)
	if errorResp != nil {
		return errorResp, nil
	}
	defer file.Close()

	req := client.Client.GoogleAPI.GooglecloudZoneList(context.Background()).
		Config(file).
		Region(strings.TrimSpace(args.Region))
	if args.CloudID > 0 {
		req = req.CloudId(args.CloudID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list Google zones"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"result":  compactJSON(result),
		"message": fmt.Sprintf("Retrieved Google zones for region %q", args.Region),
		"success": true,
	}), nil
}

func listGoogleBillingAccounts(client *taikungoclient.Client, args GoogleConfigArgs) (*mcp_golang.ToolResponse, error) {
	file, errorResp := openGoogleConfigFile(args.ConfigFilePath)
	if errorResp != nil {
		return errorResp, nil
	}
	defer file.Close()

	accounts, httpResponse, err := client.Client.GoogleAPI.GooglecloudBillingAccountList(context.Background()).
		Config(file).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list Google billing accounts"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("billingAccounts", compactJSON(accounts), len(accounts), listMessage(len(accounts), "Google billing account", "Google billing accounts")), nil
}
