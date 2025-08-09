package test

import (
	"os"
	"testing"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/openobserve"
	"github.com/bytedance/sonic"
)

func BenchmarkTransformer_Transform_SonicJson(b *testing.B) {
	b.ResetTimer()

	var sql = "select * from \"log_stream\""
	for i := 0; i < b.N; i++ {
		f, err := os.Open("./log.json")
		if err != nil {
			b.Fatal(err)
		}
		defer f.Close()
		tr := &openobserve.Transformer{}
		parsedSql, err := openobserve.NewSqlParser().ParseSql(sql)
		if err != nil {
			b.Fatal(err)
		}

		// decoder := sonic.NewDecoder(f)
		decoder := sonic.ConfigDefault.NewDecoder(f)
		searchResp := &openobserve.SearchResponse{}
		if err := decoder.Decode(searchResp); err != nil {
			b.Fatal(err)
		}
		_, err = tr.TransformStream(parsedSql, searchResp)
		if err != nil {
			b.Fatal(err)
		}
	}
}
