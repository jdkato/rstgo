package rstgo

// Port of docutils.parsers.rst.states (the reStructuredText block-level
// parser).  Method and variable names follow the Python original closely.

import (
	"fmt"
	"strings"
	"sync"
)

type rstState struct {
	kind  string
	sm    *stateMachine
	order []string
	list  []*transition // transitions in order (shared with the prototype)
	// shared reports that order/trans/list belong to the per-kind
	// prototype and must be copied before modification
	shared bool
	trans  map[string]*transition

	memo     *memo
	document *Document
	inliner  *inliner
	reporter *reporter

	// nested-list bookkeeping
	blankFinish bool
	lastordinal int
	format      string
	auto        bool
	// QuotedLiteralBlock
	messages      []*Node
	initialLineno int
}

func (s *rstState) runtimeInit() {
	m := s.sm.memo
	s.memo = m
	s.document = m.document
	s.inliner = m.inliner
	s.reporter = m.document.reporter
	if s.reporter.getSourceAndLine == nil {
		s.reporter.getSourceAndLine = s.sm.getSourceAndLine
	}
}

func (s *rstState) parent() *Node     { return s.sm.node }
func (s *rstState) setParent(n *Node) { s.sm.node = n }

func (s *rstState) gotoLine(absLineOffset int) { s.sm.gotoLine(absLineOffset) }

// unshare gives the state its own transition table.
func (s *rstState) unshare() {
	if !s.shared {
		return
	}
	trans := make(map[string]*transition, len(s.trans))
	for k, v := range s.trans {
		trans[k] = v
	}
	s.trans = trans
	s.order = append([]string{}, s.order...)
	s.shared = false
}

func (s *rstState) relist() {
	s.list = make([]*transition, len(s.order))
	for i, n := range s.order {
		s.list[i] = s.trans[n]
	}
}

func (s *rstState) addTransition(t *transition) {
	s.unshare()
	s.order = append([]string{t.name}, s.order...)
	s.trans[t.name] = t
	s.relist()
}

func (s *rstState) removeTransition(name string) {
	s.unshare()
	delete(s.trans, name)
	for i, n := range s.order {
		if n == name {
			s.order = append(s.order[:i:i], s.order[i+1:]...)
			break
		}
	}
	s.relist()
}

// nestedParse mirrors RSTState.nested_parse with the default state machine.
func (s *rstState) nestedParse(block *stringList, inputOffset int, node *Node, matchTitles bool) int {
	return s.nestedParseWith(block, inputOffset, node, matchTitles, "")
}

func (s *rstState) nestedParseWith(block *stringList, inputOffset int, node *Node, matchTitles bool, initialState string) int {
	if node == nil {
		node = s.sm.node
	}
	var my *stateMachine
	useDefault := initialState == ""
	if useDefault {
		if n := len(s.memo.smCache); n > 0 {
			my = s.memo.smCache[n-1]
			s.memo.smCache = s.memo.smCache[:n-1]
			my.parentSM = s.sm
		}
	}
	if my == nil {
		init := "Body"
		if !useDefault {
			init = initialState
		}
		my = newStateMachine(init, s.sm)
	}
	if node == s.sm.node && !node.is("document", "section") {
		matchTitles = false
	}
	if matchTitles {
		lNode := len(node.sectionHierarchy())
		lStart := len(s.parent().sectionHierarchy())
		if n := len(s.memo.titleStyles); n < lStart {
			lStart = n
		}
		my.sectionLevelOffset = lStart - lNode
	}
	blockLength := len(block.data)
	my.runNested(block, inputOffset, s.memo, node, matchTitles)
	if matchTitles {
		if node == s.sm.node {
			for sm := s.sm; sm != nil; sm = sm.parentSM {
				sm.node = my.node
			}
		}
	}
	newOffset := my.absLineOffset()
	if useDefault {
		s.memo.smCache = append(s.memo.smCache, my)
	}
	if block.parent != nil && len(block.data)-blockLength != 0 {
		s.sm.nextLine(len(block.data) - blockLength)
	}
	return newOffset
}

type nestedListOpts struct {
	blankFinishState string
	lastordinal      int
	format           string
	auto             bool
}

func (s *rstState) nestedListParse(block *stringList, inputOffset int, node *Node, initialState string, blankFinish bool, o nestedListOpts) (int, bool) {
	my := newStateMachine(initialState, s.sm)
	bfs := o.blankFinishState
	if bfs == "" {
		bfs = initialState
	}
	my.state(bfs).blankFinish = blankFinish
	ist := my.state(initialState)
	ist.lastordinal, ist.format, ist.auto = o.lastordinal, o.format, o.auto
	my.runNested(block, inputOffset, s.memo, node, false)
	return my.absLineOffset(), my.state(bfs).blankFinish
}

func (s *rstState) section(title, source, style string, lineno int, messages []*Node) {
	if s.checkSubsection(source, style, lineno) {
		s.newSubsection(title, lineno, messages)
	}
}

