package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// newTestDatasource spins up a fake OpenObserve and returns a datasource
// pointing at it.
func newTestDatasource(t *testing.T, handler http.Handler) *Datasource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	jsonData, err := json.Marshal(map[string]any{
		"organization":   "default",
		"timeoutSeconds": 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	ds, err := NewDatasource(context.Background(), backend.DataSourceInstanceSettings{
		URL:      srv.URL,
		User:     "user",
		JSONData: jsonData,
		DecryptedSecureJSONData: map[string]string{
			"password": "pass",
		},
	})
	if err != nil {
		t.Fatalf("NewDatasource: %v", err)
	}
	return ds.(*Datasource)
}

func logsHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/streams"):
			_, _ = rw.Write([]byte(`{"list":[{"name":"app","schema":[{"name":"body","type":"Utf8"}]}]}`))
		case strings.HasSuffix(req.URL.Path, "/schema"):
			_, _ = rw.Write([]byte(`{"name":"app","schema":[
				{"name":"_timestamp","type":"Int64"},
				{"name":"body","type":"Utf8"},
				{"name":"service_name","type":"Utf8"}]}`))
		default:
			_, _ = rw.Write([]byte(`{"hits":[
				{"_timestamp":1793451754293460,"body":"hello","service_name":"svc-a"},
				{"_timestamp":1793451755293460,"body":"world","service_name":"svc-b"}],"total":2}`))
		}
	})
}

func queryRequest(t *testing.T, model map[string]any) *backend.QueryDataRequest {
	t.Helper()
	raw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	return &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID: "A",
			JSON:  raw,
			TimeRange: backend.TimeRange{
				From: now.Add(-time.Hour),
				To:   now,
			},
			MaxDataPoints: 100,
			Interval:      time.Minute,
		}},
	}
}

func TestQueryDataLogsFrame(t *testing.T) {
	ds := newTestDatasource(t, logsHandler(t))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "logs",
		"rawSql":    `SELECT * FROM "app" LIMIT 10`,
		"format":    "logs",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	res := resp.Responses["A"]
	if res.Error != nil {
		t.Fatalf("query error: %v", res.Error)
	}
	if len(res.Frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(res.Frames))
	}
	frame := res.Frames[0]
	if frame.Meta == nil || frame.Meta.PreferredVisualization != "logs" {
		t.Fatalf("expected a logs frame, got %+v", frame.Meta)
	}
	if frame.Fields[1].Name != "body" {
		t.Errorf("second field = %q, want body", frame.Fields[1].Name)
	}
	if frame.Meta.ExecutedQueryString == "" {
		t.Error("executedQueryString should be set for the query inspector")
	}
}

