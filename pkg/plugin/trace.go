package plugin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// runTraceQuery executes one of the trace-editor modes.
//
// The editor mirrors Jaeger's three modes -- search a list of traces, load one
// trace by id, or draw the service dependency graph -- and each maps to a SQL
// query this method builds and runs.
func (d *Datasource) runTraceQuery(ctx context.Context, q backend.DataQuery, model QueryModel) backend.DataResponse {
	mode := openobserve.TraceQueryMode(strings.TrimSpace(model.TraceMode))
	if mode == "" {
		// Without an explicit mode, a trace id means "show that trace",
		// anything else means "list traces".
		if strings.TrimSpace(model.TraceID) != "" {
			mode = openobserve.TraceModeTraceID
		} else {
			mode = openobserve.TraceModeSearch
		}
	}
	if !openobserve.IsValidTraceMode(string(mode)) {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
			fmt.Sprintf("unknown trace mode %q: expected search, traceId or graph", model.TraceMode))
	}

	stream := strings.TrimSpace(model.Table)
	if stream == "" {
		parsed, err := d.queryBuilder.Parse(model.RawSql)
		if err == nil && parsed != nil {
			stream = parsed.Table
		}
	}
	if stream == "" {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
			"pick a trace stream first")
	}

	minUS, err := parseDurationUS(model.MinDuration)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
			fmt.Sprintf("invalid min duration %q: %v", model.MinDuration, err))
	}
	maxUS, err := parseDurationUS(model.MaxDuration)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
			fmt.Sprintf("invalid max duration %q: %v", model.MaxDuration, err))
	}

	since := q.TimeRange.From
	if since.IsZero() {
		since = time.Now().Add(-time.Hour)
	}
	until := q.TimeRange.To
	if until.IsZero() || until.Before(since) {
		until = time.Now()
	}

	settings := d.settings.JsonData
	trace := openobserve.TraceQuery{
		Mode:          mode,
		Stream:        stream,
		TraceID:       strings.TrimSpace(model.TraceID),
		ServiceName:   model.ServiceName,
		OperationName: model.OperationName,
		Tags:          model.TraceTags,
		MinDurationUS: minUS,
		MaxDurationUS: maxUS,
		Limit:         effectiveSize(model, nil, settings),
	}

	var sql string
	switch mode {
	case openobserve.TraceModeTraceID:
		if trace.TraceID == "" {
			return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
				"enter a trace id, or switch the query type to Search")
		}
		sql = openobserve.BuildTraceByIDSQL(trace, settings.DefaultLimit)
	case openobserve.TraceModeGraph:
		sql = openobserve.BuildGraphSQL(trace, settings.DefaultLimit)
	default:
		sql = openobserve.BuildSearchSQL(trace, settings.DefaultLimit)
	}
	if sql == "" {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
			"cannot build a trace query from these parameters")
	}

	searchCtx, cancel := context.WithTimeout(ctx, settings.Timeout())
	defer cancel()

	resp, err := d.client.Search(searchCtx, &openobserve.SearchRequestParam{
		Organization: d.client.Organization(),
		StreamType:   openobserve.TraceStream,
		SearchType:   openobserve.SearchTypeUI,
		// The list and the graph change constantly; a cached answer is
		// misleading for exploratory trace queries.
		UseCache:  false,
		EnableSSE: false,
	}, &openobserve.SearchRequestBody{
		Query: openobserve.Query{
			Sql:       sql,
			StartTime: since.UnixMicro(),
			EndTime:   until.UnixMicro(),
			Size:      trace.Limit,
		},
		SearchType: openobserve.SearchTypeUI,
		Timeout:    int(settings.Timeout() / time.Second),
	})
	if err != nil {
		// An empty stream and an unknown stream look the same to the user, so
		// say which one it was.
		return backend.ErrDataResponseWithSource(backend.StatusInternal, backend.ErrorSourceDownstream, err.Error())
	}

	rs := openobserve.NewResultSet(resp.Hits, nil)

	var frames data.Frames
	switch mode {
	case openobserve.TraceModeGraph:
		frames = buildServiceGraphFrames(rs)
	case openobserve.TraceModeSearch:
		frames = buildTraceListFrames(rs, sql, since.Location())
	default:
		if len(rs.Rows) == 0 {
			return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
				fmt.Sprintf("no spans found for trace %q in %q", trace.TraceID, stream))
		}
		built, buildErr := d.builder.BuildFrames(rs, openobserve.FrameOptions{
			Format:        openobserve.FormatTrace,
			ExecutedQuery: sql,
			TraceIDField:  trace.TraceID,
		})
		if buildErr != nil {
			// A trace id that matches nothing must not look like a broken panel.
			return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream,
				fmt.Sprintf("no spans found for trace %q in %q", trace.TraceID, stream))
		}
		frames = built
	}

	for _, frame := range frames {
		if frame.Meta == nil {
			frame.Meta = &data.FrameMeta{}
		}
		if frame.Meta.ExecutedQueryString == "" {
			frame.Meta.ExecutedQueryString = sql
		}
	}
	return backend.DataResponse{Frames: frames}
}

