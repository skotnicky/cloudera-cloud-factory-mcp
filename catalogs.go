package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

type CreateCatalogArgs struct {
	Name           string `json:"name" jsonschema:"required,description=The name of the catalog"`
	Description    string `json:"description" jsonschema:"required,description=The description of the catalog"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID (required for Robot User / account-wide operations when the API enforces it)"`
}

type ListCatalogsArgs struct {
	Limit  int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
	ID     int32  `json:"id,omitempty" jsonschema:"description=Get specific catalog by ID (optional)"`
}

type UpdateCatalogArgs struct {
	CatalogID   int32  `json:"catalogId" jsonschema:"required,description=The ID of the catalog to update"`
	Name        string `json:"name" jsonschema:"required,description=The new name of the catalog"`
	Description string `json:"description" jsonschema:"required,description=The new description of the catalog"`
}

type DeleteCatalogArgs struct {
	CatalogID int32 `json:"catalogId" jsonschema:"required,description=The ID of the catalog to delete"`
}

type BindProjectsToCatalogArgs struct {
	CatalogID  int32   `json:"catalogId" jsonschema:"required,description=The ID of the catalog"`
	ProjectIDs []int32 `json:"projectIds" jsonschema:"required,description=Array of project IDs to bind to the catalog"`
}

type UnbindProjectsFromCatalogArgs struct {
	CatalogID  int32   `json:"catalogId" jsonschema:"required,description=The ID of the catalog"`
	ProjectIDs []int32 `json:"projectIds" jsonschema:"required,description=Array of project IDs to unbind from the catalog"`
}

// resolveOrganizationIDForCatalog returns an organization ID for catalog APIs that require it (e.g. Robot User).
// If explicit is set, it is returned. Otherwise prefer the current Robot User context and fail closed
// when multiple organizations are visible instead of guessing.
func resolveOrganizationIDForCatalog(client *taikungoclient.Client, ctx context.Context, explicit int32) (int32, *mcp_golang.ToolResponse) {
	if explicit > 0 {
		return explicit, nil
	}

	robotCtx := getRobotUserContext()
	if robotCtx.OrganizationID > 0 {
		return robotCtx.OrganizationID, nil
	}

	result, httpResponse, err := client.Client.OrganizationsAPI.OrganizationsList(ctx).Limit(25).Execute()
	if err != nil {
		return 0, createError(httpResponse, err)
	}
	if errorResp := checkResponse(httpResponse, "list organizations"); errorResp != nil {
		return 0, errorResp
	}
	if result != nil {
		if len(result.GetData()) == 1 {
			return result.GetData()[0].GetId(), nil
		}
		if len(result.GetData()) > 1 {
			return 0, createJSONResponse(ErrorResponse{
				Error: "multiple organizations are visible to this Robot User; pass organizationId explicitly",
			})
		}
	}

	accountID := robotCtx.AccountID
	if accountID == 0 {
		acctQuery := url.Values{}
		acctQuery.Set("Limit", "25")
		acctResp, httpResponse2, err2 := performAuthenticatedJSONRequest[accountListCursorPaginatedResponse](client, http.MethodGet, "/api/v1/accounts", acctQuery, nil)
		if err2 != nil {
			return 0, createError(httpResponse2, err2)
		}
		if errorResp := checkResponse(httpResponse2, "list accounts"); errorResp != nil {
			return 0, errorResp
		}
		if acctResp == nil || len(acctResp.Data) == 0 {
			return 0, createJSONResponse(ErrorResponse{
				Error: "could not resolve organizationId: pass organizationId explicitly or ensure at least one organization exists",
			})
		}
		if len(acctResp.Data) > 1 {
			return 0, createJSONResponse(ErrorResponse{
				Error: "multiple accounts are visible to this Robot User; pass organizationId explicitly",
			})
		}
		accountID = acctResp.Data[0].ID
	}

	orgQuery := url.Values{}
	orgQuery.Set("AccountId", fmt.Sprintf("%d", accountID))
	items, httpResponse3, err3 := performAuthenticatedJSONRequest[[]taikuncore.OrganizationDropdownDto](client, http.MethodGet, "/api/v1/organizations/list", orgQuery, nil)
	if err3 != nil {
		return 0, createError(httpResponse3, err3)
	}
	if errorResp := checkResponse(httpResponse3, "list organizations by account"); errorResp != nil {
		return 0, errorResp
	}
	if items == nil || len(*items) == 0 {
		return 0, createJSONResponse(ErrorResponse{
			Error: "could not resolve organizationId: no organizations for this account — pass organizationId explicitly",
		})
	}
	if len(*items) > 1 {
		return 0, createJSONResponse(ErrorResponse{
			Error: "multiple organizations are available for this account; pass organizationId explicitly",
		})
	}
	return (*items)[0].GetId(), nil
}

