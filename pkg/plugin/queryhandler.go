package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/models"
	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// QueryModel is the JSON query sent by the Grafana query editor.
type QueryModel struct {
	// QueryType is the OpenObserve stream type: logs, metrics or traces.
	QueryType string `json:"queryType"`
	// EditorMode is "code" (raw SQL) or "builder".
	EditorMode string `json:"editorMode"`
	// RawSql is the SQL statement.
	RawSql string `json:"rawSql"`
	// RawQuery marks variable queries.
	RawQuery bool `json:"rawQuery"`
	// Format selects the frame shape: auto, logs, table, timeseries, trace.
	Format string `json:"format"`
	// QueryMode selects "sql" or "promql".
	QueryMode string `json:"queryMode"`
	// Expr is the PromQL expression used when QueryMode is promql.
	Expr string `json:"expr"`

	SearchType string `json:"searchType"`
	UseCache   *bool  `json:"useCache"`
	EnableSSE  *bool  `json:"enableSSE"`

	From int64 `json:"from"`
	Size int64 `json:"size"`

	// TimeField / LogMessageField / LogLevelField override the datasource
	// column defaults for this query.
	TimeField       string `json:"timeField"`
	LogMessageField string `json:"logMessageField"`
	LogLevelField   string `json:"logLevelField"`

	// TraceID restricts the trace view to one trace.
	TraceID string `json:"traceId"`

	AdHocFilters []AdHocVariableFilter `json:"adhocFilters"`
}

// AdHocVariableFilter is one Grafana ad-hoc variable filter.
type AdHocVariableFilter struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Operator string `json:"operator"`
}

const (
	queryModeSQL    = "sql"
	queryModePromQL = "promql"

	formatAuto        = "auto"
	formatLogs        = "logs"
	formatTable       = "table"
	formatTimeSeries  = "timeseries"
	formatTrace       = "trace"
	formatAnnotations = "annotations"
)

// queryContext bundles everything one query needs.
type queryContext struct {
	model    QueryModel
	query    backend.DataQuery
	org      string
	settings *models.JsonData
}

// parseQueryModel decodes and normalises a data query.
func parseQueryModel(q backend.DataQuery) (QueryModel, error) {
	var model QueryModel
	if len(q.JSON) > 0 {
		if err := json.Unmarshal(q.JSON, &model); err != nil {
			return model, fmt.Errorf("invalid query: %w", err)
		}
	}
	model.RawSql = strings.TrimSpace(model.RawSql)
	model.Expr = strings.TrimSpace(model.Expr)
	if model.QueryType == "" {
		model.QueryType = openobserve.LogsStream
	}
	if !openobserve.IsValidStreamType(model.QueryType) {
		return model, fmt.Errorf("invalid stream type %q, expected one of logs, metrics or traces", model.QueryType)
	}
	if model.QueryMode == "" {
		if model.Expr != "" && model.RawSql == "" {
			model.QueryMode = queryModePromQL
		} else {
			model.QueryMode = queryModeSQL
		}
	}
	if model.Format == "" {
		model.Format = formatAuto
	}
	return model, nil
}

// resolveFormat maps the plugin format onto the frame builder format.
func resolveFormat(model QueryModel, parsed *openobserve.ParsedQuery) openobserve.Format {
	switch model.Format {
	case formatLogs:
		return openobserve.FormatLogs
	case formatTable:
		return openobserve.FormatTable
	case formatTimeSeries:
		return openobserve.FormatTimeSeries
	case formatTrace:
		return openobserve.FormatTrace
	case formatAnnotations:
		return openobserve.FormatTable
	}
	// auto
	if model.QueryType == openobserve.TraceStream {
		return openobserve.FormatTrace
	}
	if parsed != nil && parsed.Mode == openobserve.SelectAll && model.QueryType == openobserve.LogsStream {
		return openobserve.FormatLogs
	}
	return openobserve.FormatAuto
}

// effectiveSize resolves the row limit with the datasource caps applied.
func effectiveSize(model QueryModel, parsed *openobserve.ParsedQuery, settings *models.JsonData) int64 {
	size := int64(0)
	switch {
	case model.Size > 0:
		size = model.Size
	case parsed != nil && parsed.Limit > 0:
		size = parsed.Limit
	default:
		size = settings.DefaultLimit
	}
	if size <= 0 {
		size = models.DefaultLimit
	}
	if size > settings.MaxLimit {
		log.DefaultLogger.Warn("requested row limit capped", "requested", size, "max", settings.MaxLimit)
		size = settings.MaxLimit
	}
	return size
}

