package rstgo

// Port of docutils.parsers.rst.states.Inliner (inline markup recognition).

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	nonWhitespaceBefore                = `(?<!\s)`
	nonWhitespaceEscapeBefore          = `(?<![\s\x00])`
	nonUnescapedWhitespaceEscapeBefore = `(?<!(?<!\x00)[\s\x00])`
	nonWhitespaceAfter                 = `(?!\s)`
	uric                               = `[-_.!~*'()[\];/:@&=+$,%a-zA-Z0-9\x00]`
	uriEndDelim                        = `[>]`
	urilast                            = `[_~*/=+a-zA-Z0-9]`
	uriEnd                             = `(?:` + urilast + `|` + uric + `(?=` + uriEndDelim + `))`
	emailc                             = "[-_!~*'{|}/#?^`&=+$%a-zA-Z0-9\\x00]"
	emailPattern                       = `
          ` + emailc + `+(?:\.` + emailc + `+)*   # name
          (?<!\x00)@                      # at
          ` + emailc + `+(?:\.` + emailc + `*)*   # host
          ` + uriEnd + `                     # final URI char
          `
)

type inlinePatterns struct {
	initial, emphasis, strong, interpretedOrPhraseRef, embeddedLink,
	literal, target, substitutionRef, email, uri, pep, rfc *pyRegexp
}

var (
	wordLevelPatterns *inlinePatterns
	charLevelPatterns *inlinePatterns
)

func buildInlinePatterns(charLevel bool) *inlinePatterns {
	var startStringPrefix, endStringSuffix string
	if charLevel {
		startStringPrefix = `(^|(?<!\x00))`
		endStringSuffix = ``
	} else {
		startStringPrefix = `(^|(?<=\s|[` + punct_openers + punct_delimiters + `]))`
		endStringSuffix = `($|(?=\s|[\x00` + punct_closing_delimiters + punct_delimiters + punct_closers + `]))`
	}
	initial := startStringPrefix + `(?P<initial_inline>` +
		`(?P<start>\*\*|\*(?!\*)|` + "``" + `|_` + "`" + `|\|(?!\|))` + nonWhitespaceAfter +
		`|` +
		`(?P<whole>(?P<refname>` + simplename + `)(?P<refend>__?)` +
		`|\[(?P<footnotelabel>[0-9]+|\#(` + simplename + `)?|\*|(?P<citationlabel>` + simplename + `))(?P<fnend>\]_))` + endStringSuffix +
		`|` +
		`(?P<role>(:` + simplename + `:)?)(?P<backquote>` + "`(?!`)" + `)` + nonWhitespaceAfter +
		`)`
	p := &inlinePatterns{}
	p.initial = pyCompile(initial)
	p.emphasis = pyCompile(nonWhitespaceEscapeBefore + `(\*)` + endStringSuffix)
	p.strong = pyCompile(nonWhitespaceEscapeBefore + `(\*\*)` + endStringSuffix)
	p.interpretedOrPhraseRef = pyCompile(`
              `+nonUnescapedWhitespaceEscapeBefore+`
              (
                `+"`"+`
                (?P<suffix>
                  (?P<role>:`+simplename+`:)?
                  (?P<refend>__?)?
                )
              )
              `+endStringSuffix+`
              `, reVerbose)
	p.embeddedLink = pyCompile(`
              (
                (?:[ \n]+|^)            # spaces or beginning of line/string
                <                       # open bracket
                `+nonWhitespaceAfter+`
                (([^<>]|\x00[<>])+)     # anything but unescaped angle brackets
                `+nonWhitespaceEscapeBefore+`
                >                       # close bracket
              )
              $                         # end of string
              `, reVerbose)
	p.literal = pyCompile(nonWhitespaceBefore + "(``)" + endStringSuffix)
	p.target = pyCompile(nonWhitespaceEscapeBefore + "(`)" + endStringSuffix)
	p.substitutionRef = pyCompile(nonWhitespaceEscapeBefore + `(\|_{0,2})` + endStringSuffix)
	p.email = pyCompile(emailPattern+`$`, reVerbose)
	p.uri = pyCompile(`
                `+startStringPrefix+`
                (?P<whole>
                  (?P<absolute>           # absolute URI
                    (?P<scheme>             # scheme (http, ftp, mailto)
                      [a-zA-Z][a-zA-Z0-9.+-]*
                    )
                    :
                    (
                      (                       # either:
                        (//?)?                  # hierarchical URI
                        `+uric+`*               # URI characters
                        `+uriEnd+`             # final URI char
                      )
                      (                       # optional query
                        \?`+uric+`*
                        `+uriEnd+`
                      )?
                      (                       # optional fragment
                        \#`+uric+`*
                        `+uriEnd+`
                      )?
                    )
                  )
                |                       # *OR*
                  (?P<email>              # email address
                    `+emailPattern+`
                  )
                )
                `+endStringSuffix+`
                `, reVerbose)
	p.pep = pyCompile(`
                `+startStringPrefix+`
                (
                  (pep-(?P<pepnum1>\d+)(.txt)?) # reference to source file
                |
                  (PEP\s+(?P<pepnum2>\d+))      # reference by name
                )
                `+endStringSuffix, reVerbose)
	if !charLevel {
		p.initial.prefilter = initialPrefilter
		p.uri.quick = func(s string) bool { return strings.ContainsAny(s, ":@") }
		p.uri.prefilter = uriPrefilter
	}
	p.rfc = pyCompile(`
                `+startStringPrefix+`
                (RFC(-|\s+)?(?P<rfcnum>\d+))
                `+endStringSuffix, reVerbose)
	return p
}

