package rstgo

// Scanners for the remaining (directive, transform, writer and math)
// patterns of Docutils; see scan.go.

import (
	"strings"
	"unicode/utf8"
)

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// hexToDollar returns the hex digits s[i:e] if [0-9a-f]+ at i is followed
// by suffix and "$" (ignoring case).
func hexToDollar(s string, i int, suffix string) (string, bool) {
	e := i
	for e < len(s) && isHexDigit(s[e]) {
		e++
	}
	if e == i || !strings.HasPrefix(s[e:], suffix) || !atDollar(s, e+len(suffix)) {
		return "", false
	}
	return s[i:e], true
}

// scanUnicodeCode implements (match, re.IGNORECASE)
//
//	(?:0x|x|\\x|U\+?|\\u)([0-9a-f]+)$|&#x([0-9a-f]+);$
//
// and returns the hex digits.
func scanUnicodeCode(s string) (string, bool) {
	lower := strings.ToLower(prefix(s, 3))
	for _, p := range []string{"0x", "x", `\x`, "u+", "u", `\u`} {
		if strings.HasPrefix(lower, p) {
			if h, ok := hexToDollar(s, len(p), ""); ok {
				return h, true
			}
		}
	}
	if strings.HasPrefix(lower, "&#x") {
		return hexToDollar(s, 3, ";")
	}
	return "", false
}

// scanMeasure implements re.fullmatch('(-?[0-9.]+) *([a-zA-Zµ]*|%?)', s).
func scanMeasure(s string) (num, unit string, ok bool) {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	d := i
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == d {
		return "", "", false
	}
	num = s[:i]
	for i < len(s) && s[i] == ' ' {
		i++
	}
	unit = s[i:]
	if unit == "%" {
		return num, unit, true
	}
	for _, r := range unit {
		if !(r < 128 && isASCIIAlpha(byte(r)) || r == 'µ') {
			return "", "", false
		}
	}
	return num, unit, true
}

// scanRoleArguments implements (match)
//
//	(SIMPLENAME)\s*(\(\s*(SIMPLENAME)\s*\)\s*)?$
func scanRoleArguments(s string) (name, base string, ok bool) {
	skipWS := func(i int) int {
		for i < len(s) {
			r, w := utf8.DecodeRuneInString(s[i:])
			if !isPySpace(r) {
				break
			}
			i += w
		}
		return i
	}
	// "$" before a final newline is subsumed by \s*
	for e := simplenameMax(s, 0); e > 0; e = simplenamePrevEnd(s, 0, e) {
		p := skipWS(e)
		if p < len(s) && s[p] == '(' {
			b := skipWS(p + 1)
			for be := simplenameMax(s, b); be > 0; be = simplenamePrevEnd(s, b, be) {
				q := skipWS(be)
				if q < len(s) && s[q] == ')' && skipWS(q+1) == len(s) {
					return s[:e], s[b:be], true
				}
			}
		}
		if p == len(s) {
			return s[:e], "", true
		}
	}
	return "", "", false
}

