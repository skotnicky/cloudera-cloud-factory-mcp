package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itera-io/taikungoclient"
	taikuncore "github.com/itera-io/taikungoclient/client"
)

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestResolveRobotUserAuthConfigDefaultsHost(t *testing.T) {
	cfg, err := resolveRobotUserAuthConfig(mapGetenv(map[string]string{
		"TAIKUN_ACCESS_KEY": "robot-access",
		"TAIKUN_SECRET_KEY": "robot-secret",
	}))
	if err != nil {
		t.Fatalf("expected valid robot user config, got error: %v", err)
	}

	if cfg.APIHost != defaultAPIHost {
		t.Fatalf("expected default API host %q, got %q", defaultAPIHost, cfg.APIHost)
	}
	if cfg.AccessKey != "robot-access" || cfg.SecretKey != "robot-secret" {
		t.Fatalf("unexpected robot user credentials in config: %+v", cfg)
	}
}

func TestResolveRobotUserAuthConfigSupportsOptionalDomainName(t *testing.T) {
	cfg, err := resolveRobotUserAuthConfig(mapGetenv(map[string]string{
		"TAIKUN_ACCESS_KEY":  "robot-access",
		"TAIKUN_SECRET_KEY":  "robot-secret",
		"TAIKUN_DOMAIN_NAME": "example-domain",
		"TAIKUN_API_HOST":    "api.example.test",
	}))
	if err != nil {
		t.Fatalf("expected valid robot user config, got error: %v", err)
	}

	if cfg.DomainName != "example-domain" {
		t.Fatalf("expected domain name to be preserved, got %q", cfg.DomainName)
	}
	if cfg.APIHost != "api.example.test" {
		t.Fatalf("expected API host override to be preserved, got %q", cfg.APIHost)
	}
}

func TestResolveRobotUserAuthConfigRejectsIncompleteCredentials(t *testing.T) {
	_, err := resolveRobotUserAuthConfig(mapGetenv(map[string]string{
		"TAIKUN_ACCESS_KEY": "robot-access",
	}))
	if err == nil {
		t.Fatal("expected incomplete robot user credentials to fail")
	}
	if !strings.Contains(err.Error(), "TAIKUN_ACCESS_KEY") || !strings.Contains(err.Error(), "TAIKUN_SECRET_KEY") {
		t.Fatalf("expected error to mention both robot user env vars, got: %v", err)
	}
}

func TestResolveRobotUserAuthConfigRejectsLegacyEmailPassword(t *testing.T) {
	_, err := resolveRobotUserAuthConfig(mapGetenv(map[string]string{
		"TAIKUN_EMAIL":    "user@example.com",
		"TAIKUN_PASSWORD": "super-secret",
	}))
	if err == nil {
		t.Fatal("expected legacy email/password auth to be rejected")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("expected legacy auth rejection message, got: %v", err)
	}
}

func TestEvaluateToolScopeAccessAllowed(t *testing.T) {
	access := evaluateToolScopeAccess("create-project", []string{"scope:projects:write"})
	if access.Status != "allowed" {
		t.Fatalf("expected allowed status, got %+v", access)
	}
}

func TestEvaluateToolScopeAccessBlocked(t *testing.T) {
	access := evaluateToolScopeAccess("create-project", []string{"scope:projects:read"})
	if access.Status != "blocked" {
		t.Fatalf("expected blocked status, got %+v", access)
	}
	if len(access.MissingScopes) != 1 || access.MissingScopes[0] != "scope:projects:write" {
		t.Fatalf("unexpected missing scopes: %+v", access.MissingScopes)
	}
}

func TestEvaluateToolScopeAccessUnknown(t *testing.T) {
	access := evaluateToolScopeAccess("unmapped-tool", nil)
	if access.Status != "unknown" {
		t.Fatalf("expected unknown status, got %+v", access)
	}
}

func TestEvaluateToolScopeAccessAllowsNoScopeTools(t *testing.T) {
	access := evaluateToolScopeAccess("robot-user-capabilities", nil)
	if access.Status != "allowed" {
		t.Fatalf("expected allowed status, got %+v", access)
	}
	if len(access.RequiredScopes) != 0 {
		t.Fatalf("expected no required scopes, got %+v", access.RequiredScopes)
	}
}

func TestRobotUserContextFromDetailsPopulatesAccountFields(t *testing.T) {
	creator := taikuncore.AuditUserDto{}
	creator.SetDisplayName("creator")
	details := taikuncore.NewRobotUsersListDto(
		"user-1",
		7,
		"domain-fallback",
		"access-key",
		creator,
		"robot-name",
		[]string{"scope:projects:read"},
		true,
		"2026-04-08T00:00:00Z",
	)
	details.AdditionalProperties = map[string]interface{}{
		"accountId":   float64(42),
		"accountName": "ccf-account",
	}

	ctx := robotUserContextFromDetails(details)
	if ctx.AccountID != 42 {
		t.Fatalf("expected account id 42, got %+v", ctx)
	}
	if ctx.AccountName != "ccf-account" {
		t.Fatalf("expected account name to come from accountName, got %+v", ctx)
	}
}