func createCatalog(client *taikungoclient.Client, args CreateCatalogArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	orgID, errResp := resolveOrganizationIDForCatalog(client, ctx, args.OrganizationID)
	if errResp != nil {
		return errResp, nil
	}

	createCmd := taikuncore.NewCreateCatalogCommand()
	createCmd.SetName(args.Name)
	createCmd.SetDescription(args.Description)
	createCmd.SetOrganizationId(orgID)

	response, err := client.Client.CatalogAPI.CatalogCreate(ctx).
		CreateCatalogCommand(*createCmd).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "create catalog"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Catalog '%s' created successfully", args.Name),
		Success: true,
	}
	return createJSONResponse(successResp), nil
}

func listCatalogs(client *taikungoclient.Client, args ListCatalogsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	req := client.Client.CatalogAPI.CatalogList(ctx)

	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.ID != 0 {
		req = req.Id(args.ID)
	}

	catalogList, response, err := req.Execute()
	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "list catalogs"); errorResp != nil {
		return errorResp, nil
	}

	// Prepare response data
	type CatalogSummary struct {
		ID             int32  `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		IsLocked       bool   `json:"isLocked"`
		IsDefault      bool   `json:"isDefault"`
		OrganizationID int32  `json:"organizationId,omitempty"`
		BoundProjects  []struct {
			ID   int32  `json:"id"`
			Name string `json:"name"`
		} `json:"boundProjects,omitempty"`
		BoundApplications []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"boundApplications,omitempty"`
		PackageIds []string `json:"packageIds,omitempty"`
	}

	var catalogs []CatalogSummary
	if catalogList != nil && len(catalogList.Data) > 0 {
		for _, catalog := range catalogList.Data {
			catalogSummary := CatalogSummary{
				ID:          catalog.GetId(),
				Name:        catalog.GetName(),
				Description: catalog.GetDescription(),
				IsLocked:    catalog.GetIsLocked(),
				IsDefault:   catalog.GetIsDefault(),
			}

			if catalog.GetOrganizationId() != 0 {
				catalogSummary.OrganizationID = catalog.GetOrganizationId()
			}

			if len(catalog.BoundProjects) > 0 {
				for _, project := range catalog.BoundProjects {
					catalogSummary.BoundProjects = append(catalogSummary.BoundProjects, struct {
						ID   int32  `json:"id"`
						Name string `json:"name"`
					}{
						ID:   project.GetId(),
						Name: project.GetName(),
					})
				}
			}

			if len(catalog.BoundApplications) > 0 {
				for _, app := range catalog.BoundApplications {
					catalogSummary.BoundApplications = append(catalogSummary.BoundApplications, struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					}{
						ID:   app.GetPackageId(),
						Name: app.GetName(),
					})
				}
			}

			if len(catalog.PackageIds) > 0 {
				catalogSummary.PackageIds = catalog.PackageIds
			}

			catalogs = append(catalogs, catalogSummary)
		}
	}

	message := fmt.Sprintf("Found %d catalogs", len(catalogs))
	if len(catalogs) == 0 {
		message = "No catalogs found"
	}

	listResp := struct {
		Catalogs []CatalogSummary `json:"catalogs"`
		Total    int              `json:"total"`
		Message  string           `json:"message"`
	}{
		Catalogs: catalogs,
		Total:    len(catalogs),
		Message:  message,
	}

	return createJSONResponse(listResp), nil
}

