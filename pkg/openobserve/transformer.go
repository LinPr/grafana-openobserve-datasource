package openobserve

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/bytedance/sonic/encoder"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type Item struct {
	TimeStamp int64           `json:"timestamp"`
	Body      string          `json:"body"`
	RawLabels json.RawMessage `json:"raw_labels"` // Labels are optional, so we can omit them if not needed
}

type ParsedSearchResult struct {
	Items []Item `json:"items"`
}

type TableResult struct {
	Headers []string         `json:"headers"`
	Table   map[string][]any `json:"table"`
}

type Transformer struct {
}

func NewTransformer() *Transformer {
	return &Transformer{}
}

// TransformsStream transforms the OpenObserve search stream response into Grafana data frame
func (t *Transformer) TransformStream(parsedSql *SQL, searchResponse *SearchResponse) (*data.Frame, error) {
	if parsedSql.selectMode == SqlSelectALlColumns {
		parsedSearchResult, err := parseSearchResponse(searchResponse)
		if err != nil {
			return nil, err
		}

		return buildLogModeDataFrame(parsedSearchResult)
	}

	tableResult, err := parseSearchResponseToTable(parsedSql.selectColumns, searchResponse)
	if err != nil {
		return nil, err
	}
	return buildGraphModeDataFrame(tableResult)
}

// TransformFallbackDisplayTables transforms the OpenObserve list streams response into Grafana data frame
// This is used when the user selects a stream from the dropdown in the query editor
func (t *Transformer) TransformFallbackDisplayTables(listStreamResp *ListStreamResponse) (*data.Frame, error) {
	streamSlice := make([]string, len(listStreamResp.List)) // stream --> []fields
	for _, streamItem := range listStreamResp.List {
		streamSlice = append(streamSlice, streamItem.Name)
	}
	sort.StringSlice(streamSlice).Sort()
	frame := data.NewFrame("openobserve_data_frame")
	// frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	frame.Fields = append(frame.Fields, data.NewField("stream", nil, streamSlice))
	return frame, nil
}

// TransformFallbackSelectFrom transforms the OpenObserve search response into Grafana data frame
// This is used when the user query a specific stream with select <columns> from SQL syntax
func (t *Transformer) TransformFallbackSelectFrom(parsedSql *SQL, searchResponse *SearchResponse) (*data.Frame, error) {
	tableResult, err := parseSearchResponseToTable(parsedSql.selectColumns, searchResponse)
	if err != nil {
		return nil, err
	}
	return buildGraphModeDataFrame(tableResult)
}

func parseSearchResponse(searchResponse *SearchResponse) (*ParsedSearchResult, error) {
	Items := make([]Item, 0, len(searchResponse.Hits))

	for _, hit := range searchResponse.Hits {
		var timestamp int64
		if ts, ok := hit["_timestamp"]; ok {
			if tsFloat, ok := ts.(float64); ok {
				timestamp = int64(tsFloat)
			}
		}

		body, err := encoder.Encode(hit, encoder.SortMapKeys) // convert hit to JSON string for labels
		if err != nil {
			return nil, err
		}

		labels, err := encoder.Encode(hit, encoder.SortMapKeys) // convert hit to JSON string for labels
		if err != nil {
			return nil, err
		}

		Items = append(Items, Item{
			TimeStamp: timestamp,
			Body:      *(*string)(unsafe.Pointer(&body)), // zero-copy conversion from []byte to string
			RawLabels: labels,
		})
	}

	sort.Slice(Items, func(i, j int) bool {
		return Items[i].TimeStamp > Items[j].TimeStamp
	})

	return &ParsedSearchResult{
		Items: Items,
	}, nil
}

