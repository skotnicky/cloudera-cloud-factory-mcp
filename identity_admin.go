package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

func listDomains(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	if args.Offset > 0 && args.CursorID == 0 {
		return createJSONResponse(ErrorResponse{
			Error: "list-domains uses cursor-based pagination; use cursorId instead of offset",
		}), nil
	}

	query := url.Values{}
	if args.Limit > 0 {
		query.Set("Limit", fmt.Sprintf("%d", args.Limit))
	}
	if args.CursorID > 0 {
		query.Set("CursorId", fmt.Sprintf("%d", args.CursorID))
	}
	if args.Search != "" {
		query.Set("Search", args.Search)
	}

	result, httpResponse, err := performAuthenticatedJSONRequest[accountListCursorPaginatedResponse](client, http.MethodGet, "/api/v1/accounts", query, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list domains"); errorResp != nil {
		return errorResp, nil
	}

	items := []accountListItem{}
	total := 0
	response := map[string]interface{}{}
	if result != nil {
		items = result.Data
		total = int(result.TotalCount)
		if total == 0 {
			total = len(items)
		}
		response["hasMore"] = result.HasMore
		if result.NextCursor != nil {
			response["nextCursor"] = *result.NextCursor
		}
	}

	response["domains"] = items
	response["total"] = total
	response["message"] = listMessage(total, "domain", "domains")
	return createJSONResponse(response), nil
}

func createDomain(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[createAccountCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	id, httpResponse, err := performAuthenticatedJSONRequest[int32](client, http.MethodPost, "/api/v1/accounts/create", nil, command)
	if id == nil {
		return finalizeIDOperation(0, httpResponse, err, "create domain", "Domain created successfully", "id")
	}
	return finalizeIDOperation(*id, httpResponse, err, "create domain", "Domain created successfully", "id")
}

func getDomainDetails(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := performAuthenticatedJSONRequest[accountDetailsDTO](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d", args.ID), nil, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get domain details"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]interface{}{
		"domain":   result,
		"message":  "Retrieved domain details",
		"success":  true,
		"domainId": args.ID,
	}), nil
}

func updateDomain(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[updateAccountCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	id, httpResponse, err := performAuthenticatedJSONRequest[int32](client, http.MethodPost, "/api/v1/accounts/update", nil, command)
	if id == nil {
		return finalizeIDOperation(0, httpResponse, err, "update domain", "Domain updated successfully", "id")
	}
	return finalizeIDOperation(*id, httpResponse, err, "update domain", "Domain updated successfully", "id")
}

func deleteDomain(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := performAuthenticatedRequest(client, http.MethodDelete, fmt.Sprintf("/api/v1/accounts/%d", args.ID), nil, nil)
	return finalizeAction(httpResponse, err, "delete domain", fmt.Sprintf("Domain %d deleted successfully", args.ID))
}

func listOrganizations(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	if args.DomainID > 0 {
		query := url.Values{}
		query.Set("AccountId", fmt.Sprintf("%d", args.DomainID))
		if args.Search != "" {
			query.Set("Search", args.Search)
		}

		items, httpResponse, err := performAuthenticatedJSONRequest[[]taikuncore.OrganizationDropdownDto](client, http.MethodGet, "/api/v1/organizations/list", query, nil)
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list organizations"); errorResp != nil {
			return errorResp, nil
		}

		if items == nil {
			empty := []taikuncore.OrganizationDropdownDto{}
			return createListResponse("organizations", empty, 0, listMessage(0, "organization", "organizations")), nil
		}
		return createListResponse("organizations", compactJSON(*items), len(*items), listMessage(len(*items), "organization", "organizations")), nil
	}

	req := client.Client.OrganizationsAPI.OrganizationsList(context.Background())
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
	if errorResp := checkResponse(httpResponse, "list organizations"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.OrganizationDetailsDto{}
	total := 0
	if result != nil {
		items = result.GetData()
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
	}
	return createListResponse("organizations", compactJSON(items), total, listMessage(total, "organization", "organizations")), nil
}

func createOrganization(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := translateDomainIDPayload(args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.ApiResponse](client, http.MethodPost, "/api/v1/organizations", nil, command)
	if apiResp == nil {
		return finalizeAPIOperation(nil, httpResponse, err, "create organization", "Organization created successfully")
	}
	return finalizeAPIOperation(apiResp, httpResponse, err, "create organization", "Organization created successfully")
}

func getOrganizationDetails(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.OrganizationsAPI.OrganizationsDetails(context.Background()).
		OrganizationId(args.ID).
		Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get organization details"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]interface{}{
		"organization":   compactJSON(result),
		"message":        "Retrieved organization details",
		"success":        true,
		"organizationId": args.ID,
	}), nil
}

func updateOrganization(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateOrganizationCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.OrganizationsAPI.OrganizationsUpdate(context.Background()).
		UpdateOrganizationCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update organization", "Organization updated successfully")
}

