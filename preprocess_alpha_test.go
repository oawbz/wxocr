package ocr

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestNativeAlphaDiscard(t *testing.T) {
	// Transparent pixels retain their encoded RGB; neither blackening nor white
	// compositing matches native color decoding. Test both preprocessing paths.
	transparent := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	opaque := image.NewNRGBA(transparent.Bounds())
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := color.NRGBA{uint8(30 + x*40), uint8(20 + y*50), 190, uint8((x + y) * 40)}
			transparent.SetNRGBA(x, y, c)
			c.A = 255
			opaque.SetNRGBA(x, y, c)
		}
	}
	a, _ := tensorRGB(transparent, 8, 8)
	b, _ := tensorRGB(opaque, 8, 8)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("detector preprocessing composites alpha")
	}
	a = recognitionTensor(transparent, 0, 0, 4, 4, true)
	b = recognitionTensor(opaque, 0, 0, 4, 4, true)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("recognition preprocessing composites alpha")
	}
}