type implicitFn func(inl *inliner, m *pyMatch, lineno int) ([]*Node, bool)

type implicitEntry struct {
	pattern *pyRegexp
	method  implicitFn
}

type inliner struct {
	patterns         *inlinePatterns
	implicitDispatch []implicitEntry
	document         *Document
	reporter         *reporter
	parent           *Node
	memo             *memo
}

func newInliner(st *settings) *inliner {
	inl := &inliner{}
	if st.characterLevelInlineMarkup {
		if charLevelPatterns == nil {
			charLevelPatterns = buildInlinePatterns(true)
		}
		inl.patterns = charLevelPatterns
	} else {
		if wordLevelPatterns == nil {
			wordLevelPatterns = buildInlinePatterns(false)
		}
		inl.patterns = wordLevelPatterns
	}
	inl.implicitDispatch = append(inl.implicitDispatch, implicitEntry{inl.patterns.uri, (*inliner).standaloneURI})
	if st.pepReferences {
		inl.implicitDispatch = append(inl.implicitDispatch, implicitEntry{inl.patterns.pep, (*inliner).pepReference})
	}
	if st.rfcReferences {
		inl.implicitDispatch = append(inl.implicitDispatch, implicitEntry{inl.patterns.rfc, (*inliner).rfcReference})
	}
	return inl
}

func (inl *inliner) parse(text string, lineno int, m *memo, parent *Node) ([]*Node, []*Node) {
	inl.document = m.document
	inl.reporter = m.document.reporter
	inl.parent = parent
	inl.memo = m
	remaining := escape2null(text)
	var processed, messages []*Node
	var unprocessed []string
	for remaining != "" {
		match := inl.patterns.initial.Search(remaining)
		if match == nil {
			break
		}
		key := match.Named("start")
		if key == "" {
			key = match.Named("backquote")
		}
		if key == "" {
			key = match.Named("refend")
		}
		if key == "" {
			key = match.Named("fnend")
		}
		var before string
		var inlines, sysmessages []*Node
		switch key {
		case "*":
			before, inlines, remaining, sysmessages, _ = inl.inlineObj(match, lineno, inl.patterns.emphasis, "emphasis", false)
		case "**":
			before, inlines, remaining, sysmessages, _ = inl.inlineObj(match, lineno, inl.patterns.strong, "strong", false)
		case "`":
			before, inlines, remaining, sysmessages = inl.interpretedOrPhraseRef(match, lineno)
		case "``":
			before, inlines, remaining, sysmessages, _ = inl.inlineObj(match, lineno, inl.patterns.literal, "literal", true)
		case "_`":
			before, inlines, remaining, sysmessages = inl.inlineInternalTarget(match, lineno)
		case "]_":
			before, inlines, remaining, sysmessages = inl.footnoteReference(match, lineno)
		case "|":
			before, inlines, remaining, sysmessages = inl.substitutionReference(match, lineno)
		case "_":
			before, inlines, remaining, sysmessages = inl.reference(match, lineno, false)
		case "__":
			before, inlines, remaining, sysmessages = inl.reference(match, lineno, true)
		default:
			panic("inliner: bad dispatch key " + strconv.Quote(key))
		}
		unprocessed = append(unprocessed, before)
		messages = append(messages, sysmessages...)
		if len(inlines) > 0 {
			processed = append(processed, inl.implicitInline(strings.Join(unprocessed, ""), lineno)...)
			processed = append(processed, inlines...)
			unprocessed = nil
		}
	}
	remaining = strings.Join(unprocessed, "") + remaining
	if remaining != "" {
		processed = append(processed, inl.implicitInline(remaining, lineno)...)
	}
	return processed, messages
}

