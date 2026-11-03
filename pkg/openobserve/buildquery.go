package openobserve

import (
	"fmt"
	"strings"
)

// BuilderQuery is the structured description the builder editor produces.
//
// The builder exists so that choosing streams and aggregations never renders
// text into a statement the user may have written: the editor holds structured
// fields, and the SQL is derived here.
type BuilderQuery struct {
	// Stream is the stream to read; required.
	Stream string
	// Aggregation is one of the Aggregations; empty means "return raw rows".
	Aggregation string
	// Field is the column the aggregation applies to. count(*) needs none.
	Field string
	// Interval is the time bucket, e.g. "1 minute". Metrics default.
	Interval string
	// GroupBy lists extra columns to group by.
	GroupBy []string
	// Limit caps the rows returned; zero falls back to the caller's default.
	Limit int64
}

// Aggregations are the functions the builder offers.
var Aggregations = []string{"avg", "sum", "min", "max", "count", "p50", "p95", "p99"}

// DefaultInterval is the time bucket used when the caller picks none.
const DefaultInterval = "1 minute"

// IsValidAggregation reports whether name is a supported aggregation.
func IsValidAggregation(name string) bool {
	if name == "" {
		return true
	}
	for _, a := range Aggregations {
		if a == name {
			return true
		}
	}
	return false
}

// BuildBuilderSQL renders the statement a builder query describes.
//
// Returns "" when there is no stream, so the caller can report an empty query
// rather than sending malformed SQL.
func BuildBuilderSQL(q BuilderQuery, defaultLimit int64) string {
	stream := strings.TrimSpace(q.Stream)
	if stream == "" {
		return ""
	}
	table := alwaysQuote(stream)
	limit := q.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit <= 0 {
		limit = 100
	}

	if q.Aggregation == "" {
		return fmt.Sprintf(`SELECT * FROM %s LIMIT %d`, table, limit)
	}

	interval := strings.TrimSpace(q.Interval)
	if interval == "" {
		interval = DefaultInterval
	}
	aggregation := q.Aggregation
	field := strings.TrimSpace(q.Field)

	var value string
	switch {
	case aggregation == "count" && field == "":
		value = "count(*) AS value"
	case field == "":
		// A column has to be named for every other function.
		return ""
	default:
		value = fmt.Sprintf("%s(%s) AS value", aggregation, builderIdent(field))
	}

	// histogram() buckets the timestamp; grouping by it makes the frame a
	// time series without any client-side transformation.
	var sb strings.Builder
	fmt.Fprintf(&sb, `SELECT histogram(_timestamp, %s) AS ts, %s FROM %s`,
		sqlString(interval), value, table)

	groups := []string{"ts"}
	for _, g := range q.GroupBy {
		g = strings.TrimSpace(g)
		if g == "" || g == "ts" {
			continue
		}
		groups = append(groups, builderIdent(g))
	}
	fmt.Fprintf(&sb, ` GROUP BY %s ORDER BY ts LIMIT %d`, strings.Join(groups, ", "), limit)
	return sb.String()
}

// builderIdent always quotes a builder column.
//
// OpenObserve treats a dotted name as a column of the flat span/log schema, so
// `avg(http.status_code)` is read as a path and fails; `avg("http.status_code")`
// is the column. The shared quoteIdent only quotes when necessary, which is
// right for whole statements but wrong here.
func builderIdent(name string) string {
	trimmed := strings.TrimSpace(name)
	if strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`) {
		return trimmed
	}
	return `"` + strings.ReplaceAll(trimmed, `"`, `""`) + `"`
}
