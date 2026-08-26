package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	yamlsig "sigs.k8s.io/yaml"
)

type PodSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	PodIP     string `json:"podIP"`
	StartTime string `json:"startTime,omitempty"`
	Restarts  int32  `json:"restarts"`
}

type DeploymentSummary struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Age           string `json:"age"`
}

type ServiceSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ClusterIP string `json:"clusterIP"`
	Age       string `json:"age"`
}

type NamespaceSummary struct {
	Name string `json:"name"`
}

type ConfigMapSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Age       string `json:"age"`
}

type SecretSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Age       string `json:"age"`
}

type IngressSummary struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Hosts     []string `json:"hosts"`
	Address   string   `json:"address"`
	Age       string   `json:"age"`
}

type CronJobSummary struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Schedule         string `json:"schedule"`
	Suspend          bool   `json:"suspend"`
	Active           int    `json:"active"`
	LastScheduleTime string `json:"lastScheduleTime,omitempty"`
	Age              string `json:"age"`
}

type DaemonSetSummary struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	DesiredScheduled int32  `json:"desiredScheduled"`
	CurrentScheduled int32  `json:"currentScheduled"`
	NumberReady      int32  `json:"numberReady"`
	NumberAvailable  int32  `json:"numberAvailable"`
	Age              string `json:"age"`
}

type JobSummary struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Completions string `json:"completions"`
	Succeeded   int32  `json:"succeeded"`
	Age         string `json:"age"`
}

type NodeSummary struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Roles   string `json:"roles"`
	Version string `json:"version"`
}

type PvcSummary struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	Volume       string `json:"volume"`
	Capacity     string `json:"capacity"`
	AccessModes  string `json:"accessModes"`
	StorageClass string `json:"storageClass"`
	Age          string `json:"age"`
}

type StorageClassSummary struct {
	Name          string `json:"name"`
	Provisioner   string `json:"provisioner"`
	ReclaimPolicy string `json:"reclaimPolicy"`
	Age           string `json:"age"`
}

type StatefulSetSummary struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Age           string `json:"age"`
}

type cursorPaginatedResponse[T any] struct {
	Data       []T     `json:"data"`
	Limit      int32   `json:"limit"`
	HasMore    bool    `json:"hasMore"`
	TotalCount int64   `json:"totalCount"`
	NextCursor *string `json:"nextCursor"`
}

type podListItem struct {
	State        string `json:"state"`
	Name         string `json:"name"`
	Ready        string `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	CreatedAt    string `json:"createdAt"`
	Namespace    string `json:"namespace"`
	Node         string `json:"node"`
	IP           string `json:"ip"`
}

type deploymentListItem struct {
	State     string   `json:"state"`
	Name      string   `json:"name"`
	Ready     string   `json:"ready"`
	CreatedAt string   `json:"createdAt"`
	Namespace string   `json:"namespace"`
	Images    []string `json:"images"`
}

type serviceListItem struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Type       string `json:"type"`
	ClusterIP  string `json:"clusterIp"`
	ExternalIP string `json:"externalIp"`
	CreatedAt  string `json:"createdAt"`
}

type nodeListItem struct {
	State   string `json:"state"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Version string `json:"version"`
	IP      string `json:"ip"`
}

type configMapListItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	CreatedAt string `json:"createdAt"`
}

type secretListItem struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

type ingressListItem struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Target       string `json:"target"`
	Default      string `json:"default"`
	IngressClass string `json:"ingressClass"`
	CreatedAt    string `json:"createdAt"`
}

