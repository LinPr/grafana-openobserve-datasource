package openobserve_test

import (
	"strings"
	"testing"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
)

func TestBuildSearchSQL(t *testing.T) {
	sql := openobserve.BuildSearchSQL(openobserve.TraceQuery{
		Mode: openobserve.TraceModeSearch, Stream: "prd",
		ServiceName: "vnet-bff", OperationName: "HTTP GET",
		Tags: []string{"http.status_code=200"}, MinDurationUS: 1000, MaxDurationUS: 5000,
		Limit: 20,
	}, 200)

	for _, want := range []string{
		`FROM "prd"`,
		`service_name = 'vnet-bff'`,
		`operation_name = 'HTTP GET'`,
		`"http.status_code" = '200'`,
		`duration >= 1000`,
		`duration <= 5000`,
		`GROUP BY trace_id`,
		`LIMIT 20`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("search sql missing %q:\n%s", want, sql)
		}
	}
	// A dotted tag key must stay one quoted identifier, not table.column.
	if strings.Contains(sql, `http.status_code = `) {
		t.Errorf("dotted tag key is not quoted:\n%s", sql)
	}
}

func TestBuildSearchSQLAvoidsAliasCollisions(t *testing.T) {
	// OpenObserve resolves a name inside the same projection, so aliasing an
	// aggregate onto its own source column fails with "unknown field".
	sql := openobserve.BuildSearchSQL(openobserve.TraceQuery{Stream: "prd"}, 200)
	for _, bad := range []string{"AS start_time", "AS service_name", "AS operation_name"} {
		if strings.Contains(sql, bad) {
			t.Errorf("aggregate aliased onto a source column (%q):\n%s", bad, sql)
		}
	}
	// OpenObserve has no min_by, so the entry span comes from a conditional
	// aggregate; aliasing an aggregate onto its own column also fails here.
	if !strings.Contains(sql, "CASE WHEN reference_parent_span_id IS NULL") {
		t.Errorf("search sql should pick the root span as the entry:\n%s", sql)
	}
	if strings.Contains(sql, "min_by(") {
		t.Errorf("min_by is not supported by OpenObserve:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY first_span DESC") {
		t.Errorf("search sql should order by the start-time alias:\n%s", sql)
	}
}

func TestBuildTraceByIDSQL(t *testing.T) {
	sql := openobserve.BuildTraceByIDSQL(openobserve.TraceQuery{Stream: "prd", TraceID: "abc'def"}, 200)
	if !strings.Contains(sql, `FROM "prd"`) {
		t.Errorf("trace sql missing stream:\n%s", sql)
	}
	if !strings.Contains(sql, `trace_id = 'abc''def'`) {
		t.Errorf("trace id is not escaped:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY start_time") {
		t.Errorf("spans must come back in start order:\n%s", sql)
	}
	if openobserve.BuildTraceByIDSQL(openobserve.TraceQuery{Stream: "prd"}, 200) != "" {
		t.Error("an empty trace id must not produce a query")
	}
}

func TestBuildGraphSQL(t *testing.T) {
	sql := openobserve.BuildGraphSQL(openobserve.TraceQuery{Stream: "prd"}, 500)
	for _, want := range []string{
		`FROM "prd" AS child`,
		`JOIN "prd" AS parent`,
		`child.reference_parent_span_id = parent.span_id`,
		`GROUP BY parent_service, child_service`,
		`ORDER BY calls DESC`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("graph sql missing %q:\n%s", want, sql)
		}
	}
}

func TestTagCondition(t *testing.T) {
	tests := map[string]string{
		"error=true":       `"error" = 'true'`,
		"http.status_code": `COALESCE("http.status_code", '') <> ''`,
		"":                 "",
	}
	for tag, want := range tests {
		// BuildSearchSQL exposes tagCondition through the WHERE clause.
		sql := openobserve.BuildSearchSQL(openobserve.TraceQuery{Stream: "s", Tags: []string{tag}}, 10)
		if want == "" {
			if strings.Contains(sql, "WHERE") {
				t.Errorf("empty tag produced a predicate: %s", sql)
			}
			continue
		}
		if !strings.Contains(sql, want) {
			t.Errorf("tag %q: want %q in\n%s", tag, want, sql)
		}
	}
}

func TestTraceLimitClamping(t *testing.T) {
	if sql := openobserve.BuildSearchSQL(openobserve.TraceQuery{Stream: "s", Limit: 999999}, 10); !strings.Contains(sql, "LIMIT 10000") {
		t.Errorf("limit is not capped:\n%s", sql)
	}
	if sql := openobserve.BuildSearchSQL(openobserve.TraceQuery{Stream: "s"}, 42); !strings.Contains(sql, "LIMIT 42") {
		t.Errorf("default limit not applied:\n%s", sql)
	}
}

func TestIsValidTraceMode(t *testing.T) {
	for _, m := range []string{"search", "traceId", "graph"} {
		if !openobserve.IsValidTraceMode(m) {
			t.Errorf("%q should be a valid mode", m)
		}
	}
	for _, m := range []string{"", "Search", "deps"} {
		if openobserve.IsValidTraceMode(m) {
			t.Errorf("%q should be rejected", m)
		}
	}
}
