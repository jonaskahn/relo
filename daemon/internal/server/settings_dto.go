// Settings DTOs: the operator-facing shapes the management API serves.
package server

import (
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
)

type retentionSettingsResponse struct {
	UsageDays int   `json:"usage_days"`
	MaxEvents int64 `json:"max_events"`
	MaxBytes  int64 `json:"max_bytes"`
}

func toRetentionSettingsResponse(budget appsettings.RetentionSettings) retentionSettingsResponse {
	return retentionSettingsResponse{
		UsageDays: budget.UsageDays, MaxEvents: budget.MaxEvents,
		MaxBytes: budget.MaxBytes,
	}
}

func toRetentionSettings(request retentionSettingsRequest) appsettings.RetentionSettings {
	return appsettings.RetentionSettings{
		UsageDays: request.UsageDays, MaxEvents: request.MaxEvents,
		MaxBytes: request.MaxBytes,
	}
}

type retentionSettingsRequest struct {
	UsageDays int   `json:"usage_days"`
	MaxEvents int64 `json:"max_events"`
	MaxBytes  int64 `json:"max_bytes"`
}

type systemSettingsResponse struct {
	Autostart       bool   `json:"autostart"`
	LogLevel        string `json:"log_level"`
	UpdatesURL      string `json:"updates_url"`
	UpdatesDownload string `json:"updates_download"`
}

func toSystemSettingsResponse(system appsettings.SystemSettings) systemSettingsResponse {
	return systemSettingsResponse{
		Autostart: system.Autostart, LogLevel: system.LogLevel,
		UpdatesURL: system.UpdatesURL, UpdatesDownload: system.UpdatesDownload,
	}
}

func toSystemSettings(request systemSettingsRequest) appsettings.SystemSettings {
	return appsettings.SystemSettings{
		Autostart: request.Autostart, LogLevel: request.LogLevel,
		UpdatesURL: request.UpdatesURL, UpdatesDownload: request.UpdatesDownload,
	}
}

type systemSettingsRequest struct {
	Autostart       bool   `json:"autostart"`
	LogLevel        string `json:"log_level"`
	UpdatesURL      string `json:"updates_url"`
	UpdatesDownload string `json:"updates_download"`
}

type serverSettingsResponse struct {
	Bind          string `json:"bind"`
	Port          int    `json:"port"`
	OpenAIPort    int    `json:"openai_port"`
	AnthropicPort int    `json:"anthropic_port"`
	GeminiPort    int    `json:"gemini_port"`
}

func toServerSettingsResponse(server appsettings.ServerSettings) serverSettingsResponse {
	return serverSettingsResponse{
		Bind: server.Bind, Port: server.Port, OpenAIPort: server.OpenAIPort,
		AnthropicPort: server.AnthropicPort, GeminiPort: server.GeminiPort,
	}
}

func toServerSettings(request serverSettingsRequest) appsettings.ServerSettings {
	return appsettings.ServerSettings{
		Bind: request.Bind, Port: request.Port, OpenAIPort: request.OpenAIPort,
		AnthropicPort: request.AnthropicPort, GeminiPort: request.GeminiPort,
	}
}

type serverSettingsRequest struct {
	Bind          string `json:"bind"`
	Port          int    `json:"port"`
	OpenAIPort    int    `json:"openai_port"`
	AnthropicPort int    `json:"anthropic_port"`
	GeminiPort    int    `json:"gemini_port"`
}

type providersSettingsResponse struct {
	CatalogURL              string   `json:"catalog_url"`
	ProxyURL                string   `json:"proxy_url"`
	TimeoutSeconds          int      `json:"timeout_seconds"`
	RetryBackoff            [][2]int `json:"retry_backoff"`
	FailoverCooldownSeconds int      `json:"failover_cooldown_seconds"`
}

func toProvidersSettingsResponse(providers appsettings.ProvidersSettings) providersSettingsResponse {
	return providersSettingsResponse{
		CatalogURL: providers.CatalogURL, ProxyURL: providers.ProxyURL,
		TimeoutSeconds: providers.TimeoutSeconds, RetryBackoff: providers.RetryBackoff,
		FailoverCooldownSeconds: providers.FailoverCooldownSeconds,
	}
}

