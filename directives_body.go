package rstgo

// Ports of docutils.parsers.rst.directives.{admonitions,body,images,parts,
// references}.

import (
	"fmt"
	"net/url"
	"strings"
)

var (
	subStructuralTags = set("title subtitle meta docinfo decoration transition")
	decorativeTags    = set("header footer")
	partTags          = set(`attribution list_item term classifier definition
		definition_list_item field_name field_body field option_string
		option_argument option option_group description option_list_item line
		label caption legend entry row colspec thead tbody tgroup`)
)

// invalidPseudoSectionParent mirrors the check in BasePseudoSection.run.
func invalidPseudoSectionParent(n *Node) bool {
	if n.is("document", "section", "sidebar") {
		return false
	}
	return subStructuralTags[n.Tag] || bibliographicTags[n.Tag] || decorativeTags[n.Tag] ||
		bodyTags[n.Tag] || partTags[n.Tag] || n.Tag == "topic"
}

var classNameSpec = map[string]optConv{"class": optClass, "name": optUnchanged}

func init() {
	for _, name := range []string{"attention", "caution", "danger", "error",
		"hint", "important", "note", "tip", "warning"} {
		tag := name
		directiveRegistry[name] = &directiveSpec{finalArgWS: true, optionSpec: classNameSpec,
			hasContent: true, run: func(d *directive) []*Node { return runAdmonition(d, tag) }}
	}
	directiveRegistry["admonition"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: classNameSpec, hasContent: true,
		run: func(d *directive) []*Node { return runAdmonition(d, "admonition") }}
	directiveRegistry["topic"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: classNameSpec, hasContent: true,
		run: func(d *directive) []*Node { return runPseudoSection(d, "topic") }}
	directiveRegistry["sidebar"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: mergeSpec(classNameSpec, map[string]optConv{"subtitle": optUnchangedRequired}),
		hasContent: true, run: runSidebar}
	directiveRegistry["line-block"] = &directiveSpec{optionSpec: classNameSpec, hasContent: true, run: runLineBlock}
	directiveRegistry["parsed-literal"] = &directiveSpec{optionSpec: classNameSpec, hasContent: true, run: runParsedLiteral}
	directiveRegistry["code"] = &directiveSpec{optionalArgs: 1,
		optionSpec: mergeSpec(classNameSpec, map[string]optConv{"number-lines": optNumberLines}),
		hasContent: true, run: runCodeBlock}
	directiveRegistry["math"] = &directiveSpec{optionSpec: classNameSpec, hasContent: true, run: runMathBlock}
	directiveRegistry["rubric"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: classNameSpec, run: runRubric}
	for name := range map[string]bool{"epigraph": true, "highlights": true, "pull-quote": true} {
		cls := name
		directiveRegistry[name] = &directiveSpec{hasContent: true,
			run: func(d *directive) []*Node { return runBlockQuote(d, cls) }}
	}
	directiveRegistry["compound"] = &directiveSpec{optionSpec: classNameSpec, hasContent: true, run: runCompound}
	directiveRegistry["container"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{"name": optUnchanged}, hasContent: true, run: runContainer}

	imageSpec := map[string]optConv{
		"alt": optUnchanged, "height": optLengthOrUnitless,
		"width": optLengthOrPercentageOrUnitless, "scale": optPercentage,
		"align":  optChoice("top", "middle", "bottom", "left", "center", "right"),
		"target": optUnchangedRequired, "loading": optChoice("embed", "link", "lazy"),
		"class": optClass, "name": optUnchanged,
	}
	directiveRegistry["image"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: imageSpec, run: runImage}
	directiveRegistry["figure"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: mergeSpec(imageSpec, map[string]optConv{
			"figwidth": func(arg string, isNone bool) (any, error) {
				if isNone {
					return nil, fmt.Errorf("'NoneType' object has no attribute 'lower'")
				}
				if strings.ToLower(arg) == "image" {
					return "image", nil
				}
				return lengthOrPercentageOrUnitless(arg, false, "px")
			},
			"figclass": optClass, "figname": optUnchanged,
			"align": optChoice("left", "center", "right"),
		}), hasContent: true, run: runFigure}

	directiveRegistry["contents"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{"depth": optNonnegativeInt, "local": optFlag,
			"backlinks": func(arg string, isNone bool) (any, error) {
				v, err := choice(arg, isNone, []string{"top", "entry", "none"})
				if err != nil {
					return nil, err
				}
				if v == "none" {
					return nil, nil
				}
				return v, nil
			}, "class": optClass}, run: runContents}
	directiveRegistry["sectnum"] = &directiveSpec{
		optionSpec: map[string]optConv{"depth": optInt, "start": optInt,
			"prefix": optUnchangedRequired, "suffix": optUnchangedRequired},
		run: func(d *directive) []*Node {
			p := newElem("pending", "")
			p.ext().pending = &pendingInfo{transform: "docutils.transforms.parts.SectNum", details: copyOptions(d.options)}
			d.sm.document.notePending(p, 0)
			return []*Node{p}
		}}
	directiveRegistry["header"] = &directiveSpec{hasContent: true, run: func(d *directive) []*Node {
		d.assertHasContent()
		header := decorationHeader(d.sm.document.getDecoration())
		d.state.nestedParse(d.content, d.contentOffset, header, false)
		return nil
	}}
	directiveRegistry["footer"] = &directiveSpec{hasContent: true, run: func(d *directive) []*Node {
		d.assertHasContent()
		footer := decorationFooter(d.sm.document.getDecoration())
		d.state.nestedParse(d.content, d.contentOffset, footer, false)
		return nil
	}}
	directiveRegistry["target-notes"] = &directiveSpec{
		optionSpec: map[string]optConv{"class": optClass},
		run: func(d *directive) []*Node {
			p := newElem("pending", "")
			p.ext().pending = &pendingInfo{transform: "docutils.transforms.references.TargetNotes"}
			d.addName(p)
			p.ext().pending.details = copyOptions(d.options)
			d.sm.document.notePending(p, 0)
			return []*Node{p}
		}}
}

