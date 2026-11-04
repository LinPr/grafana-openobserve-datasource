# 🔌 Grafana OpenObserve Datasource Plugin

<div align="center">

  ![GitHub](https://img.shields.io/github/license/LinPr/grafana-openobserve-datasource?style=flat-square)
  ![Grafana](https://img.shields.io/badge/Grafana-10.4%20%E2%80%93%2012.2-orange?style=flat-square&logo=grafana)
  ![OpenObserve](https://img.shields.io/badge/OpenObserve-Supported-blue?style=flat-square)
  ![Go](https://img.shields.io/badge/Go-1.24.6+-00ADD8?style=flat-square&logo=go)
  ![TypeScript](https://img.shields.io/badge/TypeScript-5.5.4+-3178C6?style=flat-square&logo=typescript)
  ![Node.js](https://img.shields.io/badge/Node.js-22+-339933?style=flat-square&logo=node.js)

</div>

---

## 🛰️ Overview

A Grafana data source plugin for [OpenObserve](https://github.com/openobserve/openobserve):
query **logs**, **metrics** and **traces** with SQL, or use **PromQL** against
OpenObserve's Prometheus compatible API, and put the results straight into
Grafana panels, dashboards, variables and annotations.

The plugin is non-official. The official one lives
[here](https://github.com/openobserve/openobserve-grafana-plugin); this one aims to
feel native to Grafana instead of exposing the raw API.

> **All screenshots below are captured from a real Grafana 12.2 + OpenObserve
> deployment, not mockups.**

---

## 📸 Screenshots

### Feature tour

A walkthrough recorded against a live Grafana + OpenObserve, in the order you
would set the plugin up: the datasource settings, then logs (with a row expanded
to its original columns), metrics through the builder, PromQL, the trace table,
a real 9-span waterfall, the service graph, and finally a dashboard mixing logs,
time series, a donut, a bar chart and a trace table.

![Feature tour](doc/screenshot/tour.gif)

> The GIF is 2.3 MB and holds each view for half a second. The sharper
> [WebP version](doc/screenshot/tour.webp) is only 0.3 MB and shows the same
> tour at higher resolution; every view is also available as a still image
> below.

### Logs in Explore

Log lines render with their level colour, and the details pane shows every
original column as an attribute:

| Logs in Explore | The query editor |
|---|---|
| ![Logs in Explore](doc/screenshot/explore_logs.png) | ![Query editor](doc/screenshot/query_editor.png) |

The editor puts stream type, query mode (SQL / PromQL) and Builder or Code on
the first row, then the stream, then a Raw/Aggregate switch for the builder. The
limit comes last, after the inputs that shape the query. Builder and Code are
independent, so choosing options never overwrites a statement you wrote.

### Metrics over SQL, with automatic series labels

`GROUP BY service_name` becomes one labelled series per service — no transform,
no SQL aliasing tricks. The same metric streams are queryable through
OpenObserve's Prometheus compatible API, so `rate()`, `histogram_quantile()` and
label matchers all work:

| Metrics over SQL | PromQL |
|---|---|
| ![Metrics over SQL](doc/screenshot/explore_metrics_sql.png) | ![PromQL](doc/screenshot/explore_promql.png) |

### Traces: three query types

The trace editor mirrors Jaeger's three modes and the backend builds the SQL for
each one. **Search** lists the traces matching the filters, and clicking a trace
id opens that trace inside Grafana. **TraceID** loads every span of one trace so
Explore draws the native waterfall. **Graph** derives service-to-service call
counts from the span parent links and renders them as a node graph, taking the
callee from OpenObserve's inferred service name so a client span attributes to
the system it calls rather than to the caller.

| Search | TraceID (waterfall) | Graph |
|---|---|---|
| ![Trace search](doc/screenshot/trace_search.png) | ![Trace waterfall](doc/screenshot/trace_waterfall.png) | ![Trace graph](doc/screenshot/trace_graph.png) |

### A dashboard with everything mixed

Logs, stats, a pie chart, a stacked time series, PromQL and a trace table from a
single data source:

![Dashboard](doc/screenshot/dashboard_logs_metrics.png)

### Template variables

A variable is just a SQL query — the first column becomes the value. Multi-value
selections are correctly quoted into `IN (...)`:

| Dashboard with variables | The variable query editor |
|---|---|
| ![Dashboard variables](doc/screenshot/dashboard_variables.png) | ![Variable query editor](doc/screenshot/variable_query_editor.png) |

### Ad-hoc filters

The filter keys are the real columns of the stream the panel queries, and the
selected filter is injected server side as a SQL predicate — here
`severity = 'ERROR'` narrows the log panel without touching the query:

![Ad-hoc filter](doc/screenshot/dashboard_adhoc_filter.png)

### Datasource settings

Connection, authentication (basic auth or token), TLS, query limits and column
mapping:

| Basic authentication | Token authentication |
|---|---|
| ![Datasource settings](doc/screenshot/config_editor.png) | ![Token authentication](doc/screenshot/config_editor_token.png) |

### Panel gallery

| | |
|---|---|
| **Logs panel**<br>![Logs panel](doc/screenshot/log.png) | **Bar chart**<br>![Bar chart](doc/screenshot/log_bar_chart.png) |
| **Log count histogram**<br>![Log count](doc/screenshot/log_count_time_series.png) | **Pie chart**<br>![Pie chart](doc/screenshot/metrics_pie_chart.png) |
| **Memory usage**<br>![Memory usage](doc/screenshot/memory_usage.png) | |

---

## ✨ Key Features

### Query

- **Multi-type streams** — `logs`, `metrics` and `traces` in one data source
- **SQL editor** with completion for stream names, columns, keywords and functions
- **PromQL mode** against `/api/{org}/prometheus`, so metrics panels behave like a
  native Prometheus data source
- **Result formats**: `Auto`, `Logs`, `Table`, `Time series`, `Trace`, `Annotations`
- **SSE streaming search** (`_search_stream`), with automatic fallback when a
  deployment answers with plain JSON
- **Row limits**: per-query limit → SQL `LIMIT` → datasource default, always capped
  by the configured maximum

### Frames that Grafana understands

- **Automatic time columns** — `_timestamp` (microseconds), `start_time`
  (nanoseconds) and ISO strings all become Grafana `time` fields, so time series
  work without `AS gf_time` or transformations
- **Series labels** — grouping columns become frame labels and therefore legend
  entries
- **Native logs frames** — `time`, `body`, `level`, `labels` plus every original
  column as an attribute, so Explore shows level colours and full details
- **Native trace frames** — spans are emitted with `traceID`, `spanID`,
  `parentSpanID`, `serviceName`, ... so Grafana draws the waterfall
- **Schema-aware column typing** — numbers, booleans, times and JSON are typed from
  the stream schema, with a value-based cross-check so a computed alias never gets
  the wrong type
- **Annotations** — the `Annotations` format turns rows into dashboard annotations

### Grafana integration

- **Ad-hoc filters** injected server side as real SQL predicates: filters are
  AND-ed, existing `OR` chains are parenthesised (so their meaning is preserved),
  values are escaped, `number(123)` stays numeric, and `=~` / `!~` are translated
  to OpenObserve's `~` / `!~`
- **Template variables** with quote-aware SQL formatting and multi-value support
- **Tag keys / tag values** read from the real stream schema, so ad-hoc variables
  autocomplete against your actual columns
- **Actionable errors** — the OpenObserve error body is surfaced (unknown stream,
  bad credentials, unknown organization), and an empty result caused by a stream
  type mismatch explains itself instead of showing a blank panel

### Connection

- Basic auth (user + password) **or** a token sent verbatim in `Authorization`
- Configurable per-query timeout, cancellation propagation, TLS skip-verify
- Health check reports the organization and stream count

---

## 📖 Documentation

- [**Feature guide**](doc/features.md) — query modes, formats, column mapping,
  variables, ad-hoc filters, annotations, and the SQL cookbook for logs / metrics /
  traces
- [**Development guide**](doc/develop.md) — build, run, and test the plugin
- [**Changelog**](CHANGELOG.md) — what changed between versions

---

## 🚀 Installation

### Linux

1. Download the plugin from the [releases page](https://github.com/LinPr/grafana-openobserve-datasource/releases)
2. Extract it into your Grafana plugins directory and allow unsigned plugins:
   ```bash
   # Default plugin path is /var/lib/grafana/plugins/
   sudo unzip grafana-openobserve-datasource.zip -d /var/lib/grafana/plugins/
   export GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=grafana-openobserve-datasource
   ```
3. Restart Grafana

### Docker

```bash
docker run --rm \
  -p 3000:3000 \
  --name=grafana \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=grafana-openobserve-datasource \
  -v /root/grafana/plugins/grafana-openobserve-datasource:/var/lib/grafana/plugins/grafana-openobserve-datasource \
  grafana/grafana
```

### Kubernetes

1. Build an init container that injects the plugin from [docker/Dockerfile](docker/Dockerfile).
   `ARCH` selects `amd64` or `arm64`; `PROXY` sets a module proxy:
   ```bash
   export ARCH=amd64
   export PROXY=http://<host>:<port>
   docker build \
     --target=runtime \
     --build-arg ARCH=${ARCH} \
     --build-arg PROXY=${PROXY} \
     -t grafana-openobserve-datasource:latest-${ARCH} \
     -f docker/Dockerfile \
     .
   ```
2. Copy the plugin into `${GF_PATHS_PLUGINS}` from an init container and allow it in
   the Grafana container (the manifest below is a sketch, complete it for your
   environment):
   ```yaml
   initContainers:
     - command:
         - sh
         - -c
         - rm -rf ${GF_PATHS_PLUGINS}/grafana-openobserve-datasource && mkdir -p ${GF_PATHS_PLUGINS}
           && cp -r grafana-openobserve-plugin ${GF_PATHS_PLUGINS}/grafana-openobserve-datasource
       env:
         - name: GF_PATHS_PLUGINS
           value: /var/lib/grafana/plugins
       image: your-registry/grafana-openobserve-datasource

   containers:
     - env:
         - name: GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS
           value: grafana-openobserve-datasource
       image: grafana/grafana:12.2.0
       name: grafana
   ```

### Build locally

```bash
mage build:linux     # backend binary into dist/
npm install          # frontend dependencies
npm run build        # frontend bundle into dist/

cp -r dist openobserve-grafana-plugin
zip -r openobserve-grafana-plugin.zip openobserve-grafana-plugin
```

---

## 🔍 Usage

### 1. Configure the data source

Point the URL at your OpenObserve server, fill in the organization (usually
`default`) and choose how to authenticate. See
[Datasource settings](doc/screenshot/config_editor.png) above.

### 2. Query logs

```sql
SELECT * FROM "my_logs" LIMIT 500
```

A `SELECT *` over a log stream renders as log lines with level colours and
every original column as an attribute. There is no Format switch to set: the
frame shape follows from the stream type and the statement itself.

### 3. Query metrics

```sql
SELECT histogram(_timestamp, '1 minute') AS ts,
       avg(value) AS value,
       service_name
FROM "go_goroutine_count"
GROUP BY ts, service_name
ORDER BY ts
```

A time column plus numbers renders as a time series, one line per
`service_name`, with no transformations.

Or switch **Query mode** to `PromQL` and write `go_goroutine_count` directly.

### 4. Query traces

Pick the `traces` stream type, choose a trace stream, then pick a query type:

- **Search** lists the traces matching the filters.
- **TraceID** renders one trace as the waterfall.
- **Graph** draws the service dependency graph.

### 5. Builder or Code

The editor has two modes, like Grafana's other datasources:

- **Builder** takes structured inputs — stream, Raw or Aggregate, then function,
  field, interval and group by — and the backend derives the SQL. A read-only
  preview shows the statement that will run. Nothing is written into the Code
  editor, so this is the mode to use when you would rather not write SQL.
- **Code** is the statement you write, with completion for streams and columns.

Switching modes never loses work: the structured fields and the statement are
stored separately, so going back and forth keeps both. A saved query that
already has SQL opens in Code, because that statement was written by hand.

### 6. Limit


The **Limit** field and the SQL `LIMIT` stay in step: typing one updates the
other, so the panel and the statement never disagree about row counts.

### 5. Use variables and ad-hoc filters

A template variable is a SQL query whose first column is the value:

```sql
SELECT DISTINCT service_name AS "value" FROM "my_logs"
```

Then use `$service_name` in a query — the plugin quotes multi-value selections
correctly, so `WHERE service_name IN ($service_name)` just works.

---

## ✅ Compatibility

| Grafana | Status |
| ------- | ------ |
| 12.2.x | tested (primary target) |
| 10.4.4 | tested |

Both versions were exercised against the same live OpenObserve instance: the
health check, log queries, the trace editor (search / trace id / graph) and
PromQL all return data, with no console or page errors.

Grafana provides `@grafana/ui`, `@grafana/data` and `@grafana/runtime` at
runtime, so the plugin only uses APIs that exist in the oldest supported
version. `Combobox`, for example, was added in 12.2 and is deliberately not
used; the deprecated `Select` is kept instead.

## 🛠️ Development

Requirements:

- **Node.js** 22 or newer
- **Go** 1.24.6 or newer
- **OpenObserve** 0.14.7 or newer
- **Docker** for the local Grafana used during development

```bash
npm install
npm run typecheck        # tsc --noEmit
npm run lint             # eslint
npm run test:ci          # jest
npm run build            # webpack

go build ./...
go test ./pkg/...
```

The live verification scripts used to produce the screenshots above live in
[tests/live](tests/live/README.md); they run real queries against a real
OpenObserve and check the resulting frames.

More details are in the [Development Documentation](doc/develop.md).

### Reporting issues

- Use the [GitHub Issues](https://github.com/LinPr/grafana-openobserve-datasource/issues) page
- Provide detailed reproduction steps
- Include relevant logs and screenshots

### Feature requests

- Check existing issues first
- Describe your use case clearly
- Explain the expected behavior

### Contributing

If you have any good ideas, feel free to reach out, or open a pull request for new
features or code enhancements.

---

## 🙏 Acknowledgment

- Thanks to the [Grafana](https://github.com/grafana/grafana) maintainers for the
  `@grafana/ui` library used to build the SQL query editor.
- Thanks to the [OpenObserve](https://github.com/openobserve/openobserve) maintainers
  for their help with the search API and for the many detailed technical answers.
- Thanks to the [jiandaoyun](https://github.com/jiandaoyun) team for supporting the
  project and for feedback that improved it.

## 📄 License

This project is licensed under the **Apache License 2.0** — see the
[LICENSE](LICENSE) file for details.

---

<div align="center">
  <h3>If this helps you, please give it a ⭐</h3>
</div>