type daemonSetListItem struct {
	Status    string `json:"status"`
	Name      string `json:"name"`
	Desired   int32  `json:"desired"`
	Current   int32  `json:"current"`
	Ready     int32  `json:"ready"`
	Available string `json:"available"`
	CreatedAt string `json:"createdAt"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
}

type pvcListItem struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Status       string `json:"status"`
	Volume       string `json:"volume"`
	Capacity     string `json:"capacity"`
	AccessModes  string `json:"accessModes"`
	StorageClass string `json:"storageClass"`
	CreatedAt    string `json:"createdAt"`
}

type statefulSetListItem struct {
	State     string   `json:"state"`
	Name      string   `json:"name"`
	Ready     string   `json:"ready"`
	CreatedAt string   `json:"createdAt"`
	Namespace string   `json:"namespace"`
	Images    []string `json:"images"`
}

func formatAge(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "Unknown"
	}
	duration := time.Since(timestamp.Time)
	duration = duration.Round(time.Second)

	if duration.Hours() > 24 {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}

	return duration.String()
}

func formatAgeFromString(createdAt string) string {
	if createdAt == "" {
		return "Unknown"
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, createdAt); err == nil {
			return formatAge(metav1.NewTime(parsed))
		}
	}
	return createdAt
}

func parseReadyCounts(ready string) (int32, int32) {
	parts := strings.Split(ready, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	readyCount, ok := parseInt32Strict(parts[0])
	if !ok {
		return 0, 0
	}
	totalCount, ok := parseInt32Strict(parts[1])
	if !ok {
		return 0, 0
	}
	return readyCount, totalCount
}

func parseInt32(value string) int32 {
	parsed, ok := parseInt32Strict(value)
	if !ok {
		return 0
	}
	return parsed
}

func parseInt32Strict(value string) (int32, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return 0, false
	}
	return int32(parsed), true
}

func isLikelyKubernetesYaml(payload string) bool {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "---") {
		return true
	}
	if strings.Contains(trimmed, "apiVersion:") || strings.Contains(trimmed, "kind:") {
		return true
	}
	return false
}

func tryDecodeBase64Yaml(payload string) (string, bool) {
	if strings.TrimSpace(payload) == "" {
		return "", false
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, payload)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", false
	}
	if !utf8.Valid(decoded) {
		return "", false
	}
	text := strings.TrimSpace(string(decoded))
	if !isLikelyKubernetesYaml(text) {
		return "", false
	}
	return text, true
}

func normalizeYamlInput(payload string) (string, error) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return "", errors.New("yaml input is empty")
	}
	if decoded, ok := tryDecodeBase64Yaml(trimmed); ok {
		return decoded, nil
	}
	return trimmed, nil
}

func normalizeYamlOutput(payload string) string {
	if decoded, ok := tryDecodeBase64Yaml(payload); ok {
		return strings.ReplaceAll(decoded, "\r\n", "\n")
	}
	return strings.ReplaceAll(payload, "\r\n", "\n")
}

func normalizeKubeconfigYaml(payload string) string {
	normalized := strings.ReplaceAll(payload, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		trimmedLeft := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmedLeft, "api-version:") {
			indent := line[:len(line)-len(trimmedLeft)]
			lines[i] = indent + "apiVersion:" + strings.TrimPrefix(trimmedLeft, "api-version:")
			break
		}
	}
	normalized = strings.Join(lines, "\n")
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return normalized
}

// trimKubernetesDescribeYAML removes high-noise, low-signal fields from a
// described Kubernetes object (metadata.managedFields and the
// kubectl.kubernetes.io/last-applied-configuration annotation) which routinely
// account for roughly half of a describe payload. If the input is not a single
// YAML object it is returned unchanged.
func trimKubernetesDescribeYAML(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	var obj map[string]interface{}
	if err := yamlsig.Unmarshal([]byte(trimmed), &obj); err != nil || len(obj) == 0 {
		return raw
	}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		delete(meta, "managedFields")
		if ann, ok := meta["annotations"].(map[string]interface{}); ok {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
			if len(ann) == 0 {
				delete(meta, "annotations")
			}
		}
	}
	out, err := yamlsig.Marshal(obj)
	if err != nil {
		return raw
	}
	return string(out)
}

type kubeConfigSummary struct {
	Clusters       []string `json:"clusters,omitempty"`
	ServerURLs     []string `json:"serverUrls,omitempty"`
	Contexts       []string `json:"contexts,omitempty"`
	CurrentContext string   `json:"currentContext,omitempty"`
	Users          []string `json:"users,omitempty"`
}

// summarizeKubeconfig extracts non-secret metadata from a kubeconfig so the tool
// can describe it without echoing CA/client certificates into the agent context.
func summarizeKubeconfig(content string) kubeConfigSummary {
	var summary kubeConfigSummary
	var parsed map[string]interface{}
	if err := yamlsig.Unmarshal([]byte(content), &parsed); err != nil {
		return summary
	}
	if cc, ok := parsed["current-context"].(string); ok {
		summary.CurrentContext = cc
	}
	if clusters, ok := parsed["clusters"].([]interface{}); ok {
		for _, c := range clusters {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if name, ok := cm["name"].(string); ok {
				summary.Clusters = append(summary.Clusters, name)
			}
			if cluster, ok := cm["cluster"].(map[string]interface{}); ok {
				if server, ok := cluster["server"].(string); ok {
					summary.ServerURLs = append(summary.ServerURLs, server)
				}
			}
		}
	}
	if contexts, ok := parsed["contexts"].([]interface{}); ok {
		for _, c := range contexts {
			if cm, ok := c.(map[string]interface{}); ok {
				if name, ok := cm["name"].(string); ok {
					summary.Contexts = append(summary.Contexts, name)
				}
			}
		}
	}
	if users, ok := parsed["users"].([]interface{}); ok {
		for _, u := range users {
			if um, ok := u.(map[string]interface{}); ok {
				if name, ok := um["name"].(string); ok {
					summary.Users = append(summary.Users, name)
				}
			}
		}
	}
	return summary
}

func validateKubernetesYaml(payload string) error {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(payload), 4096)
	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("invalid YAML: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		jsonData, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("failed to serialize YAML: %w", err)
		}
		if _, _, err := scheme.Codecs.UniversalDeserializer().Decode(jsonData, nil, nil); err != nil {
			return fmt.Errorf("schema validation failed: %w", err)
		}
	}
	return nil
}

func splitKubernetesYaml(payload string) ([]string, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(payload), 4096)
	var docs []string
	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		jsonData, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize YAML: %w", err)
		}
		docs = append(docs, string(jsonData))
	}
	if len(docs) == 0 {
		return nil, errors.New("yaml input contains no resources")
	}
	return docs, nil
}

type DeleteKubernetesResourceArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=The project ID of the resource"`
	Kind      string `json:"kind" jsonschema:"required,description=The kind of the resource to delete. Matching is case-insensitive; examples include Pod, Deployment, and Service. Call list-kubernetes-resource-kinds to inspect supported operationKinds."`
	Name      string `json:"name" jsonschema:"required,description=The name of the resource to delete"`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=The namespace of the resource (optional, defaults to 'default')"`
}

type DeployKubernetesResourcesArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=The project ID to deploy the resources to"`
	YAML      string `json:"yaml" jsonschema:"required,description=The Kubernetes resources in YAML format (raw or base64-encoded)"`
}

type CreateKubeConfigArgs struct {
	Name                   string `json:"name,omitempty" jsonschema:"description=The name of the kubeconfig (optional)"`
	ProjectID              int32  `json:"projectId" jsonschema:"required,description=The project ID to create the kubeconfig for"`
	IsAccessibleForAll     bool   `json:"isAccessibleForAll,omitempty" jsonschema:"description=Whether the kubeconfig is accessible for all (default: false)"`
	IsAccessibleForManager bool   `json:"isAccessibleForManager,omitempty" jsonschema:"description=Whether the kubeconfig is accessible for managers (default: false)"`
	KubeConfigRoleId       int32  `json:"kubeConfigRoleId,omitempty" jsonschema:"description=The role ID for the kubeconfig (optional). Defaults to cluster-admin (1). Role IDs: 1=cluster-admin, 2=admin, 3=edit, 4=view. AWS projects only support role 1."`
	UserId                 string `json:"userId,omitempty" jsonschema:"description=The user ID for the kubeconfig (optional)"`
	Namespace              string `json:"namespace,omitempty" jsonschema:"description=The namespace for the kubeconfig (optional)"`
	TTL                    int32  `json:"ttl,omitempty" jsonschema:"description=The TTL for the kubeconfig in minutes (optional)"`
}

type GetKubeConfigArgs struct {
	ProjectID    int32  `json:"projectId" jsonschema:"required,description=The project ID to get the kubeconfig for"`
	KubeconfigID int32  `json:"kubeconfigId,omitempty" jsonschema:"description=Optional kubeconfig ID to download. When omitted, prefers a downloadable cluster-admin or admin kubeconfig. Use list-kubeconfigs to inspect available IDs."`
	SavePath     string `json:"savePath,omitempty" jsonschema:"description=Optional path to save kubeconfig as a YAML file"`
}

type ListKubeConfigsArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=The project ID to list kubeconfigs for"`
	Limit     int32 `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset    int32 `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
}

