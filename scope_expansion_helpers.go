package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
	mcp_golang "github.com/metoro-io/mcp-golang"
)

type JSONPayloadArgs struct {
	Payload string `json:"payload" jsonschema:"required,description=JSON payload matching the underlying Cloudera Cloud Factory API command schema"`
}

type IDArgs struct {
	ID int32 `json:"id" jsonschema:"required,description=ID of the target resource"`
}

type IDPayloadArgs struct {
	ID      int32  `json:"id" jsonschema:"required,description=ID of the target resource"`
	Payload string `json:"payload" jsonschema:"required,description=JSON payload matching the underlying Cloudera Cloud Factory API command schema"`
}

type StringIDArgs struct {
	ID string `json:"id" jsonschema:"required,description=String ID of the target resource"`
}

type DomainScopedIDArgs struct {
	DomainID int32 `json:"domainId" jsonschema:"required,description=Domain ID of the parent resource"`
	ID       int32 `json:"id" jsonschema:"required,description=ID of the target resource inside the domain"`
}

type DomainScopedStringIDArgs struct {
	DomainID int32  `json:"domainId" jsonschema:"required,description=Domain ID of the parent resource"`
	ID       string `json:"id" jsonschema:"required,description=String ID of the target resource inside the domain"`
}

type GroupOrganizationPayloadArgs struct {
	GroupID        int32  `json:"groupId" jsonschema:"required,description=Identity group ID"`
	OrganizationID int32  `json:"organizationId" jsonschema:"required,description=Organization ID within the identity group"`
	Payload        string `json:"payload" jsonschema:"required,description=JSON payload matching the underlying Cloudera Cloud Factory API command schema"`
}

type ProjectIDArgs struct {
	ProjectID int32 `json:"projectId" jsonschema:"required,description=Project ID"`
}

type ProjectBackupCredentialArgs struct {
	ProjectID          int32 `json:"projectId" jsonschema:"required,description=Project ID"`
	BackupCredentialID int32 `json:"backupCredentialId" jsonschema:"required,description=Backup credential ID to use for the project"`
}

type ProjectAICredentialArgs struct {
	ProjectID      int32 `json:"projectId" jsonschema:"required,description=Project ID"`
	AICredentialID int32 `json:"aiCredentialId" jsonschema:"required,description=AI credential ID to use for the project"`
}

type ProjectPolicyProfileArgs struct {
	ProjectID       int32 `json:"projectId" jsonschema:"required,description=Project ID"`
	PolicyProfileID int32 `json:"policyProfileId" jsonschema:"required,description=Policy profile ID (OPA profile ID) to use for the project"`
}

type ProjectIDPayloadArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=Project ID"`
	Payload   string `json:"payload" jsonschema:"required,description=JSON payload matching the underlying Cloudera Cloud Factory API command schema"`
}

type ProjectNameArgs struct {
	ProjectID int32  `json:"projectId" jsonschema:"required,description=Project ID"`
	Name      string `json:"name" jsonschema:"required,description=Name of the target resource inside the project"`
}

type LockModeArgs struct {
	ID   int32  `json:"id" jsonschema:"required,description=ID of the target resource"`
	Mode string `json:"mode" jsonschema:"required,description=Lock mode to apply, typically lock or unlock"`
}

type SearchListArgs struct {
	Limit          int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return"`
	Offset         int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip"`
	CursorID       int32  `json:"cursorId,omitempty" jsonschema:"description=Cursor ID for cursor-based pagination when supported"`
	Search         string `json:"search,omitempty" jsonschema:"description=Search term to filter results"`
	SortBy         string `json:"sortBy,omitempty" jsonschema:"description=Field name to sort by when supported"`
	SortDirection  string `json:"sortDirection,omitempty" jsonschema:"description=Sort direction such as asc or desc when supported"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID filter when supported"`
	DomainID       int32  `json:"domainId,omitempty" jsonschema:"description=Account or domain ID filter when supported"`
	ID             int32  `json:"id,omitempty" jsonschema:"description=Exact resource ID filter when supported"`
	SearchID       string `json:"searchId,omitempty" jsonschema:"description=Search by related ID when supported"`
	CloudID        int32  `json:"cloudId,omitempty" jsonschema:"description=Cloud credential ID filter when supported"`
}

type ProjectSearchListArgs struct {
	ProjectID      int32  `json:"projectId" jsonschema:"required,description=Project ID"`
	Limit          int32  `json:"limit,omitempty" jsonschema:"description=Maximum number of results to return"`
	Offset         int32  `json:"offset,omitempty" jsonschema:"description=Number of results to skip"`
	Search         string `json:"search,omitempty" jsonschema:"description=Search term to filter results"`
	SortBy         string `json:"sortBy,omitempty" jsonschema:"description=Field name to sort by when supported"`
	SortDirection  string `json:"sortDirection,omitempty" jsonschema:"description=Sort direction such as asc or desc when supported"`
	FilterBy       string `json:"filterBy,omitempty" jsonschema:"description=Additional filter value when supported"`
	OrganizationID int32  `json:"organizationId,omitempty" jsonschema:"description=Organization ID filter when supported"`
	ID             int32  `json:"id,omitempty" jsonschema:"description=Exact resource ID filter when supported"`
}

