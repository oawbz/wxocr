package ocr

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
	"strings"
)

// Candidate retains nonempty text that failed the normal acceptance thresholds.
// Candidates do not participate in paragraph ordering or normal text extraction.
type Candidate struct {
	Line
	Reasons  []string `json:"reasons"`
	Evidence float32  `json:"evidence"`
}

// Diagnostic records every detected region, including rejected and duplicate ones.
// Coordinates are relative to the input image, independently of Bounds().Min.
type Diagnostic struct {
	RecognitionQuad   [][2]float64    `json:"recognition_quad,omitempty"`
	RetryOriginalText string          `json:"retry_original_text,omitempty"`
	RetryMethod       string          `json:"retry_method,omitempty"`
	ReviewReasons     []string        `json:"review_reasons,omitempty"`
	SplitMethod       string          `json:"split_method,omitempty"`
	Pass              int             `json:"pass"`
	Tile              image.Rectangle `json:"tile"`
	Quad              [4][2]float64   `json:"quad"`
	DetectionScore    float32         `json:"detection_score"`
	DetectionArea     float64         `json:"detection_area"`
	Text              string          `json:"text,omitempty"`
	Evidence          float32         `json:"evidence"`
	Reason            string          `json:"reason"`
	CropWidth         int             `json:"crop_width,omitempty"`
	CropHeight        int             `json:"crop_height,omitempty"`
	DuplicateOf       *int            `json:"duplicate_of,omitempty"`
	ParentRegion      *int            `json:"parent_region,omitempty"`
	CropPNG           []byte          `json:"crop_png,omitempty"`
}

type diagnosticCropsKey struct{}

