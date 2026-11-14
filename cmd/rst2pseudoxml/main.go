// Command rst2pseudoxml prints the Docutils pseudo-XML representation of a
// reStructuredText document.
package main

import (
	"os"

	"github.com/jdkato/rstgo/internal/cli"
)

func main() { os.Exit(cli.Main("rst2pseudoxml", os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
