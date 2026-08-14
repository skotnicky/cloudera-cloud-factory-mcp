package main

import (
	"context"
	"fmt"
	"time"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

// waitForVirtualClusterReady polls the virtual cluster status until it's ready or times out
func waitForVirtualClusterReady(client *taikungoclient.Client, parentProjectID int32, clusterName string, timeoutSeconds int32) error {
	ctx := context.Background()
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeoutSeconds == 0 {
		timeout = 15 * time.Minute // Default 15 minutes
	}

	start := time.Now()
	for {
		// Check if we've exceeded the timeout
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for virtual cluster '%s' to be ready after %v", clusterName, timeout)
		}

		// Query the virtual cluster status
		req := client.Client.VirtualClusterAPI.VirtualClusterList(ctx, parentProjectID)
		req = req.Search(clusterName)
		virtualClusterList, response, err := req.Execute()

		if err != nil {
			return fmt.Errorf("error checking virtual cluster status: %v", err)
		}

		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("HTTP error checking virtual cluster status: %d", response.StatusCode)
		}

		// Find our specific virtual cluster
		if virtualClusterList != nil && len(virtualClusterList.Data) > 0 {
			for _, vc := range virtualClusterList.Data {
				if vc.Name == clusterName {
					status := string(vc.Status)
					health := string(vc.Health)

					// Log current status
					logger.Printf("Virtual cluster '%s' status: %s, health: %s", clusterName, status, health)

					// Check if cluster is ready
					if status == "Ready" && health == "Healthy" {
						return nil // Success!
					}

					// Check for failure states
					if status == "Failed" || health == "Unhealthy" {
						return fmt.Errorf("virtual cluster '%s' creation failed - status: %s, health: %s", clusterName, status, health)
					}

					// Continue polling if still updating/pending
					break
				}
			}
		}

		// Wait before next poll
		time.Sleep(10 * time.Second)
	}
}

type CreateVirtualClusterArgs struct {
	ProjectID          int32  `json:"projectId" jsonschema:"required,description=The parent project ID for the virtual cluster"`
	Name               string `json:"name" jsonschema:"required,description=The name of the virtual cluster"`
	ExpiredAt          string `json:"expiredAt,omitempty" jsonschema:"description=Expiration date in RFC3339 format (optional)"`
	DeleteOnExpiration bool   `json:"deleteOnExpiration,omitempty" jsonschema:"description=Whether to delete cluster on expiration (default: false)"`
	AlertingProfileID  int32  `json:"alertingProfileId,omitempty" jsonschema:"description=ID of alerting profile to use (optional)"`
	WaitForCreation    bool   `json:"waitForCreation,omitempty" jsonschema:"description=Wait for virtual cluster to be fully created before returning (default: false)"`
	Timeout            int32  `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds when waiting for creation (default: 900)"`
}

type DeleteVirtualClusterArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=The project ID of the virtual cluster to delete"`
}

type ListVirtualClustersArgs struct {
	ParentProjectID int32  `json:"parentProjectId" jsonschema:"required,description=The parent project ID to list virtual clusters from"`
	Limit           int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset          int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search          string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

func createVirtualCluster(client *taikungoclient.Client, args CreateVirtualClusterArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	createCmd := taikuncore.NewCreateVirtualClusterCommand()
	createCmd.SetProjectId(args.ProjectID)
	createCmd.SetName(args.Name)
	createCmd.SetDeleteOnExpiration(args.DeleteOnExpiration)

	if args.ExpiredAt != "" {
		expTime, err := time.Parse(time.RFC3339, args.ExpiredAt)
		if err != nil {
			errorResp := ErrorResponse{
				Error: fmt.Sprintf("Error parsing expiration date: %v", err),
			}
			return createJSONResponse(errorResp), nil
		}
		createCmd.SetExpiredAt(expTime)
	}

	if args.AlertingProfileID != 0 {
		createCmd.SetAlertingProfileId(args.AlertingProfileID)
	}

	response, err := client.Client.VirtualClusterAPI.VirtualClusterCreate(ctx).
		CreateVirtualClusterCommand(*createCmd).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "create virtual cluster"); errorResp != nil {
		return errorResp, nil
	}

	// Wait for creation if requested
	var message string
	if args.WaitForCreation {
		logger.Printf("Waiting for virtual cluster '%s' to be ready...", args.Name)
		err := waitForVirtualClusterReady(client, args.ProjectID, args.Name, args.Timeout)
		if err != nil {
			errorResp := ErrorResponse{
				Error: fmt.Sprintf("Virtual cluster creation initiated but failed during wait: %v", err),
			}
			return createJSONResponse(errorResp), nil
		}
		message = fmt.Sprintf("Virtual cluster '%s' created and is ready in project %d", args.Name, args.ProjectID)
		logger.Printf("Virtual cluster '%s' is now ready", args.Name)
	} else {
		message = fmt.Sprintf("Virtual cluster '%s' creation initiated in project %d", args.Name, args.ProjectID)
	}

	successResp := SuccessResponse{
		Message: message,
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func deleteVirtualCluster(client *taikungoclient.Client, args DeleteVirtualClusterArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	deleteCmd := taikuncore.NewDeleteVirtualClusterCommand()
	deleteCmd.SetProjectId(args.ProjectID)

	response, err := client.Client.VirtualClusterAPI.VirtualClusterDelete(ctx).
		DeleteVirtualClusterCommand(*deleteCmd).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "delete virtual cluster"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Virtual cluster with project ID %d deleted successfully", args.ProjectID),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func listVirtualClusters(client *taikungoclient.Client, args ListVirtualClustersArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	req := client.Client.VirtualClusterAPI.VirtualClusterList(ctx, args.ParentProjectID)

	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}

	virtualClusterList, response, err := req.Execute()
	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "list virtual clusters"); errorResp != nil {
		return errorResp, nil
	}

	// Prepare response data
	var virtualClusters []VirtualClusterSummary
	for _, virtualCluster := range virtualClusterList.Data {
		vcSummary := VirtualClusterSummary{
			ID:                 virtualCluster.Id,
			Name:               virtualCluster.Name,
			Status:             string(virtualCluster.Status),
			Health:             string(virtualCluster.Health),
			KubernetesVersion:  virtualCluster.KubernetesVersion,
			CreatedAt:          virtualCluster.CreatedAt,
			CreatedBy:          auditUserName(virtualCluster.CreatedBy),
			DeleteOnExpiration: virtualCluster.DeleteOnExpiration,
			Organization:       virtualCluster.OrganizationName,
			IsLocked:           virtualCluster.IsLocked,
			HasKubeconfig:      virtualCluster.HasKubeConfigFile,
		}

		if virtualCluster.ExpiredAt != "" {
			vcSummary.ExpiresAt = virtualCluster.ExpiredAt
		}

		virtualClusters = append(virtualClusters, vcSummary)
	}

	// Create response
	message := fmt.Sprintf("Found %d virtual clusters in parent project %d", len(virtualClusters), args.ParentProjectID)
	if len(virtualClusters) == 0 {
		message = fmt.Sprintf("No virtual clusters found in parent project %d", args.ParentProjectID)
	}

	listResp := VirtualClusterListResponse{
		VirtualClusters: virtualClusters,
		Total:           len(virtualClusters),
		ParentProjectID: args.ParentProjectID,
		Message:         message,
	}

	return createJSONResponse(listResp), nil
}
