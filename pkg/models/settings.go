package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const (
	// AuthMethodBasic uses HTTP basic auth (user:password), like the
	// OpenObserve UI password login.
	AuthMethodBasic = "basic"
	// AuthMethodToken sends an OpenObserve API/session token. The token is
	// sent verbatim in the Authorization header, so both "Basic <base64>"
	// and "Bearer <jwt>" work.
	AuthMethodToken = "token"

	DefaultOrganization = "default"
	DefaultTimeout      = 60 * time.Second
	// DefaultLimit matches the editor's default so a query without an explicit
	// limit behaves the same whether or not the frontend sent one.
	DefaultLimit    = 100
	DefaultMaxLimit = 10000
)

// PluginSettings is the parsed datasource configuration.
type PluginSettings struct {
	URL      string
	Username string
	Password string
	JsonData *JsonData
}

// JsonData mirrors the "jsonData" block of the datasource configuration.
//
// "database" is kept as a legacy alias of "organization" because earlier
// versions stored the OpenObserve organization under that key.
type JsonData struct {
	// Organization is the OpenObserve organization identifier.
	Organization string `json:"organization"`
	// Database is the legacy key for Organization.
	Database string `json:"database"`

	// AuthMethod is either "basic" (default) or "token".
	AuthMethod string `json:"authMethod"`
	// Token is used when AuthMethod is "token". Storing it in secureJsonData
	// is preferred; jsonData is accepted for backwards compatibility.
	Token string `json:"token"`

	// TLSInsecureSkipVerify disables TLS certificate verification, which is
	// common for self-hosted OpenObserve instances.
	TLSInsecureSkipVerify bool `json:"tlsSkipVerify"`

	// TimeoutSeconds caps a single query. Zero means the default.
	TimeoutSeconds int `json:"timeoutSeconds"`
	// DefaultLimit is the row count used when neither the query nor the
	// frontend specify one.
	DefaultLimit int64 `json:"defaultLimit"`
	// MaxLimit caps the row count that can be requested.
	MaxLimit int64 `json:"maxLimit"`

	// TimeField is the timestamp column used for time series / logs frames.
	// Empty means auto-detect ("_timestamp" first).
	TimeField string `json:"timeField"`
	// LogMessageField selects the column rendered as the log line. Empty
	// means auto-detect (body, message, msg, log, ...), then the whole row.
	LogMessageField string `json:"logMessageField"`
	// LogLevelField selects the column rendered as the log level.
	LogLevelField string `json:"logLevelField"`

	// EnablePrometheus toggles the PromQL query mode. OpenObserve exposes a
	// Prometheus compatible API at /api/{org}/prometheus.
	EnablePrometheus *bool `json:"enablePrometheus"`
}

// SecureJsonData holds secret values.
type SecureJsonData struct {
	Password string `json:"password"`
	Token    string `json:"token"`
}

// LoadPluginSettings parses the datasource instance settings.
func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := &PluginSettings{
		URL:      NormalizeURL(source.URL),
		Username: source.User,
		JsonData: &JsonData{},
	}

	if len(source.JSONData) > 0 {
		if err := json.Unmarshal(source.JSONData, settings.JsonData); err != nil {
			return nil, fmt.Errorf("invalid datasource jsonData: %w", err)
		}
	}
	if settings.JsonData == nil {
		settings.JsonData = &JsonData{}
	}

	secure := &SecureJsonData{}
	if len(source.DecryptedSecureJSONData) > 0 {
		if raw, err := json.Marshal(source.DecryptedSecureJSONData); err == nil {
			_ = json.Unmarshal(raw, secure)
		}
	}
	settings.Password = secure.Password
	if secure.Token != "" {
		settings.JsonData.Token = secure.Token
	}

	settings.JsonData.applyDefaults()
	return settings, nil
}

// Database is the organization the settings point at.
func (s *PluginSettings) Organization() string {
	if s == nil || s.JsonData == nil {
		return DefaultOrganization
	}
	return s.JsonData.Organization
}

func (j *JsonData) applyDefaults() {
	if j.Organization == "" {
		j.Organization = j.Database
	}
	if j.Organization == "" {
		j.Organization = DefaultOrganization
	}
	j.Database = j.Organization

	if j.AuthMethod == "" {
		j.AuthMethod = AuthMethodBasic
	}
	if j.TimeoutSeconds <= 0 {
		j.TimeoutSeconds = int(DefaultTimeout / time.Second)
	}
	if j.DefaultLimit <= 0 {
		j.DefaultLimit = DefaultLimit
	}
	if j.MaxLimit <= 0 || j.MaxLimit < j.DefaultLimit {
		j.MaxLimit = DefaultMaxLimit
	}
}

// Timeout returns the configured query timeout.
func (j *JsonData) Timeout() time.Duration {
	if j == nil || j.TimeoutSeconds <= 0 {
		return DefaultTimeout
	}
	return time.Duration(j.TimeoutSeconds) * time.Second
}

// PrometheusEnabled reports whether PromQL mode is available.
func (j *JsonData) PrometheusEnabled() bool {
	if j == nil {
		return true
	}
	return j.EnablePrometheus == nil || *j.EnablePrometheus
}

// NormalizeURL trims whitespace and trailing slashes from a datasource URL.
func NormalizeURL(raw string) string {
	return strings.TrimSuffix(strings.TrimSpace(raw), "/")
}
