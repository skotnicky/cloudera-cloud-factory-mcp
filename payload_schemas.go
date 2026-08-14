package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

// This file makes the JSON payloads that many tools accept self-documenting from
// inside MCP, so agents no longer need to inspect the taikungoclient DTOs or the
// Swagger spec out-of-band to learn what a payload should contain. The
// describe-payload tool reflects over the concrete command struct a tool decodes
// and reports its fields, types, optionality, nullability, nested objects, and an
// example skeleton.

// DescribePayloadArgs is the input for the describe-payload tool.
type DescribePayloadArgs struct {
	Name string `json:"name,omitempty" jsonschema:"description=Tool name (e.g. create-standalone-vm) or command type name (e.g. CreateStandAloneVmCommand). Omit to list every known payload schema."`
}

// PayloadFieldSchema describes a single field of a payload command.
type PayloadFieldSchema struct {
	Field    string               `json:"field"`
	Type     string               `json:"type"`
	Optional bool                 `json:"optional,omitempty"`
	Nullable bool                 `json:"nullable,omitempty"`
	Fields   []PayloadFieldSchema `json:"fields,omitempty"`
}

// PayloadSchemaResponse is returned when describing a single payload.
type PayloadSchemaResponse struct {
	Payload         string               `json:"payload"`
	Tool            string               `json:"tool,omitempty"`
	IsArray         bool                 `json:"isArray,omitempty"`
	Fields          []PayloadFieldSchema `json:"fields"`
	ExampleSkeleton interface{}          `json:"exampleSkeleton,omitempty"`
	Note            string               `json:"note,omitempty"`
	Message         string               `json:"message"`
	Success         bool                 `json:"success"`
}

// PayloadListResponse is returned when listing all known payloads.
type PayloadListResponse struct {
	Payloads    []string          `json:"payloads"`
	ToolAliases map[string]string `json:"toolAliases"`
	Total       int               `json:"total"`
	Message     string            `json:"message"`
	Success     bool              `json:"success"`
}

