package eth

import (
    "context"
)

// Provider defines the minimal RPC surface the ingester needs. Concrete adapters
// (e.g., Alchemy/Infura/QuickNode/Covalent) will satisfy this interface.
// Note: avoid floats for on-chain values; use strings or big.Int at the edges.
type Provider interface {
    // BlockNumber returns the current head block number.
    BlockNumber(ctx context.Context) (uint64, error)

    // GetLogs fetches logs for the given address/topics in the block range [from, to].
    // Implementations should page internally and rate-limit.
    GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]Log, error)

    // TraceBlock (or equivalent) returns internal traces for a block range.
    TraceBlock(ctx context.Context, from, to uint64, address string) ([]Trace, error)
}

// Log is a minimal scaffold of an Ethereum log. Extend as needed.
type Log struct {
    TxHash   string
    Index    uint32
    Address  string
    Topics   []string
    DataHex  string
    BlockNum uint64
    TsMillis int64
}

// Trace is a minimal scaffold of an internal trace. Extend as needed.
type Trace struct {
    TxHash   string
    TraceID  string
    From     string
    To       string
    ValueWei string // keep as string; decode to big.Int downstream
    BlockNum uint64
    TsMillis int64
}

