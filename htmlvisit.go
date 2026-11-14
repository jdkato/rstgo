package rstgo

import (
	"fmt"
	"path"
	"strings"
)

var attributionFormats = map[string][2]string{
	"dash": {"—", ""}, "parentheses": {"(", ")"}, "parens": {"(", ")"}, "none": {"", ""},
}

var html5InlineTags = []string{"code", "kbd", "dfn", "samp", "var", "bdi", "del",
	"ins", "mark", "small", "b", "i", "q", "s", "u"}

func (t *htmlTranslator) visit(node *Node) visitAction {
	switch node.Tag {
	case "abbreviation":
		t.out(t.starttag(node, "abbr", "", nil))
	case "acronym":
		if t.v4 {
			t.out(t.starttag(node, "acronym", "", nil))
		} else {
			t.out(t.starttag(node, "abbr", "", nil))
		}
	case "address":
		t.visitDocinfoItem(node, "address", false)
		if t.v4 {
			t.out(t.starttag(node, "pre", "\n", A{"class": "address"}))
		} else {
			t.out(t.starttag(node, "pre", "", A{"class": "address"}))
		}
	case "admonition":
		if t.v4 {
			node.Attrs["classes"] = append([]string{"admonition"}, node.list("classes")...)
			t.out(t.starttag(node, "div", "\n", nil))
			t.setFirstLast(node)
		} else {
			t.out(t.starttag(node, "aside", "\n", A{"classes": []string{"admonition"}}))
		}
	case "attribution":
		f, ok := attributionFormats[t.settings.attribution]
		if !ok {
			f = attributionFormats["dash"]
		}
		prefix := f[0]
		if t.v4 && prefix == "—" {
			prefix = "&mdash;"
		}
		t.push(f[1])
		t.out(t.starttag(node, "p", prefix, A{"class": "attribution"}))
	case "author":
		if t.v4 {
			if node.Parent.Tag == "authors" {
				if t.authorInAuthors {
					t.out("\n<br />")
				}
			} else {
				t.visitDocinfoItem(node, "author", true)
			}
		} else {
			if node.Parent.Tag != "authors" {
				t.visitDocinfoItem(node, "author", true)
			}
			t.out("<p>")
		}
	case "authors":
		if t.v4 {
			t.visitDocinfoItem(node, "authors", true)
			t.authorInAuthors = false
		} else {
			t.visitDocinfoItem(node, "authors", false)
			for _, sub := range node.Children {
				t.meta = append(t.meta, fmt.Sprintf("<meta name=\"author\" content=\"%s\" />\n", t.attval(sub.astext())))
			}
		}
	case "block_quote":
		t.out(t.starttag(node, "blockquote", "\n", nil))
	case "bullet_list":
		atts := A{}
		oldCompactSimple := t.compactSimple
		t.push([2]bool{t.compactSimple, t.compactP})
		t.compactP = false
		t.compactSimple = t.isCompactable(node)
		if t.compactSimple && !oldCompactSimple {
			atts["class"] = "simple"
		}
		t.out(t.starttag(node, "ul", "\n", atts))
	case "caption":
		if t.v4 {
			t.out(t.starttag(node, "p", "", A{"class": "caption"}))
		} else {
			if node.Parent.Tag == "figure" {
				t.out(t.starttag(node, "figcaption", "\n", nil))
			}
			t.out("<p>")
		}
	case "citation":
		if t.v4 {
			t.out(t.starttag(node, "table", "\n", A{"class": "docutils citation", "frame": "void", "rules": "none"}))
			t.out("<colgroup><col class=\"label\" /><col /></colgroup>\n<tbody valign=\"top\">\n<tr>")
			t.footnoteBackrefs(node)
		} else {
			if ps := node.previousSibling(); ps == nil || ps.Tag != "citation" {
				t.out("<div role=\"list\" class=\"citation-list\">\n")
			}
			t.out(t.starttag(node, "div", "\n", A{"classes": []string{node.Tag}, "role": "doc-biblioentry"}))
		}
	case "citation_reference":
		href := "#"
		if node.has("refid") {
			href += node.str("refid")
		} else if node.has("refname") {
			href += t.doc.nameids[node.str("refname")]
		}
		atts := A{"href": href, "classes": []string{"citation-reference"}}
		if !t.v4 {
			atts["role"] = "doc-biblioref"
		}
		t.out(t.starttag(node, "a", "[", atts))
	case "classifier":
		if t.v4 {
			t.out(` <span class="classifier-delimiter">:</span> `)
		}
		t.out(t.starttag(node, "span", "", A{"class": "classifier"}))
	case "colspec":
		t.colspecs = append(t.colspecs, node)
		t.stubs[node.Parent] = append(t.stubs[node.Parent], node.truthy("stub"))
	case "comment":
		t.out("<!-- " + spaceDashes(node.astext()) + " -->\n")
		return actSkipNode
	case "compound":
		t.out(t.starttag(node, "div", "\n", A{"class": "compound"}))
		if t.v4 && len(node.Children) > 1 {
			addClass(node.Children[0], "compound-first")
			addClass(node.Children[len(node.Children)-1], "compound-last")
			for _, child := range node.Children[1 : len(node.Children)-1] {
				addClass(child, "compound-middle")
			}
		}
	case "container":
		tagname := "div"
		if !t.v4 {
			var tags []string
			for _, cls := range node.list("classes") {
				if cls == "ins" || cls == "del" {
					tags = append(tags, cls)
				}
			}
			if len(tags) == 1 {
				tagname = tags[0]
				node.removeFrom("classes", tags[0])
			}
			t.html5tag[node] = tagname
		}
		t.out(t.starttag(node, tagname, "\n", A{"class": "docutils container"}))
	case "contact":
		t.visitDocinfoItem(node, "contact", false)
	case "copyright":
		if t.v4 {
			t.visitDocinfoItem(node, "copyright", true)
		} else {
			t.visitDocinfoItem(node, "copyright", false)
			t.meta = append(t.meta, fmt.Sprintf("<meta name=\"dcterms.rights\" content=\"%s\" />\n", t.attval(node.astext())))
		}
	case "date":
		if t.v4 {
			t.visitDocinfoItem(node, "date", true)
		} else {
			t.visitDocinfoItem(node, "date", false)
			t.meta = append(t.meta, fmt.Sprintf("<meta name=\"dcterms.date\" content=\"%s\" />\n", t.attval(node.astext())))
		}
	case "decoration":
	case "definition":
		if t.v4 {
			t.out(t.starttag(node, "dd", "", nil))
			t.setFirstLast(node)
		} else if !hasClass(node.Parent.Parent, "details") {
			t.out(t.starttag(node, "dd", "", nil))
		}
	case "definition_list":
		if t.v4 {
			t.out(t.starttag(node, "dl", "\n", A{"class": "docutils"}))
		} else if hasClass(node, "details") {
			t.out(t.starttag(node, "div", "\n", nil))
		} else {
			var classes []string
			if t.isCompactable(node) {
				classes = []string{"simple"}
			}
			t.out(t.starttag(node, "dl", "\n", A{"classes": classes}))
		}
	case "definition_list_item":
		if !t.v4 && hasClass(node.Parent, "details") {
			atts := A{}
			if hasClass(node.Parent, "open") {
				atts["open"] = "open"
			}
			t.out(t.starttag(node, "details", "\n", atts))
		}
	case "description":
		if t.v4 {
			t.out(t.starttag(node, "td", "", nil))
			t.setFirstLast(node)
		} else {
			t.out(t.starttag(node, "dd", "", nil))
		}
	case "docinfo":
		t.push(len(t.body))
		if t.v4 {
			t.out(t.starttag(node, "table", "\n", A{"class": "docinfo", "frame": "void", "rules": "none"}))
			t.out("<col class=\"docinfo-name\" />\n<col class=\"docinfo-content\" />\n<tbody valign=\"top\">\n")
			t.inDocinfo = true
		} else {
			classes := []string{"docinfo"}
			if t.isCompactable(node) {
				classes = append(classes, "simple")
			}
			t.out(t.starttag(node, "dl", "\n", A{"classes": classes}))
		}
	case "doctest_block":
		if t.v4 {
			t.out(t.starttag(node, "pre", "\n", A{"class": "doctest-block"}))
		} else {
			t.out(t.starttag(node, "pre", "", A{"classes": []string{"code", "python", "doctest"}}))
		}
	case "document":
		title := node.str("title")
		if title == "" {
			title = path.Base(strings.ReplaceAll(node.str("source"), "\\", "/"))
			if node.str("source") == "" {
				title = ""
			}
		}
		if title == "" {
			title = "untitled Docutils document"
		}
		t.head = append(t.head, "<title>"+t.encode(title)+"</title>\n")
	case "emphasis":
		t.out(t.starttag(node, "em", "", nil))
	case "entry":
		t.visitEntry(node)
	case "enumerated_list":
		if t.v4 {
			atts := A{}
			if node.has("start") {
				atts["start"] = node.get("start")
			}
			cls := ""
			if node.has("enumtype") {
				cls = node.str("enumtype")
			}
			oldCompactSimple := t.compactSimple
			t.push([2]bool{t.compactSimple, t.compactP})
			t.compactP = false
			t.compactSimple = t.isCompactable(node)
			if t.compactSimple && !oldCompactSimple {
				cls = pyStrip(cls + " simple")
			}
			if cls != "" {
				atts["class"] = cls
			}
			t.out(t.starttag(node, "ol", "\n", atts))
		} else {
			var classes []string
			atts := A{}
			if node.has("start") {
				atts["start"] = node.get("start")
			}
			if node.has("enumtype") {
				classes = append(classes, node.str("enumtype"))
			}
			if t.isCompactable(node) {
				classes = append(classes, "simple")
			}
			atts["classes"] = classes
			t.out(t.starttag(node, "ol", "\n", atts))
		}
	case "field_list":
		t.visitFieldList(node)
	case "field":
		if t.v4 {
			t.out(t.starttag(node, "tr", "", A{"class": "field"}))
		} else {
			for _, child := range node.Children {
				if child.Tag == "field_name" {
					child.appendTo("ids", node.list("ids")...)
				}
			}
		}
	case "field_name":
		if t.v4 {
			atts := A{}
			if t.inDocinfo {
				atts["class"] = "docinfo-name"
			} else {
				atts["class"] = "field-name"
			}
			if t.settings.fieldNameLimit > 0 && runeLen(node.astext()) > t.settings.fieldNameLimit {
				atts["colspan"] = 2
				t.push("</tr>\n" + t.starttag(node.Parent, "tr", "", A{"class": "field"}) + "<td>&nbsp;</td>")
			} else {
				t.push("")
			}
			t.out(t.starttag(node, "th", "", atts))
		} else {
			t.out(t.starttag(node, "dt", "", A{"classes_of": node.Parent}))
		}
	case "field_body":
		if t.v4 {
			t.out(t.starttag(node, "td", "", A{"class": "field-body"}))
			t.setClassOnChild(node, "first", 0)
			field := node.Parent
			if t.compactFieldList || field.Parent.Tag == "docinfo" ||
				field.Parent.index(field) == len(field.Parent.Children)-1 {
				t.setClassOnChild(node, "last", -1)
			}
		} else {
			t.out(t.starttag(node, "dd", "", A{"classes_of": node.Parent}))
			if len(node.Children) == 0 {
				t.out("<p></p>")
			}
		}
	case "figure":
		if t.v4 {
			cls := "figure"
			atts := A{}
			if node.truthy("width") {
				atts["style"] = "width: " + node.str("width")
			}
			if node.truthy("align") {
				cls += " align-" + node.str("align")
			}
			atts["class"] = cls
			t.out(t.starttag(node, "div", "\n", atts))
		} else {
			atts := A{}
			if node.has("width") {
				atts["style"] = "width: " + node.str("width")
			}
			if node.truthy("align") {
				atts["class"] = "align-" + node.str("align")
			}
			t.out(t.starttag(node, "figure", "\n", atts))
		}
	case "footer", "header":
		t.push(len(t.body))
	case "footnote":
		if t.v4 {
			t.out(t.starttag(node, "table", "\n", A{"class": "docutils footnote", "frame": "void", "rules": "none"}))
			t.out("<colgroup><col class=\"label\" /><col /></colgroup>\n<tbody valign=\"top\">\n<tr>")
			t.footnoteBackrefs(node)
		} else {
			labelStyle := t.settings.footnoteReferences
			if ps := node.previousSibling(); ps == nil || ps.Tag != "footnote" {
				t.out("<aside class=\"footnote-list " + labelStyle + "\">\n")
			}
			t.out(t.starttag(node, "aside", "\n", A{"classes": []string{node.Tag, labelStyle}, "role": "doc-footnote"}))
		}
	case "footnote_reference":
		href := "#" + node.str("refid")
		if t.v4 {
			var suffix string
			if t.settings.footnoteReferences == "brackets" {
				suffix = "["
				t.push("]")
			} else {
				suffix = "<sup>"
				t.push("</sup>")
			}
			t.out(t.starttag(node, "a", suffix, A{"class": "footnote-reference", "href": href}))
		} else {
			t.out(t.starttag(node, "a", "", A{"classes": []string{t.settings.footnoteReferences}, "role": "doc-noteref", "href": href}))
			t.out(`<span class="fn-bracket">[</span>`)
		}
	case "generated":
		if !t.v4 && hasClass(node, "sectnum") {
			sectnum := strings.TrimRight(node.astext(), " ")
			t.out(`<span class="sectnum">` + t.encode(sectnum) + ` </span>`)
			return actSkipNode
		}
	case "image":
		if t.v4 {
			t.visitImage4(node)
		} else {
			t.visitImage5(node)
		}
	case "inline":
		if t.v4 {
			t.out(t.starttag(node, "span", "", nil))
		} else {
			t.visitInline5(node)
		}
	case "label":
		if t.v4 {
			t.out(t.starttag(node, "td", t.popStr()+"[", A{"class": "label"}))
		} else {
			t.out(`<span class="label">`)
			t.out(`<span class="fn-bracket">[</span>`)
			if t.settings.footnoteBackl {
				backrefs := node.Parent.list("backrefs")
				if len(backrefs) == 1 {
					t.out(`<a role="doc-backlink" href="#` + backrefs[0] + `">`)
				}
			}
		}
	case "legend":
		if !t.v4 {
			if ps := node.previousSibling(); ps == nil || ps.Tag != "caption" {
				t.out("<figcaption>\n")
			}
		}
		t.out(t.starttag(node, "div", "\n", A{"class": "legend"}))
	case "line":
		t.out(t.starttag(node, "div", "", A{"class": "line"}))
		if len(node.Children) == 0 {
			t.out("<br />")
		}
	case "line_block":
		t.out(t.starttag(node, "div", "\n", A{"class": "line-block"}))
	case "list_item":
		t.out(t.starttag(node, "li", "", nil))
		if t.v4 && len(node.Children) > 0 {
			addClass(node.Children[0], "first")
		}
	case "literal":
		return t.visitLiteral(node)
	case "literal_block":
		if t.v4 {
			t.out(t.starttag(node, "pre", "\n", A{"class": "literal-block"}))
		} else {
			t.out(t.starttag(node, "pre", "", A{"class": "literal-block"}))
			if hasClass(node, "code") {
				t.out("<code>")
			}
		}
	case "math", "math_block":
		return t.visitMath(node)
	case "meta":
		atts := A{}
		if !t.v4 && node.has("lang") {
			node.Attrs["xml:lang"] = node.Attrs["lang"]
		}
		for _, k := range node.attlist() {
			atts[k] = node.Attrs[k]
		}
		t.meta = append(t.meta, t.emptytag(node, "meta", "\n", atts))
	case "option":
		if !t.v4 {
			t.out(t.starttag(node, "span", "", A{"class": "option"}))
		} else {
			t.visitOption4(node)
		}
	case "option_argument":
		d := " "
		if node.has("delimiter") {
			d = node.str("delimiter")
		}
		t.out(d)
		t.out(t.starttag(node, "var", "", nil))
	case "option_group":
		if t.v4 {
			atts := A{"class": "option-group"}
			if t.settings.optionLimit > 0 && runeLen(node.astext()) > t.settings.optionLimit {
				atts["colspan"] = 2
				t.push("</tr>\n<tr><td>&nbsp;</td>")
			} else {
				t.push("")
			}
			t.out(t.starttag(node, "td", "\n", atts))
			t.out("<kbd>")
			t.push(0)
		} else {
			t.out(t.starttag(node, "dt", "", nil))
			t.out("<kbd>")
		}
	case "option_list":
		if t.v4 {
			t.out(t.starttag(node, "table", "\n", A{"class": "docutils option-list", "frame": "void", "rules": "none"}))
			t.out("<col class=\"option\" />\n<col class=\"description\" />\n<tbody valign=\"top\">\n")
		} else {
			t.out(t.starttag(node, "dl", "\n", A{"class": "option-list"}))
		}
	case "option_list_item":
		if t.v4 {
			t.out(t.starttag(node, "tr", "", nil))
		}
	case "option_string":
	case "organization":
		t.visitDocinfoItem(node, "organization", t.v4)
	case "paragraph":
		if t.v4 {
			if t.shouldBeCompactParagraph(node) {
				t.push("")
			} else {
				t.out(t.starttag(node, "p", "", nil))
				t.push("</p>\n")
			}
		} else {
			t.out(t.starttag(node, "p", "", nil))
		}
	case "problematic":
		if node.has("refid") {
			t.out(`<a href="#` + node.str("refid") + `">`)
			t.push("</a>")
		} else {
			t.push("")
		}
		t.out(t.starttag(node, "span", "", A{"class": "problematic"}))
	case "raw":
		if containsStr(pySplit(node.str("format")), "html") {
			tagname := "div"
			if textElementTags[node.Parent.Tag] {
				tagname = "span"
			}
			has := len(node.list("classes")) > 0
			if has {
				t.out(t.starttag(node, tagname, "", nil))
			}
			t.out(node.astext())
			if has {
				t.out("</" + tagname + ">")
			}
		}
		return actSkipNode
	case "reference":
		classes := []string{"reference"}
		atts := A{}
		suffix := ""
		if node.has("refuri") {
			href := node.str("refuri")
			if t.settings.cloakEmailAddresses && strings.HasPrefix(href, "mailto:") {
				href = strings.ReplaceAll(href, "@", "%40")
				t.inMailto = true
			}
			atts["href"] = href
			classes = append(classes, "external")
		} else {
			atts["href"] = "#" + node.str("refid")
			classes = append(classes, "internal")
		}
		if len(node.Children) == 1 && node.Children[0].Tag == "image" {
			classes = append(classes, "image-reference")
		}
		if !textElementTags[node.Parent.Tag] {
			suffix = "\n"
		}
		atts["classes"] = classes
		t.out(t.starttag(node, "a", suffix, atts))
	case "revision":
		t.visitDocinfoItem(node, "revision", false)
	case "row":
		t.out(t.starttag(node, "tr", "", nil))
		t.column[node] = 0
	case "rubric":
		t.out(t.starttag(node, "p", "", A{"class": "rubric"}))
	case "section":
		t.sectionLevel++
		if t.v4 {
			t.out(t.starttag(node, "div", "\n", A{"class": "section"}))
		} else {
			t.out(t.starttag(node, "section", "\n", nil))
		}
	case "sidebar":
		if t.v4 {
			t.out(t.starttag(node, "div", "\n", A{"class": "sidebar"}))
			t.setFirstLast(node)
		} else {
			t.out(t.starttag(node, "aside", "\n", A{"class": "sidebar"}))
		}
		t.inSidebar = true
	case "status":
		t.visitDocinfoItem(node, "status", false)
	case "strong":
		t.out(t.starttag(node, "strong", "", nil))
	case "subscript", "superscript":
		tag := "sub"
		cls := "subscript"
		if node.Tag == "superscript" {
			tag, cls = "sup", "superscript"
		}
		if t.v4 && node.Parent.Tag == "literal_block" {
			t.out(t.starttag(node, "span", "", A{"class": cls}))
		} else {
			t.out(t.starttag(node, tag, "", nil))
		}
	case "substitution_definition":
		return actSkipNode
	case "subtitle":
		t.visitSubtitle(node)
	case "system_message":
		t.visitSystemMessage(node)
	case "table":
		if t.v4 {
			t.push(t.compactP)
			t.compactP = true
			atts := A{"border": 1}
			classes := []string{"docutils", t.settings.tableStyle}
			if node.has("align") {
				classes = append(classes, "align-"+node.str("align"))
			}
			if node.has("width") {
				width := node.str("width")
				if width != "" && strings.ContainsRune("0123456789.", rune(width[len(width)-1])) {
					width += "px"
				}
				atts["style"] = "width: " + width
			}
			atts["class"] = strings.Join(classes, " ")
			t.out(t.starttag(node, "table", "\n", atts))
		} else {
			classes := pySplit(strings.ReplaceAll(t.settings.tableStyle, ",", " "))
			atts := A{}
			if node.has("align") {
				classes = append(classes, "align-"+node.str("align"))
			}
			if node.has("width") {
				width := node.str("width")
				if width != "" && strings.ContainsRune("0123456789.", rune(width[len(width)-1])) {
					width += "px"
				}
				atts["style"] = "width: " + width + ";"
			}
			atts["classes"] = classes
			t.out(t.starttag(node, "table", "\n", atts))
		}
	case "target":
		if !node.has("refuri") && !node.has("refid") && !node.has("refname") {
			t.out(t.starttag(node, "span", "", A{"class": "target"}))
			t.push("</span>")
		} else {
			t.push("")
		}
	case "tbody":
		if t.v4 {
			t.out(t.starttag(node, "tbody", "\n", A{"valign": "top"}))
		} else {
			t.out(t.starttag(node, "tbody", "\n", nil))
		}
	case "term":
		if !t.v4 && hasClass(node.Parent.Parent, "details") {
			t.out(t.starttag(node, "summary", "", nil))
		} else {
			t.out(t.starttag(node, "dt", "", A{"classes_of": node.Parent,
				"ids": append([]string{}, node.Parent.list("ids")...)}))
		}
	case "tgroup":
		t.colspecs = nil
		t.stubs[node] = nil
	case "thead":
		if t.v4 {
			t.out(t.starttag(node, "thead", "\n", A{"valign": "bottom"}))
		} else {
			t.out(t.starttag(node, "thead", "\n", nil))
		}
	case "title":
		t.visitTitle(node)
	case "title_reference":
		t.out(t.starttag(node, "cite", "", nil))
	case "topic":
		if t.v4 {
			t.out(t.starttag(node, "div", "\n", A{"class": "topic"}))
		} else {
			atts := A{"classes": []string{"topic"}}
			var tag string
			if hasClass(node, "contents") {
				tag = "nav"
				delete(atts, "classes")
				if node.Parent.Tag == "document" {
					atts["role"] = "doc-toc"
					t.bodyPrefix[0] = "</head>\n<body class=\"with-toc\">\n"
				}
			} else if hasClass(node, "abstract") {
				tag = "div"
				atts["role"] = "doc-abstract"
			} else if hasClass(node, "dedication") {
				tag = "div"
				atts["role"] = "doc-dedication"
			} else {
				tag = "aside"
			}
			t.html5tag[node] = tag
			t.out(t.starttag(node, tag, "\n", atts))
		}
	case "transition":
		t.out(t.emptytag(node, "hr", "\n", A{"class": "docutils"}))
	case "version":
		t.visitDocinfoItem(node, "version", false)
	case "pending", "substitution_reference":
		// Docutils raises NotImplementedError for these; we skip them.
		return actSkipNode
	}
	return actProceed
}

