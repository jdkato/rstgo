package rstgo

// Port of docutils.parsers.rst.directives.misc.

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// The standard "include" files (character entity substitution definition
// sets) shipped with Docutils; public domain.
//
//go:embed include/*.txt
var standardIncludes embed.FS

const standardIncludePrefix = "\x00std-include\x00/"

func init() {
	directiveRegistry["include"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{
			"literal": optFlag, "code": optUnchanged, "encoding": optEncoding,
			"parser": func(arg string, isNone bool) (any, error) {
				if isNone || arg == "" {
					return nil, nil
				}
				switch strings.ToLower(arg) {
				case "rst", "restructuredtext", "rest", "restx", "rtxt", "rstx":
					return "rst", nil
				}
				return nil, fmt.Errorf(`Parser "%s" not found. No module named '%s'.`, arg, arg)
			},
			"tab-width": optInt, "start-line": optInt, "end-line": optInt,
			"start-after": optUnchanged, "end-before": optUnchanged,
			"number-lines": optNumberLines, "class": optClass, "name": optUnchanged,
		}, run: runInclude}
	directiveRegistry["raw"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{"file": optPath, "url": optURI,
			"encoding": optEncoding, "class": optClass},
		hasContent: true, run: runRaw}
	directiveRegistry["replace"] = &directiveSpec{hasContent: true, run: runReplace}
	directiveRegistry["unicode"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{"trim": optFlag, "ltrim": optFlag, "rtrim": optFlag},
		run:        runUnicode}
	directiveRegistry["class"] = &directiveSpec{requiredArgs: 1, finalArgWS: true, hasContent: true, run: runClass}
	directiveRegistry["role"] = &directiveSpec{hasContent: true, run: runRole}
	directiveRegistry["default-role"] = &directiveSpec{optionalArgs: 1, run: runDefaultRole}
	directiveRegistry["title"] = &directiveSpec{requiredArgs: 1, finalArgWS: true,
		run: func(d *directive) []*Node {
			d.sm.document.Root.setAttr("title", d.arguments[0])
			return nil
		}}
	directiveRegistry["meta"] = &directiveSpec{hasContent: true, run: runMeta}
	directiveRegistry["date"] = &directiveSpec{hasContent: true, run: runDate}
	directiveRegistry["restructuredtext-test-directive"] = &directiveSpec{optionalArgs: 1, finalArgWS: true,
		optionSpec: map[string]optConv{"option": optUnchangedRequired}, hasContent: true, run: runTestDirective}
}

// relativePath mirrors utils.relative_path(None, target): the path of
// target relative to the current working directory.
func relativePath(target string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(target)
	}
	abs := target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, target)
	}
	abs = filepath.Clean(abs)
	sourceParts := strings.Split(filepath.Join(cwd, "dummy_file"), string(os.PathSeparator))
	targetParts := strings.Split(abs, string(os.PathSeparator))
	if len(sourceParts) < 2 || len(targetParts) < 2 || sourceParts[0] != targetParts[0] || sourceParts[1] != targetParts[1] {
		return strings.Join(targetParts, "/")
	}
	i := 0
	for i < len(sourceParts) && i < len(targetParts) && sourceParts[i] == targetParts[i] {
		i++
	}
	sourceParts = sourceParts[i:]
	targetParts = targetParts[i:]
	var parts []string
	for j := 0; j < len(sourceParts)-1; j++ {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts...)
	return strings.Join(parts, "/")
}

// adaptPath mirrors directives.misc.adapt_path.
func adaptPath(path, source, rootPrefix string) string {
	var base string
	if rootPrefix != "" && strings.HasPrefix(path, "/") {
		base = rootPrefix
		path = path[1:]
	} else {
		base = filepath.Dir(source)
	}
	if filepath.IsAbs(path) {
		return relativePath(path)
	}
	return relativePath(filepath.Join(base, path))
}

func ioErrorString(err error, path string) string {
	if os.IsNotExist(err) {
		return fmt.Sprintf("InputError: [Errno 2] No such file or directory: '%s'", path)
	}
	if os.IsPermission(err) {
		return fmt.Sprintf("InputError: [Errno 13] Permission denied: '%s'", path)
	}
	if fi, e2 := os.Stat(path); e2 == nil && fi.IsDir() {
		return fmt.Sprintf("InputError: [Errno 21] Is a directory: '%s'", path)
	}
	return "InputError: " + err.Error()
}

