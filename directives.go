package rstgo

// Port of docutils.parsers.rst.directives (registry and option
// conversion functions).

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type optConv func(arg string, isNone bool) (any, error)

type directiveSpec struct {
	requiredArgs int
	optionalArgs int
	finalArgWS   bool
	optionSpec   map[string]optConv
	hasContent   bool
	run          func(d *directive) []*Node
}

type directive struct {
	name          string
	arguments     []string
	options       map[string]any
	content       *stringList
	lineno        int
	contentOffset int
	blockText     string
	state         *rstState
	sm            *stateMachine
	spec          *directiveSpec
}

func (d *directive) reporter() *reporter { return d.sm.reporter }

// fail raises a DirectiveError (level, message).
func (d *directive) fail(level int, msg string) {
	panic(&directiveError{level, msg})
}

func (d *directive) assertHasContent() {
	if len(d.content.data) == 0 {
		d.fail(3, fmt.Sprintf(`Content block expected for the "%s" directive; none found.`, d.name))
	}
}

func (d *directive) addName(node *Node) {
	if v, ok := d.options["name"]; ok {
		name := fullyNormalizeName(v.(string))
		delete(d.options, "name")
		node.delAttr("name")
		node.appendTo("names", name)
		d.state.document.noteExplicitTarget(node, node)
	}
}

func (d *directive) hasOpt(k string) bool { _, ok := d.options[k]; return ok }

func (d *directive) optClasses() []string {
	if v, ok := d.options["class"]; ok {
		return v.([]string)
	}
	return nil
}

func (d *directive) literalBlockOfSource() []*Node {
	return []*Node{newTextElem("literal_block", d.blockText, d.blockText)}
}

// normalizedOptions mirrors roles.normalize_options(self.options).
func (d *directive) normalizedOptions() map[string]any {
	return normalizeRoleOptions(d.options)
}

var enDirectives = map[string]string{
	"attention": "attention", "caution": "caution", "danger": "danger",
	"error": "error", "hint": "hint", "important": "important", "note": "note",
	"tip": "tip", "warning": "warning", "admonition": "admonition",
	"sidebar": "sidebar", "topic": "topic", "line-block": "line-block",
	"parsed-literal": "parsed-literal", "code": "code", "code-block": "code",
	"sourcecode": "code", "math": "math", "rubric": "rubric",
	"epigraph": "epigraph", "highlights": "highlights", "pull-quote": "pull-quote",
	"compound": "compound", "container": "container", "table": "table",
	"csv-table": "csv-table", "list-table": "list-table", "meta": "meta",
	"image": "image", "figure": "figure", "include": "include", "raw": "raw",
	"replace": "replace", "unicode": "unicode", "date": "date", "class": "class",
	"rst-class": "class", "role": "role", "default-role": "default-role",
	"title": "title", "contents": "contents", "sectnum": "sectnum",
	"section-numbering": "sectnum", "header": "header", "footer": "footer",
	"target-notes":                    "target-notes",
	"restructuredtext-test-directive": "restructuredtext-test-directive",
}

var directiveRegistry = map[string]*directiveSpec{}

// lookupDirective mirrors directives.directive().
func lookupDirective(directiveName string, m *memo, doc *Document) (*directiveSpec, []*Node) {
	normname := pyLower(directiveName)
	var messages []*Node
	if spec, ok := m.directives[normname]; ok {
		return spec, messages
	}
	var msgText []string
	canonicalname, ok := enDirectives[normname]
	if !ok {
		msgText = append(msgText, fmt.Sprintf(`No directive entry for "%s" in module "docutils.parsers.rst.languages.en".`, directiveName))
		msgText = append(msgText, fmt.Sprintf(`Trying "%s" as canonical directive name.`, directiveName))
		canonicalname = normname
	}
	if len(msgText) > 0 {
		var opts []msgOpt
		if doc.hasCurrentLine {
			opts = append(opts, atLine(doc.currentLine))
		}
		messages = append(messages, doc.reporter.info(strings.Join(msgText, "\n"), nil, opts...))
	}
	spec, ok := directiveRegistry[canonicalname]
	if !ok {
		return nil, messages
	}
	m.directives[normname] = spec
	return spec, messages
}

// ---------------------------------------------------------------- option converters

func optFlag(arg string, isNone bool) (any, error) {
	if !isNone && pyStrip(arg) != "" {
		return nil, fmt.Errorf(`no argument is allowed; "%s" supplied`, arg)
	}
	return nil, nil
}

func optUnchangedRequired(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, errors.New("argument required but none supplied")
	}
	return arg, nil
}

