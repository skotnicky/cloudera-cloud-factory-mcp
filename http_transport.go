package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/itera-io/taikungoclient"
	"github.com/metoro-io/mcp-golang/transport"
)

// authHTTPTransport is a stateless HTTP MCP transport that extracts CCF
// credentials from X-CCF-Access-Key / X-CCF-Secret-Key request headers and
// injects a per-request taikungoclient.Client into the handler context.
// It implements transport.Transport from github.com/metoro-io/mcp-golang.
type authHTTPTransport struct {
	mu             sync.RWMutex
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()

	respMu      sync.Mutex
	responseMap map[int64]chan *transport.BaseJsonRpcMessage
	nextKey     atomic.Int64

	server   *http.Server
	addr     string
	endpoint string
}

func newAuthHTTPTransport(addr, endpoint string) *authHTTPTransport {
	return &authHTTPTransport{
		addr:        addr,
		endpoint:    endpoint,
		responseMap: make(map[int64]chan *transport.BaseJsonRpcMessage),
	}
}

// Start satisfies transport.Transport. It is called by mcp_golang.Server.Serve()
// via protocol.Connect — it must return quickly so Serve() can complete tool
// registration. Actual HTTP listening is started separately via ListenAndServe.
func (t *authHTTPTransport) Start(_ context.Context) error { return nil }

// ListenAndServe starts the HTTP server and blocks until it is closed.
// Call this after server.Serve() has returned.
func (t *authHTTPTransport) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc(t.endpoint, t.handleRequest)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	t.server = &http.Server{Addr: t.addr, Handler: mux}
	host := t.addr
	if len(host) > 0 && host[0] == ':' {
		host = "localhost" + host
	}
	msg := fmt.Sprintf("MCP HTTP server listening — endpoint: http://%s%s  health: http://%s/health", host, t.endpoint, host)
	logger.Println(msg)
	fmt.Fprintln(os.Stderr, msg)
	return t.server.ListenAndServe()
}

// noAuthMethods lists JSON-RPC methods that do not require CCF credentials.
var noAuthMethods = map[string]bool{
	"initialize":               true,
	"ping":                     true,
	"tools/list":               true,
	"notifications/initialized": true,
}

func (t *authHTTPTransport) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var peek struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &peek)

	accessKey := r.Header.Get("X-CCF-Access-Key")
	secretKey := r.Header.Get("X-CCF-Secret-Key")
	credentialsProvided := accessKey != "" || secretKey != ""

	// Also check for Bearer token auth
	bearerToken := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		bearerToken = strings.TrimPrefix(authHeader, "Bearer ")
	}

	var ctx = r.Context()
	if credentialsProvided || bearerToken != "" || !noAuthMethods[peek.Method] {
		var client *taikungoclient.Client
		var err error
		apiHost := r.Header.Get("X-CCF-Api-Host")
		if apiHost == "" {
			apiHost = defaultAPIHost
		}

		if credentialsProvided {
			client, err = createTaikunClientFromCreds(accessKey, secretKey, apiHost)
		} else if bearerToken != "" {
			client = taikungoclient.NewClientFromToken(bearerToken, apiHost)
		} else {
			err = fmt.Errorf("missing credentials: provide X-CCF-Access-Key/X-CCF-Secret-Key or Authorization: Bearer <token>")
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		ctx = contextWithClient(ctx, client)
		if bearerToken != "" && !credentialsProvided {
			ctx = contextWithSkipRobotScope(ctx)
		}
	}

	response, err := t.processMessage(ctx, body)
	if err != nil {
		if t.errorHandler != nil {
			t.errorHandler(err)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if response == nil {
		// Notification — no response body expected
		w.WriteHeader(http.StatusAccepted)
		return
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(jsonData); err != nil && t.errorHandler != nil {
		t.errorHandler(fmt.Errorf("failed to write response: %w", err))
	}
}

// processMessage parses the raw JSON-RPC body, routes it through the MCP
// message handler, and returns the response (nil for notifications).
func (t *authHTTPTransport) processMessage(ctx context.Context, body []byte) (*transport.BaseJsonRpcMessage, error) {
	t.mu.RLock()
	handler := t.messageHandler
	t.mu.RUnlock()

	// Try to parse as a request (has id, jsonrpc, method).
	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(body, &request); err == nil {
		key := t.nextKey.Add(1)
		responseCh := make(chan *transport.BaseJsonRpcMessage, 1)
		t.respMu.Lock()
		t.responseMap[key] = responseCh
		t.respMu.Unlock()
		defer func() {
			t.respMu.Lock()
			delete(t.responseMap, key)
			t.respMu.Unlock()
		}()

		origID := request.Id
		request.Id = transport.RequestId(key)
		if handler != nil {
			handler(ctx, transport.NewBaseMessageRequest(&request))
		}

		response := <-responseCh
		if response.JsonRpcResponse != nil {
			response.JsonRpcResponse.Id = origID
		}
		if response.JsonRpcError != nil {
			response.JsonRpcError.Id = origID
		}
		return response, nil
	}

	// Try to parse as a notification (has jsonrpc, method; no id).
	var notification transport.BaseJSONRPCNotification
	if err := json.Unmarshal(body, &notification); err == nil {
		if handler != nil {
			handler(ctx, transport.NewBaseMessageNotification(&notification))
		}
		return nil, nil
	}

	return nil, fmt.Errorf("could not parse MCP message body")
}

func (t *authHTTPTransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	var key transport.RequestId
	switch message.Type {
	case transport.BaseMessageTypeJSONRPCResponseType:
		if message.JsonRpcResponse == nil {
			return fmt.Errorf("Send: response type with nil JsonRpcResponse")
		}
		key = message.JsonRpcResponse.Id
	case transport.BaseMessageTypeJSONRPCErrorType:
		if message.JsonRpcError == nil {
			return fmt.Errorf("Send: error type with nil JsonRpcError")
		}
		key = message.JsonRpcError.Id
	default:
		return fmt.Errorf("Send: unexpected message type %q", message.Type)
	}

	t.respMu.Lock()
	ch, ok := t.responseMap[int64(key)]
	t.respMu.Unlock()

	if !ok {
		return fmt.Errorf("Send: no response channel for key %d", key)
	}
	ch <- message
	return nil
}

func (t *authHTTPTransport) Close() error {
	if t.server != nil {
		err := t.server.Close()
		if t.closeHandler != nil {
			t.closeHandler()
		}
		return err
	}
	return nil
}

func (t *authHTTPTransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

func (t *authHTTPTransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

func (t *authHTTPTransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}