type ListKubernetesResourcesArgs struct {
	ProjectID  int32  `json:"projectId" jsonschema:"required,description=The project ID to list resources from"`
	Kind       string `json:"kind" jsonschema:"required,description=The kind of Kubernetes resource to list. Matching is case-insensitive and uses list-specific canonical names such as Pods, Deployments, Services, Namespaces, ConfigMaps, Secrets, Ingress, DaemonSets, Nodes, Pvcs, and Sts. Call list-kubernetes-resource-kinds to inspect supported listKinds and unavailableListKinds."`
	Limit      int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return (optional)"`
	Offset     int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip (optional)"`
	SearchTerm string `json:"searchTerm,omitempty" jsonschema:"description=Search term to filter results (optional)"`
}

type DescribeKubernetesResourceArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=The project ID of the resource"`
	Name      string `json:"name" jsonschema:"required,description=The name of the resource"`
	Kind      string `json:"kind" jsonschema:"required,description=The kind of the resource to describe. Matching is case-insensitive; examples include Pod, Deployment, and Service. Call list-kubernetes-resource-kinds to inspect supported operationKinds."`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=The namespace of the resource (optional, defaults to 'default')"`
}

type PatchKubernetesResourceArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=The project ID of the resource"`
	Name      string `json:"name" jsonschema:"required,description=The name of the resource to patch"`
	Yaml      string `json:"yaml" jsonschema:"required,description=The YAML patch to apply to the resource (raw or base64-encoded)"`
	Namespace string `json:"namespace,omitempty" jsonschema:"description=The namespace of the resource (optional, defaults to 'default')"`
}

type KubernetesResourceKindsArgs struct{}

type ListKubeConfigRolesArgs struct{}

type KubeConfigListItemSummary struct {
	ID                     int32  `json:"id"`
	DisplayName            string `json:"displayName,omitempty"`
	ProjectID              int32  `json:"projectId"`
	KubeConfigRoleName     string `json:"kubeConfigRoleName"`
	KubeConfigRoleID       int32  `json:"kubeConfigRoleId,omitempty"`
	CanDownload            bool   `json:"canDownload"`
	CanAccessTerminal      bool   `json:"canAccessTerminal"`
	CanDelete              bool   `json:"canDelete"`
	ExpirationDate         string `json:"expirationDate,omitempty"`
	IsAccessibleForAll     bool   `json:"isAccessibleForAll"`
	IsAccessibleForManager bool   `json:"isAccessibleForManager"`
	Namespace              string `json:"namespace,omitempty"`
	CreatedAt              string `json:"createdAt,omitempty"`
	CreatedBy              string `json:"createdBy,omitempty"`
}

type kubeConfigSelection struct {
	ID                 int32
	DisplayName        string
	KubeConfigRoleName string
	KubeConfigRoleID   int32
}

const awsKubeConfigRoleID int32 = 1
const defaultKubeConfigRoleID int32 = 1
const viewKubeConfigRoleID int32 = 4

type kubernetesListKindSpec struct {
	resourcePath       string
	unavailableMessage string
}

type KubernetesUnavailableResourceKind struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type KubernetesResourceKindsResponse struct {
	ListKinds            []string                            `json:"listKinds"`
	OperationKinds       []string                            `json:"operationKinds"`
	UnavailableListKinds []KubernetesUnavailableResourceKind `json:"unavailableListKinds,omitempty"`
	CaseInsensitive      bool                                `json:"caseInsensitive"`
	Success              bool                                `json:"success"`
	Message              string                              `json:"message"`
}

var kubernetesListKindOrder = []string{
	"Pods",
	"Deployments",
	"Services",
	"Namespaces",
	"ConfigMaps",
	"Secrets",
	"Ingress",
	"CronJobs",
	"DaemonSets",
	"Jobs",
	"Nodes",
	"Pvcs",
	"StorageClasses",
	"Sts",
}

var kubernetesListKindSpecs = map[string]kubernetesListKindSpec{
	"Pods": {
		resourcePath: "pods",
	},
	"Deployments": {
		resourcePath: "deployments",
	},
	"Services": {
		resourcePath: "service",
	},
	"Namespaces": {},
	"ConfigMaps": {
		resourcePath: "configmap",
	},
	"Secrets": {
		resourcePath: "secret",
	},
	"Ingress": {
		resourcePath: "ingress",
	},
	"CronJobs": {
		unavailableMessage: "CronJobs listing is not available through the Cloudera Cloud Factory Kubernetes list API",
	},
	"DaemonSets": {
		resourcePath: "daemonset",
	},
	"Jobs": {
		unavailableMessage: "Jobs listing is not available through the Cloudera Cloud Factory Kubernetes list API",
	},
	"Nodes": {
		resourcePath: "nodes",
	},
	"Pvcs": {
		resourcePath: "pvc",
	},
	"StorageClasses": {
		unavailableMessage: "StorageClasses listing is not available through the Cloudera Cloud Factory Kubernetes list API",
	},
	"Sts": {
		resourcePath: "sts",
	},
}

func normalizeListKubernetesKind(kind string) (string, bool) {
	trimmedKind := strings.TrimSpace(kind)
	for _, canonicalKind := range kubernetesListKindOrder {
		if strings.EqualFold(canonicalKind, trimmedKind) {
			return canonicalKind, true
		}
	}
	return "", false
}

func normalizeOperationKubernetesKind(kind string) (taikuncore.EKubernetesResource, bool) {
	trimmedKind := strings.TrimSpace(kind)
	for _, canonicalKind := range taikuncore.AllowedEKubernetesResourceEnumValues {
		if strings.EqualFold(string(canonicalKind), trimmedKind) {
			return canonicalKind, true
		}
	}
	return "", false
}

func kubernetesRecognizedListKinds() []string {
	return append([]string(nil), kubernetesListKindOrder...)
}

func kubernetesSupportedListKinds() []string {
	supportedKinds := make([]string, 0, len(kubernetesListKindOrder))
	for _, kind := range kubernetesListKindOrder {
		if kubernetesListKindSpecs[kind].unavailableMessage == "" {
			supportedKinds = append(supportedKinds, kind)
		}
	}
	return supportedKinds
}

func kubernetesUnavailableListKinds() []KubernetesUnavailableResourceKind {
	unavailableKinds := make([]KubernetesUnavailableResourceKind, 0)
	for _, kind := range kubernetesListKindOrder {
		spec := kubernetesListKindSpecs[kind]
		if spec.unavailableMessage == "" {
			continue
		}
		unavailableKinds = append(unavailableKinds, KubernetesUnavailableResourceKind{
			Kind:   kind,
			Reason: spec.unavailableMessage,
		})
	}
	return unavailableKinds
}

func kubernetesOperationKindStrings() []string {
	operationKinds := make([]string, 0, len(taikuncore.AllowedEKubernetesResourceEnumValues))
	for _, kind := range taikuncore.AllowedEKubernetesResourceEnumValues {
		operationKinds = append(operationKinds, string(kind))
	}
	return operationKinds
}

