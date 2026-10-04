package ocr

import (
	"image"
	"math"
	"testing"
)

func TestMinimumRectangle(t *testing.T) {
	cases := []struct {
		name   string
		points []image.Point
		area   float64
	}{
		{"horizontal", []image.Point{{36, 224}, {273, 224}, {273, 249}, {36, 249}, {100, 230}}, 5925},
		{"tilted", []image.Point{{20, 30}, {80, 60}, {70, 80}, {10, 50}, {30, 40}}, 1500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, area := minimumRectangle(c.points)
			if math.Abs(area-c.area) > .01 {
				t.Fatalf("area %v", area)
			}
			for _, p := range c.points {
				u := point{q[1].x - q[0].x, q[1].y - q[0].y}
				v := point{q[3].x - q[0].x, q[3].y - q[0].y}
				x, y := float64(p.X)-q[0].x, float64(p.Y)-q[0].y
				a := (x*u.x + y*u.y) / (u.x*u.x + u.y*u.y)
				b := (x*v.x + y*v.y) / (v.x*v.x + v.y*v.y)
				if a < -1e-5 || a > 1.00001 || b < -1e-5 || b > 1.00001 {
					t.Fatalf("point %v outside %v", p, q)
				}
			}
		})
	}
	_, area := minimumRectangle([]image.Point{{1, 1}, {2, 2}})
	if area != 0 {
		t.Fatal("degenerate rectangle has area")
	}
}
func TestDetectorSizeAndCeil(t *testing.T) {
	for _, c := range []struct{ w, h, dw, dh, rw, rh int }{{1085, 958, 960, 960, 960, 848}, {1800, 220, 960, 160, 960, 118}, {1216, 926, 960, 800, 960, 732}, {960, 660, 960, 800, 960, 660}, {640, 480, 960, 800, 960, 720}} {
		b := image.Rect(0, 0, c.w, c.h)
		dw, dh := detectorSize(b)
		rw, rh, _ := scaledImageSize(b, dw, dh)
		if dw != c.dw || dh != c.dh || rw != c.rw || rh != c.rh {
			t.Fatalf("%+v got canvas %dx%d resized %dx%d", c, dw, dh, rw, rh)
		}
	}
}

func TestDetectorBilinearOpenCVBytes(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 7, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 7; x++ {
			src.Pix[y*src.Stride+x] = uint8((x*83 + y*61) % 256)
		}
	}
	tensor, _ := tensorRGB(src, 4, 4)
	// OpenCV INTER_LINEAR uint8 fixture, including its vertical truncation.
	expected := []float32{51, 186, 107, 178, 153, 42, 188, 77, 159, 144, 65, 178}
	for i, pixel := range expected {
		want := (pixel/255 - float32(.485)) / float32(.229)
		if math.Abs(float64(tensor[i]-want)) > 1e-6 {
			t.Fatalf("pixel %d: normalized %v, want %v", i, tensor[i], want)
		}
	}
}

func TestMinimumRectangleSideBeforeCornerRounding(t *testing.T) {
	// Original cv::minAreaRect reports size [50.9999924, 2.99999952].
	// Recomputing from rounded corners instead gives a side >= 3.
	p := []image.Point{{13, 13}, {43, 53}, {40, 54}, {10, 14}}
	q, _, side := minimumRectangleWithSize(p)
	if side >= 3 || math.Abs(float64(side)-2.99999952) > 1e-7 {
		t.Fatalf("native short side mismatch: %.9g", side)
	}
	rounded := math.Min(math.Hypot(q[0].x-q[1].x, q[0].y-q[1].y), math.Hypot(q[1].x-q[2].x, q[1].y-q[2].y))
	if rounded < 3 {
		t.Fatalf("fixture must expose the corner rounding boundary: %g", rounded)
	}
}