// payloadSamples is the flat set of command structs that payload-bearing tools
// decode. reflect.TypeOf(sample) yields the type used to build the schema. One
// entry per distinct command type; the source of truth is the decodePayload[...]
// call sites across the codebase.
var payloadSamples = []interface{}{
	// Images
	taikuncore.AwsImagesPostListCommand{},
	taikuncore.ImageByIdCommand{},
	taikuncore.BindImageToProjectCommand{},
	taikuncore.DeleteImageFromProjectCommand{},
	// Security groups (standalone profile SG)
	taikuncore.CreateSecurityGroupCommand{},
	taikuncore.EditSecurityGroupCommand{},
	// Alerting
	taikuncore.CreateAlertingProfileCommand{},
	taikuncore.UpdateAlertingProfileCommand{},
	taikuncore.AttachDetachAlertingProfileCommand{},
	taikuncore.VerifyWebhookCommand{},
	taikuncore.CreateAlertingIntegrationCommand{},
	taikuncore.EditAlertingIntegrationCommand{},
	taikuncore.AlertingEmailDto{},
	taikuncore.AlertingWebhookDto{},
	// Access profiles
	taikuncore.CreateAccessProfileCommand{},
	taikuncore.UpdateAccessProfileDto{},
	// Access profile sub-resources
	taikuncore.CreateDnsServerCommand{},
	taikuncore.DnsNtpAddressEditDto{},
	taikuncore.CreateNtpServerCommand{},
	taikuncore.CreateSshUserCommand{},
	taikuncore.EditSshUserCommand{},
	taikuncore.DeleteSshUserCommand{},
	taikuncore.CreateTrustedRegistriesCommand{},
	taikuncore.TrustedRegistryEditDto{},
	// DNS credentials + certificate service
	taikuncore.DnsCredentialCreateCommand{},
	taikuncore.DnsCredentialUpdateCommand{},
	taikuncore.DnsCredentialMakeDefaultCommand{},
	taikuncore.DnsCredentialLockCommand{},
	taikuncore.AttachDetachDnsCredentialCommand{},
	taikuncore.ValidateDnsCertCommand{},
	taikuncore.EnableDnsCertCommand{},
	taikuncore.DisableDnsCertCommand{},
	taikuncore.DnsCertSyncCommand{},
	// Certificate profiles
	taikuncore.CertificateProfileCreateCommand{},
	taikuncore.CertificateProfileUpdateCommand{},
	taikuncore.CertificateProfileMakeDefaultCommand{},
	taikuncore.CertificateProfileLockCommand{},
	taikuncore.CertificateProfileValidateCommand{},
	// AI credentials
	taikuncore.CreateAiCredentialCommand{},
	// Autoscaling
	taikuncore.EnableAutoscalingCommand{},
	taikuncore.EditAutoscalingCommand{},
	taikuncore.DisableAutoscalingCommand{},
	// Standalone VMs
	taikuncore.CreateStandAloneVmCommand{},
	taikuncore.UpdateStandAloneVmFlavorCommand{},
	taikuncore.StandAloneVmIpManagementCommand{},
	taikuncore.ResetStandAloneVmStatusCommand{},
	taikuncore.VmConsoleScreenshotCommand{},
	taikuncore.RebootStandAloneVmCommand{},
	taikuncore.ShelveStandAloneVmCommand{},
	taikuncore.StartStandaloneVmCommand{},
	taikuncore.StopStandaloneVmCommand{},
	taikuncore.UnshelveStandaloneVmCommand{},
	taikuncore.CreateStandAloneDiskCommand{},
	taikuncore.UpdateStandaloneVmDiskSizeCommand{},
	taikuncore.StandAloneProfileCreateCommand{},
	taikuncore.StandAloneProfileUpdateCommand{},
	// Backup
	taikuncore.BackupCredentialsCreateCommand{},
	taikuncore.BackupCredentialsUpdateCommand{},
	taikuncore.CreateBackupPolicyCommand{},
	taikuncore.DeleteBackupCommand{},
	taikuncore.DeleteBackupStorageLocationCommand{},
	taikuncore.DeleteRestoreCommand{},
	taikuncore.DeleteScheduleCommand{},
	taikuncore.ImportBackupStorageLocationCommand{},
	taikuncore.RestoreBackupCommand{},
	// OPA profiles
	taikuncore.CreateOpaProfileCommand{},
	taikuncore.OpaProfileUpdateCommand{},
	taikuncore.OpaProfileSyncCommand{},
	taikuncore.OpaMakeDefaultCommand{},
	// Cloud credentials (polymorphic by cloud type)
	taikuncore.CreateAwsCloudCommand{},
	taikuncore.UpdateAwsCommand{},
	taikuncore.CreateAzureCloudCommand{},
	taikuncore.UpdateAzureCommand{},
	taikuncore.CreateOpenstackCloudCommand{},
	taikuncore.UpdateOpenStackCommand{},
	// Kubernetes profiles
	taikuncore.CreateKubernetesProfileCommand{},
	// Identity/admin
	taikuncore.UpdateOrganizationCommand{},
	taikuncore.UpdateGroupDto{},
	taikuncore.UpdateGroupOrganizationDto{},
	taikuncore.DeleteOrganizationFromGroupCommand{},
	taikuncore.DeleteUserFromGroupCommand{},
	taikuncore.UpdateUserCommand{},
	taikuncore.CreateGroupOrganizationDto{},
	taikuncore.CreateGroupUserDto{},
}

// payloadArrayTypes lists command types that are sent as a JSON array of the
// named object rather than a single object.
var payloadArrayTypes = map[string]struct{}{
	"AlertingEmailDto":           {},
	"AlertingWebhookDto":         {},
	"CreateGroupOrganizationDto": {},
	"CreateGroupUserDto":         {},
}

