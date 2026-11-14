package rstgo

// A small backtracking regular-expression engine implementing the subset of
// Python's "re" syntax and semantics used by Docutils (look-ahead,
// look-behind, backreferences, Unicode-aware \w \s \d, verbose mode).
//
// Go's regexp package (RE2) cannot express Docutils' inline-markup
// recognition patterns, so they are compiled with this engine instead, which
// lets the original patterns be used verbatim.  Positions are byte offsets
// into the (UTF-8) subject string.

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	reVerbose = 1 << iota
	reIgnoreCase
	reDotAll
	reMultiline
)

const reStepLimit = 20000000

type reState struct {
	s     string
	caps  []int
	steps int
}

type reFunc func(st *reState, pos int, k func(int) bool) bool

// single-rune matcher (for fast paths)
type runeTest func(r rune) bool

type reNode struct {
	fn    reFunc
	one   runeTest // non-nil if node matches exactly one rune
	width int      // fixed width in runes, -1 if variable
}

type pyRegexp struct {
	src    string
	prog   reFunc
	ncap   int
	names  map[string]int
	gnames []string
	// Optional search accelerators (must never reject a real match):
	// quick rejects whole subjects, prefilter rejects start positions.
	quick     func(s string) bool
	prefilter func(s string, pos int) bool
}

type pyMatch struct {
	re   *pyRegexp
	s    string
	caps []int
}

func (m *pyMatch) Start(i int) int { return m.caps[2*i] }
func (m *pyMatch) End(i int) int   { return m.caps[2*i+1] }
func (m *pyMatch) Has(i int) bool  { return m.caps[2*i] >= 0 }
func (m *pyMatch) Group(i int) string {
	if m.caps[2*i] < 0 {
		return ""
	}
	return m.s[m.caps[2*i]:m.caps[2*i+1]]
}
func (m *pyMatch) idx(name string) int {
	i, ok := m.re.names[name]
	if !ok {
		panic("pyre: no such group: " + name)
	}
	return i
}
func (m *pyMatch) Named(name string) string  { return m.Group(m.idx(name)) }
func (m *pyMatch) HasNamed(name string) bool { return m.Has(m.idx(name)) }
func (m *pyMatch) StartNamed(name string) int {
	return m.Start(m.idx(name))
}
func (m *pyMatch) EndNamed(name string) int { return m.End(m.idx(name)) }

func (re *pyRegexp) run(s string, pos int, full bool) *pyMatch {
	st := &reState{s: s, caps: make([]int, 2*re.ncap)}
	for i := range st.caps {
		st.caps[i] = -1
	}
	end := -1
	ok := re.prog(st, pos, func(p int) bool {
		if full && p != len(s) {
			return false
		}
		end = p
		return true
	})
	if !ok {
		return nil
	}
	st.caps[0], st.caps[1] = pos, end
	return &pyMatch{re: re, s: s, caps: st.caps}
}

// Match is like Python's pattern.match(s).
func (re *pyRegexp) Match(s string) *pyMatch { return re.run(s, 0, false) }

// MatchAt is like Python's pattern.match(s, pos).
func (re *pyRegexp) MatchAt(s string, pos int) *pyMatch { return re.run(s, pos, false) }

// FullMatch is like Python's pattern.fullmatch(s).
func (re *pyRegexp) FullMatch(s string) *pyMatch { return re.run(s, 0, true) }

// Search is like Python's pattern.search(s).
func (re *pyRegexp) Search(s string) *pyMatch { return re.SearchFrom(s, 0) }

func (re *pyRegexp) SearchFrom(s string, pos int) *pyMatch {
	if re.quick != nil && !re.quick(s[pos:]) {
		return nil
	}
	for {
		if re.prefilter == nil || re.prefilter(s, pos) {
			if m := re.run(s, pos, false); m != nil {
				return m
			}
		}
		if pos >= len(s) {
			return nil
		}
		_, w := utf8.DecodeRuneInString(s[pos:])
		pos += w
	}
}

// Split is like Python's pattern.split(s) (without capture-group output).
func (re *pyRegexp) Split(s string) []string {
	var out []string
	last := 0
	pos := 0
	for pos <= len(s) {
		m := re.SearchFrom(s, pos)
		if m == nil {
			break
		}
		if m.End(0) == m.Start(0) {
			if m.Start(0) >= len(s) {
				break
			}
			_, w := utf8.DecodeRuneInString(s[m.Start(0):])
			pos = m.Start(0) + w
			continue
		}
		out = append(out, s[last:m.Start(0)])
		last = m.End(0)
		pos = last
	}
	out = append(out, s[last:])
	return out
}

