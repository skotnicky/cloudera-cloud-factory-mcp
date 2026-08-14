package main

import (
	"encoding/json"
	"strings"

	taikuncore "github.com/itera-io/taikungoclient/client"
)

// auditUserName renders an AuditUserDto (used by API "createdBy"/"modifiedBy"
// audit fields) as a human-readable string, preferring the display name and
// falling back to the user ID.
func auditUserName(u taikuncore.AuditUserDto) string {
	if u.HasDisplayName() {
		if name := u.GetDisplayName(); name != "" {
			return name
		}
	}
	return u.GetUserId()
}

// sensitiveResponseKeys are dropped from compacted responses so secrets are not
// echoed into the agent context. Matched case-insensitively against JSON keys.
var sensitiveResponseKeys = map[string]struct{}{
	"token":        {},
	"password":     {},
	"secret":       {},
	"secretkey":    {},
	"accesskey":    {},
	"privatekey":   {},
	"clientsecret": {},
	"apikey":       {},
}

func isSensitiveResponseKey(key string) bool {
	_, ok := sensitiveResponseKeys[strings.ToLower(key)]
	return ok
}

// compactJSON trims a RAW API DTO for agent consumption: it round-trips through
// JSON, then recursively drops null values, empty arrays/objects, and known
// sensitive keys. Real scalar values (including empty strings/false/0) are kept,
// so no meaningful data is lost - only the null/secret noise that bloats context.
func compactJSON(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return v
	}
	return compactValue(decoded)
}

func compactValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			if isSensitiveResponseKey(key) {
				continue
			}
			cleaned := compactValue(val)
			if cleaned == nil {
				continue
			}
			if arr, ok := cleaned.([]any); ok && len(arr) == 0 {
				continue
			}
			if m, ok := cleaned.(map[string]any); ok && len(m) == 0 {
				continue
			}
			out[key] = cleaned
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned := compactValue(item)
			if cleaned == nil {
				continue
			}
			out = append(out, cleaned)
		}
		return out
	default:
		return v
	}
}

// idNameRef is a compact reference used to replace heavy nested DTO arrays
// (e.g. bound projects) in curated list responses with just id + name.
type idNameRef struct {
	ID   int32  `json:"id"`
	Name string `json:"name,omitempty"`
}

func compactDropdownRefs(items []taikuncore.CommonDropdownDto) []idNameRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]idNameRef, 0, len(items))
	for i := range items {
		refs = append(refs, idNameRef{ID: items[i].GetId(), Name: items[i].GetName()})
	}
	return refs
}

func compactExtendedDropdownRefs(items []taikuncore.CommonExtendedDropdownDto) []idNameRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]idNameRef, 0, len(items))
	for i := range items {
		refs = append(refs, idNameRef{ID: items[i].GetId(), Name: items[i].GetName()})
	}
	return refs
}

func compactAICredentialDropdown(items []taikuncore.AiCredentialsForOrganizationEntity) []idNameRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]idNameRef, 0, len(items))
	for i := range items {
		refs = append(refs, idNameRef{ID: items[i].GetId(), Name: items[i].GetName()})
	}
	return refs
}

func compactKubernetesProfileDropdown(items []taikuncore.KubernetesProfilesEntity) []idNameRef {
	if len(items) == 0 {
		return nil
	}
	refs := make([]idNameRef, 0, len(items))
	for i := range items {
		refs = append(refs, idNameRef{ID: items[i].GetId(), Name: items[i].GetName()})
	}
	return refs
}

// AccessProfileSummary is a curated view of AccessProfilesListDto. Heavy nested
// config arrays are reduced to counts; bound projects are reduced to id/name.
type AccessProfileSummary struct {
	ID                   int32       `json:"id"`
	Name                 string      `json:"name"`
	OrganizationID       int32       `json:"organizationId,omitempty"`
	OrganizationName     string      `json:"organizationName,omitempty"`
	IsLocked             bool        `json:"isLocked"`
	HTTPProxy            string      `json:"httpProxy,omitempty"`
	DNSServerCount       int         `json:"dnsServerCount"`
	NTPServerCount       int         `json:"ntpServerCount"`
	TrustedRegistryCount int         `json:"trustedRegistryCount"`
	AllowedHostCount     int         `json:"allowedHostCount"`
	Projects             []idNameRef `json:"projects,omitempty"`
	CreatedAt            string      `json:"createdAt,omitempty"`
	CreatedBy            string      `json:"createdBy,omitempty"`
}

