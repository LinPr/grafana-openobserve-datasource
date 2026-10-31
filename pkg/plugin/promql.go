package plugin

import (
	"context"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// runPromQLQuery executes a PromQL expression against OpenObserve's Prometheus
// compatible API.
func (d *Datasource) runPromQLQuery(ctx context.Context, q backend.DataQuery, model QueryModel) backend.DataResponse {
	if !d.settings.JsonData.PrometheusEnabled() {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
			"PromQL mode is disabled for this datasource; enable it in the datasource settings")
	}
	expr := model.Expr
	if expr == "" {
		// Fall back to the SQL field so dashboards authored before this
		// feature keep working after switching the mode.
		expr = model.RawSql
	}
	if expr == "" {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, "the PromQL expression is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, d.settings.JsonData.Timeout())
	defer cancel()

	from := q.TimeRange.From
	to := q.TimeRange.To
	if from.IsZero() {
		from = time.Now().Add(-time.Hour)
	}
	if to.IsZero() || to.Before(from) {
		to = time.Now()
	}

	// An instant query is used when the range collapses to a single point,
	// which is what Grafana's stat/gauge panels ask for.
	var (
		frames data.Frames
		err    error
	)
	if q.MaxDataPoints > 0 && to.Sub(from) <= time.Second {
		frames, err = d.client.Prom().Query(ctx, d.client.Organization(), expr, to)
	} else {
		frames, err = d.client.Prom().QueryRange(ctx, d.client.Organization(), expr, from, to, promStep(from, to, q.MaxDataPoints))
	}
	if err != nil {
		log.DefaultLogger.Error("promql query failed", "refId", q.RefID, "expr", expr, "error", err)
		return backend.ErrDataResponseWithSource(backend.StatusInternal, backend.ErrorSourceDownstream, err.Error())
	}
	if len(frames) == 0 {
		return backend.DataResponse{Frames: data.Frames{}}
	}
	for _, frame := range frames {
		if frame.Meta == nil {
			frame.Meta = &data.FrameMeta{}
		}
		frame.Meta.ExecutedQueryString = expr
	}
	return backend.DataResponse{Frames: frames}
}

// promStep derives the query step from Grafana's max data points so the graph
// resolution matches what the panel expects.
func promStep(from, to time.Time, maxDataPoints int64) time.Duration {
	if maxDataPoints <= 0 {
		maxDataPoints = 1000
	}
	span := to.Sub(from)
	if span <= 0 {
		return time.Minute
	}
	step := span / time.Duration(maxDataPoints)
	if step < time.Second {
		return time.Second
	}
	return step.Round(time.Second)
}

// nowMicros returns the current unix time in microseconds.
func nowMicros() int64 { return time.Now().UnixMicro() }

// zeroTime returns the zero time, used to omit optional query parameters.
func zeroTime() time.Time { return time.Time{} }
