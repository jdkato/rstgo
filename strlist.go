package rstgo

import "strings"

// lineInfo records where an input line came from: (source, offset).  All
// lines of one file share the source string.
type lineInfo struct {
	source *string // nil: unknown (None)
	offset int
}

func (li lineInfo) src() (string, bool) {
	if li.source == nil {
		return "", false
	}
	return *li.source, true
}

// stringList is a port of docutils.statemachine.StringList (a ViewList of
// lines): slices keep a reference to their parent so that insertions
// (e.g. by the "include" directive) propagate upwards.
type stringList struct {
	data         []string
	items        []lineInfo
	parent       *stringList
	parentOffset int
}

func newStringList(lines []string, source string, hasSource bool) *stringList {
	return newStringListOwned(append([]string{}, lines...), source, hasSource)
}

// newStringListOwned is newStringList taking ownership of lines.
func newStringListOwned(lines []string, source string, hasSource bool) *stringList {
	sl := &stringList{data: lines, items: make([]lineInfo, len(lines))}
	var src *string
	if hasSource {
		src = &source
	}
	for i := range lines {
		sl.items[i] = lineInfo{src, i}
	}
	return sl
}

func (sl *stringList) len() int { return len(sl.data) }

func clampSlice(start, end, n int) (int, int) {
	if start < 0 {
		start += n
		if start < 0 {
			start = 0
		}
	}
	if end < 0 {
		end += n
		if end < 0 {
			end = 0
		}
	}
	if start > n {
		start = n
	}
	if end > n {
		end = n
	}
	if end < start {
		end = start
	}
	return start, end
}

// slice mirrors self[start:end] (the result is a view linked to sl).
func (sl *stringList) slice(start, end int) *stringList {
	pstart := start
	start, end = clampSlice(start, end, len(sl.data))
	return &stringList{
		data: append([]string{}, sl.data[start:end]...),
		// items are never modified in place, so they can be shared; the
		// capacity is clipped so that appending reallocates
		items:        sl.items[start:end:end],
		parent:       sl,
		parentOffset: pstart,
	}
}

func (sl *stringList) from(start int) *stringList { return sl.slice(start, len(sl.data)) }

// view is like from but shares sl's storage instead of copying it, which
// keeps nested parsing of the remaining input from being quadratic. The
// capacity is clipped so that inserting into the view reallocates rather
// than writing into sl. Only use it for lists whose lines are never
// assigned in place (state machine input); use slice or from otherwise.
func (sl *stringList) view(start int) *stringList {
	pstart := start
	start, end := clampSlice(start, len(sl.data), len(sl.data))
	return &stringList{
		data:         sl.data[start:end:end],
		items:        sl.items[start:end:end],
		parent:       sl,
		parentOffset: pstart,
	}
}

// set mirrors self[i] = line (propagating to parents).
func (sl *stringList) set(i int, line string) {
	sl.data[i] = line
	if sl.parent != nil {
		j := i + sl.parentOffset
		if j >= 0 && j < len(sl.parent.data) {
			sl.parent.set(j, line)
		}
	}
}

// deleteRange mirrors del self[start:end].
func (sl *stringList) deleteRange(start, end int) {
	ps, pe := start, end
	start, end = clampSlice(start, end, len(sl.data))
	sl.data = append(sl.data[:start:start], sl.data[end:]...)
	sl.items = append(sl.items[:start:start], sl.items[end:]...)
	if sl.parent != nil {
		sl.parent.deleteRange(ps+sl.parentOffset, pe+sl.parentOffset)
	}
}

// insertLine mirrors self.insert(i, line, source, offset).
func (sl *stringList) insertLine(i int, line string, info lineInfo) {
	if i > len(sl.data) {
		i = len(sl.data)
	}
	sl.data = append(sl.data, "")
	copy(sl.data[i+1:], sl.data[i:])
	sl.data[i] = line
	sl.items = append(sl.items, lineInfo{})
	copy(sl.items[i+1:], sl.items[i:])
	sl.items[i] = info
	if sl.parent != nil {
		index := (len(sl.data) + i) % len(sl.data)
		sl.parent.insertLine(index+sl.parentOffset, line, info)
	}
}

