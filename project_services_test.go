package main

import (
	"strings"
	"testing"

	taikuncore "github.com/itera-io/taikungoclient/client"
)

func TestValidateProjectSpotAvailabilityAllowsSupportedClouds(t *testing.T) {
	for _, cloudType := range []string{"AWS", "AZURE", "GOOGLE"} {
		if response := validateProjectSpotAvailability(953, cloudType); response != nil {
			t.Fatalf("expected %s to allow spot, got %+v", cloudType, response)
		}
	}
}

func TestValidateProjectSpotAvailabilityRejectsUnsupportedClouds(t *testing.T) {
	response := validateProjectSpotAvailability(77, "OPENSTACK")
	result := decodeToolResponseJSON[ErrorResponse](t, response)

	if !strings.Contains(result.Error, "only for AWS, Azure, and GCP") {
		t.Fatalf("expected clear supported-cloud guidance, got %+v", result)
	}
	if !strings.Contains(result.Error, `project 77 uses cloudType "OPENSTACK"`) {
		t.Fatalf("expected project cloud type in validation error, got %+v", result)
	}
}

func TestBuildProjectServiceStatusResponseIncludesBindingsAndToggles(t *testing.T) {
	project := taikuncore.NewProjectDetailsForServersDtoWithDefaults()
	project.Id = 953
	project.Name = "minio-bkup-8apr2"
	project.Status = taikuncore.PROJECTSTATUS_READY
	project.Health = taikuncore.PROJECTHEALTH_HEALTHY
	project.CloudType = taikuncore.ECLOUDCREDENTIALTYPE_AWS
	project.HasKubeConfigFile = true
	project.IsAutoscalingEnabled = true
	project.IsAutoscalingSpotEnabled = true
	project.IsMonitoringEnabled = true
	project.IsBackupEnabled = true
	project.AiEnabled = true
	project.IsOpaEnabled = true
	project.AllowFullSpotKubernetes = true
	project.AllowSpotWorkers = true
	project.AllowSpotVMs = false
	project.AlertingProfileName = "alerts-default"
	project.OpaProfileName = "policy-default"
	project.SetAlertingProfileId(11)
	project.SetS3CredentialId(164)
	project.SetAiCredentialId(33)
	project.SetOpaProfileId(44)
	project.SetMinSize(1)
	project.SetMaxSize(3)
	project.SetDiskSize(40)
	project.SetFlavor("m5.large")
	project.SetMaxSpotPrice(0.42)
	project.AccessIp = "203.0.113.10"

	response := buildProjectServiceStatusResponse(*project)

	if response.ProjectID != 953 || response.ProjectName != "minio-bkup-8apr2" {
		t.Fatalf("expected project identity fields, got %+v", response)
	}
	if response.AccessIP != "203.0.113.10" {
		t.Fatalf("expected accessIp to be included, got %+v", response.AccessIP)
	}
	if !response.HasKubeconfigFile {
		t.Fatalf("expected kubeconfig flag to be true, got %+v", response)
	}
	if !response.Autoscaling.Enabled || !response.Autoscaling.SpotEnabled {
		t.Fatalf("expected autoscaling flags to be true, got %+v", response.Autoscaling)
	}
	if response.Autoscaling.MinSize == nil || *response.Autoscaling.MinSize != 1 {
		t.Fatalf("expected minSize=1, got %+v", response.Autoscaling)
	}
	if response.Autoscaling.MaxSize == nil || *response.Autoscaling.MaxSize != 3 {
		t.Fatalf("expected maxSize=3, got %+v", response.Autoscaling)
	}
	if response.Backup.CredentialID == nil || *response.Backup.CredentialID != 164 {
		t.Fatalf("expected backup credential binding, got %+v", response.Backup)
	}
	if response.AIAssistant.CredentialID == nil || *response.AIAssistant.CredentialID != 33 {
		t.Fatalf("expected AI credential binding, got %+v", response.AIAssistant)
	}
	if response.Policy.ProfileID == nil || *response.Policy.ProfileID != 44 {
		t.Fatalf("expected policy profile binding, got %+v", response.Policy)
	}
	if !response.Spot.Available || !response.Spot.FullEnabled || !response.Spot.Workers || response.Spot.VMs {
		t.Fatalf("expected spot status to reflect AWS project flags, got %+v", response.Spot)
	}
}
