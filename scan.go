package rstgo

// Hand-written scanners for the line-level patterns of the reStructuredText
// parser.  Each function implements exactly one of the regular expressions
// of docutils.parsers.rst.states (quoted in its comment); the regular
// expressions themselves are kept in the tests, where every scanner is
// checked against a backtracking regex engine (see oracle_test.go).

import (
	"strings"
	"unicode/utf8"
)

// lineMatch is the result of a line-level scanner: the equivalent of a
// Python match object anchored at the start of s.
type lineMatch struct {
	s      string
	end    int    // end of the whole match
	g1, e1 int    // group 1 (-1 if it did not participate)
	format string // enumerator format ("parens", "rparen", "period")
}

func (m lineMatch) End(int) int { return m.end }

// endCol is the end of the match counted in characters (Python's
// match.end()), for use as an indentation width.
func (m lineMatch) endCol() int {
	for i := 0; i < m.end; i++ {
		if m.s[i] >= utf8.RuneSelf {
			return utf8.RuneCountInString(m.s[:m.end])
		}
	}
	return m.end
}

func (m lineMatch) Group(i int) string {
	if i == 0 {
		return m.s[:m.end]
	}
	if m.g1 < 0 {
		return ""
	}
	return m.s[m.g1:m.e1]
}

type lineScanner func(line string) (lineMatch, bool)

func lm(s string, end int) lineMatch { return lineMatch{s: s, end: end, g1: -1, e1: -1} }