func addClass(n *Node, c string) {
	if !n.IsText() {
		n.appendTo("classes", c)
	}
}

func (t *htmlTranslator) depart(node *Node) {
	switch node.Tag {
	case "abbreviation":
		t.out("</abbr>")
	case "acronym":
		if t.v4 {
			t.out("</acronym>")
		} else {
			t.out("</abbr>")
		}
	case "address":
		t.out("\n</pre>\n")
		t.departDocinfoItem()
	case "admonition":
		if t.v4 {
			t.out("</div>\n")
		} else {
			t.out("</aside>\n")
		}
	case "attribution":
		t.out(t.popStr() + "</p>\n")
	case "author":
		if t.v4 {
			if node.Parent.Tag == "authors" {
				t.authorInAuthors = true
			} else {
				t.departDocinfoItem()
			}
		} else {
			t.out("</p>")
			if node.Parent.Tag == "authors" {
				t.out("\n")
			} else {
				t.departDocinfoItem()
			}
		}
	case "authors", "contact", "copyright", "date", "organization", "revision", "status", "version":
		t.departDocinfoItem()
	case "block_quote":
		t.out("</blockquote>\n")
	case "bullet_list":
		c := t.pop().([2]bool)
		t.compactSimple, t.compactP = c[0], c[1]
		t.out("</ul>\n")
	case "caption":
		t.out("</p>\n")
	case "citation":
		if t.v4 {
			t.out("</td></tr>\n</tbody>\n</table>\n")
		} else {
			t.out("</div>\n")
			if ns := nextSibling(node); ns == nil || ns.Tag != "citation" {
				t.out("</div>\n")
			}
		}
	case "citation_reference":
		t.out("]</a>")
	case "classifier":
		t.out("</span>")
		t.departTerm(node)
	case "colspec":
		t.departColspec(node)
	case "compound":
		t.out("</div>\n")
	case "container":
		if t.v4 {
			t.out("</div>\n")
		} else {
			t.out("</" + t.html5tag[node] + ">\n")
			delete(t.html5tag, node)
		}
	case "definition":
		if t.v4 || !hasClass(node.Parent.Parent, "details") {
			t.out("</dd>\n")
		}
	case "definition_list":
		if !t.v4 && hasClass(node, "details") {
			t.out("</div>\n")
		} else {
			t.out("</dl>\n")
		}
	case "definition_list_item":
		if !t.v4 && hasClass(node.Parent, "details") {
			t.out("</details>\n")
		}
	case "description":
		if t.v4 {
			t.out("</td>")
		} else {
			t.out("</dd>\n")
		}
	case "docinfo":
		if t.v4 {
			t.out("</tbody>\n</table>\n")
			t.inDocinfo = false
		} else {
			t.out("</dl>\n")
		}
		start := t.pop().(int)
		t.docinfo = append([]string{}, t.body[start:]...)
		t.body = nil
	case "doctest_block":
		t.out("\n</pre>\n")
	case "document":
		t.departDocument(node)
	case "emphasis":
		t.out("</em>")
	case "entry":
		t.out(t.popStr())
	case "enumerated_list":
		if t.v4 {
			c := t.pop().([2]bool)
			t.compactSimple, t.compactP = c[0], c[1]
		}
		t.out("</ol>\n")
	case "field_list":
		if t.v4 {
			t.out("</tbody>\n</table>\n")
			c := t.pop().([2]bool)
			t.compactFieldList, t.compactP = c[0], c[1]
		} else {
			t.out("</dl>\n")
		}
	case "field":
		if t.v4 {
			t.out("</tr>\n")
		}
	case "field_name":
		if t.v4 {
			t.out(":</th>")
			t.out(t.popStr())
		} else {
			t.out("<span class=\"colon\">:</span></dt>\n")
		}
	case "field_body":
		if t.v4 {
			t.out("</td>\n")
		} else {
			t.out("</dd>\n")
		}
	case "figure":
		if t.v4 {
			t.out("</div>\n")
		} else {
			if len(node.Children) > 1 {
				t.out("</figcaption>\n")
			}
			t.out("</figure>\n")
		}
	case "footer":
		start := t.pop().(int)
		var footer []string
		if t.v4 {
			footer = []string{t.starttag(node, "div", "\n", A{"class": "footer"}), "<hr class=\"footer\" />\n"}
			footer = append(footer, t.body[start:]...)
			footer = append(footer, "\n</div>\n")
		} else {
			footer = []string{t.starttag(node, "footer", "\n", nil)}
			footer = append(footer, t.body[start:]...)
			footer = append(footer, "</footer>\n")
		}
		t.footer = append(t.footer, footer...)
		t.bodySuffix = append(append([]string{}, footer...), t.bodySuffix...)
		t.body = t.body[:start]
	case "header":
		start := t.pop().(int)
		var header []string
		if t.v4 {
			header = []string{t.starttag(node, "div", "\n", A{"class": "header"})}
			header = append(header, t.body[start:]...)
			header = append(header, "\n<hr class=\"header\"/>\n</div>\n")
		} else {
			header = []string{t.starttag(node, "header", "\n", nil)}
			header = append(header, t.body[start:]...)
			header = append(header, "</header>\n")
		}
		t.bodyPrefix = append(t.bodyPrefix, header...)
		t.header = append(t.header, header...)
		t.body = t.body[:start]
	case "footnote":
		if t.v4 {
			t.out("</td></tr>\n</tbody>\n</table>\n")
		} else {
			t.out("</aside>\n")
			if ns := nextSibling(node); ns == nil || ns.Tag != "footnote" {
				t.out("</aside>\n")
			}
		}
	case "footnote_reference":
		if t.v4 {
			t.out(t.popStr() + "</a>")
		} else {
			t.out(`<span class="fn-bracket">]</span>`)
			t.out("</a>")
		}
	case "inline":
		if t.v4 {
			t.out("</span>")
		} else {
			t.departInline5(node)
		}
	case "label":
		if t.v4 {
			a := t.popStr()
			b := t.popStr()
			t.out("]" + a + "</td><td>" + b)
		} else {
			var backrefs []string
			if t.settings.footnoteBackl {
				backrefs = node.Parent.list("backrefs")
			}
			if len(backrefs) == 1 {
				t.out("</a>")
			}
			t.out("<span class=\"fn-bracket\">]</span></span>\n")
			if len(backrefs) > 1 {
				links := make([]string, len(backrefs))
				for i, ref := range backrefs {
					links[i] = fmt.Sprintf(`<a role="doc-backlink" href="#%s">%d</a>`, ref, i+1)
				}
				t.out("<span class=\"backrefs\">(" + strings.Join(links, ",") + ")</span>\n")
			}
		}
	case "legend":
		t.out("</div>\n")
	case "line", "line_block":
		t.out("</div>\n")
	case "list_item":
		t.out("</li>\n")
	case "literal":
		if t.v4 {
			t.out("</code>")
		} else {
			t.departInline5(node)
		}
	case "literal_block":
		if t.v4 {
			t.out("\n</pre>\n")
		} else {
			if hasClass(node, "code") {
				t.out("</code>")
			}
			t.out("</pre>\n")
		}
	case "math_block":
		t.reportMessages(node)
	case "option":
		if t.v4 {
			t.departOption4(node)
		} else {
			t.out("</span>")
			if ns := nextSibling(node); ns != nil && ns.Tag == "option" {
				t.out(", ")
			}
		}
	case "option_argument":
		t.out("</var>")
	case "option_group":
		if t.v4 {
			t.pop()
			t.out("</kbd></td>\n")
			t.out(t.popStr())
		} else {
			t.out("</kbd></dt>\n")
		}
	case "option_list":
		if t.v4 {
			t.out("</tbody>\n</table>\n")
		} else {
			t.out("</dl>\n")
		}
	case "option_list_item":
		if t.v4 {
			t.out("</tr>\n")
		}
	case "paragraph":
		if t.v4 {
			t.out(t.popStr())
			t.reportMessages(node)
		} else {
			t.out("</p>")
			if !(node.Parent.is("list_item", "entry") && len(node.Parent.Children) == 1) {
				t.out("\n")
				t.reportMessages(node)
			}
		}
	case "problematic":
		t.out("</span>")
		t.out(t.popStr())
	case "reference":
		t.out("</a>")
		if !textElementTags[node.Parent.Tag] {
			t.out("\n")
		}
		t.inMailto = false
	case "row":
		t.out("</tr>\n")
	case "rubric":
		t.out("</p>\n")
	case "section":
		t.sectionLevel--
		if t.v4 {
			t.out("</div>\n")
		} else {
			t.out("</section>\n")
		}
	case "sidebar":
		if t.v4 {
			t.out("</div>\n")
		} else {
			t.out("</aside>\n")
		}
		t.inSidebar = false
	case "strong":
		t.out("</strong>")
	case "subscript", "superscript":
		if t.v4 && node.Parent.Tag == "literal_block" {
			t.out("</span>")
		} else if node.Tag == "subscript" {
			t.out("</sub>")
		} else {
			t.out("</sup>")
		}
	case "subtitle":
		t.departSubtitle(node)
	case "system_message":
		if t.v4 {
			t.out("</div>\n")
		} else {
			t.out("</aside>\n")
		}
	case "table":
		if t.v4 {
			t.compactP = t.pop().(bool)
			t.out("</table>\n")
		} else {
			t.out("</table>\n")
			t.reportMessages(node)
		}
	case "target":
		t.out(t.popStr())
	case "tbody":
		t.out("</tbody>\n")
	case "term":
		t.departTerm(node)
	case "thead":
		t.out("</thead>\n")
	case "title":
		t.out(t.popStr())
		if t.inDocumentTitle != 0 {
			t.title = append([]string{}, t.body[t.inDocumentTitle:len(t.body)-1]...)
			t.inDocumentTitle = 0
			t.bodyPreDocinfo = append(t.bodyPreDocinfo, t.body...)
			t.htmlTitle = append(t.htmlTitle, t.body...)
			t.body = nil
		}
	case "title_reference":
		t.out("</cite>")
	case "topic":
		if t.v4 {
			t.out("</div>\n")
		} else {
			t.out("</" + t.html5tag[node] + ">\n")
			delete(t.html5tag, node)
		}
	}
}

