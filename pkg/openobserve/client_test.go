package openobserve_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func newTestClient(t *testing.T, handler http.Handler) *openobserve.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := openobserve.NewClient(openobserve.ClientConfig{
		BaseURL:      srv.URL,
		Organization: "default",
		Username:     "user",
		Password:     "pass",
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestClientSearchRegular(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/default/_search" {
			t.Errorf("path = %q", req.URL.Path)
		}
		if got := req.URL.Query().Get("type"); got != "logs" {
			t.Errorf("type = %q", got)
		}
		if user, pass, ok := req.BasicAuth(); !ok || user != "user" || pass != "pass" {
			t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"hits":[{"_timestamp":1793451754293460,"body":"hello"}],"total":1}`))
	}))

	now := time.Now()
	resp, err := client.Search(context.Background(), &openobserve.SearchRequestParam{
		StreamType: "logs",
		SearchType: openobserve.SearchTypeUI,
	}, &openobserve.SearchRequestBody{
		Query: openobserve.Query{Sql: `SELECT * FROM "x"`, StartTime: now.Add(-time.Hour).UnixMicro(), EndTime: now.UnixMicro()},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Hits) != 1 || resp.Total != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestClientSearchSSEMergesBatches(t *testing.T) {
	body := strings.Join([]string{
		"event: progress\ndata: {\"percent\":0}\n\n",
		"event: search_response_metadata\ndata: {\"results\":{\"total\":7,\"hits\":[]}}\n\n",
		"event: search_response_hits\ndata: {\"hits\":[{\"_timestamp\":1,\"body\":\"a\"}]}\n\n",
		"event: search_response_hits\ndata: {\"hits\":[{\"_timestamp\":2,\"body\":\"b\"}]}\n\n",
		"data: [[DONE]]\n\n",
	}, "")

	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/default/_search_stream" {
			t.Errorf("path = %q", req.URL.Path)
		}
		rw.Header().Set("Content-Type", "text/event-stream")
		_, _ = rw.Write([]byte(body))
	}))

	resp, err := client.Search(context.Background(), &openobserve.SearchRequestParam{
		StreamType: "logs",
		EnableSSE:  true,
	}, &openobserve.SearchRequestBody{Query: openobserve.Query{Sql: `SELECT * FROM "x"`}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(resp.Hits))
	}
	if resp.Total != 7 {
		t.Fatalf("total = %d, want 7 (from the metadata event)", resp.Total)
	}
}

func TestClientSearchErrorBodySurfacesMessage(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusBadRequest)
		_, _ = rw.Write([]byte(`{"code":20002,"message":"Search stream not found: nope","trace_id":"abc"}`))
	}))

	_, err := client.Search(context.Background(), &openobserve.SearchRequestParam{StreamType: "logs"},
		&openobserve.SearchRequestBody{Query: openobserve.Query{Sql: "SELECT * FROM nope"}})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Search stream not found: nope") {
		t.Errorf("error = %v, want the OpenObserve message", err)
	}
}

func TestClientAuthenticationFailureIsActionable(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusUnauthorized)
		_, _ = rw.Write([]byte(`Unauthorized Access`))
	}))

	_, err := client.Search(context.Background(), &openobserve.SearchRequestParam{StreamType: "logs"},
		&openobserve.SearchRequestBody{Query: openobserve.Query{Sql: "SELECT 1"}})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("error = %v, want an authentication hint", err)
	}
}

func TestClientTokenAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Basic abc123" {
			t.Errorf("authorization = %q, want the configured token", got)
		}
		_, _ = rw.Write([]byte(`{"list":[]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := openobserve.NewClient(openobserve.ClientConfig{
		BaseURL:    srv.URL,
		AuthMethod: "token",
		Token:      "Basic abc123",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.ListStreams(context.Background(), "", "logs"); err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
}

func TestClientContextCancellation(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.ListStreams(ctx, "", "logs")
	if err == nil {
		t.Fatal("expected a context error")
	}
}

func TestClientInvalidURL(t *testing.T) {
	if _, err := openobserve.NewClient(openobserve.ClientConfig{BaseURL: ""}); err == nil {
		t.Fatal("expected an error for an empty url")
	}
	if _, err := openobserve.NewClient(openobserve.ClientConfig{BaseURL: "://bad"}); err == nil {
		t.Fatal("expected an error for a malformed url")
	}
	client, err := openobserve.NewClient(openobserve.ClientConfig{BaseURL: "127.0.0.1:5080"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := client.Config().BaseURL; got != "127.0.0.1:5080" {
		t.Errorf("base url = %q", got)
	}
}

func TestPromQueryRangeParsesMatrix(t *testing.T) {
	var gotQuery string
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/default/prometheus/api/v1/query_range" {
			t.Errorf("path = %q", req.URL.Path)
		}
		gotQuery = req.URL.Query().Get("query")
		if req.URL.Query().Get("step") == "" {
			t.Error("step is missing")
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{
		  "status":"success",
		  "data":{
		    "resultType":"matrix",
		    "result":[
		      {"metric":{"__name__":"go_goroutine_count","service_name":"vnet-bff"},
		       "values":[[1793450880,"42"],[1793450940,"46.5"]]}
		    ]
		  }
		}`))
	}))

	frames, err := client.Prom().QueryRange(context.Background(), "", "go_goroutine_count",
		time.Unix(1793450880, 0), time.Unix(1793450940, 0), time.Minute)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if gotQuery != "go_goroutine_count" {
		t.Errorf("query = %q", gotQuery)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	frame := frames[0]
	if frame.Meta.Type != data.FrameTypeTimeSeriesWide {
		t.Errorf("frame type = %q", frame.Meta.Type)
	}
	if frame.Fields[1].Len() != 2 {
		t.Fatalf("values = %d, want 2", frame.Fields[1].Len())
	}
	labels := frame.Fields[1].Labels
	if labels["service_name"] != "vnet-bff" {
		t.Errorf("labels = %v", labels)
	}
	if _, ok := labels["__name__"]; ok {
		t.Errorf("__name__ should be dropped from the labels: %v", labels)
	}
}

func TestPromQueryRangeSurfacesAPIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write([]byte(`{"status":"error","errorType":"bad_data","error":"unexpected left brace '{' inside braces"}`))
	}))
	_, err := client.Prom().QueryRange(context.Background(), "", "bad{{", time.Now().Add(-time.Hour), time.Now(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "bad_data") {
		t.Fatalf("error = %v, want the Prometheus error", err)
	}
}

func TestClientListStreamsReadsSchema(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"list": []map[string]any{{
			"name":        "vnet_bff_dev",
			"stream_type": "logs",
			"schema":      []map[string]string{{"name": "body", "type": "Utf8"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = rw.Write(raw)
	}))
	resp, err := client.ListStreams(context.Background(), "", "logs")
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if len(resp.List) != 1 || resp.List[0].Name != "vnet_bff_dev" {
		t.Fatalf("unexpected streams: %+v", resp.List)
	}
	if len(resp.List[0].Schema) != 1 || resp.List[0].Schema[0].Name != "body" {
		t.Fatalf("unexpected schema: %+v", resp.List[0].Schema)
	}
}

func TestStreamSchemaIsCached(t *testing.T) {
	calls := 0
	client := newTestClient(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		calls++
		_, _ = rw.Write([]byte(`{"name":"x","schema":[{"name":"a","type":"Utf8"}]}`))
	}))
	for i := 0; i < 3; i++ {
		if _, err := client.StreamSchema(context.Background(), "", "x", "logs"); err != nil {
			t.Fatalf("StreamSchema: %v", err)
		}
	}
	if calls != 1 {
		t.Errorf("http calls = %d, want 1 (cached)", calls)
	}
	client.InvalidateSchema()
	if _, err := client.StreamSchema(context.Background(), "", "x", "logs"); err != nil {
		t.Fatalf("StreamSchema: %v", err)
	}
	if calls != 2 {
		t.Errorf("http calls = %d, want 2 after invalidation", calls)
	}
}

// A self-signed certificate must be accepted when TLS verification is skipped
// and rejected when it is not; this is the only realistic way users run
// OpenObserve on an internal HTTPS endpoint.
func TestClientTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(`{"hits":[],"total":0}`))
	}))
	t.Cleanup(srv.Close)

	search := func(t *testing.T, insecure bool) error {
		t.Helper()
		client, err := openobserve.NewClient(openobserve.ClientConfig{
			BaseURL:            srv.URL,
			Organization:       "default",
			Username:           "user",
			Password:           "pass",
			Timeout:            5 * time.Second,
			InsecureSkipVerify: insecure,
		})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = client.Search(t.Context(), &openobserve.SearchRequestParam{
			Organization: "default",
			StreamType:   openobserve.LogsStream,
		}, &openobserve.SearchRequestBody{Query: openobserve.Query{Sql: "SELECT 1"}})
		return err
	}

	if err := search(t, false); err == nil {
		t.Error("want a certificate error when verification is enabled, got none")
	}
	if err := search(t, true); err != nil {
		t.Errorf("want the self-signed certificate to be accepted, got %v", err)
	}
}

// blockingHandler keeps the request open until the test releases it, so a
// client-side timeout or cancellation can be observed without leaking a
// goroutine into httptest.Server.Close.
func blockingHandler(stop <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		select {
		case <-stop:
		case <-req.Context().Done():
		case <-time.After(30 * time.Second):
		}
	})
}

// The per-request timeout must abort a slow upstream instead of blocking the
// panel forever.
func TestClientTimeoutAbortsSlowRequest(t *testing.T) {
	stop := make(chan struct{})
	srv := httptest.NewServer(blockingHandler(stop))
	t.Cleanup(srv.Close)

	client, err := openobserve.NewClient(openobserve.ClientConfig{
		BaseURL:      srv.URL,
		Organization: "default",
		Timeout:      80 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	start := time.Now()
	_, err = client.Search(t.Context(), &openobserve.SearchRequestParam{Organization: "default"},
		&openobserve.SearchRequestBody{Query: openobserve.Query{Sql: "SELECT 1"}})
	elapsed := time.Since(start)
	close(stop)

	if err == nil {
		t.Fatal("want a timeout error, got none")
	}
	if elapsed > 5*time.Second {
		t.Errorf("request blocked for %v, want it aborted near the 80ms timeout", elapsed)
	}
}

// Cancelling the context (Grafana cancels when the user leaves the panel) must
// abort the request rather than run to completion.
func TestClientContextCancelAbortsRequest(t *testing.T) {
	stop := make(chan struct{})
	srv := httptest.NewServer(blockingHandler(stop))
	t.Cleanup(srv.Close)

	client, err := openobserve.NewClient(openobserve.ClientConfig{
		BaseURL:      srv.URL,
		Organization: "default",
		Timeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = client.Search(ctx, &openobserve.SearchRequestParam{Organization: "default"},
		&openobserve.SearchRequestBody{Query: openobserve.Query{Sql: "SELECT 1"}})
	elapsed := time.Since(start)
	close(stop)

	if err == nil {
		t.Fatal("want a cancellation error, got none")
	}
	if elapsed > 5*time.Second {
		t.Errorf("request blocked for %v after cancellation", elapsed)
	}
}
