# Changelog

## 2.3.3

### Fixed

- **Switching from logs or metrics to traces no longer queries the previous
  type's stream.** The switch cleared the statement but kept `table`, so the
  trace query asked OpenObserve for `go_memory_used` (or `vnet_bff_prd`) in the
  trace catalogue and failed with `Search stream not found`. The stream is now
  dropped along with the statement, and the new type's own stream is adopted.
- **A trace stream with no spans is no longer chosen as the default.** Trace
  catalogues often start with empty streams (`dev` here, next to the live
  `prd`), and defaulting to one made a freshly switched panel show "0 series
  returned". The first candidate that answers a one-row probe wins; results are
  cached per session so the cost is paid once.

## 2.3.2

### Changed

- **Switching stream type clears the panel instead of leaving the previous
  result on screen.** The type switch drops the statement, which is what makes
  the old rows go away, and marks the query deferred. The editor asks Grafana
  to repaint once, the datasource answers that request locally with an empty
  result, and nothing is sent to OpenObserve. Filtering the target out was not
  enough: Grafana keeps the previous render in that case, so stale results from
  the type the user just left stayed visible.
- The editor mode and row limit survive a type switch; clearing the statement
  used to flip the editor to Code and reset the limit to 0.

## 2.3.1

### Changed

- **Switching stream type no longer sends a query.** The new type changes which
  parameters are meaningful, so a query fired at that moment is half-built and
  usually fails; the user asked for a type, not for a run. Probes that looked
  for a non-empty stream are gone for the same reason -- they cost one request
  per candidate on every switch.
- Trace `Tags` is now `Attributes`, the name OpenTelemetry uses for the
  key/value pairs.
- The stream picker names its signal (`Log stream`, `Metric stream`, `Trace
  stream`) instead of the ambiguous "Stream".
- The limit is on its own last row in every editor.

## 2.3.0

### Changed

- **Editor layout follows the ClickHouse and Prometheus editors.** Stream type,
  query mode and editor mode on the first row, the stream on its own row, then
  a Raw/Aggregate switch. The limit sits last, after the inputs that shape the
  query. The builder shows a read-only preview of the statement instead of an
  editor-shaped box.
- The trace editor is laid out like Jaeger's: query type and trace stream
  first, then one row of filters per query type.
- **SSE and cache are always on and no longer have switches.** They are
  transport details; the toggles only invited people to disable streaming and
  then wonder why results lagged.
- The default row limit is 100, matching between the editor and the backend
  (it was 0 in the editor and 200 on the server).

### Fixed

- Editing the limit in Builder mode flipped the editor to Code: the handler
  wrote the limit into `rawSql`, which made the query look hand-written.

## 2.2.0

### Added

- **Builder / Code editor modes**, matching how Grafana's own query editors
  work. Builder takes structured inputs (stream, aggregation, field, interval,
  group by) and the backend derives the SQL from them. Code is the statement
  you write. Neither writes into the other, so switching modes never loses
  work: choosing options can no longer render generated SQL into a
  hand-written statement, and a builder query never has a half-built statement
  in its editor.

### Fixed

- Builder queries were filtered out before being sent, so the panel never ran.

## 2.1.1

### Fixed

- Switching a metric query to PromQL left the expression empty, so OpenObserve
  answered `400 invalid promql query`. Picking a stream now seeds the
  expression with that metric name, and switching SQL -> PromQL seeds it from
  the stream already selected. An expression the user wrote is never
  overwritten.

### Compatibility

- Verified against Grafana 10.4.4 as well as 12.2: the health check, log
  queries, the trace editor (search / trace id / graph) and PromQL all work,
  with no console or page errors.

## 2.1.0

### Added

- Trace ids in the search table open the trace inside Grafana (internal link to
  Explore's split view), instead of requiring the id to be pasted by hand.
- The service dependency graph is drawn as a Grafana node graph, and a Service
  Name filter selects the edges touching that service in either direction.

### Changed

- The **Format** selector is gone. The frame shape follows from the stream type
  and the statement, so a second switch could only contradict it.
- Trace `startTime` is a real time column and `duration` is expressed in
  milliseconds, so both read as a timestamp and a duration.
- The **Limit** field and the SQL `LIMIT` mirror each other, for logs, metrics
  and traces alike.
- Switching stream type runs the query once the new stream is adopted, instead
  of leaving stale results on screen.
- Service graph edges are derived from OpenObserve's inferred service name, so
  a client span attributes to the system it calls rather than to the caller.

### Fixed

- Trace `startTime` showed the year 2106: the nanosecond epoch was read as
  microseconds.
- Graph queries with a Service Name failed with `unknown field 'child'`.
- Trace queries were dropped before being sent when they carried a leftover
  `rawSql` from another stream type.
- Trace duration was 1000x too large.

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
- `sonic` raised to v1.15.5; the older v1.14.0 does not compile against Go 1.27.
  The `go` directive stays at 1.24.6 so the CI toolchain keeps working.

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
