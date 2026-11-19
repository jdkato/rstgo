package rstgo

import (
	"strings"
	"testing"
)

const sphinxDoc = `Title
=====

.. versionadded:: 1.2
   The text under the argument is prose.

.. note-extension:: An argument
   :option: ignored

   Content is prose too.

.. toctree::
   :maxdepth: 2

   intro
   usage

See :ref:` + "`" + `intro` + "`" + ` and :ref:` + "`" + `The introduction <intro>` + "`" + `, press :guilabel:` + "`" + `OK` + "`" + `, call :py:func:` + "`" + `open` + "`" + `.
`

func sphinxOptions() Options {
	return Options{
		ReportLevel: 5,
		HaltLevel:   5,
		UnknownDirective: func(name string) DirectiveFallback {
			if name == "toctree" {
				return DirectiveLiteral
			}
			return DirectiveProse
		},
		UnknownRole: func(name string) RoleFallback {
			switch name[strings.LastIndexByte(name, ':')+1:] {
			case "ref":
				return RoleReference
			case "guilabel":
				return RoleText
			}
			return RoleLiteral
		},
	}
}

// An unknown directive is prose or a literal as the caller says, an unknown
// role is words or a literal, and a reference keeps only its title.
func TestUnknownDirectivesAndRoles(t *testing.T) {
	out, err := ConvertBody([]byte(sphinxDoc), sphinxOptions())
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)

	for _, want := range []string{
		"<div class=\"docutils container\">\nThe text under the argument is prose.</div>",
		"<div class=\"docutils container\">\nContent is prose too.</div>",
		"<pre class=\"literal-block\">\nintro\nusage\n</pre>",
		"See <tt class=\"docutils literal\">intro</tt> and The introduction, press OK, call <tt class=\"docutils literal\">open</tt>.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	for _, absent := range []string{"system-message", "problematic", "Unknown directive", ":maxdepth:"} {
		if strings.Contains(html, absent) {
			t.Errorf("found %q in:\n%s", absent, html)
		}
	}
}

// Without the hooks, the same document is reported the way Docutils reports
// it: nothing about the fallbacks leaks into the default behavior.
func TestUnknownNamesStayErrorsByDefault(t *testing.T) {
	out, err := ConvertBody([]byte(sphinxDoc), Options{HaltLevel: 5})
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, want := range []string{`Unknown directive type &quot;versionadded&quot;.`, `Unknown interpreted text role &quot;guilabel&quot;.`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

func TestTitleOf(t *testing.T) {
	cases := map[string][2]any{
		"The introduction <intro>": {"The introduction", true},
		"Title<target>":            {"Title", true},
		"intro":                    {"", false},
		"<intro>":                  {"", false},
		"a <b> c":                  {"", false},
		"a <b<c>>":                 {"", false},
	}
	for in, want := range cases {
		title, ok := titleOf(in)
		if title != want[0] || ok != want[1] {
			t.Errorf("titleOf(%q) = %q, %v; want %q, %v", in, title, ok, want[0], want[1])
		}
	}
}
