package openobserve

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// Format selects the Grafana visualisation the frames are shaped for.
type Format string

const (
	// FormatAuto infers the shape from the SQL: aggregates become a table,
	// raw selections become logs/table rows.
	FormatAuto Format = "auto"
	// FormatLogs renders a log-lines frame (time + body + labels + attributes).
	FormatLogs Format = "logs"
	// FormatTable renders a plain table frame.
	FormatTable Format = "table"
	// FormatTimeSeries renders wide time series frames, one per series group.
	FormatTimeSeries Format = "timeseries"
	// FormatTrace renders a Grafana trace frame so Explore shows the trace view.
	FormatTrace Format = "trace"
)

// FrameOptions controls how a result set is turned into Grafana frames.
type FrameOptions struct {
	Format   Format
	TimeZone *time.Location

	// TimeField / MessageField / LevelField override the auto-detected
	// columns. Empty means auto-detect.
	TimeField    string
	MessageField string
	LevelField   string

	// TraceIDField is used by the trace format to pick the trace to show;
	// empty means "show the first trace id found".
	TraceIDField string

	// ExecutedQuery is copied into meta.executedQueryString.
	ExecutedQuery string
}

// FrameBuilder converts OpenObserve result sets into Grafana data frames.
type FrameBuilder struct{}

// NewFrameBuilder returns a FrameBuilder.
func NewFrameBuilder() *FrameBuilder { return &FrameBuilder{} }

// BuildFrames converts one search response into frames according to the
// requested format.
func (fb *FrameBuilder) BuildFrames(rs *ResultSet, opts FrameOptions) (data.Frames, error) {
	if opts.TimeZone == nil {
		opts.TimeZone = time.UTC
	}
	if rs.IsEmpty() {
		return data.Frames{}, nil
	}
	format := opts.Format
	if format == "" {
		format = FormatAuto
	}
	if format == FormatAuto {
		format = inferFormat(rs)
	}
	switch format {
	case FormatLogs:
		return data.Frames{fb.buildLogsFrame(rs, opts)}, nil
	case FormatTimeSeries:
		frames := fb.buildTimeSeriesFrames(rs, opts)
		if len(frames) == 0 {
			// Nothing numeric to plot: fall back to a table so the user sees
			// the data instead of an empty panel.
			return data.Frames{fb.buildTableFrame(rs, opts)}, nil
		}
		return frames, nil
	case FormatTrace:
		frame, err := fb.buildTraceFrame(rs, opts)
		if err != nil {
			return data.Frames{fb.buildTableFrame(rs, opts)}, nil
		}
		return data.Frames{frame}, nil
	default:
		return data.Frames{fb.buildTableFrame(rs, opts)}, nil
	}
}

// inferFormat picks a sensible default from the result shape.
func inferFormat(rs *ResultSet) Format {
	timeCol := rs.DetectTimeColumn("")
	if timeCol != "" && len(rs.NumericColumns(timeCol)) > 0 {
		// Time + numbers is almost always a graph in Grafana.
		return FormatTimeSeries
	}
	return FormatTable
}

// buildTableFrame renders every column with its natural type.
func (fb *FrameBuilder) buildTableFrame(rs *ResultSet, opts FrameOptions) *data.Frame {
	timeCol := rs.DetectTimeColumn(opts.TimeField)
	frame := data.NewFrame("openobserve")
	frame.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeTable,
		ExecutedQueryString:    opts.ExecutedQuery,
	}

	for i, name := range rs.Columns {
		forcedTime := name == timeCol && timeCol != ""
		ft := rs.classifyColumn(i, forcedTime)
		values := make([]any, len(rs.Rows))
		for r, row := range rs.Rows {
			if i < len(row) {
				values[r] = row[i]
			}
		}
		frame.Fields = append(frame.Fields, newFieldFromValues(name, ft, values, opts.TimeZone))
	}
	return frame
}

