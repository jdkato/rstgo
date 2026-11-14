package rstgo

import (
	"fmt"
)

func applySmartQuotes(d *Document) {}

// parseInto runs the reStructuredText parser over text, filling doc.
func parseInto(doc *Document, text string) {
	st := doc.settings
	old := doc.reporter.observer
	doc.reporter.observer = func(m *Node) { doc.parseMessages = append(doc.parseMessages, m) }
	defer func() { doc.reporter.observer = old }()
	m := &memo{document: doc, reporter: doc.reporter, inliner: newInliner(st),
		roles: &roleRegistry{local: map[string]*role{}}, directives: map[string]*directiveSpec{}}
	inputlines := string2lines(text, st.tabWidth, true)
	for i, line := range inputlines {
		if runeLen(line) > st.lineLengthLimit {
			doc.Root.append(doc.reporter.error(fmt.Sprintf("Line %d exceeds the line-length-limit.", i+1), nil))
			return
		}
	}
	sm := newStateMachine("Body", nil)
	src := doc.Root.str("source")
	sm.runTop(newStringListOwned(inputlines, src, true), doc, m)
}
