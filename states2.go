package rstgo

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------- explicit markup

type explicitConstruct struct {
	method func(s *rstState, m lineMatch) ([]*Node, bool)
	scan   lineScanner
}

var explicitConstructs []explicitConstruct

func init() {
	explicitConstructs = []explicitConstruct{
		{(*rstState).footnote, scanFootnote},
		{(*rstState).citation, scanCitation},
		{(*rstState).hyperlinkTarget, scanHyperlinkTarget},
		{(*rstState).substitutionDef, scanSubstitutionDef},
		{func(s *rstState, m lineMatch) ([]*Node, bool) { return s.directive(m, nil) }, scanDirective},
	}
}

func (s *rstState) footnote(m lineMatch) ([]*Node, bool) {
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	indented, _, offset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
	label := m.Group(1)
	name := fullyNormalizeName(label)
	footnote := newElem("footnote", indented.join())
	footnote.setSrcLine(src, hasSrc, srcline, hasLine)
	if name[0] == '#' {
		name = name[1:]
		footnote.setAttr("auto", 1)
		if name != "" {
			footnote.appendTo("names", name)
		}
		s.document.noteAutofootnote(footnote)
	} else if name == "*" {
		name = ""
		footnote.setAttr("auto", "*")
		s.document.noteSymbolFootnote(footnote)
	} else {
		footnote.append(newTextElem("label", "", label))
		footnote.appendTo("names", name)
		s.document.noteFootnote(footnote)
	}
	if name != "" {
		s.document.noteExplicitTarget(footnote, footnote)
	} else {
		s.document.setID(footnote, footnote, "")
	}
	if len(indented.data) > 0 {
		s.nestedParse(indented, offset, footnote, false)
	} else {
		footnote.append(s.reporter.warning("Footnote content expected.", nil))
	}
	return []*Node{footnote}, blankFinish
}

func (s *rstState) citation(m lineMatch) ([]*Node, bool) {
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	indented, _, offset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
	label := m.Group(1)
	name := fullyNormalizeName(label)
	citation := newElem("citation", indented.join())
	citation.setSrcLine(src, hasSrc, srcline, hasLine)
	citation.append(newTextElem("label", "", label))
	citation.appendTo("names", name)
	s.document.noteCitation(citation)
	s.document.noteExplicitTarget(citation, citation)
	if len(indented.data) > 0 {
		s.nestedParse(indented, offset, citation, false)
	} else {
		citation.append(s.reporter.warning("Citation content expected.", nil))
	}
	return []*Node{citation}, blankFinish
}

func (s *rstState) hyperlinkTarget(m lineMatch) ([]*Node, bool) {
	lineno := s.sm.absLineNumber()
	sl, _, _, blankFinish := s.sm.getFirstKnownIndented(m.End(0), true, false, true)
	blocktext := m.s[:m.End(0)] + sl.join()
	block := make([]string, len(sl.data))
	for i, line := range sl.data {
		block[i] = escape2null(line)
	}
	if len(block) == 0 {
		panic(&markupError{"malformed hyperlink target."})
	}
	escaped := block[0]
	blockindex := 0
	var targetEnd int
	var targetName string
	for {
		var ok bool
		targetEnd, targetName, ok = scanExplicitTarget(escaped)
		if ok {
			break
		}
		blockindex++
		if blockindex >= len(block) {
			panic(&markupError{"malformed hyperlink target."})
		}
		escaped += block[blockindex]
	}
	block = block[blockindex:]
	b0 := block[0] + " "
	idx := targetEnd - len(escaped) - 1 // negative index from end
	block[0] = pyStrip(pySliceFrom(b0, idx))
	target := s.makeTarget(block, blocktext, lineno, targetName)
	return []*Node{target}, blankFinish
}

// pySliceFrom is s[i:] with Python semantics for negative i.
func pySliceFrom(s string, i int) string {
	if i < 0 {
		i += len(s)
		if i < 0 {
			i = 0
		}
	}
	if i > len(s) {
		return ""
	}
	return s[i:]
}

func (s *rstState) makeTarget(block []string, blockText string, lineno int, targetName string) *Node {
	targetType, data := s.parseTarget(block)
	if targetType == "refname" {
		target := newTextElem("target", blockText, "")
		target.setAttr("refname", fullyNormalizeName(data))
		s.addTarget(targetName, "", target, lineno)
		s.document.noteIndirectTarget(target)
		return target
	}
	target := newTextElem("target", blockText, "")
	s.addTarget(targetName, data, target, lineno)
	return target
}

func (s *rstState) parseTarget(block []string) (string, string) {
	if len(block) > 0 {
		last := pyStrip(block[len(block)-1])
		if strings.HasSuffix(last, "_") {
			parts := make([]string, len(block))
			for i, l := range block {
				parts[i] = pyStrip(l)
			}
			reference := strings.Join(parts, " ")
			if refname, ok := isReference(reference); ok && refname != "" {
				return "refname", refname
			}
		}
	}
	refParts := splitEscapedWhitespace(strings.Join(block, " "))
	for i, part := range refParts {
		refParts[i] = strings.Join(pySplit(unescape(part, false)), "")
	}
	return "refuri", strings.Join(refParts, " ")
}