// insertList mirrors self.insert(i, other).
func (sl *stringList) insertList(i int, other *stringList) {
	if i > len(sl.data) {
		i = len(sl.data)
	}
	nd := make([]string, 0, len(sl.data)+len(other.data))
	nd = append(append(append(nd, sl.data[:i]...), other.data...), sl.data[i:]...)
	ni := make([]lineInfo, 0, len(sl.items)+len(other.items))
	ni = append(append(append(ni, sl.items[:i]...), other.items...), sl.items[i:]...)
	sl.data, sl.items = nd, ni
	if sl.parent != nil && len(sl.data) > 0 {
		index := (len(sl.data) + i) % len(sl.data)
		sl.parent.insertList(index+sl.parentOffset, other)
	}
}

func (sl *stringList) trimStart(n int) {
	sl.data = sl.data[n:]
	sl.items = sl.items[n:]
	if sl.parent != nil {
		sl.parentOffset += n
	}
}

func (sl *stringList) trimEnd(n int) {
	sl.data = sl.data[:len(sl.data)-n]
	k := len(sl.items) - n
	sl.items = sl.items[:k:k]
}

func (sl *stringList) pop() string {
	i := len(sl.data) - 1
	if sl.parent != nil {
		sl.parent.deleteRange(i+sl.parentOffset, i+sl.parentOffset+1)
	}
	s := sl.data[i]
	sl.data = sl.data[:i]
	sl.items = sl.items[:i:i]
	return s
}

func (sl *stringList) disconnect() { sl.parent = nil }

// info mirrors ViewList.info(i); ok is false where Python raises IndexError.
// hasOffset is false for the "just past the end" case.
func (sl *stringList) info(i int) (info lineInfo, hasOffset bool, ok bool) {
	n := len(sl.items)
	j := i
	if j < 0 {
		j += n
	}
	if j >= 0 && j < n {
		return sl.items[j], true, true
	}
	if i == n && n > 0 {
		it := sl.items[n-1]
		return lineInfo{it.source, 0}, false, true
	}
	return lineInfo{}, false, false
}

func (sl *stringList) trimLeft(length, start, end int) {
	start, end = clampSlice(start, end, len(sl.data))
	for i := start; i < end; i++ {
		sl.data[i] = cutPrefix(sl.data[i], length)
	}
}

// cutPrefix returns s[n:] (clamped).
func cutPrefix(s string, n int) string {
	if n >= len(s) {
		return ""
	}
	return s[n:]
}

func prefix(s string, n int) string {
	if n >= len(s) {
		return s
	}
	return s[:n]
}

type unexpectedIndentation struct {
	block *stringList
	info  lineInfo
	line  int
}

// textBlockEnd returns the end of the text block starting at start; indentAt
// is the index of an unexpectedly indented line (flushLeft), or -1.
func (sl *stringList) textBlockEnd(start int, flushLeft bool) (end, indentAt int) {
	end = start
	for end < len(sl.data) {
		line := sl.data[end]
		if isBlank(line) {
			break
		}
		if flushLeft && line[0] == ' ' {
			return end, end
		}
		end++
	}
	return end, -1
}

// getTextBlock mirrors StringList.get_text_block.
func (sl *stringList) getTextBlock(start int, flushLeft bool) (*stringList, *unexpectedIndentation) {
	end := start
	last := len(sl.data)
	for end < last {
		line := sl.data[end]
		if isBlank(line) {
			break
		}
		if flushLeft && line[0] == ' ' {
			info, _, _ := sl.info(end)
			return nil, &unexpectedIndentation{sl.slice(start, end), info, info.offset + 1}
		}
		end++
	}
	return sl.slice(start, end), nil
}