// beforeUnicodeComment is re.split(r'( |\n|^)\.\. ', s)[0].
func beforeUnicodeComment(s string) string {
	if strings.HasPrefix(s, ".. ") {
		return ""
	}
	for i := 0; i+3 < len(s); i++ {
		if (s[i] == ' ' || s[i] == '\n') && s[i+1] == '.' && s[i+2] == '.' && s[i+3] == ' ' {
			return s[:i]
		}
	}
	return s
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// rcsDate implements one match of (re.IGNORECASE)
//
//	\$Date: (\d\d\d\d)[-/](\d\d)[-/](\d\d)[ T][\d:]+[^$]* \$
//
// at i; it returns the end and the replacement "\1-\2-\3".
func rcsDate(s string, i int) (int, string, bool) {
	if !hasPrefixFold(s[i:], "$Date: ") {
		return 0, "", false
	}
	p := i + 7
	digits := func(n int) (string, bool) {
		start := p
		for k := 0; k < n; k++ {
			r, w := utf8.DecodeRuneInString(s[p:])
			if p >= len(s) || !isPyDigit(r) {
				return "", false
			}
			p += w
		}
		return s[start:p], true
	}
	sep := func(set string) bool {
		if p < len(s) && strings.IndexByte(set, s[p]) >= 0 {
			p++
			return true
		}
		return false
	}
	y, ok := digits(4)
	if !ok || !sep("-/") {
		return 0, "", false
	}
	m, ok := digits(2)
	if !ok || !sep("-/") {
		return 0, "", false
	}
	d, ok := digits(2)
	if !ok || !sep(" T") {
		return 0, "", false
	}
	// [\d:]+ : at least one
	r, w := utf8.DecodeRuneInString(s[p:])
	if p >= len(s) || !(r == ':' || isPyDigit(r)) {
		return 0, "", false
	}
	p += w
	// [^$]* \$ : up to the next "$", which must follow a space
	k := strings.IndexByte(s[p:], '$')
	if k < 1 || s[p+k-1] != ' ' {
		return 0, "", false
	}
	return p + k + 1, y + "-" + m + "-" + d, true
}

// rcsGreedy implements one match of  PREFIX(.+)SUFFIX  at i, where the
// group is greedy and does not cross a newline; headEnd is the position
// after PREFIX.
func rcsGreedy(s string, headEnd int, suffix string) (int, string, bool) {
	line := s[headEnd:]
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		// the suffix may end right at the newline, but (.+) cannot contain it
		line = line[:nl]
	}
	k := strings.LastIndex(line, suffix)
	if k < 1 {
		return 0, "", false
	}
	return headEnd + k + len(suffix), line[:k], true
}

// rcsKeywordSub applies the n-th RCS keyword substitution of
// transforms.frontmatter.DocInfo (pattern.sub) and reports whether the
// pattern matched at all.
func rcsKeywordSub(n int, s string) (string, bool) {
	var b strings.Builder
	matched := false
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			continue
		}
		var end int
		var repl string
		var ok bool
		switch n {
		case 0:
			end, repl, ok = rcsDate(s, i)
		case 1: // \$RCSfile: (.+),v \$  (IGNORECASE)
			if hasPrefixFold(s[i:], "$RCSfile: ") {
				end, repl, ok = rcsGreedyFold(s, i+10, ",v $")
			}
		case 2: // \$[a-zA-Z]+: (.+) \$
			p := i + 1
			for p < len(s) && isASCIIAlpha(s[p]) {
				p++
			}
			if p > i+1 && strings.HasPrefix(s[p:], ": ") {
				end, repl, ok = rcsGreedy(s, p+2, " $")
			}
		}
		if !ok {
			continue
		}
		matched = true
		b.WriteString(s[last:i])
		b.WriteString(repl)
		last = end
		i = end - 1
	}
	if !matched {
		return s, false
	}
	b.WriteString(s[last:])
	return b.String(), true
}

// rcsGreedyFold is rcsGreedy with an ASCII case-insensitive suffix.
func rcsGreedyFold(s string, headEnd int, suffix string) (int, string, bool) {
	line := s[headEnd:]
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	for k := len(line) - len(suffix); k >= 1; k-- {
		if strings.EqualFold(line[k:k+len(suffix)], suffix) {
			return headEnd + k + len(suffix), line[:k], true
		}
	}
	return 0, "", false
}