func deleteCatalog(client *taikungoclient.Client, args DeleteCatalogArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	response, err := client.Client.CatalogAPI.CatalogDelete(ctx, args.CatalogID).Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "delete catalog"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Catalog ID %d deleted successfully", args.CatalogID),
		Success: true,
	}
	return createJSONResponse(successResp), nil
}

func bindProjectsToCatalog(client *taikungoclient.Client, args BindProjectsToCatalogArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	response, err := client.Client.CatalogAPI.CatalogAddProject(ctx, args.CatalogID).
		RequestBody(args.ProjectIDs).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "bind projects to catalog"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Successfully bound %d projects to catalog ID %d", len(args.ProjectIDs), args.CatalogID),
		Success: true,
	}
	return createJSONResponse(successResp), nil
}

func unbindProjectsFromCatalog(client *taikungoclient.Client, args UnbindProjectsFromCatalogArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	response, err := client.Client.CatalogAPI.CatalogDeleteProject(ctx, args.CatalogID).
		RequestBody(args.ProjectIDs).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "unbind projects from catalog"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Successfully unbound %d projects from catalog ID %d", len(args.ProjectIDs), args.CatalogID),
		Success: true,
	}
	return createJSONResponse(successResp), nil
}

func buildCreateCatalogAppCommand(catalogID int32, repository, packageName, version string, parameters []AppParameter) *taikuncore.CreateCatalogAppCommand {
	createCmd := taikuncore.NewCreateCatalogAppCommand()
	createCmd.SetCatalogId(catalogID)
	createCmd.SetRepoName(repository)
	createCmd.SetPackageName(packageName)
	createCmd.SetVersion(version)

	params := make([]taikuncore.CatalogAppParamsDto, 0, len(parameters))
	for _, param := range parameters {
		p := taikuncore.NewCatalogAppParamsDto()
		p.SetKey(param.Key)
		p.SetValue(param.Value)
		params = append(params, *p)
	}
	// Match the working UI payload by always sending an explicit parameters array,
	// including the empty-array case.
	createCmd.SetParameters(params)
	return createCmd
}

func addAppToCatalogWithParameters(client *taikungoclient.Client, args AddAppToCatalogWithParametersArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	createCmd := buildCreateCatalogAppCommand(args.CatalogID, args.Repository, args.PackageName, args.Version, args.Parameters)

	_, response, err := client.Client.CatalogAppAPI.CatalogAppCreate(ctx).
		CreateCatalogAppCommand(*createCmd).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "add application to catalog with parameters"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Application '%s' from repository '%s' added to catalog ID %d with %d parameter overrides", args.PackageName, args.Repository, args.CatalogID, len(args.Parameters)),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func resolveCatalogAppForRemoval(applications []CatalogAppSummary, args RemoveAppFromCatalogArgs) (CatalogAppSummary, error) {
	matches := make([]CatalogAppSummary, 0)
	packageName := strings.TrimSpace(args.PackageName)
	repository := strings.TrimSpace(args.Repository)

	for _, app := range applications {
		if app.CatalogID != args.CatalogID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(app.Name), packageName) {
			continue
		}
		if repository != "" && !strings.EqualFold(strings.TrimSpace(app.Repository), repository) {
			continue
		}
		matches = append(matches, app)
	}

	switch len(matches) {
	case 0:
		if repository != "" {
			return CatalogAppSummary{}, fmt.Errorf("application '%s' from repository '%s' was not found in catalog ID %d", args.PackageName, args.Repository, args.CatalogID)
		}
		return CatalogAppSummary{}, fmt.Errorf("application '%s' was not found in catalog ID %d", args.PackageName, args.CatalogID)
	case 1:
		return matches[0], nil
	default:
		repositories := make([]string, 0, len(matches))
		for _, app := range matches {
			if app.Repository != "" {
				repositories = append(repositories, app.Repository)
			}
		}
		if len(repositories) == 0 {
			return CatalogAppSummary{}, fmt.Errorf("multiple catalog applications matched '%s' in catalog ID %d; specify repository to disambiguate", args.PackageName, args.CatalogID)
		}
		sort.Strings(repositories)
		return CatalogAppSummary{}, fmt.Errorf("multiple catalog applications matched '%s' in catalog ID %d; specify repository. Matching repositories: %s", args.PackageName, args.CatalogID, strings.Join(repositories, ", "))
	}
}

