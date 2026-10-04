package ocr

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// Ruled tables have stronger layout evidence than a paragraph probability map.
// Work in upright coordinates and require several rows plus both side borders.
type ruledTable struct {
	left, right int
	rows        []int
}
type ruleRun struct{ y, left, right int }

func darkRulePixel(r, g, b uint8) bool {
	return int(r)+int(g)+int(b) < 390 && int(max(r, g, b))-int(min(r, g, b)) < 75
}

func uprightInk(src image.Image, direction int) ([]bool, int, int) {
	bounds := src.Bounds()
	ow, oh := bounds.Dx(), bounds.Dy()
	uw, uh := ow, oh
	if direction%2 != 0 {
		uw, uh = oh, ow
	}
	scale := math.Min(1, 1600/float64(max(uw, uh)))
	w, h := max(1, int(float64(uw)*scale)), max(1, int(float64(uh)*scale))
	ink := make([]bool, w*h)
	// Pool dark source pixels rather than sampling: a one-pixel ruling must survive.
	dark := func(x, y int) bool {
		rgb := nativeRGB(src.At(bounds.Min.X+x, bounds.Min.Y+y))
		return darkRulePixel(rgb[0], rgb[1], rgb[2])
	}
	switch im := src.(type) {
	case *image.NRGBA:
		dark = func(x, y int) bool {
			i := (bounds.Min.Y+y-im.Rect.Min.Y)*im.Stride + (bounds.Min.X+x-im.Rect.Min.X)*4
			return darkRulePixel(im.Pix[i], im.Pix[i+1], im.Pix[i+2])
		}
	case *image.Gray:
		dark = func(x, y int) bool {
			i := (bounds.Min.Y+y-im.Rect.Min.Y)*im.Stride + bounds.Min.X + x - im.Rect.Min.X
			return im.Pix[i] < 130
		}
	case *image.RGBA:
		dark = func(x, y int) bool {
			i := (bounds.Min.Y+y-im.Rect.Min.Y)*im.Stride + (bounds.Min.X+x-im.Rect.Min.X)*4
			if im.Pix[i+3] == 255 {
				return darkRulePixel(im.Pix[i], im.Pix[i+1], im.Pix[i+2])
			}
			c := color.NRGBAModel.Convert(im.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			return darkRulePixel(c.R, c.G, c.B)
		}
	}
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			if !dark(x, y) {
				continue
			}
			u, v := x, y
			switch direction {
			case 1:
				u, v = y, ow-1-x
			case 2:
				u, v = ow-1-x, oh-1-y
			case 3:
				u, v = oh-1-y, x
			}
			xx, yy := min(w-1, u*w/uw), min(h-1, v*h/uh)
			ink[yy*w+xx] = true
		}
	}
	return ink, w, h
}

