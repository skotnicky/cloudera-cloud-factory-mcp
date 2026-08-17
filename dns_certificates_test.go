package main

import "testing"

func TestNormalizeDNSCertStatusProjectStatusNumericEnum(t *testing.T) {
	raw := map[string]interface{}{
		"projectStatus": float64(800),
		"synced":        false,
	}

	normalizeDNSCertStatusProjectStatus(raw)

	if raw["projectStatus"] != "800" {
		t.Fatalf("expected numeric projectStatus to normalize to string, got %+v", raw["projectStatus"])
	}
}

func TestNormalizeDNSCertStatusProjectStatusStringUnchanged(t *testing.T) {
	raw := map[string]interface{}{
		"projectStatus": "Ready",
	}

	normalizeDNSCertStatusProjectStatus(raw)

	if raw["projectStatus"] != "Ready" {
		t.Fatalf("expected string projectStatus to remain unchanged, got %+v", raw["projectStatus"])
	}
}
