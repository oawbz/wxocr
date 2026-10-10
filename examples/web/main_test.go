package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"io"
	"log"
	"mime/multipart"
	"net/http"

	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	ocr "wxocr"
)

type stubEngine struct{ called bool }

func (s *stubEngine) Recognize(_ context.Context, src image.Image) (*ocr.Result, error) {
	s.called = true
	return &ocr.Result{Width: src.Bounds().Dx(), Height: src.Bounds().Dy(), Lines: []ocr.Line{{Text: "test", Confidence: .9}}}, nil
}

func upload(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "test.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/ocr?preview=true", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+testConfig().Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	return response
}

func TestUploadAndPage(t *testing.T) {
	engine := &stubEngine{}
	r := testRouter(engine)
	page := httptest.NewRecorder()
	r.ServeHTTP(page, httptest.NewRequest("GET", "/", nil))
	if page.Code != 200 || !bytes.Contains(page.Body.Bytes(), []byte("/api/ocr")) {
		t.Fatal("missing embedded page")
	}
	bad := upload(t, r, []byte("not an image"))
	if bad.Code != 400 || engine.called {
		t.Fatal("invalid image reached engine")
	}
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	good := upload(t, r, data)
	if good.Code != 200 || !engine.called {
		t.Fatalf("upload: %s", good.Body.String())
	}
	if bytes.Contains(good.Body.Bytes(), []byte(`"document"`)) {
		t.Fatal("backend must not classify documents")
	}
	var result struct {
		Preview []byte `json:"preview_png"`
	}
	if err := json.Unmarshal(good.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(result.Preview)); err != nil {
		t.Fatal(err)
	}
	response := upload(t, r, make([]byte, maxUpload+1))
	if response.Code != 413 {
		t.Fatal("accepted oversized request")
	}

}

func TestRealEngineUpload(t *testing.T) {
	lib := os.Getenv("WXOCR_RUNTIME")
	if lib == "" {
		t.Skip("set WXOCR_RUNTIME for real model HTTP integration")
	}
	dir := filepath.Join("../..", "models")
	engine, err := ocr.New(ocr.Config{RuntimeLibrary: lib, DetectionModel: filepath.Join(dir, "detection.onnx"), RecognitionModel: filepath.Join(dir, "recognition.onnx"), ParagraphModel: filepath.Join(dir, "paragraph.onnx"), CharsetFile: filepath.Join(dir, "charset_zh13562.txt")})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	response := upload(t, testRouter(engine), data)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var got struct {
		Result ocr.Result `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Result.Lines) == 0 {
		t.Fatal("real engine returned no text")
	}
	t.Logf("real HTTP upload: %d lines, first=%q", len(got.Result.Lines), got.Result.Lines[0].Text)
}

func testConfig() config {
	c := defaultConfig()
	c.Token = "test-token-long-enough"
	c.RequestsPerMinute = 0
	return c
}
func testRouter(e recognizer) http.Handler {
	c := testConfig()
	return router(e, c, newAdmission(c), log.New(io.Discard, "", 0))
}
