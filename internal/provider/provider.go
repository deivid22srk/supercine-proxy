// Package provider defines the abstraction that lets the streaming UI
// resolve any IMDB ID to a playable video URL, regardless of which
// upstream service is actually serving the content.
//
// Today only the Supercine provider is registered, but the architecture
// is intentionally provider-agnostic: adding a new provider (e.g. a
// second streaming site, a torrent indexer, a self-hosted Jellyfin) is
// just a matter of implementing the Provider interface and registering
// it in the Registry.
//
// The flow is:
//
//   IMDB ID  ──┐
//              ├─▶ Registry.Resolve(imdb, type) ──▶ ResolveResult
//   type     ──┘                                       │
//                                                       ▼
//                                              Direct video URL
//                                              (mp4 / m3u8 / mkv)
//
// The Registry tries each registered provider in priority order until
// one returns at least one playable URL. This is what gives the
// "always returns the same output" behaviour: the UI doesn't care
// which provider served the video, it just gets a URL to play.
package provider

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Provider is the interface every upstream service must implement.
type Provider interface {
	// Name is a short identifier used in logs and the /v1/providers endpoint.
	// Examples: "supercine", "megahdfilmes", "jellyfin".
	Name() string

	// DisplayName is a human-friendly label shown in the UI.
	// Examples: "Supercine", "Mega HD Filmes", "Jellyfin (local)".
	DisplayName() string

	// Priority controls the order providers are tried. Lower = higher priority.
	// The Supercine provider is 100. New providers should pick a number that
	// reflects their reliability/speed relative to Supercine.
	Priority() int

	// Resolve takes an IMDB ID and a content type ("movies" or "tvshows")
	// and returns a list of playable video URLs (mp4/m3u8/mkv) plus the
	// list of "servers" the user can pick from (each server is one
	// alternative source within the provider).
	//
	// Implementations should:
	//   - Be context-aware (respect ctx cancellation)
	//   - Return ErrUnavailable if the provider doesn't have the title
	//   - Return ErrProviderDown if the provider itself is unreachable
	Resolve(ctx context.Context, imdbID, embedType string) (*ResolveResult, error)

	// HealthCheck pings the provider to see if it's currently reachable.
	// Used by /v1/providers to show status in the UI.
	HealthCheck(ctx context.Context) error
}

// Server is one playable source within a provider. A provider typically
// returns 1–6 servers per title (e.g. "Player Principal", "Player Alternativo").
type Server struct {
	Index       int    `json:"index"`       // 0-based position in the provider's list
	Name        string `json:"name"`        // e.g. "Player Principal"
	Description string `json:"description"` // e.g. "Velocidade ok e poucos anúncios"
}

// VideoURL is a single resolved direct video URL with its quality label.
type VideoURL struct {
	URL     string `json:"url"`
	Quality string `json:"quality"` // "Normal", "MP4 Video", "HLS (m3u8)", etc.
}

// ResolveResult is what a provider returns for a single IMDB ID.
type ResolveResult struct {
	Provider string     `json:"provider"` // e.g. "supercine"
	IMDB     string     `json:"imdb"`
	Type     string     `json:"type"` // "movies" or "tvshows"
	Servers  []Server   `json:"servers"`
	Videos   []VideoURL `json:"videos"`
}

// Sentinel errors. Use errors.Is() to check.
var (
	ErrUnavailable = fmt.Errorf("provider: title not available")
	ErrProviderDown = fmt.Errorf("provider: upstream unreachable")
	ErrNoProvider   = fmt.Errorf("provider: no provider registered for this request")
)

// Registry holds all known providers sorted by priority.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider. If a provider with the same Name() is already
// registered, it is replaced.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Get returns a provider by name, or nil if not registered.
func (r *Registry) Get(name string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[name]
}

// All returns all registered providers, sorted by priority (ascending).
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Priority() < out[j].Priority()
	})
	return out
}

// Resolve tries each provider in priority order until one succeeds.
// If preferredProvider is non-empty, that provider is tried first; if it
// fails or doesn't have the title, the remaining providers are tried in
// priority order.
//
// Returns ErrNoProvider if the registry is empty.
func (r *Registry) Resolve(ctx context.Context, imdbID, embedType, preferredProvider string) (*ResolveResult, error) {
	providers := r.All()
	if len(providers) == 0 {
		return nil, ErrNoProvider
	}

	// If a preferred provider is specified, try it first.
	if preferredProvider != "" {
		if p := r.Get(preferredProvider); p != nil {
			res, err := p.Resolve(ctx, imdbID, embedType)
			if err == nil && res != nil && len(res.Videos) > 0 {
				return res, nil
			}
		}
	}

	// Try each provider in priority order.
	var lastErr error
	for _, p := range providers {
		if p.Name() == preferredProvider {
			continue // already tried
		}
		res, err := p.Resolve(ctx, imdbID, embedType)
		if err == nil && res != nil && len(res.Videos) > 0 {
			return res, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrUnavailable
}

// Info is a lightweight provider descriptor for the /v1/providers endpoint.
type Info struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Priority    int    `json:"priority"`
	Healthy     bool   `json:"healthy"`
}

// Infos returns a snapshot of all providers with their current health status.
// The health check is done in parallel with a short timeout.
func (r *Registry) Infos(ctx context.Context) []Info {
	providers := r.All()
	out := make([]Info, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(idx int, prov Provider) {
			defer wg.Done()
			out[idx] = Info{
				Name:        prov.Name(),
				DisplayName: prov.DisplayName(),
				Priority:    prov.Priority(),
				Healthy:     prov.HealthCheck(ctx) == nil,
			}
		}(i, p)
	}
	wg.Wait()
	return out
}