func copyOptions(o map[string]any) map[string]any {
	m := map[string]any{}
	for k, v := range o {
		m[k] = v
	}
	return m
}

func runAdmonition(d *directive, tag string) []*Node {
	options := d.normalizedOptions()
	d.assertHasContent()
	text := d.content.join()
	node := newElem(tag, text)
	applyOptions(node, options)
	d.addName(node)
	node.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	if tag == "admonition" {
		titleText := d.arguments[0]
		textnodes, messages := d.state.inlineText(titleText, d.lineno)
		title := newTextElem("title", titleText, "", textnodes...)
		title.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
		node.append(title)
		node.extend(messages)
		if _, ok := options["classes"]; !ok {
			node.appendTo("classes", "admonition-"+makeID(titleText))
		}
	}
	d.state.nestedParse(d.content, d.contentOffset, node, false)
	return []*Node{node}
}

func runPseudoSection(d *directive, tag string) []*Node {
	if invalidPseudoSectionParent(d.sm.node) {
		d.fail(3, fmt.Sprintf(`The "%s" directive may not be used within topics or body elements.`, d.name))
	}
	d.assertHasContent()
	var titles, messages []*Node
	if len(d.arguments) > 0 {
		titleText := d.arguments[0]
		var textnodes []*Node
		textnodes, messages = d.state.inlineText(titleText, d.lineno)
		titles = []*Node{newTextElem("title", titleText, "", textnodes...)}
		if st, ok := d.options["subtitle"]; ok {
			textnodes, more := d.state.inlineText(st.(string), d.lineno)
			titles = append(titles, newTextElem("subtitle", st.(string), "", textnodes...))
			messages = append(messages, more...)
		}
	}
	text := d.content.join()
	node := newElem(tag, text, append(titles, messages...)...)
	node.appendTo("classes", d.optClasses()...)
	node.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	d.addName(node)
	if text != "" {
		d.state.nestedParse(d.content, d.contentOffset, node, false)
	}
	return []*Node{node}
}