func invalidKubernetesKindResponse(kind string, operation string, allowedKinds []string) *mcp_golang.ToolResponse {
	return createJSONResponse(ErrorResponse{
		Error: fmt.Sprintf("Invalid resource kind: %s", kind),
		Details: fmt.Sprintf(
			"Allowed kinds for %s are case-insensitive and must match one of: %s. Call list-kubernetes-resource-kinds for the supported listKinds and operationKinds.",
			operation,
			strings.Join(allowedKinds, ", "),
		),
	})
}

func listKubernetesResourceKinds() *mcp_golang.ToolResponse {
	return createJSONResponse(KubernetesResourceKindsResponse{
		ListKinds:            kubernetesSupportedListKinds(),
		OperationKinds:       kubernetesOperationKindStrings(),
		UnavailableListKinds: kubernetesUnavailableListKinds(),
		CaseInsensitive:      true,
		Success:              true,
		Message:              "Loaded supported Kubernetes resource kinds for list and resource operations",
	})
}

func resolveProjectCloudType(client *taikungoclient.Client, projectID int32) (string, *mcp_golang.ToolResponse) {
	ctx := context.Background()

	projectList, httpResponse, err := client.Client.ProjectsAPI.ProjectsList(ctx).
		Id(projectID).
		Execute()
	if err != nil {
		return "", createError(httpResponse, err)
	}
	if errorResp := checkResponse(httpResponse, "get project details"); errorResp != nil {
		return "", errorResp
	}
	if projectList == nil || len(projectList.Data) == 0 {
		return "", createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("Project with ID %d not found", projectID),
		})
	}

	return string(projectList.Data[0].GetCloudType()), nil
}

func kubeConfigRoleNameFromID(roleID int32) string {
	switch roleID {
	case 1:
		return "cluster-admin"
	case 2:
		return "admin"
	case 3:
		return "edit"
	case 4:
		return "view"
	default:
		return ""
	}
}

func kubeConfigRoleIDFromName(roleName string) int32 {
	switch strings.ToLower(strings.TrimSpace(roleName)) {
	case "cluster-admin":
		return 1
	case "admin":
		return 2
	case "edit":
		return 3
	case "view":
		return 4
	default:
		return 0
	}
}

func summarizeKubeConfigListItem(item taikuncore.KubeConfigForUserDto) KubeConfigListItemSummary {
	roleName := strings.TrimSpace(item.KubeConfigRoleName)
	summary := KubeConfigListItemSummary{
		ID:                     item.Id,
		DisplayName:            strings.TrimSpace(item.GetDisplayName()),
		ProjectID:              item.ProjectId,
		KubeConfigRoleName:     roleName,
		KubeConfigRoleID:       kubeConfigRoleIDFromName(roleName),
		CanDownload:            item.CanDownload,
		CanAccessTerminal:      item.CanAccessTerminal,
		CanDelete:              item.CanDelete,
		ExpirationDate:         strings.TrimSpace(item.GetExpirationDate()),
		IsAccessibleForAll:     item.IsAccessibleForAll,
		IsAccessibleForManager: item.IsAccessibleForManager,
		Namespace:              strings.TrimSpace(item.GetNamespace()),
		CreatedAt:              strings.TrimSpace(item.GetCreatedAt()),
		CreatedBy:              strings.TrimSpace(item.GetCreatedBy()),
	}
	return summary
}

func normalizeKubeConfigRoleID(projectID int32, cloudType string, requestedRoleID int32) (int32, *mcp_golang.ToolResponse) {
	if requestedRoleID == 0 {
		requestedRoleID = defaultKubeConfigRoleID
	}
	if strings.EqualFold(strings.TrimSpace(cloudType), "AWS") && requestedRoleID != awsKubeConfigRoleID {
		return 0, createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("AWS-based project %d only supports kubeConfigRoleId %d", projectID, awsKubeConfigRoleID),
			Details: fmt.Sprintf("Project cloudType %q requires kubeConfigRoleId %d when creating kubeconfigs", cloudType, awsKubeConfigRoleID),
		})
	}

	return requestedRoleID, nil
}

func selectDownloadableKubeConfig(items []taikuncore.KubeConfigForUserDto, projectID int32, requestedID int32) (*kubeConfigSelection, *ErrorResponse) {
	if requestedID != 0 {
		for _, item := range items {
			if item.ProjectId != projectID || item.Id != requestedID {
				continue
			}
			if !item.CanDownload {
				return nil, &ErrorResponse{
					Error:   fmt.Sprintf("Kubeconfig %d for project %d is not downloadable", requestedID, projectID),
					Details: "Use list-kubeconfigs to inspect canDownload and pick another kubeconfig ID, or create-kubeconfig to add one.",
				}
			}
			roleName := strings.TrimSpace(item.KubeConfigRoleName)
			return &kubeConfigSelection{
				ID:                 item.Id,
				DisplayName:        strings.TrimSpace(item.GetDisplayName()),
				KubeConfigRoleName: roleName,
				KubeConfigRoleID:   kubeConfigRoleIDFromName(roleName),
			}, nil
		}
		return nil, &ErrorResponse{
			Error:   fmt.Sprintf("Kubeconfig %d not found for project %d", requestedID, projectID),
			Details: "Use list-kubeconfigs to see available kubeconfigs for this project.",
		}
	}

	var fallback *kubeConfigSelection
	for _, item := range items {
		if item.ProjectId != projectID || !item.CanDownload {
			continue
		}
		roleName := strings.TrimSpace(item.KubeConfigRoleName)
		selection := &kubeConfigSelection{
			ID:                 item.Id,
			DisplayName:        strings.TrimSpace(item.GetDisplayName()),
			KubeConfigRoleName: roleName,
			KubeConfigRoleID:   kubeConfigRoleIDFromName(roleName),
		}
		if roleName == "cluster-admin" || roleName == "admin" {
			return selection, nil
		}
		if fallback == nil {
			fallback = selection
		}
	}

	if fallback == nil {
		return nil, &ErrorResponse{
			Error:   fmt.Sprintf("No downloadable kubeconfig found for project %d", projectID),
			Details: "Use list-kubeconfigs to inspect available entries. If canDownload is false for all entries, create-kubeconfig may be required.",
		}
	}
	return fallback, nil
}

