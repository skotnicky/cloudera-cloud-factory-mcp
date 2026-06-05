package main

import (
	"context"
	"fmt"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

type RepositorySummary struct {
	AppRepoID         int32  `json:"appRepoId"`
	RepositoryID      string `json:"repositoryId,omitempty"`
	Name              string `json:"name"`
	DisplayName       string `json:"displayName,omitempty"`
	URL               string `json:"url"`
	OrganizationName  string `json:"organizationName"`
	Disabled          bool   `json:"disabled"`
	VerifiedPublisher bool   `json:"verifiedPublisher"`
	Official          bool   `json:"official"`
	IsBound           bool   `json:"isBound"`
	IsPrivate         *bool  `json:"isPrivate,omitempty"`
	IsTaikun          bool   `json:"isTaikun"`
	HasCatalogApp     bool   `json:"hasCatalogApp"`
	PasswordProtected *bool  `json:"passwordProtected,omitempty"`
}

type RepositoryListResponse struct {
	Repositories []RepositorySummary `json:"repositories"`
	Total        int                 `json:"total"`
	Limit        int32               `json:"limit,omitempty"`
	Offset       int32               `json:"offset,omitempty"`
	Message      string              `json:"message"`
}

func listRepositories(client *taikungoclient.Client, args ListRepositoriesArgs) (*mcp_golang.ToolResponse, error) {
	// Bound by default: this keeps fetchRepositories to a single page and skips
	// the extra full private-repo fetch+merge that the unbounded path performs.
	if args.Limit <= 0 {
		args.Limit = 100
	}

	allRepositories, total, errorResp := fetchRepositories(client, args)
	if errorResp != nil {
		return errorResp, nil
	}

	// The live API excludes private repositories from the default unfiltered result set.
	// Merge in the current Robot User's private repos so a fresh import is discoverable.
	if args.IsPrivate == nil && args.OrganizationID == 0 && args.Offset == 0 && args.Limit == 0 {
		robotCtx := getRobotUserContext()
		if robotCtx.OrganizationID > 0 {
			privateArgs := args
			privateOnly := true
			privateArgs.IsPrivate = &privateOnly
			privateArgs.OrganizationID = robotCtx.OrganizationID

			privateRepositories, _, privateError := fetchRepositories(client, privateArgs)
			if privateError == nil {
				allRepositories = mergeRepositories(allRepositories, privateRepositories)
				total = len(allRepositories)
			}
		}
	}

	summaries := make([]RepositorySummary, 0, len(allRepositories))
	for _, repo := range allRepositories {
		summaries = append(summaries, repositorySummaryFromDTO(repo))
	}

	if total == 0 {
		total = len(summaries)
	}

	message := listMessage(total, "repository", "repositories")
	if total > 0 && len(summaries) == 0 {
		message = fmt.Sprintf("No repositories found on the requested page (total matches: %d)", total)
	}

	return createJSONResponse(RepositoryListResponse{
		Repositories: summaries,
		Total:        total,
		Limit:        args.Limit,
		Offset:       args.Offset,
		Message:      message,
	}), nil
}

func importRepository(client *taikungoclient.Client, args ImportRepositoryArgs) (*mcp_golang.ToolResponse, error) {
	if args.Name == "" {
		return createJSONResponse(ErrorResponse{Error: "name is required"}), nil
	}
	if args.URL == "" {
		return createJSONResponse(ErrorResponse{Error: "url is required"}), nil
	}

	ctx := context.Background()
	command := taikuncore.NewImportRepoCommand()
	command.SetName(args.Name)
	command.SetUrl(args.URL)
	if args.OrganizationID > 0 {
		command.SetOrganizationId(args.OrganizationID)
	}
	if args.Username != "" {
		command.SetUsername(args.Username)
	}
	if args.Password != "" {
		command.SetPassword(args.Password)
	}

	response, err := client.Client.AppRepositoriesAPI.RepositoryImport(ctx).
		ImportRepoCommand(*command).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "import repository"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(SuccessResponse{
		Message: fmt.Sprintf("Repository '%s' imported successfully", args.Name),
		Success: true,
	}), nil
}

func bindRepository(client *taikungoclient.Client, args BindRepositoryArgs) (*mcp_golang.ToolResponse, error) {
	if args.RepositoryID == "" && args.Name == "" {
		return createJSONResponse(ErrorResponse{
			Error: "provide repositoryId or name to bind a repository",
		}), nil
	}

	repositoryName := args.Name
	repositoryOrganizationName := args.RepositoryOrganizationName
	if args.RepositoryID != "" {
		repo, errorResp := lookupRepositoryByID(client, args.RepositoryID, args.OrganizationID)
		if errorResp != nil {
			return errorResp, nil
		}
		repositoryName = repo.GetName()
		repositoryOrganizationName = repo.GetOrganizationName()
	}

	filter := taikuncore.NewFilteringElementDto()
	filter.SetName(repositoryName)
	if repositoryOrganizationName != "" {
		filter.SetOrganizationName(repositoryOrganizationName)
	}

	command := taikuncore.NewBindAppRepositoryCommand()
	command.SetFilteringElements([]taikuncore.FilteringElementDto{*filter})
	if args.OrganizationID > 0 {
		command.SetOrganizationId(args.OrganizationID)
	}

	ctx := context.Background()
	response, err := client.Client.AppRepositoriesAPI.RepositoryBind(ctx).
		BindAppRepositoryCommand(*command).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "bind repository"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(SuccessResponse{
		Message: fmt.Sprintf("Repository '%s' bound successfully", repositoryName),
		Success: true,
	}), nil
}

func unbindRepository(client *taikungoclient.Client, args UnbindRepositoryArgs) (*mcp_golang.ToolResponse, error) {
	ids := combineRepositoryIDs(args.RepositoryID, args.RepositoryIDs)
	if len(ids) == 0 {
		return createJSONResponse(ErrorResponse{
			Error: "provide repositoryId or repositoryIds to unbind repositories",
		}), nil
	}

	command := taikuncore.NewUnbindAppRepositoryCommand()
	command.SetIds(ids)
	if args.OrganizationID > 0 {
		command.SetOrganizationId(args.OrganizationID)
	}

	ctx := context.Background()
	response, err := client.Client.AppRepositoriesAPI.RepositoryUnbind(ctx).
		UnbindAppRepositoryCommand(*command).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "unbind repository"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(SuccessResponse{
		Message: fmt.Sprintf("Unbound %d repository(s) successfully", len(ids)),
		Success: true,
	}), nil
}

func deleteRepository(client *taikungoclient.Client, args DeleteRepositoryArgs) (*mcp_golang.ToolResponse, error) {
	appRepoID := args.AppRepoID
	repositoryName := ""
	if appRepoID <= 0 {
		if args.RepositoryID == "" {
			return createJSONResponse(ErrorResponse{
				Error: "provide appRepoId or repositoryId to delete a repository",
			}), nil
		}

		repo, errorResp := lookupRepositoryByID(client, args.RepositoryID, args.OrganizationID)
		if errorResp != nil {
			return errorResp, nil
		}
		appRepoID = repo.GetAppRepoId()
		repositoryName = repo.GetName()
	}

	command := taikuncore.NewDeleteRepositoryCommand()
	command.SetAppRepoId(appRepoID)

	ctx := context.Background()
	response, err := client.Client.AppRepositoriesAPI.RepositoryDelete(ctx).
		DeleteRepositoryCommand(*command).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "delete repository"); errorResp != nil {
		return errorResp, nil
	}

	message := fmt.Sprintf("Repository appRepoId %d deleted successfully", appRepoID)
	if repositoryName != "" {
		message = fmt.Sprintf("Repository '%s' deleted successfully", repositoryName)
	}

	return createJSONResponse(SuccessResponse{
		Message: message,
		Success: true,
	}), nil
}

