package rstgo

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Helpers reproducing the behaviour of Python string methods where it
// differs from the Go standard library.

func pyStrip(s string) string  { return strings.TrimFunc(s, isPySpace) }
func pyLstrip(s string) string { return strings.TrimLeftFunc(s, isPySpace) }
func pyRstrip(s string) string { return strings.TrimRightFunc(s, isPySpace) }

// pySplit is str.split() (whitespace separated, no empty fields).
func pySplit(s string) []string { return strings.FieldsFunc(s, isPySpace) }

// pySplitN is str.split(None, n).
func pySplitN(s string, n int) []string {
	var out []string
	s = pyLstrip(s)
	for len(out) < n && s != "" {
		i := strings.IndexFunc(s, isPySpace)
		if i < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:i])
		s = pyLstrip(s[i:])
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

func pyLower(s string) string { return strings.ToLower(s) }

func isBlank(s string) bool { return pyStrip(s) == "" }

// pySplitlines is str.splitlines(keepends).
func pySplitlines(s string, keepends bool) []string {
	out := make([]string, 0, strings.Count(s, "\n")+1)
	start := 0
	i := 0
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		brk := 0
		switch r {
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			brk = w
		case '\r':
			brk = 1
			if i+1 < len(s) && s[i+1] == '\n' {
				brk = 2
			}
		}
		if brk == 0 {
			i += w
			continue
		}
		if keepends {
			out = append(out, s[start:i+brk])
		} else {
			out = append(out, s[start:i])
		}
		i += brk
		start = i
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// pyExpandtabs is str.expandtabs(tabsize).
func pyExpandtabs(s string, tabsize int) string {
	if strings.IndexByte(s, '\t') < 0 {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			if tabsize > 0 {
				n := tabsize - col%tabsize
				b.WriteString(strings.Repeat(" ", n))
				col += n
			}
		case '\n', '\r':
			b.WriteRune(r)
			col = 0
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func inRanges(tbl [][2]rune, r rune) bool {
	i := sort.Search(len(tbl), func(i int) bool { return tbl[i][1] >= r })
	return i < len(tbl) && tbl[i][0] <= r
}

func isCombining(r rune) bool { return r >= 0x300 && inRanges(combiningRanges, r) }
func isWide(r rune) bool      { return r >= 0x1100 && inRanges(wideRanges, r) }

// columnWidth mirrors utils.column_width.
func columnWidth(s string) int {
	w := 0
	for _, r := range s {
		if isCombining(r) {
			continue // counted as 1, then subtracted
		}
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func stripCombiningChars(s string) string {
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
	return strings.Map(func(r rune) rune {
		if isCombining(r) {
			return -1
		}
		return r
	}, s)
}

// pyRepr approximates Python's repr() for the value types stored in
// pending details.
func pyRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return pyStrRepr(x)
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "True"
		}
		return "False"
	case []string:
		parts := make([]string, len(x))
		for i, s := range x {
			parts[i] = pyStrRepr(s)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = pyStrRepr(k) + ": " + pyRepr(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case pyReprer:
		return x.pyRepr()
	}
	return fmt.Sprint(v)
}

type pyReprer interface{ pyRepr() string }

func pyStrRepr(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = '"'
	}
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= 0x80 && !unicode.IsPrint(r):
			if r < 0x100 {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else if r < 0x10000 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(q)
	return b.String()
}

// pyIndex returns the element index following Python's negative indexing,
// or -1 if out of range.
func pyIndex(i, n int) int {
	if i < 0 {
		i += n
	}
	if i < 0 || i >= n {
		return -1
	}
	return i
}

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Character classes with the semantics of Python's str methods and "re"
// module (\s, \w, \d on str patterns).

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