func deployKubernetesResources(client *taikungoclient.Client, args DeployKubernetesResourcesArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	normalizedYaml, err := normalizeYamlInput(args.YAML)
	if err != nil {
		return createJSONResponse(ErrorResponse{Error: err.Error()}), nil
	}
	docs, err := splitKubernetesYaml(normalizedYaml)
	if err != nil {
		return createJSONResponse(ErrorResponse{Error: err.Error()}), nil
	}
	for _, doc := range docs {
		if err := validateKubernetesYaml(doc); err != nil {
			return createJSONResponse(ErrorResponse{Error: err.Error()}), nil
		}
		encodedYaml := base64.StdEncoding.EncodeToString([]byte(doc))
		createCmd := taikuncore.NewCreateKubernetesResourceCommand(args.ProjectID, *taikuncore.NewNullableString(&encodedYaml))
		httpResponse, err := client.Client.KubernetesAPI.KubernetesCreateResource(ctx).
			CreateKubernetesResourceCommand(*createCmd).
			Execute()
		if err != nil {
			return createError(httpResponse, err), nil
		}
		if errorResp := checkResponse(httpResponse, "deploy kubernetes resources"); errorResp != nil {
			return errorResp, nil
		}
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Kubernetes resources deployed successfully (%d resource(s))", len(docs)),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func createKubeConfig(client *taikungoclient.Client, args CreateKubeConfigArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()
	projectCloudType, errorResp := resolveProjectCloudType(client, args.ProjectID)
	if errorResp != nil {
		return errorResp, nil
	}
	kubeConfigRoleID, errorResp := normalizeKubeConfigRoleID(args.ProjectID, projectCloudType, args.KubeConfigRoleId)
	if errorResp != nil {
		return errorResp, nil
	}

	createCmd := taikuncore.NewCreateKubeConfigCommand()
	createCmd.SetProjectId(args.ProjectID)

	if args.Name != "" {
		createCmd.SetName(args.Name)
	}
	createCmd.SetIsAccessibleForAll(args.IsAccessibleForAll)
	createCmd.SetIsAccessibleForManager(args.IsAccessibleForManager)

	if kubeConfigRoleID != 0 {
		createCmd.SetKubeConfigRoleId(kubeConfigRoleID)
	}
	if args.UserId != "" {
		createCmd.SetUserId(args.UserId)
	}
	if args.Namespace != "" {
		createCmd.SetNamespace(args.Namespace)
	}
	if args.TTL != 0 {
		createCmd.SetTtl(args.TTL)
	}

	apiResp, httpResponse, err := client.Client.KubeConfigAPI.KubeconfigCreate(ctx).
		CreateKubeConfigCommand(*createCmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "create kubeconfig"); errorResp != nil {
		return errorResp, nil
	}

	response := map[string]interface{}{
		"success":          true,
		"kubeConfigRoleId": kubeConfigRoleID,
		"message":          fmt.Sprintf("Kubeconfig created successfully for project %d", args.ProjectID),
	}
	if roleName := kubeConfigRoleNameFromID(kubeConfigRoleID); roleName != "" {
		response["kubeConfigRoleName"] = roleName
	}
	if apiResp != nil {
		if kubeconfigID, ok := parseInt32Strict(apiResp.GetId()); ok {
			response["kubeconfigId"] = kubeconfigID
		}
		if apiResp.GetMessage() != "" {
			response["message"] = apiResp.GetMessage()
		}
	}

	return createJSONResponse(response), nil
}

func getKubeConfig(client *taikungoclient.Client, args GetKubeConfigArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	listRequest := client.Client.KubeConfigAPI.KubeconfigList(ctx).
		ProjectId(args.ProjectID).
		Limit(100)
	listResp, listHTTPResponse, err := listRequest.Execute()
	if err != nil {
		return createError(listHTTPResponse, err), nil
	}

	if errorResp := checkResponse(listHTTPResponse, "list kubeconfigs"); errorResp != nil {
		return errorResp, nil
	}

	var items []taikuncore.KubeConfigForUserDto
	if listResp != nil {
		items = listResp.Data
	}

	selection, selectionErr := selectDownloadableKubeConfig(items, args.ProjectID, args.KubeconfigID)
	if selectionErr != nil {
		return createJSONResponse(*selectionErr), nil
	}

	downloadCmd := taikuncore.NewDownloadKubeConfigCommand()
	downloadCmd.SetId(selection.ID)
	downloadCmd.SetProjectId(args.ProjectID)
	kubeconfig, downloadHTTPResponse, err := client.Client.KubeConfigAPI.KubeconfigDownload(ctx).
		DownloadKubeConfigCommand(*downloadCmd).
		Execute()
	if err != nil {
		return createError(downloadHTTPResponse, err), nil
	}

	if errorResp := checkResponse(downloadHTTPResponse, "download kubeconfig"); errorResp != nil {
		return errorResp, nil
	}

	if kubeconfig == "" {
		errorResp := ErrorResponse{
			Error: fmt.Sprintf("Kubeconfig for project %d not found", args.ProjectID),
		}
		return createJSONResponse(errorResp), nil
	}

	normalizedKubeconfig := normalizeKubeconfigYaml(kubeconfig)

	// Kubeconfigs contain CA and client credentials. Rather than echo them into
	// the agent context, always persist to a file (defaulting to a temp path)
	// and return only the path plus a non-secret summary.
	savePath := args.SavePath
	if savePath == "" {
		savePath = filepath.Join(os.TempDir(), fmt.Sprintf("ccf-kubeconfig-%d-%d.yaml", args.ProjectID, time.Now().Unix()))
	}
	if dir := filepath.Dir(savePath); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return createJSONResponse(ErrorResponse{
				Error: fmt.Sprintf("Failed to create directory for kubeconfig: %v", err),
			}), nil
		}
	}
	if err := os.WriteFile(savePath, []byte(normalizedKubeconfig), 0o600); err != nil {
		return createJSONResponse(ErrorResponse{
			Error: fmt.Sprintf("Failed to write kubeconfig file: %v", err),
		}), nil
	}

	response := map[string]interface{}{
		"savedPath":          savePath,
		"summary":            summarizeKubeconfig(normalizedKubeconfig),
		"success":            true,
		"kubeconfigId":       selection.ID,
		"kubeConfigRoleName": selection.KubeConfigRoleName,
		"message":            fmt.Sprintf("Kubeconfig %d (%s) for project %d written to %s. It contains credentials and is intentionally not echoed into the response; read the file or pass it to kubectl with --kubeconfig.", selection.ID, selection.KubeConfigRoleName, args.ProjectID, savePath),
	}
	if selection.DisplayName != "" {
		response["displayName"] = selection.DisplayName
	}
	if selection.KubeConfigRoleID != 0 {
		response["kubeConfigRoleId"] = selection.KubeConfigRoleID
	}

	return createJSONResponse(response), nil
}