func (t *htmlTranslator) departTerm(node *Node) {
	// next_node(nodes.classifier, descend=False, siblings=True)
	if node.Parent != nil {
		i := node.Parent.index(node)
		for _, sib := range node.Parent.Children[i+1:] {
			if sib.Tag == "classifier" {
				return
			}
		}
	}
	if !t.v4 && hasClass(node.Parent.Parent, "details") {
		t.out("</summary>\n")
	} else {
		t.out("</dt>\n")
	}
}

func propwidth(n *Node) float64 {
	switch v := n.get("colwidth").(type) {
	case int:
		return float64(v)
	case float64:
		return v
	}
	return 1
}

func (t *htmlTranslator) departColspec(node *Node) {
	if ns := nextSibling(node); ns != nil && ns.Tag == "colspec" {
		return
	}
	table := node.Parent.Parent
	if t.v4 {
		if hasClass(table, "colwidths-auto") ||
			(strings.Contains(t.settings.tableStyle, "colwidths-auto") && !hasClass(table, "colwidths-given")) {
			return
		}
		total := 0.0
		for _, c := range t.colspecs {
			total += propwidth(c)
		}
		t.out(t.starttag(node, "colgroup", "\n", nil))
		for _, c := range t.colspecs {
			colwidth := propwidth(c)*100.0/total + 0.5
			t.out(t.emptytag(c, "col", "\n", A{"width": fmt.Sprintf("%d%%", int(colwidth))}))
		}
		t.out("</colgroup>\n")
		return
	}
	if hasClass(table, "colwidths-auto") ||
		(!strings.Contains(t.settings.tableStyle, "colwidths-grid") && !hasClass(table, "colwidths-given")) {
		return
	}
	t.out(t.starttag(node, "colgroup", "\n", nil))
	total := 0.0
	for _, c := range t.colspecs {
		total += propwidth(c)
	}
	for _, c := range t.colspecs {
		colwidth := propwidth(c) / total
		t.out(t.emptytag(c, "col", "\n", A{"style": fmt.Sprintf("width: %.1f%%", colwidth*100)}))
	}
	t.out("</colgroup>\n")
}