// ReplaceAllFunc is like Python's pattern.sub(func, s).
func (re *pyRegexp) ReplaceAllFunc(s string, f func(m *pyMatch) string) string {
	var b strings.Builder
	last := 0
	pos := 0
	for pos <= len(s) {
		m := re.SearchFrom(s, pos)
		if m == nil {
			break
		}
		b.WriteString(s[last:m.Start(0)])
		b.WriteString(f(m))
		last = m.End(0)
		if m.End(0) == m.Start(0) {
			if m.Start(0) >= len(s) {
				break
			}
			_, w := utf8.DecodeRuneInString(s[m.Start(0):])
			b.WriteString(s[m.Start(0) : m.Start(0)+w])
			last = m.Start(0) + w
			pos = last
		} else {
			pos = last
		}
	}
	if last < len(s) {
		b.WriteString(s[last:])
	}
	return b.String()
}

func (re *pyRegexp) ReplaceAll(s, repl string) string {
	return re.ReplaceAllFunc(s, func(*pyMatch) string { return repl })
}

// ---------------------------------------------------------------- compiler

type reParser struct {
	src   []rune
	pos   int
	flags int
	ncap  int
	names map[string]int
}

func pyCompile(pat string, flags ...int) *pyRegexp {
	f := 0
	for _, x := range flags {
		f |= x
	}
	p := &reParser{src: []rune(pat), flags: f, ncap: 1, names: map[string]int{}}
	n := p.parseAlt()
	if p.pos < len(p.src) {
		panic(fmt.Sprintf("pyre: unexpected %q at %d in %q", string(p.src[p.pos]), p.pos, pat))
	}
	return &pyRegexp{src: pat, prog: n.fn, ncap: p.ncap, names: p.names}
}

func (p *reParser) more() bool { return p.pos < len(p.src) }
func (p *reParser) peek() rune { return p.src[p.pos] }

func (p *reParser) skipVerbose() {
	if p.flags&reVerbose == 0 {
		return
	}
	for p.more() {
		c := p.peek()
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' {
			p.pos++
		} else if c == '#' {
			for p.more() && p.peek() != '\n' {
				p.pos++
			}
		} else {
			break
		}
	}
}

