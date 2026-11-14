package rstgo

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type math2htmlCase struct {
	Code   string `json:"code"`
	Block  bool   `json:"block"`
	HTML   string `json:"html"`
	Errors string `json:"errors"`
	Crash  string `json:"crash"`
}

// TestMath2HTML compares the math2html port with results recorded from
// Docutils (scripts/gen_math_cases.py): the HTML, the diagnostics printed
// to stderr and, where the original raises an exception, that exception.
// RSTGO_MATH_CASES names an alternative case file (e.g. another seed).
func TestMath2HTML(t *testing.T) {
	path := os.Getenv("RSTGO_MATH_CASES")
	if path == "" {
		path = "testdata/math2html.json"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []math2htmlCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	failed, crashes := 0, 0
	for _, c := range cases {
		html, traces, crash := math2html(wrapMathCode(c.Code, c.Block), c.Block)
		errors := ""
		if len(traces) > 0 {
			errors = strings.Join(traces, "\n") + "\n"
		}
		if c.Crash != "" {
			crashes++
		}
		ok := html == c.HTML && errors == c.Errors && crash == c.Crash
		if !ok {
			failed++
			if failed <= 15 {
				t.Errorf("math2html(%q, block=%v):\n got html %q\nwant html %q\n got errors %q\nwant errors %q\n got crash %q\nwant crash %q",
					c.Code, c.Block, html, c.HTML, errors, c.Errors, crash, c.Crash)
			}
		}
	}
	t.Logf("%d formulas, %d differ; %d raise an exception in Docutils", len(cases), failed, crashes)
}
