package ocr

import (
	"errors"
	"image"
	"math"
	"sort"
	"strings"
	"wxocr/internal/polygonclip"
)

type paragraphRegion struct {
	box     image.Rectangle
	polygon []point
	score   float32
}

func paragraphRegions(prob []float32, w, h, rw, rh int, sx, sy float32) ([]paragraphRegion, error) {
	if len(prob) != w*h || rw < 1 || rh < 1 || rw > w || rh > h {
		return nil, errors.New("invalid paragraph output shape")
	}
	mask := make([]bool, rw*rh)
	for i, v := range prob {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, errors.New("non-finite paragraph output")
		}
		x, y := i%w, i/w
		if x < rw && y < rh {
			mask[y*rw+x] = v > float32(.3)
		}
	}
	contours := binaryContours(opening3(mask, rw, rh), rw, rh)
	var regions []paragraphRegion
	for _, c := range contours {
		c = simpleContour(c)
		if len(c) < 3 {
			continue
		}
		_, perimeter := paragraphMetrics(c)
		p := simplifyContour(c, float64(float32(perimeter*.002)))
		if len(p) < 4 {
			p = c
		}
		if len(p) < 3 {
			continue
		}
		box := image.Rectangle{}
		for _, v := range p {
			box = box.Union(image.Rect(v.X, v.Y, v.X+1, v.Y+1))
		}
		var sum float64
		count := 0
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if polygonContains(p, x, y) {
					sum += float64(prob[y*w+x])
					count++
				}
			}
		}
		if count == 0 || float64(float32(sum/float64(count))) < .3 {
			continue
		}
		area, perimeter := paragraphMetrics(p)
		if perimeter == 0 {
			continue
		}
		distance := float64(float32(float64(float32(area)) * 1.6 / float64(float32(perimeter))))
		expandedPoints := polygonclip.Offset(p, distance)
		if len(expandedPoints) < 3 {
			continue
		}
		q, _, side := minimumRectangleWithSize(expandedPoints)
		if side < 3 {
			continue
		}
		polygon := make([]point, len(q))
		box = image.Rectangle{}
		for i, v := range q {
			x := max(0, min(float64(float32(rw)*sx-1), float64(float32(v.x)*sx)))
			y := max(0, min(float64(float32(rh)*sy-1), float64(float32(v.y)*sy)))
			polygon[i] = point{x, y}
			box = box.Union(image.Rect(int(x), int(y), int(x)+1, int(y)+1))
		}
		regions = append(regions, paragraphRegion{box, polygon, float32(sum / float64(count))})
	}
	return regions, nil
}

func lineLess(a, b Line, direction int) bool {
	var primaryA, primaryB, secondaryA, secondaryB float32
	descending := false
	secondaryDescending := false
	switch direction {
	case 1:
		primaryA, primaryB, secondaryA, secondaryB = a.Right, b.Right, a.Bottom, b.Bottom
		descending = true
	case 2:
		secondaryDescending = true
		primaryA, primaryB, secondaryA, secondaryB = a.Bottom, b.Bottom, a.Left, b.Left
		descending = true
	case 3:
		secondaryDescending = true
		primaryA, primaryB, secondaryA, secondaryB = a.Left, b.Left, a.Top, b.Top
	default:
		primaryA, primaryB, secondaryA, secondaryB = a.Top, b.Top, a.Right, b.Right
	}
	if int(primaryA)/4 == int(primaryB)/4 {
		if secondaryDescending {
			return secondaryA > secondaryB
		}
		return secondaryA < secondaryB
	}
	if descending {
		return primaryA > primaryB
	}
	return primaryA < primaryB
}

func sortLines(blocks []Line, direction int) {
	sort.SliceStable(blocks, func(i, j int) bool { return lineLess(blocks[i], blocks[j], direction) })
}

// Paragraph contains the indices of its lines in Lines. Bounds are the
// union of those original-image line boxes, rather than detection padding.
type Paragraph struct {
	Left        float32 `json:"left"`
	Top         float32 `json:"top"`
	Right       float32 `json:"right"`
	Bottom      float32 `json:"bottom"`
	LineIndices []int   `json:"line_indices"`
}

type paragraphGroup struct {
	indices  []int
	box      image.Rectangle
	orderBox image.Rectangle
}

func uprightRect(b image.Rectangle, direction, width, height int) image.Rectangle {
	switch direction {
	case 1:
		return image.Rect(b.Min.Y, width-b.Max.X, b.Max.Y, width-b.Min.X)
	case 2:
		return image.Rect(width-b.Max.X, height-b.Max.Y, width-b.Min.X, height-b.Min.Y)
	case 3:
		return image.Rect(height-b.Max.Y, b.Min.X, height-b.Min.Y, b.Max.X)
	default:
		return b
	}
}

