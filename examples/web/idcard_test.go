package main

import (
	"testing"
	ocr "wxocr"
)

func TestIDCardTemplates(t *testing.T) {
	tests := []struct {
		name    string
		texts   []string
		side    string
		fields  map[string]string
		unknown bool
	}{
		{"front", []string{"姓名张三", "性别男民族汉", "出生1990年1月2日", "住址北京市朝阳区", "公民身份号码110101199001020011"}, "front", map[string]string{"name": "张三", "sex": "男", "ethnicity": "汉", "birth_date": "1990年1月2日", "address": "北京市朝阳区", "id_number": "110101199001020011"}, false},
		{"back", []string{"中华人民共和国居民身份证", "签发机关北京市公安局", "有效期限2020.01.01-2040.01.01"}, "back", map[string]string{"issuing_authority": "北京市公安局", "validity_period": "2020.01.01-2040.01.01"}, false},
		{"ordinary", []string{"人员名单", "姓名张三", "联系电话123456789"}, "", nil, true},
		{"personnel form", []string{"姓名张三", "性别男", "民族汉"}, "", nil, true},
		{"number", []string{"110101199001020011"}, "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ocr.Result{}
			for i, s := range tt.texts {
				r.Lines = append(r.Lines, ocr.Line{Text: s, Top: float32(i * 30), Bottom: float32(i*30 + 20), Right: 200})
			}
			d := classifyDocument(r)
			if tt.unknown {
				if d.Type != "unknown" {
					t.Fatalf("false positive: %+v", d)
				}
				return
			}
			if d.Type != "id_card" || d.Side != tt.side {
				t.Fatalf("wrong template: %+v", d)
			}
			for k, v := range tt.fields {
				if d.Fields[k] != v {
					t.Errorf("%s=%q want %q", k, d.Fields[k], v)
				}
			}
		})
	}
}
func TestIDCardSeparateBoxes(t *testing.T) {
	r := &ocr.Result{Lines: []ocr.Line{
		{Text: "姓名", Left: 10, Right: 50, Top: 10, Bottom: 30}, {Text: "张三", Left: 65, Right: 100, Top: 11, Bottom: 31},
		{Text: "性别男民族汉", Top: 40, Bottom: 60},
		{Text: "住址北京市朝阳区", Left: 10, Right: 200, Top: 70, Bottom: 90},
		{Text: "幸福路一号", Left: 65, Right: 160, Top: 100, Bottom: 120},
		{Text: "公民身份号码110101199001020011", Top: 140, Bottom: 160},
	}}
	d := classifyDocument(r)
	if d.Fields["name"] != "张三" || d.Fields["address"] != "北京市朝阳区幸福路一号" {
		t.Fatalf("incorrect fields: %+v", d)
	}
	if !d.NeedsReview {
		t.Fatal("missing birth date must require review")
	}
}
