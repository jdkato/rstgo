package rstgo

// Port of docutils.utils.math.math2html (derived from eLyXer): conversion
// of LaTeX math to HTML with CSS, the default math output of the html4css1
// writer.  Class, method and variable names follow the Python original; the
// classes are represented by one struct with a kind.

import (
	"strconv"
	"strings"
)

type m2hKind uint8

const (
	mkConst m2hKind = iota
	mkRawText
	mkSymbol
	mkNumber
	mkComment
	mkWhiteSpace
	mkBracket
	mkSquareBracket
	mkWhole
	mkCommand
	mkEmpty
	mkSpaced
	mkAlpha
	mkOneParam
	mkSymbolFunction
	mkText
	mkFont
	mkEquation
	mkCell
	mkRow
	mkArray
	mkMatrix
	mkCases
	mkEquationEnv
	mkBegin
	mkCombining
	mkOverset
	mkUnderset
	mkLimit
	mkBracketCommand
	mkHybrid
	mkTagged
)

var m2hClassNames = [...]string{
	mkConst: "FormulaConstant", mkRawText: "RawText", mkSymbol: "FormulaSymbol",
	mkNumber: "FormulaNumber", mkComment: "Comment", mkWhiteSpace: "WhiteSpace",
	mkBracket: "Bracket", mkSquareBracket: "SquareBracket", mkWhole: "WholeFormula",
	mkCommand: "FormulaCommand", mkEmpty: "EmptyCommand", mkSpaced: "SpacedCommand",
	mkAlpha: "AlphaCommand", mkOneParam: "OneParamFunction",
	mkSymbolFunction: "SymbolFunction", mkText: "TextFunction", mkFont: "FontFunction",
	mkEquation: "FormulaEquation", mkCell: "FormulaCell", mkRow: "FormulaRow",
	mkArray: "FormulaArray", mkMatrix: "FormulaMatrix", mkCases: "FormulaCases",
	mkEquationEnv: "EquationEnvironment", mkBegin: "BeginCommand",
	mkCombining: "CombiningFunction", mkOverset: "OversetFunction",
	mkUnderset: "UndersetFunction", mkLimit: "LimitCommand",
	mkBracketCommand: "BracketCommand", mkHybrid: "HybridFunction", mkTagged: "TaggedBit",
}

// m2hExtracted is ContainerConfig.extracttext['extracted'] (the classes
// looked into when the text of a container is extracted).
var m2hExtracted = [...]bool{
	mkAlpha: true, mkBracket: true, mkBracketCommand: true, mkCombining: true,
	mkEmpty: true, mkFont: true, mkNumber: true, mkSymbol: true, mkOneParam: true,
	mkOverset: true, mkRawText: true, mkSpaced: true, mkSymbolFunction: true,
	mkText: true, mkUnderset: true, mkTagged: false,
}

type m2hOutKind uint8

const (
	moContents m2hOutKind = iota
	moTagged
	moFixed
	moEmpty
	moString
)

type m2hOutput struct {
	kind       m2hOutKind
	tag        string
	breaklines bool
	empty      bool
}

func m2hTagged(tag string) m2hOutput { return m2hOutput{kind: moTagged, tag: tag} }

type m2hParam struct {
	name         string // with the leading '$'
	literal      bool
	optional     bool
	value        *m2hBit
	literalValue string
	hasLiteral   bool
}

type m2hBit struct {
	kind     m2hKind
	contents []*m2hBit
	original string
	out      m2hOutput
	html     []string
	str      string // Constant.string
	typ      string // "" is None
	size     int
	parent   *m2hBit

	command    string
	translated string
	literal    string
	hasLiteral bool
	alignments []string
	rows       []*m2hBit
	piece      string
	params     []*m2hParam
}

// m2hCrash is raised (via panic) where the Python original dies with an
// exception; the text names that exception.
type m2hCrash string

// m2hConv is the state of one conversion (Trace, DocumentParameters and
// FormulaFactory of the original).
type m2hConv struct {
	displaymode bool
	errors      []string // Trace.error messages, in order
}

func (c *m2hConv) traceError(message string) {
	c.errors = append(c.errors, "* "+message)
}

func m2hConstant(s string) *m2hBit {
	return &m2hBit{kind: mkConst, str: s, original: s, size: 1, out: m2hOutput{kind: moString}}
}

func m2hNew(kind m2hKind) *m2hBit { return &m2hBit{kind: kind, size: 1} }

func (b *m2hBit) isFormulaBit() bool { return b.kind != mkConst }

func (b *m2hBit) String() string {
	switch b.kind {
	case mkConst:
		return "Formula constant: " + b.str
	case mkWhiteSpace:
		return "Whitespace: *" + b.original + "*"
	}
	return m2hClassNames[b.kind] + " read in " + b.original
}

func (b *m2hBit) add(bit *m2hBit) {
	b.contents = append(b.contents, bit)
	b.original += bit.original
	bit.parent = b
}

func (b *m2hBit) skiporiginal(c *m2hConv, s string, pos *m2hPos) {
	b.original += s
	if !pos.checkskip(s) {
		c.traceError("String " + s + " not at " + pos.identifier())
	}
}

func (b *m2hBit) computesize() int {
	if b.kind == mkConst {
		return b.size
	}
	if len(b.contents) == 0 {
		return 1
	}
	b.size = b.contents[0].size
	for _, e := range b.contents[1:] {
		if e.size > b.size {
			b.size = e.size
		}
	}
	return b.size
}

// ---------------------------------------------------------------------
// Positions and endings

type m2hEnding struct {
	ending   string
	runes    []rune
	optional bool
}

func (e *m2hEnding) String() string {
	s := "Ending " + e.ending
	if e.optional {
		s += " (optional)"
	}
	return s
}

type m2hPos struct {
	c       *m2hConv
	text    []rune
	pos     int
	endings []*m2hEnding
}

func newM2hPos(c *m2hConv, text string) *m2hPos {
	p := &m2hPos{c: c, text: []rune(text)}
	// checkbytemark
	if !p.finished() && p.current() == 0xfeff {
		p.skipcurrent()
	}
	return p
}

func (p *m2hPos) isout() bool { return p.pos >= len(p.text) }