func listKubeConfigs(client *taikungoclient.Client, args ListKubeConfigsArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	listRequest := client.Client.KubeConfigAPI.KubeconfigList(ctx).
		ProjectId(args.ProjectID)
	if args.Limit > 0 {
		listRequest = listRequest.Limit(args.Limit)
	}
	if args.Offset > 0 {
		listRequest = listRequest.Offset(args.Offset)
	}

	listResp, httpResponse, err := listRequest.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list kubeconfigs"); errorResp != nil {
		return errorResp, nil
	}

	items := make([]KubeConfigListItemSummary, 0)
	downloadableCount := 0
	if listResp != nil {
		for _, item := range listResp.Data {
			if item.ProjectId != args.ProjectID {
				continue
			}
			summary := summarizeKubeConfigListItem(item)
			items = append(items, summary)
			if summary.CanDownload {
				downloadableCount++
			}
		}
	}

	total := len(items)
	if listResp != nil {
		total = int(listResp.GetTotalCount())
	}

	listResponse := struct {
		Items             []KubeConfigListItemSummary `json:"items"`
		Total             int                         `json:"total"`
		DownloadableCount int                         `json:"downloadableCount"`
		ProjectID         int32                       `json:"projectId"`
		Message           string                      `json:"message"`
		Success           bool                        `json:"success"`
	}{
		Items:             items,
		Total:             total,
		DownloadableCount: downloadableCount,
		ProjectID:         args.ProjectID,
		Message:           fmt.Sprintf("Found %d kubeconfig(s) for project %d (%d downloadable)", len(items), args.ProjectID, downloadableCount),
		Success:           true,
	}

	return createJSONResponse(listResponse), nil
}

func listKubeConfigRoles(client *taikungoclient.Client, _ ListKubeConfigRolesArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	roles, httpResponse, err := client.Client.KubeConfigRoleAPI.KubeconfigroleList(ctx).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, "list kubeconfig roles"); errorResp != nil {
		return errorResp, nil
	}

	type RoleSummary struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}

	var roleSummaries []RoleSummary
	if roles != nil {
		for _, role := range roles.Data {
			roleSummaries = append(roleSummaries, RoleSummary{
				ID:   role.GetId(),
				Name: role.GetName(),
			})
		}
	}

	return createJSONResponse(roleSummaries), nil
}

func kubernetesListEndpoints(baseURL string, projectID int32, resource string) []string {
	legacyPath := fmt.Sprintf("%s/api/v1/kubernetes/list/%d/%s", baseURL, projectID, resource)
	if strings.EqualFold(resource, "pods") {
		// Pods recently moved to a dedicated endpoint in the upstream API.
		return []string{
			fmt.Sprintf("%s/api/v1/kubernetes/%d/pods-list", baseURL, projectID),
			legacyPath,
		}
	}
	return []string{legacyPath}
}

func fetchKubernetesListPage[T any](ctx context.Context, client *taikungoclient.Client, projectID int32, resource string, limit int32, cursor string, searchTerm string) (cursorPaginatedResponse[T], *http.Response, error) {
	var result cursorPaginatedResponse[T]

	if client == nil || client.Client == nil {
		return result, nil, fmt.Errorf("cloudera Cloud Factory client is not initialized")
	}

	cfg := client.Client.GetConfig()
	if cfg == nil || cfg.HTTPClient == nil {
		return result, nil, fmt.Errorf("cloudera Cloud Factory client config is not available")
	}

	baseURL := fmt.Sprintf("%s://%s", cfg.Scheme, cfg.Host)
	endpoints := kubernetesListEndpoints(baseURL, projectID, resource)

	for idx, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return result, nil, err
		}

		query := req.URL.Query()
		if limit > 0 {
			query.Set("Limit", fmt.Sprintf("%d", limit))
		}
		if cursor != "" {
			query.Set("Cursor", cursor)
		}
		if searchTerm != "" {
			query.Set("SearchTerm", searchTerm)
		}
		req.URL.RawQuery = query.Encode()
		req.Header.Set("Accept", "application/json")

		response, reqErr := cfg.HTTPClient.Do(req)
		if reqErr != nil {
			if response != nil && response.Body != nil {
				if closeErr := response.Body.Close(); closeErr != nil {
					logger.Printf("Failed to close Kubernetes list response body after request error: %v", closeErr)
				}
			}
			return result, response, reqErr
		}

		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			// If pods return 404 on one API shape, try the compatibility fallback.
			if idx < len(endpoints)-1 && response.StatusCode == http.StatusNotFound {
				if closeErr := response.Body.Close(); closeErr != nil {
					logger.Printf("Failed to close fallback Kubernetes list response body: %v", closeErr)
				}
				continue
			}
			if response.Body != nil {
				if closeErr := response.Body.Close(); closeErr != nil {
					logger.Printf("Failed to close Kubernetes list response body on error response: %v", closeErr)
				}
			}
			return result, response, fmt.Errorf("request failed with status %d", response.StatusCode)
		}

		defer func() {
			if err := response.Body.Close(); err != nil {
				logger.Printf("Failed to close Kubernetes list response body: %v", err)
			}
		}()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return result, response, err
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return result, response, err
		}

		return result, response, nil
	}

	return result, nil, fmt.Errorf("failed to query Kubernetes list endpoint")
}

func fetchKubernetesListItems[T any](ctx context.Context, client *taikungoclient.Client, projectID int32, resource string, limit int32, offset int32, searchTerm string) ([]T, *http.Response, error) {
	var allItems []T
	var cursor string
	var lastResponse *http.Response
	perPage := limit
	if perPage <= 0 {
		perPage = 50
	}

	remainingOffset := offset
	remainingLimit := limit

	for {
		page, response, err := fetchKubernetesListPage[T](ctx, client, projectID, resource, perPage, cursor, searchTerm)
		lastResponse = response
		if err != nil {
			return nil, response, err
		}

		items := page.Data
		if remainingOffset > 0 {
			if int32(len(items)) <= remainingOffset {
				remainingOffset -= int32(len(items))
				items = nil
			} else {
				items = items[remainingOffset:]
				remainingOffset = 0
			}
		}

		if remainingLimit > 0 {
			needed := remainingLimit - int32(len(allItems))
			if needed <= 0 {
				break
			}
			if int32(len(items)) > needed {
				items = items[:needed]
			}
		}

		allItems = append(allItems, items...)

		if remainingLimit > 0 && int32(len(allItems)) >= remainingLimit {
			break
		}

		if !page.HasMore || page.NextCursor == nil || *page.NextCursor == "" {
			break
		}

		cursor = *page.NextCursor
	}

	return allItems, lastResponse, nil
}

