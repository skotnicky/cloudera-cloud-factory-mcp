package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func listCloudCredentials(client *taikungoclient.Client, args ListCloudCredentialsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	// Switch to CloudcredentialsOrgList which is more standard and reliable
	req := client.Client.CloudCredentialAPI.CloudcredentialsOrgList(ctx)

	if args.Search != "" {
		req = req.Search(args.Search)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "list cloud credentials"); errorResp != nil {
		return errorResp, nil
	}

	var credentials []CloudCredentialSummary
	for _, cred := range result {
		summary := CloudCredentialSummary{
			ID:        cred.GetId(),
			Name:      cred.GetFullName(),
			CloudType: string(cred.GetCloudType()),
		}
		// Organization name is not directly available in this endpoint,
		// but we can provide the ID if we want, or leave it empty.
		// CloudCredentialSummary struct has OrganizationName, so we'll just leave it empty
		// or show the ID as a string if necessary.
		if cred.HasOrganizationId() {
			summary.OrganizationName = fmt.Sprintf("Organization ID: %d", cred.GetOrganizationId())
		}

		credentials = append(credentials, summary)
	}

	// Apply manual pagination if requested, since OrgList doesn't support it in API
	total := len(credentials)
	start := int(args.Offset)
	if start > total {
		start = total
	}

	end := total
	if args.Limit > 0 {
		end = start + int(args.Limit)
		if end > total {
			end = total
		}
	}

	pagedCredentials := credentials[start:end]

	response := CloudCredentialListResponse{
		Credentials: pagedCredentials,
		Total:       total,
		Message:     fmt.Sprintf("Found %d cloud credentials", len(pagedCredentials)),
	}

	return createJSONResponse(response), nil
}
