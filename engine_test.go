package ocr

import (
	"context"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestRecognitionPadding(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 100, 10))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	tensor := recognitionTensor(src, 0, 0, 19, 9, true)
	if len(tensor) != 3*32*128 || tensor[0] != 1 || tensor[66] != 1 || tensor[67] != -1 {
		t.Fatal("dynamic width must preserve text and black padding")
	}
	short := recognitionTensor(src, 0, 0, 9, 9, true)
	if len(short) != 3*32*64 {
		t.Fatal("short word bucket")
	}
	long := recognitionTensor(src, 0, 0, 999, 9, true)
	if len(long) != 3*32*960 || long[31*960] != -1 {
		t.Fatal("very long text must respect width cap and black bottom padding")
	}
}
func TestDetectorNormalizationAndPadding(t *testing.T) {
	src := image.NewNRGBA(image.Rect(7, 11, 11, 13))
	tensor, _ := tensorRGB(src, 4, 4)
	means := []float32{0.485, 0.456, 0.406}
	stds := []float32{0.229, 0.224, 0.225}
	for ch := range means {
		if math.Abs(float64(tensor[ch*16]-(-means[ch])/stds[ch])) > 1e-6 {
			t.Fatal("transparent pixels must retain their encoded black RGB")
		}
		if tensor[ch*16+8] != -means[ch]/stds[ch] {
			t.Fatal("detector padding must be normalized black")
		}
	}
}
func TestRecognitionFractionalSampling(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			src.SetGray(x, y, color.Gray{Y: uint8(x*3 + y*2)})
		}
	}
	tensor := recognitionTensor(src, 0.25, 0.5, 32.25, 32.5, true)
	// OpenCV INTER_LINEAR warp of this ramp gives 2,5,8 at its first row.
	for x, pixel := range []float32{2, 5, 8} {
		if math.Abs(float64(tensor[x]-(pixel/127.5-1))) > 1e-6 {
			t.Fatal("fractional warp interpolation")
		}
	}
}
func TestCTC(t *testing.T) {
	labels := []int{1, 1, 0, 1, 2, 2, 3, 0}
	prob := make([]float32, len(labels)*4)
	for i, label := range labels {
		prob[i*4+label] = 1
	}
	text, rate, err := decodeCTC(prob, len(labels), []string{"A", " ", "中"})
	if err != nil || text != "AA 中" || rate != 1 {
		t.Fatalf("%q %v %v", text, rate, err)
	}
	prob[0] = float32(math.NaN())
	if _, _, err = decodeCTC(prob, len(labels), []string{"A", " ", "中"}); err == nil {
		t.Fatal("accepted NaN")
	}
}
func TestKernelSeparatesConnectedScore(t *testing.T) {
	width, height := 32, 16
	score := make([]float32, width*height)
	kernel := make([]float32, width*height/16)
	for y := 4; y < 12; y++ {
		for x := 4; x < 28; x++ {
			score[y*width+x] = 1
		}
	}
	for _, x := range []int{1, 2, 5, 6} {
		kernel[8+x] = 1
	}
	regions, err := kernelRegions(score, kernel, width, height, 0.5)
	if err != nil || len(regions) != 2 {
		t.Fatalf("regions %v error %v", regions, err)
	}
	if regions[0].box.Max.X > regions[1].box.Min.X {
		t.Fatal("text seeds merged")
	}
}
func TestInvalidMaps(t *testing.T) {
	if _, err := kernelRegions([]float32{1}, nil, 32, 16, 0.5); err == nil {
		t.Fatal("accepted invalid shape")
	}
	score := make([]float32, 32*16)
	score[0] = float32(math.Inf(1))
	if _, err := kernelRegions(score, make([]float32, 32), 32, 16, 0.5); err == nil {
		t.Fatal("accepted infinity")
	}
}

func TestRealModelsLifecycle(t *testing.T) {
	c := Config{RuntimeLibrary: os.Getenv("WXOCR_RUNTIME"), DetectionModel: os.Getenv("WXOCR_DETECTOR"), RecognitionModel: os.Getenv("WXOCR_RECOGNIZER"), ParagraphModel: os.Getenv("WXOCR_PARAGRAPH"), CharsetFile: os.Getenv("WXOCR_CHARSET")}
	if c.RuntimeLibrary == "" {
		t.Skip("set WXOCR_RUNTIME, WXOCR_DETECTOR, WXOCR_RECOGNIZER and WXOCR_CHARSET")
	}
	first, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = second.RecognizeFile(ctx, "testdata/native-cases/chinese.png"); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	result, err := second.RecognizeFile(context.Background(), "testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, block := range result.Lines {
		texts = append(texts, block.Text)
	}
	if !reflect.DeepEqual(texts, []string{"微信模型跨平台测试", "中文识别2026", "Hello OCR 123"}) {
		t.Fatalf("%v", texts)
	}
	if _, err = first.Recognize(context.Background(), image.NewGray(image.Rect(0, 0, 2, 2))); err == nil {
		t.Fatal("closed engine accepted call")
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	bad := c
	bad.DetectionModel = "missing-model.onnx"
	if _, err = New(bad); err == nil {
		t.Fatal("accepted missing model")
	}
	rebuilt, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
}
