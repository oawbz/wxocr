package ocr

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestSplitTextQuadIgnoresTableRules(t *testing.T) {
	src := image.NewGray(image.Rect(7, 11, 247, 131))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	// Two text bands within a ruled cell. Separate glyphs prevent a line of ink
	// from being mistaken for the horizontal border itself.
	for _, y := range []int{33, 81} {
		for x := 25; x < 215; x += 25 {
			draw.Draw(src, image.Rect(x, y, x+12, y+18), image.NewUniform(color.Black), image.Point{}, draw.Src)
		}
	}
	for _, r := range []image.Rectangle{image.Rect(8, 12, 246, 14), image.Rect(8, 128, 246, 130), image.Rect(8, 12, 10, 130), image.Rect(244, 12, 246, 130)} {
		draw.Draw(src, r, image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	q := quadrilateral{{7, 11}, {246, 11}, {246, 130}, {7, 130}}
	pieces := splitTextQuad(src, q)
	if len(pieces) != 2 {
		t.Fatalf("expected two text bands, got %d", len(pieces))
	}
	_, top0, _, bottom0 := pieces[0].bounds()
	_, top1, _, bottom1 := pieces[1].bounds()
	if top0 > 33 || bottom0 < 50 || top1 > 81 || bottom1 < 98 || bottom0 >= top1 {
		t.Fatal("split cuts or overlaps text", pieces)
	}
	// One band must remain one recognizer input.
	draw.Draw(src, image.Rect(15, 75, 235, 110), image.NewUniform(color.White), image.Point{}, draw.Src)
	if len(splitTextQuad(src, q)) != 0 {
		t.Fatal("single line was split")
	}
}
func TestHighResolutionTilesAreBounded(t *testing.T) {
	b := image.Rect(7, 11, 2487, 3517)
	tiles := detectionTiles(b)
	if len(tiles) != 13 {
		t.Fatalf("expected whole image and twelve local passes: %d", len(tiles))
	}
	coverage := image.Rectangle{}
	for _, tile := range tiles[1:] {
		if !tile.In(b) || tile.Dx() > 1280 || tile.Dy() > 1280 {
			t.Fatal(tile)
		}
		coverage = coverage.Union(tile)
	}
	if coverage != b {
		t.Fatal("uncovered page area")
	}
}
func TestMergePrefersCompleteLocalText(t *testing.T) {
	global := Line{Left: 10, Top: 10, Right: 110, Bottom: 30, Text: "错字", Confidence: .99}
	local := Line{Left: 11, Top: 11, Right: 109, Bottom: 29, Text: "正确", Confidence: .9}
	diag := func(l Line, pass int) Diagnostic {
		return Diagnostic{Pass: pass, Quad: [4][2]float64{{float64(l.Left), float64(l.Top)}, {float64(l.Right), float64(l.Top)}, {float64(l.Right), float64(l.Bottom)}, {float64(l.Left), float64(l.Bottom)}}, Text: l.Text, Reason: "accepted"}
	}
	r := Result{Lines: []Line{global, local}, Diagnostics: []Diagnostic{diag(global, 0), diag(local, 1)}}
	mergePassResults(&r)
	if len(r.Lines) != 1 || r.Lines[0].Text != "正确" || r.Diagnostics[0].Reason != "duplicate" {
		t.Fatalf("coarse text won %+v", r)
	}
	// Two cells inside an erroneous merged detection replace that coarse box.
	joined := Line{Left: 0, Top: 0, Right: 200, Bottom: 30, Text: "错合框", Confidence: .99}
	a := Line{Left: 0, Top: 0, Right: 95, Bottom: 30, Text: "甲", Confidence: .9}
	b := Line{Left: 105, Top: 0, Right: 200, Bottom: 30, Text: "乙", Confidence: .9}
	r = Result{Lines: []Line{joined, a, b}, Diagnostics: []Diagnostic{diag(joined, 0), diag(a, 1), diag(b, 1)}}
	mergePassResults(&r)
	if len(r.Lines) != 2 {
		t.Fatalf("joined box retained %+v", r.Lines)
	}
}

func TestSplitMergedCellsRequiresContinuousRule(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 240, 180))
	draw.Draw(src, src.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for _, x := range []int{40, 65, 140, 165} {
		draw.Draw(src, image.Rect(x, 70, x+10, 90), image.NewUniform(color.Black), image.Point{}, draw.Src)
	}
	q := quadrilateral{{20, 60}, {210, 60}, {210, 100}, {20, 100}}
	if len(splitTextQuad(src, q)) != 0 {
		t.Fatal("words in a single line split without a table rule")
	}
	draw.Draw(src, image.Rect(110, 10, 113, 150), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if got := len(splitTextQuad(src, q)); got != 2 {
		t.Fatalf("merged cells not separated: %d", got)
	}
}
func TestDominantTableKeepsFooterRowsTogether(t *testing.T) {
	lines := []Line{
		{Left: 10, Top: 10, Right: 90, Bottom: 30, Text: "标题"},
		{Left: 10, Top: 120, Right: 90, Bottom: 140, Text: "表内"},
		{Left: 10, Top: 220, Right: 90, Bottom: 240, Text: "12左"},
		{Left: 110, Top: 220, Right: 190, Bottom: 240, Text: "12右"},
		{Left: 10, Top: 250, Right: 90, Bottom: 270, Text: "13左"},
		{Left: 110, Top: 250, Right: 190, Bottom: 270, Text: "13右"},
	}
	got := gatherTablePageOrder(lines, []int{2, 4, 0, 1, 3, 5}, ruledTable{left: 0, right: 200, rows: []int{100, 200}}, 200, 300, 200, 300, 0, 200, 300)
	for i, index := range got {
		if index != i {
			t.Fatal("header, table and footer reading order", got)
		}
	}
}

func TestDetectionPruningPreservesFallbackOrder(t *testing.T) {
	makeDiagnostic := func(pass int, bottom float64, area float64) Diagnostic {
		return Diagnostic{Pass: pass, Tile: image.Rect(0, 0, 1280, 1280), Quad: [4][2]float64{{100, 100}, {140, 100}, {140, bottom}, {100, bottom}}, DetectionScore: .82, DetectionArea: area}
	}
	ds := []Diagnostic{makeDiagnostic(0, 125, 400), makeDiagnostic(1, 120, 377), makeDiagnostic(2, 125, 493)}
	pruneDetectionDuplicates(ds, 2480, 3506)
	if ds[0].Reason != "" {
		t.Fatal("whole-page fallback pruned")
	}
	if ds[1].Reason != "redundant_detection" || ds[1].DuplicateOf == nil || *ds[1].DuplicateOf != 2 {
		t.Fatalf("preferred crop missing %+v", ds)
	}
	order := recognitionOrder(ds)
	if order[0] != 0 || order[1] != 2 || order[2] != 1 {
		t.Fatal("fallback attempted before preferred crop", order)
	}
}