// spacesOrEnd implements "( +|$)" at position i: it returns the end of the
// match, or -1.
func spacesOrEnd(s string, i int) int {
	if i >= len(s) {
		return i
	}
	if s[i] != ' ' {
		if s[i] == '\n' && i == len(s)-1 {
			return i // "$" matches before a final newline
		}
		return -1
	}
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

// atDollar reports whether "$" matches at position i.
func atDollar(s string, i int) bool {
	return i == len(s) || (i == len(s)-1 && s[i] == '\n')
}

func isASCIIAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func isASCIIAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isAlnumPlus(c byte) bool  { return isASCIIAlnum(c) || c == '_' || c == '-' }

// isNonalphanum7bit: [!-/:-@[-`{-~]
func isNonalphanum7bit(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// isNameChar: (?!_)\w
func isNameChar(r rune) bool { return r != '_' && isPyWord(r) }

func isNameSep(c byte) bool { return c == '-' || c == '.' || c == '_' || c == '+' || c == ':' }

// simplenameMax returns the end of the longest match of
// (?:(?!_)\w)+(?:[-._+:](?:(?!_)\w)+)* at i, or -1.
func simplenameMax(s string, i int) int {
	end := -1
	p := i
	for {
		q := p
		for q < len(s) {
			c := s[q]
			if c < utf8.RuneSelf {
				if c == '_' || !(isASCIIAlnum(c)) {
					break
				}
				q++
				continue
			}
			r, w := utf8.DecodeRuneInString(s[q:])
			if !isNameChar(r) {
				break
			}
			q += w
		}
		if q == p {
			return end
		}
		end = q
		if q < len(s) && isNameSep(s[q]) {
			p = q + 1
			continue
		}
		return end
	}
}

// simplenamePrevEnd returns the next shorter possible end of a simple name
// starting at i (the order in which a backtracking matcher gives up
// characters), or -1.
func simplenamePrevEnd(s string, i, end int) int {
	for {
		_, w := utf8.DecodeLastRuneInString(s[:end])
		end -= w
		if end <= i {
			return -1
		}
		r, _ := utf8.DecodeLastRuneInString(s[:end])
		if isNameChar(r) {
			return end
		}
	}
}

// isSimplename reports whether all of s is a simple name.
func isSimplename(s string) bool {
	return s != "" && simplenameMax(s, 0) == len(s)
}

// ` *$`
func scanBlank(s string) (lineMatch, bool) {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if !atDollar(s, i) {
		return lineMatch{}, false
	}
	return lm(s, i), true
}

// ` +`
func scanIndent(s string) (lineMatch, bool) {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if i == 0 {
		return lineMatch{}, false
	}
	return lm(s, i), true
}

// `[-+*•‣⁃]( +|$)`
func scanBullet(s string) (lineMatch, bool) {
	if s == "" {
		return lineMatch{}, false
	}
	w := 1
	switch s[0] {
	case '-', '+', '*':
	case 0xe2:
		if !(strings.HasPrefix(s, "•") || strings.HasPrefix(s, "‣") || strings.HasPrefix(s, "⁃")) {
			return lineMatch{}, false
		}
		w = 3
	default:
		return lineMatch{}, false
	}
	end := spacesOrEnd(s, w)
	if end < 0 {
		return lineMatch{}, false
	}
	return lm(s, end), true
}

// enumText returns the end of ([0-9]+|[a-z]|[A-Z]|[ivxlcdm]+|[IVXLCDM]+|#)
// at i when it is followed by the byte next, or -1.
func enumText(s string, i int, next byte) int {
	j := i
	for j < len(s) && (isASCIIAlnum(s[j]) || s[j] == '#') {
		j++
	}
	if j == i || j >= len(s) || s[j] != next {
		return -1
	}
	t := s[i:j]
	if t == "#" || len(t) == 1 && (isASCIIAlpha(t[0]) || t[0] >= '0' && t[0] <= '9') {
		return j
	}
	all := func(set string) bool {
		for k := 0; k < len(t); k++ {
			if strings.IndexByte(set, t[k]) < 0 {
				return false
			}
		}
		return true
	}
	if all("0123456789") || all("ivxlcdm") || all("IVXLCDM") {
		return j
	}
	return -1
}

// `((?P<parens>\(ENUM\))|(?P<rparen>ENUM\))|(?P<period>ENUM\.))( +|$)`
func scanEnumerator(s string) (lineMatch, bool) {
	if s == "" {
		return lineMatch{}, false
	}
	var m lineMatch
	if s[0] == '(' {
		j := enumText(s, 1, ')')
		if j < 0 {
			return lineMatch{}, false
		}
		m = lineMatch{s: s, g1: 0, e1: j + 1, format: "parens"}
	} else if j := enumText(s, 0, ')'); j >= 0 {
		m = lineMatch{s: s, g1: 0, e1: j + 1, format: "rparen"}
	} else if j := enumText(s, 0, '.'); j >= 0 {
		m = lineMatch{s: s, g1: 0, e1: j + 1, format: "period"}
	} else {
		return lineMatch{}, false
	}
	m.end = spacesOrEnd(s, m.e1)
	if m.end < 0 {
		return lineMatch{}, false
	}
	return m, true
}

// ":(?![: ])([^:\\\\]|\\\\.|:(?!([ `]|$)))*(?<! ):( +|$)"
func scanFieldMarker(s string) (lineMatch, bool) {
	if len(s) < 2 || s[0] != ':' || s[1] == ':' || s[1] == ' ' {
		return lineMatch{}, false
	}
	i := 1
	for i < len(s) {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) || s[i+1] == '\n' {
				return lineMatch{}, false
			}
			_, w := utf8.DecodeRuneInString(s[i+1:])
			i += 1 + w
		case ':':
			if i+1 == len(s) || s[i+1] == ' ' || (s[i+1] == '\n' && i+2 == len(s)) {
				if s[i-1] == ' ' {
					return lineMatch{}, false
				}
				end := spacesOrEnd(s, i+1)
				if end < 0 {
					return lineMatch{}, false
				}
				return lm(s, end), true
			}
			if s[i+1] == '`' {
				return lineMatch{}, false
			}
			i++
		default:
			i++
		}
	}
	return lineMatch{}, false
}