// WithDiagnosticCrops includes base64 PNGs of the actual normalized recognition
// inputs in JSON diagnostics. Opt in only when needed: images increase responses.
func WithDiagnosticCrops(ctx context.Context) context.Context {
	return context.WithValue(ctx, diagnosticCropsKey{}, true)
}
func diagnosticCrops(ctx context.Context) bool {
	v, _ := ctx.Value(diagnosticCropsKey{}).(bool)
	return v
}
func tensorPNG(tensor []float32) ([]byte, error) {
	w := len(tensor) / (3 * 32)
	im := image.NewNRGBA(image.Rect(0, 0, w, 32))
	n := w * 32
	for y := 0; y < 32; y++ {
		for x := 0; x < w; x++ {
			var c [3]uint8
			for ch := 0; ch < 3; ch++ {
				c[ch] = uint8(math.Round(math.Max(0, math.Min(255, float64(tensor[ch*n+y*w+x]+1)*127.5))))
			}
			im.SetNRGBA(x, y, color.NRGBA{c[0], c[1], c[2], 255})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, im)
	return buf.Bytes(), err
}

// A view retains original pixels and bounds without depending on SubImage support.
type imageView struct {
	image.Image
	rect image.Rectangle
}

func (v imageView) Bounds() image.Rectangle { return v.rect }
func (v imageView) At(x, y int) color.Color { return v.Image.At(x, y) }

// Whole-image context plus overlapping, near-square crops for elongated images.
// A 960 px short side at detection scale preserves small text on long screenshots.
func detectionTiles(b image.Rectangle, direction ...int) []image.Rectangle {
	out := []image.Rectangle{b}
	if min(b.Dx(), b.Dy()) >= 1600 {
		// Bound the number of passes on very large uploads, while preserving
		// approximately original character scale on ordinary scanned pages.
		size := max(1280, (max(b.Dx(), b.Dy())+3)/4)
		stride := size * 4 / 5
		strideX, strideY := stride, stride
		if len(direction) > 0 {
			if direction[0]%2 == 0 {
				strideY = size - max(64, size/16)
			} else {
				strideX = size - max(64, size/16)
			}
		}
		starts := func(length, stride int) []int {
			v := []int{0}
			for v[len(v)-1]+size < length {
				v = append(v, min(v[len(v)-1]+stride, length-size))
			}
			return v
		}
		for _, y := range starts(b.Dy(), strideY) {
			for _, x := range starts(b.Dx(), strideX) {
				out = append(out, image.Rect(b.Min.X+x, b.Min.Y+y, min(b.Max.X, b.Min.X+x+size), min(b.Max.Y, b.Min.Y+y+size)))
			}
		}
		return out
	}
	short, long := min(b.Dx(), b.Dy()), max(b.Dx(), b.Dy())
	if long <= 960 || float64(long)/float64(short) < 2 {
		return out
	}
	size := min(long, max(960, short))
	overlap := max(160, size/5)
	for start := 0; ; {
		end := min(long, start+size)
		r := b
		if b.Dy() >= b.Dx() {
			r.Min.Y = b.Min.Y + start
			r.Max.Y = b.Min.Y + end
		} else {
			r.Min.X = b.Min.X + start
			r.Max.X = b.Min.X + end
		}
		out = append(out, r)
		if end == long {
			break
		}
		start = min(start+size-overlap, long-size)
	}
	return out
}

func (e *Engine) detectPasses(ctx context.Context, src image.Image, timing *stageTimer) ([]region, int, []Diagnostic, error) {
	var all []region
	var diagnostics []Diagnostic
	direction := 0
	bounds := src.Bounds()
	tiles := []image.Rectangle{bounds}
	for pass := 0; pass < len(tiles); pass++ {
		tile := tiles[pass]
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}
		view := cropImage(src, tile)
		dw, dh := detectorSize(tile)
		pixels, _ := tensorRGB(view, dw, dh)
		rw, rh, _ := scaledImageSize(tile, dw, dh)
		timing.mark("detector_preprocess")
		maps, _, err := e.detector.runAll(pixels, int64(dh), int64(dw))
		if err != nil {
			return nil, 0, nil, err
		}
		timing.mark("detector_inference")
		if pass == 0 {
			direction, _, err = pageOrientation(maps[2])
			if HighResolutionEnabled(ctx) {
				tiles = detectionTiles(bounds, direction)
			}
			if err != nil {
				return nil, 0, nil, err
			}
		}
		regions, err := kernelRegions(maps[0], maps[1], dw, dh, 0.5)
		if err != nil {
			return nil, 0, nil, err
		}
		for _, r := range regions {
			d := Diagnostic{Pass: pass, Tile: tile.Sub(bounds.Min), DetectionScore: r.score, DetectionArea: r.area}
			for i, p := range r.quad {
				r.quad[i] = point{math.Max(0, math.Min(float64(tile.Dx()-1), p.x*float64(tile.Dx())/float64(rw))) + float64(tile.Min.X-bounds.Min.X), math.Max(0, math.Min(float64(tile.Dy()-1), p.y*float64(tile.Dy())/float64(rh))) + float64(tile.Min.Y-bounds.Min.Y)}
				d.Quad[i] = [2]float64{r.quad[i].x, r.quad[i].y}
			}
			all = append(all, r)
			diagnostics = append(diagnostics, d)
		}
		timing.mark("detector_postprocess")
	}
	if HighResolutionEnabled(ctx) {
		all, diagnostics = splitDetectedRegions(ctx, src, all, diagnostics, direction)
		pruneDetectionDuplicates(diagnostics, bounds.Dx(), bounds.Dy())
	}
	return all, direction, diagnostics, ctx.Err()
}

// Tile-edge crops may contain only part of a line. Keep them reviewable, but
// let overlapping tiles or the whole-image pass supply complete body text.
func touchesTileBoundary(d Diagnostic, width, height int) bool {
	if d.Pass == 0 {
		return false
	}
	margin := 12.0
	for _, p := range d.Quad {
		if d.Tile.Min.X > 0 && p[0] <= float64(d.Tile.Min.X)+margin {
			return true
		}
		if d.Tile.Min.Y > 0 && p[1] <= float64(d.Tile.Min.Y)+margin {
			return true
		}
		if d.Tile.Max.X < width && p[0] >= float64(d.Tile.Max.X-1)-margin {
			return true
		}
		if d.Tile.Max.Y < height && p[1] >= float64(d.Tile.Max.Y-1)-margin {
			return true
		}
	}
	return false
}