// Native XY-cut retains horizontal cuts only where column gaps in adjacent
// row bands do not overlap. This keeps aligned table columns in reading order.
func projectionOrder(groups []paragraphGroup) []paragraphGroup {
	if len(groups) < 2 {
		return groups
	}
	rows, _ := projectionBands(groups, 0)
	var bands [][]paragraphGroup
	current := rows[0]
	for i := 1; i < len(rows); i++ {
		_, leftGaps := projectionBands(rows[i-1], 1)
		_, rightGaps := projectionBands(rows[i], 1)
		// Native discards a cut when neither row has a column gap.
		overlap := len(leftGaps) == 0 && len(rightGaps) == 0
		for _, a := range leftGaps {
			for _, b := range rightGaps {
				if a[0] < b[1] && b[0] < a[1] {
					overlap = true
				}
			}
		}
		if !overlap {
			bands = append(bands, current)
			current = nil
		}
		current = append(current, rows[i]...)
	}
	bands = append(bands, current)
	var out []paragraphGroup
	for _, band := range bands {
		columns, _ := projectionBands(band, 1)
		if len(columns) > 1 {
			for _, column := range columns {
				out = append(out, projectionOrder(column)...)
			}
		} else {
			sort.SliceStable(band, func(i, j int) bool {
				if band[i].orderBox.Min.Y/4 != band[j].orderBox.Min.Y/4 {
					return band[i].orderBox.Min.Y < band[j].orderBox.Min.Y
				}
				return band[i].orderBox.Min.X < band[j].orderBox.Min.X
			})
			out = append(out, band...)
		}
	}
	return out
}

func projectionBands(groups []paragraphGroup, axis int) ([][]paragraphGroup, [][2]int) {
	sorted := append([]paragraphGroup(nil), groups...)
	start := func(g paragraphGroup) int {
		if axis == 0 {
			return g.orderBox.Min.Y
		}
		return g.orderBox.Min.X
	}
	end := func(g paragraphGroup) int {
		if axis == 0 {
			return g.orderBox.Max.Y
		}
		return g.orderBox.Max.X
	}
	sort.SliceStable(sorted, func(i, j int) bool { return start(sorted[i]) < start(sorted[j]) })
	var partitions [][]paragraphGroup
	var gaps [][2]int
	maxEnd := end(sorted[0])
	current := []paragraphGroup{sorted[0]}
	for _, g := range sorted[1:] {
		if start(g) >= maxEnd {
			partitions = append(partitions, current)
			gaps = append(gaps, [2]int{maxEnd, start(g)})
			current = nil
		}
		current = append(current, g)
		maxEnd = max(maxEnd, end(g))
	}
	return append(partitions, current), gaps
}