// option marker: `OPTION(, OPTION)*(  +| ?$)` with
//
//	OPTION   = (SHORTOPT|LONGOPT)
//	SHORTOPT = (-|\+)ALNUM( ?OPTARG)?
//	LONGOPT  = (--|/)ALNUM ALNUMPLUS*([ =]OPTARG)?
//	OPTARG   = (ALPHA ALNUMPLUS*|<[^<>]+>)
//
// The matcher explores the alternatives in the order of a backtracking
// regex engine; k is the continuation.
func scanOptionMarker(s string) (lineMatch, bool) {
	if s == "" || (s[0] != '-' && s[0] != '+' && s[0] != '/') {
		return lineMatch{}, false
	}
	end := -1
	var afterOption func(p int) bool
	optarg := func(q int, k func(int) bool) bool {
		if q < len(s) && isASCIIAlpha(s[q]) {
			e := q + 1
			for e < len(s) && isAlnumPlus(s[e]) {
				e++
			}
			for ; e > q; e-- {
				if k(e) {
					return true
				}
			}
		}
		if q < len(s) && s[q] == '<' {
			e := q + 1
			for e < len(s) && s[e] != '<' && s[e] != '>' {
				e++
			}
			if e > q+1 && e < len(s) && s[e] == '>' {
				return k(e + 1)
			}
		}
		return false
	}
	option := func(p int, k func(int) bool) bool {
		if p >= len(s) {
			return false
		}
		// short option
		if (s[p] == '-' || s[p] == '+') && p+1 < len(s) && isASCIIAlnum(s[p+1]) {
			a := p + 2
			if a < len(s) && s[a] == ' ' && optarg(a+1, k) {
				return true
			}
			if optarg(a, k) || k(a) {
				return true
			}
		}
		// long option
		b := -1
		if strings.HasPrefix(s[p:], "--") {
			b = p + 2
		} else if s[p] == '/' {
			b = p + 1
		}
		if b < 0 || b >= len(s) || !isASCIIAlnum(s[b]) {
			return false
		}
		n := b + 1
		for n < len(s) && isAlnumPlus(s[n]) {
			n++
		}
		for ; n > b; n-- {
			if n < len(s) && (s[n] == ' ' || s[n] == '=') && optarg(n+1, k) {
				return true
			}
			if k(n) {
				return true
			}
		}
		return false
	}
	afterOption = func(p int) bool {
		// (, OPTION)* greedy
		if strings.HasPrefix(s[p:], ", ") && option(p+2, afterOption) {
			return true
		}
		// (  +| ?$)
		if strings.HasPrefix(s[p:], "  ") {
			e := p
			for e < len(s) && s[e] == ' ' {
				e++
			}
			end = e
			return true
		}
		if p < len(s) && s[p] == ' ' && atDollar(s, p+1) {
			end = p + 1
			return true
		}
		if atDollar(s, p) {
			end = p
			return true
		}
		return false
	}
	if !option(0, afterOption) {
		return lineMatch{}, false
	}
	return lm(s, end), true
}

func scanLiteralPrefix(prefix string) lineScanner {
	return func(s string) (lineMatch, bool) {
		if !strings.HasPrefix(s, prefix) {
			return lineMatch{}, false
		}
		end := spacesOrEnd(s, len(prefix))
		if end < 0 {
			return lineMatch{}, false
		}
		m := lm(s, end)
		m.g1, m.e1 = len(prefix), end
		return m, true
	}
}

var (
	scanDoctest        = scanLiteralPrefix(">>>") // `>>>( +|$)`
	scanLineBlock      = scanLiteralPrefix("|")   // `\|( +|$)`
	scanExplicitMarkup = scanLiteralPrefix("..")  // `\.\.( +|$)`
	scanAnonymous      = scanLiteralPrefix("__")  // `__( +|$)`
)

// trailingSpacesToEnd implements " *$" at i and returns the end or -1.
func trailingSpacesToEnd(s string, i int) int {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if !atDollar(s, i) {
		return -1
	}
	return i
}

// gridBorder implements `\+C[C+]+C\+ *$` for C = '-' or '='.
func gridBorder(s string, c byte) (lineMatch, bool) {
	if len(s) < 5 || s[0] != '+' || s[1] != c {
		return lineMatch{}, false
	}
	i := 2
	for i < len(s) && (s[i] == c || s[i] == '+') {
		i++
	}
	// s[2:i] is the run; the pattern needs [C+]+ C + inside it
	if i-2 < 3 || s[i-1] != '+' || s[i-2] != c {
		return lineMatch{}, false
	}
	end := trailingSpacesToEnd(s, i)
	if end < 0 {
		return lineMatch{}, false
	}
	return lm(s, end), true
}

// `\+-[-+]+-\+ *$`
func scanGridTableTop(s string) (lineMatch, bool) { return gridBorder(s, '-') }

func isGridTableTop(s string) bool { _, ok := gridBorder(s, '-'); return ok }

// `\+=[=+]+=\+ *$`
func scanGridHeadBodySep(s string) (lineMatch, bool) { return gridBorder(s, '=') }