func removeAppFromCatalog(client *taikungoclient.Client, args RemoveAppFromCatalogArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	catalogApps, response, err := client.Client.CatalogAppAPI.CatalogAppList(ctx).
		CatalogId(args.CatalogID).
		Search(args.PackageName).
		Limit(100).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "list catalog applications for removal"); errorResp != nil {
		return errorResp, nil
	}

	applications := make([]CatalogAppSummary, 0)
	if catalogApps != nil {
		for _, app := range catalogApps.Data {
			summary := CatalogAppSummary{}
			if app.CatalogAppId != nil {
				summary.ID = *app.CatalogAppId
			}
			summary.Name = app.GetName()
			if app.RepoName.IsSet() && app.RepoName.Get() != nil {
				summary.Repository = *app.RepoName.Get()
			}
			if app.CatalogId != nil {
				summary.CatalogID = *app.CatalogId
			}
			if app.CatalogName.IsSet() && app.CatalogName.Get() != nil {
				summary.CatalogName = *app.CatalogName.Get()
			}
			applications = append(applications, summary)
		}
	}

	matchedApp, matchErr := resolveCatalogAppForRemoval(applications, args)
	if matchErr != nil {
		return createJSONResponse(ErrorResponse{Error: matchErr.Error()}), nil
	}

	deleteResponse, err := client.Client.CatalogAppAPI.CatalogAppDelete(ctx, matchedApp.ID).Execute()
	if err != nil {
		return createError(deleteResponse, err), nil
	}
	if errorResp := checkResponse(deleteResponse, "remove application from catalog"); errorResp != nil {
		return errorResp, nil
	}

	message := fmt.Sprintf("Application '%s' removed from catalog ID %d", matchedApp.Name, args.CatalogID)
	if matchedApp.Repository != "" {
		message = fmt.Sprintf("Application '%s' from repository '%s' removed from catalog ID %d", matchedApp.Name, matchedApp.Repository, args.CatalogID)
	}
	return createJSONResponse(SuccessResponse{
		Message: message,
		Success: true,
	}), nil
}

func listCatalogApps(client *taikungoclient.Client, args ListCatalogAppsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	req := client.Client.CatalogAppAPI.CatalogAppList(ctx)

	// Add catalogId filter only if provided
	if args.CatalogID != 0 {
		req = req.CatalogId(args.CatalogID)
	}

	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}

	catalogAppList, response, err := req.Execute()
	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "list catalog applications"); errorResp != nil {
		return errorResp, nil
	}

	// Prepare response data
	var applications []CatalogAppSummary
	if catalogAppList != nil && len(catalogAppList.Data) > 0 {
		for _, app := range catalogAppList.Data {
			appSummary := CatalogAppSummary{}

			// Handle nullable/pointer fields safely
			if app.CatalogAppId != nil {
				appSummary.ID = *app.CatalogAppId
			}

			if app.GetName() != "" {
				appSummary.Name = app.GetName()
			}

			if app.RepoName.IsSet() && app.RepoName.Get() != nil {
				appSummary.Repository = *app.RepoName.Get()
			}

			if app.CatalogId != nil {
				appSummary.CatalogID = *app.CatalogId
			}

			if app.CatalogName.IsSet() && app.CatalogName.Get() != nil {
				appSummary.CatalogName = *app.CatalogName.Get()
			}

			applications = append(applications, appSummary)
		}
	}

	// Create response
	var message string
	if args.CatalogID != 0 {
		message = fmt.Sprintf("Found %d applications in catalog ID %d", len(applications), args.CatalogID)
		if len(applications) == 0 {
			message = fmt.Sprintf("No applications found in catalog ID %d", args.CatalogID)
		}
	} else {
		message = fmt.Sprintf("Found %d applications across all catalogs", len(applications))
		if len(applications) == 0 {
			message = "No applications found in any catalog"
		}
	}

	listResp := CatalogAppListResponse{
		Applications: applications,
		Total:        len(applications),
		CatalogID:    args.CatalogID,
		Message:      message,
	}

	return createJSONResponse(listResp), nil
}

