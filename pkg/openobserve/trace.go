package openobserve

import (
	"fmt"
	"sort"
	"strings"
)

// TraceQueryMode is the query style of the trace editor, mirroring Jaeger's
// three modes.
type TraceQueryMode string

const (
	// TraceModeSearch lists traces matching the filters (Jaeger "Search").
	TraceModeSearch TraceQueryMode = "search"
	// TraceModeTraceID loads one trace by id (Jaeger "TraceID").
	TraceModeTraceID TraceQueryMode = "traceId"
	// TraceModeGraph aggregates service-to-service calls (Jaeger "Dependency graph").
	TraceModeGraph TraceQueryMode = "graph"
)

// IsValidTraceMode reports whether s names a known trace mode.
func IsValidTraceMode(s string) bool {
	switch TraceQueryMode(s) {
	case TraceModeSearch, TraceModeTraceID, TraceModeGraph:
		return true
	}
	return false
}

// TraceQuery is the filter set of the trace editor.
type TraceQuery struct {
	Mode TraceQueryMode

	// Stream is the OpenObserve trace stream, e.g. "default".
	Stream string

	// TraceID is used by TraceModeTraceID.
	TraceID string

	// ServiceName / OperationName narrow the spans. Empty means "all".
	ServiceName   string
	OperationName string

	// Tags are extra `key=value` predicates, e.g. ["http.status_code=200"].
	Tags []string

	// MinDurationUS / MaxDurationUS bound the span duration in microseconds.
	// Zero means "no bound".
	MinDurationUS int64
	MaxDurationUS int64

	// Services is the service-to-service edge list used by TraceModeGraph.
	// Each entry is `parent-service>child-service`, matching the shape of
	// Jaeger's dependency graph data.
	Services []string

	// Limit caps the number of traces (search) or spans (trace id).
	Limit int64
}

// alwaysQuote quotes an identifier unconditionally.
//
// quoteIdent leaves plain names bare, which is right for generated column
// references but wrong for a stream name or a dotted tag key:
// `http.status_code` would otherwise be read as `table.column`.
func alwaysQuote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// sqlString quotes a value as an SQL string literal.
func sqlString(s string) string { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }

// BuildSearchSQL lists the traces that match the filters.
//
// One row per trace: its id, the services it touches, the entry operation, the
// start time and the total span count, which is what a trace list needs.
func BuildSearchSQL(q TraceQuery, defaultLimit int64) string {
	if q.Stream == "" {
		return ""
	}
	var sb strings.Builder
	// Aliases must not reuse a source column name: OpenObserve resolves the
	// name inside the same projection and reports "unknown field".
	//
	// The entry span (the root, with no parent) is what names a trace, and
	// OpenObserve has no min_by, so it is picked with a conditional aggregate.
	sb.WriteString(`SELECT trace_id,
       min(start_time) AS first_span,
       max(end_time) - min(start_time) AS total_duration,
       count(*) AS spans,
       min(CASE WHEN reference_parent_span_id IS NULL OR reference_parent_span_id = '' THEN operation_name END) AS entry_operation,
       min(CASE WHEN reference_parent_span_id IS NULL OR reference_parent_span_id = '' THEN service_name END) AS entry_service,
       count(DISTINCT service_name) AS services`)
	sb.WriteString(` FROM `)
	sb.WriteString(alwaysQuote(q.Stream))
	writeTraceWhere(&sb, q)
	sb.WriteString(` GROUP BY trace_id ORDER BY first_span DESC`)
	sb.WriteString(fmt.Sprintf(` LIMIT %d`, traceLimit(q.Limit, defaultLimit)))
	return sb.String()
}

// BuildTraceByIDSQL selects every span of one trace in start order.
func BuildTraceByIDSQL(q TraceQuery, defaultLimit int64) string {
	if q.Stream == "" || q.TraceID == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(`SELECT * FROM `)
	sb.WriteString(alwaysQuote(q.Stream))
	sb.WriteString(` WHERE trace_id = `)
	sb.WriteString(sqlString(q.TraceID))
	sb.WriteString(` ORDER BY start_time`)
	sb.WriteString(fmt.Sprintf(` LIMIT %d`, traceLimit(q.Limit, defaultLimit)))
	return sb.String()
}

// BuildGraphSQL produces the service-to-service edge list.
//
// OpenObserve has no dedicated dependency API, so the graph is derived from the
// spans: for every span with a parent, pair the parent's service with the
// child's service and count the calls between them.
func BuildGraphSQL(q TraceQuery, defaultLimit int64) string {
	if q.Stream == "" {
		return ""
	}
	stream := alwaysQuote(q.Stream)
	var sb strings.Builder
	sb.WriteString(`SELECT parent.service_name AS parent_service,
       child.service_name AS child_service,
       count(*) AS calls,
       avg(child.duration) AS avg_duration_us
FROM `)
	sb.WriteString(stream)
	sb.WriteString(` AS child
JOIN `)
	sb.WriteString(stream)
	sb.WriteString(` AS parent ON child.reference_parent_span_id = parent.span_id`)
	writeGraphWhere(&sb, q)
	sb.WriteString(` GROUP BY parent_service, child_service ORDER BY calls DESC`)
	sb.WriteString(fmt.Sprintf(` LIMIT %d`, graphLimit(q.Limit, defaultLimit)))
	return sb.String()
}

