package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

// ---- DNS credentials (organization-level DNS provider credentials) ----

func listDNSCredentials(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.DNSCredentialsAPI.DnscredentialsList(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.SearchID != "" {
		req = req.SearchId(args.SearchID)
	}
	if args.ID > 0 {
		req = req.Id(args.ID)
	}
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list DNS credentials"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"result":  compactJSON(result),
		"message": "Retrieved DNS credentials",
		"success": true,
	}), nil
}

func dropdownDNSCredentials(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.DNSCredentialsAPI.DnscredentialsDropdown(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}

	items, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list DNS credential dropdown"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("dnsCredentials", compactJSON(items), len(items), listMessage(len(items), "DNS credential", "DNS credentials")), nil
}

func createDNSCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsCredentialCreateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	apiResp, httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsCreate(context.Background()).
		DnsCredentialCreateCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create DNS credential", "DNS credential created successfully")
}

func updateDNSCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsCredentialUpdateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsUpdate(context.Background()).
		DnsCredentialUpdateCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update DNS credential", "DNS credential updated successfully")
}

func deleteDNSCredential(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete DNS credential", "DNS credential deleted successfully")
}

func makeDNSCredentialDefault(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsCredentialMakeDefaultCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsMakeDefault(context.Background()).
		DnsCredentialMakeDefaultCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "make DNS credential default", "DNS credential set as default successfully")
}

func lockDNSCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsCredentialLockCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsLockManagement(context.Background()).
		DnsCredentialLockCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock DNS credential", "DNS credential lock state updated successfully")
}

func attachDNSCredentialToProject(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.AttachDetachDnsCredentialCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsAttachProject(context.Background()).
		AttachDetachDnsCredentialCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "attach DNS credential to project", "DNS credential attached to project successfully")
}

func detachDNSCredentialFromProject(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.AttachDetachDnsCredentialCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsDetachProject(context.Background()).
		AttachDetachDnsCredentialCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "detach DNS credential from project", "DNS credential detached from project successfully")
}

func validateDNSCredential(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.ValidateDnsCertCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DNSCredentialsAPI.DnscredentialsValidate(context.Background()).
		ValidateDnsCertCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "validate DNS credential", "DNS credential validated successfully")
}

// ---- DNS certificates (project-scoped ACME/DNS-01 certificate service) ----

func getDNSCertStatus(client *taikungoclient.Client, args ProjectIDArgs) (*mcp_golang.ToolResponse, error) {
	result, httpResponse, err := client.Client.DnsCertAPI.DnsCertStatus(context.Background(), args.ProjectID).Execute()
	if err == nil {
		if errorResp := checkResponse(httpResponse, "get DNS certificate status"); errorResp != nil {
			return errorResp, nil
		}
		return createJSONResponse(map[string]interface{}{
			"result":  compactJSON(result),
			"message": "Retrieved DNS certificate status",
			"success": true,
		}), nil
	}

	// dev1 returns projectStatus as a numeric enum (e.g. 800) while the client expects a string.
	if httpResponse != nil && httpResponse.StatusCode >= http.StatusOK && httpResponse.StatusCode < http.StatusMultipleChoices {
		bodyBytes, readErr := readResponseBodyPreservingBody(httpResponse)
		if readErr == nil {
			var raw map[string]interface{}
			if json.Unmarshal(bodyBytes, &raw) == nil {
				normalizeDNSCertStatusProjectStatus(raw)
				return createJSONResponse(map[string]interface{}{
					"result":  raw,
					"message": "Retrieved DNS certificate status",
					"success": true,
				}), nil
			}
		}
	}

	return createError(httpResponse, err), nil
}

func normalizeDNSCertStatusProjectStatus(raw map[string]interface{}) {
	projectStatus, ok := raw["projectStatus"]
	if !ok || projectStatus == nil {
		return
	}
	switch value := projectStatus.(type) {
	case string:
		return
	case float64:
		raw["projectStatus"] = fmt.Sprintf("%g", value)
	case json.Number:
		raw["projectStatus"] = value.String()
	default:
		raw["projectStatus"] = fmt.Sprint(value)
	}
}

