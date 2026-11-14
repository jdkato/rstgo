package rstgo

// Port of docutils.writers._html_base.HTMLTranslator together with the
// html4css1 and html5_polyglot specialisations.  Where the two writers
// differ, the code branches on t.v4 (html4css1).

import (
	"embed"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// The default stylesheets shipped with Docutils (public domain / BSD-2;
// see the headers of the individual files).
//
//go:embed css/*.css
var bundledCSS embed.FS

const docutilsVersion = "0.23"

type visitAction int

const (
	actProceed visitAction = iota
	actSkipNode
	actSkipChildren
	actSkipDeparture
)

type A = map[string]any

type htmlTranslator struct {
	doc      *Document
	settings *settings
	v4       bool

	doctype          string
	contentType      string
	documentTag      string
	documentTagClass string
	initialHeader    int
	imageLoading     string
	mathOutput       string
	mathOptions      string

	body           []byte // output so far
	last           int    // start of the most recently written piece
	bodyPrefix     []string
	bodyPreDocinfo []string
	bodySuffix     []string
	docinfo        []string
	footer         []string
	fragment       []byte
	head           []string
	headPrefix     []string
	header         []string
	htmlBody       []byte
	htmlHead       []string
	htmlProlog     []string
	htmlSubtitle   []string
	htmlTitle      []string
	meta           []string
	stylesheet     []string
	title          []string
	subtitle       []string

	context          []any
	sectionLevel     int
	colspecs         []*Node
	compactP         bool
	compactSimple    bool
	compactFieldList bool
	inDocinfo        bool
	inSidebar        bool
	inDocumentTitle  int
	inMailto         bool
	authorInAuthors  bool
	mathHeader       []string
	messages         []*Node

	mathClasses map[string]*[]string
	stubs       map[*Node][]bool
	column      map[*Node]int
	html5tag    map[*Node]string
}

func newHTMLTranslator(doc *Document) *htmlTranslator {
	st := doc.settings
	t := &htmlTranslator{doc: doc, settings: st, v4: st.writer == "html4",
		stubs: map[*Node][]bool{}, column: map[*Node]int{}, html5tag: map[*Node]string{}}
	if t.v4 {
		t.doctype = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN"` +
			` "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">` + "\n"
		t.contentType = `<meta http-equiv="Content-Type" content="text/html; charset=%s" />` + "\n"
		t.documentTag, t.documentTagClass = "div", "document"
	} else {
		t.doctype = "<!DOCTYPE html>\n"
		t.contentType = `<meta charset="%s" />` + "\n"
		t.documentTag = "main"
	}
	t.mathClasses = map[string]*[]string{"latex": {"math"}, "mathjax": {"math"},
		"html": {"formula"}, "mathml": {}, "problematic": {"math", "problematic"}}
	t.initialHeader = st.initialHeaderLevel
	t.imageLoading = st.imageLoading
	mo := pySplitN(st.mathOutput, 1)
	if len(mo) > 0 {
		t.mathOutput = strings.ToLower(mo[0])
		if len(mo) > 1 {
			t.mathOptions = mo[1]
		}
	}
	t.bodyPrefix = []string{"</head>\n<body>\n"}
	t.bodySuffix = []string{"</body>\n</html>\n"}
	t.htmlHead = []string{t.contentType}
	t.meta = []string{`<meta name="generator" content="Docutils ` + docutilsVersion +
		`: https://docutils.sourceforge.io/" />` + "\n"}
	for _, p := range st.stylesheets {
		t.stylesheet = append(t.stylesheet, t.stylesheetCall(p, st.stylesheetIsPath))
	}
	if st.xmlDeclaration {
		t.headPrefix = append(t.headPrefix, fmt.Sprintf("<?xml version=\"1.0\" encoding=\"%s\"?>\n", st.outputEncoding))
		t.htmlProlog = append(t.htmlProlog, "<?xml version=\"1.0\" encoding=\"%s\"?>\n")
	}
	t.meta = append([]string{fmt.Sprintf(t.contentType, st.outputEncoding)}, t.meta...)
	t.compactP = true
	t.body = make([]byte, 0, st.inputLen+st.inputLen/2+1024)
	if !t.v4 {
		t.meta = append(t.meta, `<meta name="viewport" content="width=device-width, initial-scale=1" />`+"\n")
	}
	return t
}

// findStylesheet resolves a stylesheet path like utils.find_file_in_dirs
// with the writer's default stylesheet directories; bundled reports that the
// name refers to one of the embedded Docutils stylesheets.
func findStylesheet(p string) (resolved string, bundled bool) {
	if filepath.IsAbs(p) {
		return p, false
	}
	if _, err := os.Stat(p); err == nil {
		return p, false
	}
	if _, err := bundledCSS.Open("css/" + p); err == nil {
		return p, true
	}
	return p, false
}

func (t *htmlTranslator) stylesheetCall(p string, adjustPath bool) string {
	bundled := false
	if adjustPath {
		p, bundled = findStylesheet(p)
	}
	if t.settings.embedStylesheet {
		var content []byte
		var err error
		if bundled {
			content, err = bundledCSS.ReadFile("css/" + p)
		} else {
			content, err = os.ReadFile(p)
		}
		if err != nil {
			msg := "Cannot embed stylesheet: " + strings.TrimPrefix(ioErrorString(err, p), "InputError: ")
			t.doc.reporter.error(msg, nil)
			return "<--- " + msg + " --->\n"
		}
		return "<style type=\"text/css\">\n\n" + string(content) + "\n</style>\n"
	}
	if adjustPath && !bundled {
		p = relativePathFrom(t.settings.destPath, p)
	}
	return `<link rel="stylesheet" href="` + t.encode(p) + `" type="text/css" />` + "\n"
}

// relativePathFrom mirrors utils.relative_path(source, target).
func relativePathFrom(source, target string) string {
	cwd, _ := os.Getwd()
	abs := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		return filepath.Clean(p)
	}
	if source == "" {
		source = "dummy_file"
	}
	sourceParts := strings.Split(abs(source), string(os.PathSeparator))
	targetParts := strings.Split(abs(target), string(os.PathSeparator))
	if len(sourceParts) < 2 || len(targetParts) < 2 || sourceParts[0] != targetParts[0] || sourceParts[1] != targetParts[1] {
		return strings.Join(targetParts, "/")
	}
	i := 0
	for i < len(sourceParts) && i < len(targetParts) && sourceParts[i] == targetParts[i] {
		i++
	}
	sourceParts, targetParts = sourceParts[i:], targetParts[i:]
	var parts []string
	for j := 0; j < len(sourceParts)-1; j++ {
		parts = append(parts, "..")
	}
	return strings.Join(append(parts, targetParts...), "/")
}