var (
	openerRunes = []rune(punct_openers)
	closerRunes = []rune(punct_closers)
)

// matchChars mirrors punctuation_chars.match_chars.
func matchChars(c1, c2 rune) bool {
	i := -1
	for j, r := range openerRunes {
		if r == c1 {
			i = j
			break
		}
	}
	if i < 0 {
		return false
	}
	if i < len(closerRunes) && c2 == closerRunes[i] {
		return true
	}
	return strings.ContainsRune(quotePairs[c1], c2)
}

func (inl *inliner) quotedStart(match *pyMatch) bool {
	s := match.s
	start := match.Start(0)
	if start == 0 {
		return false
	}
	prestart, _ := utf8.DecodeLastRuneInString(s[:start])
	if match.End(0) >= len(s) {
		return true
	}
	poststart, _ := utf8.DecodeRuneInString(s[match.End(0):])
	return matchChars(prestart, poststart)
}

func (inl *inliner) inlineObj(match *pyMatch, lineno int, endPattern *pyRegexp, tag string, restoreBackslashes bool) (string, []*Node, string, []*Node, string) {
	s := match.s
	matchstart := match.StartNamed("start")
	matchend := match.EndNamed("start")
	if inl.quotedStart(match) {
		return s[:matchend], nil, s[matchend:], nil, ""
	}
	endmatch := endPattern.Search(s[matchend:])
	if endmatch != nil && endmatch.Start(1) > 0 {
		text := endmatch.s[:endmatch.Start(1)]
		if restoreBackslashes {
			text = unescape(text, true)
		}
		textend := matchend + endmatch.End(1)
		rawsource := unescape(s[matchstart:textend], true)
		node := newTextElem(tag, rawsource, text)
		return s[:matchstart], []*Node{node}, s[textend:], nil, endmatch.Group(1)
	}
	name := tag
	if tag == "substitution_reference" || tag == "target" {
		name = tag
	}
	msg := inl.reporter.warning(fmt.Sprintf("Inline %s start-string without end-string.", name), nil, atLine(lineno))
	text := unescape(s[matchstart:matchend], true)
	prb := inl.problematic(text, text, msg)
	return s[:matchstart], []*Node{prb}, s[matchend:], []*Node{msg}, ""
}

func (inl *inliner) problematic(text, rawsource string, message *Node) *Node {
	msgid := inl.document.setID(message, inl.parent, "")
	problematic := newTextElem("problematic", rawsource, text)
	problematic.setAttr("refid", msgid)
	prbid := inl.document.setID(problematic, nil, "")
	message.addBackref(prbid)
	return problematic
}