func candidateReasons(score float32, area float64, evidence float32) []string {
	var reasons []string
	if score < 0.65 {
		reasons = append(reasons, "low_detection_score")
	}
	if area < 300 {
		reasons = append(reasons, "small_detection_area")
	}
	if evidence <= 0.4 {
		reasons = append(reasons, "low_recognition_evidence")
	}
	return reasons
}

func duplicateLine(a, b Line) bool {
	area := func(l Line) float32 { return max(0, l.Right-l.Left) * max(0, l.Bottom-l.Top) }
	intersection := max(0, min(a.Right, b.Right)-max(a.Left, b.Left)) * max(0, min(a.Bottom, b.Bottom)-max(a.Top, b.Top))
	union := area(a) + area(b) - intersection
	if union <= 0 {
		return false
	}
	iou := intersection / union
	return iou >= 0.85 || (a.Text == b.Text && iou >= 0.5)
}
func mergePassResults(r *Result) {
	// Prefer strong recognition, then detection. Keep all competing observations
	// in diagnostics, so a duplicate decision remains reviewable.
	sort.SliceStable(r.Lines, func(i, j int) bool {
		if r.Lines[i].Confidence != r.Lines[j].Confidence {
			return r.Lines[i].Confidence > r.Lines[j].Confidence
		}
		return r.Lines[i].DetectionScore > r.Lines[j].DetectionScore
	})
	// Complete local detections carry more spatial detail than a heavily
	// downscaled whole-page crop. Rank these first, then use model evidence.
	local := make(map[Line]bool)
	for _, line := range r.Lines {
		for _, d := range r.Diagnostics {
			if d.Pass > 0 && diagnosticMatches(d, line) {
				local[line] = true
				break
			}
		}
	}
	sort.SliceStable(r.Lines, func(i, j int) bool { return local[r.Lines[i]] && !local[r.Lines[j]] })
	kept := make([]Line, 0, len(r.Lines))
	for _, line := range r.Lines {
		duplicate := false
		for _, other := range kept {
			if duplicateLine(line, other) || (len(r.Diagnostics) > 0 && sameTextLocation(line, other)) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			kept = append(kept, line)
		}
	}
	// Suppress coarse boxes spanning several complete local pieces. Their
	// observations remain in diagnostics instead of repeating joined text.
	filtered := make([]Line, 0, len(kept))
	for _, line := range kept {
		covered := float32(0)
		count := 0
		if !local[line] {
			for _, other := range kept {
				if local[other] {
					intersection := lineIntersection(line, other)
					if intersection >= lineArea(other)*.65 {
						covered += intersection
						count++
					}
				}
			}
		}
		if count >= 2 && covered >= lineArea(line)*.6 {
			continue
		}
		filtered = append(filtered, line)
	}
	kept = filtered
	complete := make([]Line, 0, len(kept))
	for i, line := range kept {
		fragment := false
		for j, other := range kept {
			if i != j && textFragment(line, other) {
				fragment = true
				break
			}
		}
		if !fragment {
			complete = append(complete, line)
		}
	}
	kept = complete
	r.Lines = kept
	candidates := make([]Candidate, 0, len(r.Candidates))
	sort.SliceStable(r.Candidates, func(i, j int) bool { return r.Candidates[i].Evidence > r.Candidates[j].Evidence })
	for _, c := range r.Candidates {
		duplicate := false
		for _, line := range kept {
			if duplicateLine(c.Line, line) || sameTextLocation(c.Line, line) || candidateFragment(c.Line, line) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			for _, other := range candidates {
				if duplicateLine(c.Line, other.Line) {
					duplicate = true
					break
				}
			}
		}
		if !duplicate {
			candidates = append(candidates, c)
		}
	}
	r.Candidates = candidates
	// Each record retains its original text even when merged away.
	matchedLines := make(map[Line]bool)
	matchedCandidates := make(map[Line]bool)
	for i := range r.Diagnostics {
		d := &r.Diagnostics[i]
		if d.Reason != "accepted" && d.Reason != "candidate" {
			continue
		}
		found := false
		if d.Reason == "accepted" {
			for _, line := range kept {
				if diagnosticMatches(*d, line) && !matchedLines[line] {
					matchedLines[line] = true
					found = true
					break
				}
			}
		} else {
			for _, c := range candidates {
				if diagnosticMatches(*d, c.Line) && !matchedCandidates[c.Line] {
					matchedCandidates[c.Line] = true
					found = true
					break
				}
			}
		}
		if !found {
			d.Reason = "duplicate"
		}
	}
}
func diagnosticMatches(d Diagnostic, l Line) bool {
	left, top, right, bottom := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range d.Quad {
		left = math.Min(left, p[0])
		top = math.Min(top, p[1])
		right = math.Max(right, p[0])
		bottom = math.Max(bottom, p[1])
	}
	return d.Text == l.Text && float32(left) == l.Left && float32(top) == l.Top && float32(right) == l.Right && float32(bottom) == l.Bottom
}

