# Development guide

How to build, run and test the OpenObserve data source plugin.

## Requirements

| Tool | Version | Notes |
| ---- | ------- | ----- |
| Go | 1.27.1+ | `go.mod` declares `go 1.27.1`; CI and the Docker build use the same version |
| Node.js | 22+ | used by webpack and jest |
| Docker | any recent | optional, for the dev Grafana |
| OpenObserve | 0.14.7+ | the plugin needs the `_search`, `_search_stream` and `prometheus` APIs |

## Layout

```
pkg/
  main.go                  plugin entry point (datasource.Manage)
  models/                  datasource settings, defaults, validation
  openobserve/
    api.go                 request/response types of the OpenObserve API
    client.go              HTTP client: auth, timeout, SSE, schema cache
    result.go              column model: typing, time detection, labels
    sqlparser.go           SELECT analysis + ad-hoc filter injection
    transformer.go         result set -> Grafana frames
    prometheus.go          Prometheus compatible API client
    testdata/              captured responses used by the tests
  plugin/
    datasource.go          instance lifecycle, query routing, health check
    queryhandler.go        SQL query orchestration
    promql.go              PromQL query
    resources.go           editor resources (streams, schema, tag keys, ...)

src/
  module.ts                plugin registration
  datasource.ts            DataSourceWithBackend facade
  types.ts                 shared types and defaults
  variables.ts             template variable support
  components/              query editor, config editor, variable editor
  utils/                   SQL interpolation and completion helpers
```

Everything that understands OpenObserve lives in Go. The frontend only collects
parameters and renders results, which keeps the two sides able to evolve
separately.

## Build

### Backend

```bash
go build ./...
```

Cross-compile the binary Grafana loads:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/linpr-grafana-openobserve-datasource_linux_amd64 ./pkg
```

Or use mage, which builds every platform into `dist/`:

```bash
mage -v            # all platforms
mage build:linux   # just linux/amd64
mage -l            # list targets
```

### Frontend

```bash
npm install
npm run build      # production bundle into dist/
npm run dev        # watch mode
```

`npm run build` writes `dist/module.js` and copies `plugin.json`, `img/` and the
README into `dist/`, so `dist/` is the directory Grafana loads.

## Run against a real Grafana

The plugin is unsigned, so Grafana has to be told to load it.

```bash
export GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=linpr-openobserve-datasource
docker compose up --build
```

The compose file mounts `dist/` into the Grafana container, so re-running
`npm run build` is enough to pick up frontend changes; backend changes need the
binary rebuilt.

Provision the data source instead of clicking through the UI by copying
[provisioning/datasources/datasources.yml](../provisioning/datasources/datasources.yml)
into Grafana's provisioning directory.

## Test

### Go

```bash
go test ./pkg/...
go vet ./...
```

The tests use `httptest` servers with responses captured from a real
OpenObserve, which are stored in `pkg/openobserve/testdata/`. They cover the
client (auth, token, TLS skip-verify, timeout, cancellation, SSE, errors), the
SQL parser and ad-hoc filter injection, the frame builder (typing, time columns,
multi-series, traces), the query handler, the health check and the resource
endpoints.

### Frontend

```bash
npm run typecheck
npm run lint
npm run test:ci
```

Frontend tests render the real components with `@testing-library/react`:
`CodeEditor` (monaco) and `Select` are stubbed because neither works in jsdom,
and `jest-setup.js` provides the `IntersectionObserver` / `ResizeObserver` stubs
that `@grafana/ui` widgets expect.

Jest is configured to ignore `.gocache/` and `.runtime/` (see `jest.config.js`),
because both live inside the repository and contain thousands of foreign
`package.json` files.

### Live verification

`tests/live/` contains scripts that exercise a running Grafana + OpenObserve:

```bash
python3 tests/live/e2e_query.py <datasource-uid>   # every query shape, via /api/ds/query
node tests/live/ui_dash_check.mjs                  # creates a dashboard and screenshots it
```

See [tests/live/README.md](../tests/live/README.md).

## Releasing

```bash
npm run build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/linpr-grafana-openobserve-datasource_linux_amd64 ./pkg
mage -v                       # or build each platform explicitly
npx @grafana/sign-plugin@latest
```

Bump `info.version` in `src/plugin.json` and add a `CHANGELOG.md` entry first.

## Troubleshooting

| Symptom | Cause |
| ------- | ----- |
| `Plugin validation failed ... invalid signature` | `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS` is not set |
| Panel shows "No data" but the query works in Explore | the stream type does not match the stream (a log stream queried as `metrics`); the backend reports this explicitly |
| Time series renders but the X axis is wrong | the timestamp column was not recognised; set `Time field` in the datasource settings |
| `authentication failed (HTTP 401)` | wrong user/password, or the organization does not exist |
| Frontend build fails with `Combobox` not found | `@grafana/ui` is provided by Grafana at runtime; the plugin targets Grafana 10.4+, so it cannot use APIs added in 12.2 |
