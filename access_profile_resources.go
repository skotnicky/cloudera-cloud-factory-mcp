package main

import (
	"context"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

// AccessProfileScopedListArgs lists sub-resources that belong to an access
// profile (SSH users, NTP servers, DNS servers, trusted registries).
type AccessProfileScopedListArgs struct {
	AccessProfileID int32 `json:"accessProfileId" jsonschema:"required,description=Access profile ID that owns the sub-resources"`
}

// ---- DNS servers (access profile sub-resource) ----

func listDNSServers(client *taikungoclient.Client, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
	items, httpResponse, err := client.Client.DnsServersAPI.DnsserversList(context.Background(), args.AccessProfileID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list DNS servers"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("dnsServers", compactJSON(items), len(items), listMessage(len(items), "DNS server", "DNS servers")), nil
}

func createDNSServer(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateDnsServerCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	apiResp, httpResponse, err := client.Client.DnsServersAPI.DnsserversCreate(context.Background()).
		CreateDnsServerCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create DNS server", "DNS server created successfully")
}

func editDNSServer(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsNtpAddressEditDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.DnsServersAPI.DnsserversEdit(context.Background(), args.ID).
		DnsNtpAddressEditDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "edit DNS server", "DNS server updated successfully")
}

func deleteDNSServer(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.DnsServersAPI.DnsserversDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete DNS server", "DNS server deleted successfully")
}

// ---- NTP servers (access profile sub-resource) ----

func listNTPServers(client *taikungoclient.Client, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
	items, httpResponse, err := client.Client.NtpServersAPI.NtpserversList(context.Background(), args.AccessProfileID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list NTP servers"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("ntpServers", compactJSON(items), len(items), listMessage(len(items), "NTP server", "NTP servers")), nil
}

func createNTPServer(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateNtpServerCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	apiResp, httpResponse, err := client.Client.NtpServersAPI.NtpserversCreate(context.Background()).
		CreateNtpServerCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create NTP server", "NTP server created successfully")
}

func editNTPServer(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DnsNtpAddressEditDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.NtpServersAPI.NtpserversEdit(context.Background(), args.ID).
		DnsNtpAddressEditDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "edit NTP server", "NTP server updated successfully")
}

func deleteNTPServer(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.NtpServersAPI.NtpserversDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete NTP server", "NTP server deleted successfully")
}

// ---- SSH users (access profile sub-resource) ----

func listSSHUsers(client *taikungoclient.Client, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
	items, httpResponse, err := client.Client.SshUsersAPI.SshusersList(context.Background(), args.AccessProfileID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list SSH users"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("sshUsers", compactJSON(items), len(items), listMessage(len(items), "SSH user", "SSH users")), nil
}

func createSSHUser(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateSshUserCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	apiResp, httpResponse, err := client.Client.SshUsersAPI.SshusersCreate(context.Background()).
		CreateSshUserCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create SSH user", "SSH user created successfully")
}

func editSSHUser(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.EditSshUserCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.SshUsersAPI.SshusersEdit(context.Background()).
		EditSshUserCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "edit SSH user", "SSH user updated successfully")
}

func deleteSSHUser(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.DeleteSshUserCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.SshUsersAPI.SshusersDelete(context.Background()).
		DeleteSshUserCommand(*command).
		Execute()
	return finalizeAction(httpResponse, err, "delete SSH user", "SSH user deleted successfully")
}

// ---- Trusted registries (access profile sub-resource) ----

func listTrustedRegistries(client *taikungoclient.Client, args AccessProfileScopedListArgs) (*mcp_golang.ToolResponse, error) {
	items, httpResponse, err := client.Client.TrustedRegistriesAPI.TrustedregistriesList(context.Background(), args.AccessProfileID).Execute()
	if err != nil {
		return createError(httpResponse, err), nil
	}
	if errorResp := checkResponse(httpResponse, "list trusted registries"); errorResp != nil {
		return errorResp, nil
	}
	return createListResponse("trustedRegistries", compactJSON(items), len(items), listMessage(len(items), "trusted registry", "trusted registries")), nil
}

func createTrustedRegistry(client *taikungoclient.Client, args JSONPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.CreateTrustedRegistriesCommand](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	apiResp, httpResponse, err := client.Client.TrustedRegistriesAPI.TrustedregistriesCreate(context.Background()).
		CreateTrustedRegistriesCommand(*command).
		Execute()
	return finalizeAPIOperation(apiResp, httpResponse, err, "create trusted registry", "Trusted registry created successfully")
}

func editTrustedRegistry(client *taikungoclient.Client, args IDPayloadArgs) (*mcp_golang.ToolResponse, error) {
	command, errorResp := decodePayload[taikuncore.TrustedRegistryEditDto](args.Payload)
	if errorResp != nil {
		return errorResp, nil
	}
	httpResponse, err := client.Client.TrustedRegistriesAPI.TrustedregistriesEdit(context.Background(), args.ID).
		TrustedRegistryEditDto(*command).
		Execute()
	return finalizeAction(httpResponse, err, "edit trusted registry", "Trusted registry updated successfully")
}

func deleteTrustedRegistry(client *taikungoclient.Client, args IDArgs) (*mcp_golang.ToolResponse, error) {
	httpResponse, err := client.Client.TrustedRegistriesAPI.TrustedregistriesDelete(context.Background(), args.ID).Execute()
	return finalizeAction(httpResponse, err, "delete trusted registry", "Trusted registry deleted successfully")
}