func TestAuthorizeToolDeniesScopedToolWhenScopeDiscoveryFails(t *testing.T) {
	setRobotUserContext(RobotUserContext{ScopeDiscoveryError: "boom"})
	t.Cleanup(func() {
		setRobotUserContext(RobotUserContext{})
	})

	denied := authorizeTool(context.Background(), "create-project")
	if denied == nil {
		t.Fatal("expected scoped tool to be denied when scope discovery fails")
	}
}

func TestAuthorizeToolAllowsNoScopeToolWhenScopeDiscoveryFails(t *testing.T) {
	setRobotUserContext(RobotUserContext{ScopeDiscoveryError: "boom"})
	t.Cleanup(func() {
		setRobotUserContext(RobotUserContext{})
	})

	denied := authorizeTool(context.Background(), "robot-user-capabilities")
	if denied != nil {
		t.Fatal("expected no-scope tool to remain allowed when scope discovery fails")
	}
}

func TestNewRefreshTaikunClientResponseSuccessWhenScopeDiscoverySucceeds(t *testing.T) {
	resp := newRefreshTaikunClientResponse(RobotUserContext{
		Name:             "robot",
		OrganizationName: "org",
		Scopes:           []string{"scope:projects:read"},
	})

	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}
}

func TestNewRefreshTaikunClientResponseFailsWhenScopeDiscoveryFails(t *testing.T) {
	resp := newRefreshTaikunClientResponse(RobotUserContext{
		Name:                "robot",
		ScopeDiscoveryError: "unable to load scopes",
	})

	if resp.Success {
		t.Fatalf("expected failed response when scope discovery fails, got %+v", resp)
	}
	if resp.ScopeDiscoveryError == "" {
		t.Fatalf("expected scope discovery error to be preserved, got %+v", resp)
	}
}

// ---- parseRobotUserContext ----

func TestParseRobotUserContextParsesScopes(t *testing.T) {
	body := []byte(`{"accessKey":"ak","name":"bot","scopes":["scope:projects:read","scope:projects:write"],"isActive":true}`)
	ctx, err := parseRobotUserContext(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %v", ctx.Scopes)
	}
	if ctx.AccessKey != "ak" || ctx.Name != "bot" {
		t.Fatalf("unexpected fields: %+v", ctx)
	}
}

func TestParseRobotUserContextRejectsEmptyBody(t *testing.T) {
	_, err := parseRobotUserContext(nil)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestParseRobotUserContextRejectsMissingMetadata(t *testing.T) {
	_, err := parseRobotUserContext([]byte(`{"scopes":["scope:x"]}`))
	if err == nil {
		t.Fatal("expected error when accessKey and name are absent")
	}
}

func TestParseRobotUserContextScopesAreSorted(t *testing.T) {
	body := []byte(`{"accessKey":"ak","name":"bot","scopes":["scope:z","scope:a","scope:m"]}`)
	ctx, err := parseRobotUserContext(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 1; i < len(ctx.Scopes); i++ {
		if ctx.Scopes[i] < ctx.Scopes[i-1] {
			t.Fatalf("expected sorted scopes, got %v", ctx.Scopes)
		}
	}
}

// ---- resolveRobotUserContext ----

func newRobotAPITestServer(t *testing.T, scopes []string) (*httptest.Server, *taikungoclient.Client) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/robot/details" {
			payload := map[string]interface{}{
				"accessKey": "test-key",
				"name":      "test-robot",
				"scopes":    scopes,
				"isActive":  true,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)

	cfg := taikuncore.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = strings.TrimPrefix(ts.URL, "http://")
	client := &taikungoclient.Client{Client: taikuncore.NewAPIClient(cfg)}
	return ts, client
}

func TestResolveRobotUserContextFallsBackToGlobalWhenNoPerRequestClient(t *testing.T) {
	setRobotUserContext(RobotUserContext{Scopes: []string{"scope:projects:read"}, Name: "global-robot"})
	t.Cleanup(func() { setRobotUserContext(RobotUserContext{}) })

	prev := taikunClient
	taikunClient = nil
	defer func() { taikunClient = prev }()

	robotCtx := resolveRobotUserContext(context.Background())
	if robotCtx.Name != "global-robot" {
		t.Fatalf("expected global context, got %+v", robotCtx)
	}
}

func TestResolveRobotUserContextFetchesFromPerRequestClient(t *testing.T) {
	_, client := newRobotAPITestServer(t, []string{"scope:projects:read", "scope:projects:write"})

	prev := taikunClient
	taikunClient = nil
	defer func() { taikunClient = prev }()

	ctx := contextWithClient(context.Background(), client)
	robotCtx := resolveRobotUserContext(ctx)
	if len(robotCtx.Scopes) != 2 {
		t.Fatalf("expected 2 scopes from per-request fetch, got %v", robotCtx.Scopes)
	}
}

func TestResolveRobotUserContextHandlesFetchError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)
	client := taikungoclient.NewClientFromAccessKey("", "k", "s", ts.URL)

	prev := taikunClient
	taikunClient = nil
	defer func() { taikunClient = prev }()

	ctx := contextWithClient(context.Background(), client)
	robotCtx := resolveRobotUserContext(ctx)
	if robotCtx.ScopeDiscoveryError == "" {
		t.Fatal("expected ScopeDiscoveryError to be set when fetch fails")
	}
}

