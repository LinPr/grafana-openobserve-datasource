# Changelog

## 2.4.5

### Fixed

- **Both Go checks can now pass at once.** They pull in opposite directions:

  - the catalogue's source scan only runs toolchains up to **1.27.3**, and a
    `go` directive above that is a hard block ("Plugin source requires Go
    1.27.4 or later; this validator supports Go 1.27.3");
  - `GO-2026-6859` in `net/http` is only fixed in **1.27.4**, so a binary built
    with 1.27.3 fails the vulnerability scan.

  `go.mod` keeps `go 1.27.3` as the compatibility floor, which the validator can
  parse, while CI and the Docker build use 1.27.4, which produces a binary with
  no known stdlib vulnerabilities. Building above the floor is the normal Go
  arrangement: the directive declares what the code needs, not which patch the
  binary is compiled with.

  Verified by scanning the released binary: `go version` reports go1.27.4 and
  `govulncheck -mode=binary` reports no vulnerabilities.


## 2.4.4

### Fixed

- **The source scan could not run at all.** Pinning `go 1.27.4` made the
  catalogue's validator fail with *"Plugin source requires Go 1.27.4 or later;
  this validator supports Go 1.27.3"* -- a hard block, unlike the warnings
  beside it. `go.mod`, both workflows and the Docker build now pin **1.27.3**,
  which is the version the Go advisories name as the fix and the newest the
  validator can handle.
- The G402 suppression is kept next to the code with its justification. It is
  reported as a warning rather than a blocker: skipping TLS verification is the
  datasource's opt-in setting for OpenObserve behind a private CA, so removing
  the code would remove the feature.


## 2.4.3

### Security

- **Go 1.27.4.** The catalogue's binary scan reported 16 standard-library
  vulnerabilities against the Go 1.27.1 build and asked for 1.27.3 or later.
  `go.mod`, both workflows and the Docker build now pin 1.27.4.
- **`github.com/klauspost/compress` v1.20.4 and `golang.org/x/net` v0.62.0**
  (GO-2026-6865, GO-2026-6860, GO-2026-6866). Both were reachable from the
  plugin binary, not just the test build.
- **G402 (`InsecureSkipVerify`) is annotated rather than removed.** Skipping
  certificate verification is a deliberate, opt-in datasource setting for
  OpenObserve behind a self-signed certificate: it is off by default and
  surfaced in the form with a warning. The line now carries a `#nosec G402`
  with that justification, so the finding is a reviewed decision instead of
  an unreported risk.

Verified with `gosec ./pkg/...` (0 issues, 1 justified `#nosec`),
`govulncheck ./...` and `govulncheck -mode=binary` on a freshly built 1.27.4
binary: no vulnerabilities found in either mode.


## 2.4.2

### Fixed

- **The published checksums could not be used for a catalogue submission.** A
  single `checksums.txt` listed seven archives with the darwin build on the
  first line, while the archive meant for the catalogue is the universal one
  further down. Copying the obvious first hash made the catalogue reject the
  upload with *"The submitted plugin archive checksum does not match the
  downloaded archive."*

  Every archive now publishes its own `.sha1` and `.md5` sidecar containing
  nothing but the hash, the universal archive is the first line of
  `checksums.txt`, and the release job prints the filename/checksum pair in its
  summary.


## 2.4.1

### Changed

- **Release archives are named `grafana-openobserve-datasource-<version>…zip`.**
  The `linpr-` prefix is gone from the asset names, matching the branding of the
  container image while keeping the file name recognisable. Only the file name
  changed: the plugin id and the directory inside the archive are still
  `linpr-openobserve-datasource`, so `grafana-cli plugins install` and the
  catalogue keep working.

### Fixed

- **The packaged `README.md` was still the `@grafana/create-plugin` template.**
  `webpack` prefers `src/README.md` when it exists, and that file had never been
  filled in, so the archive shipped a page of commented-out instructions. It now
  carries a real introduction, requirements, a getting-started section and the
  screenshots, which is also what the catalogue displays.

### Documentation

- The root `README.md` gained a table of the identifiers (data source type,
  backend binary, image), an installation section that covers the catalogue, the
  release archives and Docker, a provisioning example, an upgrade note for 2.3.x
  and a troubleshooting table.
- Added `README.zh-CN.md`, a Chinese translation of the same structure, with
  language links at the top of both files.

## 2.4.0

### Changed

- **Plugin id is now `linpr-openobserve-datasource`.** The catalogue assigns an
  id per Grafana Cloud org and the first segment must be that org's slug
  (`grafana` belongs to Grafana Labs, so `grafana-…` was rejected with "Plugin
  id contains incorrect org."). After an id has been published once it cannot
  change, so the rename had to happen before the first catalogue upload.
- **Backend executable is now `linpr-grafana-openobserve-datasource`** instead
  of `gpx_open_observe`, so a running process or a stray file is identifiable as
  a Grafana plugin. The binary name is declared in `plugin.json` and discovered
  by Grafana, so nothing else has to know it.

### Upgrading

The datasource type changed, so existing dashboards and provisioned datasources
pointing at `grafana-openobserve-datasource` will not resolve. Either keep the
old version installed, or re-select the data source on the affected panels.
Provisioning files need `type: linpr-openobserve-datasource`; keep the existing
`uid` so dashboards keep resolving once the type is corrected.

## 2.3.6

### Fixed

- The catalogue screenshots had a duplicated entry: "Trace waterfall" pointed at
  the trace search image, so the listing showed the same view twice. It now
  uses the waterfall, and the dashboard screenshot is labelled for what it is.

## 2.3.5

### Fixed for the plugin catalogue

- `grafanaDependency` is now `>=10.4.0-0`. The catalogue requires a
  prerelease-inclusive range, otherwise Grafana nightly and enterprise builds
  are not considered compatible and the archive is rejected at upload.
- Backend binaries are chmod-ed to 0755 in the release job. The catalogue
  requires exactly that, and `mage` otherwise inherits the runner's umask.
- Added catalogue screenshots, so the listing is not a blank page.

### Security

- Resolved every `osv-scanner` finding in the dependency tree: 58
  vulnerabilities (2 critical) down to none, using `overrides` to pin patched
  releases of transitive packages (`ws`, `fast-uri`, `js-yaml`, `postcss`,
  `nanoid`, `form-data`, `flatted`, `serialize-javascript`, `source-map-js`,
  `websocket-driver`, `browserslist`).
- `@grafana/*` build dependencies moved to 12.4.14. They cannot go to 13.x:
  those require React 19, while Grafana 10.4 ships React 18, and dropping 10.x
  support is not on the table.

## 2.3.4

### Build

- **Multi-architecture image.** `ghcr.io/linpr/grafana-openobserve-datasource`
  is published as a manifest list for `linux/amd64` and `linux/arm64`, so
  `docker pull` selects the right one.
- **One archive per platform.** Releases now carry the universal zip plus
  separate `linux-amd64`, `linux-arm64`, `linux-arm`, `darwin-amd64`,
  `darwin-arm64` and `windows-amd64` archives, each about 10 MB instead of
  230 MB, with `checksums.txt`.
- **Go 1.27.1** is pinned in `go.mod`, CI and the Docker image, so all three
  build with the same toolchain the SDK requires.
- The image and its exports are named after the plugin id; the previous
  revision exported `openobserve-grafana-plugin`.

### Fixed

- The Dockerfile could not build: `golang:1.27.2` does not exist, the plugin
  was copied to a directory Grafana does not read, and the architecture had to
  be passed by hand.
- `.dockerignore` keeps the build context at 8 MB; it was sending the 11 GB
  Go cache and 700 MB of `node_modules` to the daemon.

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
  filled the filter dropdown with _stream names_ instead of column names. The
  stream is now read out of the statement as a fallback.
- `InlineField` clones its child and always passes `{ invalid, disabled, loading }`,
  so `disabled` set on the child was overwritten with `undefined`: the PromQL mode
  picker stayed enabled for log and trace streams. The flag now lives on
  `InlineField`.
- A computed column aliased onto a name that also exists in the stream was typed
  from the stream schema, so `SELECT count(*) AS "count"` against a stream with a
  Utf8 `count` column produced a _string_ column and every numeric panel broke.
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
