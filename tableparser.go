package rstgo

// Port of docutils.parsers.rst.tableparser.

import (
	"fmt"
	"sort"
	"strings"
)

type tableMarkupError struct {
	msg    string
	offset int
}

type tableCell struct {
	morerows, morecols, offset int
	block                      *stringList
}

type tableData struct {
	colwidths []int
	headrows  [][]*tableCell
	bodyrows  [][]*tableCell
}

var (
	gridHeadBodySepPat   = pyCompile(`\+=[=+]+=\+ *$`)
	simpleHeadBodySepPat = pyCompile(`=[ =]*$`)
	simpleSpanPat        = pyCompile(`-[ -]*$`)
)

// findHeadBodySep mirrors TableParser.find_head_body_sep; returns -1 if none.
func findHeadBodySep(block *stringList, pat *pyRegexp) (int, *tableMarkupError) {
	sep := -1
	i := 0
	for i = 0; i < len(block.data); i++ {
		line := block.data[i]
		if pat.Match(line) != nil {
			if sep > 0 {
				return 0, &tableMarkupError{fmt.Sprintf("Multiple head/body row separators "+
					"(table lines %d and %d); only one allowed.", sep+1, i+1), i}
			}
			sep = i
			block.data[i] = strings.ReplaceAll(line, "=", "-")
		}
	}
	i = len(block.data) - 1
	if sep == 0 || sep == len(block.data)-1 {
		return 0, &tableMarkupError{"The head/body row separator may not be " +
			"the first or last line of the table.", i}
	}
	return sep, nil
}

type gridParser struct {
	block         *stringList
	bottom, right int
	headBodySep   int
	done          []int
	cells         []gridCell
	rowseps       map[int]bool
	colseps       map[int]bool
	stripped      [][]rune
}

type gridCell struct {
	top, left, bottom, right int
	block                    *stringList
}

func parseGridTable(blk *stringList) (*tableData, *tableMarkupError) {
	p := &gridParser{}
	p.block = blk.slice(0, len(blk.data))
	p.block.disconnect()
	p.bottom = len(blk.data) - 1
	p.right = runeLen(blk.data[0]) - 1
	p.done = make([]int, runeLen(blk.data[0]))
	for i := range p.done {
		p.done[i] = -1
	}
	p.rowseps = map[int]bool{0: true}
	p.colseps = map[int]bool{0: true}
	sep, err := findHeadBodySep(p.block, gridHeadBodySepPat)
	if err != nil {
		return nil, err
	}
	p.headBodySep = sep
	if err := p.parseTable(); err != nil {
		return nil, err
	}
	return p.structureFromCells(), nil
}

func (p *gridParser) at(row, col int) rune {
	if row < 0 || row >= len(p.stripped) || col < 0 || col >= len(p.stripped[row]) {
		return -1
	}
	return p.stripped[row][col]
}

func (p *gridParser) parseTable() *tableMarkupError {
	p.stripped = make([][]rune, len(p.block.data))
	for i, line := range p.block.data {
		p.stripped[i] = []rune(stripCombiningChars(line))
	}
	corners := [][2]int{{0, 0}}
	for len(corners) > 0 {
		top, left := corners[0][0], corners[0][1]
		corners = corners[1:]
		if top == p.bottom || left == p.right || top <= p.done[left] {
			continue
		}
		bottom, right, rowseps, colseps, ok := p.scanCell(top, left)
		if !ok {
			continue
		}
		for k := range rowseps {
			p.rowseps[k] = true
		}
		for k := range colseps {
			p.colseps[k] = true
		}
		p.markDone(top, left, bottom, right)
		cellblock := p.block.get2DBlock(top+1, left+1, bottom, right, true)
		cellblock.disconnect()
		cellblock.replaceAll(doubleWidthPadChar, "")
		p.cells = append(p.cells, gridCell{top, left, bottom, right, cellblock})
		corners = append(corners, [2]int{top, right}, [2]int{bottom, left})
		sort.Slice(corners, func(i, j int) bool {
			if corners[i][0] != corners[j][0] {
				return corners[i][0] < corners[j][0]
			}
			return corners[i][1] < corners[j][1]
		})
	}
	if !p.checkParseComplete() {
		return &tableMarkupError{"Malformed table; parse incomplete.", 0}
	}
	return nil
}

func (p *gridParser) markDone(top, left, bottom, right int) {
	after := bottom - 1
	for col := left; col < right; col++ {
		p.done[col] = after
	}
}

func (p *gridParser) checkParseComplete() bool {
	last := p.bottom - 1
	for col := 0; col < p.right; col++ {
		if p.done[col] != last {
			return false
		}
	}
	return true
}

func (p *gridParser) scanCell(top, left int) (int, int, map[int]bool, map[int]bool, bool) {
	return p.scanRight(top, left)
}