func optUnchanged(arg string, isNone bool) (any, error) {
	if isNone {
		return "", nil
	}
	return arg, nil
}

func optPath(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, errors.New("argument required but none supplied")
	}
	return directivePath(arg), nil
}

func directivePath(arg string) string {
	var b strings.Builder
	for _, s := range pySplitlines(arg, false) {
		b.WriteString(pyStrip(s))
	}
	return b.String()
}

func optURI(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, errors.New("argument required but none supplied")
	}
	return directiveURI(arg), nil
}

func directiveURI(arg string) string {
	parts := splitEscapedWhitespace(escape2null(arg))
	for i, part := range parts {
		parts[i] = strings.Join(pySplit(unescape(part, false)), "")
	}
	return strings.Join(parts, " ")
}

const noneIntErr = "int() argument must be a string, a bytes-like object or a real number, not 'NoneType'"

func pyIntConv(arg string, isNone bool) (int, error) {
	if isNone {
		return 0, errors.New(noneIntErr)
	}
	n, ok := pyInt(arg)
	if !ok {
		return 0, fmt.Errorf("invalid literal for int() with base 10: %s", pyStrRepr(arg))
	}
	return n, nil
}

func optInt(arg string, isNone bool) (any, error) {
	n, err := pyIntConv(arg, isNone)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func optNonnegativeInt(arg string, isNone bool) (any, error) {
	n, err := pyIntConv(arg, isNone)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, errors.New("negative value; must be positive or zero")
	}
	return n, nil
}

func optPercentage(arg string, isNone bool) (any, error) {
	if !isNone {
		arg = strings.TrimRight(arg, " %")
	}
	return optNonnegativeInt(arg, isNone)
}

var css3LengthUnits = []string{"em", "ex", "ch", "rem", "vw", "vh", "vmin", "vmax",
	"cm", "mm", "Q", "in", "pt", "pc", "px"}

var measureRe = pyCompile(`(-?[0-9.]+) *([a-zA-Zµ]*|%?)`)

const noneReErr = "expected string or bytes-like object, got 'NoneType'"

// parseMeasure mirrors nodes.parse_measure; value is returned formatted the
// way Python formats int/float.
func parseMeasure(measure string) (valueStr string, value float64, unit string, err error) {
	m := measureRe.FullMatch(measure)
	if m == nil {
		return "", 0, "", fmt.Errorf(`"%s" is no valid measure.`, measure)
	}
	num := m.Group(1)
	if n, ok := pyInt(num); ok && !strings.Contains(num, ".") {
		return strconv.Itoa(n), float64(n), m.Group(2), nil
	}
	f, perr := strconv.ParseFloat(num, 64)
	if perr != nil || strings.Count(num, ".") > 1 || num == "." || num == "-." {
		return "", 0, "", fmt.Errorf(`"%s" is no valid measure.`, measure)
	}
	return pyFloatStr(f), f, m.Group(2), nil
}

func getMeasure(arg string, isNone bool, units []string) (string, error) {
	if isNone {
		return "", errors.New(noneReErr)
	}
	vs, v, unit, err := parseMeasure(arg)
	if err != nil {
		return "", err
	}
	if v < 0 || !containsStr(units, unit) {
		var us []string
		for _, u := range units {
			if u != "" {
				us = append(us, u)
			}
		}
		return "", errors.New("not a positive number or measure of one of the following units:\n" + strings.Join(us, ", "))
	}
	return vs + unit, nil
}

func optLengthOrUnitless(arg string, isNone bool) (any, error) {
	s, err := getMeasure(arg, isNone, append(append([]string{}, css3LengthUnits...), ""))
	if err != nil {
		return nil, err
	}
	return s, nil
}

func lengthOrPercentageOrUnitless(arg string, isNone bool, def string) (any, error) {
	if isNone {
		return nil, errors.New(noneReErr)
	}
	s, err := getMeasure(arg, false, append(append([]string{}, css3LengthUnits...), "%"))
	if err == nil {
		return s, nil
	}
	s2, err2 := getMeasure(arg, false, []string{""})
	if err2 != nil {
		return nil, err
	}
	return s2 + def, nil
}

func optLengthOrPercentageOrUnitless(arg string, isNone bool) (any, error) {
	return lengthOrPercentageOrUnitless(arg, isNone, "")
}

func classOption(arg string) ([]string, error) {
	var classNames []string
	for _, name := range pySplit(arg) {
		cn := makeID(name)
		if cn == "" {
			return nil, fmt.Errorf(`cannot make "%s" into a class name`, name)
		}
		classNames = append(classNames, cn)
	}
	return classNames, nil
}

