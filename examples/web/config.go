package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/goccy/go-yaml"
)

type config struct {
	ListenAddress      string `yaml:"listen_address"`
	ListenPort         int    `yaml:"listen_port"`
	Token              string `yaml:"token"`
	MaxConcurrentTasks int    `yaml:"max_concurrent_tasks"`
	RequestsPerMinute  int    `yaml:"requests_per_minute"`
	ModelsDir          string `yaml:"models_dir"`
	RuntimeLibrary     string `yaml:"runtime_library"`
	LogEnabled         bool   `yaml:"log_enabled"`
	Debug              bool   `yaml:"debug"`
}

func defaultConfig() config {
	return config{ListenAddress: "127.0.0.1", ListenPort: 7676, MaxConcurrentTasks: 1, RequestsPerMinute: 60, ModelsDir: "../../models"}
}
func loadConfig(path string) (config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data), yaml.Strict())
	if err = decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return cfg, fmt.Errorf("config must contain one YAML document")
	}
	if err = cfg.validate(); err != nil {
		return cfg, err
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return cfg, err
	}
	if !filepath.IsAbs(cfg.ModelsDir) {
		cfg.ModelsDir = filepath.Join(base, cfg.ModelsDir)
	}
	if cfg.RuntimeLibrary == "" {
		name := "libonnxruntime.so"
		if runtime.GOOS == "darwin" {
			name = "libonnxruntime.dylib"
		}
		if runtime.GOOS == "windows" {
			name = "onnxruntime.dll"
		}
		cfg.RuntimeLibrary = filepath.Join(base, "../../runtime", name)
	} else if !filepath.IsAbs(cfg.RuntimeLibrary) {
		cfg.RuntimeLibrary = filepath.Join(base, cfg.RuntimeLibrary)
	}
	return cfg, nil
}
func (c config) validate() error {
	if net.ParseIP(c.ListenAddress) == nil && c.ListenAddress != "localhost" {
		return fmt.Errorf("listen_address must be an IP address or localhost")
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		return fmt.Errorf("listen_port must be between 1 and 65535")
	}
	if len(c.Token) < 16 || len(c.Token) > 1024 || strings.ContainsAny(c.Token, " \t\r\n") {
		return fmt.Errorf("token must contain 16 to 1024 characters without whitespace")
	}
	if c.MaxConcurrentTasks < 1 || c.MaxConcurrentTasks > 32 {
		return fmt.Errorf("max_concurrent_tasks must be between 1 and 32")
	}
	if c.RequestsPerMinute < 0 || c.RequestsPerMinute > 100000 {
		return fmt.Errorf("requests_per_minute must be between 0 and 100000")
	}
	if c.ModelsDir == "" {
		return fmt.Errorf("models_dir must not be empty")
	}
	return nil
}
