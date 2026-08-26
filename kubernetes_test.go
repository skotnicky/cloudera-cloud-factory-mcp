package main

import (
	"slices"
	"strings"
	"testing"

	taikuncore "github.com/itera-io/taikungoclient/client"
)

func TestNormalizeKubeConfigRoleIDDefaultsAWSProjectsToRoleOne(t *testing.T) {
	roleID, errorResp := normalizeKubeConfigRoleID(953, "AWS", 0)
	if errorResp != nil {
		t.Fatalf("expected AWS kubeconfig role defaulting to succeed, got %+v", errorResp)
	}
	if roleID != awsKubeConfigRoleID {
		t.Fatalf("expected AWS projects to default to kubeConfigRoleId %d, got %d", awsKubeConfigRoleID, roleID)
	}
}

func TestNormalizeKubeConfigRoleIDRejectsNonDefaultAWSRole(t *testing.T) {
	_, response := normalizeKubeConfigRoleID(953, "AWS", 2)
	result := decodeToolResponseJSON[ErrorResponse](t, response)

	if !strings.Contains(result.Error, "only supports kubeConfigRoleId 1") {
		t.Fatalf("expected clear AWS kubeconfig role validation error, got %+v", result)
	}
	if !strings.Contains(result.Details, `cloudType "AWS"`) {
		t.Fatalf("expected AWS remediation guidance in details, got %+v", result)
	}
}

func TestNormalizeKubeConfigRoleIDPreservesNonAWSRoleSelection(t *testing.T) {
	roleID, errorResp := normalizeKubeConfigRoleID(77, "OpenStack", 7)
	if errorResp != nil {
		t.Fatalf("expected non-AWS kubeconfig role selection to pass through, got %+v", errorResp)
	}
	if roleID != 7 {
		t.Fatalf("expected non-AWS role selection to be preserved, got %d", roleID)
	}
}

func TestNormalizeKubeConfigRoleIDDefaultsNonAWSToClusterAdmin(t *testing.T) {
	roleID, errorResp := normalizeKubeConfigRoleID(77, "OpenStack", 0)
	if errorResp != nil {
		t.Fatalf("expected non-AWS kubeconfig role defaulting to succeed, got %+v", errorResp)
	}
	if roleID != defaultKubeConfigRoleID {
		t.Fatalf("expected non-AWS projects to default to cluster-admin role %d, got %d", defaultKubeConfigRoleID, roleID)
	}
}

func TestNormalizeKubeConfigRoleIDAllowsExplicitViewRoleOnNonAWS(t *testing.T) {
	roleID, errorResp := normalizeKubeConfigRoleID(77, "OpenStack", viewKubeConfigRoleID)
	if errorResp != nil {
		t.Fatalf("expected explicit view role on non-AWS to succeed, got %+v", errorResp)
	}
	if roleID != viewKubeConfigRoleID {
		t.Fatalf("expected view role %d, got %d", viewKubeConfigRoleID, roleID)
	}
}

func TestSelectDownloadableKubeConfigPrefersClusterAdmin(t *testing.T) {
	items := []taikuncore.KubeConfigForUserDto{
		{Id: 10, ProjectId: 77, KubeConfigRoleName: "view", CanDownload: true},
		{Id: 11, ProjectId: 77, KubeConfigRoleName: "cluster-admin", CanDownload: true},
	}
	selection, errResp := selectDownloadableKubeConfig(items, 77, 0)
	if errResp != nil {
		t.Fatalf("expected selection to succeed, got %+v", errResp)
	}
	if selection.ID != 11 || selection.KubeConfigRoleName != "cluster-admin" {
		t.Fatalf("expected cluster-admin kubeconfig 11, got %+v", selection)
	}
}

func TestSelectDownloadableKubeConfigUsesRequestedID(t *testing.T) {
	items := []taikuncore.KubeConfigForUserDto{
		{Id: 10, ProjectId: 77, KubeConfigRoleName: "view", CanDownload: true},
		{Id: 11, ProjectId: 77, KubeConfigRoleName: "cluster-admin", CanDownload: true},
	}
	selection, errResp := selectDownloadableKubeConfig(items, 77, 10)
	if errResp != nil {
		t.Fatalf("expected selection to succeed, got %+v", errResp)
	}
	if selection.ID != 10 || selection.KubeConfigRoleName != "view" {
		t.Fatalf("expected requested view kubeconfig 10, got %+v", selection)
	}
}

func TestSelectDownloadableKubeConfigRejectsNonDownloadableRequestedID(t *testing.T) {
	items := []taikuncore.KubeConfigForUserDto{
		{Id: 10, ProjectId: 77, KubeConfigRoleName: "view", CanDownload: false},
	}
	_, errResp := selectDownloadableKubeConfig(items, 77, 10)
	if errResp == nil {
		t.Fatalf("expected non-downloadable kubeconfig to be rejected")
	}
	if !strings.Contains(errResp.Error, "not downloadable") {
		t.Fatalf("expected downloadability error, got %+v", errResp)
	}
}

func TestKubeConfigRoleMappings(t *testing.T) {
	if got := kubeConfigRoleIDFromName("cluster-admin"); got != 1 {
		t.Fatalf("expected cluster-admin to map to 1, got %d", got)
	}
	if got := kubeConfigRoleNameFromID(4); got != "view" {
		t.Fatalf("expected role 4 to map to view, got %q", got)
	}
}

func TestParseInt32RejectsOutOfRangeValues(t *testing.T) {
	if got := parseInt32("2147483647"); got != 2147483647 {
		t.Fatalf("expected max int32 to parse, got %d", got)
	}
	if got := parseInt32("2147483648"); got != 0 {
		t.Fatalf("expected out-of-range int32 parse to return 0, got %d", got)
	}
}

