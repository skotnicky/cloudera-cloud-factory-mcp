package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/itera-io/taikungoclient"
	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
)

// Build-time variables (set by GoReleaser)
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
	builtBy = "unknown"
)

var (
	logger       *log.Logger
	logFilePath  = defaultLogFilePath()
	taikunClient *taikungoclient.Client
	// httpTransportMode is true when serving the HTTP transport. In that mode a
	// blocking tool ties up a request/goroutine and an upstream connection, so
	// long waits are capped and the caller is told to poll again.
	httpTransportMode bool
)

const maxHTTPWaitSeconds = 120

// effectiveWaitTimeout clamps a requested wait (seconds) to maxHTTPWaitSeconds
// in HTTP transport mode. The second return value reports whether it was capped
// so callers can return a "pending, poll again" result instead of a failure.
func effectiveWaitTimeout(requestedSeconds int) (int, bool) {
	if httpTransportMode && requestedSeconds > maxHTTPWaitSeconds {
		return maxHTTPWaitSeconds, true
	}
	return requestedSeconds, false
}

const (
	defaultAPIHost = "api-latest.osc1.sjc.cloudera.com"
	mcpServerName  = "cloudera-cloud-factory-mcp"
)

// normalizeAPIHost strips an accidental URL scheme or path from TAIKUN_API_HOST and
// maps common CCF UI hostnames (app.*) to the API hostname (api.*). taikungoclient
// expects a bare API hostname (e.g. api.ccf-dev1.osc1.sjc.cloudera.com).
func normalizeAPIHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}
	host = strings.TrimSuffix(host, "/")
	if strings.HasPrefix(host, "app.") {
		host = "api." + strings.TrimPrefix(host, "app.")
	}
	return host
}

// Response structs for JSON formatting
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type SuccessResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

type RefreshTaikunClientArgs struct{}

type ServerVersionArgs struct{}

type RefreshTaikunClientResponse struct {
	Message             string   `json:"message"`
	Success             bool     `json:"success"`
	RobotUserName       string   `json:"robotUserName,omitempty"`
	OrganizationName    string   `json:"organizationName,omitempty"`
	Scopes              []string `json:"scopes,omitempty"`
	ScopeDiscoveryError string   `json:"scopeDiscoveryError,omitempty"`
}

type ServerVersionResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	BuiltBy string `json:"builtBy"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ProjectSummary struct {
	ID                     int32   `json:"id"`
	Name                   string  `json:"name"`
	Status                 string  `json:"status"`
	Health                 string  `json:"health"`
	Type                   string  `json:"type"`
	Cloud                  string  `json:"cloud"`
	Organization           string  `json:"organization"`
	IsLocked               bool    `json:"isLocked"`
	IsVirtualCluster       bool    `json:"isVirtualCluster"`
	ParentProjectID        int32   `json:"parentProjectId,omitempty"`
	CreatedAt              string  `json:"createdAt"`
	ServersCount           int32   `json:"serversCount"`
	StandaloneVMsCount     int32   `json:"standaloneVmsCount"`
	HourlyCost             float64 `json:"hourlyCost"`
	MonitoringEnabled      bool    `json:"monitoringEnabled"`
	BackupEnabled          bool    `json:"backupEnabled"`
	AlertsCount            int32   `json:"alertsCount"`
	ReadyForVirtualCluster bool    `json:"readyForVirtualCluster"`
	VirtualClusterReason   string  `json:"virtualClusterReason,omitempty"`
}

type ProjectListResponse struct {
	Projects   []ProjectSummary `json:"projects"`
	Total      int              `json:"total"`
	FilterType string           `json:"filterType"`
	Message    string           `json:"message"`
}

type VirtualClusterSummary struct {
	ID                 int32  `json:"id"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	Health             string `json:"health"`
	KubernetesVersion  string `json:"kubernetesVersion"`
	CreatedAt          string `json:"createdAt"`
	CreatedBy          string `json:"createdBy"`
	ExpiresAt          string `json:"expiresAt,omitempty"`
	DeleteOnExpiration bool   `json:"deleteOnExpiration"`
	Organization       string `json:"organization"`
	IsLocked           bool   `json:"isLocked"`
	HasKubeconfig      bool   `json:"hasKubeconfig"`
}

type VirtualClusterListResponse struct {
	VirtualClusters []VirtualClusterSummary `json:"virtualClusters"`
	Total           int                     `json:"total"`
	ParentProjectID int32                   `json:"parentProjectId"`
	Message         string                  `json:"message"`
}

type CatalogSummary struct {
	ID            int32  `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ProjectsCount int    `json:"projectsCount"`
}

type CatalogListResponse struct {
	Catalogs []CatalogSummary `json:"catalogs"`
	Total    int              `json:"total"`
	Message  string           `json:"message"`
}

type ApplicationSummary struct {
	ID           int32  `json:"id"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	CatalogAppID int32  `json:"catalogAppId"`
	ProjectID    int32  `json:"projectId"`
}

type ApplicationListResponse struct {
	Applications []ApplicationSummary `json:"applications"`
	Total        int                  `json:"total"`
	ProjectID    int32                `json:"projectId"`
	Message      string               `json:"message"`
}

type AddAppToCatalogArgs struct {
	CatalogID   int32  `json:"catalogId" jsonschema:"required,description=The catalog ID to add the application to"`
	Repository  string `json:"repository" jsonschema:"required,description=Repository name (3-30 chars, lowercase/numeric)"`
	PackageName string `json:"packageName" jsonschema:"required,description=Package name (3-30 chars, lowercase/numeric)"`
	Version     string `json:"version,omitempty" jsonschema:"description=Specific package version to add (optional; auto-resolved when there is a single match)"`
}

type AddAppToCatalogWithParametersArgs struct {
	CatalogID   int32          `json:"catalogId" jsonschema:"required,description=The catalog ID to add the application to"`
	Repository  string         `json:"repository" jsonschema:"required,description=Repository name (3-30 chars, lowercase/numeric)"`
	PackageName string         `json:"packageName" jsonschema:"required,description=Package name (3-30 chars, lowercase/numeric)"`
	Version     string         `json:"version,omitempty" jsonschema:"description=Specific package version to add (optional; auto-resolved when there is a single match)"`
	Parameters  []AppParameter `json:"parameters,omitempty" jsonschema:"description=Default application parameters to set in the catalog (optional)"`
}