func listKubernetesError(kind string, response *http.Response, err error) *mcp_golang.ToolResponse {
	if response != nil && (response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices) {
		return createError(response, err)
	}
	errorResp := ErrorResponse{
		Error: fmt.Sprintf("Failed to list %s: %v", kind, err),
	}
	return createJSONResponse(errorResp)
}

func listKubernetesResources(client *taikungoclient.Client, args ListKubernetesResourcesArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()
	var result interface{}
	canonicalKind, ok := normalizeListKubernetesKind(args.Kind)
	if !ok {
		return invalidKubernetesKindResponse(args.Kind, "list-kubernetes-resources", kubernetesRecognizedListKinds()), nil
	}
	spec := kubernetesListKindSpecs[canonicalKind]
	if spec.unavailableMessage != "" {
		return createJSONResponse(ErrorResponse{
			Error:   spec.unavailableMessage,
			Details: "Call list-kubernetes-resource-kinds to inspect the supported listKinds and unavailableListKinds.",
		}), nil
	}

	// Bound the response by default; without a limit the cursor paginator drains
	// every page, which is unbounded on large clusters. Callers can page with
	// offset or raise limit explicitly.
	if args.Limit <= 0 {
		args.Limit = 200
	}

	switch canonicalKind {
	case "Pods":
		pods, response, err := fetchKubernetesListItems[podListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Pods", response, err), nil
		}
		summaries := make([]PodSummary, 0, len(pods))
		for _, pod := range pods {
			summaries = append(summaries, PodSummary{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Status:    pod.State,
				PodIP:     pod.IP,
				StartTime: pod.CreatedAt,
				Restarts:  pod.RestartCount,
			})
		}
		result = summaries
	case "Deployments":
		deployments, response, err := fetchKubernetesListItems[deploymentListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Deployments", response, err), nil
		}
		summaries := make([]DeploymentSummary, 0, len(deployments))
		for _, deployment := range deployments {
			readyCount, totalCount := parseReadyCounts(deployment.Ready)
			summaries = append(summaries, DeploymentSummary{
				Name:          deployment.Name,
				Namespace:     deployment.Namespace,
				Replicas:      totalCount,
				ReadyReplicas: readyCount,
				Age:           formatAgeFromString(deployment.CreatedAt),
			})
		}
		result = summaries
	case "Services":
		services, response, err := fetchKubernetesListItems[serviceListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Services", response, err), nil
		}
		summaries := make([]ServiceSummary, 0, len(services))
		for _, service := range services {
			summaries = append(summaries, ServiceSummary{
				Name:      service.Name,
				Namespace: service.Namespace,
				Type:      service.Type,
				ClusterIP: service.ClusterIP,
				Age:       formatAgeFromString(service.CreatedAt),
			})
		}
		result = summaries
	case "Namespaces":
		namespaces, response, err := client.Client.KubernetesAPI.KubernetesNamespaceList(ctx, args.ProjectID).Execute()
		if err != nil {
			return createError(response, err), nil
		}
		var summaries []NamespaceSummary
		for _, name := range namespaces {
			if args.SearchTerm != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(args.SearchTerm)) {
				continue
			}
			summaries = append(summaries, NamespaceSummary{
				Name: name,
			})
		}
		if args.Offset > 0 || args.Limit > 0 {
			start := int(args.Offset)
			if start > len(summaries) {
				start = len(summaries)
			}
			end := len(summaries)
			if args.Limit > 0 && start+int(args.Limit) < end {
				end = start + int(args.Limit)
			}
			summaries = summaries[start:end]
		}
		result = summaries
	case "ConfigMaps":
		configMaps, response, err := fetchKubernetesListItems[configMapListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("ConfigMaps", response, err), nil
		}
		summaries := make([]ConfigMapSummary, 0, len(configMaps))
		for _, cm := range configMaps {
			summaries = append(summaries, ConfigMapSummary{
				Name:      cm.Name,
				Namespace: cm.Namespace,
				Age:       formatAgeFromString(cm.CreatedAt),
			})
		}
		result = summaries
	case "Secrets":
		secrets, response, err := fetchKubernetesListItems[secretListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Secrets", response, err), nil
		}
		summaries := make([]SecretSummary, 0, len(secrets))
		for _, secret := range secrets {
			summaries = append(summaries, SecretSummary{
				Name:      secret.Name,
				Namespace: secret.Namespace,
				Type:      secret.Type,
				Age:       formatAgeFromString(secret.CreatedAt),
			})
		}
		result = summaries
	case "Ingress":
		ingresses, response, err := fetchKubernetesListItems[ingressListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Ingress", response, err), nil
		}
		summaries := make([]IngressSummary, 0, len(ingresses))
		for _, ingress := range ingresses {
			var hosts []string
			if ingress.Target != "" {
				hosts = []string{ingress.Target}
			}
			summaries = append(summaries, IngressSummary{
				Name:      ingress.Name,
				Namespace: ingress.Namespace,
				Hosts:     hosts,
				Address:   "",
				Age:       formatAgeFromString(ingress.CreatedAt),
			})
		}
		result = summaries
	case "DaemonSets":
		daemonSets, response, err := fetchKubernetesListItems[daemonSetListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("DaemonSets", response, err), nil
		}
		summaries := make([]DaemonSetSummary, 0, len(daemonSets))
		for _, ds := range daemonSets {
			summaries = append(summaries, DaemonSetSummary{
				Name:             ds.Name,
				Namespace:        ds.Namespace,
				DesiredScheduled: ds.Desired,
				CurrentScheduled: ds.Current,
				NumberReady:      ds.Ready,
				NumberAvailable:  parseInt32(ds.Available),
				Age:              formatAgeFromString(ds.CreatedAt),
			})
		}
		result = summaries
	case "Nodes":
		nodes, response, err := fetchKubernetesListItems[nodeListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Nodes", response, err), nil
		}
		summaries := make([]NodeSummary, 0, len(nodes))
		for _, node := range nodes {
			summaries = append(summaries, NodeSummary{
				Name:    node.Name,
				Status:  node.State,
				Roles:   node.Role,
				Version: node.Version,
			})
		}
		result = summaries
	case "Pvcs":
		pvcs, response, err := fetchKubernetesListItems[pvcListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Pvcs", response, err), nil
		}
		summaries := make([]PvcSummary, 0, len(pvcs))
		for _, pvc := range pvcs {
			summaries = append(summaries, PvcSummary{
				Name:         pvc.Name,
				Namespace:    pvc.Namespace,
				Status:       pvc.Status,
				Volume:       pvc.Volume,
				Capacity:     pvc.Capacity,
				AccessModes:  pvc.AccessModes,
				StorageClass: pvc.StorageClass,
				Age:          formatAgeFromString(pvc.CreatedAt),
			})
		}
		result = summaries
	case "Sts":
		statefulSets, response, err := fetchKubernetesListItems[statefulSetListItem](ctx, client, args.ProjectID, spec.resourcePath, args.Limit, args.Offset, args.SearchTerm)
		if err != nil {
			return listKubernetesError("Sts", response, err), nil
		}
		summaries := make([]StatefulSetSummary, 0, len(statefulSets))
		for _, sts := range statefulSets {
			readyCount, totalCount := parseReadyCounts(sts.Ready)
			summaries = append(summaries, StatefulSetSummary{
				Name:          sts.Name,
				Namespace:     sts.Namespace,
				Replicas:      totalCount,
				ReadyReplicas: readyCount,
				Age:           formatAgeFromString(sts.CreatedAt),
			})
		}
		result = summaries
	default:
		return invalidKubernetesKindResponse(args.Kind, "list-kubernetes-resources", kubernetesRecognizedListKinds()), nil
	}

	return createJSONResponse(result), nil
}

