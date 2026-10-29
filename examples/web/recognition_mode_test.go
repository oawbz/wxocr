package main

import (
	"context"
	"image"
	"net/http"
	"os"
	"strings"
	"testing"
	ocr "wxocr"
)

type modeEngine struct{ modes []bool }

func (e *modeEngine) Recognize(ctx context.Context, src image.Image) (*ocr.Result, error) {
	e.modes = append(e.modes, ocr.HighResolutionEnabled(ctx))
	return &ocr.Result{Width: src.Bounds().Dx(), Height: src.Bounds().Dy(), Lines: []ocr.Line{}}, nil
}
func TestRecognitionModeAPI(t *testing.T) {
	data, err := os.ReadFile("../../testdata/native-cases/chinese.png")
	if err != nil {
		t.Fatal(err)
	}
	e := &modeEngine{}
	r := testRouter(e)
	for _, tc := range []struct {
		query, want string
		enhanced    bool
	}{{"", "fast", false}, {"fast", "fast", false}, {"high_resolution", "high_resolution", true}, {"fast", "fast", false}} {
		wrapper := http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) { q.URL.RawQuery = "mode=" + tc.query; r.ServeHTTP(w, q) })
		w := upload(t, wrapper, data)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"mode":"`+tc.want+`"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if e.modes[len(e.modes)-1] != tc.enhanced {
			t.Fatal("wrong mode reached engine")
		}
	}
	body := &watchedBody{}
	w := authenticatedRequest(r, "POST", "/api/ocr?mode=invalid", testConfig().Token, body)
	if w.Code != 400 || body.reads != 0 || len(e.modes) != 4 || !strings.Contains(w.Body.String(), "invalid_mode") {
		t.Fatal("invalid mode reached upload or engine", w.Code, w.Body.String())
	}
}
