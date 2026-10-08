// Package i18n holds the messages Relo speaks and the rules for choosing
// one. The command line, the desktop tray app, and the management API all
// resolve their language through here, so one setting moves every surface.
package i18n

import (
	"embed"
	"errors"
	"fmt"
	"maps"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// Auto asks the resolver to keep looking, so the layer below decides.
const Auto = "auto"

// English is the language every catalog falls back to.
const English = "en"

// ErrUnsupportedLanguage reports a language that Relo ships no catalog
// for, named by a layer that meant it.
var ErrUnsupportedLanguage = errors.New("unsupported language")

var supported = []string{
	"en", "de", "zh-Hans",
	"ar", "cs", "es", "fr", "hi", "id", "it", "ja", "ko",
	"nl", "pl", "pt-BR", "ro", "ru", "sv", "th", "tr", "uk", "vi",
	"zh-Hant",
}

//go:embed catalogs/*.toml
var catalogFiles embed.FS

// Catalogs holds the parsed message catalogs and the fallback every
// translator reaches for when a language is missing a message.
type Catalogs struct {
	bundle  *goi18n.Bundle
	english *goi18n.Localizer
}

// Load parses every embedded catalog.
func Load() (*Catalogs, error) {
	bundle := goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	entries, err := catalogFiles.ReadDir("catalogs")
	if err != nil {
		return nil, fmt.Errorf("read message catalogs: %w", err)
	}
	for _, entry := range entries {
		data, err := catalogFiles.ReadFile("catalogs/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read catalog %s: %w", entry.Name(), err)
		}
		if _, err := bundle.ParseMessageFileBytes(data, entry.Name()); err != nil {
			return nil, fmt.Errorf("parse catalog %s: %w", entry.Name(), err)
		}
	}
	return &Catalogs{bundle: bundle, english: goi18n.NewLocalizer(bundle, English)}, nil
}

// Supported lists the language tags Relo ships.
func Supported() []string {
	tags := make([]string, len(supported))
	copy(tags, supported)
	return tags
}

// Supported lists the language tags Relo ships.
func (c *Catalogs) Supported() []string { return Supported() }

// Match maps a tag named by a user, the environment, a browser, or a
// setting onto a catalog Relo ships. A regional tag belongs to its base
// language, so pt-PT reads as the Brazilian Portuguese catalog and de-AT
// as the German one. A Chinese tag without an explicit script is read as
// Simplified, while an explicit Traditional script or a Traditional region
// (TW, HK, MO) reads as Traditional Chinese.
func Match(tag string) (string, bool) {
	if tag == "" || tag == Auto {
		return "", false
	}
	parsed, err := language.Parse(tag)
	if err != nil {
		return "", false
	}
	if match, found := matchChineseTag(parsed); found {
		return match, true
	}
	base, _ := parsed.Base()
	for _, candidate := range supported {
		parsedCandidate, _ := language.Parse(candidate)
		candidateBase, _ := parsedCandidate.Base()
		if base.String() == candidateBase.String() {
			return candidate, true
		}
	}
	return "", false
}

func matchChineseTag(parsed language.Tag) (string, bool) {
	if script, _ := parsed.Script(); script.String() == "Hant" {
		return "zh-Hant", true
	}
	base, _ := parsed.Base()
	region, _ := parsed.Region()
	if base.String() != "zh" {
		return "", false
	}
	switch region.String() {
	case "TW", "HK", "MO":
		return "zh-Hant", true
	default:
		return "zh-Hans", true
	}
}

// Match maps a tag onto a catalog Relo ships.
func (c *Catalogs) Match(tag string) (string, bool) { return Match(tag) }

// Translate returns the translator for one language. A tag with no
// catalog renders English rather than nothing, and catalogs that never
// loaded render the message IDs, which is what a build without them has.
func (c *Catalogs) Translate(tag string) *Translator {
	matched, ok := Match(tag)
	if !ok {
		matched = English
	}
	translator := &Translator{catalogs: c, language: matched}
	if c != nil {
		translator.localizer = goi18n.NewLocalizer(c.bundle, matched)
	}
	return translator
}

// Translator renders messages in one language.
type Translator struct {
	catalogs  *Catalogs
	language  string
	localizer *goi18n.Localizer
}

// Language returns the tag this translator renders in.
func (t *Translator) Language() string { return t.language }

// Text renders one message with the named values.
func (t *Translator) Text(id string, data map[string]any) string {
	return t.render(request{id: id, data: data})
}

// Plural renders one message with the count that selects its plural form,
// which the message reads as {{.Count}}.
func (t *Translator) Plural(id string, count int, data map[string]any) string {
	return t.render(request{id: id, count: count, plural: true, data: withCount(data, count)})
}

type request struct {
	id     string
	count  int
	plural bool
	data   map[string]any
}

func (t *Translator) render(r request) string {
	if t.localizer != nil {
		if rendered, err := localize(t.localizer, r); err == nil {
			return rendered
		}
	}
	if t.catalogs != nil && t.catalogs.english != nil {
		if rendered, err := localize(t.catalogs.english, r); err == nil {
			return rendered
		}
	}
	return r.id
}

func localize(localizer *goi18n.Localizer, r request) (string, error) {
	config := &goi18n.LocalizeConfig{MessageID: r.id, TemplateData: r.data}
	if r.plural {
		config.PluralCount = r.count
	}
	return localizer.Localize(config)
}

func withCount(data map[string]any, count int) map[string]any {
	filled := make(map[string]any, len(data)+1)
	maps.Copy(filled, data)
	filled["Count"] = count
	return filled
}