// writeTraceWhere appends the WHERE clause shared by the search query.
func writeTraceWhere(sb *strings.Builder, q TraceQuery) {
	conds := make([]string, 0, 6)
	if s := strings.TrimSpace(q.ServiceName); s != "" && s != "All" {
		conds = append(conds, "service_name = "+sqlString(s))
	}
	if s := strings.TrimSpace(q.OperationName); s != "" && s != "All" {
		conds = append(conds, "operation_name = "+sqlString(s))
	}
	for _, tag := range q.Tags {
		if cond, ok := tagCondition(tag); ok {
			conds = append(conds, cond)
		}
	}
	if q.MinDurationUS > 0 {
		conds = append(conds, fmt.Sprintf("duration >= %d", q.MinDurationUS))
	}
	if q.MaxDurationUS > 0 {
		conds = append(conds, fmt.Sprintf("duration <= %d", q.MaxDurationUS))
	}
	writeWhere(sb, conds)
}

// writeGraphWhere filters the self-join used by the dependency graph.
func writeGraphWhere(sb *strings.Builder, q TraceQuery) {
	conds := []string{"child.reference_parent_span_id IS NOT NULL"}
	if s := strings.TrimSpace(q.ServiceName); s != "" && s != "All" {
		conds = append(conds, "child.service_name = "+sqlString(s))
	}
	writeWhere(sb, conds)
}

func writeWhere(sb *strings.Builder, conds []string) {
	if len(conds) == 0 {
		return
	}
	sb.WriteString(` WHERE `)
	sb.WriteString(strings.Join(conds, ` AND `))
}

// tagCondition converts a `key=value` tag filter into a SQL predicate.
//
// `key>` / `key<` comparisons are not supported on purpose: trace tags are
// strings, so only equality is meaningful. A bare `key` means "exists".
func tagCondition(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", false
	}
	if idx := strings.Index(tag, "="); idx > 0 {
		key := strings.TrimSpace(tag[:idx])
		value := strings.TrimSpace(tag[idx+1:])
		if key == "" {
			return "", false
		}
		if value == "" {
			return fmt.Sprintf(`COALESCE(%s, '') <> ''`, alwaysQuote(key)), true
		}
		return fmt.Sprintf(`%s = %s`, alwaysQuote(key), sqlString(value)), true
	}
	return fmt.Sprintf(`COALESCE(%s, '') <> ''`, alwaysQuote(tag)), true
}

func traceLimit(limit, def int64) int64 {
	if limit <= 0 {
		limit = def
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 10000 {
		limit = 10000
	}
	return limit
}

func graphLimit(limit, def int64) int64 {
	if limit <= 0 {
		limit = def
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 10000 {
		limit = 10000
	}
	return limit
}

// TraceSummary is one row of the trace list.
type TraceSummary struct {
	TraceID       string
	ServiceName   string
	OperationName string
	StartTimeUS   int64
	DurationUS    int64
	SpanCount     int64
	ServiceCount  int64
}

// ServiceEdge is one edge of the dependency graph.
type ServiceEdge struct {
	ParentService string
	ChildService  string
	Calls         int64
	AvgDurationUS float64
}

// SummarizeTraces extracts the trace list from a result set.
func SummarizeTraces(rs *ResultSet) []TraceSummary {
	out := make([]TraceSummary, 0, len(rs.Rows))
	for _, row := range rs.Rows {
		summary := TraceSummary{
			TraceID:       ToString(rowValue(rs, row, "trace_id")),
			ServiceName:   ToString(rowValue(rs, row, "entry_service")),
			OperationName: ToString(rowValue(rs, row, "entry_operation")),
			StartTimeUS:   toInt64(rowValue(rs, row, "first_span")),
			DurationUS:    toInt64(rowValue(rs, row, "total_duration")),
			SpanCount:     toInt64(rowValue(rs, row, "spans")),
			ServiceCount:  toInt64(rowValue(rs, row, "services")),
		}
		if summary.TraceID != "" {
			out = append(out, summary)
		}
	}
	return out
}

// SummarizeServiceEdges extracts the dependency graph edges.
func SummarizeServiceEdges(rs *ResultSet) []ServiceEdge {
	out := make([]ServiceEdge, 0, len(rs.Rows))
	for _, row := range rs.Rows {
		edge := ServiceEdge{
			ParentService: ToString(rowValue(rs, row, "parent_service")),
			ChildService:  ToString(rowValue(rs, row, "child_service")),
			Calls:         toInt64(rowValue(rs, row, "calls")),
			AvgDurationUS: toFloat64Or(rowValue(rs, row, "avg_duration_us"), 0),
		}
		if edge.ParentService != "" && edge.ChildService != "" {
			out = append(out, edge)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Calls > out[j].Calls })
	return out
}

// toFloat64Or coerces a cell to float64, falling back when it cannot.
func toFloat64Or(v any, fallback float64) float64 {
	if f, ok := toFloat64(v); ok {
		return f
	}
	return fallback
}
