package rstgo

// Differential tests: every hand-written scanner (scan*.go) is compared
// with the original Docutils regular expression, run by the backtracking
// regex engine in pyre_test.go, over the lines and paragraphs of the test
// corpus plus randomly damaged variants.

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const oName = `(?:(?!_)\w)+(?:[-._+:](?:(?!_)\w)+)*`

const (
	oNonalnum = "[!-/:-@[-`{-~]"
	oEnum     = `([0-9]+|[a-z]|[A-Z]|[ivxlcdm]+|[IVXLCDM]+|#)`
	oOptarg   = `([a-zA-Z][a-zA-Z0-9_-]*|<[^<>]+>)`
	oOption   = `((-|\+)[a-zA-Z0-9]( ?` + oOptarg + `)?|(--|/)[a-zA-Z0-9][a-zA-Z0-9_-]*([ =]` + oOptarg + `)?)`
	oField    = ":(?![: ])([^:\\\\]|\\\\.|:(?!([ `]|$)))*(?<! ):( +|$)"
	oUric     = `[-_.!~*'()[\];/:@&=+$,%a-zA-Z0-9\x00]`
	oUriEnd   = `(?:[_~*/=+a-zA-Z0-9]|` + oUric + `(?=[>]))`
	oEmailc   = "[-_!~*'{|}/#?^`&=+$%a-zA-Z0-9\\x00]"
	oEmail    = oEmailc + `+(?:\.` + oEmailc + `+)*(?<!\x00)@` + oEmailc + `+(?:\.` + oEmailc + `*)*` + oUriEnd
)

type oracleInline struct {
	initial, emphasis, strong, interpreted, embedded, literal, target, subst, email, uri, pep, rfc *pyRegexp
}

func buildOracleInline(charLevel bool) *oracleInline {
	prefix := `(^|(?<=\s|[` + punct_openers + punct_delimiters + `]))`
	suffix := `($|(?=\s|[\x00` + punct_closing_delimiters + punct_delimiters + punct_closers + `]))`
	if charLevel {
		prefix, suffix = `(^|(?<!\x00))`, ``
	}
	o := &oracleInline{}
	o.initial = pyCompile(prefix + `(?P<initial_inline>` +
		`(?P<start>\*\*|\*(?!\*)|` + "``" + `|_` + "`" + `|\|(?!\|))(?!\s)` +
		`|(?P<whole>(?P<refname>` + oName + `)(?P<refend>__?)` +
		`|\[(?P<footnotelabel>[0-9]+|\#(` + oName + `)?|\*|(?P<citationlabel>` + oName + `))(?P<fnend>\]_))` + suffix +
		`|(?P<role>(:` + oName + `:)?)(?P<backquote>` + "`(?!`)" + `)(?!\s))`)
	o.emphasis = pyCompile(`(?<![\s\x00])(\*)` + suffix)
	o.strong = pyCompile(`(?<![\s\x00])(\*\*)` + suffix)
	o.interpreted = pyCompile(`(?<!(?<!\x00)[\s\x00])(` + "`" + `(?P<suffix>(?P<role>:` + oName + `:)?(?P<refend>__?)?))` + suffix)
	o.embedded = pyCompile(`((?:[ \n]+|^)<(?!\s)(([^<>]|\x00[<>])+)(?<![\s\x00])>)$`)
	o.literal = pyCompile(`(?<!\s)(` + "``" + `)` + suffix)
	o.target = pyCompile(`(?<![\s\x00])(` + "`" + `)` + suffix)
	o.subst = pyCompile(`(?<![\s\x00])(\|_{0,2})` + suffix)
	o.email = pyCompile(oEmail + `$`)
	o.uri = pyCompile(prefix + `(?P<whole>(?P<absolute>(?P<scheme>[a-zA-Z][a-zA-Z0-9.+-]*):(((//?)?` + oUric + `*` + oUriEnd +
		`)(\?` + oUric + `*` + oUriEnd + `)?(\#` + oUric + `*` + oUriEnd + `)?))|(?P<email>` + oEmail + `))` + suffix)
	o.pep = pyCompile(prefix + `((pep-(?P<pepnum1>\d+)(.txt)?)|(PEP\s+(?P<pepnum2>\d+)))` + suffix)
	o.rfc = pyCompile(prefix + `(RFC(-|\s+)?(?P<rfcnum>\d+))` + suffix)
	return o
}

