package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

type ImageListArgs struct {
	Provider      string `json:"provider" jsonschema:"required,description=Cloud provider or platform such as aws azure google openstack openshift proxmox vsphere or zadara"`
	Mode          string `json:"mode" jsonschema:"required,description=Image list mode such as public common or personal"`
	CloudID       int32  `json:"cloudId,omitempty" jsonschema:"description=Cloud credential ID when required by the provider API"`
	ProjectID     int32  `json:"projectId,omitempty" jsonschema:"description=Project ID when the image endpoint supports project filtering"`
	Limit         int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return when supported"`
	Offset        int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip when supported"`
	Search        string `json:"search,omitempty" jsonschema:"description=Search term when supported"`
	SortBy        string `json:"sortBy,omitempty" jsonschema:"description=Field name to sort by when supported"`
	SortDirection string `json:"sortDirection,omitempty" jsonschema:"description=Sort direction such as asc or desc when supported"`
	PublisherName string `json:"publisherName,omitempty" jsonschema:"description=Azure publisher name for azure public image lookup"`
	Offer         string `json:"offer,omitempty" jsonschema:"description=Azure offer for azure public image lookup"`
	Sku           string `json:"sku,omitempty" jsonschema:"description=Azure sku for azure public image lookup"`
	Type          string `json:"type,omitempty" jsonschema:"description=Google image type for google public image lookup"`
	Payload       string `json:"payload,omitempty" jsonschema:"description=JSON payload (AwsImagesPostListCommand) for providers that require a POST body. REQUIRED for aws public images - they are listed from a request body (owners/filters/architecture), not from cloudId alone; cloudId is ignored for aws public mode."`
}

func listImages(client *taikungoclient.Client, args ImageListArgs) (*mcp_golang.ToolResponse, error) {
	provider := strings.ToLower(strings.TrimSpace(args.Provider))
	mode := strings.ToLower(strings.TrimSpace(args.Mode))

	switch provider {
	case "aws":
		return listAWSImages(client, args, mode)
	case "azure":
		return listAzureImages(client, args, mode)
	case "google", "gcp":
		return listGoogleImages(client, args, mode)
	case "openshift":
		return listOpenShiftImages(client, args)
	case "openstack":
		return listOpenStackImages(client, args)
	case "proxmox":
		return listProxmoxImages(client, args)
	case "vsphere":
		return listVSphereImages(client, args)
	case "zadara":
		return listZadaraImages(client, args, mode)
	default:
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("unsupported image provider %q", args.Provider),
		}), nil
	}
}

func listAWSImages(client *taikungoclient.Client, args ImageListArgs, mode string) (*mcp_golang.ToolResponse, error) {
	switch mode {
	case "common":
		req := client.Client.ImagesAPI.ImagesAwsCommonImages(context.Background(), args.CloudID)
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}
		items, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list common AWS images"); errorResp != nil {
			return errorResp, nil
		}
		return createListResponse("images", compactJSON(items), len(items), listMessage(len(items), "image", "images")), nil
	case "personal":
		req := client.Client.ImagesAPI.ImagesAwsPersonalImages(context.Background(), args.CloudID)
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}
		items, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list personal AWS images"); errorResp != nil {
			return errorResp, nil
		}
		return createListResponse("images", compactJSON(items), len(items), listMessage(len(items), "image", "images")), nil
	case "public":
		if strings.TrimSpace(args.Payload) == "" {
			return createJSONResponse(ErrorResponse{
				Error:   "aws public images require a payload body",
				Details: "Provide a JSON payload (AwsImagesPostListCommand) with owners/filters/architecture; aws public images are listed from a request body, not from cloudId alone.",
			}), nil
		}
		command, errorResp := decodePayload[taikuncore.AwsImagesPostListCommand](args.Payload)
		if errorResp != nil {
			return errorResp, nil
		}
		result, httpResponse, err := client.Client.ImagesAPI.ImagesAwsImagesList(context.Background()).
			AwsImagesPostListCommand(*command).
			Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list AWS public images"); errorResp != nil {
			return errorResp, nil
		}
		return createJSONResponse(map[string]interface{}{
			"images":  compactJSON(result),
			"message": "Loaded AWS public images",
			"success": true,
		}), nil
	default:
		return createJSONResponse(ErrorResponse{Error: "aws mode must be one of public, common, or personal"}), nil
	}
}

func listAzureImages(client *taikungoclient.Client, args ImageListArgs, mode string) (*mcp_golang.ToolResponse, error) {
	switch mode {
	case "common":
		req := client.Client.ImagesAPI.ImagesAzureCommonImages(context.Background(), args.CloudID)
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}
		items, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list common Azure images"); errorResp != nil {
			return errorResp, nil
		}
		return createListResponse("images", compactJSON(items), len(items), listMessage(len(items), "image", "images")), nil
	case "personal":
		req := client.Client.ImagesAPI.ImagesAzurePersonalImages(context.Background(), args.CloudID)
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}
		items, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list personal Azure images"); errorResp != nil {
			return errorResp, nil
		}
		return createListResponse("images", compactJSON(items), len(items), listMessage(len(items), "image", "images")), nil
	case "public":
		result, httpResponse, err := client.Client.ImagesAPI.ImagesAzureImages(
			context.Background(), args.CloudID, args.PublisherName, args.Offer, args.Sku,
		).Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list Azure public images"); errorResp != nil {
			return errorResp, nil
		}
		return createJSONResponse(map[string]interface{}{
			"images":  compactJSON(result),
			"message": "Loaded Azure public images",
			"success": true,
		}), nil
	default:
		return createJSONResponse(ErrorResponse{Error: "azure mode must be one of public, common, or personal"}), nil
	}
}

