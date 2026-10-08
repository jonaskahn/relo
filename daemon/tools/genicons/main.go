// Command genicons derives every icon the product ships from the two brand
// sources in the repository. assets/logo.svg is the logo: the squircle plate
// carrying the mark, which the Dock, the installers, the launcher tiles and the
// favicon every listener serves all read. assets/logo-mark.svg is the bare mark,
// which only the tray reads, because a menu bar draws it at sixteen pixels and
// tints it to match the bar in either appearance.
//
// Both icons are drawn from the vector sources, which keeps the silhouette
// sharp at every size and hands macOS a transparent shape to tint. Each one is
// the source scaled, so the mark sits at the same share of the plate on every
// platform; only the macOS master adds back the canvas margin and the soft drop
// shadow the Dock expects from a legacy icns.
//
// Run from the daemon directory, which is what make icons does:
//
//	go run ./tools/genicons
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	xdraw "golang.org/x/image/draw"
)

// Every icon is resampled down from one render of this size, which antialiases
// better than drawing small.
const (
	renderSize       = 1024
	trayIconSize     = 64 // Windows asks for SM_CXSMICON, which is 16–48px
	templateIconSize = 44 // 22pt @2x, so the menu bar draws it sharp on Retina

	// macPlateSide is the plate's width in the macOS 27 icon grid, measured
	// off the system's own app icons: 824 of a 1024 canvas, which is the
	// margin the Dock draws around an icon and the room its shadow needs.
	macPlateSide = 824
)

// The brand sources, relative to the repository root. They are the only artwork
// a designer edits; every file written below is an output, never an input.
const (
	logoSource = "assets/logo.svg"
	markSource = "assets/logo-mark.svg"
)

// winSizes are the sizes the Windows installer icon embeds; faviconSizes the
// ones the favicon carries. The ICO directory stores a dimension in a single
// byte, so 256 is the largest entry the format can name, and the ladder between
// 16 and 128 is what covers 100% to 400% display scaling.
var (
	winSizes     = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}
	faviconSizes = []int{16, 32, 48}
	// linuxSizes is the hicolor ladder: a desktop running at 150% or 200%
	// scaling asks for the next size up, and upscaling a lone 256px icon
	// leaves the launcher entry soft.
	linuxSizes = []int{16, 24, 32, 48, 64, 128, 256, 512, 1024}
)

var (
	// ErrEmptyLogo reports a logo source that rendered to nothing, which
	// would ship a blank icon.
	ErrEmptyLogo = errors.New("the logo rendered empty")
	// ErrUnbrandedLogo reports a logo that rendered without the brand red. A
	// gradient reference that fails to resolve paints the plate solid black,
	// which is opaque enough to pass an ink check and wrong enough to ship,
	// so the render is asked for the colour itself.
	ErrUnbrandedLogo = errors.New("the logo rendered without the brand colour")
	// ErrEmptyMark reports a mark source that rendered to nothing, which
	// would ship a blank tray icon.
	ErrEmptyMark = errors.New("the mark rendered empty")
)

// icoEntry is one PNG payload of a Windows icon, with the size it carries.
type icoEntry struct {
	size int
	data []byte
}

// output is one derived file and the write that produces it.
type output struct {
	path  string
	write func(string) error
}

func main() {
	root := flag.String("root", "..", "repository root the sources are read from and the outputs written under")
	flag.Parse()
	if err := generate(*root); err != nil {
		fmt.Fprintln(os.Stderr, "genicons:", err)
		os.Exit(1)
	}
}

func generate(root string) error {
	logo, err := renderBrand(filepath.Join(root, logoSource))
	if err != nil {
		return err
	}
	if !hasInk(logo) {
		return ErrEmptyLogo
	}
	if !hasBrandRed(logo) {
		return ErrUnbrandedLogo
	}
	mark, err := renderBrand(filepath.Join(root, markSource))
	if err != nil {
		return err
	}
	if !hasInk(mark) {
		return ErrEmptyMark
	}
	return writeAll(root, logo, mark)
}