// payloadToolAliases maps a tool name to the command type name it decodes, so
// describe-payload can be queried by the friendlier tool name. Polymorphic tools
// (e.g. create-cloud-credential, which dispatches on cloudType) are intentionally
// omitted; query their concrete command types directly.
var payloadToolAliases = map[string]string{
	"create-standalone-vm":               "CreateStandAloneVmCommand",
	"update-standalone-vm-flavor":        "UpdateStandAloneVmFlavorCommand",
	"manage-standalone-vm-ip":            "StandAloneVmIpManagementCommand",
	"reset-standalone-vm-status":         "ResetStandAloneVmStatusCommand",
	"get-standalone-vm-console":          "VmConsoleScreenshotCommand",
	"reboot-standalone-vm":               "RebootStandAloneVmCommand",
	"shelve-standalone-vm":               "ShelveStandAloneVmCommand",
	"unshelve-standalone-vm":             "UnshelveStandaloneVmCommand",
	"start-standalone-vm":                "StartStandaloneVmCommand",
	"stop-standalone-vm":                 "StopStandaloneVmCommand",
	"create-standalone-vm-disk":          "CreateStandAloneDiskCommand",
	"resize-standalone-vm-disk":          "UpdateStandaloneVmDiskSizeCommand",
	"create-standalone-profile":          "StandAloneProfileCreateCommand",
	"update-standalone-profile":          "StandAloneProfileUpdateCommand",
	"create-standalone-profile-sg":       "CreateSecurityGroupCommand",
	"update-standalone-profile-sg":       "EditSecurityGroupCommand",
	"create-dns-credential":              "DnsCredentialCreateCommand",
	"update-dns-credential":              "DnsCredentialUpdateCommand",
	"make-dns-credential-default":        "DnsCredentialMakeDefaultCommand",
	"lock-dns-credential":                "DnsCredentialLockCommand",
	"attach-dns-credential-to-project":   "AttachDetachDnsCredentialCommand",
	"detach-dns-credential-from-project": "AttachDetachDnsCredentialCommand",
	"validate-dns-credential":            "ValidateDnsCertCommand",
	"enable-dns-cert":                    "EnableDnsCertCommand",
	"disable-dns-cert":                   "DisableDnsCertCommand",
	"sync-dns-cert":                      "DnsCertSyncCommand",
	"validate-dns-cert":                  "ValidateDnsCertCommand",
	"create-certificate-authority":       "CertificateProfileCreateCommand",
	"update-certificate-authority":       "CertificateProfileUpdateCommand",
	"make-certificate-authority-default": "CertificateProfileMakeDefaultCommand",
	"lock-certificate-authority":         "CertificateProfileLockCommand",
	"validate-certificate-authority":     "CertificateProfileValidateCommand",
	"create-dns-server":                  "CreateDnsServerCommand",
	"edit-dns-server":                    "DnsNtpAddressEditDto",
	"create-ntp-server":                  "CreateNtpServerCommand",
	"edit-ntp-server":                    "DnsNtpAddressEditDto",
	"create-ssh-user":                    "CreateSshUserCommand",
	"edit-ssh-user":                      "EditSshUserCommand",
	"delete-ssh-user":                    "DeleteSshUserCommand",
	"create-trusted-registry":            "CreateTrustedRegistriesCommand",
	"edit-trusted-registry":              "TrustedRegistryEditDto",
	"create-access-profile":              "CreateAccessProfileCommand",
	"update-access-profile":              "UpdateAccessProfileDto",
	"create-alerting-profile":            "CreateAlertingProfileCommand",
	"update-alerting-profile":            "UpdateAlertingProfileCommand",
	"attach-alerting-profile":            "AttachDetachAlertingProfileCommand",
	"detach-alerting-profile":            "AttachDetachAlertingProfileCommand",
	"create-alerting-integration":        "CreateAlertingIntegrationCommand",
	"update-alerting-integration":        "EditAlertingIntegrationCommand",
	"verify-alerting-webhook":            "VerifyWebhookCommand",
	"assign-alerting-emails":             "AlertingEmailDto",
	"assign-alerting-webhooks":           "AlertingWebhookDto",
	"create-ai-credential":               "CreateAiCredentialCommand",
	"enable-autoscaling":                 "EnableAutoscalingCommand",
	"update-autoscaling":                 "EditAutoscalingCommand",
	"disable-autoscaling":                "DisableAutoscalingCommand",
	"create-opa-profile":                 "CreateOpaProfileCommand",
	"update-opa-profile":                 "OpaProfileUpdateCommand",
	"sync-opa-profile":                   "OpaProfileSyncCommand",
	"make-opa-profile-default":           "OpaMakeDefaultCommand",
	"create-kubernetes-profile":          "CreateKubernetesProfileCommand",
	"create-backup-credential":           "BackupCredentialsCreateCommand",
	"update-backup-credential":           "BackupCredentialsUpdateCommand",
	"create-backup-policy":               "CreateBackupPolicyCommand",
	"delete-backup":                      "DeleteBackupCommand",
	"delete-backup-storage-location":     "DeleteBackupStorageLocationCommand",
	"delete-restore":                     "DeleteRestoreCommand",
	"delete-schedule":                    "DeleteScheduleCommand",
	"import-backup-storage-location":     "ImportBackupStorageLocationCommand",
	"restore-backup":                     "RestoreBackupCommand",
	"bind-images-to-project":             "BindImageToProjectCommand",
	"unbind-images-from-project":         "DeleteImageFromProjectCommand",
	"get-image-details":                  "ImageByIdCommand",
}