func (t *htmlTranslator) visitEntry(node *Node) {
	var classes []string
	row := node.Parent
	if row.Parent.Tag == "thead" {
		classes = append(classes, "head")
	}
	stubs := t.stubs[row.Parent.Parent]
	if col := t.column[row]; col < len(stubs) && stubs[col] {
		classes = append(classes, "stub")
	}
	tagname := "td"
	if len(classes) > 0 {
		tagname = "th"
	}
	atts := A{"classes": classes}
	t.column[row]++
	if node.has("morerows") {
		atts["rowspan"] = node.get("morerows").(int) + 1
	}
	if node.has("morecols") {
		atts["colspan"] = node.get("morecols").(int) + 1
		t.column[row] += node.get("morecols").(int)
	}
	t.out(t.starttag(node, tagname, "", atts))
	t.push("</" + tagname + ">\n")
	if t.v4 {
		if len(node.Children) == 0 {
			t.out("&nbsp;")
		}
		t.setFirstLast(node)
	}
}

func (t *htmlTranslator) visitFieldList(node *Node) {
	if t.v4 {
		t.push([2]bool{t.compactFieldList, t.compactP})
		t.compactP = false
		if hasClass(node, "compact") {
			t.compactFieldList = true
		} else if t.settings.compactFieldLists && !hasClass(node, "open") {
			t.compactFieldList = true
		}
		if t.compactFieldList {
			for _, field := range node.Children {
				fieldBody := field.Children[len(field.Children)-1]
				children := visibleChildren(fieldBody)
				if !(len(children) == 0 || (len(children) == 1 && children[0].is("paragraph", "line_block"))) {
					t.compactFieldList = false
					break
				}
			}
		}
		t.out(t.starttag(node, "table", "\n", A{"frame": "void", "rules": "none", "class": "docutils field-list"}))
		t.out("<col class=\"field-name\" />\n<col class=\"field-body\" />\n<tbody valign=\"top\">\n")
		return
	}
	atts := A{}
	classes := node.list("classes")
	for i, cls := range classes {
		if strings.HasPrefix(cls, "field-indent-") {
			v, err := lengthOrPercentageOrUnitless(strings.TrimPrefix(cls, "field-indent-"), false, "px")
			if err != nil {
				break
			}
			atts["style"] = fmt.Sprintf("--field-indent: %s;", v)
			classes = append(classes[:i:i], classes[i+1:]...)
			break
		}
	}
	classes = append(classes, "field-list")
	node.Attrs["classes"] = classes
	if t.isCompactable(node) {
		node.appendTo("classes", "simple")
	}
	t.out(t.starttag(node, "dl", "\n", atts))
}