func (p *m2hPos) current() rune {
	if p.pos >= len(p.text) || p.pos < 0 {
		panic(m2hCrash("IndexError: string index out of range"))
	}
	return p.text[p.pos]
}

func (p *m2hPos) skipN(n int) { p.pos += n }

func (p *m2hPos) skipcurrent() string {
	cur := p.current()
	p.pos++
	return string(cur)
}

func (p *m2hPos) identifier() string {
	length := 30
	if p.pos+length > len(p.text) {
		length = len(p.text) - p.pos
	}
	if length < 0 || p.pos > len(p.text) {
		return "**"
	}
	return "*" + string(p.text[p.pos:p.pos+length]) + "*"
}

func (p *m2hPos) checkforRunes(rs []rune) bool {
	if p.pos+len(rs) > len(p.text) {
		return false
	}
	for i, r := range rs {
		if p.text[p.pos+i] != r {
			return false
		}
	}
	return true
}

func (p *m2hPos) checkfor(s string) bool { return p.checkforRunes([]rune(s)) }

func (p *m2hPos) checkskip(s string) bool {
	rs := []rune(s)
	if !p.checkforRunes(rs) {
		return false
	}
	p.pos += len(rs)
	return true
}

func (p *m2hPos) endingsString() string {
	parts := make([]string, len(p.endings))
	for i, e := range p.endings {
		parts[i] = e.String()
	}
	return "endings [" + strings.Join(parts, ",") + "]"
}

func (p *m2hPos) findending() *m2hEnding {
	for i := len(p.endings) - 1; i >= 0; i-- {
		e := p.endings[i]
		if p.checkforRunes(e.runes) {
			return e
		}
		if !e.optional {
			return nil
		}
	}
	return nil
}

func (p *m2hPos) finished() bool {
	if p.isout() {
		if len(p.endings) != 0 {
			p.c.traceError("Pending " + p.endingsString() + " left open")
		}
		return true
	}
	return p.findending() != nil
}

func (p *m2hPos) glob(check func(rune) bool) string {
	start := p.pos
	for !p.finished() && check(p.current()) {
		p.pos++
	}
	return string(p.text[start:p.pos])
}

func (p *m2hPos) globalpha() string { return p.glob(isPyAlpha) }

func (p *m2hPos) isvalue() bool {
	cur := p.current()
	if isPySpace(cur) {
		return false
	}
	switch cur {
	case '{', '}', '(', ')':
		return false
	}
	return true
}

func (p *m2hPos) globvalue() string {
	start := p.pos
	for !p.finished() && p.isvalue() {
		p.pos++
	}
	return string(p.text[start:p.pos])
}

func (p *m2hPos) skipspace() string { return p.glob(isPySpace) }

func (p *m2hPos) globincluding(magic rune) string {
	g := p.glob(func(r rune) bool { return r != magic }) + string(magic)
	p.pos++
	return g
}

func (p *m2hPos) pushending(ending string, optional bool) {
	p.endings = append(p.endings, &m2hEnding{ending: ending, runes: []rune(ending), optional: optional})
}

func (p *m2hPos) popEndingList() string {
	if p.isout() {
		p.c.traceError("No ending out of bounds")
		return ""
	}
	ending := p.findending()
	if ending == nil {
		p.c.traceError("No ending at " + string(p.current()))
		return ""
	}
	for len(p.endings) > 0 {
		each := p.endings[len(p.endings)-1]
		p.endings = p.endings[:len(p.endings)-1]
		if each == ending {
			return each.ending
		} else if !each.optional {
			panic(m2hCrash(`TypeError: can only concatenate str (not "PositionEnding") to str`))
		}
	}
	p.c.traceError("No endings left")
	return ""
}

// popending is Globable.popending; expected may be "" (None).
func (p *m2hPos) popending(expected string) string {
	ending := p.popEndingList()
	if expected != "" && expected != ending {
		p.c.traceError("Expected ending " + expected + ", got " + ending)
	}
	p.pos += len([]rune(ending))
	return ending
}

func (p *m2hPos) nextending() string {
	e := p.findending()
	if e == nil {
		return ""
	}
	return e.ending
}

func isPyAlpha(r rune) bool {
	if r < 0x80 {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}
	return inRuneRanges(pyAlphaRanges, r)
}

func isPyStrDigit(r rune) bool {
	if r < 0x80 {
		return r >= '0' && r <= '9'
	}
	return inRuneRanges(pyStrDigitRanges, r)
}