// `=+( +=+)+ *$`
func scanSimpleTableTop(s string) (lineMatch, bool) {
	i, groups := 0, 0
	for {
		j := i
		for j < len(s) && s[j] == '=' {
			j++
		}
		if j == i {
			break
		}
		groups++
		i = j
		for j < len(s) && s[j] == ' ' {
			j++
		}
		if j == i || j >= len(s) || s[j] != '=' {
			break
		}
		i = j
	}
	if groups < 2 {
		return lineMatch{}, false
	}
	end := trailingSpacesToEnd(s, i)
	if end < 0 {
		return lineMatch{}, false
	}
	return lm(s, end), true
}

// charThenSet implements `C[ C]*$`.
func charThenSet(s string, c byte) bool {
	if s == "" || s[0] != c {
		return false
	}
	i := 1
	for i < len(s) && (s[i] == c || s[i] == ' ') {
		i++
	}
	return atDollar(s, i)
}

// `=+[ =]*$` and `=[ =]*$` (equivalent)
func isSimpleTableBorder(s string) bool { return charThenSet(s, '=') }

// `-[ -]*$`
func isSimpleSpanLine(s string) bool { return charThenSet(s, '-') }

// `([!-/:-@[-`{-~])\1* *$`
func scanLine(s string) (lineMatch, bool) {
	if s == "" || !isNonalphanum7bit(s[0]) {
		return lineMatch{}, false
	}
	i := 1
	for i < len(s) && s[i] == s[0] {
		i++
	}
	end := trailingSpacesToEnd(s, i)
	if end < 0 {
		return lineMatch{}, false
	}
	m := lm(s, end)
	m.g1, m.e1 = 0, 1
	return m, true
}

// “ (always matches)
func scanText(s string) (lineMatch, bool) { return lm(s, 0), true }

// `(SIMPLENAME)::( +|$)`
func scanEmbeddedDirective(s string) (lineMatch, bool) {
	for e := simplenameMax(s, 0); e > 0; e = simplenamePrevEnd(s, 0, e) {
		if strings.HasPrefix(s[e:], "::") {
			if end := spacesOrEnd(s, e+2); end >= 0 {
				m := lm(s, end)
				m.g1, m.e1 = 0, e
				return m, true
			}
		}
	}
	return lineMatch{}, false
}

// `([!-/:-@[-`{-~])`
func scanInitialQuoted(s string) (lineMatch, bool) {
	if s == "" || !isNonalphanum7bit(s[0]) {
		return lineMatch{}, false
	}
	m := lm(s, 1)
	m.g1, m.e1 = 0, 1
	return m, true
}

// explicitStart implements `\.\.[ ]+` and returns the position after it.
func explicitStart(s string) int {
	if len(s) < 3 || s[0] != '.' || s[1] != '.' || s[2] != ' ' {
		return -1
	}
	i := 3
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

// bracketLabel matches `\[(LABEL)\]([ ]+|$)` at i, where valid reports
// whether the text between the brackets is an acceptable label.
func bracketLabel(s string, i int, valid func(string) bool) (lineMatch, bool) {
	if i < 0 || i >= len(s) || s[i] != '[' {
		return lineMatch{}, false
	}
	j := strings.IndexByte(s[i+1:], ']')
	if j < 0 {
		return lineMatch{}, false
	}
	j += i + 1
	if !valid(s[i+1 : j]) {
		return lineMatch{}, false
	}
	end := spacesOrEnd(s, j+1)
	if end < 0 {
		return lineMatch{}, false
	}
	m := lm(s, end)
	m.g1, m.e1 = i+1, j
	return m, true
}

func isASCIIDigits(t string) bool {
	if t == "" {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] < '0' || t[i] > '9' {
			return false
		}
	}
	return true
}

// `\.\.[ ]+\[([0-9]+|\#|\#SIMPLENAME|\*)\]([ ]+|$)`
func scanFootnote(s string) (lineMatch, bool) {
	return bracketLabel(s, explicitStart(s), func(t string) bool {
		return isASCIIDigits(t) || t == "#" || t == "*" || (len(t) > 1 && t[0] == '#' && isSimplename(t[1:]))
	})
}

// `\.\.[ ]+\[(SIMPLENAME)\]([ ]+|$)`
func scanCitation(s string) (lineMatch, bool) {
	return bracketLabel(s, explicitStart(s), isSimplename)
}

