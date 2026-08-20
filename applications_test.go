package main

import (
	"strings"
	"testing"
)

func TestResolveInstallAppTimeoutDefaultsToTenMinutes(t *testing.T) {
	timeout, defaulted := resolveInstallAppTimeout(0)
	if timeout != defaultInstallAppTimeoutSeconds {
		t.Fatalf("expected default install timeout %d, got %d", defaultInstallAppTimeoutSeconds, timeout)
	}
	if !defaulted {
		t.Fatalf("expected timeout to be marked defaulted")
	}
}

func TestResolveInstallAppTimeoutPreservesExplicitValue(t *testing.T) {
	timeout, defaulted := resolveInstallAppTimeout(900)
	if timeout != 900 {
		t.Fatalf("expected explicit install timeout 900, got %d", timeout)
	}
	if defaulted {
		t.Fatalf("expected explicit timeout to not be marked defaulted")
	}
}

func TestResolveInstallAppTTLDefaultsToTenMinutes(t *testing.T) {
	ttl, defaulted, validationError := resolveInstallAppTTL(0)
	if ttl != defaultInstallAppTTLMinutes {
		t.Fatalf("expected default install ttl %d, got %d", defaultInstallAppTTLMinutes, ttl)
	}
	if !defaulted {
		t.Fatalf("expected ttl to be marked defaulted")
	}
	if validationError != "" {
		t.Fatalf("expected no ttl validation error, got %q", validationError)
	}
}

func TestResolveInstallAppTTLPreservesExplicitValue(t *testing.T) {
	ttl, defaulted, validationError := resolveInstallAppTTL(30)
	if ttl != 30 {
		t.Fatalf("expected explicit install ttl 30, got %d", ttl)
	}
	if defaulted {
		t.Fatalf("expected explicit ttl to not be marked defaulted")
	}
	if validationError != "" {
		t.Fatalf("expected no ttl validation error, got %q", validationError)
	}
}

func TestResolveInstallAppTTLRejectsOutOfRangeValue(t *testing.T) {
	ttl, defaulted, validationError := resolveInstallAppTTL(5)
	if ttl != 0 {
		t.Fatalf("expected rejected ttl to return 0, got %d", ttl)
	}
	if defaulted {
		t.Fatalf("expected rejected ttl to not be marked defaulted")
	}
	if validationError == "" {
		t.Fatalf("expected ttl validation error for out of range value")
	}
}

func TestKubeAppProjectPrerequisiteErrorBlocksNonReadyProject(t *testing.T) {
	errResp := kubeAppProjectPrerequisiteError(336, "Updating", "Healthy", true, "app install")
	if errResp == nil {
		t.Fatalf("expected non-ready project to be blocked")
	}
	if !strings.Contains(errResp.Error, "not ready") || !strings.Contains(errResp.Details, "Wait for project Ready") {
		t.Fatalf("expected readiness guidance, got %+v", errResp)
	}
}

func TestKubeAppProjectPrerequisiteErrorBlocksUnhealthyProject(t *testing.T) {
	errResp := kubeAppProjectPrerequisiteError(336, "Ready", "Unhealthy", true, "app install")
	if errResp == nil {
		t.Fatalf("expected unhealthy project to be blocked")
	}
	if !strings.Contains(errResp.Error, "health is not ready") || !strings.Contains(errResp.Details, "Resolve cluster health") {
		t.Fatalf("expected health guidance, got %+v", errResp)
	}
}

func TestKubeAppProjectPrerequisiteErrorBlocksUnknownHealth(t *testing.T) {
	errResp := kubeAppProjectPrerequisiteError(336, "Ready", "", true, "app install")
	if errResp == nil {
		t.Fatalf("expected empty health to be blocked")
	}
}

func TestKubeAppProjectPrerequisiteErrorBlocksMissingKubeconfig(t *testing.T) {
	errResp := kubeAppProjectPrerequisiteError(336, "Ready", "Healthy", false, "catalog binding")
	if errResp == nil {
		t.Fatalf("expected project without kubeconfig to be blocked")
	}
	if !strings.Contains(errResp.Error, "no admin kubeconfig") || !strings.Contains(errResp.Details, "preflight-project") {
		t.Fatalf("expected kubeconfig guidance, got %+v", errResp)
	}
}

func TestKubeAppProjectPrerequisiteErrorAllowsReadyProjectWithKubeconfig(t *testing.T) {
	if errResp := kubeAppProjectPrerequisiteError(336, "Ready", "Warning", true, "app install"); errResp != nil {
		t.Fatalf("expected ready project with kubeconfig to pass, got %+v", errResp)
	}
}