func (s *rstState) checkSubsection(source, style string, lineno int) bool {
	titleStyles := s.memo.titleStyles
	parentSections := s.parent().sectionHierarchy()
	oldlevel := len(parentSections) + s.sm.sectionLevelOffset
	newlevel := len(titleStyles) + 1
	for i, st := range titleStyles {
		if st == style {
			newlevel = i + 1
			break
		}
	}
	if newlevel > oldlevel+1 {
		styles := strings.Join(titleStyles, " ")
		s.parent().append(s.reporter.error(
			fmt.Sprintf("Inconsistent title style: skip from level %d to %d.", oldlevel, newlevel),
			[]*Node{newTextElem("literal_block", "", source),
				newTextElem("paragraph", "", "Established title styles: "+styles)},
			atLine(lineno)))
		return false
	}
	if newlevel <= oldlevel {
		idx := pyIndex(newlevel-oldlevel-1, len(parentSections))
		if idx < 0 {
			styles := strings.Join(titleStyles, " ")
			details := fmt.Sprintf("The parent of level %d sections cannot"+
				" be reached. The parser is at section level"+
				" %d but the current node has only"+
				" %d parent section(s)."+
				"\nOne reason may be a high level"+
				" section used in a directive that parses its"+
				" content into a base node not attached to"+
				" the document\n(up to Docutils 0.21,"+
				" these sections were silently dropped).", newlevel, oldlevel, len(parentSections))
			s.parent().append(s.reporter.error(
				fmt.Sprintf("A level %d section cannot be used here.", newlevel),
				[]*Node{newTextElem("literal_block", "", source),
					newTextElem("paragraph", "", "Established title styles: "+styles),
					newTextElem("paragraph", "", details)},
				atLine(lineno)))
			return false
		}
		s.setParent(parentSections[idx].Parent)
	}
	if newlevel > len(titleStyles) {
		s.memo.titleStyles = append(s.memo.titleStyles, style)
	}
	return true
}

func (s *rstState) newSubsection(title string, lineno int, messages []*Node) {
	sectionNode := newElem("section", "")
	s.parent().append(sectionNode)
	textnodes, titleMessages := s.inlineText(title, lineno)
	titlenode := newTextElem("title", title, "", textnodes...)
	name := fullyNormalizeName(titlenode.astext())
	sectionNode.appendTo("names", name)
	sectionNode.append(titlenode)
	sectionNode.extend(messages)
	sectionNode.extend(titleMessages)
	s.document.noteImplicitTarget(sectionNode, sectionNode)
	s.setParent(sectionNode)
}

func (s *rstState) paragraph(lines []string, lineno int) (*Node, []*Node, bool) {
	data := pyRstrip(strings.Join(lines, "\n"))
	var text string
	literalnext := false
	// data has no trailing whitespace, so the pattern can only match a
	// final "::"; checking that first avoids scanning every paragraph
	if strings.HasSuffix(data, "::") && endsWithLiteralMarker(data) {
		if len(data) == 2 {
			return nil, nil, true
		} else if c := data[len(data)-3]; c == ' ' || c == '\n' {
			text = pyRstrip(data[:len(data)-3])
		} else {
			text = data[:len(data)-1]
		}
		literalnext = true
	} else {
		text = data
	}
	textnodes, messages := s.inlineText(text, lineno)
	p := newTextElem("paragraph", data, "", textnodes...)
	p.setSrcLine(s.sm.getSourceAndLine(lineno, true))
	return p, messages, literalnext
}

func (s *rstState) inlineText(text string, lineno int) ([]*Node, []*Node) {
	return s.inliner.parse(text, lineno, s.memo, s.parent())
}

func (s *rstState) unindentWarning(nodeName string) *Node {
	lineno := s.sm.absLineNumber() + 1
	return s.reporter.warning(nodeName+" ends without a blank line; unexpected unindent.", nil, atLine(lineno))
}

// eof dispatches the per-state end-of-input handler.
func (s *rstState) eof(context []string) {
	switch s.kind {
	case "Text":
		if len(context) > 0 {
			textBlank(s, lineMatch{}, context, "")
		}
	case "Definition":
		s.sm.previousLine(2)
	case "Line":
		lineEOF(s, context)
	case "QuotedLiteralBlock":
		quotedEOF(s, context)
	}
}

// ---------------------------------------------------------------- patterns

type enumFormatInfo struct {
	prefix, suffix string
	start, end     int
}

var enumFormatinfo = map[string]enumFormatInfo{
	"parens": {"(", ")", 1, -1},
	"rparen": {"", ")", 0, -1},
	"period": {"", ".", 0, -1},
}

var enumSequences = []string{"arabic", "loweralpha", "upperalpha", "lowerroman", "upperroman"}

