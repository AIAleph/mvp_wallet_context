package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "regexp"
    "strconv"
    "strings"
    "time"

    "github.com/AIAleph/mvp_wallet_context/internal/ingest"
    cfgpkg "github.com/AIAleph/mvp_wallet_context/internal/config"
    "github.com/AIAleph/mvp_wallet_context/internal/eth"
)

var (
    // version is set via -ldflags "-X main.version=..."
    version = "dev"
    // exit is aliased to os.Exit to allow overriding in tests.
    exit = os.Exit
    // function variables allow tests to inject stubs
    newIngest             func(address string, opts ingest.Options) interface{ Backfill(context.Context) error; Delta(context.Context) error }
    newIngestWithProvider func(address string, opts ingest.Options, p eth.Provider) interface{ Backfill(context.Context) error; Delta(context.Context) error }
    newProvider           func(endpoint string, rate int, retries int, backoff time.Duration) (eth.Provider, error)
)

// default wiring functions kept separate for full coverage and testability.
// These provide a tiny indirection so tests can exercise/override wiring
// without affecting production behavior.
func defaultNewIngest(address string, opts ingest.Options) interface{ Backfill(context.Context) error; Delta(context.Context) error } {
    return ingest.New(address, opts)
}

func defaultNewIngestWithProvider(address string, opts ingest.Options, p eth.Provider) interface{ Backfill(context.Context) error; Delta(context.Context) error } {
    return ingest.NewWithProvider(address, opts, p)
}

func defaultNewProvider(endpoint string, rate int, retries int, backoff time.Duration) (eth.Provider, error) {
    return eth.NewProvider(endpoint, rate, retries, backoff)
}

func wireDefaults() {
    newIngest = defaultNewIngest
    newIngestWithProvider = defaultNewIngestWithProvider
    newProvider = defaultNewProvider
}

func init() { wireDefaults() }