func listGoogleImages(client *taikungoclient.Client, args ImageListArgs, mode string) (*mcp_golang.ToolResponse, error) {
	switch mode {
	case "common":
		req := client.Client.ImagesAPI.ImagesCommonGoogleImages(context.Background(), args.CloudID)
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}
		items, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list common Google images"); errorResp != nil {
			return errorResp, nil
		}
		return createListResponse("images", compactJSON(items), len(items), listMessage(len(items), "image", "images")), nil
	case "public":
		result, httpResponse, err := client.Client.ImagesAPI.ImagesGoogleImages(context.Background(), args.CloudID, args.Type).Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list Google images"); errorResp != nil {
			return errorResp, nil
		}
		return createJSONResponse(map[string]interface{}{
			"images":  compactJSON(result),
			"message": "Loaded Google images",
			"success": true,
		}), nil
	default:
		return createJSONResponse(ErrorResponse{Error: "google mode must be one of public or common"}), nil
	}
}

func listOpenShiftImages(client *taikungoclient.Client, args ImageListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.ImagesAPI.ImagesOpenshiftImages(context.Background(), args.CloudID)
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
	if args.ProjectID > 0 {
		req = req.ProjectId(args.ProjectID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list OpenShift images"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"images":  compactJSON(result),
		"message": "Loaded OpenShift images",
		"success": true,
	}), nil
}

func listOpenStackImages(client *taikungoclient.Client, args ImageListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.ImagesAPI.ImagesOpenstackImages(context.Background(), args.CloudID)
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
	if args.ProjectID > 0 {
		req = req.ProjectId(args.ProjectID)
	}
	if mode := strings.ToLower(strings.TrimSpace(args.Mode)); mode == "personal" {
		req = req.Personal(true)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list OpenStack images"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"images":  compactJSON(result),
		"message": "Loaded OpenStack images",
		"success": true,
	}), nil
}

func listProxmoxImages(client *taikungoclient.Client, args ImageListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.ImagesAPI.ImagesProxmoxImages(context.Background(), args.CloudID)
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
	if args.ProjectID > 0 {
		req = req.ProjectId(args.ProjectID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list Proxmox images"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"images":  compactJSON(result),
		"message": "Loaded Proxmox images",
		"success": true,
	}), nil
}

func listVSphereImages(client *taikungoclient.Client, args ImageListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.ImagesAPI.ImagesVsphereImages(context.Background(), args.CloudID)
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
	if args.ProjectID > 0 {
		req = req.ProjectId(args.ProjectID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list vSphere images"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"images":  compactJSON(result),
		"message": "Loaded vSphere images",
		"success": true,
	}), nil
}

func listZadaraImages(client *taikungoclient.Client, args ImageListArgs, mode string) (*mcp_golang.ToolResponse, error) {
	switch mode {
	case "personal":
		req := client.Client.ImagesAPI.ImagesZadaraPersonalImages(context.Background(), args.CloudID)
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}
		items, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list personal Zadara images"); errorResp != nil {
			return errorResp, nil
		}
		return createListResponse("images", compactJSON(items), len(items), listMessage(len(items), "image", "images")), nil
	default:
		req := client.Client.ImagesAPI.ImagesZadaraImagesList(context.Background(), args.CloudID)
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
		if args.ProjectID > 0 {
			req = req.ProjectId(args.ProjectID)
		}

		result, httpResponse, err := req.Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list Zadara images"); errorResp != nil {
			return errorResp, nil
		}
		return createJSONResponse(map[string]interface{}{
			"images":  compactJSON(result),
			"message": "Loaded Zadara images",
			"success": true,
		}), nil
	}
}

func getImageDetails(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.ImageByIdCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	details, httpResponse, err := client.Client.ImagesAPI.ImagesImageDetails(context.Background()).
		ImageByIdCommand(*command).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get image details"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"details": compactJSON(details),
		"message": "Loaded image details",
		"success": true,
	}), nil
}

func bindImagesToProject(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.BindImageToProjectCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.ImagesAPI.ImagesBindImagesToProject(context.Background()).
		BindImageToProjectCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "bind images to project", "Images bound to project successfully")
}

func unbindImagesFromProject(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteImageFromProjectCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.ImagesAPI.ImagesUnbindImagesFromProject(context.Background()).
		DeleteImageFromProjectCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "unbind images from project", "Images unbound from project successfully")
}

func listSelectedProjectImages(client *taikungoclient.Client, args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.ImagesAPI.ImagesSelectedImagesForProject(context.Background()).
		ProjectId(args.ProjectID)
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
	if args.FilterBy != "" {
		req = req.FilterBy(args.FilterBy)
	}
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list selected project images"); errorResp != nil {
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
	return createListResponse("images", compactJSON(items), total, listMessage(total, "image", "images")), nil
}