var bodyScanners = map[string]lineScanner{
	"blank":            scanBlank,
	"indent":           scanIndent,
	"bullet":           scanBullet,
	"enumerator":       scanEnumerator,
	"field_marker":     scanFieldMarker,
	"option_marker":    scanOptionMarker,
	"doctest":          scanDoctest,
	"line_block":       scanLineBlock,
	"grid_table_top":   scanGridTableTop,
	"simple_table_top": scanSimpleTableTop,
	"explicit_markup":  scanExplicitMarkup,
	"anonymous":        scanAnonymous,
	"line":             scanLine,
	"text":             scanText,
	"underline":        scanLine,
	// SubstitutionDef
	"embedded_directive": scanEmbeddedDirective,
	// QuotedLiteralBlock
	"initial_quoted": scanInitialQuoted,
}

var bodyTransitionNames = []string{"bullet", "enumerator", "field_marker",
	"option_marker", "doctest", "line_block", "grid_table_top",
	"simple_table_top", "explicit_markup", "anonymous", "line", "text"}

func invalidInput(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	s.sm.previousLine(1)
	panic(smEOF{})
}

func specializedTextInvalid(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	panic(smEOF{})
}

func nop(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	return context, nextState
}

var bodyMethods map[string]transFn

func init() {
	bodyMethods = map[string]transFn{
		"blank": nop, "indent": bodyIndent, "bullet": bodyBullet,
		"enumerator": bodyEnumerator, "field_marker": bodyFieldMarker,
		"option_marker": bodyOptionMarker, "doctest": bodyDoctest,
		"line_block": bodyLineBlock, "grid_table_top": bodyGridTableTop,
		"simple_table_top": bodySimpleTableTop, "explicit_markup": bodyExplicitMarkup,
		"anonymous": bodyAnonymous, "line": bodyLine, "text": bodyText,
	}
}

var stateProtos sync.Map // kind -> *rstState with the transition table

// newRSTState returns a state of the given kind; the (immutable)
// transition table is built once per kind and shared.
func newRSTState(kind string, sm *stateMachine) *rstState {
	p, ok := stateProtos.Load(kind)
	if !ok {
		proto := buildRSTState(kind)
		proto.relist()
		p, _ = stateProtos.LoadOrStore(kind, proto)
	}
	proto := p.(*rstState)
	return &rstState{kind: kind, sm: sm, order: proto.order, trans: proto.trans, list: proto.list, shared: true}
}

func buildRSTState(kind string) *rstState {
	s := &rstState{kind: kind, trans: map[string]*transition{}}
	add := func(name string, method transFn, next string) {
		if next == "" {
			next = kind
		}
		s.order = append(s.order, name)
		s.trans[name] = &transition{name, bodyScanners[name], method, next}
	}
	specialized := func(overrides map[string]transFn) {
		add("blank", nop, "")
		add("indent", invalidInput, "")
		for _, n := range bodyTransitionNames {
			add(n, invalidInput, "")
		}
		for n, f := range overrides {
			s.trans[n].method = f
		}
	}
	switch kind {
	case "Body":
		add("blank", nop, "")
		add("indent", bodyIndent, "")
		for _, n := range bodyTransitionNames {
			add(n, bodyMethods[n], "")
		}
	case "BulletList":
		specialized(map[string]transFn{"bullet": bulletListBullet})
	case "DefinitionList":
		specialized(map[string]transFn{"text": definitionListText})
	case "EnumeratedList":
		specialized(map[string]transFn{"enumerator": enumeratedListEnumerator})
	case "FieldList", "ExtensionOptions":
		specialized(map[string]transFn{"field_marker": fieldListFieldMarker})
	case "MetaBody":
		specialized(map[string]transFn{"field_marker": metaFieldMarker})
	case "OptionList":
		specialized(map[string]transFn{"option_marker": optionListOptionMarker})
	case "LineBlock":
		specialized(map[string]transFn{"line_block": lineBlockLineBlock, "blank": invalidInput})
	case "Explicit":
		specialized(map[string]transFn{"explicit_markup": explicitExplicitMarkup,
			"anonymous": explicitAnonymous, "blank": invalidInput})
	case "SubstitutionDef":
		add("blank", nop, "")
		add("indent", bodyIndent, "")
		add("embedded_directive", substEmbeddedDirective, "")
		add("text", substText, "")
	case "Text":
		add("blank", textBlank, "")
		add("indent", textIndent, "")
		add("underline", textUnderline, "Body")
		add("text", textText, "Body")
	case "Definition":
		add("blank", specializedTextInvalid, "")
		add("indent", definitionIndent, "")
		add("underline", specializedTextInvalid, "Body")
		add("text", specializedTextInvalid, "Body")
	case "Line":
		add("blank", lineBlank, "")
		add("indent", lineText, "")
		add("underline", lineUnderline, "Body")
		add("text", lineText, "Body")
	case "QuotedLiteralBlock":
		add("blank", quotedBlank, "")
		add("indent", quotedIndent, "")
		add("initial_quoted", quotedInitialQuoted, "")
		add("text", quotedText, "")
	default:
		panic("unknown state " + kind)
	}
	return s
}

// ---------------------------------------------------------------- Body