var payloadTypeRegistry map[string]reflect.Type

func init() {
	payloadTypeRegistry = make(map[string]reflect.Type, len(payloadSamples))
	for _, sample := range payloadSamples {
		t := reflect.TypeOf(sample)
		if t == nil {
			continue
		}
		payloadTypeRegistry[t.Name()] = t
	}
}

func describePayload(args DescribePayloadArgs) (*mcp_golang.ToolResponse, error) {
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return listPayloadSchemas(), nil
	}

	toolName := ""
	typeName := name
	if aliased, ok := payloadToolAliases[name]; ok {
		toolName = name
		typeName = aliased
	}

	t, ok := payloadTypeRegistry[typeName]
	if !ok {
		return createJSONResponse(ErrorResponse{
			Error:   fmt.Sprintf("Unknown payload %q", name),
			Details: "Pass a known command type name or a tool name. Call describe-payload with no arguments to list all known payloads and tool aliases.",
		}), nil
	}

	fields := reflectPayloadFields(t, 0)
	_, isArray := payloadArrayTypes[typeName]

	resp := PayloadSchemaResponse{
		Payload:         typeName,
		Tool:            toolName,
		IsArray:         isArray,
		Fields:          fields,
		ExampleSkeleton: examplePayloadSkeleton(t, 0),
		Message:         fmt.Sprintf("Payload schema for %s", typeName),
		Success:         true,
	}
	if isArray {
		resp.Note = "Send this payload as a JSON array of the described object."
		resp.ExampleSkeleton = []interface{}{examplePayloadSkeleton(t, 0)}
	}

	return createJSONResponse(resp), nil
}

func listPayloadSchemas() *mcp_golang.ToolResponse {
	names := make([]string, 0, len(payloadTypeRegistry))
	for name := range payloadTypeRegistry {
		names = append(names, name)
	}
	sort.Strings(names)

	return createJSONResponse(PayloadListResponse{
		Payloads:    names,
		ToolAliases: payloadToolAliases,
		Total:       len(names),
		Message:     fmt.Sprintf("Known payload schemas: %d. Call describe-payload with a tool name (e.g. create-standalone-vm) or a command type name to see its fields.", len(names)),
		Success:     true,
	})
}

const maxPayloadReflectDepth = 5