// explicitIndicator implements `\.\.[ ]+C(?![ ]|$)`.
func explicitIndicator(s string, c byte) (lineMatch, bool) {
	i := explicitStart(s)
	if i < 0 || i >= len(s) || s[i] != c {
		return lineMatch{}, false
	}
	if i+1 >= len(s) || s[i+1] == ' ' || atDollar(s, i+1) {
		return lineMatch{}, false
	}
	return lm(s, i+1), true
}

// `\.\.[ ]+_(?![ ]|$)`
func scanHyperlinkTarget(s string) (lineMatch, bool) { return explicitIndicator(s, '_') }

// `\.\.[ ]+\|(?![ ]|$)`
func scanSubstitutionDef(s string) (lineMatch, bool) { return explicitIndicator(s, '|') }

// `\.\.[ ]+(SIMPLENAME)[ ]?::([ ]+|$)`
func scanDirective(s string) (lineMatch, bool) {
	i := explicitStart(s)
	if i < 0 {
		return lineMatch{}, false
	}
	for e := simplenameMax(s, i); e > 0; e = simplenamePrevEnd(s, i, e) {
		p := e
		if p < len(s) && s[p] == ' ' && strings.HasPrefix(s[p+1:], "::") {
			if end := spacesOrEnd(s, p+3); end >= 0 {
				m := lm(s, end)
				m.g1, m.e1 = i, e
				return m, true
			}
		}
		if strings.HasPrefix(s[p:], "::") {
			if end := spacesOrEnd(s, p+2); end >= 0 {
				m := lm(s, end)
				m.g1, m.e1 = i, e
				return m, true
			}
		}
	}
	return lineMatch{}, false
}

// `(?<!\\)(\\\\)*::$` (search)
func endsWithLiteralMarker(s string) bool {
	n := len(s)
	if n > 0 && s[n-1] == '\n' && strings.HasSuffix(s[:n-1], "::") {
		n--
	} else if !strings.HasSuffix(s, "::") {
		return false
	}
	i := n - 2
	bs := 0
	for i > 0 && s[i-1] == '\\' {
		bs++
		i--
	}
	return bs%2 == 0
}

// "(---?(?!-)|—) *(?=[^ \\n])" (match); returns the end or -1.
func scanAttribution(s string) int {
	i := 0
	switch {
	case strings.HasPrefix(s, "---"):
		if len(s) > 3 && s[3] == '-' {
			return -1
		}
		i = 3
	case strings.HasPrefix(s, "--"):
		i = 2
	case strings.HasPrefix(s, "—"):
		i = 3
	default:
		return -1
	}
	for i < len(s) && s[i] == ' ' {
		i++
	}
	if i >= len(s) || s[i] == '\n' {
		return -1
	}
	return i
}

// splitOptions is re.split(r', (?![^<]*>)', s).
func splitOptions(s string) []string {
	var out []string
	last := 0
	for i := 0; i+1 < len(s); i++ {
		if s[i] != ',' || s[i+1] != ' ' {
			continue
		}
		rest := s[i+2:]
		gt := strings.IndexByte(rest, '>')
		if gt >= 0 && strings.IndexByte(rest[:gt], '<') < 0 {
			continue // inside <...>
		}
		out = append(out, s[last:i])
		last = i + 2
		i++
	}
	return append(out, s[last:])
}

// splitClassifiers is re.split(' +: +', s).
func splitClassifiers(s string) []string {
	if !strings.Contains(s, " : ") {
		return []string{s}
	}
	var out []string
	last := 0
	i := 0
	for i < len(s) {
		if s[i] != ' ' {
			i++
			continue
		}
		start := i
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i+1 < len(s) && s[i] == ':' && s[i+1] == ' ' {
			i++
			for i < len(s) && s[i] == ' ' {
				i++
			}
			out = append(out, s[last:start])
			last = i
		}
	}
	return append(out, s[last:])
}

