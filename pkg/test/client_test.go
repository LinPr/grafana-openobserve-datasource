package test

// import (
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"testing"

// 	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
// )

// func TestOpenObserveClient_listStreams(t *testing.T) {
// 	type fields struct {
// 		BaseUrl    string
// 		Username   string
// 		Password   string
// 		httpClient *http.Client
// 	}
// 	type args struct {
// 		listStreamReqParam *openobserve.ListStreamRequestParam
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 		{
// 			name: "Test list streams",
// 			fields: fields{
// 				BaseUrl:    "https://openobserve.svc",
// 				Username:   "username",
// 				Password:   "**********",
// 				httpClient: &http.Client{},
// 			},
// 			args: args{
// 				listStreamReqParam: &openobserve.ListStreamRequestParam{
// 					Organization: "default",
// 				},
// 			},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			c := openobserve.NewOpenObserveClient(
// 				tt.fields.BaseUrl,
// 				tt.fields.Username,
// 				tt.fields.Password,
// 			)
// 			got, err := c.ListStreams(tt.args.listStreamReqParam)
// 			if err != nil {
// 				t.Errorf("OpenObserveClient.listStreams() error = %v", err)
// 			}
// 			j, _ := json.Marshal(got)
// 			fmt.Printf("j: %v\n", string(j))

// 		})
// 	}
// }
