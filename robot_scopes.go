package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/tidwall/gjson"
)

type RobotUserCapabilitiesArgs struct {
	Detailed bool `json:"detailed,omitempty" jsonschema:"description=Return the full per-tool scope matrix instead of the compact allowed/blocked tool name lists (default: false)"`
}

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

// RobotUserCapabilitiesCompactResponse is the default, low-context response for
// robot-user-capabilities: it returns the allowed/blocked tool name lists rather
// than the full per-tool scope matrix (which is ~25 KB across 200+ tools).
type RobotUserCapabilitiesCompactResponse struct {
	RobotUser    RobotUserContext `json:"robotUser"`
	AllowedTools []string         `json:"allowedTools"`
	BlockedTools []string         `json:"blockedTools,omitempty"`
	AllowedCount int              `json:"allowedCount"`
	BlockedCount int              `json:"blockedCount"`
	Success      bool             `json:"success"`
	Message      string           `json:"message"`
}

var (
	robotUserContextMu sync.RWMutex
	robotUserContext   RobotUserContext
)

// DNS and certificate platform scopes. The UI grants a single combined
// scope:dns-cert (like scope:autoscaling) that covers DNS credentials, custom
// CAs, and project dns-cert operations. MCP tools still declare split
// read/write requirements; scopeAliases maps the combined scope to those.
const (
	dnsCredentialsReadScope       = "scope:dns-credentials:read"
	dnsCredentialsWriteScope      = "scope:dns-credentials:write"
	dnsCertReadScope              = "scope:dns-cert:read"
	dnsCertWriteScope             = "scope:dns-cert:write"
	certificateProfilesReadScope  = "scope:certificate-profiles:read"
	certificateProfilesWriteScope = "scope:certificate-profiles:write"
	dnsCertCombinedScope          = "scope:dns-cert"
)

// scopeAliases maps combined platform scopes to the split read/write scopes
// expected by individual MCP tools.
var scopeAliases = map[string][]string{
	dnsCertCombinedScope: {
		dnsCredentialsReadScope,
		dnsCredentialsWriteScope,
		dnsCertReadScope,
		dnsCertWriteScope,
		certificateProfilesReadScope,
		certificateProfilesWriteScope,
	},
}

func assignedScopeSatisfies(assignedScopes []string, required string) bool {
	if slices.Contains(assignedScopes, required) {
		return true
	}
	for _, assigned := range assignedScopes {
		if implied, ok := scopeAliases[assigned]; ok && slices.Contains(implied, required) {
			return true
		}
	}
	return false
}

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
	"list-kubeconfigs":               {"scope:kubernetes:read"},
	"list-kubeconfig-roles":          {"scope:kubernetes:read"},
	"list-kubernetes-resource-kinds": {},
	"describe-payload":               {},
	"list-kubernetes-resources":      {"scope:kubernetes:read"},
	"describe-kubernetes-resource":   {"scope:kubernetes:read"},
	"delete-kubernetes-resource":     {"scope:kubernetes:write"},
	"patch-kubernetes-resource":      {"scope:kubernetes:write"},
	"list-cloud-credentials":         {"scope:cloud-credentials:read"},
	"bind-flavors-to-project":        {"scope:flavors:write"},
	"add-server-to-project":          {"scope:servers:write"},
	"commit-project":                 {"scope:project-deployments"},
	"preflight-project":              {"scope:projects:read"},
		"get-project-details":            {"scope:projects:read"},
		"get-project-access-ip":          {"scope:projects:read"},
	"list-flavors":                   {"scope:flavors:read"},
	"list-servers":                   {"scope:servers:read"},
	"delete-servers-from-project":    {"scope:servers:write"},
}

