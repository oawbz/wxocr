package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	ocr "wxocr"
)

type blockingEngine struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (e *blockingEngine) Recognize(ctx context.Context, src image.Image) (*ocr.Result, error) {
	e.calls.Add(1)
	close(e.entered)
	select {
	case <-e.release:
		return &ocr.Result{Width: src.Bounds().Dx(), Height: src.Bounds().Dy()}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type watchedBody struct{ reads int }

func (b *watchedBody) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }
func (b *watchedBody) Close() error             { return nil }
func authenticatedRequest(handler http.Handler, method, url, token string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, body)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func TestAuthenticationBeforeReadingUpload(t *testing.T) {
	e := &stubEngine{}
	r := testRouter(e)
	for _, token := range []string{"", "wrong-token"} {
		body := &watchedBody{}
		w := authenticatedRequest(r, "POST", "/api/ocr", token, body)
		if w.Code != 401 || body.reads != 0 || e.called {
			t.Fatal("unauthenticated request read data or reached model")
		}
		if !strings.Contains(w.Body.String(), "unauthorized") {
			t.Fatal("missing structured error")
		}
	}
}
func TestBusyBeforeReadingAndDrain(t *testing.T) {
	e := &blockingEngine{entered: make(chan struct{}), release: make(chan struct{})}
	cfg := testConfig()
	gate := newAdmission(cfg)
	r := router(e, cfg, gate, log.New(io.Discard, "", 0))
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- upload(t, r, data) }()
	select {
	case <-e.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first task did not enter model")
	}
	body := &watchedBody{}
	w := authenticatedRequest(r, "POST", "/api/ocr", cfg.Token, body)
	if w.Code != 429 || body.reads != 0 || w.Header().Get("Retry-After") == "" || e.calls.Load() != 1 {
		t.Fatal("busy task was not rejected before body processing")
	}
	idle := gate.drain()
	select {
	case <-idle:
		t.Fatal("drain finished with active model call")
	default:
	}
	w = authenticatedRequest(r, "POST", "/api/ocr", cfg.Token, nil)
	if w.Code != 503 {
		t.Fatal("draining service accepted work")
	}
	close(e.release)
	if (<-done).Code != 200 {
		t.Fatal("active task failed during drain")
	}
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("active task not released")
	}
}
func TestRateLimitAndSlotRelease(t *testing.T) {
	cfg := testConfig()
	cfg.RequestsPerMinute = 1
	gate := newAdmission(cfg)
	r := router(&stubEngine{}, cfg, gate, log.New(io.Discard, "", 0))
	body := &watchedBody{}
	w := authenticatedRequest(r, "POST", "/api/ocr", cfg.Token, body)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	if gate.active != 0 {
		t.Fatal("invalid upload leaked admission slot")
	}
	body = &watchedBody{}
	w = authenticatedRequest(r, "POST", "/api/ocr", cfg.Token, body)
	if w.Code != 429 || body.reads != 0 || !strings.Contains(w.Body.String(), "rate_limited") {
		t.Fatal("rate limit failed")
	}
	gate.mu.Lock()
	gate.updated = time.Now().Add(-61 * time.Second)
	gate.mu.Unlock()
	if code, _ := gate.enter(); code != "" {
		t.Fatal("token did not replenish")
	}
	gate.leave()
}

type failedEngine struct{ panicNow bool }

func (e failedEngine) Recognize(context.Context, image.Image) (*ocr.Result, error) {
	if e.panicNow {
		panic("test panic")
	}
	return nil, errors.New("test failure")
}
func TestFailureAndPanicReleaseSlot(t *testing.T) {
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, panicNow := range []bool{false, true} {
		cfg := testConfig()
		gate := newAdmission(cfg)
		var logs bytes.Buffer
		r := router(failedEngine{panicNow}, cfg, gate, log.New(&logs, "", 0))
		for i := 0; i < 2; i++ {
			if w := upload(t, r, data); w.Code != 500 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		if gate.active != 0 {
			t.Fatal("failure leaked admission slot")
		}
		if strings.Contains(logs.String(), cfg.Token) {
			t.Fatal("recovery logs exposed token")
		}
	}
}
func TestNoPreviewByDefaultAndInvalidOption(t *testing.T) {
	cfg := testConfig()
	e := &stubEngine{}
	r := testRouter(e)
	w := authenticatedRequest(r, "POST", "/api/ocr?preview=invalid", cfg.Token, nil)
	if w.Code != 400 || e.called {
		t.Fatal("invalid preview reached model")
	}
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		q.URL.RawQuery = ""
		r.ServeHTTP(w, q)
	})
	w = upload(t, wrapper, data)
	if w.Code != 200 || strings.Contains(w.Body.String(), "preview_png") {
		t.Fatal("default API unnecessarily encoded preview")
	}
}
func TestConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	valid := "token: test-token-long-enough\nmodels_dir: ./models\n"
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogEnabled || cfg.Debug || cfg.ListenPort != 7676 || cfg.MaxConcurrentTasks != 1 || cfg.ModelsDir != filepath.Join(dir, "models") || !filepath.IsAbs(cfg.RuntimeLibrary) {
		t.Fatal("invalid defaults or path resolution")
	}
	for _, content := range []string{valid + "unknown_option: 1\n", valid + "listen_port: 0\n", valid + "max_concurrent_tasks: 0\n", valid + "requests_per_minute: -1\n", "token: short\n", valid + "---\ntoken: another-token-long-enough\n", valid + "debug: false\ndebug: true\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = loadConfig(path); err == nil {
			t.Fatalf("accepted invalid configuration %q", content)
		}
	}
}

func TestMultipartTemporaryFilesRemoved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	data := make([]byte, 2<<20)
	w := upload(t, testRouter(&stubEngine{}), data)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("multipart upload left temporary files")
	}
}
func TestCancellationReleasesSlot(t *testing.T) {
	cfg := testConfig()
	gate := newAdmission(cfg)
	e := &stubEngine{}
	r := router(e, cfg, gate, log.New(io.Discard, "", 0))
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		ctx, cancel := context.WithCancel(q.Context())
		cancel()
		r.ServeHTTP(w, q.WithContext(ctx))
	})
	upload(t, wrapper, data)
	if e.called || gate.active != 0 {
		t.Fatal("canceled request reached model or retained slot")
	}
}