func bodyIndent(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	indented, _, lineOffset, blankFinish := s.sm.getIndented(false, true)
	elements := s.blockQuote(indented, lineOffset)
	s.parent().extend(elements)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Block quote"))
	}
	return context, nextState
}

func (s *rstState) blockQuote(indented *stringList, lineOffset int) []*Node {
	var elements []*Node
	for indented != nil && len(indented.data) > 0 {
		blockquote := newElem("block_quote", indented.join())
		blockquote.setSrcLine(s.sm.getSourceAndLine(lineOffset+1, true))
		bqLines, attrLines, attrOffset, rest, newLineOffset := s.splitAttribution(indented, lineOffset)
		s.nestedParse(bqLines, lineOffset, blockquote, false)
		elements = append(elements, blockquote)
		if attrLines != nil {
			attribution, messages := s.parseAttribution(attrLines, lineOffset+attrOffset)
			blockquote.append(attribution)
			elements = append(elements, messages...)
		}
		lineOffset = newLineOffset
		indented = rest
		for indented != nil && len(indented.data) > 0 && indented.data[0] == "" {
			indented = indented.from(1)
			lineOffset++
		}
	}
	return elements
}

func (s *rstState) splitAttribution(indented *stringList, lineOffset int) (*stringList, *stringList, int, *stringList, int) {
	blank := -1
	nonblankSeen := false
	for i := 0; i < len(indented.data); i++ {
		line := pyRstrip(indented.data[i])
		if line != "" {
			if nonblankSeen && blank == i-1 {
				if matchEnd := scanAttribution(line); matchEnd >= 0 {
					attributionEnd, indent := s.checkAttribution(indented, i)
					if attributionEnd > 0 {
						aLines := indented.slice(i, attributionEnd)
						aLines.trimLeft(matchEnd, 0, 1)
						aLines.trimLeft(indent, 1, len(aLines.data))
						return indented.slice(0, i), aLines, i, indented.from(attributionEnd), lineOffset + attributionEnd
					}
				}
			}
			nonblankSeen = true
		} else {
			blank = i
		}
	}
	return indented, nil, 0, nil, 0
}

func (s *rstState) checkAttribution(indented *stringList, attributionStart int) (int, int) {
	indent := -1
	i := attributionStart + 1
	broke := false
	for i = attributionStart + 1; i < len(indented.data); i++ {
		line := pyRstrip(indented.data[i])
		if line == "" {
			broke = true
			break
		}
		li := len(line) - len(pyLstrip(line))
		if indent < 0 {
			indent = li
		} else if li != indent {
			return 0, 0
		}
	}
	if !broke {
		// Python: loop variable keeps last value, then += 1
		if attributionStart+1 < len(indented.data) {
			i = len(indented.data)
		} else {
			i = attributionStart + 2
		}
	}
	if indent < 0 {
		indent = 0
	}
	return i, indent
}

func (s *rstState) parseAttribution(indented *stringList, lineOffset int) (*Node, []*Node) {
	text := pyRstrip(indented.join())
	lineno := 1 + lineOffset
	textnodes, messages := s.inlineText(text, lineno)
	node := newTextElem("attribution", text, "", textnodes...)
	node.setSrcLine(s.sm.getSourceAndLine(lineno, true))
	return node, messages
}

func bodyBullet(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	ul := newElem("bullet_list", "")
	ul.setSrcLine(s.sm.curSrcLine())
	s.parent().append(ul)
	_, w := firstRune(m.s)
	ul.setAttr("bullet", m.s[:w])
	i, blankFinish := s.listItem(m.End(0))
	ul.append(i)
	offset := s.sm.lineOffset + 1
	newLineOffset, blankFinish := s.nestedListParse(s.sm.inputLines.view(offset),
		s.sm.absLineOffset()+1, ul, "BulletList", blankFinish, nestedListOpts{})
	s.gotoLine(newLineOffset)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Bullet list"))
	}
	return []string{}, nextState
}

func (s *rstState) listItem(indent int) (*Node, bool) {
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	var indented *stringList
	var lineOffset int
	var blankFinish bool
	if cutPrefix(s.sm.line, indent) != "" {
		indented, lineOffset, blankFinish = s.sm.getKnownIndented(indent, false, true)
	} else {
		indented, _, lineOffset, blankFinish = s.sm.getFirstKnownIndented(indent, false, true, true)
	}
	listitem := newElem("list_item", indented.join())
	listitem.setSrcLine(src, hasSrc, srcline, hasLine)
	if len(indented.data) > 0 {
		s.nestedParse(indented, lineOffset, listitem, false)
	}
	return listitem, blankFinish
}

