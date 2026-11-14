package rstgo

// Hand-written scanners for the inline markup patterns of
// docutils.parsers.rst.states.Inliner.  As in scan.go, the original regular
// expressions live in the tests and serve as an oracle.

import (
	"strings"
	"unicode/utf8"
)

var (
	startPrefixASCII [128]bool
	endSuffixASCII   [128]bool
)

func init() {
	for _, r := range startPrefixRanges {
		for c := r[0]; c <= r[1] && c < 128; c++ {
			startPrefixASCII[c] = true
		}
	}
	for _, r := range endSuffixRanges {
		for c := r[0]; c <= r[1] && c < 128; c++ {
			endSuffixASCII[c] = true
		}
	}
}

// inlineScanner holds the mode of inline markup recognition:
// character-level markup uses the start-string prefix (^|(?<!\x00)) and
// an empty end-string suffix.
type inlineScanner struct{ charLevel bool }

// startOK implements the start-string prefix at position i:
// (^|(?<=\s|[openers delimiters]))
func (sc inlineScanner) startOK(s string, i, from int) bool {
	if i <= from {
		return true
	}
	c := s[i-1]
	if sc.charLevel {
		return c != 0
	}
	if c < utf8.RuneSelf {
		return startPrefixASCII[c] || c == ' ' || c == '\n' || (c >= 0x09 && c <= 0x0d) || (c >= 0x1c && c <= 0x1f)
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return isPySpace(r) || inRanges(startPrefixRanges, r)
}

// endOK implements the end-string suffix at position i:
// ($|(?=\s|[\x00 closing_delimiters delimiters closers]))
func (sc inlineScanner) endOK(s string, i int) bool {
	if sc.charLevel || i >= len(s) {
		return true
	}
	c := s[i]
	if c < utf8.RuneSelf {
		return endSuffixASCII[c] || c == 0 || c == ' ' || c == '\n' || (c >= 0x09 && c <= 0x0d) || (c >= 0x1c && c <= 0x1f)
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return isPySpace(r) || inRanges(endSuffixRanges, r)
}

// notSpaceAt implements (?!\s) at position i.
func notSpaceAt(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	c := s[i]
	if c < utf8.RuneSelf {
		return !(c == ' ' || c == '\n' || (c >= 0x09 && c <= 0x0d) || (c >= 0x1c && c <= 0x1f))
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return !isPySpace(r)
}

// initialMatch is a match of the "initial" inline pattern.
type initialMatch struct {
	start, end   int    // whole match
	key          string // dispatch key: start-string, "`", refend or "]_"
	markS, markE int    // group "start" or "backquote"
	roleS, roleE int    // group "role" (backquote alternative)
	nameS, nameE int    // group "refname" or "footnotelabel"
	citation     bool   // group "citationlabel" participated
}

// searchInitial implements patterns.initial.search(s[from:]) (positions
// are relative to s):
//
//	PREFIX (?P<initial_inline>
//	   (?P<start>\*\*|\*(?!\*)|``|_`|\|(?!\|))(?!\s)
//	 | (?P<whole>(?P<refname>SIMPLENAME)(?P<refend>__?)
//	     |\[(?P<footnotelabel>[0-9]+|\#(SIMPLENAME)?|\*|(?P<citationlabel>SIMPLENAME))(?P<fnend>\]_))SUFFIX
//	 | (?P<role>(:SIMPLENAME:)?)(?P<backquote>`(?!`))(?!\s)
//	)
func (sc inlineScanner) searchInitial(s string, from int) (initialMatch, bool) {
	for i := from; i < len(s); {
		c := s[i]
		w := 1
		var m initialMatch
		ok := false
		switch {
		case c == '*':
			if i+1 < len(s) && s[i+1] == '*' {
				if notSpaceAt(s, i+2) {
					m, ok = initialMatch{start: i, end: i + 2, key: "**", markS: i, markE: i + 2}, true
				}
			} else if notSpaceAt(s, i+1) {
				m, ok = initialMatch{start: i, end: i + 1, key: "*", markS: i, markE: i + 1}, true
			}
		case c == '`':
			if i+1 < len(s) && s[i+1] == '`' {
				if notSpaceAt(s, i+2) {
					m, ok = initialMatch{start: i, end: i + 2, key: "``", markS: i, markE: i + 2}, true
				}
			} else if notSpaceAt(s, i+1) {
				m, ok = initialMatch{start: i, end: i + 1, key: "`", markS: i, markE: i + 1, roleS: i, roleE: i}, true
			}
		case c == '_':
			if i+1 < len(s) && s[i+1] == '`' && notSpaceAt(s, i+2) {
				m, ok = initialMatch{start: i, end: i + 2, key: "_`", markS: i, markE: i + 2}, true
			}
		case c == '|':
			if !(i+1 < len(s) && s[i+1] == '|') && notSpaceAt(s, i+1) {
				m, ok = initialMatch{start: i, end: i + 1, key: "|", markS: i, markE: i + 1}, true
			}
		case c == '[':
			m, ok = sc.footnoteRefAt(s, i)
		case c == ':':
			m, ok = roleBackquoteAt(s, i)
		case c < utf8.RuneSelf:
			if isASCIIAlnum(c) {
				// a reference name cannot start inside a word
				if !sc.charLevel && i > from && isASCIIAlnum(s[i-1]) {
					break
				}
				m, ok = sc.refnameAt(s, i)
				if !ok && !sc.charLevel {
					// skip the rest of this ASCII word: no match can start there
					j := i + 1
					for j < len(s) && isASCIIAlnum(s[j]) {
						j++
					}
					i = j
					continue
				}
			}
		default:
			var r rune
			r, w = utf8.DecodeRuneInString(s[i:])
			if isNameChar(r) {
				m, ok = sc.refnameAt(s, i)
			}
		}
		if ok && sc.startOK(s, i, from) {
			return m, true
		}
		i += w
	}
	return initialMatch{}, false
}

func (sc inlineScanner) refnameAt(s string, i int) (initialMatch, bool) {
	for e := simplenameMax(s, i); e > 0; e = simplenamePrevEnd(s, i, e) {
		if e >= len(s) || s[e] != '_' {
			continue
		}
		if e+1 < len(s) && s[e+1] == '_' && sc.endOK(s, e+2) {
			return initialMatch{start: i, end: e + 2, key: "__", nameS: i, nameE: e}, true
		}
		if sc.endOK(s, e+1) {
			return initialMatch{start: i, end: e + 1, key: "_", nameS: i, nameE: e}, true
		}
	}
	return initialMatch{}, false
}

func (sc inlineScanner) footnoteRefAt(s string, i int) (initialMatch, bool) {
	j := i + 1
	for j < len(s) && s[j] != ']' {
		c := s[j]
		if c < utf8.RuneSelf && !(isASCIIAlnum(c) || isNameSep(c) || c == '#' || c == '*') {
			return initialMatch{}, false
		}
		j++
	}
	if j+1 >= len(s) || s[j+1] != '_' || !sc.endOK(s, j+2) {
		return initialMatch{}, false
	}
	label := s[i+1 : j]
	m := initialMatch{start: i, end: j + 2, key: "]_", nameS: i + 1, nameE: j}
	switch {
	case isASCIIDigits(label):
	case label == "#" || label == "*":
	case len(label) > 1 && label[0] == '#' && isSimplename(label[1:]):
	case isSimplename(label):
		m.citation = true
	default:
		return initialMatch{}, false
	}
	return m, true
}

// roleBackquoteAt matches (:SIMPLENAME:)`(?!`)(?!\s) at i (s[i] == ':').
func roleBackquoteAt(s string, i int) (initialMatch, bool) {
	for e := simplenameMax(s, i+1); e > 0; e = simplenamePrevEnd(s, i+1, e) {
		if e+1 < len(s) && s[e] == ':' && s[e+1] == '`' &&
			!(e+2 < len(s) && s[e+2] == '`') && notSpaceAt(s, e+2) {
			return initialMatch{start: i, end: e + 2, key: "`", markS: e + 1, markE: e + 2, roleS: i, roleE: e + 1}, true
		}
	}
	return initialMatch{}, false
}

// searchEnd implements the simple end-string patterns
//
//	LOOKBEHIND (ENDSTRING) SUFFIX
//
// searched in s[from:]: emphasis `*`, strong `**`, literal “ “ “ (whose
// look-behind only excludes whitespace) and target “ ` “.  It returns the
// position of the end-string, or -1 if there is none or if the first match
// is at the very start (Docutils requires at least one character).
func (sc inlineScanner) searchEnd(s string, from int, endstring string, nulAllowed bool) int {
	p := from
	for {
		k := strings.Index(s[p:], endstring)
		if k < 0 {
			return -1
		}
		p += k
		if p == from {
			if sc.endOK(s, p+len(endstring)) {
				return -1
			}
		} else {
			r := lastRune(s, p)
			if !isPySpace(r) && (nulAllowed || r != 0) && sc.endOK(s, p+len(endstring)) {
				return p
			}
		}
		p++
	}
}

// searchSubstitutionEnd implements (?<![\s\x00])(\|_{0,2})SUFFIX searched in
// s[from:]; it returns the start and end of the end-string.
func (sc inlineScanner) searchSubstitutionEnd(s string, from int) (int, int) {
	p := from
	for {
		k := strings.IndexByte(s[p:], '|')
		if k < 0 {
			return -1, -1
		}
		p += k
		if p == from || notWsNul(lastRune(s, p)) {
			n := 0
			for n < 2 && p+1+n < len(s) && s[p+1+n] == '_' {
				n++
			}
			for ; n >= 0; n-- {
				if sc.endOK(s, p+1+n) {
					if p == from {
						return -1, -1
					}
					return p, p + 1 + n
				}
			}
		}
		p++
	}
}

// interpretedEnd is a match of patterns.interpreted_or_phrase_ref.
type interpretedEnd struct {
	start, end   int // group 1 start (the backquote), match end
	roleS, roleE int // group "role" (":name:"), empty if roleS == roleE
}

// searchInterpretedEnd implements, searched in s[from:],
//
//	(?<!(?<!\x00)[\s\x00]) ( ` (?P<suffix>(?P<role>:SIMPLENAME:)?(?P<refend>__?)?) ) SUFFIX
//
// ok is false if there is no match or the match is at the very start.
func (sc inlineScanner) searchInterpretedEnd(s string, from int) (interpretedEnd, bool) {
	p := from
	for {
		k := strings.IndexByte(s[p:], '`')
		if k < 0 {
			return interpretedEnd{}, false
		}
		p += k
		if p > from {
			r, w := utf8.DecodeLastRuneInString(s[:p])
			if r == 0 || isPySpace(r) {
				// only allowed if that character is itself escaped
				q := p - w
				if !(q > from && s[q-1] == 0) {
					p++
					continue
				}
			}
		}
		if m, ok := sc.interpretedSuffixAt(s, p); ok {
			if p == from {
				return interpretedEnd{}, false
			}
			return m, true
		}
		p++
	}
}

func (sc inlineScanner) interpretedSuffixAt(s string, p int) (interpretedEnd, bool) {
	tail := func(a int) int { // (__?)? SUFFIX at a
		if a < len(s) && s[a] == '_' {
			if a+1 < len(s) && s[a+1] == '_' && sc.endOK(s, a+2) {
				return a + 2
			}
			if sc.endOK(s, a+1) {
				return a + 1
			}
		}
		if sc.endOK(s, a) {
			return a
		}
		return -1
	}
	if p+1 < len(s) && s[p+1] == ':' {
		for e := simplenameMax(s, p+2); e > 0; e = simplenamePrevEnd(s, p+2, e) {
			if e < len(s) && s[e] == ':' {
				if end := tail(e + 1); end >= 0 {
					return interpretedEnd{start: p, end: end, roleS: p + 1, roleE: e + 1}, true
				}
			}
		}
	}
	if end := tail(p + 1); end >= 0 {
		return interpretedEnd{start: p, end: end, roleS: p + 1, roleE: p + 1}, true
	}
	return interpretedEnd{}, false
}

// searchEmbeddedLink implements patterns.embedded_link.search(s):
//
//	( (?:[ \n]+|^) < (?!\s) (([^<>]|\x00[<>])+) (?<![\s\x00]) > ) $
//
// It returns the start of the match and the bounds of group 2.
func searchEmbeddedLink(s string) (start, g2s, g2e int, ok bool) {
	g := len(s) - 1
	if g >= 0 && s[g] == '\n' {
		g--
	}
	if g < 1 || s[g] != '>' || !notWsNul(lastRune(s, g)) {
		return 0, 0, 0, false
	}
	// The content may not contain unescaped angle brackets: it has to
	// start after the last one.
	minStart := 0
	for i := g - 1; i >= 0; i-- {
		if (s[i] == '<' || s[i] == '>') && !(i > 0 && s[i-1] == 0) {
			minStart = i + 1
			break
		}
	}
	for q := strings.IndexByte(s, '<'); q >= 0 && q+1 < g; {
		if q+1 >= minStart && (q == 0 || s[q-1] == ' ' || s[q-1] == '\n') && notSpaceAt(s, q+1) {
			start := q
			for start > 0 && (s[start-1] == ' ' || s[start-1] == '\n') {
				start--
			}
			return start, q + 1, g, true
		}
		k := strings.IndexByte(s[q+1:], '<')
		if k < 0 {
			break
		}
		q += 1 + k
	}
	return 0, 0, 0, false
}

func isUric(c byte) bool {
	return isASCIIAlnum(c) || c == 0 || strings.IndexByte("-_.!~*'()[];/:@&=+$,%", c) >= 0
}
func isUrilast(c byte) bool { return isASCIIAlnum(c) || strings.IndexByte("_~*/=+", c) >= 0 }
func isEmailc(c byte) bool {
	return isASCIIAlnum(c) || c == 0 || strings.IndexByte("-_!~*'{|}/#?^`&=+$%", c) >= 0
}

// uriMatch is a match of patterns.uri.
type uriMatch struct {
	start, end int
	schemeEnd  int // end of group "scheme" (== start for e-mail addresses)
	email      bool
}

// uriPart tries URIC* URI_END at a for every possible end, longest first,
// calling k with the end; it reports whether k accepted one.
func uriPart(s string, a int, k func(int) bool) bool {
	r := a
	for r < len(s) && isUric(s[r]) {
		r++
	}
	for e := r; e > a; e-- {
		if isUrilast(s[e-1]) || (e < len(s) && s[e] == '>') {
			if k(e) {
				return true
			}
		}
	}
	return false
}

// uriAt matches the pattern at i (without the start-string prefix).
func (sc inlineScanner) uriAt(s string, i int) (uriMatch, bool) {
	c := s[i]
	if isASCIIAlpha(c) {
		e := i + 1
		for e < len(s) && (isASCIIAlnum(s[e]) || s[e] == '.' || s[e] == '+' || s[e] == '-') {
			e++
		}
		if e < len(s) && s[e] == ':' {
			end := -1
			accept := func(p int) bool {
				if sc.endOK(s, p) {
					end = p
					return true
				}
				return false
			}
			fragment := func(p int) bool {
				if p < len(s) && s[p] == '#' && uriPart(s, p+1, accept) {
					return true
				}
				return accept(p)
			}
			if uriPart(s, e+1, func(p int) bool {
				if p < len(s) && s[p] == '?' && uriPart(s, p+1, fragment) {
					return true
				}
				return fragment(p)
			}) {
				return uriMatch{start: i, end: end, schemeEnd: e}, true
			}
		}
	}
	found := -1
	emailEnds(s, i, func(p int) bool {
		if sc.endOK(s, p) {
			found = p
			return true
		}
		return false
	})
	if found >= 0 {
		return uriMatch{start: i, end: found, schemeEnd: i, email: true}, true
	}
	return uriMatch{}, false
}

// emailEnds matches EMAILC+(?:\.EMAILC+)*(?<!\x00)@EMAILC+(?:\.EMAILC*)*URI_END
// at i and calls k for every possible end, in backtracking order.
func emailEnds(s string, i int, k func(int) bool) bool {
	p := i
	for {
		q := p
		for q < len(s) && isEmailc(s[q]) {
			q++
		}
		if q == p {
			return false
		}
		p = q
		if p < len(s) && s[p] == '.' {
			p++
			continue
		}
		break
	}
	if p >= len(s) || s[p] != '@' || s[p-1] == 0 {
		return false
	}
	h0 := p + 1
	if h0 >= len(s) || !isEmailc(s[h0]) {
		return false
	}
	r := h0
	for r < len(s) && (isEmailc(s[r]) || s[r] == '.') {
		r++
	}
	for e := r; e > h0; e-- {
		if e >= len(s) {
			continue
		}
		if isUrilast(s[e]) || (isUric(s[e]) && e+1 < len(s) && s[e+1] == '>') {
			if k(e + 1) {
				return true
			}
		}
	}
	return false
}

// searchURI implements patterns.uri.search(s).
func (sc inlineScanner) searchURI(s string) (uriMatch, bool) {
	if strings.IndexByte(s, ':') < 0 && strings.IndexByte(s, '@') < 0 {
		return uriMatch{}, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf || !(isASCIIAlpha(c) || isEmailc(c)) {
			continue
		}
		if !sc.startOK(s, i, 0) {
			continue
		}
		if m, ok := sc.uriAt(s, i); ok {
			return m, true
		}
	}
	return uriMatch{}, false
}

// matchURI implements patterns.uri.match(s) != None.
func (sc inlineScanner) matchURI(s string) bool {
	if s == "" || s[0] >= utf8.RuneSelf {
		return false
	}
	_, ok := sc.uriAt(s, 0)
	return ok
}

// isEmail implements patterns.email.match(s) != None (the e-mail pattern
// followed by "$").
func isEmail(s string) bool {
	return emailEnds(s, 0, func(p int) bool { return atDollar(s, p) })
}

// searchPEP implements patterns.pep.search(s):
//
//	PREFIX ( (pep-(?P<pepnum1>\d+)(.txt)?) | (PEP\s+(?P<pepnum2>\d+)) ) SUFFIX
//
// num is the digit string.
func (sc inlineScanner) searchPEP(s string) (start, end int, num string, ok bool) {
	digitsEnd := func(a int) int {
		e := a
		for e < len(s) {
			r, w := utf8.DecodeRuneInString(s[e:])
			if !isPyDigit(r) {
				break
			}
			e += w
		}
		return e
	}
	prevDigitEnd := func(a, e int) int {
		_, w := utf8.DecodeLastRuneInString(s[:e])
		if e-w <= a {
			return -1
		}
		return e - w
	}
	for i := 0; i < len(s); i++ {
		if !(s[i] == 'p' || s[i] == 'P') || !sc.startOK(s, i, 0) {
			continue
		}
		if strings.HasPrefix(s[i:], "pep-") {
			a := i + 4
			for e := digitsEnd(a); e > a; e = prevDigitEnd(a, e) {
				// (.txt)? : any character followed by "txt"
				if e < len(s) && s[e] != '\n' {
					_, w := utf8.DecodeRuneInString(s[e:])
					if strings.HasPrefix(s[e+w:], "txt") && sc.endOK(s, e+w+3) {
						return i, e + w + 3, s[a:e], true
					}
				}
				if sc.endOK(s, e) {
					return i, e, s[a:e], true
				}
			}
		}
		if strings.HasPrefix(s[i:], "PEP") {
			a := i + 3
			for a < len(s) {
				r, w := utf8.DecodeRuneInString(s[a:])
				if !isPySpace(r) {
					break
				}
				a += w
			}
			if a == i+3 {
				continue
			}
			for e := digitsEnd(a); e > a; e = prevDigitEnd(a, e) {
				if sc.endOK(s, e) {
					return i, e, s[a:e], true
				}
			}
		}
	}
	return 0, 0, "", false
}

// searchRFC implements patterns.rfc.search(s):
//
//	PREFIX (RFC(-|\s+)?(?P<rfcnum>\d+)) SUFFIX
func (sc inlineScanner) searchRFC(s string) (start, end int, num string, ok bool) {
	for i := 0; i < len(s); i++ {
		if s[i] != 'R' || !strings.HasPrefix(s[i:], "RFC") || !sc.startOK(s, i, 0) {
			continue
		}
		a := i + 3
		starts := []int{}
		if a < len(s) && s[a] == '-' {
			starts = append(starts, a+1)
		} else {
			b := a
			for b < len(s) {
				r, w := utf8.DecodeRuneInString(s[b:])
				if !isPySpace(r) {
					break
				}
				b += w
			}
			if b > a {
				starts = append(starts, b)
			}
		}
		starts = append(starts, a)
		for _, d := range starts {
			e := d
			for e < len(s) {
				r, w := utf8.DecodeRuneInString(s[e:])
				if !isPyDigit(r) {
					break
				}
				e += w
			}
			for e > d {
				if sc.endOK(s, e) {
					return i, e, s[d:e], true
				}
				_, w := utf8.DecodeLastRuneInString(s[:e])
				e -= w
			}
		}
	}
	return 0, 0, "", false
}