func (inl *inliner) interpretedOrPhraseRef(match *pyMatch, lineno int) (string, []*Node, string, []*Node) {
	endPattern := inl.patterns.interpretedOrPhraseRef
	s := match.s
	matchstart := match.StartNamed("backquote")
	matchend := match.EndNamed("backquote")
	rolestart := match.StartNamed("role")
	role := match.Named("role")
	position := ""
	if role != "" {
		role = role[1 : len(role)-1]
		position = "prefix"
	} else if inl.quotedStart(match) {
		return s[:matchend], nil, s[matchend:], nil
	}
	endmatch := endPattern.Search(s[matchend:])
	if endmatch != nil && endmatch.Start(1) > 0 {
		textend := matchend + endmatch.End(0)
		if endmatch.Named("role") != "" {
			if role != "" {
				msg := inl.reporter.warning("Multiple roles in interpreted text (both "+
					"prefix and suffix present; only one allowed).", nil, atLine(lineno))
				text := unescape(s[rolestart:textend], true)
				prb := inl.problematic(text, text, msg)
				return s[:rolestart], []*Node{prb}, s[textend:], []*Node{msg}
			}
			sfx := endmatch.Named("suffix")
			role = sfx[1 : len(sfx)-1]
			position = "suffix"
		}
		escaped := endmatch.s[:endmatch.Start(1)]
		rawsource := unescape(s[matchstart:textend], true)
		if strings.HasSuffix(rawsource, "_") {
			if role != "" {
				msg := inl.reporter.warning(fmt.Sprintf("Mismatch: both interpreted text role %s and "+
					"reference suffix.", position), nil, atLine(lineno))
				text := unescape(s[rolestart:textend], true)
				prb := inl.problematic(text, text, msg)
				return s[:rolestart], []*Node{prb}, s[textend:], []*Node{msg}
			}
			return inl.phraseRef(s[:matchstart], s[textend:], rawsource, escaped)
		}
		rawsource = unescape(s[rolestart:textend], true)
		nodelist, messages := inl.interpreted(rawsource, escaped, role, lineno)
		return s[:rolestart], nodelist, s[textend:], messages
	}
	msg := inl.reporter.warning("Inline interpreted text or phrase reference start-string "+
		"without end-string.", nil, atLine(lineno))
	text := unescape(s[matchstart:matchend], true)
	prb := inl.problematic(text, text, msg)
	return s[:matchstart], []*Node{prb}, s[matchend:], []*Node{msg}
}

func (inl *inliner) phraseRef(before, after, rawsource, escaped string) (string, []*Node, string, []*Node) {
	match := inl.patterns.embeddedLink.Search(escaped)
	var text, unescaped, rawtext, alias, aliastype string
	var target *Node
	if match != nil {
		text = escaped[:match.Start(0)]
		unescaped = unescape(text, false)
		rawtext = unescape(text, true)
		aliastext := match.Group(2)
		rawaliastext := unescape(aliastext, true)
		underscoreEscaped := strings.HasSuffix(rawaliastext, `\_`)
		if strings.HasSuffix(aliastext, "_") && !(underscoreEscaped || inl.patterns.uri.Match(aliastext) != nil) {
			aliastype = "name"
			alias = fullyNormalizeName(unescape(aliastext[:len(aliastext)-1], false))
			target = newTextElem("target", match.Group(1), "")
			target.setAttr("refname", alias)
			target.indirectRefName = whitespaceNormalizeName(unescape(aliastext[:len(aliastext)-1], false))
			target.hasIndirectName = true
		} else {
			aliastype = "uri"
			aliasParts := splitEscapedWhitespace(match.Group(2))
			for i, part := range aliasParts {
				aliasParts[i] = strings.Join(pySplit(part), "")
			}
			alias = strings.Join(aliasParts, " ")
			alias = inl.adjustURI(unescape(alias, false))
			if strings.HasSuffix(alias, `\_`) {
				alias = alias[:len(alias)-2] + "_"
			}
			target = newTextElem("target", match.Group(1), "")
			target.setAttr("refuri", alias)
			target.referenced = true
		}
		if text == "" {
			text = alias
			unescaped = unescape(text, false)
			rawtext = rawaliastext
		}
	} else {
		text = escaped
		unescaped = unescape(text, false)
		rawtext = unescape(escaped, true)
	}
	refname := fullyNormalizeName(unescaped)
	reference := newTextElem("reference", rawsource, text)
	reference.setAttr("name", whitespaceNormalizeName(unescaped))
	if len(reference.Children) > 0 {
		reference.Children[0].Rawsource = rawtext
	}
	nodeList := []*Node{reference}
	if strings.HasSuffix(rawsource, "__") {
		if target != nil && aliastype == "name" {
			reference.setAttr("refname", alias)
			inl.document.noteRefname(reference)
		} else if target != nil && aliastype == "uri" {
			reference.setAttr("refuri", alias)
		} else {
			reference.setAttr("anonymous", true)
		}
	} else {
		if target != nil {
			target.appendTo("names", refname)
			if aliastype == "name" {
				reference.setAttr("refname", alias)
				inl.document.noteIndirectTarget(target)
				inl.document.noteRefname(reference)
			} else {
				reference.setAttr("refuri", alias)
			}
			inl.document.noteImplicitTarget(target, inl.parent)
			nodeList = append(nodeList, target)
		} else {
			reference.setAttr("refname", refname)
			inl.document.noteRefname(reference)
		}
	}
	return before, nodeList, after, nil
}