type ListAvailableAppsArgs struct {
	Repository string `json:"repository,omitempty" jsonschema:"description=Repository name to filter packages (optional)"`
	Limit      int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset     int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search     string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type GetCatalogAppParamsArgs struct {
	CatalogAppID int32  `json:"catalogAppId,omitempty" jsonschema:"description=The catalog application ID to fetch parameters for (optional if packageId+version provided)"`
	PackageID    string `json:"packageId,omitempty" jsonschema:"description=Package ID to fetch parameters for (required with version if catalogAppId not provided)"`
	Version      string `json:"version,omitempty" jsonschema:"description=Package version to fetch parameters for (required with packageId if catalogAppId not provided)"`
	IsTaikunLink *bool  `json:"isTaikunLink,omitempty" jsonschema:"description=Filter Taikun link parameters only (optional)"`
}

type SetCatalogAppDefaultParamsArgs struct {
	CatalogAppID      int32          `json:"catalogAppId" jsonschema:"required,description=The catalog application ID to update parameters for"`
	Parameters        []AppParameter `json:"parameters" jsonschema:"required,description=Catalog app parameters to set as defaults"`
	MergeWithExisting *bool          `json:"mergeWithExisting,omitempty" jsonschema:"description=Merge with existing defaults before updating (default: true)"`
}

type ListRepositoriesArgs struct {
	Limit          int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset         int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search         string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
	SortBy         string `json:"sortBy,omitempty" jsonschema:"description=Field to sort by when supported (optional)"`
	SortDirection  string `json:"sortDirection,omitempty" jsonschema:"description=Sort direction such as asc or desc (optional)"`
	ID             string `json:"id,omitempty" jsonschema:"description=Exact repository ID filter (optional)"`
	IsPrivate      *bool  `json:"isPrivate,omitempty" jsonschema:"description=Filter private or public repositories (optional)"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID filter (optional)"`
}

type ImportRepositoryArgs struct {
	Name           string `json:"name" jsonschema:"required,description=Repository display name"`
	URL            string `json:"url" jsonschema:"required,description=Repository URL such as a Helm or OCI repository"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID to associate with the import (optional)"`
	Username       string `json:"username,omitempty" jsonschema:"description=Username for private repository authentication (optional)"`
	Password       string `json:"password,omitempty" jsonschema:"description=Password for private repository authentication (optional)"`
}

type BindRepositoryArgs struct {
	RepositoryID               string `json:"repositoryId,omitempty" jsonschema:"description=Repository ID from list-repositories (optional if name is provided)"`
	Name                       string `json:"name,omitempty" jsonschema:"description=Repository name to bind when repositoryId is not provided (optional)"`
	RepositoryOrganizationName string `json:"repositoryOrganizationName,omitempty" jsonschema:"description=Repository owner or organization name to disambiguate by name (optional)"`
	OrganizationID             int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID to bind the repository to (optional when the API can infer it)"`
}

type UnbindRepositoryArgs struct {
	RepositoryID   string   `json:"repositoryId,omitempty" jsonschema:"description=Single repository ID to unbind (optional if repositoryIds is provided)"`
	RepositoryIDs  []string `json:"repositoryIds,omitempty" jsonschema:"description=Repository IDs to unbind (optional if repositoryId is provided)"`
	OrganizationID int32    `json:"organizationId,omitempty" jsonschema:"description=Organization ID to unbind the repository from (optional when the API can infer it)"`
}

type DeleteRepositoryArgs struct {
	AppRepoID      int32  `json:"appRepoId,omitempty" jsonschema:"description=Imported repository appRepoId from list-repositories (optional if repositoryId is provided)"`
	RepositoryID   string `json:"repositoryId,omitempty" jsonschema:"description=Repository ID used to resolve appRepoId before deletion (optional if appRepoId is provided)"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID filter used when resolving repositoryId (optional)"`
}

type UpdateRepositoryPasswordArgs struct {
	RepositoryID   string `json:"repositoryId" jsonschema:"required,description=Repository ID to update credentials for"`
	Username       string `json:"username" jsonschema:"required,description=Username for the repository"`
	Password       string `json:"password" jsonschema:"required,description=Password or token for the repository"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID to update the repository in (optional when the Robot User context has one)"`
}

type ListAvailablePackagesArgs struct {
	Repository string `json:"repository,omitempty" jsonschema:"description=Repository name to filter packages (optional)"`
	Limit      int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset     int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search     string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type CreateProjectArgs struct {
	Name                string `json:"name" jsonschema:"required,description=Project name (3-30 characters, alphanumeric with hyphens)"`
	CloudCredentialID   int32  `json:"cloudCredentialId" jsonschema:"required,description=ID of the cloud credential to use for this project"`
	KubernetesProfileID int32  `json:"kubernetesProfileId,omitempty" jsonschema:"description=ID of the Kubernetes profile to use (optional; Kubernetes projects only)"`
	AlertingProfileID   int32  `json:"alertingProfileId,omitempty" jsonschema:"description=ID of the alerting profile to use (optional)"`
	Monitoring          bool   `json:"monitoring,omitempty" jsonschema:"description=Enable monitoring for this project (default: false)"`
	KubernetesVersion   string `json:"kubernetesVersion,omitempty" jsonschema:"description=Kubernetes version to install (optional; Kubernetes projects only)"`
}

type CreateClusterArgs struct {
	Name                string `json:"name" jsonschema:"required,description=Cluster project name (3-30 characters, alphanumeric with hyphens)"`
	CloudCredentialID   int32  `json:"cloudCredentialId" jsonschema:"required,description=Cloud credential ID used for the cluster project"`
	KubernetesProfileID int32  `json:"kubernetesProfileId,omitempty" jsonschema:"description=Kubernetes profile ID (optional; auto-selected when omitted and deterministic)"`
	AlertingProfileID   int32  `json:"alertingProfileId,omitempty" jsonschema:"description=Alerting profile ID (optional; auto-selected when monitoring is enabled and deterministic)"`
	Monitoring          bool   `json:"monitoring,omitempty" jsonschema:"description=Enable monitoring for the cluster project (default: false)"`
	KubernetesVersion   string `json:"kubernetesVersion,omitempty" jsonschema:"description=Kubernetes version for the project (optional)"`
	BastionCount        int32  `json:"bastionCount,omitempty" jsonschema:"description=Number of bastion nodes to add (default: 1)"`
	MasterCount         int32  `json:"masterCount,omitempty" jsonschema:"description=Number of master nodes to add; must be odd (default: 1)"`
	WorkerCount         int32  `json:"workerCount,omitempty" jsonschema:"description=Number of worker nodes to add (default: 1)"`
	BastionFlavor       string `json:"bastionFlavor,omitempty" jsonschema:"description=Flavor override for bastion nodes (optional)"`
	MasterFlavor        string `json:"masterFlavor,omitempty" jsonschema:"description=Flavor override for master nodes (optional)"`
	WorkerFlavor        string `json:"workerFlavor,omitempty" jsonschema:"description=Flavor override for worker nodes (optional)"`
	DiskSizeGB          int64  `json:"diskSizeGb,omitempty" jsonschema:"description=Root disk size in GB for all nodes (default 50 when omitted; some clouds require an explicit minimum)"`
	VerifyTimeout       int32  `json:"verifyTimeout,omitempty" jsonschema:"description=Seconds to wait when verifying node creation (default: 300)"`
	WaitForCreation     *bool  `json:"waitForCreation,omitempty" jsonschema:"description=Block until the project is Ready after commit (default: false). A full initial deploy often takes 10-30 minutes and can exceed MCP client request timeouts; leave false and poll wait-for-project/get-project-details instead."`
	Timeout             int32  `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds used by wait-for-project when waitForCreation=true (default: 1800)"`
}

type DeleteProjectArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=ID of the project to delete"`
}

type DeleteStandaloneVMArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=Project ID containing the standalone VM"`
	VMID      int32 `json:"vmId" jsonschema:"required,description=Standalone VM ID to delete"`
}

type RemoveAppFromCatalogArgs struct {
	CatalogID   int32  `json:"catalogId" jsonschema:"required,description=The catalog ID to remove the application from"`
	Repository  string `json:"repository,omitempty" jsonschema:"description=Repository name (optional - if not provided, will search by package name only)"`
	PackageName string `json:"packageName" jsonschema:"required,description=Package name"`
}

type ListCatalogAppsArgs struct {
	CatalogID int32  `json:"catalogId,omitempty" jsonschema:"description=The catalog ID to list applications from (optional - if not provided, lists from all catalogs)"`
	Limit     int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset    int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search    string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type CatalogAppSummary struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	Repository  string `json:"repository"`
	CatalogID   int32  `json:"catalogId"`
	CatalogName string `json:"catalogName"`
}

type CatalogAppListResponse struct {
	Applications []CatalogAppSummary `json:"applications"`
	Total        int                 `json:"total"`
	CatalogID    int32               `json:"catalogId"`
	Message      string              `json:"message"`
}

type CloudCredentialSummary struct {
	ID               int32  `json:"id"`
	Name             string `json:"name"`
	CloudType        string `json:"cloudType"`
	OrganizationName string `json:"organizationName"`
}

type CloudCredentialListResponse struct {
	Credentials []CloudCredentialSummary `json:"credentials"`
	Total       int                      `json:"total"`
	Message     string                   `json:"message"`
}

