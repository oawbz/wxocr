package ocr

import (
	"errors"
	ort "github.com/yalue/onnxruntime_go"
	"math"
)

func (e *Engine) decodeRecognition(data []float32, shape ort.Shape) (string, float32, float32, error) {
	if len(shape) != 3 || shape[0] != 1 || shape[1] < 1 {
		return "", 0, 0, errors.New("invalid recognition output shape")
	}
	if e.recognizer.compact {
		if shape[2] != shape[1]+2 {
			return "", 0, 0, errors.New("invalid compact recognition output shape")
		}
		return recognitionCompactCTC(data, int(shape[1]), e.charset)
	}
	if shape[2] != int64(len(e.charset)+1) {
		return "", 0, 0, errors.New("invalid recognition output shape")
	}
	if !e.recognizer.probabilities {
		return recognitionLogitsCTC(data, int(shape[1]), e.charset)
	}
	return recognitionCTC(data, int(shape[1]), e.charset)
}
func narrowUprightCrop(q quadrilateral) bool {
	width := math.Hypot(q[1].x-q[0].x, q[1].y-q[0].y)
	height := math.Hypot(q[3].x-q[0].x, q[3].y-q[0].y)
	return width > 0 && height/width >= 1.5 && height/width <= 4
}

// Restore a small margin along the baseline of a narrowly detected glyph.
// The margin is proportional to glyph height, independent of its contents.
func padNarrowCrop(q quadrilateral) quadrilateral {
	width := math.Hypot(q[1].x-q[0].x, q[1].y-q[0].y)
	height := math.Hypot(q[3].x-q[0].x, q[3].y-q[0].y)
	if width <= 0 {
		return q
	}
	dx, dy := (q[1].x-q[0].x)/width*height*.15, (q[1].y-q[0].y)/width*height*.15
	q[0].x -= dx
	q[0].y -= dy
	q[3].x -= dx
	q[3].y -= dy
	q[1].x += dx
	q[1].y += dy
	q[2].x += dx
	q[2].y += dy
	return q
}
