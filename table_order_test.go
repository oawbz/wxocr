package ocr

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"reflect"
	"testing"
)

func TestRuledTableVisualOrder(t *testing.T) {
	raw, err := os.ReadFile("testdata/layout/public_046.input.json")
	if err != nil {
		t.Fatal(err)
	}
	var result Result
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("testdata/layout/public_046.truth.json")
	if err != nil {
		t.Fatal(err)
	}
	var truth struct {
		Order []int `json:"input_line_indices_in_expected_order"`
	}
	if err = json.Unmarshal(raw, &truth); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("testdata/layout/public_046.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := DecodeImage(f)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]Line(nil), result.Lines...)
	orderRuledTables(&result, im, 0)
	for i, index := range truth.Order {
		if result.Lines[i] != before[index] {
			t.Errorf("line %d: got %q; want input %d %q", i, result.Lines[i].Text, index, before[index].Text)
		}
	}
	next := 0
	for _, p := range result.Paragraphs {
		for _, i := range p.LineIndices {
			if i != next {
				t.Fatal("paragraph indices are not a complete ordered partition")
			}
			next++
		}
	}
	if next != len(before) {
		t.Fatal("omitted lines")
	}
}

func TestTableRequiresBordersAndSeveralRows(t *testing.T) {
	im := image.NewGray(image.Rect(7, 11, 307, 251))
	for i := range im.Pix {
		im.Pix[i] = 255
	}
	horizontal := func(y int) {
		for x := 27; x <= 287; x++ {
			im.SetGray(x, y, color.Gray{Y: 0})
		}
	}
	for _, y := range []int{31, 61, 91, 121, 151, 181, 211} {
		horizontal(y)
	}
	ink, w, h := uprightInk(im, 0)
	if len(ruledTables(ink, w, h)) != 0 {
		t.Fatal("underlines without side borders became a table")
	}
	for y := 31; y <= 211; y++ {
		for _, x := range []int{27, 157, 287} {
			im.SetGray(x, y, color.Gray{Y: 0})
		}
	}
	for direction := 0; direction < 4; direction++ {
		width, height := 300, 240
		if direction%2 != 0 {
			width, height = height, width
		}
		rotated := image.NewGray(image.Rect(0, 0, width, height))
		for y := 0; y < 240; y++ {
			for x := 0; x < 300; x++ {
				xx, yy := x, y
				switch direction {
				case 1:
					xx, yy = 239-y, x
				case 2:
					xx, yy = 299-x, 239-y
				case 3:
					xx, yy = y, 299-x
				}
				rotated.SetGray(xx, yy, im.GrayAt(x+7, y+11))
			}
		}
		ink, w, h = uprightInk(rotated, direction)
		if tables := ruledTables(ink, w, h); len(tables) != 1 {
			t.Fatalf("direction %d: %+v", direction, tables)
		}
	}

	result := Result{Width: 300, Height: 240, Lines: []Line{{Left: 180, Top: 55, Right: 230, Bottom: 67, Text: "value"}, {Left: 30, Top: 56, Right: 90, Bottom: 68, Text: "label"}, {Left: 30, Top: 90, Right: 90, Bottom: 100, Text: "next"}, {Left: 30, Top: 120, Right: 90, Bottom: 130, Text: "third"}, {Left: 30, Top: 150, Right: 90, Bottom: 160, Text: "fourth"}, {Left: 30, Top: 180, Right: 90, Bottom: 190, Text: "fifth"}}}
	result.Lines = append(result.Lines, Line{Left: 180, Top: 90, Right: 230, Bottom: 100, Text: "second value"}, Line{Left: 180, Top: 120, Right: 230, Bottom: 130, Text: "third value"}, Line{Left: 180, Top: 150, Right: 230, Bottom: 160, Text: "fourth value"})
	orderRuledTables(&result, im, 0)
	if result.Lines[0].Text != "label" || result.Lines[1].Text != "value" {
		t.Fatal(result.Lines)
	}
	plain := image.NewGray(im.Bounds())
	for i := range plain.Pix {
		plain.Pix[i] = 255
	}
	before := append([]Line(nil), result.Lines...)
	orderRuledTables(&result, plain, 0)
	if !reflect.DeepEqual(before, result.Lines) {
		t.Fatal("plain page order changed")
	}
}

func TestVisualNonTablePagesKeepOrder(t *testing.T) {
	for _, name := range []string{"public_014", "public_083"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/layout/" + name + ".input.json")
			if err != nil {
				t.Fatal(err)
			}
			var result Result
			if err = json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open("testdata/layout/" + name + ".image")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			im, err := DecodeImage(f)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]Line(nil), result.Lines...)
			orderRuledTables(&result, im, 0)
			if !reflect.DeepEqual(before, result.Lines) {
				t.Fatal("poster or shop window misclassified as a table")
			}
		})
	}
}

func TestDarkTerminalKeepsReadingOrder(t *testing.T) {
	raw, err := os.ReadFile("testdata/native-cases/native_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var refs map[string]Result
	if err = json.Unmarshal(raw, &refs); err != nil {
		t.Fatal(err)
	}
	result := refs["terminal"]
	if len(result.Lines) == 0 {
		t.Fatal("missing terminal fixture")
	}
	f, err := os.Open("testdata/native-cases/terminal.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := DecodeImage(f)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]Line(nil), result.Lines...)
	orderRuledTables(&result, im, 0)
	if !reflect.DeepEqual(before, result.Lines) {
		t.Fatal("dark terminal treated as a ruled table")
	}
}

func TestSecondFormVisualConstraints(t *testing.T) {
	raw, err := os.ReadFile("testdata/layout/public_047.input.json")
	if err != nil {
		t.Fatal(err)
	}
	var result Result
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	before := append([]Line(nil), result.Lines...)
	raw, err = os.ReadFile("testdata/layout/public_047.truth.json")
	if err != nil {
		t.Fatal(err)
	}
	var truth struct {
		Pairs [][2]int `json:"before_pairs"`
	}
	if err = json.Unmarshal(raw, &truth); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("testdata/layout/public_047.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := DecodeImage(f)
	if err != nil {
		t.Fatal(err)
	}
	orderRuledTables(&result, im, 0)
	indices := map[Line]int{}
	for i, line := range result.Lines {
		indices[line] = i
	}
	for _, pair := range truth.Pairs {
		if indices[before[pair[0]]] >= indices[before[pair[1]]] {
			t.Errorf("%q must precede %q", before[pair[0]].Text, before[pair[1]].Text)
		}
	}
}
