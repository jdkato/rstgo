package rstgo

import "testing"

func TestPyre(t *testing.T) {
	cases := []struct {
		pat, s string
		want   string // "" = no match; else group 0
		flags  int
	}{
		{`(a|ab)(c|bcd)(d*)`, "abcd", "abcd", 0},
		{`(?<![\s\x00])(\*)($|(?=\s))`, "foo* bar", "*", 0},
		{`(?<!(?<!\x00)[\s\x00])x`, "a x \x00 x", "x", 0},
		{`(.)\1+`, "abccd", "cc", 0},
		{`(?P<q>'?)a(?P=q)`, "'a'", "'a'", 0},
		{`a.+?c`, "abcabc", "abc", 0},
		{`\w+`, "héllo wörld", "héllo", 0},
		{`[^\]]+`, "ab]c", "ab", 0},
		{`a{2,3}`, "aaaa", "aaa", 0},
		{` a  [ ]b # c
		  `, "a b", "a b", reVerbose},
		{`x$`, "x\n", "x", 0},
		{`(?!_)\w+`, "_ab", "ab", 0},
	}
	for _, c := range cases {
		m := pyCompile(c.pat, c.flags).Search(c.s)
		got := ""
		if m != nil {
			got = m.Group(0)
		}
		if got != c.want {
			t.Errorf("%q on %q: got %q want %q", c.pat, c.s, got, c.want)
		}
	}
}