func (t *htmlTranslator) footnoteBackrefs(node *Node) {
	var backlinks []string
	backrefs := node.list("backrefs")
	if t.settings.footnoteBackl && len(backrefs) > 0 {
		if len(backrefs) == 1 {
			t.push("")
			t.push("</a>")
			t.push(`<a class="fn-backref" href="#` + backrefs[0] + `">`)
		} else {
			for i, backref := range backrefs {
				backlinks = append(backlinks, fmt.Sprintf(`<a class="fn-backref" href="#%s">%d</a>`, backref, i+1))
			}
			t.push("<em>(" + strings.Join(backlinks, ", ") + ")</em> ")
			t.push("")
			t.push("")
		}
	} else {
		t.push("")
		t.push("")
		t.push("")
	}
	if len(node.Children) > 1 {
		if len(backlinks) == 0 {
			addClass(node.Children[1], "first")
		}
		addClass(node.Children[len(node.Children)-1], "last")
	}
}

func (t *htmlTranslator) visitOption4(node *Node) {
	// html4css1 inherits visit_option from the base writer
	t.out(t.starttag(node, "span", "", A{"class": "option"}))
}

func (t *htmlTranslator) departOption4(node *Node) {
	t.out("</span>")
	if ns := nextSibling(node); ns != nil && ns.Tag == "option" {
		t.out(", ")
	}
}

