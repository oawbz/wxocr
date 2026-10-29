// Package ocr performs OCR in Go using converted XNet models.
// It requires ONNX Runtime through cgo, but neither Python nor WeChat libraries.
package ocr

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"reflect"
	"strings"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Config supplies the platform runtime and the three converted OCR models.
type Config struct {
	RuntimeLibrary   string
	DetectionModel   string
	RecognitionModel string
	ParagraphModel   string
	CharsetFile      string
}
type Line struct {
	Left   float32 `json:"left"`
	Top    float32 `json:"top"`
	Right  float32 `json:"right"`
	Bottom float32 `json:"bottom"`
	// Confidence is length-normalized CTC model evidence, not calibrated accuracy.
	Confidence     float32 `json:"confidence"`
	Text           string  `json:"text"`
	DetectionScore float32 `json:"detection_score"`
}
type Result struct {
	Width       int          `json:"width"`
	Height      int          `json:"height"`
	Paragraphs  []Paragraph  `json:"paragraphs,omitempty"`
	Lines       []Line       `json:"lines"`
	Candidates  []Candidate  `json:"candidates,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Engines own a reference-counted process-wide runtime environment. Applications
// should not separately initialize/destroy the binding's global environment.
var runtimeState struct {
	sync.Mutex
	refs int
	path string
}

func acquireRuntime(path string) error {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	if runtimeState.refs > 0 {
		if path != runtimeState.path {
			return errors.New("runtime library differs from active engines")
		}
		runtimeState.refs++
		return nil
	}
	if ort.IsInitialized() {
		return errors.New("ONNX Runtime environment already owned by another caller")
	}
	ort.SetSharedLibraryPath(path)
	if err := ort.InitializeEnvironment(); err != nil {
		return err
	}
	runtimeState.path = path
	runtimeState.refs = 1
	return nil
}
func releaseRuntime() error {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	runtimeState.refs--
	if runtimeState.refs == 0 {
		runtimeState.path = ""
		return ort.DestroyEnvironment()
	}
	return nil
}

type model struct {
	session       *ort.DynamicAdvancedSession
	shape         ort.Shape
	outputs       int
	compact       bool
	probabilities bool
}

func loadModel(path string, shape ort.Shape, outputShape ort.Shape) (*model, error) {
	inputs, outputs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return nil, err
	}
	compactRecognition := shape[2] == 32 && len(outputs) == 1 && outputs[0].Name == "ctc_compact_v1" && reflect.DeepEqual(outputs[0].Dimensions, ort.NewShape(1, -1, -1))
	dynamicRecognition := len(inputs) == 1 && shape[2] == 32 && reflect.DeepEqual(inputs[0].Dimensions, ort.NewShape(1, 3, 32, -1)) && len(outputs) > 0 && (compactRecognition || reflect.DeepEqual(outputs[0].Dimensions, ort.NewShape(1, -1, 13564)))
	dynamicDetection := len(inputs) == 1 && shape[2] == 960 && reflect.DeepEqual(inputs[0].Dimensions, ort.NewShape(1, 3, -1, -1)) && len(outputs) > 1 && reflect.DeepEqual(outputs[0].Dimensions, ort.NewShape(1, -1, -1))
	if shape[2] == 768 && (len(inputs) != 1 || len(outputs) != 1 || !reflect.DeepEqual(inputs[0].Dimensions, shape) || !reflect.DeepEqual(outputs[0].Dimensions, outputShape)) {
		return nil, errors.New("paragraph model requires shape [1,3,768,768] -> [1,768,768]")
	}
	if shape[2] == 32 && !dynamicRecognition {
		return nil, fmt.Errorf("recognizer requires a dynamic recognition export: %s", path)
	}
	if shape[2] == 960 && !dynamicDetection {
		return nil, fmt.Errorf("detector requires a dynamic detection export: %s", path)
	}
	if len(inputs) != 1 || len(outputs) < 1 || inputs[0].DataType != ort.TensorElementDataTypeFloat || outputs[0].DataType != ort.TensorElementDataTypeFloat {
		return nil, fmt.Errorf("unsupported model signature: %s", path)
	}
	if shape[2] == 32 && (len(outputs) != 1 || (outputs[0].Name != "logits" && outputs[0].Name != "probabilities" && !compactRecognition)) {
		return nil, errors.New("recognizer requires logits, probabilities or compact CTC output")
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer options.Destroy()
	if err = options.SetIntraOpNumThreads(1); err != nil {
		return nil, err
	}
	names := []string{outputs[0].Name}
	if shape[2] == 960 {
		if len(outputs) < 2 || !reflect.DeepEqual(outputs[1].Dimensions, ort.NewShape(1, -1, -1)) || outputs[1].DataType != ort.TensorElementDataTypeFloat {
			return nil, errors.New("detector is missing the kernel map")
		}
		if len(outputs) < 3 || !reflect.DeepEqual(outputs[2].Dimensions, ort.NewShape(1, 4)) || outputs[2].DataType != ort.TensorElementDataTypeFloat {
			return nil, errors.New("detector is missing the orientation head")
		}
		names = append(names, outputs[1].Name, outputs[2].Name)
	}
	session, err := ort.NewDynamicAdvancedSession(path, []string{inputs[0].Name}, names, options)
	if err != nil {
		return nil, err
	}
	if dynamicRecognition {
		shape = ort.NewShape(1, 3, 32, -1)
	}
	if dynamicDetection {
		shape = ort.NewShape(1, 3, -1, -1)
	}
	return &model{session: session, shape: shape, outputs: len(names), compact: compactRecognition, probabilities: shape[2] == 32 && outputs[0].Name == "probabilities"}, nil
}
func (m *model) run(data []float32) ([]float32, ort.Shape, error) {
	values, shapes, err := m.runAll(data)
	if err != nil {
		return nil, nil, err
	}
	return values[0], shapes[0], nil
}
func (m *model) runAll(data []float32, spatial ...int64) ([][]float32, []ort.Shape, error) {
	shape := append(ort.Shape(nil), m.shape...)
	if shape[2] == -1 {
		if len(spatial) != 2 || spatial[0] < 1 || spatial[1] < 1 {
			return nil, nil, errors.New("detector dimensions are required")
		}
		shape[2], shape[3] = spatial[0], spatial[1]
	} else if shape[3] == -1 {
		if len(data) == 0 || len(data)%(3*32) != 0 {
			return nil, nil, errors.New("invalid recognition tensor size")
		}
		shape[3] = int64(len(data) / (3 * 32))
	}
	input, err := ort.NewTensor(shape, data)
	if err != nil {
		return nil, nil, err
	}
	defer input.Destroy()
	outputs := make([]ort.Value, m.outputs)
	defer func() {
		for _, v := range outputs {
			if v != nil {
				v.Destroy()
			}
		}
	}()
	if err = m.session.Run([]ort.Value{input}, outputs); err != nil {
		return nil, nil, err
	}
	values := make([][]float32, len(outputs))
	shapes := make([]ort.Shape, len(outputs))
	for i, output := range outputs {
		value, ok := output.(*ort.Tensor[float32])
		if !ok {
			return nil, nil, errors.New("model returned non-float tensor")
		}
		values[i] = append([]float32(nil), value.GetData()...)
		shapes[i] = value.GetShape()
	}
	return values, shapes, nil
}

type Engine struct {
	mu                   sync.Mutex
	detector, recognizer *model
	paragraph            *model
	charset              []string
	closed               bool
}

func New(config Config) (*Engine, error) {
	for _, field := range []struct{ name, path string }{{"runtime", config.RuntimeLibrary}, {"detection model", config.DetectionModel}, {"recognition model", config.RecognitionModel}, {"paragraph model", config.ParagraphModel}, {"charset", config.CharsetFile}} {
		if field.path == "" {
			return nil, fmt.Errorf("%s path is required", field.name)
		}
	}
	raw, err := os.ReadFile(config.CharsetFile)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	charset := strings.Split(text, "\n")
	if len(charset) != 13563 {
		return nil, fmt.Errorf("expected 13563 charset entries, got %d", len(charset))
	}
	if err = acquireRuntime(config.RuntimeLibrary); err != nil {
		return nil, err
	}
	det, err := loadModel(config.DetectionModel, ort.NewShape(1, 3, 960, 960), ort.NewShape(1, 960, 960))
	if err != nil {
		releaseRuntime()
		return nil, err
	}
	rec, err := loadModel(config.RecognitionModel, ort.NewShape(1, 3, 32, -1), ort.NewShape(1, -1, 13564))
	if err != nil {
		det.session.Destroy()
		releaseRuntime()
		return nil, err
	}
	paragraph, err := loadModel(config.ParagraphModel, ort.NewShape(1, 3, 768, 768), ort.NewShape(1, 768, 768))
	if err != nil {
		return nil, errors.Join(err, det.session.Destroy(), rec.session.Destroy(), releaseRuntime())
	}
	return &Engine{detector: det, recognizer: rec, paragraph: paragraph, charset: charset}, nil
}

// Close waits for active recognition and is idempotent.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	return errors.Join(e.detector.session.Destroy(), e.recognizer.session.Destroy(), e.paragraph.session.Destroy(), releaseRuntime())
}

// Recognize checks cancellation between inference stages; an active native
// inference call is not interrupted. Calls on one engine are serialized.
func (e *Engine) Recognize(ctx context.Context, src image.Image) (*Result, error) {
	timing := newStageTimer(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	timing.mark("engine_wait")
	if e.closed {
		return nil, errors.New("engine is closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if src == nil || src.Bounds().Empty() {
		return nil, errors.New("empty image")
	}
	bounds := src.Bounds()
	enhanced := HighResolutionEnabled(ctx) && min(bounds.Dx(), bounds.Dy()) >= 1600
	regions, direction, diagnostics, err := e.detectPasses(ctx, src, &timing)
	if err != nil {
		return nil, err
	}
	result := &Result{Width: bounds.Dx(), Height: bounds.Dy(), Lines: make([]Line, 0, len(regions)), Diagnostics: diagnostics}
	var pixels []float32
	lineGeometry := make(map[Line]quadrilateral)
	for _, regionIndex := range recognitionOrder(result.Diagnostics, enhanced) {
		region := regions[regionIndex]
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		diagnostic := &result.Diagnostics[regionIndex]
		if diagnostic.Reason == "split_parent" {
			continue
		}
		if diagnostic.Pass == 0 && enhanced && coveredByAcceptedLocal(*diagnostic, result.Diagnostics) {
			diagnostic.Reason = "covered_by_local"
			continue
		}
		if diagnostic.Reason == "redundant_detection" {
			if diagnostic.DuplicateOf != nil && result.Diagnostics[*diagnostic.DuplicateOf].Reason == "accepted" {
				continue
			}
			// The preferred crop failed recognition; try the overlapping crop rather
			// than losing content solely because of detector-stage deduplication.
			diagnostic.Reason = ""
		}
		q := region.quad
		left, top, right, bottom := q.bounds()
		if right <= left || bottom <= top {
			diagnostic.Reason = "invalid_geometry"
			continue
		}
		for i := range q {
			q[i].x += float64(bounds.Min.X)
			q[i].y += float64(bounds.Min.Y)
		}
		for i := range q {
			q[i] = point{float64(float32(q[i].x)), float64(float32(q[i].y))}
		}
		if region.score < 0.3 || region.area < 16 {
			diagnostic.Reason = "below_candidate_floor"
			continue
		}
		pixels = recognitionQuadTensor(src, q.oriented(direction), true)
		diagnostic.CropWidth = len(pixels) / (3 * 32)
		diagnostic.CropHeight = 32
		if diagnosticCrops(ctx) {
			diagnostic.CropPNG, err = tensorPNG(pixels)
			if err != nil {
				return nil, err
			}
		}
		timing.mark("recognizer_preprocess")
		prob, shape, err := e.recognizer.run(pixels)
		if err != nil {
			return nil, err
		}
		timing.mark("recognizer_inference")
		text, rate, evidence, err := e.decodeRecognition(prob, shape)
		if err != nil {
			return nil, err
		}
		if enhanced && region.score >= .7 && region.area < 1000 && evidence < .6 && narrowUprightCrop(q.oriented(direction)) {
			retryQuad := padNarrowCrop(q.oriented(direction))
			retryPixels := recognitionAlignedTensor(src, retryQuad, true, false)
			timing.mark("recognizer_decode")
			retryProb, retryShape, retryErr := e.recognizer.run(retryPixels)
			if retryErr != nil {
				return nil, retryErr
			}
			timing.mark("recognizer_inference")
			retryText, retryRate, retryEvidence, retryErr := e.decodeRecognition(retryProb, retryShape)
			if retryErr != nil {
				return nil, retryErr
			}
			if retryText != "" && retryEvidence >= .8 && retryEvidence > evidence+.1 {
				diagnostic.RetryOriginalText = text
				diagnostic.RetryMethod = "upright_small_crop"
				diagnostic.RecognitionQuad = make([][2]float64, 4)
				for j, p := range retryQuad {
					diagnostic.RecognitionQuad[j] = [2]float64{p.x - float64(bounds.Min.X), p.y - float64(bounds.Min.Y)}
				}
				text, rate, evidence = retryText, retryRate, retryEvidence
				diagnostic.CropWidth = len(retryPixels) / (3 * 32)
				if diagnosticCrops(ctx) {
					diagnostic.CropPNG, err = tensorPNG(retryPixels)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		timing.mark("recognizer_decode")
		// Candidate acceptance uses mean greedy emission evidence. The reported
		// sequence confidence has different semantics and must not share its cutoff.
		diagnostic.Text, diagnostic.Evidence = text, evidence
		if diagnostic.SplitMethod == "table_continuation" {
			diagnostic.ReviewReasons = continuationReviewReasons(text)
		}
		block := Line{float32(left), float32(top), float32(right), float32(bottom), rate, text, region.score}
		if text == "" {
			diagnostic.Reason = "empty_recognition"
			continue
		}
		areaForAcceptance := region.area
		// Tiny, clear local glyphs (for example table row numbers) should not
		// inherit the whole-page area cutoff after a high-resolution re-detection.
		if diagnostic.Pass > 0 && region.score >= .75 && evidence >= .9 && region.area >= 64 {
			areaForAcceptance = max(areaForAcceptance, 300)
		}
		reasons := candidateReasons(region.score, areaForAcceptance, evidence)
		if touchesTileBoundary(*diagnostic, result.Width, result.Height) {
			reasons = append(reasons, "tile_boundary")
		}
		if len(reasons) > 0 {
			diagnostic.Reason = "candidate"
			result.Candidates = append(result.Candidates, Candidate{Line: block, Reasons: reasons, Evidence: evidence})
			continue
		}
		diagnostic.Reason = "accepted"
		result.Lines = append(result.Lines, block)
		for i := range q {
			q[i].x -= float64(bounds.Min.X)
			q[i].y -= float64(bounds.Min.Y)
		}
		lineGeometry[block] = q
		timing.mark("line_assembly")
	}
	mergePassResults(result)
	if changed, joinErr := e.joinRecognizedFragments(ctx, src, result, direction, lineGeometry, &timing); joinErr != nil {
		return nil, joinErr
	} else if changed {
		mergePassResults(result)
	}
	sortLines(result.Lines, direction)
	timing.mark("line_order")
	if len(result.Lines) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pixels, _ = tensorRGB(src, 768, 768)
		timing.mark("paragraph_preprocess")
		prob, shape, err := e.paragraph.run(pixels)
		if err != nil {
			return nil, err
		}
		timing.mark("paragraph_inference")
		if !reflect.DeepEqual(shape, ort.NewShape(1, 768, 768)) {
			return nil, errors.New("invalid paragraph output shape")
		}
		rw, rh, _ := scaledImageSize(bounds, 768, 768)
		regions, err := paragraphRegions(prob, 768, 768, rw, rh, float32(bounds.Dx())/float32(rw), float32(bounds.Dy())/float32(rh))
		if err != nil {
			return nil, err
		}
		timing.mark("paragraph_postprocess")
		groupParagraphs(result, regions, direction, lineGeometry)
		orderRuledTables(result, src, direction)
		timing.mark("layout_order")
	}
	return result, nil
}

func (e *Engine) RecognizeFile(ctx context.Context, path string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := DecodeImage(f)
	if err != nil {
		return nil, err
	}
	return e.Recognize(ctx, src)
}