func deleteOrganization(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.OrganizationsAPI.OrganizationsDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete organization", fmt.Sprintf("Organization %d deleted successfully", args.ID))
}

func listIdentityGroups(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	if args.DomainID <= 0 {
		return createJSONResponse(ErrorResponse{
			Error: "domainId is required to list identity groups",
		}), nil
	}

	if args.Offset > 0 || args.CursorID > 0 {
		return createJSONResponse(ErrorResponse{
			Error: "list-identity-groups does not support offset or cursorId pagination on the live API",
		}), nil
	}

	query := url.Values{}
	query.Set("AccountId", fmt.Sprintf("%d", args.DomainID))
	if args.Search != "" {
		query.Set("Search", args.Search)
	}
	result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.GroupList](client, http.MethodGet, "/api/v1/groups", query, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list identity groups"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.GroupListItem{}
	if result != nil {
		items = result.GetData()
	}
	return createListResponse("identityGroups", compactJSON(items), len(items), listMessage(len(items), "identity group", "identity groups")), nil
}

func createIdentityGroup(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := translateDomainIDPayload(args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	id, httpResponse, err := performAuthenticatedJSONRequest[int32](client, http.MethodPost, "/api/v1/groups/create", nil, command)
	if id == nil {
		return finalizeIDOperation(0, httpResponse, err, "create identity group", "Identity group created successfully", "id")
	}
	return finalizeIDOperation(*id, httpResponse, err, "create identity group", "Identity group created successfully", "id")
}

func getIdentityGroupDetails(client *taikungoclient.Client, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.GroupDetailsDto](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/groups/%d", args.DomainID, args.ID), nil, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get identity group details"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]interface{}{
		"identityGroup": compactJSON(result),
		"message":       "Retrieved identity group details",
		"success":       true,
		"domainId":      args.DomainID,
		"id":            args.ID,
	}), nil
}

func listIdentityGroupOrganizations(client *taikungoclient.Client, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.GroupDetailsDto](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/groups/%d", args.DomainID, args.ID), nil, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list identity group organizations"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.GroupedOrganizationDto{}
	if result != nil {
		items = result.GetOrganizations()
	}
	return createListResponse("organizations", compactJSON(items), len(items), listMessage(len(items), "organization", "organizations")), nil
}

func listIdentityGroupUsers(client *taikungoclient.Client, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.GroupDetailsDto](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/groups/%d", args.DomainID, args.ID), nil, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list identity group users"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.GroupedUserDto{}
	if result != nil {
		items = result.GetUsers()
	}
	return createListResponse("users", compactJSON(items), len(items), listMessage(len(items), "user", "users")), nil
}

func listAvailableIdentityGroupOrganizations(client *taikungoclient.Client, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
	query := url.Values{}
	query.Set("GroupId", fmt.Sprintf("%d", args.ID))
	items, httpResponse, err := performAuthenticatedJSONRequest[[]taikuncore.CommonDropdownDto](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/organizations/available", args.DomainID), query, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list available identity group organizations"); errorResp != nil {
		return errorResp, nil
	}

	if items == nil {
		empty := []taikuncore.CommonDropdownDto{}
		return createListResponse("availableOrganizations", empty, 0, listMessage(0, "available organization", "available organizations")), nil
	}
	return createListResponse("availableOrganizations", compactJSON(*items), len(*items), listMessage(len(*items), "available organization", "available organizations")), nil
}

func addOrganizationsToIdentityGroup(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[[]taikuncore.CreateGroupOrganizationDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.GroupsAPI.GroupsAddOrganizations(context.Background(), args.ID).
		CreateGroupOrganizationDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "add organizations to identity group", fmt.Sprintf("Organizations added to identity group %d successfully", args.ID))
}

func updateIdentityGroup(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateGroupDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.GroupsAPI.GroupsUpdate(context.Background(), args.ID).
		UpdateGroupDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update identity group", fmt.Sprintf("Identity group %d updated successfully", args.ID))
}

func deleteIdentityGroup(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.GroupsAPI.GroupsDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete identity group", fmt.Sprintf("Identity group %d deleted successfully", args.ID))
}

func updateIdentityGroupOrganization(client *taikungoclient.Client, args GroupOrganizationPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateGroupOrganizationDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.GroupsAPI.GroupsUpdateGroupOrganization(context.Background(), args.GroupID, args.OrganizationID).
		UpdateGroupOrganizationDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update identity group organization", fmt.Sprintf("Organization %d in identity group %d updated successfully", args.OrganizationID, args.GroupID))
}

func removeOrganizationsFromIdentityGroup(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteOrganizationFromGroupCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.GroupsAPI.GroupsDeleteOrganizations(context.Background()).
		DeleteOrganizationFromGroupCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "remove organizations from identity group", fmt.Sprintf("Organizations removed from identity group %d successfully", command.GetGroupId()))
}