var (
	oracleSamplesOnce sync.Once
	oracleLines       []string
	oracleParas       []string
)

// oracleSamples returns corpus lines and (escape2null-ed) paragraphs, with
// damaged variants.
func oracleSamples(t testing.TB) ([]string, []string) {
	oracleSamplesOnce.Do(func() {
		var docs []string
		docs = append(docs, corpusInputs(t)...)
		filepath.Walk("testdata/docutils", func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(p, ".rst") {
				if data, err := os.ReadFile(p); err == nil {
					docs = append(docs, string(data))
				}
			}
			return nil
		})
		rng := rand.New(rand.NewSource(7))
		junk := []string{"*", "**", "`", "``", "_", "__", "|", "\\", ":", "::", "..", " ", "  ", "-", "--", "=", "+",
			"[", "]", "]_", "<", ">", "\x00", "é", "日", ".. ", "#", "(", ")", "́", "@", ".", ",", "/", "?", "\n",
			"http://", "a_", "`_", "`__", ":r:", "[1]_", "[#]_", "|x|", "-o", "--opt", "<x>", "$", "'", "\"", "«", "»", "—", "\t"}
		mutate := func(s string) string {
			if s == "" {
				return junk[rng.Intn(len(junk))]
			}
			pos := rng.Intn(len(s) + 1)
			for pos < len(s) && s[pos]&0xc0 == 0x80 {
				pos++
			}
			j := junk[rng.Intn(len(junk))]
			if rng.Intn(3) == 0 {
				end := pos + rng.Intn(3)
				for end < len(s) && s[end]&0xc0 == 0x80 {
					end++
				}
				if end > len(s) {
					end = len(s)
				}
				return s[:pos] + j + s[end:]
			}
			return s[:pos] + j + s[pos:]
		}
		seenL := map[string]bool{}
		seenP := map[string]bool{}
		addLine := func(l string) {
			if !seenL[l] {
				seenL[l] = true
				oracleLines = append(oracleLines, l)
			}
		}
		addPara := func(p string) {
			if !seenP[p] {
				seenP[p] = true
				oracleParas = append(oracleParas, p)
			}
		}
		for _, doc := range docs {
			lines := string2lines(doc, 8, true)
			for _, l := range lines {
				addLine(l)
				addLine(strings.TrimLeft(l, " "))
				addLine(mutate(l))
				addLine(mutate(strings.TrimLeft(l, " ")))
				addLine(escape2null(l))
			}
			for _, para := range strings.Split(strings.Join(lines, "\n"), "\n\n") {
				p := escape2null(strings.TrimSpace(para))
				if len(p) > 600 {
					p = p[:600]
					for len(p) > 0 && p[len(p)-1]&0x80 != 0 {
						p = p[:len(p)-1]
					}
				}
				addPara(p)
				addPara(mutate(p))
				addPara(escape2null(mutate(strings.TrimSpace(para))))
			}
		}
		for _, l := range oracleLines[:len(oracleLines)/8] {
			addPara(l)
		}
	})
	return oracleLines, oracleParas
}

type diffCounter struct {
	t       *testing.T
	name    string
	n, hits int
	errs    int
}

func (d *diffCounter) fail(input string, format string, args ...any) {
	d.errs++
	if d.errs <= 5 {
		d.t.Errorf("%s: %q: %s", d.name, input, fmt.Sprintf(format, args...))
	}
}

func (d *diffCounter) done() {
	d.t.Logf("%-28s %7d inputs, %6d matches", d.name, d.n, d.hits)
	if d.errs > 5 {
		d.t.Errorf("%s: %d mismatches in total", d.name, d.errs)
	}
}