func (t *htmlTranslator) encode(text string) string {
	clean := true
	for i := 0; i < len(text) && clean; i++ {
		switch text[i] {
		case '&', '<', '"', '>', '@', 0xc2:
			clean = false
		}
	}
	if clean {
		return text
	}
	var b strings.Builder
	for _, r := range text {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '"':
			b.WriteString("&quot;")
		case '>':
			b.WriteString("&gt;")
		case '@':
			b.WriteString("&#64;")
		case 0xa0:
			if t.v4 {
				b.WriteString("&nbsp;")
			} else {
				b.WriteRune(r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (t *htmlTranslator) attval(text string) string {
	if strings.ContainsAny(text, "\n\r\t\v\f") {
		text = strings.Map(func(r rune) rune {
			switch r {
			case '\n', '\r', '\t', '\v', '\f':
				return ' '
			}
			return r
		}, text)
	}
	encoded := t.encode(text)
	if t.inMailto && t.settings.cloakEmailAddresses {
		encoded = strings.ReplaceAll(encoded, "%40", "&#37;&#52;&#48;")
		encoded = strings.ReplaceAll(encoded, ".", "&#46;")
	}
	return encoded
}

// out appends pieces to the body.  (Docutils keeps a list of strings; the
// few places that depend on piece boundaries use t.last.)
func (t *htmlTranslator) out(s ...string) {
	for _, x := range s {
		t.last = len(t.body)
		t.body = append(t.body, x...)
	}
}

// outTag writes a start tag directly to the body.
func (t *htmlTranslator) outTag(node *Node, tagname, suffix string, atts A) {
	t.last = len(t.body)
	t.body = t.appendTag(t.body, node, tagname, suffix, false, atts)
}

func (t *htmlTranslator) outEmptyTag(node *Node, tagname, suffix string, atts A) {
	t.last = len(t.body)
	t.body = t.appendTag(t.body, node, tagname, suffix, true, atts)
}

func (t *htmlTranslator) push(v any) { t.context = append(t.context, v) }
func (t *htmlTranslator) pop() any {
	v := t.context[len(t.context)-1]
	t.context = t.context[:len(t.context)-1]
	return v
}
func (t *htmlTranslator) popStr() string { return t.pop().(string) }

var dummyNode = &Node{Tag: "dummy", Attrs: map[string]any{}}

// starttag mirrors HTMLTranslator.starttag.  Attribute names are given in
// lower case; "class" is a space separated string, "classes" and "ids" are
// []string.
func (t *htmlTranslator) starttag(node *Node, tagname, suffix string, atts A) string {
	return t.tag(node, tagname, suffix, false, atts)
}

func (t *htmlTranslator) emptytag(node *Node, tagname, suffix string, atts A) string {
	return t.tag(node, tagname, suffix, true, atts)
}

func (t *htmlTranslator) tag(node *Node, tagname, suffix string, empty bool, attributes A) string {
	return string(t.appendTag(nil, node, tagname, suffix, empty, attributes))
}

type tagAttr struct{ name, value string }

// appendTag appends the tag to dst.  It must not retain attributes.
func (t *htmlTranslator) appendTag(dst []byte, node *Node, tagname, suffix string, empty bool, attributes A) []byte {
	tagname = strings.ToLower(tagname)
	var cbuf [8]string
	classes := cbuf[:0]
	var classStr string
	var classesOf *Node
	var classesPtr *[]string
	var extraIDs []string
	var abuf [8]tagAttr
	atts := abuf[:0]
	for k, v := range attributes {
		switch k {
		case "classes":
			if c, ok := v.([]string); ok {
				classes = append(classes, c...)
			}
		case "class":
			classStr, _ = v.(string)
		// Docutils passes some class lists by reference and extends them
		// in place (the list of a parent node, or the writer's shared math
		// class lists); reproduce that aliasing.
		case "classes_of":
			classesOf, _ = v.(*Node)
		case "classes_ptr":
			classesPtr, _ = v.(*[]string)
		case "ids":
			extraIDs, _ = v.([]string)
		default:
			var val string
			switch x := v.(type) {
			case []string:
				val = strings.Join(x, " ")
			case string:
				val = x
			case int:
				val = strconv.Itoa(x)
			case bool:
				if x {
					val = "True"
				} else {
					val = "False"
				}
			case float64:
				val = pyFloatStr(x)
			default:
				val = fmt.Sprint(v)
			}
			atts = append(atts, tagAttr{strings.ToLower(k), val})
		}
	}
	if classesOf != nil {
		classes = append(classes[:0], classesOf.list("classes")...)
	} else if classesPtr != nil {
		classes = append(classes[:0], *classesPtr...)
	}
	lang := ""
	addClass := func(cls string) {
		if strings.HasPrefix(cls, "language-") {
			if lang == "" {
				lang = strings.TrimPrefix(cls, "language-")
			}
		} else if cls != "" && !containsStr(classes, cls) {
			classes = append(classes, cls)
		}
	}
	for _, cls := range node.list("classes") {
		if pyStrip(cls) == "" {
			continue
		}
		addClass(cls)
	}
	if classStr != "" {
		if strings.IndexFunc(classStr, isPySpace) < 0 {
			addClass(classStr)
		} else {
			for _, cls := range pySplit(classStr) {
				addClass(cls)
			}
		}
	}
	if classesOf != nil {
		classesOf.setAttr("classes", append([]string{}, classes...))
	} else if classesPtr != nil {
		*classesPtr = append([]string{}, classes...)
	}
	if lang != "" {
		replaced := false
		for i := range atts {
			if atts[i].name == "lang" {
				atts[i].value, replaced = lang, true
			}
		}
		if !replaced {
			atts = append(atts, tagAttr{"lang", lang})
		}
	}
	if node.Tag == "table" {
		kept := classes[:0]
		for _, c := range classes {
			if c != "colwidths-auto" && c != "colwidths-given" && c != "colwidths-grid" {
				kept = append(kept, c)
			}
		}
		classes = kept
	}
	if len(extraIDs) > 0 {
		node.appendTo("ids", extraIDs...)
	}
	ids := node.list("ids")
	if len(ids) > 0 {
		atts = append(atts, tagAttr{"id", ids[0]})
		for _, id := range ids[1:] {
			if empty || sequentialTags[node.Tag] || node.Tag == "docinfo" || node.Tag == "table" {
				dst = append(dst, `<span id="`...)
				dst = append(dst, id...)
				dst = append(dst, `"></span>`...)
			}
		}
	}
	if len(classes) > 0 {
		atts = append(atts, tagAttr{"class", ""})
	}
	// sort by attribute name
	for i := 1; i < len(atts); i++ {
		for j := i; j > 0 && atts[j].name < atts[j-1].name; j-- {
			atts[j], atts[j-1] = atts[j-1], atts[j]
		}
	}
	dst = append(dst, '<')
	dst = append(dst, tagname...)
	for _, a := range atts {
		dst = append(dst, ' ')
		dst = append(dst, a.name...)
		dst = append(dst, `="`...)
		if a.name == "class" && len(classes) > 0 {
			for i, c := range classes {
				if i > 0 {
					dst = append(dst, ' ')
				}
				dst = append(dst, t.attval(c)...)
			}
		} else {
			dst = append(dst, t.attval(a.value)...)
		}
		dst = append(dst, '"')
	}
	if empty {
		dst = append(dst, " /"...)
	}
	dst = append(dst, '>')
	dst = append(dst, suffix...)
	if len(ids) > 1 && !(empty || sequentialTags[node.Tag] || node.Tag == "docinfo" || node.Tag == "table") {
		for _, id := range ids[1:] {
			dst = append(dst, `<span id="`...)
			dst = append(dst, id...)
			dst = append(dst, `"></span>`...)
		}
	}
	return dst
}

func (t *htmlTranslator) reportMessages(node *Node) {
	if node.Parent != nil && node.Parent.is("system_message", "entry") {
		return
	}
	for len(t.messages) > 0 {
		message := t.messages[0]
		t.messages = t.messages[1:]
		if lv, _ := message.get("level").(int); t.settings.reportLevel <= lv {
			t.walkabout(message)
		}
	}
}

func visibleChildren(node *Node) []*Node {
	var out []*Node
	for _, c := range node.Children {
		if !invisibleTags[c.Tag] {
			out = append(out, c)
		}
	}
	return out
}

func (t *htmlTranslator) setClassOnChild(node *Node, class string, index int) {
	var child *Node
	switch index {
	case 0:
		for _, c := range node.Children {
			if !invisibleTags[c.Tag] {
				child = c
				break
			}
		}
	case -1:
		for i := len(node.Children) - 1; i >= 0; i-- {
			if c := node.Children[i]; !invisibleTags[c.Tag] {
				child = c
				break
			}
		}
	default:
		children := visibleChildren(node)
		if i := pyIndex(index, len(children)); i >= 0 {
			child = children[i]
		}
	}
	if child == nil || child.IsText() {
		return
	}
	child.appendTo("classes", class)
}

func (t *htmlTranslator) setFirstLast(node *Node) {
	t.setClassOnChild(node, "first", 0)
	t.setClassOnChild(node, "last", -1)
}

func hasClass(n *Node, c string) bool { return containsStr(n.list("classes"), c) }

// walkabout mirrors Node.walkabout for this translator.
func (t *htmlTranslator) walkabout(n *Node) {
	if n.IsText() {
		encoded := t.encode(n.astext())
		if t.inMailto && t.settings.cloakEmailAddresses {
			encoded = strings.ReplaceAll(encoded, "&#64;", "<span>&#64;</span>")
			encoded = strings.ReplaceAll(encoded, ".", "<span>&#46;</span>")
		}
		t.out(encoded)
		return
	}
	act := t.visit(n)
	if act == actSkipNode {
		return
	}
	if act != actSkipChildren {
		for _, c := range n.Children {
			t.walkabout(c)
		}
	}
	if act != actSkipDeparture {
		t.depart(n)
	}
}

// ---------------------------------------------------------------- simple list check

// checkSimpleList mirrors check_simple_list / SimpleListChecker.  (Both
// writers use the checker class of _html_base.)
func checkSimpleList(node *Node) bool {
	var walk func(n *Node) bool // false: NodeFound
	listItemOK := func(n *Node, withFieldList bool) bool {
		children := visibleChildren(n)
		if len(children) > 0 && children[0].Tag == "paragraph" {
			last := children[len(children)-1]
			if last.is("bullet_list", "enumerated_list") || (withFieldList && last.Tag == "field_list") {
				children = children[:len(children)-1]
			}
		}
		return len(children) <= 1
	}
	walk = func(n *Node) bool {
		if n.IsText() {
			return true
		}
		switch n.Tag {
		case "paragraph", "author", "copyright", "date", "organization", "status",
			"term", "field_name", "comment", "substitution_definition", "target", "pending":
			return true // ignore_node (SkipNode)
		case "bullet_list", "enumerated_list", "docinfo", "contact", "definition_list",
			"definition_list_item", "classifier", "field_list", "field":
			// pass_node
		case "list_item", "authors", "address", "version", "definition", "field_body":
			if !listItemOK(n, true) {
				return false
			}
		default:
			return false
		}
		for _, c := range n.Children {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	return walk(node)
}

func (t *htmlTranslator) isCompactable(node *Node) bool {
	if t.v4 {
		return hasClass(node, "compact") ||
			(t.settings.compactLists && !hasClass(node, "open") &&
				(t.compactSimple || hasClass(node.Parent, "contents") || checkSimpleList(node)))
	}
	if hasClass(node, "compact") {
		return true
	}
	if hasClass(node, "open") {
		return false
	}
	if node.is("field_list", "definition_list") && !t.settings.compactFieldLists {
		return false
	}
	if node.is("enumerated_list", "bullet_list") && !t.settings.compactLists {
		return false
	}
	if hasClass(node.Parent, "contents") {
		return true
	}
	return checkSimpleList(node)
}

// nextSibling mirrors node.next_node(descend=False, siblings=True).
func nextSibling(node *Node) *Node {
	if node.Parent == nil {
		return nil
	}
	i := node.Parent.index(node)
	if i+1 < len(node.Parent.Children) {
		return node.Parent.Children[i+1]
	}
	return nil
}

var docinfoLabels = map[string]string{
	"author": "Author", "authors": "Authors", "organization": "Organization",
	"address": "Address", "contact": "Contact", "version": "Version",
	"revision": "Revision", "status": "Status", "date": "Date", "copyright": "Copyright",
}

func (t *htmlTranslator) visitDocinfoItem(node *Node, name string, meta bool) {
	if meta {
		t.meta = append(t.meta, fmt.Sprintf("<meta name=\"%s\" content=\"%s\" />\n", name, t.attval(node.astext())))
	}
	if t.v4 {
		t.outTag(node, "tr", "", nil)
		t.out(fmt.Sprintf("<th class=\"docinfo-name\">%s:</th>\n<td>", docinfoLabels[name]))
		if len(node.Children) > 0 {
			if f := node.Children[0]; !f.IsText() {
				f.appendTo("classes", "first")
			}
			if l := node.Children[len(node.Children)-1]; !l.IsText() {
				l.appendTo("classes", "last")
			}
		}
		return
	}
	t.out(fmt.Sprintf("<dt class=\"%s\">%s<span class=\"colon\">:</span></dt>\n", name, docinfoLabels[name]))
	t.outTag(node, "dd", "", A{"class": name})
}

func (t *htmlTranslator) departDocinfoItem() {
	if t.v4 {
		t.out("</td></tr>\n")
	} else {
		t.out("</dd>\n")
	}
}

func (t *htmlTranslator) sectionTitleTags(node *Node) (string, string) {
	hLevel := t.sectionLevel + t.initialHeader - 1
	lvl := hLevel
	if lvl > 6 {
		lvl = 6
	}
	tagname := fmt.Sprintf("h%d", lvl)
	var startTag, closeTag string
	if t.v4 {
		var classes []string
		if len(node.Parent.Children) >= 2 && node.Parent.Children[1].Tag == "subtitle" {
			classes = append(classes, "with-subtitle")
		}
		if hLevel > 6 {
			classes = append(classes, fmt.Sprintf("h%d", hLevel))
		}
		startTag = t.starttag(node, tagname, "", A{"classes": classes})
		if node.has("refid") {
			startTag += t.starttag(dummyNode, "a", "", A{"class": "toc-backref", "href": "#" + node.str("refid")})
			closeTag = "</a></" + tagname + ">\n"
		} else {
			closeTag = "</" + tagname + ">\n"
		}
		return startTag, closeTag
	}
	atts := A{}
	if hLevel > 6 {
		atts["aria-level"] = hLevel
	}
	startTag = t.starttag(node, tagname, "", atts)
	if node.has("refid") {
		startTag += t.starttag(dummyNode, "a", "", A{"class": "toc-backref", "role": "doc-backlink", "href": "#" + node.str("refid")})
		closeTag = "</a></" + tagname + ">\n"
	} else {
		closeTag = "</" + tagname + ">\n"
	}
	ids := node.Parent.list("ids")
	if len(ids) > 0 && t.settings.sectionSelfLink && node.Parent.Tag != "document" {
		selfLink := `<a class="self-link" title="link to this section" href="#` + ids[len(ids)-1] + `"></a>`
		closeTag = strings.ReplaceAll(closeTag, "</h", selfLink+"</h")
	}
	return startTag, closeTag
}

var mimeTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jpe": "image/jpeg",
	".gif": "image/gif", ".svg": "image/svg+xml", ".mp4": "video/mp4", ".webm": "video/webm",
	".ogg": "audio/ogg", ".ogv": "video/ogg", ".webp": "image/webp", ".bmp": "image/bmp",
	".ico": "image/vnd.microsoft.icon", ".tif": "image/tiff", ".tiff": "image/tiff",
	".pdf": "application/pdf", ".avif": "image/avif", ".apng": "image/apng",
	".mov": "video/quicktime", ".swf": "application/vnd.adobe.flash.movie",
	".eps": "application/postscript", ".txt": "text/plain", ".html": "text/html",
}

// guessMimeType approximates mimetypes.guess_type(uri)[0].
func guessMimeType(uri string) string {
	p := uri
	if i := strings.Index(p, "://"); i > 1 {
		rest := p[i+3:]
		if j := strings.IndexAny(rest, "/"); j >= 0 {
			p = rest[j:]
		} else {
			p = ""
		}
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 && strings.Contains(uri, ":") {
		p = p[:i]
	}
	ext := path.Ext(p)
	if m, ok := mimeTypes[ext]; ok {
		return m
	}
	return mimeTypes[strings.ToLower(ext)]
}

func isVideoType(m string) bool { return m == "video/mp4" || m == "video/webm" || m == "video/ogg" }

func pyRound(f float64) int { return int(math.RoundToEven(f)) }

func fmtG(f float64) string { return strconv.FormatFloat(f, 'g', 6, 64) }
