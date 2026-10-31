package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LinPr/grafana-openobserve-datasource/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func TestLoadPluginSettingsDefaults(t *testing.T) {
	settings, err := models.LoadPluginSettings(backend.DataSourceInstanceSettings{
		URL: "http://localhost:5080/",
	})
	if err != nil {
		t.Fatalf("LoadPluginSettings: %v", err)
	}
	if settings.URL != "http://localhost:5080" {
		t.Errorf("url = %q, want the trailing slash trimmed", settings.URL)
	}
	if got := settings.Organization(); got != models.DefaultOrganization {
		t.Errorf("organization = %q, want %q", got, models.DefaultOrganization)
	}
	if got := settings.JsonData.AuthMethod; got != models.AuthMethodBasic {
		t.Errorf("auth method = %q, want basic", got)
	}
	if got := settings.JsonData.Timeout(); got != 60*time.Second {
		t.Errorf("timeout = %v, want 60s", got)
	}
	if got := settings.JsonData.DefaultLimit; got != models.DefaultLimit {
		t.Errorf("default limit = %d, want %d", got, models.DefaultLimit)
	}
	if got := settings.JsonData.MaxLimit; got != models.DefaultMaxLimit {
		t.Errorf("max limit = %d, want %d", got, models.DefaultMaxLimit)
	}
	if !settings.JsonData.PrometheusEnabled() {
		t.Error("PromQL should be enabled by default")
	}
}

func TestLoadPluginSettingsLegacyDatabaseKey(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"database": "my-org"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := models.LoadPluginSettings(backend.DataSourceInstanceSettings{
		URL:      "http://localhost:5080",
		JSONData: raw,
	})
	if err != nil {
		t.Fatalf("LoadPluginSettings: %v", err)
	}
	if got := settings.Organization(); got != "my-org" {
		t.Errorf("organization = %q, want my-org (legacy database key)", got)
	}
}

func TestLoadPluginSettingsSecureToken(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"authMethod": "token", "token": "json-token"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := models.LoadPluginSettings(backend.DataSourceInstanceSettings{
		URL:                     "https://openobserve.example.com",
		JSONData:                raw,
		DecryptedSecureJSONData: map[string]string{"token": "secure-token"},
	})
	if err != nil {
		t.Fatalf("LoadPluginSettings: %v", err)
	}
	if got := settings.JsonData.Token; got != "secure-token" {
		t.Errorf("token = %q, want the secureJsonData token to win", got)
	}
}

func TestPrometheusDisabled(t *testing.T) {
	disabled := false
	j := &models.JsonData{EnablePrometheus: &disabled}
	if j.PrometheusEnabled() {
		t.Error("PromQL should be disabled")
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"  http://a:5080/  ": "http://a:5080",
		"http://a:5080":      "http://a:5080",
		"":                   "",
	}
	for in, want := range cases {
		if got := models.NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}