// decodeInput decodes file content like docutils.io.FileInput (UTF-8 with
// optional BOM; Latin-1 on request).
func decodeInput(data []byte, encoding string) (string, error) {
	enc := strings.ToLower(encoding)
	switch enc {
	case "latin-1", "latin1", "latin_1", "iso-8859-1", "iso8859-1", "l1":
		rs := make([]rune, len(data))
		for i, b := range data {
			rs[i] = rune(b)
		}
		return string(rs), nil
	}
	s := string(data)
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("UnicodeDecodeError: 'utf-8' codec can't decode bytes")
	}
	// universal newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s, nil
}

func runInclude(d *directive) []*Node {
	st := d.state.document.settings
	if !st.fileInsertionEnabled {
		d.fail(2, fmt.Sprintf(`"%s" directive disabled.`, d.name))
	}
	tabWidth := st.tabWidth
	if v, ok := d.options["tab-width"]; ok {
		tabWidth = v.(int)
	}
	clip := fmt.Sprintf("%v|%v|%v|%v", optRepr(d.options, "start-line"), optRepr(d.options, "end-line"),
		optRepr(d.options, "start-after"), optRepr(d.options, "end-before"))
	path := directivePath(d.arguments[0])
	var readPath string
	if strings.HasPrefix(path, "<") && strings.HasSuffix(path, ">") {
		name := path[1 : len(path)-1]
		// Docutils reports the path of its installed "include" directory;
		// we use a stable pseudo path instead.
		path = "<" + name + ">"
		readPath = standardIncludePrefix + name
	} else {
		path = adaptPath(path, d.state.document.currentSource, st.rootPrefix)
		readPath = path
	}
	d.options["source"] = path
	encoding := "utf-8"
	if e, ok := d.options["encoding"]; ok {
		encoding = e.(string)
	}
	var text string
	if strings.HasPrefix(readPath, standardIncludePrefix) {
		data, err := standardIncludes.ReadFile("include/" + strings.TrimPrefix(readPath, standardIncludePrefix))
		if err != nil {
			d.fail(3, fmt.Sprintf("Problems with \"%s\" directive path:\nInputError: [Errno 2] No such file or directory: '%s'.", d.name, path))
		}
		text = string(data)
	} else {
		data, err := os.ReadFile(readPath)
		if err != nil {
			d.fail(3, fmt.Sprintf("Problems with \"%s\" directive path:\n%s.", d.name, ioErrorString(err, path)))
		}
		var derr error
		text, derr = decodeInput(data, encoding)
		if derr != nil {
			d.fail(3, fmt.Sprintf("Problem with \"%s\" directive:\n%s", d.name, derr.Error()))
		}
	}
	startline, hasStart := d.options["start-line"].(int)
	endline, hasEnd := d.options["end-line"].(int)
	if (hasStart && startline != 0) || hasEnd {
		lines := pySplitlines(text, false)
		a, b := 0, len(lines)
		if hasStart {
			a = startline
		}
		if hasEnd {
			b = endline
		}
		a, b = clampSlice(a, b, len(lines))
		text = strings.Join(lines[a:b], "\n")
	}
	starttext, hasST := d.options["start-after"].(string)
	if hasST && starttext == "" {
		starttext = "\n\n"
	}
	if hasST && starttext != "" {
		idx := strings.Index(text, starttext)
		if idx < 0 {
			d.fail(3, fmt.Sprintf("Problem with \"start-after\" option of \"%s\" directive:\nText not found.", d.name))
		}
		text = text[idx+len(starttext):]
	}
	endtext, hasET := d.options["end-before"].(string)
	if hasET && endtext == "" {
		idx := strings.Index(text, "\n\n")
		if idx > 0 {
			text = text[:idx+1]
		}
	} else if hasET {
		idx := strings.Index(text, endtext)
		if idx < 0 {
			d.fail(3, fmt.Sprintf("Problem with \"end-before\" option of \"%s\" directive:\nText not found.", d.name))
		}
		text = text[:idx]
	}
	if d.hasOpt("literal") {
		if tabWidth >= 0 {
			text = pyExpandtabs(text, tabWidth)
		}
		lb := newElem("literal_block", "")
		lb.Attrs["source"] = path
		lb.Attrs["classes"] = append([]string{}, d.optClasses()...)
		ln := 1
		if hasStart {
			ln = startline + 1
		}
		lb.setSrcLine(path, true, ln, true)
		d.addName(lb)
		if d.hasOpt("number-lines") {
			firstline := 1
			if v, ok := d.options["number-lines"].(int); ok {
				firstline = v
			}
			text = strings.TrimSuffix(text, "\n")
			lastline := firstline + len(pySplitlines(text, false))
			for _, t := range numberLines([]codeToken{{nil, text}}, firstline, lastline) {
				if len(t.classes) > 0 {
					in := newTextElem("inline", "", t.value)
					in.Attrs["classes"] = append([]string{}, t.classes...)
					lb.append(in)
				} else {
					lb.append(newText(t.value))
				}
			}
		} else {
			lb.append(newText(text))
		}
		return []*Node{lb}
	}
	if c, ok := d.options["code"]; ok {
		if tabWidth >= 0 {
			text = pyExpandtabs(text, tabWidth)
		}
		delete(d.options, "code")
		return codeBlock(d, c.(string), []string{strings.TrimSuffix(text, "\n")})
	}
	if p, ok := d.options["parser"]; ok && p != nil {
		return includeCustomParse(d, text, path)
	}
	// insert into input lines
	textlines := string2lines(text, tabWidth, true)
	for i, line := range textlines {
		if runeLen(line) > st.lineLengthLimit {
			lineNo := i + 1
			if hasStart {
				lineNo += startline
			}
			d.fail(2, fmt.Sprintf(`"%s": line %d exceeds the line-length-limit.`, path, lineNo))
		}
	}
	doc := d.state.document
	if len(doc.includeLog) == 0 {
		doc.includeLog = append(doc.includeLog, includeEntry{relativePath(doc.currentSource), "None|None|None|None"})
	}
	for _, e := range doc.includeLog {
		if e.path == path && e.clip == clip {
			chain := []string{path}
			for i := len(doc.includeLog) - 1; i >= 0; i-- {
				chain = append(chain, doc.includeLog[i].path)
			}
			d.fail(2, fmt.Sprintf("circular inclusion in \"%s\" directive:\n%s", d.name, strings.Join(chain, "\n> ")))
		}
	}
	doc.includeLog = append(doc.includeLog, includeEntry{path, clip})
	textlines = append(textlines, "", fmt.Sprintf(`.. end of inclusion from "%s"`, path))
	d.sm.insertInput(textlines, path)
	return nil
}