// getIndented mirrors StringList.get_indented.  blockIndent and firstIndent
// are -1 for None.
func (sl *stringList) getIndented(start int, untilBlank, stripIndent bool, blockIndent, firstIndent int) (*stringList, int, bool) {
	indent := blockIndent
	end := start
	if blockIndent >= 0 && firstIndent < 0 {
		firstIndent = blockIndent
	}
	if firstIndent >= 0 {
		end++
	}
	last := len(sl.data)
	blankFinish := true
	for end < last {
		line := sl.data[end]
		if line != "" && (line[0] != ' ' || (blockIndent >= 0 && !isBlank(prefix(line, blockIndent)))) {
			blankFinish = end > start && isBlank(sl.data[end-1])
			break
		}
		stripped := pyLstrip(line)
		if stripped == "" {
			if untilBlank {
				blankFinish = true
				break
			}
		} else if blockIndent < 0 {
			lineIndent := len(line) - len(stripped)
			if indent < 0 || lineIndent < indent {
				indent = lineIndent
			}
		}
		end++
	}
	block := sl.slice(start, end)
	if firstIndent >= 0 && len(block.data) > 0 {
		block.data[0] = cutPrefix(block.data[0], firstIndent)
	}
	if indent > 0 && stripIndent {
		s := 0
		if firstIndent >= 0 {
			s = 1
		}
		block.trimLeft(indent, s, len(block.data))
	}
	if indent < 0 {
		indent = 0
	}
	return block, indent, blankFinish
}

// get2DBlock mirrors StringList.get_2D_block; left/right are column
// positions (not counting combining characters).
func (sl *stringList) get2DBlock(top, left, bottom, right int, stripIndent bool) *stringList {
	block := sl.slice(top, bottom)
	indent := right
	for i, line := range block.data {
		rs := []rune(line)
		// adjusted indices: positions of non-combining chars
		var adj []int
		for j, r := range rs {
			if !isCombining(r) {
				adj = append(adj, j)
			}
		}
		li, ri := left, len(rs)
		if left < len(adj) {
			li = adj[left]
		}
		if right < len(adj) {
			ri = adj[right]
		}
		if li > len(rs) {
			li = len(rs)
		}
		if ri < li {
			ri = li
		}
		line = pyRstrip(string(rs[li:ri]))
		block.data[i] = line
		if line != "" {
			ind := runeLen(line) - runeLen(pyLstrip(line))
			if ind < indent {
				indent = ind
			}
		}
	}
	if stripIndent && 0 < indent && indent < right {
		for i, line := range block.data {
			rs := []rune(line)
			if indent >= len(rs) {
				block.data[i] = ""
			} else {
				block.data[i] = string(rs[indent:])
			}
		}
	}
	return block
}

func (sl *stringList) padDoubleWidth(pad string) {
	for i, line := range sl.data {
		ascii := true
		for j := 0; j < len(line); j++ {
			if line[j] >= 0x80 {
				ascii = false
				break
			}
		}
		if ascii {
			continue
		}
		var b strings.Builder
		for _, r := range line {
			b.WriteRune(r)
			if isWide(r) {
				b.WriteString(pad)
			}
		}
		sl.data[i] = b.String()
	}
}

func (sl *stringList) replaceAll(old, new string) {
	for i := range sl.data {
		sl.data[i] = strings.ReplaceAll(sl.data[i], old, new)
	}
}

func (sl *stringList) join() string { return strings.Join(sl.data, "\n") }

// string2lines mirrors statemachine.string2lines.
func string2lines(s string, tabWidth int, convertWhitespace bool) []string {
	if convertWhitespace && strings.ContainsAny(s, "\v\f") {
		s = strings.Map(func(r rune) rune {
			if r == '\v' || r == '\f' {
				return ' '
			}
			return r
		}, s)
	}
	lines := pySplitlines(s, false)
	for i, l := range lines {
		lines[i] = pyRstrip(pyExpandtabs(l, tabWidth))
	}
	return lines
}