// splitUnescaped is re.split('(?<!\x00)' + sep, s) for a one-byte sep.
func splitUnescaped(s string, sep byte) []string {
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep && !(i > 0 && s[i-1] == 0) {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	return append(out, s[last:])
}

// spaceDashes is re.sub('-(?=-)', '- ', s).
func spaceDashes(s string) string {
	if !strings.Contains(s, "--") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteByte(s[i])
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// scanNumUnit implements re.match(r'([0-9.]+)(\S*)$', s).
func scanNumUnit(s string) (num, unit string, ok bool) {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == 0 {
		return "", "", false
	}
	e := i
	for e < len(s) {
		r, w := utf8.DecodeRuneInString(s[e:])
		if isPySpace(r) {
			break
		}
		e += w
	}
	if !atDollar(s, e) {
		return "", "", false
	}
	return s[:i], s[i:e], true
}

// isNumOnly implements re.match(r'^[0-9.]+$', s).
func isNumOnly(s string) bool {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	return i > 0 && atDollar(s, i)
}

// wordsAndSpacesTokens is re.findall(r'[^ \n]+| +|\n', s).
func wordsAndSpacesTokens(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		j := i
		switch s[i] {
		case '\n':
			j++
		case ' ':
			for j < len(s) && s[j] == ' ' {
				j++
			}
		default:
			for j < len(s) && s[j] != ' ' && s[j] != '\n' {
				j++
			}
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// hasWordWrapPoint implements re.search(r'.+\W\W.+|[-?].+', s).
func hasWordWrapPoint(s string) bool {
	// second alternative: "-" or "?" followed by a character (not newline)
	for i := 0; i+1 < len(s); i++ {
		if (s[i] == '-' || s[i] == '?') && s[i+1] != '\n' {
			return true
		}
	}
	// first alternative: two adjacent non-word characters with a
	// character other than newline on either side
	var p2, p1 rune = -1, -1 // the two runes before the current one
	nonWord := func(r rune) bool { return r >= 0 && !isPyWord(r) }
	pending := false // p2,p1 was a valid pair start (\W\W preceded by .)
	var p3 rune = -1
	for _, r := range s {
		if pending && r != '\n' {
			return true
		}
		pending = nonWord(p1) && nonWord(r) && p2 >= 0 && p2 != '\n'
		p3, p2, p1 = p2, p1, r
	}
	_ = p3
	return false
}

// tex scanners (docutils.utils.math.latex2mathml)

// texCmdnameSplit implements re.match(r'([a-zA-Z]+)[ \n]*(.*)', s, DOTALL).
func texCmdnameSplit(s string) (string, string, bool) {
	i := 0
	for i < len(s) && isASCIIAlpha(s[i]) {
		i++
	}
	if i == 0 {
		return "", "", false
	}
	j := i
	for j < len(s) && (s[j] == ' ' || s[j] == '\n') {
		j++
	}
	return s[:i], s[j:], true
}

// texNumberSplit implements re.match(r'([0-9.,]*[0-9]+)(.*)', s, DOTALL).
func texNumberSplit(s string) (string, string, bool) {
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ',') {
		i++
	}
	for i > 0 && !(s[i-1] >= '0' && s[i-1] <= '9') {
		i--
	}
	if i == 0 {
		return "", "", false
	}
	return s[:i], s[i:], true
}

// texCmdTokenEnd implements re.match(r'(\\[a-zA-Z]+)\s*', s): it returns
// the end of the command and of the whole match.
func texCmdTokenEnd(s string) (int, int, bool) {
	if len(s) < 2 || s[0] != '\\' || !isASCIIAlpha(s[1]) {
		return 0, 0, false
	}
	i := 2
	for i < len(s) && isASCIIAlpha(s[i]) {
		i++
	}
	j := i
	for j < len(s) {
		r, w := utf8.DecodeRuneInString(s[j:])
		if !isPySpace(r) {
			break
		}
		j += w
	}
	return i, j, true
}

// texOptargSplit implements (match, VERBOSE|DOTALL)
//
//	\s*\[(?P<optarg>(\\]|[^\[\]]|\\])*)\](?P<remainder>.*$)
func texOptargSplit(s string) (string, string, bool) {
	i := 0
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !isPySpace(r) {
			break
		}
		i += w
	}
	if i >= len(s) || s[i] != '[' {
		return "", "", false
	}
	closer := -1
	for j := i + 1; j < len(s); j++ {
		if s[j] == '[' {
			break
		}
		if s[j] == ']' {
			closer = j
			if s[j-1] != '\\' {
				break
			}
		}
	}
	if closer < 0 {
		return "", "", false
	}
	return s[i+1 : closer], s[closer+1:], true
}

// texTextEdges is re.sub('(^[ \n]|[ \n]$)', ' ', s).
func texTextEdges(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c == ' ' || c == '\n') && (i == 0 || atDollar(s, i+1)) {
			b.WriteString(" ")
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
