// CLI localization: loading the operator's language.
package cli

import (
	"io"

	"github.com/spf13/pflag"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/platform"
)

const langFlag = "lang"

type ui struct {
	catalogs   *i18n.Catalogs
	language   string
	translator *i18n.Translator
}

func (u ui) text(id string, data map[string]any) string {
	return u.translator.Text(id, data)
}

func loadUI(home, language string) (ui, error) {
	resolution, err := platform.ResolveLanguage(home, language)
	if err != nil {
		return ui{}, err
	}
	catalogs, err := i18n.Load()
	if err != nil {
		return ui{}, err
	}
	return ui{
		catalogs:   catalogs,
		language:   resolution.Language,
		translator: catalogs.Translate(resolution.Language),
	}, nil
}

func preflight(args []string) (home, language string) {
	flags := pflag.NewFlagSet("relo", pflag.ContinueOnError)
	flags.ParseErrorsAllowlist.UnknownFlags = true
	// The real parse owns every message this run prints, so this pass
	// stays silent about a command line it only reads two flags from.
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.StringVar(&home, homeFlag, "", "")
	flags.StringVar(&language, langFlag, "", "")
	_ = flags.Parse(args)
	return home, language
}

func preflightHome(flagged string) (string, error) {
	if flagged != "" {
		return flagged, nil
	}
	return config.ReloHome()
}

func storeLanguage(home, tag string) error {
	_, err := config.UpdateAppearance(config.ConfigPath(home), config.AppearancePatch{Language: &tag})
	return err
}