func runSidebar(d *directive) []*Node {
	if d.sm.node.Tag == "sidebar" {
		d.fail(3, fmt.Sprintf(`The "%s" directive may not be used within a sidebar element.`, d.name))
	}
	if d.hasOpt("subtitle") && len(d.arguments) == 0 {
		d.fail(3, `The "subtitle" option may not be used without a title.`)
	}
	return runPseudoSection(d, "sidebar")
}

func runLineBlock(d *directive) []*Node {
	d.assertHasContent()
	block := newElem("line_block", "")
	block.appendTo("classes", d.optClasses()...)
	block.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	d.addName(block)
	nodeList := []*Node{block}
	for i, lineText := range d.content.data {
		textNodes, messages := d.state.inlineText(pyStrip(lineText), d.lineno+d.contentOffset)
		line := newTextElem("line", lineText, "", textNodes...)
		line.setSrcLine(block.Source, block.hasSrc, block.Line+i, block.hasLine)
		if pyStrip(lineText) != "" {
			line.setIndent(len(lineText) - len(pyLstrip(lineText)))
		}
		block.append(line)
		nodeList = append(nodeList, messages...)
		d.contentOffset++
	}
	if len(block.Children) > 0 {
		if !block.Children[0].hasIndent() {
			// Python would raise comparing None; treat as 0
			block.Children[0].setIndent(0)
		}
		nestLineBlockLines(block)
	}
	return nodeList
}

func runParsedLiteral(d *directive) []*Node {
	options := d.normalizedOptions()
	d.assertHasContent()
	text := d.content.join()
	textNodes, messages := d.state.inlineText(text, d.lineno)
	node := newTextElem("literal_block", text, "", textNodes...)
	applyOptions(node, options)
	node.setLine(d.contentOffset+1, true)
	d.addName(node)
	return append([]*Node{node}, messages...)
}

type codeToken struct {
	classes []string
	value   string
}

// numberLines mirrors utils.code_analyzer.NumberLines.
func numberLines(tokens []codeToken, startline, endline int) []codeToken {
	width := len(fmt.Sprint(endline))
	f := func(n int) string { return fmt.Sprintf("%*d ", width, n) }
	lineno := startline
	out := []codeToken{{[]string{"ln"}, f(lineno)}}
	for _, t := range tokens {
		lines := strings.Split(t.value, "\n")
		for _, line := range lines[:len(lines)-1] {
			out = append(out, codeToken{t.classes, line + "\n"})
			lineno++
			out = append(out, codeToken{[]string{"ln"}, f(lineno)})
		}
		out = append(out, codeToken{t.classes, lines[len(lines)-1]})
	}
	return out
}

func runCodeBlock(d *directive) []*Node {
	d.assertHasContent()
	language := ""
	if len(d.arguments) > 0 {
		language = d.arguments[0]
	}
	return codeBlock(d, language, d.content.data)
}

func codeBlock(d *directive, language string, content []string) []*Node {
	options := d.normalizedOptions()
	classes := []string{"code"}
	if language != "" {
		classes = append(classes, language)
	}
	if c, ok := options["classes"]; ok {
		classes = append(classes, c.([]string)...)
	}
	st := d.state.document.settings
	code := strings.Join(content, "\n")
	if lexerFails(language, st.syntaxHighlight) {
		if st.reportLevel <= 2 {
			d.fail(2, pygmentsMissing)
		}
	}
	tokens := []codeToken{{nil, code}}
	if _, ok := options["number-lines"]; ok {
		startline := 1
		if v, ok := d.options["number-lines"].(int); ok {
			startline = v
		}
		endline := startline + len(content)
		tokens = numberLines(tokens, startline, endline)
	}
	node := newElem("literal_block", code)
	node.setAttr("classes", classes)
	d.addName(node)
	if src, ok := options["source"]; ok {
		node.setAttr("source", src)
	}
	for _, t := range tokens {
		if len(t.classes) > 0 {
			in := newTextElem("inline", t.value, t.value)
			in.setAttr("classes", append([]string{}, t.classes...))
			node.append(in)
		} else {
			node.append(newText(t.value))
		}
	}
	return []*Node{node}
}

