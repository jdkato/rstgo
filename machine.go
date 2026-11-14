package rstgo

// Port of docutils.statemachine.StateMachine(WS) merged with
// docutils.parsers.rst.states.RSTStateMachine / NestedStateMachine.

// Control-flow signals (Python exceptions in Docutils).
type smEOF struct{}
type transitionCorrection struct{ name string }
type stateCorrection struct {
	state      string
	transition string
}
type markupError struct{ msg string }

type transFn func(s *rstState, m lineMatch, context []string, nextState string) ([]string, string)

type transition struct {
	name      string
	scan      lineScanner
	method    transFn
	nextState string
}

type memo struct {
	document    *Document
	reporter    *reporter
	titleStyles []string
	inliner     *inliner
	smCache     []*stateMachine
	roles       *roleRegistry
	directives  map[string]*directiveSpec
}

type stateMachine struct {
	inputLines   *stringList
	inputOffset  int
	line         string
	hasLine      bool
	lineOffset   int
	initialState string
	currentState string
	states       []*rstState
	observing    bool

	matchTitles        bool
	memo               *memo
	document           *Document
	reporter           *reporter
	node               *Node
	sectionLevelOffset int
	parentSM           *stateMachine
	ctxBuf             [1]string
}

func newStateMachine(initialState string, parent *stateMachine) *stateMachine {
	return &stateMachine{initialState: initialState, currentState: initialState,
		parentSM: parent, lineOffset: -1}
}

func (sm *stateMachine) getState(next string) *rstState {
	if next != "" {
		sm.currentState = next
	}
	return sm.state(sm.currentState)
}

func (sm *stateMachine) state(name string) *rstState {
	var st *rstState
	for _, x := range sm.states {
		if x.kind == name {
			st = x
			break
		}
	}
	if st == nil {
		st = newRSTState(name, sm)
		sm.states = append(sm.states, st)
		if sm.memo != nil {
			st.runtimeInit()
		}
	}
	return st
}

func (sm *stateMachine) notifyObservers() {
	if !sm.observing {
		return
	}
	info, hasOffset, ok := sm.inputLines.info(sm.lineOffset)
	if !ok {
		sm.document.noteSource("", false, 0, false)
		return
	}
	src, hasSrc := info.src()
	sm.document.noteSource(src, hasSrc, info.offset, hasOffset)
}

func (sm *stateMachine) lineAt(off int) (string, bool) {
	i := pyIndex(off, len(sm.inputLines.data))
	if i < 0 {
		return "", false
	}
	return sm.inputLines.data[i], true
}

// nextLine advances n lines; ok is false at EOF (Python: EOFError).
func (sm *stateMachine) nextLine(n int) (string, bool) {
	sm.lineOffset += n
	sm.line, sm.hasLine = sm.lineAt(sm.lineOffset)
	sm.notifyObservers()
	return sm.line, sm.hasLine
}

func (sm *stateMachine) isNextLineBlank() bool {
	l, ok := sm.lineAt(sm.lineOffset + 1)
	if !ok {
		return true
	}
	return isBlank(l)
}

func (sm *stateMachine) atEOF() bool { return sm.lineOffset >= len(sm.inputLines.data)-1 }

func (sm *stateMachine) previousLine(n int) {
	sm.lineOffset -= n
	if sm.lineOffset < 0 {
		sm.line, sm.hasLine = "", false
	} else {
		sm.line, sm.hasLine = sm.lineAt(sm.lineOffset)
	}
	sm.notifyObservers()
}

func (sm *stateMachine) gotoLine(lineOffset int) bool {
	sm.lineOffset = lineOffset - sm.inputOffset
	sm.line, sm.hasLine = sm.lineAt(sm.lineOffset)
	sm.notifyObservers()
	return sm.hasLine
}

func (sm *stateMachine) absLineOffset() int { return sm.lineOffset + sm.inputOffset }
func (sm *stateMachine) absLineNumber() int { return sm.lineOffset + sm.inputOffset + 1 }

