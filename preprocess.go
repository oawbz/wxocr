package ocr

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
)

// tensorRGB applies native detector preprocessing: OpenCV-style half-pixel
// bilinear resizing, black bottom/right padding and ImageNet normalization.
func tensorRGB(src image.Image, width, height int) ([]float32, float64) {
	b := src.Bounds()
	w, h, scale := scaledImageSize(b, width, height)
	n := width * height
	out := make([]float32, 3*n)
	mean := [3]float32{0.485, 0.456, 0.406}
	std := [3]float32{0.229, 0.224, 0.225}
	for ch := 0; ch < 3; ch++ {
		for i := 0; i < n; i++ {
			out[ch*n+i] = -mean[ch] / std[ch]
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sx := float64(float32((float64(x)+0.5)*float64(b.Dx())/float64(w) - 0.5))
			sy := float64(float32((float64(y)+0.5)*float64(b.Dy())/float64(h) - 0.5))
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(ix), sy-float64(iy)
			if ix < 0 {
				ix = 0
				fx = 0
			}
			if ix >= b.Dx()-1 {
				ix = b.Dx() - 1
				fx = 0
			}
			if iy < 0 {
				iy = 0
				fy = 0
			}
			if iy >= b.Dy()-1 {
				iy = b.Dy() - 1
				fy = 0
			}
			ax := [2]int{int(math.RoundToEven(float64(1-float32(fx)) * 2048)), int(math.RoundToEven(fx * 2048))}
			ay := [2]int{int(math.RoundToEven(float64(1-float32(fy)) * 2048)), int(math.RoundToEven(fy * 2048))}
			var rows [2][3]int64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					rgb := nativeRGB(src.At(b.Min.X+min(ix+dx, b.Dx()-1), b.Min.Y+min(iy+dy, b.Dy()-1)))
					for ch, value := range rgb {
						rows[dy][ch] += int64(value) * int64(ax[dx])
					}
				}
			}
			for ch := 0; ch < 3; ch++ {
				// OpenCV VResizeLinear<uchar> truncates each vertical term before adding.
				pixel := float32((((int64(ay[0]) * (rows[0][ch] >> 4)) >> 16) + ((int64(ay[1]) * (rows[1][ch] >> 4)) >> 16) + 2) >> 2)
				out[ch*n+y*width+x] = (pixel/255 - mean[ch]) / std[ch]
			}
		}
	}
	return out, scale
}

type region struct {
	box   image.Rectangle
	score float32
	quad  quadrilateral
	area  float64
}

// recognitionTensor delegates a horizontal box to oriented crop sampling.
// Destination
// coordinate (0,0) maps to the floating-point box origin, with black padding.
// Fractional coordinates use OpenCV INTER_LINEAR's 1/32 interpolation grid.
func recognitionTensor(src image.Image, left, top, right, bottom float64, dynamic bool) []float32 {
	return recognitionQuadTensor(src, quadrilateral{{left, top}, {right, top}, {right, bottom}, {left, bottom}}, dynamic)
}

