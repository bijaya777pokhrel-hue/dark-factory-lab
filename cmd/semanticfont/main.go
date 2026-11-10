// Command semanticfont renders text as an HTML page whose typography carries
// each word's sentiment scores. All logic lives in internal/cli.
package main

import (
	"os"

	"github.com/bijaya777pokhrel-hue/dark-factory-lab/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