func getCatalogAppParameters(client *taikungoclient.Client, args GetCatalogAppParamsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	cmd := taikuncore.NewGetCatalogAppValueAutocompleteCommand()
	if args.CatalogAppID == 0 && (args.PackageID == "" || args.Version == "") {
		errorResp := ErrorResponse{
			Error: "Provide catalogAppId or both packageId and version",
		}
		return createJSONResponse(errorResp), nil
	}

	if args.CatalogAppID != 0 {
		cmd.SetCatalogAppId(args.CatalogAppID)
	}

	if (args.PackageID == "" || args.Version == "") && args.CatalogAppID != 0 {
		details, response, err := client.Client.CatalogAppAPI.CatalogAppDetails(ctx, args.CatalogAppID).Execute()
		if err != nil {
			return createError(response, err), nil
		}
		if errorResp := checkResponse(response, "get catalog app details"); errorResp != nil {
			return errorResp, nil
		}

		if details != nil {
			if value, ok := details.GetPackageIdOk(); ok && value != nil {
				args.PackageID = *value
			}
			if value, ok := details.GetVersionOk(); ok && value != nil {
				args.Version = *value
			}
		}
	}

	if args.PackageID == "" {
		errorResp := ErrorResponse{
			Error: "Package ID is required when catalogAppId is not provided",
		}
		return createJSONResponse(errorResp), nil
	}
	cmd.SetPackageId(args.PackageID)

	if args.Version == "" {
		errorResp := ErrorResponse{
			Error: "Version is required when catalogAppId is not provided",
		}
		return createJSONResponse(errorResp), nil
	}
	cmd.SetVersion(args.Version)

	availableParams, response, err := client.Client.PackageAPI.PackageValueAutocomplete(ctx).
		GetCatalogAppValueAutocompleteCommand(*cmd).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "get catalog app available parameters"); errorResp != nil {
		return errorResp, nil
	}

	var addedParams []taikuncore.CatalogAppParamsDetailsDto
	if args.CatalogAppID != 0 {
		addedParams, response, err = client.Client.CatalogAppAPI.CatalogAppParamDetails(ctx, args.CatalogAppID).Execute()
		if err != nil {
			return createError(response, err), nil
		}
		if errorResp := checkResponse(response, "get catalog app added parameters"); errorResp != nil {
			return errorResp, nil
		}
	}

	type AvailableParam struct {
		Key          string   `json:"key,omitempty"`
		Value        string   `json:"value,omitempty"`
		Description  string   `json:"description,omitempty"`
		Type         string   `json:"type,omitempty"`
		IsQuestion   *bool    `json:"isQuestion,omitempty"`
		Options      []string `json:"options,omitempty"`
		IsTaikunLink *bool    `json:"isTaikunLink,omitempty"`
	}

	type AddedParam struct {
		ID                          int32  `json:"id,omitempty"`
		CatalogAppName              string `json:"catalogAppName,omitempty"`
		Key                         string `json:"key,omitempty"`
		Value                       string `json:"value,omitempty"`
		IsEditableWhenInstalling    *bool  `json:"isEditableWhenInstalling,omitempty"`
		IsEditableAfterInstallation *bool  `json:"isEditableAfterInstallation,omitempty"`
		IsMandatory                 *bool  `json:"isMandatory,omitempty"`
		HasJsonSchema               *bool  `json:"hasJsonSchema,omitempty"`
		IsTaikunLink                *bool  `json:"isTaikunLink,omitempty"`
	}

	available := make([]AvailableParam, 0, len(availableParams))
	for _, param := range availableParams {
		if args.IsTaikunLink != nil {
			if param.IsTaikunLink == nil || *param.IsTaikunLink != *args.IsTaikunLink {
				continue
			}
		}

		detail := AvailableParam{}
		if value, ok := param.GetKeyOk(); ok && value != nil {
			detail.Key = *value
		}
		if value, ok := param.GetValueOk(); ok && value != nil {
			detail.Value = *value
		}
		if value, ok := param.GetDescriptionOk(); ok && value != nil {
			detail.Description = *value
		}
		if value, ok := param.GetTypeOk(); ok && value != nil {
			detail.Type = string(*value)
		}
		if value, ok := param.GetIsQuestionOk(); ok {
			detail.IsQuestion = value
		}
		if len(param.Options) > 0 {
			detail.Options = param.Options
		}
		if value, ok := param.GetIsTaikunLinkOk(); ok {
			detail.IsTaikunLink = value
		}

		available = append(available, detail)
	}

	added := make([]AddedParam, 0, len(addedParams))
	for _, param := range addedParams {
		if args.IsTaikunLink != nil {
			if value, ok := param.GetIsTaikunLinkOk(); ok {
				if value == nil || *value != *args.IsTaikunLink {
					continue
				}
			} else {
				continue
			}
		}

		detail := AddedParam{}
		if param.Id != nil {
			detail.ID = *param.Id
		}
		if value, ok := param.GetCatalogAppNameOk(); ok && value != nil {
			detail.CatalogAppName = *value
		}
		if value, ok := param.GetKeyOk(); ok && value != nil {
			detail.Key = *value
		}
		if value, ok := param.GetValueOk(); ok && value != nil {
			detail.Value = *value
		}
		if value, ok := param.GetIsEditableWhenInstallingOk(); ok {
			detail.IsEditableWhenInstalling = value
		}
		if value, ok := param.GetIsEditableAfterInstallationOk(); ok {
			detail.IsEditableAfterInstallation = value
		}
		if value, ok := param.GetIsMandatoryOk(); ok {
			detail.IsMandatory = value
		}
		if value, ok := param.GetHasJsonSchemaOk(); ok {
			detail.HasJsonSchema = value
		}
		if value, ok := param.GetIsTaikunLinkOk(); ok {
			detail.IsTaikunLink = value
		}

		added = append(added, detail)
	}

	message := fmt.Sprintf("Found %d available parameters and %d added parameters", len(available), len(added))
	if len(available) == 0 && len(added) == 0 {
		message = "No parameters found"
	}

	listResp := struct {
		CatalogAppID   int32            `json:"catalogAppId,omitempty"`
		PackageID      string           `json:"packageId,omitempty"`
		Version        string           `json:"version,omitempty"`
		Available      []AvailableParam `json:"available"`
		Added          []AddedParam     `json:"added"`
		TotalAvailable int              `json:"totalAvailable"`
		TotalAdded     int              `json:"totalAdded"`
		Message        string           `json:"message"`
	}{
		CatalogAppID:   args.CatalogAppID,
		PackageID:      args.PackageID,
		Version:        args.Version,
		Available:      available,
		Added:          added,
		TotalAvailable: len(available),
		TotalAdded:     len(added),
		Message:        message,
	}

	return createJSONResponse(listResp), nil
}

