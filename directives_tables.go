package rstgo

// Port of docutils.parsers.rst.directives.tables.

import (
	"fmt"
	"os"
	"strings"
)

var tableAlign = optChoice("left", "center", "right")

func init() {
	directiveRegistry["table"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{"class": optClass, "name": optUnchanged,
			"align": tableAlign, "width": optLengthOrPercentageOrUnitless,
			"widths": optWidths("auto", "grid")},
		hasContent: true, run: runRSTTable}
	directiveRegistry["csv-table"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{
			"header-rows": optNonnegativeInt, "stub-columns": optNonnegativeInt,
			"header": optUnchanged, "width": optLengthOrPercentageOrUnitless,
			"widths": optWidths("auto"), "file": optPath, "url": optURI,
			"encoding": optEncoding, "class": optClass, "name": optUnchanged,
			"align": tableAlign, "delim": optSingleCharOrWhitespaceOrUnicode,
			"keepspace": optFlag, "quote": optSingleCharOrUnicode,
			"escape": optSingleCharOrUnicode},
		hasContent: true, run: runCSVTable}
	directiveRegistry["list-table"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{
			"header-rows": optNonnegativeInt, "stub-columns": optNonnegativeInt,
			"width": optLengthOrPercentageOrUnitless, "widths": optWidths("auto"),
			"class": optClass, "name": optUnchanged, "align": tableAlign},
		hasContent: true, run: runListTable}
}

// systemMessagePropagation is raised (panic) with an error node.
type systemMessagePropagation struct{ node *Node }

func (d *directive) propagate(n *Node) { panic(&systemMessagePropagation{n}) }

func catchPropagation(f func()) (node *Node) {
	defer func() {
		if r := recover(); r != nil {
			if p, ok := r.(*systemMessagePropagation); ok {
				node = p.node
				return
			}
			panic(r)
		}
	}()
	f()
	return nil
}

func (d *directive) makeTitle() (*Node, []*Node) {
	if len(d.arguments) > 0 {
		titleText := d.arguments[0]
		textNodes, messages := d.state.inlineText(titleText, d.lineno)
		title := newTextElem("title", titleText, "", textNodes...)
		title.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
		return title, messages
	}
	return nil, nil
}

func (d *directive) checkTableDimensions(rowLens []int, headerRows, stubColumns int) {
	if len(rowLens) < headerRows {
		d.propagate(d.reporter().error(fmt.Sprintf(`%d header row(s) specified but only %d row(s) of data supplied ("%s" directive).`,
			headerRows, len(rowLens), d.name), d.literalBlockOfSource(), atLine(d.lineno)))
	}
	if len(rowLens) == headerRows && headerRows > 0 {
		d.propagate(d.reporter().error(fmt.Sprintf(`Insufficient data supplied (%d row(s)); no data remaining for table body, required by "%s" directive.`,
			len(rowLens), d.name), d.literalBlockOfSource(), atLine(d.lineno)))
	}
	for _, n := range rowLens {
		if n < stubColumns {
			d.propagate(d.reporter().error(fmt.Sprintf(`%d stub column(s) specified but only %d columns(s) of data supplied ("%s" directive).`,
				stubColumns, n, d.name), d.literalBlockOfSource(), atLine(d.lineno)))
		}
		if n == stubColumns && stubColumns > 0 {
			d.propagate(d.reporter().error(fmt.Sprintf(`Insufficient data supplied (%d columns(s)); no data remaining for table body, required by "%s" directive.`,
				n, d.name), d.literalBlockOfSource(), atLine(d.lineno)))
		}
	}
}

func (d *directive) setTableWidth(table *Node) {
	if w, ok := d.options["width"]; ok {
		table.setAttr("width", w)
	}
}

// widthsKind returns "" (unset), "auto", "grid" or "list".
func (d *directive) widthsKind() string {
	switch v := d.options["widths"].(type) {
	case string:
		return v
	case []int:
		return "list"
	}
	return ""
}

func (d *directive) getColumnWidths(nCols int) []int {
	if w, ok := d.options["widths"].([]int); ok {
		if len(w) != nCols {
			d.propagate(d.reporter().error(fmt.Sprintf(`"%s" widths do not match the number of columns in table (%d).`, d.name, nCols),
				d.literalBlockOfSource(), atLine(d.lineno)))
		}
		return w
	} else if nCols > 0 {
		cw := make([]int, nCols)
		for i := range cw {
			cw[i] = 100 / nCols
		}
		return cw
	}
	d.propagate(d.reporter().error("No table data detected in CSV file.", d.literalBlockOfSource(), atLine(d.lineno)))
	return nil
}

