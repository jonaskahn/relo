package i18n_test

import (
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/i18n"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name  string
		tag   string
		want  string
		found bool
	}{
		{"english", "en", "en", true},
		{"english with a region", "en-GB", "en", true},
		{"german", "de", "de", true},
		{"german with a region", "de-AT", "de", true},
		{"german that differs only in case", "DE", "de", true},
		{"french", "fr", "fr", true},
		{"french with a region", "fr-CA", "fr", true},
		{"spanish with a region", "es-MX", "es", true},
		{"japanese", "ja", "ja", true},
		{"brazilian portuguese", "pt-BR", "pt-BR", true},
		{"portuguese from portugal", "pt-PT", "pt-BR", true},
		{"chinese without a script", "zh", "zh-Hans", true},
		{"simplified chinese", "zh-Hans", "zh-Hans", true},
		{"chinese from the mainland", "zh-CN", "zh-Hans", true},
		{"chinese from singapore", "zh-SG", "zh-Hans", true},
		{"traditional chinese by script", "zh-Hant", "zh-Hant", true},
		{"traditional chinese by region", "zh-TW", "zh-Hant", true},
		{"traditional chinese from hong kong", "zh-HK", "zh-Hant", true},
		{"swahili has no catalog", "sw", "", false},
		{"nonsense", "not a tag", "", false},
		{"empty", "", "", false},
		{"auto is not a language", i18n.Auto, "", false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, found := i18n.Match(testCase.tag)
			if got != testCase.want || found != testCase.found {
				t.Fatalf("Match(%q) = %q, %v, want %q, %v", testCase.tag, got, found, testCase.want, testCase.found)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name       string
		options    i18n.Options
		want       string
		wantSource i18n.Source
	}{
		{
			"the flag wins over every other layer",
			i18n.Options{Flag: "de", Stored: "en", Config: "en", System: "en-US"},
			"de", i18n.SourceFlag,
		},
		{
			"what is stored wins over the configuration and the system locale",
			i18n.Options{Stored: "zh-Hans", Config: "de", System: "de-DE"},
			"zh-Hans", i18n.SourceStored,
		},
		{
			"a stored choice wins over the configuration",
			i18n.Options{Stored: "de", Config: "en", System: "en-US"},
			"de", i18n.SourceStored,
		},
		{
			"the configuration wins over the system locale",
			i18n.Options{Config: "zh-Hans", System: "de-DE"},
			"zh-Hans", i18n.SourceConfig,
		},
		{
			"the system locale decides when nothing else does",
			i18n.Options{System: "de-DE"},
			"de", i18n.SourceSystem,
		},
		{
			"a newly shipped system locale is honored",
			i18n.Options{System: "fr-FR"},
			"fr", i18n.SourceSystem,
		},
		{
			"an unreadable system locale falls back quietly",
			i18n.Options{System: "sw-KE"},
			"en", i18n.SourceDefault,
		},
		{
			"nothing set is English",
			i18n.Options{},
			"en", i18n.SourceDefault,
		},
		{
			"auto keeps looking at the layer below",
			i18n.Options{Flag: i18n.Auto, Config: i18n.Auto, System: "zh-CN"},
			"zh-Hans", i18n.SourceSystem,
		},
		{
			"a stored auto leaves the configuration deciding",
			i18n.Options{Stored: i18n.Auto, Config: "de", System: "en-US"},
			"de", i18n.SourceConfig,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			resolution, err := i18n.Resolve(testCase.options)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if resolution.Language != testCase.want || resolution.Source != testCase.wantSource {
				t.Fatalf("Resolve() = %+v, want %q from %s", resolution, testCase.want, testCase.wantSource)
			}
		})
	}
}

func TestResolveRefusesWhatItCannotSpeak(t *testing.T) {
	tests := []struct {
		name    string
		options i18n.Options
	}{
		{"the flag", i18n.Options{Flag: "sw"}},
		{"what is stored", i18n.Options{Stored: "sw"}},
		{"the configuration", i18n.Options{Config: "sw"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := i18n.Resolve(testCase.options); !errors.Is(err, i18n.ErrUnsupportedLanguage) {
				t.Fatalf("Resolve() error = %v, want ErrUnsupportedLanguage", err)
			}
		})
	}
}

func TestSystemReadsTheShellLocale(t *testing.T) {
	tests := []struct {
		name   string
		locale string
		want   string
	}{
		{"a language with an encoding", "de_DE.UTF-8", "de-DE"},
		{"a language alone", "de", "de"},
		{"a modifier", "zh_CN@latin", "zh-CN"},
		{"the C locale names no language", "C", ""},
		{"the POSIX locale names no language", "POSIX", ""},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("LC_ALL", testCase.locale)
			t.Setenv("LC_MESSAGES", "")
			t.Setenv("LANG", "")
			if got := i18n.System(); got != testCase.want {
				t.Fatalf("System() = %q, want %q", got, testCase.want)
			}
		})
	}

	t.Run("the shell variables are read in order", func(t *testing.T) {
		t.Setenv("LC_ALL", "")
		t.Setenv("LC_MESSAGES", "zh_CN.UTF-8")
		t.Setenv("LANG", "de_DE.UTF-8")
		if got := i18n.System(); got != "zh-CN" {
			t.Fatalf("System() = %q, want the LC_MESSAGES locale", got)
		}
		t.Setenv("LC_MESSAGES", "")
		if got := i18n.System(); got != "de-DE" {
			t.Fatalf("System() = %q, want the LANG locale", got)
		}
	})
}