func ruledTables(ink []bool, w, h int) []ruledTable {
	if w < 20 || h < 20 {
		return nil
	}
	// Tolerate slight skew by a narrow vertical dilation. Keep the original ink
	// for side-border and cell checks; dilation alone is not table evidence.
	radius := max(1, w/350)
	wide := make([]bool, len(ink))
	for x := 0; x < w; x++ {
		count := 0
		for y := 0; y < min(h, radius+1); y++ {
			if ink[y*w+x] {
				count++
			}
		}
		for y := 0; y < h; y++ {
			wide[y*w+x] = count > 0
			if y-radius >= 0 && ink[(y-radius)*w+x] {
				count--
			}
			if y+radius+1 < h && ink[(y+radius+1)*w+x] {
				count++
			}
		}
	}
	var runs []ruleRun
	for y := 0; y < h; y++ {
		start, last, gaps := -1, 0, 0
		for x := 0; x <= w; x++ {
			black := x < w && wide[y*w+x]
			if black {
				if start < 0 {
					start = x
				}
				last = x
				gaps = 0
			} else if start >= 0 {
				gaps++
				if gaps > 2 || x == w {
					if last-start+1 >= max(20, w*3/10) {
						runs = append(runs, ruleRun{y, start, last})
					}
					start = -1
					gaps = 0
				}
			}
		}
	}
	var clusters [][]ruleRun
	tolerance := max(3, w/50)
	for _, r := range runs {
		found := false
		for i, c := range clusters {
			if absInt(r.left-c[0].left) <= tolerance && absInt(r.right-c[0].right) <= tolerance {
				clusters[i] = append(c, r)
				found = true
				break
			}
		}
		if !found {
			clusters = append(clusters, []ruleRun{r})
		}
	}
	var tables []ruledTable
	for _, c := range clusters {
		var rows []int
		for i := 0; i < len(c); {
			j := i + 1
			for j < len(c) && c[j].y <= c[j-1].y+1 {
				j++
			}
			rows = append(rows, (c[i].y+c[j-1].y)/2)
			i = j
		}
		if len(rows) < 6 || rows[len(rows)-1]-rows[0] < h/10 {
			continue
		}
		lefts, rights := make([]int, len(c)), make([]int, len(c))
		for i, r := range c {
			lefts[i], rights[i] = r.left, r.right
		}
		sort.Ints(lefts)
		sort.Ints(rights)
		left, right := lefts[len(c)/2], rights[len(c)/2]
		black := 0
		top, bottom := rows[0], rows[len(rows)-1]
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				if ink[y*w+x] {
					black++
				}
			}
		}
		// Dark screens/backgrounds produce long negative-space runs, not rules.
		if float64(black)/float64((right-left+1)*(bottom-top+1)) > .3 {
			continue
		}
		if !verticalRule(ink, w, h, left, rows[0], rows[len(rows)-1], radius+2, .65) || !verticalRule(ink, w, h, right, rows[0], rows[len(rows)-1], radius+2, .65) {
			continue
		}
		// A merged cell can interrupt a row rule before the opposite border.
		// Keep long partial rules attached to either outer side of this grid.
		partial := make([]bool, h)
		for _, run := range runs {
			if run.y < rows[0] || run.y > rows[len(rows)-1] {
				continue
			}
			attached := absInt(run.left-left) <= tolerance || absInt(run.right-right) <= tolerance
			if attached && run.left >= left-tolerance && run.right <= right+tolerance && run.right-run.left >= (right-left)*3/5 {
				partial[run.y] = true
			}
		}
		var complete []int
		for y := rows[0]; y <= rows[len(rows)-1]; {
			if !partial[y] {
				y++
				continue
			}
			end := y + 1
			for end < h && partial[end] {
				end++
			}
			complete = append(complete, (y+end-1)/2)
			y = end
		}
		if len(complete) >= len(rows) {
			rows = complete
		}
		tables = append(tables, ruledTable{left, right, rows})
	}
	sort.Slice(tables, func(i, j int) bool {
		return (tables[i].right-tables[i].left)*(tables[i].rows[len(tables[i].rows)-1]-tables[i].rows[0]) > (tables[j].right-tables[j].left)*(tables[j].rows[len(tables[j].rows)-1]-tables[j].rows[0])
	})
	return tables
}
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func verticalRule(ink []bool, w, h, x, top, bottom, radius int, coverage float64) bool {
	top, bottom = max(0, top), min(h-1, bottom)
	if bottom <= top {
		return false
	}
	hits := 0
	for y := top; y <= bottom; y++ {
		for xx := max(0, x-radius); xx <= min(w-1, x+radius); xx++ {
			if ink[y*w+xx] {
				hits++
				break
			}
		}
	}
	return float64(hits)/float64(bottom-top+1) >= coverage
}
func tableColumns(ink []bool, w, h int, t ruledTable, row int) []int {
	top, bottom := t.rows[row]+2, t.rows[row+1]-2
	columns := []int{t.left}
	active := -1
	for x := t.left + 3; x < t.right-3; x++ {
		hit := verticalRule(ink, w, h, x, top, bottom, 1, .8)
		if hit && active < 0 {
			active = x
		}
		if !hit && active >= 0 {
			columns = append(columns, (active+x-1)/2)
			active = -1
		}
	}
	if active >= 0 {
		columns = append(columns, (active+t.right-4)/2)
	}
	return append(columns, t.right)
}