func runRSTTable(d *directive) []*Node {
	if len(d.content.data) == 0 {
		return []*Node{d.reporter().warning(fmt.Sprintf(`Content block expected for the "%s" directive; none found.`, d.name),
			d.literalBlockOfSource(), atLine(d.lineno))}
	}
	title, messages := d.makeTitle()
	node := newElem("Element", "")
	d.state.nestedParse(d.content, d.contentOffset, node, false)
	if len(node.Children) != 1 || node.Children[0].Tag != "table" {
		return []*Node{d.reporter().error(fmt.Sprintf(`Error parsing content block for the "%s" directive: exactly one table expected.`, d.name),
			d.literalBlockOfSource(), atLine(d.lineno))}
	}
	tableNode := node.Children[0]
	tableNode.appendTo("classes", d.optClasses()...)
	d.setTableWidth(tableNode)
	_, _, line, hasLine := d.sm.getSourceAndLine(d.lineno, true)
	tableNode.setLine(line, hasLine)
	if a, ok := d.options["align"]; ok {
		tableNode.setAttr("align", a)
	}
	if d.widthsKind() == "list" {
		tgroup := tableNode.Children[0]
		var colWidths []int
		if e := catchPropagation(func() { colWidths = d.getColumnWidths(tgroup.get("cols").(int)) }); e != nil {
			return []*Node{e}
		}
		i := 0
		for _, child := range tgroup.Children {
			if child.Tag == "colspec" && i < len(colWidths) {
				child.setAttr("colwidth", colWidths[i])
				i++
			}
		}
	}
	if k := d.widthsKind(); k == "auto" {
		tableNode.appendTo("classes", "colwidths-auto")
	} else if k != "" {
		tableNode.appendTo("classes", "colwidths-given")
	}
	d.addName(tableNode)
	if title != nil {
		tableNode.insert(0, title)
	}
	return append([]*Node{tableNode}, messages...)
}

// ---------------------------------------------------------------- CSV

type csvDialect struct {
	delimiter        rune
	quotechar        rune
	escapechar       rune
	hasEscape        bool
	doublequote      bool
	skipinitialspace bool
}

type csvError struct{ msg string }

