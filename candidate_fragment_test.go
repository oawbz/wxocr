package ocr

import "testing"

func TestCandidateFragmentOnlySuppressesCoveredText(t *testing.T) {
	full := Line{Left: 10, Top: 10, Right: 210, Bottom: 30, Text: "07-01", Confidence: .9}
	prefix := Line{Left: 10, Top: 10, Right: 80, Bottom: 30, Text: "07-", Confidence: .99}
	suffix := Line{Left: 90, Top: 10, Right: 210, Bottom: 30, Text: "-01", Confidence: .9}
	adjacent := suffix
	adjacent.Left = 230
	adjacent.Right = 350
	different := prefix
	different.Text = "08"
	large := full
	large.Text = "07"
	outside := prefix
	outside.Left = 190
	outside.Right = 240
	vertical := Line{Left: 1250, Top: 1610, Right: 1290, Bottom: 1750, Text: "07-01"}
	padded := Line{Left: 1240, Top: 1615, Right: 1304, Bottom: 1670, Text: "07"}
	chinese := Line{Left: 10, Top: 50, Right: 210, Bottom: 80, Text: "技术岗位", Confidence: .9}
	glyph := Line{Left: 10, Top: 50, Right: 60, Bottom: 80, Text: "技", Confidence: .99}
	for _, tc := range []struct {
		name string
		a, b Line
		want bool
	}{
		{"prefix", prefix, full, true}, {"suffix", suffix, full, true}, {"padded coarse box", padded, vertical, true}, {"Chinese glyph", glyph, chinese, true},
		{"neighboring cell", adjacent, full, false}, {"different transcription", different, full, false}, {"coarse ambiguous extent", large, full, false}, {"center outside", outside, full, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := candidateFragment(tc.a, tc.b); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	r := Result{Lines: []Line{full, chinese}, Candidates: []Candidate{{Line: prefix}, {Line: suffix}, {Line: glyph}, {Line: adjacent}, {Line: different}}}
	mergePassResults(&r)
	if len(r.Lines) != 2 || len(r.Candidates) != 2 {
		t.Fatalf("body or independent candidates lost: %+v", r)
	}
	for _, c := range r.Candidates {
		if c.Line != adjacent && c.Line != different {
			t.Fatalf("retained fragment: %+v", c)
		}
	}
}

// The decision must depend on relative geometry and text containment, independent
// of content vocabulary, image position, resolution, and page orientation.
func TestCandidateFragmentGeneralGeometryInvariance(t *testing.T) {
	rotate := func(l Line) Line {
		l.Left, l.Top, l.Right, l.Bottom = -l.Bottom, l.Left, -l.Top, l.Right
		return l
	}
	transform := func(l Line, scale float32) Line {
		l.Left, l.Right = l.Left*scale-75, l.Right*scale-75
		l.Top, l.Bottom = l.Top*scale+143, l.Bottom*scale+143
		return l
	}
	for _, texts := range [][2]string{{"ABC", "ABC-1234"}, {"研发", "研发工程师"}, {"234", "123456789"}, {"école", "école测试"}, {"研 发", "研发工程师"}} {
		full := Line{Left: 0, Top: 0, Right: 200, Bottom: 20, Text: texts[1]}
		fragment := Line{Left: 10, Top: -5, Right: 60, Bottom: 25, Text: texts[0]}
		neighbor := fragment
		neighbor.Top += 40
		neighbor.Bottom += 40
		disagree := fragment
		disagree.Text = "unrelated"
		for rotation := 0; rotation < 4; rotation++ {
			for _, scale := range []float32{.5, 1, 2, 4} {
				a, b := transform(fragment, scale), transform(full, scale)
				if !candidateFragment(a, b) {
					t.Fatalf("lost covered fragment %q rotation%d scale%v", texts[0], rotation, scale)
				}
				if candidateFragment(transform(neighbor, scale), b) || candidateFragment(transform(disagree, scale), b) {
					t.Fatalf("lost independent text at rotation%d scale%v", rotation, scale)
				}
			}
			full, fragment, neighbor, disagree = rotate(full), rotate(fragment), rotate(neighbor), rotate(disagree)
		}
	}
}