func optRepr(o map[string]any, k string) string {
	v, ok := o[k]
	if !ok {
		return "None"
	}
	return pyRepr(v)
}

// includeCustomParse handles ":parser: rst" (the only parser available).
func includeCustomParse(d *directive, text, source string) []*Node {
	parent := d.state.document
	doc := newDocument(source, parent.settings, parent.reporter.stream)
	doc.reporter = &reporter{source: source, reportLevel: parent.reporter.reportLevel,
		haltLevel: parent.reporter.haltLevel, stream: parent.reporter.stream}
	doc.includeLog = parent.includeLog
	doc.names = parent.names
	doc.ids = parent.ids
	doc.nameids = parent.nameids
	doc.nametypes = parent.nametypes
	parseInto(doc, text)
	parent.parseMessages = append(parent.parseMessages, doc.parseMessages...)
	applyParserTransforms(doc)
	parent.transformMessages = append(parent.transformMessages, doc.transformMessages...)
	return doc.Root.Children
}

func runRaw(d *directive) []*Node {
	st := d.state.document.settings
	if !st.rawEnabled || (!st.fileInsertionEnabled && (d.hasOpt("file") || d.hasOpt("url"))) {
		d.fail(2, fmt.Sprintf(`"%s" directive disabled.`, d.name))
	}
	format := strings.Join(pySplit(pyLower(d.arguments[0])), " ")
	encoding := "utf-8"
	if e, ok := d.options["encoding"]; ok {
		encoding = e.(string)
	}
	var text string
	source, hasSource := "", false
	if len(d.content.data) > 0 {
		if d.hasOpt("file") || d.hasOpt("url") {
			d.fail(3, fmt.Sprintf(`"%s" directive may not both specify an external file and have content.`, d.name))
		}
		text = d.content.join()
	} else if f, ok := d.options["file"]; ok {
		if d.hasOpt("url") {
			d.fail(3, fmt.Sprintf(`The "file" and "url" options may not be simultaneously specified for the "%s" directive.`, d.name))
		}
		path := adaptPath(f.(string), d.state.document.currentSource, st.rootPrefix)
		data, err := os.ReadFile(path)
		if err != nil {
			d.fail(3, fmt.Sprintf("Problems with \"%s\" directive path:\n%s.", d.name, ioErrorString(err, path)))
		}
		var derr error
		text, derr = decodeInput(data, encoding)
		if derr != nil {
			d.fail(3, fmt.Sprintf("Problem with \"%s\" directive:\n%s", d.name, derr.Error()))
		}
		source, hasSource = path, true
	} else if u, ok := d.options["url"]; ok {
		var errStr string
		text, errStr = readURL(u.(string))
		if errStr != "" {
			d.fail(3, fmt.Sprintf("Problems with \"%s\" directive URL \"%s\":\nURLError: %s.", d.name, u, errStr))
		}
		source, hasSource = u.(string), true
	} else {
		d.assertHasContent()
	}
	rawNode := newTextElem("raw", "", text)
	rawNode.Attrs["classes"] = append([]string{}, d.optClasses()...)
	rawNode.Attrs["format"] = format
	if hasSource {
		rawNode.Attrs["source"] = source
	}
	rawNode.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	return []*Node{rawNode}
}