// csvRead mirrors Python's csv.reader with strict=True, QUOTE_MINIMAL.
func csvRead(lines []string, dia csvDialect) (rows [][]string, err *csvError) {
	const (
		startRecord = iota
		startField
		escapedChar
		inField
		inQuotedField
		escapeInQuotedField
		quoteInQuotedField
		eatCRNL
		afterEscapedCRNL
	)
	const eol = rune(-2)
	state := startRecord
	var fields []string
	var field []rune
	saveField := func() {
		fields = append(fields, string(field))
		field = field[:0]
	}
	process := func(c rune) *csvError {
		switch state {
		case startRecord:
			if c == eol {
				return nil
			} else if c == '\n' || c == '\r' {
				state = eatCRNL
				return nil
			}
			state = startField
			fallthrough
		case startField:
			if c == '\n' || c == '\r' || c == eol {
				saveField()
				if c == eol {
					state = startRecord
				} else {
					state = eatCRNL
				}
			} else if c == dia.quotechar {
				state = inQuotedField
			} else if dia.hasEscape && c == dia.escapechar {
				state = escapedChar
			} else if c == ' ' && dia.skipinitialspace {
			} else if c == dia.delimiter {
				saveField()
			} else {
				field = append(field, c)
				state = inField
			}
		case escapedChar:
			if c == '\n' || c == '\r' {
				field = append(field, c)
				state = afterEscapedCRNL
				return nil
			}
			if c == eol {
				c = '\n'
			}
			field = append(field, c)
			state = inField
		case afterEscapedCRNL, inField:
			if state == afterEscapedCRNL && c == eol {
				return nil
			}
			if c == '\n' || c == '\r' || c == eol {
				saveField()
				if c == eol {
					state = startRecord
				} else {
					state = eatCRNL
				}
			} else if dia.hasEscape && c == dia.escapechar {
				state = escapedChar
			} else if c == dia.delimiter {
				saveField()
				state = startField
			} else {
				field = append(field, c)
				state = inField
			}
		case inQuotedField:
			if c == eol {
			} else if dia.hasEscape && c == dia.escapechar {
				state = escapeInQuotedField
			} else if c == dia.quotechar {
				if dia.doublequote {
					state = quoteInQuotedField
				} else {
					state = inField
				}
			} else {
				field = append(field, c)
			}
		case escapeInQuotedField:
			if c == eol {
				c = '\n'
			}
			field = append(field, c)
			state = inQuotedField
		case quoteInQuotedField:
			if c == dia.quotechar {
				field = append(field, c)
				state = inQuotedField
			} else if c == dia.delimiter {
				saveField()
				state = startField
			} else if c == '\n' || c == '\r' || c == eol {
				saveField()
				if c == eol {
					state = startRecord
				} else {
					state = eatCRNL
				}
			} else {
				return &csvError{fmt.Sprintf("'%c' expected after '%c'", dia.delimiter, dia.quotechar)}
			}
		case eatCRNL:
			if c == '\n' || c == '\r' {
			} else if c == eol {
				state = startRecord
			} else {
				return &csvError{"new-line character seen in unquoted field - do you need to open the file with newline=''?"}
			}
		}
		return nil
	}
	i := 0
	for i < len(lines) {
		// one record
		fields = nil
		field = field[:0]
		state = startRecord
		for {
			if i >= len(lines) {
				if len(field) != 0 || state == inQuotedField {
					return nil, &csvError{"unexpected end of data"}
				}
				return rows, nil
			}
			line := lines[i] + "\n"
			i++
			for _, c := range line {
				if c == 0 {
					return nil, &csvError{"line contains NUL"}
				}
				if e := process(c); e != nil {
					return nil, e
				}
			}
			if e := process(eol); e != nil {
				return nil, e
			}
			if state == startRecord {
				break
			}
		}
		if fields == nil {
			fields = []string{}
		}
		rows = append(rows, fields)
	}
	return rows, nil
}

func (d *directive) csvDialect() csvDialect {
	dia := csvDialect{delimiter: ',', quotechar: '"', doublequote: true, skipinitialspace: true}
	first := func(s string) rune { r, _ := firstRune(s); return r }
	if v, ok := d.options["delim"]; ok {
		dia.delimiter = first(v.(string))
	}
	if d.hasOpt("keepspace") {
		dia.skipinitialspace = false
	}
	if v, ok := d.options["quote"]; ok {
		dia.quotechar = first(v.(string))
	}
	if v, ok := d.options["escape"]; ok {
		dia.doublequote = false
		dia.escapechar = first(v.(string))
		dia.hasEscape = true
	}
	return dia
}

func (d *directive) parseCSVDataIntoRows(csvData []string, source string, hasSource bool) ([][]*tableCell, int) {
	recs, err := csvRead(csvData, d.csvDialect())
	if err != nil {
		panic(err)
	}
	var rows [][]*tableCell
	maxCols := 0
	for _, rec := range recs {
		row := []*tableCell{}
		for _, cell := range rec {
			row = append(row, &tableCell{0, 0, 0, newStringList(pySplitlines(cell, false), source, hasSource)})
		}
		rows = append(rows, row)
		if len(rec) > maxCols {
			maxCols = len(rec)
		}
	}
	return rows, maxCols
}

