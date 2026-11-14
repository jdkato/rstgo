package rstgo

// Port of docutils.utils.math.latex2mathml and mathml_elements: conversion
// of LaTeX math to MathML (used by the html5 writer).

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type mathError struct{ msg string }

type mmlAttr struct{ k, v string }

type mmlElem struct {
	tag       string
	text      string
	hasText   bool
	attrs     []mmlAttr
	children  []*mmlElem
	parent    *mmlElem
	nchildren int // -1: unlimited
	sw        bool
}

var mmlNChildren = map[string]int{
	"mtext": 0, "mi": 0, "mn": 0, "mo": 0, "mspace": 0, "mfrac": 2, "msqrt": 1,
	"mroot": 2, "menclose": 1, "mphantom": 1, "msub": 2, "msup": 2, "msubsup": 3,
	"munder": 2, "mover": 2, "munderover": 3,
}

func mmlStr(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	}
	return fmt.Sprint(v)
}

// mml creates an element; args are children (*mmlElem) and attribute
// key/value pairs given as mmlAttr.
func mml(tag string, args ...any) *mmlElem {
	e := &mmlElem{tag: tag, nchildren: -1}
	if n, ok := mmlNChildren[tag]; ok {
		e.nchildren = n
	}
	var children []*mmlElem
	for _, a := range args {
		switch x := a.(type) {
		case *mmlElem:
			children = append(children, x)
		case mmlAttr:
			e.attrs = append(e.attrs, x)
		case []mmlAttr:
			e.attrs = append(e.attrs, x...)
		}
	}
	for _, c := range children {
		e.append(c)
	}
	return e
}

func at(k string, v any) mmlAttr { return mmlAttr{k, mmlStr(v)} }

func mmlToken(tag, text string, attrs ...mmlAttr) *mmlElem {
	e := mml(tag)
	e.attrs = append(e.attrs, attrs...)
	e.text, e.hasText = text, true
	return e
}

func mmlSwitch(e *mmlElem) *mmlElem { e.sw = true; return e }

func (e *mmlElem) get(k string) (string, bool) {
	for _, a := range e.attrs {
		if a.k == k {
			return a.v, true
		}
	}
	return "", false
}

func (e *mmlElem) set(k string, v any) {
	for i, a := range e.attrs {
		if a.k == k {
			e.attrs[i].v = mmlStr(v)
			return
		}
	}
	e.attrs = append(e.attrs, mmlAttr{k, mmlStr(v)})
}

func (e *mmlElem) isFull() bool { return e.nchildren >= 0 && len(e.children) >= e.nchildren }

func (e *mmlElem) isRow() bool {
	switch e.tag {
	case "math", "mrow", "msqrt", "mstyle", "merror", "menclose", "mpadded", "mphantom", "mtr", "mtd":
		return true
	}
	return false
}

func (e *mmlElem) close() *mmlElem {
	if e.tag == "mrow" {
		parent := e.parent
		if parent != nil && len(e.children) == 1 {
			child := e.children[0]
			idx := -1
			for i, c := range parent.children {
				if c == e {
					idx = i
					break
				}
			}
			if idx < 0 {
				return nil
			}
			if parent.nchildren == 0 {
				panic(&mathError{fmt.Sprintf(`Element "%s" does not take children.`, parent.tag)})
			}
			parent.children[idx] = child
			child.parent = parent
			// transfer_attributes
			for _, a := range e.attrs {
				v := a.v
				if (a.k == "class" || a.k == "style") && v != "" {
					delim := " "
					if a.k == "style" {
						delim = "; "
					}
					if ov, ok := child.get(a.k); ok && ov != "" {
						v = strings.TrimRight(ov, delim) + delim + v
					}
				}
				child.set(a.k, v)
			}
		}
	}
	e.nchildren = len(e.children)
	parent := e.parent
	for parent != nil && parent.isFull() {
		parent = parent.parent
	}
	return parent
}

