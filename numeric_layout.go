package ocr

import (
	"image"
	"math"
	"regexp"
	"sort"
)

type continuationLayout struct {
	source                   image.Image
	columns                  []float64
	bottom                   float64
	direction, width, height int
	origin                   image.Point
}

// Extend recurring column boundaries only below a dominant ruled table. This
// provides geometric separation for unruled continuation rows, without reading
// field labels, enforcing a number length, or supplying missing characters.
func tableContinuationLayout(src image.Image, direction int) continuationLayout {
	layout := continuationLayout{source: src, direction: direction, width: src.Bounds().Dx(), height: src.Bounds().Dy(), origin: src.Bounds().Min}
	ink, w, h := uprightInk(src, direction)
	tables := ruledTables(ink, w, h)
	if len(tables) != 1 {
		return layout
	}
	t := tables[0]
	if t.right-t.left < w*7/10 || t.rows[len(t.rows)-1]-t.rows[0] < h/2 {
		return layout
	}
	var positions []int
	for row := 0; row+1 < len(t.rows); row++ {
		for _, x := range tableColumns(ink, w, h, t, row) {
			if x > t.left+3 && x < t.right-3 {
				positions = append(positions, x)
			}
		}
	}
	sort.Ints(positions)
	uw, uh := layout.width, layout.height
	if direction%2 != 0 {
		uw, uh = uh, uw
	}
	for start := 0; start < len(positions); {
		end := start + 1
		for end < len(positions) && positions[end]-positions[start] <= 3 {
			end++
		}
		if end-start >= max(2, (len(t.rows)-1)/2) {
			layout.columns = append(layout.columns, float64(positions[(start+end-1)/2])*float64(uw)/float64(w))
		}
		start = end
	}
	layout.bottom = float64(t.rows[len(t.rows)-1]) * float64(uh) / float64(h)
	return layout
}
func (l continuationLayout) upright(p point) point {
	x, y := p.x-float64(l.origin.X), p.y-float64(l.origin.Y)
	switch l.direction {
	case 1:
		return point{y, float64(l.width) - x}
	case 2:
		return point{float64(l.width) - x, float64(l.height) - y}
	case 3:
		return point{float64(l.height) - y, x}
	}
	return point{x, y}
}
func (l continuationLayout) split(q quadrilateral) []quadrilateral {
	if len(l.columns) < 3 {
		return nil
	}
	a, b, c, d := l.upright(q[0]), l.upright(q[1]), l.upright(q[2]), l.upright(q[3])
	if (a.y+b.y+c.y+d.y)/4 <= l.bottom || b.x-a.x < 30 {
		return nil
	}
	// Do not apply table alignment to distant notes or unrelated material.
	if (a.y+b.y+c.y+d.y)/4 > l.bottom+float64(l.height)*.2 {
		return nil
	}
	height := (math.Abs(d.y-a.y) + math.Abs(c.y-b.y)) * .5
	fractions := []float64{0}
	for _, x := range l.columns {
		if x-a.x > height*.4 && b.x-x > height*.4 {
			fractions = append(fractions, (x-a.x)/(b.x-a.x))
		}
	}
	if len(fractions) == 1 {
		return nil
	}
	fractions = l.snapCuts(q, fractions)
	fractions = append(fractions, 1)
	lerp := func(a, b point, t float64) point { return point{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t} }
	var out []quadrilateral
	for i := 0; i+1 < len(fractions); i++ {
		t0, t1 := fractions[i], fractions[i+1]
		out = append(out, quadrilateral{lerp(q[0], q[1], t0), lerp(q[0], q[1], t1), lerp(q[3], q[2], t1), lerp(q[3], q[2], t0)})
	}
	return out
}

// Snap inferred column boundaries into nearby whitespace. This keeps glyphs
// intact when scan skew or continuation-row offsets differ from the grid.
func (l continuationLayout) snapCuts(q quadrilateral, fractions []float64) []float64 {
	if l.source == nil {
		return fractions
	}
	width := int(math.Hypot(q[1].x-q[0].x, q[1].y-q[0].y))
	height := int(math.Hypot(q[3].x-q[0].x, q[3].y-q[0].y))
	if width < 20 || height < 8 || width > 4096 || height > 256 {
		return fractions
	}
	transform := cropTransform(q, width, height)
	rgb := rgbSampler(l.source)
	ink := make([]int, width)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			den := transform[6]*float64(x) + transform[7]*float64(y) + transform[8]
			sx := int(math.Round((transform[0]*float64(x) + transform[1]*float64(y) + transform[2]) / den))
			sy := int(math.Round((transform[3]*float64(x) + transform[4]*float64(y) + transform[5]) / den))
			if !image.Pt(sx, sy).In(l.source.Bounds()) {
				continue
			}
			c := rgb(sx, sy)
			if (299*int(c[0])+587*int(c[1])+114*int(c[2]))/1000 < 140 {
				ink[x]++
			}
		}
	}
	out := append([]float64(nil), fractions...)
	for i := 1; i < len(out); i++ {
		target := int(fractions[i] * float64(width))
		radius := height * 3 / 4
		left, right := max(1, target-radius), min(width-1, target+radius)
		best, bestScore := target, math.Inf(-1)
		for x := left; x < right; {
			if ink[x] >= 2 {
				x++
				continue
			}
			start := x
			for x < right && ink[x] < 2 {
				x++
			}
			if x-start < 3 {
				continue
			}
			mid := (start + x) / 2
			score := float64(x-start) - .35*math.Abs(float64(mid-target))
			if score > bestScore {
				best, bestScore = mid, score
			}
		}
		if bestScore > 0 {
			out[i] = float64(best) / float64(width)
		}
	}
	// A narrow column must not collapse when two boundaries snap to one gap.
	for i := 1; i < len(out); i++ {
		if out[i] <= out[i-1]+.01 {
			return fractions
		}
	}
	return out
}

// Review-only validation: never pad an ID, repair a checksum, or supply hidden
// date digits. Original OCR text remains unchanged for incomplete source cells.
func continuationReviewReasons(text string) []string {
	reasons := []string{"inferred_table_column"}
	if numericDateLike.MatchString(text) && !completeDate.MatchString(text) {
		reasons = append(reasons, "incomplete_or_ambiguous_date")
	}
	if numericEdgeFragment.MatchString(text) {
		reasons = append(reasons, "possible_boundary_fragment")
	}
	return reasons
}

var numericDateLike = regexp.MustCompile(`[0-9]{4}[-/][0-9]{2}[-/]`)
var completeDate = regexp.MustCompile(`^[0-9]{4}[-/][0-9]{2}[-/][0-9]{2}$`)
var numericEdgeFragment = regexp.MustCompile(`^[(:：][0-9]|^[0-9]{5}[-/][0-9]{2}`)