func runCSVTable(d *directive) (result []*Node) {
	st := d.state.document.settings
	var title *Node
	var messages []*Node
	var tableHead, tableBody [][]*tableCell
	var colWidths []int
	stubColumns := 0
	var csvErr *csvError
	propagated := func() (node *Node) {
		defer func() {
			if r := recover(); r != nil {
				switch e := r.(type) {
				case *systemMessagePropagation:
					node = e.node
				case *csvError:
					csvErr = e
				default:
					panic(r)
				}
			}
		}()
		if !st.fileInsertionEnabled && (d.hasOpt("file") || d.hasOpt("url")) {
			return d.reporter().warning(fmt.Sprintf(`File and URL access deactivated; ignoring "%s" directive.`, d.name),
				d.literalBlockOfSource(), atLine(d.lineno))
		}
		title, messages = d.makeTitle()
		csvData, source, hasSource := d.getCSVData()
		// process_header_option
		maxHeaderCols := 0
		if h, ok := d.options["header"]; ok {
			info, _, ok2 := d.sm.inputLines.info(d.lineno - 1 - d.sm.inputOffset)
			hsrc, hhas := "", false
			if ok2 {
				hsrc, hhas = info.source, info.hasSource
			}
			var rows [][]*tableCell
			rows, maxHeaderCols = d.parseCSVDataIntoRows(strings.Split(h.(string), "\n"), hsrc, hhas)
			tableHead = append(tableHead, rows...)
		}
		rows, maxCols := d.parseCSVDataIntoRows(csvData, source, hasSource)
		if maxHeaderCols > maxCols {
			maxCols = maxHeaderCols
		}
		headerRows, _ := d.options["header-rows"].(int)
		stubColumns, _ = d.options["stub-columns"].(int)
		rowLens := make([]int, len(rows))
		for i, r := range rows {
			rowLens[i] = len(r)
		}
		d.checkTableDimensions(rowLens, headerRows, stubColumns)
		tableHead = append(tableHead, rows[:headerRows]...)
		tableBody = rows[headerRows:]
		colWidths = d.getColumnWidths(maxCols)
		for _, part := range [][][]*tableCell{tableHead, tableBody} {
			for i, row := range part {
				for len(row) < maxCols {
					row = append(row, &tableCell{0, 0, 0, newStringList(nil, "", false)})
				}
				part[i] = row
			}
		}
		return nil
	}()
	if propagated != nil {
		return []*Node{propagated}
	}
	if csvErr != nil {
		return []*Node{d.reporter().error(fmt.Sprintf("Error with CSV data in \"%s\" directive:\n%s", d.name, csvErr.msg),
			d.literalBlockOfSource(), atLine(d.lineno))}
	}
	td := &tableData{colwidths: colWidths, headrows: tableHead, bodyrows: tableBody}
	wk := d.widthsKind()
	if wk == "list" {
		wk = "given"
	}
	tableNode := d.state.buildTable(td, d.contentOffset, stubColumns, nil, wk)
	tableNode.appendTo("classes", d.optClasses()...)
	if a, ok := d.options["align"]; ok {
		tableNode.setAttr("align", a)
	}
	d.setTableWidth(tableNode)
	d.addName(tableNode)
	if title != nil {
		tableNode.insert(0, title)
	}
	tableNode.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	return append([]*Node{tableNode}, messages...)
}

func (d *directive) getCSVData() ([]string, string, bool) {
	st := d.state.document.settings
	if len(d.content.data) > 0 {
		if d.hasOpt("file") || d.hasOpt("url") {
			d.propagate(d.reporter().error(fmt.Sprintf(`"%s" directive may not both specify an external file and have content.`, d.name),
				d.literalBlockOfSource(), atLine(d.lineno)))
		}
		info, _, _ := d.content.info(0)
		return d.content.data, info.source, info.hasSource
	} else if f, ok := d.options["file"]; ok {
		if d.hasOpt("url") {
			d.propagate(d.reporter().error(fmt.Sprintf(`The "file" and "url" options may not be simultaneously specified for the "%s" directive.`, d.name),
				d.literalBlockOfSource(), atLine(d.lineno)))
		}
		source := adaptPath(f.(string), d.state.document.currentSource, st.rootPrefix)
		data, err := os.ReadFile(source)
		if err != nil {
			d.propagate(d.reporter().severe(fmt.Sprintf("Problems with \"%s\" directive path:\n%s.", d.name, ioErrorString(err, source)),
				d.literalBlockOfSource(), atLine(d.lineno)))
		}
		encoding := "utf-8"
		if e, ok := d.options["encoding"]; ok {
			encoding = e.(string)
		}
		text, derr := decodeInput(data, encoding)
		if derr != nil {
			d.propagate(d.reporter().severe(fmt.Sprintf("Problems with \"%s\" directive path:\n%s.", d.name, derr.Error()),
				d.literalBlockOfSource(), atLine(d.lineno)))
		}
		return pySplitlines(text, false), source, true
	} else if u, ok := d.options["url"]; ok {
		d.propagate(d.reporter().severe(fmt.Sprintf("Problems with \"%s\" directive URL \"%s\":\nnetwork access is not supported.", d.name, u),
			d.literalBlockOfSource(), atLine(d.lineno)))
	}
	d.propagate(d.reporter().warning(fmt.Sprintf(`The "%s" directive requires content; none supplied.`, d.name),
		d.literalBlockOfSource(), atLine(d.lineno)))
	return nil, "", false
}

