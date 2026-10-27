// Accent colours: the preset ids the console offers, and the colours the
// releases before them shipped under.
package config

import "strconv"

// shippedAccents are the preset ids of the 2026 accent board. The colour each
// one paints is defined once, in the console stylesheet, and nowhere else.
var shippedAccents = []string{
	"terracotta", "persimmon", "cherry-cola", "clay-rose", "saffron", "mocha",
	"moss", "sage", "pine", "matcha",
	"lagoon", "ocean", "cobalt", "dusk-blue", "midnight",
	"lavender-haze", "orchid", "plum", "graphite",
}

// retiredAccents maps an accent id the console no longer offers to the colour
// it painted, so an existing config keeps the operator's look in the custom
// colour slot instead of falling back to the default.
var retiredAccents = map[string]string{
	"red":     "#c8102e",
	"orange":  "#d9622b",
	"amber":   "#cc7c2e",
	"green":   "#43946c",
	"emerald": "#43946c",
	"teal":    "#439288",
	"cyan":    "#418fae",
	"blue":    "#3662e3",
	"indigo":  "#4e46dc",
	"purple":  "#743ee4",
	"violet":  "#743ee4",
	"rose":    "#cf364c",
}

// isAccentValue reports whether an accent is one this build ships, one a
// config written before the 2026 board still carries, or a colour the radar
// picked.
func isAccentValue(accent string) bool {
	if isHexColour(accent) {
		return true
	}
	if retiredAccents[accent] != "" {
		return true
	}
	for _, id := range shippedAccents {
		if id == accent {
			return true
		}
	}
	return false
}

// normalizeAccent rewrites a retired accent id as the colour it painted, so
// every answer the daemon gives carries one of the two shapes the console
// applies: a preset id, or a custom colour. The file keeps its old id until
// the next appearance save.
func normalizeAccent(accent string) string {
	if hex := retiredAccents[accent]; hex != "" {
		return hex
	}
	return accent
}

// isHexColour reports whether a value is a #rrggbb colour the radar writes.
func isHexColour(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 32)
	return err == nil
}
