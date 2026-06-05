package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/itera-io/taikungoclient"
)

type ctxKey int

const (
	ctxKeyClient ctxKey = iota
	ctxKeyCredentialKey
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

// contextWithCredentialKey stores a stable, non-reversible identity for the
// per-request credentials so caches (robot-user context, clients) can be keyed
// without retaining the raw secret in cache keys.
func contextWithCredentialKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ctxKeyCredentialKey, key)
}

// credentialKeyFromContext returns the credential identity stored in ctx, or ""
// when none is present (stdio transport / global client).
func credentialKeyFromContext(ctx context.Context) string {
	if key, ok := ctx.Value(ctxKeyCredentialKey).(string); ok {
		return key
	}
	return ""
}

// credentialKeySalt is a per-process random salt used when deriving the cache
// identity from credentials. Generating it once per process keeps the derived
// identity stable for the lifetime of the process (so caches hit) while making
// the derivation non-reproducible across restarts or other processes.
var credentialKeySalt = newCredentialKeySalt()

func newCredentialKeySalt() []byte {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand failure is effectively impossible on supported platforms.
		// The derived value is only an in-memory cache identity (not stored
		// password material), so a fixed fallback salt is acceptable here.
		return []byte("ccf-mcp-credential-cache-salt")
	}
	return salt
}

// credentialCacheKey derives a stable, non-reversible identity for a set of
// credentials. Used to key per-credential caches (robot context, reusable
// clients) so we do not store raw secrets as map keys. PBKDF2 (a salted,
// computationally expensive KDF) is used rather than a bare hash so the
// derived identity does not expose the underlying secret to brute-force search.
func credentialCacheKey(accessKey, secretKey, apiHost string) string {
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	apiHost = strings.TrimSpace(apiHost)
	if apiHost == "" {
		apiHost = defaultAPIHost
	}
	material := accessKey + "\x00" + secretKey + "\x00" + apiHost
	key, err := pbkdf2.Key(sha256.New, material, credentialKeySalt, 4096, 32)
	if err != nil {
		// pbkdf2.Key only errors on invalid parameters, which are fixed and
		// valid here, so this branch is unreachable in practice.
		panic(fmt.Sprintf("credentialCacheKey: pbkdf2 derivation failed: %v", err))
	}
	return hex.EncodeToString(key)
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

// Reusable CCF client cache for the HTTP/per-request transport. The upstream
// taikungoclient creates a fresh http.Transport per client, so building a new
// client for every request prevents HTTP keep-alive reuse and forces a TLS
// handshake on each tool call. Caching clients per credential identity lets the
// agent's many sequential tool calls share connections. Bounded by size + TTL
// so we neither grow without limit nor retain credentials indefinitely.
type cachedTaikunClient struct {
	client    *taikungoclient.Client
	expiresAt time.Time
	lastUsed  time.Time
}

const (
	clientCacheTTL     = 10 * time.Minute
	clientCacheMaxSize = 256
)

var (
	clientCacheMu sync.Mutex
	clientCache   = make(map[string]*cachedTaikunClient)
)

// getOrCreateTaikunClient returns a cached client for the given credentials when
// available (refreshing its TTL) or creates, caches, and returns a new one.
func getOrCreateTaikunClient(accessKey, secretKey, apiHost string) (*taikungoclient.Client, error) {
	key := credentialCacheKey(accessKey, secretKey, apiHost)

	clientCacheMu.Lock()
	if entry, ok := clientCache[key]; ok && time.Now().Before(entry.expiresAt) {
		now := time.Now()
		entry.lastUsed = now
		entry.expiresAt = now.Add(clientCacheTTL)
		client := entry.client
		clientCacheMu.Unlock()
		return client, nil
	}
	clientCacheMu.Unlock()

	// Build (and validate credentials) outside the lock.
	client, err := createTaikunClientFromCreds(accessKey, secretKey, apiHost)
	if err != nil {
		return nil, err
	}

	clientCacheMu.Lock()
	defer clientCacheMu.Unlock()
	// A concurrent request may have populated the entry meanwhile; reuse it so
	// connection pools are not fragmented.
	if entry, ok := clientCache[key]; ok && time.Now().Before(entry.expiresAt) {
		now := time.Now()
		entry.lastUsed = now
		entry.expiresAt = now.Add(clientCacheTTL)
		return entry.client, nil
	}
	evictClientCacheLocked()
	now := time.Now()
	clientCache[key] = &cachedTaikunClient{
		client:    client,
		expiresAt: now.Add(clientCacheTTL),
		lastUsed:  now,
	}
	return client, nil
}

// evictClientCacheLocked bounds the cache: it first drops expired entries and,
// if still at capacity, evicts the least-recently-used entry. Caller holds the lock.
func evictClientCacheLocked() {
	if len(clientCache) < clientCacheMaxSize {
		return
	}
	now := time.Now()
	for k, v := range clientCache {
		if now.After(v.expiresAt) {
			delete(clientCache, k)
		}
	}
	if len(clientCache) < clientCacheMaxSize {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, v := range clientCache {
		if oldestKey == "" || v.lastUsed.Before(oldest) {
			oldestKey = k
			oldest = v.lastUsed
		}
	}
	if oldestKey != "" {
		delete(clientCache, oldestKey)
	}
}
