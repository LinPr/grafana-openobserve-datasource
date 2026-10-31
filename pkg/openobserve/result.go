package openobserve

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FieldType is the logical type of a result column, derived from the
// OpenObserve schema plus the actual returned values.
type FieldType int

const (
	FieldTypeString FieldType = iota
	FieldTypeNumber
	FieldTypeBool
	FieldTypeTime
	FieldTypeJSON
)

// Column is one typed column of a result set.
type Column struct {
	Name string
	Type FieldType
	// Values holds one Go value per row: nil, string, float64, bool,
	// time.Time or json.RawMessage.
	Values []any
	// Labels are the series labels when the column belongs to a wide time
	// series frame.
	Labels map[string]string
}

// ResultSet is a column-oriented view of one search response.
type ResultSet struct {
	Columns []string
	Rows    [][]any
	// Schema is the OpenObserve column type (Utf8, Int64, Float64, ...) keyed
	// by column name. It may be empty when the query did not need it.
	Schema map[string]string
}

// NewResultSet builds a result set from raw hits, preserving the column order
// OpenObserve returned (JSON object order of the first hit) and appending any
// extra columns found in later hits.
func NewResultSet(hits []map[string]any, schema []Schema) *ResultSet {
	rs := &ResultSet{
		Rows:   make([][]any, 0, len(hits)),
		Schema: make(map[string]string, len(schema)),
	}
	for _, s := range schema {
		rs.Schema[s.Name] = s.Type
	}

	seen := make(map[string]bool, 16)
	for _, hit := range hits {
		for name := range hit {
			if !seen[name] {
				seen[name] = true
				rs.Columns = append(rs.Columns, name)
			}
		}
	}
	// Prefer the schema order for the columns it knows about: it matches the
	// stream definition and gives stable column ordering across queries.
	if len(schema) > 0 {
		ordered := make([]string, 0, len(rs.Columns))
		for _, s := range schema {
			if seen[s.Name] {
				ordered = append(ordered, s.Name)
			}
		}
		if len(ordered) > 0 {
			rest := make([]string, 0, len(rs.Columns)-len(ordered))
			for _, name := range rs.Columns {
				if _, ok := rs.Schema[name]; !ok {
					rest = append(rest, name)
				}
			}
			rs.Columns = append(ordered, rest...)
		}
	}

	for _, hit := range hits {
		row := make([]any, len(rs.Columns))
		for i, name := range rs.Columns {
			row[i] = hit[name]
		}
		rs.Rows = append(rs.Rows, row)
	}
	return rs
}

// IsEmpty reports whether the result set has no columns.
func (rs *ResultSet) IsEmpty() bool { return rs == nil || len(rs.Columns) == 0 }

// ColumnIndex returns the position of name, or -1.
func (rs *ResultSet) ColumnIndex(name string) int {
	for i, c := range rs.Columns {
		if c == name {
			return i
		}
	}
	return -1
}

// columnTimeAliases are column names treated as the time axis.
var columnTimeAliases = []string{
	"_timestamp", "timestamp", "ts", "_ts", "time", "_time", "datetime",
	"date", "event_time", "start_time", "starttime",
}

// columnBodyAliases are column names treated as the log line.
var columnBodyAliases = []string{
	"body", "message", "msg", "log", "content", "log_line", "text", "_msg",
}

// columnLevelAliases are column names treated as the log level.
var columnLevelAliases = []string{
	"level", "severity", "log_level", "severity_text", "loglevel", "_level",
	"levelname", "detected_level",
}

func firstExisting(columns []string, aliases []string) string {
	lower := make(map[string]string, len(columns))
	for _, c := range columns {
		lower[strings.ToLower(c)] = c
	}
	for _, alias := range aliases {
		if name, ok := lower[alias]; ok {
			return name
		}
	}
	return ""
}

// DetectTimeColumn picks the time column: the explicit override first, then
// the well-known aliases, then the first column whose values parse as times.
func (rs *ResultSet) DetectTimeColumn(override string) string {
	if override != "" && rs.ColumnIndex(override) >= 0 {
		return override
	}
	if name := firstExisting(rs.Columns, columnTimeAliases); name != "" {
		if idx := rs.ColumnIndex(name); idx >= 0 && columnLooksLikeTime(rs, idx) {
			return name
		}
	}
	for i := range rs.Columns {
		if columnLooksLikeTime(rs, i) {
			return rs.Columns[i]
		}
	}
	return ""
}

// DetectBodyColumn picks the log message column.
func (rs *ResultSet) DetectBodyColumn(override string) string {
	if override != "" && rs.ColumnIndex(override) >= 0 {
		return override
	}
	return firstExisting(rs.Columns, columnBodyAliases)
}

// DetectLevelColumn picks the log level column.
func (rs *ResultSet) DetectLevelColumn(override string) string {
	if override != "" && rs.ColumnIndex(override) >= 0 {
		return override
	}
	return firstExisting(rs.Columns, columnLevelAliases)
}