func parseSearchResponseToTable(columns []string, searchResponse *SearchResponse) (*TableResult, error) {
	// build data table
	table := make(map[string][]any, len(columns))
	for _, column := range columns {
		table[column] = make([]any, 0, len(searchResponse.Hits))

	}

	// fill the table with data from searchResponse.hit
	for i, hit := range searchResponse.Hits {
		for key, value := range hit {
			table[key] = append(table[key], value)
		}

		currentColumnLenth := i + 1
		//fill missing columns cell with corresponding zero values for compatibility
		for _, column := range columns {
			for len(table[column]) < currentColumnLenth {
				if len(table[column]) == 0 {
					table[column] = append(table[column], "") // put a placeholder
				} else {
					columnElemType := reflect.TypeOf(table[column][i-1])
					zeroValue := reflect.Zero(columnElemType).Interface()
					table[column] = append(table[column], zeroValue)
				}
			}
		}

		// replace placeholder with zero values for the current column
		// to ensure each column has the same and consistent element type
		for _, column := range columns {
			columnElemType := reflect.TypeOf(table[column][currentColumnLenth-1])
			for j := currentColumnLenth - 1; j >= 0; j-- {
				if reflect.TypeOf(table[column][j]) != columnElemType {
					table[column][j] = reflect.Zero(columnElemType).Interface()
				}
			}
		}
	}

	return &TableResult{
		Headers: columns,
		Table:   table,
	}, nil
}

func buildLogModeDataFrame(parsedSearchResult *ParsedSearchResult) (*data.Frame, error) {
	frame := data.NewFrame("openobserve_data_frame")
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeLogs}

	timestampFields := make([]time.Time, 0, len(parsedSearchResult.Items))
	bodyFields := make([]string, 0, len(parsedSearchResult.Items))
	labelFields := make([]json.RawMessage, 0, len(parsedSearchResult.Items))
	for _, Item := range parsedSearchResult.Items {
		timestampFields = append(timestampFields, time.UnixMicro(Item.TimeStamp))
		bodyFields = append(bodyFields, Item.Body)
		labelFields = append(labelFields, Item.RawLabels)
	}

	frame.Fields = append(frame.Fields, data.NewField("time", nil, timestampFields))
	frame.Fields = append(frame.Fields, data.NewField("body", nil, bodyFields))
	frame.Fields = append(frame.Fields, data.NewField("labels", nil, labelFields))
	return frame, nil
}

func buildGraphModeDataFrame(tableResult *TableResult) (*data.Frame, error) {

	frame := data.NewFrame("openobserve_data_frame")
	for _, header := range tableResult.Headers {
		if len(tableResult.Table[header]) == 0 {
			continue
		}
		columnElemType := reflect.TypeOf(tableResult.Table[header][0])

		// Special handling for timestamp fields: convert to int64 timestamp format required by Grafana
		if strings.Contains(header, "gf_time") {
			timestampVec := make([]time.Time, 0, len(tableResult.Table[header]))
			for _, v := range tableResult.Table[header] {
				switch v := v.(type) {
				case string:
					// time.DateTime
					timestamp, err := time.Parse("2006-01-02T15:04:05", v) // original time format is RFC3339
					if err != nil {
						return nil, err
					}
					timestampVec = append(timestampVec, time.UnixMilli(timestamp.UnixMilli()))
				case float64:
					timestampVec = append(timestampVec, time.UnixMilli(int64(v)/1000))
				}

			}
			frame.Fields = append(frame.Fields, data.NewField(header, nil, timestampVec))
			continue
		}

		// Get the type and create a slice of that type
		sliceType := reflect.SliceOf(columnElemType)
		columnSlice := reflect.MakeSlice(sliceType, 0, len(tableResult.Table[header]))

		// Fill the slice with values
		for _, v := range tableResult.Table[header] {
			columnSlice = reflect.Append(columnSlice, reflect.ValueOf(v))
		}

		// Convert to interface{} for data.NewField
		frame.Fields = append(frame.Fields, data.NewField(header, nil, columnSlice.Interface()))
	}
	return frame, nil
}
