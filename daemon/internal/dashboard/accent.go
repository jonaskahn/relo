// The accent the shell paints before it paints anything else.
package dashboard

import (
	"math"
	"strconv"
	"strings"
)

// isCustomAccent reports whether an accent is a colour the radar picked
// rather than a preset the stylesheet names.
func isCustomAccent(accent string) bool {
	return strings.HasPrefix(accent, "#")
}

// accentSelector is the data-accent the stylesheet keys on: the preset id as
// it stands, or custom for a colour the radar picked.
func accentSelector(accent string) string {
	if isCustomAccent(accent) {
		return "custom"
	}
	return accent
}

// accentOn picks the ink that reads better on a custom accent base, measured
// the way WCAG measures contrast, so the light theme keeps its 4.5:1 rule for
// a colour no preset shipped with.
func accentOn(base string) string {
	if contrast(base, "#ffffff") >= contrast(base, "#17171c") {
		return "#ffffff"
	}
	return "#17171c"
}

// contrast is the WCAG ratio of two #rrggbb colours. A value that is not one
// reports no contrast, which leaves the caller with the other ink.
func contrast(a, b string) float64 {
	fa, okA := luminance(a)
	fb, okB := luminance(b)
	if !okA || !okB {
		return 0
	}
	if fa < fb {
		fa, fb = fb, fa
	}
	return (fa + 0.05) / (fb + 0.05)
}

// luminance is the WCAG relative luminance of a #rrggbb colour.
func luminance(hex string) (float64, bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, false
	}
	value, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return 0, false
	}
	channel := func(part float64) float64 {
		if part <= 0.04045 {
			return part / 12.92
		}
		return math.Pow((part+0.055)/1.055, 2.4)
	}
	red := channel(float64(value>>16&0xff) / 255)
	green := channel(float64(value>>8&0xff) / 255)
	blue := channel(float64(value&0xff) / 255)
	return 0.2126*red + 0.7152*green + 0.0722*blue, true
}