// columnLooksLikeTime decides whether a column carries timestamps.
func columnLooksLikeTime(rs *ResultSet, idx int) bool {
	name := strings.ToLower(rs.Columns[idx])
	hint := strings.ToLower(rs.Schema[rs.Columns[idx]])
	if strings.Contains(hint, "time") || strings.Contains(hint, "date") || strings.Contains(hint, "timestamp") {
		return true
	}
	// A well-known time column name wins over a numeric schema type: OpenObserve
	// stores _timestamp as Int64 microseconds.
	namedTime := false
	for _, alias := range columnTimeAliases {
		if name == alias {
			namedTime = true
			break
		}
	}
	if namedTime {
		return true
	}
	if hint != "" && !strings.Contains(hint, "utf8") && !strings.Contains(hint, "string") {
		// Non-string, non-time types are not the time axis.
		return false
	}

	sampled := 0
	for _, row := range rs.Rows {
		if idx >= len(row) {
			break
		}
		v := row[idx]
		if v == nil {
			continue
		}
		sampled++
		if sampled > 20 {
			break
		}
		switch tv := v.(type) {
		case time.Time:
			return true
		case string:
			if _, ok := parseTimeString(tv); !ok {
				return strings.Contains(name, "time") || strings.Contains(name, "date") || strings.Contains(name, "ts")
			}
		case int64:
			if isLikelyEpoch(tv) {
				continue
			}
			return false
		case float64:
			if isLikelyEpoch(int64(tv)) {
				continue
			}
			return false
		default:
			return false
		}
	}
	return sampled > 0
}

// isLikelyEpoch reports whether n looks like a unix timestamp in
// seconds/ms/µs/ns rather than an ordinary number.
func isLikelyEpoch(n int64) bool {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1e17 && abs < 4e18: // nanoseconds (up to year 2096)
		return true
	case abs >= 1e14 && abs < 1e17: // microseconds
		return true
	case abs >= 1e11 && abs < 1e14: // milliseconds
		return true
	case abs >= 1e8 && abs < 1e11: // seconds
		return true
	}
	return false
}

// parseTimeString parses the timestamp formats OpenObserve emits.
func parseTimeString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ToTime converts a raw cell into a time.Time using the magnitude of numeric
// values to pick the epoch unit.
func ToTime(v any) (time.Time, bool) { return toTime(v) }

// ToString renders a raw cell as a display string.
func ToString(v any) string { return toString(v) }

// toTime converts a raw cell into a time.Time using the magnitude of numeric
// values to pick the epoch unit.
func toTime(v any) (time.Time, bool) {
	switch tv := v.(type) {
	case time.Time:
		return tv, true
	case int64:
		return epochToTime(tv)
	case float64:
		return epochToTime(int64(tv))
	case int:
		return epochToTime(int64(tv))
	case json.Number:
		if n, err := tv.Int64(); err == nil {
			return epochToTime(n)
		}
		return time.Time{}, false
	case string:
		if t, ok := parseTimeString(tv); ok {
			return t, true
		}
		if n, err := strconv.ParseInt(strings.TrimSpace(tv), 10, 64); err == nil {
			return epochToTime(n)
		}
	}
	return time.Time{}, false
}

func epochToTime(n int64) (time.Time, bool) {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1e17: // ns
		return time.Unix(0, n), true
	case abs >= 1e14: // µs
		return time.UnixMicro(n), true
	case abs >= 1e11: // ms
		return time.UnixMilli(n), true
	case abs >= 1e8: // s
		return time.Unix(n, 0), true
	}
	return time.Time{}, false
}

// schemaType maps an OpenObserve column type to a FieldType. The second return
// value reports whether the mapping is known.
func schemaType(hint string) (FieldType, bool) {
	switch {
	case hint == "":
		return FieldTypeString, false
	case strings.Contains(hint, "utf8"), strings.Contains(hint, "string"), strings.Contains(hint, "varchar"):
		return FieldTypeString, true
	case strings.Contains(hint, "bool"):
		return FieldTypeBool, true
	case strings.Contains(hint, "int"), strings.Contains(hint, "float"),
		strings.Contains(hint, "decimal"), strings.Contains(hint, "double"):
		return FieldTypeNumber, true
	case strings.Contains(hint, "timestamp"), strings.Contains(hint, "date/time"):
		return FieldTypeTime, true
	}
	return FieldTypeString, false
}

// valueType guesses a FieldType from the actual values of one column. The
// second return value is false when the values are mixed or empty, so the
// caller can fall back to the schema.
func (rs *ResultSet) valueType(idx int, name string) (FieldType, bool) {
	sawNumber, sawString, sawBool, sawTime, sawJSON := false, false, false, false, false
	checked := 0
	for _, row := range rs.Rows {
		if idx >= len(row) {
			continue
		}
		v := row[idx]
		if v == nil {
			continue
		}
		checked++
		switch tv := v.(type) {
		case bool:
			sawBool = true
		case int64, int, float64, json.Number:
			if isLikelyEpoch(toInt64(tv)) && columnIsTimeNamed(name) {
				sawTime = true
			} else {
				sawNumber = true
			}
		case string:
			if _, ok := parseTimeString(tv); ok && columnIsTimeNamed(name) {
				sawTime = true
			} else {
				sawString = true
			}
		default:
			sawJSON = true
		}
		if checked >= 50 {
			break
		}
	}
	switch {
	case sawJSON:
		return FieldTypeJSON, true
	case checked == 0:
		return FieldTypeString, false
	case sawTime && !sawString && !sawNumber && !sawBool:
		return FieldTypeTime, true
	case sawString && !sawNumber && !sawBool && !sawTime:
		return FieldTypeString, true
	case sawNumber && !sawString && !sawBool && !sawTime:
		return FieldTypeNumber, true
	case sawBool && !sawString && !sawNumber && !sawTime:
		return FieldTypeBool, true
	}
	// Mixed values: not decisive.
	return FieldTypeString, false
}