func (p *reParser) parseAlt() reNode {
	alts := []reNode{p.parseSeq()}
	for p.more() && p.peek() == '|' {
		p.pos++
		alts = append(alts, p.parseSeq())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	width := alts[0].width
	fns := make([]reFunc, len(alts))
	allOne := true
	for i, a := range alts {
		fns[i] = a.fn
		if a.width != width {
			width = -1
		}
		if a.one == nil {
			allOne = false
		}
	}
	node := reNode{width: width}
	if allOne {
		tests := make([]runeTest, len(alts))
		for i, a := range alts {
			tests[i] = a.one
		}
		node.one = func(r rune) bool {
			for _, t := range tests {
				if t(r) {
					return true
				}
			}
			return false
		}
		node.fn = oneFn(node.one)
		return node
	}
	node.fn = func(st *reState, pos int, k func(int) bool) bool {
		for _, f := range fns {
			if f(st, pos, k) {
				return true
			}
		}
		return false
	}
	return node
}

func oneFn(t runeTest) reFunc {
	return func(st *reState, pos int, k func(int) bool) bool {
		if pos >= len(st.s) {
			return false
		}
		r, w := utf8.DecodeRuneInString(st.s[pos:])
		if !t(r) {
			return false
		}
		return k(pos + w)
	}
}

func (p *reParser) parseSeq() reNode {
	var items []reNode
	for {
		p.skipVerbose()
		if !p.more() || p.peek() == '|' || p.peek() == ')' {
			break
		}
		atom := p.parseAtom()
		p.skipVerbose()
		atom = p.parseQuant(atom)
		items = append(items, atom)
	}
	if len(items) == 0 {
		return reNode{fn: func(st *reState, pos int, k func(int) bool) bool { return k(pos) }, width: 0}
	}
	if len(items) == 1 {
		return items[0]
	}
	width := 0
	for _, it := range items {
		if it.width < 0 || width < 0 {
			width = -1
		} else {
			width += it.width
		}
	}
	// right fold
	fn := items[len(items)-1].fn
	for i := len(items) - 2; i >= 0; i-- {
		a, b := items[i].fn, fn
		fn = func(st *reState, pos int, k func(int) bool) bool {
			return a(st, pos, func(p int) bool { return b(st, p, k) })
		}
	}
	return reNode{fn: fn, width: width}
}

func (p *reParser) parseQuant(atom reNode) reNode {
	for p.more() {
		min, max := 0, -1
		c := p.peek()
		switch c {
		case '*':
			p.pos++
		case '+':
			min = 1
			p.pos++
		case '?':
			max = 1
			p.pos++
		case '{':
			// try to parse {m,n}
			save := p.pos
			p.pos++
			m, okm := p.parseInt()
			n := m
			okn := okm
			if p.more() && p.peek() == ',' {
				p.pos++
				n, okn = p.parseInt()
				if !okn {
					n = -1
				}
				if !okm {
					m = 0
				}
				okm = true
			}
			if !p.more() || p.peek() != '}' || !okm {
				p.pos = save
				return atom
			}
			p.pos++
			min, max = m, n
		default:
			return atom
		}
		lazy := false
		if p.more() && p.peek() == '?' {
			lazy = true
			p.pos++
		}
		atom = makeQuant(atom, min, max, lazy)
		p.skipVerbose()
	}
	return atom
}

func (p *reParser) parseInt() (int, bool) {
	n, ok := 0, false
	for p.more() && p.peek() >= '0' && p.peek() <= '9' {
		n = n*10 + int(p.peek()-'0')
		p.pos++
		ok = true
	}
	return n, ok
}

func makeQuant(atom reNode, min, max int, lazy bool) reNode {
	width := -1
	if min == max && atom.width >= 0 {
		width = atom.width * min
	}
	if atom.one != nil {
		t := atom.one
		if !lazy {
			return reNode{width: width, fn: func(st *reState, pos int, k func(int) bool) bool {
				// collect greedy positions
				s := st.s
				var stack [64]int
				ends := stack[:0]
				p := pos
				n := 0
				for (max < 0 || n < max) && p < len(s) {
					r, w := utf8.DecodeRuneInString(s[p:])
					if !t(r) {
						break
					}
					p += w
					n++
					ends = append(ends, p)
				}
				for n >= min {
					q := pos
					if n > 0 {
						q = ends[n-1]
					}
					st.steps++
					if st.steps > reStepLimit {
						return false
					}
					if k(q) {
						return true
					}
					n--
				}
				return false
			}}
		}
		return reNode{width: width, fn: func(st *reState, pos int, k func(int) bool) bool {
			s := st.s
			p := pos
			n := 0
			for n < min {
				if p >= len(s) {
					return false
				}
				r, w := utf8.DecodeRuneInString(s[p:])
				if !t(r) {
					return false
				}
				p += w
				n++
			}
			for {
				st.steps++
				if st.steps > reStepLimit {
					return false
				}
				if k(p) {
					return true
				}
				if (max >= 0 && n >= max) || p >= len(s) {
					return false
				}
				r, w := utf8.DecodeRuneInString(s[p:])
				if !t(r) {
					return false
				}
				p += w
				n++
			}
		}}
	}
	inner := atom.fn
	var rec func(st *reState, pos int, count int, k func(int) bool) bool
	if !lazy {
		rec = func(st *reState, pos int, count int, k func(int) bool) bool {
			st.steps++
			if st.steps > reStepLimit {
				return false
			}
			if max < 0 || count < max {
				if inner(st, pos, func(p int) bool {
					if p == pos && count >= min {
						return false
					}
					return rec(st, p, count+1, k)
				}) {
					return true
				}
			}
			if count >= min {
				return k(pos)
			}
			return false
		}
	} else {
		rec = func(st *reState, pos int, count int, k func(int) bool) bool {
			st.steps++
			if st.steps > reStepLimit {
				return false
			}
			if count >= min {
				if k(pos) {
					return true
				}
			}
			if max < 0 || count < max {
				return inner(st, pos, func(p int) bool {
					if p == pos && count >= min {
						return false
					}
					return rec(st, p, count+1, k)
				})
			}
			return false
		}
	}
	return reNode{width: width, fn: func(st *reState, pos int, k func(int) bool) bool {
		return rec(st, pos, 0, k)
	}}
}

func (p *reParser) litNode(r rune) reNode {
	var t runeTest
	if p.flags&reIgnoreCase != 0 {
		lr := unicode.ToLower(r)
		ur := unicode.ToUpper(r)
		t = func(c rune) bool { return c == r || c == lr || c == ur }
	} else {
		t = func(c rune) bool { return c == r }
	}
	return reNode{fn: oneFn(t), one: t, width: 1}
}

func isPySpace(r rune) bool {
	switch {
	case r >= 0x09 && r <= 0x0d, r >= 0x1c && r <= 0x20, r == 0x85, r == 0xa0,
		r == 0x1680, r >= 0x2000 && r <= 0x200a, r == 0x2028, r == 0x2029,
		r == 0x202f, r == 0x205f, r == 0x3000:
		return true
	}
	return false
}

func isPyWord(r rune) bool {
	if r < 0x80 {
		return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func isPyDigit(r rune) bool {
	if r < 0x80 {
		return r >= '0' && r <= '9'
	}
	return unicode.IsDigit(r)
}

func classEscape(c rune) runeTest {
	switch c {
	case 'd':
		return isPyDigit
	case 'D':
		return func(r rune) bool { return !isPyDigit(r) }
	case 's':
		return isPySpace
	case 'S':
		return func(r rune) bool { return !isPySpace(r) }
	case 'w':
		return isPyWord
	case 'W':
		return func(r rune) bool { return !isPyWord(r) }
	}
	return nil
}

// parseEscapeChar parses a character escape after the backslash and returns
// the literal rune.
func (p *reParser) parseEscapeChar() rune {
	c := p.peek()
	p.pos++
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case 'f':
		return '\f'
	case 'v':
		return '\v'
	case 'a':
		return 7
	case '0':
		return 0
	case 'x':
		return p.hex(2)
	case 'u':
		return p.hex(4)
	case 'U':
		return p.hex(8)
	}
	return c
}

func (p *reParser) hex(n int) rune {
	var v rune
	for i := 0; i < n && p.more(); i++ {
		c := p.peek()
		var d rune
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			panic("pyre: bad hex escape")
		}
		v = v*16 + d
		p.pos++
	}
	return v
}

func (p *reParser) parseClass() reNode {
	// p.pos is after '['
	neg := false
	if p.more() && p.peek() == '^' {
		neg = true
		p.pos++
	}
	type rng struct{ lo, hi rune }
	var ranges []rng
	var tests []runeTest
	first := true
	for {
		if !p.more() {
			panic("pyre: unterminated class")
		}
		c := p.peek()
		if c == ']' && !first {
			p.pos++
			break
		}
		first = false
		p.pos++
		var lo rune
		if c == '\\' {
			e := p.peek()
			if t := classEscape(e); t != nil {
				p.pos++
				tests = append(tests, t)
				continue
			}
			lo = p.parseEscapeChar()
		} else {
			lo = c
		}
		hi := lo
		if p.pos+1 < len(p.src) && p.peek() == '-' && p.src[p.pos+1] != ']' {
			p.pos++
			h := p.peek()
			p.pos++
			if h == '\\' {
				h = p.parseEscapeChar()
			}
			hi = h
		}
		ranges = append(ranges, rng{lo, hi})
	}
	ic := p.flags&reIgnoreCase != 0
	var ascii [128]bool
	var others []rng
	for _, r := range ranges {
		for c := r.lo; c <= r.hi && c < 128; c++ {
			ascii[c] = true
			if ic {
				ascii[unicode.ToLower(c)] = true
				ascii[unicode.ToUpper(c)] = true
			}
		}
		if r.hi >= 128 {
			others = append(others, r)
		}
	}
	t := func(c rune) bool {
		in := false
		if c < 128 && ascii[c] {
			in = true
		} else {
			if c >= 128 {
				for _, r := range others {
					if c >= r.lo && c <= r.hi {
						in = true
						break
					}
				}
			}
			if !in {
				for _, t := range tests {
					if t(c) {
						in = true
						break
					}
				}
			}
		}
		return in != neg
	}
	return reNode{fn: oneFn(t), one: t, width: 1}
}

func (p *reParser) parseAtom() reNode {
	c := p.peek()
	p.pos++
	switch c {
	case '.':
		dotall := p.flags&reDotAll != 0
		t := func(r rune) bool { return dotall || r != '\n' }
		return reNode{fn: oneFn(t), one: t, width: 1}
	case '^':
		ml := p.flags&reMultiline != 0
		return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
			if pos == 0 || (ml && st.s[pos-1] == '\n') {
				return k(pos)
			}
			return false
		}}
	case '$':
		ml := p.flags&reMultiline != 0
		return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
			if pos == len(st.s) || (pos == len(st.s)-1 && st.s[pos] == '\n') || (ml && st.s[pos] == '\n') {
				return k(pos)
			}
			return false
		}}
	case '[':
		return p.parseClass()
	case '(':
		return p.parseGroup()
	case '\\':
		e := p.peek()
		if t := classEscape(e); t != nil {
			p.pos++
			return reNode{fn: oneFn(t), one: t, width: 1}
		}
		switch e {
		case 'b', 'B':
			p.pos++
			want := e == 'b'
			return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
				before, after := false, false
				if pos > 0 {
					r, _ := utf8.DecodeLastRuneInString(st.s[:pos])
					before = isPyWord(r)
				}
				if pos < len(st.s) {
					r, _ := utf8.DecodeRuneInString(st.s[pos:])
					after = isPyWord(r)
				}
				if (before != after) == want {
					return k(pos)
				}
				return false
			}}
		case 'A':
			p.pos++
			return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
				return pos == 0 && k(pos)
			}}
		case 'Z':
			p.pos++
			return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
				return pos == len(st.s) && k(pos)
			}}
		}
		if e >= '1' && e <= '9' {
			p.pos++
			return backref(int(e - '0'))
		}
		return p.litNode(p.parseEscapeChar())
	}
	return p.litNode(c)
}

