# Changelog

## 1.0.0 (Unreleased)

Initial release.

## Bug Fixes

### Fixed `SELECT *` queries in Dashboard panels
- Fixed issue where `SELECT *` queries returned "No Data" in Dashboard panels when `queryType` was empty
- `TransformFallbackSelectFrom` now correctly handles `SELECT *` queries by using log mode instead of table mode
- This ensures consistency with `TransformStream` behavior for `SELECT *` queries