func (e *mmlElem) append(c *mmlElem) *mmlElem {
	if e.isFull() {
		if e.nchildren > 0 {
			panic(&mathError{fmt.Sprintf(`Element "%s" takes only %d children.`, e.tag, e.nchildren)})
		}
		panic(&mathError{fmt.Sprintf(`Element "%s" does not take children.`, e.tag)})
	}
	e.children = append(e.children, c)
	c.parent = e
	var cur *mmlElem = e
	if e.isFull() {
		cur = e.close()
	}
	if e.sw && e.isFull() && len(e.children) >= 2 {
		n := len(e.children)
		e.children[n-1], e.children[n-2] = e.children[n-2], e.children[n-1]
		e.sw = false
	}
	return cur
}

func (e *mmlElem) pop() *mmlElem {
	if len(e.children) == 0 {
		panic(&mathError{"pop from empty list"})
	}
	c := e.children[len(e.children)-1]
	e.children = e.children[:len(e.children)-1]
	return c
}

func (e *mmlElem) inBlock() bool {
	for x := e; x != nil; x = x.parent {
		if v, ok := x.get("display"); ok {
			return v == "block"
		}
	}
	return false
}

func xmlEscapeText(s string) string {
	if !strings.ContainsAny(s, "&<>") {
		return s
	}
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

func xmlEscapeAttr(s string) string {
	if !strings.ContainsAny(s, "&<>\"\r\n\t") {
		return s
	}
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
		"\r", "&#13;", "\n", "&#10;", "\t", "&#09;")
	return r.Replace(s)
}

// toxml serialises like ET.indent() + ET.tostring(short_empty_elements=False).
func (e *mmlElem) toxml(b *strings.Builder, level int) {
	b.WriteByte('<')
	b.WriteString(e.tag)
	for _, a := range e.attrs {
		b.WriteByte(' ')
		b.WriteString(a.k)
		b.WriteString(`="`)
		b.WriteString(xmlEscapeAttr(a.v))
		b.WriteByte('"')
	}
	b.WriteByte('>')
	if len(e.children) > 0 {
		childIndent := "\n" + strings.Repeat("  ", level+1)
		if strings.TrimSpace(e.text) == "" {
			b.WriteString(childIndent)
		} else {
			b.WriteString(xmlEscapeText(e.text))
		}
		for i, c := range e.children {
			c.toxml(b, level+1)
			if i == len(e.children)-1 {
				b.WriteString("\n" + strings.Repeat("  ", level))
			} else {
				b.WriteString(childIndent)
			}
		}
	} else if e.hasText {
		b.WriteString(xmlEscapeText(e.text))
	}
	b.WriteString("</")
	b.WriteString(e.tag)
	b.WriteByte('>')
}

var mmlGreekCapitals = set("Phi Xi Sigma Psi Delta Theta Upsilon Pi Omega Gamma Lambda")

var mmlFunctions = func() map[string]string {
	m := map[string]string{
		"liminf": "lim inf", "limsup": "lim sup", "injlim": "inj lim",
		"projlim": "proj lim", "varlimsup": "lim", "varliminf": "lim",
		"varprojlim": "lim", "varinjlim": "lim", "operatorname": "",
	}
	for _, n := range strings.Fields(`arccos arcsin arctan arg cos cosh cot coth csc deg det dim
		exp gcd hom ker lg ln log Pr sec sin sinh tan tanh`) {
		m[n] = n
	}
	return m
}()

var mmlModulo = map[string]struct {
	binary, named, parentheses bool
	padding                    string
}{
	"bmod": {true, true, false, "0.278em"}, "pmod": {false, true, true, "0.444em"},
	"mod": {false, true, false, "0.667em"}, "pod": {false, false, true, "0.444em"},
}

var mmlMathAlphabets = map[string]string{
	"mathbb": "double-struck", "mathbf": "bold", "mathbfit": "bold-italic",
	"mathcal": "script", "mathfrak": "fraktur", "mathit": "italic", "mathrm": "normal",
	"mathscr": "script", "mathsf": "sans-serif", "mathbfsfit": "sans-serif-bold-italic",
	"mathsfbfit": "sans-serif-bold-italic", "mathsfit": "sans-serif-italic", "mathtt": "monospace",
}

