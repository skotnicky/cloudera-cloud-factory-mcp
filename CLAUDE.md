# Cloudera Cloud Factory MCP Server Development Guidelines

## Project Overview

This project provides an MCP (Model Context Protocol) server for Cloudera Cloud Factory (formerly Taikun), enabling AI assistants to interact with the platform's infrastructure management capabilities through structured tools.

**Agent hint:** Taikun has been renamed to Cloudera Cloud Factory. Prefer the new name in user-facing text, while keeping legacy identifiers (like `taikungoclient`, `TAIKUN_*`, and `taikun-mcp`) when required for compatibility.

**Agent hint (VM vs Kubernetes node):** When the user asks for a **VM** or **server**, they usually mean a **standalone VM** (`create-standalone-vm`, related standalone VM tools, `commit-project` as needed)—not a Kubernetes cluster server role. When they ask for a **node** (or **worker node**, **cluster node**), they usually mean **adding or scaling capacity on the project’s Kubernetes cluster** (e.g. `add-server-to-project` with the appropriate role, then `commit-project`)—not a standalone VM. Confirm if the request is ambiguous.

## Resources

### Official Cloudera Cloud Factory (Taikun) Resources
- **Cloudera Cloud Factory API Documentation**: https://api.taikun.cloud/swagger/
- **Cloudera Cloud Factory Showback API**: https://api.taikun.cloud/showback/swagger/
- **Cloudera Cloud Factory Documentation**: https://docs.taikun.cloud/

### Go Client Library
- **Taikun Go Client (legacy name)**: https://github.com/itera-io/taikungoclient
  - Auto-generated nightly from OpenAPI specs
  - Used by this project for all API interactions
  - Contains all API models and client methods
  - Example usage: `client.Client.CatalogAPI.CatalogList(ctx)`

### Terraform Provider (Reference Implementation)
- **Terraform Provider Taikun (legacy name)**: https://github.com/itera-io/terraform-provider-taikun
  - Excellent reference for API usage patterns
  - Shows how to handle Cloudera Cloud Factory API responses
  - Resource implementations demonstrate proper field handling
  - Error handling patterns to follow

### MCP Protocol
- **MCP Specification**: https://modelcontextprotocol.io/
- **MCP Go SDK**: https://github.com/metoro-io/mcp-golang
  - Used for tool registration and responses
  - Provides structured communication with AI assistants

### Development Tools
- **Go Documentation**: Use `go doc github.com/itera-io/taikungoclient/client` to explore API types
- **API Inspection**: Create temporary Go files to inspect struct fields when needed
- **Taikun CLI (legacy name)**: May provide additional usage examples

### Common API Patterns to Reference

#### Boolean Parameters
- **ALWAYS call boolean setters unconditionally**: Throughout the codebase, boolean setters (e.g., `SetForceDeleteVClusters(args.ForceDeleteVClusters)`) must be called with their actual value (true or false) rather than being wrapped in an `if` block. This ensures the API receives the user-provided value rather than falling back to an internal default.

#### Catalog Operations
- **Adding Apps**: See `terraform-provider-taikun/taikun/catalog/resource_taikun_catalog.go`
- **Listing**: Most list operations follow similar patterns in the Terraform provider
- **Validation**: Check Terraform provider for input validation patterns

#### Project Management
- **Virtual Clusters**: See `terraform-provider-taikun/taikun/project/` directory
- **Project Apps**: Application deployment patterns in Terraform provider

#### Authentication
- **Client Creation**: `taikungoclient.NewClientFromAccessKey(domainName, accessKey, secretKey, apiHost)`
- **Environment Variables (legacy names)**: `TAIKUN_ACCESS_KEY`, `TAIKUN_SECRET_KEY`, `TAIKUN_API_HOST`, `TAIKUN_DOMAIN_NAME`
- **Robot User Scopes**: Prefer checking the `robot-user-capabilities` tool when you need to understand which MCP actions the current Robot User can perform.
- **Agent Scope Check**: At the beginning of any task that will call Cloudera Cloud Factory, check `robot-user-capabilities` first to confirm the current Robot User has the needed scopes. If the required access is missing, tell the user early instead of starting work that cannot succeed.

### Troubleshooting Resources

#### API Field Investigation
When unsure about API response fields:
1. Check the Terraform provider implementation
2. Use `go doc` to explore struct definitions
3. Create temporary inspection files to examine field types
4. Refer to Swagger documentation for field descriptions