func TestQueryDataAutoFormatPicksTimeSeriesForAggregates(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/schema") {
			_, _ = rw.Write([]byte(`{"schema":[{"name":"ts","type":"Utf8"},{"name":"val","type":"Float64"}]}`))
			return
		}
		_, _ = rw.Write([]byte(`{"hits":[
			{"ts":"2026-10-31T12:00:00","val":1.5},
			{"ts":"2026-10-31T12:01:00","val":2.5}]}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "metrics",
		"rawSql":    `SELECT histogram(_timestamp, '1 minute') AS ts, avg(value) AS val FROM "m" GROUP BY ts`,
		"format":    "auto",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	res := resp.Responses["A"]
	if res.Error != nil {
		t.Fatalf("query error: %v", res.Error)
	}
	if len(res.Frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(res.Frames))
	}
	if got := res.Frames[0].Meta.Type; got != "timeseries-wide" {
		t.Errorf("frame type = %q, want timeseries-wide", got)
	}
}

func TestQueryDataRejectsEmptyAndInvalidQueries(t *testing.T) {
	ds := newTestDatasource(t, logsHandler(t))

	t.Run("empty sql", func(t *testing.T) {
		resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{"queryType": "logs"}))
		if err != nil {
			t.Fatalf("QueryData: %v", err)
		}
		res := resp.Responses["A"]
		if res.Error == nil {
			t.Fatal("expected an error for an empty query")
		}
		if res.ErrorSource != backend.ErrorSourceDownstream {
			t.Errorf("errorSource = %v, want downstream", res.ErrorSource)
		}
	})

	t.Run("not a select", func(t *testing.T) {
		resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
			"queryType": "logs",
			"rawSql":    `DELETE FROM "app"`,
		}))
		if err != nil {
			t.Fatalf("QueryData: %v", err)
		}
		if resp.Responses["A"].Error == nil {
			t.Fatal("expected an error for a non-SELECT statement")
		}
	})

	t.Run("invalid stream type", func(t *testing.T) {
		resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
			"queryType": "bogus",
			"rawSql":    `SELECT * FROM "app"`,
		}))
		if err != nil {
			t.Fatalf("QueryData: %v", err)
		}
		if resp.Responses["A"].Error == nil {
			t.Fatal("expected an error for an invalid stream type")
		}
	})
}

func TestQueryDataAppliesAdHocFilters(t *testing.T) {
	var gotSQL string
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/schema") {
			_, _ = rw.Write([]byte(`{"schema":[]}`))
			return
		}
		if strings.HasSuffix(req.URL.Path, "/streams") {
			_, _ = rw.Write([]byte(`{"list":[{"name":"app"}]}`))
			return
		}
		var body struct {
			Query struct {
				Sql string `json:"sql"`
			} `json:"query"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		gotSQL = body.Query.Sql
		_, _ = rw.Write([]byte(`{"hits":[]}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "logs",
		"rawSql":    `SELECT * FROM "app" LIMIT 10`,
		"format":    "table",
		"adhocFilters": []map[string]string{
			{"key": "service_name", "value": "svc-a", "operator": "="},
		},
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	if resp.Responses["A"].Error != nil {
		t.Fatalf("query error: %v", resp.Responses["A"].Error)
	}
	want := `SELECT * FROM "app" WHERE service_name = 'svc-a' LIMIT 10`
	if gotSQL != want {
		t.Errorf("sql = %q, want %q", gotSQL, want)
	}
}

func TestQueryDataClampsSize(t *testing.T) {
	var gotSize float64
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/schema") {
			_, _ = rw.Write([]byte(`{"schema":[]}`))
			return
		}
		// The result is empty, so the stream type is verified: answer that
		// "app" really is a log stream.
		if strings.HasSuffix(req.URL.Path, "/streams") {
			_, _ = rw.Write([]byte(`{"list":[{"name":"app"}]}`))
			return
		}
		var body struct {
			Query struct {
				Size float64 `json:"size"`
			} `json:"query"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		gotSize = body.Query.Size
		_, _ = rw.Write([]byte(`{"hits":[]}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "logs",
		"rawSql":    `SELECT * FROM "app"`,
		"size":      1000000,
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	if resp.Responses["A"].Error != nil {
		t.Fatalf("query error: %v", resp.Responses["A"].Error)
	}
	if gotSize != 10000 {
		t.Errorf("size = %v, want the 10000 cap", gotSize)
	}
}

func TestQueryDataPromQL(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.URL.Path, "/prometheus/api/v1/query_range") {
			t.Errorf("path = %q", req.URL.Path)
		}
		_, _ = rw.Write([]byte(`{"status":"success","data":{"resultType":"matrix",
			"result":[{"metric":{"__name__":"up","job":"api"},"values":[[1793450880,"1"],[1793450940,"0"]]}]}}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "metrics",
		"queryMode": "promql",
		"expr":      `up`,
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	res := resp.Responses["A"]
	if res.Error != nil {
		t.Fatalf("query error: %v", res.Error)
	}
	if len(res.Frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(res.Frames))
	}
	if res.Frames[0].Fields[1].Labels["job"] != "api" {
		t.Errorf("labels = %v", res.Frames[0].Fields[1].Labels)
	}
}

func TestCheckHealth(t *testing.T) {
	ds := newTestDatasource(t, logsHandler(t))

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if result.Status != backend.HealthStatusOk {
		t.Fatalf("status = %v, message = %q", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "default") {
		t.Errorf("message = %q, want the organization name", result.Message)
	}
}

func TestCheckHealthReportsAuthFailure(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusUnauthorized)
		_, _ = rw.Write([]byte("Unauthorized Access"))
	}))

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if result.Status != backend.HealthStatusError {
		t.Fatalf("status = %v, want error", result.Status)
	}
	if !strings.Contains(result.Message, "authentication failed") {
		t.Errorf("message = %q, want an authentication hint", result.Message)
	}
}

func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name  string
		model QueryModel
		want  string
	}{
		{"explicit logs", QueryModel{Format: "logs"}, "logs"},
		{"explicit table", QueryModel{Format: "table"}, "table"},
		{"trace stream auto", QueryModel{Format: "auto", QueryType: "traces"}, "trace"},
		{"logs auto", QueryModel{Format: "auto", QueryType: "logs"}, "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(resolveFormat(tc.model, nil)); got != tc.want {
				t.Errorf("format = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStripTrailingSemicolon(t *testing.T) {
	if got := stripTrailingSemicolon("SELECT 1;  "); got != "SELECT 1" {
		t.Errorf("got %q", got)
	}
}

func TestQueryDataExplainsStreamTypeMismatch(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/schema"):
			_, _ = rw.Write([]byte(`{"schema":[]}`))
		case strings.HasSuffix(req.URL.Path, "/streams"):
			// "app" only exists as a log stream, never as a metric stream.
			if req.URL.Query().Get("type") == "logs" {
				_, _ = rw.Write([]byte(`{"list":[{"name":"app"}]}`))
				return
			}
			_, _ = rw.Write([]byte(`{"list":[]}`))
		default:
			_, _ = rw.Write([]byte(`{"hits":[],"total":0}`))
		}
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "metrics",
		"rawSql":    `SELECT * FROM "app" LIMIT 10`,
		"format":    "table",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	dataResp := resp.Responses["A"]
	if dataResp.Error == nil {
		t.Fatal("want an error explaining the stream type mismatch, got a successful empty result")
	}
	msg := dataResp.Error.Error()
	for _, want := range []string{`"app"`, "logs stream", "metrics"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error = %q, want it to mention %q", msg, want)
		}
	}
}