type ListCloudCredentialsArgs struct {
	Limit  int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type BindFlavorsArgs struct {
	ProjectId int32    `json:"projectId" jsonschema:"description=The ID of the project to bind flavors to"`
	Flavors   []string `json:"flavors" jsonschema:"description=List of flavor names to bind"`
}

type AddServerArgs struct {
	ProjectId            int32  `json:"projectId" jsonschema:"description=The ID of the project to add the server to"`
	Name                 string `json:"name" jsonschema:"description=The name of the server"`
	Role                 string `json:"role" jsonschema:"description=The role of the server (Bastion, Kubemaster, Kubeworker)"`
	Flavor               string `json:"flavor" jsonschema:"description=The flavor name for the server"`
	DiskSize             int64  `json:"diskSize,omitempty" jsonschema:"description=The disk size in GB (optional)"`
	Count                int32  `json:"count,omitempty" jsonschema:"description=Number of servers to add (default: 1)"`
	VerifyTimeoutSeconds int32  `json:"verifyTimeoutSeconds,omitempty" jsonschema:"description=Seconds to wait for server verification (default: 300)"`
}

type CommitProjectArgs struct {
	ProjectId int32 `json:"projectId" jsonschema:"description=The ID of the project to commit"`
}

type GetProjectDetailsArgs struct {
	ProjectId int32 `json:"projectId" jsonschema:"description=The ID of the project to get details for"`
}

type WaitForProjectArgs struct {
	ProjectId   int32 `json:"projectId" jsonschema:"required,description=The ID of the project to wait for"`
	Timeout     int32 `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (default: 600 for creation, 300 for deletion). When waitDeleted is true and the project was empty (no servers, VMs, or other resources to tear down), prefer a short timeout such as 10 to 30 seconds because purge usually finishes quickly."`
	WaitDeleted bool  `json:"waitDeleted,omitempty" jsonschema:"description=Wait for the project to be deleted (default: false). For an empty project after delete-project, set timeout to a small value (e.g. 10 to 30); use the default or longer when the project had infrastructure to remove."`
}

type WaitForAppArgs struct {
	ProjectAppId              int32 `json:"projectAppId" jsonschema:"required,description=The ID of the project application to wait for"`
	Timeout                   int32 `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (default: 60 for creation, 30 for deletion)"`
	WaitDeleted               bool  `json:"waitDeleted,omitempty" jsonschema:"description=Wait for the application to be deleted (default: false)"`
	ReadyStabilizationSeconds int32 `json:"readyStabilizationSeconds,omitempty" jsonschema:"description=Seconds the app must remain in Ready state before success (default: 30)"`
}

type DeleteServersArgs struct {
	ProjectId                int32   `json:"projectId" jsonschema:"required,description=The ID of the project"`
	ServerIds                []int32 `json:"serverIds" jsonschema:"required,description=List of server IDs to delete"`
	ForceDeleteVClusters     bool    `json:"forceDeleteVClusters,omitempty" jsonschema:"description=Force delete virtual clusters on these servers (default: false)"`
	DeleteAutoscalingServers bool    `json:"deleteAutoscalingServers,omitempty" jsonschema:"description=Delete autoscaling servers (default: false)"`
}

type ListFlavorsArgs struct {
	CloudCredentialId int32  `json:"cloudCredentialId" jsonschema:"description=The ID of the cloud credential to list flavors for"`
	Limit             int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset            int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search            string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type FlavorSummary struct {
	Name string  `json:"name"`
	CPU  int32   `json:"cpu"`
	RAM  float64 `json:"ram"`
}

type FlavorListResponse struct {
	Flavors []FlavorSummary `json:"flavors"`
	Total   int32           `json:"total"`
	Message string          `json:"message"`
}

type ListServersArgs struct {
	ProjectId int32 `json:"projectId" jsonschema:"description=The ID of the project to list servers for"`
}

type ServerSummary struct {
	ID        int32  `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	IPAddress string `json:"ipAddress"`
	Flavor    string `json:"flavor"`
}

type ServerListResponse struct {
	Servers []ServerSummary `json:"servers"`
	Total   int32           `json:"total"`
	Message string          `json:"message"`
}

type ProjectStatusResponse struct {
	ID        int32  `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Health    string `json:"health"`
	CloudType string `json:"cloudType"`
}

type ProjectAccessIPHints struct {
	IngressExposure string `json:"ingressExposure"`
	GatewayExposure string `json:"gatewayExposure"`
	ServerIPsNote   string `json:"serverIPsNote"`
	DNSNote         string `json:"dnsNote"`
}

type ProjectAccessIPResponse struct {
	ProjectID   int32                `json:"projectId"`
	ProjectName string               `json:"projectName"`
	AccessIP    string               `json:"accessIp"`
	CloudType   string               `json:"cloudType"`
	Status      string               `json:"status"`
	Health      string               `json:"health"`
	Hints       ProjectAccessIPHints `json:"hints"`
	Success     bool                 `json:"success"`
	Message     string               `json:"message"`
}

// createJSONResponse creates a JSON response using NewTextContent
func createJSONResponse(data interface{}) *mcp_golang.ToolResponse {
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Printf("Error marshaling JSON: %v", err)
		errorResp := ErrorResponse{Error: "Failed to serialize response data"}
		jsonData, _ = json.Marshal(errorResp)
	}
	return mcp_golang.NewToolResponse(
		mcp_golang.NewTextContent(string(jsonData)),
	)
}

// createError creates a formatted error response for MCP tools
func createError(response *http.Response, err error) *mcp_golang.ToolResponse {
	return apiErrorInfoFromResponse(response, err).toolResponse()
}

// checkResponse validates HTTP response status codes
func checkResponse(response *http.Response, operation string) *mcp_golang.ToolResponse {
	if response == nil {
		errorMsg := fmt.Sprintf("No response received for %s", operation)
		logger.Printf("Error: %s", errorMsg)
		return createJSONResponse(ErrorResponse{
			Error: errorMsg,
		})
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return apiErrorInfoFromResponse(response, nil).toolResponse()
	}

	return nil
}

// defaultLogFilePath resolves a writable, cross-platform default log location.
// It honors CCF_MCP_LOG_FILE when set; otherwise it uses /tmp when that
// directory exists (preserving the historical Linux path) and falls back to the
// OS temp directory (e.g. %TEMP% on Windows) so the server can start anywhere.
func defaultLogFilePath() string {
	if override := strings.TrimSpace(os.Getenv("CCF_MCP_LOG_FILE")); override != "" {
		return override
	}
	const logName = "cloudera_cloud_factory_mcp_server.log"
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		return "/tmp/" + logName
	}
	return filepath.Join(os.TempDir(), logName)
}

func initLogger() {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", logFilePath, err)
		os.Exit(1)
	}
	logger = log.New(logFile, "[cloudera-cloud-factory-mcp] ", log.LstdFlags|log.Lshortfile)
	logger.Println("Logger initialized")
}

type robotUserAuthConfig struct {
	APIHost    string
	DomainName string
	AccessKey  string
	SecretKey  string
}

func resolveRobotUserAuthConfig(getenv func(string) string) (robotUserAuthConfig, error) {
	cfg := robotUserAuthConfig{
		APIHost:    normalizeAPIHost(getenv("TAIKUN_API_HOST")),
		DomainName: strings.TrimSpace(getenv("TAIKUN_DOMAIN_NAME")),
		AccessKey:  strings.TrimSpace(getenv("TAIKUN_ACCESS_KEY")),
		SecretKey:  strings.TrimSpace(getenv("TAIKUN_SECRET_KEY")),
	}

	if cfg.APIHost == "" {
		cfg.APIHost = defaultAPIHost
	}

	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		if cfg.AccessKey == "" || cfg.SecretKey == "" {
			return robotUserAuthConfig{}, fmt.Errorf("incomplete Robot User credentials: set both TAIKUN_ACCESS_KEY and TAIKUN_SECRET_KEY")
		}
		return cfg, nil
	}

	email := strings.TrimSpace(getenv("TAIKUN_EMAIL"))
	password := strings.TrimSpace(getenv("TAIKUN_PASSWORD"))
	if email != "" || password != "" {
		return robotUserAuthConfig{}, fmt.Errorf("email/password authentication is no longer supported by this MCP server; configure Robot User credentials with TAIKUN_ACCESS_KEY and TAIKUN_SECRET_KEY")
	}

	return robotUserAuthConfig{}, fmt.Errorf("missing Robot User credentials: set TAIKUN_ACCESS_KEY and TAIKUN_SECRET_KEY; optionally set TAIKUN_API_HOST and TAIKUN_DOMAIN_NAME")
}

func createTaikunClient() *taikungoclient.Client {
	cfg, err := resolveRobotUserAuthConfig(os.Getenv)
	if err != nil {
		logger.Fatal(err.Error())
		return nil
	}

	apiHost := cfg.APIHost
	if apiHost == "" {
		apiHost = defaultAPIHost
	}
	logger.Printf("Using API host: %s", apiHost)

	if cfg.DomainName != "" {
		logger.Printf("Using Cloudera Cloud Factory domain name: %s", cfg.DomainName)
	}
	if strings.TrimSpace(os.Getenv("TAIKUN_AUTH_MODE")) != "" {
		logger.Printf("Ignoring TAIKUN_AUTH_MODE for Robot User authentication")
	}

	logger.Printf("Using Robot User authentication via access key/secret key")
	return taikungoclient.NewClientFromAccessKey(cfg.DomainName, cfg.AccessKey, cfg.SecretKey, apiHost)
}

func refreshTaikunClientCtx(ctx context.Context) *mcp_golang.ToolResponse {
	client := clientFromContext(ctx)
	if client == taikunClient {
		// stdio mode: re-read credentials from environment variables
		taikunClient = createTaikunClient()
		robotCtx := refreshRobotUserContext()
		return createJSONResponse(newRefreshTaikunClientResponse(robotCtx))
	}
	// HTTP mode: fetch robot user context for the per-request credentials and
	// refresh the cached entry so subsequent tool calls observe updated scopes.
	credKey := credentialKeyFromContext(ctx)
	invalidateCachedRobotContext(credKey)
	robotCtx, err := fetchRobotUserContext(client)
	if err != nil {
		robotCtx = RobotUserContext{ScopeDiscoveryError: err.Error()}
	} else {
		setCachedRobotContext(credKey, robotCtx)
	}
	return createJSONResponse(newRefreshTaikunClientResponse(robotCtx))
}

func getRobotUserCapabilitiesCtx(ctx context.Context, detailed bool) *mcp_golang.ToolResponse {
	// resolveRobotUserContext serves the global context in stdio mode and a
	// short-TTL cached context (per credentials) in HTTP mode.
	robotCtx := resolveRobotUserContext(ctx)
	if detailed {
		return createJSONResponse(buildCapabilitiesResponse(robotCtx))
	}
	return createJSONResponse(buildCompactCapabilitiesResponse(robotCtx))
}

func newRefreshTaikunClientResponse(robotCtx RobotUserContext) RefreshTaikunClientResponse {
	return RefreshTaikunClientResponse{
		Message:             "Cloudera Cloud Factory client refreshed successfully",
		Success:             robotCtx.ScopeDiscoveryError == "",
		RobotUserName:       robotCtx.Name,
		OrganizationName:    robotCtx.OrganizationName,
		Scopes:              robotCtx.Scopes,
		ScopeDiscoveryError: robotCtx.ScopeDiscoveryError,
	}
}

