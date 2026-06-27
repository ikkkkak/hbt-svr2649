package videoprocessing

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// writeSlideshowWheelPNG creates a transparent ring wheel used as a rotating brand badge.
func writeSlideshowWheelPNG(path string) error {
	const size = 128
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	brand := color.RGBA{R: 209, G: 96, B: 36, A: 255}
	ringInner := color.RGBA{R: 255, G: 255, B: 255, A: 210}
	spoke := color.RGBA{R: 255, G: 255, B: 255, A: 160}
	hub := color.RGBA{R: 18, G: 18, B: 18, A: 230}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx + 0.5
			dy := float64(y) - cy + 0.5
			dist := math.Sqrt(dx*dx + dy*dy)
			switch {
			case dist >= 50 && dist <= 58:
				img.Set(x, y, brand)
			case dist >= 42 && dist < 50:
				img.Set(x, y, ringInner)
			case dist <= 16:
				img.Set(x, y, hub)
			default:
				angle := math.Atan2(dy, dx)
				for i := 0; i < 8; i++ {
					target := float64(i) * math.Pi / 4
					delta := math.Abs(math.Mod(angle-target+math.Pi, 2*math.Pi) - math.Pi)
					if delta < 0.07 && dist > 18 && dist < 46 {
						img.Set(x, y, spoke)
						break
					}
				}
			}
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