func init() {
	for toolName, requiredScopes := range map[string][]string{
		"list-domains":                        {"scope:domain:read"},
		"create-domain":                       {"scope:domain:write"},
		"get-domain-details":                  {"scope:domain:read"},
		"update-domain":                       {"scope:domain:write"},
		"delete-domain":                       {"scope:domain:write"},
		"list-organizations":                  {"scope:domain:read"},
		"create-organization":                 {"scope:domain:write"},
		"get-organization-details":            {"scope:domain:read"},
		"update-organization":                 {"scope:domain:write"},
		"delete-organization":                 {"scope:domain:write"},
		"list-identity-groups":                {"scope:domain:read"},
		"create-identity-group":               {"scope:domain:write"},
		"get-identity-group-details":          {"scope:domain:read"},
		"list-identity-group-organizations":   {"scope:domain:read"},
		"list-identity-group-users":           {"scope:domain:read"},
		"list-available-group-organizations":  {"scope:domain:read"},
		"list-available-identity-group-users": {"scope:domain:read"},
		"add-organizations-to-identity-group": {"scope:domain:write"},
		"update-identity-group-organization":  {"scope:domain:write"},
		"remove-organizations-from-group":     {"scope:domain:write"},
		"add-users-to-identity-group":         {"scope:domain:write"},
		"remove-users-from-identity-group":    {"scope:domain:write"},
		"update-identity-group":               {"scope:domain:write"},
		"delete-identity-group":               {"scope:domain:write"},
		"list-users":                          {"scope:domain:read"},
		"create-user":                         {"scope:domain:write"},
		"get-user-details":                    {"scope:domain:read"},
		"update-user":                         {"scope:domain:write"},
		"delete-user":                         {"scope:domain:write"},
		"list-access-profiles":                {"scope:access-profiles:read"},
		"create-access-profile":               {"scope:access-profiles:write"},
		"update-access-profile":               {"scope:access-profiles:write"},
		"delete-access-profile":               {"scope:access-profiles:write"},
		"dropdown-access-profiles":            {"scope:access-profiles:read"},
		"lock-access-profile":                 {"scope:access-profiles:write"},
		"list-ai-credentials":                 {"scope:ai-credentials:read"},
		"create-ai-credential":                {"scope:ai-credentials:write"},
		"delete-ai-credential":                {"scope:ai-credentials:write"},
		"dropdown-ai-credentials":             {"scope:ai-credentials:read"},
		"list-kubernetes-profiles":            {"scope:kubernetes-profiles:read"},
		"create-kubernetes-profile":           {"scope:kubernetes-profiles:write"},
		"delete-kubernetes-profile":           {"scope:kubernetes-profiles:write"},
		"dropdown-kubernetes-profiles":        {"scope:kubernetes-profiles:read"},
		"lock-kubernetes-profile":             {"scope:kubernetes-profiles:write"},
		"list-opa-profiles":                   {"scope:opa-profiles:read"},
		"create-opa-profile":                  {"scope:opa-profiles:write"},
		"update-opa-profile":                  {"scope:opa-profiles:write"},
		"delete-opa-profile":                  {"scope:opa-profiles:write"},
		"dropdown-opa-profiles":               {"scope:opa-profiles:read"},
		"lock-opa-profile":                    {"scope:opa-profiles:write"},
		"sync-opa-profile":                    {"scope:opa-profiles:write"},
		"make-opa-profile-default":            {"scope:opa-profiles:write"},
		"list-alerting-profiles":              {"scope:alerting-profiles:read"},
		"create-alerting-profile":             {"scope:alerting-profiles:write"},
		"update-alerting-profile":             {"scope:alerting-profiles:write"},
		"delete-alerting-profile":             {"scope:alerting-profiles:write"},
		"dropdown-alerting-profiles":          {"scope:alerting-profiles:read"},
		"lock-alerting-profile":               {"scope:alerting-profiles:write"},
		"attach-alerting-profile":             {"scope:alerting-profiles:write"},
		"detach-alerting-profile":             {"scope:alerting-profiles:write"},
		"assign-alerting-emails":              {"scope:alerting-profiles:write"},
		"assign-alerting-webhooks":            {"scope:alerting-profiles:write"},
		"verify-alerting-webhook":             {"scope:alerting-profiles:write"},
		"list-alerting-integrations":          {"scope:alerting-profiles:read"},
		"create-alerting-integration":         {"scope:alerting-profiles:write"},
		"update-alerting-integration":         {"scope:alerting-profiles:write"},
		"delete-alerting-integration":         {"scope:alerting-profiles:write"},
		"list-backup-credentials":             {"scope:backup-credentials:read"},
		"create-backup-credential":            {"scope:backup-credentials:write"},
		"update-backup-credential":            {"scope:backup-credentials:write"},
		"delete-backup-credential":            {"scope:backup-credentials:write"},
		"dropdown-backup-credentials":         {"scope:backup-credentials:read"},
		"make-backup-credential-default":      {"scope:backup-credentials:write"},
		"lock-backup-credential":              {"scope:backup-credentials:write"},
		"create-backup-policy":                {"scope:backup-policies:write"},
		"get-backup-by-name":                  {"scope:backup-policies:read"},
		"list-project-backups":                {"scope:backup-policies:read"},
		"list-project-restore-requests":       {"scope:backup-policies:read"},
		"list-project-backup-schedules":       {"scope:backup-policies:read"},
		"list-project-backup-locations":       {"scope:backup-policies:read"},
		"list-project-backup-delete-requests": {"scope:backup-policies:read"},
		"describe-backup":                     {"scope:backup-policies:read"},
		"describe-restore":                    {"scope:backup-policies:read"},
		"describe-schedule":                   {"scope:backup-policies:read"},
		"delete-backup":                       {"scope:backup-policies:write"},
		"delete-backup-storage-location":      {"scope:backup-policies:write"},
		"delete-restore":                      {"scope:backup-policies:write"},
		"delete-schedule":                     {"scope:backup-policies:write"},
		"import-backup-storage-location":      {"scope:backup-policies:write"},
		"restore-backup":                      {"scope:backup-policies:write"},
		"enable-project-backup":               {"scope:project-deployments"},
		"disable-project-backup":              {"scope:project-deployments"},
		"enable-project-monitoring":           {"scope:project-deployments"},
		"disable-project-monitoring":          {"scope:project-deployments"},
		"get-project-monitoring-alerts":       {"scope:projects:read"},
		"list-project-alerts":                 {"scope:projects:read"},
		"query-project-loki-logs":             {"scope:projects:read"},
		"export-project-loki-logs":            {"scope:projects:read"},
		"query-project-prometheus-metrics":    {"scope:projects:read"},
		"autocomplete-project-metrics":        {"scope:projects:read"},
		"enable-project-ai-assistant":         {"scope:project-deployments"},
		"disable-project-ai-assistant":        {"scope:project-deployments"},
		"enable-project-policy":               {"scope:project-deployments"},
		"disable-project-policy":              {"scope:project-deployments"},
		"enable-project-full-spot":            {"scope:projects:write"},
		"disable-project-full-spot":           {"scope:projects:write"},
		"enable-project-spot-workers":         {"scope:projects:write"},
		"disable-project-spot-workers":        {"scope:projects:write"},
		"enable-project-spot-vms":             {"scope:projects:write"},
		"disable-project-spot-vms":            {"scope:projects:write"},
		"get-project-service-status":          {"scope:servers:read"},
		"list-images":                         {"scope:images:read"},
		"get-image-details":                   {"scope:images:read"},
		"bind-images-to-project":              {"scope:images:write"},
		"unbind-images-from-project":          {"scope:images:write"},
		"list-selected-project-images":        {"scope:images:read"},
		"enable-autoscaling":                  {"scope:autoscaling"},
		"update-autoscaling":                  {"scope:autoscaling"},
		"disable-autoscaling":                 {"scope:autoscaling"},
		"get-autoscaling-status":              {"scope:autoscaling"},
		"list-standalone-vms":                 {"scope:vms:read"},
		"get-standalone-vm-details":           {"scope:vms:read"},
		"create-standalone-vm":                {"scope:vms:write"},
		"delete-standalone-vm":                {"scope:vms:write"},
		"update-standalone-vm-flavor":         {"scope:vms:write"},
		"manage-standalone-vm-ip":             {"scope:vms:write"},
		"reset-standalone-vm-status":          {"scope:vms:write"},
		"get-standalone-vm-console":           {"scope:vms:read"},
		"download-standalone-vm-rdp":          {"scope:vms:read"},
		"reboot-standalone-vm":                {"scope:vms:write"},
		"shelve-standalone-vm":                {"scope:vms:write"},
		"start-standalone-vm":                 {"scope:vms:write"},
		"get-standalone-vm-status":            {"scope:vms:read"},
		"stop-standalone-vm":                  {"scope:vms:write"},
		"unshelve-standalone-vm":              {"scope:vms:write"},
		"get-standalone-vm-windows-password":  {"scope:vms:read"},
		"create-standalone-vm-disk":           {"scope:vms:write"},
		"resize-standalone-vm-disk":           {"scope:vms:write"},
		"list-standalone-profiles":            {"scope:vms:read"},
		"create-standalone-profile":           {"scope:vms:write"},
		"update-standalone-profile":           {"scope:vms:write"},
		"delete-standalone-profile":           {"scope:vms:write"},
		"dropdown-standalone-profiles":        {"scope:vms:read"},
		"lock-standalone-profile":             {"scope:vms:write"},
		"create-standalone-profile-sg":        {"scope:vms:write"},
		"update-standalone-profile-sg":        {"scope:vms:write"},
		"delete-standalone-profile-sg":        {"scope:vms:write"},
		"create-cloud-credential":             {"scope:cloud-credentials:write"},
		"update-cloud-credential":             {"scope:cloud-credentials:write"},
		"delete-cloud-credential":             {"scope:cloud-credentials:write"},
		"make-cloud-credential-default":       {"scope:cloud-credentials:write"},
		"lock-cloud-credential":               {"scope:cloud-credentials:write"},
		"create-google-cloud-credential":      {"scope:cloud-credentials:write"},
		"list-google-regions":                 {"scope:cloud-credentials:read"},
		"list-google-zones":                   {"scope:cloud-credentials:read"},
		"list-google-billing-accounts":        {"scope:cloud-credentials:read"},
		// Access profile sub-resources are gated by the access-profiles scope.
		"list-dns-servers":        {"scope:access-profiles:read"},
		"create-dns-server":       {"scope:access-profiles:write"},
		"edit-dns-server":         {"scope:access-profiles:write"},
		"delete-dns-server":       {"scope:access-profiles:write"},
		"list-ntp-servers":        {"scope:access-profiles:read"},
		"create-ntp-server":       {"scope:access-profiles:write"},
		"edit-ntp-server":         {"scope:access-profiles:write"},
		"delete-ntp-server":       {"scope:access-profiles:write"},
		"list-ssh-users":          {"scope:access-profiles:read"},
		"create-ssh-user":         {"scope:access-profiles:write"},
		"edit-ssh-user":           {"scope:access-profiles:write"},
		"delete-ssh-user":         {"scope:access-profiles:write"},
		"list-trusted-registries": {"scope:access-profiles:read"},
		"create-trusted-registry": {"scope:access-profiles:write"},
		"edit-trusted-registry":   {"scope:access-profiles:write"},
		"delete-trusted-registry": {"scope:access-profiles:write"},
		// DNS credentials, the DNS certificate service, and certificate profiles
		// are gated by dedicated platform scopes that follow the same
		// scope:<resource>:<action> convention as the rest of the platform
		// (e.g. scope:cloud-credentials:read/write). These scopes are not yet
		// granted to Robot Users on the backend, so the API currently returns
		// 403 for every DNS/cert call. Mapping the anticipated scope names here
		// means robot-user-capabilities correctly reports these tools as blocked
		// today and will automatically flip them to allowed once the backend
		// publishes and grants the scopes. If the backend finalizes different
		// scope identifiers, only these string constants need to change.
		"list-dns-credentials":               {dnsCredentialsReadScope},
		"dropdown-dns-credentials":           {dnsCredentialsReadScope},
		"create-dns-credential":              {dnsCredentialsWriteScope},
		"update-dns-credential":              {dnsCredentialsWriteScope},
		"delete-dns-credential":              {dnsCredentialsWriteScope},
		"make-dns-credential-default":        {dnsCredentialsWriteScope},
		"lock-dns-credential":                {dnsCredentialsWriteScope},
		"attach-dns-credential-to-project":   {dnsCredentialsWriteScope},
		"detach-dns-credential-from-project": {dnsCredentialsWriteScope},
		"validate-dns-credential":            {dnsCredentialsReadScope},
		// DNS certificate service (project-scoped ACME/DNS-01 certificate issuance).
		"get-dns-cert-status": {dnsCertReadScope},
		"enable-dns-cert":     {dnsCertWriteScope},
		"disable-dns-cert":    {dnsCertWriteScope},
		"sync-dns-cert":       {dnsCertWriteScope},
		"validate-dns-cert":   {dnsCertReadScope},
		// Certificate profiles (custom certificate authorities).
		"list-certificate-authorities":       {certificateProfilesReadScope},
		"dropdown-certificate-authorities":   {certificateProfilesReadScope},
		"create-certificate-authority":       {certificateProfilesWriteScope},
		"update-certificate-authority":       {certificateProfilesWriteScope},
		"delete-certificate-authority":       {certificateProfilesWriteScope},
		"make-certificate-authority-default": {certificateProfilesWriteScope},
		"lock-certificate-authority":         {certificateProfilesWriteScope},
		"validate-certificate-authority":     {certificateProfilesReadScope},
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
		CreatedBy:        auditUserNameFromJSON(gjson.GetBytes(body, "createdBy")),
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
		AccountID:        details.GetAccountId(),
		AccountName:      details.GetAccountName(),
		AccessKey:        details.GetAccessKey(),
		OrganizationID:   details.GetOrganizationId(),
		OrganizationName: details.GetOrganizationName(),
		CreatedBy:        auditUserName(details.GetCreatedBy()),
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
		if !assignedScopeSatisfies(assignedScopes, required) {
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

func buildCapabilitiesResponse(robotCtx RobotUserContext) RobotUserCapabilitiesResponse {
	toolNames := make([]string, 0, len(toolRequiredScopes))
	for toolName := range toolRequiredScopes {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)

	toolAccess := make([]ToolScopeAccess, 0, len(toolNames))
	for _, toolName := range toolNames {
		toolAccess = append(toolAccess, evaluateToolScopeAccess(toolName, robotCtx.Scopes))
	}

	message := fmt.Sprintf("Loaded Robot User capabilities for %d scoped tool(s)", len(toolAccess))
	if robotCtx.ScopeDiscoveryError != "" {
		message = "Robot User scope discovery failed"
	}

	return RobotUserCapabilitiesResponse{
		RobotUser:  robotCtx,
		ToolAccess: toolAccess,
		Success:    robotCtx.ScopeDiscoveryError == "",
		Message:    message,
	}
}

func buildCompactCapabilitiesResponse(robotCtx RobotUserContext) RobotUserCapabilitiesCompactResponse {
	toolNames := make([]string, 0, len(toolRequiredScopes))
	for toolName := range toolRequiredScopes {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)

	allowed := make([]string, 0, len(toolNames))
	var blocked []string
	for _, toolName := range toolNames {
		switch evaluateToolScopeAccess(toolName, robotCtx.Scopes).Status {
		case "allowed":
			allowed = append(allowed, toolName)
		case "blocked":
			blocked = append(blocked, toolName)
		}
	}

	message := fmt.Sprintf("Robot User can use %d of %d scoped tools (call with detailed=true for the full scope matrix)", len(allowed), len(toolNames))
	if robotCtx.ScopeDiscoveryError != "" {
		message = "Robot User scope discovery failed"
	}

	return RobotUserCapabilitiesCompactResponse{
		RobotUser:    robotCtx,
		AllowedTools: allowed,
		BlockedTools: blocked,
		AllowedCount: len(allowed),
		BlockedCount: len(blocked),
		Success:      robotCtx.ScopeDiscoveryError == "",
		Message:      message,
	}
}

func scopeDeniedResponseWithCtx(toolName string, access ToolScopeAccess, robotCtx RobotUserContext) *mcp_golang.ToolResponse {
	details := fmt.Sprintf("Required scopes: %s. Missing scopes: %s.",
		strings.Join(access.RequiredScopes, ", "),
		strings.Join(access.MissingScopes, ", "),
	)

	if len(robotCtx.Scopes) > 0 {
		details += fmt.Sprintf(" Assigned scopes: %s.", strings.Join(robotCtx.Scopes, ", "))
	}
	if robotCtx.ScopeDiscoveryError != "" {
		details += fmt.Sprintf(" Scope discovery warning: %s.", robotCtx.ScopeDiscoveryError)
	}

	return createJSONResponse(ErrorResponse{
		Error:   fmt.Sprintf("Robot User cannot use tool %q", toolName),
		Details: details,
	})
}

// Robot User context cache for the HTTP/per-request transport. Without this,
// every scoped tool call performs an extra RobotDetails round-trip just to
// authorize. Scopes change rarely, so a short TTL keeps authorization cheap
// while still picking up scope changes within a couple of minutes.
type cachedRobotUserContext struct {
	ctx       RobotUserContext
	expiresAt time.Time
}

const (
	robotContextCacheTTL     = 2 * time.Minute
	robotContextCacheMaxSize = 1024
)

var (
	robotContextCacheMu sync.Mutex
	robotContextCache   = make(map[string]cachedRobotUserContext)
)

func getCachedRobotContext(key string) (RobotUserContext, bool) {
	if key == "" {
		return RobotUserContext{}, false
	}
	robotContextCacheMu.Lock()
	defer robotContextCacheMu.Unlock()
	entry, ok := robotContextCache[key]
	if !ok {
		return RobotUserContext{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(robotContextCache, key)
		return RobotUserContext{}, false
	}
	return entry.ctx, true
}

func setCachedRobotContext(key string, ctx RobotUserContext) {
	// Never cache discovery failures; they should be retried promptly.
	if key == "" || ctx.ScopeDiscoveryError != "" {
		return
	}
	robotContextCacheMu.Lock()
	defer robotContextCacheMu.Unlock()
	if len(robotContextCache) >= robotContextCacheMaxSize {
		now := time.Now()
		for k, v := range robotContextCache {
			if now.After(v.expiresAt) {
				delete(robotContextCache, k)
			}
		}
		if len(robotContextCache) >= robotContextCacheMaxSize {
			robotContextCache = make(map[string]cachedRobotUserContext)
		}
	}
	robotContextCache[key] = cachedRobotUserContext{
		ctx:       ctx,
		expiresAt: time.Now().Add(robotContextCacheTTL),
	}
}

func invalidateCachedRobotContext(key string) {
	if key == "" {
		return
	}
	robotContextCacheMu.Lock()
	delete(robotContextCache, key)
	robotContextCacheMu.Unlock()
}

func resolveRobotUserContext(reqCtx context.Context) RobotUserContext {
	client := clientFromContext(reqCtx)
	if client != nil && client != taikunClient {
		// HTTP/per-request mode: serve the Robot User context from a short-TTL
		// cache keyed by credential identity to avoid a RobotDetails round-trip
		// on every scoped tool call.
		key := credentialKeyFromContext(reqCtx)
		if cached, ok := getCachedRobotContext(key); ok {
			return cached
		}
		fetched, err := fetchRobotUserContext(client)
		if err != nil {
			return RobotUserContext{ScopeDiscoveryError: err.Error()}
		}
		setCachedRobotContext(key, fetched)
		return fetched
	}
	return getRobotUserContext()
}

func authorizeTool(reqCtx context.Context, toolName string) *mcp_golang.ToolResponse {
	robotCtx := resolveRobotUserContext(reqCtx)
	access := evaluateToolScopeAccess(toolName, robotCtx.Scopes)
	if access.Status == "unknown" {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Robot User authorization is not configured for tool %q", toolName),
			Details: "This tool is registered for scope-aware authorization, but no scope mapping is defined for it.",
		})
	}
	if robotCtx.ScopeDiscoveryError != "" {
		if len(access.RequiredScopes) == 0 {
			return nil
		}
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Cannot authorize tool %q because Robot User scope discovery failed", toolName),
			Details: robotCtx.ScopeDiscoveryError,
		})
	}
	if access.Status == "blocked" {
		return scopeDeniedResponseWithCtx(toolName, access, robotCtx)
	}
	return nil
}

