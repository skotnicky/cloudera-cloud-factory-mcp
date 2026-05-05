package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/tidwall/gjson"
)

type RobotUserCapabilitiesArgs struct{}

type RobotUserContext struct {
	UserID              string   `json:"userId,omitempty"`
	AccountID           int32    `json:"accountId,omitempty"`
	AccountName         string   `json:"accountName,omitempty"`
	AccessKey           string   `json:"accessKey,omitempty"`
	OrganizationID      int32    `json:"organizationId,omitempty"`
	OrganizationName    string   `json:"organizationName,omitempty"`
	CreatedBy           string   `json:"createdBy,omitempty"`
	Name                string   `json:"name,omitempty"`
	Description         string   `json:"description,omitempty"`
	Scopes              []string `json:"scopes"`
	IsActive            bool     `json:"isActive"`
	CreatedAt           string   `json:"createdAt,omitempty"`
	ExpiresAt           string   `json:"expiresAt,omitempty"`
	LastUsedAt          string   `json:"lastUsedAt,omitempty"`
	ScopeDiscoveryError string   `json:"scopeDiscoveryError,omitempty"`
}

type ToolScopeAccess struct {
	Tool           string   `json:"tool"`
	Status         string   `json:"status"`
	RequiredScopes []string `json:"requiredScopes,omitempty"`
	MissingScopes  []string `json:"missingScopes,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type RobotUserCapabilitiesResponse struct {
	RobotUser  RobotUserContext  `json:"robotUser"`
	ToolAccess []ToolScopeAccess `json:"toolAccess"`
	Success    bool              `json:"success"`
	Message    string            `json:"message"`
}

var (
	robotUserContextMu sync.RWMutex
	robotUserContext   RobotUserContext
)

var toolRequiredScopes = map[string][]string{
	"server-version":                 {},
	"refresh-taikun-client":          {},
	"robot-user-capabilities":        {},
	"mcp-lock":                       {},
	"mcp-lock-status":                {},
	"mcp-lock-clear":                 {},
	"create-virtual-cluster":         {"scope:virtual-clusters:write"},
	"delete-virtual-cluster":         {"scope:virtual-clusters:write"},
	"list-virtual-clusters":          {"scope:virtual-clusters:read"},
	"catalog-create":                 {"scope:applications:write"},
	"catalog-list":                   {"scope:applications:read"},
	"catalog-delete":                 {"scope:applications:write"},
	"bind-projects-to-catalog":       {"scope:applications:write"},
	"unbind-projects-from-catalog":   {"scope:applications:write"},
	"available-apps-list":            {"scope:applications:read"},
	"list-repositories":              {"scope:applications:read"},
	"import-repository":              {"scope:applications:write"},
	"bind-repository":                {"scope:applications:write"},
	"unbind-repository":              {"scope:applications:write"},
	"delete-repository":              {"scope:applications:write"},
	"update-repository-password":     {"scope:applications:write"},
	"catalog-app-add":                {"scope:applications:write"},
	"catalog-app-remove":             {"scope:applications:write"},
	"catalog-apps-list":              {"scope:applications:read"},
	"catalog-app-params":             {"scope:applications:read"},
	"catalog-app-defaults-set":       {"scope:applications:write"},
	"app-install":                    {"scope:applications:write"},
	"list-apps":                      {"scope:applications:read"},
	"get-app":                        {"scope:applications:read"},
	"update-app-autosync":            {"scope:applications:write"},
	"update-sync-app":                {"scope:applications:write"},
	"uninstall-app":                  {"scope:applications:write"},
	"wait-for-app":                   {"scope:applications:read"},
	"list-projects":                  {"scope:projects:read"},
	"create-project":                 {"scope:projects:write"},
	"create-cluster":                 {"scope:projects:write", "scope:servers:write", "scope:project-deployments"},
	"delete-project":                 {"scope:projects:write"},
	"wait-for-project":               {"scope:projects:read"},
	"deploy-kubernetes-resources":    {"scope:kubernetes:write"},
	"create-kubeconfig":              {"scope:kubernetes:read"},
	"get-kubeconfig":                 {"scope:kubernetes:read"},
	"list-kubeconfig-roles":          {"scope:kubernetes:read"},
	"list-kubernetes-resource-kinds": {},
	"list-kubernetes-resources":      {"scope:kubernetes:read"},
	"describe-kubernetes-resource":   {"scope:kubernetes:read"},
	"delete-kubernetes-resource":     {"scope:kubernetes:write"},
	"patch-kubernetes-resource":      {"scope:kubernetes:write"},
	"list-cloud-credentials":         {"scope:cloud-credentials:read"},
	"bind-flavors-to-project":        {"scope:flavors:write"},
	"add-server-to-project":          {"scope:servers:write"},
	"commit-project":                 {"scope:project-deployments"},
	"get-project-details":            {"scope:projects:read"},
	"list-flavors":                   {"scope:flavors:read"},
	"list-servers":                   {"scope:servers:read"},
	"delete-servers-from-project":    {"scope:servers:write"},
}

func init() {
	for toolName, requiredScopes := range map[string][]string{
		"list-domains":                         {"scope:domain:read"},
		"create-domain":                        {"scope:domain:write"},
		"get-domain-details":                   {"scope:domain:read"},
		"update-domain":                        {"scope:domain:write"},
		"delete-domain":                        {"scope:domain:write"},
		"list-organizations":                   {"scope:domain:read"},
		"create-organization":                  {"scope:domain:write"},
		"get-organization-details":             {"scope:domain:read"},
		"update-organization":                  {"scope:domain:write"},
		"delete-organization":                  {"scope:domain:write"},
		"list-identity-groups":                 {"scope:domain:read"},
		"create-identity-group":                {"scope:domain:write"},
		"get-identity-group-details":           {"scope:domain:read"},
		"list-identity-group-organizations":    {"scope:domain:read"},
		"list-identity-group-users":            {"scope:domain:read"},
		"list-available-group-organizations":   {"scope:domain:read"},
		"list-available-identity-group-users":  {"scope:domain:read"},
		"add-organizations-to-identity-group":  {"scope:domain:write"},
		"update-identity-group-organization":   {"scope:domain:write"},
		"remove-organizations-from-group":      {"scope:domain:write"},
		"add-users-to-identity-group":          {"scope:domain:write"},
		"remove-users-from-identity-group":     {"scope:domain:write"},
		"update-identity-group":                {"scope:domain:write"},
		"delete-identity-group":                {"scope:domain:write"},
		"list-users":                           {"scope:domain:read"},
		"create-user":                          {"scope:domain:write"},
		"get-user-details":                     {"scope:domain:read"},
		"update-user":                          {"scope:domain:write"},
		"delete-user":                          {"scope:domain:write"},
		"list-access-profiles":                 {"scope:access-profiles:read"},
		"create-access-profile":                {"scope:access-profiles:write"},
		"update-access-profile":                {"scope:access-profiles:write"},
		"delete-access-profile":                {"scope:access-profiles:write"},
		"dropdown-access-profiles":             {"scope:access-profiles:read"},
		"lock-access-profile":                  {"scope:access-profiles:write"},
		"list-ai-credentials":                  {"scope:ai-credentials:read"},
		"create-ai-credential":                 {"scope:ai-credentials:write"},
		"delete-ai-credential":                 {"scope:ai-credentials:write"},
		"dropdown-ai-credentials":              {"scope:ai-credentials:read"},
		"list-kubernetes-profiles":             {"scope:kubernetes-profiles:read"},
		"create-kubernetes-profile":            {"scope:kubernetes-profiles:write"},
		"delete-kubernetes-profile":            {"scope:kubernetes-profiles:write"},
		"dropdown-kubernetes-profiles":         {"scope:kubernetes-profiles:read"},
		"lock-kubernetes-profile":              {"scope:kubernetes-profiles:write"},
		"list-opa-profiles":                    {"scope:opa-profiles:read"},
		"create-opa-profile":                   {"scope:opa-profiles:write"},
		"update-opa-profile":                   {"scope:opa-profiles:write"},
		"delete-opa-profile":                   {"scope:opa-profiles:write"},
		"dropdown-opa-profiles":                {"scope:opa-profiles:read"},
		"lock-opa-profile":                     {"scope:opa-profiles:write"},
		"sync-opa-profile":                     {"scope:opa-profiles:write"},
		"make-opa-profile-default":             {"scope:opa-profiles:write"},
		"list-alerting-profiles":               {"scope:alerting-profiles:read"},
		"create-alerting-profile":              {"scope:alerting-profiles:write"},
		"update-alerting-profile":              {"scope:alerting-profiles:write"},
		"delete-alerting-profile":              {"scope:alerting-profiles:write"},
		"dropdown-alerting-profiles":           {"scope:alerting-profiles:read"},
		"lock-alerting-profile":                {"scope:alerting-profiles:write"},
		"attach-alerting-profile":              {"scope:alerting-profiles:write"},
		"detach-alerting-profile":              {"scope:alerting-profiles:write"},
		"assign-alerting-emails":               {"scope:alerting-profiles:write"},
		"assign-alerting-webhooks":             {"scope:alerting-profiles:write"},
		"verify-alerting-webhook":              {"scope:alerting-profiles:write"},
		"list-alerting-integrations":           {"scope:alerting-profiles:read"},
		"create-alerting-integration":          {"scope:alerting-profiles:write"},
		"update-alerting-integration":          {"scope:alerting-profiles:write"},
		"delete-alerting-integration":          {"scope:alerting-profiles:write"},
		"list-backup-credentials":              {"scope:backup-credentials:read"},
		"create-backup-credential":             {"scope:backup-credentials:write"},
		"update-backup-credential":             {"scope:backup-credentials:write"},
		"delete-backup-credential":             {"scope:backup-credentials:write"},
		"dropdown-backup-credentials":          {"scope:backup-credentials:read"},
		"make-backup-credential-default":       {"scope:backup-credentials:write"},
		"lock-backup-credential":               {"scope:backup-credentials:write"},
		"create-backup-policy":                 {"scope:backup-policies:write"},
		"get-backup-by-name":                   {"scope:backup-policies:read"},
		"list-project-backups":                 {"scope:backup-policies:read"},
		"list-project-restore-requests":        {"scope:backup-policies:read"},
		"list-project-backup-schedules":        {"scope:backup-policies:read"},
		"list-project-backup-locations":        {"scope:backup-policies:read"},
		"list-project-backup-delete-requests":  {"scope:backup-policies:read"},
		"describe-backup":                      {"scope:backup-policies:read"},
		"describe-restore":                     {"scope:backup-policies:read"},
		"describe-schedule":                    {"scope:backup-policies:read"},
		"delete-backup":                        {"scope:backup-policies:write"},
		"delete-backup-storage-location":       {"scope:backup-policies:write"},
		"delete-restore":                       {"scope:backup-policies:write"},
		"delete-schedule":                      {"scope:backup-policies:write"},
		"import-backup-storage-location":       {"scope:backup-policies:write"},
		"restore-backup":                       {"scope:backup-policies:write"},
		"enable-project-backup":                {"scope:project-deployments"},
		"disable-project-backup":               {"scope:project-deployments"},
		"enable-project-monitoring":            {"scope:project-deployments"},
		"disable-project-monitoring":           {"scope:project-deployments"},
		"get-project-monitoring-alerts":        {"scope:projects:read"},
		"list-project-alerts":                  {"scope:projects:read"},
		"query-project-loki-logs":              {"scope:projects:read"},
		"export-project-loki-logs":             {"scope:projects:read"},
		"query-project-prometheus-metrics":     {"scope:projects:read"},
		"autocomplete-project-metrics":         {"scope:projects:read"},
		"enable-project-ai-assistant":          {"scope:project-deployments"},
		"disable-project-ai-assistant":         {"scope:project-deployments"},
		"enable-project-policy":                {"scope:project-deployments"},
		"disable-project-policy":               {"scope:project-deployments"},
		"enable-project-full-spot":             {"scope:projects:write"},
		"disable-project-full-spot":            {"scope:projects:write"},
		"enable-project-spot-workers":          {"scope:projects:write"},
		"disable-project-spot-workers":         {"scope:projects:write"},
		"enable-project-spot-vms":              {"scope:projects:write"},
		"disable-project-spot-vms":             {"scope:projects:write"},
		"get-project-service-status":           {"scope:servers:read"},
		"list-images":                          {"scope:images:read"},
		"get-image-details":                    {"scope:images:read"},
		"bind-images-to-project":               {"scope:images:write"},
		"unbind-images-from-project":           {"scope:images:write"},
		"list-selected-project-images":         {"scope:images:read"},
		"enable-autoscaling":                   {"scope:autoscaling"},
		"update-autoscaling":                   {"scope:autoscaling"},
		"disable-autoscaling":                  {"scope:autoscaling"},
		"get-autoscaling-status":               {"scope:autoscaling"},
		"list-standalone-vms":                  {"scope:vms:read"},
		"get-standalone-vm-details":            {"scope:vms:read"},
		"create-standalone-vm":                 {"scope:vms:write"},
		"delete-standalone-vm":                 {"scope:vms:write"},
		"update-standalone-vm-flavor":          {"scope:vms:write"},
		"manage-standalone-vm-ip":              {"scope:vms:write"},
		"reset-standalone-vm-status":           {"scope:vms:write"},
		"get-standalone-vm-console":            {"scope:vms:read"},
		"download-standalone-vm-rdp":           {"scope:vms:read"},
		"reboot-standalone-vm":                 {"scope:vms:write"},
		"shelve-standalone-vm":                 {"scope:vms:write"},
		"start-standalone-vm":                  {"scope:vms:write"},
		"get-standalone-vm-status":             {"scope:vms:read"},
		"stop-standalone-vm":                   {"scope:vms:write"},
		"unshelve-standalone-vm":               {"scope:vms:write"},
		"get-standalone-vm-windows-password":   {"scope:vms:read"},
		"create-standalone-vm-disk":            {"scope:vms:write"},
		"resize-standalone-vm-disk":            {"scope:vms:write"},
		"list-standalone-profiles":             {"scope:vms:read"},
		"create-standalone-profile":            {"scope:vms:write"},
		"update-standalone-profile":            {"scope:vms:write"},
		"delete-standalone-profile":            {"scope:vms:write"},
		"dropdown-standalone-profiles":         {"scope:vms:read"},
		"lock-standalone-profile":              {"scope:vms:write"},
		"create-standalone-profile-sg":         {"scope:vms:write"},
		"update-standalone-profile-sg":         {"scope:vms:write"},
		"delete-standalone-profile-sg":         {"scope:vms:write"},
		"create-aws-cloud-credential":          {"scope:cloud-credentials:write"},
		"update-aws-cloud-credential":          {"scope:cloud-credentials:write"},
		"create-azure-cloud-credential":        {"scope:cloud-credentials:write"},
		"update-azure-cloud-credential":        {"scope:cloud-credentials:write"},
		"create-openstack-cloud-credential":    {"scope:cloud-credentials:write"},
		"update-openstack-cloud-credential":    {"scope:cloud-credentials:write"},
		"create-proxmox-cloud-credential":      {"scope:cloud-credentials:write"},
		"update-proxmox-cloud-credential":      {"scope:cloud-credentials:write"},
		"create-vsphere-cloud-credential":      {"scope:cloud-credentials:write"},
		"update-vsphere-cloud-credential":      {"scope:cloud-credentials:write"},
		"create-zadara-cloud-credential":       {"scope:cloud-credentials:write"},
		"update-zadara-cloud-credential":       {"scope:cloud-credentials:write"},
		"update-generic-kubernetes-credential": {"scope:cloud-credentials:write"},
		"delete-cloud-credential":              {"scope:cloud-credentials:write"},
		"make-cloud-credential-default":        {"scope:cloud-credentials:write"},
		"lock-cloud-credential":                {"scope:cloud-credentials:write"},
	} {
		toolRequiredScopes[toolName] = requiredScopes
	}
}

func setRobotUserContext(ctx RobotUserContext) {
	robotUserContextMu.Lock()
	defer robotUserContextMu.Unlock()
	robotUserContext = ctx
}

func getRobotUserContext() RobotUserContext {
	robotUserContextMu.RLock()
	defer robotUserContextMu.RUnlock()
	ctx := robotUserContext
	ctx.Scopes = append([]string(nil), robotUserContext.Scopes...)
	return ctx
}

func parseRobotUserContext(body []byte) (RobotUserContext, error) {
	if len(body) == 0 {
		return RobotUserContext{}, fmt.Errorf("robot details response body was empty")
	}

	ctx := RobotUserContext{
		UserID:           gjson.GetBytes(body, "userId").String(),
		AccountID:        int32(gjson.GetBytes(body, "accountId").Int()),
		AccountName:      gjson.GetBytes(body, "accountName").String(),
		AccessKey:        gjson.GetBytes(body, "accessKey").String(),
		OrganizationID:   int32(gjson.GetBytes(body, "organizationId").Int()),
		OrganizationName: gjson.GetBytes(body, "organizationName").String(),
		CreatedBy:        gjson.GetBytes(body, "createdBy").String(),
		Name:             gjson.GetBytes(body, "name").String(),
		Description:      gjson.GetBytes(body, "description").String(),
		IsActive:         gjson.GetBytes(body, "isActive").Bool(),
		CreatedAt:        gjson.GetBytes(body, "createdAt").String(),
		ExpiresAt:        gjson.GetBytes(body, "expiresAt").String(),
		LastUsedAt:       gjson.GetBytes(body, "lastUsedAt").String(),
	}

	scopes := gjson.GetBytes(body, "scopes")
	for _, scope := range scopes.Array() {
		if scope.Str != "" {
			ctx.Scopes = append(ctx.Scopes, scope.Str)
		}
	}
	sort.Strings(ctx.Scopes)

	if ctx.AccessKey == "" && ctx.Name == "" {
		return RobotUserContext{}, fmt.Errorf("robot details response did not contain robot user metadata")
	}

	return ctx, nil
}

func fetchRobotUserContext(client *taikungoclient.Client) (RobotUserContext, error) {
	ctx := context.Background()
	details, httpResponse, err := client.Client.RobotAPI.RobotDetails(ctx).Execute()

	if httpResponse != nil && httpResponse.Body != nil {
		body, readErr := readResponseBodyPreservingBody(httpResponse)
		if readErr == nil {
			parsedCtx, parseErr := parseRobotUserContext(body)
			if parseErr == nil {
				return parsedCtx, nil
			}
		}
	}

	if err != nil {
		return RobotUserContext{}, taikungoclient.CreateError(httpResponse, err)
	}
	if details == nil {
		return RobotUserContext{}, fmt.Errorf("robot details response was empty")
	}

	parsed := robotUserContextFromDetails(details)
	sort.Strings(parsed.Scopes)
	return parsed, nil
}

func robotUserContextFromDetails(details *taikuncore.RobotUsersListDto) RobotUserContext {
	parsed := RobotUserContext{
		UserID:           details.GetUserId(),
		AccountID:        details.GetDomainId(),
		AccountName:      details.GetDomainName(),
		AccessKey:        details.GetAccessKey(),
		OrganizationID:   details.GetOrganizationId(),
		OrganizationName: details.GetOrganizationName(),
		CreatedBy:        details.GetCreatedBy(),
		Name:             details.GetName(),
		Description:      details.GetDescription(),
		Scopes:           append([]string(nil), details.GetScopes()...),
		IsActive:         details.GetIsActive(),
		CreatedAt:        details.GetCreatedAt(),
		ExpiresAt:        details.GetExpiresAt(),
		LastUsedAt:       details.GetLastUsedAt(),
	}

	if value, ok := details.AdditionalProperties["accountId"]; ok {
		if accountID, ok := int32FromAny(value); ok {
			parsed.AccountID = accountID
		}
	}
	if value, ok := details.AdditionalProperties["accountName"]; ok {
		if accountName, ok := stringFromAny(value); ok {
			parsed.AccountName = accountName
		}
	}

	return parsed
}

func int32FromAny(value interface{}) (int32, bool) {
	switch typed := value.(type) {
	case int:
		return int32(typed), true
	case int32:
		return typed, true
	case int64:
		return int32(typed), true
	case float64:
		return int32(typed), true
	case float32:
		return int32(typed), true
	default:
		return 0, false
	}
}

func stringFromAny(value interface{}) (string, bool) {
	typed, ok := value.(string)
	return typed, ok
}

func refreshRobotUserContext() RobotUserContext {
	ctx, err := fetchRobotUserContext(taikunClient)
	if err != nil {
		logger.Printf("Unable to refresh Robot User scopes: %v", err)
		setRobotUserContext(RobotUserContext{
			ScopeDiscoveryError: err.Error(),
		})
		return getRobotUserContext()
	}

	logger.Printf("Loaded Robot User scopes for %q (%d scope(s))", ctx.Name, len(ctx.Scopes))
	setRobotUserContext(ctx)
	return ctx
}

func evaluateToolScopeAccess(toolName string, assignedScopes []string) ToolScopeAccess {
	requiredScopes, ok := toolRequiredScopes[toolName]
	if !ok {
		return ToolScopeAccess{
			Tool:   toolName,
			Status: "unknown",
			Reason: "No scope mapping is defined for this tool yet",
		}
	}

	if len(requiredScopes) == 0 {
		return ToolScopeAccess{
			Tool:           toolName,
			Status:         "allowed",
			RequiredScopes: []string{},
			Reason:         "This tool does not require any Robot User scopes",
		}
	}

	missing := make([]string, 0, len(requiredScopes))
	for _, required := range requiredScopes {
		if !slices.Contains(assignedScopes, required) {
			missing = append(missing, required)
		}
	}

	if len(missing) == 0 {
		return ToolScopeAccess{
			Tool:           toolName,
			Status:         "allowed",
			RequiredScopes: append([]string(nil), requiredScopes...),
		}
	}

	return ToolScopeAccess{
		Tool:           toolName,
		Status:         "blocked",
		RequiredScopes: append([]string(nil), requiredScopes...),
		MissingScopes:  missing,
		Reason:         "Robot User is missing required scopes for this tool",
	}
}

func currentRobotUserCapabilities() RobotUserCapabilitiesResponse {
	ctx := getRobotUserContext()

	toolNames := make([]string, 0, len(toolRequiredScopes))
	for toolName := range toolRequiredScopes {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)

	toolAccess := make([]ToolScopeAccess, 0, len(toolNames))
	for _, toolName := range toolNames {
		toolAccess = append(toolAccess, evaluateToolScopeAccess(toolName, ctx.Scopes))
	}

	message := fmt.Sprintf("Loaded Robot User capabilities for %d scoped tool(s)", len(toolAccess))
	if ctx.ScopeDiscoveryError != "" {
		message = "Robot User scope discovery failed"
	}

	return RobotUserCapabilitiesResponse{
		RobotUser:  ctx,
		ToolAccess: toolAccess,
		Success:    ctx.ScopeDiscoveryError == "",
		Message:    message,
	}
}

func getRobotUserCapabilities() *mcp_golang.ToolResponse {
	return createJSONResponse(currentRobotUserCapabilities())
}

func scopeDeniedResponse(toolName string, access ToolScopeAccess) *mcp_golang.ToolResponse {
	details := fmt.Sprintf("Required scopes: %s. Missing scopes: %s.",
		strings.Join(access.RequiredScopes, ", "),
		strings.Join(access.MissingScopes, ", "),
	)

	ctx := getRobotUserContext()
	if len(ctx.Scopes) > 0 {
		details += fmt.Sprintf(" Assigned scopes: %s.", strings.Join(ctx.Scopes, ", "))
	}
	if ctx.ScopeDiscoveryError != "" {
		details += fmt.Sprintf(" Scope discovery warning: %s.", ctx.ScopeDiscoveryError)
	}

	return createJSONResponse(ErrorResponse{
		Error:   fmt.Sprintf("Robot User cannot use tool %q", toolName),
		Details: details,
	})
}

func authorizeTool(toolName string) *mcp_golang.ToolResponse {
	ctx := getRobotUserContext()
	access := evaluateToolScopeAccess(toolName, ctx.Scopes)
	if access.Status == "unknown" {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Robot User authorization is not configured for tool %q", toolName),
			Details: "This tool is registered for scope-aware authorization, but no scope mapping is defined for it.",
		})
	}
	if ctx.ScopeDiscoveryError != "" {
		if len(access.RequiredScopes) == 0 {
			return nil
		}
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Cannot authorize tool %q because Robot User scope discovery failed", toolName),
			Details: ctx.ScopeDiscoveryError,
		})
	}
	if access.Status == "blocked" {
		return scopeDeniedResponse(toolName, access)
	}
	return nil
}

func registerScopedTool[T any](server *mcp_golang.Server, name, description string, handler func(args T) (*mcp_golang.ToolResponse, error)) error {
	if _, ok := toolRequiredScopes[name]; !ok {
		return fmt.Errorf("missing scope mapping for scoped tool %q", name)
	}

	return server.RegisterTool(name, description, func(args T) (*mcp_golang.ToolResponse, error) {
		if denied := authorizeTool(name); denied != nil {
			return denied, nil
		}
		if denied := enforceMCPLock(name, args); denied != nil {
			return denied, nil
		}
		response, err := handler(args)
		updateCreatedProjectAllowlistAfterTool(name, args, response, err)
		return response, err
	})
}

func mustRegisterScopedTool[T any](server *mcp_golang.Server, name, description string, handler func(args T) (*mcp_golang.ToolResponse, error)) {
	if err := registerScopedTool(server, name, description, handler); err != nil {
		logger.Fatalf("Failed to register %s tool: %v", name, err)
	}
	logger.Printf("Registered %s tool", name)
}