func toAccessProfileSummaries(items []taikuncore.AccessProfilesListDto) []AccessProfileSummary {
	summaries := make([]AccessProfileSummary, 0, len(items))
	for i := range items {
		item := items[i]
		summaries = append(summaries, AccessProfileSummary{
			ID:                   item.GetId(),
			Name:                 item.GetName(),
			OrganizationID:       item.GetOrganizationId(),
			OrganizationName:     item.GetOrganizationName(),
			IsLocked:             item.GetIsLocked(),
			HTTPProxy:            item.GetHttpProxy(),
			DNSServerCount:       len(item.DnsServers),
			NTPServerCount:       len(item.NtpServers),
			TrustedRegistryCount: len(item.TrustedRegistries),
			AllowedHostCount:     len(item.AllowedHosts),
			Projects:             compactDropdownRefs(item.Projects),
			CreatedAt:            item.GetCreatedAt(),
			CreatedBy:            auditUserName(item.GetCreatedBy()),
		})
	}
	return summaries
}

// AICredentialSummary is a curated view of AiCredentialsListDto.
type AICredentialSummary struct {
	ID               int32       `json:"id"`
	Name             string      `json:"name,omitempty"`
	Type             string      `json:"type,omitempty"`
	URL              string      `json:"url,omitempty"`
	OrganizationID   int32       `json:"organizationId,omitempty"`
	OrganizationName string      `json:"organizationName,omitempty"`
	IsDefault        bool        `json:"isDefault"`
	Projects         []idNameRef `json:"projects,omitempty"`
}

func toAICredentialSummaries(items []taikuncore.AiCredentialsListDto) []AICredentialSummary {
	summaries := make([]AICredentialSummary, 0, len(items))
	for i := range items {
		item := items[i]
		summaries = append(summaries, AICredentialSummary{
			ID:               item.GetId(),
			Name:             item.GetName(),
			Type:             string(item.GetType()),
			URL:              item.GetUrl(),
			OrganizationID:   item.GetOrganizationId(),
			OrganizationName: item.GetOrganizationName(),
			IsDefault:        item.GetIsDefault(),
			Projects:         compactDropdownRefs(item.Projects),
		})
	}
	return summaries
}

// OPAProfileSummary is a curated view of OpaProfileListDto keeping the core
// policy toggles and dropping the long whitelist string arrays.
type OPAProfileSummary struct {
	ID                    int32       `json:"id"`
	Name                  string      `json:"name"`
	OrganizationID        int32       `json:"organizationId,omitempty"`
	OrganizationName      string      `json:"organizationName,omitempty"`
	IsLocked              bool        `json:"isLocked"`
	IsDefault             bool        `json:"isDefault"`
	Revision              int32       `json:"revision,omitempty"`
	ForbidNodePort        bool        `json:"forbidNodePort"`
	ForbidHTTPIngress     bool        `json:"forbidHttpIngress"`
	RequireProbe          bool        `json:"requireProbe"`
	UniqueIngresses       bool        `json:"uniqueIngresses"`
	UniqueServiceSelector bool        `json:"uniqueServiceSelector"`
	ForcePodResource      bool        `json:"forcePodResource"`
	Projects              []idNameRef `json:"projects,omitempty"`
}

