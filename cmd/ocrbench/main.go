// ocrbench measures decoded-image stages without HTTP or preview overhead.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"
	ocr "wxocr"
)

func main() {
	runtimeLibrary := flag.String("runtime", "runtime/libonnxruntime.dylib", "runtime library")
	models := flag.String("models", "models", "model directory")
	runs := flag.Int("runs", 5, "measured requests per image, after one warmup")
	profile := flag.String("cpuprofile", "", "CPU profile output")
	output := flag.String("output", "", "JSON output (default stdout)")
	flag.Parse()
	if *runs < 1 || len(flag.Args()) == 0 {
		log.Fatal("provide images and runs >= 1")
	}
	engine, err := ocr.New(ocr.Config{RuntimeLibrary: *runtimeLibrary, DetectionModel: filepath.Join(*models, "detection.onnx"), RecognitionModel: filepath.Join(*models, "recognition.onnx"), ParagraphModel: filepath.Join(*models, "paragraph.onnx"), CharsetFile: filepath.Join(*models, "charset_zh13562.txt")})
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()
	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			log.Fatal(err)
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	type sample struct {
		Image   string             `json:"image"`
		Run     int                `json:"run"`
		TotalMS float64            `json:"total_ms"`
		Stages  map[string]float64 `json:"stages_ms"`
		Result  *ocr.Result        `json:"result"`
	}
	samples := []sample{}
	for _, path := range flag.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		for run := -1; run < *runs; run++ {
			stages := map[string]float64{}
			start := time.Now()
			src, err := ocr.DecodeImage(bytes.NewReader(data))
			if err != nil {
				log.Fatal(err)
			}
			stages["decode"] = float64(time.Since(start)) / float64(time.Millisecond)
			ctx := ocr.WithTimingObserver(context.Background(), func(name string, d time.Duration) { stages[name] += float64(d) / float64(time.Millisecond) })
			result, err := engine.Recognize(ctx, src)
			if err != nil {
				log.Fatal(err)
			}
			if run >= 0 {
				samples = append(samples, sample{filepath.Base(path), run, float64(time.Since(start)) / float64(time.Millisecond), stages, result})
			}
		}
	}
	raw, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if *output != "" {
		if err = os.WriteFile(*output, raw, 0644); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println(string(raw))
	}
}
