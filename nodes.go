package rstgo

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Node is an element or text node of the document tree.  The element names
// (Tag) and attributes mirror docutils.nodes.
type Node struct {
	// Tag is the element name ("paragraph", "section", ...) or "#text" for
	// text nodes.
	Tag string
	// Text holds the content of a text node.
	Text string
	// Attrs maps attribute names to values (string, int, bool or []string).
	Attrs    map[string]any
	Children []*Node
	Parent   *Node
	// Rawsource is the source text the node was generated from, if known.
	Rawsource string
	Source    string
	Line      int // 0 if unknown

	hasLine bool
	hasSrc  bool
	doc     *Document

	// internal (non-attribute) state, as in docutils
	referenced      bool
	resolved        bool
	indirectRefName string
	hasIndirectName bool
	indent          int // for <line>
	hasIndent       bool
	pending         *pendingInfo
	expectByName    map[string]*Node
	expectByID      map[string]*Node
}

type pendingInfo struct {
	transform string // e.g. "docutils.transforms.parts.Contents"
	details   map[string]any
}

const textTag = "#text"

func newText(s string) *Node { return &Node{Tag: textTag, Text: s} }

func newElem(tag string, rawsource string, children ...*Node) *Node {
	n := &Node{Tag: tag, Rawsource: rawsource, Attrs: map[string]any{}}
	if fixedTextElements[tag] {
		n.Attrs["xml:space"] = "preserve"
	}
	for _, c := range children {
		n.append(c)
	}
	return n
}

// newTextElem mirrors TextElement.__init__(rawsource, text, *children).
func newTextElem(tag, rawsource, text string, children ...*Node) *Node {
	n := newElem(tag, rawsource)
	if text != "" {
		n.append(newText(text))
	}
	for _, c := range children {
		n.append(c)
	}
	return n
}

var fixedTextElements = map[string]bool{
	"comment": true, "address": true, "literal_block": true,
	"doctest_block": true, "math_block": true, "raw": true,
}

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		for _, f := range strings.Fields(n) {
			m[f] = true
		}
	}
	return m
}

var (
	inlineTags = set(`target raw reference image abbreviation acronym emphasis
		generated inline literal strong subscript superscript title_reference
		footnote_reference citation_reference substitution_reference math
		problematic`)
	admonitionTags = set(`attention caution danger error important note tip
		hint warning admonition`)
	bodyTags = set(`attention caution danger error important note tip
		hint warning admonition bullet_list enumerated_list definition_list
		field_list option_list paragraph rubric compound container block_quote
		reference literal_block doctest_block math_block line_block footnote
		citation image figure table comment substitution_definition target
		system_message pending raw`)
	invisibleTags       = set("comment substitution_definition target pending")
	preBibliographicTag = set(`comment substitution_definition target pending
		system_message raw title subtitle meta decoration`)
	titularTags       = set("title subtitle rubric")
	structuralTags    = set("topic sidebar section")
	bibliographicTags = set(`author organization address contact version
		revision status date copyright authors field`)
	textElementTags = set(`comment substitution_definition target raw title
		subtitle author organization address contact version revision status
		date copyright paragraph rubric attribution reference term classifier
		field_name option_string option_argument literal_block doctest_block
		math_block line label caption abbreviation acronym emphasis generated
		inline literal strong subscript superscript title_reference
		footnote_reference citation_reference substitution_reference math
		problematic`)
	backLinkableTags = set("system_message footnote citation")
	sequentialTags   = set("bullet_list enumerated_list definition_list field_list option_list")
)

func (n *Node) IsText() bool    { return n.Tag == textTag }
func (n *Node) isInline() bool  { return inlineTags[n.Tag] }
func (n *Node) isBody() bool    { return bodyTags[n.Tag] }
func (n *Node) isTitular() bool { return titularTags[n.Tag] }
func (n *Node) is(tags ...string) bool {
	for _, t := range tags {
		if n.Tag == t {
			return true
		}
	}
	return false
}

func (n *Node) document() *Document {
	for x := n; x != nil; x = x.Parent {
		if x.doc != nil {
			return x.doc
		}
	}
	return nil
}

func (n *Node) setupChild(c *Node) {
	c.Parent = n
	if d := n.document(); d != nil {
		c.doc = d
		if !c.hasSrc {
			c.Source, c.hasSrc = d.currentSource, d.hasCurrentSource
		}
		if !c.hasLine {
			c.Line, c.hasLine = d.currentLine, d.hasCurrentLine
		}
	}
}