func listAvailableIdentityGroupUsers(client *taikungoclient.Client, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
	query := url.Values{}
	query.Set("GroupId", fmt.Sprintf("%d", args.ID))
	items, httpResponse, err := performAuthenticatedJSONRequest[[]taikuncore.CommonStringBasedDropdownDto](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/users/available", args.DomainID), query, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list available identity group users"); errorResp != nil {
		return errorResp, nil
	}

	if items == nil {
		empty := []taikuncore.CommonStringBasedDropdownDto{}
		return createListResponse("availableUsers", empty, 0, listMessage(0, "available user", "available users")), nil
	}
	return createListResponse("availableUsers", compactJSON(*items), len(*items), listMessage(len(*items), "available user", "available users")), nil
}

func addUsersToIdentityGroup(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[[]taikuncore.CreateGroupUserDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.GroupsAPI.GroupsAddUsers(context.Background(), args.ID).
		CreateGroupUserDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "add users to identity group", fmt.Sprintf("Users added to identity group %d successfully", args.ID))
}

func removeUsersFromIdentityGroup(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteUserFromGroupCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.GroupsAPI.GroupsDeleteUsers(context.Background()).
		DeleteUserFromGroupCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "remove users from identity group", fmt.Sprintf("Users removed from identity group %d successfully", command.GetGroupId()))
}

func listUsers(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	if args.DomainID <= 0 {
		return createJSONResponse(ErrorResponse{
			Error: "domainId is required to list users",
		}), nil
	}

	if args.Offset > 0 && args.CursorID > 0 {
		return createJSONResponse(ErrorResponse{
			Error: "use either offset or cursorId when listing users, not both",
		}), nil
	}

	query := url.Values{}
	if args.Limit > 0 {
		query.Set("Limit", fmt.Sprintf("%d", args.Limit))
	}
	if args.Search != "" {
		query.Set("Search", args.Search)
	}

	if args.Offset > 0 {
		query.Set("Offset", fmt.Sprintf("%d", args.Offset))
		result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.UserOffsetPaginationDropdownList](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/user/offset-based/dropdown", args.DomainID), query, nil)
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "list users"); errorResp != nil {
			return errorResp, nil
		}

		items := []taikuncore.UserBriefDto{}
		total := 0
		response := map[string]interface{}{}
		if result != nil {
			items = result.GetData()
			total = int(result.GetTotalCount())
			if total == 0 {
				total = len(items)
			}
			response["hasMore"] = result.GetHasMore()
			response["offset"] = result.GetOffset()
			if result.NextOffset.IsSet() && result.NextOffset.Get() != nil {
				response["nextOffset"] = result.GetNextOffset()
			}
		}

		response["users"] = compactJSON(items)
		response["total"] = total
		response["message"] = listMessage(total, "user", "users")
		return createJSONResponse(response), nil
	}

	if args.CursorID > 0 {
		query.Set("CursorId", fmt.Sprintf("%d", args.CursorID))
	}
	result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.UserBriefDtoCursorTimestampPaginatedResponse](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/user/dropdown", args.DomainID), query, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list users"); errorResp != nil {
		return errorResp, nil
	}

	items := []taikuncore.UserBriefDto{}
	total := 0
	response := map[string]interface{}{}
	if result != nil {
		items = result.GetData()
		total = int(result.GetTotalCount())
		if total == 0 {
			total = len(items)
		}
		response["hasMore"] = result.GetHasMore()
		if result.NextCursor.IsSet() && result.NextCursor.Get() != nil {
			response["nextCursor"] = result.GetNextCursor()
		}
	}

	response["users"] = compactJSON(items)
	response["total"] = total
	response["message"] = listMessage(total, "user", "users")
	return createJSONResponse(response), nil
}

func createUser(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := translateDomainIDPayload(args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	apiResp, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.ApiResponse](client, http.MethodPost, "/api/v1/users/create", nil, command)
	if apiResp == nil {
		return finalizeAPIOperation(nil, httpResponse, err, "create user", "User created successfully")
	}
	return finalizeAPIOperation(apiResp, httpResponse, err, "create user", "User created successfully")
}

func getUserDetails(client *taikungoclient.Client, args DomainScopedStringIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := performAuthenticatedJSONRequest[taikuncore.UserDetailsDto](client, http.MethodGet, fmt.Sprintf("/api/v1/accounts/%d/users/%s", args.DomainID, url.PathEscape(args.ID)), nil, nil)
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "get user details"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(map[string]interface{}{
		"user":     compactJSON(result),
		"message":  "Retrieved user details",
		"success":  true,
		"domainId": args.DomainID,
		"id":       args.ID,
	}), nil
}

func updateUser(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.UpdateUserCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}

	httpResponse, err := client.Client.UsersAPI.UsersUpdateUser(context.Background()).
		UpdateUserCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update user", "User updated successfully")
}

func deleteUser(client *taikungoclient.Client, args StringIDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.UsersAPI.UsersDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete user", fmt.Sprintf("User %s deleted successfully", args.ID))
}