// buildLogsFrame renders a logs frame: time, body, labels plus every original
// column as an extra attribute so Explore's log details show them all.
func (fb *FrameBuilder) buildLogsFrame(rs *ResultSet, opts FrameOptions) *data.Frame {
	timeCol := rs.DetectTimeColumn(opts.TimeField)
	if timeCol == "" {
		// Without a timestamp Grafana cannot render logs; degrade to a table.
		return fb.buildTableFrame(rs, opts)
	}
	bodyCol := rs.DetectBodyColumn(opts.MessageField)
	levelCol := rs.DetectLevelColumn(opts.LevelField)

	frame := data.NewFrame("logs")
	frame.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeLogs,
		ExecutedQueryString:    opts.ExecutedQuery,
	}

	timeValues := make([]*time.Time, 0, len(rs.Rows))
	bodyValues := make([]string, 0, len(rs.Rows))
	labelValues := make([]json.RawMessage, 0, len(rs.Rows))
	levelValues := make([]string, 0, len(rs.Rows))

	timeIdx := rs.ColumnIndex(timeCol)
	bodyIdx := rs.ColumnIndex(bodyCol)
	levelIdx := rs.ColumnIndex(levelCol)

	// Columns rendered as structured labels: everything that is not the time,
	// body or level column and is not a known low-cardinality scalar.
	labelColumns := make([]int, 0, len(rs.Columns))
	for i := range rs.Columns {
		if i == timeIdx || i == bodyIdx || i == levelIdx {
			continue
		}
		labelColumns = append(labelColumns, i)
	}

	for _, row := range rs.Rows {
		var ts time.Time
		if timeIdx >= 0 && timeIdx < len(row) {
			ts, _ = toTime(row[timeIdx])
		}
		timeValues = append(timeValues, &ts)

		body := ""
		if bodyIdx >= 0 && bodyIdx < len(row) {
			body = toString(row[bodyIdx])
		}
		if body == "" {
			body = buildBodyFromRow(rs, row, bodyIdx)
		}
		bodyValues = append(bodyValues, body)

		labelValues = append(labelValues, buildLabels(rs, row, labelColumns, timeIdx, bodyIdx))

		if levelIdx >= 0 && levelIdx < len(row) {
			levelValues = append(levelValues, toString(row[levelIdx]))
		}
	}

	frame.Fields = append(frame.Fields, data.NewField("time", nil, timeValues))
	frame.Fields = append(frame.Fields, data.NewField("body", nil, bodyValues))
	if levelIdx >= 0 {
		frame.Fields = append(frame.Fields, data.NewField("level", nil, levelValues))
	}
	frame.Fields = append(frame.Fields, data.NewField("labels", nil, labelValues))

	// Extra attributes stay addressable in the log details pane.
	for i, name := range rs.Columns {
		if i == timeIdx || i == bodyIdx || i == levelIdx {
			continue
		}
		values := make([]string, 0, len(rs.Rows))
		for _, row := range rs.Rows {
			if i < len(row) {
				values = append(values, toString(row[i]))
				continue
			}
			values = append(values, "")
		}
		field := data.NewField(name, nil, values)
		field.Config = &data.FieldConfig{Custom: map[string]any{"hidden": false}}
		frame.Fields = append(frame.Fields, field)
	}
	return frame
}

// buildBodyFromRow renders the whole row as JSON, used when the stream has no
// dedicated message column.
func buildBodyFromRow(rs *ResultSet, row []any, skip int) string {
	m := make(map[string]any, len(rs.Columns))
	for i, name := range rs.Columns {
		if i == skip || i >= len(row) {
			continue
		}
		m[name] = row[i]
	}
	b, err := jsonAPI.Marshal(m)
	if err != nil {
		return fmt.Sprint(m)
	}
	return string(b)
}

