package ocr

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func intersectionOverUnion(a, b Line) float64 {
	area := func(r Line) float64 {
		return math.Max(0, float64(r.Right-r.Left)) * math.Max(0, float64(r.Bottom-r.Top))
	}
	overlap := math.Max(0, math.Min(float64(a.Right), float64(b.Right))-math.Max(float64(a.Left), float64(b.Left))) * math.Max(0, math.Min(float64(a.Bottom), float64(b.Bottom))-math.Max(float64(a.Top), float64(b.Top)))
	if overlap == 0 {
		return 0
	}
	return overlap / (area(a) + area(b) - overlap)
}
func TestNativeCorpus(t *testing.T) {
	if os.Getenv("WXOCR_NATIVE_CORPUS") != "1" {
		t.Skip("set WXOCR_NATIVE_CORPUS=1 and real model environment")
	}
	e, err := New(Config{RuntimeLibrary: os.Getenv("WXOCR_RUNTIME"), DetectionModel: os.Getenv("WXOCR_DETECTOR"), RecognitionModel: os.Getenv("WXOCR_RECOGNIZER"), ParagraphModel: os.Getenv("WXOCR_PARAGRAPH"), CharsetFile: os.Getenv("WXOCR_CHARSET")})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	raw, err := os.ReadFile("testdata/native-cases/native_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var refs map[string]Result
	if err = json.Unmarshal(raw, &refs); err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile("testdata/native-cases/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		File string `json:"file"`
	}
	if err = json.Unmarshal(manifestRaw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		name := c.File[:len(c.File)-len(filepath.Ext(c.File))]
		expected, ok := refs[name]
		if !ok {
			t.Fatalf("missing native reference for %s", name)
		}
		t.Run(name, func(t *testing.T) {
			got, err := e.RecognizeFile(context.Background(), filepath.Join("testdata/native-cases", c.File))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Lines) != len(expected.Lines) {
				t.Fatalf("native %d blocks, Go %d", len(expected.Lines), len(got.Lines))
			}
			used := make([]bool, len(got.Lines))
			var confidenceMax float64
			// Compare corresponding boxes, independent of paragraph reading order.
			for _, native := range expected.Lines {
				best, index := 0.0, -1
				for j, b := range got.Lines {
					if !used[j] {
						overlap := intersectionOverUnion(native, b)
						if overlap > best {
							best, index = overlap, j
						}
					}
				}
				if index < 0 || best < .98 {
					t.Fatalf("missing native box %+v; best IoU %v", native, best)
				}
				used[index] = true
				b := got.Lines[index]
				if b.Text != native.Text {
					t.Errorf("native %q, Go %q", native.Text, b.Text)
				}
				for _, d := range []float32{b.Left - native.Left, b.Top - native.Top, b.Right - native.Right, b.Bottom - native.Bottom} {
					if math.Abs(float64(d)) > .001 {
						t.Errorf("coordinate delta %v for %q", d, native.Text)
					}
				}
				confidenceMax = math.Max(confidenceMax, math.Abs(float64(b.Confidence-native.Confidence)))
			}
			for _, b := range got.Lines {
				if math.IsNaN(float64(b.Confidence)) || math.IsInf(float64(b.Confidence), 0) || b.Confidence < 0 || b.Confidence > 1 {
					t.Errorf("invalid sequence confidence for %q: %v", b.Text, b.Confidence)
				}
			}
			t.Logf("%d native blocks matched; max confidence difference %.6f (native float output, 9 significant digits)", len(expected.Lines), confidenceMax)
		})
	}
}
