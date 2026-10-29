package ocr

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func tensorRGBReference(src image.Image, width, height int) ([]float32, float64) {
	b := src.Bounds()
	w, h, scale := scaledImageSize(b, width, height)
	n := width * height
	out := make([]float32, 3*n)
	mean := [3]float32{0.485, 0.456, 0.406}
	std := [3]float32{0.229, 0.224, 0.225}
	for ch := 0; ch < 3; ch++ {
		for i := 0; i < n; i++ {
			out[ch*n+i] = -mean[ch] / std[ch]
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := float64(float32((float64(x)+0.5)*float64(b.Dx())/float64(w) - 0.5))
			sy := float64(float32((float64(y)+0.5)*float64(b.Dy())/float64(h) - 0.5))
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(ix), sy-float64(iy)
			if ix < 0 {
				ix = 0
				fx = 0
			}
			if ix >= b.Dx()-1 {
				ix = b.Dx() - 1
				fx = 0
			}
			if iy < 0 {
				iy = 0
				fy = 0
			}
			if iy >= b.Dy()-1 {
				iy = b.Dy() - 1
				fy = 0
			}
			ax := [2]int{int(math.RoundToEven(float64(1-float32(fx)) * 2048)), int(math.RoundToEven(fx * 2048))}
			ay := [2]int{int(math.RoundToEven(float64(1-float32(fy)) * 2048)), int(math.RoundToEven(fy * 2048))}
			var rows [2][3]int64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					rgb := nativeRGB(src.At(b.Min.X+min(ix+dx, b.Dx()-1), b.Min.Y+min(iy+dy, b.Dy()-1)))
					for ch, value := range rgb {
						rows[dy][ch] += int64(value) * int64(ax[dx])
					}
				}
			}
			for ch := 0; ch < 3; ch++ {
				// OpenCV VResizeLinear<uchar> truncates each vertical term before adding.
				pixel := float32((((int64(ay[0]) * (rows[0][ch] >> 4)) >> 16) + ((int64(ay[1]) * (rows[1][ch] >> 4)) >> 16) + 2) >> 2)
				out[ch*n+y*width+x] = (pixel/255 - mean[ch]) / std[ch]
			}
		}
	}
	return out, scale
}

func TestTensorRGBFastPathExact(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	images := []image.Image{image.NewNRGBA(image.Rect(-3, 4, 20, 21)), image.NewRGBA(image.Rect(-3, 4, 20, 21)), image.NewGray(image.Rect(-3, 4, 20, 21)), image.NewNRGBA64(image.Rect(-3, 4, 20, 21))}
	for _, im := range images {
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				c := color.NRGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256))}
				switch p := im.(type) {
				case *image.NRGBA:
					p.Set(x, y, c)
				case *image.RGBA:
					p.Set(x, y, c)
				case *image.Gray:
					p.Set(x, y, c)
				case *image.NRGBA64:
					p.Set(x, y, c)
				}
			}
		}
		for _, size := range [][2]int{{1, 1}, {7, 9}, {40, 30}, {23, 17}, {16, 40}} {
			got, _ := tensorRGB(im, size[0], size[1])
			want, _ := tensorRGBReference(im, size[0], size[1])
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%T size %v mismatch", im, size)
			}
		}
	}
	// A subimage retains the parent's stride and starts at a shifted Pix offset.
	parent := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	rng.Read(parent.Pix)
	sub := parent.SubImage(image.Rect(8, 9, 25, 31))
	got, _ := tensorRGB(sub, 32, 48)
	want, _ := tensorRGBReference(sub, 32, 48)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("subimage mismatch")
	}
}

// Wrapping hides concrete pixel storage and exercises the original Color conversion.
type genericImage struct{ image.Image }

func TestRecognitionPixelFastPathExact(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	nrgba := image.NewNRGBA(image.Rect(-2, 3, 27, 22))
	rng.Read(nrgba.Pix)
	rgba := image.NewRGBA(nrgba.Bounds())
	for y := rgba.Rect.Min.Y; y < rgba.Rect.Max.Y; y++ {
		for x := rgba.Rect.Min.X; x < rgba.Rect.Max.X; x++ {
			rgba.Set(x, y, nrgba.At(x, y))
		}
	}
	for _, im := range []image.Image{nrgba, rgba} {
		for _, q := range []quadrilateral{{{-3, 2}, {21, 4}, {23, 19}, {-1, 18}}, {{1, 4}, {20, 4}, {20, 20}, {1, 20}}} {
			got := recognitionQuadTensor(im, q, true)
			want := recognitionQuadTensor(genericImage{im}, q, true)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%T crop mismatch", im)
			}
		}
	}
}
