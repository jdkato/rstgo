package rstgo

import (
	"fmt"
	"io"
	"strings"
)

// Document is a parsed reStructuredText document tree (docutils.nodes.document)
// together with its bookkeeping tables.
type Document struct {
	// Root is the <document> element.
	Root *Node

	settings *settings
	reporter *reporter

	currentSource    string
	hasCurrentSource bool
	currentLine      int
	hasCurrentLine   bool

	indirectTargets   []*Node
	substitutionDefs  map[string]*Node
	substitutionNames map[string]string
	refnames          map[string][]*Node
	refnameOrder      []string
	refids            map[string][]*Node
	names             map[string]*Node // value nil: only dupname
	ids               map[string]*Node
	nameids           map[string]string // "" stands for None
	nametypes         map[string]bool
	footnoteRefs      map[string][]*Node
	citationRefs      map[string][]*Node
	autofootnotes     []*Node
	autofootnoteRefs  []*Node
	symbolFootnotes   []*Node
	symbolFootnoteRef []*Node
	footnotes         []*Node
	citations         []*Node
	autofootnoteStart int
	symbolFootStart   int
	idCounter         map[string]int
	parseMessages     []*Node
	transformMessages []*Node
	pendings          []pendingEntry
	includeLog        []includeEntry
	decoration        *Node
	serial            int
	extraTransforms   []transformEntry
}

type includeEntry struct {
	path string
	clip string
}

func newDocument(sourcePath string, st *settings, stderr io.Writer) *Document {
	d := &Document{
		settings:          st,
		substitutionDefs:  map[string]*Node{},
		substitutionNames: map[string]string{},
		refnames:          map[string][]*Node{},
		refids:            map[string][]*Node{},
		names:             map[string]*Node{},
		ids:               map[string]*Node{},
		nameids:           map[string]string{},
		nametypes:         map[string]bool{},
		footnoteRefs:      map[string][]*Node{},
		citationRefs:      map[string][]*Node{},
		autofootnoteStart: 1,
		idCounter:         map[string]int{},
	}
	d.reporter = &reporter{source: sourcePath, reportLevel: st.reportLevel,
		haltLevel: st.haltLevel, stream: stderr}
	d.Root = newElem("document", "")
	d.Root.Attrs["source"] = sourcePath
	d.Root.doc = d
	d.noteSource(sourcePath, true, -1, true)
	return d
}

func (d *Document) noteSource(source string, hasSource bool, offset int, hasOffset bool) {
	d.currentSource, d.hasCurrentSource = source, hasSource
	if hasOffset {
		d.currentLine, d.hasCurrentLine = offset+1, true
	} else {
		d.currentLine, d.hasCurrentLine = 0, false
	}
}

// PseudoXML renders the tree like Docutils' pseudo-XML writer.
func (d *Document) PseudoXML() string {
	var b strings.Builder
	d.Root.pformat(&b, 0)
	return b.String()
}

func (d *Document) setID(node, msgnode *Node, suggestedPrefix string) string {
	if len(node.list("ids")) == 0 {
		node.appendTo("ids", d.createID(node, suggestedPrefix))
	}
	id := ""
	for _, id = range node.list("ids") {
		if _, ok := d.ids[id]; !ok {
			d.ids[id] = node
		}
		if d.ids[id] != node {
			msg := d.reporter.error(fmt.Sprintf(`Duplicate ID: "%s" used by %s and %s`,
				id, d.ids[id].starttag(), node.starttag()), nil, baseNode(node))
			if msgnode != nil {
				msgnode.append(msg)
			}
		}
	}
	for _, name := range node.list("names") {
		d.nameids[name] = id
	}
	return id
}

func (d *Document) createID(node *Node, suggestedPrefix string) string {
	idPrefix := d.settings.idPrefix
	autoIDPrefix := d.settings.autoIDPrefix
	baseID, id := "", ""
	found := false
	for _, name := range node.list("names") {
		if idPrefix != "" {
			baseID = makeID("x" + name)[1:]
		} else {
			baseID = makeID(name)
		}
		id = idPrefix + baseID
		if baseID != "" {
			if _, ok := d.ids[id]; !ok {
				found = true
				break
			}
		}
	}
	if found {
		return id
	}
	var prefix string
	dn := node.list("dupnames")
	if baseID != "" && strings.HasSuffix(autoIDPrefix, "%") {
		prefix = id + "-"
	} else if len(dn) > 0 && strings.HasSuffix(autoIDPrefix, "%") && makeID(dn[0]) != "" {
		prefix = makeID(dn[0]) + "-"
	} else {
		prefix = idPrefix + autoIDPrefix
		if strings.HasSuffix(prefix, "%") {
			p := suggestedPrefix
			if p == "" {
				p = makeID(node.Tag)
			}
			prefix = prefix[:len(prefix)-1] + p + "-"
		}
	}
	for {
		d.idCounter[prefix]++
		id = fmt.Sprintf("%s%d", prefix, d.idCounter[prefix])
		if _, ok := d.ids[id]; !ok {
			break
		}
	}
	return id
}