var mmlThickOperators = map[string]string{"thicksim": "∼", "thickapprox": "≈"}

var mmlSmallOperators = map[string]string{
	"shortmid": "∣", "shortparallel": "∥", "nshortmid": "∤",
	"nshortparallel": "∦", "smallfrown": "⌢", "smallsmile": "⌣", "smallint": "∫",
}

var mmlMovablelimits = set(`bigcap bigcup bigodot bigoplus bigotimes bigsqcup biguplus bigvee
	bigwedge coprod intop ointop prod sum lim max min sup inf`)

var mmlSpaces = map[string]string{
	"qquad": "2em", "quad": "1em", "thickspace": "0.2778em", ";": "0.2778em",
	" ": "0.25em", "\n": "0.25em", "medspace": "0.2222em", ":": "0.2222em",
	"thinspace": "0.1667em", ",": "0.1667em", "negthinspace": "-0.1667em",
	"!": "-0.1667em", "negmedspace": "-0.2222em", "negthickspace": "-0.2778em",
}

var mmlAccents = map[string]string{
	"acute": "´", "bar": "ˉ", "breve": "˘", "check": "ˇ", "dot": "˙", "ddot": "¨",
	"dddot": "˙˙˙", "ddddot": "˙˙˙˙", "grave": "`", "hat": "ˆ", "mathring": "˚",
	"tilde": "~", "vec": "→",
}

var mmlOver = map[string]string{
	"overbrace": "⏞", "overleftarrow": "←", "overleftrightarrow": "↔",
	"overline": "_", "overrightarrow": "→", "widehat": "^", "widetilde": "~",
}

var mmlUnder = map[string]string{
	"underbrace": "⏟", "underleftarrow": "←", "underleftrightarrow": "↔",
	"underline": "_", "underrightarrow": "→",
}

var mmlAnomalous = map[rune]string{'-': "−", ':': "∶", '~': " "}

var mmlMatrices = map[string][2]string{
	"matrix": {"", ""}, "smallmatrix": {"", ""}, "pmatrix": {"(", ")"},
	"bmatrix": {"[", "]"}, "Bmatrix": {"{", "}"}, "vmatrix": {"|", "|"},
	"Vmatrix": {"‖", "‖"}, "aligned": {"", ""}, "cases": {"{", ""},
}

var mmlLayoutStyles = map[string][]mmlAttr{
	"displaystyle":      {{"displaystyle", "true"}, {"scriptlevel", "0"}},
	"textstyle":         {{"displaystyle", "false"}, {"scriptlevel", "0"}},
	"scriptstyle":       {{"displaystyle", "false"}, {"scriptlevel", "1"}},
	"scriptscriptstyle": {{"displaystyle", "false"}, {"scriptlevel", "2"}},
}

var mmlFractions = map[string][]mmlAttr{
	"frac":   {},
	"cfrac":  {{"displaystyle", "true"}, {"scriptlevel", "0"}, {"class", "cfrac"}},
	"dfrac":  {{"displaystyle", "true"}, {"scriptlevel", "0"}},
	"tfrac":  {{"displaystyle", "false"}, {"scriptlevel", "0"}},
	"binom":  {{"linethickness", "0"}},
	"dbinom": {{"displaystyle", "true"}, {"scriptlevel", "0"}, {"linethickness", "0"}},
	"tbinom": {{"displaystyle", "false"}, {"scriptlevel", "0"}, {"linethickness", "0"}},
}

var mmlDelimiterSizes = []string{"", "1.2em", "1.623em", "2.047em", "2.470em"}

var mmlBigDelimiters = map[string]int{"left": 0, "right": 0, "bigl": 1, "bigr": 1,
	"Bigl": 2, "Bigr": 2, "biggl": 3, "biggr": 3, "Biggl": 4, "Biggr": 4}

func texCmdname(s string) (string, string) {
	if name, rest, ok := texCmdnameSplit(s); ok {
		return name, rest
	}
	if s == "" {
		return "", ""
	}
	_, w := utf8.DecodeRuneInString(s)
	return s[:w], s[w:]
}

