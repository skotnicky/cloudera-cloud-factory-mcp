package main

import (
	"context"
	"strings"
	"testing"

	"github.com/itera-io/taikungoclient"
)

// makeClient is a helper that creates a non-nil taikungoclient for use in tests.
// The credentials are fake — no network calls are made just from creating a client.
func makeClient(accessKey string) *taikungoclient.Client {
	return taikungoclient.NewClientFromAccessKey("", accessKey, "secret", defaultAPIHost)
}

// ---- createTaikunClientFromCreds ----

func TestCreateTaikunClientFromCredsRequiresBothKeys(t *testing.T) {
	cases := []struct {
		name      string
		accessKey string
		secretKey string
	}{
		{"missing access key", "", "secret"},
		{"missing secret key", "access", ""},
		{"both missing", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createTaikunClientFromCreds(tc.accessKey, tc.secretKey, "")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCreateTaikunClientFromCredsSucceeds(t *testing.T) {
	client, err := createTaikunClientFromCreds("access", "secret", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestCreateTaikunClientFromCredsDefaultsAPIHost(t *testing.T) {
	// No error means the default host was applied (empty string triggers fallback)
	_, err := createTaikunClientFromCreds("access", "secret", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateTaikunClientFromCredsUsesCustomAPIHost(t *testing.T) {
	// Custom host is accepted without error
	_, err := createTaikunClientFromCreds("access", "secret", "api.custom.example.com")
	if err != nil {
		t.Fatalf("unexpected error with custom host: %v", err)
	}
}

func TestCreateTaikunClientFromCredsTrimsWhitespace(t *testing.T) {
	_, err := createTaikunClientFromCreds("  access  ", "  secret  ", "  api.example.com  ")
	if err != nil {
		t.Fatalf("expected whitespace to be trimmed, got error: %v", err)
	}
}

func TestCreateTaikunClientFromCredsErrorMentionsMissingHeader(t *testing.T) {
	_, err := createTaikunClientFromCreds("", "", "")
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
	if !strings.Contains(err.Error(), "X-CCF-Access-Key") || !strings.Contains(err.Error(), "X-CCF-Secret-Key") {
		t.Errorf("expected error to mention header names, got: %v", err)
	}
}

// ---- contextWithClient / clientFromContext ----

func TestContextWithClientRoundTrip(t *testing.T) {
	client := makeClient("test-access")
	ctx := contextWithClient(context.Background(), client)

	got := clientFromContext(ctx)
	if got != client {
		t.Fatalf("expected to retrieve the same client pointer, got different value")
	}
}

func TestClientFromContextFallsBackToGlobal(t *testing.T) {
	// When no client is stored in context, clientFromContext returns the global.
	prev := taikunClient
	sentinel := makeClient("global")
	taikunClient = sentinel
	defer func() { taikunClient = prev }()

	got := clientFromContext(context.Background())
	if got != sentinel {
		t.Fatal("expected global taikunClient as fallback when context has no client")
	}
}

func TestClientFromContextPreferContextOverGlobal(t *testing.T) {
	// Per-request client takes priority over the global.
	perRequest := makeClient("per-request")
	global := makeClient("global")

	prev := taikunClient
	taikunClient = global
	defer func() { taikunClient = prev }()

	ctx := contextWithClient(context.Background(), perRequest)
	got := clientFromContext(ctx)
	if got != perRequest {
		t.Fatal("expected per-request client to override global taikunClient")
	}
}

func TestClientFromContextNilClientFallsBackToGlobal(t *testing.T) {
	// Explicitly storing nil should still fall back to the global.
	sentinel := makeClient("global")
	prev := taikunClient
	taikunClient = sentinel
	defer func() { taikunClient = prev }()

	ctx := context.WithValue(context.Background(), ctxKeyClient, (*taikungoclient.Client)(nil))
	got := clientFromContext(ctx)
	if got != sentinel {
		t.Fatal("expected nil context value to fall back to global taikunClient")
	}
}

// ---- contextWithSkipRobotScope / shouldSkipRobotScope ----

func TestShouldSkipRobotScopeDefaultFalse(t *testing.T) {
	if shouldSkipRobotScope(context.Background()) {
		t.Error("expected shouldSkipRobotScope=false on a bare context")
	}
}

func TestContextWithSkipRobotScopeRoundTrip(t *testing.T) {
	ctx := contextWithSkipRobotScope(context.Background())
	if !shouldSkipRobotScope(ctx) {
		t.Error("expected shouldSkipRobotScope=true after contextWithSkipRobotScope")
	}
}

func TestSkipRobotScopeDoesNotAffectClientKey(t *testing.T) {
	client := makeClient("test")
	ctx := contextWithClient(context.Background(), client)
	ctx = contextWithSkipRobotScope(ctx)

	if clientFromContext(ctx) != client {
		t.Error("setting skipRobotScope should not interfere with client context key")
	}
	if !shouldSkipRobotScope(ctx) {
		t.Error("expected skipRobotScope=true")
	}
}