func isReference(reference string) (string, bool) {
	ref, ok := scanExplicitReference(whitespaceNormalizeName(reference))
	if !ok {
		return "", false
	}
	return unescape(ref, false), true
}

func (s *rstState) addTarget(targetname, refuri string, target *Node, lineno int) {
	target.setLine(lineno, true)
	if targetname != "" {
		name := fullyNormalizeName(unescape(targetname, false))
		target.appendTo("names", name)
		if refuri != "" {
			uri := s.inliner.adjustURI(refuri)
			target.setAttr("refuri", uri)
		}
		s.document.noteExplicitTarget(target, s.parent())
	} else {
		if refuri != "" {
			target.setAttr("refuri", refuri)
		}
		target.setAttr("anonymous", true)
		s.document.noteAnonymousTarget(target)
	}
}

func (s *rstState) substitutionDef(m lineMatch) ([]*Node, bool) {
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	block, _, offset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, false, true)
	blocktext := m.s[:m.End(0)] + block.join()
	block.disconnect()
	if len(block.data) == 0 {
		panic(&markupError{"malformed substitution definition."})
	}
	escaped := escape2null(pyRstrip(block.data[0]))
	blockindex := 0
	var subdefEnd int
	var subname string
	for {
		var ok bool
		subdefEnd, subname, ok = scanExplicitSubstitution(escaped)
		if ok {
			break
		}
		blockindex++
		if blockindex >= len(block.data) {
			panic(&markupError{"malformed substitution definition."})
		}
		escaped = escaped + " " + escape2null(pyStrip(block.data[blockindex]))
	}
	block.deleteRange(0, blockindex)
	start := subdefEnd - len(escaped) - 1
	b0 := pyStrip(block.data[0]) + " "
	// Python: b0[start:-1]
	i := start
	if i < 0 {
		i += len(b0)
		if i < 0 {
			i = 0
		}
	}
	if i > len(b0)-1 {
		block.data[0] = ""
	} else {
		block.data[0] = b0[i : len(b0)-1]
	}
	if block.data[0] == "" {
		block.deleteRange(0, 1)
		offset++
	}
	for len(block.data) > 0 && isBlank(block.data[len(block.data)-1]) {
		block.pop()
	}
	substitutionNode := newElem("substitution_definition", blocktext)
	substitutionNode.setSrcLine(src, hasSrc, srcline, hasLine)
	if len(block.data) == 0 {
		msg := s.reporter.warning(fmt.Sprintf(`Substitution definition "%s" missing contents.`, subname),
			[]*Node{newTextElem("literal_block", blocktext, blocktext)},
			atSrcLine(src, hasSrc, srcline, hasLine))
		return []*Node{msg}, blankFinish
	}
	block.data[0] = pyStrip(block.data[0])
	substitutionNode.appendTo("names", whitespaceNormalizeName(subname))
	_, blankFinish = s.nestedListParse(block, offset, substitutionNode, "SubstitutionDef", blankFinish, nestedListOpts{})
	i = 0
	for _, node := range append([]*Node{}, substitutionNode.Children...) {
		if !(node.isInline() || node.IsText()) {
			s.parent().append(substitutionNode.Children[i])
			substitutionNode.removeAt(i)
		} else {
			i++
		}
	}
	for _, node := range substitutionNode.findall(func(n *Node) bool { return !n.IsText() })[1:] {
		if node.Tag == "problematic" {
			msg := s.reporter.error("Problematic content in substitution definition",
				[]*Node{newTextElem("literal_block", "", blocktext)},
				atSrcLine(src, hasSrc, srcline, hasLine))
			msg.append(newElem("block_quote", "", newTextElem("paragraph", "", "", substitutionNode.Children...)))
			return []*Node{msg}, blankFinish
		}
		illegal := disallowedInsideSubstitutionDefinitions(node)
		if illegal != "" {
			msg := s.reporter.error(illegal+" are not supported in a substitution definition.",
				[]*Node{newTextElem("literal_block", "", blocktext)},
				atSrcLine(src, hasSrc, srcline, hasLine))
			return []*Node{msg}, blankFinish
		}
	}
	if len(substitutionNode.Children) == 0 {
		msg := s.reporter.warning(fmt.Sprintf(`Substitution definition "%s" empty or invalid.`, subname),
			[]*Node{newTextElem("literal_block", blocktext, blocktext)},
			atSrcLine(src, hasSrc, srcline, hasLine))
		return []*Node{msg}, blankFinish
	}
	s.document.noteSubstitutionDef(substitutionNode, subname, s.parent())
	return []*Node{substitutionNode}, blankFinish
}

