package rstgo

import (
	"fmt"
	"io"
)

var levelNames = []string{"DEBUG", "INFO", "WARNING", "ERROR", "SEVERE"}

type reporter struct {
	source      string
	reportLevel int
	haltLevel   int
	stream      io.Writer
	maxLevel    int
	// getSourceAndLine is installed by the parser (first state machine).
	getSourceAndLine func(lineno int, hasLineno bool) (src string, hasSrc bool, line int, hasLine bool)
}

// haltError is raised (via panic) when a system message reaches the halt
// level; it is converted to an error by the public API.
type haltError struct {
	level int
	text  string
}

func (e *haltError) Error() string {
	return fmt.Sprintf("%s\nExiting due to level-%d (%s) system message.", e.text, e.level, levelNames[e.level])
}

type msgOpt func(*msgConf)

type msgConf struct {
	line      int
	hasLine   bool
	source    string
	hasSource bool
	base      *Node
	backrefs  []string
	hasBackr  bool
	extra     map[string]any
}

func atLine(n int) msgOpt { return func(c *msgConf) { c.line, c.hasLine = n, true } }
func atSrcLine(src string, hasSrc bool, line int, hasLine bool) msgOpt {
	return func(c *msgConf) {
		c.source, c.hasSource = src, true
		_ = hasSrc
		if !hasSrc {
			c.source = "None"
		}
		c.line, c.hasLine = line, hasLine
		if !hasLine {
			// line=None passed explicitly: attribute present with value None
			c.hasLine = false
		}
	}
}
func baseNode(n *Node) msgOpt        { return func(c *msgConf) { c.base = n } }
func withBackrefs(b []string) msgOpt { return func(c *msgConf) { c.backrefs, c.hasBackr = b, true } }
func withAttr(k string, v any) msgOpt {
	return func(c *msgConf) {
		if c.extra == nil {
			c.extra = map[string]any{}
		}
		c.extra[k] = v
	}
}

func (r *reporter) systemMessage(level int, message string, children []*Node, opts ...msgOpt) *Node {
	var c msgConf
	for _, o := range opts {
		o(&c)
	}
	attrs := map[string]any{}
	for k, v := range c.extra {
		attrs[k] = v
	}
	if c.hasBackr {
		attrs["backrefs"] = append([]string{}, c.backrefs...)
	}
	if c.hasSource {
		attrs["source"] = c.source
	}
	if c.hasLine {
		attrs["line"] = c.line
	}
	if c.base != nil {
		src, hasSrc, line, hasLine := getSourceLine(c.base)
		if hasSrc {
			if _, ok := attrs["source"]; !ok {
				attrs["source"] = src
			}
		}
		if hasLine {
			if _, ok := attrs["line"]; !ok {
				attrs["line"] = line
			}
		}
	}
	if _, ok := attrs["source"]; !ok {
		if r.getSourceAndLine != nil {
			ln, has := 0, false
			if v, ok := attrs["line"]; ok {
				ln, has = v.(int), true
			}
			src, hasSrc, line, hasLine := r.getSourceAndLine(ln, has)
			if hasSrc {
				attrs["source"] = src
			}
			if hasLine {
				attrs["line"] = line
			}
		}
	}
	if _, ok := attrs["source"]; !ok {
		attrs["source"] = r.source
	}
	msg := newElem("system_message", "")
	if message != "" {
		msg.append(newTextElem("paragraph", "", message))
	}
	for _, ch := range children {
		msg.append(ch)
	}
	for k, v := range attrs {
		msg.Attrs[k] = v
	}
	msg.Attrs["level"] = level
	msg.Attrs["type"] = levelNames[level]
	if r.stream != nil && (level >= r.reportLevel || level >= r.haltLevel) {
		io.WriteString(r.stream, msg.astext()+"\n")
	}
	if level >= r.haltLevel {
		panic(&haltError{level: level, text: msg.astext()})
	}
	if level > r.maxLevel {
		r.maxLevel = level
	}
	return msg
}

func (r *reporter) info(msg string, children []*Node, opts ...msgOpt) *Node {
	return r.systemMessage(1, msg, children, opts...)
}
func (r *reporter) warning(msg string, children []*Node, opts ...msgOpt) *Node {
	return r.systemMessage(2, msg, children, opts...)
}
func (r *reporter) error(msg string, children []*Node, opts ...msgOpt) *Node {
	return r.systemMessage(3, msg, children, opts...)
}
func (r *reporter) severe(msg string, children []*Node, opts ...msgOpt) *Node {
	return r.systemMessage(4, msg, children, opts...)
}

// getSourceLine mirrors utils.get_source_line.
func getSourceLine(node *Node) (src string, hasSrc bool, line int, hasLine bool) {
	for n := node; n != nil; n = n.Parent {
		if (n.hasSrc && n.Source != "") || (n.hasLine && n.Line != 0) {
			return n.Source, n.hasSrc, n.Line, n.hasLine
		}
	}
	return "", false, 0, false
}
