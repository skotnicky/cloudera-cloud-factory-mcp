package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	mcp_golang "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport"
)

// newTestTransport creates an authHTTPTransport and an httptest.Server backed by it.
// The test server is closed automatically when the test ends.
func newTestTransport(t *testing.T) (*authHTTPTransport, *httptest.Server) {
	t.Helper()
	tr := newAuthHTTPTransport("", "/mcp")
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", tr.handleRequest)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return tr, ts
}

// mcpPost sends a JSON-RPC POST to the given URL with optional credential headers.
func mcpPost(t *testing.T, url, body, accessKey, secretKey string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", url+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if accessKey != "" {
		req.Header.Set("X-CCF-Access-Key", accessKey)
	}
	if secretKey != "" {
		req.Header.Set("X-CCF-Secret-Key", secretKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	return resp
}

// ---- HTTP method / auth guards ----

func TestAuthHTTPTransportRejectsNonPost(t *testing.T) {
	_, ts := newTestTransport(t)

	for _, method := range []string{"GET", "PUT", "DELETE", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			req, _ := http.NewRequest(method, ts.URL+"/mcp", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("expected 405, got %d", resp.StatusCode)
			}
		})
	}
}

func TestAuthHTTPTransportRejectsMissingBothCredentials(t *testing.T) {
	_, ts := newTestTransport(t)
	resp := mcpPost(t, ts.URL, `{}`, "", "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAuthHTTPTransportRejectsMissingAccessKey(t *testing.T) {
	_, ts := newTestTransport(t)
	resp := mcpPost(t, ts.URL, `{}`, "", "secret-only")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 when access key is missing, got %d", resp.StatusCode)
	}
}

func TestAuthHTTPTransportRejectsMissingSecretKey(t *testing.T) {
	_, ts := newTestTransport(t)
	resp := mcpPost(t, ts.URL, `{}`, "access-only", "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 when secret key is missing, got %d", resp.StatusCode)
	}
}