func runReplace(d *directive) []*Node {
	if d.state.kind != "SubstitutionDef" {
		d.fail(3, fmt.Sprintf(`Invalid context: the "%s" directive can only be used within a substitution definition.`, d.name))
	}
	d.assertHasContent()
	element := newElem("Element", d.content.join())
	d.state.nestedParse(d.content, d.contentOffset, element, false)
	var node *Node
	var messages []*Node
	for _, elem := range element.Children {
		if node == nil && elem.Tag == "paragraph" {
			node = elem
		} else if elem.Tag == "system_message" {
			elem.Attrs["backrefs"] = []string{}
			messages = append(messages, elem)
		} else {
			return []*Node{d.reporter().error(fmt.Sprintf(`Error in "%s" directive: may contain a single paragraph only.`, d.name),
				nil, atLine(d.lineno))}
		}
	}
	if node != nil {
		return append(messages, node.Children...)
	}
	return messages
}

func runUnicode(d *directive) []*Node {
	if d.state.kind != "SubstitutionDef" {
		d.fail(3, fmt.Sprintf(`Invalid context: the "%s" directive can only be used within a substitution definition.`, d.name))
	}
	subdef := d.sm.node
	if d.hasOpt("trim") {
		subdef.Attrs["ltrim"] = 1
		subdef.Attrs["rtrim"] = 1
	}
	if d.hasOpt("ltrim") {
		subdef.Attrs["ltrim"] = 1
	}
	if d.hasOpt("rtrim") {
		subdef.Attrs["rtrim"] = 1
	}
	codes := pySplit(beforeUnicodeComment(d.arguments[0]))
	var out []*Node
	for _, code := range codes {
		decoded, err := unicodeCode(code)
		if err != nil {
			d.fail(3, fmt.Sprintf("Invalid character code: %s\nValueError: %s", code, err.Error()))
		}
		out = append(out, newText(decoded))
	}
	return out
}

func runClass(d *directive) []*Node {
	classValue, err := classOption(d.arguments[0])
	if err != nil {
		d.fail(3, fmt.Sprintf(`Invalid class attribute value for "%s" directive: "%s".`, d.name, d.arguments[0]))
	}
	var nodeList []*Node
	if len(d.content.data) > 0 {
		container := newElem("Element", "")
		d.state.nestedParse(d.content, d.contentOffset, container, false)
		for _, node := range container.Children {
			node.appendTo("classes", classValue...)
		}
		nodeList = append(nodeList, container.Children...)
	} else {
		pending := newElem("pending", d.blockText)
		pending.pending = &pendingInfo{transform: "docutils.transforms.misc.ClassAttribute",
			details: map[string]any{"class": classValue, "directive": d.name}}
		d.sm.document.notePending(pending, 0)
		nodeList = append(nodeList, pending)
	}
	return nodeList
}