func bodyEnumerator(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	format, sequence, text, ordinal, okOrd := s.parseEnumerator(m, "")
	if !s.isEnumeratedListItem(ordinal, okOrd, sequence, format) {
		panic(&transitionCorrection{"text"})
	}
	enumlist := newElem("enumerated_list", "")
	enumlist.setSrcLine(s.sm.curSrcLine())
	s.parent().append(enumlist)
	if sequence == "#" {
		enumlist.setAttr("enumtype", "arabic")
	} else {
		enumlist.setAttr("enumtype", sequence)
	}
	enumlist.setAttr("prefix", enumFormatinfo[format].prefix)
	enumlist.setAttr("suffix", enumFormatinfo[format].suffix)
	if ordinal != 1 {
		enumlist.setAttr("start", ordinal)
		msg := s.reporter.info(fmt.Sprintf(`Enumerated list start value not ordinal-1: "%s" (ordinal %d)`, text, ordinal),
			nil, baseNode(enumlist))
		s.parent().append(msg)
	}
	listitem, blankFinish := s.listItem(m.End(0))
	enumlist.append(listitem)
	offset := s.sm.lineOffset + 1
	newlineOffset, blankFinish := s.nestedListParse(s.sm.inputLines.view(offset),
		s.sm.absLineOffset()+1, enumlist, "EnumeratedList", blankFinish,
		nestedListOpts{lastordinal: ordinal, format: format, auto: sequence == "#"})
	s.gotoLine(newlineOffset)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Enumerated list"))
	}
	return []string{}, nextState
}

func (s *rstState) parseEnumerator(m lineMatch, expectedSequence string) (format, sequence, text string, ordinal int, ok bool) {
	format = m.format
	g := m.Group(1)
	fi := enumFormatinfo[format]
	text = g[fi.start : len(g)+fi.end]
	if text == "#" {
		sequence = "#"
	} else if expectedSequence != "" {
		if enumSequenceMatches(expectedSequence, text) {
			sequence = expectedSequence
		}
	} else if text == "i" {
		sequence = "lowerroman"
	} else if text == "I" {
		sequence = "upperroman"
	}
	if sequence == "" {
		for _, seq := range enumSequences {
			if enumSequenceMatches(seq, text) {
				sequence = seq
				break
			}
		}
	}
	ok = true
	switch sequence {
	case "#":
		ordinal = 1
	case "arabic":
		ordinal = atoiBig(text)
	case "loweralpha":
		ordinal = int(text[0]) - 'a' + 1
	case "upperalpha":
		ordinal = int(text[0]) - 'A' + 1
	default:
		ordinal, ok = romanFromString(text)
	}
	return
}

func atoiBig(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if n > 1<<55 {
			return n
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func (s *rstState) isEnumeratedListItem(ordinal int, okOrd bool, sequence, format string) bool {
	if !okOrd {
		return false
	}
	nextLine, ok := s.sm.nextLine(1)
	s.sm.previousLine(1)
	if !ok {
		return true
	}
	if isBlank(prefix(nextLine, 1)) {
		return true
	}
	next, auto, ok := makeEnumerator(ordinal+1, sequence, format)
	if ok {
		if strings.HasPrefix(nextLine, next) || strings.HasPrefix(nextLine, auto) {
			return true
		}
	}
	return false
}

func makeEnumerator(ordinal int, sequence, format string) (string, string, bool) {
	var enumerator string
	switch {
	case sequence == "#":
		enumerator = "#"
	case sequence == "arabic":
		enumerator = fmt.Sprint(ordinal)
	case strings.HasSuffix(sequence, "alpha"):
		if ordinal > 26 {
			return "", "", false
		}
		enumerator = string(rune(ordinal + 'a' - 1))
	default:
		r, ok := toRoman(ordinal)
		if !ok {
			return "", "", false
		}
		enumerator = r
	}
	if sequence != "#" && sequence != "arabic" {
		if strings.HasPrefix(sequence, "lower") {
			enumerator = strings.ToLower(enumerator)
		} else {
			enumerator = strings.ToUpper(enumerator)
		}
	}
	fi := enumFormatinfo[format]
	return fi.prefix + enumerator + fi.suffix + " ", fi.prefix + "#" + fi.suffix + " ", true
}

func bodyFieldMarker(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	fieldList := newElem("field_list", "")
	s.parent().append(fieldList)
	field, blankFinish := s.field(m)
	fieldList.append(field)
	offset := s.sm.lineOffset + 1
	newlineOffset, blankFinish := s.nestedListParse(s.sm.inputLines.view(offset),
		s.sm.absLineOffset()+1, fieldList, "FieldList", blankFinish, nestedListOpts{})
	s.gotoLine(newlineOffset)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Field list"))
	}
	return []string{}, nextState
}

func (s *rstState) field(m lineMatch) (*Node, bool) {
	name := parseFieldMarker(m)
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	lineno := s.sm.absLineNumber()
	indented, _, lineOffset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
	fieldNode := newElem("field", "")
	fieldNode.setSrcLine(src, hasSrc, srcline, hasLine)
	nameNodes, nameMessages := s.inlineText(name, lineno)
	fieldNode.append(newTextElem("field_name", name, "", nameNodes...))
	fieldBody := newElem("field_body", indented.join(), nameMessages...)
	fieldNode.append(fieldBody)
	if len(indented.data) > 0 {
		s.parseFieldBody(indented, lineOffset, fieldBody)
	}
	return fieldNode, blankFinish
}

func parseFieldMarker(m lineMatch) string {
	field := m.Group(0)[1:]
	return field[:strings.LastIndex(field, ":")]
}

func (s *rstState) parseFieldBody(indented *stringList, offset int, node *Node) {
	if s.kind == "ExtensionOptions" {
		var lines []string
		for _, line := range append(append([]string{}, indented.data...), "") {
			if !isBlank(line) {
				lines = append(lines, line)
			} else if len(lines) > 0 {
				text := strings.Join(lines, "\n")
				node.append(newTextElem("paragraph", text, text))
				lines = nil
			}
		}
		return
	}
	s.nestedParse(indented, offset, node, false)
}

func bodyOptionMarker(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	optionlist := newElem("option_list", "")
	optionlist.setSrcLine(s.sm.curSrcLine())
	listitem, blankFinish, err := s.optionListItem(m)
	if err != nil {
		msg := s.reporter.error("Invalid option list marker: "+err.msg, nil)
		s.parent().append(msg)
		indented, _, lineOffset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
		elements := s.blockQuote(indented, lineOffset)
		s.parent().extend(elements)
		if !blankFinish {
			s.parent().append(s.unindentWarning("Option list"))
		}
		return []string{}, nextState
	}
	s.parent().append(optionlist)
	optionlist.append(listitem)
	offset := s.sm.lineOffset + 1
	newlineOffset, blankFinish := s.nestedListParse(s.sm.inputLines.view(offset),
		s.sm.absLineOffset()+1, optionlist, "OptionList", blankFinish, nestedListOpts{})
	s.gotoLine(newlineOffset)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Option list"))
	}
	return []string{}, nextState
}

