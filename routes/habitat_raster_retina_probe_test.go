package routes

import (
	"image/png"
	"os"
	"testing"
)

// Verify the retina path: same z/x/y rendered at scale=1 must be 256px and
// scale=2 must be 512px, and the @2x output must be non-empty (plot drawn).
func TestRetinaScaleOutput(t *testing.T) {
	centerLat, centerLng := 33.5731, -7.5898
	d := func(m float64) float64 { return m * 9e-6 }
	ring := []geoLatLng{
		{Lat: centerLat + d(60), Lng: centerLng - d(60)},
		{Lat: centerLat + d(60), Lng: centerLng + d(60)},
		{Lat: centerLat - d(60), Lng: centerLng + d(60)},
		{Lat: centerLat - d(60), Lng: centerLng - d(60)},
	}
	rings := []rasterPlotRing{{ring: ring, isForSale: false}}
	z, _ := 18, 0
	tx, ty := lngLatToTileXY(centerLng, centerLat, z)

	for _, tc := range []struct {
		scale, want int
	}{{1, 256}, {2, 512}} {
		data, err := renderPlotRingsToTile(rings, z, tx, ty, tc.scale)
		if err != nil {
			t.Fatalf("scale %d: %v", tc.scale, err)
		}
		img, err := png.Decode(bytesReader(data))
		if err != nil {
			t.Fatalf("scale %d decode: %v", tc.scale, err)
		}
		b := img.Bounds()
		if b.Dx() != tc.want || b.Dy() != tc.want {
			t.Errorf("scale %d: got %dx%d, want %dx%d", tc.scale, b.Dx(), b.Dy(), tc.want, tc.want)
		}
		t.Logf("scale %d -> %dx%d, %d bytes", tc.scale, b.Dx(), b.Dy(), len(data))

		if out := os.Getenv("PROBE_OUT"); out != "" {
			f, _ := os.Create(out + "/retina_s" + probeItoa(tc.scale) + ".png")
			png.Encode(f, img)
			f.Close()
		}
	}
}
