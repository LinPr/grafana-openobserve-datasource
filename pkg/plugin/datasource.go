package plugin

import (
	"context"
	"math/rand"
	"net/http"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/models"
	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// Make sure Datasource implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. In this example datasource instance implements backend.QueryDataHandler,
// backend.CheckHealthHandler interfaces. Plugin should not implement all these
// interfaces - only those which are required for a particular task.
var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
)

// Datasource is an example datasource which can respond to data queries, reports
// its health and has streaming skills.
type Datasource struct {
	connectionID      int
	openObserveClient *openobserve.OpenObserveClient
	SqlParser         *openobserve.SqlParser
	transformer       *openobserve.Transformer
	resourceHandler   backend.CallResourceHandler
	queryHandler      backend.QueryDataHandler
}

// NewDatasource creates a new datasource instance.
func NewDatasource(_ context.Context, req backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(req)
	if err != nil {
		return nil, err
	}
	openobserveClient := openobserve.NewOpenObserveClient(config.Url, config.Username, config.DecryptedSecureJSONData.Password)

	// adapterMux is a HTTP request multiplexer that handles resource requests.
	adapterMux := http.NewServeMux()
	adapterMux.Handle("/openobserve/streams", http.HandlerFunc(openobserveClient.HandleListStreams))

	ds := &Datasource{
		connectionID:      rand.Intn(1000000),
		openObserveClient: openobserveClient,
		SqlParser:         openobserve.NewSqlParser(),
		transformer:       openobserve.NewTransformer(),
		resourceHandler:   httpadapter.New(adapterMux),
	}

	//queryTypes multiplexer, automatically dispatches requests to the appropriate handler based on the queryType in request.
	ds.registerQueryHandlers()
	return ds, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created. As soon as datasource settings change detected by SDK old datasource instance will
// be disposed and a new one will be created using NewSampleDatasource factory function.
func (d *Datasource) Dispose() {
	// Clean up datasource instance resources.
}

// QueryData query data source
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	// dispatch the request to the appropriate handler based on the query type.
	return d.queryHandler.QueryData(ctx, req)
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (d *Datasource) CheckHealth(_ context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	log.DefaultLogger.Debug("CheckHealth callend", "request", req)

	res := &backend.CheckHealthResult{}
	// config, err := models.LoadPluginSettings(*req.PluginContext.DataSourceInstanceSettings)

	if err := d.openObserveClient.HealthCheck(); err != nil {
		res.Status = backend.HealthStatusError
		res.Message = err.Error()
		return res, nil
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Data source is working",
	}, nil
}

func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return d.resourceHandler.CallResource(ctx, req, sender)
}