func disallowedInsideSubstitutionDefinitions(node *Node) string {
	if node.Tag == "reference" && node.truthy("anonymous") {
		return "Anonymous references"
	}
	if node.Tag == "footnote_reference" && node.truthy("auto") {
		return "References to auto-numbered and auto-symbol footnotes"
	}
	if len(node.list("names")) > 0 || len(node.list("ids")) > 0 {
		return "Targets (names and identifiers)"
	}
	return ""
}

func (s *rstState) directive(m lineMatch, optionPresets map[string]any) ([]*Node, bool) {
	typeName := m.Group(1)
	spec, messages := lookupDirective(typeName, s.memo, s.document)
	s.parent().extend(messages)
	if spec != nil {
		return s.runDirective(spec, m, typeName, optionPresets)
	}
	return s.unknownDirective(typeName)
}

type directiveError struct {
	level int
	msg   string
}

func (s *rstState) runDirective(spec *directiveSpec, m lineMatch, typeName string, optionPresets map[string]any) ([]*Node, bool) {
	lineno := s.sm.absLineNumber()
	initialLineOffset := s.sm.lineOffset
	indented, _, lineOffset, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, false)
	a, b := clampSlice(initialLineOffset, s.sm.lineOffset+1, len(s.sm.inputLines.data))
	blockText := strings.Join(s.sm.inputLines.data[a:b], "\n")
	var d *directive
	var merr *markupError
	func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(*markupError); ok {
					merr = e
					return
				}
				panic(r)
			}
		}()
		arguments, options, content, contentOffset := s.parseDirectiveBlock(indented, lineOffset, spec, optionPresets)
		d = &directive{name: typeName, arguments: arguments, options: options,
			content: content, lineno: lineno, contentOffset: contentOffset,
			blockText: blockText, state: s, sm: s.sm, spec: spec}
	}()
	if merr != nil {
		e := s.reporter.error(fmt.Sprintf("Error in \"%s\" directive:\n%s.", typeName, merr.msg),
			[]*Node{newTextElem("literal_block", blockText, blockText)}, atLine(lineno))
		return []*Node{e}, blankFinish
	}
	var result []*Node
	func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(*directiveError); ok {
					msgNode := s.reporter.systemMessage(e.level, e.msg, nil, atLine(lineno))
					msgNode.append(newTextElem("literal_block", blockText, blockText))
					result = []*Node{msgNode}
					return
				}
				panic(r)
			}
		}()
		result = spec.run(d)
	}()
	return result, blankFinish || s.sm.isNextLineBlank()
}

func (s *rstState) parseDirectiveBlock(indented *stringList, lineOffset int, spec *directiveSpec, optionPresets map[string]any) ([]string, map[string]any, *stringList, int) {
	if len(indented.data) > 0 && isBlank(indented.data[0]) {
		indented.trimStart(1)
		lineOffset++
	}
	for len(indented.data) > 0 && isBlank(indented.data[len(indented.data)-1]) {
		indented.trimEnd(1)
	}
	var argBlock, content *stringList
	var contentOffset int
	i := 0
	hasArgs := spec.requiredArgs > 0 || spec.optionalArgs > 0
	if len(indented.data) > 0 && (hasArgs || spec.optionSpec != nil) {
		for i = 0; i < len(indented.data); i++ {
			if isBlank(indented.data[i]) {
				break
			}
		}
		argBlock = indented.slice(0, i)
		content = indented.from(i + 1)
		contentOffset = lineOffset + i + 1
	} else {
		content = indented
		contentOffset = lineOffset
		argBlock = nil
	}
	var options map[string]any
	if spec.optionSpec != nil {
		options, argBlock = s.parseDirectiveOptions(optionPresets, spec.optionSpec, argBlock)
	} else {
		options = map[string]any{}
	}
	if argBlock != nil && len(argBlock.data) > 0 && !hasArgs {
		// content = arg_block + indented[i:]
		rest := indented.from(i)
		content = &stringList{data: append(append([]string{}, argBlock.data...), rest.data...),
			items: append(append([]lineInfo{}, argBlock.items...), rest.items...)}
		contentOffset = lineOffset
		argBlock = nil
	}
	for len(content.data) > 0 && isBlank(content.data[0]) {
		content.trimStart(1)
		contentOffset++
	}
	var arguments []string
	if hasArgs {
		arguments = s.parseDirectiveArguments(spec, argBlock)
	}
	if len(content.data) > 0 && !spec.hasContent {
		panic(&markupError{"no content permitted"})
	}
	return arguments, options, content, contentOffset
}

