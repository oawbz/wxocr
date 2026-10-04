package ocr

import (
	"image"
	"math"
)

// Binary border following retains outer and hole contours in scan order.
func binaryContours(mask []bool, w, h int) [][]image.Point {
	stride := w + 2
	pixels := make([]int, (h+2)*stride)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] {
				pixels[(y+1)*stride+x+1] = 1
			}
		}
	}
	dirs := [8]image.Point{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	index := func(p image.Point) int { return p.Y*stride + p.X }
	neighbor := func(p, from image.Point, clockwise bool, skip bool) (image.Point, bool) {
		delta := from.Sub(p)
		start := 0
		for i, d := range dirs {
			if d == delta {
				start = i
				break
			}
		}
		offset := 0
		if skip {
			offset = 1
		}
		for k := offset; k < 8+offset; k++ {
			i := (start + k) % 8
			if !clockwise {
				i = (start - k + 16) % 8
			}
			q := p.Add(dirs[i])
			if pixels[index(q)] != 0 {
				return q, true
			}
		}
		return image.Point{}, false
	}
	var contours [][]image.Point
	number := 1
	for y := 1; y <= h; y++ {
		for x := 1; x <= w; x++ {
			p := image.Pt(x, y)
			v := pixels[index(p)]
			var back image.Point
			if v == 1 && pixels[index(p)-1] == 0 {
				back = p.Add(image.Pt(-1, 0))
			} else if v >= 1 && pixels[index(p)+1] == 0 {
				back = p.Add(image.Pt(1, 0))
			} else {
				continue
			}
			number++
			first, ok := neighbor(p, back, true, false)
			if !ok {
				pixels[index(p)] = -number
				contours = append(contours, []image.Point{p.Sub(image.Pt(1, 1))})
				continue
			}
			previous, current := first, p
			var contour []image.Point
			for steps := 0; steps < 8*w*h; steps++ {
				next, _ := neighbor(current, previous, false, true)
				contour = append(contour, current.Sub(image.Pt(1, 1)))
				if pixels[index(current)+1] == 0 {
					pixels[index(current)] = -number
				} else if pixels[index(current)] == 1 {
					pixels[index(current)] = number
				}
				if next == p && current == first {
					break
				}
				previous, current = current, next
			}
			contours = append(contours, contour)
		}
	}
	// RETR_LIST prepends each discovered contour.
	for i, j := 0, len(contours)-1; i < j; i, j = i+1, j-1 {
		contours[i], contours[j] = contours[j], contours[i]
	}
	return contours
}

func opening3(mask []bool, w, h int) []bool {
	eroded := make([]bool, len(mask))
	out := make([]bool, len(mask))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			value := true
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx >= 0 && nx < w && ny >= 0 && ny < h && !mask[ny*w+nx] {
						value = false
					}
				}
			}
			eroded[y*w+x] = value
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			value := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx >= 0 && nx < w && ny >= 0 && ny < h && eroded[ny*w+nx] {
						value = true
					}
				}
			}
			out[y*w+x] = value
		}
	}
	return out
}

func polygonMetrics(p []image.Point) (float64, float64) {
	var area, length float64
	for i, a := range p {
		b := p[(i+1)%len(p)]
		area += float64(a.X*b.Y - b.X*a.Y)
		length += math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
	}
	return math.Abs(area) * .5, length
}

// Closed-contour approximation follows OpenCV 4.5.5 approx.cpp, including
// cyclic farthest-point tie order and the final near-collinear cleanup.
// See LICENSE.opencv for upstream attribution.
func simplifyContour(p []image.Point, epsilon float64) []image.Point {
	if len(p) < 3 {
		return append([]image.Point(nil), p...)
	}
	n := len(p)
	eps := epsilon * epsilon
	pos, far := 0, 0
	type span struct{ start, end int }
	read := func() image.Point { v := p[pos]; pos = (pos + 1) % n; return v }
	var start image.Point
	closeEnough := false
	for k := 0; k < 3; k++ {
		pos = (pos + far) % n
		start = read()
		maxDistance := 0.0
		for j := 1; j < n; j++ {
			q := read()
			dx, dy := float64(q.X-start.X), float64(q.Y-start.Y)
			d := dx*dx + dy*dy
			if d > maxDistance {
				maxDistance = d
				far = j
			}
		}
		closeEnough = maxDistance <= eps
	}
	out := make([]image.Point, 0, n)
	stack := []span{}
	if closeEnough {
		return []image.Point{start}
	}
	first := pos % n
	last := (far + first) % n
	stack = append(stack, span{last, first}, span{first, last})
	for len(stack) > 0 {
		slice := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		end := p[slice.end]
		pos = slice.start
		start = read()
		maximum, at := 0.0, slice.start
		dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
		for pos != slice.end {
			q := read()
			d := math.Abs(float64(q.Y-start.Y)*dx - float64(q.X-start.X)*dy)
			if d > maximum {
				maximum = d
				at = (pos + n - 1) % n
			}
		}
		if maximum*maximum <= eps*(dx*dx+dy*dy) {
			out = append(out, start)
		} else {
			stack = append(stack, span{at, slice.end}, span{slice.start, at})
		}
	}
	count, newCount := len(out), len(out)
	pos = count - 1
	next := func() image.Point { v := out[pos]; pos = (pos + 1) % count; return v }
	start = next()
	write := pos
	middle := next()
	for i := 0; i < count && newCount > 2; i++ {
		end := next()
		dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
		distance := math.Abs(float64(middle.X-start.X)*dy - float64(middle.Y-start.Y)*dx)
		inner := float64(middle.X-start.X)*float64(end.X-middle.X) + float64(middle.Y-start.Y)*float64(end.Y-middle.Y)
		if distance*distance <= 0.5*eps*(dx*dx+dy*dy) && dx != 0 && dy != 0 && inner >= 0 {
			newCount--
			start = end
			out[write] = start
			write = (write + 1) % count
			middle = next()
			i++
			continue
		}
		start = middle
		out[write] = start
		write = (write + 1) % count
		middle = end
	}
	return out[:newCount]
}

func polygonContains(p []image.Point, x, y int) bool {
	inside := false
	for i, a := range p {
		b := p[(i+1)%len(p)]
		cross := (x-a.X)*(b.Y-a.Y) - (y-a.Y)*(b.X-a.X)
		if cross == 0 && x >= min(a.X, b.X) && x <= max(a.X, b.X) && y >= min(a.Y, b.Y) && y <= max(a.Y, b.Y) {
			return true
		}
		if (a.Y > y) != (b.Y > y) && float64(x) < float64(b.X-a.X)*float64(y-a.Y)/float64(b.Y-a.Y)+float64(a.X) {
			inside = !inside
		}
	}
	return inside
}

// CHAIN_APPROX_SIMPLE keeps only vertices where the border direction changes.
func simpleContour(p []image.Point) []image.Point {
	if len(p) < 3 {
		return p
	}
	out := make([]image.Point, 0, len(p))
	for i, v := range p {
		before, after := p[(i+len(p)-1)%len(p)], p[(i+1)%len(p)]
		if v.Sub(before) != after.Sub(v) {
			out = append(out, v)
		}
	}
	return out
}
