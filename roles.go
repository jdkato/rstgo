package rstgo

// Port of docutils.parsers.rst.roles.

import (
	"fmt"
	"strings"
)

type roleFunc func(r *role, roleName, rawtext, text string, lineno int, inl *inliner,
	options map[string]any, content []string) ([]*Node, []*Node)

type role struct {
	name    string
	fn      roleFunc
	options map[string]optConv
	content bool
	// GenericRole
	nodeTag string
	// CustomRole
	base            *role
	suppliedOptions map[string]any
	suppliedContent []string
}

func (r *role) call(roleName, rawtext, text string, lineno int, inl *inliner,
	options map[string]any, content []string) ([]*Node, []*Node) {
	return r.fn(r, roleName, rawtext, text, lineno, inl, options, content)
}

type roleRegistry struct {
	local map[string]*role
}

const defaultInterpretedRole = "title-reference"

var canonicalRoles map[string]*role

var enRoles = map[string]string{
	"abbreviation": "abbreviation", "ab": "abbreviation", "acronym": "acronym",
	"ac": "acronym", "code": "code", "emphasis": "emphasis", "literal": "literal",
	"math": "math", "pep-reference": "pep-reference", "pep": "pep-reference",
	"rfc-reference": "rfc-reference", "rfc": "rfc-reference", "strong": "strong",
	"subscript": "subscript", "sub": "subscript", "superscript": "superscript",
	"sup": "superscript", "title-reference": "title-reference",
	"title": "title-reference", "t": "title-reference", "raw": "raw",
	"index": "index", "i": "index", "anonymous-reference": "anonymous-reference",
	"citation-reference": "citation-reference", "footnote-reference": "footnote-reference",
	"named-reference": "named-reference", "substitution-reference": "substitution-reference",
	"uri-reference": "uri-reference", "uri": "uri-reference", "url": "uri-reference",
	"target": "target",
}

func init() {
	canonicalRoles = map[string]*role{}
	generic := func(name, tag string) {
		canonicalRoles[name] = &role{name: name, fn: genericRole, nodeTag: tag}
	}
	generic("abbreviation", "abbreviation")
	generic("acronym", "acronym")
	generic("emphasis", "emphasis")
	generic("literal", "literal")
	generic("strong", "strong")
	generic("subscript", "subscript")
	generic("superscript", "superscript")
	generic("title-reference", "title_reference")
	canonicalRoles["pep-reference"] = &role{name: "pep-reference", fn: pepReferenceRole}
	canonicalRoles["rfc-reference"] = &role{name: "rfc-reference", fn: rfcReferenceRole}
	canonicalRoles["raw"] = &role{name: "raw", fn: rawRole, options: map[string]optConv{"format": optUnchanged}}
	canonicalRoles["code"] = &role{name: "code", fn: codeRole, options: map[string]optConv{"language": optUnchanged}}
	canonicalRoles["math"] = &role{name: "math", fn: mathRole}
	for _, n := range []string{"index", "named-reference", "anonymous-reference",
		"uri-reference", "footnote-reference", "citation-reference",
		"substitution-reference", "target", "restructuredtext-unimplemented-role"} {
		canonicalRoles[n] = &role{name: n, fn: unimplementedRole}
	}
	for _, r := range canonicalRoles {
		setImplicitOptions(r)
	}
}

func setImplicitOptions(r *role) {
	if r.options == nil {
		r.options = map[string]optConv{}
	}
	if _, ok := r.options["class"]; !ok {
		r.options["class"] = optClass
	}
}

// lookupRole mirrors roles.role().
func lookupRole(roleName string, m *memo, lineno int, rep *reporter) (*role, []*Node) {
	normname := pyLower(roleName)
	var messages []*Node
	var msgText []string
	if r, ok := m.roles.local[normname]; ok {
		return r, messages
	}
	canonicalname := ""
	if roleName != "" {
		if c, ok := enRoles[normname]; ok {
			canonicalname = c
		} else {
			msgText = append(msgText, fmt.Sprintf(`No role entry for "%s" in module "docutils.parsers.rst.languages.en".`, roleName))
		}
	} else {
		canonicalname = defaultInterpretedRole
	}
	if canonicalname == "" {
		if c, ok := enRoles[normname]; ok {
			canonicalname = c
			msgText = append(msgText, fmt.Sprintf(`Using English fallback for role "%s".`, roleName))
		} else {
			msgText = append(msgText, fmt.Sprintf(`Trying "%s" as canonical role name.`, roleName))
			canonicalname = normname
		}
	}
	if len(msgText) > 0 {
		messages = append(messages, rep.info(strings.Join(msgText, "\n"), nil, atLine(lineno)))
	}
	if r, ok := canonicalRoles[canonicalname]; ok {
		m.roles.local[normname] = r
		return r, messages
	}
	return nil, messages
}

func normalizeRoleOptions(options map[string]any) map[string]any {
	n := map[string]any{}
	for k, v := range options {
		if k == "class" {
			n["classes"] = v
		} else {
			n[k] = v
		}
	}
	return n
}

func applyOptions(n *Node, options map[string]any) {
	for k, v := range options {
		if l, ok := v.([]string); ok {
			n.Attrs[k] = append([]string{}, l...)
		} else {
			n.Attrs[k] = v
		}
	}
}

func genericRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	n := newTextElem(r.nodeTag, rawtext, text)
	applyOptions(n, normalizeRoleOptions(options))
	return []*Node{n}, nil
}

func customRoleFn(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	opts := normalizeRoleOptions(r.suppliedOptions)
	for k, v := range options {
		opts[k] = v
	}
	var c []string
	c = append(c, r.suppliedContent...)
	if len(r.suppliedContent) > 0 && len(content) > 0 {
		c = append(c, "\n")
	}
	c = append(c, content...)
	return r.base.call(roleName, rawtext, text, lineno, inl, opts, c)
}

func genericCustomRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	n := newTextElem("inline", rawtext, text)
	applyOptions(n, normalizeRoleOptions(options))
	return []*Node{n}, nil
}

func pepReferenceRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	options = normalizeRoleOptions(options)
	pepnum, ok := pyInt(unescape(text, false))
	if !ok || pepnum < 0 || pepnum > 9999 {
		msg := inl.reporter.error(fmt.Sprintf(`PEP number must be a number from 0 to 9999; "%s" is invalid.`, text), nil, atLine(lineno))
		prb := inl.problematic(rawtext, rawtext, msg)
		return []*Node{prb}, []*Node{msg}
	}
	st := inl.document.settings
	ref := st.pepBaseURL + fmt.Sprintf(st.pepFileURLTemplate, pepnum)
	n := newTextElem("reference", rawtext, "PEP "+text)
	n.setAttr("refuri", ref)
	applyOptions(n, options)
	return []*Node{n}, nil
}

func rfcReferenceRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	options = normalizeRoleOptions(options)
	rfcStr, section, hasSection := unescape(text, false), "", false
	if strings.Contains(text, "#") {
		parts := strings.SplitN(unescape(text, false), "#", 2)
		rfcStr, section, hasSection = parts[0], parts[1], true
	}
	rfcnum, ok := pyInt(rfcStr)
	if !ok || rfcnum < 1 {
		msg := inl.reporter.error(fmt.Sprintf("RFC number must be a number greater than or equal to 1; "+
			`"%s" is invalid.`, text), nil, atLine(lineno))
		prb := inl.problematic(rawtext, rawtext, msg)
		return []*Node{prb}, []*Node{msg}
	}
	ref := inl.document.settings.rfcBaseURL + fmt.Sprintf("rfc%d.html", rfcnum)
	if hasSection {
		ref += "#" + section
	}
	n := newTextElem("reference", rawtext, fmt.Sprintf("RFC %d", rfcnum))
	n.setAttr("refuri", ref)
	applyOptions(n, options)
	return []*Node{n}, nil
}

// pyInt mimics Python's int(str) for decimal strings.
func pyInt(s string) (int, bool) {
	s = pyStrip(s)
	if s == "" {
		return 0, false
	}
	neg := false
	if s[0] == '+' || s[0] == '-' {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0, false
	}
	n := 0
	prevUnderscore := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			if prevUnderscore {
				return 0, false
			}
			prevUnderscore = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		prevUnderscore = false
		if n < 1<<55 {
			n = n*10 + int(c-'0')
		}
	}
	if prevUnderscore {
		return 0, false
	}
	if neg {
		n = -n
	}
	return n, true
}

func rawRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	options = normalizeRoleOptions(options)
	if !inl.document.settings.rawEnabled {
		msg := inl.reporter.warning("raw (and derived) roles disabled", nil)
		prb := inl.problematic(rawtext, rawtext, msg)
		return []*Node{prb}, []*Node{msg}
	}
	if _, ok := options["format"]; !ok {
		msg := inl.reporter.error(fmt.Sprintf("No format (Writer name) is associated with this role: \"%s\".\n"+
			"The \"raw\" role cannot be used directly.\n"+
			"Instead, use the \"role\" directive to create a new role with "+
			"an associated format.", roleName), nil, atLine(lineno))
		prb := inl.problematic(rawtext, rawtext, msg)
		return []*Node{prb}, []*Node{msg}
	}
	n := newTextElem("raw", rawtext, unescape(text, true))
	applyOptions(n, options)
	n.setSrcLine(inl.reporter.getSourceAndLine(lineno, true))
	return []*Node{n}, nil
}

const pygmentsMissing = "Cannot analyze code. Pygments package not found."

func codeRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	options = normalizeRoleOptions(options)
	language := roleName
	if roleName == "code" {
		language = ""
	}
	if l, ok := options["language"]; ok {
		language = l.(string)
	}
	if strings.ToLower(language) == "none" {
		language = ""
	}
	classes := []string{"code"}
	if c, ok := options["classes"]; ok {
		classes = append(classes, c.([]string)...)
	}
	if language != "" && !containsStr(classes, language) {
		classes = append(classes, language)
	}
	// Lexer(): without Pygments, any language (with highlighting enabled)
	// raises LexerError.
	if lexerFails(language, inl.document.settings.syntaxHighlight) {
		if _, ok := options["language"]; ok {
			msg := inl.reporter.warning(pygmentsMissing, nil)
			prb := inl.problematic(rawtext, rawtext, msg)
			return []*Node{prb}, []*Node{msg}
		}
	}
	node := newTextElem("literal", rawtext, "")
	node.Attrs["classes"] = classes
	if v := unescape(text, true); v != "" || true {
		node.append(newText(v))
	}
	return []*Node{node}, nil
}

// lexerFails reports whether docutils.utils.code_analyzer.Lexer would raise
// LexerError in an installation without Pygments.
func lexerFails(language, tokennames string) bool {
	if language == "" || language == "text" || tokennames == "none" {
		return false
	}
	return true
}

func mathRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	options = normalizeRoleOptions(options)
	n := newTextElem("math", rawtext, unescape(text, true))
	applyOptions(n, options)
	return []*Node{n}, nil
}

func unimplementedRole(r *role, roleName, rawtext, text string, lineno int, inl *inliner, options map[string]any, content []string) ([]*Node, []*Node) {
	msg := inl.reporter.error(fmt.Sprintf(`Interpreted text role "%s" not implemented.`, roleName), nil, atLine(lineno))
	prb := inl.problematic(rawtext, rawtext, msg)
	return []*Node{prb}, []*Node{msg}
}