// classifyColumn decides the FrameBuilder type for a column.
//
// The stream schema is the better source when it applies, but it is keyed by
// column *name*, and a query is free to alias a computed expression onto a name
// that also exists in the stream (`SELECT count(*) AS "count"` against a stream
// that has a Utf8 "count" column). Blindly trusting the schema then types an
// aggregate as a string, which breaks every numeric panel.
//
// So: when the observed values unambiguously contradict the schema, believe the
// values; otherwise keep the schema, which is more specific (it tells time apart
// from numbers, for instance).
func (rs *ResultSet) classifyColumn(idx int, forceTime bool) FieldType {
	name := rs.Columns[idx]
	if forceTime {
		return FieldTypeTime
	}
	fromValues, valuesDecisive := rs.valueType(idx, name)
	hint, hintKnown := schemaType(strings.ToLower(rs.Schema[name]))
	if !hintKnown {
		return fromValues
	}
	if valuesDecisive && !compatibleTypes(hint, fromValues) {
		return fromValues
	}
	return hint
}

// compatibleTypes reports whether a value-derived type is consistent with the
// schema-derived type.
func compatibleTypes(schema, fromValues FieldType) bool {
	if schema == fromValues {
		return true
	}
	// A time column read back as a string or a number is still that column: the
	// schema is what tells us the epoch means a timestamp.
	if schema == FieldTypeTime && (fromValues == FieldTypeNumber || fromValues == FieldTypeString) {
		return true
	}
	return false
}

func columnIsTimeNamed(name string) bool {
	lower := strings.ToLower(name)
	for _, alias := range columnTimeAliases {
		if lower == alias {
			return true
		}
	}
	return strings.Contains(lower, "time") || strings.Contains(lower, "date") ||
		strings.HasSuffix(lower, "_ts") || lower == "ts"
}

func toInt64(v any) int64 {
	switch tv := v.(type) {
	case int64:
		return tv
	case int:
		return int64(tv)
	case float64:
		return int64(tv)
	case json.Number:
		n, _ := tv.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(tv), 10, 64)
		return n
	}
	return 0
}

// toFloat64 coerces a cell into a float64 for numeric frames.
func toFloat64(v any) (float64, bool) {
	switch tv := v.(type) {
	case float64:
		return tv, true
	case int64:
		return float64(tv), true
	case int:
		return float64(tv), true
	case json.Number:
		f, err := tv.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(tv), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	case bool:
		if tv {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// toString renders a cell as a string for log bodies and label values.
func toString(v any) string {
	switch tv := v.(type) {
	case nil:
		return ""
	case string:
		return tv
	case float64:
		if tv == math.Trunc(tv) && math.Abs(tv) < 1e15 {
			return strconv.FormatFloat(tv, 'f', -1, 64)
		}
		return strconv.FormatFloat(tv, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(tv, 10)
	case bool:
		return strconv.FormatBool(tv)
	case time.Time:
		return tv.Format(time.RFC3339Nano)
	case json.RawMessage:
		return string(tv)
	default:
		b, err := jsonAPI.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

// toJSONValue renders a cell as raw JSON for the labels field.
func toJSONValue(v any) json.RawMessage {
	switch tv := v.(type) {
	case json.RawMessage:
		return tv
	case nil:
		return json.RawMessage("null")
	case string:
		b, err := jsonAPI.Marshal(tv)
		if err != nil {
			return json.RawMessage(`""`)
		}
		return b
	}
	b, err := jsonAPI.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// LabelColumns returns the string columns that should become series labels for
// a wide time series frame, in result order.
func (rs *ResultSet) LabelColumns(timeCol string) []string {
	labels := make([]string, 0, 4)
	for i, name := range rs.Columns {
		if name == timeCol {
			continue
		}
		if rs.classifyColumn(i, false) == FieldTypeString {
			labels = append(labels, name)
		}
	}
	return labels
}

// NumericColumns returns the numeric columns in result order.
func (rs *ResultSet) NumericColumns(timeCol string) []string {
	nums := make([]string, 0, 4)
	for i, name := range rs.Columns {
		if name == timeCol {
			continue
		}
		if rs.classifyColumn(i, false) == FieldTypeNumber {
			nums = append(nums, name)
		}
	}
	return nums
}

// sortedKeys returns map keys in a stable order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