// ---- authorizeTool with per-request client ----

func TestAuthorizeToolAllowsToolWhenPerRequestClientHasRequiredScope(t *testing.T) {
	_, client := newRobotAPITestServer(t, []string{"scope:projects:read"})

	prev := taikunClient
	taikunClient = nil
	defer func() { taikunClient = prev }()

	ctx := contextWithClient(context.Background(), client)
	denied := authorizeTool(ctx, "list-projects")
	if denied != nil {
		t.Fatalf("expected tool to be allowed, got denial: %v", denied)
	}
}

func TestAuthorizeToolBlocksToolWhenPerRequestClientLacksScope(t *testing.T) {
	_, client := newRobotAPITestServer(t, []string{"scope:applications:read"})

	prev := taikunClient
	taikunClient = nil
	defer func() { taikunClient = prev }()

	ctx := contextWithClient(context.Background(), client)
	denied := authorizeTool(ctx, "list-projects")
	if denied == nil {
		t.Fatal("expected tool to be blocked when required scope is missing")
	}
}

func TestAuthorizeToolAllowsScopeToolWhenGlobalHasScope(t *testing.T) {
	setRobotUserContext(RobotUserContext{Scopes: []string{"scope:projects:read"}})
	t.Cleanup(func() { setRobotUserContext(RobotUserContext{}) })

	denied := authorizeTool(context.Background(), "list-projects")
	if denied != nil {
		t.Fatalf("expected tool to be allowed via global context, got denial")
	}
}

func TestAuthorizeToolBlocksScopeToolWhenGlobalLacksScope(t *testing.T) {
	setRobotUserContext(RobotUserContext{Scopes: []string{}})
	t.Cleanup(func() { setRobotUserContext(RobotUserContext{}) })

	denied := authorizeTool(context.Background(), "list-projects")
	if denied == nil {
		t.Fatal("expected tool to be blocked when global context has no scopes")
	}
}

func TestAuthorizeToolBlocksUnknownTool(t *testing.T) {
	denied := authorizeTool(context.Background(), "not-a-real-tool")
	if denied == nil {
		t.Fatal("expected denial for tool with no scope mapping")
	}
}

// ---- scopeDeniedResponseWithCtx error message ----

func TestScopeDeniedResponseWithCtxIncludesAssignedScopes(t *testing.T) {
	access := ToolScopeAccess{
		Tool:           "list-projects",
		Status:         "blocked",
		RequiredScopes: []string{"scope:projects:read"},
		MissingScopes:  []string{"scope:projects:read"},
	}
	robotCtx := RobotUserContext{Scopes: []string{"scope:applications:read"}}
	resp := scopeDeniedResponseWithCtx("list-projects", access, robotCtx)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	text := resp.Content[0].TextContent.Text
	if !strings.Contains(text, "scope:applications:read") {
		t.Errorf("expected assigned scopes in error details, got: %s", text)
	}
	if !strings.Contains(text, "scope:projects:read") {
		t.Errorf("expected required scope in error details, got: %s", text)
	}
}

func TestScopeDeniedResponseWithCtxIncludesScopeDiscoveryWarning(t *testing.T) {
	access := ToolScopeAccess{
		Tool:           "list-projects",
		Status:         "blocked",
		RequiredScopes: []string{"scope:projects:read"},
		MissingScopes:  []string{"scope:projects:read"},
	}
	robotCtx := RobotUserContext{ScopeDiscoveryError: "timeout reaching API"}
	resp := scopeDeniedResponseWithCtx("list-projects", access, robotCtx)
	text := resp.Content[0].TextContent.Text
	if !strings.Contains(text, "timeout reaching API") {
		t.Errorf("expected scope discovery warning in error details, got: %s", text)
	}
}

// ---- buildCapabilitiesResponse ----

func TestBuildCapabilitiesResponseListsAllMappedTools(t *testing.T) {
	robotCtx := RobotUserContext{Scopes: []string{"scope:projects:read"}}
	resp := buildCapabilitiesResponse(robotCtx)
	if len(resp.ToolAccess) != len(toolRequiredScopes) {
		t.Fatalf("expected %d tools, got %d", len(toolRequiredScopes), len(resp.ToolAccess))
	}
}

func TestBuildCapabilitiesResponseSuccessFlag(t *testing.T) {
	robotCtx := RobotUserContext{Name: "bot", Scopes: []string{}}
	resp := buildCapabilitiesResponse(robotCtx)
	if !resp.Success {
		t.Fatal("expected Success=true when no ScopeDiscoveryError")
	}
}

func TestBuildCapabilitiesResponseFailFlagOnDiscoveryError(t *testing.T) {
	robotCtx := RobotUserContext{ScopeDiscoveryError: "unreachable"}
	resp := buildCapabilitiesResponse(robotCtx)
	if resp.Success {
		t.Fatal("expected Success=false when ScopeDiscoveryError is set")
	}
}