func toOPAProfileSummaries(items []taikuncore.OpaProfileListDto) []OPAProfileSummary {
	summaries := make([]OPAProfileSummary, 0, len(items))
	for i := range items {
		item := items[i]
		summaries = append(summaries, OPAProfileSummary{
			ID:                    item.GetId(),
			Name:                  item.GetName(),
			OrganizationID:        item.GetOrganizationId(),
			OrganizationName:      item.GetOrganizationName(),
			IsLocked:              item.GetIsLocked(),
			IsDefault:             item.GetIsDefault(),
			Revision:              item.GetRevision(),
			ForbidNodePort:        item.GetForbidNodePort(),
			ForbidHTTPIngress:     item.GetForbidHttpIngress(),
			RequireProbe:          item.GetRequireProbe(),
			UniqueIngresses:       item.GetUniqueIngresses(),
			UniqueServiceSelector: item.GetUniqueServiceSelector(),
			ForcePodResource:      item.GetForcePodResource(),
			Projects:              compactDropdownRefs(item.Projects),
		})
	}
	return summaries
}

// KubernetesProfileSummary is a curated view of KubernetesProfilesListDto.
type KubernetesProfileSummary struct {
	ID                      int32       `json:"id"`
	Name                    string      `json:"name"`
	OrganizationID          int32       `json:"organizationId,omitempty"`
	OrganizationName        string      `json:"organizationName,omitempty"`
	CNI                     string      `json:"cni,omitempty"`
	OctaviaEnabled          bool        `json:"octaviaEnabled"`
	TaikunLBEnabled         bool        `json:"taikunLBEnabled"`
	ExposeNodePortOnBastion bool        `json:"exposeNodePortOnBastion"`
	AllowSchedulingOnMaster bool        `json:"allowSchedulingOnMaster"`
	IsLocked                bool        `json:"isLocked"`
	Projects                []idNameRef `json:"projects,omitempty"`
}

func toKubernetesProfileSummaries(items []taikuncore.KubernetesProfilesListDto) []KubernetesProfileSummary {
	summaries := make([]KubernetesProfileSummary, 0, len(items))
	for i := range items {
		item := items[i]
		summaries = append(summaries, KubernetesProfileSummary{
			ID:                      item.GetId(),
			Name:                    item.GetName(),
			OrganizationID:          item.GetOrganizationId(),
			OrganizationName:        item.GetOrganizationName(),
			CNI:                     string(item.GetCni()),
			OctaviaEnabled:          item.GetOctaviaEnabled(),
			TaikunLBEnabled:         item.GetTaikunLBEnabled(),
			ExposeNodePortOnBastion: item.GetExposeNodePortOnBastion(),
			AllowSchedulingOnMaster: item.GetAllowSchedulingOnMaster(),
			IsLocked:                item.GetIsLocked(),
			Projects:                compactDropdownRefs(item.Projects),
		})
	}
	return summaries
}

// AlertingProfileSummary is a curated view of AlertingProfilesListDto; emails
// and webhooks are reduced to counts and bound projects to id/name.
type AlertingProfileSummary struct {
	ID                     int32       `json:"id"`
	Name                   string      `json:"name"`
	OrganizationID         int32       `json:"organizationId,omitempty"`
	OrganizationName       string      `json:"organizationName,omitempty"`
	IsLocked               bool        `json:"isLocked"`
	Reminder               string      `json:"reminder,omitempty"`
	SlackConfigurationName string      `json:"slackConfigurationName,omitempty"`
	EmailCount             int         `json:"emailCount"`
	WebhookCount           int         `json:"webhookCount"`
	Projects               []idNameRef `json:"projects,omitempty"`
}

func toAlertingProfileSummaries(items []taikuncore.AlertingProfilesListDto) []AlertingProfileSummary {
	summaries := make([]AlertingProfileSummary, 0, len(items))
	for i := range items {
		item := items[i]
		summaries = append(summaries, AlertingProfileSummary{
			ID:                     item.GetId(),
			Name:                   item.GetName(),
			OrganizationID:         item.GetOrganizationId(),
			OrganizationName:       item.GetOrganizationName(),
			IsLocked:               item.GetIsLocked(),
			Reminder:               string(item.GetReminder()),
			SlackConfigurationName: item.GetSlackConfigurationName(),
			EmailCount:             len(item.Emails),
			WebhookCount:           len(item.Webhooks),
			Projects:               compactDropdownRefs(item.Projects),
		})
	}
	return summaries
}