func lineArea(l Line) float32 { return max(0, l.Right-l.Left) * max(0, l.Bottom-l.Top) }
func lineIntersection(a, b Line) float32 {
	return max(0, min(a.Right, b.Right)-max(a.Left, b.Left)) * max(0, min(a.Bottom, b.Bottom)-max(a.Top, b.Top))
}
func diagnosticLine(d Diagnostic) Line {
	l := Line{Left: float32(math.Inf(1)), Top: float32(math.Inf(1)), Right: float32(math.Inf(-1)), Bottom: float32(math.Inf(-1))}
	for _, p := range d.Quad {
		l.Left = min(l.Left, float32(p[0]))
		l.Top = min(l.Top, float32(p[1]))
		l.Right = max(l.Right, float32(p[0]))
		l.Bottom = max(l.Bottom, float32(p[1]))
	}
	return l
}
func sameTextLocation(a, b Line) bool {
	// A short fragment and its complete word may differ in transcription and box
	// length, but must agree along the text baseline and substantially overlap.
	aw, ah, bw, bh := a.Right-a.Left, a.Bottom-a.Top, b.Right-b.Left, b.Bottom-b.Top
	overlapX := max(0, min(a.Right, b.Right)-max(a.Left, b.Left))
	overlapY := max(0, min(a.Bottom, b.Bottom)-max(a.Top, b.Top))
	if max(aw, bw) < max(ah, bh) {
		aw, ah, bw, bh = ah, aw, bh, bw
		overlapX, overlapY = overlapY, overlapX
	}
	return min(aw, bw) > 0 && min(ah, bh) > 0 && max(aw, bw) <= min(aw, bw)*1.8 && max(ah, bh) <= min(ah, bh)*1.8 && overlapX >= min(aw, bw)*.8 && overlapY >= min(ah, bh)*.65
}
func pruneDetectionDuplicates(ds []Diagnostic, width, height int) {
	if min(width, height) < 1600 {
		return
	}
	indices := make([]int, 0, len(ds))
	for i, d := range ds {
		if d.Pass > 0 && d.Reason == "" && d.DetectionScore >= .65 && d.DetectionArea >= 300 && !touchesTileBoundary(d, width, height) {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := ds[indices[i]], ds[indices[j]]
		as, bs := max(a.Tile.Dx(), a.Tile.Dy()), max(b.Tile.Dx(), b.Tile.Dy())
		if as != bs {
			return as < bs
		}
		if int(a.DetectionScore*5) != int(b.DetectionScore*5) {
			return int(a.DetectionScore*5) > int(b.DetectionScore*5)
		}
		return a.DetectionArea > b.DetectionArea
	})
	kept := []int{}
	for _, i := range indices {
		a := diagnosticLine(ds[i])
		duplicate := false
		for _, j := range kept {
			b := diagnosticLine(ds[j])
			intersection := lineIntersection(a, b)
			union := lineArea(a) + lineArea(b) - intersection
			if union > 0 && intersection/union >= .72 {
				duplicate = true
				chosen := j
				ds[i].DuplicateOf = &chosen
				break
			}
		}
		if duplicate {
			ds[i].Reason = "redundant_detection"
		} else {
			kept = append(kept, i)
		}
	}
}

