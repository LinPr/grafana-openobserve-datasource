# Live verification scripts

These scripts exercise the plugin against a running Grafana instance with the
plugin installed and an OpenObserve datasource provisioned. They are the harness
used while developing; they are not part of the build.

## Prerequisites

1. Grafana is running on `http://127.0.0.1:3000` with an admin user.
2. The datasource is provisioned and pointing at a live OpenObserve
   (see `provisioning/datasources/datasources.yml`).

## `e2e_query.py`

Runs every query shape the plugin supports through Grafana's `/api/ds/query`
and checks the resulting frames:

```bash
python3 tests/live/e2e_query.py <datasource-uid>
```

Set `GRAFANA_URL` when Grafana is not on `http://127.0.0.1:3000`.

It covers logs, tables, histograms, multi-series metrics, PromQL, traces,
ad-hoc filters, template variables, annotations and the error paths (unknown
stream, non-SELECT statement). Exit code is non-zero when a case fails.

## `ui_dash_check.mjs`

Creates a dashboard through the Grafana API with one panel per feature and
screenshots it, so the rendering can be checked visually:

```bash
DS_UID=<datasource-uid> node tests/live/ui_dash_check.mjs
```

Requires `playwright` and a Chrome binary at `/usr/bin/google-chrome`.

## `tour/` — the README animation

Produces `doc/screenshot/tour.gif` and `doc/screenshot/tour.webp`, the feature
walkthrough embedded in the README. Every frame is a real screenshot of a live
Grafana driving a live OpenObserve.

```bash
# 1. record: writes tests/live/tour/frames/*.png
GRAFANA_URL=http://127.0.0.1:3000 DS_UID=<datasource-uid> node tests/live/tour/record.mjs

# 2. assemble: downscales, dedupes held frames, writes both formats
python3 tests/live/tour/make_tour.py
```

The tour visits the dashboards `oo-showcase` and `oo-vars`, which are created by
the other scripts in this directory; create them first or the recording will
capture "not found" pages. `tests/live/tour/frames/` is git-ignored — only the
two finished animations are committed.