// buildLabels collects the non-time, non-body columns of one row as JSON so
// Explore can display and filter by them.
func buildLabels(rs *ResultSet, row []any, indexes []int, timeIdx, bodyIdx int) json.RawMessage {
	m := make(map[string]any, len(indexes))
	for _, i := range indexes {
		if i == timeIdx || i == bodyIdx || i >= len(row) || row[i] == nil {
			continue
		}
		m[rs.Columns[i]] = row[i]
	}
	if len(m) == 0 {
		return json.RawMessage("{}")
	}
	b, err := jsonAPI.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// buildTimeSeriesFrames turns [time, labels..., values...] rows into one wide
// frame per series group, which is what Grafana's time series panel expects.
func (fb *FrameBuilder) buildTimeSeriesFrames(rs *ResultSet, opts FrameOptions) data.Frames {
	timeCol := rs.DetectTimeColumn(opts.TimeField)
	if timeCol == "" {
		return nil
	}
	timeIdx := rs.ColumnIndex(timeCol)
	valueCols := rs.NumericColumns(timeCol)
	if len(valueCols) == 0 {
		return nil
	}
	labelCols := rs.LabelColumns(timeCol)

	type series struct {
		labels map[string]string
		times  []*time.Time
		values map[string][]*float64
	}
	order := make([]string, 0, 8)
	byKey := make(map[string]*series, 8)

	for _, row := range rs.Rows {
		labels := make(map[string]string, len(labelCols))
		keyParts := make([]string, 0, len(labelCols))
		for _, name := range labelCols {
			idx := rs.ColumnIndex(name)
			v := ""
			if idx >= 0 && idx < len(row) {
				v = toString(row[idx])
			}
			labels[name] = v
			keyParts = append(keyParts, name+"="+v)
		}
		key := strings.Join(keyParts, "\x00")
		s, ok := byKey[key]
		if !ok {
			s = &series{
				labels: labels,
				values: make(map[string][]*float64, len(valueCols)),
			}
			for _, vc := range valueCols {
				s.values[vc] = make([]*float64, 0, len(rs.Rows))
			}
			byKey[key] = s
			order = append(order, key)
		}
		var ts time.Time
		if timeIdx >= 0 && timeIdx < len(row) {
			ts, _ = toTime(row[timeIdx])
		}
		s.times = append(s.times, &ts)
		for _, vc := range valueCols {
			idx := rs.ColumnIndex(vc)
			var ptr *float64
			if idx >= 0 && idx < len(row) {
				if f, ok := toFloat64(row[idx]); ok {
					value := f
					ptr = &value
				}
			}
			s.values[vc] = append(s.values[vc], ptr)
		}
	}

	frames := make(data.Frames, 0, len(order))
	for _, key := range order {
		s := byKey[key]
		frame := data.NewFrame(seriesFrameName(s.labels))
		frame.Meta = &data.FrameMeta{
			Type:                   data.FrameTypeTimeSeriesWide,
			TypeVersion:            data.FrameTypeVersion{0, 1},
			PreferredVisualization: data.VisTypeGraph,
			ExecutedQueryString:    opts.ExecutedQuery,
		}
		frame.Fields = append(frame.Fields, data.NewField("time", nil, s.times))
		for _, vc := range valueCols {
			field := data.NewField(vc, labelsToDataLabels(s.labels), s.values[vc])
			frame.Fields = append(frame.Fields, field)
		}
		frames = append(frames, frame)
	}
	return frames
}

func seriesFrameName(labels map[string]string) string {
	if len(labels) == 0 {
		return "series"
	}
	keys := sortedKeys(labels)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if labels[k] == "" {
			continue
		}
		parts = append(parts, k+"="+labels[k])
	}
	if len(parts) == 0 {
		return "series"
	}
	return strings.Join(parts, " ")
}