func texNumber(s string) (string, string) {
	if num, rest, ok := texNumberSplit(s); ok {
		return num, rest
	}
	return "", s
}

func texToken(s string) (string, string) {
	if cmdEnd, end, ok := texCmdTokenEnd(s); ok {
		return s[:cmdEnd], s[end:]
	}
	if strings.HasPrefix(s, `\`) && len(s) > 1 {
		_, w := utf8.DecodeRuneInString(s[1:])
		return s[:1+w], s[1+w:]
	}
	if s == "" {
		return "", ""
	}
	_, w := utf8.DecodeRuneInString(s)
	return s[:w], s[w:]
}

func texGroup(s string) (string, string) {
	if !strings.HasPrefix(s, "{") {
		if s == "" {
			return "", ""
		}
		_, w := utf8.DecodeRuneInString(s)
		return s[:w], s[w:]
	}
	nest := 0
	escape := false
	for i, c := range s {
		if escape {
			escape = false
		} else if c == '\\' {
			escape = true
		} else if c == '{' {
			nest++
		} else if c == '}' {
			nest--
		}
		if nest == 0 {
			end := i + utf8.RuneLen(c)
			return s[1 : end-1], s[end:]
		}
	}
	panic(&mathError{"Group without closing bracket!"})
}

func texTokenOrGroup(s string) (string, string) {
	arg, rest := texToken(s)
	if arg == "{" {
		arg, rest = texGroup(pyLstrip(s))
	}
	return arg, rest
}

func texOptarg(s string) (string, string) {
	optarg, rest, ok := texOptargSplit(s)
	if !ok {
		if !strings.HasPrefix(s, "[") {
			return "", s
		}
		panic(&mathError{fmt.Sprintf(`Could not extract optional argument from "%s"!`, s)})
	}
	return optarg, rest
}

func isAlphaStr(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func parseLatexMath(root *mmlElem, source string) *mmlElem {
	s := source
	node := root
	for len(s) > 0 {
		c, w := utf8.DecodeRuneInString(s)
		s = s[w:]
		if c == ' ' || c == '\n' {
			continue
		}
		switch {
		case c == '\\':
			var cmdname string
			cmdname, s = texCmdname(s)
			node, s = mmlHandleCmd(cmdname, node, s)
		case c == '_' || c == '^':
			node = mmlHandleScriptOrLimit(node, c, "")
		case c == '{':
			if node.isRow() && node.nchildren == 1 {
				node.nchildren = -1
			} else {
				n := mml("mrow")
				node.append(n)
				node = n
			}
		case c == '}':
			node = node.close()
		case c == '&':
			n := mml("mtd")
			p := node.close()
			if p == nil {
				panic(&mathError{"No insertion point."})
			}
			p.append(n)
			node = n
		case unicode.IsLetter(c):
			node = node.append(mmlToken("mi", string(c)))
		case unicode.IsDigit(c):
			var number string
			number, s = texNumber(s)
			node = node.append(mmlToken("mn", string(c)+number))
		case mmlAnomalous[c] != "":
			var attrs []mmlAttr
			if c == '-' && len(node.children) > 0 {
				prev := node.children[len(node.children)-1]
				cls, _ := prev.get("class")
				if (prev.text != "" && strings.Contains("([=", prev.text)) || cls == "mathopen" {
					attrs = append(attrs, at("form", "prefix"))
				}
			}
			node = node.append(mmlToken("mo", mmlAnomalous[c], attrs...))
		case strings.ContainsRune("/()[]|", c):
			node = node.append(mmlToken("mo", string(c), at("stretchy", false)))
		case strings.ContainsRune("+*=<>,.!?`';@", c):
			node = node.append(mmlToken("mo", string(c)))
		default:
			panic(&mathError{fmt.Sprintf(`Unsupported character: "%c"!`, c)})
		}
		if node == nil {
			if s == "" {
				return root
			}
			panic(&mathError{fmt.Sprintf(`No insertion point for "%s". Unbalanced braces in "%s"?`,
				s, source[:len(source)-len(s)])})
		}
	}
	if node.nchildren > 0 && len(node.children) < node.nchildren {
		panic(&mathError{"Last node missing children. Source incomplete?"})
	}
	return root
}

