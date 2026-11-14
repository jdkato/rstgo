package rstgo

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	doc *Document
	ex  *nodeExtra // rarely needed state

	hasLine bool
	hasSrc  bool
	// internal (non-attribute) state, as in docutils
	referenced bool
	resolved   bool
}

// nodeExtra holds node state that only a few nodes need.
type nodeExtra struct {
	indent       int // for <line>
	hasIndent    bool
	pending      *pendingInfo
	expectByName map[string]*Node
	expectByID   map[string]*Node
}

func (n *Node) ext() *nodeExtra {
	if n.ex == nil {
		n.ex = &nodeExtra{}
	}
	return n.ex
}

func (n *Node) pend() *pendingInfo {
	if n.ex == nil {
		return nil
	}
	return n.ex.pending
}

func (n *Node) hasIndent() bool { return n.ex != nil && n.ex.hasIndent }

func (n *Node) indentOf() int {
	if n.ex == nil {
		return 0
	}
	return n.ex.indent
}

func (n *Node) setIndent(i int) { e := n.ext(); e.indent, e.hasIndent = i, true }

type pendingInfo struct {
	transform string // e.g. "docutils.transforms.parts.Contents"
	details   map[string]any
}

const textTag = "#text"

// Nodes and small child lists are carved from pooled slabs: a document
// consists of many small nodes, and allocating them one by one dominates
// the cost of building the tree.
type nodeSlab struct {
	n     int
	nodes [64]Node
}

type childSlab struct {
	ptrs [256]*Node
	n    int
}

var (
	nodeSlabs  sync.Pool
	childSlabs sync.Pool
)

func allocNode() *Node {
	s, _ := nodeSlabs.Get().(*nodeSlab)
	if s == nil {
		s = new(nodeSlab)
	}
	n := &s.nodes[s.n]
	s.n++
	if s.n < len(s.nodes) {
		nodeSlabs.Put(s)
	}
	return n
}

// allocChildren returns an empty child list with capacity k.
func allocChildren(k int) []*Node {
	if k > 32 {
		return make([]*Node, 0, k)
	}
	s, _ := childSlabs.Get().(*childSlab)
	if s == nil || s.n+k > len(s.ptrs) {
		s = new(childSlab)
	}
	l := s.ptrs[s.n : s.n : s.n+k]
	s.n += k
	if s.n < len(s.ptrs) {
		childSlabs.Put(s)
	}
	return l
}

func newText(s string) *Node {
	n := allocNode()
	n.Tag, n.Text = textTag, s
	return n
}

func newElem(tag string, rawsource string, children ...*Node) *Node {
	n := allocNode()
	n.Tag, n.Rawsource = tag, rawsource
	if fixedTextElements[tag] {
		n.Attrs = map[string]any{"xml:space": "preserve"}
	}
	if len(children) > 0 {
		n.Children = allocChildren(len(children))
	}
	for _, c := range children {
		n.append(c)
	}
	return n
}

// newTextElem mirrors TextElement.__init__(rawsource, text, *children).
func newTextElem(tag, rawsource, text string, children ...*Node) *Node {
	n := newElem(tag, rawsource)
	k := len(children)
	if text != "" {
		k++
	}
	if k > 0 {
		n.Children = allocChildren(k)
	}
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
	if cap(n.Children) == 0 {
		n.Children = allocChildren(4)
	}
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
	if n == nil {
		return -1
	}
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
	if n == nil {
		return
	}
	if i := n.index(c); i >= 0 {
		n.removeAt(i)
	}
}