func serverVersion() *mcp_golang.ToolResponse {
	return createJSONResponse(ServerVersionResponse{
		Name:    mcpServerName,
		Version: version,
		Commit:  commit,
		Date:    date,
		BuiltBy: builtBy,
		Success: true,
		Message: fmt.Sprintf("Loaded MCP server version information for %s", mcpServerName),
	})
}

func main() {
	// Handle version command
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("Cloudera Cloud Factory MCP Server %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built: %s\n", date)
		fmt.Printf("  by: %s\n", builtBy)
		return
	}

	transportFlag := flag.String("transport", "stdio", "Transport type: stdio or http")
	addrFlag := flag.String("addr", ":8080", "Listen address for HTTP transport (e.g. :8080)")
	endpointFlag := flag.String("endpoint", "/mcp", "HTTP endpoint path for the MCP handler")
	flag.Parse()

	initLogger()
	logger.Printf("Starting Cloudera Cloud Factory MCP server v%s (transport=%s)", version, *transportFlag)
	if err := initMCPLockFromConfig(os.Getenv, flag.Args()); err != nil {
		logger.Fatalf("Failed to initialize MCP lock: %v", err)
	}

	var server *mcp_golang.Server
	var httpTransport *authHTTPTransport

	switch *transportFlag {
	case "http":
		httpTransportMode = true
		httpTransport = newAuthHTTPTransport(*addrFlag, *endpointFlag)
		server = mcp_golang.NewServer(
			httpTransport,
			mcp_golang.WithName(mcpServerName),
			mcp_golang.WithVersion(version),
		)
		logger.Println("MCP server created (HTTP transport, per-request credentials)")
	default:
		server = mcp_golang.NewServer(
			stdio.NewStdioServerTransport(),
			mcp_golang.WithName(mcpServerName),
			mcp_golang.WithVersion(version),
		)
		logger.Println("MCP server created (stdio transport)")
		// In stdio mode, initialize the global client once from environment variables
		taikunClient = createTaikunClient()
		refreshRobotUserContext()
		logger.Println("Cloudera Cloud Factory client initialized")
	}

	logger.Println("Starting tool registration...")

	// --- MCP Tool Registrations ---

	err := registerScopedTool(server, "server-version", "Show MCP server version and build metadata", func(ctx context.Context, args ServerVersionArgs) (*mcp_golang.ToolResponse, error) {
		return serverVersion(), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register server-version tool: %v", err)
	}
	logger.Println("Registered server-version tool")

	err = registerScopedTool(server, "refresh-taikun-client", "Refresh the Cloudera Cloud Factory API client using current environment credentials", func(ctx context.Context, args RefreshTaikunClientArgs) (*mcp_golang.ToolResponse, error) {
		return refreshTaikunClientCtx(ctx), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register refresh-taikun-client tool: %v", err)
	}
	logger.Println("Registered refresh-taikun-client tool")

	err = registerScopedTool(server, "robot-user-capabilities", "Show the current Robot User identity, scopes, and which MCP tools it can use. Returns compact allowed/blocked tool name lists by default; pass detailed=true for the full per-tool scope matrix.", func(ctx context.Context, args RobotUserCapabilitiesArgs) (*mcp_golang.ToolResponse, error) {
		return getRobotUserCapabilitiesCtx(ctx, args.Detailed), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register robot-user-capabilities tool: %v", err)
	}
	logger.Println("Registered robot-user-capabilities tool")

	err = registerScopedTool(server, "mcp-lock", "Set runtime MCP org/project scope lock allowlists", func(ctx context.Context, args MCPLockArgs) (*mcp_golang.ToolResponse, error) {
		return mcpLock(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register mcp-lock tool: %v", err)
	}
	logger.Println("Registered mcp-lock tool")

	err = registerScopedTool(server, "mcp-lock-status", "Show current MCP lock configuration and effective scope", func(ctx context.Context, args MCPLockStatusArgs) (*mcp_golang.ToolResponse, error) {
		return mcpLockStatus(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register mcp-lock-status tool: %v", err)
	}
	logger.Println("Registered mcp-lock-status tool")

	err = registerScopedTool(server, "mcp-lock-clear", "Clear runtime MCP lock and fall back to environment lock", func(ctx context.Context, args MCPLockClearArgs) (*mcp_golang.ToolResponse, error) {
		return mcpLockClear(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register mcp-lock-clear tool: %v", err)
	}
	logger.Println("Registered mcp-lock-clear tool")

	err = registerScopedTool(server, "create-virtual-cluster", "Create a new virtual cluster (a project in Cloudera Cloud Factory) with optional wait for completion", func(ctx context.Context, args CreateVirtualClusterArgs) (*mcp_golang.ToolResponse, error) {
		return createVirtualCluster(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-virtual-cluster tool: %v", err)
	}
	logger.Println("Registered create-virtual-cluster tool")

	err = registerScopedTool(server, "delete-virtual-cluster", "Delete a virtual cluster (a project in Cloudera Cloud Factory)", func(ctx context.Context, args DeleteVirtualClusterArgs) (*mcp_golang.ToolResponse, error) {
		return deleteVirtualCluster(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-virtual-cluster tool: %v", err)
	}
	logger.Println("Registered delete-virtual-cluster tool")

	err = registerScopedTool(server, "list-virtual-clusters", "List virtual clusters in a parent project (projects in Cloudera Cloud Factory)", func(ctx context.Context, args ListVirtualClustersArgs) (*mcp_golang.ToolResponse, error) {
		return listVirtualClusters(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-virtual-clusters tool: %v", err)
	}
	logger.Println("Registered list-virtual-clusters tool")

	err = registerScopedTool(server, "catalog-create", "Create a new catalog", func(ctx context.Context, args CreateCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return createCatalog(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-create tool: %v", err)
	}
	logger.Println("Registered catalog-create tool")

	err = registerScopedTool(server, "catalog-list", "List catalogs with optional filtering", func(ctx context.Context, args ListCatalogsArgs) (*mcp_golang.ToolResponse, error) {
		return listCatalogs(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-list tool: %v", err)
	}
	logger.Println("Registered catalog-list tool")

	err = registerScopedTool(server, "catalog-delete", "Delete a catalog", func(ctx context.Context, args DeleteCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return deleteCatalog(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-delete tool: %v", err)
	}
	logger.Println("Registered catalog-delete tool")

	err = registerScopedTool(server, "bind-projects-to-catalog", "Bind one or more projects to a catalog so they can install apps from it. Projects must be Ready and have an admin kubeconfig; run preflight-project or wait-for-project first.", func(ctx context.Context, args BindProjectsToCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return bindProjectsToCatalog(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register bind-projects-to-catalog tool: %v", err)
	}
	logger.Println("Registered bind-projects-to-catalog tool")

	err = registerScopedTool(server, "unbind-projects-from-catalog", "Unbind projects from a catalog", func(ctx context.Context, args UnbindProjectsFromCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return unbindProjectsFromCatalog(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register unbind-projects-from-catalog tool: %v", err)
	}
	logger.Println("Registered unbind-projects-from-catalog tool")

	err = registerScopedTool(server, "available-apps-list", "List available apps from the package repository", func(ctx context.Context, args ListAvailableAppsArgs) (*mcp_golang.ToolResponse, error) {
		return listAvailableApps(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register available-apps-list tool: %v", err)
	}
	logger.Println("Registered available-apps-list tool")

	err = registerScopedTool(server, "list-repositories", "List repositories with optional filtering", func(ctx context.Context, args ListRepositoriesArgs) (*mcp_golang.ToolResponse, error) {
		return listRepositories(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-repositories tool: %v", err)
	}
	logger.Println("Registered list-repositories tool")

	err = registerScopedTool(server, "import-repository", "Import a repository from a URL with optional credentials", func(ctx context.Context, args ImportRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return importRepository(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register import-repository tool: %v", err)
	}
	logger.Println("Registered import-repository tool")

	err = registerScopedTool(server, "bind-repository", "Bind a repository to an organization so its apps become available", func(ctx context.Context, args BindRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return bindRepository(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register bind-repository tool: %v", err)
	}
	logger.Println("Registered bind-repository tool")

	err = registerScopedTool(server, "unbind-repository", "Unbind one or more repository IDs from an organization", func(ctx context.Context, args UnbindRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return unbindRepository(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register unbind-repository tool: %v", err)
	}
	logger.Println("Registered unbind-repository tool")

	err = registerScopedTool(server, "delete-repository", "Delete an imported repository", func(ctx context.Context, args DeleteRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return deleteRepository(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-repository tool: %v", err)
	}
	logger.Println("Registered delete-repository tool")

	err = registerScopedTool(server, "update-repository-password", "Update stored credentials for a private repository", func(ctx context.Context, args UpdateRepositoryPasswordArgs) (*mcp_golang.ToolResponse, error) {
		return updateRepositoryPassword(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register update-repository-password tool: %v", err)
	}
	logger.Println("Registered update-repository-password tool")

	err = registerScopedTool(server, "catalog-app-add", "Add an application to a catalog with optional default parameters", func(ctx context.Context, args AddAppToCatalogWithParametersArgs) (*mcp_golang.ToolResponse, error) {
		return addAppToCatalogWithParameters(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-add tool: %v", err)
	}
	logger.Println("Registered catalog-app-add tool")

	err = registerScopedTool(server, "catalog-app-remove", "Remove an application from a catalog by package name and optional repository", func(ctx context.Context, args RemoveAppFromCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return removeAppFromCatalog(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-remove tool: %v", err)
	}
	logger.Println("Registered catalog-app-remove tool")

	err = registerScopedTool(server, "catalog-apps-list", "List applications in a specific catalog or all catalogs", func(ctx context.Context, args ListCatalogAppsArgs) (*mcp_golang.ToolResponse, error) {
		return listCatalogApps(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-apps-list tool: %v", err)
	}
	logger.Println("Registered catalog-apps-list tool")

	err = registerScopedTool(server, "catalog-app-params", "Get available and added parameters for a catalog application", func(ctx context.Context, args GetCatalogAppParamsArgs) (*mcp_golang.ToolResponse, error) {
		return getCatalogAppParameters(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-params tool: %v", err)
	}
	logger.Println("Registered catalog-app-params tool")

	err = registerScopedTool(server, "catalog-app-defaults-set", "Update default parameters for a catalog application (merges with existing defaults by default)", func(ctx context.Context, args SetCatalogAppDefaultParamsArgs) (*mcp_golang.ToolResponse, error) {
		return updateCatalogAppParameters(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-defaults-set tool: %v", err)
	}
	logger.Println("Registered catalog-app-defaults-set tool")

	err = registerScopedTool(server, "app-install", "Install a new application instance with optional defaults and overrides. The target project must be Ready and have an admin kubeconfig; run preflight-project or wait-for-project first. If timeout is omitted, the install request defaults to 10 minutes; TTL defaults to 10 minutes; larger applications may need a higher timeout.", func(ctx context.Context, args InstallAppArgs) (*mcp_golang.ToolResponse, error) {
		return installApp(ctx, clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register app-install tool: %v", err)
	}
	logger.Println("Registered app-install tool")

	err = registerScopedTool(server, "list-apps", "List application instances in a project", func(ctx context.Context, args ListAppsArgs) (*mcp_golang.ToolResponse, error) {
		return listApps(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-apps tool: %v", err)
	}
	logger.Println("Registered list-apps tool")

	err = registerScopedTool(server, "get-app", "Get detailed application instance information", func(ctx context.Context, args GetAppArgs) (*mcp_golang.ToolResponse, error) {
		return getApp(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-app tool: %v", err)
	}
	logger.Println("Registered get-app tool")

	err = registerScopedTool(server, "update-app-autosync", "Update application autosync configuration", func(ctx context.Context, args UpdateAppAutoSyncArgs) (*mcp_golang.ToolResponse, error) {
		return updateAppAutoSync(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register update-app-autosync tool: %v", err)
	}
	logger.Println("Registered update-app-autosync tool")

	err = registerScopedTool(server, "update-sync-app", "Update application values and sync", func(ctx context.Context, args UpdateSyncAppArgs) (*mcp_golang.ToolResponse, error) {
		return updateSyncApp(ctx, clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register update-sync-app tool: %v", err)
	}
	logger.Println("Registered update-sync-app tool")

	err = registerScopedTool(server, "uninstall-app", "Uninstall an application instance", func(ctx context.Context, args UninstallAppArgs) (*mcp_golang.ToolResponse, error) {
		return uninstallApp(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register uninstall-app tool: %v", err)
	}
	logger.Println("Registered uninstall-app tool")

	err = registerScopedTool(server, "wait-for-app", "Wait for an application instance to be ready. In HTTP mode the wait is capped at 120 seconds to avoid long-held requests; if the result has status \"pending\", call wait-for-app again or poll get-app.", func(ctx context.Context, args WaitForAppArgs) (*mcp_golang.ToolResponse, error) {
		return waitForApp(ctx, clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register wait-for-app tool: %v", err)
	}
	logger.Println("Registered wait-for-app tool")

	err = registerScopedTool(server, "list-projects", "List projects with optional virtual cluster filtering", func(ctx context.Context, args ListProjectsArgs) (*mcp_golang.ToolResponse, error) {
		return listProjects(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-projects tool: %v", err)
	}
	logger.Println("Registered list-projects tool")

	err = registerScopedTool(server, "create-project", "Create a project in Cloudera Cloud Factory", func(ctx context.Context, args CreateProjectArgs) (*mcp_golang.ToolResponse, error) {
		return createProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-project tool: %v", err)
	}
	logger.Println("Registered create-project tool")

	err = registerScopedTool(server, "create-cluster", "Create a Kubernetes cluster end-to-end (project, nodes, commit) using profile-aware defaults and cloud-credential flavor discovery. Returns once the commit is accepted; the cluster then provisions asynchronously (often 10-30 minutes). Poll wait-for-project or get-project-details for readiness. Set waitForCreation=true only if you accept a long-blocking call that may exceed MCP client timeouts.", func(ctx context.Context, args CreateClusterArgs) (*mcp_golang.ToolResponse, error) {
		return createCluster(ctx, clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-cluster tool: %v", err)
	}
	logger.Println("Registered create-cluster tool")

	err = registerScopedTool(server, "delete-project", "Delete a project in Cloudera Cloud Factory. To confirm removal, call wait-for-project with waitDeleted true; if the project was empty, use a short timeout (about 10 to 30 seconds) because purge is usually fast.", func(ctx context.Context, args DeleteProjectArgs) (*mcp_golang.ToolResponse, error) {
		return deleteProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-project tool: %v", err)
	}
	logger.Println("Registered delete-project tool")

	err = registerScopedTool(server, "wait-for-project", "Wait for a project to be ready and healthy, or with waitDeleted for completion of project deletion. After delete-project on an empty project, pass a short timeout (e.g. 10 to 30 seconds) with waitDeleted true; projects that had servers or VMs typically need longer. In HTTP mode the wait is capped at 120 seconds; if the result has status \"pending\", call wait-for-project again or poll get-project-details.", func(ctx context.Context, args WaitForProjectArgs) (*mcp_golang.ToolResponse, error) {
		return waitForProject(ctx, clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register wait-for-project tool: %v", err)
	}
	logger.Println("Registered wait-for-project tool")

	err = registerScopedTool(server, "deploy-kubernetes-resources", "Deploy Kubernetes resources via YAML in a project", func(ctx context.Context, args DeployKubernetesResourcesArgs) (*mcp_golang.ToolResponse, error) {
		return deployKubernetesResources(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register deploy-kubernetes-resources tool: %v", err)
	}
	logger.Println("Registered deploy-kubernetes-resources tool")

	err = registerScopedTool(server, "create-kubeconfig", "Create a new kubeconfig for a project", func(ctx context.Context, args CreateKubeConfigArgs) (*mcp_golang.ToolResponse, error) {
		return createKubeConfig(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-kubeconfig tool: %v", err)
	}
	logger.Println("Registered create-kubeconfig tool")

	err = registerScopedTool(server, "get-kubeconfig", "Retrieve the kubeconfig content for a project (optionally save as YAML)", func(ctx context.Context, args GetKubeConfigArgs) (*mcp_golang.ToolResponse, error) {
		return getKubeConfig(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-kubeconfig tool: %v", err)
	}
	logger.Println("Registered get-kubeconfig tool")

	err = registerScopedTool(server, "list-kubeconfig-roles", "List available roles for kubeconfigs", func(ctx context.Context, args ListKubeConfigRolesArgs) (*mcp_golang.ToolResponse, error) {
		return listKubeConfigRoles(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-kubeconfig-roles tool: %v", err)
	}
	logger.Println("Registered list-kubeconfig-roles tool")

	err = registerScopedTool(server, "list-kubernetes-resource-kinds", "Show the supported Kubernetes resource kinds for list, describe, and delete operations. Kind matching is case-insensitive.", func(ctx context.Context, args KubernetesResourceKindsArgs) (*mcp_golang.ToolResponse, error) {
		return listKubernetesResourceKinds(), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register list-kubernetes-resource-kinds tool: %v", err)
	}
	logger.Println("Registered list-kubernetes-resource-kinds tool")

	err = registerScopedTool(server, "list-kubernetes-resources", "List specialized Kubernetes resources in a project. Kind matching is case-insensitive; call list-kubernetes-resource-kinds to inspect supported listKinds and unavailableListKinds.", func(ctx context.Context, args ListKubernetesResourcesArgs) (*mcp_golang.ToolResponse, error) {
		return listKubernetesResources(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-kubernetes-resources tool: %v", err)
	}
	logger.Println("Registered list-kubernetes-resources tool")

	err = registerScopedTool(server, "describe-kubernetes-resource", "Describe a specialized Kubernetes resource in a project. Kind matching is case-insensitive; call list-kubernetes-resource-kinds to inspect supported operationKinds.", func(ctx context.Context, args DescribeKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
		return describeKubernetesResource(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register describe-kubernetes-resource tool: %v", err)
	}
	logger.Println("Registered describe-kubernetes-resource tool")

	err = registerScopedTool(server, "delete-kubernetes-resource", "Delete a Kubernetes resource. Kind matching is case-insensitive; call list-kubernetes-resource-kinds to inspect supported operationKinds.", func(ctx context.Context, args DeleteKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
		return deleteKubernetesResource(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-kubernetes-resource tool: %v", err)
	}
	logger.Println("Registered delete-kubernetes-resource tool")

	err = registerScopedTool(server, "patch-kubernetes-resource", "Patch a Kubernetes resource using YAML", func(ctx context.Context, args PatchKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
		return patchKubernetesResource(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register patch-kubernetes-resource tool: %v", err)
	}
	logger.Println("Registered patch-kubernetes-resource tool")

	err = registerScopedTool(server, "list-cloud-credentials", "List cloud credentials", func(ctx context.Context, args ListCloudCredentialsArgs) (*mcp_golang.ToolResponse, error) {
		return listCloudCredentials(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-cloud-credentials tool: %v", err)
	}
	logger.Println("Registered list-cloud-credentials tool")

	err = registerScopedTool(server, "bind-flavors-to-project", "Bind flavors to a project", func(ctx context.Context, args BindFlavorsArgs) (*mcp_golang.ToolResponse, error) {
		return bindFlavorsToProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register bind-flavors-to-project tool: %v", err)
	}
	logger.Println("Registered bind-flavors-to-project tool")

	err = registerScopedTool(server, "add-server-to-project", "Add a Kubernetes server to a project. Recommendation: Bastion can use 2 CPUs / 2GB RAM; Kubemaster must be at least 4 CPUs / 4GB RAM; if monitoring is enabled, include at least one Kubeworker with 4 CPUs / 4GB RAM before commit-project.", func(ctx context.Context, args AddServerArgs) (*mcp_golang.ToolResponse, error) {
		return addServerToProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register add-server-to-project tool: %v", err)
	}
	logger.Println("Registered add-server-to-project tool")

	err = registerScopedTool(server, "commit-project", "Commit and provision pending project infrastructure in the cloud. For Kubernetes changes, commit-project validates that all Kubemaster nodes are at least 4 CPUs / 4GB RAM and requires at least one Kubeworker at 4 CPUs / 4GB RAM when monitoring is enabled. For VM-only changes, this tool automatically falls back to the VM commit endpoint used by the UI when the cluster-style commit path is not applicable. If the project is already Updating (a commit or repair is in progress) this tool returns an error asking you to wait; poll get-project-details or wait-for-project first. A full initial Kubernetes deploy often takes 10-30 minutes.", func(ctx context.Context, args CommitProjectArgs) (*mcp_golang.ToolResponse, error) {
		return commitProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register commit-project tool: %v", err)
	}
	logger.Println("Registered commit-project tool")

	err = registerScopedTool(server, "describe-payload", "Describe the JSON payload a tool expects, so you don't have to inspect API DTOs or Swagger. Pass a tool name (e.g. create-standalone-vm) or a command type name (e.g. CreateStandAloneVmCommand) to get its fields, types, optional/nullable flags, nested objects, and an example skeleton. Call with no arguments to list every known payload and tool alias.", func(ctx context.Context, args DescribePayloadArgs) (*mcp_golang.ToolResponse, error) {
		return describePayload(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register describe-payload tool: %v", err)
	}
	logger.Println("Registered describe-payload tool")

	err = registerScopedTool(server, "preflight-project", "Check whether a project is ready to commit and to host catalog/app deployments. Reports project status, Kubemaster/Kubeworker sizing against commit minimums, and kubeconfig availability as a single pass/warn/fail checklist so prerequisites are surfaced before commit-project or app-install fail.", func(ctx context.Context, args GetProjectDetailsArgs) (*mcp_golang.ToolResponse, error) {
		return preflightProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register preflight-project tool: %v", err)
	}
	logger.Println("Registered preflight-project tool")

	err = registerScopedTool(server, "get-project-details", "Get detailed status of a project. For ingress and Gateway API reachability, also call get-project-access-ip — server private IPs from list-servers are not the external entry point on private clouds.", func(ctx context.Context, args GetProjectDetailsArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectDetails(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-project-details tool: %v", err)
	}
	logger.Println("Registered get-project-details tool")

	err = registerScopedTool(server, "get-project-access-ip", "Get the project Access IP from GET /api/v1/servers/{projectId} (project.accessIp). This is the external entry point for default Ingress class taikun and Gateway API routes. On OpenStack and similar private clouds, Traefik is typically NodePort on this IP — not on server private IPs from list-servers.", func(ctx context.Context, args GetProjectDetailsArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectAccessIP(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-project-access-ip tool: %v", err)
	}
	logger.Println("Registered get-project-access-ip tool")

	err = registerScopedTool(server, "list-flavors", "List available flavors for a cloud credential", func(ctx context.Context, args ListFlavorsArgs) (*mcp_golang.ToolResponse, error) {
		return listFlavors(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-flavors tool: %v", err)
	}
	logger.Println("Registered list-flavors tool")

	err = registerScopedTool(server, "list-servers", "List servers in a project. ipAddress values are typically private/internal addresses. For ingress and Gateway API traffic use get-project-access-ip instead.", func(ctx context.Context, args ListServersArgs) (*mcp_golang.ToolResponse, error) {
		return listServers(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-servers tool: %v", err)
	}
	logger.Println("Registered list-servers tool")

	err = registerScopedTool(server, "delete-servers-from-project", "Delete servers from a project", func(ctx context.Context, args DeleteServersArgs) (*mcp_golang.ToolResponse, error) {
		return deleteServersFromProject(clientFromContext(ctx), args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-servers-from-project tool: %v", err)
	}
	logger.Println("Registered delete-servers-from-project tool")

	mustRegisterScopedTool(server, "list-domains", "List domains", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listDomains(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-domain", "Create a domain", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createDomain(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-domain-details", "Get domain details", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return getDomainDetails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-domain", "Update a domain", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateDomain(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-domain", "Delete a domain", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteDomain(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-organizations", "List organizations", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listOrganizations(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-organization", "Create an organization", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createOrganization(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-organization-details", "Get organization details", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return getOrganizationDetails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-organization", "Update an organization", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateOrganization(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-organization", "Delete an organization", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteOrganization(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-identity-groups", "List identity groups within a domain", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listIdentityGroups(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-identity-group", "Create an identity group", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-identity-group-details", "Get identity group details within a domain", func(ctx context.Context, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return getIdentityGroupDetails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-identity-group-organizations", "List organizations assigned to an identity group", func(ctx context.Context, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listIdentityGroupOrganizations(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-identity-group-users", "List users assigned to an identity group", func(ctx context.Context, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listIdentityGroupUsers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-available-group-organizations", "List organizations available to add to an identity group", func(ctx context.Context, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listAvailableIdentityGroupOrganizations(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-available-identity-group-users", "List users available to add to an identity group", func(ctx context.Context, args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listAvailableIdentityGroupUsers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "add-organizations-to-identity-group", "Add organizations to an identity group", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return addOrganizationsToIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-identity-group-organization", "Update an organization's membership settings in an identity group", func(ctx context.Context, args GroupOrganizationPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateIdentityGroupOrganization(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "remove-organizations-from-group", "Remove organizations from an identity group", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return removeOrganizationsFromIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "add-users-to-identity-group", "Add users to an identity group", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return addUsersToIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "remove-users-from-identity-group", "Remove users from an identity group", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return removeUsersFromIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-identity-group", "Update an identity group", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-identity-group", "Delete an identity group", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteIdentityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-users", "List users within a domain. domainId is required (it scopes the lookup to a specific domain/account).", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listUsers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-user", "Create a user", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createUser(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-user-details", "Get user details within a domain", func(ctx context.Context, args DomainScopedStringIDArgs) (*mcp_golang.ToolResponse, error) {
		return getUserDetails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-user", "Update a user", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateUser(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-user", "Delete a user", func(ctx context.Context, args StringIDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteUser(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-access-profiles", "List access profiles", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listAccessProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-access-profile", "Create an access profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAccessProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-access-profile", "Update an access profile", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAccessProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-access-profile", "Delete an access profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAccessProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-access-profiles", "List access profile dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownAccessProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-access-profile", "Lock or unlock an access profile", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockAccessProfile(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-ai-credentials", "List AI credentials", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listAICredentials(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-ai-credential", "Create an AI credential", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAICredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-ai-credential", "Delete an AI credential", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAICredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-ai-credentials", "List AI credential dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownAICredentials(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-kubernetes-profiles", "List Kubernetes profiles", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listKubernetesProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-kubernetes-profile", "Create a Kubernetes profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createKubernetesProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-kubernetes-profile", "Delete a Kubernetes profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteKubernetesProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-kubernetes-profiles", "List Kubernetes profile dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownKubernetesProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-kubernetes-profile", "Lock or unlock a Kubernetes profile", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockKubernetesProfile(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-opa-profiles", "List OPA profiles", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listOPAProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-opa-profile", "Create an OPA profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createOPAProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-opa-profile", "Update an OPA profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateOPAProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-opa-profile", "Delete an OPA profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteOPAProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-opa-profiles", "List OPA profile dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownOPAProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-opa-profile", "Lock or unlock an OPA profile", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockOPAProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "sync-opa-profile", "Sync an OPA profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return syncOPAProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "make-opa-profile-default", "Make an OPA profile default", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return makeOPAProfileDefault(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-alerting-profiles", "List alerting profiles", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listAlertingProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-alerting-profile", "Create an alerting profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAlertingProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-alerting-profile", "Update an alerting profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAlertingProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-alerting-profile", "Delete an alerting profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAlertingProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-alerting-profiles", "List alerting profile dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownAlertingProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-alerting-profile", "Lock or unlock an alerting profile", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockAlertingProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "attach-alerting-profile", "Attach an alerting profile to a project", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return attachAlertingProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "detach-alerting-profile", "Detach an alerting profile from a project", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return detachAlertingProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "assign-alerting-emails", "Assign alerting emails to a profile", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return assignAlertingEmails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "assign-alerting-webhooks", "Assign alerting webhooks to a profile", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return assignAlertingWebhooks(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "verify-alerting-webhook", "Verify an alerting webhook", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return verifyAlertingWebhook(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-alerting-integrations", "List alerting integrations for a profile", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return listAlertingIntegrations(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-alerting-integration", "Create an alerting integration", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAlertingIntegration(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-alerting-integration", "Update an alerting integration", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAlertingIntegration(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-alerting-integration", "Delete an alerting integration", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAlertingIntegration(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-backup-credentials", "List backup credentials", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listBackupCredentials(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-backup-credential", "Create a backup credential", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createBackupCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-backup-credential", "Update a backup credential", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateBackupCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-backup-credential", "Delete a backup credential", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteBackupCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-backup-credentials", "List backup credential dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownBackupCredentials(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "make-backup-credential-default", "Make a backup credential default", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return makeBackupCredentialDefault(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-backup-credential", "Lock or unlock a backup credential", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockBackupCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-backup-policy", "Create a backup policy", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createBackupPolicy(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-backup-by-name", "Get backup details by project and name", func(ctx context.Context, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return getBackupByName(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-project-backups", "List backups for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackups(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-project-restore-requests", "List restore requests for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectRestoreRequests(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-project-backup-schedules", "List backup schedules for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackupSchedules(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-project-backup-locations", "List backup storage locations for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackupStorageLocations(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-project-backup-delete-requests", "List backup delete requests for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackupDeleteRequests(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "describe-backup", "Describe a backup by project and name", func(ctx context.Context, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return describeBackup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "describe-restore", "Describe a restore by project and name", func(ctx context.Context, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return describeRestore(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "describe-schedule", "Describe a backup schedule by project and name", func(ctx context.Context, args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return describeSchedule(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-backup", "Delete a backup", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteBackup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-backup-storage-location", "Delete a backup storage location", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteBackupStorageLocation(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-restore", "Delete a restore request", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteRestore(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-schedule", "Delete a backup schedule", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteSchedule(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "import-backup-storage-location", "Import a backup storage location", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return importBackupStorageLocation(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "restore-backup", "Restore a backup", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return restoreBackup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-backup", "Enable backup for a project using a backup credential", func(ctx context.Context, args ProjectBackupCredentialArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectBackup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-backup", "Disable backup for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectBackup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-monitoring", "Enable monitoring for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectMonitoring(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-monitoring", "Disable monitoring for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectMonitoring(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-project-monitoring-alerts", "Read Prometheus-style monitoring alerts for a project. Monitoring must be enabled on the project first.", func(ctx context.Context, args ProjectMonitoringAlertsArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectMonitoringAlerts(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-project-alerts", "Read project detail alerts/messages for a project. Monitoring must be enabled on the project first.", func(ctx context.Context, args ProjectAlertsArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectAlerts(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "query-project-loki-logs", "Query Loki logs for a project. Monitoring must be enabled and results can be large.", func(ctx context.Context, args QueryProjectLokiLogsArgs) (*mcp_golang.ToolResponse, error) {
		return queryProjectLokiLogs(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "export-project-loki-logs", "Export Loki logs for a project. Monitoring must be enabled; returns the API CSV export payload.", func(ctx context.Context, args ExportProjectLokiLogsArgs) (*mcp_golang.ToolResponse, error) {
		return exportProjectLokiLogs(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "query-project-prometheus-metrics", "Query Prometheus metrics for a project. Monitoring must be enabled and the result payload may be large.", func(ctx context.Context, args QueryProjectPrometheusMetricsArgs) (*mcp_golang.ToolResponse, error) {
		return queryProjectPrometheusMetrics(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "autocomplete-project-metrics", "Return Prometheus metric autocomplete suggestions for a project. Monitoring must be enabled.", func(ctx context.Context, args ProjectPrometheusMetricsAutocompleteArgs) (*mcp_golang.ToolResponse, error) {
		return autocompleteProjectPrometheusMetrics(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-ai-assistant", "Enable AI Assistant for a project using an AI credential", func(ctx context.Context, args ProjectAICredentialArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectAIAssistant(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-ai-assistant", "Disable AI Assistant for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectAIAssistant(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-policy", "Enable policy enforcement for a project using a policy profile", func(ctx context.Context, args ProjectPolicyProfileArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectPolicy(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-policy", "Disable policy enforcement for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectPolicy(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-full-spot", "Enable full spot support for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectFullSpot(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-full-spot", "Disable full spot support for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectFullSpot(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-spot-workers", "Enable spot workers for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectSpotWorkers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-spot-workers", "Disable spot workers for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectSpotWorkers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-project-spot-vms", "Enable spot VMs for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectSpotVMs(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-project-spot-vms", "Disable spot VMs for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectSpotVMs(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-project-service-status", "Get current project service settings and bindings, including project accessIp (external entry point for default Ingress class taikun and Gateway API routes)", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectServiceStatus(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-images", "List images for a provider. For aws public images, supply a payload body (AwsImagesPostListCommand with owners/filters/architecture); cloudId alone is not sufficient for aws public mode.", func(ctx context.Context, args ImageListArgs) (*mcp_golang.ToolResponse, error) {
		return listImages(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-image-details", "Get image details", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return getImageDetails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "bind-images-to-project", "Bind images to a project. Primarily for standalone VM workflows; Kubernetes project deployment does not require image binding.", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return bindImagesToProject(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "unbind-images-from-project", "Unbind images from a project", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return unbindImagesFromProject(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-selected-project-images", "List selected images for a project", func(ctx context.Context, args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listSelectedProjectImages(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "enable-autoscaling", "Enable project autoscaling", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return enableAutoscaling(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-autoscaling", "Update project autoscaling", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAutoscaling(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-autoscaling", "Disable project autoscaling", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return disableAutoscaling(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-autoscaling-status", "Get project autoscaling status", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return getAutoscalingStatus(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "list-standalone-vms", "List standalone VMs in a project", func(ctx context.Context, args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listStandaloneVMs(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-details", "Get standalone VM details", func(ctx context.Context, args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMDetails(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-standalone-vm", "Create a standalone VM (payload: CreateStandAloneVmCommand). The image field expects the provider image ID (for example an AWS AMI ID), not the display name. If payload omits volumeSize, the tool defaults it to 10 GiB; for Windows images, prefer 50 GiB. You can batch multiple VM changes and then call commit-project once for the project; for VM-only projects, commit-project automatically falls back to the VM commit endpoint used by the UI when needed.", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-standalone-vm", "Queue deletion of a standalone VM from a project. You can batch this with other VM changes and then call commit-project once for the project to apply the deletion.", func(ctx context.Context, args DeleteStandaloneVMArgs) (*mcp_golang.ToolResponse, error) {
		return deleteStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-standalone-vm-flavor", "Update standalone VM flavor. You can batch this with other VM changes and then call commit-project once for the project.", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateStandaloneVMFlavor(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "manage-standalone-vm-ip", "Manage standalone VM IP assignment. You can batch this with other VM changes and then call commit-project once for the project.", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return manageStandaloneVMIP(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "reset-standalone-vm-status", "Reset standalone VM status", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return resetStandaloneVMStatus(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-console", "Get standalone VM console information", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMConsole(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "download-standalone-vm-rdp", "Download standalone VM RDP content", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return downloadStandaloneVMRDP(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "reboot-standalone-vm", "Reboot a standalone VM", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return rebootStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "shelve-standalone-vm", "Shelve a standalone VM", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return shelveStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "start-standalone-vm", "Start a standalone VM", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return startStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-status", "Get standalone VM status", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMStatus(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "stop-standalone-vm", "Stop a standalone VM", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return stopStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "unshelve-standalone-vm", "Unshelve a standalone VM", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return unshelveStandaloneVM(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-windows-password", "Get standalone VM Windows password", func(ctx context.Context, args StandaloneWindowsPasswordArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMWindowsPassword(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-standalone-vm-disk", "Create a standalone VM disk. You can batch this with other VM changes and then call commit-project once for the project.", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneVMDisk(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "resize-standalone-vm-disk", "Resize a standalone VM disk. You can batch this with other VM changes and then call commit-project once for the project.", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return resizeStandaloneVMDisk(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-standalone-profiles", "List standalone profiles", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listStandaloneProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-standalone-profile", "Create a standalone profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-standalone-profile", "Update a standalone profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateStandaloneProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-standalone-profile", "Delete a standalone profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteStandaloneProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-standalone-profiles", "List standalone profile dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownStandaloneProfiles(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-standalone-profile", "Lock or unlock a standalone profile", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockStandaloneProfile(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-standalone-profile-sg", "Create a security group rule for a standalone profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneProfileSecurityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-standalone-profile-sg", "Update a security group rule for a standalone profile", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateStandaloneProfileSecurityGroup(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-standalone-profile-sg", "Delete a security group rule from a standalone profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteStandaloneProfileSecurityGroup(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "create-cloud-credential", "Create a cloud credential. Set cloudType to one of aws, azure, openstack; payload matches that cloud's create command. For GCP use create-google-cloud-credential.", func(ctx context.Context, args CloudCredentialWriteArgs) (*mcp_golang.ToolResponse, error) {
		return createCloudCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-cloud-credential", "Update a cloud credential. Set cloudType to one of aws, azure, openstack; payload matches that cloud's update command. GCP credentials have no update endpoint.", func(ctx context.Context, args CloudCredentialWriteArgs) (*mcp_golang.ToolResponse, error) {
		return updateCloudCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-cloud-credential", "Delete a cloud credential", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteCloudCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "make-cloud-credential-default", "Make a cloud credential default", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return makeCloudCredentialDefault(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-cloud-credential", "Lock or unlock a cloud credential", func(ctx context.Context, args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockCloudCredential(clientFromContext(ctx), args)
	})

	mustRegisterScopedTool(server, "create-google-cloud-credential", "Create a Google (GCP) cloud credential from a service-account JSON key file. Provide configFilePath (path to the GCP service-account key file) and a name; optionally region, billingAccountId, folderId, importProject, azCount, and organizationId. Discover valid values with list-google-regions, list-google-zones, and list-google-billing-accounts.", func(ctx context.Context, args CreateGoogleCloudCredentialArgs) (*mcp_golang.ToolResponse, error) {
		return createGoogleCloudCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-google-regions", "List available GCP regions for a service-account JSON key file (configFilePath)", func(ctx context.Context, args GoogleConfigArgs) (*mcp_golang.ToolResponse, error) {
		return listGoogleRegions(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-google-zones", "List available GCP zones for a region using a service-account JSON key file (configFilePath, region)", func(ctx context.Context, args GoogleZoneListArgs) (*mcp_golang.ToolResponse, error) {
		return listGoogleZones(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-google-billing-accounts", "List available GCP billing accounts for a service-account JSON key file (configFilePath)", func(ctx context.Context, args GoogleConfigArgs) (*mcp_golang.ToolResponse, error) {
		return listGoogleBillingAccounts(clientFromContext(ctx), args)
	})

	// Access profile sub-resources: DNS servers, NTP servers, SSH users, trusted registries.
	mustRegisterScopedTool(server, "list-dns-servers", "List DNS servers configured on an access profile (accessProfileId)", func(ctx context.Context, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
		return listDNSServers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-dns-server", "Add a DNS server to an access profile (payload: CreateDnsServerCommand with address and accessProfileId)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createDNSServer(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "edit-dns-server", "Edit a DNS server address on an access profile (id + payload: DnsNtpAddressEditDto)", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return editDNSServer(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-dns-server", "Delete a DNS server from an access profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteDNSServer(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-ntp-servers", "List NTP servers configured on an access profile (accessProfileId)", func(ctx context.Context, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
		return listNTPServers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-ntp-server", "Add an NTP server to an access profile (payload: CreateNtpServerCommand with address and accessProfileId)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createNTPServer(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "edit-ntp-server", "Edit an NTP server address on an access profile (id + payload: DnsNtpAddressEditDto)", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return editNTPServer(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-ntp-server", "Delete an NTP server from an access profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteNTPServer(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-ssh-users", "List SSH users configured on an access profile (accessProfileId)", func(ctx context.Context, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
		return listSSHUsers(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-ssh-user", "Add an SSH user to an access profile (payload: CreateSshUserCommand with name, sshPublicKey, accessProfileId)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createSSHUser(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "edit-ssh-user", "Edit an SSH user on an access profile (payload: EditSshUserCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return editSSHUser(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-ssh-user", "Delete an SSH user from an access profile (payload: DeleteSshUserCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteSSHUser(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "list-trusted-registries", "List trusted container registries configured on an access profile (accessProfileId)", func(ctx context.Context, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
		return listTrustedRegistries(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-trusted-registry", "Add a trusted registry to an access profile (payload: CreateTrustedRegistriesCommand with registry and accessProfileId)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createTrustedRegistry(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "edit-trusted-registry", "Edit a trusted registry on an access profile (id + payload: TrustedRegistryEditDto)", func(ctx context.Context, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return editTrustedRegistry(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-trusted-registry", "Delete a trusted registry from an access profile", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteTrustedRegistry(clientFromContext(ctx), args)
	})

	// DNS provider credentials (organization-level).
	mustRegisterScopedTool(server, "list-dns-credentials", "List DNS provider credentials with optional filtering", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listDNSCredentials(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-dns-credentials", "List DNS credential dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownDNSCredentials(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-dns-credential", "Create a DNS provider credential (payload: DnsCredentialCreateCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createDNSCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-dns-credential", "Update a DNS provider credential (payload: DnsCredentialUpdateCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateDNSCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-dns-credential", "Delete a DNS provider credential", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteDNSCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "make-dns-credential-default", "Make a DNS provider credential default (payload: DnsCredentialMakeDefaultCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return makeDNSCredentialDefault(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-dns-credential", "Lock or unlock a DNS provider credential (payload: DnsCredentialLockCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return lockDNSCredential(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "attach-dns-credential-to-project", "Attach a DNS credential to a project (payload: AttachDetachDnsCredentialCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return attachDNSCredentialToProject(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "detach-dns-credential-from-project", "Detach a DNS credential from a project (payload: AttachDetachDnsCredentialCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return detachDNSCredentialFromProject(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "validate-dns-credential", "Validate a DNS provider credential (payload: ValidateDnsCertCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return validateDNSCredential(clientFromContext(ctx), args)
	})

	// DNS certificate service (project-scoped ACME/DNS-01 certificates).
	mustRegisterScopedTool(server, "get-dns-cert-status", "Get the DNS certificate status for a project", func(ctx context.Context, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return getDNSCertStatus(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "enable-dns-cert", "Enable the DNS certificate service for a project (payload: EnableDnsCertCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return enableDNSCert(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "disable-dns-cert", "Disable the DNS certificate service for a project (payload: DisableDnsCertCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return disableDNSCert(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "sync-dns-cert", "Sync the DNS certificate for a project (payload: DnsCertSyncCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return syncDNSCert(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "validate-dns-cert", "Validate DNS certificate settings (payload: ValidateDnsCertCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return validateDNSCert(clientFromContext(ctx), args)
	})

	// Certificate authorities (custom CA / certificate profiles).
	mustRegisterScopedTool(server, "list-certificate-authorities", "List certificate profiles (custom certificate authorities) with optional filtering", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listCertificateAuthorities(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "dropdown-certificate-authorities", "List certificate profile (certificate authority) dropdown entries", func(ctx context.Context, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownCertificateAuthorities(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "create-certificate-authority", "Create a certificate profile / custom certificate authority (payload: CertificateProfileCreateCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createCertificateAuthority(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "update-certificate-authority", "Update a certificate profile / custom certificate authority (payload: CertificateProfileUpdateCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateCertificateAuthority(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "delete-certificate-authority", "Delete a certificate profile / custom certificate authority", func(ctx context.Context, args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteCertificateAuthority(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "make-certificate-authority-default", "Make a certificate profile / custom certificate authority default (payload: CertificateProfileMakeDefaultCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return makeCertificateAuthorityDefault(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "lock-certificate-authority", "Lock or unlock a certificate profile / custom certificate authority (payload: CertificateProfileLockCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return lockCertificateAuthority(clientFromContext(ctx), args)
	})
	mustRegisterScopedTool(server, "validate-certificate-authority", "Validate a certificate profile / custom certificate authority (payload: CertificateProfileValidateCommand)", func(ctx context.Context, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return validateCertificateAuthority(clientFromContext(ctx), args)
	})

	logger.Println("All tools registered successfully. Starting MCP server...")
	if err := server.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		logger.Fatalf("Server error: %v", err)
	}

	if httpTransport != nil {
		// HTTP mode: ListenAndServe blocks until the server is shut down
		if err := httpTransport.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			logger.Fatalf("HTTP server error: %v", err)
		}
	} else {
		// stdio mode: block forever; the stdio transport drives the event loop
		done := make(chan struct{})
		<-done
	}
}