// reflectPayloadFields walks a struct type and returns a field schema. Pointer
// and taikungoclient Nullable* wrappers are reported as optional/nullable and
// unwrapped to their underlying type; nested structs recurse up to a depth cap.
func reflectPayloadFields(t reflect.Type, depth int) []PayloadFieldSchema {
	if t == nil || t.Kind() != reflect.Struct || depth > maxPayloadReflectDepth {
		return nil
	}

	fields := make([]PayloadFieldSchema, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		jsonName, omitempty, skip := jsonFieldName(f)
		if skip {
			continue
		}

		schema := PayloadFieldSchema{Field: jsonName}
		ft := f.Type

	if ft.Kind() == reflect.Pointer {
		schema.Optional = true
		ft = ft.Elem()
	}

		if nullable, underlying := nullableUnderlying(ft); nullable {
			schema.Nullable = true
			schema.Optional = true
			schema.Type = underlying
			fields = append(fields, schema)
			continue
		}

		if omitempty {
			schema.Optional = true
		}

		schema.Type = friendlyTypeName(ft)
		if ft.Kind() == reflect.Struct && !isTimeType(ft) {
			schema.Fields = reflectPayloadFields(ft, depth+1)
		} else if ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct && !isTimeType(ft.Elem()) {
			schema.Fields = reflectPayloadFields(ft.Elem(), depth+1)
		}

		fields = append(fields, schema)
	}

	return fields
}

func jsonFieldName(f reflect.StructField) (name string, omitempty, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	if name == "" {
		name = f.Name
	}
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

// nullableUnderlying detects taikungoclient Nullable* wrapper types (e.g.
// NullableString) and returns the friendly underlying type name.
func nullableUnderlying(t reflect.Type) (bool, string) {
	if t.Kind() != reflect.Struct {
		return false, ""
	}
	name := t.Name()
	if !strings.HasPrefix(name, "Nullable") {
		return false, ""
	}
	base := strings.TrimPrefix(name, "Nullable")
	switch base {
	case "String":
		return true, "string"
	case "Bool":
		return true, "bool"
	case "Int32":
		return true, "int32"
	case "Int64":
		return true, "int64"
	case "Float32":
		return true, "float32"
	case "Float64":
		return true, "float64"
	case "Time":
		return true, "string (date-time)"
	default:
		return true, strings.ToLower(base)
	}
}

func friendlyTypeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return "array<" + friendlyTypeName(t.Elem()) + ">"
	case reflect.Map:
		return "map<" + friendlyTypeName(t.Key()) + "," + friendlyTypeName(t.Elem()) + ">"
	case reflect.Pointer:
		return friendlyTypeName(t.Elem())
	case reflect.Struct:
		if isTimeType(t) {
			return "string (date-time)"
		}
		if t.Name() != "" {
			return "object (" + t.Name() + ")"
		}
		return "object"
	default:
		return t.Kind().String()
	}
}

func isTimeType(t reflect.Type) bool {
	return t.PkgPath() == "time" && t.Name() == "Time"
}

// examplePayloadSkeleton builds a minimal example value for a type so agents can
// see the JSON shape. Optional/nullable fields are included to show every key.
func examplePayloadSkeleton(t reflect.Type, depth int) interface{} {
	if t == nil || depth > maxPayloadReflectDepth {
		return nil
	}

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if nullable, underlying := nullableUnderlying(t); nullable {
		return exampleScalar(underlying)
	}

	switch t.Kind() {
	case reflect.Struct:
		if isTimeType(t) {
			return ""
		}
		obj := map[string]interface{}{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			jsonName, _, skip := jsonFieldName(f)
			if skip {
				continue
			}
			obj[jsonName] = examplePayloadSkeleton(f.Type, depth+1)
		}
		return obj
	case reflect.Slice, reflect.Array:
		return []interface{}{examplePayloadSkeleton(t.Elem(), depth+1)}
	case reflect.Map:
		return map[string]interface{}{}
	case reflect.String:
		return ""
	case reflect.Bool:
		return false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 0
	case reflect.Float32, reflect.Float64:
		return 0.0
	default:
		return nil
	}
}

func exampleScalar(underlying string) interface{} {
	switch underlying {
	case "string", "string (date-time)":
		return ""
	case "bool":
		return false
	case "float32", "float64":
		return 0.0
	default:
		return 0
	}
}
