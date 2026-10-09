// Processing renders the native menu's bounded shutdown indicator.
package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
)

var processingPNG = sync.OnceValue(func() []byte {
	const side = 32
	canvas := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			dx, dy := float64(x)-15.5, float64(y)-15.5
			radius := math.Hypot(dx, dy)
			angle := math.Mod(math.Atan2(dy, dx)+2*math.Pi, math.Pi/4)
			if radius >= 8 && radius <= 13 && (angle < 0.10 || angle > math.Pi/4-0.10) {
				canvas.SetNRGBA(x, y, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
			}
		}
	}
	var output bytes.Buffer
	_ = png.Encode(&output, canvas)
	return output.Bytes()
})

func processingIcon() []byte { return processingPNG() }
