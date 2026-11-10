// Package cli implements the semanticfont command: it reads text, scores
// every word with lexiscore and renders the result with renderer.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bijaya777pokhrel-hue/dark-factory-lab/lexiscore"
	"github.com/bijaya777pokhrel-hue/dark-factory-lab/renderer"
)

const usage = `Usage: semanticfont [-h | --help] [--] [FILE | -]

Reads text from FILE, or from standard input when FILE is "-" or absent,
and writes one self-contained HTML page to standard output in which every
word's typography carries its valence, salience, surprise and certainty.

Exit status: 0 on success, 1 on a read or write error, 2 on a usage error.
`

// Run reads text from stdin, or from the file named by the single argument,
// and writes one self-contained HTML page to stdout. It returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var operands []string
	options := true
	for _, a := range args {
		switch {
		case options && a == "--":
			options = false
		case options && (a == "-h" || a == "--help"):
			if _, err := io.WriteString(stdout, usage); err != nil {
				return fail(stderr, 1, "writing output: %v", err)
			}
			return 0
		case options && a != "-" && strings.HasPrefix(a, "-"):
			return fail(stderr, 2, "unknown option %q (try --help)", a)
		default:
			operands = append(operands, a)
		}
	}
	if len(operands) > 1 {
		return fail(stderr, 2, "expected at most one FILE, got %d (try --help)", len(operands))
	}

	var data []byte
	var err error
	if len(operands) == 0 || operands[0] == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(operands[0])
	}
	if err != nil {
		return fail(stderr, 1, "reading input: %v", err)
	}

	page := renderer.Render(convert(lexiscore.Analyze(string(data))))
	if _, err := io.WriteString(stdout, page); err != nil {
		return fail(stderr, 1, "writing output: %v", err)
	}
	return 0
}

// convert maps lexiscore tokens onto renderer tokens; the two packages
// deliberately do not import each other.
func convert(in []lexiscore.Token) []renderer.Token {
	out := make([]renderer.Token, len(in))
	for i, t := range in {
		out[i] = renderer.Token{
			Text:      t.Text,
			Word:      t.Word,
			Valence:   t.Scores.Valence,
			Salience:  t.Scores.Salience,
			Surprise:  t.Scores.Surprise,
			Certainty: t.Scores.Certainty,
		}
	}
	return out
}

// fail prints one line, prefixed with the command name, to stderr and returns code.
func fail(stderr io.Writer, code int, format string, a ...any) int {
	msg := fmt.Sprintf(format, a...)
	msg = strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(msg)
	_, _ = fmt.Fprintln(stderr, "semanticfont: "+msg)
	return code
}
