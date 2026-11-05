# 🔌 Grafana OpenObserve Data Source Plugin

<div align="center">

![GitHub](https://img.shields.io/github/license/LinPr/grafana-openobserve-datasource?style=flat-square)
![Grafana](https://img.shields.io/badge/Grafana-10.4%2B-orange?style=flat-square&logo=grafana)
![OpenObserve](https://img.shields.io/badge/OpenObserve-Supported-blue?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat-square&logo=go)
![TypeScript](https://img.shields.io/badge/TypeScript-5.5.4+-3178C6?style=flat-square&logo=typescript)
![Node.js](https://img.shields.io/badge/Node.js-22+-339933?style=flat-square&logo=node.js)

**English** · [简体中文](README.zh-CN.md)

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

The plugin is identified as `linpr-openobserve-datasource`, which is what a
dashboard, a provisioned data source or `grafana-cli` refers to:

| Item               | Value                                          |
| ------------------ | ---------------------------------------------- |
| Data source type   | `linpr-openobserve-datasource`                 |
| Backend executable | `linpr-grafana-openobserve-datasource`         |
| Container image    | `ghcr.io/linpr/grafana-openobserve-datasource` |
| Minimum Grafana    | 10.4.0                                         |

> The type's first segment is the Grafana Cloud organization slug (`linpr`), as the
> plugin catalogue requires. The **binary** keeps the `grafana-` prefix so a
> running process or a stray file is still recognisable as a Grafana plugin.

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

> The GIF is 2.4 MB and holds each view for half a second. The sharper
> [WebP version](doc/screenshot/tour.webp) is only 0.35 MB and shows the same
> tour at higher resolution; every view is also available as a still image
> below.

### Logs in Explore

Log lines render with their level colour, and the details pane shows every
original column as an attribute:

| Logs in Explore                                     | The query editor                                 |
| --------------------------------------------------- | ------------------------------------------------ |
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

| Metrics over SQL                                            | PromQL                                       |
| ----------------------------------------------------------- | -------------------------------------------- |
| ![Metrics over SQL](doc/screenshot/explore_metrics_sql.png) | ![PromQL](doc/screenshot/explore_promql.png) |

### Traces: three query types

The trace editor mirrors Jaeger's three modes and the backend builds the SQL for
each one. **Search** lists the traces matching the filters, and clicking a trace
id opens that trace inside Grafana. **TraceID** loads every span of one trace so
Explore draws the native waterfall. **Graph** derives service-to-service call
counts from the span parent links and renders them as a node graph, taking the
callee from OpenObserve's inferred service name so a client span attributes to
the system it calls rather than to the caller.

| Search                                           | TraceID (waterfall)                                    | Graph                                          |
| ------------------------------------------------ | ------------------------------------------------------ | ---------------------------------------------- |
| ![Trace search](doc/screenshot/trace_search.png) | ![Trace waterfall](doc/screenshot/trace_waterfall.png) | ![Trace graph](doc/screenshot/trace_graph.png) |

### A dashboard with everything mixed

Logs, stats, a pie chart, a stacked time series, PromQL and a trace table from a
single data source:

![Dashboard](doc/screenshot/dashboard_logs_metrics.png)

### Template variables

A variable is just a SQL query — the first column becomes the value. Multi-value
selections are correctly quoted into `IN (...)`:

| Dashboard with variables                                       | The variable query editor                                          |
| -------------------------------------------------------------- | ------------------------------------------------------------------ |
| ![Dashboard variables](doc/screenshot/dashboard_variables.png) | ![Variable query editor](doc/screenshot/variable_query_editor.png) |

### Ad-hoc filters

The filter keys are the real columns of the stream the panel queries, and the
selected filter is injected server side as a SQL predicate — here
`severity = 'ERROR'` narrows the log panel without touching the query:

![Ad-hoc filter](doc/screenshot/dashboard_adhoc_filter.png)

### Datasource settings

Connection, authentication (basic auth or token), TLS, query limits and column
mapping:

| Basic authentication                                     | Token authentication                                            |
| -------------------------------------------------------- | --------------------------------------------------------------- |
| ![Datasource settings](doc/screenshot/config_editor.png) | ![Token authentication](doc/screenshot/config_editor_token.png) |

### Panel gallery

|                                                                                   |                                                                     |
| --------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| **Logs panel**<br>![Logs panel](doc/screenshot/log.png)                           | **Bar chart**<br>![Bar chart](doc/screenshot/log_bar_chart.png)     |
| **Log count histogram**<br>![Log count](doc/screenshot/log_count_time_series.png) | **Pie chart**<br>![Pie chart](doc/screenshot/metrics_pie_chart.png) |
| **Memory usage**<br>![Memory usage](doc/screenshot/memory_usage.png)              |                                                                     |

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
- [**中文说明**](README.zh-CN.md) — Chinese documentation

---

## 🚀 Installation

Pick the route that matches your Grafana:

1. **Grafana catalogue** — once the plugin is listed and signed, install it from
   Grafana Cloud or with
   `grafana-cli plugins install linpr-openobserve-datasource`. Unsigned builds
   from the releases page cannot be installed this way.
2. **Release archive** — download the archive for your platform from
   [Releases](https://github.com/LinPr/grafana-openobserve-datasource/releases)
   and unpack it into the plugin directory, as described below.
3. **Container image** — copy the plugin out of
   `ghcr.io/linpr/grafana-openobserve-datasource` (see [Docker](#docker)).

Every release ships **one archive per platform** plus a universal archive that
holds all of them:

| Archive                           | Contents                                         |
| --------------------------------- | ------------------------------------------------ |
| `...-<version>.zip`               | all platforms, what Grafana's catalogue expects  |
| `...-<version>.linux-amd64.zip`   | Linux x86-64                                     |
| `...-<version>.linux-arm64.zip`   | Linux ARM64 (Ampere, Graviton, Raspberry Pi 4/5) |
| `...-<version>.linux-arm.zip`     | Linux ARMv7 (32-bit)                             |
| `...-<version>.darwin-amd64.zip`  | macOS Intel                                      |
| `...-<version>.darwin-arm64.zip`  | macOS Apple Silicon                              |
| `...-<version>.windows-amd64.zip` | Windows x86-64                                   |

`checksums.txt` lists the SHA-1 of each.

### Linux / macOS

```bash
# Pick the archive for your platform, then:
sudo unzip linpr-openobserve-datasource-<version>.linux-amd64.zip \
  -d /var/lib/grafana/plugins/
export GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource
```

Restart Grafana afterwards.

### Docker

A multi-architecture image is published for `linux/amd64` and `linux/arm64`, so
`docker pull` selects the right one:

```bash
docker run -d --name grafana -p 3000:3000 \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource \
  -v grafana-plugins:/var/lib/grafana/plugins \
  grafana/grafana

# copy the plugin in from the published image
docker run --rm -v grafana-plugins:/var/lib/grafana/plugins \
  ghcr.io/linpr/grafana-openobserve-datasource:latest \
  cp -a /var/lib/grafana/plugins/linpr-openobserve-datasource /var/lib/grafana/plugins/
```

Or mount it directly, which needs no volume juggling:

```bash
docker run -d --name grafana -p 3000:3000 \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource \
  --mount type=bind,source=/opt/grafana-plugins/linpr-openobserve-datasource,target=/var/lib/grafana/plugins/linpr-openobserve-datasource \
  grafana/grafana
```

### Provisioning

A provisioned data source is the fastest way to get a fleet onto the plugin.
[`provisioning/datasources/datasources.yml`](provisioning/datasources/datasources.yml)
is a complete example:

```yaml
apiVersion: 1

datasources:
  - name: 'OpenObserve'
    type: 'linpr-openobserve-datasource'
    access: proxy
    url: 'http://localhost:8000'
    user: 'user'
    jsonData:
      organization: 'default' # the legacy `database` key still works
      authMethod: 'basic' # or 'token'
      timeoutSeconds: 60
      defaultLimit: 500
      maxLimit: 10000
      enablePrometheus: true
    secureJsonData:
      password: 'password' # or `token` when authMethod is 'token'
```

Keep the generated `uid` when you migrate an existing data source, so dashboards
that reference it keep resolving.

### Upgrading from 2.3.x

Version 2.4.0 renamed the data source type from `grafana-openobserve-datasource`
to `linpr-openobserve-datasource`, because the catalogue rejected the old id
(`grafana` belongs to Grafana Labs' organization). Consequences:

- Dashboards and provisioned data sources that point at the old type will not
  resolve. Re-select the data source on the affected panels, or edit the
  provisioning file's `type:`. Keep the `uid` and the panels keep working.
- The backend binary is now `linpr-grafana-openobserve-datasource` instead of
  `gpx_open_observe`. Nothing needs to know that name — Grafana reads it from
  `plugin.json` — but a manual install must replace the whole plugin directory
  rather than just the binary.
- Nothing else changes; queries, settings and stored JSON are compatible.

---

## 🔍 Usage

### 1. Configure the data source

Point the URL at your OpenObserve server, fill in the organization (usually
`default`) and choose how to authenticate. See
[Datasource settings](doc/screenshot/config_editor.png) above. **Save & test**
reports the organization it reached and how many streams it found, so a wrong
URL, a wrong organization or bad credentials are caught before you write a query.

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

### 7. Use variables and ad-hoc filters

A template variable is a SQL query whose first column is the value:

```sql
SELECT DISTINCT service_name AS "value" FROM "my_logs"
```

Then use `$service_name` in a query — the plugin quotes multi-value selections
correctly, so `WHERE service_name IN ($service_name)` just works.

For dashboards, enable an **ad-hoc filter** variable of type
`linpr-openobserve-datasource`; its keys come from the stream schema and the
chosen filter becomes an extra `WHERE` predicate on every panel that uses the
data source.

### 8. Annotations

Add an annotation query, pick the `Annotations` format, and return a time column
plus the text you want to see:

```sql
SELECT _timestamp AS time, body AS text
FROM "my_logs"
WHERE level = 'error'
```

The time column becomes the annotation timestamp and the body/text column becomes
the title and description, so deploys, alerts and incidents from your log stream
show up on the dashboard timeline.

---

## ✅ Compatibility

The last patch release of every supported major was tested against the same live
OpenObserve instance:

| Grafana | Health | Logs (builder) | Metrics (builder) | PromQL | Trace search | Trace graph | UI  |
| ------- | :----: | :------------: | :---------------: | :----: | :----------: | :---------: | :-: |
| 10.4.19 |   ✅   |       ✅       |        ✅         |   ✅   |      ✅      |     ✅      | ✅  |
| 11.6.16 |   ✅   |       ✅       |        ✅         |   ✅   |      ✅      |     ✅      | ✅  |
| 12.4.14 |   ✅   |       ✅       |        ✅         |   ✅   |      ✅      |     ✅      | ✅  |
| 13.3.0  |   ✅   |       ✅       |        ✅         |   ✅   |      ✅      |     ✅      | ✅  |

CI also runs the same Playwright suite against the `nightly` image, so a future
Grafana breaking the plugin is noticed before it is released.

"UI" means the trace editor rendered in Explore with its three query types, the
Attributes field and a populated result table, with no page errors and no
console errors.

Grafana provides `@grafana/ui`, `@grafana/data` and `@grafana/runtime` at
runtime, so the plugin only uses APIs that exist in the oldest supported
version. `Combobox`, for example, was added in 12.2 and is deliberately not
used; the deprecated `Select` is kept instead.

## 🛠️ Development

Requirements:

- **Node.js** 22 or newer
- **Go** 1.27.1 or newer (`go.mod` pins it, so CI and local agree)
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

To run the plugin against a real Grafana, set
`GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource` and start
`docker compose up --build`; the compose file mounts `dist/` into the container,
so `npm run build` (or `npm run dev` for watch mode) is enough to pick up frontend
changes.

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

## 🩺 Troubleshooting

| Symptom                                                  | Cause and fix                                                                                                                                    |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| _Plugin not found_ in the data source list               | Grafana skips unsigned plugins. Set `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource` and restart.                        |
| _Plugin is not signed_ warning in the plugin list        | Expected for a self-built or release-archive install; it disappears once the plugin is installed from the catalogue.                             |
| A panel is empty with no error                           | Usually a stream-type mismatch — a `metrics` stream queried as `logs`. The backend explains the empty result in the panel's error/notice area.   |
| `PromQL` is missing from **Query mode**                  | It is offered for the `metrics` stream type only, and only when _Enable PromQL_ is on in the data source settings.                               |
| `connection refused` / `no such host` on **Save & test** | The URL must be reachable from the Grafana server, not from your browser. In Docker, `localhost` is the container.                               |
| `401` / `403` on **Save & test**                         | Check the OpenObserve organization, the user/password pair, or that the token was sent verbatim (both `Basic <base64>` and `Bearer <jwt>` work). |
| Time series have no time axis                            | The time column was not recognised. Set _Time field_ explicitly, or alias it (`AS ts`) and keep it in `GROUP BY`.                                |

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