func (d *Document) setDuplicateName(node *Node, name string, msgnode *Node, explicit bool) {
	oldNode := d.names[name]
	oldExplicit := d.nametypes[name]
	level := 0
	d.nametypes[name] = oldExplicit || explicit
	var s string
	if oldNode != nil && ((node.has("refname") && node.str("refname") == oldNode.str("refname") && oldNode.has("refname")) ||
		(node.has("refuri") && oldNode.has("refuri") && node.str("refuri") == oldNode.str("refuri"))) {
		level = 1
		ref := node.str("refuri")
		if ref == "" {
			ref = node.str("refname")
		}
		s = fmt.Sprintf(`Duplicate name "%s" for external target "%s".`, name, ref)
		dupname(node, name)
	} else if explicit {
		if oldExplicit {
			level = 2
			s = fmt.Sprintf(`Duplicate explicit target name: "%s".`, name)
			dupname(node, name)
			if oldNode != nil {
				dupname(oldNode, name)
				d.names[name] = nil
				d.nameids[name] = ""
			}
		} else {
			d.names[name] = node
			if oldNode != nil {
				level = 1
				s = fmt.Sprintf(`Target name overrides implicit target name "%s".`, name)
				dupname(oldNode, name)
			}
		}
	} else {
		level = 1
		s = fmt.Sprintf(`Duplicate implicit target name: "%s".`, name)
		dupname(node, name)
		if oldNode != nil && !oldExplicit {
			dupname(oldNode, name)
			d.names[name] = nil
			d.nameids[name] = ""
			d.setID(oldNode, nil, "")
		}
	}
	if level > 0 {
		var backrefs []string
		if !(node.Tag == "target" && len(node.Children) == 0) {
			backrefs = []string{d.setID(node, nil, "")}
		}
		msg := d.reporter.systemMessage(level, s, nil, withBackrefs(backrefs), baseNode(node))
		if msgnode != nil {
			msgnode.append(msg)
		}
	}
}

func (d *Document) noteNames(node, msgnode *Node, explicit bool) {
	for _, name := range append([]string{}, node.list("names")...) {
		if old, ok := d.names[name]; ok && old != node {
			d.setDuplicateName(node, name, msgnode, explicit)
		} else {
			d.names[name] = node
			if _, ok := d.nametypes[name]; !ok {
				d.nametypes[name] = explicit
			}
		}
	}
}

func (d *Document) hasName(name string) bool { _, ok := d.names[name]; return ok }

func (d *Document) noteImplicitTarget(target, msgnode *Node) {
	d.noteNames(target, msgnode, false)
	if d.settings.legacyIDs {
		d.setID(target, msgnode, "")
	}
}

func (d *Document) noteExplicitTarget(target, msgnode *Node) {
	d.noteNames(target, msgnode, true)
	d.setID(target, msgnode, "")
}

func (d *Document) noteRefname(node *Node) {
	name := node.str("refname")
	if _, ok := d.refnames[name]; !ok {
		d.refnameOrder = append(d.refnameOrder, name)
	}
	d.refnames[name] = append(d.refnames[name], node)
}

func (d *Document) noteRefid(node *Node) {
	id := node.str("refid")
	d.refids[id] = append(d.refids[id], node)
}

func (d *Document) noteIndirectTarget(target *Node) {
	d.indirectTargets = append(d.indirectTargets, target)
	if len(target.list("names")) > 0 {
		d.noteRefname(target)
	}
}

func (d *Document) noteAnonymousTarget(target *Node) { d.setID(target, nil, "") }

