# Changelog

## 2.0.0 (Unreleased)

Backend and query experience rewrite. The plugin id, the SQL syntax and existing
dashboard queries keep working.

### Backend

- New OpenObserve client: request context/timeout propagation, TLS skip-verify,
  basic or token authentication, actionable error messages (the OpenObserve error
  body is surfaced), schema caching, SSE + plain JSON response handling.
- New result set model: column typing derived from the stream schema first and from
  the values as a fallback, automatic time column detection for microseconds,
  milliseconds, nanoseconds and ISO strings.
- New frame builder with `auto`, `logs`, `table`, `timeseries`, `trace` and
  `annotations` formats; multi-series results are split into one labelled wide frame
  per series group.
- Logs frames expose `time`, `body`, `level`, `labels` plus every original column, and
  the level is mapped so Explore colors it correctly.
- Trace frames are built in Grafana's span format so the trace view renders the
  waterfall.
- PromQL query mode against OpenObserve's Prometheus compatible API
  (`/api/{org}/prometheus`), including label values for metric completion.
- Grafana ad-hoc filters are injected server side: proper SQL predicates, existing
  `OR` chains parenthesised, values escaped, `number(...)` kept numeric, `=~`/`!~`
  translated to OpenObserve's `~`/`!~`.
- SQL parsing no longer depends on a third party parser: the query builder only needs
  the statement shape (projection, source stream, LIMIT/OFFSET, GROUP BY), which keeps
  OpenObserve's dialect working and removes two heavy dependencies.
- Row limit resolution: query `size` > SQL `LIMIT` > datasource default, always capped
  by the configured maximum.
- Queries run concurrently per panel request; a panic in one query no longer takes the
  plugin down.
- Health check reports the organization and the number of log streams.
- New resource endpoints: `/openobserve/streams`, `/openobserve/schema`,
  `/openobserve/tag-keys`, `/openobserve/tag-values`, `/openobserve/metric-names`,
  `/openobserve/promql-metadata`.
- `go` directive raised to 1.27.1 and `sonic` to v1.15.5 for Go 1.27 compatibility.

### Frontend

- New query editor: stream type, query mode (SQL/PromQL), result format, stream picker
  with completion, row limit, SSE and cache toggles.
- New datasource configuration editor: connection, authentication (basic/token), TLS,
  timeout, limits, PromQL toggle and column mapping.
- Template variables: quote-aware SQL formatting, `NULL` for empty values, multi-value
  support, first column becomes the variable value.
- Ad-hoc filter keys/values now come from the real stream schema.
- Annotation support.
- Removed the `lodash` dependency and the deprecated `Select` component.

### Fixed

- The ad-hoc filter asked for tag keys without a stream whenever the SQL was
  typed by hand (`query.table` is only set when the picker is used), so Grafana
  filled the filter dropdown with *stream names* instead of column names. The
  stream is now read out of the statement as a fallback.
- `InlineField` clones its child and always passes `{ invalid, disabled, loading }`,
  so `disabled` set on the child was overwritten with `undefined`: the PromQL mode
  picker stayed enabled for log and trace streams. The flag now lives on
  `InlineField`.
- A computed column aliased onto a name that also exists in the stream was typed
  from the stream schema, so `SELECT count(*) AS "count"` against a stream with a
  Utf8 `count` column produced a *string* column and every numeric panel broke.
  The observed values now override the schema when the two disagree.
- An empty result caused by querying through the wrong stream type (a log stream
  selected as `metrics`) silently rendered an empty panel; the backend now reports
  which stream type the stream actually belongs to.
- Frames were built with an untyped `[]any` column which could panic for mixed types.
- `_timestamp` was only rendered as time when the SQL aliased it to `gf_time`.
- A single numeric column produced one unlabeled series even when the query grouped by
  a label.
- `SELECT *` results could not be shown as logs with attributes.
- `getTagKeys`/`getTagValues` returned an empty list, so ad-hoc variables were unusable.
- The datasource URL with a trailing slash produced malformed API URLs.
- SSE responses were parsed without bounds, so very large log lines broke the search.

## 1.0.0

Initial release.
