package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/models"
	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
)

// Datasource is one configured OpenObserve data source instance.
type Datasource struct {
	settings *models.PluginSettings
	client   *openobserve.Client

	queryBuilder *openobserve.QueryBuilder
	builder      *openobserve.FrameBuilder

	resourceHandler backend.CallResourceHandler
}

// NewDatasource builds a datasource instance from the Grafana settings.
func NewDatasource(ctx context.Context, req backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	settings, err := models.LoadPluginSettings(req)
	if err != nil {
		return nil, err
	}
	client, err := openobserve.NewClient(openobserve.ClientConfig{
		BaseURL:            settings.URL,
		Organization:       settings.JsonData.Organization,
		Username:           settings.Username,
		Password:           settings.Password,
		AuthMethod:         settings.JsonData.AuthMethod,
		Token:              settings.JsonData.Token,
		InsecureSkipVerify: settings.JsonData.TLSInsecureSkipVerify,
		Timeout:            settings.JsonData.Timeout(),
	})
	if err != nil {
		return nil, err
	}

	ds := &Datasource{
		settings:     settings,
		client:       client,
		queryBuilder: openobserve.NewQueryBuilder(),
		builder:      openobserve.NewFrameBuilder(),
	}

	mux := http.NewServeMux()
	handler := &resourceHandler{ds: ds}
	mux.HandleFunc("/openobserve/streams", handler.handleStreams)
	mux.HandleFunc("/openobserve/schema", handler.handleSchema)
	mux.HandleFunc("/openobserve/tag-keys", handler.handleTagKeys)
	mux.HandleFunc("/openobserve/tag-values", handler.handleTagValues)
	mux.HandleFunc("/openobserve/metric-names", handler.handleMetricNames)
	mux.HandleFunc("/openobserve/promql-metadata", handler.handlePromQLMetadata)
	ds.resourceHandler = httpadapter.New(mux)

	log.DefaultLogger.Debug("openobserve datasource created",
		"url", settings.URL, "organization", settings.JsonData.Organization)
	return ds, nil
}

// Dispose releases instance resources.
func (d *Datasource) Dispose() {}

// QueryData dispatches the queries of one request, running them concurrently
// with a bounded worker pool.
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()
	if len(req.Queries) == 0 {
		return response, nil
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	sem := make(chan struct{}, 10)

	for _, q := range req.Queries {
		wg.Add(1)
		go func(q backend.DataQuery) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := d.runQuery(ctx, q)

			mu.Lock()
			response.Responses[q.RefID] = res
			mu.Unlock()
		}(q)
	}
	wg.Wait()

	return response, nil
}

// runQuery routes one data query to the SQL or PromQL runner.
func (d *Datasource) runQuery(ctx context.Context, q backend.DataQuery) (res backend.DataResponse) {
	defer func() {
		// A panic in one query must not take down the whole plugin process.
		if r := recover(); r != nil {
			log.DefaultLogger.Error("query panicked", "refId", q.RefID, "panic", fmt.Sprint(r))
			res = backend.ErrDataResponseWithSource(backend.StatusInternal, backend.ErrorSourcePlugin, fmt.Sprintf("query failed: %v", r))
		}
	}()

	model, err := parseQueryModel(q)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, err.Error())
	}
	if model.QueryMode == queryModePromQL {
		return d.runPromQLQuery(ctx, q, model)
	}
	// The trace editor drives its own SQL, so it must not go through the plain
	// SQL path that expects the user to have written a statement.
	if model.QueryType == openobserve.TraceStream && strings.TrimSpace(model.RawSql) == "" {
		return d.runTraceQuery(ctx, q, model)
	}
	return d.runSQLQuery(ctx, q)
}

// CheckHealth verifies the connection and reports a useful summary.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, d.settings.JsonData.Timeout())
	defer cancel()

	detail, err := d.client.HealthDetail(timeoutCtx)
	if err != nil {
		status := backend.HealthStatusError
		var httpErr *openobserve.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode >= 500 {
			status = backend.HealthStatusError
		}
		return &backend.CheckHealthResult{Status: status, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: detail,
		JSONDetails: mustJSON(map[string]any{
			"url":          d.settings.URL,
			"organization": d.settings.JsonData.Organization,
			"authMethod":   d.settings.JsonData.AuthMethod,
		}),
	}, nil
}

// CallResource serves the plugin's HTTP resources.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return d.resourceHandler.CallResource(ctx, req, sender)
}

func mustJSON(v any) []byte {
	b, err := jsonMarshal(v)
	if err != nil {
		return nil
	}
	return b
}