// shouldAdvertiseTool decides whether a tool is registered into tools/list.
//
// Every scoped tool is always advertised, regardless of transport or the Robot
// User's scopes. Call-time authorization (authorizeTool) still enforces access
// and returns an immediate scope-denied JSON response for blocked tools.
//
// We intentionally do NOT hide blocked tools: MCP clients (including Cursor)
// cache the tools/list manifest at connect time and do not re-fetch it when the
// server's advertised set changes. If a blocked tool were unregistered, a client
// working from a stale manifest would call a tool with no handler and hang until
// the request timed out. Keeping every tool registered means such calls fail
// fast with a clear "missing scopes" error instead, and the manifest stays
// consistent with reality after any client reload.
func shouldAdvertiseTool(name string) bool {
	return true
}

func registerScopedTool[T any](server *mcp_golang.Server, name, description string, handler func(ctx context.Context, args T) (*mcp_golang.ToolResponse, error)) error {
	if _, ok := toolRequiredScopes[name]; !ok {
		return fmt.Errorf("missing scope mapping for scoped tool %q", name)
	}

	if !shouldAdvertiseTool(name) {
		logger.Printf("Not advertising tool %q: Robot User lacks the required scopes", name)
		return nil
	}

	return server.RegisterTool(name, description, func(ctx context.Context, args T) (*mcp_golang.ToolResponse, error) {
		if denied := authorizeTool(ctx, name); denied != nil {
			return denied, nil
		}
		if denied := enforceMCPLock(name, args); denied != nil {
			return denied, nil
		}
		response, err := handler(ctx, args)
		updateCreatedProjectAllowlistAfterTool(name, args, response, err)
		return response, err
	})
}

func mustRegisterScopedTool[T any](server *mcp_golang.Server, name, description string, handler func(ctx context.Context, args T) (*mcp_golang.ToolResponse, error)) {
	if err := registerScopedTool(server, name, description, handler); err != nil {
		logger.Fatalf("Failed to register %s tool: %v", name, err)
	}
	logger.Printf("Registered %s tool", name)
}