func updateCatalogAppParameters(client *taikungoclient.Client, args SetCatalogAppDefaultParamsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	updateCmd := taikuncore.NewEditCatalogAppParamCommand()
	updateCmd.SetCatalogAppId(args.CatalogAppID)

	mergeWithExisting := true
	if args.MergeWithExisting != nil {
		mergeWithExisting = *args.MergeWithExisting
	}

	paramMap := map[string]string{}
	if mergeWithExisting {
		existing, response, err := client.Client.CatalogAppAPI.CatalogAppParamDetails(ctx, args.CatalogAppID).Execute()
		if err != nil {
			return createError(response, err), nil
		}
		if errorResp := checkResponse(response, "get catalog app default parameters"); errorResp != nil {
			return errorResp, nil
		}

		for _, param := range existing {
			if key, ok := param.GetKeyOk(); ok && key != nil {
				if value, ok := param.GetValueOk(); ok && value != nil {
					paramMap[*key] = *value
				}
			}
		}
	}

	for _, param := range args.Parameters {
		paramMap[param.Key] = param.Value
	}

	keys := make([]string, 0, len(paramMap))
	for key := range paramMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]taikuncore.CatalogAppParamsDto, 0, len(keys))
	for _, key := range keys {
		p := taikuncore.NewCatalogAppParamsDto()
		p.SetKey(key)
		p.SetValue(paramMap[key])
		params = append(params, *p)
	}
	updateCmd.SetParameters(params)

	response, err := client.Client.CatalogAppAPI.CatalogAppEditParams(ctx).
		EditCatalogAppParamCommand(*updateCmd).
		Execute()

	if err != nil {
		return createError(response, err), nil
	}

	if errorResp := checkResponse(response, "update catalog app parameters"); errorResp != nil {
		return errorResp, nil
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Catalog app ID %d parameters updated successfully", args.CatalogAppID),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func fetchAvailablePackages(client *taikungoclient.Client, search string, startOffset int32) ([]taikuncore.AvailablePackagesDto, int, *mcp_golang.ToolResponse) {
	ctx := context.Background()

	const pageSize int32 = 100
	var allPackages []taikuncore.AvailablePackagesDto
	total := 0
	for pageOffset := startOffset; ; pageOffset += pageSize {
		req := client.Client.PackageAPI.PackageList(ctx).
			Limit(pageSize).
			Offset(pageOffset)
		if search != "" {
			req = req.Search(search)
		}

		packageList, response, err := req.Execute()
		if err != nil {
			return nil, 0, createError(response, err)
		}

		if errorResp := checkResponse(response, "list available packages"); errorResp != nil {
			return nil, 0, errorResp
		}

		if packageList == nil || len(packageList.Data) == 0 {
			break
		}

		if total == 0 {
			total = int(packageList.GetTotalCount())
		}
		allPackages = append(allPackages, packageList.Data...)
		if int32(len(packageList.Data)) < pageSize {
			break
		}
	}
	if total == 0 {
		total = len(allPackages)
	}
	return allPackages, total, nil
}

