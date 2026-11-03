package openobserve_test

import (
	"strings"
	"testing"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
)

func TestBuildBuilderSQLRawRows(t *testing.T) {
	got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{Stream: "vnet_bff_dev", Limit: 50}, 100)
	want := `SELECT * FROM "vnet_bff_dev" LIMIT 50`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildBuilderSQLAggregation(t *testing.T) {
	got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{
		Stream:      "go_goroutine_count",
		Aggregation: "avg",
		Field:       "value",
		Interval:    "1 minute",
		GroupBy:     []string{"service_name"},
		Limit:       100,
	}, 100)
	for _, want := range []string{
		"histogram(_timestamp, '1 minute') AS ts",
		"avg(\"value\") AS value",
		`FROM "go_goroutine_count"`,
		"GROUP BY ts, \"service_name\"",
		"ORDER BY ts",
		"LIMIT 100",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildBuilderSQLCountNeedsNoField(t *testing.T) {
	got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{
		Stream: "go_goroutine_count", Aggregation: "count", Interval: "5 minute", Limit: 10,
	}, 100)
	if !strings.Contains(got, "count(*) AS value") {
		t.Errorf("count(*) missing: %q", got)
	}
	if !strings.Contains(got, "histogram(_timestamp, '5 minute')") {
		t.Errorf("interval not applied: %q", got)
	}
}

// Every aggregation except count(*) needs a column; without one the statement
// would be malformed, so the builder reports "incomplete" instead.
func TestBuildBuilderSQLIncomplete(t *testing.T) {
	if got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{Stream: "x", Aggregation: "avg"}, 100); got != "" {
		t.Errorf("avg without a field = %q, want empty", got)
	}
	if got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{Stream: ""}, 100); got != "" {
		t.Errorf("no stream = %q, want empty", got)
	}
}

func TestBuildBuilderSQLDefaultInterval(t *testing.T) {
	got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{
		Stream: "x", Aggregation: "sum", Field: "v", Limit: 10,
	}, 100)
	if !strings.Contains(got, "histogram(_timestamp, '1 minute')") {
		t.Errorf("no default interval applied: %q", got)
	}
}

func TestIsValidAggregation(t *testing.T) {
	for _, ok := range []string{"", "avg", "count", "p99"} {
		if !openobserve.IsValidAggregation(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"median", "AVG", "drop"} {
		if openobserve.IsValidAggregation(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestBuildBuilderSQLEscapesIdentifiers(t *testing.T) {
	// A dotted column has to be quoted or OpenObserve reads it as a table path.
	got := openobserve.BuildBuilderSQL(openobserve.BuilderQuery{
		Stream: "prd", Aggregation: "avg", Field: "http.status_code", Interval: "1 minute", Limit: 5,
	}, 100)
	if !strings.Contains(got, `avg("http.status_code")`) {
		t.Errorf("dotted field not quoted: %q", got)
	}
}
