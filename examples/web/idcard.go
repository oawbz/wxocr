package main

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	ocr "wxocr"
)

type documentResult struct {
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Side        string            `json:"side,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
	Missing     []string          `json:"missing_fields,omitempty"`
	NeedsReview bool              `json:"needs_review"`
}

var idNumberPattern = regexp.MustCompile(`[0-9]{17}[0-9Xx]`)
var birthPattern = regexp.MustCompile(`(?:[0-9]{4}年[0-9]{1,2}月[0-9]{1,2}日|[0-9]{4}[-./][0-9]{1,2}[-./][0-9]{1,2})`)
var idLabels = []string{"公民身份号码", "签发机关", "有效期限", "姓名", "性别", "民族", "出生", "住址"}

func compactText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == ':' || r == '：' {
			return -1
		}
		return r
	}, s)
}

// classifyDocument structures OCR output without changing the recognition engine.
// Labels and nearby boxes are used instead of fixed pixel coordinates.
func classifyDocument(result *ocr.Result) documentResult {
	doc := documentResult{Type: "unknown", Name: "普通图片", NeedsReview: false}
	lines := append([]ocr.Line(nil), result.Lines...)
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].Top == lines[j].Top {
			return lines[i].Left < lines[j].Left
		}
		return lines[i].Top < lines[j].Top
	})
	var all strings.Builder
	for _, l := range lines {
		all.WriteString(compactText(l.Text))
		all.WriteByte('\n')
	}
	text := all.String()
	has := func(label string) bool { return strings.Contains(text, label) }
	front := 0
	for _, label := range []string{"姓名", "性别", "民族", "出生", "住址", "公民身份号码"} {
		if has(label) {
			front++
		}
	}
	back := has("签发机关") && has("有效期限")
	if !back && !(front >= 5 || front >= 2 && (has("公民身份号码") || idNumberPattern.MatchString(text))) {
		return doc
	}
	doc.Type = "id_card"
	doc.Name = "身份证"
	doc.Side = "front"
	doc.Fields = map[string]string{}
	if back {
		doc.Side = "back"
	}
	labels := []string{"姓名", "性别", "民族", "出生", "住址", "公民身份号码"}
	if back {
		labels = []string{"签发机关", "有效期限"}
	}
	for _, label := range labels {
		for _, anchor := range lines {
			value := compactText(anchor.Text)
			index := strings.Index(value, label)
			if index < 0 {
				continue
			}
			value = value[index+len(label):]
			// A single OCR box may contain several labels, e.g. 性别男 民族汉.
			value = beforeLabel(value)
			height := anchor.Bottom - anchor.Top
			if height < 1 {
				height = 1
			}
			if value == "" {
				nearest := float32(1e9)
				for _, candidate := range lines {
					center := (candidate.Top + candidate.Bottom - anchor.Top - anchor.Bottom) / 2
					if center < -height*.7 || center > height*.7 || candidate.Left < anchor.Right-height*.2 {
						continue
					}
					candidateText := compactText(candidate.Text)
					if candidateText == "" || containsLabel(candidateText) {
						continue
					}
					distance := candidate.Left - anchor.Right
					if distance < nearest {
						nearest = distance
						value = candidateText
					}
				}
			}
			if label == "住址" {
				// Continue below the address label until the next labelled field.
				for _, candidate := range lines {
					if candidate.Top <= anchor.Top+height*.7 || candidate.Top > anchor.Bottom+height*3.5 {
						continue
					}
					candidateText := compactText(candidate.Text)
					if containsLabel(candidateText) || idNumberPattern.MatchString(candidateText) {
						break
					}
					if candidate.Left >= anchor.Left-height && candidate.Left <= anchor.Right+height*4 {
						value += candidateText
					}
				}
			}
			if value != "" {
				doc.Fields[fieldKey(label)] = value
				break
			}
		}
	}
	if !back {
		if number := idNumberPattern.FindString(text); number != "" {
			doc.Fields["id_number"] = strings.ToUpper(number)
		}
		if birth := birthPattern.FindString(doc.Fields["birth_date"]); birth != "" {
			doc.Fields["birth_date"] = birth
		}
	}
	for _, label := range labels {
		key := fieldKey(label)
		if doc.Fields[key] == "" {
			doc.Missing = append(doc.Missing, key)
		}
	}
	doc.NeedsReview = len(doc.Missing) > 0
	if !back && !idNumberPattern.MatchString(doc.Fields["id_number"]) {
		doc.NeedsReview = true
	}
	return doc
}
func beforeLabel(s string) string {
	for _, label := range idLabels {
		if i := strings.Index(s, label); i >= 0 {
			s = s[:i]
		}
	}
	return s
}
func containsLabel(s string) bool {
	for _, label := range idLabels {
		if strings.Contains(s, label) {
			return true
		}
	}
	return false
}
func fieldKey(label string) string {
	return map[string]string{"姓名": "name", "性别": "sex", "民族": "ethnicity", "出生": "birth_date", "住址": "address", "公民身份号码": "id_number", "签发机关": "issuing_authority", "有效期限": "validity_period"}[label]
}
