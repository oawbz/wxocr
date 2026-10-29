package ocr

import (
	"context"
	"image"
	"math"
	"sort"
	"strings"
)

type fragmentItem struct {
	line, upright Line
	candidate     bool
	diagnostic    int
	area          float64
}

func uprightLine(l Line, direction, width, height int) Line {
	switch direction {
	case 1:
		l.Left, l.Top, l.Right, l.Bottom = l.Top, float32(width)-l.Right, l.Bottom, float32(width)-l.Left
	case 2:
		l.Left, l.Top, l.Right, l.Bottom = float32(width)-l.Right, float32(height)-l.Bottom, float32(width)-l.Left, float32(height)-l.Top
	case 3:
		l.Left, l.Top, l.Right, l.Bottom = float32(height)-l.Bottom, l.Left, float32(height)-l.Top, l.Right
	}
	return l
}
func adjacentFragments(a, b Line) bool {
	if a.Left > b.Left {
		a, b = b, a
	}
	ah, bh := a.Bottom-a.Top, b.Bottom-b.Top
	aw, bw := a.Right-a.Left, b.Right-b.Left
	if min(aw, bw, ah, bh) <= 0 || min(ah, bh) < max(ah, bh)*.6 {
		return false
	}
	if float32(math.Abs(float64((a.Top+a.Bottom-b.Top-b.Bottom)/2))) > max(ah, bh)*.3 {
		return false
	}
	if min(a.Bottom, b.Bottom)-max(a.Top, b.Top) < min(ah, bh)*.65 {
		return false
	}
	gap := b.Left - a.Right
	return gap <= min(ah, bh)*.45 && gap >= -min(aw, bw)*.45
}

// A continuous rule extending beyond both glyphs separates cells. Probe original
// pixels only near a proposed join; do not infer field labels or table templates.
func fragmentSeparator(src image.Image, a, b Line, direction int) bool {
	if a.Left > b.Left {
		a, b = b, a
	}
	h := max(a.Bottom-a.Top, b.Bottom-b.Top)
	x0, x1 := int(math.Floor(float64(min(a.Right, b.Left))))-2, int(math.Ceil(float64(max(a.Right, b.Left))))+2
	y0, y1 := int(float32(min(a.Top, b.Top))-h*.4), int(float32(max(a.Bottom, b.Bottom))+h*.4)
	bounds := src.Bounds()
	rgb := rgbSampler(src)
	dark := func(x, y int) bool {
		ox, oy := x, y
		switch direction {
		case 1:
			ox, oy = bounds.Dx()-1-y, x
		case 2:
			ox, oy = bounds.Dx()-1-x, bounds.Dy()-1-y
		case 3:
			ox, oy = y, bounds.Dy()-1-x
		}
		ox += bounds.Min.X
		oy += bounds.Min.Y
		if !image.Pt(ox, oy).In(bounds) {
			return false
		}
		c := rgb(ox, oy)
		return darkRulePixel(c[0], c[1], c[2])
	}
	for x := x0; x <= x1; x++ {
		hits, total := 0, 0
		for y := y0; y <= y1; y += max(1, int(h)/20) {
			total++
			if dark(x-1, y) || dark(x, y) || dark(x+1, y) {
				hits++
			}
		}
		if total > 0 && hits*100 >= total*85 {
			return true
		}
	}
	return false
}
func compactText(s string) string { return strings.Join(strings.Fields(s), "") }