func (s *rstState) optionListItem(m lineMatch) (*Node, bool, *markupError) {
	offset := s.sm.absLineOffset()
	options, err := s.parseOptionMarker(m)
	if err != nil {
		return nil, false, err
	}
	indented, _, lineOffset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
	if len(indented.data) == 0 {
		s.gotoLine(offset)
		panic(&transitionCorrection{"text"})
	}
	optionGroup := newElem("option_group", "", options...)
	description := newElem("description", indented.join())
	item := newElem("option_list_item", "", optionGroup, description)
	if len(indented.data) > 0 {
		s.nestedParse(indented, lineOffset, description, false)
	}
	return item, blankFinish, nil
}

func (s *rstState) parseOptionMarker(m lineMatch) ([]*Node, *markupError) {
	var optlist []*Node
	optionstrings := splitOptions(pyRstrip(m.Group(0)))
	for _, optionstring := range optionstrings {
		tokens := pySplit(optionstring)
		delimiter := " "
		firstopt := strings.SplitN(tokens[0], "=", 2)
		if len(firstopt) > 1 {
			tokens = append(firstopt, tokens[1:]...)
			delimiter = "="
		} else if len(tokens[0]) > 2 &&
			((strings.HasPrefix(tokens[0], "-") && !strings.HasPrefix(tokens[0], "--")) ||
				strings.HasPrefix(tokens[0], "+")) {
			tokens = append([]string{tokens[0][:2], tokens[0][2:]}, tokens[1:]...)
			delimiter = ""
		}
		if len(tokens) > 1 && strings.HasPrefix(tokens[1], "<") && strings.HasSuffix(tokens[len(tokens)-1], ">") {
			tokens = []string{tokens[0], strings.Join(tokens[1:], " ")}
		}
		if 0 < len(tokens) && len(tokens) <= 2 {
			option := newElem("option", optionstring)
			option.append(newTextElem("option_string", tokens[0], tokens[0]))
			if len(tokens) > 1 {
				arg := newTextElem("option_argument", tokens[1], tokens[1])
				arg.setAttr("delimiter", delimiter)
				option.append(arg)
			}
			optlist = append(optlist, option)
		} else {
			return nil, &markupError{fmt.Sprintf(
				`wrong number of option tokens (=%d), should be 1 or 2: "%s"`, len(tokens), optionstring)}
		}
	}
	return optlist, nil
}

func bodyDoctest(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	line, hasLine := s.document.currentLine, s.document.hasCurrentLine
	block, _ := s.sm.getTextBlock(false)
	data := block.join()
	n := newTextElem("doctest_block", data, data)
	n.setLine(line, hasLine)
	s.parent().append(n)
	return []string{}, nextState
}