// enumSequenceMatches implements the enum.sequenceregexps: the pattern of
// the given sequence followed by "$", matched at the start of text.
func enumSequenceMatches(sequence, text string) bool {
	if strings.HasSuffix(text, "\n") {
		text = text[:len(text)-1]
	}
	if text == "" {
		return false
	}
	all := func(set string) bool {
		for k := 0; k < len(text); k++ {
			if strings.IndexByte(set, text[k]) < 0 {
				return false
			}
		}
		return true
	}
	switch sequence {
	case "arabic":
		return all("0123456789")
	case "loweralpha":
		return len(text) == 1 && text[0] >= 'a' && text[0] <= 'z'
	case "upperalpha":
		return len(text) == 1 && text[0] >= 'A' && text[0] <= 'Z'
	case "lowerroman":
		return all("ivxlcdm")
	case "upperroman":
		return all("IVXLCDM")
	}
	return false
}

func notWsNul(r rune) bool { return r != 0 && !isPySpace(r) }

func lastRune(s string, i int) rune {
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

// colonTail implements `[ ]?:([ ]+|$)` at p and returns the end or -1.
func colonTail(s string, p int) int {
	if p < len(s) && s[p] == ' ' && p+1 < len(s) && s[p+1] == ':' {
		if end := spacesOrEnd(s, p+2); end >= 0 {
			return end
		}
	}
	if p < len(s) && s[p] == ':' {
		return spacesOrEnd(s, p+1)
	}
	return -1
}

// scanExplicitTarget implements explicit.patterns.target (match):
//
//	( _ | (?!_)(?P<quote>`?)(?![ `])(?P<name>.+?)(?<![\s\x00])(?P=quote) )
//	(?<!(?<!\x00):)(?<![\s\x00])[ ]?:([ ]+|$)
func scanExplicitTarget(s string) (end int, name string, ok bool) {
	if s == "" {
		return 0, "", false
	}
	if s[0] == '_' {
		if e := colonTail(s, 1); e >= 0 {
			return e, "", true
		}
		return 0, "", false
	}
	start := 0
	quote := false
	if s[0] == '`' {
		quote = true
		start = 1
	}
	if start >= len(s) || s[start] == ' ' || s[start] == '`' {
		return 0, "", false
	}
	e := start
	for e < len(s) {
		r, w := utf8.DecodeRuneInString(s[e:])
		if r == '\n' {
			break
		}
		e += w
		// name = s[start:e]
		if !notWsNul(r) {
			continue
		}
		p := e
		if quote {
			if p >= len(s) || s[p] != '`' {
				continue
			}
			p++
		}
		// (?<!(?<!\x00):) : no unescaped colon before p
		if s[p-1] == ':' && !(p >= 2 && s[p-2] == 0) {
			continue
		}
		if tail := colonTail(s, p); tail >= 0 {
			return tail, s[start:e], true
		}
	}
	return 0, "", false
}

// scanExplicitReference implements explicit.patterns.reference (match):
//
//	( (?P<simple>SIMPLENAME)_ | `(?![ ])(?P<phrase>.+?)(?<![\s\x00])`_ )$
func scanExplicitReference(s string) (ref string, ok bool) {
	for e := simplenameMax(s, 0); e > 0; e = simplenamePrevEnd(s, 0, e) {
		if e < len(s) && s[e] == '_' && atDollar(s, e+1) {
			return s[:e], true
		}
	}
	if len(s) < 4 || s[0] != '`' || s[1] == ' ' {
		return "", false
	}
	try := func(n int) (string, bool) { // text ends at n
		if n < 4 || s[n-2] != '`' || s[n-1] != '_' {
			return "", false
		}
		phrase := s[1 : n-2]
		if phrase == "" || strings.ContainsRune(phrase, '\n') || !notWsNul(lastRune(phrase, len(phrase))) {
			return "", false
		}
		return phrase, true
	}
	if s[len(s)-1] == '\n' {
		if p, ok := try(len(s) - 1); ok {
			return p, true
		}
	}
	return try(len(s))
}

// scanExplicitSubstitution implements explicit.patterns.substitution
// (match):  ((?![ ])(?P<name>.+?)(?<![\s\x00])\|)([ ]+|$)
func scanExplicitSubstitution(s string) (end int, name string, ok bool) {
	if s == "" || s[0] == ' ' {
		return 0, "", false
	}
	e := 0
	for e < len(s) {
		r, w := utf8.DecodeRuneInString(s[e:])
		if r == '\n' {
			break
		}
		e += w
		if e < len(s) && s[e] == '|' && notWsNul(r) {
			if tail := spacesOrEnd(s, e+1); tail >= 0 {
				return tail, s[:e], true
			}
		}
	}
	return 0, "", false
}