func TestAuthHTTPTransportAcceptsValidCredentials(t *testing.T) {
	tr, ts := newTestTransport(t)

	// Set a message handler that immediately sends back a valid response so
	// handleRequest does not block waiting on the channel.
	tr.SetMessageHandler(func(ctx context.Context, msg *transport.BaseJsonRpcMessage) {
		if msg.JsonRpcRequest == nil {
			return
		}
		_ = tr.Send(ctx, &transport.BaseJsonRpcMessage{
			Type: transport.BaseMessageTypeJSONRPCResponseType,
			JsonRpcResponse: &transport.BaseJSONRPCResponse{
				Id:      msg.JsonRpcRequest.Id,
				Jsonrpc: "2.0",
				Result:  json.RawMessage(`{}`),
			},
		})
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`
	resp := mcpPost(t, ts.URL, body, "valid-access", "valid-secret")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 with valid credentials, got %d", resp.StatusCode)
	}
}

// ---- Per-request client injection ----

func TestAuthHTTPTransportInjectsClientIntoContext(t *testing.T) {
	tr, ts := newTestTransport(t)

	var (
		mu             sync.Mutex
		capturedClient = make(chan bool, 1)
	)

	tr.SetMessageHandler(func(ctx context.Context, msg *transport.BaseJsonRpcMessage) {
		if msg.JsonRpcRequest == nil {
			return
		}
		mu.Lock()
		client := clientFromContext(ctx)
		capturedClient <- client != nil
		mu.Unlock()

		_ = tr.Send(ctx, &transport.BaseJsonRpcMessage{
			Type: transport.BaseMessageTypeJSONRPCResponseType,
			JsonRpcResponse: &transport.BaseJSONRPCResponse{
				Id:      msg.JsonRpcRequest.Id,
				Jsonrpc: "2.0",
				Result:  json.RawMessage(`{}`),
			},
		})
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`
	resp := mcpPost(t, ts.URL, body, "access-key", "secret-key")
	_ = resp.Body.Close()

	hasClient := <-capturedClient
	if !hasClient {
		t.Error("expected non-nil client in context, got nil")
	}
}

func TestAuthHTTPTransportDifferentClientsPerRequest(t *testing.T) {
	// Two concurrent requests with different keys must get different clients.
	tr, ts := newTestTransport(t)

	type capture struct {
		clientPtr uintptr
	}
	results := make(chan capture, 2)

	tr.SetMessageHandler(func(ctx context.Context, msg *transport.BaseJsonRpcMessage) {
		if msg.JsonRpcRequest == nil {
			return
		}
		client := clientFromContext(ctx)
		var ptr uintptr
		if client != nil {
			ptr = uintptr(client.Client.GetConfig().Host[0]) // just some non-zero differentiator
			_ = ptr
		}
		results <- capture{clientPtr: ptr}

		_ = tr.Send(ctx, &transport.BaseJsonRpcMessage{
			Type: transport.BaseMessageTypeJSONRPCResponseType,
			JsonRpcResponse: &transport.BaseJSONRPCResponse{
				Id:      msg.JsonRpcRequest.Id,
				Jsonrpc: "2.0",
				Result:  json.RawMessage(`{}`),
			},
		})
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`

	var wg sync.WaitGroup
	for _, key := range []string{"user-a", "user-b"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			resp := mcpPost(t, ts.URL, body, k, "secret")
			_ = resp.Body.Close()
		}(key)
	}
	wg.Wait()
	close(results)

	var clients []capture
	for c := range results {
		clients = append(clients, c)
	}
	if len(clients) != 2 {
		t.Fatalf("expected 2 handler invocations, got %d", len(clients))
	}
}

// ---- Full MCP server integration ----

// checkClientArgs is a named empty args type used by the check-client test tool.
// mcp-golang's JSON schema reflector requires a named (non-anonymous) struct type.
type checkClientArgs struct{}

type checkClientResult struct {
	HasClient bool `json:"hasClient"`
}

// newMCPTestServer wires a real mcp_golang.Server to an authHTTPTransport and
// starts an httptest.Server. Returns the test server URL.
func newMCPTestServer(t *testing.T) string {
	t.Helper()

	// Ensure no global client interferes.
	prev := taikunClient
	taikunClient = nil
	t.Cleanup(func() { taikunClient = prev })

	tr := newAuthHTTPTransport("", "/mcp")
	server := mcp_golang.NewServer(tr,
		mcp_golang.WithName("test-server"),
		mcp_golang.WithVersion("0.1"),
	)

	// Register a tool that reports whether a per-request client is present.
	if err := server.RegisterTool("check-client", "returns whether a per-request client is in context",
		func(ctx context.Context, args checkClientArgs) (*mcp_golang.ToolResponse, error) {
			client := clientFromContext(ctx)
			result := checkClientResult{HasClient: client != nil}
			data, _ := json.Marshal(result)
			return mcp_golang.NewToolResponse(mcp_golang.NewTextContent(string(data))), nil
		},
	); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	// Serve wires the MCP protocol handlers into the transport; Start() is now a
	// no-op so Serve returns immediately without binding a port.
	if err := server.Serve(); err != nil {
		t.Fatalf("server.Serve: %v", err)
	}

	// Expose the transport's HTTP handler via an httptest.Server.
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", tr.handleRequest)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestMCPInitializeHandshake(t *testing.T) {
	url := newMCPTestServer(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	resp := mcpPost(t, url, body, "access", "secret")
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var rpc struct {
		Result struct {
			ServerInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rpc.Result.ServerInfo.Name != "test-server" {
		t.Errorf("expected serverInfo.name=test-server, got %q", rpc.Result.ServerInfo.Name)
	}
	if rpc.Result.ProtocolVersion == "" {
		t.Error("expected non-empty protocolVersion")
	}
}

func TestMCPToolListReturnsRegisteredTools(t *testing.T) {
	url := newMCPTestServer(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	resp := mcpPost(t, url, body, "access", "secret")
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var rpc struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found := false
	for _, tool := range rpc.Result.Tools {
		if tool.Name == "check-client" {
			found = true
		}
	}
	if !found {
		t.Error("expected check-client tool in tools/list response")
	}
}

func TestMCPToolCallReceivesPerRequestClient(t *testing.T) {
	url := newMCPTestServer(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check-client","arguments":{}}}`
	resp := mcpPost(t, url, body, "access", "secret")
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var rpc struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rpc.Result.Content) == 0 {
		t.Fatal("expected content in tool response")
	}

	var result struct {
		HasClient bool `json:"hasClient"`
	}
	if err := json.Unmarshal([]byte(rpc.Result.Content[0].Text), &result); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if !result.HasClient {
		t.Error("expected hasClient=true: per-request client was not injected into context")
	}
}

func TestMCPToolCallReturns401WithoutCredentials(t *testing.T) {
	url := newMCPTestServer(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check-client","arguments":{}}}`
	resp := mcpPost(t, url, body, "", "")
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", resp.StatusCode)
	}
}

// ---- No-auth methods (tools/list, initialize, ping) ----

func TestToolsListAllowedWithoutCredentials(t *testing.T) {
	url := newMCPTestServer(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	resp := mcpPost(t, url, body, "", "")
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for tools/list without credentials, got %d", resp.StatusCode)
	}

	var rpc struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rpc.Result.Tools) == 0 {
		t.Error("expected at least one tool in tools/list response")
	}
}

func TestInitializeAllowedWithoutCredentials(t *testing.T) {
	url := newMCPTestServer(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	resp := mcpPost(t, url, body, "", "")
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for initialize without credentials, got %d", resp.StatusCode)
	}

	var rpc struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rpc.Result.ProtocolVersion == "" {
		t.Error("expected non-empty protocolVersion in initialize response")
	}
}

func TestPingAllowedWithoutCredentials(t *testing.T) {
	tr, ts := newTestTransport(t)

	replied := make(chan struct{})
	tr.SetMessageHandler(func(ctx context.Context, msg *transport.BaseJsonRpcMessage) {
		if msg.JsonRpcRequest == nil {
			return
		}
		_ = tr.Send(ctx, &transport.BaseJsonRpcMessage{
			Type: transport.BaseMessageTypeJSONRPCResponseType,
			JsonRpcResponse: &transport.BaseJSONRPCResponse{
				Id:      msg.JsonRpcRequest.Id,
				Jsonrpc: "2.0",
				Result:  json.RawMessage(`{}`),
			},
		})
		close(replied)
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`
	resp := mcpPost(t, ts.URL, body, "", "")
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for ping without credentials, got %d", resp.StatusCode)
	}
	<-replied
}

func TestNoAuthMethodsDoNotInjectClientIntoContext(t *testing.T) {
	type testCase struct {
		method         string
		body           string
		expectStatus   int
		isNotification bool
	}
	cases := []testCase{
		{method: "tools/list", body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, expectStatus: http.StatusOK},
		{method: "initialize", body: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, expectStatus: http.StatusOK},
		{method: "ping", body: `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`, expectStatus: http.StatusOK},
		{method: "notifications/initialized", body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`, expectStatus: http.StatusAccepted, isNotification: true},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			tr, ts := newTestTransport(t)

			clientPresent := make(chan bool, 1)
			tr.SetMessageHandler(func(ctx context.Context, msg *transport.BaseJsonRpcMessage) {
				hasClient := clientFromContext(ctx) != nil
				if msg.JsonRpcRequest != nil {
					clientPresent <- hasClient
					_ = tr.Send(ctx, &transport.BaseJsonRpcMessage{
						Type: transport.BaseMessageTypeJSONRPCResponseType,
						JsonRpcResponse: &transport.BaseJSONRPCResponse{
							Id:      msg.JsonRpcRequest.Id,
							Jsonrpc: "2.0",
							Result:  json.RawMessage(`{}`),
						},
					})
				} else if msg.JsonRpcNotification != nil {
					clientPresent <- hasClient
				}
			})

			resp := mcpPost(t, ts.URL, tc.body, "", "")
			_ = resp.Body.Close()

			if resp.StatusCode != tc.expectStatus {
				t.Fatalf("expected %d, got %d", tc.expectStatus, resp.StatusCode)
			}
			if hasClient := <-clientPresent; hasClient {
				t.Error("expected nil client in context for no-auth method without credentials")
			}
		})
	}
}

