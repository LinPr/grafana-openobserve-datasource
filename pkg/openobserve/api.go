package openobserve

const (
	SearchTypeDefault    = ""
	SearchTypeUI         = "ui"
	SearchTypeDashBoards = "dashboards"
	SearchTypeReports    = "reports"
	SearchTypeAlerts     = "alerts"
)

const (
	LogsStream   = "logs"
	MetricStream = "metrics"
	TraceStream  = "traces"
)

// SearchRequestParam defines the parameters for the OpenObserve search request
type SearchRequestParam struct {
	Organization string `json:"organization"`
	StreamType   string `json:"stream_type"`
	SearchType   string `json:"search_type"`
	UseCache     bool   `json:"use_cache"`
}

// SearchRequestBody defines the body of the OpenObserve search request
type SearchRequestBody struct {
	Query      `json:"query"`
	SearchType string `json:"search_type"`
	Timeout    int    `json:"timeout"`
}

type Query struct {
	Sql       string `json:"sql"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	From      int64  `json:"from"`
	Size      int64  `json:"size"`
}

// SearchResponse defines the structure of the OpenObserve search response
type SearchResponse struct {
	Took             int              `json:"took"`
	TookDetail       SearchTookDetail `json:"took_detail"`
	Hits             []map[string]any `json:"hits"`
	Total            int              `json:"total"`
	From             int              `json:"from"`
	Size             int              `json:"size"`
	CachedRatio      int              `json:"cached_ratio"`
	ScanSize         int              `json:"scan_size"`
	IdxScanSize      int              `json:"idx_scan_size"`
	ScanRecords      int              `json:"scan_records"`
	TraceID          string           `json:"trace_id"`
	IsPartial        bool             `json:"is_partial"`
	ResultCacheRatio int              `json:"result_cache_ratio"`
	WorkGroup        string           `json:"work_group"`
	OrderBy          string           `json:"order_by"`
}

type SearchTookDetail struct {
	Total        int `json:"total"`
	CacheTook    int `json:"cache_took"`
	FileListTook int `json:"file_list_took"`
	WaitInQueue  int `json:"wait_in_queue"`
	IdxTook      int `json:"idx_took"`
	SearchTook   int `json:"search_took"`
}

// ListStreamRequestParam defines the request parameters for listing OpenObserve streams information
type ListStreamRequestParam struct {
	Organization string `json:"organization"`
	StreamType   string `json:"stream_type"` //stream type, e.g., "logs", "metrics", "traces"
	SortBy       string `json:"sort_by"`     // sort result by name or other fields
	Ascending    bool   `json:"ascending"`   // ascending or descending order
}

// ListStreamResponse defines the response structure for listing OpenObserve streams
type ListStreamResponse struct {
	List []StreamInfo `json:"list"`
}

// StreamInfo defines the structure of each stream in the OpenObserve cluster
type StreamInfo struct {
	Name        string   `json:"name"`
	StorageType string   `json:"storage_type"`
	StreamType  string   `json:"stream_type"`
	Schema      []Schema `json:"schema"`
}

type Schema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