func describeKubernetesResource(client *taikungoclient.Client, args DescribeKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	if hint := unsupportedClusterIssuerDescribeResponse(args.Kind, args.Name); hint != nil {
		return hint, nil
	}

	kind, ok := normalizeOperationKubernetesKind(args.Kind)
	if !ok {
		return invalidKubernetesKindResponse(args.Kind, "describe-kubernetes-resource", kubernetesOperationKindStrings()), nil
	}

	describeCmd := taikuncore.NewDescribeKubernetesResourceCommand(args.ProjectID, args.Name, kind)
	if args.Namespace != "" {
		describeCmd.SetNamespace(args.Namespace)
	}

	description, httpResponse, err := client.Client.KubernetesAPI.KubernetesDescribeResource(ctx).
		DescribeKubernetesResourceCommand(*describeCmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, fmt.Sprintf("describe %s %s", kind, args.Name)); errorResp != nil {
		return errorResp, nil
	}

	type DescribeResponse struct {
		YAML    string `json:"yaml"`
		Success bool   `json:"success"`
	}
	resp := DescribeResponse{
		YAML:    trimKubernetesDescribeYAML(normalizeYamlOutput(description)),
		Success: true,
	}
	return createJSONResponse(resp), nil
}

func unsupportedClusterIssuerDescribeResponse(kind string, name string) *mcp_golang.ToolResponse {
	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	if normalizedKind == "clusterissuer" ||
		normalizedKind == "clusterissuers" ||
		normalizedKind == "clusterissuer.cert-manager.io" ||
		normalizedKind == "clusterissuers.cert-manager.io" ||
		((normalizedKind == "crd" || normalizedKind == "customresourcedefinition" || normalizedKind == "customresourcedefinitions") && normalizedName == "ccf-default") {
		issuerName := strings.TrimSpace(name)
		if issuerName == "" {
			issuerName = "ccf-default"
		}
		details := fmt.Sprintf("The current Taikun API enum does not expose ClusterIssuer; use a downloaded kubeconfig with kubectl get clusterissuer %s, or add ClusterIssuer support to the Taikun API before using this MCP tool.", issuerName)
		if normalizedKind == "crd" || normalizedKind == "customresourcedefinition" || normalizedKind == "customresourcedefinitions" {
			details = fmt.Sprintf("%s is a ClusterIssuer object name, not a CustomResourceDefinition name. %s", issuerName, details)
		}
		return createJSONResponse(ErrorResponse{
			Error:   "ClusterIssuer describe is not supported by the Taikun Kubernetes resource API",
			Details: details,
		})
	}

	return nil
}

func deleteKubernetesResource(client *taikungoclient.Client, args DeleteKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	kind, ok := normalizeOperationKubernetesKind(args.Kind)
	if !ok {
		return invalidKubernetesKindResponse(args.Kind, "delete-kubernetes-resource", kubernetesOperationKindStrings()), nil
	}

	// Create the action request with name and namespace
	actionRequest := taikuncore.NewKubernetesActionRequest(args.Name)
	if args.Namespace != "" {
		actionRequest.SetNamespace(args.Namespace)
	}

	// Create the delete command
	deleteCmd := taikuncore.NewDeleteKubernetesResourceCommand(args.ProjectID, kind, []taikuncore.KubernetesActionRequest{*actionRequest})

	_, httpResponse, err := client.Client.KubernetesAPI.KubernetesDeleteResource(ctx).
		DeleteKubernetesResourceCommand(*deleteCmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, fmt.Sprintf("delete %s %s", kind, args.Name)); errorResp != nil {
		return errorResp, nil
	}

	namespace := args.Namespace
	if namespace == "" {
		namespace = "default"
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("%s '%s' deleted successfully from namespace '%s'", kind, args.Name, namespace),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}

func patchKubernetesResource(client *taikungoclient.Client, args PatchKubernetesResourceArgs) (*mcp_golang.ToolResponse, error) {
	ctx := context.Background()

	normalizedYaml, err := normalizeYamlInput(args.Yaml)
	if err != nil {
		return createJSONResponse(ErrorResponse{Error: err.Error()}), nil
	}
	if err := validateKubernetesYaml(normalizedYaml); err != nil {
		return createJSONResponse(ErrorResponse{Error: err.Error()}), nil
	}

	encodedYaml := base64.StdEncoding.EncodeToString([]byte(normalizedYaml))
	patchCmd := taikuncore.NewPatchKubernetesResourceCommand(args.ProjectID, encodedYaml, args.Name)
	if args.Namespace != "" {
		patchCmd.SetNamespace(args.Namespace)
	}

	httpResponse, err := client.Client.KubernetesAPI.KubernetesPatchResource(ctx).
		PatchKubernetesResourceCommand(*patchCmd).
		Execute()

	if err != nil {
		return createError(httpResponse, err), nil
	}

	if errorResp := checkResponse(httpResponse, fmt.Sprintf("patch resource %s", args.Name)); errorResp != nil {
		return errorResp, nil
	}

	namespace := args.Namespace
	if namespace == "" {
		namespace = "default"
	}

	successResp := SuccessResponse{
		Message: fmt.Sprintf("Resource '%s' patched successfully in namespace '%s'", args.Name, namespace),
		Success: true,
	}

	return createJSONResponse(successResp), nil
}