func (s *rstState) parseDirectiveOptions(optionPresets map[string]any, optionSpec map[string]optConv, argBlock *stringList) (map[string]any, *stringList) {
	options := map[string]any{}
	for k, v := range optionPresets {
		options[k] = v
	}
	var optBlock *stringList
	if argBlock != nil {
		for i, line := range argBlock.data {
			if _, ok := scanFieldMarker(line); ok {
				optBlock = argBlock.from(i)
				argBlock = argBlock.slice(0, i)
				break
			}
		}
	}
	if optBlock != nil && len(optBlock.data) > 0 {
		data, errStr := s.parseExtensionOptions(optionSpec, optBlock)
		if errStr != "" {
			panic(&markupError{errStr})
		}
		for k, v := range data {
			options[k] = v
		}
	}
	return options, argBlock
}

func (s *rstState) parseDirectiveArguments(spec *directiveSpec, argBlock *stringList) []string {
	required := spec.requiredArgs
	optional := spec.optionalArgs
	argText := ""
	if argBlock != nil {
		argText = argBlock.join()
	}
	arguments := pySplit(argText)
	if len(arguments) < required {
		panic(&markupError{fmt.Sprintf("%d argument(s) required, %d supplied", required, len(arguments))})
	} else if len(arguments) > required+optional {
		if spec.finalArgWS {
			arguments = pySplitN(argText, required+optional-1)
		} else {
			panic(&markupError{fmt.Sprintf("maximum %d argument(s) allowed, %d supplied", required+optional, len(arguments))})
		}
	}
	return arguments
}

func (s *rstState) parseExtensionOptions(optionSpec map[string]optConv, datalines *stringList) (map[string]any, string) {
	node := newElem("field_list", "")
	newlineOffset, blankFinish := s.nestedListParse(datalines, 0, node, "ExtensionOptions", true, nestedListOpts{})
	if newlineOffset != len(datalines.data) {
		return nil, "invalid option block"
	}
	options, errStr := extractExtensionOptions(node, optionSpec)
	if errStr != "" {
		return nil, errStr
	}
	if blankFinish {
		return options, ""
	}
	return nil, "option data incompletely parsed"
}

// extractExtensionOptions mirrors utils.extract_extension_options; the
// returned string is the error text ("" on success).
func extractExtensionOptions(fieldList *Node, spec map[string]optConv) (map[string]any, string) {
	type opt struct {
		name   string
		data   string
		isNone bool
	}
	var optionList []opt
	for _, field := range fieldList.Children {
		if len(pySplit(field.Children[0].astext())) != 1 {
			return nil, "invalid option data: extension option field name may not contain multiple words"
		}
		name := pyLower(field.Children[0].astext())
		body := field.Children[1]
		o := opt{name: name}
		if len(body.Children) == 0 {
			o.isNone = true
		} else if len(body.Children) > 1 || body.Children[0].Tag != "paragraph" ||
			len(body.Children[0].Children) != 1 || !body.Children[0].Children[0].IsText() {
			return nil, fmt.Sprintf("invalid option data: extension option field body may contain\na single paragraph only (option \"%s\")", name)
		} else {
			o.data = body.Children[0].Children[0].astext()
		}
		optionList = append(optionList, o)
	}
	options := map[string]any{}
	for _, o := range optionList {
		conv, ok := spec[o.name]
		if !ok || conv == nil {
			return nil, fmt.Sprintf(`unknown option: "%s"`, o.name)
		}
		if _, dup := options[o.name]; dup {
			return nil, fmt.Sprintf(`invalid option data: duplicate option "%s"`, o.name)
		}
		v, err := conv(o.data, o.isNone)
		if err != nil {
			val := pyStrRepr(o.data)
			if o.isNone {
				val = "None"
			}
			return nil, fmt.Sprintf("invalid option value: (option: \"%s\"; value: %s)\n%s", o.name, val, err.Error())
		}
		options[o.name] = v
	}
	return options, ""
}

func (s *rstState) unknownDirective(typeName string) ([]*Node, bool) {
	lineno := s.sm.absLineNumber()
	indented, _, _, blankFinish := s.sm.getFirstKnownIndented(0, false, false, true)
	text := indented.join()
	e := s.reporter.error(fmt.Sprintf(`Unknown directive type "%s".`, typeName),
		[]*Node{newTextElem("literal_block", text, text)}, atLine(lineno))
	return []*Node{e}, blankFinish
}

func (s *rstState) comment(m lineMatch) ([]*Node, bool) {
	if s.sm.isNextLineBlank() {
		first := m.s[m.End(0):]
		if isBlank(first) {
			return []*Node{newElem("comment", "")}, true
		}
		if strings.HasPrefix(first, `end of inclusion from "`) {
			if n := len(s.document.includeLog); n > 0 {
				s.document.includeLog = s.document.includeLog[:n-1]
			}
			return nil, true
		}
	}
	indented, _, _, blankFinish := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
	for len(indented.data) > 0 && isBlank(indented.data[len(indented.data)-1]) {
		indented.trimEnd(1)
	}
	text := indented.join()
	return []*Node{newTextElem("comment", text, text)}, blankFinish
}

