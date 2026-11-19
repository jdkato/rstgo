# rstgo

A Go port of [Docutils](https://docutils.sourceforge.io/) 0.23. It converts
reStructuredText to HTML, and its output matches `rst2html` byte for byte.
It uses only the Go standard library.

```
go install github.com/jdkato/rstgo/cmd/rst2html@latest
go install github.com/jdkato/rstgo/cmd/rst2html5@latest
go install github.com/jdkato/rstgo/cmd/rst2pseudoxml@latest
```

## Command line

```
rst2html [options] [<source> [<destination>]]
```

Source defaults to stdin and destination to stdout. `rst2html` writes
XHTML 1.0 Transitional (the `html4css1` writer), `rst2html5` writes HTML5,
and `rst2pseudoxml` prints the document tree.

These options work as they do in Docutils:

| Area | Options |
|------|---------|
| General | `--output`, `--title`, `--generator`/`--no-generator`, `--date`, `--time`, `--no-datestamp`, `--source-link`, `--source-url`, `--no-source-link`, `--toc-entry-backlinks`, `--toc-top-backlinks`, `--no-toc-backlinks`, `--footnote-backlinks`, `--no-footnote-backlinks`, `--section-numbering`, `--no-section-numbering`, `--strip-comments`, `--leave-comments`, `--strip-elements-with-class`, `--strip-class`, `--report`/`-r`, `--verbose`/`-v`, `--quiet`/`-q`, `--halt`, `--strict`, `--exit-status`, `--root-prefix`, `--version`, `--help` |
| Parser | `--no-file-insertion`, `--file-insertion-enabled`, `--no-raw`, `--raw-enabled`, `--line-length-limit`, `--pep-references`, `--rfc-references`, `--tab-width`, `--trim-footnote-reference-space`, `--leave-footnote-reference-space`, `--syntax-highlight`, `--smart-quotes`, `--character-level-inline-markup`, `--word-level-inline-markup` |
| Reader | `--no-doc-title`, `--no-doc-info`, `--section-subtitles`, `--no-section-subtitles` |
| HTML | `--stylesheet`, `--stylesheet-path`, `--embed-stylesheet`, `--link-stylesheet`, `--initial-header-level`, `--footnote-references`, `--attribution`, `--compact-lists`, `--no-compact-lists`, `--compact-field-lists`, `--no-compact-field-lists`, `--table-style`, `--math-output`, `--xml-declaration`, `--no-xml-declaration`, `--cloak-email-addresses`, `--field-name-limit`, `--option-limit`, `--section-self-link`, `--image-loading`, `--embed-images`, `--link-images` |

`--input-encoding`, `--output-encoding`, `--language`, `--config`, and
`--traceback` are accepted, but only the Docutils defaults apply: UTF-8,
English, and no configuration files.

Docutils' default stylesheets are embedded in the binary.

Exit status is 0 on success and 1 when a system message reaches `--halt`.
With `--exit-status` it is 10 plus the message level.

## Library

```go
import "github.com/jdkato/rstgo"

html, err := rstgo.Convert(src, rstgo.Options{})            // full document, like rst2html
html, err  = rstgo.Convert(src, rstgo.Options{Writer: rstgo.HTML5})
body, err := rstgo.ConvertBody(src, rstgo.Options{})        // publish_parts()["html_body"]
parts, err := rstgo.ConvertParts(src, rstgo.Options{})      // Body, Title, Docinfo, Head, ...
pxml, err := rstgo.PseudoXML(src, rstgo.Options{})          // like rst2pseudoxml
doc, err  := rstgo.Parse(src, rstgo.Options{})              // *Document; doc.Root is the tree
```

`Options` mirrors the Docutils settings, and its zero value is the Docutils
defaults. The fields are documented in `api.go`.

Markup errors don't fail a conversion. They go to `Options.Stderr` and into
the output, as in Docutils. A conversion returns an error only when the
input isn't valid UTF-8 or a system message reaches the halt level.

Set `NoFileInsertion` and `NoRaw` for input you don't trust. Otherwise the
`include`, `raw`, and `csv-table` directives can read local files.

## Compatibility

The test suite converts 1125 documents and compares the document tree, the
`html4css1` output, the `html5` output, and stderr with Docutils 0.23. All
of them match.

| Corpus | Documents | Source |
|--------|----------:|--------|
| `testdata/suites` | 956 | inputs of Docutils' parser, directive, and transform tests |
| `testdata/docutils` | 132 | Docutils' documentation and functional tests |
| `testdata/fixtures` | 37 | written for this project |

`--math-output` supports `HTML`, `MathML`, `MathJax`, and `LaTeX`.

Not supported:

* External MathML converters, such as `--math-output="MathML pandoc"`.
* Pygments. `code` behaves as it does in Docutils without Pygments.
* `--smart-quotes`. The option is accepted and ignored.
* Languages other than English.
* Image sizes for formats other than PNG, JPEG, and GIF.
* `:url:` on `raw` and `csv-table`, except for `file:` URLs.
* Input that isn't UTF-8, except `:encoding: latin-1`.
* Other writers (LaTeX, ODT, man, XML, S5), the PEP reader, configuration
  files, `--template`, and `--id-prefix`.
* Sphinx directives and roles. They are reported as unknown, as in
  Docutils.

Docutils prints two paths from its own installation: the directory of its
standard include files and the directory of its default stylesheets. rstgo
reports the first as `rstgo.StandardIncludeDir` and links the stylesheets
by file name.

## Performance

`benchmarks/` converts the same generated document, written in
reStructuredText and in Markdown, with rstgo and with
[goldmark](https://github.com/yuin/goldmark). Best of 8 runs on a 4-core
Intel Xeon at 2.1 GHz with Go 1.24:

| Document | Converter | Time | Throughput | Allocated | Allocations |
|----------|-----------|-----:|-----------:|----------:|------------:|
| 8 sections (15 KB)    | goldmark | 0.75 ms  | 19.5 MB/s | 413 KB  | 2,714   |
|                       | rstgo    | 1.02 ms  | 15.1 MB/s | 444 KB  | 1,635   |
| 80 sections (150 KB)  | goldmark | 8.5 ms   | 17.1 MB/s | 3.98 MB | 26,910  |
|                       | rstgo    | 10.7 ms  | 14.2 MB/s | 4.13 MB | 14,057  |
| 800 sections (1.5 MB) | goldmark | 79.9 ms  | 18.2 MB/s | 38.8 MB | 268,843 |
|                       | rstgo    | 108.5 ms | 14.1 MB/s | 40.8 MB | 137,943 |

```
cd benchmarks && go test -bench Convert -benchmem
```

## Development

```
go test ./...
go test -run TestCorpus -v .        # pass counts per output
go test -fuzz=FuzzConvert .
```

Docutils generates the expected output:

```
pip install docutils==0.23
scripts/regen.sh                    # all of testdata/, a few minutes
scripts/regen.sh fixtures           # only testdata/fixtures
```

To add a test, put a `.rst` file in `testdata/fixtures/` and run
`scripts/regen.sh fixtures`. Settings for a fixture go in `NAME.json` next
to it, under the Docutils setting names.

The code follows the Docutils pipeline: `states.go` and `inliner.go` parse,
`transforms*.go` run the transforms in Docutils' order, and `html*.go`
write. Docutils' regular expressions are hand-written scanners in
`scan*.go`, and `oracle_test.go` checks each one against the pattern it
replaces.

## AI attribution

rstgo was written by [Claude Code](https://claude.com/claude-code),
Anthropic's AI coding agent, working under the direction of the repository
owner. The code is a port of the Docutils source, and its correctness rests
on the differential test corpus described above (byte-for-byte comparison
with Docutils 0.23) rather than on line-by-line human authorship. Commits
produced this way carry a `Co-Authored-By: Claude` trailer.

## License

MIT (see [LICENSE](LICENSE)). rstgo is derived from Docutils. Most of
Docutils is in the public domain, but the HTML writer base, the HTML5
writer, the LaTeX-to-MathML converter, the punctuation tables and most of
the bundled stylesheets are under the BSD 2-Clause License; their copyright
notices and the license text are in [NOTICE](NOTICE). The standard include
files under `include/` and the documents under `testdata/docutils/` are
unmodified Docutils files and keep their own terms.
