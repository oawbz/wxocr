package ocr

import (
	"image"
	"math"
	"sort"
)

type point struct{ x, y float64 }
type quadrilateral [4]point

func cross(a, b, c image.Point) int64 {
	return int64(b.X-a.X)*int64(c.Y-a.Y) - int64(b.Y-a.Y)*int64(c.X-a.X)
}

// Only row endpoints are needed: all interior pixels lie inside their hull.
func convexHull(points []image.Point) []image.Point {
	sort.Slice(points, func(i, j int) bool {
		if points[i].X != points[j].X {
			return points[i].X < points[j].X
		}
		return points[i].Y < points[j].Y
	})
	unique := points[:0]
	for _, p := range points {
		if len(unique) == 0 || p != unique[len(unique)-1] {
			unique = append(unique, p)
		}
	}
	if len(unique) < 3 {
		return unique
	}
	hull := make([]image.Point, 0, 2*len(unique))
	for _, p := range unique {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lower := len(hull)
	for i := len(unique) - 2; i >= 0; i-- {
		p := unique[i]
		for len(hull) > lower && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	return hull[:len(hull)-1]
}

// minimumRectangle follows OpenCV rotating calipers using float32 arithmetic.
// Rounding is observable: a width just below an integer changes native OCR crops.
// Adapted from OpenCV 4.5.5 rotcalipers.cpp; see LICENSE.opencv.
func minimumRectangle(points []image.Point) (quadrilateral, float64) {
	q, area, _ := minimumRectangleWithSize(points)
	return q, area
}

// Keep the caliper side length before corners are rounded for size filtering.
func minimumRectangleWithSize(points []image.Point) (quadrilateral, float64, float32) {
	hull := convexHull(points)
	// OpenCV's row-ordered pixel input starts its hull at the bottommost
	// point on the right edge. Caliper tie-breaking depends on this origin.
	origin := 0
	for i, p := range hull {
		if p.X > hull[origin].X || (p.X == hull[origin].X && p.Y > hull[origin].Y) {
			origin = i
		}
	}
	hull = append(append([]image.Point(nil), hull[origin:]...), hull[:origin]...)

	n := len(hull)
	if n < 3 {
		return quadrilateral{}, 0, 0
	}
	type vec struct{ x, y float32 }
	pts := make([]vec, n)
	edges := make([]vec, n)
	inverse := make([]float32, n)
	left, right, top, bottom := 0, 0, 0, 0
	for i, p := range hull {
		pts[i] = vec{float32(p.X), float32(p.Y)}
		if p.X < hull[left].X {
			left = i
		}
		if p.X > hull[right].X {
			right = i
		}
		if p.Y > hull[top].Y {
			top = i
		}
		if p.Y < hull[bottom].Y {
			bottom = i
		}
		next := hull[(i+1)%n]
		dx, dy := float64(next.X-p.X), float64(next.Y-p.Y)
		edges[i] = vec{float32(dx), float32(dy)}
		inverse[i] = float32(1 / math.Hypot(dx, dy))
	}
	seq := [4]int{bottom, right, top, left}
	best := float32(math.MaxFloat32)
	var a, b, w, h float32
	bl, bb := 0, 0
	for k := 0; k < n; k++ {
		e0, e1, e2, e3 := edges[seq[0]], edges[seq[1]], edges[seq[2]], edges[seq[3]]
		rotated := [4]vec{e0, {e1.y, -e1.x}, {-e2.x, -e2.y}, {-e3.y, e3.x}}
		main := 0
		for i := 1; i < 4; i++ {
			if float32(rotated[i].y*rotated[main].x)-float32(rotated[i].x*rotated[main].y) < 0 {
				main = i
			}
		}
		index := seq[main]
		lx, ly := float32(edges[index].x*inverse[index]), float32(edges[index].y*inverse[index])
		var ba, bc float32
		switch main {
		case 0:
			ba, bc = lx, ly
		case 1:
			ba, bc = ly, -lx
		case 2:
			ba, bc = -lx, -ly
		case 3:
			ba, bc = -ly, lx
		}
		seq[main] = (seq[main] + 1) % n
		dx, dy := pts[seq[1]].x-pts[seq[3]].x, pts[seq[1]].y-pts[seq[3]].y
		width := float32(dx*ba) + float32(dy*bc)
		dx, dy = pts[seq[2]].x-pts[seq[0]].x, pts[seq[2]].y-pts[seq[0]].y
		height := float32(-dx*bc) + float32(dy*ba)
		area := float32(width * height)
		if area <= best {
			best = area
			a, b, w, h = ba, bc, width, height
			bl, bb = seq[3], seq[0]
		}
	}
	c1 := float32(a*pts[bl].x) + float32(pts[bl].y*b)
	c2 := float32(-b*pts[bb].x) + float32(pts[bb].y*a)
	inv := float32(1 / (float32(a*a) + float32(b*b)))
	px := float32((float32(c1*a) - float32(c2*b)) * inv)
	py := float32((float32(a*c2) + float32(b*c1)) * inv)
	vx, vy, wx, wy := float32(a*w), float32(b*w), float32(-b*h), float32(a*h)
	cx := px + float32(float32(vx+wx)*0.5)
	cy := py + float32(float32(vy+wy)*0.5)
	rw, rh := float32(math.Hypot(float64(vx), float64(vy))), float32(math.Hypot(float64(wx), float64(wy)))
	angle := float32(float64(float32(math.Atan2(float64(vy), float64(vx)))) * 180 / math.Pi)
	theta := float64(angle) * math.Pi / 180
	sine, cosine := float32(math.Sin(theta)*0.5), float32(math.Cos(theta)*0.5)
	corners := [4]vec{}
	corners[0] = vec{cx - float32(sine*rh) - float32(cosine*rw), cy + float32(cosine*rh) - float32(sine*rw)}
	corners[1] = vec{cx + float32(sine*rh) - float32(cosine*rw), cy - float32(cosine*rh) - float32(sine*rw)}
	corners[2] = vec{float32(2*cx) - corners[0].x, float32(2*cy) - corners[0].y}
	corners[3] = vec{float32(2*cx) - corners[1].x, float32(2*cy) - corners[1].y}
	// The side nearest the horizontal points left-to-right, then clockwise.
	side := 0
	found := false
	for i, p := range corners {
		next := corners[(i+1)%4]
		dx, dy := next.x-p.x, next.y-p.y
		if dx > 0 && math.Abs(float64(dx)) >= math.Abs(float64(dy)) {
			side = i
			found = true
			break
		}
	}
	if !found {
		return quadrilateral{}, 0, 0
	}
	var q quadrilateral
	for i := range q {
		p := corners[(side+i)%4]
		q[i] = point{float64(p.x), float64(p.y)}
	}
	return q, float64(float32(rw * rh)), min(rw, rh)
}

func (q quadrilateral) bounds() (left, top, right, bottom float64) {
	left, top, right, bottom = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range q {
		left = math.Min(left, p.x)
		top = math.Min(top, p.y)
		right = math.Max(right, p.x)
		bottom = math.Max(bottom, p.y)
	}
	return
}