func runRole(d *directive) []*Node {
	if d.contentOffset > d.lineno || len(d.content.data) == 0 {
		d.fail(3, fmt.Sprintf(`"%s" directive requires arguments on the first line.`, d.name))
	}
	args := d.content.data[0]
	newRoleName, baseRoleName, ok := scanRoleArguments(args)
	if !ok {
		d.fail(3, fmt.Sprintf(`"%s" directive arguments not valid role names: "%s".`, d.name, args))
	}
	var messages []*Node
	var baseRole *role
	if baseRoleName != "" {
		baseRole, messages = lookupRole(baseRoleName, d.state.memo, d.lineno, d.state.reporter)
		if baseRole == nil {
			e := d.state.reporter.error(fmt.Sprintf(`Unknown interpreted text role "%s".`, baseRoleName),
				d.literalBlockOfSource(), atLine(d.lineno))
			return append(messages, e)
		}
	} else {
		baseRole = &role{name: "generic_custom_role", fn: genericCustomRole,
			options: map[string]optConv{"class": optClass}}
	}
	spec := &directiveSpec{optionSpec: baseRole.options, hasContent: baseRole.content}
	var options map[string]any
	var content *stringList
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
		_, options, content, _ = d.state.parseDirectiveBlock(d.content.from(1), d.contentOffset, spec, map[string]any{})
	}()
	if merr != nil {
		e := d.reporter().error(fmt.Sprintf("Error in \"%s\" directive:\n%s.", d.name, merr.msg),
			d.literalBlockOfSource(), atLine(d.lineno))
		return append(messages, e)
	}
	if _, ok := options["class"]; !ok {
		cls, err := classOption(newRoleName)
		if err != nil {
			e := d.reporter().error(fmt.Sprintf("Invalid argument for \"%s\" directive:\n%s.", d.name, err.Error()),
				d.literalBlockOfSource(), atLine(d.lineno))
			return append(messages, e)
		}
		options["class"] = cls
	}
	r := &role{name: newRoleName, fn: customRoleFn, base: baseRole, options: baseRole.options,
		content: baseRole.content, suppliedOptions: options, suppliedContent: content.data}
	setImplicitOptions(r)
	d.state.memo.roles.local[pyLower(newRoleName)] = r
	return messages
}

func runDefaultRole(d *directive) []*Node {
	if len(d.arguments) == 0 {
		delete(d.state.memo.roles.local, "")
		return nil
	}
	roleName := d.arguments[0]
	r, messages := lookupRole(roleName, d.state.memo, d.lineno, d.state.reporter)
	if r == nil {
		e := d.state.reporter.error(fmt.Sprintf(`Unknown interpreted text role "%s".`, roleName),
			d.literalBlockOfSource(), atLine(d.lineno))
		return append(messages, e)
	}
	d.state.memo.roles.local[""] = r
	return messages
}

// ---- meta

func metaFieldMarker(s *rstState, m lineMatch, context []string, nextState string) ([]string, string) {
	node := s.parsemeta(m)
	s.parent().append(node)
	return []string{}, nextState
}

func (s *rstState) parsemeta(m lineMatch) *Node {
	name := parseFieldMarker(m)
	name = unescape(escape2null(name), false)
	indented, _, _, _ := s.sm.getFirstKnownIndented(m.End(0), false, true, true)
	node := newElem("meta", "")
	node.Attrs["content"] = unescape(escape2null(strings.Join(indented.data, " ")), false)
	if len(indented.data) == 0 {
		line := s.sm.line
		return s.reporter.info(fmt.Sprintf(`No content for meta tag "%s".`, name),
			[]*Node{newTextElem("literal_block", line, line)})
	}
	tokens := pySplit(name)
	if attname, val, err := extractNameValue(tokens[0]); err == "" {
		node.Attrs[strings.ToLower(attname)] = val
	} else {
		node.Attrs["name"] = tokens[0]
	}
	for _, token := range tokens[1:] {
		attname, val, err := extractNameValue(token)
		if err != "" {
			line := s.sm.line
			return s.reporter.error(fmt.Sprintf(`Error parsing meta tag attribute "%s": %s.`, token, err),
				[]*Node{newTextElem("literal_block", line, line)})
		}
		node.Attrs[strings.ToLower(attname)] = val
	}
	return node
}