func (n *Node) setLine(line int, ok bool)   { n.Line, n.hasLine = line, ok }
func (n *Node) setSource(s string, ok bool) { n.Source, n.hasSrc = s, ok }
func (n *Node) setSrcLine(src string, hasSrc bool, line int, hasLine bool) {
	n.Source, n.hasSrc, n.Line, n.hasLine = src, hasSrc, line, hasLine
}

func (n *Node) append(c *Node) {
	if c == nil {
		return
	}
	n.setupChild(c)
	n.Children = append(n.Children, c)
}

func (n *Node) extend(cs []*Node) {
	for _, c := range cs {
		n.append(c)
	}
}

func (n *Node) insert(i int, c *Node) {
	if i < 0 {
		i += len(n.Children)
		if i < 0 {
			i = 0
		}
	}
	if i > len(n.Children) {
		i = len(n.Children)
	}
	n.setupChild(c)
	n.Children = append(n.Children, nil)
	copy(n.Children[i+1:], n.Children[i:])
	n.Children[i] = c
}

func (n *Node) insertAll(i int, cs []*Node) {
	for j, c := range cs {
		n.insert(i+j, c)
	}
}

func (n *Node) index(c *Node) int {
	for i, x := range n.Children {
		if x == c {
			return i
		}
	}
	return -1
}

func (n *Node) removeAt(i int) *Node {
	c := n.Children[i]
	n.Children = append(n.Children[:i:i], n.Children[i+1:]...)
	return c
}

func (n *Node) remove(c *Node) {
	if i := n.index(c); i >= 0 {
		n.removeAt(i)
	}
}

// replace replaces child old with the given nodes.
func (n *Node) replace(old *Node, news ...*Node) {
	i := n.index(old)
	if i < 0 {
		return
	}
	for _, c := range news {
		n.setupChild(c)
	}
	rest := append([]*Node{}, n.Children[i+1:]...)
	n.Children = append(append(n.Children[:i:i], news...), rest...)
}

// replaceSelf mirrors Element.replace_self.
func (n *Node) replaceSelf(news ...*Node) {
	if len(news) > 0 && !news[0].IsText() {
		news[0].updateBasicAtts(n)
	}
	n.Parent.replace(n, news...)
}

var basicAttributes = []string{"ids", "classes", "names", "dupnames"}

func (n *Node) updateBasicAtts(from *Node) {
	for _, att := range basicAttributes {
		for _, v := range from.list(att) {
			n.appendUnique(att, v)
		}
	}
}

func (n *Node) appendUnique(att, v string) {
	if !containsStr(n.list(att), v) {
		n.appendTo(att, v)
	}
}

func containsStr(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// ---- attributes

func (n *Node) list(key string) []string {
	if n.Attrs == nil {
		return nil
	}
	l, _ := n.Attrs[key].([]string)
	return l
}

func (n *Node) appendTo(key string, vals ...string) {
	n.Attrs[key] = append(n.list(key), vals...)
}

func (n *Node) removeFrom(key, val string) {
	l := n.list(key)
	for i, x := range l {
		if x == val {
			n.Attrs[key] = append(l[:i:i], l[i+1:]...)
			return
		}
	}
}

func isListAttr(key string) bool {
	switch key {
	case "ids", "classes", "names", "dupnames", "backrefs":
		return true
	}
	return false
}

// has reports whether the attribute is present (list attributes: always).
func (n *Node) has(key string) bool {
	if n.Attrs == nil {
		return false
	}
	if _, ok := n.Attrs[key]; ok {
		return true
	}
	return key == "ids" || key == "classes" || key == "names" || key == "dupnames"
}

func (n *Node) get(key string) any {
	if n.Attrs == nil {
		return nil
	}
	return n.Attrs[key]
}

// str returns the attribute as string ("" if missing).
func (n *Node) str(key string) string {
	if n.Attrs == nil {
		return ""
	}
	v, ok := n.Attrs[key]
	if !ok || v == nil {
		return ""
	}
	return attrString(v)
}

// truthy mirrors Python truthiness of node.get(key).
func (n *Node) truthy(key string) bool {
	if n.Attrs == nil {
		return false
	}
	switch v := n.Attrs[key].(type) {
	case nil:
		return false
	case string:
		return v != ""
	case int:
		return v != 0
	case bool:
		return v
	case []string:
		return len(v) > 0
	case float64:
		return v != 0
	}
	return true
}

func (n *Node) setAttr(key string, v any) { n.Attrs[key] = v }
func (n *Node) delAttr(key string)        { delete(n.Attrs, key) }

func attrString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case float64:
		return pyFloatStr(x)
	case []string:
		parts := make([]string, len(x))
		for i, s := range x {
			parts[i] = serialEscape(s)
		}
		return strings.Join(parts, " ")
	case nil:
		return "True"
	}
	return fmt.Sprint(v)
}