func runMathBlock(d *directive) []*Node {
	options := d.normalizedOptions()
	d.assertHasContent()
	var out []*Node
	for _, block := range strings.Split(d.content.join(), "\n\n") {
		if block == "" {
			continue
		}
		node := newTextElem("math_block", d.blockText, block)
		applyOptions(node, options)
		node.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
		d.addName(node)
		out = append(out, node)
	}
	return out
}

func runRubric(d *directive) []*Node {
	options := d.normalizedOptions()
	rubricText := d.arguments[0]
	textnodes, messages := d.state.inlineText(rubricText, d.lineno)
	rubric := newTextElem("rubric", rubricText, "", textnodes...)
	applyOptions(rubric, options)
	rubric.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	d.addName(rubric)
	return append([]*Node{rubric}, messages...)
}

func runBlockQuote(d *directive, class string) []*Node {
	d.assertHasContent()
	elements := d.state.blockQuote(d.content, d.contentOffset)
	for _, e := range elements {
		if e.Tag == "block_quote" {
			e.appendTo("classes", class)
		}
	}
	return elements
}

func runCompound(d *directive) []*Node {
	d.assertHasContent()
	node := newElem("compound", d.content.join())
	node.appendTo("classes", d.optClasses()...)
	node.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	d.addName(node)
	d.state.nestedParse(d.content, d.contentOffset, node, false)
	return []*Node{node}
}

func runContainer(d *directive) []*Node {
	d.assertHasContent()
	var classes []string
	if len(d.arguments) > 0 {
		var err error
		classes, err = classOption(d.arguments[0])
		if err != nil {
			d.fail(3, fmt.Sprintf(`Invalid class attribute value for "%s" directive: "%s".`, d.name, d.arguments[0]))
		}
	}
	node := newElem("container", d.content.join())
	node.appendTo("classes", classes...)
	node.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	d.addName(node)
	d.state.nestedParse(d.content, d.contentOffset, node, false)
	return []*Node{node}
}

func runImage(d *directive) []*Node {
	if a, ok := d.options["align"]; ok {
		align := a.(string)
		if d.state.kind == "SubstitutionDef" {
			if !containsStr([]string{"top", "middle", "bottom"}, align) {
				d.fail(3, fmt.Sprintf(`Error in "%s" directive: "%s" is not a valid value `+
					`for the "align" option within a substitution `+
					`definition.  Valid values for "align" are: "%s".`, d.name, align, `top", "middle", "bottom`))
			}
		} else if !containsStr([]string{"left", "center", "right"}, align) {
			d.fail(3, fmt.Sprintf(`Error in "%s" directive: "%s" is not a valid value for `+
				`the "align" option.  Valid values for "align" are: "%s".`, d.name, align, `left", "center", "right`))
		}
	}
	reference := directiveURI(d.arguments[0])
	d.options["uri"] = reference
	var referenceNode *Node
	if t, ok := d.options["target"]; ok {
		block := pySplitlines(escape2null(t.(string)), false)
		targetType, data := d.state.parseTarget(block)
		if targetType == "refuri" {
			referenceNode = newTextElem("reference", "", "")
			referenceNode.setAttr("refuri", data)
		} else {
			referenceNode = newTextElem("reference", "", "")
			referenceNode.setAttr("refname", fullyNormalizeName(data))
			referenceNode.setAttr("name", whitespaceNormalizeName(data))
			d.state.document.noteRefname(referenceNode)
		}
		delete(d.options, "target")
	}
	options := d.normalizedOptions()
	imageNode := newElem("image", d.blockText)
	applyOptions(imageNode, options)
	imageNode.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	d.addName(imageNode)
	if referenceNode != nil {
		referenceNode.append(imageNode)
		return []*Node{referenceNode}
	}
	return []*Node{imageNode}
}