func groupParagraphs(result *Result, regions []paragraphRegion, direction int, geometry ...map[Line]quadrilateral) {
	blocks := result.Lines
	// ParagraphRecognizer falls back to a whole-page polygon when detection
	// produces no regions, before assigning the recognized text lines.
	if len(regions) == 0 && len(blocks) > 0 {
		regions = []paragraphRegion{{box: image.Rect(0, 0, result.Width+1, result.Height+1)}}
	}
	groups := make([]paragraphGroup, len(regions))
	for i, r := range regions {
		groups[i].orderBox = uprightRect(r.box, direction, result.Width, result.Height)
	}
	for i, b := range blocks {
		rect := image.Rect(int(b.Left), int(b.Top), int(b.Right)+1, int(b.Bottom)+1)
		area := float32(rect.Dx() * rect.Dy())
		best := -1
		coverage := float32(.75)
		for j, r := range regions {
			intersection := rect.Intersect(r.box)
			score := float32(intersection.Dx()*intersection.Dy()) / area
			if score >= coverage {
				best = j
				break
			}
		}
		if best < 0 {
			groups = append(groups, paragraphGroup{indices: []int{i}, box: rect, orderBox: uprightRect(rect, direction, result.Width, result.Height)})
		} else {
			groups[best].indices = append(groups[best].indices, i)
			groups[best].box = groups[best].box.Union(rect)
		}
	}
	var nonempty []paragraphGroup
	for _, g := range groups {
		if len(g.indices) > 0 {
			sort.SliceStable(g.indices, func(i, j int) bool { return lineLess(blocks[g.indices[i]], blocks[g.indices[j]], direction) })
			split := true
			for j, index := range g.indices {
				if j == len(g.indices)-1 || paragraphLineGap(blocks[index], blocks[g.indices[j+1]], direction, geometry) {
					text := []rune(blocks[index].Text)
					if len(text) == 0 || !strings.ContainsRune("。？！.?!", text[len(text)-1]) {
						split = false
						break
					}
				}
			}
			var piece paragraphGroup
			for j, index := range g.indices {
				if split && j > 0 && paragraphLineGap(blocks[g.indices[j-1]], blocks[index], direction, geometry) {
					piece.orderBox = uprightRect(piece.box, direction, result.Width, result.Height)
					nonempty = append(nonempty, piece)
					piece = paragraphGroup{}
				}
				b := blocks[index]
				piece.indices = append(piece.indices, index)
				piece.box = piece.box.Union(image.Rect(int(b.Left), int(b.Top), int(b.Right)+1, int(b.Bottom)+1))
			}
			piece.orderBox = uprightRect(piece.box, direction, result.Width, result.Height)
			if !split {
				piece.orderBox = g.orderBox
			}
			nonempty = append(nonempty, piece)
		}
	}
	groups = projectionOrder(nonempty)
	ordered := make([]Line, 0, len(blocks))
	result.Paragraphs = make([]Paragraph, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.indices, func(i, j int) bool { return lineLess(blocks[g.indices[i]], blocks[g.indices[j]], direction) })
		p := Paragraph{Left: float32(math.Inf(1)), Top: float32(math.Inf(1)), Right: float32(math.Inf(-1)), Bottom: float32(math.Inf(-1)), LineIndices: make([]int, 0, len(g.indices))}
		for _, i := range g.indices {
			b := blocks[i]
			p.Left = min(p.Left, b.Left)
			p.Top = min(p.Top, b.Top)
			p.Right = max(p.Right, b.Right)
			p.Bottom = max(p.Bottom, b.Bottom)
			p.LineIndices = append(p.LineIndices, len(ordered))
			ordered = append(ordered, b)
		}
		result.Paragraphs = append(result.Paragraphs, p)
	}
	result.Lines = ordered
}

// Native paragraph splitting compares adjacent center segments. Intersection
// has zero distance; otherwise average distances to the infinite center lines.
func paragraphLineGap(a, b Line, direction int, geometry []map[Line]quadrilateral) bool {
	quad := func(b Line) quadrilateral {
		if len(geometry) > 0 {
			if q, ok := geometry[0][b]; ok {
				return q
			}
		}
		return quadrilateral{{float64(b.Left), float64(b.Top)}, {float64(b.Right), float64(b.Top)}, {float64(b.Right), float64(b.Bottom)}, {float64(b.Left), float64(b.Bottom)}}
	}
	segment := func(q quadrilateral) (point, point, float64) {
		if direction%2 == 0 {
			q = quadrilateral{q[1], q[2], q[3], q[0]}
		}
		mid := func(a, b point) point {
			return point{float64(float32(float32(a.x)+float32(b.x)) * .5), float64(float32(float32(a.y)+float32(b.y)) * .5)}
		}
		edge := func(a, b point) float64 {
			return float64(int(math.Hypot(float64(float32(a.x)-float32(b.x)), float64(float32(a.y)-float32(b.y)))))
		}
		return mid(q[0], q[3]), mid(q[1], q[2]), (edge(q[0], q[3]) + edge(q[1], q[2])) * .5
	}
	a0, a1, ha := segment(quad(a))
	b0, b1, hb := segment(quad(b))
	cross := func(a, b, c point) float64 { return (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x) }
	if cross(a0, a1, b0)*cross(a0, a1, b1) <= 0 && cross(b0, b1, a0)*cross(b0, b1, a1) <= 0 {
		// Collinear segments need an overlapping projection as well.
		if max(min(a0.x, a1.x), min(b0.x, b1.x)) <= min(max(a0.x, a1.x), max(b0.x, b1.x)) && max(min(a0.y, a1.y), min(b0.y, b1.y)) <= min(max(a0.y, a1.y), max(b0.y, b1.y)) {
			return false
		}
	}
	distance := func(p, a, b point) float64 {
		dx, dy := b.x-a.x, b.y-a.y
		denominator := dx*dx + dy*dy
		if denominator == 0 {
			return math.Hypot(p.x-a.x, p.y-a.y)
		}
		t := ((p.x-a.x)*dx + (p.y-a.y)*dy) / denominator
		return math.Hypot(p.x-a.x-t*dx, p.y-a.y-t*dy)
	}
	gap := (distance(a0, b0, b1) + distance(a1, b0, b1) + distance(b0, a0, a1) + distance(b1, a0, a1)) * .25
	return gap >= float64(float32((ha+hb)*.4))
}