#### Common Issues
- **Nullable Fields**: Always check `.IsSet()` and `.Get() != nil` for `NullableString` types
- **Pointer Fields**: Check for `!= nil` before dereferencing
- **Pagination**: Most list APIs support `limit`, `offset`, and `search` parameters
- **Error Handling**: **ALWAYS use `createError()` for ALL Cloudera Cloud Factory API errors** - provides clear, detailed error messages from the API. Use `ErrorResponse` only for custom validation errors
- **Kubeconfig TTL**: When creating kubeconfigs, use short expirations (around 60 minutes) unless the user explicitly requests longer.
- **Kubeconfig Storage**: Prefer saving kubeconfigs in temporary directories (e.g., `/tmp`) unless the user requests a specific path.
- **Cloud Credential Types**: Treat AWS/GCP/Azure as public clouds; OpenStack/Proxmox/Zadara/vSphere as private clouds; OpenShift/Generic K8s as Kubernetes clusters.
- **GCP Flavors**: Avoid machine types that support only hyperdisk attachments; they will fail to deploy.
- **AWS LoadBalancer**: When creating LoadBalancer Services on AWS, set `service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: instance` to avoid ENI resolution issues with pod IP targets.
- **Image Binding**: Binding images to a project is for standalone VM workflows. Kubernetes project deployment does not require `bind-images-to-project`, so do not treat image binding as a prerequisite for `commit-project` on Kubernetes clusters.
- **Project Changes**: If you add servers to an already deployed project, you must run `commit-project` again to apply the changes.
- **Standalone VMs**: Projects can host both Kubernetes resources and standalone VMs. After `create-standalone-vm` (or other standalone VM mutations such as disk or flavor changes), run `commit-project` for that project so changes are provisioned, unless your organization auto-commits. The MCP `commit-project` tool falls back to the VM-specific commit endpoint when the cluster-style commit path is not applicable.
- **Project Status `Updating`**: Treat `Updating` as an ongoing commit or repair operation. Do not call `commit-project` again while a project is `Updating`; wait for it to settle into a terminal or steady state first.
- **Sizing**: Treat Kubernetes control plane nodes (`Kubemaster`) as minimum 4 CPU / 4 GB RAM. When monitoring is enabled, ensure at least one `Kubeworker` is also at least 4 CPU / 4 GB RAM. This is validated by `commit-project`, which fails fast until these minimums are met. Bastion can use smaller sizing such as 2 CPU / 2 GB RAM for basic setups.
- **Virtual Clusters**: Agents can create virtual clusters within projects to isolate workloads while saving resources; ideal for testing and fast iteration.
- **Project Access IP**: Use `get-project-access-ip` for the external entry point of a project. Default Ingress class `taikun` (Traefik) and Gateway API routes are exposed through this Access IP — not through server `ipAddress` values from `list-servers` (those are usually private/internal on OpenStack and similar private clouds). On NodePort setups, find Traefik ports via `list-kubernetes-resources` (Service `taikun-ingress-traefik` in namespace `taikun-ingress`). Point ingress hostnames / DNS A records at the Access IP.

## Project Structure

### File Organization
- **`main.go`**: Tool registration, MCP server setup, response helpers
- **`catalogs.go`**: Catalog and catalog app management tools
- **`applications.go`**: Project application lifecycle tools
- **`projects.go`**: Project listing and management tools
- **`virtualclusters.go`**: Virtual cluster (project) creation and management
- **`basic_test.go`**: Compilation and JSON marshaling tests
- **`CLAUDE.md`**: This file - development guidelines

### Adding New Tool Categories
When adding new functionality that doesn't fit existing files:
1. Create a new `.go` file (e.g., `storage.go`, `networking.go`)
2. Follow the same import pattern as existing files
3. Add argument structs to `main.go` if they're shared
4. Update `basic_test.go` with new argument types
5. Register tools in `main.go`

### Integration Testing
- **`test-integration.sh`**: Integration test script
- **`test_mcp.sh`**: MCP-specific testing script
- Set up proper environment variables before running tests

## Code Standards

### JSON Response Format
**ALL tools must return JSON responses, never plain text.**

#### Required Response Types
- **Success operations**: Use `SuccessResponse` struct with `createJSONResponse()`
- **Error responses**: Use `ErrorResponse` struct with `createJSONResponse()`  
- **List operations**: Return structured JSON with arrays of typed objects
- **Detail operations**: Return structured JSON objects with all relevant fields

#### Examples

**✅ CORRECT - Success Response:**
```go
successResp := SuccessResponse{
    Message: "Operation completed successfully",
    Success: true,
}
return createJSONResponse(successResp), nil
```

**✅ CORRECT - List Response:**
```go
listResp := struct {
    Items   []ItemType `json:"items"`
    Total   int        `json:"total"`
    Message string     `json:"message"`
}{
    Items:   items,
    Total:   len(items),
    Message: "Found X items",
}
return createJSONResponse(listResp), nil
```

**❌ INCORRECT - Plain Text:**
```go
// NEVER DO THIS
return mcp_golang.NewToolResponse(
    mcp_golang.NewTextContent("Operation completed"),
), nil
```

