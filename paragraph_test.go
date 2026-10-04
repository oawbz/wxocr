package ocr

import (
	"context"
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLineSortDirections(t *testing.T) {
	a := Line{Left: 10, Top: 10, Right: 20, Bottom: 20, Text: "a"}
	b := Line{Left: 11, Top: 11, Right: 21, Bottom: 21, Text: "b"}
	for direction, want := range []bool{true, true, false, false} {
		if lineLess(a, b, direction) != want || lineLess(b, a, direction) == want {
			t.Fatalf("direction %d", direction)
		}
	}
	// Primary coordinate buckets take precedence over the secondary coordinate.
	a.Top, b.Top = 3, 4
	a.Right, b.Right = 100, 10
	if !lineLess(a, b, 0) {
		t.Fatal("row bucket ignored")
	}
}

func TestParagraphCoverageAndIndices(t *testing.T) {
	result := &Result{Width: 200, Height: 100, Lines: []Line{
		{Left: 100, Top: 10, Right: 119, Bottom: 29, Text: "right"},
		{Left: 0, Top: 10, Right: 19, Bottom: 29, Text: "left"},
		{Left: 0, Top: 50, Right: 19, Bottom: 69, Text: "unassigned"},
	}}
	// A separate model region covers exactly 75 percent of the third line.
	regions := []paragraphRegion{{box: image.Rect(0, 10, 20, 30)}, {box: image.Rect(100, 10, 120, 30)}, {box: image.Rect(0, 50, 15, 70)}}
	groupParagraphs(result, regions, 0)
	var texts []string
	for _, b := range result.Lines {
		texts = append(texts, b.Text)
	}
	if !reflect.DeepEqual(texts, []string{"left", "right", "unassigned"}) {
		t.Fatal(texts)
	}
	if len(result.Paragraphs) != 3 {
		t.Fatal(result.Paragraphs)
	}
	for i, p := range result.Paragraphs {
		if !reflect.DeepEqual(p.LineIndices, []int{i}) {
			t.Fatal(p)
		}
	}
}

func TestParagraphLineSeparation(t *testing.T) {
	a := Line{Left: 0, Top: 0, Right: 100, Bottom: 20}
	b := Line{Left: 0, Top: 100, Right: 80, Bottom: 120}
	if paragraphLineGap(a, b, 0, nil) {
		t.Fatal("aligned column must remain together")
	}
	b.Left, b.Right = 200, 300
	b.Top, b.Bottom = 0, 20
	if !paragraphLineGap(a, b, 0, nil) {
		t.Fatal("distant columns must create a candidate boundary")
	}
}

func TestParagraphRegions(t *testing.T) {
	const size = 32
	prob := make([]float32, size*size)
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			prob[y*size+x] = 1
		}
	}
	regions, err := paragraphRegions(prob, size, size, size, size, 1, 1)
	if err != nil || len(regions) != 1 {
		t.Fatalf("%v %v", regions, err)
	}
	if regions[0].box.Min.X >= 8 || regions[0].box.Max.X <= 24 {
		t.Fatal("polygon did not expand")
	}
	prob[0] = float32(math.NaN())
	if _, err = paragraphRegions(prob, size, size, size, size, 1, 1); err == nil {
		t.Fatal("accepted NaN")
	}
}