func bodyExplicitMarkup(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	nodelist, blankFinish := s.explicitConstruct(m)
	s.parent().extend(nodelist)
	s.explicitList(blankFinish)
	return []string{}, nextState
}

func (s *rstState) explicitConstruct(m lineMatch) ([]*Node, bool) {
	var errors []*Node
	for _, c := range explicitConstructs {
		expmatch, ok := c.scan(m.s)
		if !ok {
			continue
		}
		var nodelist []*Node
		var blankFinish bool
		var merr *markupError
		func() {
			defer func() {
				if r := recover(); r != nil {
					if e, ok := r.(*markupError); ok {
						merr = e
						return
					}
					panic(r)
				}
			}()
			nodelist, blankFinish = c.method(s, expmatch)
		}()
		if merr == nil {
			return nodelist, blankFinish
		}
		lineno := s.sm.absLineNumber()
		errors = append(errors, s.reporter.warning(merr.msg, nil, atLine(lineno)))
		break
	}
	nodelist, blankFinish := s.comment(m)
	return append(nodelist, errors...), blankFinish
}

func (s *rstState) explicitList(blankFinish bool) {
	offset := s.sm.lineOffset + 1
	newlineOffset, blankFinish := s.nestedListParse(s.sm.inputLines.view(offset),
		s.sm.absLineOffset()+1, s.parent(), "Explicit", blankFinish, nestedListOpts{})
	s.gotoLine(newlineOffset)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Explicit markup"))
	}
}

func bodyAnonymous(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	nodelist, blankFinish := s.anonymousTarget(m)
	s.parent().extend(nodelist)
	s.explicitList(blankFinish)
	return []string{}, nextState
}

func (s *rstState) anonymousTarget(m lineMatch) ([]*Node, bool) {
	lineno := s.sm.absLineNumber()
	sl, _, _, blankFinish := s.sm.getFirstKnownIndented(m.End(0), true, true, true)
	blocktext := m.s[:m.End(0)] + sl.join()
	block := make([]string, len(sl.data))
	for i, line := range sl.data {
		block[i] = escape2null(line)
	}
	target := s.makeTarget(block, blocktext, lineno, "")
	return []*Node{target}, blankFinish
}

func bodyLine(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	if s.sm.matchTitles {
		return s.sm.context1(m.s), "Line"
	} else if pyStrip(m.s) == "::" {
		panic(&transitionCorrection{"text"})
	} else if len(pyStrip(m.s)) < 4 {
		msg := s.reporter.info("Unexpected possible title overline or transition.\n"+
			"Treating it as ordinary text because it's so short.", nil, atLine(s.sm.absLineNumber()))
		s.parent().append(msg)
		panic(&transitionCorrection{"text"})
	}
	blocktext := s.sm.line
	msg := s.reporter.error("Unexpected section title or transition.",
		[]*Node{newTextElem("literal_block", blocktext, blocktext)}, atLine(s.sm.absLineNumber()))
	s.parent().append(msg)
	return []string{}, nextState
}

func bodyText(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	return s.sm.context1(m.s), "Text"
}

// ---------------------------------------------------------------- specialized

func bulletListBullet(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	_, w := firstRune(m.s)
	if m.s[:w] != s.parent().str("bullet") {
		invalidInput(s, m, context, nextState)
	}
	listitem, blankFinish := s.listItem(m.End(0))
	s.parent().append(listitem)
	s.blankFinish = blankFinish
	return []string{}, nextState
}

func definitionListText(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	return s.sm.context1(m.s), "Definition"
}

func enumeratedListEnumerator(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	format, sequence, _, ordinal, okOrd := s.parseEnumerator(m, s.parent().str("enumtype"))
	if format != s.format ||
		(sequence != "#" && (sequence != s.parent().str("enumtype") || s.auto || !okOrd || ordinal != s.lastordinal+1)) ||
		!s.isEnumeratedListItem(ordinal, okOrd, sequence, format) {
		invalidInput(s, m, context, nextState)
	}
	if sequence == "#" {
		s.auto = true
	}
	listitem, blankFinish := s.listItem(m.End(0))
	s.parent().append(listitem)
	s.blankFinish = blankFinish
	s.lastordinal = ordinal
	return []string{}, nextState
}

func fieldListFieldMarker(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	field, blankFinish := s.field(m)
	s.parent().append(field)
	s.blankFinish = blankFinish
	return []string{}, nextState
}

func optionListOptionMarker(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	item, blankFinish, err := s.optionListItem(m)
	if err != nil {
		invalidInput(s, m, context, nextState)
	}
	s.parent().append(item)
	s.blankFinish = blankFinish
	return []string{}, nextState
}

func lineBlockLineBlock(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	lineno := s.sm.absLineNumber()
	line, messages, blankFinish := s.lineBlockLine(m, lineno)
	s.parent().append(line)
	s.parent().Parent.extend(messages)
	s.blankFinish = blankFinish
	return []string{}, nextState
}

