package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/itera-io/taikungoclient"
)

type ctxKey int

const (
	ctxKeyClient ctxKey = iota
	ctxKeySkipRobotScope
)

// clientFromContext returns the per-request CCF client stored in ctx.
// Falls back to the global taikunClient (used in stdio transport mode).
func clientFromContext(ctx context.Context) *taikungoclient.Client {
	if c, ok := ctx.Value(ctxKeyClient).(*taikungoclient.Client); ok && c != nil {
		return c
	}
	return taikunClient
}

func contextWithClient(ctx context.Context, client *taikungoclient.Client) context.Context {
	return context.WithValue(ctx, ctxKeyClient, client)
}

func contextWithSkipRobotScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipRobotScope, true)
}

func shouldSkipRobotScope(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySkipRobotScope).(bool)
	return v
}

// createTaikunClientFromCreds validates credentials and returns a new CCF client.
func createTaikunClientFromCreds(accessKey, secretKey, apiHost string) (*taikungoclient.Client, error) {
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	apiHost = strings.TrimSpace(apiHost)
	if apiHost == "" {
		apiHost = defaultAPIHost
	}
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("missing CCF credentials: provide X-CCF-Access-Key and X-CCF-Secret-Key headers")
	}
	return taikungoclient.NewClientFromAccessKey("", accessKey, secretKey, apiHost), nil
}