func inRuneRanges(ranges [][2]rune, r rune) bool {
	lo, hi := 0, len(ranges)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case r < ranges[mid][0]:
			hi = mid
		case r > ranges[mid][1]:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// FormulaFactory

func (c *m2hConv) detectSymbol(pos *m2hPos) bool {
	cur := string(pos.current())
	if m2hUnmodified[cur] {
		return true
	}
	_, ok := m2hModified[cur]
	return ok
}

func (c *m2hConv) detectSymbolFunction(pos *m2hPos) bool {
	_, ok := m2hSymbolfunctions[string(pos.current())]
	return ok
}

// parseany is FormulaFactory.parseany: try the bit types in order.
func (c *m2hConv) parseany(pos *m2hPos) *m2hBit {
	// types: FormulaSymbol, RawText, FormulaNumber, Bracket, Comment,
	// WhiteSpace, FormulaCommand, SymbolFunction; then the skipped types
	// Comment and WhiteSpace again.  Every detection first asks whether
	// the position is finished.
	if !pos.finished() && c.detectSymbol(pos) {
		return c.parseSymbol(pos)
	}
	if !pos.finished() && isPyAlpha(pos.current()) {
		b := m2hNew(mkRawText)
		b.add(m2hConstant(pos.globalpha()))
		b.typ = "alpha"
		return b
	}
	if !pos.finished() && isPyStrDigit(pos.current()) {
		b := m2hNew(mkNumber)
		b.add(m2hConstant(pos.glob(isPyStrDigit)))
		b.typ = "number"
		return b
	}
	if !pos.finished() && pos.checkfor("{") {
		b := m2hNew(mkBracket)
		c.bracketParsecomplete(b, pos, c.innerformula)
		return b
	}
	if !pos.finished() && pos.current() == '%' {
		return c.parseComment(pos)
	}
	if !pos.finished() && isPySpace(pos.current()) {
		return c.parseWhiteSpace(pos)
	}
	if !pos.finished() && pos.checkfor(`\`) {
		return c.parseCommand(pos)
	}
	if !pos.finished() && c.detectSymbolFunction(pos) {
		b := m2hNew(mkSymbolFunction)
		c.setcommand(b, string(pos.current()), m2hSymbolfunctions)
		pos.skipN(1)
		b.out = m2hTagged(b.translated)
		c.parseparameter(b, pos)
		return b
	}
	if !pos.finished() && pos.current() == '%' {
		return c.parseComment(pos)
	}
	if !pos.finished() && isPySpace(pos.current()) {
		return c.parseWhiteSpace(pos)
	}
	c.traceError("Unrecognized formula at " + pos.identifier())
	return m2hConstant(pos.skipcurrent())
}

func (c *m2hConv) parseSymbol(pos *m2hPos) *m2hBit {
	b := m2hNew(mkSymbol)
	cur := string(pos.current())
	symbol := cur
	if !m2hUnmodified[cur] {
		symbol = m2hModified[cur]
	}
	b.skiporiginal(c, cur, pos)
	b.contents = append(b.contents, m2hConstant(symbol))
	return b
}

func (c *m2hConv) parseComment(pos *m2hPos) *m2hBit {
	b := m2hNew(mkComment)
	b.original += pos.globincluding('\n')
	return b
}

func (c *m2hConv) parseWhiteSpace(pos *m2hPos) *m2hBit {
	b := m2hNew(mkWhiteSpace)
	b.original += pos.skipspace()
	return b
}

// clearskipped skips comments and white space.
func (c *m2hConv) clearskipped(pos *m2hPos) {
	for !pos.finished() {
		switch {
		case pos.current() == '%':
			c.parseComment(pos)
		case isPySpace(pos.current()):
			c.parseWhiteSpace(pos)
		default:
			return
		}
	}
}

func (c *m2hConv) parseWhole(pos *m2hPos) *m2hBit {
	whole := m2hNew(mkWhole)
	for !pos.finished() {
		whole.add(c.parseany(pos))
	}
	return whole
}

// parseformula is FormulaFactory.parseformula.
func (c *m2hConv) parseformula(formula string) *m2hBit {
	pos := newM2hPos(c, formula)
	if !pos.finished() {
		return c.parseWhole(pos)
	}
	whole := m2hNew(mkWhole)
	if !pos.finished() {
		c.traceError("Unknown formula at: " + pos.identifier())
		t := m2hNew(mkTagged)
		t.out = m2hTagged(`span class="unknown"`)
		t.add(m2hConstant(formula))
		whole.add(t)
	}
	return whole
}

// ---------------------------------------------------------------------
// Brackets

func (c *m2hConv) bracketEnds(b *m2hBit) (string, string) {
	if b.kind == mkSquareBracket {
		return "[", "]"
	}
	return "{", "}"
}

func (c *m2hConv) bracketParsecomplete(b *m2hBit, pos *m2hPos, inner func(*m2hBit, *m2hPos)) {
	start, ending := c.bracketEnds(b)
	if !pos.checkfor(start) {
		c.traceError("Bracket should start with " + start + " at " + pos.identifier())
		return
	}
	b.skiporiginal(c, start, pos)
	pos.pushending(ending, false)
	inner(b, pos)
	b.original += pos.popending(ending)
	b.computesize()
}

func (c *m2hConv) innerformula(b *m2hBit, pos *m2hPos) {
	for !pos.finished() {
		b.add(c.parseany(pos))
	}
}

func (c *m2hConv) innertext(b *m2hBit, pos *m2hPos) {
	for !pos.finished() {
		switch pos.current() {
		case '^', '_', '\\', '{', '%':
			b.add(c.parseany(pos))
			if pos.checkskip(" ") {
				b.original += " "
			}
		default:
			b.add(m2hConstant(pos.skipcurrent()))
		}
	}
}

func (c *m2hConv) innerliteral(b *m2hBit, pos *m2hPos) {
	start, ending := c.bracketEnds(b)
	b.literal, b.hasLiteral = "", true
	for !pos.finished() && string(pos.current()) != ending {
		if string(pos.current()) == start {
			c.bracketParsecomplete(b, pos, c.innerliteral)
		} else {
			b.literal += pos.skipcurrent()
		}
	}
	b.original += b.literal
}

// ---------------------------------------------------------------------
// Commands

func (c *m2hConv) setcommand(b *m2hBit, command string, commandmap map[string]string) {
	b.command = command
	b.original += command
	b.translated = commandmap[command]
}

func (c *m2hConv) extractcommand(pos *m2hPos) string {
	if !pos.checkskip(`\`) {
		c.traceError(`Missing command start \: ` + pos.identifier())
		panic(m2hCrash("AttributeError: 'NoneType' object has no attribute 'startswith'"))
	}
	if pos.finished() {
		// emptycommand: a command disguised as ending
		command := ""
		if !pos.isout() {
			ending := pos.nextending()
			if ending != "" && pos.checkskip(ending) {
				command = ending
			}
		}
		return `\` + command
	}
	if isPyAlpha(pos.current()) {
		command := `\` + pos.globalpha()
		pos.checkskip("*")
		return command
	}
	return `\` + pos.skipcurrent()
}

func (c *m2hConv) parseCommand(pos *m2hPos) *m2hBit {
	command := c.extractcommand(pos)
	if bit := c.parsewithcommand(command, pos); bit != nil {
		return bit
	}
	if strings.HasPrefix(command, `\up`) || strings.HasPrefix(command, `\Up`) {
		if upgreek := c.parseupgreek(command, pos); upgreek != nil {
			return upgreek
		}
	}
	c.traceError("Unknown command " + command)
	b := m2hNew(mkCommand)
	b.out = m2hTagged(`span class="unknown"`)
	b.add(m2hConstant(command))
	return b
}

func (c *m2hConv) parseupgreek(command string, pos *m2hPos) *m2hBit {
	rs := []rune(command)
	if len(rs) < 4 {
		return nil
	}
	var upcommand string
	if strings.HasPrefix(command, `\up`) {
		upcommand = `\` + string(rs[3:])
	} else if pos.checkskip(`\Up`) {
		upcommand = `\` + pyUpper(string(rs[3:4])) + string(rs[4:])
	} else {
		c.traceError("Impossible upgreek command: " + command)
		return nil
	}
	upgreek := c.parsewithcommand(upcommand, pos)
	if upgreek != nil {
		upgreek.typ = "font"
	}
	return upgreek
}

var m2hGreekCapitals = map[string]bool{
	`\Xi`: true, `\Theta`: true, `\Pi`: true, `\Sigma`: true, `\Gamma`: true,
	`\Lambda`: true, `\Phi`: true, `\Psi`: true, `\Delta`: true,
	`\Upsilon`: true, `\Omega`: true,
}

// parsewithcommand tries the command classes in the order of
// FormulaCommand.types.
func (c *m2hConv) parsewithcommand(command string, pos *m2hPos) *m2hBit {
	if _, ok := m2hAlphacommands[command]; ok {
		b := m2hNew(mkAlpha)
		c.setcommand(b, command, m2hAlphacommands)
		b.contents = []*m2hBit{m2hConstant(b.translated)}
		if !m2hGreekCapitals[command] {
			b.typ = "alpha"
		}
		return b
	}
	if _, ok := m2hCommands[command]; ok {
		b := m2hNew(mkEmpty)
		c.setcommand(b, command, m2hCommands)
		b.contents = []*m2hBit{m2hConstant(b.translated)}
		return b
	}
	if _, ok := m2hOnefunctions[command]; ok {
		b := m2hNew(mkOneParam)
		c.setcommand(b, command, m2hOnefunctions)
		c.oneParamParsebit(b, pos)
		return b
	}
	if _, ok := m2hFontfunctions[command]; ok {
		b := m2hNew(mkFont)
		c.setcommand(b, command, m2hFontfunctions)
		c.oneParamParsebit(b, pos)
		return b
	}
	if _, ok := m2hTextfunctions[command]; ok {
		b := m2hNew(mkText)
		c.setcommand(b, command, m2hTextfunctions)
		b.out = m2hTagged(b.translated)
		c.parsetext(b, pos)
		return b
	}
	if _, ok := m2hSpacedcommands[command]; ok {
		b := m2hNew(mkSpaced)
		c.setcommand(b, command, m2hSpacedcommands)
		b.contents = []*m2hBit{m2hConstant(" " + b.translated + " ")}
		return b
	}
	if command == `\begin` {
		b := m2hNew(mkBegin)
		b.command = command
		b.original += command
		c.beginParsebit(b, pos)
		return b
	}
	if _, ok := m2hOversetfunctions[command]; ok {
		b := m2hNew(mkOverset)
		c.setcommand(b, command, m2hOversetfunctions)
		c.embellishParsebit(b, pos, "sup")
		return b
	}
	if _, ok := m2hUndersetfunctions[command]; ok {
		b := m2hNew(mkUnderset)
		c.setcommand(b, command, m2hUndersetfunctions)
		c.embellishParsebit(b, pos, "sub")
		return b
	}
	if _, ok := m2hCombiningfunctions[command]; ok {
		b := m2hNew(mkCombining)
		c.setcommand(b, command, m2hCombiningfunctions)
		c.combiningParsebit(b, pos)
		return b
	}
	if _, ok := m2hLimitcommands[command]; ok {
		b := m2hNew(mkLimit)
		c.setcommand(b, command, m2hLimitcommands)
		b.out = m2hTagged(`span class="limits"`)
		b.contents = append(b.contents, c.taggedConstant(b.translated, `span class="limit"`))
		return b
	}
	if _, ok := m2hBracketcommands[command]; ok {
		b := m2hNew(mkBracketCommand)
		c.setcommand(b, command, m2hBracketcommands)
		c.oneParamParsebit(b, pos)
		return b
	}
	if _, ok := m2hHybridfunctions[command]; ok {
		b := m2hNew(mkHybrid)
		b.command = command
		b.original += command
		c.hybridParsebit(b, pos)
		return b
	}
	return nil
}

func (c *m2hConv) taggedConstant(constant, tag string) *m2hBit {
	t := m2hNew(mkTagged)
	t.out = m2hTagged(tag)
	t.add(m2hConstant(constant))
	return t
}

func (c *m2hConv) taggedComplete(contents []*m2hBit, tag string, breaklines bool) *m2hBit {
	t := m2hNew(mkTagged)
	t.contents = contents
	t.out = m2hOutput{kind: moTagged, tag: tag, breaklines: breaklines}
	return t
}

func (c *m2hConv) parseparameter(b *m2hBit, pos *m2hPos) *m2hBit {
	c.clearskipped(pos)
	if pos.finished() {
		return nil
	}
	parameter := c.parseany(pos)
	b.add(parameter)
	return parameter
}

func (c *m2hConv) parsesquare(b *m2hBit, pos *m2hPos) *m2hBit {
	c.clearskipped(pos)
	if pos.finished() || !pos.checkfor("[") {
		return nil
	}
	bracket := m2hNew(mkSquareBracket)
	c.bracketParsecomplete(bracket, pos, c.innerformula)
	b.add(bracket)
	return bracket
}

// parseliteral returns the literal and whether there is one (None
// otherwise).
func (c *m2hConv) parseliteral(b *m2hBit, pos *m2hPos) (string, bool) {
	c.clearskipped(pos)
	if pos.finished() || !pos.checkfor("{") {
		if !pos.isvalue() {
			c.traceError("No literal parameter found at: " + pos.identifier())
			return "", false
		}
		return pos.globvalue(), true
	}
	bracket := m2hNew(mkBracket)
	c.bracketParsecomplete(bracket, pos, c.innerliteral)
	b.add(bracket)
	return bracket.literal, true
}

func (c *m2hConv) parsesquareliteral(b *m2hBit, pos *m2hPos) (string, bool) {
	c.clearskipped(pos)
	if pos.finished() || !pos.checkfor("[") {
		return "", false
	}
	bracket := m2hNew(mkSquareBracket)
	c.bracketParsecomplete(bracket, pos, c.innerliteral)
	b.add(bracket)
	return bracket.literal, true
}

func (c *m2hConv) parsetext(b *m2hBit, pos *m2hPos) *m2hBit {
	c.clearskipped(pos)
	if pos.finished() || !pos.checkfor("{") {
		c.traceError("No text parameter for " + b.command)
		return nil
	}
	bracket := m2hNew(mkBracket)
	c.bracketParsecomplete(bracket, pos, c.innertext)
	b.add(bracket)
	return bracket
}

func (c *m2hConv) commandmapOf(b *m2hBit) map[string]string {
	switch b.kind {
	case mkOneParam:
		return m2hOnefunctions
	case mkFont:
		return m2hFontfunctions
	case mkBracketCommand:
		return m2hBracketcommands
	case mkOverset:
		return m2hOversetfunctions
	case mkUnderset:
		return m2hUndersetfunctions
	}
	return nil
}

func (c *m2hConv) simplifyifpossible(b *m2hBit) {
	if v, ok := c.commandmapOf(b)[b.original]; ok {
		b.out = m2hOutput{kind: moFixed}
		b.html = []string{v}
	}
}

func (c *m2hConv) oneParamParsebit(b *m2hBit, pos *m2hPos) {
	b.out = m2hTagged(b.translated)
	c.parseparameter(b, pos)
	c.simplifyifpossible(b)
}

func (c *m2hConv) embellishParsebit(b *m2hBit, pos *m2hPos, tag string) {
	symbol := c.taggedConstant(b.translated, tag)
	parameter := c.parseparameter(b, pos)
	b.out = m2hTagged(`span class="embellished"`)
	b.contents = append([]*m2hBit{symbol}, b.contents...)
	if parameter == nil {
		panic(m2hCrash("AttributeError: 'NoneType' object has no attribute 'output' and no __dict__ for setting new attributes"))
	}
	parameter.out = m2hTagged(`span class="base"`)
	c.simplifyifpossible(b)
}

func (c *m2hConv) combiningParsebit(b *m2hBit, pos *m2hPos) {
	combining := b.translated
	c.clearskipped(pos)
	var parameter *m2hBit
	if !pos.finished() {
		parameter = c.parseparameter(b, pos)
	}
	if parameter == nil {
		c.traceError("Missing parameter for combining function " + b.command)
		return
	}
	if parameter.kind != mkConst {
		parameter = m2hFirstConstant(parameter)
		if parameter == nil {
			c.traceError(`No base character found for "` + b.command + `".`)
			return
		}
	}
	rs := []rune(parameter.str)
	i := 1
	if strings.HasPrefix(parameter.str, " ") {
		i = 2
	}
	if i > len(rs) {
		i = len(rs)
	}
	parameter.str = nfcNormalize(string(rs[:i]) + combining + string(rs[i:]))
}

// m2hExtract is ContainerExtractor.extract with the "extracttext"
// configuration: the FormulaConstants found by looking into the
// "extracted" containers.
func m2hExtract(b *m2hBit, lst []*m2hBit) []*m2hBit {
	for _, e := range b.contents {
		if m2hExtracted[e.kind] {
			lst = m2hExtract(e, lst)
		}
		if e.kind == mkConst {
			lst = append(lst, e)
		}
	}
	return lst
}

func m2hFirstConstant(b *m2hBit) *m2hBit {
	lst := m2hExtract(b, nil)
	if len(lst) == 0 {
		return nil
	}
	return lst[0]
}

func (b *m2hBit) extracttext() string {
	if b.kind == mkConst {
		return b.str
	}
	var sb strings.Builder
	for _, k := range m2hExtract(b, nil) {
		sb.WriteString(k.str)
	}
	return sb.String()
}

// ---------------------------------------------------------------------
// Environments

func (c *m2hConv) beginParsebit(b *m2hBit, pos *m2hPos) {
	command, ok := c.parseliteral(b, pos)
	if !ok {
		panic(m2hCrash("AttributeError: 'NoneType' object has no attribute 'replace'"))
	}
	var bit *m2hBit
	switch strings.ReplaceAll(command, "*", "") {
	case "equation":
		bit = m2hNew(mkEquation)
	case "array":
		bit = m2hNew(mkArray)
	case "cases":
		bit = m2hNew(mkCases)
	case "matrix":
		bit = m2hNew(mkMatrix)
	default:
		bit = m2hNew(mkEquationEnv)
		bit.piece = command
	}
	ending := `\end{` + command + "}"
	pos.pushending(ending, false)
	switch bit.kind {
	case mkEquation:
		bit.add(c.parseWhole(pos))
	case mkArray:
		bit.out = m2hTagged(`span class="array"`)
		c.parsesquareliteral(bit, pos)
		literal, ok := c.parseliteral(bit, pos)
		if !ok {
			panic(m2hCrash("TypeError: 'NoneType' object is not iterable"))
		}
		bit.alignments = []string{}
		for _, r := range literal {
			bit.alignments = append(bit.alignments, string(r))
		}
		c.parserows(bit, pos)
	case mkMatrix:
		bit.out = m2hTagged(`span class="array"`)
		bit.alignments = []string{"c"}
		c.parserows(bit, pos)
	case mkCases:
		bit.alignments = []string{"l", "l"}
		c.parserows(bit, pos)
		for _, row := range bit.contents {
			for _, cell := range row.contents {
				cell.out.kind, cell.out.tag, cell.out.breaklines = moTagged, `span class="case align-l"`, true
				cell.contents = append(cell.contents, m2hConstant("\u2003"))
			}
		}
		array := c.taggedComplete(bit.contents, `span class="bracketcases"`, true)
		bit.contents = append(c.bigBracketContents(len(bit.contents), "{", "l"), array)
	case mkEquationEnv:
		environment := strings.ReplaceAll(bit.piece, "*", "")
		bit.out = m2hTagged(`span class="environment ` + environment + `"`)
		if al, ok := m2hEnvironments[environment]; ok {
			bit.alignments = al
		} else {
			c.traceError("Unknown equation environment " + bit.piece)
			bit.out = m2hTagged(`span class="unknown"`)
			bit.add(m2hConstant(`\begin{` + environment + "} "))
			bit.alignments = []string{"l"}
		}
		c.parserows(bit, pos)
	}
	b.add(bit)
	b.original += pos.popending(ending)
	b.size = bit.size
}

func (c *m2hConv) newRow(alignments []string) *m2hBit {
	row := m2hNew(mkRow)
	row.alignments = alignments
	row.out = m2hOutput{kind: moTagged, tag: `span class="arrayrow"`, breaklines: true}
	return row
}

func (c *m2hConv) createcell(row *m2hBit, index int) *m2hBit {
	if len(row.alignments) == 0 {
		panic(m2hCrash("ZeroDivisionError: integer modulo by zero"))
	}
	alignment := row.alignments[index%len(row.alignments)]
	cell := m2hNew(mkCell)
	cell.out = m2hOutput{kind: moTagged, tag: `span class="arraycell align-` + alignment + `"`, breaklines: true}
	return cell
}

func (c *m2hConv) rowParsebit(row *m2hBit, pos *m2hPos) {
	index := 0
	pos.pushending("&", true)
	for !pos.finished() {
		cell := c.createcell(row, index)
		// FormulaCell.parsebit
		c.clearskipped(pos)
		if !pos.finished() {
			cell.add(c.parseWhole(pos))
		}
		row.add(cell)
		index++
		pos.checkskip("&")
	}
	if len(row.contents) == 0 {
		row.out = m2hOutput{kind: moEmpty}
	}
}

func (c *m2hConv) parserows(b *m2hBit, pos *m2hPos) {
	const rowseparator = `\\`
	b.rows = nil
	first := true
	for {
		pos.pushending(rowseparator, true)
		row := c.newRow(b.alignments)
		if first {
			first = false
		} else {
			// addempty
			empty := c.newRow(b.alignments)
			for index := range b.rows[len(b.rows)-1].contents {
				cell := c.createcell(empty, index)
				cell.add(m2hConstant("\u2005"))
				empty.add(cell)
			}
			b.rows = append(b.rows, empty)
			b.add(empty)
		}
		c.rowParsebit(row, pos)
		b.rows = append(b.rows, row)
		b.add(row)
		if pos.checkfor(rowseparator) {
			b.original += pos.popending(rowseparator)
		} else {
			break
		}
	}
	b.size = len(b.rows)
}

// bigBracketContents is BigBracket(size, bracket, alignment).getcontents().
func (c *m2hConv) bigBracketContents(size int, bracket, alignment string) []*m2hBit {
	pieces := m2hBigbrackets[bracket]
	if size == 1 || len(pieces) == 0 {
		if bracket == "." {
			return []*m2hBit{c.taggedConstant("", `span class="emptydot"`)}
		}
		return []*m2hBit{c.taggedConstant(bracket, `span class="stretchy"`)}
	}
	var rows []*m2hBit
	for index := 0; index < size; index++ {
		var piece string
		switch len(pieces) {
		case 1:
			piece = pieces[0]
		case 3:
			switch {
			case index == 0:
				piece = pieces[0]
			case index == size-1:
				piece = pieces[2]
			default:
				piece = pieces[1]
			}
		default:
			switch {
			case index == 0:
				piece = pieces[0]
			case index == size-1:
				piece = pieces[3]
			case (size-1)%2 == 0 && index == (size-1)/2:
				piece = pieces[2]
			default:
				piece = pieces[1]
			}
		}
		cell := c.taggedConstant(piece, `span class="bracket align-`+alignment+`"`)
		rows = append(rows, c.taggedComplete([]*m2hBit{cell}, `span class="arrayrow"`, false))
	}
	return []*m2hBit{c.taggedComplete(rows, `span class="array"`, false)}
}

// ---------------------------------------------------------------------
// Hybrid functions

func (c *m2hConv) hybridParsebit(b *m2hBit, pos *m2hPos) {
	translated := m2hHybridfunctions[b.command]
	// readparams
	tp := newM2hPos(c, translated[0])
	for !tp.finished() {
		def := c.parseParamDef(tp)
		if def == nil {
			continue
		}
		// ParameterDefinition.read
		switch {
		case def.literal:
			if def.optional {
				def.literalValue, def.hasLiteral = c.parsesquareliteral(b, pos)
			} else {
				def.literalValue, def.hasLiteral = c.parseliteral(b, pos)
			}
			if def.literalValue != "" {
				def.value = m2hConstant(def.literalValue)
			}
		case def.optional:
			def.value = c.parsesquare(b, pos)
		default:
			def.value = c.parseparameter(b, pos)
		}
		replaced := false
		for i, p := range b.params {
			if p.name == def.name {
				b.params[i], replaced = def, true
			}
		}
		if !replaced {
			b.params = append(b.params, def)
		}
	}
	b.contents = c.writepos(b, translated, newM2hPos(c, translated[1]))
	// computehybridsize
	sizestring, ok := m2hHybridsizes[b.command]
	if !ok {
		b.computesize()
		return
	}
	for _, p := range b.params {
		if strings.Contains(sizestring, p.name) {
			if p.value == nil {
				panic(m2hCrash("AttributeError: 'NoneType' object has no attribute 'computesize'"))
			}
			sizestring = strings.ReplaceAll(sizestring, p.name, strconv.Itoa(p.value.computesize()))
		}
	}
	size := 0
	for _, term := range strings.Split(sizestring, "+") {
		n, _ := strconv.Atoi(term)
		size += n
	}
	b.size = size
	for _, e := range b.contents {
		e.size = size
	}
}

func (c *m2hConv) parseParamDef(pos *m2hPos) *m2hParam {
	def := &m2hParam{}
	for _, br := range [2][2]string{{"[", "]"}, {"{", "}"}} {
		if pos.checkskip(br[0]) {
			if br[0] == "[" {
				def.optional = true
			}
			if !pos.checkskip("$") {
				c.traceError("Wrong parameter name, did you mean $" + string(pos.current()) + "?")
				return nil
			}
			def.name = "$" + pos.skipcurrent()
			if pos.checkskip("!") {
				def.literal = true
			}
			if !pos.checkskip(br[1]) {
				c.traceError("Wrong parameter closing " + pos.skipcurrent())
				return nil
			}
			return def
		}
	}
	c.traceError("Wrong character in parameter template: " + pos.skipcurrent())
	return nil
}

func (b *m2hBit) param(name string) *m2hParam {
	for _, p := range b.params {
		if p.name == name {
			return p
		}
	}
	return nil
}

func (c *m2hConv) writepos(b *m2hBit, translated []string, pos *m2hPos) []*m2hBit {
	result := []*m2hBit{}
	for !pos.finished() {
		switch {
		case pos.checkskip("$"):
			name := "$" + pos.skipcurrent()
			p := b.param(name)
			if p == nil {
				c.traceError("Unknown parameter " + name)
				continue
			}
			if pos.checkskip(".") {
				typ := pos.globalpha()
				if p.value == nil {
					panic(m2hCrash("AttributeError: 'NoneType' object has no attribute 'type' and no __dict__ for setting new attributes"))
				}
				p.value.typ = typ
			}
			if p.value != nil {
				result = append(result, p.value)
			}
		case pos.checkskip("f"):
			if function := c.writefunction(b, translated, pos); function != nil {
				function.typ = ""
				result = append(result, function)
			}
		case pos.checkskip("("):
			result = append(result, c.writebracket("left", "("))
		case pos.checkskip(")"):
			result = append(result, c.writebracket("right", ")"))
		default:
			result = append(result, m2hConstant(pos.skipcurrent()))
		}
	}
	return result
}

func (c *m2hConv) writebracket(direction, character string) *m2hBit {
	b := m2hNew(mkBracketCommand)
	b.original = character
	b.command = `\` + direction
	b.contents = []*m2hBit{m2hConstant(character)}
	return b
}

func (c *m2hConv) writefunction(b *m2hBit, translated []string, pos *m2hPos) *m2hBit {
	tag := c.readtag(b, translated, pos)
	if tag == "" {
		return nil
	}
	if pos.checkskip("/") {
		t := m2hNew(mkTagged)
		t.out = m2hOutput{kind: moTagged, tag: tag, empty: true}
		return t
	}
	if !pos.checkskip("{") {
		c.traceError("Function should be defined in {}")
		return nil
	}
	pos.pushending("}", false)
	contents := c.writepos(b, translated, pos)
	pos.popending("")
	if len(contents) == 0 {
		return nil
	}
	return c.taggedComplete(contents, tag, false)
}

func (c *m2hConv) readtag(b *m2hBit, translated []string, pos *m2hPos) string {
	if !isPyStrDigit(pos.current()) {
		c.traceError("Function should be f0,...,f9: f" + string(pos.current()))
		return ""
	}
	index := int(pos.current() - '0')
	pos.skipN(1)
	if 2+index > len(translated) {
		c.traceError("Function f" + strconv.Itoa(index) + " is not defined")
		return ""
	}
	if 2+index == len(translated) {
		panic(m2hCrash("IndexError: list index out of range"))
	}
	tag := translated[2+index]
	if !strings.Contains(tag, "$") {
		return tag
	}
	for _, p := range b.params {
		if strings.Contains(tag, p.name) {
			if !p.literal {
				c.traceError("Parameters in tag " + tag + " should be literal: {" + p.name + "!}")
				continue
			}
			tag = strings.ReplaceAll(tag, p.name, p.literalValue)
		}
	}
	return tag
}

// ---------------------------------------------------------------------
// FormulaProcessor

func (c *m2hConv) process(whole *m2hBit) {
	c.processcontents(whole)
	c.processinsides(whole)
	c.traversewhole(whole)
}

func (c *m2hConv) processcontents(bit *m2hBit) {
	if !bit.isFormulaBit() {
		return
	}
	switch bit.kind {
	case mkText:
		bit.typ = "font"
	case mkFont:
		bit.typ = "font"
		c.simplifyifpossible(bit)
	}
	for _, e := range bit.contents {
		c.processcontents(e)
	}
}

func (c *m2hConv) processinsides(bit *m2hBit) {
	if !bit.isFormulaBit() {
		return
	}
	for index := 0; index < len(bit.contents); index++ {
		element := bit.contents[index]
		c.processLimits(bit, index)
		c.processBrackets(bit, index)
		c.processinsides(element)
	}
}

func m2hIsKind(contents []*m2hBit, index int, kind m2hKind) bool {
	return index < len(contents) && contents[index].kind == kind
}

// getscript removes the script at index from the contents of bit.
func (c *m2hConv) getscript(bit *m2hBit, index int) *m2hBit {
	s := bit.contents[index]
	s.out.tag += ` class="script"`
	bit.contents = append(bit.contents[:index:index], bit.contents[index+1:]...)
	return s
}

func (c *m2hConv) getlimit(bit *m2hBit, index int) *m2hBit {
	limit := c.getscript(bit, index)
	limit.out.tag = strings.ReplaceAll(limit.out.tag, "script", "limit")
	return limit
}

// processLimits is LimitsProcessor.process.
func (c *m2hConv) processLimits(bit *m2hBit, index int) {
	if c.displaymode && m2hIsKind(bit.contents, index, mkLimit) && m2hIsKind(bit.contents, index+1, mkSymbolFunction) {
		// modifylimits
		limited := bit.contents[index]
		subscript := c.getlimit(bit, index+1)
		var superscript *m2hBit
		if m2hIsKind(bit.contents, index+1, mkSymbolFunction) {
			superscript = c.getlimit(bit, index+1)
		} else {
			superscript = c.taggedConstant(" ", `sup class="limit"`)
		}
		if subscript.command == "^" {
			superscript, subscript = subscript, superscript
		}
		limited.contents = append(limited.contents, subscript)
		limited.contents = append([]*m2hBit{superscript}, limited.contents...)
	}
	if m2hIsKind(bit.contents, index, mkSymbolFunction) && m2hIsKind(bit.contents, index+1, mkSymbolFunction) {
		// modifyscripts
		subscript := c.getscript(bit, index)
		superscript := c.getscript(bit, index)
		if subscript.command == "^" {
			superscript, subscript = subscript, superscript
		}
		scripts := c.taggedComplete([]*m2hBit{superscript, subscript}, `span class="scripts"`, false)
		rest := append([]*m2hBit{scripts}, bit.contents[index:]...)
		bit.contents = append(bit.contents[:index:index], rest...)
	}
}

func m2hIsDirection(b *m2hBit, command string) bool {
	return b.kind == mkBracketCommand && b.command == command
}

// processBrackets is BracketProcessor.process.
func (c *m2hConv) processBrackets(bit *m2hBit, index int) {
	contents := bit.contents
	if index >= len(contents) {
		panic(m2hCrash("IndexError: list index out of range"))
	}
	if !m2hIsDirection(contents[index], `\left`) {
		return
	}
	// findright
	rightindex := -1
	depth := 1
	for i := index + 1; i < len(contents); i++ {
		if m2hIsDirection(contents[i], `\left`) {
			depth++
		}
		if m2hIsDirection(contents[i], `\right`) {
			depth--
		}
		if depth == 0 {
			rightindex = i
			break
		}
	}
	if rightindex <= 0 {
		return
	}
	size := contents[index].size
	for _, e := range contents[index+1 : rightindex] {
		if e.size > size {
			size = e.size
		}
	}
	c.resize(contents[index], size)
	c.resize(contents[rightindex], size)
}

func (c *m2hConv) resize(command *m2hBit, size int) {
	character := command.extracttext()
	alignment := strings.ReplaceAll(command.command, `\`, "")
	command.out = m2hOutput{kind: moContents}
	command.contents = c.bigBracketContents(size, character, alignment)
}

func (c *m2hConv) traversewhole(formula *m2hBit) {
	var last *m2hBit
	var traverse func(bit *m2hBit)
	traverse = func(bit *m2hBit) {
		for i := 0; i < len(bit.contents); i++ {
			element := bit.contents[i]
			if element.typ != "" {
				if element.typ == "alpha" {
					// italicize: replaces the first occurrence of
					// the element in the list
					for j, e := range bit.contents {
						if e == element {
							bit.contents[j] = c.taggedComplete([]*m2hBit{element}, "i", false)
							break
						}
					}
				} else if element.typ == "font" && last != nil && last.typ == "number" {
					element.contents = append([]*m2hBit{m2hConstant(" ")}, element.contents...)
				}
				last = element
			} else if element.isFormulaBit() {
				traverse(element)
			}
		}
	}
	traverse(formula)
}

// ---------------------------------------------------------------------
// Output

func (c *m2hConv) gethtml(b *m2hBit, sb *strings.Builder) {
	switch b.out.kind {
	case moEmpty:
	case moFixed:
		for _, h := range b.html {
			sb.WriteString(h)
		}
	case moString:
		sb.WriteString(b.str)
	case moContents:
		for _, e := range b.contents {
			c.gethtml(e, sb)
		}
	case moTagged:
		o := b.out
		if o.empty {
			if c.checktag(b) {
				sb.WriteString("<" + o.tag + "/>")
				if o.breaklines {
					sb.WriteString("\n")
				}
			}
			return
		}
		if c.checktag(b) {
			sb.WriteString("<" + o.tag + ">")
			if o.breaklines {
				sb.WriteString("\n")
			}
		}
		for _, e := range b.contents {
			c.gethtml(e, sb)
		}
		if c.checktag(b) {
			fields := pySplit(o.tag)
			if len(fields) == 0 {
				panic(m2hCrash("IndexError: list index out of range"))
			}
			if o.breaklines {
				sb.WriteString("\n</" + fields[0] + ">\n")
			} else {
				sb.WriteString("</" + fields[0] + ">")
			}
		}
	}
}

func (c *m2hConv) checktag(b *m2hBit) bool {
	if b.out.tag == "" {
		c.traceError("No tag in " + b.String())
		return false
	}
	return true
}

// math2html converts a LaTeX formula to HTML like
// docutils.utils.math.math2html.math2html().  It returns the messages the
// original prints to stderr and, if the original raises an exception, a
// description of that exception.
func math2html(formula string, displaymode bool) (html string, errors []string, crash string) {
	c := &m2hConv{displaymode: displaymode}
	defer func() {
		if r := recover(); r != nil {
			if cr, ok := r.(m2hCrash); ok {
				html, errors, crash = "", c.errors, string(cr)
				return
			}
			panic(r)
		}
	}()
	whole := c.parseformula(formula)
	c.process(whole)
	var sb strings.Builder
	c.gethtml(whole, &sb)
	return sb.String(), c.errors, ""
}

// ---------------------------------------------------------------------
// NFC normalization (unicodedata.normalize('NFC', s))

const (
	hangulSBase  = 0xAC00
	hangulLBase  = 0x1100
	hangulVBase  = 0x1161
	hangulTBase  = 0x11A7
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulNCount = hangulVCount * hangulTCount
	hangulSCount = hangulLCount * hangulNCount
)

func nfcDecompose(dst []rune, r rune) []rune {
	if r >= hangulSBase && r < hangulSBase+hangulSCount {
		s := r - hangulSBase
		dst = append(dst, hangulLBase+s/hangulNCount, hangulVBase+(s%hangulNCount)/hangulTCount)
		if t := s % hangulTCount; t != 0 {
			dst = append(dst, hangulTBase+t)
		}
		return dst
	}
	if d, ok := nfcDecomp[r]; ok {
		for _, x := range d {
			dst = nfcDecompose(dst, x)
		}
		return dst
	}
	return append(dst, r)
}

func nfcNormalize(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var rs []rune
	for _, r := range s {
		rs = nfcDecompose(rs, r)
	}
	// canonical ordering
	for i := 1; i < len(rs); i++ {
		ci := nfcClass[rs[i]]
		if ci == 0 {
			continue
		}
		for j := i; j > 0 && nfcClass[rs[j-1]] > ci; j-- {
			rs[j-1], rs[j] = rs[j], rs[j-1]
		}
	}
	// canonical composition
	out := rs[:0]
	starter := -1
	lastClass := -1
	for _, r := range rs {
		cc := int(nfcClass[r])
		if starter >= 0 && (lastClass < cc || lastClass == -1) {
			if comp, ok := nfcCompose(out[starter], r); ok {
				out[starter] = comp
				continue
			}
		}
		if cc == 0 {
			starter = len(out)
			lastClass = -1
		} else {
			lastClass = cc
		}
		out = append(out, r)
	}
	return string(out)
}

func nfcCompose(a, b rune) (rune, bool) {
	if a >= hangulLBase && a < hangulLBase+hangulLCount && b >= hangulVBase && b < hangulVBase+hangulVCount {
		return hangulSBase + ((a-hangulLBase)*hangulVCount+(b-hangulVBase))*hangulTCount, true
	}
	if a >= hangulSBase && a < hangulSBase+hangulSCount && (a-hangulSBase)%hangulTCount == 0 &&
		b > hangulTBase && b < hangulTBase+hangulTCount {
		return a + (b - hangulTBase), true
	}
	c, ok := nfcComp[[2]rune{a, b}]
	return c, ok
}
