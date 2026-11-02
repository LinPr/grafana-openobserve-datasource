package plugin

import (
	"context"
	"fmt"
	"sort"
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
		frames = buildTraceListFrames(rs, sql, since.Location(), d.datasourceUID(), stream)
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
// buildTraceListFrames renders the trace search result.
//
// The columns match what Grafana's Jaeger plugin returns for a search, so the
// trace list behaves the same way here: traceID / traceName / startTime /
// duration, with startTime in milliseconds and duration in microseconds. A
// trace name is "service: operation", which is what Jaeger shows in its list.
func buildTraceListFrames(rs *openobserve.ResultSet, executedQuery string, loc *time.Location, datasourceUID, stream string) data.Frames {
	if loc == nil {
		loc = time.UTC
	}
	summaries := openobserve.SummarizeTraces(rs)

	traceID := make([]string, 0, len(summaries))
	traceName := make([]string, 0, len(summaries))
	startTime := make([]*time.Time, 0, len(summaries))
	duration := make([]float64, 0, len(summaries))

	for _, s := range summaries {
		name := s.OperationName
		if s.ServiceName != "" {
			if name == "" {
				name = s.ServiceName
			} else {
				name = s.ServiceName + ": " + name
			}
		}
		// start_time is a nanosecond epoch here (19 digits); reading it as
		// microseconds put every trace in the year 2106.
		ts := time.Unix(0, s.StartTimeNS).In(loc)
		traceID = append(traceID, s.TraceID)
		traceName = append(traceName, name)
		// A real time field and a milliseconds field let Grafana format both
		// as a timestamp and a readable duration instead of raw numbers.
		startTime = append(startTime, &ts)
		duration = append(duration, float64(s.DurationUS)/1000)
	}

	startField := data.NewField("startTime", nil, startTime)
	durationField := data.NewField("duration", nil, duration)
	// Unit "ms" is what makes Grafana render "459 ms" rather than "459".
	durationField.Config = &data.FieldConfig{Unit: "ms"}

	traceIDField := data.NewField("traceID", nil, traceID)
	if len(traceID) > 0 {
		traceIDField.Config = &data.FieldConfig{Links: traceIDLinks(datasourceUID, stream, traceID)}
	}

	frame := data.NewFrame("traces",
		traceIDField,
		data.NewField("traceName", nil, traceName),
		startField,
		durationField,
	)
	frame.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeTable,
		ExecutedQueryString:    executedQuery,
	}
	return data.Frames{frame}
}

func traceIDLinks(datasourceUID, stream string, ids []string) []data.DataLink {
	if len(ids) == 0 {
		return nil
	}
	// An internal link opens the id inside Grafana's own Explore rather than
	// sending the user to OpenObserve's web UI. The query mirrors what the
	// TraceID tab produces, so clicking a row lands on the trace view with the
	// waterfall already rendered.
	query := map[string]any{
		"refId":      "A",
		"datasource": map[string]any{"type": "grafana-openobserve-datasource", "uid": datasourceUID},
		"queryType":  "traces",
		"queryMode":  "sql",
		"format":     "auto",
		"traceMode":  "traceId",
		"table":      stream,
		// Grafana substitutes the field value of the clicked row here.
		"traceId":   "${__value.raw}",
		"enableSSE": true,
		"useCache":  true,
	}
	return []data.DataLink{{
		Title: "View trace",
		Internal: &data.InternalDataLink{
			DatasourceUID: datasourceUID,
			Query:         query,
			// Without a time range the target Explore pane keeps the current one,
			// which is what the user is already looking at.
		},
	}}
}

// buildServiceGraphFrames renders the service dependency graph in the shape
// Grafana's node graph panel consumes.
//
// This mirrors what the Jaeger plugin returns for its dependency graph: two
// frames, one with the nodes (id + title) and one with the edges (id, source,
// target, mainstat), both tagged with the nodeGraph visualisation. Grafana then
// draws the graph itself, so the panel needs no extra configuration.
func buildServiceGraphFrames(rs *openobserve.ResultSet) data.Frames {
	edges := openobserve.SummarizeServiceEdges(rs)

	// Stable node ids: the node graph matches edges to nodes by id, and using
	// the service name keeps them readable in the panel.
	seen := map[string]bool{}
	nodeIDs := make([]string, 0, len(edges)*2)
	for _, e := range edges {
		for _, svc := range []string{e.ParentService, e.ChildService} {
			if svc == "" || seen[svc] {
				continue
			}
			seen[svc] = true
			nodeIDs = append(nodeIDs, svc)
		}
	}
	sort.Strings(nodeIDs)

	edgeIDs := make([]string, 0, len(edges))
	sources := make([]string, 0, len(edges))
	targets := make([]string, 0, len(edges))
	mainstats := make([]float64, 0, len(edges))
	for _, e := range edges {
		edgeIDs = append(edgeIDs, fmt.Sprintf("%s->%s", e.ParentService, e.ChildService))
		sources = append(sources, e.ParentService)
		targets = append(targets, e.ChildService)
		// The edge label is the call count, which is what Jaeger shows.
		mainstats = append(mainstats, float64(e.Calls))
	}

	nodes := data.NewFrame("nodes",
		data.NewField("id", nil, nodeIDs),
		data.NewField("title", nil, nodeIDs),
	)
	nodes.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeNodeGraph,
	}

	edgeFrame := data.NewFrame("edges",
		data.NewField("id", nil, edgeIDs),
		data.NewField("source", nil, sources),
		data.NewField("target", nil, targets),
		data.NewField("mainstat", nil, mainstats),
	)
	edgeFrame.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeNodeGraph,
	}

	if len(edges) == 0 {
		nodes.Meta.Notices = []data.Notice{{
			Severity: data.NoticeSeverityInfo,
			Text: "No caller/callee pairs in this range. The graph is derived from spans " +
				"that carry a parent span id, so a stream of single-span traces produces no edges.",
		}}
	}
	return data.Frames{nodes, edgeFrame}
}