func (t *htmlTranslator) shouldBeCompactParagraph(node *Node) bool {
	if node.Parent.is("document", "compound") {
		return false
	}
	for _, key := range node.attlist() {
		if key == "classes" {
			c := node.list("classes")
			s := strings.Join(c, " ")
			if s == "first" || s == "last" || s == "first last" {
				continue
			}
		}
		return false
	}
	first := 0
	if len(node.Parent.Children) > 0 && node.Parent.Children[0].Tag == "label" {
		first = 1
	}
	for _, child := range node.Parent.Children[first:] {
		if invisibleTags[child.Tag] {
			continue
		}
		if child == node {
			break
		}
		return false
	}
	parentLength := 0
	for _, n := range node.Parent.Children {
		if !invisibleTags[n.Tag] && n.Tag != "label" {
			parentLength++
		}
	}
	return t.compactSimple || t.compactFieldList || (t.compactP && parentLength == 1)
}

func (t *htmlTranslator) visitLiteral(node *Node) visitAction {
	classes := node.list("classes")
	if t.v4 {
		if containsStr(classes, "code") {
			var kept []string
			for _, c := range classes {
				if c != "code" {
					kept = append(kept, c)
				}
			}
			node.Attrs["classes"] = kept
			t.out(t.starttag(node, "code", "", nil))
			return actProceed
		}
		t.out(t.starttag(node, "tt", "", A{"class": "docutils literal"}))
		text := node.astext()
		for _, token := range wordsAndSpacesTokens(text) {
			if pyStrip(token) != "" {
				if hasWordWrapPoint(token) {
					t.out(`<span class="pre">` + t.encode(token) + `</span>`)
				} else {
					t.out(t.encode(token))
				}
			} else if token == "\n" || token == " " {
				t.out(token)
			} else {
				t.out(strings.Repeat("&nbsp;", len(token)-1) + " ")
			}
		}
		t.out("</tt>")
		return actSkipNode
	}
	tagname := "span"
	for _, tag := range html5InlineTags {
		if containsStr(classes, tag) {
			tagname = tag
			node.removeFrom("classes", tag)
			break
		}
	}
	if tagname == "code" {
		t.html5tag[node] = tagname
		t.out(t.starttag(node, tagname, "", nil))
		return actProceed
	}
	t.out(t.starttag(node, tagname, "", A{"class": "docutils literal"}))
	text := node.astext()
	if node.Parent.Tag != "literal_block" {
		text = strings.ReplaceAll(text, "\n", " ")
	}
	for _, token := range wordsAndSpacesTokens(text) {
		if pyStrip(token) != "" && hasWordWrapPoint(token) {
			t.out(`<span class="pre">` + t.encode(token) + `</span>`)
		} else {
			t.out(t.encode(token))
		}
	}
	t.out("</" + tagname + ">")
	return actSkipNode
}