func decodePayload[T any](payload string) (*T, *mcp_golang.ToolResponse) {
	if strings.TrimSpace(payload) == "" {
		return nil, createJSONResponse(ErrorResponse{
			Error: "payload is required",
		})
	}

	var parsed T
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return nil, createJSONResponse(ErrorResponse{
			Error:   "invalid JSON payload",
			Details: err.Error(),
		})
	}

	return &parsed, nil
}

func finalizeAction(httpResponse *http.Response, err error, operation, message string) (*mcp_golang.ToolResponse, error) {
	if err != nil {
		return apiErrorInfoFromResponse(httpResponse, err).toolResponse(), nil
	}
	if errorResp := checkResponse(httpResponse, operation); errorResp != nil {
		return errorResp, nil
	}
	return createJSONResponse(SuccessResponse{
		Message: message,
		Success: true,
	}), nil
}

func finalizeAPIOperation(apiResp *taikuncore.ApiResponse, httpResponse *http.Response, err error, operation, fallbackMessage string) (*mcp_golang.ToolResponse, error) {
	if err != nil {
		return apiErrorInfoFromResponse(httpResponse, err).toolResponse(), nil
	}
	if errorResp := checkResponse(httpResponse, operation); errorResp != nil {
		return errorResp, nil
	}

	message := fallbackMessage
	if apiResp != nil && apiResp.GetMessage() != "" {
		message = apiResp.GetMessage()
	}

	resp := map[string]interface{}{
		"message": message,
		"success": true,
	}
	if apiResp != nil && apiResp.GetId() != "" {
		resp["id"] = apiResp.GetId()
	}
	if apiResp != nil && apiResp.Result != nil {
		resp["result"] = apiResp.Result
	}
	return createJSONResponse(resp), nil
}

func finalizeIDOperation(id int32, httpResponse *http.Response, err error, operation, fallbackMessage, idField string) (*mcp_golang.ToolResponse, error) {
	if err != nil {
		return apiErrorInfoFromResponse(httpResponse, err).toolResponse(), nil
	}
	if errorResp := checkResponse(httpResponse, operation); errorResp != nil {
		return errorResp, nil
	}

	if idField == "" {
		idField = "id"
	}

	return createJSONResponse(map[string]interface{}{
		"message": fallbackMessage,
		"success": true,
		idField:   id,
	}), nil
}

func createListResponse(key string, items interface{}, total int, message string) *mcp_golang.ToolResponse {
	return createJSONResponse(map[string]interface{}{
		key:       items,
		"total":   total,
		"message": message,
	})
}

func listMessage(total int, singular, plural string) string {
	switch total {
	case 0:
		return fmt.Sprintf("No %s found", plural)
	case 1:
		return fmt.Sprintf("Found 1 %s", singular)
	default:
		return fmt.Sprintf("Found %d %s", total, plural)
	}
}

func readResponseBodyPreservingBody(httpResponse *http.Response) ([]byte, error) {
	if httpResponse == nil || httpResponse.Body == nil {
		return nil, nil
	}

	bodyBytes, err := io.ReadAll(httpResponse.Body)
	_ = httpResponse.Body.Close()
	httpResponse.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, err
}

func restoreResponseBody(httpResponse *http.Response, bodyBytes []byte) {
	if httpResponse == nil {
		return
	}

	httpResponse.Body = io.NopCloser(bytes.NewReader(bodyBytes))
}

type apiErrorInfo struct {
	StatusCode int
	Message    string
}

func apiErrorInfoFromResponse(httpResponse *http.Response, err error) apiErrorInfo {
	statusCode := 0
	if httpResponse != nil {
		statusCode = httpResponse.StatusCode
	}

	message := "Unknown error occurred"
	var bodyBytes []byte
	if httpResponse != nil && httpResponse.Body != nil {
		preservedBody, readErr := readResponseBodyPreservingBody(httpResponse)
		if readErr != nil {
			message = fmt.Sprintf("failed to read error response body: %v", readErr)
			logger.Printf("Error occurred: %s", message)
			return apiErrorInfo{
				StatusCode: statusCode,
				Message:    message,
			}
		}
		bodyBytes = preservedBody
		defer restoreResponseBody(httpResponse, bodyBytes)
	}

	if taikunErr := taikungoclient.CreateError(httpResponse, err); taikunErr != nil {
		message = taikunErr.Error()
	}

	logger.Printf("Error occurred: %s", message)
	return apiErrorInfo{
		StatusCode: statusCode,
		Message:    message,
	}
}

func (e apiErrorInfo) isNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

func (e apiErrorInfo) contains(fragment string) bool {
	return strings.Contains(strings.ToLower(e.Message), strings.ToLower(fragment))
}

func (e apiErrorInfo) toolResponse() *mcp_golang.ToolResponse {
	return createJSONResponse(ErrorResponse{
		Error: e.Message,
	})
}

// toolResponseWithHint returns the API error augmented with an actionable hint
// in Details. When hint is empty it is equivalent to toolResponse().
func (e apiErrorInfo) toolResponseWithHint(hint string) *mcp_golang.ToolResponse {
	if strings.TrimSpace(hint) == "" {
		return e.toolResponse()
	}
	return createJSONResponse(ErrorResponse{
		Error:   e.Message,
		Details: hint,
	})
}