func runFigure(d *directive) []*Node {
	pop := func(k string) (any, bool) {
		v, ok := d.options[k]
		delete(d.options, k)
		return v, ok
	}
	figwidth, hasFigwidth := pop("figwidth")
	figclasses, _ := pop("figclass")
	figname, _ := pop("figname")
	align, _ := pop("align")
	imageNode := runImage(d)[0]
	figureNode := newElem("figure", "", imageNode)
	figureNode.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	if hasFigwidth && figwidth != nil {
		if figwidth.(string) == "image" {
			if d.state.document.settings.fileInsertionEnabled {
				p, err := url.PathUnescape(imageNode.str("uri"))
				if err != nil {
					p = imageNode.str("uri")
				}
				if w, _, e := readImageSize(p); e == "" {
					figureNode.setAttr("width", fmt.Sprintf("%dpx", w))
				}
			}
		} else {
			figureNode.setAttr("width", figwidth)
		}
	}
	if fc, ok := figclasses.([]string); ok && len(fc) > 0 {
		figureNode.appendTo("classes", fc...)
	}
	if fn, ok := figname.(string); ok && fn != "" {
		figureNode.appendTo("names", fullyNormalizeName(fn))
		d.state.document.noteExplicitTarget(figureNode, figureNode)
	}
	if a, ok := align.(string); ok && a != "" {
		figureNode.setAttr("align", a)
	}
	if len(d.content.data) > 0 {
		node := newElem("Element", "")
		d.state.nestedParse(d.content, d.contentOffset, node, false)
		children := append([]*Node{}, node.Children...)
		i := 0
		done := false
		for i = 0; i < len(children); i++ {
			child := children[i]
			if child.is("target", "pending") {
				figureNode.append(child)
				continue
			}
			if child.Tag == "paragraph" {
				caption := newTextElem("caption", child.Rawsource, "", child.Children...)
				caption.setSrcLine(child.Source, child.hasSrc, child.Line, child.hasLine)
				figureNode.append(caption)
				done = true
				break
			}
			if child.Tag == "comment" && len(child.Children) == 0 {
				done = true
				break
			}
			e := d.reporter().error("Figure caption must be a paragraph or empty comment.",
				d.literalBlockOfSource(), atLine(d.lineno))
			return []*Node{figureNode, e}
		}
		if !done {
			i = len(children) - 1
		}
		if len(children) > i+1 {
			figureNode.append(newElem("legend", "", children[i+1:]...))
		}
	}
	return []*Node{figureNode}
}

func runContents(d *directive) []*Node {
	if invalidPseudoSectionParent(d.sm.node) {
		d.fail(3, fmt.Sprintf(`The "%s" directive may not be used within topics or body elements.`, d.name))
	}
	document := d.sm.document
	var title *Node
	var messages []*Node
	if len(d.arguments) > 0 {
		titleText := d.arguments[0]
		var textNodes []*Node
		textNodes, messages = d.state.inlineText(titleText, d.lineno)
		title = newTextElem("title", titleText, "", textNodes...)
	} else if !d.hasOpt("local") {
		title = newTextElem("title", "", "Contents")
	}
	topic := newElem("topic", "")
	topic.appendTo("classes", "contents")
	topic.appendTo("classes", d.optClasses()...)
	src, hasSrc, line, hasLine := d.sm.curSrcLine()
	topic.setSrcLine(src, hasSrc, line-1, hasLine)
	if d.hasOpt("local") {
		topic.appendTo("classes", "local")
	}
	name := "Contents"
	if title != nil {
		name = title.astext()
		topic.append(title)
	}
	name = fullyNormalizeName(name)
	if !document.hasName(name) {
		topic.appendTo("names", name)
	}
	document.noteImplicitTarget(topic, nil)
	pending := newElem("pending", d.blockText)
	pending.ext().pending = &pendingInfo{transform: "docutils.transforms.parts.Contents", details: copyOptions(d.options)}
	document.notePending(pending, 0)
	topic.append(pending)
	return append([]*Node{topic}, messages...)
}
