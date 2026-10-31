# Feature guide

This page describes what the OpenObserve data source can do and how the pieces map
onto the OpenObserve API. It is meant for users who want to build log, metric and
trace dashboards without writing a custom plugin.

## Query modes

| Mode | Endpoint | When to use |
| ---- | -------- | ----------- |
| SQL | `POST /api/{org}/_search` (or `_search_stream` for SSE) | logs, traces, ad-hoc metric queries, anything OpenObserve's SQL dialect can express |
| PromQL | `GET /api/{org}/prometheus/api/v1/query_range` | metrics the way Grafana expects them: `rate(...)`, `histogram_quantile(...)`, label matchers |

Both modes are selected per query in the editor (`Query mode`). PromQL is only offered
for the `metrics` stream type because OpenObserve only exposes a Prometheus compatible
API over metric streams.

## Result formats

| Format | What the backend builds | Panel to use |
| ------ | ----------------------- | ------------ |
| `Auto` | time + numbers -> time series, otherwise table | any |
| `Logs` | `time`, `body`, `level`, `labels` + every original column | Logs / Explore |
| `Table` | typed columns, one row per hit | Table |
| `Time series` | one wide frame per series (labels become the legend) | Time series |
| `Trace` | Grafana trace frame (`traceID`, `spanID`, `parentSpanID`, ...) | Traces / Explore |
| `Annotations` | `time`, `title`, `text`, `tags` | Dashboard annotations |

`Auto` resolves to `Logs` for `SELECT *` on log streams, to `Trace` for the trace
stream type and to `Time series` when the result has a time column and numeric values.

## Column mapping

The backend auto-detects the important columns; override them in the datasource
settings (or per query) when your schema differs:

* `timeField` - default `_timestamp` (also recognises `timestamp`, `ts`, `time`, `start_time`, ...)
* `logMessageField` - default `body` (also `message`, `msg`, `log`, ...)
* `logLevelField` - default `level` (also `severity`, `severity_text`, ...)

Timestamps coming back as microseconds (`_timestamp`), nanoseconds (`start_time`) or
ISO strings are all converted to Grafana `time` columns.

## Logs

```
SELECT * FROM "my_logs" LIMIT 500
```

The frame contains `time`, `body`, `level`, `labels` and one extra field per original
column, so Explore shows the log line, the level colors and the full attribute set in
the details pane.

## Metrics

SQL:

```
SELECT histogram(_timestamp, '1 minute') AS ts,
       avg(value) AS value,
       service_name
FROM "go_goroutine_count"
GROUP BY ts, service_name
ORDER BY ts
```

One series per `service_name`, no Grafana transformations required.

PromQL:

```
sum by (service_name) (rate(http_server_request_duration_count[5m]))
```

## Traces

```
SELECT * FROM "my_traces" WHERE trace_id = '<id>' ORDER BY start_time
```

or, to browse spans of the selected time range:

```
SELECT * FROM "my_traces" ORDER BY start_time DESC LIMIT 200
```

The trace format picks the first trace id in the result (or `traceId` from the query)
and renders the waterfall in Explore / the Traces panel.

## Variables and ad-hoc filters

A template variable is a SQL query whose first column becomes the value:

```
SELECT DISTINCT service_name AS "value" FROM "my_logs"
```

Ad-hoc filters from a dashboard variable are injected server side:

* `=` and `!=` become `=` / `<>`
* `=~` / `!~` become OpenObserve's `~` / `!~`
* `IN` accepts a comma separated list
* values are escaped (`it's` -> `'it''s'`), `number(123)` stays numeric
* the filters are AND-ed with an existing predicate, and an existing `OR` chain is
  parenthesised first so its meaning does not change

## Annotations

Create an annotation query with the `Annotations` format and return a time column plus
a text column, e.g.

```
SELECT _timestamp AS time, body AS text FROM "my_logs" WHERE level = 'error'
```

## Datasource settings

| Setting | Description |
| ------- | ----------- |
| URL | OpenObserve base URL, e.g. `http://localhost:5080` |
| Organization | OpenObserve organization (legacy key: `database`) |
| Authentication | basic auth (user + password) or a token sent verbatim in `Authorization` |
| Skip TLS verification | for self-signed certificates |
| Timeout | upper bound for a single query |
| Default / max limit | row count used when the SQL has no LIMIT, and the hard cap |
| Enable PromQL | toggles the Prometheus compatible query mode |
| Column mapping | time / log message / log level column overrides |
