# syntax=docker/dockerfile:1
#
# Builds a Grafana plugin image, one per architecture.
#
# The image is a *plugin carrier*: it holds the built plugin in the layout
# Grafana reads from /var/lib/grafana/plugins, so it can be used as an init
# container or bind-mounted into a Grafana deployment. It is not itself a
# Grafana image.
#
# Everything is named after the plugin id (linpr-openobserve-datasource).
# The backend binary is named after the project instead, so a stray process or
# artefact is recognisable as this Grafana plugin.
ARG PLUGIN_ID=linpr-openobserve-datasource
ARG PLUGIN_EXE=linpr-grafana-openobserve-datasource

FROM --platform=$BUILDPLATFORM node:22-bookworm AS frontend
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm AS backend
ARG TARGETOS=linux
ARG TARGETARCH
ARG PLUGIN_EXE=linpr-grafana-openobserve-datasource
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Each platform builds its own binary, named the way Grafana looks one up:
# <executable>_<os>_<arch>. The frontend above is platform independent, so it
# only runs once even when several architectures are built.
RUN set -eux; \
    mkdir -p /out; \
    suffix=""; \
    if [ "${TARGETOS}" = "windows" ]; then suffix=".exe"; fi; \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
      go build -trimpath -ldflags="-s -w" \
      -o "/out/${PLUGIN_EXE}_${TARGETOS}_${TARGETARCH}${suffix}" ./pkg

FROM busybox:1.37
ARG TARGETOS=linux
ARG TARGETARCH
ARG PLUGIN_ID=linpr-openobserve-datasource
ARG PLUGIN_EXE=linpr-grafana-openobserve-datasource

COPY --from=frontend /app/dist /var/lib/grafana/plugins/${PLUGIN_ID}
COPY --from=backend /out/${PLUGIN_EXE}_${TARGETOS}_${TARGETARCH} \
     /var/lib/grafana/plugins/${PLUGIN_ID}/${PLUGIN_EXE}_${TARGETOS}_${TARGETARCH}

LABEL org.opencontainers.image.title="linpr-grafana-openobserve-datasource" \
      org.opencontainers.image.description="OpenObserve data source for Grafana: logs, metrics and traces" \
      org.opencontainers.image.source="https://github.com/LinPr/grafana-openobserve-datasource" \
      org.opencontainers.image.licenses="Apache-2.0"

ENV GF_PLUGINS_DIR=/var/lib/grafana/plugins
WORKDIR /var/lib/grafana/plugins/${PLUGIN_ID}