func (t *htmlTranslator) visitInline5(node *Node) {
	classes := node.list("classes")
	tagname := "span"
	p := node.Parent
	if (p.Tag == "literal_block" && hasClass(p, "code")) || (p.Tag == "literal" && t.html5tag[p] == "code") {
		if len(classes) == 1 && classes[0] == "ln" {
			if len(t.body) > 0 && t.body[len(t.body)-1] == "<code>" {
				t.body = t.body[:len(t.body)-1]
			} else {
				t.out("</code>")
			}
			tagname = "small"
		}
	} else {
		for _, tag := range html5InlineTags {
			if containsStr(classes, tag) {
				tagname = tag
				node.removeFrom("classes", tag)
				break
			}
		}
	}
	t.html5tag[node] = tagname
	t.out(t.starttag(node, tagname, "", nil))
}

func (t *htmlTranslator) departInline5(node *Node) {
	tagname := t.html5tag[node]
	t.out("</" + tagname + ">")
	c := node.list("classes")
	if tagname == "small" && len(c) == 1 && c[0] == "ln" && node.Parent.Tag == "literal_block" {
		t.out(`<code data-lineno="` + node.astext() + `">`)
	}
	delete(t.html5tag, node)
}

func (t *htmlTranslator) visitSubtitle(node *Node) {
	if t.v4 {
		switch node.Parent.Tag {
		case "sidebar":
			t.out(t.starttag(node, "p", "", A{"class": "sidebar-subtitle"}))
			t.push("</p>\n")
		case "document":
			t.out(t.starttag(node, "h2", "", A{"class": "subtitle"}))
			t.push("</h2>\n")
			t.inDocumentTitle = len(t.body)
		case "section":
			tag := fmt.Sprintf("h%d", t.sectionLevel+t.initialHeader-1)
			t.out(t.starttag(node, tag, "", A{"class": "section-subtitle"}) +
				t.starttag(dummyNode, "span", "", A{"class": "section-subtitle"}))
			t.push("</span></" + tag + ">\n")
		default:
			t.push("")
		}
		return
	}
	var classes []string
	switch node.Parent.Tag {
	case "sidebar":
		classes = []string{"sidebar-subtitle"}
	case "document":
		classes = []string{"subtitle"}
		t.inDocumentTitle = len(t.body) + 1
	case "section":
		classes = []string{"section-subtitle"}
	}
	t.out(t.starttag(node, "p", "", A{"classes": classes}))
}

func (t *htmlTranslator) departSubtitle(node *Node) {
	if t.v4 {
		t.out(t.popStr())
		if t.inDocumentTitle != 0 {
			t.subtitle = append([]string{}, t.body[t.inDocumentTitle:len(t.body)-1]...)
			t.inDocumentTitle = 0
			t.bodyPreDocinfo = append(t.bodyPreDocinfo, t.body...)
			t.htmlSubtitle = append(t.htmlSubtitle, t.body...)
			t.body = nil
		}
		return
	}
	t.out("</p>\n")
	if node.Parent.Tag == "document" {
		a := t.inDocumentTitle
		if a > len(t.body)-1 {
			a = len(t.body) - 1
		}
		t.subtitle = append([]string{}, t.body[a:len(t.body)-1]...)
		t.inDocumentTitle = 0
		t.bodyPreDocinfo = append(t.bodyPreDocinfo, t.body...)
		t.htmlSubtitle = append(t.htmlSubtitle, t.body...)
		t.body = nil
	}
}

func (t *htmlTranslator) visitSystemMessage(node *Node) {
	if t.v4 {
		t.out(t.starttag(node, "div", "\n", A{"class": "system-message"}))
	} else {
		t.out(t.starttag(node, "aside", "\n", A{"class": "system-message"}))
	}
	t.out(`<p class="system-message-title">`)
	backrefText := ""
	if backrefs := node.list("backrefs"); len(backrefs) > 0 {
		if len(backrefs) == 1 {
			backrefText = fmt.Sprintf(`; <em><a href="#%s">backlink</a></em>`, backrefs[0])
		} else {
			links := make([]string, len(backrefs))
			for i, b := range backrefs {
				links[i] = fmt.Sprintf(`<a href="#%s">%d</a>`, b, i+1)
			}
			backrefText = "; <em>backlinks: " + strings.Join(links, ", ") + "</em>"
		}
	}
	line := ""
	if node.has("line") {
		line = ", line " + node.str("line")
	}
	if t.v4 {
		t.out(fmt.Sprintf("System Message: %s/%s (<tt class=\"docutils\">%s</tt>%s)%s</p>\n",
			node.str("type"), node.str("level"), t.encode(node.str("source")), line, backrefText))
	} else {
		t.out(fmt.Sprintf("System Message: %s/%s (<span class=\"docutils literal\">%s</span>%s)%s</p>\n",
			node.str("type"), node.str("level"), t.encode(node.str("source")), line, backrefText))
	}
}

func (t *htmlTranslator) visitTitle(node *Node) {
	closeTag := "</p>\n"
	p := node.Parent
	switch {
	case p.Tag == "topic":
		t.out(t.starttag(node, "p", "", A{"class": "topic-title"}))
		if t.settings.tocBacklinks != "" && hasClass(p, "contents") {
			t.out(`<a class="reference internal" href="#top">`)
			closeTag = "</a></p>\n"
		}
	case p.Tag == "sidebar":
		t.out(t.starttag(node, "p", "", A{"class": "sidebar-title"}))
	case admonitionTags[p.Tag]:
		t.out(t.starttag(node, "p", "", A{"class": "admonition-title"}))
	case p.Tag == "table":
		t.out(t.starttag(node, "caption", "", nil))
		closeTag = "</caption>\n"
	case p.Tag == "document":
		t.out(t.starttag(node, "h1", "", A{"class": "title"}))
		closeTag = "</h1>\n"
		t.inDocumentTitle = len(t.body)
	default:
		var startTag string
		startTag, closeTag = t.sectionTitleTags(node)
		t.out(startTag)
	}
	t.push(closeTag)
}

func (t *htmlTranslator) departDocument(node *Node) {
	t.headPrefix = append(t.headPrefix, t.doctype,
		`<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="`+t.settings.languageCode+
			`" lang="`+t.settings.languageCode+`">`+"\n<head>\n")
	t.htmlProlog = append(t.htmlProlog, t.doctype)
	t.head = append(append([]string{}, t.meta...), t.head...)
	if strings.Contains(strings.Join(t.meta, ""), `name="dcterms.`) {
		t.head = append(t.head, `<link rel="schema.dcterms" href="http://purl.org/dc/terms/"/>`)
	}
	if len(t.mathHeader) > 0 {
		if t.mathOutput == "mathjax" {
			t.head = append(t.head, t.mathHeader...)
		} else {
			t.stylesheet = append(t.stylesheet, t.mathHeader...)
		}
	}
	t.htmlHead = append(t.htmlHead, t.head[1:]...)
	atts := A{}
	if t.documentTagClass != "" {
		atts["class"] = t.documentTagClass
	}
	t.bodyPrefix = append(t.bodyPrefix, t.starttag(node, t.documentTag, "\n", atts))
	t.bodySuffix = append([]string{"</" + t.documentTag + ">\n"}, t.bodySuffix...)
	t.fragment = append(t.fragment, t.body...)
	t.htmlBody = append(t.htmlBody, t.bodyPrefix[1:]...)
	t.htmlBody = append(t.htmlBody, t.bodyPreDocinfo...)
	t.htmlBody = append(t.htmlBody, t.docinfo...)
	t.htmlBody = append(t.htmlBody, t.body...)
	t.htmlBody = append(t.htmlBody, t.bodySuffix[:len(t.bodySuffix)-1]...)
}

