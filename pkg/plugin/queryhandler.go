package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/grafana/grafana-plugin-sdk-go/experimental/concurrent"
)

// registerQueryHandlers registers the query handlers for different query types.
func (ds *Datasource) registerQueryHandlers() {
	queryTypeMux := datasource.NewQueryTypeMux()
	queryTypeMux.HandleFunc("logs", ds.handleLogsQueryData)
	queryTypeMux.HandleFunc("metrics", ds.handleMetricsQueryData)
	queryTypeMux.HandleFunc("traces", ds.handleTracesQueryData)
	queryTypeMux.HandleFunc("", ds.handleFallback)
	ds.queryHandler = queryTypeMux
}

// handleLogsQueryData handles log queries.
func (ds *Datasource) handleLogsQueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return concurrent.QueryData(ctx, req, ds.queryStream, 10)
}

// handleMetricsQueryData handles metric queries.
func (ds *Datasource) handleMetricsQueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	// return concurrent.QueryData(ctx, req, ds.queryMetrics, 10)
	return concurrent.QueryData(ctx, req, ds.queryStream, 10)
}

// handleTracesQueryData handles trace queries.
func (ds *Datasource) handleTracesQueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return concurrent.QueryData(ctx, req, ds.queryStream, 10)
}

// handleFallback handles fallback queries that do not match any specific type.
func (ds *Datasource) handleFallback(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	return concurrent.QueryData(ctx, req, ds.queryFallback, 10)
}

func (ds *Datasource) queryStream(ctx context.Context, query concurrent.Query) backend.DataResponse {
	log.DefaultLogger.Debug("queryStream called", "refId", query.DataQuery.RefID)

	searchReqParam, searchReqBody, err := ds.prepareSearchRequest(query)
	if err != nil {
		log.DefaultLogger.Warn("queryStream: prepareSearchRequest error", "error", err, "refId", query.DataQuery.RefID)
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("prepareSearchRequest errpr: %v", err.Error()))
	}
	log.DefaultLogger.Debug("queryStream: prepareSearchRequest completed", "sql", searchReqBody.Sql, "streamType", searchReqParam.StreamType, "refId", query.DataQuery.RefID)

	searchResponse, err := ds.openObserveClient.Search(searchReqParam, searchReqBody)
	if err != nil {
		log.DefaultLogger.Warn("queryStream: Search error", "error", err, "refId", query.DataQuery.RefID)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("openObserveClient.Search error: %v", err.Error()))
	}
	log.DefaultLogger.Debug("queryStream: Search completed", "hitsCount", len(searchResponse.Hits), "refId", query.DataQuery.RefID)

	parsedSql, err := ds.SqlParser.ParseSql(searchReqBody.Sql)
	if err != nil {
		log.DefaultLogger.Warn("queryStream: ParseSql error", "error", err, "refId", query.DataQuery.RefID)
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("SqlParser.ParseSql error: %v", err.Error()))
	}
	log.DefaultLogger.Debug("queryStream: ParseSql completed", "refId", query.DataQuery.RefID)

	log.DefaultLogger.Debug("queryStream: About to call TransformStream", "hitsCount", len(searchResponse.Hits), "refId", query.DataQuery.RefID)

	// transform the OpenObserve response data into Grafana data frame
	// doc: https://grafana.com/developers/plugin-tools/introduction/data-frames
	frame, err := ds.transformer.TransformStream(parsedSql, searchResponse)
	if err != nil {
		log.DefaultLogger.Warn("queryStream: TransformStream error", "error", err, "refId", query.DataQuery.RefID)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("transformer.TransformLogsStream error: %v", err.Error()))
	}
	log.DefaultLogger.Debug("queryStream: TransformStream completed", "frameRows", frame.Rows(), "refId", query.DataQuery.RefID)

	frames := data.Frames{}
	frames = append(frames, frame)

	log.DefaultLogger.Debug("queryStream: Returning response", "framesCount", len(frames), "refId", query.DataQuery.RefID)
	return backend.DataResponse{
		Frames: frames,
	}

}

