Code Cleanup Summary (current ref)

Purpose
- Trim non-essential additions from recent hardening work and leave a minimal, well-documented surface.

Scope
- API health metrics rationalization
- Script safety polish
- ClickHouse client robustness
- Migration tooling

What Stayed (essential)
- Health endpoints hardening: timeouts, rate limiting, caching, safe DSN handling (no secrets logged).
- Prometheus basics: `http_requests_total`, `process_resident_memory_bytes`, `process_uptime_seconds`.
- Shell safety: `set -euo pipefail`, quoted variables, schema file validation.
- ClickHouse client retries (network/429/5xx) with small exponential backoff.
- Migration helper `scripts/migrate_schema.sh` and `schema_version` table.
- Pre-commit hooks and CI shellcheck.

What Was Removed (de-bloat)
- Extra per-endpoint health metrics (`health_checks_total`, `health_cache_hits_total`). These duplicated request counters and added marginal value. The basic Prometheus series remain.

Notes
- API README documents all remaining env knobs and metrics. If more observability is required, consider adding a proper metrics library and a dedicated dashboard rather than growing bespoke counters.