func updateRepositoryPassword(client *taikungoclient.Client, args UpdateRepositoryPasswordArgs) (*mcp_golang.ToolResponse, error) {
	if args.RepositoryID == "" {
		return createJSONResponse(ErrorResponse{
			Error: "repositoryId is required",
		}), nil
	}
	if args.Username == "" {
		return createJSONResponse(ErrorResponse{
			Error: "username is required",
		}), nil
	}
	if args.Password == "" {
		return createJSONResponse(ErrorResponse{
			Error: "password is required",
		}), nil
	}

	organizationID := args.OrganizationID
	if organizationID == 0 {
		robotCtx := getRobotUserContext()
		organizationID = robotCtx.OrganizationID
	}
	if organizationID == 0 {
		return createJSONResponse(ErrorResponse{
			Error: "organizationId is required when the Robot User context does not expose one",
		}), nil
	}

	command := taikuncore.NewUpdateRepoPasswordCommand()
	command.SetRepositoryId(args.RepositoryID)
	command.SetUsername(args.Username)
	command.SetPassword(args.Password)
	command.SetOrganizationId(organizationID)

	ctx := context.Background()
	response, err := client.Client.AppRepositoriesAPI.RepositoryUpdatePassword(ctx).
		UpdateRepoPasswordCommand(*command).
		Execute()
	if err != nil {
		return createError(response, err), nil
	}
	if errorResp := checkResponse(response, "update repository password"); errorResp != nil {
		return errorResp, nil
	}

	return createJSONResponse(SuccessResponse{
		Message: fmt.Sprintf("Repository credentials updated successfully for repositoryId '%s'", args.RepositoryID),
		Success: true,
	}), nil
}

func applyRepositoryAvailableFilters(req taikuncore.ApiRepositoryAvailableListRequest, args ListRepositoriesArgs) taikuncore.ApiRepositoryAvailableListRequest {
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.ID != "" {
		req = req.Id(args.ID)
	}
	if args.IsPrivate != nil {
		req = req.IsPrivate(*args.IsPrivate)
	}
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	return req
}