func (p *gridParser) scanRight(top, left int) (int, int, map[int]bool, map[int]bool, bool) {
	colseps := map[int]bool{}
	for i := left + 1; i <= p.right; i++ {
		c := p.at(top, i)
		if c == '+' {
			colseps[i] = true
			bottom, rowseps, newcolseps, ok := p.scanDown(top, left, i)
			if ok {
				for k := range newcolseps {
					colseps[k] = true
				}
				return bottom, i, rowseps, colseps, true
			}
		} else if c != '-' {
			return 0, 0, nil, nil, false
		}
	}
	return 0, 0, nil, nil, false
}

func (p *gridParser) scanDown(top, left, right int) (int, map[int]bool, map[int]bool, bool) {
	rowseps := map[int]bool{}
	for i := top + 1; i <= p.bottom; i++ {
		c := p.at(i, right)
		if c == '+' {
			rowseps[i] = true
			newrowseps, colseps, ok := p.scanLeft(top, left, i, right)
			if ok {
				for k := range newrowseps {
					rowseps[k] = true
				}
				return i, rowseps, colseps, true
			}
		} else if c != '|' {
			return 0, nil, nil, false
		}
	}
	return 0, nil, nil, false
}

func (p *gridParser) scanLeft(top, left, bottom, right int) (map[int]bool, map[int]bool, bool) {
	colseps := map[int]bool{}
	for i := right - 1; i > left; i-- {
		c := p.at(bottom, i)
		if c == '+' {
			colseps[i] = true
		} else if c != '-' {
			return nil, nil, false
		}
	}
	if p.at(bottom, left) != '+' {
		return nil, nil, false
	}
	rowseps, ok := p.scanUp(top, left, bottom, right)
	if ok {
		return rowseps, colseps, true
	}
	return nil, nil, false
}

func (p *gridParser) scanUp(top, left, bottom, right int) (map[int]bool, bool) {
	rowseps := map[int]bool{}
	for i := bottom - 1; i > top; i-- {
		c := p.at(i, left)
		if c == '+' {
			rowseps[i] = true
		} else if c != '|' {
			return nil, false
		}
	}
	return rowseps, true
}