func pyFloatStr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".einf") {
		s += ".0"
	}
	return s
}

func serialEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), " ", `\ `)
}

// attlist returns the sorted non-default attributes.
func (n *Node) attlist() []string {
	keys := make([]string, 0, len(n.Attrs))
	for k, v := range n.Attrs {
		if l, ok := v.([]string); ok && len(l) == 0 && isListAttr(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (n *Node) starttag() string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(n.Tag)
	for _, k := range n.attlist() {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(attrString(n.Attrs[k]))
		b.WriteByte('"')
	}
	b.WriteByte('>')
	return b.String()
}

// ---- text

func childTextSeparator(tag string) string {
	if textElementTags[tag] || tag == "option" {
		return ""
	}
	switch tag {
	case "option_group":
		return ", "
	case "option_list_item":
		return "  "
	}
	return "\n\n"
}

// astext mirrors Node.astext().
func (n *Node) astext() string {
	if n.IsText() {
		return unescape(n.Text, false)
	}
	switch n.Tag {
	case "image":
		return n.str("alt")
	case "system_message":
		return fmt.Sprintf("%s:%s: (%s/%s) %s", n.str("source"), n.str("line"),
			n.str("type"), n.str("level"), n.elementText())
	case "option_argument":
		d := " "
		if n.has("delimiter") {
			d = n.str("delimiter")
		}
		return d + n.elementText()
	}
	return n.elementText()
}

func (n *Node) elementText() string {
	if len(n.Children) == 1 {
		return n.Children[0].astext()
	}
	parts := make([]string, len(n.Children))
	for i, c := range n.Children {
		parts[i] = c.astext()
	}
	return strings.Join(parts, childTextSeparator(n.Tag))
}

// ---- copying

func (n *Node) copyNode() *Node {
	if n.IsText() {
		return newText(n.Text)
	}
	c := &Node{Tag: n.Tag, Rawsource: n.Rawsource, Attrs: map[string]any{}}
	for k, v := range n.Attrs {
		if l, ok := v.([]string); ok {
			c.Attrs[k] = append([]string{}, l...)
		} else {
			c.Attrs[k] = v
		}
	}
	c.doc = n.doc
	c.Source, c.hasSrc, c.Line, c.hasLine = n.Source, n.hasSrc, n.Line, n.hasLine
	if n.pending != nil {
		c.pending = n.pending
	}
	return c
}

func (n *Node) deepcopy() *Node {
	c := n.copyNode()
	for _, ch := range n.Children {
		c.append(ch.deepcopy())
	}
	return c
}

// ---- traversal

// findall returns all nodes (self included) in document order satisfying
// cond (nil: all).
func (n *Node) findall(cond func(*Node) bool) []*Node {
	var out []*Node
	var walk func(x *Node)
	walk = func(x *Node) {
		if cond == nil || cond(x) {
			out = append(out, x)
		}
		for _, c := range x.Children {
			walk(c)
		}
	}
	walk(n)
	return out
}

func (n *Node) findTag(tags ...string) []*Node {
	return n.findall(func(x *Node) bool { return x.is(tags...) })
}

// nextNode mirrors Node.next_node(condition, include_self=False,
// descend=..., siblings=..., ascend=...).
func (n *Node) nextNode(cond func(*Node) bool, descend, siblings, ascend bool) *Node {
	if ascend {
		siblings = true
	}
	var search func(x *Node) *Node
	search = func(x *Node) *Node {
		if cond == nil || cond(x) {
			return x
		}
		if descend {
			for _, c := range x.Children {
				if r := search(c); r != nil {
					return r
				}
			}
		}
		return nil
	}
	if descend {
		for _, c := range n.Children {
			if r := search(c); r != nil {
				return r
			}
		}
	}
	if siblings {
		node := n
		for node.Parent != nil {
			idx := node.Parent.index(node)
			for _, sib := range node.Parent.Children[idx+1:] {
				if r := search(sib); r != nil {
					return r
				}
			}
			if !ascend {
				break
			}
			node = node.Parent
		}
	}
	return nil
}

func (n *Node) firstChildMatching(pred func(*Node) bool) int {
	for i, c := range n.Children {
		if pred(c) {
			return i
		}
	}
	return -1
}

func (n *Node) firstChildNotMatching(pred func(*Node) bool) int {
	for i, c := range n.Children {
		if !pred(c) {
			return i
		}
	}
	return -1
}

func (n *Node) sectionHierarchy() []*Node {
	var secs []*Node
	for node := n; node.Parent != nil; node = node.Parent {
		if node.Tag == "section" {
			secs = append(secs, node)
		}
	}
	for i, j := 0, len(secs)-1; i < j; i, j = i+1, j-1 {
		secs[i], secs[j] = secs[j], secs[i]
	}
	return secs
}

func (n *Node) previousSibling() *Node {
	if n.Parent == nil {
		return nil
	}
	i := n.Parent.index(n)
	if i > 0 {
		return n.Parent.Children[i-1]
	}
	return nil
}

func (n *Node) noteReferencedBy(name, id string) {
	n.referenced = true
	if name != "" {
		if x := n.expectByName[name]; x != nil {
			x.referenced = true
		}
	}
	if id != "" {
		if x := n.expectByID[id]; x != nil {
			x.referenced = true
		}
	}
}

func (n *Node) addBackref(id string) { n.appendTo("backrefs", id) }

func (n *Node) getLanguageCode(fallback string) string {
	for x := n; x != nil; x = x.Parent {
		for _, c := range x.list("classes") {
			if strings.HasPrefix(c, "language-") {
				return strings.TrimPrefix(c, "language-")
			}
		}
	}
	return fallback
}

// ---- pseudo-XML

func (n *Node) pformat(b *strings.Builder, level int) {
	indent := strings.Repeat("    ", level)
	if n.IsText() {
		for _, line := range pySplitlines(n.astext(), false) {
			b.WriteString(indent)
			b.WriteString(line)
			b.WriteByte('\n')
		}
		return
	}
	b.WriteString(indent)
	b.WriteString(n.starttag())
	b.WriteByte('\n')
	for _, c := range n.Children {
		c.pformat(b, level+1)
	}
	if n.pending != nil {
		internals := []string{".. internal attributes:",
			"     .transform: " + n.pending.transform,
			"     .details:"}
		keys := make([]string, 0, len(n.pending.details))
		for k := range n.pending.details {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := n.pending.details[k]
			switch x := v.(type) {
			case *Node:
				internals = append(internals, "       "+k+":")
				var sb strings.Builder
				x.pformat(&sb, 0)
				for _, l := range pySplitlines(sb.String(), false) {
					internals = append(internals, "         "+l)
				}
			case []*Node:
				if len(x) == 0 {
					internals = append(internals, "       "+k+": []")
					break
				}
				internals = append(internals, "       "+k+":")
				for _, nd := range x {
					var sb strings.Builder
					nd.pformat(&sb, 0)
					for _, l := range pySplitlines(sb.String(), false) {
						internals = append(internals, "         "+l)
					}
				}
			default:
				internals = append(internals, "       "+k+": "+pyRepr(v))
			}
		}
		for _, l := range internals {
			b.WriteString("    ")
			b.WriteString(indent)
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
}

// unescape mirrors nodes.unescape.
func unescape(text string, restoreBackslashes bool) string {
	if strings.IndexByte(text, 0) < 0 {
		return text
	}
	if restoreBackslashes {
		return strings.ReplaceAll(text, "\x00", `\`)
	}
	text = strings.ReplaceAll(text, "\x00 ", "")
	text = strings.ReplaceAll(text, "\x00\n", "")
	return strings.ReplaceAll(text, "\x00", "")
}

func fullyNormalizeName(name string) string {
	return strings.Join(pySplit(pyLower(name)), " ")
}

func whitespaceNormalizeName(name string) string {
	return strings.Join(pySplit(name), " ")
}

func dupname(node *Node, name string) {
	node.appendTo("dupnames", name)
	node.removeFrom("names", name)
	node.referenced = true
}