func (inl *inliner) adjustURI(uri string) string {
	if inl.patterns.email.Match(uri) != nil {
		return "mailto:" + uri
	}
	return uri
}

func (inl *inliner) interpreted(rawsource, text, role string, lineno int) ([]*Node, []*Node) {
	r, messages := lookupRole(role, inl.memo, lineno, inl.reporter)
	if r != nil {
		nodes, messages2 := r.call(role, rawsource, text, lineno, inl, nil, nil)
		return nodes, append(messages, messages2...)
	}
	msg := inl.reporter.error(fmt.Sprintf(`Unknown interpreted text role "%s".`, role), nil, atLine(lineno))
	return []*Node{inl.problematic(rawsource, rawsource, msg)}, append(messages, msg)
}

func (inl *inliner) inlineInternalTarget(match *pyMatch, lineno int) (string, []*Node, string, []*Node) {
	before, inlines, remaining, sysmessages, _ := inl.inlineObj(match, lineno, inl.patterns.target, "target", false)
	if len(inlines) > 0 && inlines[0].Tag == "target" {
		target := inlines[0]
		name := fullyNormalizeName(target.astext())
		target.appendTo("names", name)
		inl.document.noteExplicitTarget(target, inl.parent)
	}
	return before, inlines, remaining, sysmessages
}

func (inl *inliner) substitutionReference(match *pyMatch, lineno int) (string, []*Node, string, []*Node) {
	before, inlines, remaining, sysmessages, endstring := inl.inlineObj(match, lineno,
		inl.patterns.substitutionRef, "substitution_reference", false)
	if len(inlines) == 1 {
		subrefNode := inlines[0]
		if subrefNode.Tag == "substitution_reference" {
			subrefText := subrefNode.astext()
			inl.document.noteSubstitutionRef(subrefNode, subrefText)
			if strings.HasSuffix(endstring, "_") {
				referenceNode := newTextElem("reference", "|"+subrefText+endstring, "")
				if strings.HasSuffix(endstring, "__") {
					referenceNode.setAttr("anonymous", true)
				} else {
					referenceNode.setAttr("refname", fullyNormalizeName(subrefText))
					inl.document.noteRefname(referenceNode)
				}
				referenceNode.append(subrefNode)
				inlines = []*Node{referenceNode}
			}
		}
	}
	return before, inlines, remaining, sysmessages
}

func (inl *inliner) footnoteReference(match *pyMatch, lineno int) (string, []*Node, string, []*Node) {
	label := match.Named("footnotelabel")
	refname := fullyNormalizeName(label)
	s := match.s
	before := s[:match.StartNamed("whole")]
	remaining := s[match.EndNamed("whole"):]
	var refnode *Node
	if match.Named("citationlabel") != "" {
		refnode = newTextElem("citation_reference", "["+label+"]_", "")
		refnode.setAttr("refname", refname)
		refnode.append(newText(label))
		inl.document.noteCitationRef(refnode)
	} else {
		refnode = newTextElem("footnote_reference", "["+label+"]_", "")
		if refname[0] == '#' {
			refname = refname[1:]
			refnode.setAttr("auto", 1)
			inl.document.noteAutofootnoteRef(refnode)
		} else if refname == "*" {
			refname = ""
			refnode.setAttr("auto", "*")
			inl.document.noteSymbolFootnoteRef(refnode)
		} else {
			refnode.append(newText(label))
		}
		if refname != "" {
			refnode.setAttr("refname", refname)
			inl.document.noteFootnoteRef(refnode)
		}
		if inl.document.settings.trimFootnoteRefSpace() {
			before = pyRstrip(before)
		}
	}
	return before, []*Node{refnode}, remaining, nil
}

