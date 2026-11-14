# rstgo

A native Go implementation of [Docutils](https://docutils.sourceforge.io/)'
`rst2html`: reStructuredText in, HTML out, with output that is meant to be
**byte-for-byte identical** to what Docutils 0.23 produces. No Python, no
cgo, no dependencies outside the Go standard library.

The code is a port, not a reinterpretation of the reStructuredText
specification. It follows the Docutils pipeline step by step:

```
parser (state machine + inliner)  ->  document tree  ->  transforms  ->  writer
states.go, inliner.go                 nodes.go           transforms*.go   html*.go
```

* the block-level parser is a port of `docutils.parsers.rst.states` and
  `docutils.statemachine` (same states, same transitions, same patterns);
* Docutils' regular expressions are implemented as hand-written scanners
  (`scan*.go`) that reproduce the matching order of the original patterns
  (look-ahead, look-behind, backtracking, Unicode-aware `\w`/`\s`). No
  regular-expression engine runs during a conversion; a small engine with
  Python `re` semantics (`pyre_engine_test.go`) is kept in the tests, where
  every scanner is checked against the pattern it replaces
  (`oracle_test.go`);
* the document tree uses the element and attribute names of `docutils.nodes`
  and can be dumped as pseudo-XML exactly like `rst2pseudoxml`;
* the transforms (doctitle, docinfo, substitutions, hyperlink and footnote
  resolution, contents, sectnum, ...) run in Docutils' priority order;
* there are two writers: `html4css1` (what the `rst2html` command uses in
  Docutils 0.23, and the default here) and `html5_polyglot` (`rst2html5`);
* warnings and errors are reported like Docutils does: on stderr, and as
  `system_message` elements inside the document, with "problematic" spans and
  backlinks.

## Install

```
go install github.com/jdkato/rstgo/cmd/rst2html@latest
go install github.com/jdkato/rstgo/cmd/rst2html5@latest
go install github.com/jdkato/rstgo/cmd/rst2pseudoxml@latest
```

## Command line

```
rst2html [options] [<source> [<destination>]]
```

Reads from `<source>` (default: stdin) and writes to `<destination>`
(default: stdout), like the Docutils front ends. `rst2html` produces
XHTML 1.0 Transitional (html4css1), `rst2html5` produces HTML5, and
`rst2pseudoxml` prints the document tree.

Supported options (same spelling and meaning as in Docutils):

| Area | Options |
|------|---------|
| General | `--output`, `--title`, `--generator`/`--no-generator`, `--date`, `--time`, `--no-datestamp`, `--source-link`, `--source-url`, `--no-source-link`, `--toc-entry-backlinks`, `--toc-top-backlinks`, `--no-toc-backlinks`, `--footnote-backlinks`, `--no-footnote-backlinks`, `--section-numbering`, `--no-section-numbering`, `--strip-comments`, `--leave-comments`, `--strip-elements-with-class`, `--strip-class`, `--report`/`-r`, `--verbose`/`-v`, `--quiet`/`-q`, `--halt`, `--strict`, `--exit-status`, `--root-prefix`, `--version`, `--help` |
| Parser | `--no-file-insertion`, `--file-insertion-enabled`, `--no-raw`, `--raw-enabled`, `--line-length-limit`, `--pep-references`, `--rfc-references`, `--tab-width`, `--trim-footnote-reference-space`, `--leave-footnote-reference-space`, `--syntax-highlight`, `--smart-quotes`, `--character-level-inline-markup`, `--word-level-inline-markup` |
| Reader | `--no-doc-title`, `--no-doc-info`, `--section-subtitles`, `--no-section-subtitles` |
| HTML | `--stylesheet`, `--stylesheet-path`, `--embed-stylesheet`, `--link-stylesheet`, `--initial-header-level`, `--footnote-references`, `--attribution`, `--compact-lists`, `--no-compact-lists`, `--compact-field-lists`, `--no-compact-field-lists`, `--table-style`, `--math-output`, `--xml-declaration`, `--no-xml-declaration`, `--cloak-email-addresses`, `--field-name-limit`, `--option-limit`, `--section-self-link`, `--image-loading`, `--embed-images`, `--link-images` |

A few more (`--input-encoding`, `--output-encoding`, `--language`, `--config`,
`--traceback`, ...) are accepted for command-line compatibility but only the
Docutils defaults are implemented (UTF-8, English, no configuration files).

Docutils' default stylesheets (`html4css1.css`, `minimal.css`, `plain.css`,
`math.css`, `responsive.css`, `tuftig.css`, `italic-field-names.css`) are
embedded in the binary and are used when `--stylesheet-path` names one of
them (or by default), exactly as Docutils finds them in its installation
directory. They are unmodified copies from Docutils and carry their own
copyright headers (public domain / BSD 2-Clause).

