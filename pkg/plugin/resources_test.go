package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
)

// callResource invokes one resource handler and returns the status code and the
// decoded JSON body. The handlers are plain http.HandlerFuncs, so the test does
// not need the Grafana resource adapter.
func callResource(t *testing.T, h http.HandlerFunc, rawQuery string) (int, any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/resource?"+rawQuery, nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	var payload any
	if body := rec.Body.Bytes(); len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("response is not JSON: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, payload
}

// ooHandler serves the OpenObserve endpoints the resource handlers touch.
func ooHandler(t *testing.T, streamsJSON, schemaJSON, searchJSON string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/streams"):
			_, _ = rw.Write([]byte(streamsJSON))
		case strings.HasSuffix(req.URL.Path, "/schema"):
			_, _ = rw.Write([]byte(schemaJSON))
		default:
			_, _ = rw.Write([]byte(searchJSON))
		}
	})
}

func TestHandleStreamsNamesAndSchema(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t,
		`{"list":[{"name":"b_stream","schema":[{"name":"body","type":"Utf8"}]},{"name":"a_stream","schema":[{"name":"body","type":"Utf8"}]}]}`,
		`{}`, `{}`))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleStreams, "type=logs")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got := payload.([]any); len(got) != 2 || got[0] != "a_stream" || got[1] != "b_stream" {
		t.Errorf("names = %v, want the streams sorted by name", got)
	}

	code, payload = callResource(t, h.handleStreams, "type=logs&withSchema=true")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	byStream := payload.(map[string]any)
	if _, ok := byStream["a_stream"]; !ok {
		t.Errorf("schema map = %v, want one entry per stream", byStream)
	}
}

func TestHandleStreamsReportsUpstreamError(t *testing.T) {
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusUnauthorized)
		_, _ = rw.Write([]byte(`Unauthorized Access`))
	}))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleStreams, "type=logs")
	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (the upstream status)", code)
	}
	if msg, _ := payload.(map[string]any)["error"].(string); !strings.Contains(msg, "Unauthorized") {
		t.Errorf("error = %q, want the upstream message", msg)
	}
}

func TestHandleSchemaRequiresStream(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t, `{}`, `{}`, `{}`))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleSchema, "type=logs")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if msg, _ := payload.(map[string]any)["error"].(string); !strings.Contains(msg, "stream") {
		t.Errorf("error = %q, want it to name the missing parameter", msg)
	}
}

func TestHandleSchemaReturnsTypedColumns(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t, `{}`,
		`{"name":"app","schema":[{"name":"_timestamp","type":"Int64"},{"name":"body","type":"Utf8"}]}`, `{}`))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleSchema, "type=logs&stream=app")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	fields := payload.([]any)
	if len(fields) != 2 {
		t.Fatalf("fields = %v, want 2", fields)
	}
	first := fields[0].(map[string]any)
	if first["name"] != "_timestamp" || first["type"] != "Int64" {
		t.Errorf("first field = %v, want the stream schema entry", first)
	}
}

func TestHandleTagKeysWithoutStreamListsStreams(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t,
		`{"list":[{"name":"zeta"},{"name":"alpha"}]}`, `{}`, `{}`))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleTagKeys, "type=logs")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	keys := payload.([]any)
	if len(keys) != 2 || keys[0] != "alpha" || keys[1] != "zeta" {
		t.Errorf("keys = %v, want the stream names sorted", keys)
	}
}

func TestHandleTagKeysWithStreamListsColumns(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t, `{}`,
		`{"name":"app","schema":[{"name":"service_name","type":"Utf8"},{"name":"body","type":"Utf8"}]}`, `{}`))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleTagKeys, "type=logs&stream=app")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	keys := payload.([]any)
	if len(keys) != 2 || keys[0] != "body" || keys[1] != "service_name" {
		t.Errorf("keys = %v, want the stream columns sorted", keys)
	}
}

func TestHandleTagValuesRequiresParams(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t, `{}`, `{}`, `{}`))
	h := &resourceHandler{ds: ds}

	for _, query := range []string{"type=logs&key=body", "type=logs&stream=app"} {
		code, payload := callResource(t, h.handleTagValues, query)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, code)
		}
		if msg, _ := payload.(map[string]any)["error"].(string); !strings.Contains(msg, "stream and key") {
			t.Errorf("%s: error = %q, want it to name both parameters", query, msg)
		}
	}
}

func TestHandleTagValuesDedupesAndSorts(t *testing.T) {
	ds := newTestDatasource(t, ooHandler(t, `{}`, `{}`,
		`{"hits":[{"service_name":"svc-b"},{"service_name":"svc-a"},{"service_name":"svc-b"}],"total":3}`))
	h := &resourceHandler{ds: ds}

	code, payload := callResource(t, h.handleTagValues, "type=logs&stream=app&key=service_name")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	values := payload.([]any)
	if len(values) != 2 || values[0] != "svc-a" || values[1] != "svc-b" {
		t.Errorf("values = %v, want the distinct values sorted", values)
	}
}

func TestDistinctValuesEscapesIdentifiers(t *testing.T) {
	var gotSQL string
	ds := newTestDatasource(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		var body openobserve.SearchRequestBody
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("cannot decode search body: %v", err)
		}
		gotSQL = body.Query.Sql
		_, _ = rw.Write([]byte(`{"hits":[],"total":0}`))
	}))

	if _, err := ds.distinctValues(t.Context(), "default", openobserve.LogsStream, `we"ird`, `ke"y`, 0, 0); err != nil {
		t.Fatalf("distinctValues: %v", err)
	}
	if !strings.Contains(gotSQL, `"we""ird"`) || !strings.Contains(gotSQL, `"ke""y"`) {
		t.Errorf("sql = %q, want the identifiers escaped by doubling the quotes", gotSQL)
	}
}

func TestParseEpochMicro(t *testing.T) {
	cases := map[string]int64{
		"":             0,
		"   ":          0,
		"1793451754":   1793451754,
		" 1793451754 ": 1793451754,
		"not-a-number": 0,
	}
	for raw, want := range cases {
		if got := parseEpochMicro(raw); got != want {
			t.Errorf("parseEpochMicro(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestQueryParamTrimsWhitespace(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/resource?"+url.Values{"key": {"  value  "}}.Encode(), nil)
	if got := queryParam(req, "key"); got != "value" {
		t.Errorf("queryParam = %q, want the value trimmed", got)
	}
}