func enableDNSCert(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.EnableDnsCertCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DnsCertAPI.DnsCertEnable(context.Background()).
		EnableDnsCertCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "enable DNS certificate", "DNS certificate enabled successfully")
}

func disableDNSCert(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DisableDnsCertCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DnsCertAPI.DnsCertDisable(context.Background()).
		DisableDnsCertCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "disable DNS certificate", "DNS certificate disabled successfully")
}

func syncDNSCert(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsCertSyncCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DnsCertAPI.DnsCertSync(context.Background()).
		DnsCertSyncCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "sync DNS certificate", "DNS certificate sync requested successfully")
}

func validateDNSCert(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.ValidateDnsCertCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DnsCertAPI.DnsCertValidate(context.Background()).
		ValidateDnsCertCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "validate DNS certificate", "DNS certificate validated successfully")
}

// ---- Custom certificate authorities (certificate profiles) ----

func listCertificateAuthorities(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.CertificateProfilesAPI.CertificateProfilesList(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.DomainID > 0 {
		req = req.AccountId(args.DomainID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}
	if args.SearchID != "" {
		req = req.SearchId(args.SearchID)
	}
	if args.ID > 0 {
		req = req.Id(args.ID)
	}
	if args.SortBy != "" {
		req = req.SortBy(args.SortBy)
	}
	if args.SortDirection != "" {
		req = req.SortDirection(args.SortDirection)
	}
	if args.Limit > 0 {
		req = req.Limit(args.Limit)
	}
	if args.Offset > 0 {
		req = req.Offset(args.Offset)
	}

	result, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list certificate authorities"); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(map[string]interface{}{
		"result":  compactJSON(result),
		"message": "Retrieved certificate authorities",
		"success": true,
	}), nil
}

func dropdownCertificateAuthorities(client *taikungoclient.Client, args SearchListArgs) (*mcp_golang.ToolResponse, error) {
	req := client.Client.CertificateProfilesAPI.CertificateProfilesDropdown(context.Background())
	if args.OrganizationID > 0 {
		req = req.OrganizationId(args.OrganizationID)
	}
	if args.Search != "" {
		req = req.Search(args.Search)
	}

	items, httpResponse, err := req.Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list certificate authority dropdown"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("certificateAuthorities", compactJSON(items), len(items), listMessage(len(items), "certificate authority", "certificate authorities")), nil
}

func createCertificateAuthority(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CertificateProfileCreateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	apiResp, httpResponse, err := client.Client.CertificateProfilesAPI.CertificateProfilesCreate(context.Background()).
		CertificateProfileCreateCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create certificate authority", "Certificate authority created successfully")
}

func updateCertificateAuthority(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CertificateProfileUpdateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.CertificateProfilesAPI.CertificateProfilesUpdate(context.Background()).
		CertificateProfileUpdateCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "update certificate authority", "Certificate authority updated successfully")
}

func deleteCertificateAuthority(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.CertificateProfilesAPI.CertificateProfilesDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete certificate authority", "Certificate authority deleted successfully")
}

func makeCertificateAuthorityDefault(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CertificateProfileMakeDefaultCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.CertificateProfilesAPI.CertificateProfilesMakeDefault(context.Background()).
		CertificateProfileMakeDefaultCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "make certificate authority default", "Certificate authority set as default successfully")
}

func lockCertificateAuthority(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CertificateProfileLockCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.CertificateProfilesAPI.CertificateProfilesLockManagement(context.Background()).
		CertificateProfileLockCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "lock certificate authority", "Certificate authority lock state updated successfully")
}

func validateCertificateAuthority(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CertificateProfileValidateCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.CertificateProfilesAPI.CertificateProfilesValidate(context.Background()).
		CertificateProfileValidateCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "validate certificate authority", "Certificate authority validated successfully")
}
