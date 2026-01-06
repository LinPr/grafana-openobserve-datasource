# Changelog

## 1.0.0 (Unreleased)

Initial release.

## Bug Fixes

### Fixed `SELECT *` queries in Dashboard panels
- Fixed issue where `SELECT *` queries returned "No Data" in Dashboard panels when `queryType` was empty
- `TransformFallbackSelectFrom` now correctly handles `SELECT *` queries by using log mode instead of table mode
- This ensures consistency with `TransformStream` behavior for `SELECT *` queries

## Features

### SQL LIMIT clause support
- Added automatic extraction of `LIMIT` clause from SQL queries
- The `LIMIT` value is now used as the search `Size` parameter in OpenObserve API requests
- Previously, all queries were capped at 200 results regardless of SQL `LIMIT` clause
- Maximum limit cap of 10,000 to prevent browser crashes and excessive memory usage
- Supports both direct LIMIT values (e.g., `LIMIT 1000`) and template variables (e.g., `LIMIT ${limit_var}`)
- Default limit of 200 is used when no `LIMIT` clause is present in the SQL query
