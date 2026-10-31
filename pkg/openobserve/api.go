package openobserve

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

// StreamTypes lists the stream types OpenObserve exposes.
var StreamTypes = []string{LogsStream, MetricStream, TraceStream}

// IsValidStreamType reports whether t is a known OpenObserve stream type.
func IsValidStreamType(t string) bool {
	for _, s := range StreamTypes {
		if s == t {
			return true
		}
	}
	return false
}

// SearchRequestParam defines the parameters for the OpenObserve search request.
type SearchRequestParam struct {
	Organization string `json:"organization"`
	StreamType   string `json:"stream_type"`
	SearchType   string `json:"search_type"`
	UseCache     bool   `json:"use_cache"`
	EnableSSE    bool   `json:"enable_sse"`
}

// SearchRequestBody defines the body of the OpenObserve search request.
type SearchRequestBody struct {
	Query      `json:"query"`
	SearchType string `json:"search_type,omitempty"`
	Timeout    int    `json:"timeout,omitempty"`
}

// Query is the OpenObserve search payload.
type Query struct {
	Sql       string `json:"sql"`
	StartTime int64  `json:"start_time"` // microseconds
	EndTime   int64  `json:"end_time"`   // microseconds
	From      int64  `json:"from"`
	Size      int64  `json:"size"`
}

// SearchResponse defines the structure of the OpenObserve search response.
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
	// Histogram holds the per-bucket counts OpenObserve returns for the
	// log-volume histogram in Explore.
	Histogram []HistogramBucket `json:"histogram"`
}

// HistogramBucket is one bucket of the search histogram.
type HistogramBucket struct {
	BucketStart int64 `json:"zo_sql_key,string"`
	Count       int64 `json:"zo_sql_num"`
}

// TookDetail reports where the query time went.
type SearchTookDetail struct {
	Total        int `json:"total"`
	CacheTook    int `json:"cache_took"`
	FileListTook int `json:"file_list_took"`
	WaitInQueue  int `json:"wait_in_queue"`
	IdxTook      int `json:"idx_took"`
	SearchTook   int `json:"search_took"`
}

// ListStreamRequestParam defines the request parameters for listing streams.
type ListStreamRequestParam struct {
	Organization string `json:"organization"`
	StreamType   string `json:"stream_type"` // "logs", "metrics", "traces"
	SortBy       string `json:"sort_by"`
	Ascending    bool   `json:"ascending"`
	Offset       int    `json:"offset"`
	Limit        int    `json:"limit"`
}

// ListStreamResponse defines the response structure for listing streams.
type ListStreamResponse struct {
	List  []StreamInfo `json:"list"`
	Total int          `json:"total"`
}

// StreamInfo describes one stream of the cluster.
type StreamInfo struct {
	Name        string         `json:"name"`
	StorageType string         `json:"storage_type"`
	StreamType  string         `json:"stream_type"`
	Schema      []Schema       `json:"schema"`
	Stats       *StreamStats   `json:"stats,omitempty"`
	Settings    map[string]any `json:"settings,omitempty"`
}

// StreamStats carries the per-stream row/time statistics OpenObserve reports.
type StreamStats struct {
	CreatedAt   int64   `json:"created_at"`
	DocTimeMin  int64   `json:"doc_time_min"`
	DocTimeMax  int64   `json:"doc_time_max"`
	DocNum      int64   `json:"doc_num"`
	FileNum     int64   `json:"file_num"`
	StorageSize float64 `json:"storage_size"`
}

// Schema is one column of a stream.
type Schema struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// APIError is the error body OpenObserve returns for failed requests.
type APIError struct {
	Code        int    `json:"code"`
	Message     string `json:"message"`
	ErrorDetail string `json:"error_detail"`
	Hint        string `json:"hint"`
	TraceID     string `json:"trace_id"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	if e.Message != "" {
		b.WriteString(e.Message)
	}
	if e.ErrorDetail != "" && e.ErrorDetail != e.Message {
		if b.Len() > 0 {
			b.WriteString(": ")
		}
		b.WriteString(e.ErrorDetail)
	}
	if e.Hint != "" {
		if b.Len() > 0 {
			b.WriteString(" (")
			b.WriteString(e.Hint)
			b.WriteString(")")
		} else {
			b.WriteString(e.Hint)
		}
	}
	if b.Len() == 0 {
		if e.Code != 0 {
			return fmt.Sprintf("openobserve: request failed with code %d", e.Code)
		}
		return "openobserve: request failed"
	}
	return b.String()
}

// HTTPError carries a non-2xx status plus the decoded API error, if any.
type HTTPError struct {
	StatusCode int
	Body       string
	API        *APIError
	// Hint replaces the default message when set. It is used to turn the raw
	// upstream body into an actionable sentence (bad credentials, unknown
	// organization, ...) while keeping the status code introspectable with
	// errors.As.
	Hint string
}

func (e *HTTPError) Error() string {
	if e.Hint != "" {
		return e.Hint
	}
	if e.API != nil && e.API.Error() != "" {
		return fmt.Sprintf("openobserve: HTTP %d: %s", e.StatusCode, e.API.Error())
	}
	body := strings.TrimSpace(e.Body)
	if len(body) > 400 {
		body = body[:400] + "..."
	}
	if body == "" {
		return fmt.Sprintf("openobserve: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("openobserve: HTTP %d: %s", e.StatusCode, body)
}

// ---- Prometheus compatible API -------------------------------------------

// PromResponse is the envelope of OpenObserve's Prometheus compatible API.
type PromResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings"`
}

// PromData is the query_range/query payload.
type PromData struct {
	ResultType string       `json:"resultType"`
	Result     []PromSeries `json:"result"`
}

// PromSeries is one time series of a matrix/vector response.
type PromSeries struct {
	Metric map[string]string `json:"metric"`
	// Values is used by matrix results: [[unixSeconds, "value"], ...]
	Values [][]json.RawMessage `json:"values"`
	// Value is used by vector results: [unixSeconds, "value"]
	Value []json.RawMessage `json:"value"`
}

// PromLabelValues is the label values payload.
type PromLabelValues []string