func TestLineScannersAgainstRegex(t *testing.T) {
	lines, _ := oracleSamples(t)
	type entry struct {
		name string
		re   *pyRegexp
		scan lineScanner
		g1   bool
	}
	entries := []entry{
		{"blank", pyCompile(` *$`), scanBlank, false},
		{"indent", pyCompile(` +`), scanIndent, false},
		{"bullet", pyCompile("[-+*•‣⁃]( +|$)"), scanBullet, false},
		{"field_marker", pyCompile(oField), scanFieldMarker, false},
		{"option_marker", pyCompile(oOption + `(, ` + oOption + `)*(  +| ?$)`), scanOptionMarker, false},
		{"doctest", pyCompile(`>>>( +|$)`), scanDoctest, true},
		{"line_block", pyCompile(`\|( +|$)`), scanLineBlock, true},
		{"grid_table_top", pyCompile(`\+-[-+]+-\+ *$`), scanGridTableTop, false},
		{"grid_head_body_sep", pyCompile(`\+=[=+]+=\+ *$`), scanGridHeadBodySep, false},
		{"simple_table_top", pyCompile(`=+( +=+)+ *$`), scanSimpleTableTop, false},
		{"explicit_markup", pyCompile(`\.\.( +|$)`), scanExplicitMarkup, true},
		{"anonymous", pyCompile(`__( +|$)`), scanAnonymous, true},
		{"line", pyCompile(`(` + oNonalnum + `)\1* *$`), scanLine, true},
		{"text", pyCompile(``), scanText, false},
		{"embedded_directive", pyCompile(`(` + oName + `)::( +|$)`), scanEmbeddedDirective, true},
		{"initial_quoted", pyCompile(`(` + oNonalnum + `)`), scanInitialQuoted, true},
		{"footnote", pyCompile(`\.\.[ ]+\[([0-9]+|\#|\#` + oName + `|\*)\]([ ]+|$)`), scanFootnote, true},
		{"citation", pyCompile(`\.\.[ ]+\[(` + oName + `)\]([ ]+|$)`), scanCitation, true},
		{"hyperlink_target", pyCompile(`\.\.[ ]+_(?![ ]|$)`), scanHyperlinkTarget, false},
		{"substitution_def", pyCompile(`\.\.[ ]+\|(?![ ]|$)`), scanSubstitutionDef, false},
		{"directive", pyCompile(`\.\.[ ]+(` + oName + `)[ ]?::([ ]+|$)`), scanDirective, true},
	}
	for _, e := range entries {
		d := &diffCounter{t: t, name: e.name}
		for _, l := range lines {
			d.n++
			want := e.re.Match(l)
			got, ok := e.scan(l)
			if (want != nil) != ok {
				d.fail(l, "match %v, want %v", ok, want != nil)
				continue
			}
			if !ok {
				continue
			}
			d.hits++
			if got.end != want.End(0) {
				d.fail(l, "end %d, want %d", got.end, want.End(0))
			} else if e.g1 && got.Group(1) != want.Group(1) {
				d.fail(l, "group 1 %q, want %q", got.Group(1), want.Group(1))
			}
		}
		d.done()
	}

	// enumerator (named groups)
	enumRe := pyCompile(`((?P<parens>\(` + oEnum + `\))|(?P<rparen>` + oEnum + `\))|(?P<period>` + oEnum + `\.))( +|$)`)
	d := &diffCounter{t: t, name: "enumerator"}
	for _, l := range lines {
		d.n++
		want := enumRe.Match(l)
		got, ok := scanEnumerator(l)
		if (want != nil) != ok {
			d.fail(l, "match %v, want %v", ok, want != nil)
			continue
		}
		if !ok {
			continue
		}
		d.hits++
		if got.end != want.End(0) || want.Named(got.format) == "" || want.Named(got.format) != got.Group(1) {
			d.fail(l, "end %d format %s group %q", got.end, got.format, got.Group(1))
		}
	}
	d.done()

	// boolean line predicates
	type pred struct {
		name string
		re   *pyRegexp
		fn   func(string) bool
		srch bool
	}
	for _, p := range []pred{
		{"simple_table_border", pyCompile(`=+[ =]*$`), isSimpleTableBorder, false},
		{"simple_head_body_sep", pyCompile(`=[ =]*$`), isSimpleTableBorder, false},
		{"simple_span", pyCompile(`-[ -]*$`), isSimpleSpanLine, false},
		{"paragraph_literal", pyCompile(`(?<!\\)(\\\\)*::$`), endsWithLiteralMarker, true},
		{"num_only", pyCompile(`^[0-9.]+$`), isNumOnly, false},
		{"word_wrap_point", pyCompile(`.+\W\W.+|[-?].+`), hasWordWrapPoint, true},
		{"enum arabic", pyCompile(`[0-9]+$`), func(s string) bool { return enumSequenceMatches("arabic", s) }, false},
		{"enum loweralpha", pyCompile(`[a-z]$`), func(s string) bool { return enumSequenceMatches("loweralpha", s) }, false},
		{"enum upperalpha", pyCompile(`[A-Z]$`), func(s string) bool { return enumSequenceMatches("upperalpha", s) }, false},
		{"enum lowerroman", pyCompile(`[ivxlcdm]+$`), func(s string) bool { return enumSequenceMatches("lowerroman", s) }, false},
		{"enum upperroman", pyCompile(`[IVXLCDM]+$`), func(s string) bool { return enumSequenceMatches("upperroman", s) }, false},
	} {
		d := &diffCounter{t: t, name: p.name}
		for _, l := range lines {
			inputs := []string{l}
			if f := strings.Fields(l); len(f) > 0 {
				inputs = append(inputs, f[0], f[len(f)-1])
			}
			for _, in := range inputs {
				d.n++
				var want bool
				if p.srch {
					want = p.re.Search(in) != nil
				} else {
					want = p.re.Match(in) != nil
				}
				if want {
					d.hits++
				}
				if got := p.fn(in); got != want {
					d.fail(in, "got %v, want %v", got, want)
				}
			}
		}
		d.done()
	}
}