func writeAll(root string, logo, mark *image.RGBA) error {
	// The macOS master and the favicon are the one geometry drawn inside the
	// macOS grid. Everything else fills its canvas, because nothing draws it
	// within a rounded frame of its own.
	mac := withShadow(macPlate(logo))
	trayDir := filepath.Join(root, "daemon", "internal", "desktop", "assets")
	outputs := []output{
		// The repo logo is the plate at the brand-art size, so the README
		// shows a full square rather than a floating shadow.
		{filepath.Join(root, "assets", "logo.png"), func(path string) error {
			return writePNG(path, logo)
		}},
		// The macOS master keeps the canvas margins and the baked shadow the
		// Dock expects from a legacy icns.
		{filepath.Join(root, "packaging", "darwin", "app-icon.png"), func(path string) error {
			return writePNG(path, mac)
		}},
		// The Dock icon is set from a single PNG at runtime, where the system
		// draws it in its own slot and scales the backing image to match, so
		// it carries the full-resolution master rather than one screen size.
		{filepath.Join(trayDir, "app-icon.png"), func(path string) error {
			return writePNG(path, mac)
		}},
		{filepath.Join(root, "packaging", "windows", "icon.ico"), func(path string) error {
			return writeICO(path, logo, winSizes)
		}},
		// The icon every listener serves, and the copy the console build
		// ships: one rendering, written to both trees so the icon an inference
		// port shows is the icon the console shows.
		{filepath.Join(root, "daemon", "internal", "server", "assets", "favicon.ico"), func(path string) error {
			return writeICO(path, mac, faviconSizes)
		}},
		{filepath.Join(root, "console", "static", "favicon.ico"), func(path string) error {
			return writeICO(path, mac, faviconSizes)
		}},
		{filepath.Join(trayDir, "icon.png"), func(path string) error {
			return writePNG(path, scale(mark, trayIconSize, trayIconSize))
		}},
		{filepath.Join(trayDir, "icon-template.png"), func(path string) error {
			return writePNG(path, templateMask(scale(mark, templateIconSize, templateIconSize)))
		}},
	}
	for _, size := range linuxSizes {
		outputs = append(outputs, output{
			filepath.Join(root, "packaging", "linux", "icons", fmt.Sprintf("%dx%d", size, size), "apps", "relo.png"),
			func(path string) error {
				return writePNG(path, scale(logo, size, size))
			},
		})
	}
	for _, output := range outputs {
		if err := output.write(output.path); err != nil {
			return err
		}
		fmt.Println("wrote", output.path)
	}
	return nil
}

func renderBrand(path string) (*image.RGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	icon, err := oksvg.ReadIconStream(file)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	icon.SetTarget(0, 0, float64(renderSize), float64(renderSize))
	canvas := image.NewRGBA(image.Rect(0, 0, renderSize, renderSize))
	scanner := rasterx.NewScannerGV(renderSize, renderSize, canvas, canvas.Bounds())
	icon.Draw(rasterx.NewDasher(renderSize, renderSize, scanner), 1)
	return canvas, nil
}

func macPlate(logo *image.RGBA) *image.RGBA {
	size := logo.Bounds().Dx()
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	at := (size - macPlateSide) / 2
	draw.Draw(out, image.Rect(at, at, at+macPlateSide, at+macPlateSide), scale(logo, macPlateSide, macPlateSide), image.Point{}, draw.Over)
	return out
}

// withShadow places the soft drop shadow macOS app icons carry under the plate.
// It is drawn first so the plate composites over it.
func withShadow(plate *image.RGBA) *image.RGBA {
	bounds := plate.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	alpha := make([]float32, w*h)
	for i := range alpha {
		alpha[i] = float32(plate.Pix[i*4+3]) / 255
	}
	soft := blurAlpha(alpha, w, h, float64(w)*0.012)
	out := image.NewRGBA(bounds)
	drop := int(float64(w) * 0.004)
	for y := 0; y < h; y++ {
		sy := y - drop
		if sy < 0 {
			continue
		}
		for x := 0; x < w; x++ {
			if a := float64(soft[sy*w+x]) * 0.24; a > 0.002 {
				out.SetRGBA(x, y, color.RGBA{A: uint8(a * 255)})
			}
		}
	}
	draw.Draw(out, bounds, plate, bounds.Min, draw.Over)
	return out
}

