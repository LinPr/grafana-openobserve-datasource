package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// resourceHandler serves the HTTP resources the query editor calls.
type resourceHandler struct {
	ds *Datasource
}

func (h *resourceHandler) writeJSON(rw http.ResponseWriter, status int, payload any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(rw).Encode(payload); err != nil {
		log.DefaultLogger.Error("cannot encode resource response", "error", err)
	}
}

func (h *resourceHandler) writeError(rw http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var httpErr *openobserve.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden:
			status = http.StatusUnauthorized
		case httpErr.StatusCode == http.StatusNotFound:
			status = http.StatusNotFound
		case httpErr.StatusCode >= 400 && httpErr.StatusCode < 500:
			status = http.StatusBadRequest
		}
	}
	h.writeJSON(rw, status, map[string]string{"error": err.Error()})
}

// handleStreams returns the streams of one type as a name -> columns map,
// which is the shape the SQL editor completion expects.
//
// GET /openobserve/streams?type=logs&organization=default&withSchema=true
func (h *resourceHandler) handleStreams(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	org := firstNonEmpty(queryParam(req, "organization"), h.ds.client.Organization())
	streamType := firstNonEmpty(queryParam(req, "type"), openobserve.LogsStream)
	withSchema := strings.EqualFold(queryParam(req, "withSchema"), "true")

	resp, err := h.ds.client.ListStreams(ctx, org, streamType)
	if err != nil {
		h.writeError(rw, err)
		return
	}

	if !withSchema {
		names := make([]string, 0, len(resp.List))
		for _, s := range resp.List {
			names = append(names, s.Name)
		}
		sort.Strings(names)
		h.writeJSON(rw, http.StatusOK, names)
		return
	}

	out := make(map[string][]string, len(resp.List))
	for _, s := range resp.List {
		columns := make([]string, 0, len(s.Schema))
		for _, col := range s.Schema {
			columns = append(columns, col.Name)
		}
		sort.Strings(columns)
		out[s.Name] = columns
	}
	h.writeJSON(rw, http.StatusOK, out)
}

// handleSchema returns the typed schema of one stream.
//
// GET /openobserve/schema?type=logs&stream=foo
func (h *resourceHandler) handleSchema(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	org := firstNonEmpty(queryParam(req, "organization"), h.ds.client.Organization())
	streamType := firstNonEmpty(queryParam(req, "type"), openobserve.LogsStream)
	stream := queryParam(req, "stream")
	if stream == "" {
		h.writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "the stream parameter is required"})
		return
	}

	schema, err := h.ds.client.StreamSchema(ctx, org, stream, streamType)
	if err != nil {
		h.writeError(rw, err)
		return
	}
	h.writeJSON(rw, http.StatusOK, schema)
}

// handleTagKeys returns the field names of a stream for ad-hoc filters.
//
// GET /openobserve/tag-keys?type=logs&stream=foo
func (h *resourceHandler) handleTagKeys(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	org := firstNonEmpty(queryParam(req, "organization"), h.ds.client.Organization())
	streamType := firstNonEmpty(queryParam(req, "type"), openobserve.LogsStream)
	stream := queryParam(req, "stream")

	var keys []string
	if stream == "" {
		resp, err := h.ds.client.ListStreams(ctx, org, streamType)
		if err != nil {
			h.writeError(rw, err)
			return
		}
		// Without a stream, expose the stream names themselves: that is the
		// only key that is guaranteed to exist across all streams.
		for _, s := range resp.List {
			keys = append(keys, s.Name)
		}
		sort.Strings(keys)
	} else {
		schema, err := h.ds.client.StreamSchema(ctx, org, stream, streamType)
		if err != nil {
			h.writeError(rw, err)
			return
		}
		for _, col := range schema {
			keys = append(keys, col.Name)
		}
		sort.Strings(keys)
	}
	h.writeJSON(rw, http.StatusOK, keys)
}

