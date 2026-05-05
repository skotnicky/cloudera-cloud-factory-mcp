package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
	logFilePath  = "/tmp/cloudera_cloud_factory_mcp_server.log"
	taikunClient *taikungoclient.Client
)

const (
	defaultAPIHost = "api-latest.osc1.sjc.cloudera.com"
	mcpServerName  = "cloudera-cloud-factory-mcp"
)

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
	WaitForCreation     *bool  `json:"waitForCreation,omitempty" jsonschema:"description=Wait for project readiness after commit (default: true)"`
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
	Limit   int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset  int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	Search  string `json:"search,omitempty" jsonschema:"description=Search term to filter results (optional)"`
	IsAdmin bool   `json:"isAdmin,omitempty" jsonschema:"description=Whether to list as admin (optional)"`
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

func initLogger() {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
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
		APIHost:    strings.TrimSpace(getenv("TAIKUN_API_HOST")),
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

func refreshTaikunClient() *mcp_golang.ToolResponse {
	taikunClient = createTaikunClient()
	robotCtx := refreshRobotUserContext()
	successResp := newRefreshTaikunClientResponse(robotCtx)
	return createJSONResponse(successResp)
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

	initLogger()
	logger.Printf("Starting Cloudera Cloud Factory MCP server v%s", version)
	if err := initMCPLockFromConfig(os.Getenv, os.Args[1:]); err != nil {
		logger.Fatalf("Failed to initialize MCP lock: %v", err)
	}

	server := mcp_golang.NewServer(
		stdio.NewStdioServerTransport(),
		mcp_golang.WithName(mcpServerName),
		mcp_golang.WithVersion(version),
	)
	logger.Println("MCP server created")

	// Initialize the Cloudera Cloud Factory client once
	taikunClient = createTaikunClient()
	refreshRobotUserContext()
	logger.Println("Cloudera Cloud Factory client initialized")

	logger.Println("Starting tool registration...")

	// --- MCP Tool Registrations ---

	err := registerScopedTool(server, "server-version", "Show MCP server version and build metadata", func(args ServerVersionArgs) (*mcp_golang.ToolResponse, error) {
		return serverVersion(), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register server-version tool: %v", err)
	}
	logger.Println("Registered server-version tool")

	err = registerScopedTool(server, "refresh-taikun-client", "Refresh the Cloudera Cloud Factory API client using current environment credentials", func(args RefreshTaikunClientArgs) (*mcp_golang.ToolResponse, error) {
		return refreshTaikunClient(), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register refresh-taikun-client tool: %v", err)
	}
	logger.Println("Registered refresh-taikun-client tool")

	err = registerScopedTool(server, "robot-user-capabilities", "Show the current Robot User identity, scopes, and MCP tool access", func(args RobotUserCapabilitiesArgs) (*mcp_golang.ToolResponse, error) {
		return getRobotUserCapabilities(), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register robot-user-capabilities tool: %v", err)
	}
	logger.Println("Registered robot-user-capabilities tool")

	err = registerScopedTool(server, "mcp-lock", "Set runtime MCP org/project scope lock allowlists", func(args MCPLockArgs) (*mcp_golang.ToolResponse, error) {
		return mcpLock(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register mcp-lock tool: %v", err)
	}
	logger.Println("Registered mcp-lock tool")

	err = registerScopedTool(server, "mcp-lock-status", "Show current MCP lock configuration and effective scope", func(args MCPLockStatusArgs) (*mcp_golang.ToolResponse, error) {
		return mcpLockStatus(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register mcp-lock-status tool: %v", err)
	}
	logger.Println("Registered mcp-lock-status tool")

	err = registerScopedTool(server, "mcp-lock-clear", "Clear runtime MCP lock and fall back to environment lock", func(args MCPLockClearArgs) (*mcp_golang.ToolResponse, error) {
		return mcpLockClear(args)
	})
	if err != nil {
		logger.Fatalf("Failed to register mcp-lock-clear tool: %v", err)
	}
	logger.Println("Registered mcp-lock-clear tool")

	err = registerScopedTool(server, "create-virtual-cluster", "Create a new virtual cluster (a project in Cloudera Cloud Factory) with optional wait for completion", func(args CreateVirtualClusterArgs) (*mcp_golang.ToolResponse, error) {
		return createVirtualCluster(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-virtual-cluster tool: %v", err)
	}
	logger.Println("Registered create-virtual-cluster tool")

	err = registerScopedTool(server, "delete-virtual-cluster", "Delete a virtual cluster (a project in Cloudera Cloud Factory)", func(args DeleteVirtualClusterArgs) (*mcp_golang.ToolResponse, error) {
		return deleteVirtualCluster(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-virtual-cluster tool: %v", err)
	}
	logger.Println("Registered delete-virtual-cluster tool")

	err = registerScopedTool(server, "list-virtual-clusters", "List virtual clusters in a parent project (projects in Cloudera Cloud Factory)", func(args ListVirtualClustersArgs) (*mcp_golang.ToolResponse, error) {
		return listVirtualClusters(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-virtual-clusters tool: %v", err)
	}
	logger.Println("Registered list-virtual-clusters tool")

	err = registerScopedTool(server, "catalog-create", "Create a new catalog", func(args CreateCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return createCatalog(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-create tool: %v", err)
	}
	logger.Println("Registered catalog-create tool")

	err = registerScopedTool(server, "catalog-list", "List catalogs with optional filtering", func(args ListCatalogsArgs) (*mcp_golang.ToolResponse, error) {
		return listCatalogs(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-list tool: %v", err)
	}
	logger.Println("Registered catalog-list tool")

	err = registerScopedTool(server, "catalog-delete", "Delete a catalog", func(args DeleteCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return deleteCatalog(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-delete tool: %v", err)
	}
	logger.Println("Registered catalog-delete tool")

	err = registerScopedTool(server, "bind-projects-to-catalog", "Bind one or more projects to a catalog so they can install apps from it", func(args BindProjectsToCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return bindProjectsToCatalog(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register bind-projects-to-catalog tool: %v", err)
	}
	logger.Println("Registered bind-projects-to-catalog tool")

	err = registerScopedTool(server, "unbind-projects-from-catalog", "Unbind projects from a catalog", func(args UnbindProjectsFromCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return unbindProjectsFromCatalog(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register unbind-projects-from-catalog tool: %v", err)
	}
	logger.Println("Registered unbind-projects-from-catalog tool")

	err = registerScopedTool(server, "available-apps-list", "List available apps from the package repository", func(args ListAvailableAppsArgs) (*mcp_golang.ToolResponse, error) {
		return listAvailableApps(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register available-apps-list tool: %v", err)
	}
	logger.Println("Registered available-apps-list tool")

	err = registerScopedTool(server, "list-repositories", "List repositories with optional filtering", func(args ListRepositoriesArgs) (*mcp_golang.ToolResponse, error) {
		return listRepositories(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-repositories tool: %v", err)
	}
	logger.Println("Registered list-repositories tool")

	err = registerScopedTool(server, "import-repository", "Import a repository from a URL with optional credentials", func(args ImportRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return importRepository(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register import-repository tool: %v", err)
	}
	logger.Println("Registered import-repository tool")

	err = registerScopedTool(server, "bind-repository", "Bind a repository to an organization so its apps become available", func(args BindRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return bindRepository(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register bind-repository tool: %v", err)
	}
	logger.Println("Registered bind-repository tool")

	err = registerScopedTool(server, "unbind-repository", "Unbind one or more repository IDs from an organization", func(args UnbindRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return unbindRepository(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register unbind-repository tool: %v", err)
	}
	logger.Println("Registered unbind-repository tool")

	err = registerScopedTool(server, "delete-repository", "Delete an imported repository", func(args DeleteRepositoryArgs) (*mcp_golang.ToolResponse, error) {
		return deleteRepository(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-repository tool: %v", err)
	}
	logger.Println("Registered delete-repository tool")

	err = registerScopedTool(server, "update-repository-password", "Update stored credentials for a private repository", func(args UpdateRepositoryPasswordArgs) (*mcp_golang.ToolResponse, error) {
		return updateRepositoryPassword(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register update-repository-password tool: %v", err)
	}
	logger.Println("Registered update-repository-password tool")

	err = registerScopedTool(server, "catalog-app-add", "Add an application to a catalog with optional default parameters", func(args AddAppToCatalogWithParametersArgs) (*mcp_golang.ToolResponse, error) {
		return addAppToCatalogWithParameters(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-add tool: %v", err)
	}
	logger.Println("Registered catalog-app-add tool")

	err = registerScopedTool(server, "catalog-app-remove", "Remove an application from a catalog by package name and optional repository", func(args RemoveAppFromCatalogArgs) (*mcp_golang.ToolResponse, error) {
		return removeAppFromCatalog(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-remove tool: %v", err)
	}
	logger.Println("Registered catalog-app-remove tool")

	err = registerScopedTool(server, "catalog-apps-list", "List applications in a specific catalog or all catalogs", func(args ListCatalogAppsArgs) (*mcp_golang.ToolResponse, error) {
		return listCatalogApps(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-apps-list tool: %v", err)
	}
	logger.Println("Registered catalog-apps-list tool")

	err = registerScopedTool(server, "catalog-app-params", "Get available and added parameters for a catalog application", func(args GetCatalogAppParamsArgs) (*mcp_golang.ToolResponse, error) {
		return getCatalogAppParameters(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-params tool: %v", err)
	}
	logger.Println("Registered catalog-app-params tool")

	err = registerScopedTool(server, "catalog-app-defaults-set", "Update default parameters for a catalog application (merges with existing defaults by default)", func(args SetCatalogAppDefaultParamsArgs) (*mcp_golang.ToolResponse, error) {
		return updateCatalogAppParameters(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register catalog-app-defaults-set tool: %v", err)
	}
	logger.Println("Registered catalog-app-defaults-set tool")

	err = registerScopedTool(server, "app-install", "Install a new application instance with optional defaults and overrides. If timeout is omitted, the install request defaults to 10 minutes; TTL defaults to 10 minutes; larger applications may need a higher timeout.", func(args InstallAppArgs) (*mcp_golang.ToolResponse, error) {
		return installApp(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register app-install tool: %v", err)
	}
	logger.Println("Registered app-install tool")

	err = registerScopedTool(server, "list-apps", "List application instances in a project", func(args ListAppsArgs) (*mcp_golang.ToolResponse, error) {
		return listApps(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-apps tool: %v", err)
	}
	logger.Println("Registered list-apps tool")

	err = registerScopedTool(server, "get-app", "Get detailed application instance information", func(args GetAppArgs) (*mcp_golang.ToolResponse, error) {
		return getApp(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-app tool: %v", err)
	}
	logger.Println("Registered get-app tool")

	err = registerScopedTool(server, "update-app-autosync", "Update application autosync configuration", func(args UpdateAppAutoSyncArgs) (*mcp_golang.ToolResponse, error) {
		return updateAppAutoSync(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register update-app-autosync tool: %v", err)
	}
	logger.Println("Registered update-app-autosync tool")

	err = registerScopedTool(server, "update-sync-app", "Update application values and sync", func(args UpdateSyncAppArgs) (*mcp_golang.ToolResponse, error) {
		return updateSyncApp(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register update-sync-app tool: %v", err)
	}
	logger.Println("Registered update-sync-app tool")

	err = registerScopedTool(server, "uninstall-app", "Uninstall an application instance", func(args UninstallAppArgs) (*mcp_golang.ToolResponse, error) {
		return uninstallApp(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register uninstall-app tool: %v", err)
	}
	logger.Println("Registered uninstall-app tool")

	err = registerScopedTool(server, "wait-for-app", "Wait for an application instance to be ready", func(args WaitForAppArgs) (*mcp_golang.ToolResponse, error) {
		return waitForApp(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register wait-for-app tool: %v", err)
	}
	logger.Println("Registered wait-for-app tool")

	err = registerScopedTool(server, "list-projects", "List projects with optional virtual cluster filtering", func(args ListProjectsArgs) (*mcp_golang.ToolResponse, error) {
		return listProjects(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-projects tool: %v", err)
	}
	logger.Println("Registered list-projects tool")

	err = registerScopedTool(server, "create-project", "Create a project in Cloudera Cloud Factory", func(args CreateProjectArgs) (*mcp_golang.ToolResponse, error) {
		return createProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-project tool: %v", err)
	}
	logger.Println("Registered create-project tool")

	err = registerScopedTool(server, "create-cluster", "Create a Kubernetes cluster end-to-end (project, nodes, commit, optional wait) using profile-aware defaults and cloud-credential flavor discovery", func(args CreateClusterArgs) (*mcp_golang.ToolResponse, error) {
		return createCluster(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-cluster tool: %v", err)
	}
	logger.Println("Registered create-cluster tool")

	err = registerScopedTool(server, "delete-project", "Delete a project in Cloudera Cloud Factory. To confirm removal, call wait-for-project with waitDeleted true; if the project was empty, use a short timeout (about 10 to 30 seconds) because purge is usually fast.", func(args DeleteProjectArgs) (*mcp_golang.ToolResponse, error) {
		return deleteProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-project tool: %v", err)
	}
	logger.Println("Registered delete-project tool")

	err = registerScopedTool(server, "wait-for-project", "Wait for a project to be ready and healthy, or with waitDeleted for completion of project deletion. After delete-project on an empty project, pass a short timeout (e.g. 10 to 30 seconds) with waitDeleted true; projects that had servers or VMs typically need longer.", func(args WaitForProjectArgs) (*mcp_golang.ToolResponse, error) {
		return waitForProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register wait-for-project tool: %v", err)
	}
	logger.Println("Registered wait-for-project tool")

	err = registerScopedTool(server, "deploy-kubernetes-resources", "Deploy Kubernetes resources via YAML in a project", func(args DeployKubernetesResourcesArgs) (*mcp_golang.ToolResponse, error) {
		return deployKubernetesResources(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register deploy-kubernetes-resources tool: %v", err)
	}
	logger.Println("Registered deploy-kubernetes-resources tool")

	err = registerScopedTool(server, "create-kubeconfig", "Create a new kubeconfig for a project", func(args CreateKubeConfigArgs) (*mcp_golang.ToolResponse, error) {
		return createKubeConfig(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register create-kubeconfig tool: %v", err)
	}
	logger.Println("Registered create-kubeconfig tool")

	err = registerScopedTool(server, "get-kubeconfig", "Retrieve the kubeconfig content for a project (optionally save as YAML)", func(args GetKubeConfigArgs) (*mcp_golang.ToolResponse, error) {
		return getKubeConfig(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-kubeconfig tool: %v", err)
	}
	logger.Println("Registered get-kubeconfig tool")

	err = registerScopedTool(server, "list-kubeconfig-roles", "List available roles for kubeconfigs", func(args ListKubeConfigRolesArgs) (*mcp_golang.ToolResponse, error) {
		return listKubeConfigRoles(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-kubeconfig-roles tool: %v", err)
	}
	logger.Println("Registered list-kubeconfig-roles tool")

	err = registerScopedTool(server, "list-kubernetes-resource-kinds", "Show the supported Kubernetes resource kinds for list, describe, and delete operations. Kind matching is case-insensitive.", func(args KubernetesResourceKindsArgs) (*mcp_golang.ToolResponse, error) {
		return listKubernetesResourceKinds(), nil
	})
	if err != nil {
		logger.Fatalf("Failed to register list-kubernetes-resource-kinds tool: %v", err)
	}
	logger.Println("Registered list-kubernetes-resource-kinds tool")

	err = registerScopedTool(server, "list-kubernetes-resources", "List specialized Kubernetes resources in a project. Kind matching is case-insensitive; call list-kubernetes-resource-kinds to inspect supported listKinds and unavailableListKinds.", func(args ListKubernetesResourcesArgs) (*mcp_golang.ToolResponse, error) {
		return listKubernetesResources(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-kubernetes-resources tool: %v", err)
	}
	logger.Println("Registered list-kubernetes-resources tool")

	err = registerScopedTool(server, "describe-kubernetes-resource", "Describe a specialized Kubernetes resource in a project. Kind matching is case-insensitive; call list-kubernetes-resource-kinds to inspect supported operationKinds.", func(args DescribeKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
		return describeKubernetesResource(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register describe-kubernetes-resource tool: %v", err)
	}
	logger.Println("Registered describe-kubernetes-resource tool")

	err = registerScopedTool(server, "delete-kubernetes-resource", "Delete a Kubernetes resource. Kind matching is case-insensitive; call list-kubernetes-resource-kinds to inspect supported operationKinds.", func(args DeleteKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
		return deleteKubernetesResource(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-kubernetes-resource tool: %v", err)
	}
	logger.Println("Registered delete-kubernetes-resource tool")

	err = registerScopedTool(server, "patch-kubernetes-resource", "Patch a Kubernetes resource using YAML", func(args PatchKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
		return patchKubernetesResource(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register patch-kubernetes-resource tool: %v", err)
	}
	logger.Println("Registered patch-kubernetes-resource tool")

	err = registerScopedTool(server, "list-cloud-credentials", "List cloud credentials", func(args ListCloudCredentialsArgs) (*mcp_golang.ToolResponse, error) {
		return listCloudCredentials(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-cloud-credentials tool: %v", err)
	}
	logger.Println("Registered list-cloud-credentials tool")

	err = registerScopedTool(server, "bind-flavors-to-project", "Bind flavors to a project", func(args BindFlavorsArgs) (*mcp_golang.ToolResponse, error) {
		return bindFlavorsToProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register bind-flavors-to-project tool: %v", err)
	}
	logger.Println("Registered bind-flavors-to-project tool")

	err = registerScopedTool(server, "add-server-to-project", "Add a Kubernetes server to a project. Recommendation: Bastion can use 2 CPUs / 2GB RAM; Kubemaster must be at least 4 CPUs / 4GB RAM; if monitoring is enabled, include at least one Kubeworker with 4 CPUs / 4GB RAM before commit-project.", func(args AddServerArgs) (*mcp_golang.ToolResponse, error) {
		return addServerToProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register add-server-to-project tool: %v", err)
	}
	logger.Println("Registered add-server-to-project tool")

	err = registerScopedTool(server, "commit-project", "Commit and provision pending project infrastructure in the cloud. For Kubernetes changes, commit-project validates that all Kubemaster nodes are at least 4 CPUs / 4GB RAM and requires at least one Kubeworker at 4 CPUs / 4GB RAM when monitoring is enabled. For VM-only changes, this tool automatically falls back to the VM commit endpoint used by the UI when the cluster-style commit path is not applicable. Do not call while project status is Updating; full initial Kubernetes deploy often takes 10-30 minutes.", func(args CommitProjectArgs) (*mcp_golang.ToolResponse, error) {
		return commitProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register commit-project tool: %v", err)
	}
	logger.Println("Registered commit-project tool")

	err = registerScopedTool(server, "get-project-details", "Get detailed status of a project", func(args GetProjectDetailsArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectDetails(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register get-project-details tool: %v", err)
	}
	logger.Println("Registered get-project-details tool")

	err = registerScopedTool(server, "list-flavors", "List available flavors for a cloud credential", func(args ListFlavorsArgs) (*mcp_golang.ToolResponse, error) {
		return listFlavors(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-flavors tool: %v", err)
	}
	logger.Println("Registered list-flavors tool")

	err = registerScopedTool(server, "list-servers", "List servers in a project", func(args ListServersArgs) (*mcp_golang.ToolResponse, error) {
		return listServers(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register list-servers tool: %v", err)
	}
	logger.Println("Registered list-servers tool")

	err = registerScopedTool(server, "delete-servers-from-project", "Delete servers from a project", func(args DeleteServersArgs) (*mcp_golang.ToolResponse, error) {
		return deleteServersFromProject(taikunClient, args)
	})
	if err != nil {
		logger.Fatalf("Failed to register delete-servers-from-project tool: %v", err)
	}
	logger.Println("Registered delete-servers-from-project tool")

	mustRegisterScopedTool(server, "list-domains", "List domains", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listDomains(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-domain", "Create a domain", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createDomain(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-domain-details", "Get domain details", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return getDomainDetails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-domain", "Update a domain", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateDomain(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-domain", "Delete a domain", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteDomain(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-organizations", "List organizations", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listOrganizations(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-organization", "Create an organization", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createOrganization(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-organization-details", "Get organization details", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return getOrganizationDetails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-organization", "Update an organization", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateOrganization(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-organization", "Delete an organization", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteOrganization(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-identity-groups", "List identity groups within a domain", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listIdentityGroups(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-identity-group", "Create an identity group", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-identity-group-details", "Get identity group details within a domain", func(args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return getIdentityGroupDetails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-identity-group-organizations", "List organizations assigned to an identity group", func(args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listIdentityGroupOrganizations(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-identity-group-users", "List users assigned to an identity group", func(args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listIdentityGroupUsers(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-available-group-organizations", "List organizations available to add to an identity group", func(args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listAvailableIdentityGroupOrganizations(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-available-identity-group-users", "List users available to add to an identity group", func(args DomainScopedIDArgs) (*mcp_golang.ToolResponse, error) {
		return listAvailableIdentityGroupUsers(taikunClient, args)
	})
	mustRegisterScopedTool(server, "add-organizations-to-identity-group", "Add organizations to an identity group", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return addOrganizationsToIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-identity-group-organization", "Update an organization's membership settings in an identity group", func(args GroupOrganizationPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateIdentityGroupOrganization(taikunClient, args)
	})
	mustRegisterScopedTool(server, "remove-organizations-from-group", "Remove organizations from an identity group", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return removeOrganizationsFromIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "add-users-to-identity-group", "Add users to an identity group", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return addUsersToIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "remove-users-from-identity-group", "Remove users from an identity group", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return removeUsersFromIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-identity-group", "Update an identity group", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-identity-group", "Delete an identity group", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteIdentityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-users", "List users within a domain", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listUsers(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-user", "Create a user", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createUser(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-user-details", "Get user details within a domain", func(args DomainScopedStringIDArgs) (*mcp_golang.ToolResponse, error) {
		return getUserDetails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-user", "Update a user", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateUser(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-user", "Delete a user", func(args StringIDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteUser(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-access-profiles", "List access profiles", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listAccessProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-access-profile", "Create an access profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAccessProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-access-profile", "Update an access profile", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAccessProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-access-profile", "Delete an access profile", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAccessProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-access-profiles", "List access profile dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownAccessProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-access-profile", "Lock or unlock an access profile", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockAccessProfile(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-ai-credentials", "List AI credentials", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listAICredentials(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-ai-credential", "Create an AI credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAICredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-ai-credential", "Delete an AI credential", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAICredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-ai-credentials", "List AI credential dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownAICredentials(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-kubernetes-profiles", "List Kubernetes profiles", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listKubernetesProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-kubernetes-profile", "Create a Kubernetes profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createKubernetesProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-kubernetes-profile", "Delete a Kubernetes profile", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteKubernetesProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-kubernetes-profiles", "List Kubernetes profile dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownKubernetesProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-kubernetes-profile", "Lock or unlock a Kubernetes profile", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockKubernetesProfile(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-opa-profiles", "List OPA profiles", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listOPAProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-opa-profile", "Create an OPA profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createOPAProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-opa-profile", "Update an OPA profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateOPAProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-opa-profile", "Delete an OPA profile", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteOPAProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-opa-profiles", "List OPA profile dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownOPAProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-opa-profile", "Lock or unlock an OPA profile", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockOPAProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "sync-opa-profile", "Sync an OPA profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return syncOPAProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "make-opa-profile-default", "Make an OPA profile default", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return makeOPAProfileDefault(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-alerting-profiles", "List alerting profiles", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listAlertingProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-alerting-profile", "Create an alerting profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAlertingProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-alerting-profile", "Update an alerting profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAlertingProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-alerting-profile", "Delete an alerting profile", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAlertingProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-alerting-profiles", "List alerting profile dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownAlertingProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-alerting-profile", "Lock or unlock an alerting profile", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockAlertingProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "attach-alerting-profile", "Attach an alerting profile to a project", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return attachAlertingProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "detach-alerting-profile", "Detach an alerting profile from a project", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return detachAlertingProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "assign-alerting-emails", "Assign alerting emails to a profile", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return assignAlertingEmails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "assign-alerting-webhooks", "Assign alerting webhooks to a profile", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return assignAlertingWebhooks(taikunClient, args)
	})
	mustRegisterScopedTool(server, "verify-alerting-webhook", "Verify an alerting webhook", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return verifyAlertingWebhook(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-alerting-integrations", "List alerting integrations for a profile", func(args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return listAlertingIntegrations(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-alerting-integration", "Create an alerting integration", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAlertingIntegration(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-alerting-integration", "Update an alerting integration", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAlertingIntegration(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-alerting-integration", "Delete an alerting integration", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteAlertingIntegration(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-backup-credentials", "List backup credentials", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listBackupCredentials(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-backup-credential", "Create a backup credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createBackupCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-backup-credential", "Update a backup credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateBackupCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-backup-credential", "Delete a backup credential", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteBackupCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-backup-credentials", "List backup credential dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownBackupCredentials(taikunClient, args)
	})
	mustRegisterScopedTool(server, "make-backup-credential-default", "Make a backup credential default", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return makeBackupCredentialDefault(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-backup-credential", "Lock or unlock a backup credential", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockBackupCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-backup-policy", "Create a backup policy", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createBackupPolicy(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-backup-by-name", "Get backup details by project and name", func(args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return getBackupByName(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-project-backups", "List backups for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackups(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-project-restore-requests", "List restore requests for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectRestoreRequests(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-project-backup-schedules", "List backup schedules for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackupSchedules(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-project-backup-locations", "List backup storage locations for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackupStorageLocations(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-project-backup-delete-requests", "List backup delete requests for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectBackupDeleteRequests(taikunClient, args)
	})
	mustRegisterScopedTool(server, "describe-backup", "Describe a backup by project and name", func(args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return describeBackup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "describe-restore", "Describe a restore by project and name", func(args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return describeRestore(taikunClient, args)
	})
	mustRegisterScopedTool(server, "describe-schedule", "Describe a backup schedule by project and name", func(args ProjectNameArgs) (*mcp_golang.ToolResponse, error) {
		return describeSchedule(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-backup", "Delete a backup", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteBackup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-backup-storage-location", "Delete a backup storage location", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteBackupStorageLocation(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-restore", "Delete a restore request", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteRestore(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-schedule", "Delete a backup schedule", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return deleteSchedule(taikunClient, args)
	})
	mustRegisterScopedTool(server, "import-backup-storage-location", "Import a backup storage location", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return importBackupStorageLocation(taikunClient, args)
	})
	mustRegisterScopedTool(server, "restore-backup", "Restore a backup", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return restoreBackup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-backup", "Enable backup for a project using a backup credential", func(args ProjectBackupCredentialArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectBackup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-backup", "Disable backup for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectBackup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-monitoring", "Enable monitoring for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectMonitoring(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-monitoring", "Disable monitoring for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectMonitoring(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-project-monitoring-alerts", "Read Prometheus-style monitoring alerts for a project. Monitoring must be enabled on the project first.", func(args ProjectMonitoringAlertsArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectMonitoringAlerts(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-project-alerts", "Read project detail alerts/messages for a project. Monitoring must be enabled on the project first.", func(args ProjectAlertsArgs) (*mcp_golang.ToolResponse, error) {
		return listProjectAlerts(taikunClient, args)
	})
	mustRegisterScopedTool(server, "query-project-loki-logs", "Query Loki logs for a project. Monitoring must be enabled and results can be large.", func(args QueryProjectLokiLogsArgs) (*mcp_golang.ToolResponse, error) {
		return queryProjectLokiLogs(taikunClient, args)
	})
	mustRegisterScopedTool(server, "export-project-loki-logs", "Export Loki logs for a project. Monitoring must be enabled; returns the API CSV export payload.", func(args ExportProjectLokiLogsArgs) (*mcp_golang.ToolResponse, error) {
		return exportProjectLokiLogs(taikunClient, args)
	})
	mustRegisterScopedTool(server, "query-project-prometheus-metrics", "Query Prometheus metrics for a project. Monitoring must be enabled and the result payload may be large.", func(args QueryProjectPrometheusMetricsArgs) (*mcp_golang.ToolResponse, error) {
		return queryProjectPrometheusMetrics(taikunClient, args)
	})
	mustRegisterScopedTool(server, "autocomplete-project-metrics", "Return Prometheus metric autocomplete suggestions for a project. Monitoring must be enabled.", func(args ProjectPrometheusMetricsAutocompleteArgs) (*mcp_golang.ToolResponse, error) {
		return autocompleteProjectPrometheusMetrics(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-ai-assistant", "Enable AI Assistant for a project using an AI credential", func(args ProjectAICredentialArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectAIAssistant(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-ai-assistant", "Disable AI Assistant for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectAIAssistant(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-policy", "Enable policy enforcement for a project using a policy profile", func(args ProjectPolicyProfileArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectPolicy(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-policy", "Disable policy enforcement for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectPolicy(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-full-spot", "Enable full spot support for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectFullSpot(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-full-spot", "Disable full spot support for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectFullSpot(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-spot-workers", "Enable spot workers for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectSpotWorkers(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-spot-workers", "Disable spot workers for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectSpotWorkers(taikunClient, args)
	})
	mustRegisterScopedTool(server, "enable-project-spot-vms", "Enable spot VMs for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return enableProjectSpotVMs(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-project-spot-vms", "Disable spot VMs for a project", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return disableProjectSpotVMs(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-project-service-status", "Get current project service settings and bindings", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return getProjectServiceStatus(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-images", "List images for a provider", func(args ImageListArgs) (*mcp_golang.ToolResponse, error) {
		return listImages(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-image-details", "Get image details", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return getImageDetails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "bind-images-to-project", "Bind images to a project. Primarily for standalone VM workflows; Kubernetes project deployment does not require image binding.", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return bindImagesToProject(taikunClient, args)
	})
	mustRegisterScopedTool(server, "unbind-images-from-project", "Unbind images from a project", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return unbindImagesFromProject(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-selected-project-images", "List selected images for a project", func(args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listSelectedProjectImages(taikunClient, args)
	})

	mustRegisterScopedTool(server, "enable-autoscaling", "Enable project autoscaling", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return enableAutoscaling(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-autoscaling", "Update project autoscaling", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAutoscaling(taikunClient, args)
	})
	mustRegisterScopedTool(server, "disable-autoscaling", "Disable project autoscaling", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return disableAutoscaling(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-autoscaling-status", "Get project autoscaling status", func(args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
		return getAutoscalingStatus(taikunClient, args)
	})

	mustRegisterScopedTool(server, "list-standalone-vms", "List standalone VMs in a project", func(args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listStandaloneVMs(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-details", "Get standalone VM details", func(args ProjectSearchListArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMDetails(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-standalone-vm", "Create a standalone VM (payload: CreateStandAloneVmCommand). The image field expects the provider image ID (for example an AWS AMI ID), not the display name. If payload omits volumeSize, the tool defaults it to 10 GiB; for Windows images, prefer 50 GiB. You can batch multiple VM changes and then call commit-project once for the project; for VM-only projects, commit-project automatically falls back to the VM commit endpoint used by the UI when needed.", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-standalone-vm", "Queue deletion of a standalone VM from a project. You can batch this with other VM changes and then call commit-project once for the project to apply the deletion.", func(args DeleteStandaloneVMArgs) (*mcp_golang.ToolResponse, error) {
		return deleteStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-standalone-vm-flavor", "Update standalone VM flavor. You can batch this with other VM changes and then call commit-project once for the project.", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateStandaloneVMFlavor(taikunClient, args)
	})
	mustRegisterScopedTool(server, "manage-standalone-vm-ip", "Manage standalone VM IP assignment. You can batch this with other VM changes and then call commit-project once for the project.", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return manageStandaloneVMIP(taikunClient, args)
	})
	mustRegisterScopedTool(server, "reset-standalone-vm-status", "Reset standalone VM status", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return resetStandaloneVMStatus(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-console", "Get standalone VM console information", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMConsole(taikunClient, args)
	})
	mustRegisterScopedTool(server, "download-standalone-vm-rdp", "Download standalone VM RDP content", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return downloadStandaloneVMRDP(taikunClient, args)
	})
	mustRegisterScopedTool(server, "reboot-standalone-vm", "Reboot a standalone VM", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return rebootStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "shelve-standalone-vm", "Shelve a standalone VM", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return shelveStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "start-standalone-vm", "Start a standalone VM", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return startStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-status", "Get standalone VM status", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMStatus(taikunClient, args)
	})
	mustRegisterScopedTool(server, "stop-standalone-vm", "Stop a standalone VM", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return stopStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "unshelve-standalone-vm", "Unshelve a standalone VM", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return unshelveStandaloneVM(taikunClient, args)
	})
	mustRegisterScopedTool(server, "get-standalone-vm-windows-password", "Get standalone VM Windows password", func(args StandaloneWindowsPasswordArgs) (*mcp_golang.ToolResponse, error) {
		return getStandaloneVMWindowsPassword(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-standalone-vm-disk", "Create a standalone VM disk. You can batch this with other VM changes and then call commit-project once for the project.", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneVMDisk(taikunClient, args)
	})
	mustRegisterScopedTool(server, "resize-standalone-vm-disk", "Resize a standalone VM disk. You can batch this with other VM changes and then call commit-project once for the project.", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return resizeStandaloneVMDisk(taikunClient, args)
	})
	mustRegisterScopedTool(server, "list-standalone-profiles", "List standalone profiles", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return listStandaloneProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-standalone-profile", "Create a standalone profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-standalone-profile", "Update a standalone profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateStandaloneProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-standalone-profile", "Delete a standalone profile", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteStandaloneProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "dropdown-standalone-profiles", "List standalone profile dropdown entries", func(args SearchListArgs) (*mcp_golang.ToolResponse, error) {
		return dropdownStandaloneProfiles(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-standalone-profile", "Lock or unlock a standalone profile", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockStandaloneProfile(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-standalone-profile-sg", "Create a security group rule for a standalone profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createStandaloneProfileSecurityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-standalone-profile-sg", "Update a security group rule for a standalone profile", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateStandaloneProfileSecurityGroup(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-standalone-profile-sg", "Delete a security group rule from a standalone profile", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteStandaloneProfileSecurityGroup(taikunClient, args)
	})

	mustRegisterScopedTool(server, "create-aws-cloud-credential", "Create an AWS cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAWSCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-aws-cloud-credential", "Update an AWS cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAWSCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-azure-cloud-credential", "Create an Azure cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createAzureCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-azure-cloud-credential", "Update an Azure cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateAzureCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-openstack-cloud-credential", "Create an OpenStack cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createOpenStackCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-openstack-cloud-credential", "Update an OpenStack cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateOpenStackCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-proxmox-cloud-credential", "Create a Proxmox cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createProxmoxCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-proxmox-cloud-credential", "Update a Proxmox cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateProxmoxCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-vsphere-cloud-credential", "Create a vSphere cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createVSphereCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-vsphere-cloud-credential", "Update a vSphere cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateVSphereCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "create-zadara-cloud-credential", "Create a Zadara cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return createZadaraCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-zadara-cloud-credential", "Update a Zadara cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateZadaraCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "update-generic-kubernetes-credential", "Update a generic Kubernetes cloud credential", func(args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
		return updateGenericKubernetesCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "delete-cloud-credential", "Delete a cloud credential", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return deleteCloudCredential(taikunClient, args)
	})
	mustRegisterScopedTool(server, "make-cloud-credential-default", "Make a cloud credential default", func(args IDArgs) (*mcp_golang.ToolResponse, error) {
		return makeCloudCredentialDefault(taikunClient, args)
	})
	mustRegisterScopedTool(server, "lock-cloud-credential", "Lock or unlock a cloud credential", func(args LockModeArgs) (*mcp_golang.ToolResponse, error) {
		return lockCloudCredential(taikunClient, args)
	})

	logger.Println("All tools registered successfully. Starting MCP server...")
	logger.Println("About to call server.Serve()...")
	err = server.Serve()
	logger.Printf("server.Serve() returned with error: %v", err)
	if err != nil {
		logger.Fatalf("Server error: %v", err)
	}

	done := make(chan struct{})
	<-done
}
