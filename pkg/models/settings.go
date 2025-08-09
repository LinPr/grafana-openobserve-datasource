package models

import (
	"encoding/json"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type PluginSettings struct {
	Url                     string                   `json:"url"`
	Username                string                   `json:"username"`
	JsonData                *JsonData                `json:"-"`
	DecryptedSecureJSONData *DecryptedSecureJSONData `json:"-"`
}

type JsonData struct {
}

type DecryptedSecureJSONData struct {
	Password string `json:"password"`
}

func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := PluginSettings{
		Url:                     source.URL,
		Username:                source.User,
		JsonData:                &JsonData{},
		DecryptedSecureJSONData: &DecryptedSecureJSONData{},
	}

	if source.JSONData != nil {
		if err := json.Unmarshal(source.JSONData, settings.JsonData); err != nil {
			return nil, err
		}
	}
	if source.DecryptedSecureJSONData != nil {
		settings.DecryptedSecureJSONData = loadSecretPluginSettings(source.DecryptedSecureJSONData)
	}

	return &settings, nil
}

func loadSecretPluginSettings(source map[string]string) *DecryptedSecureJSONData {
	return &DecryptedSecureJSONData{
		Password: source["password"],
	}
}