// toAdHocFilters converts the query model filters.
func toAdHocFilters(filters []AdHocVariableFilter) []openobserve.WhereFilter {
	out := make([]openobserve.WhereFilter, 0, len(filters))
	for _, f := range filters {
		if strings.TrimSpace(f.Key) == "" {
			continue
		}
		out = append(out, openobserve.WhereFilter{
			Key:       f.Key,
			Value:     f.Value,
			Operation: f.Operator,
		})
	}
	return out
}

// frameOptions assemble the frame builder options.
func frameOptions(model QueryModel, settings *models.JsonData, loc *time.Location, format openobserve.Format) openobserve.FrameOptions {
	timeField := firstNonEmpty(model.TimeField, settings.TimeField)
	msgField := firstNonEmpty(model.LogMessageField, settings.LogMessageField)
	levelField := firstNonEmpty(model.LogLevelField, settings.LogLevelField)
	return openobserve.FrameOptions{
		Format:       format,
		TimeZone:     loc,
		TimeField:    timeField,
		MessageField: msgField,
		LevelField:   levelField,
		TraceIDField: model.TraceID,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// runSQLQuery executes one SQL query and shapes the response.
func (d *Datasource) runSQLQuery(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	model, err := parseQueryModel(q)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, err.Error())
	}
	if model.RawSql == "" {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, "the query is empty: write a SELECT statement such as SELECT * FROM \"my_stream\" LIMIT 100")
	}

	settings := d.settings.JsonData

	// Ad-hoc filters are injected before parsing so the LIMIT/stream analysis
	// sees the final statement.
	finalSQL := model.RawSql
	if !strings.HasPrefix(strings.ToLower(finalSQL), "\\dt") {
		finalSQL, err = d.queryBuilder.ApplyAdHocFilters(model.RawSql, toAdHocFilters(model.AdHocFilters))
		if err != nil {
			return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, fmt.Sprintf("cannot apply ad-hoc filters: %v", err))
		}
	}

	parsed, err := d.queryBuilder.Parse(finalSQL)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, err.Error())
	}

	size := effectiveSize(model, parsed, settings)
	from := model.From
	if from < 0 {
		from = 0
	}

	sse := true
	if model.EnableSSE != nil {
		sse = *model.EnableSSE
	}
	useCache := true
	if model.UseCache != nil {
		useCache = *model.UseCache
	}
	searchType := model.SearchType
	if searchType == "" {
		searchType = openobserve.SearchTypeUI
	}

	// The SQL stream is scoped by the query time range unless the statement
	// aggregates, in which case OpenObserve still applies start/end.
	start := q.TimeRange.From
	end := q.TimeRange.To
	if end.IsZero() || end.Before(start) {
		end = time.Now()
	}

	ctx, cancel := context.WithTimeout(ctx, settings.Timeout())
	defer cancel()

	param := &openobserve.SearchRequestParam{
		Organization: d.client.Organization(),
		StreamType:   model.QueryType,
		SearchType:   searchType,
		UseCache:     useCache,
		EnableSSE:    sse,
	}
	body := &openobserve.SearchRequestBody{
		Query: openobserve.Query{
			Sql:       stripTrailingSemicolon(finalSQL),
			StartTime: start.UnixMicro(),
			EndTime:   end.UnixMicro(),
			From:      from,
			Size:      size,
		},
		SearchType: searchType,
		Timeout:    int(settings.Timeout() / time.Second),
	}

	resp, err := d.client.Search(ctx, param, body)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusInternal, backend.ErrorSourceDownstream, err.Error())
	}
	// An empty result is the normal answer for "nothing matched", but it is also
	// what a stream type mismatch looks like: querying a log stream as "metrics"
	// silently returns no rows because OpenObserve searches a different data
	// directory per type. Say so instead of showing an empty panel.
	if len(resp.Hits) == 0 && parsed != nil && parsed.Table != "" {
		if hint := d.streamTypeHint(ctx, model.QueryType, parsed.Table); hint != "" {
			return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, hint)
		}
	}

	rs := d.buildResultSet(ctx, model.QueryType, parsed, resp)
	format := resolveFormat(model, parsed)

	frames, err := d.builder.BuildFrames(rs, frameOptions(model, settings, q.TimeRange.From.Location(), format))
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusInternal, backend.ErrorSourceDownstream, fmt.Sprintf("cannot build response: %v", err))
	}
	if model.Format == formatAnnotations {
		frames = buildAnnotationFrames(rs, q.TimeRange.From.Location())
	}
	for _, frame := range frames {
		if frame.Meta == nil {
			frame.Meta = &data.FrameMeta{}
		}
		if frame.Meta.ExecutedQueryString == "" {
			frame.Meta.ExecutedQueryString = body.Sql
		}
	}
	return backend.DataResponse{Frames: frames}
}

