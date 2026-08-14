package main

import (
	"strings"
	"testing"

	taikuncore "github.com/itera-io/taikungoclient/client"
	"github.com/tidwall/gjson"
)

func standaloneVMWithName(name string) taikuncore.StandaloneVmsListForDetailsDto {
	vm := taikuncore.StandaloneVmsListForDetailsDto{}
	vm.SetName(name)
	return vm
}

func TestFilterStandaloneVMsClientSide(t *testing.T) {
	vms := []taikuncore.StandaloneVmsListForDetailsDto{
		standaloneVMWithName("web-01"),
		standaloneVMWithName("web-02"),
		standaloneVMWithName("db-01"),
		standaloneVMWithName("cache-01"),
	}

	tests := []struct {
		name          string
		search        string
		offset        int32
		limit         int32
		expectedNames []string
	}{
		{name: "no filters returns all", expectedNames: []string{"web-01", "web-02", "db-01", "cache-01"}},
		{name: "search matches substring case-insensitively", search: "WEB", expectedNames: []string{"web-01", "web-02"}},
		{name: "search with no match returns empty", search: "nope", expectedNames: nil},
		{name: "offset skips leading items", offset: 2, expectedNames: []string{"db-01", "cache-01"}},
		{name: "offset beyond length returns empty", offset: 10, expectedNames: nil},
		{name: "limit caps results", limit: 2, expectedNames: []string{"web-01", "web-02"}},
		{name: "search then offset then limit compose", search: "0", offset: 1, limit: 2, expectedNames: []string{"web-02", "db-01"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterStandaloneVMsClientSide(vms, tc.search, tc.offset, tc.limit)
			if len(got) != len(tc.expectedNames) {
				t.Fatalf("expected %d VMs %v, got %d %v", len(tc.expectedNames), tc.expectedNames, len(got), namesOf(got))
			}
			for i, want := range tc.expectedNames {
				if got[i].GetName() != want {
					t.Fatalf("index %d: expected %q, got %q (full: %v)", i, want, got[i].GetName(), namesOf(got))
				}
			}
		})
	}
}

func namesOf(vms []taikuncore.StandaloneVmsListForDetailsDto) []string {
	names := make([]string, 0, len(vms))
	for _, vm := range vms {
		names = append(names, vm.GetName())
	}
	return names
}

func TestProjectStatusBlocksCommit(t *testing.T) {
	blocking := []string{"Updating", "updating", "  UPDATING  "}
	for _, status := range blocking {
		if !projectStatusBlocksCommit(status) {
			t.Errorf("expected status %q to block commit", status)
		}
	}

	allowed := []string{"", "Ready", "Failure", "Deploying", "Stopped"}
	for _, status := range allowed {
		if projectStatusBlocksCommit(status) {
			t.Errorf("expected status %q to NOT block commit", status)
		}
	}
}

