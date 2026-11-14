// Command rst2html converts a
// reStructuredText document to HTML (Docutils html4css1 writer).
package main

import (
	"os"

	"github.com/jdkato/rstgo/internal/cli"
)

func main() { os.Exit(cli.Main("rst2html", os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