func TestQueryDataKeepsEmptyResultForMatchingStreamType(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/schema"):
			_, _ = rw.Write([]byte(`{"schema":[]}`))
		case strings.HasSuffix(req.URL.Path, "/streams"):
			_, _ = rw.Write([]byte(`{"list":[{"name":"app"}]}`))
		default:
			_, _ = rw.Write([]byte(`{"hits":[],"total":0}`))
		}
	}))

	// "nothing matched the time range" must stay a normal empty result.
	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "logs",
		"rawSql":    `SELECT * FROM "app" WHERE body = 'nope' LIMIT 10`,
		"format":    "table",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	if dataResp := resp.Responses["A"]; dataResp.Error != nil {
		t.Fatalf("want an empty result, got error: %v", dataResp.Error)
	}
}

func TestParseDurationUS(t *testing.T) {
	cases := map[string]int64{
		"":      0,
		"1000":  1000,
		"1.2s":  1200000,
		"100ms": 100000,
		"500us": 500,
		"2m":    120000000,
	}
	for raw, want := range cases {
		got, err := parseDurationUS(raw)
		if err != nil {
			t.Errorf("parseDurationUS(%q): %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("parseDurationUS(%q) = %d, want %d", raw, got, want)
		}
	}
	for _, raw := range []string{"abc", "-5s", "1x"} {
		if _, err := parseDurationUS(raw); err == nil {
			t.Errorf("parseDurationUS(%q) should fail", raw)
		}
	}
}

func TestQueryDataTraceSearchBuildsListFrame(t *testing.T) {
	var gotSQL string
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		var body openobserveSearchBody
		_ = json.NewDecoder(req.Body).Decode(&body)
		gotSQL = body.Query.Sql
		_, _ = rw.Write([]byte(`{"hits":[
			{"trace_id":"aaa","first_span":1793451754293460,"total_duration":458975,"spans":7,"entry_operation":"HTTP GET","entry_service":"vnet-bff","services":2},
			{"trace_id":"bbb","first_span":1793451754293000,"total_duration":1000,"spans":1,"entry_operation":"GET /healthz","entry_service":"vnet-bff","services":1}
		],"total":2}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "traces",
		"traceMode": "search",
		"table":     "prd",
		"format":    "table",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	if e := resp.Responses["A"].Error; e != nil {
		t.Fatalf("query error: %v", e)
	}
	frames := resp.Responses["A"].Frames
	if len(frames) == 0 {
		t.Fatal("no frames returned")
	}
	if !strings.Contains(gotSQL, `FROM "prd"`) || !strings.Contains(gotSQL, "GROUP BY trace_id") {
		t.Errorf("unexpected sql: %s", gotSQL)
	}
	// One row per trace, with the columns the list needs.
	if got := frames[0].Fields[0].Len(); got != 2 {
		t.Errorf("rows = %d, want 2 traces", got)
	}
	// The columns mirror Grafana's Jaeger plugin, so the list reads the same.
	wantCols := []string{"traceID", "traceName", "startTime", "duration"}
	for i, want := range wantCols {
		if i >= len(frames[0].Fields) || frames[0].Fields[i].Name != want {
			t.Errorf("column %d = %q, want %q", i, frames[0].Fields[i].Name, want)
		}
	}
	// traceName is "service: operation", like Jaeger renders it.
	if got := frames[0].Fields[1].At(0); got != "vnet-bff: HTTP GET" {
		t.Errorf("traceName = %q, want \"vnet-bff: HTTP GET\"", got)
	}
}

func TestQueryDataTraceIDRequiresID(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(`{"hits":[]}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "traces", "traceMode": "traceId", "table": "prd",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	e := resp.Responses["A"].Error
	if e == nil {
		t.Fatal("want an error telling the user to enter a trace id")
	}
	if !strings.Contains(e.Error(), "trace id") {
		t.Errorf("error = %q, want it to mention the trace id", e.Error())
	}
}

func TestQueryDataTraceGraphBuildsEdges(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(`{"hits":[
			{"parent_service":"gateway","child_service":"orders","calls":120,"avg_duration_us":4500},
			{"parent_service":"orders","child_service":"db","calls":300,"avg_duration_us":900}
		],"total":2}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType": "traces", "traceMode": "graph", "table": "prd",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	if e := resp.Responses["A"].Error; e != nil {
		t.Fatalf("query error: %v", e)
	}
	frames := resp.Responses["A"].Frames
	if len(frames) == 0 {
		t.Fatal("no graph frame")
	}
	// Two frames in the node-graph shape Grafana's panel consumes: nodes and
	// edges, both tagged with the nodeGraph visualisation.
	if len(frames) != 2 {
		t.Fatalf("frames = %d, want nodes + edges", len(frames))
	}
	nodes, edges := frames[0], frames[1]
	for _, f := range []*data.Frame{nodes, edges} {
		if f.Meta == nil || f.Meta.PreferredVisualization != data.VisTypeNodeGraph {
			t.Errorf("%s is not marked as a node graph", f.Name)
		}
	}
	names := func(f *data.Frame) map[string]bool {
		out := map[string]bool{}
		for _, field := range f.Fields {
			out[field.Name] = true
		}
		return out
	}
	for _, want := range []string{"id", "title"} {
		if !names(nodes)[want] {
			t.Errorf("node frame is missing %q", want)
		}
	}
	for _, want := range []string{"id", "source", "target", "mainstat"} {
		if !names(edges)[want] {
			t.Errorf("edge frame is missing %q", want)
		}
	}
	// Three services across the two edges -> three nodes.
	if got := nodes.Fields[0].Len(); got != 3 {
		t.Errorf("nodes = %d, want 3 (gateway, orders, db)", got)
	}
	if got := edges.Fields[0].Len(); got != 2 {
		t.Errorf("edges = %d, want 2", got)
	}
}

// openobserveSearchBody is the subset of the search payload the trace tests read.
type openobserveSearchBody struct {
	Query struct {
		Sql string `json:"sql"`
	} `json:"query"`
}

// A trace query keeps a stale rawSql whenever the editor was used for another
// stream type first. It must still go through the trace path, which is what
// this reproduces: the request below is copied from a real failing panel.
func TestQueryDataTraceIgnoresLeftoverRawSql(t *testing.T) {
	var gotSQL string
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		var body openobserveSearchBody
		_ = json.NewDecoder(req.Body).Decode(&body)
		gotSQL = body.Query.Sql
		_, _ = rw.Write([]byte(`{"hits":[
			{"trace_id":"aaa","first_span":1793591689400000,"total_duration":1000,
			 "spans":1,"entry_operation":"HTTP GET","entry_service":"vnet-bff","services":1}
		],"total":1}`))
	}))

	resp, err := ds.QueryData(context.Background(), queryRequest(t, map[string]any{
		"queryType":     "traces",
		"queryMode":     "sql",
		"format":        "auto",
		"traceMode":     "search",
		"table":         "prd",
		"rawSql":        `SELECT * FROM "dev" LIMIT 100`,
		"serviceName":   "",
		"operationName": "",
	}))
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	if e := resp.Responses["A"].Error; e != nil {
		t.Fatalf("query error: %v", e)
	}
	if len(resp.Responses["A"].Frames) == 0 {
		t.Fatal("no frames: the query fell through to the plain SQL path")
	}
	// The trace handler builds its own SQL against the selected stream, ignoring
	// the stale statement.
	if !strings.Contains(gotSQL, `FROM "prd"`) || !strings.Contains(gotSQL, "GROUP BY trace_id") {
		t.Errorf("trace query was not used; sql = %q", gotSQL)
	}
	if strings.Contains(gotSQL, `"dev"`) {
		t.Errorf("the leftover rawSql leaked into the query: %q", gotSQL)
	}
}
