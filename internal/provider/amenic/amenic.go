// Package amenic is a stub provider for amenic-file.com.
//
// ⚠️ NOTE: This is a placeholder implementation. The full Amenic Plus
// provider was never committed to this repository (the import exists in
// cmd/server/main.go but the package files were missing). The stub exists
// so that `go build ./...` and `go mod tidy` succeed without modifying
// the existing server entry point.
//
// When AmenicEnabled is true, the stub returns ErrProviderDown from
// every Resolve/HealthCheck call — the registry will fall through to
// the next provider (Supercine). This preserves the existing behavior
// described in the README's troubleshooting section: "When Cloudflare
// blocks the request, the provider returns ErrProviderDown and the
// registry moves on to the next provider."
//
// To implement the real provider, replace this file with the full
// implementation (HTTP client + HTML parsing + extractor dispatch,
// equivalent to internal/provider/supercine/supercine.go).
package amenic

import (
	"context"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/extractors"
	"github.com/deivid22srk/supercine-proxy/internal/provider"
)

// ProviderConfig holds the upstream connection settings for amenic-file.com.
// These fields mirror the structure of supercine.ProviderConfig and are
// populated from config.Config in cmd/server/main.go.
type ProviderConfig struct {
	// BaseURL is the Amenic file server root (e.g. https://amenic-file.com).
	BaseURL string

	// ThumbBase is the thumbnail/asset CDN root.
	ThumbBase string

	// AppVersion is sent in the `v` query parameter to amenic-file.com.
	AppVersion string

	// DeviceID is sent in the `r` query parameter.
	DeviceID string

	// UserAgent is sent on every outbound request.
	UserAgent string

	// HTTPTimeout is the per-request HTTP timeout.
	HTTPTimeout time.Duration
}

// AmenicProvider implements provider.Provider as a stub.
type AmenicProvider struct {
	cfg      ProviderConfig
	registry *extractors.Registry
}

// New constructs an AmenicProvider. The registry parameter is kept for
// API symmetry with supercine.New even though the stub doesn't use it.
func New(cfg ProviderConfig, registry *extractors.Registry) *AmenicProvider {
	return &AmenicProvider{cfg: cfg, registry: registry}
}

// Name returns the provider identifier.
func (p *AmenicProvider) Name() string { return "amenic" }

// DisplayName returns the human-friendly label.
func (p *AmenicProvider) DisplayName() string { return "Amenic Plus (stub)" }

// Priority is 200 — higher than Supercine's 100, so Supercine is tried first.
func (p *AmenicProvider) Priority() int { return 200 }

// Resolve always returns ErrProviderDown — the stub cannot resolve titles.
// The registry will fall through to the next provider.
func (p *AmenicProvider) Resolve(ctx context.Context, imdbID, embedType string) (*provider.ResolveResult, error) {
	return nil, provider.ErrProviderDown
}

// HealthCheck always returns ErrProviderDown — the stub is never healthy.
func (p *AmenicProvider) HealthCheck(ctx context.Context) error {
	return provider.ErrProviderDown
}