func toProvidersSettings(request providersSettingsRequest) appsettings.ProvidersSettings {
	return appsettings.ProvidersSettings{
		CatalogURL: request.CatalogURL, ProxyURL: request.ProxyURL,
		TimeoutSeconds: request.TimeoutSeconds, RetryBackoff: request.RetryBackoff,
		FailoverCooldownSeconds: request.FailoverCooldownSeconds,
	}
}

type providersSettingsRequest struct {
	CatalogURL              string   `json:"catalog_url"`
	ProxyURL                string   `json:"proxy_url"`
	TimeoutSeconds          int      `json:"timeout_seconds"`
	RetryBackoff            [][2]int `json:"retry_backoff"`
	FailoverCooldownSeconds int      `json:"failover_cooldown_seconds"`
}

type accessSettingsResponse struct {
	AllowExternal bool `json:"allow_external"`
	Login         bool `json:"login"`
}

func toAccessSettingsResponse(access appsettings.AccessSettings) accessSettingsResponse {
	return accessSettingsResponse{
		AllowExternal: access.AllowExternal, Login: access.Login,
	}
}

type secretsSettingsResponse struct {
	Keychain bool   `json:"keychain"`
	KeyFile  string `json:"key_file"`
}

func toSecretsSettingsResponse(secrets appsettings.SecretsSettings) secretsSettingsResponse {
	return secretsSettingsResponse{
		Keychain: secrets.Keychain, KeyFile: secrets.KeyFile,
	}
}

type appearanceSettingsResponse struct {
	Theme        string `json:"theme"`
	Accent       string `json:"accent"`
	QuotaDisplay string `json:"quota_display"`
}

func toAppearanceSettingsResponse(appearance appsettings.AppearanceSettings) appearanceSettingsResponse {
	return appearanceSettingsResponse{
		Theme: appearance.Theme, Accent: appearance.Accent,
		QuotaDisplay: appearance.QuotaDisplay,
	}
}

type appearancePatchRequest struct {
	Theme        *string `json:"theme,omitempty"`
	Accent       *string `json:"accent,omitempty"`
	QuotaDisplay *string `json:"quota_display,omitempty"`
}

func toAppearancePatch(request appearancePatchRequest) appsettings.AppearancePatch {
	return appsettings.AppearancePatch{
		Theme: request.Theme, Accent: request.Accent,
		QuotaDisplay: request.QuotaDisplay,
	}
}

type settingsResponse struct {
	Retention      retentionSettingsResponse  `json:"retention"`
	Language       string                     `json:"language"`
	Appearance     appearanceSettingsResponse `json:"appearance"`
	System         systemSettingsResponse     `json:"system"`
	Server         serverSettingsResponse     `json:"server"`
	Providers      providersSettingsResponse  `json:"providers"`
	Access         accessSettingsResponse     `json:"access"`
	Secrets        secretsSettingsResponse    `json:"secrets"`
	RestartPending bool                       `json:"restart_pending"`
}

func toSettingsResponse(settings appsettings.Settings) settingsResponse {
	return settingsResponse{
		Retention:      toRetentionSettingsResponse(settings.Retention),
		Language:       settings.Language,
		Appearance:     toAppearanceSettingsResponse(settings.Appearance),
		System:         toSystemSettingsResponse(settings.System),
		Server:         toServerSettingsResponse(settings.Server),
		Providers:      toProvidersSettingsResponse(settings.Providers),
		Access:         toAccessSettingsResponse(settings.Access),
		Secrets:        toSecretsSettingsResponse(settings.Secrets),
		RestartPending: settings.RestartPending,
	}
}

type templateSettingResponse struct {
	AutoRefresh bool `json:"auto_refresh"`
}

func toTemplateSettingResponse(setting apptemplates.Setting) templateSettingResponse {
	return templateSettingResponse{AutoRefresh: setting.AutoRefresh}
}

type templateSettingPatchRequest struct {
	AutoRefresh *bool `json:"auto_refresh,omitempty"`
}

func toTemplateSettingPatch(request templateSettingPatchRequest) apptemplates.Patch {
	return apptemplates.Patch{AutoRefresh: request.AutoRefresh}
}
