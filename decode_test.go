package ocr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	strictjpeg "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestJPEGEntropyRecovery(t *testing.T) {
	data, err := os.ReadFile("testdata/decode/baseline.jpg")
	if err != nil {
		t.Fatal(err)
	}
	marker := bytes.Index(data, []byte{0xff, 0xda})
	if marker < 0 {
		t.Fatal("fixture has no scan")
	}
	scanLength := int(data[marker+2])*256 + int(data[marker+3])
	start := marker + 2 + scanLength
	// Terminate an incomplete entropy scan with EOI. The native decoder
	// recovers missing coefficients; Go's strict JPEG decoder rejects it.
	corrupt := append(append([]byte{}, data[:start+(len(data)-2-start)/2]...), 0xff, 0xd9)
	if _, err = strictjpeg.Decode(bytes.NewReader(corrupt)); err == nil {
		t.Fatal("corruption did not exercise recovery")
	}
	img, err := DecodeImage(bytes.NewReader(corrupt))
	if err != nil {
		t.Fatal("native entropy recovery:", err)
	}
	if img.Bounds() != image.Rect(0, 0, 37, 23) {
		t.Fatal("recovered dimensions")
	}
}

func TestJPEGDimensionLimit(t *testing.T) {
	data, err := os.ReadFile("testdata/decode/baseline.jpg")
	if err != nil {
		t.Fatal(err)
	}
	marker := bytes.Index(data, []byte{0xff, 0xc0})
	if marker < 0 {
		t.Fatal("fixture has no baseline frame")
	}
	copy(data[marker+5:marker+9], []byte{0xff, 0xff, 0xff, 0xff})
	if _, err = DecodeImage(bytes.NewReader(data)); err == nil {
		t.Fatal("accepted oversized JPEG header")
	}
}

func TestJPEGColorFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/decode/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		File    string `json:"file"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
		RGBHash string `json:"rgb_sha256"`
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.File, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata/decode", c.File))
			if err != nil {
				t.Fatal(err)
			}
			img, err := DecodeImage(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds() != image.Rect(0, 0, c.Width, c.Height) {
				t.Fatal("JPEG dimensions")
			}
			hash := sha256.New()
			for y := 0; y < c.Height; y++ {
				for x := 0; x < c.Width; x++ {
					pixel := nativeRGB(img.At(x, y))
					hash.Write(pixel[:])
				}
			}
			if hex.EncodeToString(hash.Sum(nil)) != c.RGBHash {
				t.Fatal("JPEG RGB differs from OpenCV reference")
			}
			// libjpeg recovers a missing EOI, while truly invalid headers fail.
			if _, err = DecodeImage(bytes.NewReader(data[:len(data)-2])); err != nil {
				t.Fatal("missing EOI recovery:", err)
			}
		})
	}
}

func TestDecodeInvalidJPEG(t *testing.T) {
	for _, data := range [][]byte{nil, {0xff, 0xd8}, {0xff, 0xd8, 0xff, 0xd9}, []byte("not an image")} {
		if _, err := DecodeImage(bytes.NewReader(data)); err == nil {
			t.Fatal("accepted invalid image")
		}
	}
}

func TestDecodePNGTransparentRGB(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 37, G: 91, B: 123, A: 0})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeImage(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if nativeRGB(got.At(0, 0)) != [3]uint8{37, 91, 123} {
		t.Fatal("PNG transparent RGB lost")
	}
}
