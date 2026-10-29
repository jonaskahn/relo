// Settings DTOs: the operator-facing shapes the management API serves.
package server

import (
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
)

type connectionsSettingsResponse struct {
	FreeEnabled bool `json:"free_enabled"`
}
type connectionsSettingsRequest struct {
	FreeEnabled *bool `json:"free_enabled"`
}

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
	LogTraffic      bool   `json:"log_traffic"`
	UpdatesURL      string `json:"updates_url"`
	UpdatesDownload string `json:"updates_download"`
}

func toSystemSettingsResponse(system appsettings.SystemSettings) systemSettingsResponse {
	return systemSettingsResponse{
		Autostart: system.Autostart, LogLevel: system.LogLevel, LogTraffic: system.LogTraffic,
		UpdatesURL: system.UpdatesURL, UpdatesDownload: system.UpdatesDownload,
	}
}

func toSystemSettings(request systemSettingsRequest) appsettings.SystemSettings {
	return appsettings.SystemSettings{
		Autostart: request.Autostart, LogLevel: request.LogLevel, LogTraffic: request.LogTraffic,
		UpdatesURL: request.UpdatesURL, UpdatesDownload: request.UpdatesDownload,
	}
}

type systemSettingsRequest struct {
	Autostart       bool   `json:"autostart"`
	LogLevel        string `json:"log_level"`
	LogTraffic      bool   `json:"log_traffic"`
	UpdatesURL      string `json:"updates_url"`
	UpdatesDownload string `json:"updates_download"`
}

type serverSettingsResponse struct {
	Bind string `json:"bind"`
	Port int    `json:"port"`
}

func toServerSettingsResponse(server appsettings.ServerSettings) serverSettingsResponse {
	return serverSettingsResponse{
		Bind: server.Bind, Port: server.Port,
	}
}

func toServerSettings(request serverSettingsRequest) appsettings.ServerSettings {
	return appsettings.ServerSettings{
		Bind: request.Bind, Port: request.Port,
	}
}

type serverSettingsRequest struct {
	Bind string `json:"bind"`
	Port int    `json:"port"`
}

type providersSettingsResponse struct {
	CatalogURL              string `json:"catalog_url"`
	ProxyURL                string `json:"proxy_url"`
	TimeoutSeconds          int    `json:"timeout_seconds"`
	RetryAttempts           int    `json:"retry_attempts"`
	FailoverCooldownSeconds int    `json:"failover_cooldown_seconds"`
}

func toProvidersSettingsResponse(providers appsettings.ProvidersSettings) providersSettingsResponse {
	return providersSettingsResponse{
		CatalogURL: providers.CatalogURL, ProxyURL: providers.ProxyURL,
		TimeoutSeconds: providers.TimeoutSeconds, RetryAttempts: providers.RetryAttempts,
		FailoverCooldownSeconds: providers.FailoverCooldownSeconds,
	}
}

func toProvidersSettings(request providersSettingsRequest) appsettings.ProvidersSettings {
	return appsettings.ProvidersSettings{
		CatalogURL: request.CatalogURL, ProxyURL: request.ProxyURL,
		TimeoutSeconds: request.TimeoutSeconds, RetryAttempts: request.RetryAttempts,
		FailoverCooldownSeconds: request.FailoverCooldownSeconds,
	}
}

type providersSettingsRequest struct {
	CatalogURL              string `json:"catalog_url"`
	ProxyURL                string `json:"proxy_url"`
	TimeoutSeconds          int    `json:"timeout_seconds"`
	RetryAttempts           int    `json:"retry_attempts"`
	FailoverCooldownSeconds int    `json:"failover_cooldown_seconds"`
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
	Connections    connectionsSettingsResponse `json:"connections"`
	Retention      retentionSettingsResponse   `json:"retention"`
	Language       string                      `json:"language"`
	Appearance     appearanceSettingsResponse  `json:"appearance"`
	System         systemSettingsResponse      `json:"system"`
	Server         serverSettingsResponse      `json:"server"`
	Providers      providersSettingsResponse   `json:"providers"`
	Access         accessSettingsResponse      `json:"access"`
	Secrets        secretsSettingsResponse     `json:"secrets"`
	RestartPending bool                        `json:"restart_pending"`
}

func toSettingsResponse(settings appsettings.Settings) settingsResponse {
	return settingsResponse{
		Connections:    connectionsSettingsResponse{FreeEnabled: settings.Connections.FreeEnabled},
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
	AutoRefresh   bool     `json:"auto_refresh"`
	ToolCutoff    bool     `json:"tool_cutoff"`
	ModelPrefixes []string `json:"model_prefixes,omitempty"`
}

func toTemplateSettingResponse(setting apptemplates.Setting) templateSettingResponse {
	return templateSettingResponse{
		AutoRefresh: setting.AutoRefresh, ToolCutoff: setting.ToolCutoff,
		ModelPrefixes: setting.ModelPrefixes,
	}
}

type templateSettingPatchRequest struct {
	AutoRefresh   *bool     `json:"auto_refresh,omitempty"`
	ToolCutoff    *bool     `json:"tool_cutoff,omitempty"`
	ModelPrefixes *[]string `json:"model_prefixes,omitempty"`
}

func toTemplateSettingPatch(request templateSettingPatchRequest) apptemplates.Patch {
	return apptemplates.Patch{
		AutoRefresh: request.AutoRefresh, ToolCutoff: request.ToolCutoff,
		ModelPrefixes: request.ModelPrefixes,
	}
}
