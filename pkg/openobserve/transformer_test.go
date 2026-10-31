package openobserve_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func loadSearchResponse(t *testing.T, name string) *openobserve.SearchResponse {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var resp openobserve.SearchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return &resp
}

func loadSchema(t *testing.T, name string) []openobserve.Schema {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var info openobserve.StreamInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	return info.Schema
}

func TestBuildLogsFrameFromRealResponse(t *testing.T) {
	resp := loadSearchResponse(t, "search_logs_all.json")
	rs := openobserve.NewResultSet(resp.Hits, nil)
	if got, want := len(rs.Rows), len(resp.Hits); got != want {
		t.Fatalf("row count = %d, want %d", got, want)
	}

	frames, err := openobserve.NewFrameBuilder().BuildFrames(rs, openobserve.FrameOptions{
		Format: openobserve.FormatLogs,
	})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}

	frame := frames[0]
	if frame.Meta == nil || frame.Meta.PreferredVisualization != "logs" {
		t.Fatalf("expected a logs frame, got meta %+v", frame.Meta)
	}

	timeField := frame.Fields[0]
	if timeField.Name != "time" {
		t.Fatalf("first field = %q, want time", timeField.Name)
	}
	if got := timeField.Type(); got != data.FieldTypeNullableTime {
		t.Errorf("time field type = %v, want %v", got, data.FieldTypeNullableTime)
	}
	if body := frame.Fields[1]; body.Name != "body" {
		t.Fatalf("second field = %q, want body", body.Name)
	} else if body.Len() != len(resp.Hits) {
		t.Fatalf("body field rows = %d, want %d", body.Len(), len(resp.Hits))
	}

	// The whole row must survive as extra attributes so Explore can show them.
	names := map[string]bool{}
	for _, f := range frame.Fields {
		names[f.Name] = true
	}
	for _, expected := range []string{"time", "body", "labels", "level", "service_name"} {
		if !names[expected] {
			t.Errorf("frame is missing the %q field", expected)
		}
	}
}

func TestBuildTimeSeriesFramesFromRealResponse(t *testing.T) {
	resp := loadSearchResponse(t, "search_metrics_timeseries.json")
	rs := openobserve.NewResultSet(resp.Hits, nil)

	frames, err := openobserve.NewFrameBuilder().BuildFrames(rs, openobserve.FrameOptions{
		Format: openobserve.FormatTimeSeries,
	})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	// Two services in the fixture -> two series.
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want 2 (one per service)", len(frames))
	}
	for _, frame := range frames {
		if frame.Meta == nil || frame.Meta.Type != "timeseries-wide" {
			t.Errorf("frame %q has meta type %v, want timeseries-wide", frame.Name, frame.Meta)
		}
		if frame.Fields[0].Name != "time" {
			t.Errorf("frame %q first field = %q, want time", frame.Name, frame.Fields[0].Name)
		}
		if len(frame.Fields) != 2 {
			t.Errorf("frame %q has %d fields, want time + value", frame.Name, len(frame.Fields))
		}
		if labels := frame.Fields[1].Labels; labels["service_name"] == "" {
			t.Errorf("frame %q value field is missing the service_name label: %v", frame.Name, labels)
		}
	}
}

func TestBuildTraceFrame(t *testing.T) {
	resp := loadSearchResponse(t, "search_traces_spans.json")
	rs := openobserve.NewResultSet(resp.Hits, nil)

	frames, err := openobserve.NewFrameBuilder().BuildFrames(rs, openobserve.FrameOptions{
		Format: openobserve.FormatTrace,
	})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	frame := frames[0]
	if frame.Meta.PreferredVisualization != "trace" {
		t.Fatalf("preferred visualization = %q, want trace", frame.Meta.PreferredVisualization)
	}

	required := map[string]bool{
		"traceID": false, "spanID": false, "parentSpanID": false,
		"operationName": false, "serviceName": false, "serviceTags": false,
		"startTime": false, "duration": false, "tags": false,
	}
	for _, f := range frame.Fields {
		if _, ok := required[f.Name]; ok {
			required[f.Name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("trace frame is missing %q", name)
		}
	}

	// startTime must be milliseconds: the fixture's first span starts at
	// 1793451754293460208 ns == 1793451754293.460208 ms.
	startField := frame.Fields[6]
	first, ok := startField.At(0).(float64)
	if !ok {
		t.Fatalf("startTime[0] type = %T, want float64", startField.At(0))
	}
	if first < 1.79e12 || first > 1.79e13 {
		t.Errorf("startTime[0] = %v, expected millisecond epoch", first)
	}
}

func TestBuildTableFrameUsesSchemaForTyping(t *testing.T) {
	resp := loadSearchResponse(t, "search_logs_all.json")
	schema := loadSchema(t, "schema_vnet_bff_dev.json")
	rs := openobserve.NewResultSet(resp.Hits, schema)

	frames, err := openobserve.NewFrameBuilder().BuildFrames(rs, openobserve.FrameOptions{
		Format: openobserve.FormatTable,
	})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	frame := frames[0]
	if frame.Meta.PreferredVisualization != "table" {
		t.Errorf("preferred visualization = %q, want table", frame.Meta.PreferredVisualization)
	}

	byName := map[string]int{}
	for i, f := range frame.Fields {
		byName[f.Name] = i
	}
	timeIdx, ok := byName["_timestamp"]
	if !ok {
		t.Fatalf("_timestamp field missing, fields: %v", byName)
	}
	if got := frame.Fields[timeIdx].Type(); got != data.FieldTypeNullableTime {
		t.Errorf("_timestamp type = %v, want %v", got, data.FieldTypeNullableTime)
	}
	if idx, ok := byName["code_line_number"]; ok {
		if got := frame.Fields[idx].Type(); got != data.FieldTypeNullableFloat64 {
			t.Errorf("code_line_number type = %v, want %v", got, data.FieldTypeNullableFloat64)
		}
	}
}

func TestCompareFormatsFromHistogramFixture(t *testing.T) {
	resp := loadSearchResponse(t, "search_logs_histogram.json")
	rs := openobserve.NewResultSet(resp.Hits, nil)

	// x_axis_1 is an ISO timestamp string, y_axis_1 a count: auto format must
	// produce a time series rather than a table.
	frames, err := openobserve.NewFrameBuilder().BuildFrames(rs, openobserve.FrameOptions{Format: openobserve.FormatAuto})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("no frames returned")
	}
	if got := frames[0].Meta.Type; got != "timeseries-wide" {
		t.Errorf("auto format chose %q, want timeseries-wide", got)
	}
}

