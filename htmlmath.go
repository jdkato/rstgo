package rstgo

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// toplevelCode mirrors docutils.utils.math.toplevel_code.
func toplevelCode(code string) string {
	chunks := strings.Split(code, `\begin{`)
	for i, chunk := range chunks {
		parts := strings.Split(chunk, `\end{`)
		chunks[i] = parts[len(parts)-1]
	}
	return strings.Join(chunks, `\begin{`)
}

// pickMathEnvironment mirrors docutils.utils.math.pick_math_environment.
func pickMathEnvironment(code string, numbered bool) string {
	env := "equation"
	if strings.Contains(toplevelCode(code), `\\`) {
		env = "align"
	}
	if !numbered {
		env += "*"
	}
	return env
}

func wrapMathCode(code string, asBlock bool) string {
	if asBlock {
		env := pickMathEnvironment(code, false)
		return fmt.Sprintf("\\begin{%s}\n%s\n\\end{%s}", env, code, env)
	}
	return "$" + code + "$"
}

// mathCrashError reports a formula on which Docutils' math2html raises a
// Python exception (and Docutils exits with a traceback).
type mathCrashError struct{ text string }

func (e *mathCrashError) Error() string {
	return "rstgo: cannot convert math to HTML (Docutils fails here with " + e.text + ")"
}

func (t *htmlTranslator) visitMath(node *Node) visitAction {
	isBlock := node.Tag == "math_block"
	format := t.mathOutput
	var b strings.Builder
	for _, r := range node.astext() {
		if s, ok := uni2tex[r]; ok {
			b.WriteString(s)
		} else {
			b.WriteRune(r)
		}
	}
	mathCode := b.String()
	switch format {
	case "mathjax":
		if len(t.mathHeader) == 0 {
			mathjaxURL := "file:/usr/share/javascript/mathjax/MathJax.js"
			if t.mathOptions != "" {
				mathjaxURL = t.mathOptions
			} else {
				t.doc.reporter.warning("No MathJax URL specified, using local fallback (see config.html).", nil, baseNode(node))
			}
			if !strings.Contains(mathjaxURL, "?") {
				mathjaxURL += "?config=TeX-AMS_CHTML"
			}
			t.mathHeader = []string{`<script type="text/javascript" src="` + mathjaxURL + `"></script>` + "\n"}
		}
		if isBlock {
			mathCode = wrapMathCode(mathCode, true)
		} else {
			mathCode = `\(` + mathCode + `\)`
		}
		mathCode = t.encode(mathCode)
	case "mathml":
		if t.mathOptions != "" {
			// external converters (latexml, ttm, blahtexml, pandoc) are not
			// supported; fall back to LaTeX source
			format = "latex"
			mathCode = t.encode(mathCode)
			break
		}
		converted, errStr := tex2mathml(mathCode, isBlock)
		if errStr != "" {
			t.messages = append(t.messages, t.doc.reporter.warning(errStr, nil, baseNode(node)))
			mathCode = t.encode(node.astext())
			if t.settings.reportLevel <= 2 {
				format = "problematic"
			} else {
				format = "latex"
			}
		} else {
			mathCode = converted
		}
	case "html":
		if t.mathOptions != "" && len(t.mathHeader) == 0 {
			for _, sheet := range strings.Split(t.mathOptions, ",") {
				t.mathHeader = append(t.mathHeader, t.stylesheetCall(sheet, true))
			}
		}
		converted, traces, crash := math2html(wrapMathCode(mathCode, isBlock), isBlock)
		// math2html reports problems on stderr ("* message")
		if stream := t.doc.reporter.stream; stream != nil {
			for _, msg := range traces {
				io.WriteString(stream, msg+"\n")
			}
		}
		if crash != "" {
			panic(&mathCrashError{text: crash})
		}
		mathCode = converted
	default:
		format = "latex"
		mathCode = t.encode(mathCode)
	}
	var tag string
	switch format {
	case "html":
		tag = "span"
		if isBlock {
			tag = "div"
		}
	case "latex":
		tag = "tt"
		if isBlock {
			tag = "pre"
		}
	case "mathjax":
		tag = "span"
		if isBlock {
			tag = "div"
		}
	case "mathml":
		if isBlock {
			tag = "div"
		}
	case "problematic":
		tag = "span"
		if isBlock {
			tag = "pre"
		}
	}
	suffix := ""
	if isBlock {
		suffix = "\n"
	}
	if tag != "" {
		t.outTag(node, tag, suffix, A{"classes_ptr": t.mathClasses[format]})
	}
	t.out(mathCode, suffix)
	if tag != "" {
		t.out("</" + tag + ">" + suffix)
	}
	return actSkipChildren
}

// uri2path mirrors DoctreeTranslator.uri2path.
func (t *htmlTranslator) uri2path(uri string) (string, string) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Sprintf("Cannot get file path corresponding to %s.", uri)
	}
	if u.Scheme == "file" {
		return u.Path, ""
	}
	if u.Scheme != "" {
		return "", fmt.Sprintf("Cannot get file path corresponding to %s.", uri)
	}
	p := u.Path
	if t.settings.rootPrefix != "" && strings.HasPrefix(p, "/") {
		return filepath.Join(t.settings.rootPrefix, strings.TrimPrefix(p, "/")), ""
	}
	if t.settings.destPath != "" && !filepath.IsAbs(p) {
		destDir, _ := filepath.Abs(filepath.Dir(t.settings.destPath))
		p = relativePath(filepath.Join(destDir, p))
	}
	return p, ""
}

func (t *htmlTranslator) embedImage(uri, mimetype string) (data string, mt string, errStr string) {
	p, e := t.uri2path(uri)
	if e != "" {
		return "", "", e
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", "", strings.TrimPrefix(ioErrorString(err, p), "InputError: ")
	}
	if mimetype == "" {
		mimetype = "None"
	}
	return base64.StdEncoding.EncodeToString(raw), mimetype, ""
}
