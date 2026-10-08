package tray_test

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/jonaskahn/relo/internal/desktop"
)

func TestDashboardURL(t *testing.T) {
	tests := []struct {
		name string
		bind string
		port int
		want string
	}{
		{"loopback stays loopback", "127.0.0.1", 10101, "http://127.0.0.1:10101"},
		{"a wildcard bind is read as this machine", "0.0.0.0", 10101, "http://127.0.0.1:10101"},
		{"an empty bind is read as this machine", "", 8080, "http://127.0.0.1:8080"},
		{"another interface is kept", "192.168.1.5", 9000, "http://192.168.1.5:9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desktop.DashboardURL(tt.bind, tt.port); got != tt.want {
				t.Errorf("DashboardURL(%q, %d) = %q, want %q", tt.bind, tt.port, got, tt.want)
			}
		})
	}
}

func TestIconAssets(t *testing.T) {
	t.Run("the tray icon is a PNG", func(t *testing.T) {
		assertPNG(t, desktop.Icon())
	})
	t.Run("the template icon is a PNG", func(t *testing.T) {
		assertPNG(t, desktop.TemplateIcon())
	})
}

// assertPNG decodes the bytes and fails the test when they are not a
// decodable PNG image.
func assertPNG(t *testing.T, data []byte) {
	t.Helper()
	if len(data) == 0 {
		t.Fatal("the embedded icon is empty")
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the embedded icon does not decode: %v", err)
	}
	if image.Bounds().Dx() == 0 || image.Bounds().Dy() == 0 {
		t.Fatal("the embedded icon has no pixels")
	}
}