func (inl *inliner) reference(match *pyMatch, lineno int, anonymous bool) (string, []*Node, string, []*Node) {
	referencename := match.Named("refname")
	refname := fullyNormalizeName(referencename)
	referencenode := newTextElem("reference", referencename+match.Named("refend"), referencename)
	referencenode.setAttr("name", whitespaceNormalizeName(referencename))
	referencenode.Children[0].Rawsource = referencename
	if anonymous {
		referencenode.setAttr("anonymous", true)
	} else {
		referencenode.setAttr("refname", refname)
		inl.document.noteRefname(referencenode)
	}
	s := match.s
	return s[:match.StartNamed("whole")], []*Node{referencenode}, s[match.EndNamed("whole"):], nil
}

func (inl *inliner) standaloneURI(match *pyMatch, lineno int) ([]*Node, bool) {
	scheme := match.Named("scheme")
	if scheme == "" || uriSchemes[strings.ToLower(scheme)] {
		addscheme := ""
		if match.Named("email") != "" {
			addscheme = "mailto:"
		}
		text := match.Named("whole")
		refuri := addscheme + unescape(text, false)
		reference := newTextElem("reference", unescape(text, true), text)
		reference.setAttr("refuri", refuri)
		return []*Node{reference}, true
	}
	return nil, false
}

func (inl *inliner) pepReference(match *pyMatch, lineno int) ([]*Node, bool) {
	text := match.Group(0)
	var pepnum int
	if strings.HasPrefix(text, "pep-") {
		pepnum = atoi(unescape(match.Named("pepnum1"), false))
	} else if strings.HasPrefix(text, "PEP") {
		pepnum = atoi(unescape(match.Named("pepnum2"), false))
	} else {
		return nil, false
	}
	st := inl.document.settings
	ref := st.pepBaseURL + fmt.Sprintf(pyFmtToGo(st.pepFileURLTemplate), pepnum)
	n := newTextElem("reference", unescape(text, true), text)
	n.setAttr("refuri", ref)
	return []*Node{n}, true
}

func pyFmtToGo(f string) string { return f }

func (inl *inliner) rfcReference(match *pyMatch, lineno int) ([]*Node, bool) {
	text := match.Group(0)
	if !strings.HasPrefix(text, "RFC") {
		return nil, false
	}
	rfcnum := atoi(unescape(match.Named("rfcnum"), false))
	ref := inl.document.settings.rfcBaseURL + fmt.Sprintf("rfc%d.html", rfcnum)
	n := newTextElem("reference", unescape(text, true), text)
	n.setAttr("refuri", ref)
	return []*Node{n}, true
}

func (inl *inliner) implicitInline(text string, lineno int) []*Node {
	if text == "" {
		return nil
	}
	for _, e := range inl.implicitDispatch {
		match := e.pattern.Search(text)
		if match != nil {
			before := inl.implicitInline(text[:match.Start(0)], lineno)
			nodes, ok := e.method(inl, match, lineno)
			if !ok {
				continue
			}
			out := append(before, nodes...)
			return append(out, inl.implicitInline(text[match.End(0):], lineno)...)
		}
	}
	return []*Node{newText(text)}
}

// initialPrefilter cheaply rejects positions where the "initial" inline
// pattern (word-level markup) cannot match: it must start with a markup
// character, or with a word that is not preceded by a word character and
// that is followed by "_" before the next whitespace (a reference name).
func initialPrefilter(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	c := s[pos]
	switch c {
	case '*', '`', '_', '|', '[', ':':
		return true
	}
	if c < 0x80 && !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		return false
	}
	if pos > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:pos])
		if isPyWord(r) {
			return false
		}
	}
	for i := pos; i < len(s); i++ {
		switch s[i] {
		case '_':
			return true
		case ' ', '\n', '\t':
			return false
		}
	}
	return false
}

// uriPrefilter rejects positions where neither a URI scheme nor an e-mail
// address can start.
func uriPrefilter(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	c := s[pos]
	if c >= 0x80 || c == ' ' || c == '\n' {
		return false
	}
	if pos > 0 {
		p := s[pos-1]
		if p >= '0' && p <= '9' || p >= 'a' && p <= 'z' || p >= 'A' && p <= 'Z' {
			return false
		}
	}
	for i := pos; i < len(s); i++ {
		switch s[i] {
		case ':', '@':
			return true
		case ' ', '\n':
			return false
		}
	}
	return false
}
