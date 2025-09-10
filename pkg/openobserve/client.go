package openobserve

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// OpenObserveClient is a client for interacting with the OpenObserve API
type OpenObserveClient struct {
	BaseUrl    string
	username   string
	password   string
	httpClient *http.Client
}

// NewOpenObserveClient creates a new OpenObserve client with the given base URL, username, and password
func NewOpenObserveClient(baseUrl, username, password string) *OpenObserveClient {
	return &OpenObserveClient{
		BaseUrl:  baseUrl,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Set request timeout to 60 seconds
		},
	}
}

// Search performs a search request to the OpenObserve API
func (c *OpenObserveClient) Search(searchReqParam *SearchRequestParam, searchReqBody *SearchRequestBody) (*SearchResponse, error) {

	// handle SSE request
	if searchReqParam.EnableSSE {
		req, err := c.newSSESearchRequest(searchReqParam, searchReqBody)
		if err != nil {
			return nil, err
		}

		log.DefaultLogger.Debug("http SSE request created", "request", req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		return handleSSEResponse(resp)
	}

	// handle regular HTTP request
	req, err := c.newSearchRequest(searchReqParam, searchReqBody)
	if err != nil {
		return nil, err
	}

	log.DefaultLogger.Debug("http request created", "request", req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return handleRegularResponse(resp)
}

func handleRegularResponse(resp *http.Response) (*SearchResponse, error) {

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http response status code: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var searchResponse SearchResponse
	decoder := sonic.ConfigDefault.NewDecoder(resp.Body)
	if err := decoder.Decode(&searchResponse); err != nil {
		return nil, err
	}
	log.DefaultLogger.Debug("Regular", "len(searchResponse.Hits)", len(searchResponse.Hits))
	return &searchResponse, nil
}

func handleSSEResponse(resp *http.Response) (*SearchResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http response status code: %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if strings.HasPrefix(line, "event: search_response_hits") {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			hits := bytes.TrimPrefix(line, []byte("data: "))
			var searchResponse SearchResponse
			decoder := sonic.ConfigDefault.NewDecoder(bytes.NewBuffer(hits))
			if err := decoder.Decode(&searchResponse); err != nil {
				return nil, err
			}
			log.DefaultLogger.Debug("SSE", "len(searchResponse.Hits)", len(searchResponse.Hits))
			return &searchResponse, nil
		}
	}
	return nil, fmt.Errorf("no search_response_hits found in SSE response")
}

// newSearchRequest creates a new HTTP request for the search operation
func (c *OpenObserveClient) newSearchRequest(searchReqParam *SearchRequestParam, searchReqBody *SearchRequestBody) (*http.Request, error) {
	log.DefaultLogger.Debug("newSearchRequest called", "searchReqParam", searchReqParam, "searchReqBody", searchReqBody)
	searchReqBodyBytes, err := sonic.Marshal(searchReqBody)
	if err != nil {
		return nil, err
	}

	// construct the search URL
	searchUrl := fmt.Sprintf("%s/api/%s/_search", c.BaseUrl, searchReqParam.Organization)

	// create a new HTTP POST request with basic info
	req, err := http.NewRequest(http.MethodPost, searchUrl, bytes.NewBuffer(searchReqBodyBytes))
	if err != nil {
		return nil, err
	}

	// set query parameters
	q := req.URL.Query()
	q.Set("search_type", searchReqParam.SearchType)
	q.Set("type", searchReqParam.StreamType)
	q.Set("use_cache", fmt.Sprintf("%t", searchReqParam.UseCache))

	req.URL.RawQuery = q.Encode()

	// set http headers
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.username, c.password)
	// req.Header.Set("Authorization", fmt.Sprintf("Basic %s", c.BasicAuth))

	return req, nil
}

