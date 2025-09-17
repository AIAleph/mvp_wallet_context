Schema Migrations and Rollbacks

Overview
- Two schemas are maintained:
  - canonical: production tables with indices and DateTime64(3) columns.
  - dev: lightweight preview tables (dev_*).
- The schema_version table tracks applied versions and descriptions.

Apply Schema
- Use scripts/schema.sh to apply a specific file:
  - canonical: `SCHEMA_FILE=sql/schema.sql scripts/schema.sh`
  - dev: `SCHEMA_FILE=sql/schema_dev.sql scripts/schema.sh`
- Or use the guided migration wrapper:
  - `scripts/migrate_schema.sh TO=canonical`
  - `scripts/migrate_schema.sh TO=dev`

Record Version
- migrate_schema.sh inserts a row into `schema_version` with a version and description.
- canonical uses version 2; dev uses version 1.

Rollback Procedures
- Minor: revert individual table changes by reapplying the previous schema file.
- Full: snapshot/export tables, drop/recreate from the desired schema file, and reimport.
  - Always confirm application downtime and data loss expectations before full rollback.
- Record rollback in `schema_version` by inserting a new row noting the rollback target.

Safety Tips
- Test schema changes in a staging ClickHouse first.
- Monitor for query performance regressions.
- Keep backups and export critical tables prior to destructive changes.

