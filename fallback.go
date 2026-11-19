package rstgo

import "strings"

// Docutils reports a directive or role it does not know as an error and
// drops its body. A Sphinx project's documents are full of such names, from
// Sphinx and its extensions, and a reader of the output usually wants the
// words inside them. Options.UnknownDirective and Options.UnknownRole say
// what to do with each unknown name; their zero values keep Docutils'
// behavior.

// DirectiveFallback is what becomes of a directive rstgo has no
// implementation for.
type DirectiveFallback int

const (
	// DirectiveError reports the directive as Docutils does: an error, with
	// the directive's source kept as a literal block.
	DirectiveError DirectiveFallback = iota
	// DirectiveProse parses the directive's content as body text, along
	// with any argument lines after the first: a "versionadded" puts its
	// text there. Options are accepted and ignored.
	DirectiveProse
	// DirectiveLiteral keeps the directive's argument and content as one
	// literal block, for a directive whose body is code or data.
	DirectiveLiteral
)

// RoleFallback is what becomes of an interpreted text role rstgo has no
// implementation for.
type RoleFallback int

const (
	// RoleError reports the role as Docutils does.
	RoleError RoleFallback = iota
	// RoleText keeps the role's text as plain words of the sentence. Text
	// of the form "title <target>" keeps the title alone.
	RoleText
	// RoleReference is RoleText for text that names a title, and a literal
	// for bare text, which is then a target rather than words: ":ref:`intro`"
	// against ":ref:`The introduction <intro>`".
	RoleReference
	// RoleLiteral keeps the role's text as a literal.
	RoleLiteral
)

// anyOptionSpec accepts every option, unchanged, for a fallback directive.
var anyOptionSpec = map[string]optConv{"*": optUnchanged}

func fallbackDirectiveSpec(kind DirectiveFallback) *directiveSpec {
	switch kind {
	case DirectiveProse:
		return &directiveSpec{optionalArgs: 1, finalArgWS: true, hasContent: true, optionSpec: anyOptionSpec, run: runProseDirective}
	case DirectiveLiteral:
		return &directiveSpec{optionalArgs: 1, finalArgWS: true, hasContent: true, optionSpec: anyOptionSpec, run: runLiteralDirective}
	}
	return nil
}

func runProseDirective(d *directive) []*Node {
	node := newElem("container", "")
	node.setSrcLine(d.sm.getSourceAndLine(d.lineno, true))
	if len(d.arguments) > 0 {
		if rest := strings.SplitN(d.arguments[0], "\n", 2); len(rest) == 2 {
			lines := strings.Split(rest[1], "\n")
			src, hasSrc, _, _ := d.sm.getSourceAndLine(d.lineno, true)
			block := newStringList(lines, src, hasSrc)
			d.state.nestedParse(block, d.lineno, node, false)
		}
	}
	if len(d.content.data) > 0 {
		d.state.nestedParse(d.content, d.contentOffset, node, false)
	}
	return []*Node{node}
}

func runLiteralDirective(d *directive) []*Node {
	lines := append(append([]string{}, d.arguments...), d.content.data...)
	text := strings.Join(lines, "\n")
	return []*Node{newTextElem("literal_block", text, text)}
}

func fallbackRole(name string, kind RoleFallback) *role {
	switch kind {
	case RoleText:
		return &role{name: name, fn: textRole, options: map[string]optConv{}}
	case RoleReference:
		return &role{name: name, fn: referenceRole, options: map[string]optConv{}}
	case RoleLiteral:
		return &role{name: name, fn: literalRole, options: map[string]optConv{}}
	}
	return nil
}

func textRole(_ *role, _, _, text string, _ int, _ *inliner, _ map[string]any, _ []string) ([]*Node, []*Node) {
	if title, ok := titleOf(text); ok {
		text = title
	}
	return []*Node{newText(text)}, nil
}

func referenceRole(_ *role, _, rawtext, text string, _ int, _ *inliner, _ map[string]any, _ []string) ([]*Node, []*Node) {
	if title, ok := titleOf(text); ok {
		return []*Node{newText(title)}, nil
	}
	return []*Node{newTextElem("literal", rawtext, text)}, nil
}

func literalRole(_ *role, _, rawtext, text string, _ int, _ *inliner, _ map[string]any, _ []string) ([]*Node, []*Node) {
	return []*Node{newTextElem("literal", rawtext, text)}, nil
}

// titleOf splits "title <target>" and returns the title, trailing space
// trimmed; false when text has no such target at its end.
func titleOf(text string) (string, bool) {
	if !strings.HasSuffix(text, ">") {
		return "", false
	}
	open := strings.LastIndexByte(text, '<')
	if open <= 0 || open == len(text)-2 {
		return "", false
	}
	if strings.ContainsAny(text[open+1:len(text)-1], "<>") {
		return "", false
	}
	title := strings.TrimRight(text[:open], " \t\n\r\f\v")
	if title == "" {
		return "", false
	}
	return title, true
}