// extractNameValue mirrors utils.extract_name_value for a single token
// (returns the first name/value pair).
func extractNameValue(line string) (string, string, string) {
	eq := strings.Index(line, "=")
	if eq == -1 {
		return "", "", `missing "="`
	}
	attname := pyStrip(line[:eq])
	if eq == 0 || attname == "" {
		return "", "", `missing attribute name before "="`
	}
	line = pyLstrip(line[eq+1:])
	if line == "" {
		return "", "", fmt.Sprintf(`missing value after "%s="`, attname)
	}
	var data string
	if line[0] == '\'' || line[0] == '"' {
		end := strings.IndexByte(line[1:], line[0])
		if end == -1 {
			return "", "", fmt.Sprintf(`attribute "%s" missing end quote (%c)`, attname, line[0])
		}
		end++
		if len(line) > end+1 && pyStrip(line[end+1:end+2]) != "" {
			return "", "", fmt.Sprintf(`attribute "%s" end quote (%c) not followed by whitespace`, attname, line[0])
		}
		data = line[1:end]
	} else {
		sp := strings.Index(line, " ")
		if sp == -1 {
			data = line
		} else {
			data = line[:sp]
		}
	}
	return strings.ToLower(attname), data, ""
}

func runMeta(d *directive) []*Node {
	d.assertHasContent()
	node := newElem("Element", "")
	my := newStateMachine("MetaBody", d.sm)
	my.state("MetaBody").blankFinish = true
	my.runNested(d.content, d.contentOffset, d.state.memo, node, false)
	newLineOffset := my.absLineOffset()
	if newLineOffset-d.contentOffset != len(d.content.data) {
		e := d.reporter().error("Invalid meta directive.", d.literalBlockOfSource(), atLine(d.lineno))
		node.append(e)
	}
	root := d.state.document.Root
	index := root.firstChildNotMatching(func(n *Node) bool { return n.isTitular() || n.Tag == "meta" })
	if index < 0 {
		index = 0
	}
	root.insertAll(index, node.Children)
	return nil
}

func runDate(d *directive) []*Node {
	if d.state.kind != "SubstitutionDef" {
		d.fail(3, fmt.Sprintf(`Invalid context: the "%s" directive can only be used within a substitution definition.`, d.name))
	}
	format := d.content.join()
	if format == "" {
		format = "%Y-%m-%d"
	}
	return []*Node{newText(strftime(format, time.Now()))}
}

// strftime implements the common subset of C strftime directives.
func strftime(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			fmt.Fprintf(&b, "%d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'p':
			b.WriteString(t.Format("PM"))
		case 'B':
			b.WriteString(t.Month().String())
		case 'b', 'h':
			b.WriteString(t.Month().String()[:3])
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'Z':
			b.WriteString(t.Format("MST"))
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'F':
			b.WriteString(t.Format("2006-01-02"))
		case 'T':
			b.WriteString(t.Format("15:04:05"))
		case 'D':
			b.WriteString(t.Format("01/02/06"))
		case 'c':
			b.WriteString(t.Format("Mon Jan _2 15:04:05 2006"))
		case 'x':
			b.WriteString(t.Format("01/02/06"))
		case 'X':
			b.WriteString(t.Format("15:04:05"))
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			fmt.Fprintf(&b, "%d", wd)
		case 'w':
			fmt.Fprintf(&b, "%d", int(t.Weekday()))
		case 's':
			fmt.Fprintf(&b, "%d", t.Unix())
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

func runTestDirective(d *directive) []*Node {
	args := make([]string, len(d.arguments))
	for i, a := range d.arguments {
		args[i] = pyStrRepr(a)
	}
	head := fmt.Sprintf(`Directive processed. Type="%s", arguments=[%s], options=%s, content:`,
		d.name, strings.Join(args, ", "), pyRepr(d.options))
	if len(d.content.data) > 0 {
		text := d.content.join()
		return []*Node{d.reporter().info(head, []*Node{newTextElem("literal_block", text, text)}, atLine(d.lineno))}
	}
	return []*Node{d.reporter().info(head+" None", nil, atLine(d.lineno))}
}