// blurAlpha smooths a coverage field with a separable Gaussian kernel, clamping
// at the edges so the shadow does not darken past the canvas.
func blurAlpha(src []float32, width, height int, sigma float64) []float32 {
	kernel := gaussian(sigma)
	return blurPass(blurPass(src, width, height, kernel, true), width, height, kernel, false)
}

func gaussian(sigma float64) []float64 {
	reach := int(sigma*3 + 0.5)
	kernel := make([]float64, 2*reach+1)
	total := 0.0
	for i := -reach; i <= reach; i++ {
		v := math.Exp(-(float64(i) * float64(i)) / (2 * sigma * sigma))
		kernel[i+reach] = v
		total += v
	}
	for i := range kernel {
		kernel[i] /= total
	}
	return kernel
}

func blurPass(src []float32, width, height int, kernel []float64, horizontal bool) []float32 {
	reach := len(kernel) / 2
	dst := make([]float32, len(src))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			acc := 0.0
			for i := -reach; i <= reach; i++ {
				s := x + i
				if !horizontal {
					s = y + i
				}
				if s < 0 {
					s = 0
				} else if horizontal && s >= width {
					s = width - 1
				} else if !horizontal && s >= height {
					s = height - 1
				}
				if horizontal {
					acc += float64(src[y*width+s]) * kernel[i+reach]
				} else {
					acc += float64(src[s*width+x]) * kernel[i+reach]
				}
			}
			dst[y*width+x] = float32(acc)
		}
	}
	return dst
}

func hasInk(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				return true
			}
		}
	}
	return false
}

// hasBrandRed reports whether the plate carries the brand red. The gradient runs
// #e01833 to #b00d22, so across the whole ramp red leads green by more than a
// hundred and blue by more than thirty, and the white mark leads neither.
func hasBrandRed(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && r > 0x8000 && r > g+0x8000 && r > b+0x1800 {
				return true
			}
		}
	}
	return false
}

// templateMask turns a transparent-background mark into a macOS template icon:
// black pixels whose alpha is the mark's own coverage, which the system tints
// to match the menu bar in either appearance.
func templateMask(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			dst.SetRGBA(x, y, color.RGBA{A: uint8(a >> 8)})
		}
	}
	return dst
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return png.Encode(file, img)
}

// scale resamples src to the given square size. Large reductions step through
// repeated halvings first, because a single CatmullRom pass samples a
// four-pixel kernel and turns fine edges into aliasing noise.
func scale(src image.Image, width, height int) *image.RGBA {
	step := src
	for b := step.Bounds(); b.Dx() > 2*width && b.Dy() > 2*height; b = step.Bounds() {
		next := image.NewRGBA(image.Rect(0, 0, b.Dx()/2, b.Dy()/2))
		xdraw.CatmullRom.Scale(next, next.Bounds(), step, b, draw.Over, nil)
		step = next
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), step, step.Bounds(), draw.Over, nil)
	return dst
}

func writeICO(path string, src image.Image, sizes []int) error {
	entries := make([]icoEntry, 0, len(sizes))
	for _, size := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, scale(src, size, size)); err != nil {
			return err
		}
		entries = append(entries, icoEntry{size: size, data: buf.Bytes()})
	}
	encoded, err := encodeICO(entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

// encodeICO lays out the ICO directory and the PNG payloads it points at.
func encodeICO(entries []icoEntry) ([]byte, error) {
	var out bytes.Buffer
	for _, value := range []any{uint16(0), uint16(1), uint16(len(entries))} {
		if err := binary.Write(&out, binary.LittleEndian, value); err != nil {
			return nil, err
		}
	}
	offset := 6 + 16*len(entries)
	for _, entry := range entries {
		dimension := byte(entry.size)
		if entry.size >= 256 {
			dimension = 0 // zero means 256 in the ICO directory
		}
		if _, err := out.Write([]byte{dimension, dimension, 0, 0}); err != nil {
			return nil, err
		}
		for _, value := range []any{uint16(1), uint16(32), uint32(len(entry.data)), uint32(offset)} {
			if err := binary.Write(&out, binary.LittleEndian, value); err != nil {
				return nil, err
			}
		}
		offset += len(entry.data)
	}
	for _, entry := range entries {
		if _, err := out.Write(entry.data); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}