func TestNotificationsInitializedAllowedWithoutCredentials(t *testing.T) {
	// notifications/initialized is a JSON-RPC notification (no id field).
	// The server must return 202 Accepted without requiring credentials.
	_, ts := newTestTransport(t)

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp := mcpPost(t, ts.URL, body, "", "")
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 for notifications/initialized without credentials, got %d", resp.StatusCode)
	}
}

func TestMCPConnectHandshakeWithoutCredentials(t *testing.T) {
	// Simulates the two-step MCP connection handshake a client performs:
	// 1. initialize request  → 200 with server capabilities
	// 2. notifications/initialized notification → 202 no body
	// Both must succeed without credentials.
	url := newMCPTestServer(t)

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	initResp := mcpPost(t, url, initBody, "", "")
	t.Cleanup(func() { _ = initResp.Body.Close() })
	if initResp.StatusCode != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d", initResp.StatusCode)
	}

	notifBody := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	notifResp := mcpPost(t, url, notifBody, "", "")
	_ = notifResp.Body.Close()
	if notifResp.StatusCode != http.StatusAccepted {
		t.Fatalf("notifications/initialized: expected 202, got %d", notifResp.StatusCode)
	}
}

func TestNoAuthMethodsInjectClientWhenCredentialsProvided(t *testing.T) {
	type testCase struct {
		method       string
		body         string
		expectStatus int
	}
	cases := []testCase{
		{method: "tools/list", body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, expectStatus: http.StatusOK},
		{method: "initialize", body: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, expectStatus: http.StatusOK},
		{method: "ping", body: `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`, expectStatus: http.StatusOK},
		{method: "notifications/initialized", body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`, expectStatus: http.StatusAccepted},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			tr, ts := newTestTransport(t)

			clientPresent := make(chan bool, 1)
			tr.SetMessageHandler(func(ctx context.Context, msg *transport.BaseJsonRpcMessage) {
				hasClient := clientFromContext(ctx) != nil
				if msg.JsonRpcRequest != nil {
					clientPresent <- hasClient
					_ = tr.Send(ctx, &transport.BaseJsonRpcMessage{
						Type: transport.BaseMessageTypeJSONRPCResponseType,
						JsonRpcResponse: &transport.BaseJSONRPCResponse{
							Id:      msg.JsonRpcRequest.Id,
							Jsonrpc: "2.0",
							Result:  json.RawMessage(`{}`),
						},
					})
				} else if msg.JsonRpcNotification != nil {
					clientPresent <- hasClient
				}
			})

			resp := mcpPost(t, ts.URL, tc.body, "access-key", "secret-key")
			_ = resp.Body.Close()

			if resp.StatusCode != tc.expectStatus {
				t.Fatalf("expected %d, got %d", tc.expectStatus, resp.StatusCode)
			}
			if hasClient := <-clientPresent; !hasClient {
				t.Error("expected non-nil client in context when credentials are provided")
			}
		})
	}
}