func TestAuditUserNameFromJSON(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "object with displayName", json: `{"createdBy":{"userId":"abc","displayName":"Adam Skotnický"}}`, want: "Adam Skotnický"},
		{name: "object falls back to userId", json: `{"createdBy":{"userId":"abc","displayName":""}}`, want: "abc"},
		{name: "legacy string value", json: `{"createdBy":"legacy-user"}`, want: "legacy-user"},
		{name: "missing field", json: `{}`, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := auditUserNameFromJSON(gjson.Get(tc.json, "createdBy"))
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestStandaloneVMNotFoundHint(t *testing.T) {
	command := taikuncore.NewCreateStandAloneVmCommand()
	command.SetFlavorName("c02m04")
	command.SetImage("img-guid-123")

	t.Run("flavor not found suggests bind-flavors", func(t *testing.T) {
		info := apiErrorInfo{StatusCode: 404, Message: "Taikun Error: c02m04 not found"}
		hint := standaloneVMNotFoundHint(command, info)
		if hint == "" || !strings.Contains(hint, "bind-flavors-to-project") {
			t.Fatalf("expected bind-flavors hint, got %q", hint)
		}
	})

	t.Run("image not found suggests bind-images", func(t *testing.T) {
		info := apiErrorInfo{StatusCode: 404, Message: "Taikun Error: img-guid-123 not found"}
		hint := standaloneVMNotFoundHint(command, info)
		if hint == "" || !strings.Contains(hint, "bind-images-to-project") {
			t.Fatalf("expected bind-images hint, got %q", hint)
		}
	})

	t.Run("generic not found still hints binding", func(t *testing.T) {
		info := apiErrorInfo{StatusCode: 404, Message: "something not found"}
		hint := standaloneVMNotFoundHint(command, info)
		if hint == "" {
			t.Fatalf("expected a generic binding hint for not-found errors")
		}
	})

	t.Run("non-not-found errors produce no hint", func(t *testing.T) {
		info := apiErrorInfo{StatusCode: 400, Message: "bad request"}
		if hint := standaloneVMNotFoundHint(command, info); hint != "" {
			t.Fatalf("expected no hint for non-not-found error, got %q", hint)
		}
	})
}

// TestToolRequiredScopesNoAccidentalEmpty guards against re-introducing the
// class of bug where a tool is registered with an empty required-scope slice
// and therefore silently reported as "allowed" while the API still 403s. Any
// tool that legitimately needs no scopes must be added to the allowlist below.
func TestToolRequiredScopesNoAccidentalEmpty(t *testing.T) {
	noScopeAllowlist := map[string]struct{}{
		"server-version":                 {},
		"refresh-taikun-client":          {},
		"robot-user-capabilities":        {},
		"mcp-lock":                       {},
		"mcp-lock-status":                {},
		"mcp-lock-clear":                 {},
		"list-kubernetes-resource-kinds": {},
		"describe-payload":               {},
	}

	for tool, scopes := range toolRequiredScopes {
		if len(scopes) > 0 {
			continue
		}
		if _, ok := noScopeAllowlist[tool]; !ok {
			t.Errorf("tool %q has no required scopes; either map it to real scope(s) or add it to the no-scope allowlist", tool)
		}
	}
}

// TestPayloadSchemaRegistryConsistency guards the describe-payload registry so
// aliases and array-type markers can't drift away from the actual command types.
func TestPayloadSchemaRegistryConsistency(t *testing.T) {
	if len(payloadTypeRegistry) == 0 {
		t.Fatal("payloadTypeRegistry is empty; init did not populate command types")
	}

	for tool, typeName := range payloadToolAliases {
		if _, ok := payloadTypeRegistry[typeName]; !ok {
			t.Errorf("tool alias %q -> %q is not a known payload type", tool, typeName)
		}
		if _, ok := toolRequiredScopes[tool]; !ok {
			t.Errorf("tool alias %q is not a registered tool (missing from toolRequiredScopes)", tool)
		}
	}

	for typeName := range payloadArrayTypes {
		if _, ok := payloadTypeRegistry[typeName]; !ok {
			t.Errorf("array payload %q is not a known payload type", typeName)
		}
	}
}

func TestDescribePayloadByToolAndType(t *testing.T) {
	// By tool alias.
	resp := decodeDescribePayload(t, DescribePayloadArgs{Name: "create-standalone-vm"})
	if !resp.Success || resp.Payload != "CreateStandAloneVmCommand" || resp.Tool != "create-standalone-vm" {
		t.Fatalf("unexpected response for tool alias: %+v", resp)
	}
	if len(resp.Fields) == 0 {
		t.Fatal("expected fields for CreateStandAloneVmCommand")
	}

	// Nullable fields should be flagged as nullable+optional and unwrapped.
	var imageField *PayloadFieldSchema
	for i := range resp.Fields {
		if resp.Fields[i].Field == "image" {
			imageField = &resp.Fields[i]
		}
	}
	if imageField == nil {
		t.Fatal("expected an 'image' field")
	}
	if !imageField.Nullable || !imageField.Optional || imageField.Type != "string" {
		t.Fatalf("expected image to be nullable optional string, got %+v", *imageField)
	}

	// By command type name directly.
	byType := decodeDescribePayload(t, DescribePayloadArgs{Name: "CreateStandAloneVmCommand"})
	if !byType.Success || byType.Payload != "CreateStandAloneVmCommand" {
		t.Fatalf("unexpected response for type name: %+v", byType)
	}
}

func TestDescribePayloadArrayType(t *testing.T) {
	resp := decodeDescribePayload(t, DescribePayloadArgs{Name: "assign-alerting-emails"})
	if !resp.Success || !resp.IsArray {
		t.Fatalf("expected array payload for assign-alerting-emails, got %+v", resp)
	}
	if resp.Note == "" {
		t.Fatal("expected a note explaining the payload is a JSON array")
	}
}

func TestDescribePayloadUnknownAndList(t *testing.T) {
	// Unknown returns an error response (Success false / no Payload).
	unknown := decodeDescribePayload(t, DescribePayloadArgs{Name: "does-not-exist"})
	if unknown.Success {
		t.Fatalf("expected failure for unknown payload, got %+v", unknown)
	}

	// Empty name lists all payloads.
	listResp, err := describePayload(DescribePayloadArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := decodeToolResponseJSON[PayloadListResponse](t, listResp)
	if !parsed.Success || parsed.Total == 0 || len(parsed.Payloads) == 0 {
		t.Fatalf("expected a non-empty payload list, got %+v", parsed)
	}
}

func decodeDescribePayload(t *testing.T, args DescribePayloadArgs) PayloadSchemaResponse {
	t.Helper()
	resp, err := describePayload(args)
	if err != nil {
		t.Fatalf("describePayload returned error: %v", err)
	}
	return decodeToolResponseJSON[PayloadSchemaResponse](t, resp)
}