func TestParseReadyCountsRejectsOutOfRangeValues(t *testing.T) {
	ready, total := parseReadyCounts("12/2147483648")
	if ready != 0 || total != 0 {
		t.Fatalf("expected out-of-range ready counts to return 0/0, got %d/%d", ready, total)
	}
}

func TestNormalizeListKubernetesKindCaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "pods", expected: "Pods"},
		{input: "PODS", expected: "Pods"},
		{input: "sts", expected: "Sts"},
	}

	for _, tt := range tests {
		got, ok := normalizeListKubernetesKind(tt.input)
		if !ok {
			t.Fatalf("expected %q to normalize successfully", tt.input)
		}
		if got != tt.expected {
			t.Fatalf("expected %q to normalize to %q, got %q", tt.input, tt.expected, got)
		}
	}
}

func TestNormalizeOperationKubernetesKindCaseInsensitive(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "POD", expected: "Pod"},
		{input: "deployment", expected: "Deployment"},
		{input: "service", expected: "Service"},
	}

	for _, tt := range tests {
		got, ok := normalizeOperationKubernetesKind(tt.input)
		if !ok {
			t.Fatalf("expected %q to normalize successfully", tt.input)
		}
		if string(got) != tt.expected {
			t.Fatalf("expected %q to normalize to %q, got %q", tt.input, tt.expected, got)
		}
	}
}

func TestListKubernetesResourceKindsResponse(t *testing.T) {
	result := decodeToolResponseJSON[KubernetesResourceKindsResponse](t, listKubernetesResourceKinds())
	if !result.Success {
		t.Fatalf("expected success response, got %+v", result)
	}
	if !result.CaseInsensitive {
		t.Fatalf("expected resource kind discovery to mark matching as case-insensitive")
	}
	if !slices.Contains(result.ListKinds, "Pods") {
		t.Fatalf("expected listKinds to include Pods, got %+v", result.ListKinds)
	}
	if !slices.Contains(result.OperationKinds, "Pod") {
		t.Fatalf("expected operationKinds to include Pod, got %+v", result.OperationKinds)
	}
	foundUnavailable := false
	for _, kind := range result.UnavailableListKinds {
		if kind.Kind == "CronJobs" && strings.Contains(kind.Reason, "not available") {
			foundUnavailable = true
			break
		}
	}
	if !foundUnavailable {
		t.Fatalf("expected unavailable list kinds to include CronJobs, got %+v", result.UnavailableListKinds)
	}
}

func TestListKubernetesResourcesInvalidKindIncludesAllowedKinds(t *testing.T) {
	response, err := listKubernetesResources(nil, ListKubernetesResourcesArgs{
		ProjectID: 1,
		Kind:      "NotARealKind",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "Invalid resource kind") {
		t.Fatalf("expected invalid resource kind error, got %+v", result)
	}
	if !strings.Contains(result.Details, "Pods") || !strings.Contains(result.Details, "Sts") {
		t.Fatalf("expected allowed list kinds in details, got %+v", result)
	}
}

func TestDescribeClusterIssuerReturnsActionableUnsupportedError(t *testing.T) {
	response, err := describeKubernetesResource(nil, DescribeKubernetesResourceArgs{
		ProjectID: 1,
		Kind:      "ClusterIssuer",
		Name:      "ccf-default",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Error, "ClusterIssuer describe is not supported") {
		t.Fatalf("expected ClusterIssuer unsupported error, got %+v", result)
	}
	if !strings.Contains(result.Details, "kubectl get clusterissuer ccf-default") {
		t.Fatalf("expected kubectl remediation hint, got %+v", result)
	}
}

func TestDescribeCrdCcfDefaultReturnsClusterIssuerHint(t *testing.T) {
	response, err := describeKubernetesResource(nil, DescribeKubernetesResourceArgs{
		ProjectID: 1,
		Kind:      "Crd",
		Name:      "ccf-default",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	result := decodeToolResponseJSON[ErrorResponse](t, response)
	if !strings.Contains(result.Details, "not a CustomResourceDefinition name") {
		t.Fatalf("expected CRD-vs-ClusterIssuer hint, got %+v", result)
	}
}

func TestKubernetesListEndpointsUsesPodsCompatibilityPaths(t *testing.T) {
	endpoints := kubernetesListEndpoints("https://api.example.com", 1832, "pods")
	if len(endpoints) != 2 {
		t.Fatalf("expected two pods endpoints for compatibility fallback, got %d (%v)", len(endpoints), endpoints)
	}
	if endpoints[0] != "https://api.example.com/api/v1/kubernetes/1832/pods-list" {
		t.Fatalf("expected pods-list endpoint first, got %q", endpoints[0])
	}
	if endpoints[1] != "https://api.example.com/api/v1/kubernetes/list/1832/pods" {
		t.Fatalf("expected legacy pods list endpoint second, got %q", endpoints[1])
	}
}

func TestKubernetesListEndpointsKeepsLegacyPathForNonPods(t *testing.T) {
	endpoints := kubernetesListEndpoints("https://api.example.com", 77, "service")
	if len(endpoints) != 1 {
		t.Fatalf("expected one endpoint for non-pods resources, got %d (%v)", len(endpoints), endpoints)
	}
	if endpoints[0] != "https://api.example.com/api/v1/kubernetes/list/77/service" {
		t.Fatalf("expected legacy non-pods endpoint, got %q", endpoints[0])
	}
}
