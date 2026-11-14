// Command rst2html5 converts a
// reStructuredText document to HTML (Docutils html5_polyglot writer).
package main

import (
	"os"

	"github.com/jdkato/rstgo/internal/cli"
)

func main() { os.Exit(cli.Main("rst2html5", os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