func TestExplicitScannersAgainstRegex(t *testing.T) {
	lines, paras := oracleSamples(t)
	targetRe := pyCompile(`(_|(?!_)(?P<quote>` + "`" + `?)(?![ ` + "`" + `])(?P<name>.+?)(?<![\s\x00])(?P=quote))(?<!(?<!\x00):)(?<![\s\x00])[ ]?:([ ]+|$)`)
	referenceRe := pyCompile(`((?P<simple>` + oName + `)_|` + "`" + `(?![ ])(?P<phrase>.+?)(?<![\s\x00])` + "`" + `_)$`)
	substRe := pyCompile(`((?![ ])(?P<name>.+?)(?<![\s\x00])\|)([ ]+|$)`)
	attributionRe := pyCompile("(---?(?!-)|—) *(?=[^ \\n])")
	optionSplitRe := pyCompile(`, (?![^<]*>)`)
	classifierRe := pyCompile(` +: +`)
	roleArgRe := pyCompile(`(` + oName + `)\s*(\(\s*(` + oName + `)\s*\)\s*)?$`)
	unicodeCommentRe := pyCompile(`( |\n|^)\.\. `)
	unicodeRe := pyCompile(`(?:0x|x|\\x|U\+?|\\u)([0-9a-f]+)$|&#x([0-9a-f]+);$`, reIgnoreCase)
	measureRe := pyCompile(`(-?[0-9.]+) *([a-zA-Zµ]*|%?)`)
	numUnitRe := pyCompile(`([0-9.]+)(\S*)$`)
	wordsRe := pyCompile(`[^ \n]+| +|\n`)
	dashRe := pyCompile(`-(?=-)`)
	authorRe := pyCompile(`(?<!\x00);`)

	var inputs []string
	for _, l := range lines {
		inputs = append(inputs, l)
		// the texts these patterns are applied to follow explicit markup
		for _, pfx := range []string{".. _", ".. |", "__ ", ".. "} {
			if strings.HasPrefix(l, pfx) {
				inputs = append(inputs, l[len(pfx):], escape2null(l[len(pfx):]))
			}
		}
		if i := strings.Index(l, ":: "); i >= 0 {
			inputs = append(inputs, l[i+3:])
		}
	}
	inputs = append(inputs, paras[:len(paras)/4]...)

	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	counters := map[string]*diffCounter{}
	c := func(name string) *diffCounter {
		if counters[name] == nil {
			counters[name] = &diffCounter{t: t, name: name}
		}
		counters[name].n++
		return counters[name]
	}
	for _, in := range inputs {
		// target
		d := c("explicit target")
		want := targetRe.Match(in)
		end, name, ok := scanExplicitTarget(in)
		if (want != nil) != ok {
			d.fail(in, "match %v, want %v", ok, want != nil)
		} else if ok {
			d.hits++
			if end != want.End(0) || name != want.Named("name") {
				d.fail(in, "end %d name %q, want %d %q", end, name, want.End(0), want.Named("name"))
			}
		}
		// reference
		d = c("explicit reference")
		want = referenceRe.Match(in)
		ref, ok := scanExplicitReference(in)
		if (want != nil) != ok {
			d.fail(in, "match %v, want %v", ok, want != nil)
		} else if ok {
			d.hits++
			w := want.Named("simple")
			if w == "" {
				w = want.Named("phrase")
			}
			if ref != w {
				d.fail(in, "ref %q, want %q", ref, w)
			}
		}
		// substitution
		d = c("explicit substitution")
		want = substRe.Match(in)
		end, name, ok = scanExplicitSubstitution(in)
		if (want != nil) != ok {
			d.fail(in, "match %v, want %v", ok, want != nil)
		} else if ok {
			d.hits++
			if end != want.End(0) || name != want.Named("name") {
				d.fail(in, "end %d name %q, want %d %q", end, name, want.End(0), want.Named("name"))
			}
		}
		// attribution
		d = c("attribution")
		want = attributionRe.Match(in)
		ae := scanAttribution(in)
		if (want != nil) != (ae >= 0) || (want != nil && want.End(0) != ae) {
			d.fail(in, "end %d", ae)
		} else if want != nil {
			d.hits++
		}
		// splits
		d = c("option split")
		if got, w := splitOptions(in), optionSplitRe.Split(in); !eq(got, w) {
			d.fail(in, "%q, want %q", got, w)
		}
		d = c("classifier split")
		if got, w := splitClassifiers(in), classifierRe.Split(in); !eq(got, w) {
			d.fail(in, "%q, want %q", got, w)
		}
		d = c("author split")
		if got, w := splitUnescaped(in, ';'), authorRe.Split(in); !eq(got, w) {
			d.fail(in, "%q, want %q", got, w)
		}
		d = c("unicode comment split")
		if got, w := beforeUnicodeComment(in), unicodeCommentRe.Split(in)[0]; got != w {
			d.fail(in, "%q, want %q", got, w)
		}
		d = c("words and spaces")
		if got, w := wordsAndSpacesTokens(in), findAllOracle(wordsRe, in); !eq(got, w) {
			d.fail(in, "%q, want %q", got, w)
		}
		d = c("comment dashes")
		if got, w := spaceDashes(in), dashRe.ReplaceAll(in, "- "); got != w {
			d.fail(in, "%q, want %q", got, w)
		}
		// role arguments
		d = c("role arguments")
		want = roleArgRe.Match(in)
		rn, rb, ok := scanRoleArguments(in)
		if (want != nil) != ok || (ok && (rn != want.Group(1) || rb != want.Group(3))) {
			d.fail(in, "%v %q %q", ok, rn, rb)
		} else if ok {
			d.hits++
		}
		// words of the line: unicode codes, measures
		for _, w := range strings.Fields(in) {
			d = c("unicode code")
			want = unicodeRe.Match(w)
			h, ok := scanUnicodeCode(w)
			if (want != nil) != ok {
				d.fail(w, "match %v", ok)
			} else if ok {
				d.hits++
				wh := want.Group(1)
				if wh == "" {
					wh = want.Group(2)
				}
				if h != wh {
					d.fail(w, "%q, want %q", h, wh)
				}
			}
			d = c("measure")
			want = measureRe.FullMatch(w)
			num, unit, ok := scanMeasure(w)
			if (want != nil) != ok || (ok && (num != want.Group(1) || unit != want.Group(2))) {
				d.fail(w, "%v %q %q", ok, num, unit)
			} else if ok {
				d.hits++
			}
			d = c("num unit")
			want = numUnitRe.Match(w)
			num, unit, ok = scanNumUnit(w)
			if (want != nil) != ok || (ok && (num != want.Group(1) || unit != want.Group(2))) {
				d.fail(w, "%v %q %q", ok, num, unit)
			} else if ok {
				d.hits++
			}
		}
	}
	for _, extra := range []string{"0x41", "x41", `\x41`, "U+41", "u41", `A`, "&#x41;", "0xZ", "U+", "&#x41", "1.5em", "-3 px", "50%", "5 %", "1e3", ".", "1..2µm", "10 cm x"} {
		h, ok := scanUnicodeCode(extra)
		want := unicodeRe.Match(extra)
		wh := ""
		if want != nil {
			wh = want.Group(1) + want.Group(2)
		}
		if (want != nil) != ok || h != wh {
			t.Errorf("unicode code %q: %v %q", extra, ok, h)
		}
		num, unit, ok := scanMeasure(extra)
		want = measureRe.FullMatch(extra)
		if (want != nil) != ok || (ok && (num != want.Group(1) || unit != want.Group(2))) {
			t.Errorf("measure %q: %v %q %q", extra, ok, num, unit)
		}
	}
	for _, d := range counters {
		d.done()
	}
}