// getSourceAndLine mirrors StateMachine.get_source_and_line.
func (sm *stateMachine) getSourceAndLine(lineno int, hasLineno bool) (string, bool, int, bool) {
	offset := sm.lineOffset
	if hasLineno {
		offset = lineno - sm.inputOffset - 1
	}
	return sm.srcLineAtOffset(offset, 0)
}

func (sm *stateMachine) srcLineAtOffset(offset int, depth int) (string, bool, int, bool) {
	if sm.inputLines == nil || depth > 50 {
		return "", false, 0, false
	}
	info, hasOffset, ok := sm.inputLines.info(offset)
	if !ok {
		return "", false, 0, false
	}
	if !hasOffset {
		// Python: TypeError -> retry with lineno = offset + input_offset
		src, hasSrc, line, hasLine := sm.srcLineAtOffset(offset-1, depth+1)
		if !hasLine {
			// would raise TypeError in Python (None + 1); treat as unknown
			return src, hasSrc, 0, false
		}
		return src, hasSrc, line + 1, true
	}
	src, hasSrc := info.src()
	return src, hasSrc, info.offset + 1, true
}

func (sm *stateMachine) curSrcLine() (string, bool, int, bool) {
	return sm.getSourceAndLine(0, false)
}

func (sm *stateMachine) insertInput(lines []string, source string) {
	after, before := "internal padding after "+source, "internal padding before "+source
	sm.inputLines.insertLine(sm.lineOffset+1, "",
		lineInfo{&after, len(lines)})
	sm.inputLines.insertLine(sm.lineOffset+1, "",
		lineInfo{&before, -1})
	sm.inputLines.insertList(sm.lineOffset+2, newStringList(lines, source, true))
}

func (sm *stateMachine) getTextBlock(flushLeft bool) (*stringList, *unexpectedIndentation) {
	block, err := sm.inputLines.getTextBlock(sm.lineOffset, flushLeft)
	if err != nil {
		sm.nextLine(len(err.block.data) - 1)
		return err.block, err
	}
	sm.nextLine(len(block.data) - 1)
	return block, nil
}

// getTextLines is getTextBlock for callers that only read the lines: it
// returns them without copying.
func (sm *stateMachine) getTextLines(flushLeft bool) ([]string, *unexpectedIndentation) {
	sl := sm.inputLines
	end, indentAt := sl.textBlockEnd(sm.lineOffset, flushLeft)
	if indentAt >= 0 {
		info, _, _ := sl.info(indentAt)
		err := &unexpectedIndentation{sl.slice(sm.lineOffset, end), info, info.offset + 1}
		sm.nextLine(len(err.block.data) - 1)
		return err.block.data, err
	}
	lines := sl.data[sm.lineOffset:end:end]
	sm.nextLine(len(lines) - 1)
	return lines, nil
}

func (sm *stateMachine) getIndented(untilBlank, stripIndent bool) (*stringList, int, int, bool) {
	offset := sm.absLineOffset()
	indented, indent, blankFinish := sm.inputLines.getIndented(sm.lineOffset, untilBlank, stripIndent, -1, -1)
	if len(indented.data) > 0 {
		sm.nextLine(len(indented.data) - 1)
	}
	for len(indented.data) > 0 && isBlank(indented.data[0]) {
		indented.trimStart(1)
		offset++
	}
	return indented, indent, offset, blankFinish
}

func (sm *stateMachine) getKnownIndented(indent int, untilBlank, stripIndent bool) (*stringList, int, bool) {
	offset := sm.absLineOffset()
	indented, _, blankFinish := sm.inputLines.getIndented(sm.lineOffset, untilBlank, stripIndent, indent, -1)
	sm.nextLine(len(indented.data) - 1)
	for len(indented.data) > 0 && isBlank(indented.data[0]) {
		indented.trimStart(1)
		offset++
	}
	return indented, offset, blankFinish
}

