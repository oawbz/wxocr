// Package ocr performs OCR in Go using converted XNet models.
// It requires ONNX Runtime through cgo, but neither Python nor WeChat libraries.
package ocr

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
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
	// InferenceThreads controls threads within each model operator; 0 selects ORT defaults.
	InferenceThreads int
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
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Paragraphs []Paragraph `json:"paragraphs,omitempty"`
	Lines      []Line      `json:"lines"`
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
	session *ort.DynamicAdvancedSession
	shape   ort.Shape
	outputs int
}

func loadModel(path string, shape ort.Shape, outputShape ort.Shape, threads int) (*model, error) {
	inputs, outputs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return nil, err
	}
	dynamicRecognition := len(inputs) == 1 && shape[2] == 32 && reflect.DeepEqual(inputs[0].Dimensions, ort.NewShape(1, 3, 32, -1)) && len(outputs) > 0 && reflect.DeepEqual(outputs[0].Dimensions, ort.NewShape(1, -1, 13564))
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
	if shape[2] == 32 && (len(outputs) != 1 || outputs[0].Name != "logits") {
		return nil, errors.New("recognizer requires a recognition model with logits output")
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer options.Destroy()
	if err = options.SetIntraOpNumThreads(threads); err != nil {
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
	return &model{session, shape, len(names)}, nil
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
	if config.InferenceThreads < 0 || config.InferenceThreads > 128 {
		return nil, errors.New("inference threads must be between 0 and 128")
	}
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
	det, err := loadModel(config.DetectionModel, ort.NewShape(1, 3, 960, 960), ort.NewShape(1, 960, 960), config.InferenceThreads)
	if err != nil {
		releaseRuntime()
		return nil, err
	}
	rec, err := loadModel(config.RecognitionModel, ort.NewShape(1, 3, 32, -1), ort.NewShape(1, -1, 13564), config.InferenceThreads)
	if err != nil {
		det.session.Destroy()
		releaseRuntime()
		return nil, err
	}
	paragraph, err := loadModel(config.ParagraphModel, ort.NewShape(1, 3, 768, 768), ort.NewShape(1, 768, 768), config.InferenceThreads)
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
	e.mu.Lock()
	defer e.mu.Unlock()
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
	dw, dh := detectorSize(bounds)
	pixels, _ := tensorRGB(src, dw, dh)
	rw, rh, _ := scaledImageSize(bounds, dw, dh)
	scaleY := float64(rh) / float64(bounds.Dy())
	scaleX := float64(rw) / float64(bounds.Dx())
	maps, _, err := e.detector.runAll(pixels, int64(dh), int64(dw))
	if err != nil {
		return nil, err
	}
	direction, _, err := pageOrientation(maps[2])
	if err != nil {
		return nil, err
	}
	regions, err := kernelRegions(maps[0], maps[1], dw, dh, 0.5)
	if err != nil {
		return nil, err
	}
	result := &Result{Width: bounds.Dx(), Height: bounds.Dy(), Lines: make([]Line, 0, len(regions))}
	lineGeometry := make(map[Line]quadrilateral)
	for _, region := range regions {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if region.score < 0.65 || region.area < 300 {
			continue
		}
		q := region.quad
		for i, p := range q {
			q[i] = point{math.Max(0, math.Min(float64(bounds.Dx()-1), p.x/scaleX)), math.Max(0, math.Min(float64(bounds.Dy()-1), p.y/scaleY))}
		}
		left, top, right, bottom := q.bounds()
		if right <= left || bottom <= top {
			continue
		}
		for i := range q {
			q[i].x += float64(bounds.Min.X)
			q[i].y += float64(bounds.Min.Y)
		}
		for i := range q {
			q[i] = point{float64(float32(q[i].x)), float64(float32(q[i].y))}
		}
		pixels = recognitionQuadTensor(src, q.oriented(direction), true)
		prob, shape, err := e.recognizer.run(pixels)
		if err != nil {
			return nil, err
		}
		if len(shape) != 3 || shape[0] != 1 || shape[2] != int64(len(e.charset)+1) {
			return nil, errors.New("invalid recognition output shape")
		}
		if err := recognitionProbabilities(prob, len(e.charset)+1); err != nil {
			return nil, err
		}
		text, rate, evidence, err := recognitionCTC(prob, int(shape[1]), e.charset)
		if err != nil {
			return nil, err
		}
		// Candidate acceptance uses mean greedy emission evidence. The reported
		// sequence confidence has different semantics and must not share its cutoff.
		if text == "" || evidence <= 0.4 {
			continue
		}
		block := Line{float32(left), float32(top), float32(right), float32(bottom), rate, text, region.score}
		result.Lines = append(result.Lines, block)
		for i := range q {
			q[i].x -= float64(bounds.Min.X)
			q[i].y -= float64(bounds.Min.Y)
		}
		lineGeometry[block] = q
	}
	sortLines(result.Lines, direction)
	if len(result.Lines) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pixels, _ = tensorRGB(src, 768, 768)
		prob, shape, err := e.paragraph.run(pixels)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(shape, ort.NewShape(1, 768, 768)) {
			return nil, errors.New("invalid paragraph output shape")
		}
		rw, rh, _ := scaledImageSize(bounds, 768, 768)
		regions, err := paragraphRegions(prob, 768, 768, rw, rh, float32(bounds.Dx())/float32(rw), float32(bounds.Dy())/float32(rh))
		if err != nil {
			return nil, err
		}
		groupParagraphs(result, regions, direction, lineGeometry)
		orderRuledTables(result, src, direction)
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