func findAllOracle(re *pyRegexp, s string) []string {
	var out []string
	pos := 0
	for pos <= len(s) {
		m := re.SearchFrom(s, pos)
		if m == nil {
			break
		}
		out = append(out, m.Group(0))
		if m.End(0) == m.Start(0) {
			pos = m.End(0) + 1
		} else {
			pos = m.End(0)
		}
	}
	return out
}

func TestInlineScannersAgainstRegex(t *testing.T) {
	_, paras := oracleSamples(t)
	for _, charLevel := range []bool{false, true} {
		o := buildOracleInline(charLevel)
		sc := inlineScanner{charLevel: charLevel}
		mode := "word-level "
		if charLevel {
			mode = "char-level "
			paras = paras[:len(paras)/4]
		}
		counters := []*diffCounter{}
		nc := func(name string) *diffCounter {
			d := &diffCounter{t: t, name: mode + name}
			counters = append(counters, d)
			return d
		}
		dInit, dEmph, dStrong, dLit, dTarget := nc("initial"), nc("emphasis end"), nc("strong end"), nc("literal end"), nc("target end")
		dSubst, dInterp, dEmb, dURI, dURIm, dEmail, dPEP, dRFC := nc("substitution end"), nc("interpreted end"), nc("embedded link"),
			nc("uri search"), nc("uri match"), nc("email"), nc("pep"), nc("rfc")
		endCheck := func(d *diffCounter, re *pyRegexp, s string, from int, got int) {
			d.n++
			want := -1
			if m := re.Search(s[from:]); m != nil && m.Start(1) > 0 {
				want = from + m.Start(1)
				d.hits++
			}
			if got != want {
				d.fail(s[from:], "end-string at %d, want %d", got-from, want-from)
			}
		}
		for _, p := range paras {
			// initial: walk through all matches like Inliner.parse
			rest := p
			for guard := 0; rest != "" && guard < 200; guard++ {
				dInit.n++
				want := o.initial.Search(rest)
				got, ok := sc.searchInitial(rest, 0)
				if (want != nil) != ok {
					dInit.fail(rest, "match %v, want %v", ok, want != nil)
					break
				}
				if !ok {
					break
				}
				dInit.hits++
				key := want.Named("start")
				for _, n := range []string{"backquote", "refend", "fnend"} {
					if key == "" {
						key = want.Named(n)
					}
				}
				bad := got.start != want.Start(0) || got.end != want.End(0) || got.key != key
				switch key {
				case "*", "**", "``", "_`", "|":
					bad = bad || got.markS != want.StartNamed("start") || got.markE != want.EndNamed("start")
				case "`":
					bad = bad || got.markS != want.StartNamed("backquote") || got.markE != want.EndNamed("backquote") ||
						got.roleS != want.StartNamed("role") || got.roleE != want.EndNamed("role")
				case "_", "__":
					bad = bad || rest[got.nameS:got.nameE] != want.Named("refname")
				case "]_":
					bad = bad || rest[got.nameS:got.nameE] != want.Named("footnotelabel") ||
						got.citation != (want.Named("citationlabel") != "")
				}
				if bad {
					dInit.fail(rest, "got %+v, want [%d,%d] key %q", got, want.Start(0), want.End(0), key)
					break
				}
				// end-string searches from the end of this match
				from := got.end
				endCheck(dEmph, o.emphasis, rest, from, sc.searchEnd(rest, from, "*", false))
				endCheck(dStrong, o.strong, rest, from, sc.searchEnd(rest, from, "**", false))
				endCheck(dLit, o.literal, rest, from, sc.searchEnd(rest, from, "``", true))
				endCheck(dTarget, o.target, rest, from, sc.searchEnd(rest, from, "`", false))
				// substitution
				dSubst.n++
				ss, se := sc.searchSubstitutionEnd(rest, from)
				if m := o.subst.Search(rest[from:]); m != nil && m.Start(1) > 0 {
					dSubst.hits++
					if ss != from+m.Start(1) || se != from+m.End(1) {
						dSubst.fail(rest[from:], "got %d,%d want %d,%d", ss-from, se-from, m.Start(1), m.End(1))
					}
				} else if ss >= 0 {
					dSubst.fail(rest[from:], "got %d,%d want none", ss-from, se-from)
				}
				// interpreted
				dInterp.n++
				ie, iok := sc.searchInterpretedEnd(rest, from)
				if m := o.interpreted.Search(rest[from:]); m != nil && m.Start(1) > 0 {
					dInterp.hits++
					if !iok || ie.start != from+m.Start(1) || ie.end != from+m.End(0) ||
						rest[ie.roleS:ie.roleE] != m.Named("role") {
						dInterp.fail(rest[from:], "got %+v ok=%v want %d,%d role %q", ie, iok, m.Start(1), m.End(0), m.Named("role"))
					} else {
						// embedded link in the phrase text
						text := rest[from:ie.start]
						dEmb.n++
						em := o.embedded.Search(text)
						st, g2s, g2e, eok := searchEmbeddedLink(text)
						if (em != nil) != eok || (eok && (st != em.Start(0) || g2s != em.Start(2) || g2e != em.End(2))) {
							dEmb.fail(text, "got %v %d %d %d", eok, st, g2s, g2e)
						} else if eok {
							dEmb.hits++
							alias := text[g2s:g2e]
							dURIm.n++
							if got, want := sc.matchURI(alias), o.uri.Match(alias) != nil; got != want {
								dURIm.fail(alias, "got %v want %v", got, want)
							} else if got {
								dURIm.hits++
							}
							dEmail.n++
							if got, want := isEmail(alias), o.email.Match(alias) != nil; got != want {
								dEmail.fail(alias, "got %v want %v", got, want)
							} else if got {
								dEmail.hits++
							}
						}
					}
				} else if iok {
					dInterp.fail(rest[from:], "got %+v want none", ie)
				}
				rest = rest[got.end:]
			}
			// embedded link / email on the raw paragraph and its tail pieces
			for _, text := range []string{p, strings.TrimRight(p, ".,;:_` \n")} {
				dEmb.n++
				em := o.embedded.Search(text)
				st, g2s, g2e, eok := searchEmbeddedLink(text)
				if (em != nil) != eok || (eok && (st != em.Start(0) || g2s != em.Start(2) || g2e != em.End(2))) {
					dEmb.fail(text, "got %v %d %d %d", eok, st, g2s, g2e)
				} else if eok {
					dEmb.hits++
				}
			}
			// implicit patterns: all matches
			rest = p
			for guard := 0; rest != "" && guard < 100; guard++ {
				dURI.n++
				want := o.uri.Search(rest)
				got, ok := sc.searchURI(rest)
				if (want != nil) != ok {
					dURI.fail(rest, "match %v, want %v", ok, want != nil)
					break
				}
				if !ok {
					break
				}
				dURI.hits++
				if got.start != want.StartNamed("whole") || got.end != want.End(0) ||
					rest[got.start:got.schemeEnd] != want.Named("scheme") || got.email != (want.Named("email") != "") {
					dURI.fail(rest, "got %+v, want [%d,%d] scheme %q", got, want.Start(0), want.End(0), want.Named("scheme"))
					break
				}
				word := rest[got.start:got.end]
				dEmail.n++
				if g, w := isEmail(word), o.email.Match(word) != nil; g != w {
					dEmail.fail(word, "got %v want %v", g, w)
				} else if g {
					dEmail.hits++
				}
				dURIm.n++
				if g, w := sc.matchURI(word), o.uri.Match(word) != nil; g != w {
					dURIm.fail(word, "got %v want %v", g, w)
				} else if g {
					dURIm.hits++
				}
				rest = rest[got.end:]
			}
			dPEP.n++
			ps, pe, pn, pok := sc.searchPEP(p)
			if m := o.pep.Search(p); (m != nil) != pok {
				dPEP.fail(p, "match %v", pok)
			} else if pok {
				dPEP.hits++
				wn := m.Named("pepnum1")
				if wn == "" {
					wn = m.Named("pepnum2")
				}
				if ps != m.Start(2) && ps != m.Start(0) || pe != m.End(0) || pn != wn {
					dPEP.fail(p, "got %d %d %q", ps, pe, pn)
				}
			}
			dRFC.n++
			rs, re, rn, rok := sc.searchRFC(p)
			if m := o.rfc.Search(p); (m != nil) != rok {
				dRFC.fail(p, "match %v", rok)
			} else if rok {
				dRFC.hits++
				if re != m.End(0) || rn != m.Named("rfcnum") || rs > m.End(0) {
					dRFC.fail(p, "got %d %d %q", rs, re, rn)
				}
			}
		}
		for _, d := range counters {
			d.done()
		}
	}
}