// handleTagValues lists the distinct values of one field.
//
// GET /openobserve/tag-values?type=logs&stream=foo&key=service_name&timeFrom=..&timeTo=..
func (h *resourceHandler) handleTagValues(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	org := firstNonEmpty(queryParam(req, "organization"), h.ds.client.Organization())
	streamType := firstNonEmpty(queryParam(req, "type"), openobserve.LogsStream)
	stream := queryParam(req, "stream")
	key := queryParam(req, "key")
	if stream == "" || key == "" {
		h.writeJSON(rw, http.StatusBadRequest, map[string]string{"error": "the stream and key parameters are required"})
		return
	}

	start := parseEpochMicro(queryParam(req, "timeFrom"))
	end := parseEpochMicro(queryParam(req, "timeTo"))
	values, err := h.ds.distinctValues(ctx, org, streamType, stream, key, start, end)
	if err != nil {
		h.writeError(rw, err)
		return
	}
	h.writeJSON(rw, http.StatusOK, values)
}

// distinctValues runs the SELECT DISTINCT query used for ad-hoc values.
func (d *Datasource) distinctValues(ctx context.Context, org, streamType, stream, key string, startUS, endUS int64) ([]string, error) {
	if startUS <= 0 || endUS <= 0 {
		now := nowMicros()
		startUS = now - 3_600_000_000
		endUS = now
	}
	sql := `SELECT DISTINCT "` + strings.ReplaceAll(key, `"`, `""`) + `" FROM "` +
		strings.ReplaceAll(stream, `"`, `""`) + `" ORDER BY 1 LIMIT 1000`

	resp, err := d.client.Search(ctx, &openobserve.SearchRequestParam{
		Organization: org,
		StreamType:   streamType,
		SearchType:   openobserve.SearchTypeUI,
		UseCache:     true,
		EnableSSE:    false,
	}, &openobserve.SearchRequestBody{
		Query: openobserve.Query{
			Sql:       sql,
			StartTime: startUS,
			EndTime:   endUS,
			From:      0,
			Size:      1000,
		},
		SearchType: openobserve.SearchTypeUI,
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(resp.Hits))
	values := make([]string, 0, len(resp.Hits))
	column := ""
	for _, hit := range resp.Hits {
		for name, raw := range hit {
			if column == "" {
				column = name
			}
			if name != column {
				continue
			}
			v := openobserve.ToString(raw)
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			values = append(values, v)
		}
	}
	sort.Strings(values)
	if len(values) > 1000 {
		values = values[:1000]
	}
	return values, nil
}

// handleMetricNames lists metric stream names for PromQL completion.
//
// GET /openobserve/metric-names
func (h *resourceHandler) handleMetricNames(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	org := firstNonEmpty(queryParam(req, "organization"), h.ds.client.Organization())

	if names, err := h.ds.client.Prom().LabelValues(ctx, org, "__name__", "", zeroTime(), zeroTime()); err == nil && len(names) > 0 {
		sort.Strings(names)
		h.writeJSON(rw, http.StatusOK, names)
		return
	}

	resp, err := h.ds.client.ListStreams(ctx, org, openobserve.MetricStream)
	if err != nil {
		h.writeError(rw, err)
		return
	}
	names := make([]string, 0, len(resp.List))
	for _, s := range resp.List {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	h.writeJSON(rw, http.StatusOK, names)
}

// handlePromQLMetadata returns label names plus per-metric metadata for the
// PromQL editor.
//
// GET /openobserve/promql-metadata
func (h *resourceHandler) handlePromQLMetadata(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	org := firstNonEmpty(queryParam(req, "organization"), h.ds.client.Organization())

	labels, err := h.ds.client.Prom().LabelNames(ctx, org)
	if err != nil {
		h.writeError(rw, err)
		return
	}
	payload := map[string]any{"labels": labels}

	if target := queryParam(req, "metric"); target != "" {
		values, err := h.ds.client.Prom().LabelValues(ctx, org, "service_name", target, zeroTime(), zeroTime())
		if err == nil {
			payload["service_names"] = values
		}
	}
	h.writeJSON(rw, http.StatusOK, payload)
}

func queryParam(req *http.Request, key string) string {
	return strings.TrimSpace(req.URL.Query().Get(key))
}

func parseEpochMicro(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n
	}
	return 0
}