func backref(idx int) reNode {
	return reNode{width: -1, fn: func(st *reState, pos int, k func(int) bool) bool {
		a, b := st.caps[2*idx], st.caps[2*idx+1]
		if a < 0 {
			return false
		}
		sub := st.s[a:b]
		if !strings.HasPrefix(st.s[pos:], sub) {
			return false
		}
		return k(pos + len(sub))
	}}
}

func (p *reParser) parseGroup() reNode {
	// after '('
	capIdx := -1
	if p.more() && p.peek() == '?' {
		p.pos++
		c := p.peek()
		p.pos++
		switch c {
		case ':':
			n := p.parseAlt()
			p.expect(')')
			return n
		case '=', '!':
			n := p.parseAlt()
			p.expect(')')
			inner := n.fn
			neg := c == '!'
			return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
				ok := inner(st, pos, func(int) bool { return true })
				if ok == neg {
					return false
				}
				return k(pos)
			}}
		case '<':
			c2 := p.peek()
			p.pos++
			if c2 != '=' && c2 != '!' {
				panic("pyre: bad group syntax")
			}
			n := p.parseAlt()
			p.expect(')')
			if n.width < 0 {
				panic("pyre: look-behind requires fixed-width pattern")
			}
			inner := n.fn
			width := n.width
			neg := c2 == '!'
			return reNode{width: 0, fn: func(st *reState, pos int, k func(int) bool) bool {
				start := pos
				ok := true
				for i := 0; i < width; i++ {
					if start <= 0 {
						ok = false
						break
					}
					_, w := utf8.DecodeLastRuneInString(st.s[:start])
					start -= w
				}
				if ok {
					ok = inner(st, start, func(q int) bool { return q == pos })
				}
				if ok == neg {
					return false
				}
				return k(pos)
			}}
		case 'P':
			c2 := p.peek()
			p.pos++
			if c2 == '<' {
				start := p.pos
				for p.peek() != '>' {
					p.pos++
				}
				name := string(p.src[start:p.pos])
				p.pos++
				capIdx = p.ncap
				p.ncap++
				p.names[name] = capIdx
			} else if c2 == '=' {
				start := p.pos
				for p.peek() != ')' {
					p.pos++
				}
				name := string(p.src[start:p.pos])
				p.pos++
				return backref(p.names[name])
			} else {
				panic("pyre: bad (?P group")
			}
		default:
			panic("pyre: unsupported group (?" + string(c))
		}
	} else {
		capIdx = p.ncap
		p.ncap++
	}
	n := p.parseAlt()
	p.expect(')')
	inner := n.fn
	idx := capIdx
	return reNode{width: n.width, fn: func(st *reState, pos int, k func(int) bool) bool {
		return inner(st, pos, func(q int) bool {
			o0, o1 := st.caps[2*idx], st.caps[2*idx+1]
			st.caps[2*idx], st.caps[2*idx+1] = pos, q
			if k(q) {
				return true
			}
			st.caps[2*idx], st.caps[2*idx+1] = o0, o1
			return false
		})
	}}
}

func (p *reParser) expect(c rune) {
	if !p.more() || p.peek() != c {
		panic(fmt.Sprintf("pyre: expected %q at %d in %q", string(c), p.pos, string(p.src)))
	}
	p.pos++
}
