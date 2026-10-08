package desktop

import _ "embed"

// iconPNG is the color tray icon Windows and Linux draw.
//
//go:embed assets/icon.png
var iconPNG []byte

// templateIconPNG is the monochrome icon the macOS menu bar draws; the
// system tints it to match the bar in either appearance.
//
//go:embed assets/icon-template.png
var templateIconPNG []byte

//go:embed assets/app-icon.png
var appIconPNG []byte

// Icon returns the color tray icon bytes.
func Icon() []byte { return iconPNG }

// TemplateIcon returns the macOS menu bar icon bytes.
func TemplateIcon() []byte { return templateIconPNG }

// AppIcon returns the application icon bytes used by the Dock.
func AppIcon() []byte { return appIconPNG }