// ---------------------------------------------------------------- images

func (t *htmlTranslator) blockImageSuffix(node *Node) string {
	if !textElementTags[node.Parent.Tag] ||
		(node.Parent.Tag == "reference" && !textElementTags[node.Parent.Parent.Tag]) {
		return "\n"
	}
	return ""
}

var objectImageTypes = map[string]string{".svg": "image/svg+xml",
	".swf": "application/x-shockwave-flash", ".mp4": "video/mp4",
	".webm": "video/webm", ".ogg": "video/ogg"}

func (t *htmlTranslator) visitImage4(node *Node) {
	atts := A{}
	uri := node.str("uri")
	ext := strings.ToLower(path.Ext(uri))
	alt := uri
	if node.has("alt") {
		alt = node.str("alt")
	}
	objType, isObject := objectImageTypes[ext]
	if isObject {
		atts["data"] = uri
		atts["type"] = objType
	} else {
		atts["src"] = uri
		atts["alt"] = alt
	}
	if node.has("width") {
		atts["width"] = node.str("width")
	}
	if node.has("height") {
		atts["height"] = node.str("height")
	}
	if node.has("scale") {
		scale, _ := node.get("scale").(int)
		if (!node.has("width") || !node.has("height")) && t.settings.fileInsertionEnabled {
			if imagepath, e := t.uri2path(uri); e != "" {
				t.doc.reporter.warning("Problem reading image file: "+e, nil)
			} else if w, h, e := readImageSize(imagepath); e != "" {
				t.doc.reporter.warning("Problem reading image file: "+e, nil)
			} else {
				if _, ok := atts["width"]; !ok {
					atts["width"] = fmt.Sprintf("%dpx", w)
				}
				if _, ok := atts["height"]; !ok {
					atts["height"] = fmt.Sprintf("%dpx", h)
				}
			}
		}
		for _, name := range []string{"width", "height"} {
			if v, ok := atts[name].(string); ok {
				if num, unit, ok := scanNumUnit(v); ok {
					f, _ := parseFloat(num)
					atts[name] = pyFloatStr(f*(float64(scale)/100)) + unit
				}
			}
		}
	}
	var style []string
	for _, name := range []string{"width", "height"} {
		if v, ok := atts[name].(string); ok {
			if isNumOnly(v) {
				v += "px"
			}
			style = append(style, name+": "+v+";")
			delete(atts, name)
		}
	}
	if len(style) > 0 {
		atts["style"] = strings.Join(style, " ")
	}
	suffix := t.blockImageSuffix(node)
	if node.has("align") {
		atts["class"] = "align-" + node.str("align")
	}
	if isObject {
		t.out(t.starttag(node, "object", "", atts) + alt + "</object>" + suffix)
	} else {
		t.out(t.emptytag(node, "img", suffix, atts))
	}
}

func parseFloat(s string) (float64, bool) {
	var f float64
	_, err := fmt.Sscanf(s, "%g", &f)
	return f, err == nil
}

func (t *htmlTranslator) imageSize(node *Node) A {
	type measure struct {
		value float64
		unit  string
	}
	dims := []string{"width", "height"}
	measures := map[string]measure{}
	var order []string
	for _, d := range dims {
		if node.has(d) {
			_, v, u, err := parseMeasure(node.str(d))
			if err == nil {
				measures[d] = measure{v, u}
				order = append(order, d)
			}
		}
	}
	if node.has("scale") && len(measures) < 2 {
		if w, h, ok := t.readSize(node); ok {
			for i, d := range dims {
				if _, has := measures[d]; !has {
					measures[d] = measure{float64([]int{w, h}[i]), ""}
				}
			}
			order = nil
			for _, d := range dims {
				if _, has := measures[d]; has {
					order = append(order, d)
				}
			}
		}
	}
	scaling := 1.0
	if s, ok := node.get("scale").(int); ok {
		scaling = float64(s) / 100
	}
	atts := A{}
	var declarations []string
	for _, d := range order {
		m := measures[d]
		value := m.value * scaling
		if m.unit != "" {
			declarations = append(declarations, fmt.Sprintf("%s: %s%s;", d, fmtG(value), m.unit))
		} else {
			atts[d] = fmt.Sprint(pyRound(value))
		}
	}
	if len(declarations) > 0 {
		atts["style"] = strings.Join(declarations, " ")
	}
	return atts
}

// readSize mirrors read_size_with_PIL.
func (t *htmlTranslator) readSize(node *Node) (int, int, bool) {
	uri := node.str("uri")
	var problems []string
	if isVideoType(guessMimeType(uri)) {
		problems = append(problems, "PIL cannot read video images.")
	}
	if !t.settings.fileInsertionEnabled {
		problems = append(problems, "Reading external files disabled.")
	}
	var w, h int
	if len(problems) == 0 {
		imagepath, e := t.uri2path(uri)
		if e == "" {
			w, h, e = readImageSize(imagepath)
		}
		if e != "" {
			problems = append(problems, e)
		}
	}
	if len(problems) > 0 {
		msg := append([]string{"Cannot scale image!", fmt.Sprintf(`Could not get size from "%s":`, uri)}, problems...)
		t.messages = append(t.messages, t.doc.reporter.warning(strings.Join(msg, "\n  "), nil, baseNode(node)))
		return 0, 0, false
	}
	return w, h, true
}

func (t *htmlTranslator) visitImage5(node *Node) {
	uri := node.str("uri")
	alt := uri
	if node.has("alt") {
		alt = node.str("alt")
	}
	mimetype := guessMimeType(uri)
	element := ""
	atts := t.imageSize(node)
	if node.has("align") {
		atts["classes"] = []string{"align-" + node.str("align")}
	}
	loading := t.imageLoading
	if isVideoType(mimetype) {
		loading = "link"
	}
	if node.has("loading") {
		loading = node.str("loading")
	}
	if loading == "lazy" {
		atts["loading"] = "lazy"
	} else if loading == "embed" {
		if data, mt, err := t.embedImage(uri, mimetype); err != "" {
			t.messages = append(t.messages, t.doc.reporter.error(
				fmt.Sprintf("Cannot embed image \"%s\":\n  %s", uri, err), nil, baseNode(node)))
		} else {
			uri = "data:" + mt + ";base64," + data
		}
	}
	suffix := t.blockImageSuffix(node)
	if isVideoType(mimetype) {
		atts["title"] = alt
		if hasClass(node, "controls") {
			node.removeFrom("classes", "controls")
			atts["controls"] = "controls"
		}
		atts["src"] = uri
		element = t.starttag(node, "video", suffix, atts) +
			fmt.Sprintf(`<a href="%s">%s</a>%s`, node.str("uri"), alt, suffix) + "</video>" + suffix
	} else if mimetype == "application/x-shockwave-flash" {
		atts["type"] = mimetype
		atts["data"] = uri
		element = t.starttag(node, "object", "", atts) + alt + "</object>" + suffix
	} else {
		atts["alt"] = alt
		atts["src"] = uri
		element = t.emptytag(node, "img", suffix, atts)
	}
	t.out(element)
	if suffix != "" {
		t.reportMessages(node)
	}
}