func listAvailableApps(client *taikungoclient.Client, args ListAvailableAppsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	// Bound by default so the no-filter path takes the single-page branch
	// instead of looping every package page into memory.
	if args.Limit <= 0 {
		args.Limit = 100
	}

	type AvailableAppInfo struct {
		PackageID      string `json:"packageId,omitempty"`
		Name           string `json:"name,omitempty"`
		Repository     string `json:"repository,omitempty"`
		Version        string `json:"version,omitempty"`
		AppVersion     string `json:"appVersion,omitempty"`
		Description    string `json:"description,omitempty"`
		CatalogAppID   int32  `json:"catalogAppId,omitempty"`
		CatalogID      int32  `json:"catalogId,omitempty"`
		InstalledCount int32  `json:"installedInstanceCount,omitempty"`
		IsAdded        *bool  `json:"isAdded,omitempty"`
		Deprecated     *bool  `json:"deprecated,omitempty"`
	}

	mapAvailableAppInfo := func(pkg taikuncore.AvailablePackagesDto) AvailableAppInfo {
		app := AvailableAppInfo{}

		if value, ok := pkg.GetPackageIdOk(); ok && value != nil {
			app.PackageID = *value
		}
		if value, ok := pkg.GetNameOk(); ok && value != nil {
			app.Name = *value
		}
		if pkg.Repository != nil && pkg.Repository.Name.IsSet() && pkg.Repository.Name.Get() != nil {
			app.Repository = *pkg.Repository.Name.Get()
		}
		if value, ok := pkg.GetVersionOk(); ok && value != nil {
			app.Version = *value
		}
		if value, ok := pkg.GetAppVersionOk(); ok && value != nil {
			app.AppVersion = *value
		}
		if value, ok := pkg.GetDescriptionOk(); ok && value != nil {
			app.Description = *value
		}
		if value, ok := pkg.GetCatalogAppIdOk(); ok && value != nil {
			app.CatalogAppID = *value
		}
		if value, ok := pkg.GetCatalogIdOk(); ok && value != nil {
			app.CatalogID = *value
		}
		if value, ok := pkg.GetInstalledInstanceCountOk(); ok && value != nil {
			app.InstalledCount = *value
		}
		if value, ok := pkg.GetIsAddedOk(); ok {
			app.IsAdded = value
		}
		if pkg.Deprecated != nil {
			app.Deprecated = pkg.Deprecated
		}
		return app
	}

	if args.Repository == "" && args.Limit > 0 {
		req := client.Client.PackageAPI.PackageList(ctx).
			Limit(args.Limit)
		if args.Offset > 0 {
			req = req.Offset(args.Offset)
		}
		if args.Search != "" {
			req = req.Search(args.Search)
		}

		packageList, response, err := req.Execute()
		if err != nil {
			return createError(response, err), nil
		}
		if errorResp := checkResponse(response, "list available apps"); errorResp != nil {
			return errorResp, nil
		}

		apps := []AvailableAppInfo{}
		total := 0
		if packageList != nil {
			total = int(packageList.GetTotalCount())
			for _, pkg := range packageList.Data {
				apps = append(apps, mapAvailableAppInfo(pkg))
			}
		}
		if total == 0 {
			total = len(apps)
		}

		message := fmt.Sprintf("Found %d available apps", total)
		if total == 0 {
			message = "No available apps found"
		} else if len(apps) == 0 {
			message = fmt.Sprintf("No available apps found on the requested page (total matches: %d)", total)
		}

		listResp := struct {
			Apps    []AvailableAppInfo `json:"apps"`
			Total   int                `json:"total"`
			Message string             `json:"message"`
		}{
			Apps:    apps,
			Total:   total,
			Message: message,
		}

		return createJSONResponse(listResp), nil
	}

	startOffset := int32(0)
	if args.Repository == "" && args.Offset > 0 {
		startOffset = args.Offset
	}
	allPackages, total, errorResp := fetchAvailablePackages(client, args.Search, startOffset)
	if errorResp != nil {
		return errorResp, nil
	}

	var apps []AvailableAppInfo
	for _, pkg := range allPackages {
		app := mapAvailableAppInfo(pkg)

		if args.Repository != "" && app.Repository != args.Repository {
			continue
		}

		apps = append(apps, app)
	}

	appPage := apps
	if args.Repository != "" {
		total = len(apps)
		appPage = applyOffsetLimit(apps, args.Offset, args.Limit)
	}
	message := fmt.Sprintf("Found %d available apps", total)
	if total == 0 {
		message = "No available apps found"
	} else if len(appPage) == 0 {
		message = fmt.Sprintf("No available apps found on the requested page (total matches: %d)", total)
	}
	if args.Repository != "" {
		message += fmt.Sprintf(" in repository '%s'", args.Repository)
	}

	listResp := struct {
		Apps    []AvailableAppInfo `json:"apps"`
		Total   int                `json:"total"`
		Message string             `json:"message"`
	}{
		Apps:    appPage,
		Total:   total,
		Message: message,
	}

	return createJSONResponse(listResp), nil
}
