package rstgo

// Port of docutils.parsers.rst.states.Inliner (inline markup recognition).

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type implicitFn func(inl *inliner, text string, lineno int) (before string, nodes []*Node, after string, found, ok bool)

type inliner struct {
	scan             inlineScanner
	implicitDispatch []implicitFn
	document         *Document
	reporter         *reporter
	parent           *Node
	memo             *memo
}

func newInliner(st *settings) *inliner {
	inl := &inliner{scan: inlineScanner{charLevel: st.characterLevelInlineMarkup}}
	inl.implicitDispatch = append(inl.implicitDispatch, (*inliner).standaloneURI)
	if st.pepReferences {
		inl.implicitDispatch = append(inl.implicitDispatch, (*inliner).pepReference)
	}
	if st.rfcReferences {
		inl.implicitDispatch = append(inl.implicitDispatch, (*inliner).rfcReference)
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
	// unprocessed text is always a contiguous piece of the input, so it is
	// tracked as a slice instead of a list of strings
	unprocessed := ""
	for remaining != "" {
		match, ok := inl.scan.searchInitial(remaining, 0)
		if !ok {
			break
		}
		var before string
		var inlines, sysmessages []*Node
		s := remaining
		switch match.key {
		case "*":
			before, inlines, remaining, sysmessages, _ = inl.inlineObj(s, match, lineno, "emphasis")
		case "**":
			before, inlines, remaining, sysmessages, _ = inl.inlineObj(s, match, lineno, "strong")
		case "`":
			before, inlines, remaining, sysmessages = inl.interpretedOrPhraseRef(s, match, lineno)
		case "``":
			before, inlines, remaining, sysmessages, _ = inl.inlineObj(s, match, lineno, "literal")
		case "_`":
			before, inlines, remaining, sysmessages = inl.inlineInternalTarget(s, match, lineno)
		case "]_":
			before, inlines, remaining, sysmessages = inl.footnoteReference(s, match, lineno)
		case "|":
			before, inlines, remaining, sysmessages = inl.substitutionReference(s, match, lineno)
		case "_":
			before, inlines, remaining, sysmessages = inl.reference(s, match, lineno, false)
		case "__":
			before, inlines, remaining, sysmessages = inl.reference(s, match, lineno, true)
		}
		unprocessed += before
		messages = append(messages, sysmessages...)
		if len(inlines) > 0 {
			processed = inl.implicitInline(processed, unprocessed, lineno)
			processed = append(processed, inlines...)
			unprocessed = ""
		}
	}
	remaining = unprocessed + remaining
	if remaining != "" {
		processed = inl.implicitInline(processed, remaining, lineno)
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

func (inl *inliner) quotedStart(s string, match initialMatch) bool {
	if match.start == 0 {
		return false
	}
	prestart, _ := utf8.DecodeLastRuneInString(s[:match.start])
	if match.end >= len(s) {
		return true
	}
	poststart, _ := utf8.DecodeRuneInString(s[match.end:])
	return matchChars(prestart, poststart)
}

// inlineObj mirrors Inliner.inline_obj; tag selects the end pattern.
func (inl *inliner) inlineObj(s string, match initialMatch, lineno int, tag string) (string, []*Node, string, []*Node, string) {
	matchstart, matchend := match.markS, match.markE
	if inl.quotedStart(s, match) {
		return s[:matchend], nil, s[matchend:], nil, ""
	}
	endS, endE := -1, -1
	switch tag {
	case "emphasis":
		endS = inl.scan.searchEnd(s, matchend, "*", false)
		endE = endS + 1
	case "strong":
		endS = inl.scan.searchEnd(s, matchend, "**", false)
		endE = endS + 2
	case "literal":
		endS = inl.scan.searchEnd(s, matchend, "``", true)
		endE = endS + 2
	case "target":
		endS = inl.scan.searchEnd(s, matchend, "`", false)
		endE = endS + 1
	case "substitution_reference":
		endS, endE = inl.scan.searchSubstitutionEnd(s, matchend)
	}
	if endS >= 0 {
		text := s[matchend:endS]
		if tag == "literal" {
			text = unescape(text, true)
		}
		rawsource := unescape(s[matchstart:endE], true)
		node := newTextElem(tag, rawsource, text)
		return s[:matchstart], []*Node{node}, s[endE:], nil, s[endS:endE]
	}
	msg := inl.reporter.warning("Inline "+tag+" start-string without end-string.", nil, atLine(lineno))
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

func (inl *inliner) interpretedOrPhraseRef(s string, match initialMatch, lineno int) (string, []*Node, string, []*Node) {
	matchstart, matchend := match.markS, match.markE
	rolestart := match.roleS
	role := s[match.roleS:match.roleE]
	position := ""
	if role != "" {
		role = role[1 : len(role)-1]
		position = "prefix"
	} else if inl.quotedStart(s, match) {
		return s[:matchend], nil, s[matchend:], nil
	}
	endmatch, ok := inl.scan.searchInterpretedEnd(s, matchend)
	if ok {
		textend := endmatch.end
		if endmatch.roleE > endmatch.roleS {
			if role != "" {
				msg := inl.reporter.warning("Multiple roles in interpreted text (both "+
					"prefix and suffix present; only one allowed).", nil, atLine(lineno))
				text := unescape(s[rolestart:textend], true)
				prb := inl.problematic(text, text, msg)
				return s[:rolestart], []*Node{prb}, s[textend:], []*Node{msg}
			}
			// Docutils takes the role name from the "suffix" group
			// (role plus reference suffix) without its first and last
			// character
			sfx := s[endmatch.roleS:textend]
			role = sfx[1 : len(sfx)-1]
			position = "suffix"
		}
		escaped := s[matchend:endmatch.start]
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
	var text, unescaped, rawtext, alias, aliastype string
	var target *Node
	if mstart, g2s, g2e, ok := searchEmbeddedLink(escaped); ok {
		group1 := escaped[mstart : g2e+1]
		text = escaped[:mstart]
		unescaped = unescape(text, false)
		rawtext = unescape(text, true)
		aliastext := escaped[g2s:g2e]
		rawaliastext := unescape(aliastext, true)
		underscoreEscaped := strings.HasSuffix(rawaliastext, `\_`)
		if strings.HasSuffix(aliastext, "_") && !(underscoreEscaped || inl.scan.matchURI(aliastext)) {
			aliastype = "name"
			alias = fullyNormalizeName(unescape(aliastext[:len(aliastext)-1], false))
			target = newTextElem("target", group1, "")
			target.setAttr("refname", alias)
			target.indirectRefName = whitespaceNormalizeName(unescape(aliastext[:len(aliastext)-1], false))
			target.hasIndirectName = true
		} else {
			aliastype = "uri"
			aliasParts := splitEscapedWhitespace(aliastext)
			for i, part := range aliasParts {
				aliasParts[i] = strings.Join(pySplit(part), "")
			}
			alias = strings.Join(aliasParts, " ")
			alias = inl.adjustURI(unescape(alias, false))
			if strings.HasSuffix(alias, `\_`) {
				alias = alias[:len(alias)-2] + "_"
			}
			target = newTextElem("target", group1, "")
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
	if isEmail(uri) {
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

func (inl *inliner) inlineInternalTarget(s string, match initialMatch, lineno int) (string, []*Node, string, []*Node) {
	before, inlines, remaining, sysmessages, _ := inl.inlineObj(s, match, lineno, "target")
	if len(inlines) > 0 && inlines[0].Tag == "target" {
		target := inlines[0]
		name := fullyNormalizeName(target.astext())
		target.appendTo("names", name)
		inl.document.noteExplicitTarget(target, inl.parent)
	}
	return before, inlines, remaining, sysmessages
}

func (inl *inliner) substitutionReference(s string, match initialMatch, lineno int) (string, []*Node, string, []*Node) {
	before, inlines, remaining, sysmessages, endstring := inl.inlineObj(s, match, lineno, "substitution_reference")
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

func (inl *inliner) footnoteReference(s string, match initialMatch, lineno int) (string, []*Node, string, []*Node) {
	label := s[match.nameS:match.nameE]
	refname := fullyNormalizeName(label)
	before := s[:match.start]
	remaining := s[match.end:]
	var refnode *Node
	if match.citation {
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

func (inl *inliner) reference(s string, match initialMatch, lineno int, anonymous bool) (string, []*Node, string, []*Node) {
	referencename := s[match.nameS:match.nameE]
	refname := fullyNormalizeName(referencename)
	referencenode := newTextElem("reference", s[match.nameS:match.end], referencename)
	referencenode.setAttr("name", whitespaceNormalizeName(referencename))
	referencenode.Children[0].Rawsource = referencename
	if anonymous {
		referencenode.setAttr("anonymous", true)
	} else {
		referencenode.setAttr("refname", refname)
		inl.document.noteRefname(referencenode)
	}
	return s[:match.start], []*Node{referencenode}, s[match.end:], nil
}

// The implicit-markup functions search text for one pattern.  found is
// false if the pattern does not occur; ok is false for a match that is
// rejected (Docutils: MarkupMismatch).

func (inl *inliner) standaloneURI(text string, lineno int) (string, []*Node, string, bool, bool) {
	m, found := inl.scan.searchURI(text)
	if !found {
		return "", nil, "", false, false
	}
	scheme := text[m.start:m.schemeEnd]
	if scheme != "" && !uriSchemes[strings.ToLower(scheme)] {
		return "", nil, "", true, false
	}
	addscheme := ""
	if m.email {
		addscheme = "mailto:"
	}
	whole := text[m.start:m.end]
	reference := newTextElem("reference", unescape(whole, true), whole)
	reference.setAttr("refuri", addscheme+unescape(whole, false))
	return text[:m.start], []*Node{reference}, text[m.end:], true, true
}

func (inl *inliner) pepReference(text string, lineno int) (string, []*Node, string, bool, bool) {
	start, end, num, found := inl.scan.searchPEP(text)
	if !found {
		return "", nil, "", false, false
	}
	whole := text[start:end]
	pepnum := atoi(unescape(num, false))
	st := inl.document.settings
	n := newTextElem("reference", unescape(whole, true), whole)
	n.setAttr("refuri", st.pepBaseURL+fmt.Sprintf(st.pepFileURLTemplate, pepnum))
	return text[:start], []*Node{n}, text[end:], true, true
}

func (inl *inliner) rfcReference(text string, lineno int) (string, []*Node, string, bool, bool) {
	start, end, num, found := inl.scan.searchRFC(text)
	if !found {
		return "", nil, "", false, false
	}
	whole := text[start:end]
	rfcnum := atoi(unescape(num, false))
	n := newTextElem("reference", unescape(whole, true), whole)
	n.setAttr("refuri", inl.document.settings.rfcBaseURL+fmt.Sprintf("rfc%d.html", rfcnum))
	return text[:start], []*Node{n}, text[end:], true, true
}

// implicitInline mirrors Inliner.implicit_inline, appending to out.
func (inl *inliner) implicitInline(out []*Node, text string, lineno int) []*Node {
	if text == "" {
		return out
	}
	for _, f := range inl.implicitDispatch {
		before, nodes, after, found, ok := f(inl, text, lineno)
		if !found || !ok {
			continue
		}
		out = inl.implicitInline(out, before, lineno)
		out = append(out, nodes...)
		return inl.implicitInline(out, after, lineno)
	}
	return append(out, newText(text))
}