func mmlHandleCmd(name string, node *mmlElem, s string) (*mmlElem, string) {
	if v, ok := mml_letters[name]; ok {
		n := mmlToken("mi", v)
		if mmlGreekCapitals[name] {
			n.set("class", "capital-greek")
		}
		return node.append(n), s
	}
	if v, ok := mml_ordinary[name]; ok {
		return node.append(mmlToken("mi", v)), s
	}
	if v, ok := mmlFunctions[name]; ok {
		var n *mmlElem
		if name == "operatorname" {
			var arg string
			arg, s = texTokenOrGroup(s)
			n = mmlToken("mi", arg, at("mathvariant", "normal"))
		} else {
			n = mmlToken("mi", v)
		}
		switch name {
		case "varliminf":
			n = mml("munder", n, mmlToken("mo", "_"))
		case "varlimsup":
			n = mml("mover", n, mmlToken("mo", "¯"), at("accent", false))
		case "varprojlim":
			n = mml("munder", n, mmlToken("mo", "←"))
		case "varinjlim":
			n = mml("munder", n, mmlToken("mo", "→"))
		}
		node = node.append(n)
		if s != "" && s[0] != '^' && s[0] != '_' {
			node = node.append(mmlToken("mo", "⁡"))
		}
		return node, s
	}
	if mf, ok := mmlModulo[name]; ok {
		padding := mf.padding
		if mf.binary {
			return node.append(mmlToken("mo", "mod", at("lspace", padding), at("rspace", padding))), s
		}
		if node.inBlock() {
			padding = "1em"
		}
		node = node.append(mml("mspace", at("width", padding)))
		if mf.parentheses {
			node = node.append(mmlToken("mo", "(", at("stretchy", false)))
		}
		if mf.named {
			node = node.append(mmlToken("mi", "mod"))
			node = node.append(mml("mspace", at("width", "0.333em")))
		}
		var arg string
		arg, s = texTokenOrGroup(s)
		node = parseLatexMath(node, arg)
		if mf.parentheses {
			node = node.append(mmlToken("mo", ")", at("stretchy", false)))
		}
		return node, s
	}
	if name == "boldsymbol" || name == "pmb" {
		n := mml("mrow", at("class", "boldsymbol"))
		node.append(n)
		return n, s
	}
	if _, ok := mmlMathAlphabets[name]; ok {
		return mmlHandleMathAlphabet(name, node, s)
	}
	if name == "colon" {
		return node.append(mmlToken("mo", ":", at("form", "postfix"), at("lspace", "0"), at("rspace", "0.28em"))), s
	}
	if name == "idotsint" {
		return parseLatexMath(node, `\int\dotsi\int`), s
	}
	if v, ok := mmlThickOperators[name]; ok {
		return node.append(mmlToken("mo", v, at("style", "font-weight: bold"))), s
	}
	if v, ok := mmlSmallOperators[name]; ok {
		return node.append(mmlToken("mo", v, at("mathsize", "75%"))), s
	}
	if v, ok := mml_operators[name]; ok {
		var attrs []mmlAttr
		if mmlMovablelimits[name] && s != "" && strings.ContainsRune(" _^", rune(s[0])) {
			attrs = append(attrs, at("movablelimits", true))
		} else if name == "lvert" || name == "lVert" {
			attrs = append(attrs, at("class", "mathopen"))
		}
		return node.append(mmlToken("mo", v, attrs...)), s
	}
	if sizeIdx, ok := mmlBigDelimiters[name]; ok {
		var attrs []mmlAttr
		size := mmlDelimiterSizes[sizeIdx]
		var delimiter string
		delimiter, s = texTokenOrGroup(s)
		if !strings.Contains("()[]/|.", delimiter) {
			v, ok := mml_stretchables[strings.TrimLeft(delimiter, `\`)]
			if !ok {
				panic(&mathError{fmt.Sprintf(`Unsupported "\%s" delimiter "%s"!`, name, delimiter)})
			}
			delimiter = v
		}
		if size != "" {
			attrs = append(attrs, at("maxsize", size), at("minsize", size), at("symmetric", true))
		}
		if name == "left" || strings.HasSuffix(name, "l") {
			row := mml("mrow")
			node.append(row)
			node = row
		}
		if delimiter != "." {
			node.append(mmlToken("mo", delimiter, attrs...))
		}
		if name == "right" || strings.HasSuffix(name, "r") {
			node = node.close()
		}
		return node, s
	}
	if name == "not" {
		var arg string
		arg, s = texToken(s)
		if arg == "{" {
			return node, `{\not ` + s
		}
		if strings.HasPrefix(arg, `\`) {
			v, ok := mml_operators[arg[1:]]
			if !ok {
				panic(&mathError{fmt.Sprintf(`"\not" cannot negate: "%s"!`, arg)})
			}
			arg = v
		}
		// NFC(arg + U+0338)
		rs := []rune(arg)
		if len(rs) > 0 {
			if c, ok := nfcNot[rs[len(rs)-1]]; ok {
				rs[len(rs)-1] = c
				arg = string(rs)
			} else {
				arg += "̸"
			}
		} else {
			arg = "̸"
		}
		return node.append(mmlToken("mo", arg)), s
	}
	if name == "text" || name == "mbox" || name == "textrm" {
		var arg string
		arg, s = texTokenOrGroup(s)
		for i, part := range strings.Split(arg, "$") {
			if i%2 == 0 {
				part = texTextEdges(part)
				node = node.append(mmlToken("mtext", part))
			} else {
				parseLatexMath(node, part)
			}
		}
		return node, s
	}
	if v, ok := mmlSpaces[name]; ok {
		return node.append(mml("mspace", at("width", v))), s
	}
	if name == "hspace" || name == "mspace" {
		var arg string
		arg, s = texGroup(s)
		if strings.HasSuffix(arg, "mu") {
			f, err := strconv.ParseFloat(strings.TrimSpace(arg[:len(arg)-2]), 64)
			if err != nil {
				panic(&mathError{fmt.Sprintf("could not convert string to float: %s", pyStrRepr(arg[:len(arg)-2]))})
			}
			arg = pyFloatStr(f/18) + "em"
		}
		return node.append(mml("mspace", at("width", arg))), s
	}
	if name == "phantom" {
		n := mml("mphantom")
		node.append(n)
		return n, s
	}
	if name == "boxed" {
		n := mml("menclose", at("notation", "box"), at("class", "boxed"))
		node.append(n)
		return n, s
	}
	if name == "sqrt" {
		var radix string
		radix, s = texOptarg(s)
		var n *mmlElem
		if radix != "" {
			indexnode := mml("mrow")
			n = mmlSwitch(mml("mroot", indexnode))
			parseLatexMath(indexnode, radix)
			indexnode.close()
		} else {
			n = mml("msqrt")
		}
		node.append(n)
		return n, s
	}
	if fa, ok := mmlFractions[name]; ok {
		attrs := append([]mmlAttr{}, fa...)
		if name == "cfrac" {
			var optarg string
			optarg, s = texOptarg(s)
			if al, ok := map[string]string{"l": "left", "r": "right"}[optarg]; ok {
				for i := range attrs {
					if attrs[i].k == "class" {
						attrs[i].v += " numalign-" + al
					}
				}
				attrs = append(attrs, at("numalign", al))
			}
		}
		frac := mml("mfrac", attrs)
		n := frac
		if strings.HasSuffix(name, "binom") {
			n = mml("mrow", mmlToken("mo", "("), frac, mmlToken("mo", ")"), at("class", "binom"))
			n.nchildren = 3
		}
		node.append(n)
		return frac, s
	}
	if name == `\` {
		entry := mml("mtd")
		n := mml("mtr", entry)
		p := node.close()
		if p != nil {
			p = p.close()
		}
		if p == nil {
			panic(&mathError{`No insertion point for "\\".`})
		}
		p.append(n)
		return entry, s
	}
	if v, ok := mmlAccents[name]; ok {
		accent := mmlToken("mo", v, at("stretchy", false))
		if name == "vec" {
			accent.set("scriptlevel", "+1")
		}
		n := mmlSwitch(mml("mover", accent, at("accent", true)))
		node.append(n)
		return n, s
	}
	if v, ok := mmlOver[name]; ok {
		n := mmlSwitch(mml("mover", mmlToken("mo", v, at("accent", true)), at("accent", false)))
		node.append(n)
		return n, s
	}
	if name == "overset" {
		n := mmlSwitch(mml("mover"))
		node.append(n)
		return n, s
	}
	if v, ok := mmlUnder[name]; ok {
		n := mmlSwitch(mml("munder", mmlToken("mo", v)))
		node.append(n)
		return n, s
	}
	if name == "underset" {
		n := mmlSwitch(mml("munder"))
		node.append(n)
		return n, s
	}
	if name == "xleftarrow" || name == "xrightarrow" {
		var subscript string
		subscript, s = texOptarg(s)
		base := mmlToken("mo", mml_operators["long"+name[1:]])
		var n *mmlElem
		if subscript != "" {
			n = mml("munderover", base)
			sub := parseLatexMath(mml("mrow"), subscript)
			if len(sub.children) == 1 {
				sub = sub.children[0]
			}
			n.append(sub)
		} else {
			n = mml("mover", base)
		}
		node.append(n)
		return n, s
	}
	if ls, ok := mmlLayoutStyles[name]; ok {
		if len(node.children) > 0 {
			panic(&mathError{fmt.Sprintf(`Declaration "\%s" must be first command in a group!`, name)})
		}
		for _, a := range ls {
			node.set(a.k, a.v)
		}
		return node, s
	}
	if strings.HasSuffix(name, "limits") {
		arg, remainder := texToken(s)
		if strings.Contains("_^", arg) {
			s = remainder
			if arg != "" {
				node = mmlHandleScriptOrLimit(node, rune(arg[0]), name)
			} else {
				panic(&mathError{"Last node missing children. Source incomplete?"})
			}
		}
		return node, s
	}
	if name == "begin" {
		return mmlBeginEnvironment(node, s)
	}
	if name == "end" {
		return mmlEndEnvironment(node, s)
	}
	panic(&mathError{fmt.Sprintf(`Unknown LaTeX command "\%s".`, name)})
}

func (e *mmlElem) iter(f func(*mmlElem)) {
	f(e)
	for _, c := range e.children {
		c.iter(f)
	}
}

func mmlHandleMathAlphabet(name string, node *mmlElem, s string) (*mmlElem, string) {
	var attrs []mmlAttr
	if name == "mathscr" {
		attrs = append(attrs, at("class", "mathscr"))
	}
	var arg string
	arg, s = texTokenOrGroup(s)
	if name == "mathrm" && isAlphaStr(arg) && runeLen(arg) > 1 {
		return node.append(mmlToken("mi", arg)), s
	}
	container := mml("mrow", attrs)
	node.append(container)
	parseLatexMath(container, arg)
	key := strings.ReplaceAll(strings.ReplaceAll(name, "mathscr", "mathcal"), "mathbfsfit", "mathsfbfit")
	a2ch := mmlAlphabets[key]
	container.iter(func(sub *mmlElem) {
		switch sub.tag {
		case "mn":
			var b strings.Builder
			for _, ch := range sub.text {
				if v, ok := a2ch[string(ch)]; ok {
					b.WriteString(v)
				} else {
					b.WriteRune(ch)
				}
			}
			sub.text = b.String()
		case "mi":
			if v, ok := a2ch[sub.text]; ok {
				sub.text = v
			}
			if name == "mathrm" && isAlphaStr(sub.text) {
				sub.set("mathvariant", "normal")
			}
		}
	})
	return container.close(), s
}

func mmlHandleScriptOrLimit(node *mmlElem, c rune, limits string) *mmlElem {
	child := node.pop()
	if limits == "limits" {
		child.set("movablelimits", "false")
	} else if limits == "movablelimits" || (child.hasText && mmlMovablelimits[child.text]) {
		child.set("movablelimits", "true")
	}
	ml, _ := child.get("movablelimits")
	useLimits := limits == "limits" || limits == "movablelimits" || (limits == "" && ml != "")
	var n *mmlElem
	detach := func(e *mmlElem) []any {
		out := make([]any, len(e.children))
		for i, ch := range e.children {
			out[i] = ch
		}
		return out
	}
	if c == '_' {
		switch {
		case child.tag == "mover":
			n = mmlSwitch(mml("munderover", detach(child)...))
		case child.tag == "msup":
			n = mmlSwitch(mml("msubsup", detach(child)...))
		case useLimits:
			n = mml("munder", child)
		default:
			n = mml("msub", child)
		}
	} else {
		switch {
		case child.tag == "munder":
			n = mml("munderover", detach(child)...)
		case child.tag == "msub":
			n = mml("msubsup", detach(child)...)
		case useLimits:
			n = mml("mover", child)
		default:
			n = mml("msup", child)
		}
	}
	node.append(n)
	return n
}

func mmlBeginEnvironment(node *mmlElem, s string) (*mmlElem, string) {
	var name string
	name, s = texGroup(s)
	delims, ok := mmlMatrices[name]
	if !ok {
		panic(&mathError{fmt.Sprintf(`Environment "%s" not supported!`, name)})
	}
	var attrs []mmlAttr
	if delims[0] != "" {
		wrapper := mml("mrow", mmlToken("mo", delims[0]))
		if name == "cases" {
			wrapper = mml("mrow", mmlToken("mo", delims[0], at("rspace", "0.17em")))
			attrs = append(attrs, at("columnalign", "left"), at("class", "cases"))
		}
		node.append(wrapper)
		node = wrapper
	} else if name == "smallmatrix" {
		attrs = append(attrs, at("rowspacing", "0.02em"), at("columnspacing", "0.333em"), at("scriptlevel", "1"))
	} else if name == "aligned" {
		attrs = append(attrs, at("class", "ams-align"))
	}
	entry := mml("mtd")
	node.append(mml("mtable", mml("mtr", entry), attrs))
	return entry, s
}

func mmlEndEnvironment(node *mmlElem, s string) (*mmlElem, string) {
	var name string
	name, s = texGroup(s)
	delims, ok := mmlMatrices[name]
	if !ok {
		panic(&mathError{fmt.Sprintf(`Environment "%s" not supported!`, name)})
	}
	for i := 0; i < 3 && node != nil; i++ {
		node = node.close()
	}
	if node == nil {
		panic(&mathError{"No insertion point after environment."})
	}
	if delims[1] != "" {
		node = node.append(mmlToken("mo", delims[1]))
		if node != nil {
			node = node.close()
		}
	} else if name == "cases" {
		node = node.close()
	}
	return node, s
}

// tex2mathml mirrors latex2mathml.tex2mathml; errStr is the MathError text.
func tex2mathml(texMath string, asBlock bool) (out string, errStr string) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*mathError); ok {
				out, errStr = "", e.msg
				return
			}
			panic(r)
		}
	}()
	tree := mml("math", at("xmlns", "http://www.w3.org/1998/Math/MathML"))
	node := tree
	if asBlock {
		tree.set("display", "block")
		if len(strings.Split(toplevelCode(texMath), `\\`)) > 1 {
			node = mml("mtd")
			tree.append(mml("mtable", mml("mtr", node), at("class", "ams-align"), at("displaystyle", true)))
		}
	}
	parseLatexMath(node, texMath)
	var b strings.Builder
	tree.toxml(&b, 0)
	return strings.ReplaceAll(b.String(), "⁡", "&ApplyFunction;"), ""
}