Exit status: 0 on success, 1 when processing was halted by a system message
at or above `--halt` (default: severe), `10 + level` with `--exit-status`.

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

`Options` mirrors the Docutils settings: `Writer`, `SourcePath` (used for
messages and to resolve `include`), `Stderr` (where system messages go;
`nil` discards them), `InitialHeaderLevel`, `NoDocTitle`, `Stylesheet`,
`StylesheetPath`, `NoStylesheet`, `LinkStylesheet`, `ReportLevel`,
`HaltLevel`, `NoFileInsertion`, `NoRaw`, `TabWidth`, and so on; see the
field documentation in `api.go`. The zero value means "Docutils defaults".

A conversion only fails with an error when the input is not valid UTF-8 or
when a system message reaches the halt level (default: SEVERE), in which
case Docutils would exit with "Exiting due to level-4 (SEVERE) system
message". Ordinary markup errors do not fail the conversion: they are
reported on `Options.Stderr` and embedded in the output.

Untrusted input: as with Docutils, the `include`, `raw` and `csv-table`
directives can read local files. Set `NoFileInsertion` (and `NoRaw`) when
converting documents you do not trust.

## Compatibility

The reference is Docutils 0.23 (`rst2html` = `html4css1` writer). The test
corpus under `testdata/` is compared byte for byte, for the pseudo-XML tree,
the html4css1 document, the html5 document and the stderr output:

* `testdata/suites/` — the inputs of Docutils' own parser, directive and
  transform unit tests (956 small documents extracted from the Docutils
  source distribution; expected output generated by the real Docutils);
* `testdata/docutils/` — Docutils' documentation (including the
  reStructuredText specification and directive/role references) and the
  inputs of its functional tests (132 documents);
* `testdata/fixtures/` — additional documents written for this project
  (feature overviews, error cases, option combinations, garbled input).

