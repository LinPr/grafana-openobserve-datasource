package openobserve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// PromClient talks to the Prometheus compatible endpoints of OpenObserve
// (/api/{org}/prometheus/api/v1/...).
type PromClient struct{ c *Client }

// Prom returns a Prometheus API client sharing this client's connection.
func (c *Client) Prom() *PromClient { return &PromClient{c: c} }

func (p *PromClient) promPath(org, sub string) string {
	if org == "" {
		org = p.c.cfg.Organization
	}
	return fmt.Sprintf("/api/%s/prometheus/api/v1/%s", org, sub)
}

// promQuery executes one Prometheus API call and decodes the envelope.
func (p *PromClient) promQuery(ctx context.Context, path string, values url.Values, out any) error {
	full := path
	if len(values) > 0 {
		full += "?" + values.Encode()
	}
	req, err := p.c.newRequest(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}

	resp, err := p.c.httpClient.Do(req)
	if err != nil {
		return wrapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeHTTPError(resp)
	}

	var envelope PromResponse
	if err := jsonAPI.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("openobserve prometheus: decode response: %w", err)
	}
	if envelope.Status == "error" {
		return fmt.Errorf("openobserve prometheus: %s: %s", envelope.ErrorType, envelope.Error)
	}
	if out == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := jsonAPI.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("openobserve prometheus: decode data: %w", err)
	}
	return nil
}

// QueryRange runs an instant/range PromQL query.
func (p *PromClient) QueryRange(ctx context.Context, org, expr string, start, end time.Time, step time.Duration) (data.Frames, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, fmt.Errorf("the PromQL expression is empty")
	}
	if step <= 0 {
		step = defaultStep(start, end)
	}
	values := url.Values{}
	values.Set("query", expr)
	values.Set("start", strconv.FormatInt(start.Unix(), 10))
	values.Set("end", strconv.FormatInt(end.Unix(), 10))
	values.Set("step", formatStep(step))

	var payload PromData
	if err := p.promQuery(ctx, p.promPath(org, "query_range"), values, &payload); err != nil {
		return nil, err
	}
	return promDataToFrames(payload, "query_range", expr)
}

// Query runs an instant PromQL query at one point in time.
func (p *PromClient) Query(ctx context.Context, org, expr string, at time.Time) (data.Frames, error) {
	values := url.Values{}
	values.Set("query", expr)
	if !at.IsZero() {
		values.Set("time", strconv.FormatInt(at.Unix(), 10))
	}
	var payload PromData
	if err := p.promQuery(ctx, p.promPath(org, "query"), values, &payload); err != nil {
		return nil, err
	}
	return promDataToFrames(payload, "query", expr)
}

// Series returns the label sets matching the expression.
func (p *PromClient) Series(ctx context.Context, org string, match []string, start, end time.Time) ([]map[string]string, error) {
	values := url.Values{}
	for _, m := range match {
		values.Add("match[]", m)
	}
	if !start.IsZero() {
		values.Set("start", strconv.FormatInt(start.Unix(), 10))
	}
	if !end.IsZero() {
		values.Set("end", strconv.FormatInt(end.Unix(), 10))
	}
	var series []map[string]string
	if err := p.promQuery(ctx, p.promPath(org, "series"), values, &series); err != nil {
		return nil, err
	}
	return series, nil
}

// LabelNames returns the label names known to OpenObserve.
func (p *PromClient) LabelNames(ctx context.Context, org string) ([]string, error) {
	var names []string
	if err := p.promQuery(ctx, p.promPath(org, "labels"), nil, &names); err != nil {
		return nil, err
	}
	return names, nil
}

