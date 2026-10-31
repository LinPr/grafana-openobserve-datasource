package openobserve_test

import (
	"strings"
	"testing"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
)

func TestParseSelectAll(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	pq, err := qb.Parse(`SELECT * FROM "vnet_bff_dev" LIMIT 100`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pq.Mode != openobserve.SelectAll {
		t.Errorf("mode = %v, want SelectAll", pq.Mode)
	}
	if pq.Table != "vnet_bff_dev" {
		t.Errorf("table = %q, want vnet_bff_dev", pq.Table)
	}
	if pq.Limit != 100 {
		t.Errorf("limit = %d, want 100", pq.Limit)
	}
}

func TestParseSelectColumns(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	sql := `SELECT histogram(_timestamp, '1 minute') AS ts, avg(value) AS val, service_name
FROM "go_goroutine_count"
GROUP BY ts, service_name
ORDER BY ts
LIMIT 500`

	pq, err := qb.Parse(sql)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pq.Mode != openobserve.SelectColumns {
		t.Errorf("mode = %v, want SelectColumns", pq.Mode)
	}
	if pq.Table != "go_goroutine_count" {
		t.Errorf("table = %q", pq.Table)
	}
	if pq.Limit != 500 {
		t.Errorf("limit = %d, want 500", pq.Limit)
	}
	if !pq.HasGroupBy || !pq.HasOrderBy {
		t.Errorf("expected group by and order by to be detected: %+v", pq)
	}
	if len(pq.Columns) != 3 {
		t.Errorf("columns = %v, want 3 entries", pq.Columns)
	}
}

func TestParseRejectsNonSelect(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	if _, err := qb.Parse(`DELETE FROM "x"`); err == nil {
		t.Fatal("expected an error for a non-SELECT statement")
	}
	if _, err := qb.Parse(`   `); err == nil {
		t.Fatal("expected an error for an empty statement")
	}
}

func TestParseCTE(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	pq, err := qb.Parse(`WITH t AS (SELECT * FROM "stream_a") SELECT count(*) FROM t LIMIT 10`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pq.Table != "t" {
		t.Errorf("table = %q, want t", pq.Table)
	}
	if pq.Mode != openobserve.SelectAggregate {
		t.Errorf("mode = %v, want SelectAggregate", pq.Mode)
	}
}

func TestApplyAdHocFiltersAddsWhere(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	got, err := qb.ApplyAdHocFilters(
		`SELECT * FROM "logs" LIMIT 10`,
		[]openobserve.WhereFilter{{Key: "service_name", Value: "vnet-bff", Operation: "="}},
	)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	want := `SELECT * FROM "logs" WHERE service_name = 'vnet-bff' LIMIT 10`
	if got != want {
		t.Errorf("sql = %q, want %q", got, want)
	}
}

func TestApplyAdHocFiltersWhereBeforeGroupBy(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	got, err := qb.ApplyAdHocFilters(
		`SELECT service_name, count(*) AS c FROM "logs" GROUP BY service_name ORDER BY c DESC`,
		[]openobserve.WhereFilter{{Key: "level", Value: "error", Operation: "="}},
	)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	if !strings.Contains(got, `WHERE level = 'error' GROUP BY`) {
		t.Errorf("filter not inserted before GROUP BY: %q", got)
	}
}

func TestApplyAdHocFiltersKeepsExistingPrecedence(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	got, err := qb.ApplyAdHocFilters(
		`SELECT * FROM "logs" WHERE a = '1' OR b = '2' LIMIT 5`,
		[]openobserve.WhereFilter{{Key: "env", Value: "prod", Operation: "="}},
	)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	want := `SELECT * FROM "logs" WHERE (a = '1' OR b = '2') AND env = 'prod' LIMIT 5`
	if got != want {
		t.Errorf("sql = %q, want %q", got, want)
	}
}

func TestApplyAdHocFiltersEscapesValue(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	got, err := qb.ApplyAdHocFilters(
		`SELECT * FROM "logs"`,
		[]openobserve.WhereFilter{{Key: "msg", Value: "it's broken", Operation: "="}},
	)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	if !strings.Contains(got, `msg = 'it''s broken'`) {
		t.Errorf("value not escaped: %q", got)
	}
}

func TestApplyAdHocFiltersNumericAndRegex(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	got, err := qb.ApplyAdHocFilters(
		`SELECT * FROM "logs"`,
		[]openobserve.WhereFilter{
			{Key: "code_line_number", Value: "number(100)", Operation: ">="},
			{Key: "service_name", Value: "vnet.*", Operation: "=~"},
		},
	)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	if !strings.Contains(got, `code_line_number >= 100`) {
		t.Errorf("numeric filter not rendered as a number: %q", got)
	}
	// OpenObserve uses '~' for regex, not Grafana's '=~'.
	if !strings.Contains(got, `service_name ~ 'vnet.*'`) {
		t.Errorf("regex filter not translated to '~': %q", got)
	}
}

func TestApplyAdHocFiltersInList(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	got, err := qb.ApplyAdHocFilters(
		`SELECT * FROM "logs"`,
		[]openobserve.WhereFilter{{Key: "service_name", Value: "a,b", Operation: "IN"}},
	)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	if !strings.Contains(got, `service_name IN ('a', 'b')`) {
		t.Errorf("in-list not rendered: %q", got)
	}
}

func TestApplyAdHocFiltersUnsupportedOperator(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	if _, err := qb.ApplyAdHocFilters(`SELECT * FROM "logs"`,
		[]openobserve.WhereFilter{{Key: "a", Value: "b", Operation: "~~"}}); err == nil {
		t.Fatal("expected an error for an unsupported operator")
	}
}

func TestApplyAdHocFiltersNoFiltersIsIdentity(t *testing.T) {
	qb := openobserve.NewQueryBuilder()
	const sql = `SELECT * FROM "logs" WHERE a = '1'`
	got, err := qb.ApplyAdHocFilters(sql, nil)
	if err != nil {
		t.Fatalf("ApplyAdHocFilters: %v", err)
	}
	if got != sql {
		t.Errorf("sql changed: %q", got)
	}
}