func optClass(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, errors.New("argument required but none supplied")
	}
	l, err := classOption(arg)
	if err != nil {
		return nil, err
	}
	if l == nil {
		l = []string{}
	}
	return l, nil
}

var unicodePattern = pyCompile(`(?:0x|x|\\x|U\+?|\\u)([0-9a-f]+)$|&#x([0-9a-f]+);$`, reIgnoreCase)

func unicodeCode(code string) (string, error) {
	isDigit := code != ""
	for _, r := range code {
		if !isPyDigit(r) {
			isDigit = false
			break
		}
	}
	var v int64
	var perr error
	if isDigit {
		v, perr = strconv.ParseInt(code, 10, 64)
	} else {
		m := unicodePattern.Match(code)
		if m == nil {
			return code, nil
		}
		value := m.Group(1)
		if value == "" {
			value = m.Group(2)
		}
		v, perr = strconv.ParseInt(value, 16, 64)
	}
	if perr != nil || v > 0x7fffffff {
		return "", errors.New("code too large (Python int too large to convert to C int)")
	}
	if v > 0x10ffff {
		return "", errors.New("chr() arg not in range(0x110000)")
	}
	return string(rune(v)), nil
}

func optSingleCharOrUnicode(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, errors.New("'NoneType' object has no attribute 'isdigit'")
	}
	char, err := unicodeCode(arg)
	if err != nil {
		return nil, err
	}
	if runeLen(char) > 1 {
		return nil, fmt.Errorf("%s invalid; must be a single character or a Unicode code", pyStrRepr(char))
	}
	return char, nil
}

func optSingleCharOrWhitespaceOrUnicode(arg string, isNone bool) (any, error) {
	if arg == "tab" && !isNone {
		return "\t", nil
	}
	if arg == "space" && !isNone {
		return " ", nil
	}
	return optSingleCharOrUnicode(arg, isNone)
}

func positiveInt(arg string) (int, error) {
	n, err := pyIntConv(arg, false)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, errors.New("negative or zero value; must be positive")
	}
	return n, nil
}

func positiveIntList(arg string, isNone bool) ([]int, error) {
	if isNone {
		return nil, errors.New("argument of type 'NoneType' is not iterable")
	}
	var entries []string
	if strings.Contains(arg, ",") {
		entries = strings.Split(arg, ",")
	} else {
		entries = pySplit(arg)
	}
	var out []int
	for _, e := range entries {
		n, err := positiveInt(e)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

var knownEncodings = set(`utf-8 utf8 utf_8 u8 utf latin-1 latin1 latin_1 iso-8859-1
	iso8859-1 l1 ascii us-ascii 646 utf-16 utf16 utf-8-sig utf_8_sig cp1252
	windows-1252 iso-8859-15 latin9 latin-9 utf-16-le utf-16-be utf-32`)

func optEncoding(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, errors.New("lookup() argument must be str, not None")
	}
	if !knownEncodings[strings.ToLower(arg)] {
		return nil, fmt.Errorf(`unknown encoding: "%s"`, arg)
	}
	return arg, nil
}

func formatValues(values []string) string {
	q := make([]string, len(values)-1)
	for i, v := range values[:len(values)-1] {
		q[i] = `"` + v + `"`
	}
	return fmt.Sprintf(`%s, or "%s"`, strings.Join(q, ", "), values[len(values)-1])
}

func choice(arg string, isNone bool, values []string) (string, error) {
	if isNone {
		return "", fmt.Errorf("must supply an argument; choose from %s", formatValues(values))
	}
	value := pyStrip(pyLower(arg))
	if containsStr(values, value) {
		return value, nil
	}
	return "", fmt.Errorf(`"%s" unknown; choose from %s`, arg, formatValues(values))
}

func optChoice(values ...string) optConv {
	return func(arg string, isNone bool) (any, error) {
		v, err := choice(arg, isNone, values)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
}

// optNumberLines is value_or((None,), int).
func optNumberLines(arg string, isNone bool) (any, error) {
	if isNone {
		return nil, nil
	}
	return optInt(arg, false)
}

func optWidths(keywords ...string) optConv {
	return func(arg string, isNone bool) (any, error) {
		if !isNone && containsStr(keywords, arg) {
			return arg, nil
		}
		l, err := positiveIntList(arg, isNone)
		if err != nil {
			return nil, err
		}
		return l, nil
	}
}

func mergeSpec(base map[string]optConv, extra map[string]optConv) map[string]optConv {
	m := map[string]optConv{}
	for k, v := range base {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}
