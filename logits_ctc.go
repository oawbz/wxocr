package ocr

import "wxocr/internal/ctcfast"

func recognitionLogitsCTC(data []float32, steps int, charset []string) (string, float32, float32, error) {
	compact, err := ctcfast.Compact(data, steps, len(charset)+1)
	if err != nil {
		return "", 0, 0, err
	}
	return recognitionCompactCTC(compact, steps, charset)
}