func recognitionQuadTensor(src image.Image, q quadrilateral, dynamic bool) []float32 {
	iw := max(1, int(math.Hypot(float64(float32(q[1].x)-float32(q[0].x)), float64(float32(q[1].y)-float32(q[0].y)))))
	ih := max(1, int(math.Hypot(float64(float32(q[3].x)-float32(q[0].x)), float64(float32(q[3].y)-float32(q[0].y)))))
	// Native crop handling rotates tall boxes once after page orientation.
	if float64(ih) >= 1.5*float64(iw) {
		q = q.oriented(1)
		iw, ih = ih, iw
	}
	w, h := max(1, 32*iw/ih), 32
	limit := 320
	if dynamic {
		limit = 960
	}
	if w > limit {
		w = limit
		h = max(1, limit*ih/iw)
	}
	bucket := 320
	if dynamic {
		bucket = 960
		// Native constant table at 0x1517c00; choose first width >= content.
		for _, candidate := range []int{64, 128, 224, 352, 512, 704, 960} {
			if candidate >= w {
				bucket = candidate
				break
			}
		}
	}
	n := bucket * 32
	out := make([]float32, 3*n)
	for i := range out {
		out[i] = -1
	}
	bounds := src.Bounds()
	transform := cropTransform(q, w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			denominator := transform[6]*float64(x) + transform[7]*float64(y) + transform[8]
			sx := math.RoundToEven((transform[0]*float64(x)+transform[1]*float64(y)+transform[2])/denominator*32) / 32
			sy := math.RoundToEven((transform[3]*float64(x)+transform[4]*float64(y)+transform[5])/denominator*32) / 32
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := int(math.Round((sx-float64(ix))*32)), int(math.Round((sy-float64(iy))*32))
			var sum [3]int
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					point := image.Pt(ix+dx, iy+dy)
					if !point.In(bounds) {
						continue
					}
					rgb := nativeRGB(src.At(point.X, point.Y))
					wx, wy := 32-fx, 32-fy
					if dx == 1 {
						wx = fx
					}
					if dy == 1 {
						wy = fy
					}
					for ch, value := range rgb {
						sum[ch] += int(value) * wx * wy
					}
				}
			}
			for ch, value := range sum {
				out[ch*n+y*bucket+x] = float32((value+512)/1024)/127.5 - 1
			}
		}
	}
	return out
}

// kernelRegions follows native TextDetection.GetResult: connected labels at
// quarter resolution, nearest resize to half resolution, 6x6 max dilation
// (anchor 3), then nearest resize to full resolution. Higher labels win overlaps.
func kernelRegions(score, kernel []float32, width, height int, threshold float32) ([]region, error) {
	kw, kh := width/4, height/4
	if width%4 != 0 || height%4 != 0 || len(score) != width*height || len(kernel) != kw*kh {
		return nil, fmt.Errorf("invalid detector maps")
	}
	for _, values := range [][]float32{score, kernel} {
		for _, v := range values {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return nil, fmt.Errorf("non-finite detector maps")
			}
		}
	}
	labels := make([]int, len(kernel))
	count := 0
	for start, v := range kernel {
		if labels[start] != 0 || v <= threshold {
			continue
		}
		count++
		labels[start] = count
		queue := []int{start}
		for head := 0; head < len(queue); head++ {
			i := queue[head]
			x, y := i%kw, i/kw
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= kw || ny < 0 || ny >= kh {
						continue
					}
					j := ny*kw + nx
					if labels[j] == 0 && kernel[j] > threshold {
						labels[j] = count
						queue = append(queue, j)
					}
				}
			}
		}
	}
	// OpenCV's default 8-connected algorithm numbers components in 2x2
	// block order. Label precedence affects overlapping numeric dilation.
	remap := make([]int, count+1)
	next := 0
	for y := 0; y < kh; y += 2 {
		for x := 0; x < kw; x += 2 {
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					if y+dy >= kh || x+dx >= kw {
						continue
					}
					label := labels[(y+dy)*kw+x+dx]
					if label > 0 && remap[label] == 0 {
						next++
						remap[label] = next
					}
				}
			}
		}
	}
	for i, label := range labels {
		labels[i] = remap[label]
	}
	hw, hh := width/2, height/2
	horizontal := make([]int, hw*hh)
	expanded := make([]int, len(horizontal))
	for y := 0; y < hh; y++ {
		for x := 0; x < hw; x++ {
			value := 0
			for dx := -3; dx <= 2; dx++ {
				sx := x + dx
				if sx >= 0 && sx < hw {
					value = max(value, labels[(y/2)*kw+sx/2])
				}
			}
			horizontal[y*hw+x] = value
		}
	}
	for y := 0; y < hh; y++ {
		for x := 0; x < hw; x++ {
			value := 0
			for dy := -3; dy <= 2; dy++ {
				sy := y + dy
				if sy >= 0 && sy < hh {
					value = max(value, horizontal[sy*hw+x])
				}
			}
			expanded[y*hw+x] = value
		}
	}
	boxes := make([]image.Rectangle, count+1)
	sums := make([]float64, count+1)
	areas := make([]int, count+1)
	endpoints := make([][]image.Point, count+1)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			label := expanded[(y/2)*hw+x/2]
			if label == 0 {
				continue
			}
			boxes[label] = boxes[label].Union(image.Rect(x, y, x+1, y+1))
			sums[label] += float64(score[y*width+x])
			areas[label]++
			if x == 0 || x == width-1 || expanded[(y/2)*hw+(x-1)/2] != label || expanded[(y/2)*hw+(x+1)/2] != label {
				endpoints[label] = append(endpoints[label], image.Pt(x, y))
			}
		}
	}
	var out []region
	for i, box := range boxes {
		if i == 0 || areas[i] < 2 {
			continue
		}
		quad, area := minimumRectangle(endpoints[i])
		out = append(out, region{box: box, score: float32(sums[i] / float64(areas[i])), quad: quad, area: area})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].box, out[j].box
		if a.Min.Y != b.Min.Y {
			return a.Min.Y < b.Min.Y
		}
		return a.Min.X < b.Min.X
	})
	return out, nil
}

