package i18n

import "fmt"

// Source names the layer a language came from, which is what tells an
// operator why Relo speaks the way it does.
type Source string

// The layers a language is resolved from, highest first.
const (
	SourceFlag    Source = "flag"
	SourceStored  Source = "stored"
	SourceConfig  Source = "config"
	SourceSystem  Source = "system"
	SourceDefault Source = "default"
)

// Resolution is the language in force and the layer that chose it.
type Resolution struct {
	Language string
	Source   Source
}

// Options carries the layers a language is resolved from, highest first.
// Every field is a raw tag: auto, or one of the shipped languages.
type Options struct {
	Flag   string
	Stored string
	Config string
	System string
}

// Resolve picks the language in force. auto keeps looking at the layer
// below, a layer that names a language Relo does not ship is an error, and
// the system locale is a hint that falls back quietly.
func Resolve(opts Options) (Resolution, error) {
	for _, layer := range []struct {
		tag    string
		source Source
	}{
		{opts.Flag, SourceFlag},
		{opts.Stored, SourceStored},
		{opts.Config, SourceConfig},
	} {
		if layer.tag == "" || layer.tag == Auto {
			continue
		}
		tag, ok := Match(layer.tag)
		if !ok {
			return Resolution{}, fmt.Errorf("%w: %q set by %s", ErrUnsupportedLanguage, layer.tag, layer.source)
		}
		return Resolution{Language: tag, Source: layer.source}, nil
	}
	if tag, ok := Match(opts.System); ok {
		return Resolution{Language: tag, Source: SourceSystem}, nil
	}
	return Resolution{Language: English, Source: SourceDefault}, nil
}