func explicitExplicitMarkup(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	nodelist, blankFinish := s.explicitConstruct(m)
	s.parent().extend(nodelist)
	s.blankFinish = blankFinish
	return []string{}, nextState
}

func explicitAnonymous(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	nodelist, blankFinish := s.anonymousTarget(m)
	s.parent().extend(nodelist)
	s.blankFinish = blankFinish
	return []string{}, nextState
}

func substEmbeddedDirective(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	nodelist, blankFinish := s.directive(m, map[string]any{"alt": s.parent().list("names")[0]})
	s.parent().extend(nodelist)
	if !s.sm.atEOF() {
		s.blankFinish = blankFinish
	}
	panic(smEOF{})
}

func substText(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	if !s.sm.atEOF() {
		s.blankFinish = s.sm.isNextLineBlank()
	}
	panic(smEOF{})
}

// ---------------------------------------------------------------- Text

func textBlank(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	paragraph, messages, literalnext := s.paragraph(context, s.sm.absLineNumber()-1)
	s.parent().append(paragraph)
	s.parent().extend(messages)
	if literalnext {
		s.parent().extend(s.literalBlock())
	}
	return []string{}, "Body"
}

func textIndent(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	dl := newElem("definition_list", "")
	lineno := s.sm.absLineNumber() - 1
	dl.setSrcLine(s.sm.getSourceAndLine(lineno, true))
	dlItem, blankFinish := s.definitionListItem(context)
	dl.append(dlItem)
	s.parent().append(dl)
	offset := s.sm.lineOffset + 1
	newlineOffset, blankFinish := s.nestedListParse(s.sm.inputLines.view(offset),
		s.sm.absLineOffset()+1, dl, "DefinitionList", blankFinish,
		nestedListOpts{blankFinishState: "Definition"})
	s.gotoLine(newlineOffset)
	if !blankFinish {
		s.parent().append(s.unindentWarning("Definition list"))
	}
	return []string{}, "Body"
}

func textUnderline(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	lineno := s.sm.absLineNumber()
	title := pyRstrip(context[0])
	underline := pyRstrip(m.s)
	source := title + "\n" + underline
	var messages []*Node
	if columnWidth(title) > len(underline) {
		if len(underline) < 4 {
			if s.sm.matchTitles {
				msg := s.reporter.info("Possible title underline, too short for the title.\n"+
					"Treating it as ordinary text because it's so short.", nil, atLine(lineno))
				s.parent().append(msg)
			}
			panic(&transitionCorrection{"text"})
		}
		blocktext := context[0] + "\n" + s.sm.line
		msg := s.reporter.warning("Title underline too short.",
			[]*Node{newTextElem("literal_block", blocktext, blocktext)}, atLine(lineno))
		messages = append(messages, msg)
	}
	if !s.sm.matchTitles {
		blocktext := context[0] + "\n" + s.sm.line
		src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
		msg := s.reporter.error("Unexpected section title.",
			[]*Node{newTextElem("literal_block", blocktext, blocktext)},
			atSrcLine(src, hasSrc, srcline, hasLine))
		s.parent().extend(messages)
		s.parent().append(msg)
		return []string{}, nextState
	}
	style := underline[:1]
	s.section(title, source, style, lineno-1, messages)
	return []string{}, nextState
}

func textText(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	startline := s.sm.absLineNumber() - 1
	var msg *Node
	block, uerr := s.sm.getTextBlock(true)
	if uerr != nil {
		msg = s.reporter.error("Unexpected indentation.", nil,
			atSrcLine(uerr.info.source, uerr.info.hasSource, uerr.line, true))
	}
	lines := append(append([]string{}, context...), block.data...)
	paragraph, messages, literalnext := s.paragraph(lines, startline)
	s.parent().append(paragraph)
	s.parent().extend(messages)
	s.parent().append(msg)
	if literalnext {
		s.sm.nextLine(1)
		s.parent().extend(s.literalBlock())
	}
	return []string{}, nextState
}

func (s *rstState) literalBlock() []*Node {
	indented, _, offset, blankFinish := s.sm.getIndented(false, true)
	for len(indented.data) > 0 && isBlank(indented.data[len(indented.data)-1]) {
		indented.trimEnd(1)
	}
	if len(indented.data) == 0 {
		return s.quotedLiteralBlock()
	}
	data := indented.join()
	lb := newTextElem("literal_block", data, data)
	lb.setSrcLine(s.sm.getSourceAndLine(offset+1, true))
	nodelist := []*Node{lb}
	if !blankFinish {
		nodelist = append(nodelist, s.unindentWarning("Literal block"))
	}
	return nodelist
}