func repositorySummaryFromDTO(repo taikuncore.ArtifactRepositoryDto) RepositorySummary {
	summary := RepositorySummary{
		AppRepoID:         repo.GetAppRepoId(),
		Name:              repo.GetName(),
		URL:               repo.GetUrl(),
		OrganizationName:  repo.GetOrganizationName(),
		Disabled:          repo.GetDisabled(),
		VerifiedPublisher: repo.GetVerifiedPublisher(),
		Official:          repo.GetOfficial(),
		IsBound:           repo.GetIsBound(),
		IsTaikun:          repo.GetIsTaikun(),
		HasCatalogApp:     repo.GetHasCatalogApp(),
	}

	if value, ok := repo.GetRepositoryIdOk(); ok && value != nil {
		summary.RepositoryID = *value
	}
	if value, ok := repo.GetDisplayNameOk(); ok && value != nil {
		summary.DisplayName = *value
	}
	if value, ok := repo.GetIsPrivateOk(); ok && value != nil {
		isPrivate := *value
		summary.IsPrivate = &isPrivate
	}
	if value, ok := repo.GetPasswordProtectedOk(); ok && value != nil {
		passwordProtected := *value
		summary.PasswordProtected = &passwordProtected
	}

	return summary
}

func lookupRepositoryByID(client *taikungoclient.Client, repositoryID string, organizationID int32) (*taikuncore.ArtifactRepositoryDto, *mcp_golang.ToolResponse) {
	lookupArgs := ListRepositoriesArgs{
		ID:    repositoryID,
		Limit: 1,
	}
	if organizationID > 0 {
		lookupArgs.OrganizationID = organizationID
	}

	repositories, _, errorResp := fetchRepositories(client, lookupArgs)
	if errorResp != nil {
		return nil, errorResp
	}
	if len(repositories) > 0 {
		repo := repositories[0]
		return &repo, nil
	}

	privateOnly := true
	lookupArgs.IsPrivate = &privateOnly
	if lookupArgs.OrganizationID == 0 {
		robotCtx := getRobotUserContext()
		if robotCtx.OrganizationID > 0 {
			lookupArgs.OrganizationID = robotCtx.OrganizationID
		}
	}

	repositories, _, errorResp = fetchRepositories(client, lookupArgs)
	if errorResp != nil {
		return nil, errorResp
	}
	if len(repositories) > 0 {
		repo := repositories[0]
		return &repo, nil
	}

	return nil, createJSONResponse(ErrorResponse{
		Error: fmt.Sprintf("Repository with ID '%s' not found", repositoryID),
	})
}

func combineRepositoryIDs(repositoryID string, repositoryIDs []string) []string {
	seen := make(map[string]struct{})
	combined := make([]string, 0, len(repositoryIDs)+1)

	appendID := func(id string) {
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		combined = append(combined, id)
	}

	appendID(repositoryID)
	for _, id := range repositoryIDs {
		appendID(id)
	}

	return combined
}

func fetchRepositories(client *taikungoclient.Client, args ListRepositoriesArgs) ([]taikuncore.ArtifactRepositoryDto, int, *mcp_golang.ToolResponse) {
	ctx := context.Background()
	const pageSize int32 = 100

	var allRepositories []taikuncore.ArtifactRepositoryDto
	total := 0
	nextOffset := args.Offset

	for {
		req := client.Client.AppRepositoriesAPI.RepositoryAvailableList(ctx)
		req = applyRepositoryAvailableFilters(req, args)

		requestLimit := args.Limit
		if requestLimit <= 0 {
			requestLimit = pageSize
		}
		req = req.Limit(requestLimit)
		if nextOffset > 0 {
			req = req.Offset(nextOffset)
		}

		result, response, err := req.Execute()
		if err != nil {
			return nil, 0, createError(response, err)
		}
		if errorResp := checkResponse(response, "list repositories"); errorResp != nil {
			return nil, 0, errorResp
		}

		if result == nil || len(result.Data) == 0 {
			break
		}

		if total == 0 {
			total = int(result.GetTotalCount())
			if total == 0 {
				total = len(result.Data)
			}
		}

		allRepositories = append(allRepositories, result.Data...)
		if args.Limit > 0 {
			break
		}
		if int32(len(result.Data)) < requestLimit {
			break
		}

		nextOffset += requestLimit
		if total > 0 && len(allRepositories) >= total {
			break
		}
	}

	if total == 0 {
		total = len(allRepositories)
	}

	return allRepositories, total, nil
}

func mergeRepositories(base, extra []taikuncore.ArtifactRepositoryDto) []taikuncore.ArtifactRepositoryDto {
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]taikuncore.ArtifactRepositoryDto, 0, len(base)+len(extra))

	appendRepository := func(repo taikuncore.ArtifactRepositoryDto) {
		key := repositoryKey(repo)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, repo)
	}

	for _, repo := range base {
		appendRepository(repo)
	}
	for _, repo := range extra {
		appendRepository(repo)
	}

	return merged
}

func repositoryKey(repo taikuncore.ArtifactRepositoryDto) string {
	if value, ok := repo.GetRepositoryIdOk(); ok && value != nil && *value != "" {
		return "repositoryId:" + *value
	}
	return fmt.Sprintf("appRepoId:%d", repo.GetAppRepoId())
}
