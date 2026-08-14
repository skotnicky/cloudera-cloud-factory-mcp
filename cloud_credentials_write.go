package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

// CloudCredentialWriteArgs is the consolidated argument set for the
// create-cloud-credential / update-cloud-credential tools. A single
// cloudType discriminator replaces the per-provider create/update tools,
// shrinking the tool manifest while keeping every provider reachable.
type CloudCredentialWriteArgs struct {
	CloudType string `json:"cloudType" jsonschema:"required,description=Cloud type: aws, azure, openstack (use create-google-cloud-credential for gcp)"`
	Payload   string `json:"payload" jsonschema:"required,description=JSON payload matching the underlying create/update command for the chosen cloudType"`
}

func normalizeCloudType(cloudType string) string {
	normalized := strings.ToLower(strings.TrimSpace(cloudType))
	return strings.ReplaceAll(normalized, "_", "-")
}

// googleCloudCredentialNotSupportedResponse explains that Google/GCP credentials
// are not manageable through the JSON-payload create/update tools: the Google
// create API requires a multipart service-account key file upload and has no
// update endpoint, so it does not fit this tool's command-body model. Use the
// dedicated create-google-cloud-credential tool (which accepts a key file path)
// for creation instead.
func googleCloudCredentialNotSupportedResponse(operation string) *mcp_golang.ToolResponse {
	details := "The Google cloud credential API requires a multipart service-account key file upload, so it cannot be driven through a JSON payload here. Use the create-google-cloud-credential tool, which accepts the path to a GCP service-account JSON key file."
	if strings.Contains(operation, "update") {
		details = "The Google cloud credential API has no update endpoint. Delete and recreate the credential with create-google-cloud-credential (which accepts a GCP service-account JSON key file path) instead."
	}
	return createJSONResponse(ErrorResponse{
		Error:   fmt.Sprintf("Google/GCP cloud credentials are not supported by %s", operation),
		Details: details,
	})
}

func createCloudCredential(client *taikungoclient.Client, args CloudCredentialWriteArgs) (*mcp_golang.ToolResponse, error) {
	payload := JSONPayloadArgs{Payload: args.Payload}
	switch normalizeCloudType(args.CloudType) {
	case "aws":
		return createAWSCloudCredential(client, payload)
	case "azure":
		return createAzureCloudCredential(client, payload)
	case "openstack":
		return createOpenStackCloudCredential(client, payload)
	case "google", "gcp":
		return googleCloudCredentialNotSupportedResponse("create-cloud-credential"), nil
	default:
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("unsupported cloudType %q for create-cloud-credential", args.CloudType),
			Details: "Supported cloudType values: aws, azure, openstack (use create-google-cloud-credential for gcp).",
		}), nil
	}
}

func updateCloudCredential(client *taikungoclient.Client, args CloudCredentialWriteArgs) (*mcp_golang.ToolResponse, error) {
	payload := JSONPayloadArgs{Payload: args.Payload}
	switch normalizeCloudType(args.CloudType) {
	case "aws":
		return updateAWSCloudCredential(client, payload)
	case "azure":
		return updateAzureCloudCredential(client, payload)
	case "openstack":
		return updateOpenStackCloudCredential(client, payload)
	case "google", "gcp":
		return googleCloudCredentialNotSupportedResponse("update-cloud-credential"), nil
	default:
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("unsupported cloudType %q for update-cloud-credential", args.CloudType),
			Details: "Supported cloudType values: aws, azure, openstack (use create-google-cloud-credential for gcp).",
		}), nil
	}
}

func createAWSCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateAwsCloudCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AWSCloudCredentialAPI.AwsCreate(context.Background()).
		CreateAwsCloudCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create AWS cloud credential", "AWS cloud credential created successfully")
}

func updateAWSCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateAwsCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AWSCloudCredentialAPI.AwsUpdate(context.Background()).
		UpdateAwsCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update AWS cloud credential", "AWS cloud credential updated successfully")
}

func createAzureCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateAzureCloudCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.AzureCloudCredentialAPI.AzureCreate(context.Background()).
		CreateAzureCloudCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create Azure cloud credential", "Azure cloud credential created successfully")
}

func updateAzureCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateAzureCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.AzureCloudCredentialAPI.AzureUpdate(context.Background()).
		UpdateAzureCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update Azure cloud credential", "Azure cloud credential updated successfully")
}

func createOpenStackCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateOpenstackCloudCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.OpenstackCloudCredentialAPI.OpenstackCreate(context.Background()).
		CreateOpenstackCloudCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create OpenStack cloud credential", "OpenStack cloud credential created successfully")
}

func updateOpenStackCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateOpenStackCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.OpenstackCloudCredentialAPI.OpenstackUpdate(context.Background()).
		UpdateOpenStackCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update OpenStack cloud credential", "OpenStack cloud credential updated successfully")
}

func deleteCloudCredential(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.CloudCredentialAPI.CloudcredentialsDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete cloud credential", fmt.Sprintf("Cloud credential %d deleted successfully", args.ID))
}

func makeCloudCredentialDefault(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewCredentialMakeDefaultCommand()
	command.SetId(args.ID)

	httpResponse, err := client.Client.CloudCredentialAPI.CloudcredentialsMakeDefault(context.Background()).
		CredentialMakeDefaultCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "make cloud credential default", fmt.Sprintf("Cloud credential %d set as default successfully", args.ID))
}

func lockCloudCredential(client *taikungoclient.Client, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
	command := taikuncore.NewCloudLockManagerCommand()
	command.SetId(args.ID)
	command.SetMode(args.Mode)

	httpResponse, err := client.Client.CloudCredentialAPI.CloudcredentialsLockManager(context.Background()).
		CloudLockManagerCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock cloud credential", fmt.Sprintf("Cloud credential %d lock mode updated to %q", args.ID, args.Mode))
}