func bodyLineBlock(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	block := newElem("line_block", "")
	s.parent().append(block)
	lineno := s.sm.absLineNumber()
	block.setSrcLine(s.sm.getSourceAndLine(lineno, true))
	line, messages, blankFinish := s.lineBlockLine(m, lineno)
	block.append(line)
	s.parent().extend(messages)
	if !blankFinish {
		offset := s.sm.lineOffset + 1
		var newLineOffset int
		newLineOffset, blankFinish = s.nestedListParse(s.sm.inputLines.view(offset),
			s.sm.absLineOffset()+1, block, "LineBlock", false, nestedListOpts{})
		s.gotoLine(newLineOffset)
	}
	if !blankFinish {
		s.parent().append(s.reporter.warning("Line block ends without a blank line.", nil, atLine(lineno+1)))
	}
	if len(block.Children) > 0 {
		if !block.Children[0].hasIndent() {
			block.Children[0].setIndent(0)
		}
		nestLineBlockLines(block)
	}
	return []string{}, nextState
}

func (s *rstState) lineBlockLine(m lineMatch, lineno int) (*Node, []*Node, bool) {
	indented, _, _, blankFinish := s.sm.getFirstKnownIndented(m.End(0), true, true, true)
	text := indented.join()
	textNodes, messages := s.inlineText(text, lineno)
	line := newTextElem("line", text, "", textNodes...)
	line.setSrcLine(s.sm.getSourceAndLine(lineno, true))
	if pyRstrip(m.s) != "|" {
		line.setIndent(len(m.Group(1)) - 1)
	}
	return line, messages, blankFinish
}

func nestLineBlockLines(block *Node) {
	for i := 1; i < len(block.Children); i++ {
		if !block.Children[i].hasIndent() {
			block.Children[i].setIndent(block.Children[i-1].indentOf())
		}
	}
	nestLineBlockSegment(block)
}

func nestLineBlockSegment(block *Node) {
	least := block.Children[0].indentOf()
	for _, item := range block.Children {
		if item.indentOf() < least {
			least = item.indentOf()
		}
	}
	var newItems []*Node
	newBlock := newElem("line_block", "")
	for _, item := range block.Children {
		if item.indentOf() > least {
			newBlock.append(item)
		} else {
			if len(newBlock.Children) > 0 {
				nestLineBlockSegment(newBlock)
				newItems = append(newItems, newBlock)
				newBlock = newElem("line_block", "")
			}
			newItems = append(newItems, item)
		}
	}
	if len(newBlock.Children) > 0 {
		nestLineBlockSegment(newBlock)
		newItems = append(newItems, newBlock)
	}
	block.Children = nil
	for _, it := range newItems {
		block.append(it)
	}
}

func bodyGridTableTop(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	return s.tableTop(nextState, true)
}

func bodySimpleTableTop(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	return s.tableTop(nextState, false)
}

func (s *rstState) tableTop(nextState string, grid bool) ([]string, string) {
	nodelist, blankFinish := s.table(grid)
	s.parent().extend(nodelist)
	if !blankFinish {
		msg := s.reporter.warning("Blank line required after table.", nil, atLine(s.sm.absLineNumber()+1))
		s.parent().append(msg)
	}
	return []string{}, nextState
}

func (s *rstState) table(grid bool) ([]*Node, bool) {
	var block *stringList
	var messages []*Node
	var blankFinish bool
	if grid {
		block, messages, blankFinish = s.isolateGridTable()
	} else {
		block, messages, blankFinish = s.isolateSimpleTable()
	}
	var nodelist []*Node
	if block != nil && len(block.data) > 0 {
		var tabledata *tableData
		var err *tableMarkupError
		if grid {
			tabledata, err = parseGridTable(block)
		} else {
			tabledata, err = parseSimpleTable(block)
		}
		if err != nil {
			nodelist = append(s.malformedTable(block, err.msg, err.offset), messages...)
		} else {
			tableline := s.sm.absLineNumber() - len(block.data) + 1
			table := s.buildTable(tabledata, tableline, 0, nil, "")
			nodelist = append([]*Node{table}, messages...)
		}
	} else {
		nodelist = messages
	}
	return nodelist, blankFinish
}

const doubleWidthPadChar = "\x00"

func (s *rstState) isolateGridTable() (*stringList, []*Node, bool) {
	var messages []*Node
	blankFinish := true
	block, uerr := s.sm.getTextBlock(true)
	if uerr != nil {
		messages = append(messages, s.reporter.error("Unexpected indentation.", nil,
			atSrcLine(uerr.info.source, uerr.info.hasSource, uerr.line, true)))
		blankFinish = false
	}
	block.disconnect()
	block.padDoubleWidth(doubleWidthPadChar)
	width := runeLen(pyStrip(block.data[0]))
	i := 0
	for i = 0; i < len(block.data); i++ {
		block.data[i] = pyStrip(block.data[i])
		if c := block.data[i][0]; c != '+' && c != '|' {
			blankFinish = false
			s.sm.previousLine(len(block.data) - i)
			block.deleteRange(i, len(block.data))
			break
		}
	}
	if i >= len(block.data) && i > 0 {
		// Python leaves the loop variable at its last value
		if i > len(block.data) {
			i = len(block.data)
		} else if !(i == len(block.data) && !blankFinish) {
			i = len(block.data) - 1
		}
	}
	if len(block.data) == 0 {
		return nil, messages, blankFinish
	}
	if !isGridTableTop(block.data[len(block.data)-1]) {
		found := false
		for j := len(block.data) - 2; j > 1; j-- {
			i = j
			if isGridTableTop(block.data[j]) {
				s.sm.previousLine(len(block.data) - j + 1)
				block.deleteRange(j+1, len(block.data))
				blankFinish = false
				found = true
				break
			}
		}
		if !found {
			messages = append(messages, s.malformedTable(block, "Bottom border missing or corrupt.", i)...)
			return nil, messages, blankFinish
		}
	}
	for i := 0; i < len(block.data); i++ {
		line := block.data[i]
		if runeLen(stripCombiningChars(line)) != width || (line[len(line)-1] != '+' && line[len(line)-1] != '|') {
			messages = append(messages, s.malformedTable(block, "Right border not aligned or missing.", i)...)
			return nil, messages, blankFinish
		}
	}
	return block, messages, blankFinish
}