// Re-read spatially adjacent fragments from original pixels. Every component's
// transcription must be contained in the strong merged result; failed proposals
// retain all original observations. No expected words, formats or coordinates.
func (e *Engine) joinRecognizedFragments(ctx context.Context, src image.Image, r *Result, direction int, geometry map[Line]quadrilateral, timing *stageTimer) (bool, error) {
	var items []fragmentItem
	add := func(line Line, candidate bool) {
		for i, d := range r.Diagnostics {
			if !diagnosticMatches(d, line) || (candidate && d.Reason != "candidate") || (!candidate && d.Reason != "accepted") {
				continue
			}
			if candidate && (d.Evidence < .8 || touchesTileBoundary(d, r.Width, r.Height)) {
				return
			}
			items = append(items, fragmentItem{line, uprightLine(line, direction, r.Width, r.Height), candidate, i, d.DetectionArea})
			return
		}
	}
	for _, c := range r.Candidates {
		add(c.Line, true)
	}
	if len(items) == 0 {
		return false, nil
	}
	for _, line := range r.Lines {
		add(line, false)
	}
	parent := make([]int, len(items))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for i := range items {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		for j := i + 1; j < len(items); j++ {
			if adjacentFragments(items[i].upright, items[j].upright) && !fragmentSeparator(src, items[i].upright, items[j].upright, direction) {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]int{}
	for i := range items {
		groups[find(i)] = append(groups[find(i)], i)
	}
	// Stable order makes evidence and diagnostic indices reproducible.
	keys := make([]int, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	removed := map[Line]bool{}
	var newLines []Line
	var newCandidates []Candidate
	changed := false
	for _, key := range keys {
		group := groups[key]
		if len(group) < 2 || len(group) > 6 {
			continue
		}
		hasCandidate, hasBody := false, false
		box := items[group[0]].line
		score := box.DetectionScore
		area := float64(0)
		for _, i := range group {
			it := items[i]
			hasCandidate = hasCandidate || it.candidate
			hasBody = hasBody || !it.candidate
			area += it.area
			score = min(score, it.line.DetectionScore)
			box.Left = min(box.Left, it.line.Left)
			box.Top = min(box.Top, it.line.Top)
			box.Right = max(box.Right, it.line.Right)
			box.Bottom = max(box.Bottom, it.line.Bottom)
		}
		if !hasCandidate {
			continue
		}
		if err := ctx.Err(); err != nil {
			return changed, err
		}
		q := quadrilateral{{float64(box.Left), float64(box.Top)}, {float64(box.Right), float64(box.Top)}, {float64(box.Right), float64(box.Bottom)}, {float64(box.Left), float64(box.Bottom)}}
		absolute := q
		for i := range absolute {
			absolute[i].x += float64(src.Bounds().Min.X)
			absolute[i].y += float64(src.Bounds().Min.Y)
		}
		pixels := recognitionQuadTensor(src, absolute.oriented(direction), true)
		timing.mark("recognizer_preprocess")
		data, shape, err := e.recognizer.run(pixels)
		if err != nil {
			return changed, err
		}
		timing.mark("recognizer_inference")
		text, confidence, evidence, err := e.decodeRecognition(data, shape)
		if err != nil {
			return changed, err
		}
		timing.mark("recognizer_decode")
		if evidence < .8 || text == "" {
			continue
		}
		covers := true
		for _, i := range group {
			if compactText(items[i].line.Text) == "" || !strings.Contains(compactText(text), compactText(items[i].line.Text)) {
				covers = false
				break
			}
		}
		if !covers {
			continue
		}
		reasons := candidateReasons(score, area, evidence)
		if hasBody && len(reasons) > 0 {
			continue
		}
		box.Text, box.Confidence, box.DetectionScore = text, confidence, score
		d := Diagnostic{Pass: 0, Tile: image.Rect(0, 0, r.Width, r.Height), Text: text, Evidence: evidence, DetectionScore: score, DetectionArea: area, SplitMethod: "adjacent_fragments", CropWidth: len(pixels) / (3 * 32), CropHeight: 32}
		for j, p := range q {
			d.Quad[j] = [2]float64{p.x, p.y}
		}
		if diagnosticCrops(ctx) {
			d.CropPNG, err = tensorPNG(pixels)
			if err != nil {
				return changed, err
			}
		}
		if len(reasons) == 0 {
			d.Reason = "accepted"
			newLines = append(newLines, box)
			geometry[box] = q
		} else {
			d.Reason = "candidate"
			newCandidates = append(newCandidates, Candidate{Line: box, Reasons: reasons, Evidence: evidence})
		}
		joinedIndex := len(r.Diagnostics)
		for _, i := range group {
			removed[items[i].line] = true
			r.Diagnostics[items[i].diagnostic].Reason = "duplicate"
			target := joinedIndex
			r.Diagnostics[items[i].diagnostic].DuplicateOf = &target
		}
		r.Diagnostics = append(r.Diagnostics, d)
		changed = true
	}
	if changed {
		lines := r.Lines[:0]
		for _, l := range r.Lines {
			if !removed[l] {
				lines = append(lines, l)
			}
		}
		r.Lines = append(lines, newLines...)
		candidates := r.Candidates[:0]
		for _, c := range r.Candidates {
			if !removed[c.Line] {
				candidates = append(candidates, c)
			}
		}
		r.Candidates = append(candidates, newCandidates...)
	}
	return changed, nil
}
