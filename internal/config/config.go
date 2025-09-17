package config

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxBatchBlocks       = 20000
	minBatchBlocks       = 1
	maxRateLimit         = 200
	minRateLimit         = 0
	maxSyncConfirmations = 576 // ~2 days at 15s blocks
	minSyncConfirmations = 0
	minIngestTimeout     = 100 * time.Millisecond
	maxIngestTimeout     = 30 * time.Minute
)

// Config holds 12-factor environment configuration used across binaries.
type Config struct {
	ProviderURL       string
	ClickHouseDSN     string
	SyncConfirmations int
	BatchBlocks       int
	RateLimit         int
	RedisURL          string
	EmbeddingModel    string
	Timeout           time.Duration
	HTTPRetries       int
	HTTPBackoffBase   time.Duration
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseIntEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	return def
}

func parseDurEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampDuration(v, min, max time.Duration) time.Duration {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// BuildClickHouseDSN assembles a ClickHouse DSN from individual env vars if provided.
// Prefers CLICKHOUSE_DSN if set; otherwise tries CLICKHOUSE_URL/DB/USER/PASS.
func BuildClickHouseDSN() string {
	if dsn := env("CLICKHOUSE_DSN", ""); dsn != "" {
		return dsn
	}
	base := env("CLICKHOUSE_URL", "") // e.g., http://localhost:8123
	db := env("CLICKHOUSE_DB", "")
	user := env("CLICKHOUSE_USER", "")
	pass := env("CLICKHOUSE_PASS", "")
	if base == "" || db == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err == nil {
		if user != "" {
			if pass != "" {
				u.User = url.UserPassword(user, pass)
			} else {
				u.User = url.User(user)
			}
		}
		// Normalize path and append db only when missing
		p := strings.TrimRight(u.Path, "/")
		switch {
		case p == "":
			u.Path = "/" + db
		case strings.HasSuffix(p, "/"+db):
			// already includes db; leave as-is
			u.Path = p
		default:
			u.Path = p + "/" + db
		}
		return u.String()
	}
	// Fallback for unparsable base URL
	base = strings.TrimRight(base, "/")
	return base + "/" + db
}

// RedactDSN hides credentials in DSN-like URLs to avoid logging secrets.
func RedactDSN(s string) string {
	if s == "" {
		return s
	}
	if u, err := url.Parse(s); err == nil {
		if u.User != nil {
			name := u.User.Username()
			if name != "" {
				u.User = url.UserPassword(name, "***")
			} else {
				u.User = url.User("***")
			}
			return u.String()
		}
		// Best-effort fallback even if parse succeeded but user info was not parsed
		if i := strings.Index(s, "//"); i >= 0 {
			j := strings.Index(s[i+2:], "@")
			if j > 0 {
				prefix := s[:i+2]
				creds := s[i+2 : i+2+j]
				if strings.Contains(creds, ":") {
					user := strings.SplitN(creds, ":", 2)[0]
					return prefix + user + ":***@" + s[i+2+j+1:]
				}
			}
		}
		return s
	}
	if i := strings.Index(s, "//"); i >= 0 {
		j := strings.Index(s[i+2:], "@")
		if j > 0 {
			prefix := s[:i+2]
			creds := s[i+2 : i+2+j]
			if strings.Contains(creds, ":") {
				user := strings.SplitN(creds, ":", 2)[0]
				return prefix + user + ":***@" + s[i+2+j+1:]
			}
		}
	}
	return s
}

// Load reads environment variables and returns a Config with defaults applied.
func Load() Config {
	syncConf := clampInt(parseIntEnv("SYNC_CONFIRMATIONS", 12), minSyncConfirmations, maxSyncConfirmations)
	batch := clampInt(parseIntEnv("BATCH_BLOCKS", 5000), minBatchBlocks, maxBatchBlocks)
	rateLimit := clampInt(parseIntEnv("RATE_LIMIT", 0), minRateLimit, maxRateLimit)
	timeout := clampDuration(parseDurEnv("INGEST_TIMEOUT", 30*time.Second), minIngestTimeout, maxIngestTimeout)
	return Config{
		ProviderURL:       env("ETH_PROVIDER_URL", ""),
		ClickHouseDSN:     BuildClickHouseDSN(),
		SyncConfirmations: syncConf,
		BatchBlocks:       batch,
		RateLimit:         rateLimit,
		RedisURL:          env("REDIS_URL", ""),
		EmbeddingModel:    env("EMBEDDING_MODEL", ""),
		Timeout:           timeout,
		HTTPRetries:       parseIntEnv("HTTP_RETRIES", 2),
		HTTPBackoffBase:   parseDurEnv("HTTP_BACKOFF_BASE", 100*time.Millisecond),
	}
}