func (sm *stateMachine) getFirstKnownIndented(indent int, untilBlank, stripIndent, stripTop bool) (*stringList, int, int, bool) {
	offset := sm.absLineOffset()
	indented, ind, blankFinish := sm.inputLines.getIndented(sm.lineOffset, untilBlank, stripIndent, -1, indent)
	sm.nextLine(len(indented.data) - 1)
	if stripTop {
		for len(indented.data) > 0 && isBlank(indented.data[0]) {
			indented.trimStart(1)
			offset++
		}
	}
	return indented, ind, offset, blankFinish
}

func (sm *stateMachine) checkLine(context []string, st *rstState, transitions []string) ([]string, string) {
	if transitions == nil {
		for _, t := range st.list {
			if m, ok := t.scan(sm.line); ok {
				return t.method(st, m, context, t.nextState)
			}
		}
		return context, ""
	}
	for _, name := range transitions {
		t := st.trans[name]
		if t == nil {
			continue
		}
		if m, ok := t.scan(sm.line); ok {
			return t.method(st, m, context, t.nextState)
		}
	}
	return context, ""
}

// run mirrors StateMachine.run for RST state machines.
func (sm *stateMachine) run(inputLines *stringList, inputOffset int) {
	for _, st := range sm.states {
		st.runtimeInit()
	}
	sm.inputLines = inputLines
	sm.inputOffset = inputOffset
	sm.lineOffset = -1
	sm.currentState = sm.initialState
	sm.observing = true
	var transitions []string
	st := sm.getState("")
	context := []string{}
	for {
		var exc any
		nextState := ""
		done := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					switch r.(type) {
					case smEOF, *transitionCorrection, *stateCorrection:
						exc = r
					default:
						panic(r)
					}
				}
			}()
			if _, ok := sm.nextLine(1); !ok {
				done = true
				st.eof(context)
				return
			}
			context, nextState = sm.checkLine(context, st, transitions)
		}()
		switch e := exc.(type) {
		case smEOF:
			// EOFError raised by a transition method
			var exc2 any
			func() {
				defer func() {
					if r := recover(); r != nil {
						switch r.(type) {
						case *transitionCorrection, *stateCorrection:
							exc2 = r
						default:
							panic(r)
						}
					}
				}()
				st.eof(context)
			}()
			if exc2 == nil {
				done = true
			} else {
				exc = exc2
				done = false
			}
		default:
			_ = e
		}
		if done && exc == nil {
			break
		}
		if _, isEOF := exc.(smEOF); isEOF && done {
			break
		}
		switch e := exc.(type) {
		case *transitionCorrection:
			sm.previousLine(1)
			transitions = []string{e.name}
			continue
		case *stateCorrection:
			sm.previousLine(1)
			nextState = e.state
			context = []string{}
			if e.transition == "" {
				transitions = nil
			} else {
				transitions = []string{e.transition}
			}
		default:
			transitions = nil
		}
		st = sm.getState(nextState)
	}
	sm.observing = false
}

// runTop mirrors RSTStateMachine.run.
func (sm *stateMachine) runTop(inputLines *stringList, doc *Document, m *memo) {
	sm.matchTitles = true
	sm.memo = m
	sm.document = doc
	sm.reporter = doc.reporter
	sm.node = doc.Root
	sm.run(inputLines, 0)
}

// runNested mirrors NestedStateMachine.run.
func (sm *stateMachine) runNested(inputLines *stringList, inputOffset int, m *memo, node *Node, matchTitles bool) {
	sm.matchTitles = matchTitles
	sm.memo = m
	sm.document = m.document
	sm.reporter = m.document.reporter
	sm.node = node
	sm.run(inputLines, inputOffset)
}

// context1 returns a one-line context.  The backing array belongs to the
// state machine; contexts are consumed before the next line is read.
func (sm *stateMachine) context1(line string) []string {
	sm.ctxBuf[0] = line
	return sm.ctxBuf[:1:1]
}
