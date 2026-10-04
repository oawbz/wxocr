package ocr

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"reflect"
	"testing"
)

// Replay outputs captured by directly calling the native paragraph merge
// function with identical geometry. This runs without models or native libraries.
func TestNativeMergeFixtures(t *testing.T) {
	var reference struct {
		Cases []struct {
			Name                     string
			Width, Height, Direction int
			Lines                    []struct {
				Text    string
				Polygon [][2]float64
			}
			Regions [][][2]float64
			Groups  [][]int
		}
	}
	raw, err := os.ReadFile("testdata/native-cases/native_merge_expected.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Cases) == 0 {
		t.Fatal("empty native merge reference")
	}
	for _, c := range reference.Cases {
		t.Run(c.Name, func(t *testing.T) {
			result := &Result{Width: c.Width, Height: c.Height}
			geometry := map[Line]quadrilateral{}
			for i, input := range c.Lines {
				if len(input.Polygon) != 4 {
					t.Fatal("invalid native text polygon")
				}
				var q quadrilateral
				for j, p := range input.Polygon {
					q[j] = point{p[0], p[1]}
				}
				if c.Direction%2 == 0 {
					q = quadrilateral{q[3], q[0], q[1], q[2]}
				}
				l, top, r, bottom := q.bounds()
				line := Line{Left: float32(l), Top: float32(top), Right: float32(r), Bottom: float32(bottom), Text: input.Text, DetectionScore: float32(i)}
				result.Lines = append(result.Lines, line)
				geometry[line] = q
			}
			var regions []paragraphRegion
			for _, input := range c.Regions {
				region := paragraphRegion{}
				for _, p := range input {
					region.polygon = append(region.polygon, point{p[0], p[1]})
					region.box = region.box.Union(image.Rect(int(math.Floor(p[0])), int(math.Floor(p[1])), int(math.Floor(p[0]))+1, int(math.Floor(p[1]))+1))
				}
				regions = append(regions, region)
			}
			groupParagraphs(result, regions, c.Direction, geometry)
			groups := [][]int{}
			for _, p := range result.Paragraphs {
				ids := []int{}
				for _, index := range p.LineIndices {
					ids = append(ids, int(result.Lines[index].DetectionScore))
				}
				groups = append(groups, ids)
			}
			if !reflect.DeepEqual(groups, c.Groups) {
				t.Fatalf("Go groups %v; native %v", groups, c.Groups)
			}
		})
	}
}
