package ocr

import (
	"context"
	"image"
	"math"
	"sort"
)

// Project ink across the upright crop at near source resolution. Ignore long
// rules in either axis so table borders cannot connect otherwise separate lines.
func splitTextQuad(src image.Image, q quadrilateral) []quadrilateral {
	iw := math.Hypot(q[1].x-q[0].x, q[1].y-q[0].y)
	ih := math.Hypot(q[3].x-q[0].x, q[3].y-q[0].y)
	if ih < 24 || iw < 20 {
		return nil
	}
	scale := math.Min(1, math.Min(1600/iw, 512/ih))
	w, h := max(1, int(iw*scale)), max(1, int(ih*scale))
	if h < 20 {
		return nil
	}
	transform := cropTransform(q, w, h)
	rgb := rgbSampler(src)
	ink := make([]bool, w*h)
	cols := make([]int, w)
	rows := make([]int, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			den := transform[6]*float64(x) + transform[7]*float64(y) + transform[8]
			sx := int(math.Round((transform[0]*float64(x) + transform[1]*float64(y) + transform[2]) / den))
			sy := int(math.Round((transform[3]*float64(x) + transform[4]*float64(y) + transform[5]) / den))
			if !image.Pt(sx, sy).In(src.Bounds()) {
				continue
			}
			c := rgb(sx, sy)
			if (299*int(c[0])+587*int(c[1])+114*int(c[2]))/1000 < 160 {
				ink[y*w+x] = true
				cols[x]++
				rows[y]++
			}
		}
	}
	// A rule passing through a text crop means neighboring table cells were
	// merged. Verify its continuation outside the crop to distinguish it from
	// vertical strokes in Chinese characters and digits.
	darkAt := func(x, y int) bool {
		den := transform[6]*float64(x) + transform[7]*float64(y) + transform[8]
		sx := int(math.Round((transform[0]*float64(x) + transform[1]*float64(y) + transform[2]) / den))
		sy := int(math.Round((transform[3]*float64(x) + transform[4]*float64(y) + transform[5]) / den))
		if !image.Pt(sx, sy).In(src.Bounds()) {
			return false
		}
		c := rgb(sx, sy)
		return (299*int(c[0])+587*int(c[1])+114*int(c[2]))/1000 < 160
	}
	type rule struct{ start, end int }
	var rules []rule
	for x := max(3, w/50); x < w-max(3, w/50); x++ {
		if cols[x]*10 < h*8 {
			continue
		}
		continuation := true
		for _, band := range [][2]int{{-h / 2, -h / 10}, {h + h/10, h + h/2}} {
			hits, total := 0, 0
			for y := band[0]; y <= band[1]; y += 2 {
				total++
				if darkAt(x-1, y) || darkAt(x, y) || darkAt(x+1, y) {
					hits++
				}
			}
			if total == 0 || hits*10 < total*9 {
				continuation = false
				break
			}
		}
		if !continuation {
			continue
		}
		if len(rules) > 0 && x-rules[len(rules)-1].end < 3 {
			rules[len(rules)-1].end = x + 1
		} else {
			rules = append(rules, rule{x, x + 1})
		}
	}
	if len(rules) > 0 {
		lerp := func(a, b point, t float64) point { return point{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t} }
		var pieces []quadrilateral
		start := 0
		for i := 0; i <= len(rules); i++ {
			end := w
			if i < len(rules) {
				end = rules[i].start - 2
			}
			if end-start >= max(12, h/3) {
				count := 0
				for y := 0; y < h; y++ {
					for x := start; x < end; x++ {
						if ink[y*w+x] && cols[x]*10 < h*7 {
							count++
						}
					}
				}
				if count >= max(20, (end-start)*h/100) {
					a, b := float64(start)/float64(w), float64(end)/float64(w)
					piece := quadrilateral{lerp(q[0], q[1], a), lerp(q[0], q[1], b), lerp(q[3], q[2], b), lerp(q[3], q[2], a)}
					children := splitTextQuad(src, piece)
					if len(children) > 0 {
						pieces = append(pieces, children...)
					} else {
						pieces = append(pieces, piece)
					}
				}
			}
			if i < len(rules) {
				start = rules[i].end + 2
			}
		}
		if len(pieces) > 1 {
			return pieces
		}
	}
	projection := make([]int, h)
	for y := 0; y < h; y++ {
		if rows[y]*10 > w*7 {
			continue
		}
		for x := 0; x < w; x++ {
			if cols[x]*10 < h*7 && ink[y*w+x] {
				projection[y]++
			}
		}
	}
	type band struct{ start, end int }
	var bands []band
	for y := 0; y < h; {
		if projection[y] < max(2, w/150) {
			y++
			continue
		}
		start := y
		for y < h && projection[y] >= max(2, w/150) {
			y++
		}
		if len(bands) > 0 && start-bands[len(bands)-1].end <= max(2, h/100) {
			bands[len(bands)-1].end = y
		} else {
			bands = append(bands, band{start, y})
		}
	}
	var useful []band
	for _, b := range bands {
		if b.end-b.start >= max(5, int(6*scale)) {
			useful = append(useful, b)
		}
	}
	if len(useful) < 2 {
		return nil
	}
	heights := make([]int, len(useful))
	for i, b := range useful {
		heights[i] = b.end - b.start
	}
	sort.Ints(heights)
	median := heights[len(heights)/2]
	// Large differences usually indicate diacritics, punctuation or a stamp.
	for _, b := range useful {
		if b.end-b.start < median/2 || b.end-b.start > median*2 {
			return nil
		}
	}
	var out []quadrilateral
	lerp := func(a, b point, t float64) point { return point{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t} }
	for i, b := range useful {
		top, bottom := max(0, b.start-2), min(h, b.end+2)
		if i > 0 {
			top = max(top, (useful[i-1].end+b.start)/2)
		}
		if i+1 < len(useful) {
			bottom = min(bottom, (b.end+useful[i+1].start)/2)
		}
		t0, t1 := float64(top)/float64(h), float64(bottom)/float64(h)
		out = append(out, quadrilateral{lerp(q[0], q[3], t0), lerp(q[1], q[2], t0), lerp(q[1], q[2], t1), lerp(q[0], q[3], t1)})
	}
	return out
}

