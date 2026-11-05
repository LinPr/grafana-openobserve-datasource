package openobserve

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// jsonAPI is the shared JSON codec.
//
// It decodes numbers into json.Number instead of float64: OpenObserve returns
// `_timestamp` as 16-digit microseconds and `start_time` as 19-digit
// nanoseconds, neither of which survives a float64 round trip. Consumers use
// openobserve.ToTime / toInt64 / toFloat64, which all understand json.Number.
//
// The obvious candidate, bytedance/sonic with Config.UseInt64, does the same
// job but only compiles for 64-bit targets, and the release workflow builds
// linux/arm as well.
var jsonAPI = &stdJSON{}

// stdJSON adapts encoding/json to the small surface the client uses, keeping
// number precision via UseNumber.
type stdJSON struct{}

func (stdJSON) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (stdJSON) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func (stdJSON) NewDecoder(r io.Reader) *json.Decoder {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec
}

// ClientConfig is the connection configuration of one datasource instance.
type ClientConfig struct {
	BaseURL      string
	Organization string
	Username     string
	Password     string
	// AuthMethod is "basic" or "token".
	AuthMethod string
	// Token is sent verbatim in the Authorization header when AuthMethod is
	// "token".
	Token              string
	InsecureSkipVerify bool
	Timeout            time.Duration
}

func (c ClientConfig) authMethod() string {
	if c.AuthMethod == "" {
		return "basic"
	}
	return c.AuthMethod
}

// Client talks to one OpenObserve instance. It is safe for concurrent use.
type Client struct {
	cfg        ClientConfig
	baseURL    string
	httpClient *http.Client

	schemaMu    sync.RWMutex
	schemaCache map[string][]Schema
}

// NewClient validates the configuration and builds a client.
func NewClient(cfg ClientConfig) (*Client, error) {
	base := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("openobserve url is not configured")
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid openobserve url %q: %w", cfg.BaseURL, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid openobserve url %q: missing host", cfg.BaseURL)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	if cfg.Organization == "" {
		cfg.Organization = "default"
	}

	return &Client{
		cfg:         cfg,
		baseURL:     base,
		httpClient:  &http.Client{Transport: transport, Timeout: timeout},
		schemaCache: map[string][]Schema{},
	}, nil
}

// Config returns the client configuration.
func (c *Client) Config() ClientConfig { return c.cfg }

// Organization returns the configured organization.
func (c *Client) Organization() string { return c.cfg.Organization }

// ---- request plumbing -----------------------------------------------------

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
  req.Header.Set("User-Agent", "linpr-openobserve-datasource")
	c.applyAuth(req)
	return req, nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.cfg.authMethod() == "token" && c.cfg.Token != "" {
		req.Header.Set("Authorization", c.cfg.Token)
		return
	}
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
}

// do performs the request and decodes a successful JSON response into out.
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeHTTPError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := jsonAPI.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("openobserve: decode response: %w", err)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) postJSON(ctx context.Context, path string, body, out any) error {
	payload, err := jsonAPI.Marshal(body)
	if err != nil {
		return fmt.Errorf("openobserve: encode request: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func wrapTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errorsIsContext(err) {
		return err
	}
	return fmt.Errorf("openobserve: request failed: %w", err)
}

func errorsIsContext(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded"))
}

// decodeHTTPError reads an error body and converts it into *HTTPError.
func decodeHTTPError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	httpErr := &HTTPError{StatusCode: resp.StatusCode, Body: string(raw)}
	var apiErr APIError
	if len(raw) > 0 && jsonAPI.Unmarshal(raw, &apiErr) == nil && (apiErr.Message != "" || apiErr.Code != 0) {
		httpErr.API = &apiErr
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body := strings.TrimSpace(httpErr.Body)
		if body == "" {
			body = http.StatusText(resp.StatusCode)
		}
		httpErr.Hint = fmt.Sprintf(
			"openobserve: authentication failed (HTTP %d): %s -- check the configured user/password or token",
			resp.StatusCode, body)
	}
	return httpErr
}

// ---- search ---------------------------------------------------------------

// Search runs a search request. It transparently uses the streaming endpoint
// when requested and merges every hit batch into one response.
func (c *Client) Search(ctx context.Context, param *SearchRequestParam, body *SearchRequestBody) (*SearchResponse, error) {
	org := param.Organization
	if org == "" {
		org = c.cfg.Organization
	}
	streamType := param.StreamType
	if streamType == "" {
		streamType = LogsStream
	}

	query := url.Values{}
	if param.SearchType != "" {
		query.Set("search_type", param.SearchType)
	}
	query.Set("type", streamType)
	if param.EnableSSE {
		query.Set("use_cache", strconv.FormatBool(param.UseCache))
		return c.searchStream(ctx, org, query, body)
	}
	query.Set("use_cache", strconv.FormatBool(param.UseCache))

	payload, err := jsonAPI.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openobserve: encode search body: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/api/%s/_search?%s", org, query.Encode()), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var resp SearchResponse
	if err := c.do(req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) searchStream(ctx context.Context, org string, query url.Values, body *SearchRequestBody) (*SearchResponse, error) {
	payload, err := jsonAPI.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openobserve: encode search body: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, fmt.Sprintf("/api/%s/_search_stream?%s", org, query.Encode()), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, wrapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, decodeHTTPError(resp)
	}

	// Some deployments answer a _search_stream request with a plain JSON body
	// instead of an event stream. Sniff the first non-space byte so the caller
	// gets the rows instead of an empty result.
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	if head, err := reader.Peek(1); err == nil && head[0] == '{' {
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			var plain SearchResponse
			if err := jsonAPI.NewDecoder(reader).Decode(&plain); err != nil {
				return nil, fmt.Errorf("openobserve: decode response: %w", err)
			}
			return &plain, nil
		}
	}
	return readSearchStream(reader)
}