// queryFallback is a fallback handler for queries that do not match any specific type
// here we use it to handle queries emitted by the Grfana dynamic variables feature
func (ds *Datasource) queryFallback(ctx context.Context, q concurrent.Query) backend.DataResponse {
	log.DefaultLogger.Debug("queryFallback called", "query", q.DataQuery)

	searchReqParam, searchReqBody, err := ds.prepareSearchRequest(q)
	if err != nil {
		log.DefaultLogger.Warn("queryFallback: prepareSearchRequest error", "error", err)
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("prepareSearchRequest error: %v", err.Error()))
	}

	// If the rawSql is "\\dt" (postgresql style), fetch databease tables(openobserve streams)
	if strings.HasPrefix(searchReqBody.Sql, "\\dt") {
		log.DefaultLogger.Debug("queryFallback: Using fallbackDisplayTables")
		frame, err := ds.fallbackDisplayTables(searchReqParam.Organization, searchReqBody.Sql)
		if err != nil {
			return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("fallbackDisplayTables error: %v", err.Error()))
		}
		frames := data.Frames{}
		frames = append(frames, frame)

		return backend.DataResponse{
			Frames: frames,
		}
	}

	log.DefaultLogger.Debug("queryFallback: Using fallbackSelectFrom", "sql", searchReqBody.Sql)
	frame, err := ds.fallbackSelectFrom(searchReqParam, searchReqBody)
	if err != nil {
		log.DefaultLogger.Warn("queryFallback: fallbackSelectFrom error", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("fallbackSelectFrom error: %v", err.Error()))
	}

	frames := data.Frames{}
	frames = append(frames, frame)

	return backend.DataResponse{
		Frames: frames,
	}
}

func (ds *Datasource) fallbackDisplayTables(organization string, rawSql string) (*data.Frame, error) {
	parts := strings.Split(rawSql, " ")
	if len(parts) != 2 || parts[0] != "\\dt" {
		return nil, fmt.Errorf("invalid rawSql: %s, expected format: \\dt <stream_Type>", rawSql)
	}

	if parts[1] != "logs" && parts[1] != "metrics" && parts[1] != "traces" {
		return nil, fmt.Errorf("invalid stream type: %s, expected one of: logs, metrics, traces", parts[1])
	}

	listStreamParam := &openobserve.ListStreamRequestParam{
		Organization: organization,
		StreamType:   parts[1],
	}
	listStreamResp, err := ds.openObserveClient.ListStreams(listStreamParam)
	if err != nil {
		return nil, fmt.Errorf("openObserveClient.ListStreams error: %v", err.Error())
	}

	frame, err := ds.transformer.TransformFallbackDisplayTables(listStreamResp)
	if err != nil {
		return nil, fmt.Errorf("transformer.TransformFallbackDisplayTables error: %v", err.Error())
	}
	return frame, nil
}

func (ds *Datasource) fallbackSelectFrom(searchReqParam *openobserve.SearchRequestParam, searchReqBody *openobserve.SearchRequestBody) (*data.Frame, error) {
	log.DefaultLogger.Debug("fallbackSelectFrom called", "searchReqParam", searchReqParam, "searchReqBody.Sql", searchReqBody.Sql)

	// Perform the search request to OpenObserve
	searchResponse, err := ds.openObserveClient.Search(searchReqParam, searchReqBody)
	if err != nil {
		log.DefaultLogger.Warn("fallbackSelectFrom: Search error", "error", err)
		return nil, fmt.Errorf("openObserveClient.Search error: %v", err.Error())
	}
	log.DefaultLogger.Debug("fallbackSelectFrom: Search completed", "hitsCount", len(searchResponse.Hits))

	parsedSql, err := ds.SqlParser.ParseSql(searchReqBody.Sql)
	if err != nil {
		log.DefaultLogger.Warn("fallbackSelectFrom: ParseSql error", "error", err)
		return nil, fmt.Errorf("SqlParser.ParseSql error: %v", err.Error())
	}
	log.DefaultLogger.Debug("fallbackSelectFrom: ParseSql completed")

	// transform the OpenObserve response data into Grafana data frame
	// doc: https://grafana.com/developers/plugin-tools/introduction/data-frames
	log.DefaultLogger.Debug("fallbackSelectFrom: About to call TransformFallbackSelectFrom", "hitsCount", len(searchResponse.Hits))
	frame, err := ds.transformer.TransformFallbackSelectFrom(parsedSql, searchResponse)
	if err != nil {
		log.DefaultLogger.Warn("fallbackSelectFrom: TransformFallbackSelectFrom error", "error", err)
		return nil, fmt.Errorf("transformer.TransformFallbackSelectFrom error: %v", err.Error())
	}
	log.DefaultLogger.Debug("fallbackSelectFrom: TransformFallbackSelectFrom completed", "frameRows", frame.Rows())

	return frame, nil
}