// replace replaces child old with the given nodes.
func (n *Node) replace(old *Node, news ...*Node) {
	if n == nil {
		return
	}
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
	if n.Parent == nil || n.Parent.index(n) < 0 {
		return // (Docutils raises an exception here)
	}
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

// Shared values for the class lists that the HTML writers add to very many
// nodes.  (Attribute lists are never modified in place.)
var (
	classesFirst     any = []string{"first"}
	classesLast      any = []string{"last"}
	classesFirstLast any = []string{"first", "last"}
)

func (n *Node) appendTo(key string, vals ...string) {
	if n.Attrs == nil {
		n.Attrs = make(map[string]any, 2)
	}
	old := n.list(key)
	if key == "classes" && len(vals) == 1 {
		switch {
		case len(old) == 0 && vals[0] == "first":
			n.Attrs[key] = classesFirst
			return
		case len(old) == 0 && vals[0] == "last":
			n.Attrs[key] = classesLast
			return
		case len(old) == 1 && old[0] == "first" && vals[0] == "last":
			n.Attrs[key] = classesFirstLast
			return
		}
	}
	n.Attrs[key] = append(old[:len(old):len(old)], vals...)
}

func (n *Node) removeFrom(key, val string) {
	l := n.list(key)
	for i, x := range l {
		if x == val {
			n.setAttr(key, append(l[:i:i], l[i+1:]...))
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
	if _, ok := n.Attrs[key]; ok {
		return true
	}
	return n.Tag != textTag && (key == "ids" || key == "classes" || key == "names" || key == "dupnames")
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

func (n *Node) setAttr(key string, v any) {
	if n.Attrs == nil {
		n.Attrs = make(map[string]any, 2)
	}
	n.Attrs[key] = v
}
func (n *Node) delAttr(key string) { delete(n.Attrs, key) }

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
		line := n.str("line")
		if v, ok := n.Attrs["line"]; ok && v == nil {
			line = "None"
		}
		return fmt.Sprintf("%s:%s: (%s/%s) %s", n.str("source"), line,
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
	c := allocNode()
	c.Tag, c.Rawsource = n.Tag, n.Rawsource
	if len(n.Attrs) > 0 {
		c.Attrs = make(map[string]any, len(n.Attrs))
	}
	for k, v := range n.Attrs {
		if l, ok := v.([]string); ok {
			c.Attrs[k] = append([]string{}, l...)
		} else {
			c.Attrs[k] = v
		}
	}
	c.doc = n.doc
	c.Source, c.hasSrc, c.Line, c.hasLine = n.Source, n.hasSrc, n.Line, n.hasLine
	if p := n.pend(); p != nil {
		c.ext().pending = p
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

// Tag classes recorded by Document.tagKinds.
const (
	kindTransition = 1 << iota
	kindSubstitutionRef
	kindAdmonition
)

// tagKinds reports which of a few rarely used element kinds occur in the
// parsed document, so that transforms for absent kinds need not walk the
// tree. None of these kinds is created by a transform, so the result of the
// first walk stays valid.
func (d *Document) tagKinds() int {
	if !d.kindsKnown {
		d.kinds = collectKinds(d.Root)
		d.kindsKnown = true
	}
	return d.kinds
}

func collectKinds(n *Node) int {
	k := 0
	for _, c := range n.Children {
		switch len(c.Tag) {
		case 3, 4, 5, 6, 7, 9, 10, 22:
			switch c.Tag {
			case "transition":
				k |= kindTransition
			case "substitution_reference":
				k |= kindSubstitutionRef
			default:
				if admonitionTags[c.Tag] {
					k |= kindAdmonition
				}
			}
		}
		if len(c.Children) != 0 {
			k |= collectKinds(c)
		}
	}
	return k
}

func (n *Node) findTag(tag string) []*Node {
	var out []*Node
	collectTag(n, tag, &out)
	return out
}

func collectTag(n *Node, tag string, out *[]*Node) {
	if n.Tag == tag {
		*out = append(*out, n)
	}
	for _, c := range n.Children {
		if len(c.Children) == 0 && c.Tag != tag {
			continue
		}
		collectTag(c, tag, out)
	}
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
	if n.ex == nil {
		return
	}
	if name != "" {
		if x := n.ex.expectByName[name]; x != nil {
			x.referenced = true
		}
	}
	if id != "" {
		if x := n.ex.expectByID[id]; x != nil {
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
	if pending := n.pend(); pending != nil {
		internals := []string{".. internal attributes:",
			"     .transform: " + pending.transform,
			"     .details:"}
		keys := make([]string, 0, len(pending.details))
		for k := range pending.details {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := pending.details[k]
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
	return whitespaceNormalizeName(pyLower(name))
}

func whitespaceNormalizeName(name string) string {
	// fast path: already normalized (single spaces between words)
	ok := name == "" || (name[0] != ' ' && name[len(name)-1] != ' ')
	for i := 0; ok && i < len(name); i++ {
		c := name[i]
		if c >= 0x80 || (c < ' ' && c != 0) || (c == ' ' && name[i+1] == ' ') {
			ok = false
		}
	}
	if ok {
		return name
	}
	return strings.Join(pySplit(name), " ")
}

func dupname(node *Node, name string) {
	node.appendTo("dupnames", name)
	node.removeFrom("names", name)
	node.referenced = true
}
