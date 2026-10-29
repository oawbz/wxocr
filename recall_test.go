package ocr

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

func TestDetectionTilesCoverLongImages(t *testing.T) {
	for _, b := range []image.Rectangle{image.Rect(7, 11, 707, 9011), image.Rect(-9, -20, 9991, 480)} {
		tiles := detectionTiles(b)
		if tiles[0] != b || len(tiles) < 3 {
			t.Fatal(tiles)
		}
		covered := image.Rectangle{}
		for i, r := range tiles[1:] {
			if !r.In(b) {
				t.Fatal("outside image", r)
			}
			covered = covered.Union(r)
			if i > 0 && !r.Overlaps(tiles[i]) {
				t.Fatal("missing overlap")
			}
			if max(r.Dx(), r.Dy()) > 960 {
				t.Fatal("excessive downscale")
			}
		}
		if covered != b {
			t.Fatal("incomplete coverage", covered, b)
		}
	}
	if len(detectionTiles(image.Rect(0, 0, 800, 1000))) != 1 {
		t.Fatal("ordinary image tiled")
	}
}
func TestCandidateThresholds(t *testing.T) {
	if len(candidateReasons(.65, 300, .401)) != 0 {
		t.Fatal("accepted line marked low")
	}
	if len(candidateReasons(.64, 299, .4)) != 3 {
		t.Fatal("lost rejection reasons")
	}
}
func TestMergeKeepsNearbyRepeatedTextAndCandidates(t *testing.T) {
	a := Line{Left: 10, Top: 10, Right: 100, Bottom: 30, Text: "相同", Confidence: .9}
	b := a
	b.Left++
	b.Right++
	b.Confidence = .8
	c := a
	c.Top = 40
	c.Bottom = 60
	low := a
	low.Top = 80
	low.Bottom = 100
	low.Text = "可疑"
	r := Result{Lines: []Line{a, b, c}, Candidates: []Candidate{{Line: b}, {Line: low}, {Line: low}}}
	mergePassResults(&r)
	if len(r.Lines) != 2 || len(r.Candidates) != 1 || r.Candidates[0].Text != "可疑" {
		t.Fatalf("merge %+v", r)
	}
}
func TestDiagnosticCropMatchesTensor(t *testing.T) {
	tensor := make([]float32, 3*64*32)
	for i := range tensor {
		tensor[i] = -1
	}
	tensor[0] = 1
	data, err := tensorPNG(tensor)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := im.At(0, 0).RGBA()
	if im.Bounds().Dx() != 64 || r != 65535 || g != 0 || b != 0 {
		t.Fatal("crop differs from input")
	}
	if diagnosticCrops(context.Background()) || !diagnosticCrops(WithDiagnosticCrops(context.Background())) {
		t.Fatal("crop opt-in")
	}
}

func TestRealLongImageRecall(t *testing.T) {
	if os.Getenv("WXOCR_RUNTIME") == "" {
		t.Skip("set model environment for real long-image regression")
	}
	e, err := New(Config{RuntimeLibrary: os.Getenv("WXOCR_RUNTIME"), DetectionModel: os.Getenv("WXOCR_DETECTOR"), RecognitionModel: os.Getenv("WXOCR_RECOGNIZER"), ParagraphModel: os.Getenv("WXOCR_PARAGRAPH"), CharsetFile: os.Getenv("WXOCR_CHARSET")})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	f, err := os.Open("testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	// Offset bounds also verify full-image coordinate mapping.
	long := image.NewNRGBA(image.Rect(7, 11, 967, 2891))
	for i := 0; i < 3; i++ {
		r := image.Rect(7, 11+i*960, 967, 11+(i+1)*960)
		draw.Draw(long, r, src, src.Bounds().Min, draw.Src)
	}
	result, err := e.Recognize(WithDiagnosticCrops(context.Background()), long)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, line := range result.Lines {
		counts[line.Text]++
	}
	for _, text := range []string{"微信模型跨平台测试", "中文识别2026", "Hello OCR 123"} {
		if counts[text] != 3 {
			t.Fatalf("expected each repeated line exactly three times: %+v", counts)
		}
	}
	tiled, crops := false, false
	for _, d := range result.Diagnostics {
		if d.Pass > 0 {
			tiled = true
		}
		if len(d.CropPNG) > 0 {
			crops = true
		}
		if d.Reason == "" {
			t.Fatal("unexplained region")
		}
	}
	if !tiled || !crops {
		t.Fatal("missing tile or crop diagnostics")
	}
	t.Logf("lines=%d candidates=%d diagnostic regions=%d", len(result.Lines), len(result.Candidates), len(result.Diagnostics))
}

func TestTileBoundaryOnlyInternalEdges(t *testing.T) {
	d := Diagnostic{Pass: 1, Tile: image.Rect(0, 0, 960, 960), Quad: [4][2]float64{{20, 40}, {959, 40}, {959, 70}, {20, 70}}}
	if !touchesTileBoundary(d, 1500, 200) {
		t.Fatal("truncated line accepted")
	}
	if touchesTileBoundary(d, 960, 960) {
		t.Fatal("image edge treated as tile seam")
	}
	d.Pass = 0
	if touchesTileBoundary(d, 1500, 200) {
		t.Fatal("whole image treated as tile seam")
	}
}
