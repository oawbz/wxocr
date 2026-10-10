package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	ocr "wxocr"
)

type lifecycleStub struct {
	closed int
	fail   bool
}

func (s *lifecycleStub) Recognize(context.Context, image.Image) (*ocr.Result, error) {
	if s.fail {
		return nil, errors.New("inference failed")
	}
	return &ocr.Result{}, nil
}
func (s *lifecycleStub) Close() error { s.closed++; return nil }

func TestOnDemandLifecycle(t *testing.T) {
	loads := 0
	var sessions []*lifecycleStub
	e := &onDemandEngine{load: func() (managedRecognizer, error) {
		loads++
		s := &lifecycleStub{fail: loads == 2}
		sessions = append(sessions, s)
		return s, nil
	}}
	for i := 0; i < 3; i++ {
		_, err := e.Recognize(context.Background(), image.NewRGBA(image.Rect(0, 0, 1, 1)))
		if (err != nil) != (i == 1) {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if loads != 3 {
		t.Fatal(loads)
	}
	for _, s := range sessions {
		if s.closed != 1 {
			t.Fatal("session not released exactly once")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Recognize(ctx, nil); !errors.Is(err, context.Canceled) || loads != 3 {
		t.Fatal("canceled request loaded model")
	}
	e.Close()
	e.Close()
	if _, err := e.Recognize(context.Background(), nil); err == nil || loads != 3 {
		t.Fatal("closed engine loaded model")
	}
}

func TestOnDemandLoadFailureRetry(t *testing.T) {
	loads := 0
	e := &onDemandEngine{load: func() (managedRecognizer, error) {
		loads++
		if loads == 1 {
			return nil, errors.New("load failed")
		}
		return &lifecycleStub{}, nil
	}}
	if _, err := e.Recognize(context.Background(), nil); err == nil {
		t.Fatal("missing load error")
	}
	if _, err := e.Recognize(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestModelResidentConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	for _, value := range []string{"true", "false"} {
		if err := os.WriteFile(p, []byte("token: test-token-long-enough\nmodel_resident: "+value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(p)
		if err != nil || cfg.ModelResident != (value == "true") {
			t.Fatalf("%s: %+v %v", value, cfg, err)
		}
	}
	if !defaultConfig().ModelResident {
		t.Fatal("default must remain resident")
	}
}

func TestRealOnDemandEngine(t *testing.T) {
	lib := os.Getenv("WXOCR_RUNTIME")
	if lib == "" {
		t.Skip("set WXOCR_RUNTIME for real models")
	}
	cfg := defaultConfig()
	cfg.ModelResident = false
	cfg.RuntimeLibrary = lib
	cfg.ModelsDir = "../../models"
	e, err := newServiceEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		result, err := e.Recognize(context.Background(), img)
		if err != nil || len(result.Lines) == 0 {
			t.Fatalf("request %d: %v", i, err)
		}
		var texts []string
		for _, line := range result.Lines {
			texts = append(texts, line.Text)
		}
		if !reflect.DeepEqual(texts, []string{"微信模型跨平台测试", "中文识别2026", "Hello OCR 123"}) {
			t.Fatalf("request %d: unexpected text %v", i, texts)
		}
	}
}