func orderRuledTables(result *Result, src image.Image, direction int) {
	if len(result.Lines) < 6 {
		return
	}
	ink, w, h := uprightInk(src, direction)
	tables := ruledTables(ink, w, h)
	if len(tables) == 0 {
		return
	}
	uw, uh := result.Width, result.Height
	if direction%2 != 0 {
		uw, uh = uh, uw
	}
	old := append([]Line(nil), result.Lines...)
	order := make([]int, len(old))
	owner := make([]int, len(old))
	for i := range old {
		order[i] = i
		owner[i] = len(result.Paragraphs) + i
	}
	for i, p := range result.Paragraphs {
		for _, index := range p.LineIndices {
			owner[index] = i
		}
	}
	nextOwner := len(result.Paragraphs) + len(old)
	claimed := make([]bool, len(old))
	for _, table := range tables {
		type cellKey struct{ row, col int }
		cells := map[cellKey][]int{}
		var slots []int
		columns := make([][]int, len(table.rows)-1)
		for row := range columns {
			columns[row] = tableColumns(ink, w, h, table, row)
		}
		for i, b := range old {
			if claimed[i] {
				continue
			}
			r := uprightRect(image.Rect(int(math.Floor(float64(b.Left))), int(math.Floor(float64(b.Top))), int(math.Ceil(float64(b.Right))), int(math.Ceil(float64(b.Bottom)))), direction, result.Width, result.Height)
			x, y := float64(r.Min.X+r.Max.X)*.5*float64(w)/float64(uw), float64(r.Min.Y+r.Max.Y)*.5*float64(h)/float64(uh)
			if x <= float64(table.left) || x >= float64(table.right) || y <= float64(table.rows[0]) || y >= float64(table.rows[len(table.rows)-1]) {
				continue
			}
			// A line outside the table must not be pulled in by its center alone.
			left, right := float64(r.Min.X)*float64(w)/float64(uw), float64(r.Max.X)*float64(w)/float64(uw)
			if math.Max(0, math.Min(right, float64(table.right))-math.Max(left, float64(table.left))) < .8*(right-left) {
				continue
			}
			row := sort.Search(len(table.rows), func(j int) bool { return float64(table.rows[j]) > y }) - 1
			col := sort.Search(len(columns[row]), func(j int) bool { return float64(columns[row][j]) > x }) - 1
			cells[cellKey{row, col}] = append(cells[cellKey{row, col}], i)
			slots = append(slots, i)
		}
		occupiedRows := map[int]map[int]bool{}
		for key := range cells {
			if occupiedRows[key.row] == nil {
				occupiedRows[key.row] = map[int]bool{}
			}
			occupiedRows[key.row][key.col] = true
		}
		multiCellRows := 0
		for _, columns := range occupiedRows {
			if len(columns) > 1 {
				multiCellRows++
			}
		}
		// Window frames, shutters and underlined text are not enough: demand
		// text distributed across several rows and multiple populated cells.
		if len(slots) < 6 || len(occupiedRows) < 3 || multiCellRows < 2 {
			continue
		}
		for _, i := range slots {
			claimed[i] = true
		}
		keys := make([]cellKey, 0, len(cells))
		for key := range cells {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].row != keys[j].row {
				return keys[i].row < keys[j].row
			}
			return keys[i].col < keys[j].col
		})
		var sorted []int
		for _, key := range keys {
			indices := cells[key]
			sort.SliceStable(indices, func(i, j int) bool { return lineLess(old[indices[i]], old[indices[j]], direction) })
			for _, i := range indices {
				owner[i] = nextOwner
			}
			nextOwner++
			sorted = append(sorted, indices...)
		}
		for i, slot := range slots {
			order[slot] = sorted[i]
		}
		// Nearby text above the grid is a header, not another table column.
		// Limit this to the table's horizontal span so neighboring articles
		// retain their existing column order.
		var headerSlots []int
		for i, b := range old {
			if claimed[i] {
				continue
			}
			r := uprightRect(image.Rect(int(b.Left), int(b.Top), int(math.Ceil(float64(b.Right))), int(math.Ceil(float64(b.Bottom)))), direction, result.Width, result.Height)
			x0, x1, y1 := float64(r.Min.X)*float64(w)/float64(uw), float64(r.Max.X)*float64(w)/float64(uw), float64(r.Max.Y)*float64(h)/float64(uh)
			headerGap := math.Max(3*float64(r.Dy())*float64(h)/float64(uh), .08*float64(table.rows[len(table.rows)-1]-table.rows[0]))
			if y1 < float64(table.rows[0]) && float64(table.rows[0])-y1 <= headerGap && math.Max(0, math.Min(x1, float64(table.right))-math.Max(x0, float64(table.left))) >= .8*(x1-x0) {
				headerSlots = append(headerSlots, i)
			}
		}
		headers := append([]int(nil), headerSlots...)
		sort.SliceStable(headers, func(i, j int) bool { return lineLess(old[headers[i]], old[headers[j]], direction) })
		for i, slot := range headerSlots {
			order[slot] = headers[i]
			claimed[headers[i]] = true
		}

	}
	result.Paragraphs = nil
	current := -1
	for i, original := range order {
		b := old[original]
		result.Lines[i] = b
		if current != owner[original] {
			result.Paragraphs = append(result.Paragraphs, Paragraph{Left: b.Left, Top: b.Top, Right: b.Right, Bottom: b.Bottom})
			current = owner[original]
		}
		p := &result.Paragraphs[len(result.Paragraphs)-1]
		p.Left = min(p.Left, b.Left)
		p.Top = min(p.Top, b.Top)
		p.Right = max(p.Right, b.Right)
		p.Bottom = max(p.Bottom, b.Bottom)
		p.LineIndices = append(p.LineIndices, i)
	}
}