func TestNativeParagraphCorpus(t *testing.T) {
	if os.Getenv("WXOCR_NATIVE_CORPUS") != "1" || os.Getenv("WXOCR_PARAGRAPH") == "" {
		t.Skip("set real models, WXOCR_PARAGRAPH and WXOCR_NATIVE_CORPUS=1")
	}
	config := Config{RuntimeLibrary: os.Getenv("WXOCR_RUNTIME"), DetectionModel: os.Getenv("WXOCR_DETECTOR"), RecognitionModel: os.Getenv("WXOCR_RECOGNIZER"), ParagraphModel: os.Getenv("WXOCR_PARAGRAPH"), CharsetFile: os.Getenv("WXOCR_CHARSET")}
	e, err := New(config)
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
	for name, expected := range refs {
		t.Run(name, func(t *testing.T) {
			got, err := e.RecognizeFile(context.Background(), filepath.Join("testdata/native-cases", name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Lines) != len(expected.Lines) {
				t.Fatalf("block count: %d vs %d", len(got.Lines), len(expected.Lines))
			}
			next := 0
			for _, p := range got.Paragraphs {
				for _, index := range p.LineIndices {
					if index != next {
						t.Fatalf("paragraph index %d, want %d", index, next)
					}
					next++
				}
			}
			if next != len(got.Lines) {
				t.Fatal("paragraphs omitted lines")
			}
			for i, native := range expected.Lines {
				b := got.Lines[i]
				if b.Text != native.Text {
					t.Errorf("line %d: Go %q, native %q", i, b.Text, native.Text)
				}
				for _, d := range []float32{b.Left - native.Left, b.Top - native.Top, b.Right - native.Right, b.Bottom - native.Bottom} {
					if math.Abs(float64(d)) > .001 {
						t.Errorf("line %d coordinate delta %v", i, d)
					}
				}
			}
		})
	}
	// Failed paragraph-model initialization must release its runtime reference.
	bad := config
	bad.ParagraphModel = "missing-paragraph.onnx"
	if _, err := New(bad); err == nil {
		t.Fatal("accepted missing paragraph model")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
}

func TestParagraphFirstCoverageWins(t *testing.T) {
	result := &Result{Width: 100, Height: 40, Lines: []Line{
		{Left: 0, Top: 0, Right: 19, Bottom: 19, Text: "first"},
		{Left: 50, Top: 0, Right: 69, Bottom: 19, Text: "second"},
	}}
	regions := []paragraphRegion{{box: image.Rect(0, 0, 15, 20)}, {box: image.Rect(0, 0, 80, 20)}}
	groupParagraphs(result, regions, 0)
	if len(result.Paragraphs) != 2 {
		t.Fatal("must use first coverage >=75%, not maximum coverage", result.Paragraphs)
	}
}

func TestParagraphSentenceBoundaries(t *testing.T) {
	for _, punctuation := range []bool{false, true} {
		texts := []string{"第一行", "第二行"}
		want := 1
		if punctuation {
			texts = []string{"第一行。", "第二行。"}
			want = 2
		}
		result := &Result{Width: 500, Height: 200, Lines: []Line{
			{Left: 0, Top: 0, Right: 99, Bottom: 19, Text: texts[0]},
			{Left: 300, Top: 100, Right: 399, Bottom: 119, Text: texts[1]},
		}}
		groupParagraphs(result, []paragraphRegion{{box: image.Rect(0, 0, 500, 150)}}, 0)
		if len(result.Paragraphs) != want {
			t.Fatalf("punctuation %v: paragraphs %v", punctuation, result.Paragraphs)
		}
	}
}

func TestContoursRetainHoles(t *testing.T) {
	const size = 16
	mask := make([]bool, size*size)
	for y := 1; y < 15; y++ {
		for x := 1; x < 15; x++ {
			mask[y*size+x] = !(x >= 5 && x < 11 && y >= 5 && y < 11)
		}
	}
	contours := binaryContours(mask, size, size)
	if len(contours) != 2 {
		t.Fatalf("contours %d", len(contours))
	}
	// OpenCV RETR_LIST reference: the hole appears before its enclosing contour.
	for i, want := range []float64{47, 169} {
		area, _ := polygonMetrics(contours[i])
		if area != want {
			t.Fatalf("contour %d area %v, want %v", i, area, want)
		}
	}
}

func TestParagraphOpeningAtBorder(t *testing.T) {
	mask := make([]bool, 16)
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			mask[y*4+x] = true
		}
	}
	if !reflect.DeepEqual(opening3(mask, 4, 4), mask) {
		t.Fatal("OpenCV default morphology border must preserve the corner square")
	}
}

func TestReadingOrderKeepsAlignedColumns(t *testing.T) {
	groups := []paragraphGroup{
		{indices: []int{0}, orderBox: image.Rect(0, 0, 20, 10)},
		{indices: []int{1}, orderBox: image.Rect(40, 0, 60, 10)},
		{indices: []int{2}, orderBox: image.Rect(0, 20, 20, 30)},
		{indices: []int{3}, orderBox: image.Rect(40, 20, 60, 30)},
	}
	var order []int
	for _, g := range projectionOrder(groups) {
		order = append(order, g.indices...)
	}
	if !reflect.DeepEqual(order, []int{0, 2, 1, 3}) {
		t.Fatal(order)
	}
}

func TestEmptyParagraphDetectionUsesWholePage(t *testing.T) {
	result := &Result{Width: 200, Height: 100, Lines: []Line{
		{Left: 0, Top: 0, Right: 19, Bottom: 19, Text: "first"},
		{Left: 100, Top: 50, Right: 119, Bottom: 69, Text: "second"},
	}}
	groupParagraphs(result, nil, 0)
	if len(result.Paragraphs) != 1 || !reflect.DeepEqual(result.Paragraphs[0].LineIndices, []int{0, 1}) {
		t.Fatal(result.Paragraphs)
	}
}

// A tilted line's text box can extend beyond its paragraph detection region.
// Sentence-separated pieces must sort using text bounds, including a singleton.
func TestReadingOrderUsesSentencePieceBounds(t *testing.T) {
	result := &Result{Width: 100, Height: 50, Lines: []Line{
		{Left: 0, Top: 0, Right: 20, Bottom: 20, Text: "A."},
		{Left: 60, Top: 0, Right: 80, Bottom: 20, Text: "R."},
		{Left: 0, Top: 18, Right: 20, Bottom: 38, Text: "B."},
	}}
	regions := []paragraphRegion{
		{box: image.Rect(0, 0, 21, 18)},
		{box: image.Rect(60, 0, 81, 21)},
		{box: image.Rect(0, 20, 21, 39)},
	}
	groupParagraphs(result, regions, 0)
	var texts []string
	for _, line := range result.Lines {
		texts = append(texts, line.Text)
	}
	if !reflect.DeepEqual(texts, []string{"A.", "B.", "R."}) {
		t.Fatal(texts)
	}
}