func (s *rstState) isolateSimpleTable() (*stringList, []*Node, bool) {
	start := s.sm.lineOffset
	lines := s.sm.inputLines
	limit := len(lines.data) - 1
	toplen := len(pyStrip(lines.data[start]))
	found := 0
	foundAt := -1
	i := start + 1
	end := -1
	for i <= limit {
		line := lines.data[i]
		if isSimpleTableBorder(line) {
			if len(pyStrip(line)) != toplen {
				s.sm.nextLine(i - start)
				messages := s.malformedTable(lines.slice(start, i+1),
					"Bottom border or header rule does not match top border.", i-start)
				return nil, messages, i == limit || isBlank(lines.data[i+1])
			}
			found++
			foundAt = i
			if found == 2 || i == limit || isBlank(lines.data[i+1]) {
				end = i
				break
			}
		}
		i++
	}
	if end < 0 {
		details := "No bottom table border found"
		var block *stringList
		if found > 0 {
			details += " or no blank line after table bottom"
			s.sm.nextLine(foundAt - start)
			block = lines.slice(start, foundAt+1)
		} else {
			s.sm.nextLine(i - start - 1)
			block = lines.from(start)
		}
		messages := s.malformedTable(block, details+".", 0)
		return nil, messages, found == 0
	}
	s.sm.nextLine(end - start)
	block := lines.slice(start, end+1)
	block.padDoubleWidth(doubleWidthPadChar)
	return block, nil, end == limit || isBlank(lines.data[end+1])
}

func (s *rstState) malformedTable(block *stringList, detail string, offset int) []*Node {
	block.replaceAll(doubleWidthPadChar, "")
	data := block.join()
	message := "Malformed table."
	startline := s.sm.absLineNumber() - len(block.data) + 1
	if detail != "" {
		message += "\n" + detail
	}
	e := s.reporter.error(message, []*Node{newTextElem("literal_block", data, data)}, atLine(startline+offset))
	return []*Node{e}
}

// buildTable mirrors Body.build_table; widthsKind is "", "auto" or "given".
func (s *rstState) buildTable(td *tableData, tableline int, stubColumns int, _ []int, widthsKind string) *Node {
	table := newElem("table", "")
	table.setSrcLine(s.sm.getSourceAndLine(tableline, true))
	if widthsKind == "auto" {
		table.appendTo("classes", "colwidths-auto")
	} else if widthsKind != "" {
		table.appendTo("classes", "colwidths-given")
	}
	tgroup := newElem("tgroup", "")
	tgroup.setAttr("cols", len(td.colwidths))
	table.append(tgroup)
	for _, cw := range td.colwidths {
		colspec := newElem("colspec", "")
		colspec.setAttr("colwidth", cw)
		if stubColumns > 0 {
			colspec.setAttr("stub", true)
			stubColumns--
		}
		tgroup.append(colspec)
	}
	if len(td.headrows) > 0 {
		thead := newElem("thead", "")
		tgroup.append(thead)
		for _, row := range td.headrows {
			thead.append(s.buildTableRow(row, tableline))
		}
	}
	tbody := newElem("tbody", "")
	tgroup.append(tbody)
	for _, row := range td.bodyrows {
		tbody.append(s.buildTableRow(row, tableline))
	}
	return table
}

func (s *rstState) buildTableRow(rowdata []*tableCell, tableline int) *Node {
	row := newElem("row", "")
	for _, cell := range rowdata {
		if cell == nil {
			continue
		}
		entry := newElem("entry", "")
		if cell.morerows != 0 {
			entry.setAttr("morerows", cell.morerows)
		}
		if cell.morecols != 0 {
			entry.setAttr("morecols", cell.morecols)
		}
		row.append(entry)
		if strings.Join(cell.block.data, "") != "" {
			s.nestedParse(cell.block, tableline+cell.offset-1, entry, false)
		}
	}
	return row
}

func firstRune(s string) (rune, int) {
	for i, r := range s {
		_ = i
		return r, len(string(r))
	}
	return 0, 0
}