func TestResultSetPreservesSchemaOrder(t *testing.T) {
	hits := []map[string]any{
		{"b": 1, "a": 2, "c": 3},
	}
	schema := []openobserve.Schema{{Name: "a", Type: "Int64"}, {Name: "c", Type: "Utf8"}}
	rs := openobserve.NewResultSet(hits, schema)
	want := []string{"a", "c", "b"}
	if len(rs.Columns) != len(want) {
		t.Fatalf("columns = %v, want %v", rs.Columns, want)
	}
	for i := range want {
		if rs.Columns[i] != want[i] {
			t.Fatalf("columns = %v, want %v", rs.Columns, want)
		}
	}
}

func TestEpochDetection(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"microseconds", float64(1793451754293460), true},
		{"milliseconds", float64(1793451754293), true},
		{"seconds", float64(1793451754), true},
		{"nanoseconds", float64(1793451754293460208), true},
		{"small number", float64(42), false},
		{"line number", float64(272), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := openobserve.ToTime(tc.in)
			if ok != tc.want {
				t.Errorf("ToTime(%v) = %v, want %v", tc.in, ok, tc.want)
			}
		})
	}
}

// A query may alias a computed expression onto a column name that also exists
// in the stream. The stream schema must not win over the actual values.
func TestAggregateAliasOverridesStreamSchema(t *testing.T) {
	rs := &openobserve.ResultSet{
		Columns: []string{"count", "errors", "service_name"},
		Schema: map[string]string{
			// The stream really has Utf8 columns with these names.
			"count":        "Utf8",
			"errors":       "Utf8",
			"service_name": "Utf8",
		},
		Rows: [][]any{
			{int64(64), int64(4), "svc-a"},
			{int64(12), int64(0), "svc-b"},
		},
	}

	frames, err := (&openobserve.FrameBuilder{}).BuildFrames(rs, openobserve.FrameOptions{Format: openobserve.FormatTable})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	frame := frames[0]
	want := map[string]data.FieldType{
		"count":  data.FieldTypeNullableFloat64,
		"errors": data.FieldTypeNullableFloat64,
	}
	for _, field := range frame.Fields {
		if expected, ok := want[field.Name]; ok && field.Type() != expected {
			t.Errorf("%s type = %v, want %v (the values are numeric)", field.Name, field.Type(), expected)
		}
	}
	// A genuine string column keeps the schema type.
	if got := frame.Fields[2].Type(); got != data.FieldTypeNullableString {
		t.Errorf("service_name type = %v, want string", got)
	}
}

// A time column that came back as a number must stay a time column: that is
// exactly what the schema is for.
func TestSchemaKeepsTimeTypeForNumericEpochs(t *testing.T) {
	rs := &openobserve.ResultSet{
		Columns: []string{"_timestamp", "value"},
		Schema:  map[string]string{"_timestamp": "Int64", "value": "Float64"},
		Rows:    [][]any{{int64(1793451754293460), 1.5}},
	}
	frames, err := (&openobserve.FrameBuilder{}).BuildFrames(rs, openobserve.FrameOptions{Format: openobserve.FormatTimeSeries})
	if err != nil {
		t.Fatalf("BuildFrames: %v", err)
	}
	frame := frames[0]
	if got := frame.Fields[0].Type(); got != data.FieldTypeNullableTime {
		t.Errorf("time column type = %v, want the schema-derived time type", got)
	}
}
