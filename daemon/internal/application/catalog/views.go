// Provider display shapes: the kinds, formats, variables, and login methods
// the catalog views read off templates and stored rows.
package catalog

import "github.com/jonaskahn/relo/internal/catalog"

func (s *Service) providerKind(host catalog.Provider) string {
	if t, found := s.templates.Curated(host.TemplateID); found && t.Kind != "" {
		return string(t.Kind)
	}
	if host.Origin == catalog.OriginCustom {
		return string(catalog.KindLocal)
	}
	return string(catalog.KindKey)
}

func (s *Service) providerFormats(host catalog.Provider) []catalog.FormatOption {
	if t, found := s.templates.Curated(host.TemplateID); found {
		if len(t.AvailableFormats) > 0 {
			return t.AvailableFormats
		}
		return []catalog.FormatOption{formatOptionOf(t)}
	}
	if host.Origin == catalog.OriginCustom {
		return []catalog.FormatOption{}
	}
	return []catalog.FormatOption{formatOptionOf(storedShape(host))}
}

func storedShape(host catalog.Provider) catalog.Template {
	return catalog.Template{
		ID: host.ID, Label: host.Label, Kind: catalog.KindKey,
		Origin: host.Origin, Auth: host.Auth,
		KeyHeader: host.KeyHeader, DefaultFormat: host.APIFormat,
		DefaultBaseURL: host.BaseURL, ModelsSource: host.ModelsSource,
		ModelsFormat: host.ModelsFormat, ModelsDevProviderID: host.ModelsDevProviderID,
		Headers: host.Headers,
	}
}

func formatOptionOf(t catalog.Template) catalog.FormatOption {
	return catalog.FormatOption{
		Format: t.DefaultFormat, DefaultBaseURL: t.DefaultBaseURL,
		KeyHeader: t.KeyHeader, ModelsFormat: t.ModelsFormat,
		Label: formatLabel(t.DefaultFormat),
	}
}

func formatLabel(format catalog.APIFormat) string {
	return string(format)
}

func (s *Service) providerVariables(host catalog.Provider) []catalog.Variable {
	if t, found := s.templates.Curated(host.TemplateID); found {
		if len(t.Variables) > 0 {
			return t.Variables
		}
	}
	// A row whose template is unknown still reports the names its base URL
	// asks for, so an operator can fill them in.
	names := host.NeedsSetup
	if len(names) == 0 {
		return []catalog.Variable{}
	}
	vars := make([]catalog.Variable, 0, len(names))
	for _, name := range names {
		vars = append(vars, catalog.Variable{Name: name, Label: name})
	}
	return vars
}

func (s *Service) providerLoginMethods(host catalog.Provider) []catalog.LoginMethod {
	if t, found := s.templates.Curated(host.TemplateID); found && len(t.LoginMethods) > 0 {
		methods := make([]catalog.LoginMethod, len(t.LoginMethods))
		copy(methods, t.LoginMethods)
		return methods
	}
	methods := make([]catalog.LoginMethod, 0, len(host.LoginFlows))
	for _, flow := range host.LoginFlows {
		methods = append(methods, catalog.LoginMethod{Flow: flow, Kind: catalog.LoginBrowser})
	}
	return methods
}