// streamTypeHint explains an empty result that is caused by querying a stream
// through the wrong stream type. It returns "" when the stream really is of the
// requested type (or when the type cannot be determined).
func (d *Datasource) streamTypeHint(ctx context.Context, streamType, stream string) string {
	if streamType == "" || stream == "" {
		return ""
	}
	org := d.client.Organization()
	if has, err := d.streamExists(ctx, org, streamType, stream); err != nil || has {
		return ""
	}
	for _, other := range openobserve.StreamTypes {
		if other == streamType {
			continue
		}
		if has, err := d.streamExists(ctx, org, other, stream); err == nil && has {
			return fmt.Sprintf(
				"no data: %q is a %s stream, but the query is set to %s. Switch the stream type to %q, or pick another stream.",
				stream, other, streamType, other)
		}
	}
	return ""
}

// streamExists reports whether stream is known under the given stream type.
func (d *Datasource) streamExists(ctx context.Context, org, streamType, stream string) (bool, error) {
	resp, err := d.client.ListStreams(ctx, org, streamType)
	if err != nil {
		return false, err
	}
	for _, s := range resp.List {
		if s.Name == stream {
			return true, nil
		}
	}
	return false, nil
}

// buildResultSet augments the raw hits with the stream schema when the query
// touches a single known stream, which improves column typing.
func (d *Datasource) buildResultSet(ctx context.Context, streamType string, parsed *openobserve.ParsedQuery, resp *openobserve.SearchResponse) *openobserve.ResultSet {
	var schema []openobserve.Schema
	if parsed != nil && parsed.Table != "" {
		cached, err := d.client.StreamSchema(ctx, "", parsed.Table, streamType)
		if err != nil {
			log.DefaultLogger.Debug("cannot load stream schema, falling back to value inference",
				"stream", parsed.Table, "error", err)
		} else {
			schema = cached
		}
	}
	return openobserve.NewResultSet(resp.Hits, schema)
}

func stripTrailingSemicolon(sql string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sql), ";"))
}

// buildAnnotationFrames converts a result set into Grafana annotations.
func buildAnnotationFrames(rs *openobserve.ResultSet, loc *time.Location) data.Frames {
	if loc == nil {
		loc = time.UTC
	}
	timeCol := rs.DetectTimeColumn("")
	frame := data.NewFrame("annotations")
	frame.Meta = &data.FrameMeta{}

	timeIdx := rs.ColumnIndex(timeCol)
	titleIdx := rs.ColumnIndex(rs.DetectBodyColumn(""))

	times := make([]*time.Time, 0, len(rs.Rows))
	titles := make([]string, 0, len(rs.Rows))
	texts := make([]string, 0, len(rs.Rows))
	tags := make([]string, 0, len(rs.Rows))

	for _, row := range rs.Rows {
		var ts time.Time
		if timeIdx >= 0 && timeIdx < len(row) {
			ts, _ = openobserve.ToTime(row[timeIdx])
		}
		times = append(times, &ts)
		title := ""
		text := ""
		if titleIdx >= 0 && titleIdx < len(row) {
			title = openobserve.ToString(row[titleIdx])
			text = title
		}
		titles = append(titles, title)
		texts = append(texts, text)
		tags = append(tags, "")
	}
	frame.Fields = append(frame.Fields,
		data.NewField("time", nil, times),
		data.NewField("title", nil, titles),
		data.NewField("text", nil, texts),
		data.NewField("tags", nil, tags),
	)
	return data.Frames{frame}
}