func splitDetectedRegions(ctx context.Context, src image.Image, regions []region, diagnostics []Diagnostic, direction int) ([]region, []Diagnostic) {
	// Restrict this refinement to pages for which downscaling can merge lines;
	// small native-model inputs retain their established geometry and decoding.
	if min(src.Bounds().Dx(), src.Bounds().Dy()) < 1600 {
		return regions, diagnostics
	}
	layout := tableContinuationLayout(src, direction)
	count := len(regions)
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			break
		}
		if regions[i].score < .3 {
			continue
		}
		q := regions[i].quad.oriented(direction)
		for j := range q {
			q[j].x += float64(src.Bounds().Min.X)
			q[j].y += float64(src.Bounds().Min.Y)
		}
		pieces := splitTextQuad(src, q)
		if len(pieces) == 0 {
			pieces = []quadrilateral{q}
		}
		var refined []quadrilateral
		method := "projection"
		for _, piece := range pieces {
			children := layout.split(piece)
			if len(children) > 0 {
				refined = append(refined, children...)
				method = "table_continuation"
			} else {
				refined = append(refined, piece)
			}
		}
		pieces = refined
		if len(pieces) < 2 {
			continue
		}
		diagnostics[i].Reason = "split_parent"
		for _, piece := range pieces {
			r := regions[i]
			r.quad = piece
			d := diagnostics[i]
			d.Reason = ""
			d.SplitMethod = method
			parent := i
			d.ParentRegion = &parent
			for j := range r.quad {
				r.quad[j].x -= float64(src.Bounds().Min.X)
				r.quad[j].y -= float64(src.Bounds().Min.Y)
				d.Quad[j] = [2]float64{r.quad[j].x, r.quad[j].y}
			}
			// Area is used only as a noise floor; local pieces retain the detector
			// evidence of their parent and scale area by their crop-height fraction.
			r.area /= float64(len(pieces))
			d.DetectionArea = r.area
			// Recognition expects the detector's original corner order before applying
			// page orientation. Invert the rotation applied above.
			r.quad = r.quad.oriented((4 - direction) % 4)
			regions = append(regions, r)
			diagnostics = append(diagnostics, d)
		}
	}
	return regions, diagnostics
}
