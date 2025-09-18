# Observability Playbook

## Receipt Lookup Alert: `receipt_calls`

### Why it matters

Each canonical ingestion range emits a structured log with the message key `receipt_lookup` (see `internal/eth/http_provider.go`). The payload includes `receipt_calls`, `block_span`, and the `provider` host. Spikes in `receipt_calls` signal that the ingester is issuing one JSON-RPC receipt request per matching transaction, which can exhaust provider quotas during large backfills.

### Recommended pipeline

1. **Ship JSON logs** from the ingester to your log backend (e.g., Loki, Elasticsearch, Datadog) without transformation; be sure to preserve the `component`, `provider`, and `receipt_calls` fields.
2. **Label/parse** on ingest so `component="eth.http_provider.transactions"` and `provider` are indexed for filtering.

### Grafana Loki example

Query (promtail labels with `component` and structured JSON parsing enabled):

```
sum by (provider) (
  rate({component="eth.http_provider.transactions"}
       | json
       | message="receipt_lookup"
       | unwrap receipt_calls [5m]
  )
)
```

Panel recommendation:
- **Visualization:** Time series, stacked by `provider`.
- **Threshold:** Horizontal line at 80% of your provider’s documented `eth_getTransactionReceipt` quota (e.g., 30 req/s ⇒ draw at 24 req/s).
- **Alert rule:** Trigger WARN when the 5-minute rate exceeds 70% for 10 minutes; escalate to CRIT above 90%.
- **Annotations:** Include `address`, `block_span`, and `tx_examined` from the log for context in alert notifications.

### Alternative backends

- **CloudWatch Logs Insights**: `stats avg(receipt_calls) as rate by provider` over 5m windows; create metric filters for alerting.
- **Datadog Log-Based Metric**: facet on `provider`, define a rate metric from `receipt_calls`, and alert via composite monitor.

### Runbook snippet

If the alert fires:
1. Inspect the offending `address`/`block_span` in the log payload to confirm whether a one-off backfill is running.
2. Consider reducing `BATCH_BLOCKS` or enabling provider-level batching APIs (e.g., `eth_getBlockReceipts`) before re-running.
3. If sustained, request higher RPC quota or add caching at the provider factory.
