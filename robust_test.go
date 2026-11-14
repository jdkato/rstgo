package rstgo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// corpusInputs returns the suite inputs and fixture files.
func corpusInputs(t testing.TB) []string {
	var inputs []string
	files, _ := filepath.Glob("testdata/suites/*.json")
	for _, fn := range files {
		if strings.HasSuffix(fn, ".expected.json") {
			continue
		}
		var suite suiteFile
		data, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &suite); err != nil {
			t.Fatal(err)
		}
		for _, c := range suite.Cases {
			inputs = append(inputs, c.Input)
		}
	}
	files, _ = filepath.Glob("testdata/fixtures/*.rst")
	for _, fn := range files {
		data, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, string(data))
	}
	return inputs
}

// convertGuarded runs a conversion and fails on panics and hangs.
func convertGuarded(t *testing.T, what string, src string, html5 bool) {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Sprintf("panic: %v", r)
			}
		}()
		opts := Options{NoStylesheet: true, NoFileInsertion: true, ReportLevel: 1}
		if html5 {
			opts.Writer = HTML5
		}
		_, err := Convert([]byte(src), opts)
		if err != nil {
			if _, ok := err.(*haltError); !ok && !strings.Contains(err.Error(), "UTF-8") &&
				!strings.Contains(err.Error(), "nesting") {
				done <- "error: " + err.Error()
				return
			}
		}
		done <- ""
	}()
	select {
	case msg := <-done:
		if msg != "" {
			t.Errorf("%s: %s\ninput: %q", what, msg, src)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: timeout\ninput: %q", what, src)
	}
}

// TestMutatedCorpus feeds truncated and randomly damaged variants of the
// corpus inputs to the converter: it must neither panic nor hang.
func TestMutatedCorpus(t *testing.T) {
	inputs := corpusInputs(t)
	rng := rand.New(rand.NewSource(1))
	junk := []string{"*", "`", "_", "|", "\\", ":", "::", "..", " ", "\n", "\n\n", "  ", "-", "=", "+", "[", "]_",
		"<", ">", "\t", "\x00", "é", "日本", ".. ", "__", "#", "(", ")", "\u0301", "``", "**", "|x|", ":r:`", "\r"}
	n := 0
	for i, in := range inputs {
		if testing.Short() && i%10 != 0 {
			continue
		}
		var variants []string
		// truncations
		for k := 1; k <= 6; k++ {
			cut := len(in) * k / 7
			variants = append(variants, in[:cut])
		}
		lines := strings.Split(in, "\n")
		for k := 0; k < 6; k++ {
			ls := append([]string{}, lines...)
			switch k % 3 {
			case 0: // delete a line
				j := rng.Intn(len(ls))
				ls = append(ls[:j], ls[j+1:]...)
			case 1: // duplicate a line, changing its indentation
				j := rng.Intn(len(ls))
				ls = append(ls[:j+1], append([]string{strings.Repeat(" ", rng.Intn(5)) + strings.TrimLeft(ls[j], " ")}, ls[j+1:]...)...)
			case 2: // swap two lines
				a, b := rng.Intn(len(ls)), rng.Intn(len(ls))
				ls[a], ls[b] = ls[b], ls[a]
			}
			variants = append(variants, strings.Join(ls, "\n"))
		}
		for k := 0; k < 8; k++ {
			if len(in) == 0 {
				break
			}
			pos := rng.Intn(len(in))
			j := junk[rng.Intn(len(junk))]
			if k%2 == 0 {
				variants = append(variants, in[:pos]+j+in[pos:])
			} else {
				end := pos + rng.Intn(4)
				if end > len(in) {
					end = len(in)
				}
				variants = append(variants, in[:pos]+j+in[end:])
			}
		}
		for k, v := range variants {
			convertGuarded(t, fmt.Sprintf("input %d variant %d", i, k), v, (i+k)%2 == 0)
			n++
		}
	}
	t.Logf("%d mutated inputs converted without panic or timeout", n)
}

// TestPathological exercises deep nesting and unterminated constructs.
func TestPathological(t *testing.T) {
	cases := map[string]string{
		"deep block quote": func() string {
			var b strings.Builder
			for i := 0; i < 300; i++ {
				b.WriteString(strings.Repeat(" ", i) + "x\n\n")
			}
			return b.String()
		}(),
		"deep bullets": func() string {
			var b strings.Builder
			for i := 0; i < 200; i++ {
				b.WriteString(strings.Repeat("  ", i) + "- x\n\n")
			}
			return b.String()
		}(),
		"same-line bullets": strings.Repeat("- ", 2000) + "x\n",
		"same-line enums":   strings.Repeat("1. ", 500) + "x\n",
		"many stars":        strings.Repeat("*a ", 5000) + "\n",
		"many backquotes":   strings.Repeat("`a ", 5000) + "\n",
		"many refs":         strings.Repeat("a_ ", 5000) + "\n",
		"many subst":        strings.Repeat("|a ", 3000) + "\n",
		"long word":         strings.Repeat("a", 9000) + "_\n",
		"long underscores":  strings.Repeat("_", 9000) + "\n",
		"colons":            strings.Repeat(":", 5000) + "\n",
		"nested sections":   strings.Repeat("T\n=\n\nS\n-\n\n", 500),
		"many directives":   strings.Repeat(".. note:: x\n", 2000),
		"nested notes": func() string {
			var b strings.Builder
			for i := 0; i < 150; i++ {
				b.WriteString(strings.Repeat("   ", i) + ".. note::\n\n")
			}
			return b.String()
		}(),
		"big grid table":  "+" + strings.Repeat("--+", 300) + "\n|" + strings.Repeat("  |", 300) + "\n+" + strings.Repeat("--+", 300) + "\n",
		"broken table":    "+---+\n| a\n+---+---+\n|\n",
		"subst loop":      ".. |a| replace:: |b|\n.. |b| replace:: |a|\n\n|a| |b|\n",
		"target loop":     ".. _a: b_\n.. _b: c_\n.. _c: a_\n\na_ b_ c_\n",
		"include self":    ".. include:: /dev/stdin\n",
		"nul bytes":       "a\x00b *c\x00* ``d\x00``\n\x00\n",
		"invalid utf8":    "caf\xe9 \xff\xfe\n",
		"only whitespace": " \n\t\n  \n",
		"email flood":     strings.Repeat("a@b.c ", 3000) + "\n",
		"uri flood":       strings.Repeat("http://a.b/c ", 3000) + "\n",
		"embedded uris":   strings.Repeat("`a <b>`_ ", 2000) + "\n",
		"field markers":   strings.Repeat(":a:b", 3000) + "\n",
	}
	for name, src := range cases {
		convertGuarded(t, name, src, false)
		convertGuarded(t, name, src, true)
	}
}

// FuzzConvert is a native fuzz target: go test -fuzz=FuzzConvert
func FuzzConvert(f *testing.F) {
	for i, in := range corpusInputs(f) {
		if i%7 == 0 {
			f.Add(in)
		}
	}
	f.Fuzz(func(t *testing.T, src string) {
		for _, w := range []Writer{HTML4, HTML5} {
			_, _ = Convert([]byte(src), Options{Writer: w, NoStylesheet: true, NoFileInsertion: true, ReportLevel: 1})
		}
	})
}