func textFragment(a, b Line) bool {
	compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
	at, bt := compact(a.Text), compact(b.Text)
	return at != "" && at != bt && strings.Contains(bt, at) && lineArea(a) < lineArea(b)*.85 && lineIntersection(a, b) >= lineArea(a)*.7
}

func recognitionOrder(ds []Diagnostic, preferLocal ...bool) []int {
	order := make([]int, 0, len(ds))
	localFirst := len(preferLocal) > 0 && preferLocal[0]
	for i, d := range ds {
		if d.Reason != "redundant_detection" && (!localFirst || d.Pass > 0) {
			order = append(order, i)
		}
	}
	for i, d := range ds {
		if d.Reason == "redundant_detection" {
			order = append(order, i)
		}
	}
	if localFirst {
		for i, d := range ds {
			if d.Pass == 0 && d.Reason != "redundant_detection" {
				order = append(order, i)
			}
		}
	}
	return order
}

func coveredByAcceptedLocal(d Diagnostic, ds []Diagnostic) bool {
	box := diagnosticLine(d)
	area := lineArea(box)
	if area <= 0 {
		return false
	}
	var clipped []Line
	for _, other := range ds {
		if other.Pass == 0 || other.Reason != "accepted" {
			continue
		}
		local := diagnosticLine(other)
		intersection := lineIntersection(box, local)
		if intersection >= lineArea(local)*.65 {
			clipped = append(clipped, Line{Left: max(box.Left, local.Left), Top: max(box.Top, local.Top), Right: min(box.Right, local.Right), Bottom: min(box.Bottom, local.Bottom)})
		}
	}
	return unionLineArea(clipped) >= area*.85
}
func unionLineArea(boxes []Line) float32 {
	xs := make([]float32, 0, 2*len(boxes))
	for _, b := range boxes {
		xs = append(xs, b.Left, b.Right)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	var area float32
	for i := 1; i < len(xs); i++ {
		if xs[i] <= xs[i-1] {
			continue
		}
		var spans [][2]float32
		for _, b := range boxes {
			if b.Left < xs[i] && b.Right > xs[i-1] {
				spans = append(spans, [2]float32{b.Top, b.Bottom})
			}
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
		var length, bottom float32
		for j, s := range spans {
			if j == 0 || s[0] >= bottom {
				length += s[1] - s[0]
				bottom = s[1]
			} else if s[1] > bottom {
				length += s[1] - bottom
				bottom = s[1]
			}
		}
		area += (xs[i] - xs[i-1]) * length
	}
	return area
}

// Preserve concrete pixel layouts for native resize; the generic view is only
// needed for image implementations without a zero-copy SubImage operation.
func cropImage(src image.Image, rect image.Rectangle) image.Image {
	if rect == src.Bounds() {
		return src
	}
	if im, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return im.SubImage(rect)
	}
	return imageView{src, rect}
}

// Whole-page detections can split a word that a local pass already recognized
// completely. Suppress only strict text fragments spatially covered by body
// text; keep the observation in Diagnostics. A wider coarse detection may have
// up to 40% padding outside the accurate local box, but its center must be inside.
func candidateFragment(fragment, complete Line) bool {
	area := lineArea(fragment)
	if area <= 0 || area >= lineArea(complete)*.85 || lineIntersection(fragment, complete) < area*.6 {
		return false
	}
	cx, cy := (fragment.Left+fragment.Right)/2, (fragment.Top+fragment.Bottom)/2
	if cx < complete.Left || cx >= complete.Right || cy < complete.Top || cy >= complete.Bottom {
		return false
	}
	compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
	a, b := compact(fragment.Text), compact(complete.Text)
	return a != "" && a != b && strings.Contains(b, a)
}
