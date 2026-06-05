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
	CloudType string `json:"cloudType" jsonschema:"required,description=Cloud type: aws, azure, openstack, proxmox, vsphere, zadara (and generic-kubernetes for update)"`
	Payload   string `json:"payload" jsonschema:"required,description=JSON payload matching the underlying create/update command for the chosen cloudType"`
}

func normalizeCloudType(cloudType string) string {
	normalized := strings.ToLower(strings.TrimSpace(cloudType))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "generickubernetes", "generic-kubernetes", "k8s", "kubernetes":
		return "generic-kubernetes"
	default:
		return normalized
	}
}

// googleCloudCredentialNotSupportedResponse explains that Google/GCP credentials
// are not manageable through the JSON-payload create/update tools: the Google
// create API requires a multipart service-account key file upload and has no
// update endpoint, so it does not fit this tool's command-body model.
func googleCloudCredentialNotSupportedResponse(operation string) *mcp_golang.ToolResponse {
	return createJSONResponse(ErrorResponse{
		Error:   fmt.Sprintf("Google/GCP cloud credentials are not supported by %s", operation),
		Details: "The Google cloud credential API requires a multipart service-account key file upload (and has no update endpoint), so it cannot be driven through a JSON payload here. Create or update Google credentials in the Cloudera Cloud Factory UI.",
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
	case "proxmox":
		return createProxmoxCloudCredential(client, payload)
	case "vsphere":
		return createVSphereCloudCredential(client, payload)
	case "zadara":
		return createZadaraCloudCredential(client, payload)
	case "google", "gcp":
		return googleCloudCredentialNotSupportedResponse("create-cloud-credential"), nil
	default:
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("unsupported cloudType %q for create-cloud-credential", args.CloudType),
			Details: "Supported cloudType values: aws, azure, openstack, proxmox, vsphere, zadara.",
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
	case "proxmox":
		return updateProxmoxCloudCredential(client, payload)
	case "vsphere":
		return updateVSphereCloudCredential(client, payload)
	case "zadara":
		return updateZadaraCloudCredential(client, payload)
	case "generic-kubernetes":
		return updateGenericKubernetesCloudCredential(client, payload)
	case "google", "gcp":
		return googleCloudCredentialNotSupportedResponse("update-cloud-credential"), nil
	default:
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("unsupported cloudType %q for update-cloud-credential", args.CloudType),
			Details: "Supported cloudType values: aws, azure, openstack, proxmox, vsphere, zadara, generic-kubernetes.",
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

func createProxmoxCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateProxmoxCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.ProxmoxCloudCredentialAPI.ProxmoxCreate(context.Background()).
		CreateProxmoxCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create Proxmox cloud credential", "Proxmox cloud credential created successfully")
}

func updateProxmoxCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateProxmoxCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.ProxmoxCloudCredentialAPI.ProxmoxUpdate(context.Background()).
		UpdateProxmoxCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update Proxmox cloud credential", "Proxmox cloud credential updated successfully")
}

func createVSphereCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateVsphereCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.VsphereCloudCredentialAPI.VsphereCreate(context.Background()).
		CreateVsphereCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create vSphere cloud credential", "vSphere cloud credential created successfully")
}

func updateVSphereCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateVsphereCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.VsphereCloudCredentialAPI.VsphereUpdate(context.Background()).
		UpdateVsphereCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update vSphere cloud credential", "vSphere cloud credential updated successfully")
}

func createZadaraCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateZadaraCloudCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := client.Client.ZadaraCloudCredentialAPI.ZadaraCreate(context.Background()).
		CreateZadaraCloudCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create Zadara cloud credential", "Zadara cloud credential created successfully")
}

func updateZadaraCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateZadaraCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.ZadaraCloudCredentialAPI.ZadaraUpdate(context.Background()).
		UpdateZadaraCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update Zadara cloud credential", "Zadara cloud credential updated successfully")
}

func updateGenericKubernetesCloudCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateGenericKubernetesCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	result, httpResponse, err := client.Client.GenericKubernetesCloudCredentialAPI.GenericKubernetesUpdate(context.Background()).
		UpdateGenericKubernetesCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "update generic Kubernetes cloud credential"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"result":  compactJSON(result),
		"message": "Generic Kubernetes cloud credential updated successfully",
		"success": true,
	}), nil
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
