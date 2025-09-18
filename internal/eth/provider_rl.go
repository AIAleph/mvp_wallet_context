package eth

import "context"

// RLProvider wraps a Provider with a Limiter.
type RLProvider struct {
	p Provider
	l Limiter
}

func WrapWithLimiter(p Provider, l Limiter) Provider { return RLProvider{p: p, l: l} }

func (r RLProvider) BlockNumber(ctx context.Context) (uint64, error) {
	if err := r.l.Wait(ctx); err != nil {
		return 0, err
	}
	return r.p.BlockNumber(ctx)
}

func (r RLProvider) BlockTimestamp(ctx context.Context, block uint64) (int64, error) {
	if err := r.l.Wait(ctx); err != nil {
		return 0, err
	}
	return r.p.BlockTimestamp(ctx, block)
}

func (r RLProvider) GetLogs(ctx context.Context, address string, from, to uint64, topics [][]string) ([]Log, error) {
	if err := r.l.Wait(ctx); err != nil {
		return nil, err
	}
	return r.p.GetLogs(ctx, address, from, to, topics)
}

func (r RLProvider) TraceBlock(ctx context.Context, from, to uint64, address string) ([]Trace, error) {
	if err := r.l.Wait(ctx); err != nil {
		return nil, err
	}
	return r.p.TraceBlock(ctx, from, to, address)
}

func (r RLProvider) Transactions(ctx context.Context, address string, from, to uint64) ([]Transaction, error) {
	if err := r.l.Wait(ctx); err != nil {
		return nil, err
	}
	return r.p.Transactions(ctx, address, from, to)
}