// newSearchRequest creates a new HTTP request for the search operation
func (c *OpenObserveClient) newSSESearchRequest(searchReqParam *SearchRequestParam, searchReqBody *SearchRequestBody) (*http.Request, error) {
	log.DefaultLogger.Debug("newSSESearchRequest called", "searchReqParam", searchReqParam, "searchReqBody", searchReqBody)
	searchReqBodyBytes, err := sonic.Marshal(searchReqBody)
	if err != nil {
		return nil, err
	}

	// construct the SSE search URL
	searchUrl := fmt.Sprintf("%s/api/%s/_search_stream", c.BaseUrl, searchReqParam.Organization)

	// create a new HTTP POST request with basic info
	req, err := http.NewRequest(http.MethodPost, searchUrl, bytes.NewBuffer(searchReqBodyBytes))
	if err != nil {
		return nil, err
	}

	// set query parameters
	q := req.URL.Query()
	q.Set("search_type", searchReqParam.SearchType)
	q.Set("type", searchReqParam.StreamType)
	q.Set("use_cache", fmt.Sprintf("%t", searchReqParam.UseCache))

	req.URL.RawQuery = q.Encode()

	// set http headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.SetBasicAuth(c.username, c.password)

	return req, nil
}

// HealthCheck checks the health of the OpenObserve cluster
func (c *OpenObserveClient) HealthCheck() error {
	clusterUrl := fmt.Sprintf("%s/api/clusters", c.BaseUrl)

	req, err := http.NewRequest(http.MethodGet, clusterUrl, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d:%s", resp.StatusCode, resp.Status)
	}

	return nil
}

// HandleListStreams handles the HTTP request to list streams
func (c *OpenObserveClient) HandleListStreams(rw http.ResponseWriter, req *http.Request) {
	log.DefaultLogger.Debug("HandleListStreams called")

	// initialize with the default values
	listRequestParam := &ListStreamRequestParam{
		Organization: "default",
		StreamType:   "logs",
		SortBy:       "name",
		Ascending:    true,
	}

	// override with query parameters if provided
	if organization := req.URL.Query().Get("organization"); organization != "" {
		listRequestParam.Organization = organization
	}
	if streamType := req.URL.Query().Get("type"); streamType != "" {
		listRequestParam.StreamType = streamType
	}

	listStreamResp, err := c.ListStreams(listRequestParam)
	if err != nil {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte(fmt.Sprintf(`{"error": "failed to list streams: %s"}`, err.Error())))
		return
	}

	streamMap := make(map[string][]string, len(listStreamResp.List)) // stream --> []fields
	for _, streamItem := range listStreamResp.List {
		colunns := make([]string, 0, len(streamItem.Schema))
		for _, column := range streamItem.Schema {
			colunns = append(colunns, column.Name)
		}
		streamMap[streamItem.Name] = colunns
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if err := sonic.ConfigDefault.NewEncoder(rw).Encode(streamMap); err != nil {
		rw.Write([]byte(fmt.Sprintf(`{"error": "failed to encode stream map: %s"}`, err.Error())))
		return
	}
}

// ListStreams lists the streams information in the OpenObserve cluster
func (c *OpenObserveClient) ListStreams(listStreamReqParam *ListStreamRequestParam) (*ListStreamResponse, error) {
	// Implement the logic to list streams here.
	// construct the search URL
	listStreamUrl := fmt.Sprintf("%s/api/%s/streams", c.BaseUrl, listStreamReqParam.Organization)
	req, err := http.NewRequest(http.MethodGet, listStreamUrl, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("type", listStreamReqParam.StreamType)
	q.Set("sort", listStreamReqParam.SortBy)
	q.Set("asc", fmt.Sprintf("%t", listStreamReqParam.Ascending))
	q.Set("fetchSchema", "true") // always fetch schema information

	req.URL.RawQuery = q.Encode()

	// set http headers
	// req.Header.Set("Accept", "application/json")
	// req.Header.Set("Authorization", fmt.Sprintf("Basic %s", c.BasicAuth))
	req.SetBasicAuth(c.username, c.password)

	// fmt.Printf("req.URL.String(): %v\n", req.URL.String())
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http response status code: %d", resp.StatusCode)
	}

	var listStreamResponse ListStreamResponse
	decoder := sonic.ConfigDefault.NewDecoder(resp.Body)
	if err := decoder.Decode(&listStreamResponse); err != nil {
		return nil, err
	}

	return &listStreamResponse, nil
}