// LabelValues returns the values of one label. OpenObserve requires at least
// one match[] selector, so the caller must pass the metric expression.
func (p *PromClient) LabelValues(ctx context.Context, org, label, match string, start, end time.Time) ([]string, error) {
	values := url.Values{}
	if match != "" {
		values.Set("match[]", match)
	}
	if !start.IsZero() {
		values.Set("start", strconv.FormatInt(start.Unix(), 10))
	}
	if !end.IsZero() {
		values.Set("end", strconv.FormatInt(end.Unix(), 10))
	}
	var out []string
	path := p.promPath(org, "label/"+url.PathEscape(label)+"/values")
	if err := p.promQuery(ctx, path, values, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Metadata returns the metric metadata OpenObserve knows about.
func (p *PromClient) Metadata(ctx context.Context, org string) (map[string][]struct {
	Type string `json:"type"`
	Help string `json:"help"`
	Unit string `json:"unit"`
}, error) {
	var out map[string][]struct {
		Type string `json:"type"`
		Help string `json:"help"`
		Unit string `json:"unit"`
	}
	if err := p.promQuery(ctx, p.promPath(org, "metadata"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// defaultStep mirrors Grafana's Prometheus step heuristics closely enough for
// a readable graph: aim for roughly 1000 points per range, bounded to 15s..5m.
func defaultStep(start, end time.Time) time.Duration {
	span := end.Sub(start)
	if span <= 0 {
		return time.Minute
	}
	step := span / 1000
	switch {
	case step < 15*time.Second:
		step = 15 * time.Second
	case step > 5*time.Minute:
		step = 5 * time.Minute
	}
	return step.Round(time.Second)
}

func formatStep(step time.Duration) string {
	seconds := int64(step / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return strconv.FormatInt(seconds, 10)
}

// promDataToFrames converts a Prometheus matrix/vector payload into Grafana
// time series frames: one wide frame per series with a single value field.
func promDataToFrames(payload PromData, kind, expr string) (data.Frames, error) {
	if len(payload.Result) == 0 {
		return data.Frames{}, nil
	}
	frames := make(data.Frames, 0, len(payload.Result))

	for _, series := range payload.Result {
		labels := make(data.Labels, len(series.Metric))
		for k, v := range series.Metric {
			if k == "__name__" {
				continue
			}
			labels[k] = v
		}
		name := series.Metric["__name__"]
		if name == "" {
			name = expr
		}

		switch payload.ResultType {
		case "matrix", "":
			times := make([]*time.Time, 0, len(series.Values))
			values := make([]*float64, 0, len(series.Values))
			for _, pair := range series.Values {
				if len(pair) < 2 {
					continue
				}
				ts, val, ok := parsePromPair(pair[0], pair[1])
				if !ok {
					continue
				}
				times = append(times, &ts)
				values = append(values, &val)
			}
			if len(times) == 0 {
				continue
			}
			frame := data.NewFrame(name,
				data.NewField("time", nil, times),
				data.NewField(name, labels, values),
			)
			frame.Meta = &data.FrameMeta{
				Type:                   data.FrameTypeTimeSeriesWide,
				TypeVersion:            data.FrameTypeVersion{0, 1},
				PreferredVisualization: data.VisTypeGraph,
				ExecutedQueryString:    expr,
			}
			frames = append(frames, frame)
		case "vector":
			if len(series.Value) < 2 {
				continue
			}
			ts, val, ok := parsePromPair(series.Value[0], series.Value[1])
			if !ok {
				continue
			}
			frame := data.NewFrame(name,
				data.NewField("time", nil, []*time.Time{&ts}),
				data.NewField(name, labels, []*float64{&val}),
			)
			frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeGraph, ExecutedQueryString: expr}
			frames = append(frames, frame)
		case "scalar", "string":
			value := ""
			if len(series.Value) >= 2 {
				value = string(series.Value[1])
			}
			frame := data.NewFrame(name, data.NewField("value", nil, []string{value}))
			frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable, ExecutedQueryString: expr}
			frames = append(frames, frame)
		default:
			return nil, fmt.Errorf("unsupported prometheus result type %q", payload.ResultType)
		}
	}
	return frames, nil
}

// parsePromPair decodes one [timestamp, "value"] sample. Prometheus encodes
// both elements as JSON scalars, so the value arrives as a quoted string and
// the timestamp as a bare number.
func parsePromPair(rawTS, rawVal json.RawMessage) (time.Time, float64, bool) {
	tsStr := strings.Trim(strings.TrimSpace(string(rawTS)), `"`)
	ts, err := strconv.ParseFloat(tsStr, 64)
	if err != nil {
		return time.Time{}, 0, false
	}

	valStr := strings.Trim(strings.TrimSpace(string(rawVal)), `"`)
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		// Prometheus uses "NaN"/"+Inf"/"-Inf" literals, which Grafana renders
		// as gaps; report them as unusable samples.
		return time.Unix(int64(ts), 0), 0, false
	}
	return time.Unix(int64(ts), 0), val, true
}