func labelsToDataLabels(labels map[string]string) data.Labels {
	if len(labels) == 0 {
		return nil
	}
	out := make(data.Labels, len(labels))
	for k, v := range labels {
		if v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildTraceFrame renders spans as a Grafana trace frame, so Explore shows the
// native trace view with the waterfall.
func (fb *FrameBuilder) buildTraceFrame(rs *ResultSet, opts FrameOptions) (*data.Frame, error) {
	get := func(row []any, name string) any {
		idx := rs.ColumnIndex(name)
		if idx < 0 || idx >= len(row) {
			return nil
		}
		return row[idx]
	}
	str := func(row []any, name string) string { return toString(get(row, name)) }
	num := func(row []any, name string) int64 { return toInt64(get(row, name)) }

	rows := rs.Rows
	if len(rows) == 0 {
		return nil, fmt.Errorf("no spans returned")
	}

	// Pick the trace to display: explicit field value if configured, else all
	// rows of the first trace id we see.
	targetTrace := ""
	if opts.TraceIDField != "" {
		targetTrace = opts.TraceIDField
	} else {
		for _, row := range rows {
			if id := str(row, "trace_id"); id != "" {
				targetTrace = id
				break
			}
		}
	}

	selected := make([][]any, 0, len(rows))
	for _, row := range rows {
		if targetTrace != "" && str(row, "trace_id") != targetTrace {
			continue
		}
		selected = append(selected, row)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no spans for trace %q", targetTrace)
	}
	if len(selected) > 50000 {
		selected = selected[:50000]
	}

	// Grafana expects milliseconds for startTime/duration; OpenObserve returns
	// start/end in nanoseconds and duration in microseconds.
	minStart := int64(0)
	for i, row := range selected {
		start := num(row, "start_time")
		if i == 0 || start < minStart {
			minStart = start
		}
	}

	var traceID, serviceName, operationName []string
	var spanID, parentSpanID []string
	var startTime, duration []float64
	var tags, serviceTags []json.RawMessage
	var statusCode []int64
	var statusMessage, spanKind []string

	for _, row := range selected {
		start := num(row, "start_time")
		end := num(row, "end_time")
		durUS := num(row, "duration")
		if durUS == 0 && end > start {
			durUS = (end - start) / 1000
		}
		span := str(row, "span_id")
		parent := str(row, "reference_parent_span_id")
		if parent == "" {
			parent = str(row, "parent_span_id")
		}
		svc := str(row, "service_name")
		op := str(row, "operation_name")

		traceID = append(traceID, targetTrace)
		spanID = append(spanID, span)
		parentSpanID = append(parentSpanID, parent)
		startTime = append(startTime, float64(start)/1e6)
		duration = append(duration, float64(durUS)/1000)
		serviceName = append(serviceName, svc)
		operationName = append(operationName, op)
		serviceTags = append(serviceTags, spanAttributesJSON(row, rs, true))
		tags = append(tags, spanAttributesJSON(row, rs, false))
		code := num(row, "status_code")
		statusCode = append(statusCode, code)
		statusMessage = append(statusMessage, str(row, "status_message"))
		spanKind = append(spanKind, str(row, "span_kind"))
	}

	frame := data.NewFrame("traces")
	frame.Meta = &data.FrameMeta{
		PreferredVisualization: data.VisTypeTrace,
		ExecutedQueryString:    opts.ExecutedQuery,
	}
	frame.Fields = append(frame.Fields,
		data.NewField("traceID", nil, traceID),
		data.NewField("spanID", nil, spanID),
		data.NewField("parentSpanID", nil, parentSpanID),
		data.NewField("operationName", nil, operationName),
		data.NewField("serviceName", nil, serviceName),
		data.NewField("serviceTags", nil, serviceTags),
		data.NewField("startTime", nil, startTime),
		data.NewField("duration", nil, duration),
		data.NewField("tags", nil, tags),
		data.NewField("statusCode", nil, statusCode),
		data.NewField("statusMessage", nil, statusMessage),
		data.NewField("spanKind", nil, spanKind),
	)
	return frame, nil
}

// spanAttributesJSON encodes a span row as the key/value tag array Grafana's
// trace view renders.
func spanAttributesJSON(row []any, rs *ResultSet, serviceOnly bool) json.RawMessage {
	type kv struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	out := make([]kv, 0, len(rs.Columns))
	for i, name := range rs.Columns {
		if i >= len(row) || row[i] == nil {
			continue
		}
		if serviceOnly && !strings.HasPrefix(name, "service_") && name != "host_name" {
			continue
		}
		if !serviceOnly && (name == "trace_id" || name == "span_id" || name == "reference_parent_span_id") {
			continue
		}
		out = append(out, kv{Key: name, Value: row[i]})
	}
	b, err := jsonAPI.Marshal(out)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// newFieldFromValues builds a typed field, converting values as needed.
func newFieldFromValues(name string, ft FieldType, values []any, loc *time.Location) *data.Field {
	switch ft {
	case FieldTypeTime:
		converted := make([]*time.Time, len(values))
		for i, v := range values {
			if t, ok := toTime(v); ok {
				t = t.In(loc)
				converted[i] = &t
			}
		}
		return data.NewField(name, nil, converted)
	case FieldTypeNumber:
		converted := make([]*float64, len(values))
		for i, v := range values {
			if v == nil {
				continue
			}
			if f, ok := toFloat64(v); ok {
				converted[i] = &f
			}
		}
		return data.NewField(name, nil, converted)
	case FieldTypeBool:
		converted := make([]*bool, len(values))
		for i, v := range values {
			if b, ok := v.(bool); ok {
				converted[i] = &b
			}
		}
		return data.NewField(name, nil, converted)
	case FieldTypeJSON:
		converted := make([]json.RawMessage, len(values))
		for i, v := range values {
			converted[i] = toJSONValue(v)
		}
		return data.NewField(name, nil, converted)
	default:
		converted := make([]*string, len(values))
		for i, v := range values {
			if v == nil {
				continue
			}
			s := toString(v)
			converted[i] = &s
		}
		return data.NewField(name, nil, converted)
	}
}

// SortResultSetByTime orders rows ascending by the detected time column, which
// time series frames require.
func SortResultSetByTime(rs *ResultSet, timeCol string) {
	idx := rs.ColumnIndex(timeCol)
	if idx < 0 {
		return
	}
	sort.SliceStable(rs.Rows, func(i, j int) bool {
		ti, _ := toTime(rs.Rows[i][idx])
		tj, _ := toTime(rs.Rows[j][idx])
		return ti.Before(tj)
	})
}
