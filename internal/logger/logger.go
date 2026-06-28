package logger

import (
	"sync"
	"time"

	"github.com/deivid22srk/supercine-proxy/internal/types"
	"github.com/google/uuid"
)

// Logger is an in-memory ring buffer of recent proxied requests.
// It is concurrency-safe and is used both by the dashboard and for stats.
type Logger struct {
	mu       sync.Mutex
	entries  []types.LogEntry
	max      int
	stats    types.Stats
	statsMu  sync.RWMutex
}

// New creates a new Logger with a fixed capacity.
func New(max int) *Logger {
	if max <= 0 {
		max = 500
	}
	return &Logger{
		entries: make([]types.LogEntry, 0, max),
		max:     max,
		stats: types.Stats{
			ByStatus: make(map[string]int64),
			ByPath:   make(map[string]int64),
		},
	}
}

// Append adds a new log entry and returns its assigned ID.
func (l *Logger) Append(method, path, upstream string, statusCode int, duration time.Duration, size int64, cached bool, errStr string) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	id := uuid.NewString()
	entry := types.LogEntry{
		ID:         id,
		Timestamp:  time.Now(),
		Method:     method,
		Path:       path,
		Upstream:   upstream,
		StatusCode: statusCode,
		Duration:   duration.String(),
		Size:       size,
		Cached:     cached,
		Error:      errStr,
	}

	if len(l.entries) >= l.max {
		// Drop oldest by shifting — fine for small max values.
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry)

	// Update stats
	l.statsMu.Lock()
	defer l.statsMu.Unlock()
	l.stats.TotalRequests++
	if cached {
		l.stats.CacheHits++
	} else {
		l.stats.CacheMisses++
	}
	if statusCode >= 400 || errStr != "" {
		l.stats.Errors++
	}
	l.stats.BytesProxied += size
	statusKey := statusClass(statusCode)
	l.stats.ByStatus[statusKey]++
	l.stats.ByPath[path]++

	return id
}

// Entries returns a reversed copy (most recent first) of the log entries.
func (l *Logger) Entries(limit int) []types.LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}
	out := make([]types.LogEntry, limit)
	for i := 0; i < limit; i++ {
		out[i] = l.entries[len(l.entries)-1-i]
	}
	return out
}

// Stats returns a snapshot of aggregate stats.
func (l *Logger) Stats() types.Stats {
	l.statsMu.RLock()
	defer l.statsMu.RUnlock()
	// Deep-copy maps so caller can't mutate us
	cp := l.stats
	cp.ByStatus = make(map[string]int64, len(l.stats.ByStatus))
	for k, v := range l.stats.ByStatus {
		cp.ByStatus[k] = v
	}
	cp.ByPath = make(map[string]int64, len(l.stats.ByPath))
	for k, v := range l.stats.ByPath {
		cp.ByPath[k] = v
	}
	return cp
}

// IncExtractor increments extractor call counters.
func (l *Logger) IncExtractor(isError bool) {
	l.statsMu.Lock()
	defer l.statsMu.Unlock()
	l.stats.ExtractorCalls++
	if isError {
		l.stats.ExtractorErrors++
	}
}

// Reset clears all logs and stats.
func (l *Logger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statsMu.Lock()
	defer l.statsMu.Unlock()

	l.entries = l.entries[:0]
	l.stats = types.Stats{
		ByStatus: make(map[string]int64),
		ByPath:   make(map[string]int64),
	}
}

func statusClass(code int) string {
	switch {
	case code == 0:
		return "0 (no-response)"
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}