#### Error Handling
- **ALWAYS use `createError()` for ALL Cloudera Cloud Factory API errors** - this uses `taikungoclient.CreateError()` internally
- The `createError()` function provides clear, detailed error messages from the Cloudera Cloud Factory API
- For custom validation errors (non-API), use `ErrorResponse` struct:
```go
// ✅ CORRECT - For API errors
if err != nil {
    return createError(response, err), nil
}

// ✅ CORRECT - For custom validation errors
errorResp := ErrorResponse{
    Error: "Custom validation error message",
}
return createJSONResponse(errorResp), nil
```

**Why use `createError()`?**
- Provides detailed Cloudera Cloud Factory API error messages
- Handles HTTP response codes properly
- Extracts meaningful error details from API responses
- Maintains consistent error formatting across all tools

**The `createError()` Helper Function:**
```go
// Located in main.go - uses taikungoclient.CreateError internally
func createError(response *http.Response, err error) *mcp_golang.ToolResponse {
    // Use taikungoclient's CreateError for detailed error messages
    taikunErr := taikungoclient.CreateError(response, err)
    
    var errorResp ErrorResponse
    if taikunErr != nil {
        errorResp.Error = taikunErr.Error()
    } else {
        errorResp.Error = "Unknown error occurred"
    }

    logger.Printf("Error occurred: %s", errorResp.Error)
    return createJSONResponse(errorResp)
}
```

**Example of Clear Error Messages:**
- **Without `createError()`**: `"HTTP error 404"` (unclear)
- **With `createError()`**: `"Cloudera Cloud Factory Error: wordpress not found (HTTP 404)"` (clear and actionable)

This is why we **ALWAYS** use `createError()` for Cloudera Cloud Factory API errors!

### Code Organization

#### Function Structure
1. Context creation
2. API request building with parameters
3. API execution
4. Error handling with `createError()` and `checkResponse()`
5. Data transformation to structured types
6. JSON response creation with `createJSONResponse()`

#### Type Definitions
- Define response structs with proper JSON tags
- Use `omitempty` for optional fields
- Group related types together
- Use meaningful field names

### Testing Requirements

#### Add New Tools to Tests
When adding new tools, update `basic_test.go`:
```go
{
    name: "YourNewToolArgs",
    data: YourNewToolArgs{
        // test data
    },
},
```

#### Compilation Verification
- Run `go test -v` to verify compilation
- Run `go build` to ensure no build errors
- Check for unused imports

### API Client Usage

#### Nullable Fields
Handle Cloudera Cloud Factory API nullable fields properly:
```go
// For NullableString
if field.IsSet() && field.Get() != nil {
    result.Field = *field.Get()
}

// For pointer fields
if field != nil {
    result.Field = *field
}
```

#### Pagination Support
Always support standard pagination parameters:
```go
if args.Limit > 0 {
    req = req.Limit(args.Limit)
}
if args.Offset > 0 {
    req = req.Offset(args.Offset)
}
if args.Search != "" {
    req = req.Search(args.Search)
}
```

### Tool Registration

#### Registration Pattern
```go
err = server.RegisterTool("tool-name", "Clear description", func(args ToolArgs) (*mcp_golang.ToolResponse, error) {
    client := taikungoclient.NewClientFromCredentials(username, password, "", "", "", apiHost)
    return toolFunction(client, args)
})
if err != nil {
    logger.Fatalf("Failed to register tool-name tool: %v", err)
}
logger.Println("Registered tool-name tool")
```

### Documentation

#### Tool Descriptions
- Use clear, concise descriptions
- Follow kebab-case naming (e.g., `list-available-packages`)
- Include parameter descriptions in struct tags

#### JSON Schema
Always include proper jsonschema tags:
```go
type ToolArgs struct {
    Field string `json:"field" jsonschema:"required,description=Field description"`
}
```

## Development Workflow

1. **Design**: Plan the tool's JSON response structure first
2. **Implement**: Write the function with proper error handling
3. **Test**: Add to `basic_test.go` and verify compilation
4. **Register**: Add tool registration with proper error handling
5. **Validate**: Ensure all responses are JSON formatted

## Quality Checklist

Before submitting any new tool:
- [ ] All responses use `createJSONResponse()`
- [ ] **ALL Cloudera Cloud Factory API errors use `createError()` helper** (provides clear error messages)
- [ ] Custom validation errors use `ErrorResponse` struct
- [ ] Struct types defined with proper JSON tags
- [ ] Added to `basic_test.go`
- [ ] `go test -v` passes
- [ ] `go build` succeeds
- [ ] No unused imports
- [ ] Tool registered with proper error handling
- [ ] Documentation includes clear descriptions