func (d *Document) noteAutofootnote(fn *Node) {
	d.setID(fn, nil, "")
	d.autofootnotes = append(d.autofootnotes, fn)
}
func (d *Document) noteAutofootnoteRef(ref *Node) {
	d.setID(ref, nil, "")
	d.autofootnoteRefs = append(d.autofootnoteRefs, ref)
}
func (d *Document) noteSymbolFootnote(fn *Node) {
	d.setID(fn, nil, "")
	d.symbolFootnotes = append(d.symbolFootnotes, fn)
}
func (d *Document) noteSymbolFootnoteRef(ref *Node) {
	d.setID(ref, nil, "")
	d.symbolFootnoteRef = append(d.symbolFootnoteRef, ref)
}
func (d *Document) noteFootnote(fn *Node) {
	d.setID(fn, nil, "")
	d.footnotes = append(d.footnotes, fn)
}
func (d *Document) noteFootnoteRef(ref *Node) {
	d.setID(ref, nil, "")
	name := ref.str("refname")
	d.footnoteRefs[name] = append(d.footnoteRefs[name], ref)
	d.noteRefname(ref)
}
func (d *Document) noteCitation(c *Node) { d.citations = append(d.citations, c) }
func (d *Document) noteCitationRef(ref *Node) {
	d.setID(ref, nil, "")
	name := ref.str("refname")
	d.citationRefs[name] = append(d.citationRefs[name], ref)
	d.noteRefname(ref)
}

func (d *Document) noteSubstitutionDef(subdef *Node, defName string, msgnode *Node) {
	name := whitespaceNormalizeName(defName)
	if old, ok := d.substitutionDefs[name]; ok {
		msg := d.reporter.error(fmt.Sprintf(`Duplicate substitution definition name: "%s".`, name),
			nil, baseNode(subdef))
		if msgnode != nil {
			msgnode.append(msg)
		}
		dupname(old, name)
	}
	d.substitutionDefs[name] = subdef
	d.substitutionNames[fullyNormalizeName(name)] = name
}

func (d *Document) noteSubstitutionRef(subref *Node, refname string) {
	subref.setAttr("refname", whitespaceNormalizeName(refname))
}

type pendingEntry struct {
	node     *Node
	priority int
	serial   int
}

func (d *Document) notePending(p *Node, priority int) {
	d.serial++
	d.pendings = append(d.pendings, pendingEntry{p, priority, d.serial})
}

func (d *Document) getDecoration() *Node {
	if d.decoration == nil {
		d.decoration = newElem("decoration", "")
		idx := d.Root.firstChildNotMatching(func(n *Node) bool { return n.isTitular() || n.Tag == "meta" })
		if idx < 0 {
			d.Root.append(d.decoration)
		} else {
			d.Root.insert(idx, d.decoration)
		}
	}
	return d.decoration
}

func decorationHeader(dec *Node) *Node {
	if len(dec.Children) == 0 || dec.Children[0].Tag != "header" {
		dec.insert(0, newElem("header", ""))
	}
	return dec.Children[0]
}

func decorationFooter(dec *Node) *Node {
	if len(dec.Children) == 0 || dec.Children[len(dec.Children)-1].Tag != "footer" {
		dec.append(newElem("footer", ""))
	}
	return dec.Children[len(dec.Children)-1]
}

// ---------------------------------------------------------------- make_id

var nonIDTranslate = map[rune]string{
	0x00f8: "o", 0x0111: "d", 0x0127: "h", 0x0131: "i", 0x0142: "l",
	0x0167: "t", 0x0180: "b", 0x0183: "b", 0x0188: "c", 0x018c: "d",
	0x0192: "f", 0x0199: "k", 0x019a: "l", 0x019e: "n", 0x01a5: "p",
	0x01ab: "t", 0x01ad: "t", 0x01b4: "y", 0x01b6: "z", 0x01e5: "g",
	0x0225: "z", 0x0234: "l", 0x0235: "n", 0x0236: "t", 0x0237: "j",
	0x023c: "c", 0x023f: "s", 0x0240: "z", 0x0247: "e", 0x0249: "j",
	0x024b: "q", 0x024d: "r", 0x024f: "y",
	// digraphs
	0x00df: "sz", 0x00e6: "ae", 0x0153: "oe", 0x0238: "db", 0x0239: "qp",
}

// makeID mirrors nodes.make_id.
func makeID(s string) string {
	s = pyLower(s)
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 {
			b.WriteRune(r)
		} else if t, ok := nonIDTranslate[r]; ok {
			b.WriteString(t)
		} else if t, ok := nfkdASCII[r]; ok {
			b.WriteString(t)
		} else if isPySpace(r) {
			b.WriteByte(' ')
		}
	}
	s = strings.Join(pySplit(b.String()), " ")
	// replace runs of non [a-z0-9] with "-"
	var o strings.Builder
	inRun := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			o.WriteByte(c)
			inRun = false
		} else if !inRun {
			o.WriteByte('-')
			inRun = true
		}
	}
	id := o.String()
	// strip leading [-0-9]+ and trailing -+
	i := 0
	for i < len(id) && (id[i] == '-' || (id[i] >= '0' && id[i] <= '9')) {
		i++
	}
	id = id[i:]
	return strings.TrimRight(id, "-")
}