func (s *rstState) quotedLiteralBlock() []*Node {
	absLineOffset := s.sm.absLineOffset()
	offset := s.sm.lineOffset
	parentNode := newElem("Element", "")
	newAbsOffset := s.nestedParseWith(s.sm.inputLines.view(offset), absLineOffset, parentNode, false, "QuotedLiteralBlock")
	s.gotoLine(newAbsOffset)
	return parentNode.Children
}

func (s *rstState) definitionListItem(termline []string) (*Node, bool) {
	ddLineno := s.sm.absLineNumber()
	dtLineno := ddLineno - 1
	indented, _, lineOffset, blankFinish := s.sm.getIndented(false, true)
	dlItem := newElem("definition_list_item", strings.Join(append(append([]string{}, termline...), indented.data...), "\n"))
	dlItem.setSrcLine(s.sm.getSourceAndLine(dtLineno, true))
	dtNodes, messages := s.term(termline, dtLineno)
	dlItem.extend(dtNodes)
	dd := newElem("definition", "", messages...)
	dd.setSrcLine(s.sm.getSourceAndLine(ddLineno, true))
	dlItem.append(dd)
	if strings.HasSuffix(termline[0], "::") {
		dd.append(s.reporter.info("Blank line missing before literal block (after the \"::\")? "+
			"Interpreted as a definition list item.", nil, atLine(ddLineno)))
	}
	s.nestedParse(indented, lineOffset, dd, false)
	return dlItem, blankFinish
}

func (s *rstState) term(lines []string, lineno int) ([]*Node, []*Node) {
	textNodes, messages := s.inlineText(lines[0], lineno)
	dt := newElem("term", lines[0])
	dt.setSrcLine(s.sm.getSourceAndLine(lineno, true))
	nodeList := []*Node{dt}
	for _, node := range textNodes {
		last := nodeList[len(nodeList)-1]
		if node.IsText() {
			parts := splitClassifiers(node.Text)
			if len(parts) == 1 {
				last.append(node)
			} else {
				text := pyRstrip(parts[0])
				last.append(newText(text))
				for _, part := range parts[1:] {
					nodeList = append(nodeList, newTextElem("classifier", unescape(part, true), part))
				}
			}
		} else {
			last.append(node)
		}
	}
	return nodeList, messages
}

func definitionIndent(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	dlItem, blankFinish := s.definitionListItem(context)
	s.parent().append(dlItem)
	s.blankFinish = blankFinish
	return []string{}, "DefinitionList"
}

// ---------------------------------------------------------------- Line

func lineEOF(s *rstState, context []string) {
	marker := pyStrip(context[0])
	if len(marker) < 4 {
		s.stateCorrection(1)
	}
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	transition := newElem("transition", context[0])
	transition.setSrcLine(src, hasSrc, srcline-1, hasLine)
	s.parent().append(transition)
}

func lineBlank(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	src, hasSrc, srcline, hasLine := s.sm.curSrcLine()
	marker := pyStrip(context[0])
	if len(marker) < 4 {
		s.stateCorrection(1)
	}
	transition := newElem("transition", marker)
	transition.setSrcLine(src, hasSrc, srcline-1, hasLine)
	s.parent().append(transition)
	return []string{}, "Body"
}

func lineText(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	lineno := s.sm.absLineNumber() - 1
	overline := context[0]
	title := m.s
	underline, ok := s.sm.nextLine(1)
	if !ok {
		blocktext := overline + "\n" + title
		if len(pyRstrip(overline)) < 4 {
			s.shortOverline(lineno, 2)
		} else {
			msg := s.reporter.error("Incomplete section title.",
				[]*Node{newTextElem("literal_block", blocktext, blocktext)}, atLine(lineno))
			s.parent().append(msg)
			return []string{}, "Body"
		}
	}
	source := overline + "\n" + title + "\n" + underline
	overline = pyRstrip(overline)
	underline = pyRstrip(underline)
	if _, ok := scanLine(underline); !ok {
		if len(pyRstrip(overline)) < 4 {
			s.shortOverline(lineno, 2)
		} else {
			msg := s.reporter.error("Missing matching underline for section title overline.",
				[]*Node{newTextElem("literal_block", source, source)}, atLine(lineno))
			s.parent().append(msg)
			return []string{}, "Body"
		}
	} else if overline != underline {
		if len(pyRstrip(overline)) < 4 {
			s.shortOverline(lineno, 2)
		} else {
			msg := s.reporter.error("Title overline & underline mismatch.",
				[]*Node{newTextElem("literal_block", source, source)}, atLine(lineno))
			s.parent().append(msg)
			return []string{}, "Body"
		}
	}
	title = pyRstrip(title)
	var messages []*Node
	if columnWidth(title) > len(overline) {
		if len(pyRstrip(overline)) < 4 {
			s.shortOverline(lineno, 2)
		} else {
			msg := s.reporter.warning("Title overline too short.",
				[]*Node{newTextElem("literal_block", source, source)}, atLine(lineno))
			messages = append(messages, msg)
		}
	}
	style := overline[:1] + "/" + underline[:1]
	s.section(pyLstrip(title), source, style, lineno+1, messages)
	return []string{}, "Body"
}