// TestMiscScannersAgainstRegex covers the transform and math scanners.
func TestMiscScannersAgainstRegex(t *testing.T) {
	lines, paras := oracleSamples(t)
	rcs := []struct {
		re   *pyRegexp
		repl func(m *pyMatch) string
	}{
		{pyCompile(`\$Date: (\d\d\d\d)[-/](\d\d)[-/](\d\d)[ T][\d:]+[^$]* \$`, reIgnoreCase),
			func(m *pyMatch) string { return m.Group(1) + "-" + m.Group(2) + "-" + m.Group(3) }},
		{pyCompile(`\$RCSfile: (.+),v \$`, reIgnoreCase), func(m *pyMatch) string { return m.Group(1) }},
		{pyCompile(`\$[a-zA-Z]+: (.+) \$`), func(m *pyMatch) string { return m.Group(1) }},
	}
	texCmdnameRe := pyCompile(`([a-zA-Z]+)[ \n]*(.*)`, reDotAll)
	texNumberRe := pyCompile(`([0-9.,]*[0-9]+)(.*)`, reDotAll)
	texCmdTokenRe := pyCompile(`(\\[a-zA-Z]+)\s*`, reDotAll)
	texOptargRe := pyCompile(`\s*\[((\\]|[^\[\]]|\\])*)\](.*$)`, reDotAll)
	texTextEdgeRe := pyCompile(`(^[ \n]|[ \n]$)`)

	inputs := append([]string{}, lines...)
	inputs = append(inputs, paras[:len(paras)/6]...)
	inputs = append(inputs,
		"$Date: 2024-01-02 03:04:05 +0000 (Tue, 02 Jan 2024) $", "$date: 2024/01/02T03:04 $ and $Date: 1999-12-31 1 $",
		"$Date: 2024-01-02 $", "$Date: 2024-01-02 x $", "$RCSfile: foo.rst,v $", "$rcsfile: a,v $ b,v $", "$RCSfile: ,v $",
		"$Revision: 1.2 $", "$Id: x y z $ and $Author: me $", "$Id: $", "$: x $", "$Id: a\n b $", "x $Revision: 42 $\n",
		`[a]b`, ` [a\]b]c]d`, `[a[b]`, `[]x`, `[\]`, `[a]\n`, "\\alpha  x", "\\\\", "12,345.6x", "1.", ".5", " a \n", "\n", " ")
	nRCS, nTex := 0, 0
	for _, in := range inputs {
		for n, r := range rcs {
			want, wantOK := in, r.re.Search(in) != nil
			if wantOK {
				want = r.re.ReplaceAllFunc(in, r.repl)
				nRCS++
			}
			got, ok := rcsKeywordSub(n, in)
			if ok != wantOK || got != want {
				t.Errorf("rcs %d %q: got %q %v, want %q %v", n, in, got, ok, want, wantOK)
			}
		}
		for _, s := range []string{in, strings.TrimLeft(in, " .*-`:|_\\"), "\\" + in, "[" + in} {
			if m := texCmdnameRe.Match(s); true {
				a, b, ok := texCmdnameSplit(s)
				if (m != nil) != ok || (ok && (a != m.Group(1) || b != m.Group(2))) {
					t.Errorf("texCmdname %q: %q %q %v", s, a, b, ok)
				}
			}
			if m := texNumberRe.Match(s); true {
				a, b, ok := texNumberSplit(s)
				if (m != nil) != ok || (ok && (a != m.Group(1) || b != m.Group(2))) {
					t.Errorf("texNumber %q: %q %q %v", s, a, b, ok)
				}
			}
			if m := texCmdTokenRe.Match(s); true {
				a, b, ok := texCmdTokenEnd(s)
				if (m != nil) != ok || (ok && (a != m.End(1) || b != m.End(0))) {
					t.Errorf("texCmdToken %q: %d %d %v", s, a, b, ok)
				}
			}
			if m := texOptargRe.Match(s); true {
				a, b, ok := texOptargSplit(s)
				if (m != nil) != ok || (ok && (a != m.Group(1) || b != m.Group(3))) {
					t.Errorf("texOptarg %q: %q %q %v", s, a, b, ok)
				} else if ok {
					nTex++
				}
			}
			if got, want := texTextEdges(s), texTextEdgeRe.ReplaceAll(s, " "); got != want {
				t.Errorf("texTextEdges %q: %q, want %q", s, got, want)
			}
		}
		if t.Failed() {
			break
		}
	}
	t.Logf("%d inputs; %d RCS keyword matches, %d optarg matches", len(inputs), nRCS, nTex)
}