type sseEvent struct {
	name string
	data string
}

// readSearchStream consumes the SSE search stream and merges hits/metadata.
func readSearchStream(r io.Reader) (*SearchResponse, error) {
	var (
		result   SearchResponse
		pending  sseEvent
		totalSet bool
	)

	flush := func(ev sseEvent) error {
		if ev.data == "" || ev.data == "[[DONE]]" {
			return nil
		}
		switch ev.name {
		case "", "search_response_hits", "message":
			var part struct {
				Hits  []map[string]any `json:"hits"`
				Total *int             `json:"total"`
			}
			if err := jsonAPI.Unmarshal([]byte(ev.data), &part); err != nil {
				return fmt.Errorf("openobserve: decode stream chunk: %w", err)
			}
			result.Hits = append(result.Hits, part.Hits...)
			if part.Total != nil && *part.Total > result.Total {
				result.Total = *part.Total
				totalSet = true
			}
		case "search_response_metadata":
			var meta struct {
				Results SearchResponse `json:"results"`
			}
			if err := jsonAPI.Unmarshal([]byte(ev.data), &meta); err != nil {
				return fmt.Errorf("openobserve: decode stream metadata: %w", err)
			}
			if meta.Results.Total > result.Total {
				result.Total = meta.Results.Total
				totalSet = true
			}
			if result.TraceID == "" {
				result.TraceID = meta.Results.TraceID
			}
			if len(meta.Results.Histogram) > 0 {
				result.Histogram = meta.Results.Histogram
			}
		case "error":
			var apiErr APIError
			text := ev.data
			if err := jsonAPI.Unmarshal([]byte(ev.data), &apiErr); err == nil && apiErr.Error() != "" {
				text = apiErr.Error()
			}
			return fmt.Errorf("openobserve: stream error: %s", text)
		}
		return nil
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := flush(pending); err != nil {
				return nil, err
			}
			pending = sseEvent{}
		case strings.HasPrefix(line, "event:"):
			pending.name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			chunk := strings.TrimPrefix(line, "data:")
			chunk = strings.TrimPrefix(chunk, " ")
			if pending.data != "" {
				pending.data += "\n"
			}
			pending.data += chunk
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, wrapTransportError(err)
	}
	if err := flush(pending); err != nil {
		return nil, err
	}
	if !totalSet && result.Total == 0 {
		result.Total = len(result.Hits)
	}
	return &result, nil
}

// ---- streams --------------------------------------------------------------

// ListStreams returns every stream of the given type in the organization.
func (c *Client) ListStreams(ctx context.Context, org, streamType string) (*ListStreamResponse, error) {
	if org == "" {
		org = c.cfg.Organization
	}
	if streamType == "" {
		streamType = LogsStream
	}
	query := url.Values{}
	query.Set("type", streamType)
	query.Set("sort", "name")
	query.Set("asc", "true")
	query.Set("fetchSchema", "true")

	var resp ListStreamResponse
	if err := c.getJSON(ctx, fmt.Sprintf("/api/%s/streams?%s", org, query.Encode()), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StreamSchema returns the column schema of one stream. Results are cached per
// (org, stream, type) because the schema rarely changes and the query editor
// asks for it constantly.
func (c *Client) StreamSchema(ctx context.Context, org, stream, streamType string) ([]Schema, error) {
	if org == "" {
		org = c.cfg.Organization
	}
	if streamType == "" {
		streamType = LogsStream
	}
	key := org + "|" + streamType + "|" + stream
	c.schemaMu.RLock()
	cached, ok := c.schemaCache[key]
	c.schemaMu.RUnlock()
	if ok {
		return cached, nil
	}

	var resp StreamInfo
	query := url.Values{}
	query.Set("type", streamType)
	if err := c.getJSON(ctx, fmt.Sprintf("/api/%s/streams/%s/schema?%s", org, url.PathEscape(stream), query.Encode()), &resp); err != nil {
		return nil, err
	}
	c.schemaMu.Lock()
	c.schemaCache[key] = resp.Schema
	c.schemaMu.Unlock()
	return resp.Schema, nil
}

// InvalidateSchema drops the cached schema, e.g. after a datasource update.
func (c *Client) InvalidateSchema() {
	c.schemaMu.Lock()
	c.schemaCache = map[string][]Schema{}
	c.schemaMu.Unlock()
}

// ---- health ---------------------------------------------------------------

// Health verifies connectivity and credentials with a cheap request.
func (c *Client) Health(ctx context.Context) error {
	var resp ListStreamResponse
	query := url.Values{}
	query.Set("type", LogsStream)
	query.Set("limit", "1")
	if err := c.getJSON(ctx, fmt.Sprintf("/api/%s/streams?%s", c.cfg.Organization, query.Encode()), &resp); err != nil {
		return err
	}
	return nil
}

// HealthDetail returns a human readable summary of the cluster, used by the
// datasource health check so users can see which org/streams they reached.
func (c *Client) HealthDetail(ctx context.Context) (string, error) {
	if err := c.Health(ctx); err != nil {
		return "", err
	}
	streams, err := c.ListStreams(ctx, c.cfg.Organization, LogsStream)
	if err != nil {
		return fmt.Sprintf("connected to organization %q", c.cfg.Organization), nil
	}
	return fmt.Sprintf("connected to OpenObserve organization %q (%d log streams)", c.cfg.Organization, len(streams.List)), nil
}