func sortedKeys(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func (p *gridParser) structureFromCells() *tableData {
	rowseps := sortedKeys(p.rowseps)
	rowindex := map[int]int{}
	for i, r := range rowseps {
		rowindex[r] = i
	}
	colseps := sortedKeys(p.colseps)
	colindex := map[int]int{}
	for i, c := range colseps {
		colindex[c] = i
	}
	td := &tableData{}
	for i := 1; i < len(colseps); i++ {
		td.colwidths = append(td.colwidths, colseps[i]-colseps[i-1]-1)
	}
	rows := make([][]*tableCell, len(rowseps)-1)
	for i := range rows {
		rows[i] = make([]*tableCell, len(colseps)-1)
	}
	for _, c := range p.cells {
		rownum := rowindex[c.top]
		colnum := colindex[c.left]
		morerows := rowindex[c.bottom] - rownum - 1
		morecols := colindex[c.right] - colnum - 1
		rows[rownum][colnum] = &tableCell{morerows, morecols, c.top + 1, c.block}
	}
	if p.headBodySep > 0 {
		numheadrows := rowindex[p.headBodySep]
		td.headrows = rows[:numheadrows]
		td.bodyrows = rows[numheadrows:]
	} else {
		td.bodyrows = rows
	}
	return td
}

// ---------------------------------------------------------------- simple tables

type simpleParser struct {
	block       *stringList
	headBodySep int
	columns     [][2]int
	borderEnd   int
	table       [][]*tableCell
}

func runeFind(rs []rune, c rune, from int) int {
	if from < 0 {
		from = 0
	}
	for i := from; i < len(rs); i++ {
		if rs[i] == c {
			return i
		}
	}
	return -1
}

func runeSlice(rs []rune, a, b int) string {
	if a < 0 {
		a = 0
	}
	if b > len(rs) {
		b = len(rs)
	}
	if a >= b {
		return ""
	}
	return string(rs[a:b])
}

func parseSimpleTable(blk *stringList) (*tableData, *tableMarkupError) {
	p := &simpleParser{}
	p.block = blk.slice(0, len(blk.data))
	p.block.disconnect()
	n := len(p.block.data)
	p.block.data[0] = strings.ReplaceAll(p.block.data[0], "=", "-")
	p.block.data[n-1] = strings.ReplaceAll(p.block.data[n-1], "=", "-")
	sep, err := findHeadBodySep(p.block, simpleHeadBodySepPat)
	if err != nil {
		return nil, err
	}
	p.headBodySep = sep
	if err := p.parseTable(); err != nil {
		return nil, err
	}
	td := &tableData{}
	for _, c := range p.columns {
		td.colwidths = append(td.colwidths, c[1]-c[0])
	}
	firstBodyRow := 0
	if p.headBodySep > 0 {
		for i := range p.table {
			if p.table[i][0].offset > p.headBodySep {
				firstBodyRow = i
				break
			}
		}
	}
	td.headrows = p.table[:firstBodyRow]
	td.bodyrows = p.table[firstBodyRow:]
	return td, nil
}

func (p *simpleParser) parseTable() *tableMarkupError {
	cols, err := p.parseColumns(p.block.data[0], 0)
	if err != nil {
		return err
	}
	p.columns = cols
	p.borderEnd = p.columns[len(p.columns)-1][1]
	firststart, firstend := p.columns[0][0], p.columns[0][1]
	offset := 1
	start := 1
	textFound := false
	for offset < len(p.block.data) {
		line := p.block.data[offset]
		if simpleSpanPat.Match(line) != nil {
			if err := p.parseRow(p.block.slice(start, offset), start, pyRstrip(line), offset, true); err != nil {
				return err
			}
			start = offset + 1
			textFound = false
		} else if !isBlank(runeSlice([]rune(line), firststart, firstend)) {
			if textFound && offset != start {
				if err := p.parseRow(p.block.slice(start, offset), start, "", 0, false); err != nil {
					return err
				}
			}
			start = offset
			textFound = true
		} else if !textFound {
			start = offset + 1
		}
		offset++
	}
	return nil
}

func (p *simpleParser) parseColumns(line string, offset int) ([][2]int, *tableMarkupError) {
	rs := []rune(line)
	var cols [][2]int
	end := 0
	for {
		begin := runeFind(rs, '-', end)
		if begin < 0 {
			break
		}
		end = runeFind(rs, ' ', begin)
		if end < 0 {
			end = len(rs)
		}
		cols = append(cols, [2]int{begin, end})
	}
	if len(p.columns) > 0 {
		if len(cols) == 0 || cols[len(cols)-1][1] != p.borderEnd {
			return nil, &tableMarkupError{fmt.Sprintf("Column span incomplete in table line %d.", offset+1), offset}
		}
		cols[len(cols)-1] = [2]int{cols[len(cols)-1][0], p.columns[len(p.columns)-1][1]}
	}
	return cols, nil
}

func (p *simpleParser) initRow(colspec [][2]int, offset int) ([]*tableCell, *tableMarkupError) {
	i := 0
	var cells []*tableCell
	for _, c := range colspec {
		start, end := c[0], c[1]
		morecols := 0
		bad := false
		if i >= len(p.columns) || start != p.columns[i][0] {
			bad = true
		} else {
			for end != p.columns[i][1] {
				i++
				morecols++
				if i >= len(p.columns) {
					bad = true
					break
				}
			}
		}
		if bad {
			return nil, &tableMarkupError{fmt.Sprintf("Column span alignment problem in table line %d.", offset+2), offset + 1}
		}
		cells = append(cells, &tableCell{0, morecols, offset, nil})
		i++
	}
	return cells, nil
}

func (p *simpleParser) parseRow(lines *stringList, start int, spanline string, spanOffset int, hasSpan bool) *tableMarkupError {
	if len(lines.data) == 0 && !hasSpan {
		return nil
	}
	var columns [][2]int
	if hasSpan {
		var err *tableMarkupError
		columns, err = p.parseColumns(spanline, spanOffset)
		if err != nil {
			return err
		}
	} else {
		columns = append([][2]int{}, p.columns...)
	}
	if err := p.checkColumns(lines, start, &columns); err != nil {
		return err
	}
	row, err := p.initRow(columns, start)
	if err != nil {
		return err
	}
	for i := range columns {
		cstart, cend := columns[i][0], columns[i][1]
		cellblock := lines.get2DBlock(0, cstart, len(lines.data), cend, true)
		cellblock.disconnect()
		cellblock.replaceAll(doubleWidthPadChar, "")
		row[i].block = cellblock
	}
	p.table = append(p.table, row)
	return nil
}

func (p *simpleParser) checkColumns(lines *stringList, firstLine int, columnsp *[][2]int) *tableMarkupError {
	columns := *columnsp
	const maxsize = int(^uint(0) >> 1)
	columns = append(columns, [2]int{maxsize, -1})
	lastcol := len(columns) - 2
	stripped := make([][]rune, len(lines.data))
	for i, l := range lines.data {
		stripped[i] = []rune(stripCombiningChars(l))
	}
	for i := 0; i < len(columns)-1; i++ {
		start, end := columns[i][0], columns[i][1]
		nextstart := columns[i+1][0]
		offset := 0
		for _, line := range stripped {
			if i == lastcol && !isBlank(runeSlice(line, end, len(line))) {
				text := pyRstrip(runeSlice(line, start, len(line)))
				newEnd := start + runeLen(text)
				mainStart, mainEnd := p.columns[len(p.columns)-1][0], p.columns[len(p.columns)-1][1]
				e := mainEnd
				if newEnd > e {
					e = newEnd
				}
				columns[i] = [2]int{start, e}
				if newEnd > mainEnd {
					p.columns[len(p.columns)-1] = [2]int{mainStart, newEnd}
				}
			} else if !isBlank(runeSlice(line, end, nextstart)) {
				return &tableMarkupError{fmt.Sprintf("Text in column margin in table line %d.", firstLine+offset+1), firstLine + offset}
			}
			offset++
		}
	}
	*columnsp = columns[:len(columns)-1]
	return nil
}
