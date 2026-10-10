package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	ocr "wxocr"
)

//go:embed index.html
var page []byte

const maxUpload = 20 << 20
const recognitionTimeout = 90 * time.Second

type recognizer interface {
	Recognize(context.Context, image.Image) (*ocr.Result, error)
}

// Admission happens before upload parsing or image allocation. No requests queue in memory.
type admission struct {
	mu            sync.Mutex
	active, limit int
	draining      bool
	idle          chan struct{}
	rate          int
	tokens        float64
	updated       time.Time
}

func newAdmission(cfg config) *admission {
	idle := make(chan struct{})
	close(idle)
	return &admission{limit: cfg.MaxConcurrentTasks, idle: idle, rate: cfg.RequestsPerMinute, tokens: float64(cfg.RequestsPerMinute), updated: time.Now()}
}
func (a *admission) enter() (string, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.draining {
		return "service_stopping", 1
	}
	if a.active >= a.limit {
		return "server_busy", 1
	}
	if a.rate > 0 {
		now := time.Now()
		a.tokens = min(float64(a.rate), a.tokens+now.Sub(a.updated).Seconds()*float64(a.rate)/60)
		a.updated = now
		if a.tokens < 1 {
			return "rate_limited", max(1, int((1-a.tokens)*60/float64(a.rate))+1)
		}
		a.tokens--
	}
	if a.active == 0 {
		a.idle = make(chan struct{})
	}
	a.active++
	return "", 0
}
func (a *admission) leave() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active--
	if a.active == 0 {
		close(a.idle)
	}
}
func (a *admission) drain() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.draining = true
	return a.idle
}
func apiError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
func requestCanceled(c *gin.Context) bool {
	if err := c.Request.Context().Err(); err != nil {
		if err == context.DeadlineExceeded {
			apiError(c, 504, "recognition_timeout", "识别超时，请缩小图片后重试")
		}
		return true
	}
	return false
}
func router(engine recognizer, cfg config, gate *admission, logger *log.Logger) *gin.Engine {
	mode := gin.ReleaseMode
	if cfg.Debug {
		mode = gin.DebugMode
	}
	gin.SetMode(mode)
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(gin.CustomRecoveryWithWriter(logger.Writer(), func(c *gin.Context, _ any) { apiError(c, 500, "internal_error", "服务处理失败") }))
	if cfg.LogEnabled {
		r.Use(func(c *gin.Context) {
			start := time.Now()
			c.Next()
			logger.Printf("%s %s status=%d elapsed=%s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
		})
	}
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	r.GET("/", func(c *gin.Context) { c.Data(200, "text/html; charset=utf-8", page) })
	expected := sha256.Sum256([]byte("Bearer " + cfg.Token))
	api := r.Group("/api")
	api.Use(func(c *gin.Context) {
		provided := sha256.Sum256([]byte(c.GetHeader("Authorization")))
		if subtle.ConstantTimeCompare(provided[:], expected[:]) != 1 {
			c.Header("WWW-Authenticate", "Bearer")
			apiError(c, 401, "unauthorized", "Token 不正确或未填写")
			return
		}
		c.Next()
	})
	api.POST("/ocr", func(c *gin.Context) {
		previewRequested := c.Query("preview")
		if previewRequested != "" && previewRequested != "true" && previewRequested != "false" {
			apiError(c, 400, "invalid_preview", "preview 必须为 true 或 false")
			return
		}
		code, retry := gate.enter()
		if code != "" {
			c.Header("Retry-After", strconv.Itoa(retry))
			status := 429
			message := "服务正在处理其他任务，请稍后重试"
			if code == "rate_limited" {
				message = "请求过于频繁，请稍后重试"
			}
			if code == "service_stopping" {
				status = 503
				message = "服务正在停止"
			}
			apiError(c, status, code, message)
			return
		}
		defer gate.leave()
		start := time.Now()
		requestContext, cancel := context.WithTimeout(c.Request.Context(), recognitionTimeout)
		defer cancel()
		c.Request = c.Request.WithContext(requestContext)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUpload)
		err := c.Request.ParseMultipartForm(1 << 20)
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		if err != nil {
			status, code := 400, "invalid_upload"
			if _, ok := err.(*http.MaxBytesError); ok {
				status, code = 413, "upload_too_large"
			}
			apiError(c, status, code, "请上传有效图片，完整请求不超过 20 MiB")
			return
		}
		file, _, err := c.Request.FormFile("image")
		if err != nil {
			apiError(c, 400, "missing_image", "缺少 image 文件")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			apiError(c, 400, "invalid_upload", "读取图片失败")
			return
		}
		dimensions, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || (format != "jpeg" && format != "png") || dimensions.Width <= 0 || dimensions.Height <= 0 || int64(dimensions.Width)*int64(dimensions.Height) > 25_000_000 {
			apiError(c, 400, "invalid_image", "仅支持 JPEG / PNG，最多 2500 万像素")
			return
		}
		if requestCanceled(c) {
			return
		}
		src, err := ocr.DecodeImage(bytes.NewReader(data))
		if err != nil {
			apiError(c, 400, "invalid_image", "图片解码失败")
			return
		}
		result, err := engine.Recognize(c.Request.Context(), src)
		if err != nil {
			if requestCanceled(c) {
				return
			}
			logger.Printf("recognize: %v", err)
			apiError(c, 500, "recognition_failed", "识别失败")
			return
		}
		if requestCanceled(c) {
			return
		}
		response := gin.H{"result": result, "elapsed_ms": time.Since(start).Milliseconds()}
		// API clients can omit preview to save PNG encoding and response traffic.
		if previewRequested == "true" {
			var preview bytes.Buffer
			if err := png.Encode(&preview, src); err != nil {
				apiError(c, 500, "preview_failed", "生成预览失败")
				return
			}
			response["preview_png"] = preview.Bytes()
		}
		if requestCanceled(c) {
			return
		}
		c.JSON(200, response)
	})
	r.NoRoute(func(c *gin.Context) { apiError(c, 404, "not_found", "接口不存在") })
	r.NoMethod(func(c *gin.Context) { apiError(c, 405, "method_not_allowed", "请求方法不支持") })
	r.HandleMethodNotAllowed = true
	return r
}
func run() error {
	path := flag.String("config", "config.yaml", "YAML configuration file")
	flag.Parse()
	cfg, err := loadConfig(*path)
	if err != nil {
		return err
	}
	writer := io.Writer(io.Discard)
	if cfg.LogEnabled {
		writer = os.Stderr
	}
	logger := log.New(writer, "", log.LstdFlags)
	gin.DefaultWriter = writer
	gin.DefaultErrorWriter = writer
	engine, err := newServiceEngine(cfg)
	if err != nil {
		return fmt.Errorf("load OCR models: %w", err)
	}
	defer engine.Close()
	gate := newAdmission(cfg)
	addr := net.JoinHostPort(cfg.ListenAddress, strconv.Itoa(cfg.ListenPort))
	server := &http.Server{Addr: addr, Handler: router(engine, cfg, gate, logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, ErrorLog: logger}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	logger.Printf("OCR ready: http://%s", addr)
	select {
	case err := <-done:
		idle := gate.drain()
		<-idle
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		idle := gate.drain()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			logger.Printf("shutdown: %v", err)
			server.Close()
		}
		<-idle
		return nil
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