func lineUnderline(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	overline := context[0]
	blocktext := overline + "\n" + s.sm.line
	lineno := s.sm.absLineNumber() - 1
	if len(pyRstrip(overline)) < 4 {
		s.shortOverline(lineno, 1)
	}
	msg := s.reporter.error("Invalid section title or transition marker.",
		[]*Node{newTextElem("literal_block", blocktext, blocktext)}, atLine(lineno))
	s.parent().append(msg)
	return []string{}, "Body"
}

func (s *rstState) shortOverline(lineno, lines int) {
	msg := s.reporter.info("Possible incomplete section title.\nTreating the overline as "+
		"ordinary text because it's so short.", nil, atLine(lineno))
	s.parent().append(msg)
	s.stateCorrection(lines)
}

func (s *rstState) stateCorrection(lines int) {
	s.sm.previousLine(lines)
	panic(&stateCorrection{"Body", "text"})
}

// ---------------------------------------------------------------- QuotedLiteralBlock

func quotedBlank(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	if len(context) > 0 {
		panic(smEOF{})
	}
	return context, nextState
}

func quotedEOF(s *rstState, context []string) {
	if len(context) > 0 {
		src, hasSrc, srcline, hasLine := s.sm.getSourceAndLine(s.initialLineno, true)
		text := strings.Join(context, "\n")
		lb := newTextElem("literal_block", text, text)
		lb.setSrcLine(src, hasSrc, srcline, hasLine)
		s.parent().append(lb)
	} else {
		s.parent().append(s.reporter.warning("Literal block expected; none found.", nil,
			atLine(s.sm.absLineNumber())))
		s.sm.previousLine(1)
	}
	s.parent().extend(s.messages)
}

func quotedIndent(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	s.messages = append(s.messages, s.reporter.error("Unexpected indentation.", nil,
		atLine(s.sm.absLineNumber())))
	s.sm.previousLine(1)
	panic(smEOF{})
}

func quotedInitialQuoted(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	s.removeTransition("initial_quoted")
	quote := m.s[:1]
	s.addTransition(&transition{"quoted", func(line string) (lineMatch, bool) {
		if !strings.HasPrefix(line, quote) {
			return lineMatch{}, false
		}
		return lm(line, 1), true
	}, quotedQuoted, s.kind})
	s.initialLineno = s.sm.absLineNumber()
	return []string{m.s}, nextState
}

func quotedQuoted(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	return append(context, m.s), nextState
}

func quotedText(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	if len(context) > 0 {
		s.messages = append(s.messages, s.reporter.error("Inconsistent literal block quoting.", nil,
			atLine(s.sm.absLineNumber())))
		s.sm.previousLine(1)
	}
	panic(smEOF{})
}

// ---------------------------------------------------------------- helpers

// escape2null mirrors utils.escape2null.
func escape2null(text string) string {
	if strings.IndexByte(text, '\\') < 0 {
		return text
	}
	var b strings.Builder
	start := 0
	for {
		i := strings.IndexByte(text[start:], '\\')
		if i < 0 {
			b.WriteString(text[start:])
			return b.String()
		}
		i += start
		b.WriteString(text[start:i])
		b.WriteByte(0)
		// append the following character (one code point)
		j := i + 1
		if j < len(text) {
			_, w := firstRune(text[j:])
			b.WriteString(text[j : j+w])
			j += w
		}
		start = j
		if start >= len(text) {
			return b.String()
		}
	}
}

func splitEscapedWhitespace(text string) []string {
	var out []string
	for _, s := range strings.Split(text, "\x00 ") {
		out = append(out, strings.Split(s, "\x00\n")...)
	}
	return out
}

// ---------------------------------------------------------------- roman numerals

var romanPrefixes = []struct {
	value int
	name  string
}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"},
	{50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}}

func toRoman(n int) (string, bool) {
	if n < 1 || n > 4999 {
		return "", false
	}
	var b strings.Builder
	for _, p := range romanPrefixes {
		for n >= p.value {
			b.WriteString(p.name)
			n -= p.value
		}
	}
	return b.String(), true
}

// romanFromString parses a (strict) roman numeral in a single case.
func romanFromString(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	u := strings.ToUpper(s)
	if s != u && s != strings.ToLower(s) {
		return 0, false
	}
	vals := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total := 0
	for i := 0; i < len(u); i++ {
		v, ok := vals[u[i]]
		if !ok {
			return 0, false
		}
		if i+1 < len(u) && vals[u[i+1]] > v {
			total -= v
		} else {
			total += v
		}
	}
	// strictness: must round-trip
	r, ok := toRoman(total)
	if !ok || r != u {
		return 0, false
	}
	return total, true
}
