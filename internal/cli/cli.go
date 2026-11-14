// Package cli implements the command line front ends (rst2html, rst2html5,
// rst2pseudoxml), accepting the common Docutils options.
package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/jdkato/rstgo"
)

const version = "0.1.0"

var levels = map[string]int{"info": 1, "warning": 2, "error": 3, "severe": 4, "none": 5,
	"1": 1, "2": 2, "3": 3, "4": 4, "5": 5}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Main runs the named tool and returns the process exit status.
func Main(tool string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts := rstgo.Options{Stderr: stderr}
	if tool == "rst2html5" {
		opts.Writer = rstgo.HTML5
	}
	var positional []string
	output := ""
	exitStatusLevel := 5
	usage := func(msg string) int {
		fmt.Fprintf(stderr, "Usage\n=====\n  %s [options] [<source> [<destination>]]\n\n%s: error: %s\n", tool, tool, msg)
		return 2
	}
	// options taking a value
	valued := map[string]bool{"--output": true, "--output-path": true, "--title": true,
		"--source-url": true, "--report": true, "-r": true, "--halt": true,
		"--exit-status": true, "--stylesheet": true, "--stylesheet-path": true,
		"--initial-header-level": true, "--tab-width": true, "--line-length-limit": true,
		"--strip-class": true, "--strip-elements-with-class": true,
		"--footnote-references": true, "--syntax-highlight": true, "--math-output": true,
		"--table-style": true, "--attribution": true, "--field-name-limit": true,
		"--option-limit": true, "--image-loading": true, "--root-prefix": true,
		"--writer": true, "--language": true, "-l": true, "--input-encoding": true,
		"--output-encoding": true, "--error-encoding": true, "-e": true,
		"--warnings": true, "--config": true, "--record-dependencies": true,
		"--smart-quotes": true, "--pep-base-url": true, "--rfc-base-url": true,
		"--template": true, "--stylesheet-dirs": true, "--smartquotes-locales": true,
		"--pep-file-url-template": true, "--id-prefix": true, "--auto-id-prefix": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		name, val, hasVal := a, "", false
		if strings.HasPrefix(a, "--") {
			if eq := strings.Index(a, "="); eq >= 0 {
				name, val, hasVal = a[:eq], a[eq+1:], true
			}
		} else if len(a) > 2 && valued[a[:2]] {
			name, val, hasVal = a[:2], a[2:], true
		}
		if valued[name] && !hasVal {
			if i+1 >= len(args) {
				return usage(name + " option requires 1 argument")
			}
			i++
			val = args[i]
		}
		num := func() (int, bool) {
			n, err := strconv.Atoi(val)
			return n, err == nil
		}
		switch name {
		case "--version", "-V":
			fmt.Fprintf(stdout, "%s (rstgo %s; compatible with Docutils 0.23)\n", tool, version)
			return 0
		case "--help", "-h":
			fmt.Fprintf(stdout, "Usage: %s [options] [<source> [<destination>]]\n\n"+
				"Reads reStructuredText from <source> (default: stdin) and writes to\n"+
				"<destination> (default: stdout).  Accepts the common Docutils options;\n"+
				"see README.md for the supported set.\n", tool)
			return 0
		case "--output", "--output-path":
			output = val
		case "--writer":
			switch strings.ToLower(val) {
			case "html5", "html5_polyglot", "xhtml":
				opts.Writer = rstgo.HTML5
			case "html", "html4", "html4css1", "xhtml10":
				opts.Writer = rstgo.HTML4
			case "pseudoxml", "pprint", "pformat":
				tool = "rst2pseudoxml"
			default:
				return usage("unsupported writer: " + val)
			}
		case "--title":
			opts.Title = val
		case "--generator", "-g":
			opts.Generator = true
		case "--no-generator":
			opts.Generator = false
		case "--date", "-d":
			opts.Datestamp = "%Y-%m-%d"
		case "--time", "-t":
			opts.Datestamp = "%Y-%m-%d %H:%M UTC"
		case "--no-datestamp":
			opts.Datestamp = ""
		case "--source-link", "-s":
			opts.SourceLink = true
		case "--source-url":
			opts.SourceURL = val
		case "--no-source-link":
			opts.SourceLink, opts.SourceURL = false, ""
		case "--toc-entry-backlinks":
			opts.TocBacklinks = "entry"
		case "--toc-top-backlinks":
			opts.TocBacklinks = "top"
		case "--no-toc-backlinks":
			opts.TocBacklinks = "none"
		case "--footnote-backlinks":
			opts.NoFootnoteBacklinks = false
		case "--no-footnote-backlinks":
			opts.NoFootnoteBacklinks = true
		case "--section-numbering":
			opts.NoSectionNumbering = false
		case "--no-section-numbering":
			opts.NoSectionNumbering = true
		case "--strip-comments":
			opts.StripComments = true
		case "--leave-comments":
			opts.StripComments = false
		case "--strip-elements-with-class":
			opts.StripElementsWithClasses = append(opts.StripElementsWithClasses, val)
		case "--strip-class":
			opts.StripClasses = append(opts.StripClasses, val)
		case "--report", "-r":
			l, ok := levels[strings.ToLower(val)]
			if !ok {
				return usage("invalid threshold: " + val)
			}
			opts.ReportLevel = l
		case "--verbose", "-v":
			opts.ReportLevel = 1
		case "--quiet", "-q":
			opts.ReportLevel = 5
		case "--halt":
			l, ok := levels[strings.ToLower(val)]
			if !ok {
				return usage("invalid threshold: " + val)
			}
			opts.HaltLevel = l
		case "--strict":
			opts.HaltLevel = 1
		case "--exit-status":
			l, ok := levels[strings.ToLower(val)]
			if !ok {
				return usage("invalid threshold: " + val)
			}
			exitStatusLevel = l
		case "--no-file-insertion":
			opts.NoFileInsertion = true
		case "--file-insertion-enabled":
			opts.NoFileInsertion = false
		case "--no-raw":
			opts.NoRaw = true
		case "--raw-enabled":
			opts.NoRaw = false
		case "--line-length-limit":
			n, ok := num()
			if !ok {
				return usage("invalid integer: " + val)
			}
			opts.LineLengthLimit = n
		case "--pep-references":
			opts.PEPReferences = true
		case "--rfc-references":
			opts.RFCReferences = true
		case "--tab-width":
			n, ok := num()
			if !ok {
				return usage("invalid integer: " + val)
			}
			opts.TabWidth = n
		case "--trim-footnote-reference-space":
			t := true
			opts.TrimFootnoteReferenceSpace = &t
		case "--leave-footnote-reference-space":
			f := false
			opts.TrimFootnoteReferenceSpace = &f
		case "--syntax-highlight":
			opts.SyntaxHighlight = val
		case "--word-level-inline-markup":
			opts.CharacterLevelInlineMarkup = false
		case "--character-level-inline-markup":
			opts.CharacterLevelInlineMarkup = true
		case "--no-doc-title":
			opts.NoDocTitle = true
		case "--no-doc-info":
			opts.NoDocInfo = true
		case "--section-subtitles":
			opts.SectionSubtitles = true
		case "--no-section-subtitles":
			opts.SectionSubtitles = false
		case "--stylesheet":
			opts.Stylesheet = splitList(val)
			opts.StylesheetPath = nil
			opts.NoStylesheet = len(opts.Stylesheet) == 0
		case "--stylesheet-path":
			opts.StylesheetPath = splitList(val)
			opts.Stylesheet = nil
			opts.NoStylesheet = len(opts.StylesheetPath) == 0
		case "--embed-stylesheet":
			opts.LinkStylesheet = false
		case "--link-stylesheet":
			opts.LinkStylesheet = true
		case "--initial-header-level":
			n, ok := num()
			if !ok {
				return usage("invalid integer: " + val)
			}
			opts.InitialHeaderLevel = n
		case "--footnote-references":
			opts.FootnoteReferences = val
		case "--attribution":
			opts.Attribution = val
		case "--compact-lists":
			opts.NoCompactLists = false
		case "--no-compact-lists":
			opts.NoCompactLists = true
		case "--compact-field-lists":
			opts.NoCompactFieldLists = false
		case "--no-compact-field-lists":
			opts.NoCompactFieldLists = true
		case "--table-style":
			opts.TableStyle = val
		case "--math-output":
			opts.MathOutput = val
		case "--xml-declaration":
			opts.XMLDeclaration, opts.NoXMLDeclaration = true, false
		case "--no-xml-declaration":
			opts.XMLDeclaration, opts.NoXMLDeclaration = false, true
		case "--cloak-email-addresses":
			opts.CloakEmailAddresses = true
		case "--field-name-limit":
			n, ok := num()
			if !ok {
				return usage("invalid integer: " + val)
			}
			if n == 0 {
				n = -1
			}
			opts.FieldNameLimit = n
		case "--option-limit":
			n, ok := num()
			if !ok {
				return usage("invalid integer: " + val)
			}
			if n == 0 {
				n = -1
			}
			opts.OptionLimit = n
		case "--section-self-link":
			opts.SectionSelfLink = true
		case "--no-section-self-link":
			opts.SectionSelfLink = false
		case "--image-loading":
			opts.ImageLoading = val
		case "--embed-images":
			opts.ImageLoading = "embed"
		case "--link-images":
			opts.ImageLoading = "link"
		case "--root-prefix":
			opts.RootPrefix = val
		case "--input-encoding", "--output-encoding", "--error-encoding", "-e",
			"--language", "-l", "--config", "--traceback", "--no-traceback",
			"--debug", "--no-debug", "--legacy-ids", "--no-validation", "--smart-quotes", "--smartquotes-locales",
			"--record-dependencies", "--pep-base-url", "--rfc-base-url",
			"--pep-file-url-template", "--stylesheet-dirs", "--warnings":
			// accepted for compatibility; only the Docutils defaults
			// (UTF-8, English) are implemented
			if (name == "--language" || name == "-l") && !strings.HasPrefix(strings.ToLower(val), "en") {
				return usage("only the English language module is supported")
			}
		default:
			return usage("no such option: " + name)
		}
	}
	if len(positional) > 2 {
		return usage("Maximum 2 arguments allowed.")
	}
	var src []byte
	var err error
	if len(positional) > 0 && positional[0] != "-" {
		opts.SourcePath = positional[0]
		src, err = os.ReadFile(positional[0])
		if err != nil {
			fmt.Fprintf(stderr, "Unable to open source file for reading:\n  %v\n", err)
			return 1
		}
	} else {
		src, err = io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return 1
		}
	}
	if len(positional) > 1 {
		output = positional[1]
	}
	if output != "" && output != "-" {
		opts.DestPath = output
	}
	var out []byte
	var res rstgo.Result
	if tool == "rst2pseudoxml" {
		out, res, err = rstgo.PseudoXMLResult(src, opts)
	} else {
		out, res, err = rstgo.ConvertResult(src, opts)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if opts.DestPath != "" {
		if err := os.WriteFile(opts.DestPath, out, 0o644); err != nil {
			fmt.Fprintf(stderr, "Unable to open destination file for writing:\n  %v\n", err)
			return 1
		}
	} else {
		stdout.Write(out)
	}
	if res.MaxLevel >= exitStatusLevel {
		return res.MaxLevel + 10
	}
	return 0
}