func decodeCTC(prob []float32, steps int, charset []string) (string, float32, error) {
	text, confidence, _, err := recognitionCTC(prob, steps, charset)
	return text, confidence, err
}

func recognitionCTC(prob []float32, steps int, charset []string) (string, float32, float32, error) {
	classes := len(charset) + 1
	if steps < 1 || len(prob) != steps*classes {
		return "", 0, 0, fmt.Errorf("recognition output/charset mismatch")
	}
	var text strings.Builder
	var labels []int
	var emissionSum float64
	previous := 0
	first, last := -1, -1
	for t := 0; t < steps; t++ {
		row := prob[t*classes : (t+1)*classes]
		label := 0
		var rowSum float64
		for i, v := range row {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < 0 || v > 1 {
				return "", 0, 0, fmt.Errorf("invalid recognition probability")
			}
			rowSum += float64(v)
			if v > row[label] {
				label = i
			}
		}
		if math.Abs(rowSum-1) > 1e-3 {
			return "", 0, 0, fmt.Errorf("recognition probabilities are not normalized")
		}
		if label != 0 {
			if first < 0 {
				first = t
			}
			last = t
		}
		if label != 0 && label != previous {
			text.WriteString(charset[label-1])
			labels = append(labels, label)
			emissionSum += float64(row[label])
		}
		previous = label
	}
	if first < 0 {
		return "", 0, 0, nil
	}
	// Blank padding before and after the active text is not transcription evidence.
	return text.String(), ctcSequenceConfidence(prob[first*classes:(last+1)*classes], last-first+1, classes, labels), float32(emissionSum / float64(len(labels))), nil
}

// ImageAlign uses a fixed maximum side 960 and pads each axis to 160.
func detectorSize(b image.Rectangle) (width, height int) {
	maxSide := int64(max(b.Dx(), b.Dy()))
	width = int((int64(b.Dx())*960 + maxSide - 1) / maxSide)
	height = int((int64(b.Dy())*960 + maxSide - 1) / maxSide)
	return (width + 159) / 160 * 160, (height + 159) / 160 * 160
}
func scaledImageSize(b image.Rectangle, width, height int) (w, h int, scale float64) {
	scale = math.Min(float64(width)/float64(b.Dx()), float64(height)/float64(b.Dy()))
	// Native clamps the dominant side after ceil; floating products can exceed it.
	w = min(width, max(1, int(math.Ceil(float64(b.Dx())*scale))))
	h = min(height, max(1, int(math.Ceil(float64(b.Dy())*scale))))
	return
}

// Native color decoding discards alpha and retains unassociated RGB, including
// the RGB bytes of fully transparent PNG pixels. RGBA() alone loses those bytes.
func nativeRGB(c color.Color) [3]uint8 {
	switch c := c.(type) {
	case color.NRGBA:
		return [3]uint8{c.R, c.G, c.B}
	case color.NRGBA64:
		return [3]uint8{uint8(c.R >> 8), uint8(c.G >> 8), uint8(c.B >> 8)}
	default:
		n := color.NRGBAModel.Convert(c).(color.NRGBA)
		return [3]uint8{n.R, n.G, n.B}
	}
}