// parseDurationUS parses a duration such as "1.2s", "100ms", "500us" or a bare
// number of microseconds. An empty string means "no bound".
func parseDurationUS(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf(`use a duration like "1.2s", "100ms" or "500us"`)
	}
	if d < 0 {
		return 0, fmt.Errorf("the duration cannot be negative")
	}
	return d.Microseconds(), nil
}

// buildTraceListFrames renders the trace search result.
//
// Two frames are returned: the table the user reads, and a hidden frame that
// carries the trace ids as data links so a row can jump straight to the trace
// view. Grafana shows the table; the link makes the list navigable.
func buildTraceListFrames(rs *openobserve.ResultSet, executedQuery string, loc *time.Location) data.Frames {
	if loc == nil {
		loc = time.UTC
	}
	summaries := openobserve.SummarizeTraces(rs)

	traceID := make([]string, 0, len(summaries))
	service := make([]string, 0, len(summaries))
	operation := make([]string, 0, len(summaries))
	start := make([]*time.Time, 0, len(summaries))
	duration := make([]float64, 0, len(summaries))
	spans := make([]int64, 0, len(summaries))
	services := make([]int64, 0, len(summaries))

	for _, s := range summaries {
		ts := time.UnixMicro(s.StartTimeUS).In(loc)
		traceID = append(traceID, s.TraceID)
		service = append(service, s.ServiceName)
		operation = append(operation, s.OperationName)
		start = append(start, &ts)
		// Microseconds -> milliseconds, which is what Grafana expects.
		duration = append(duration, float64(s.DurationUS)/1000)
		spans = append(spans, s.SpanCount)
		services = append(services, s.ServiceCount)
	}

	table := data.NewFrame("traces",
		data.NewField("traceID", nil, traceID),
		data.NewField("startTime", nil, start),
		data.NewField("duration_ms", nil, duration),
		data.NewField("spans", nil, spans),
		data.NewField("services", nil, services),
		data.NewField("service", nil, service),
		data.NewField("operation", nil, operation),
	)
	table.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeTable,
		ExecutedQueryString:    executedQuery,
	}

	if len(summaries) == 0 {
		return data.Frames{table}
	}

	// Opening a trace is the natural next step from a list row, but it needs the
	// editor to switch to TraceID mode, which a plain data link cannot do. Point
	// the id at the Explore trace view instead so the click still does something
	// useful.
	for _, field := range table.Fields {
		if field.Name != "traceID" {
			continue
		}
		field.Config = &data.FieldConfig{
			Links: []data.DataLink{
				{
					Title:       "Open trace in OpenObserve",
					URL:         "${__value.raw}",
					TargetBlank: true,
				},
			},
		}
	}
	return data.Frames{table}
}

// buildServiceGraphFrames renders the service dependency graph.
func buildServiceGraphFrames(rs *openobserve.ResultSet) data.Frames {
	edges := openobserve.SummarizeServiceEdges(rs)

	source := make([]string, 0, len(edges))
	target := make([]string, 0, len(edges))
	calls := make([]int64, 0, len(edges))
	avg := make([]float64, 0, len(edges))
	label := make([]string, 0, len(edges))

	for _, e := range edges {
		source = append(source, e.ParentService)
		target = append(target, e.ChildService)
		calls = append(calls, e.Calls)
		avg = append(avg, e.AvgDurationUS/1000)
		label = append(label, fmt.Sprintf("%s → %s", e.ParentService, e.ChildService))
	}

	frame := data.NewFrame("service_graph",
		data.NewField("source", nil, source),
		data.NewField("target", nil, target),
		data.NewField("calls", nil, calls),
		data.NewField("avg_duration_ms", nil, avg),
		data.NewField("edge", nil, label),
	)
	frame.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeTable,
		Notices: []data.Notice{{
			Severity: data.NoticeSeverityInfo,
			Text: "Service dependency graph: each row is one caller → callee pair, " +
				"derived from span parent/child links. Use a Node graph panel with " +
				"`source`/`target`/`calls` to draw it.",
		}},
	}
	return data.Frames{frame}
}