// env gets an environment variable or returns a fallback.
func env(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

// parseIntEnv parses an int from env or returns default if unset/invalid.
func parseIntEnv(key string, def int) int {
    v := os.Getenv(key)
    if v == "" {
        return def
    }
    i, err := strconv.Atoi(v)
    if err != nil {
        return def
    }
    return i
}

// parseDurEnv parses a time.Duration from env or returns default.
func parseDurEnv(key string, def time.Duration) time.Duration {
    v := os.Getenv(key)
    if v == "" {
        return def
    }
    d, err := time.ParseDuration(v)
    if err != nil {
        return def
    }
    return d
}

// printUsage prints a detailed CLI help with env mappings and examples.
func printUsage() {
    fmt.Fprintf(flag.CommandLine.Output(), "\nUsage:\n  %s --address 0x... [--mode backfill|delta] [flags]\n\n", os.Args[0])
    fmt.Fprintln(flag.CommandLine.Output(), "Flags:")
    flag.PrintDefaults()
    fmt.Fprintln(flag.CommandLine.Output(), "\nEnvironment variables (defaults):")
    fmt.Fprintln(flag.CommandLine.Output(), "  ETH_PROVIDER_URL   RPC endpoint (default empty)")
    fmt.Fprintln(flag.CommandLine.Output(), "  CLICKHOUSE_DSN     ClickHouse DSN (preferred if set)")
    fmt.Fprintln(flag.CommandLine.Output(), "  CLICKHOUSE_URL     ClickHouse base URL (e.g., http://localhost:8123)")
    fmt.Fprintln(flag.CommandLine.Output(), "  CLICKHOUSE_DB      ClickHouse database name")
    fmt.Fprintln(flag.CommandLine.Output(), "  CLICKHOUSE_USER    ClickHouse username (optional)")
    fmt.Fprintln(flag.CommandLine.Output(), "  CLICKHOUSE_PASS    ClickHouse password (optional)")
    fmt.Fprintln(flag.CommandLine.Output(), "  SYNC_CONFIRMATIONS Confirmations for delta (default 12)")
    fmt.Fprintln(flag.CommandLine.Output(), "  BATCH_BLOCKS       Block batch size (default 5000)")
    fmt.Fprintln(flag.CommandLine.Output(), "  RATE_LIMIT         RPC rate limit (req/s, default 0 = unlimited)")
    fmt.Fprintln(flag.CommandLine.Output(), "  HTTP_RETRIES       HTTP retries on 5xx/429/network (default 2)")
    fmt.Fprintln(flag.CommandLine.Output(), "  HTTP_BACKOFF_BASE  Backoff base for retries (default 100ms)")
    fmt.Fprintln(flag.CommandLine.Output(), "  REDIS_URL          Redis connection URL (optional)")
    fmt.Fprintln(flag.CommandLine.Output(), "  EMBEDDING_MODEL    Embedding model identifier (optional)")
    fmt.Fprintln(flag.CommandLine.Output(), "  INGEST_TIMEOUT     Request timeout (default 30s)")
    fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
    fmt.Fprintln(flag.CommandLine.Output(), "  Ingest full history for an address:")
    fmt.Fprintln(flag.CommandLine.Output(), "    ingester --address 0xabc... --mode backfill --provider $ETH_PROVIDER_URL")
    fmt.Fprintln(flag.CommandLine.Output(), "  Delta update with 12 confirmations:")
    fmt.Fprintln(flag.CommandLine.Output(), "    ingester --address 0xabc... --mode delta --confirmations 12")
}

// MVP ingester entrypoint. Offers helpful flags, env fallbacks, and validation.
func main() {
    // Load centralized defaults from env.
    defaults := cfgpkg.Load()
    var (
        address       string
        mode          string
        fromBlock     uint64
        toBlock       uint64
        confirmations int
        batch         int
        providerURL   string
        chDSN         string
        rateLimit     int
        redisURL      string
        embeddingModel string
        timeout       time.Duration
        dryRun        bool
        showVersion   bool
    )

    flag.Usage = printUsage
    flag.StringVar(&address, "address", "", "Ethereum address to sync (0x...) [required]")
    flag.StringVar(&mode, "mode", "backfill", "Mode: backfill | delta")
    flag.Uint64Var(&fromBlock, "from-block", 0, "Start block (0 = auto)")
    flag.Uint64Var(&toBlock, "to-block", 0, "End block (0 = head)")
    flag.IntVar(&confirmations, "confirmations", defaults.SyncConfirmations, "Required confirmations for finality")
    flag.IntVar(&batch, "batch", defaults.BatchBlocks, "Block batch size per request")
    flag.StringVar(&providerURL, "provider", defaults.ProviderURL, "Ethereum RPC provider URL (ETH_PROVIDER_URL)")
    flag.StringVar(&chDSN, "clickhouse", defaults.ClickHouseDSN, "ClickHouse DSN (CLICKHOUSE_DSN or built from CLICKHOUSE_URL/DB/USER/PASS)")
    flag.IntVar(&rateLimit, "rate-limit", defaults.RateLimit, "RPC rate limit (req/s, 0 = unlimited)")
    flag.StringVar(&redisURL, "redis", defaults.RedisURL, "Redis connection URL (REDIS_URL)")
    flag.StringVar(&embeddingModel, "embedding-model", defaults.EmbeddingModel, "Embedding model identifier (EMBEDDING_MODEL)")
    flag.DurationVar(&timeout, "timeout", defaults.Timeout, "Ingestion timeout")
    flag.BoolVar(&dryRun, "dry-run", false, "Print plan and exit")
    flag.BoolVar(&showVersion, "version", false, "Print version and exit")
    flag.Parse()

    if showVersion {
        fmt.Println(version)
        return
    }

    if address == "" {
        fmt.Fprintln(os.Stderr, "missing --address (0x...); see --help")
        exit(2)
    }
    // Basic address shape validation. Full EIP-55 checksum is enforced upstream.
    if ok, _ := regexp.MatchString(`^0x[a-fA-F0-9]{40}$`, address); !ok {
        fmt.Fprintln(os.Stderr, "invalid --address; expected 0x-prefixed 40 hex chars")
        exit(2)
    }

    mode = strings.ToLower(mode)
    if mode != "backfill" && mode != "delta" {
        fmt.Fprintf(os.Stderr, "unknown --mode %q (use backfill|delta)\n", mode)
        exit(2)
    }
    if toBlock > 0 && fromBlock > toBlock {
        fmt.Fprintln(os.Stderr, "--from-block cannot be greater than --to-block")
        exit(2)
    }
    if confirmations < 0 {
        fmt.Fprintln(os.Stderr, "--confirmations must be >= 0")
        exit(2)
    }
    if batch <= 0 {
        fmt.Fprintln(os.Stderr, "--batch must be > 0")
        exit(2)
    }

    opts := ingest.Options{
        ProviderURL:   providerURL,
        ClickHouseDSN: chDSN,
        FromBlock:     fromBlock,
        ToBlock:       toBlock,
        Confirmations: confirmations,
        BatchBlocks:   batch,
        RateLimit:     rateLimit,
        RedisURL:      redisURL,
        DryRun:        dryRun,
        Timeout:       timeout,
    }

    if dryRun {
        // Print a compact JSON plan and exit.
        plan := map[string]any{
            "address":       address,
            "mode":          mode,
            "provider":      providerURL,
            // Avoid logging secrets; show redacted DSN if configured.
            "clickhouse_dsn": func() string { if chDSN == "" { return "" } ; return cfgpkg.RedactDSN(chDSN) }(),
            "from_block":    fromBlock,
            "to_block":      toBlock,
            "confirmations": confirmations,
            "batch":         batch,
            "rate_limit":    rateLimit,
            "redis_url":     redisURL,
            "embedding_model": embeddingModel,
            "timeout":       timeout.String(),
        }
        enc := json.NewEncoder(os.Stdout)
        enc.SetIndent("", "  ")
        _ = enc.Encode(plan)
        return
    }

    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    // If a provider URL is configured, build a provider and inject it. Otherwise
    // fall back to a stubbed ingester (tests).
    var ing interface{ Backfill(context.Context) error; Delta(context.Context) error }
    if providerURL != "" {
        p, err := newProvider(providerURL, rateLimit, defaults.HTTPRetries, defaults.HTTPBackoffBase)
        if err != nil {
            fmt.Fprintf(os.Stderr, "provider error: %v\n", err)
            exit(1)
        }
        ing = newIngestWithProvider(address, opts, p)
    } else {
        ing = newIngest(address, opts)
    }
    var err error
    switch mode {
    case "backfill":
        err = ing.Backfill(ctx)
    case "delta":
        err = ing.Delta(ctx)
    }
    if err != nil {
        fmt.Fprintf(os.Stderr, "ingestion error: %v\n", err)
        exit(1)
    }
    fmt.Println("ok")
}