type grafanaQueryModel struct {
	// Organization string `json:"organization"`
	QueryType    string                `json:"queryType"`  // logs, metrics, traces
	SearchType   string                `json:"searchType"` // e.g., "ui", "api"
	UseCache     bool                  `json:"useCache"`   // Whether to use cache or not
	EnableSSE    bool                  `json:"enableSSE"`  // Whether to enable Server-Sent Events (SSE) for real-time data streaming
	RawSql       string                `json:"rawSql"`
	From         int64                 `json:"from"`
	Size         int64                 `json:"size"`
	AdHocFilters []AdHocVariableFilter `json:"adhocFilters"` // Ad-hoc filters for the query
}

type AdHocVariableFilter struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Operator string `json:"operator"` // e.g., "=", "!=", "IN", "NOT IN"
}

type Organization struct {
	Database string `json:"database"`
}

func (ds *Datasource) prepareSearchRequest(q concurrent.Query) (*openobserve.SearchRequestParam, *openobserve.SearchRequestBody, error) {
	pCtx := q.PluginContext
	query := q.DataQuery
	log.DefaultLogger.Debug("prepareSearchRequest called", "refId", query.RefID, "queryType", query.QueryType)

	var organization Organization
	if err := json.Unmarshal(pCtx.DataSourceInstanceSettings.JSONData, &organization); err != nil {
		log.DefaultLogger.Warn("prepareSearchRequest: Failed to unmarshal JSONData", "error", err)
		return nil, nil, err
	}
	log.DefaultLogger.Debug("prepareSearchRequest: Organization loaded", "database", organization.Database, "refId", query.RefID)

	// Unmarshal the JSON into our queryModel.
	var gqm grafanaQueryModel

	if err := json.Unmarshal(query.JSON, &gqm); err != nil {
		log.DefaultLogger.Warn("prepareSearchRequest: Failed to unmarshal query JSON", "error", err, "refId", query.RefID)
		return nil, nil, fmt.Errorf("json unmarshal query error: %v", err.Error())
	}
	log.DefaultLogger.Debug("prepareSearchRequest: Query model loaded", "queryType", gqm.QueryType, "rawSql", gqm.RawSql, "refId", query.RefID)
	filters := make([]openobserve.WhereFilter, 0, len(gqm.AdHocFilters))
	for _, filter := range gqm.AdHocFilters {
		filters = append(filters, openobserve.WhereFilter{
			Key:       filter.Key,
			Value:     filter.Value,
			Operation: filter.Operator,
		})
	}

	var completedSql string
	if strings.HasPrefix(gqm.RawSql, "\\dt") {
		completedSql = gqm.RawSql
	} else {
		sql, err := ds.SqlParser.CompeleteSqlWithAdhocFilters(gqm.RawSql, filters)
		if err != nil {
			// If the parser fails, use the raw SQL.
			log.DefaultLogger.Warn("SqlParser.CompeleteSqlWithAdhocFilters failed, using RawSql", "error", err)
			completedSql = gqm.RawSql
		} else {
			completedSql = sql
		}
		log.DefaultLogger.Debug("Completed SQL", "completedSql", completedSql)
	}

	// TODO: set the default values for
	gqm.SearchType = openobserve.SearchTypeUI // Default to UI search type if not specified
	gqm.UseCache = true                       // Default to using cache
	gqm.Size = 200                            // default search result size to 200

	log.DefaultLogger.Debug("prepareSearchRequest: Setting request params", "queryType", gqm.QueryType, "streamType", gqm.QueryType, "enableSSE", gqm.EnableSSE, "refId", query.RefID)

	searchReqParam := &openobserve.SearchRequestParam{
		Organization: organization.Database,
		StreamType:   gqm.QueryType,
		SearchType:   gqm.SearchType,
		UseCache:     gqm.UseCache,
		EnableSSE:    gqm.EnableSSE,
	}
	log.DefaultLogger.Debug("prepareSearchRequest: SearchRequestParam created", "organization", searchReqParam.Organization, "streamType", searchReqParam.StreamType, "enableSSE", searchReqParam.EnableSSE, "refId", query.RefID)

	searchReqBody := &openobserve.SearchRequestBody{
		Query: openobserve.Query{
			Sql:       completedSql,
			StartTime: query.TimeRange.From.UnixMicro(),
			EndTime:   query.TimeRange.To.UnixMicro(),
			From:      gqm.From,
			Size:      gqm.Size, // Default size, can be adjusted based on requirements
		},
		SearchType: openobserve.SearchTypeUI,
		Timeout:    60, // default to 60 seconds timeout
	}

	return searchReqParam, searchReqBody, nil
}
