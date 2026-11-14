package benchmarks

import (
	"fmt"
	"strings"
)

var words = strings.Fields(`the quick brown fox jumps over a lazy dog while parsing lightweight markup into
structured output documents render sections lists tables inline emphasis links code literal text converts
simple readable source files stream buffer token node tree walker visitor writer reader state machine`)

type rng uint64

func (r *rng) next() int {
	*r = *r*6364136223846793005 + 1442695040888963407
	return int(uint64(*r) >> 33)
}

func (r *rng) sentence(n int) string {
	var w []string
	for i := 0; i < n; i++ {
		w = append(w, words[r.next()%len(words)])
	}
	return strings.Join(w, " ")
}

// Gen returns the same document in the given syntax ("md", "adoc", "rst")
// with the given number of sections.
func Gen(syntax string, sections int) string {
	var b strings.Builder
	r := rng(42)
	bold := func(s string) string {
		if syntax == "adoc" {
			return "*" + s + "*"
		}
		return "**" + s + "**"
	}
	em := func(s string) string {
		if syntax == "adoc" {
			return "_" + s + "_"
		}
		return "*" + s + "*"
	}
	code := func(s string) string {
		if syntax == "rst" {
			return "``" + s + "``"
		}
		return "`" + s + "`"
	}
	link := func(text, url string) string {
		switch syntax {
		case "md":
			return "[" + text + "](" + url + ")"
		case "adoc":
			return url + "[" + text + "]"
		}
		return "`" + text + " <" + url + ">`__"
	}
	para := func() string {
		var lines []string
		for i := 0; i < 4; i++ {
			lines = append(lines, fmt.Sprintf("%s %s %s %s %s %s %s.",
				strings.Title(r.sentence(3)), bold(r.sentence(2)), r.sentence(3), em(r.sentence(1)),
				r.sentence(2), code(r.sentence(1)+"()"), link(r.sentence(2), "https://example.com/"+words[r.next()%len(words)])))
		}
		return strings.Join(lines, "\n")
	}
	switch syntax {
	case "md":
		b.WriteString("# Benchmark Document\n\n")
	case "adoc":
		b.WriteString("= Benchmark Document\n\n")
	case "rst":
		b.WriteString("==================\nBenchmark Document\n==================\n\n")
	}
	for s := 0; s < sections; s++ {
		title := fmt.Sprintf("Section %d %s", s+1, r.sentence(3))
		switch syntax {
		case "md":
			b.WriteString("## " + title + "\n\n")
		case "adoc":
			b.WriteString("== " + title + "\n\n")
		case "rst":
			b.WriteString(title + "\n" + strings.Repeat("=", len(title)) + "\n\n")
		}
		b.WriteString(para() + "\n\n")
		// bullet list with a nested list
		bullet, nested := "- ", "  - "
		if syntax == "adoc" {
			bullet, nested = "* ", "** "
		}
		for i := 0; i < 5; i++ {
			b.WriteString(bullet + r.sentence(6) + " " + em(r.sentence(1)) + "\n")
			if i == 2 {
				if syntax == "rst" {
					b.WriteString("\n")
				}
				b.WriteString(nested + r.sentence(4) + "\n" + nested + r.sentence(4) + "\n")
				if syntax == "rst" {
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")
		b.WriteString(para() + "\n\n")
		// block quote
		q := r.sentence(12) + "."
		switch syntax {
		case "md":
			b.WriteString("> " + q + "\n\n")
		case "adoc":
			b.WriteString("____\n" + q + "\n____\n\n")
		case "rst":
			b.WriteString("    " + q + "\n\n")
		}
		// ordered list
		for i := 0; i < 4; i++ {
			if syntax == "adoc" {
				b.WriteString(". " + r.sentence(7) + "\n")
			} else {
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.sentence(7)))
			}
		}
		b.WriteString("\n")
		// code block
		body := "func main() {\n    x := compute(1, 2) // " + r.sentence(3) + "\n    fmt.Println(x < 3 && x > 0)\n}\n"
		switch syntax {
		case "md":
			b.WriteString("```\n" + body + "```\n\n")
		case "adoc":
			b.WriteString("----\n" + body + "----\n\n")
		case "rst":
			b.WriteString("::\n\n")
			for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
				b.WriteString("    " + l + "\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
