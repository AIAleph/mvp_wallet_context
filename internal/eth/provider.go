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

	// BlockTimestamp returns the block timestamp in milliseconds since epoch.
	BlockTimestamp(ctx context.Context, block uint64) (int64, error)

	// GetLogs fetches logs for the given address/topics in the block range [from, to].
	// Implementations should page internally and rate-limit.
	GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]Log, error)

	// TraceBlock (or equivalent) returns internal traces for a block range.
	TraceBlock(ctx context.Context, from, to uint64, address string) ([]Trace, error)

	// Transactions returns external transactions touching the address within
	// the inclusive block range [from, to]. Providers may return ErrUnsupported
	// when a filtered view is not available.
	Transactions(ctx context.Context, address string, from, to uint64) ([]Transaction, error)
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

// Transaction models an external transaction (is_internal=0). ValueWei remains
// a string to avoid loss of precision when Jackson-coded into big.Int later on.
type Transaction struct {
	Hash     string
	From     string
	To       string
	ValueWei string
	InputHex string
	GasUsed  uint64
	Status   uint8
	BlockNum uint64
	TsMillis int64
}