func runListTable(d *directive) []*Node {
	if len(d.content.data) == 0 {
		return []*Node{d.reporter().error(fmt.Sprintf(`The "%s" directive is empty; content required.`, d.name),
			d.literalBlockOfSource(), atLine(d.lineno))}
	}
	title, messages := d.makeTitle()
	node := newElem("Element", "")
	d.state.nestedParse(d.content, d.contentOffset, node, false)
	var colWidths []int
	var tableData [][][]*Node
	headerRows, _ := d.options["header-rows"].(int)
	stubColumns, _ := d.options["stub-columns"].(int)
	if e := catchPropagation(func() {
		colWidths = d.checkListContent(node)
		var rowLens []int
		for _, rowList := range node.Children[0].Children {
			var row [][]*Node
			for _, item := range rowList.Children[0].Children {
				row = append(row, item.Children)
			}
			tableData = append(tableData, row)
			rowLens = append(rowLens, len(row))
		}
		d.checkTableDimensions(rowLens, headerRows, stubColumns)
	}); e != nil {
		return []*Node{e}
	}
	// build_table_from_list
	table := newElem("table", "")
	table.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	if k := d.widthsKind(); k == "auto" {
		table.appendTo("classes", "colwidths-auto")
	} else if k != "" {
		table.appendTo("classes", "colwidths-given")
	}
	tgroup := newElem("tgroup", "")
	tgroup.setAttr("cols", len(colWidths))
	table.append(tgroup)
	sc := stubColumns
	for _, cw := range colWidths {
		colspec := newElem("colspec", "")
		colspec.setAttr("colwidth", cw)
		if sc > 0 {
			colspec.setAttr("stub", true)
			sc--
		}
		tgroup.append(colspec)
	}
	var rows []*Node
	for _, row := range tableData {
		rowNode := newElem("row", "")
		for _, cell := range row {
			entry := newElem("entry", "")
			entry.extend(cell)
			rowNode.append(entry)
		}
		rows = append(rows, rowNode)
	}
	if headerRows > 0 {
		thead := newElem("thead", "")
		thead.extend(rows[:headerRows])
		tgroup.append(thead)
	}
	tbody := newElem("tbody", "")
	tbody.extend(rows[headerRows:])
	tgroup.append(tbody)
	if a, ok := d.options["align"]; ok {
		table.setAttr("align", a)
	}
	table.appendTo("classes", d.optClasses()...)
	d.setTableWidth(table)
	d.addName(table)
	if title != nil {
		table.insert(0, title)
	}
	return append([]*Node{table}, messages...)
}

func (d *directive) checkListContent(node *Node) []int {
	if len(node.Children) != 1 || node.Children[0].Tag != "bullet_list" {
		d.propagate(d.reporter().error(fmt.Sprintf(`Error parsing content block for the "%s" directive: exactly one bullet list expected.`, d.name),
			d.literalBlockOfSource(), atLine(d.lineno)))
	}
	listNode := node.Children[0]
	numCols := 0
	for itemIndex, item := range listNode.Children {
		if len(item.Children) != 1 || item.Children[0].Tag != "bullet_list" {
			d.propagate(d.reporter().error(fmt.Sprintf(`Error parsing content block for the "%s" directive: `+
				`two-level bullet list expected, but row %d does not contain a second-level bullet list.`, d.name, itemIndex+1),
				d.literalBlockOfSource(), atLine(d.lineno)))
		} else if itemIndex > 0 {
			if len(item.Children[0].Children) != numCols {
				d.propagate(d.reporter().error(fmt.Sprintf(`Error parsing content block for the "%s" directive: `+
					`uniform two-level bullet list expected, but row %d does not contain the same number of items as row 1 (%d vs %d).`,
					d.name, itemIndex+1, len(item.Children[0].Children), numCols),
					d.literalBlockOfSource(), atLine(d.lineno)))
			}
		} else {
			numCols = len(item.Children[0].Children)
		}
	}
	return d.getColumnWidths(numCols)
}