Current results are in [Test status](#test-status) below; every case that
does not match is listed in `testdata/known_failures.txt`.

### Known gaps

* **Math output of the html4css1 writer.** With the default
  `--math-output="HTML math.css"`, Docutils converts LaTeX math to HTML+CSS
  with its `math2html` module. That converter is not ported: `rst2html`
  writes math as with `--math-output=LaTeX` (`<tt class="math">` /
  `<pre class="math">` containing the LaTeX source) and does not embed
  `math.css`. This is the cause of all known html4 corpus failures; the
  same documents are identical to Docutils when both sides use
  `--math-output=LaTeX` (the `html4m`/`html5m` corpus runs).
  `--math-output=MathJax` and `LaTeX` are implemented for both writers, and
  the html5 writer's default MathML output is a port of Docutils'
  `latex2mathml` (external converters such as `--math-output="MathML
  pandoc"` are not supported).
* **Syntax highlighting.** Pygments is not available; `code` directives and
  roles behave like Docutils without Pygments installed (a warning and no
  output for a `code` block that names a language, unless the report level
  is above "warning" or `--syntax-highlight=none` is used).
* **Smart quotes** (`--smart-quotes`) are not implemented; the option is
  accepted and ignored.
* **Languages.** Only English (`--language=en`) directive/role names and
  labels.
* **Images.** Image sizes (for `:scale:` and `:figwidth: image`) are read
  for PNG, JPEG and GIF only (Docutils uses Pillow). Embedding SVG images
  (`:loading: embed`) inserts a `data:` URI instead of inline `<svg>`.
* **Network.** `raw`/`csv-table` `:url:` only supports `file:` URLs.
* **Encodings.** Input must be UTF-8 (Latin-1 for `:encoding: latin-1`).
* **Environment-dependent paths.** `.. include:: <isonum.txt>` and friends
  use embedded copies of Docutils' standard include files; messages that
  mention their location show `<isonum.txt>` instead of the path of a
  Docutils installation. With `--link-stylesheet`, the bundled default
  stylesheets are linked by bare file name.
* Not implemented: other writers (LaTeX, ODT, man, XML, S5), the PEP reader,
  configuration files, `--record-dependencies`, `--stylesheet-dirs`,
  `--template`, `--id-prefix`/`--auto-id-prefix`, `:parser:` other than rst
  for `include`, Sphinx directives and roles (they are reported as unknown,
  exactly like Docutils does).
* **Document tree API.** `Node.Rawsource` is not recorded for `list_item`,
  `block_quote`, `description` and `definition_list_item` elements (Docutils
  stores the joined source block there; no output depends on it).
* Where Docutils itself crashes with a Python exception (there are a few
  such inputs), rstgo produces some output instead; no attempt is made to
  be compatible there.

### Test status

<!-- test-status -->
Corpus: 1120 documents (956 Docutils unit-test inputs, 132 Docutils
documentation / functional-test documents, 32 project fixtures). Each is
compared in three forms, output and stderr both byte for byte
(`go test -run TestCorpus -v .`):

| Output | Identical to Docutils 0.23 |
|--------|----------------------------|
| pseudo-XML document tree (`pxml`) | 1119 / 1120 |
| html4css1 document (`html4`, the `rst2html` default) | 1091 / 1120 |
| html5 document (`html5`) | 1119 / 1120 |
| html4css1 with `--math-output=LaTeX`, 23 documents containing math (`html4m`) | 23 / 23 |
| html5 with `--math-output=LaTeX`, same 23 documents (`html5m`) | 23 / 23 |

The 31 known failures (`testdata/known_failures.txt`):

* 28 html4 cases are documents containing math (unported `math2html`
  conversion, see above);
* 3 cases (one input, all three outputs) are an error message that contains
  the installation path of Docutils' standard include directory
  (`.. include:: <nonexistent>`).

Robustness: `TestMutatedCorpus` converts about 19 700 truncated or randomly
damaged variants of the corpus inputs, `TestPathological` a set of deeply
nested and unterminated constructs, and `FuzzConvert` is a native fuzz
target; none may panic or hang.
<!-- /test-status -->

## Performance

No regular-expression engine runs during a conversion (see above), and the
document tree, the transforms and the writers avoid per-node allocations
where they can. `benchmarks/` is a separate module that converts a
generated document (headings, paragraphs with inline markup, lists, code
blocks, block quotes), written equivalently in reStructuredText and
Markdown, with rstgo (`ConvertBody`) and with
[goldmark](https://github.com/yuin/goldmark):

```
cd benchmarks && go test -bench Convert -benchmem
```

Best of 8 runs on one machine (4 cores, Intel Xeon 2.1 GHz, Go 1.24);
sections / input size:

| Document | | time | throughput | B/op | allocs/op |
|----------|---|------|-----------|------|-----------|
| 8 sections (15 KB) | goldmark | 0.75 ms | 19.5 MB/s | 413 KB | 2 714 |
| | rstgo | 1.02 ms | 15.1 MB/s | 444 KB | 1 635 |
| 80 sections (150 KB) | goldmark | 8.5 ms | 17.1 MB/s | 3.98 MB | 26 910 |
| | rstgo | 10.7 ms | 14.2 MB/s | 4.13 MB | 14 057 |
| 800 sections (1.5 MB) | goldmark | 79.9 ms | 18.2 MB/s | 38.8 MB | 268 843 |
| | rstgo | 108.5 ms | 14.1 MB/s | 40.8 MB | 137 943 |

That is 1.25x to 1.4x goldmark's time at about the same memory and half the
allocations, and linear in the input size. `go test -bench Convert -benchmem .`
in the root module converts four documents of the corpus to complete html4css1
documents (same machine, best of 6):

| Document | size | time | B/op | allocs/op |
|----------|------|------|------|-----------|
| Docutils README | 12 KB | 1.18 ms | 385 KB | 2 544 |
| Docutils FAQ | 46 KB | 3.47 ms | 1.27 MB | 7 798 |
| reStructuredText specification | 124 KB | 9.16 ms | 3.26 MB | 18 297 |
| Docutils HISTORY | 173 KB | 14.5 ms | 4.98 MB | 21 938 |

## Development

```
go test ./...                       # corpus comparison, robustness tests
go test -run TestCorpus -v .        # pass counts per output kind
go test -fuzz=FuzzConvert .         # fuzzing
```

The expected outputs are generated by the reference implementation:

```
pip install docutils==0.23
scripts/regen.sh                    # all of testdata/ (takes a few minutes)
scripts/regen.sh fixtures           # only testdata/fixtures
```

`scripts/regen.py` runs Docutils in a fresh (forked) process per document,
with the default settings except that no stylesheet is embedded (to keep
the corpus small; `testdata/fixtures/basic_css` covers the default
stylesheet). Nothing is normalised: the generator `<meta>` tag carries the
Docutils version this port tracks, and the corpus avoids time-dependent
features (the `date` directive, `--date`). Fixture-specific settings live in
`NAME.json` next to `NAME.rst` and use the Docutils setting names.

To add a test, drop a `.rst` file into `testdata/fixtures/` and run
`scripts/regen.sh fixtures`. If it cannot be made to pass, add it to
`testdata/known_failures.txt` (`RSTGO_UPDATE_KNOWN=1 go test -run TestCorpus .`
rewrites the list from the current results).

Other scripts: `scripts/gen_unidata.py` regenerates the Unicode tables
(`unidata.go`, `mathdata.go`) from Python's `unicodedata` and Docutils;
`scripts/import_docutils_tests.py` extracts the unit-test inputs from a
Docutils source distribution.

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
