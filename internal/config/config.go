package config

import (
        "fmt"
        "os"
        "strconv"
        "time"
)

// Config holds all runtime configuration for the proxy.
type Config struct {
        // ListenAddr is the address the proxy HTTP server binds to.
        ListenAddr string

        // UpstreamBase is the WordPress REST root for supercine-tv.net.
        UpstreamBase string

        // EmbedBase is the HTML embed endpoint root (https://supercine-tv.net/embed-api/).
        EmbedBase string

        // UserAgent is sent on every outbound request to the upstream.
        UserAgent string

        // CacheTTL is how long a successful GET response is cached.
        CacheTTL time.Duration

        // CacheMaxEntries caps the in-memory cache size.
        CacheMaxEntries int

        // LogMaxEntries caps the in-memory log ring buffer.
        LogMaxEntries int

        // RequestTimeout is the per-request HTTP timeout when calling upstream.
        RequestTimeout time.Duration

        // AdminToken, when non-empty, gates the /admin endpoints (clear-cache, etc).
        AdminToken string

        // Verbose toggles verbose request logging.
        Verbose bool

        // TMDBAPIKey, when set, enables /v1/tmdb/* search/discovery endpoints
        // used by the streaming UI. Get one for free at
        // https://www.themoviedb.org/settings/api
        TMDBAPIKey string

        // TMDBBase is the TMDB REST root.
        TMDBBase string

        // AmenicEnabled controls whether the Amenic Plus provider is
        // registered. The provider is off by default because
        // amenic-file.com is behind Cloudflare's managed challenge,
        // which blocks datacenter IPs (like Render's). Enable only if
        // the proxy runs from a residential IP.
        AmenicEnabled bool

        // AmenicBase is the Amenic file server root.
        AmenicBase string

        // AmenicThumbBase is the thumbnail/asset CDN root.
        AmenicThumbBase string

        // AmenicAppVersion is the version string sent in the `v` query
        // parameter to amenic-file.com.
        AmenicAppVersion string

        // AmenicDeviceID is sent in the `r` query parameter.
        AmenicDeviceID string
}

// Default returns a Config populated from env vars with sensible defaults.
func Default() *Config {
        return &Config{
                ListenAddr:      envStr("LISTEN_ADDR", ":8080"),
                UpstreamBase:    envStr("UPSTREAM_BASE", "https://supercine-tv.net/wp-json"),
                EmbedBase:       envStr("EMBED_BASE", "https://supercine-tv.net/embed-api/"),
                UserAgent:       envStr("USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"),
                CacheTTL:        envDuration("CACHE_TTL", 5*time.Minute),
                CacheMaxEntries: envInt("CACHE_MAX_ENTRIES", 1000),
                LogMaxEntries:   envInt("LOG_MAX_ENTRIES", 500),
                RequestTimeout:  envDuration("REQUEST_TIMEOUT", 20*time.Second),
                AdminToken:      envStr("ADMIN_TOKEN", ""),
                Verbose:         envBool("VERBOSE", false),
                TMDBAPIKey:      envStr("TMDB_API_KEY", ""),
                TMDBBase:        envStr("TMDB_BASE", "https://api.themoviedb.org/3"),
                AmenicEnabled:   envBool("AMENIC_ENABLED", false),
                AmenicBase:      envStr("AMENIC_BASE", "https://amenic-file.com"),
                AmenicThumbBase: envStr("AMENIC_THUMB_BASE", "https://thumb.fvs.io/asset"),
                AmenicAppVersion: envStr("AMENIC_APP_VERSION", "1.7.3"),
                AmenicDeviceID:   envStr("AMENIC_DEVICE_ID", "supercine-proxy-amenic"),
        }
}

// String returns a human-readable summary for the startup banner.
func (c *Config) String() string {
        return fmt.Sprintf(
                "listen=%s upstream=%s embed=%s cache_ttl=%s cache_max=%d log_max=%d timeout=%s admin_token_set=%v verbose=%v tmdb_key_set=%v amenic_enabled=%v",
                c.ListenAddr, c.UpstreamBase, c.EmbedBase,
                c.CacheTTL, c.CacheMaxEntries, c.LogMaxEntries,
                c.RequestTimeout, c.AdminToken != "", c.Verbose, c.TMDBAPIKey != "",
                c.AmenicEnabled,
        )
}

func envStr(key, def string) string {
        if v := os.Getenv(key); v != "" {
                return v
        }
        return def
}

func envInt(key string, def int) int {
        if v := os.Getenv(key); v != "" {
                if n, err := strconv.Atoi(v); err == nil {
                        return n
                }
        }
        return def
}

func envBool(key string, def bool) bool {
        if v := os.Getenv(key); v != "" {
                if b, err := strconv.ParseBool(v); err == nil {
                        return b
                }
        }
        return def
}

func envDuration(key string, def time.Duration) time.Duration {
        if v := os.Getenv(key); v != "" {
                if d, err := time.ParseDuration(v); err == nil {
                        return d
                }
        }
        return def
}
