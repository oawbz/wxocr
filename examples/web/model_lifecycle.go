package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
	"time"

	ocr "wxocr"
)

type managedRecognizer interface {
	recognizer
	Close() error
}

// Serialize loading and inference to retain the single-engine memory bound.
type onDemandEngine struct {
	mu     sync.Mutex
	closed bool
	load   func() (managedRecognizer, error)
}

func newServiceEngine(cfg config) (managedRecognizer, error) {
	load := func() (*ocr.Engine, error) {
		return ocr.New(ocr.Config{RuntimeLibrary: cfg.RuntimeLibrary, DetectionModel: filepath.Join(cfg.ModelsDir, "detection.onnx"), RecognitionModel: filepath.Join(cfg.ModelsDir, "recognition.onnx"), ParagraphModel: filepath.Join(cfg.ModelsDir, "paragraph.onnx"), CharsetFile: filepath.Join(cfg.ModelsDir, "charset_zh13562.txt")})
	}
	if cfg.ModelResident {
		resident := func() (managedRecognizer, error) {
			e, err := load()
			if err != nil {
				return nil, err
			}
			if cfg.StartupWarmup {
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				err = e.Warmup(ctx)
				cancel()
				if err != nil {
					return nil, errors.Join(fmt.Errorf("startup warmup: %w", err), e.Close())
				}
				fmt.Fprintf(os.Stderr, "OCR startup warmup completed in %s\n", time.Since(start).Round(time.Millisecond))
			}
			return e, nil
		}
		workers := cfg.EngineWorkers
		if workers < 1 {
			workers = 1
		}
		engines := make([]managedRecognizer, 0, workers)
		for i := 0; i < workers; i++ {
			e, err := resident()
			if err != nil {
				for _, prior := range engines {
					err = errors.Join(err, prior.Close())
				}
				return nil, err
			}
			engines = append(engines, e)
		}
		if len(engines) == 1 {
			return engines[0], nil
		}
		return newEnginePool(engines), nil
	}
	return &onDemandEngine{load: func() (managedRecognizer, error) { return load() }}, nil
}

func (e *onDemandEngine) Recognize(ctx context.Context, src image.Image) (result *ocr.Result, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, errors.New("engine is closed")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	engine, err := e.load()
	if err != nil {
		return nil, fmt.Errorf("load OCR models: %w", err)
	}
	defer func() { err = errors.Join(err, engine.Close()) }()
	return engine.Recognize(ctx, src)
}

func (e *onDemandEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return nil
}
