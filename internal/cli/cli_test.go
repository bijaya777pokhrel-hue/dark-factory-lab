package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"

	"github.com/bijaya777pokhrel-hue/dark-factory-lab/internal/cli"
)

const targetSentence = "The meal was perhaps good, but dessert was horrible."

// failingWriter rejects every write, like a closed or full stdout.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write refused") }

func TestRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	textFile := filepath.Join(dir, "in put.txt") // a space in the name on purpose
	if err := os.WriteFile(textFile, []byte("Truly wonderful news."), 0o600); err != nil {
		t.Fatal(err)
	}
	dashFile := filepath.Join(dir, "-dash.txt")
	if err := os.WriteFile(dashFile, []byte("Bad luck"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does-not-exist.txt")

	tests := []struct {
		name       string
		args       []string
		stdin      io.Reader
		failStdout bool
		wantCode   int
		wantSpans  int      // words in the input; -1 when stdout must be empty
		wantParas  int      // number of <p> elements; -1 to skip
		wantOut    []string // substrings stdout must contain
		notOut     []string // substrings stdout must not contain
		wantErr    bool     // stderr must hold exactly one non-empty line
	}{
		{name: "stdin text", stdin: strings.NewReader(targetSentence), wantSpans: 9, wantParas: 1,
			wantOut: []string{">The</span>", ">horrible</span>.", ">good</span>,"}},
		{name: "dash reads stdin", args: []string{"-"}, stdin: strings.NewReader("good day"), wantSpans: 2, wantParas: 1,
			wantOut: []string{">good</span>", ">day</span>"}},
		{name: "file path", args: []string{textFile}, stdin: strings.NewReader("ignored stdin words here"), wantSpans: 3, wantParas: 1,
			wantOut: []string{">Truly</span>", ">wonderful</span>", ">news</span>."}, notOut: []string{"ignored"}},
		{name: "double dash ends options", args: []string{"--", dashFile}, stdin: strings.NewReader(""), wantSpans: 2, wantParas: 1,
			wantOut: []string{">Bad</span>", ">luck</span>"}},
		{name: "help long", args: []string{"--help"}, stdin: strings.NewReader("x"), wantSpans: 0, wantParas: 0,
			wantOut: []string{"Usage", "semanticfont"}, notOut: []string{"<!doctype html>"}},
		{name: "help short", args: []string{"-h"}, stdin: strings.NewReader("x"), wantSpans: 0, wantParas: 0,
			wantOut: []string{"Usage", "semanticfont"}, notOut: []string{"<!doctype html>"}},
		{name: "unknown flag", args: []string{"-x"}, stdin: strings.NewReader("good"), wantCode: 2, wantSpans: -1, wantErr: true},
		{name: "unknown long flag", args: []string{"--verbose"}, stdin: strings.NewReader("good"), wantCode: 2, wantSpans: -1, wantErr: true},
		{name: "two operands", args: []string{textFile, textFile}, stdin: strings.NewReader("good"), wantCode: 2, wantSpans: -1, wantErr: true},
		{name: "two operands after double dash", args: []string{"--", "-", textFile}, stdin: strings.NewReader("good"), wantCode: 2, wantSpans: -1, wantErr: true},
		{name: "missing file", args: []string{missing}, stdin: strings.NewReader("good"), wantCode: 1, wantSpans: -1, wantErr: true},
		{name: "directory", args: []string{dir}, stdin: strings.NewReader("good"), wantCode: 1, wantSpans: -1, wantErr: true},
		{name: "reader error", stdin: iotest.ErrReader(errors.New("disk on fire")), wantCode: 1, wantSpans: -1, wantErr: true},
		{name: "reader error after data", stdin: io.MultiReader(strings.NewReader("good "), iotest.ErrReader(errors.New("cut"))), wantCode: 1, wantSpans: -1, wantErr: true},
		{name: "writer error", stdin: strings.NewReader(targetSentence), failStdout: true, wantCode: 1, wantSpans: -1, wantErr: true},
		{name: "empty input", stdin: strings.NewReader(""), wantSpans: 0, wantParas: 0,
			wantOut: []string{"<!doctype html>", "<main>", "</main>", "</html>"}},
		{name: "invalid UTF-8", stdin: strings.NewReader("\xff\xfe good \xc3 bad\x80"), wantSpans: 2, wantParas: 1,
			wantOut: []string{">good</span>", ">bad</span>"}},
		{name: "CRLF paragraphs", stdin: strings.NewReader("Good food.\r\n\r\nBad day.\r\n"), wantSpans: 4, wantParas: 2,
			notOut: []string{"\r"}},
		{name: "script is inert text", stdin: strings.NewReader("<script>alert(1)</script>"), wantSpans: 3, wantParas: 1,
			wantOut: []string{"&lt;", "&gt;"}, notOut: []string{"<script", "<SCRIPT"}},
		{name: "multi paragraph", stdin: strings.NewReader("One good day.\n\nTwo bad days.\n\nThree."), wantSpans: 7, wantParas: 3},
		{name: "punctuation only", stdin: strings.NewReader("?!... --- ,,,"), wantSpans: 0, wantParas: 1},
		{name: "apostrophes", stdin: strings.NewReader("It's not the dog’s fault"), wantSpans: 5, wantParas: 1,
			wantOut: []string{">It&#39;s</span>", ">dog’s</span>"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			var out io.Writer = &stdout
			if tc.failStdout {
				out = failingWriter{}
			}
			code := cli.Run(tc.args, tc.stdin, out, &stderr)
			got := stdout.String()

			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if tc.wantErr {
				msg := stderr.String()
				if strings.TrimSpace(msg) == "" || strings.Count(msg, "\n") != 1 || !strings.HasSuffix(msg, "\n") {
					t.Errorf("stderr = %q, want exactly one non-empty line", msg)
				}
			} else if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty on success", stderr.String())
			}
			if tc.wantSpans < 0 {
				if got != "" {
					t.Errorf("stdout = %q, want nothing on error", got)
				}
				return
			}
			if !utf8.ValidString(got) {
				t.Errorf("stdout is not valid UTF-8")
			}
			if tc.name != "help long" && tc.name != "help short" {
				if !strings.HasPrefix(got, "<!doctype html>") || !strings.HasSuffix(strings.TrimSpace(got), "</html>") {
					t.Errorf("stdout is not one complete HTML page:\n%s", got)
				}
			}
			if n := strings.Count(got, "<span "); n != tc.wantSpans {
				t.Errorf("<span> count = %d, want %d (one per word)", n, tc.wantSpans)
			}
			if tc.wantParas >= 0 {
				if n := strings.Count(got, "<p>"); n != tc.wantParas {
					t.Errorf("<p> count = %d, want %d", n, tc.wantParas)
				}
			}
			for _, s := range tc.wantOut {
				if !strings.Contains(got, s) {
					t.Errorf("stdout lacks %q", s)
				}
			}
			for _, s := range tc.notOut {
				if strings.Contains(got, s) {
					t.Errorf("stdout contains %q", s)
				}
			}
		})
	}
}

// ---- end-to-end comparison with spec/target.html ----

var (
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	paraRe    = regexp.MustCompile(`(?s)<p>(.*?)</p>`)
	spanRe    = regexp.MustCompile(`<span[^>]*\sstyle="([^"]*)"[^>]*>([^<]*)</span>`)
	tagRe     = regexp.MustCompile(`<[^>]*>`)
	degRe     = regexp.MustCompile(`^oblique (-?[0-9.]+)deg$`)
	hexRe     = regexp.MustCompile(`^#([0-9a-fA-F]{2})[0-9a-fA-F]{2}([0-9a-fA-F]{2})$`)
)

// styledWord is one <span>: its text and its six style scalars.
type styledWord struct {
	text   string
	values [6]float64
}

// property describes one compared style property: its name, its full range,
// and how its CSS value becomes a scalar.
type property struct {
	name  string
	css   string
	rng   float64
	scale func(string) (float64, error)
}

func plainNumber(suffix string) func(string) (float64, error) {
	return func(v string) (float64, error) {
		if !strings.HasSuffix(v, suffix) {
			return 0, fmt.Errorf("%q lacks suffix %q", v, suffix)
		}
		return strconv.ParseFloat(strings.TrimSuffix(v, suffix), 64)
	}
}

var properties = []property{
	{"weight", "font-weight", 500, plainNumber("")},
	{"size", "font-size", 0.5, plainNumber("em")},
	{"colour", "color", 325, func(v string) (float64, error) {
		m := hexRe.FindStringSubmatch(v)
		if m == nil {
			return 0, fmt.Errorf("colour %q is not #rrggbb", v)
		}
		r, _ := strconv.ParseUint(m[1], 16, 8)
		b, _ := strconv.ParseUint(m[2], 16, 8)
		return float64(r) - float64(b), nil
	}},
	{"slant", "font-style", 14, func(v string) (float64, error) {
		if v == "normal" {
			return 0, nil
		}
		m := degRe.FindStringSubmatch(v)
		if m == nil {
			return 0, fmt.Errorf("font-style %q is neither normal nor oblique Ndeg", v)
		}
		return strconv.ParseFloat(m[1], 64)
	}},
	{"spacing", "letter-spacing", 0.12, plainNumber("em")},
	{"opacity", "opacity", 0.2, plainNumber("")},
}

// paragraphHTML returns the inner HTML of every <p>, comments removed.
func paragraphHTML(page string) string {
	page = commentRe.ReplaceAllString(page, "")
	var parts []string
	for _, m := range paraRe.FindAllStringSubmatch(page, -1) {
		parts = append(parts, m[1])
	}
	return strings.Join(parts, "\n\n")
}

// sentenceOf returns the visible text of the page's paragraphs: each span's
// text plus the plain text between spans.
func sentenceOf(page string) string {
	return html.UnescapeString(tagRe.ReplaceAllString(paragraphHTML(page), ""))
}

// styledWords parses every span of the page into its word and six scalars.
func styledWords(page string) ([]styledWord, error) {
	var out []styledWord
	for _, m := range spanRe.FindAllStringSubmatch(paragraphHTML(page), -1) {
		decls := map[string]string{}
		for decl := range strings.SplitSeq(m[1], ";") {
			name, value, ok := strings.Cut(decl, ":")
			if ok {
				decls[strings.TrimSpace(name)] = strings.TrimSpace(value)
			}
		}
		w := styledWord{text: html.UnescapeString(m[2])}
		for i, p := range properties {
			v, ok := decls[p.css]
			if !ok {
				return nil, fmt.Errorf("word %q has no %s", w.text, p.css)
			}
			x, err := p.scale(v)
			if err != nil {
				return nil, fmt.Errorf("word %q %s: %w", w.text, p.css, err)
			}
			w.values[i] = x
		}
		out = append(out, w)
	}
	return out, nil
}

// compareStyles checks that got makes the same relative styling choices as
// want and returns one message per violation.
func compareStyles(want, got []styledWord) []string {
	if len(want) != len(got) {
		return []string{fmt.Sprintf("word count: target has %d, output has %d", len(want), len(got))}
	}
	var failures []string
	for i := range want {
		if want[i].text != got[i].text {
			failures = append(failures, fmt.Sprintf("word %d: target %q, output %q", i+1, want[i].text, got[i].text))
		}
	}
	if failures != nil {
		return failures
	}
	label := func(i int) string { return fmt.Sprintf("%q (word %d)", want[i].text, i+1) }
	for p, prop := range properties {
		for i := range want {
			for j := i + 1; j < len(want); j++ {
				dt := want[j].values[p] - want[i].values[p]
				dg := got[j].values[p] - got[i].values[p]
				switch {
				case dt > prop.rng/20 && dg <= 0, dt < -prop.rng/20 && dg >= 0:
					failures = append(failures, fmt.Sprintf("%s: %s vs %s: target %g -> %g, output %g -> %g (direction differs)",
						prop.name, label(i), label(j), want[i].values[p], want[j].values[p], got[i].values[p], got[j].values[p]))
				case dt >= -prop.rng/20 && dt <= prop.rng/20 && (dg > prop.rng/5 || dg < -prop.rng/5):
					failures = append(failures, fmt.Sprintf("%s: %s vs %s: target ties (%g, %g), output %g and %g differ by more than %g",
						prop.name, label(i), label(j), want[i].values[p], want[j].values[p], got[i].values[p], got[j].values[p], prop.rng/5))
				}
			}
		}
	}
	return failures
}

func readTarget(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "spec", "target.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRun_matchesTarget(t *testing.T) {
	t.Parallel()

	target := readTarget(t)
	sentence := sentenceOf(target)
	if sentence != targetSentence {
		t.Fatalf("extracted target sentence %q, want %q", sentence, targetSentence)
	}
	want, err := styledWords(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 9 {
		t.Fatalf("target has %d styled words, want 9", len(want))
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(nil, strings.NewReader(sentence), &stdout, &stderr); code != 0 {
		t.Fatalf("Run exit %d, stderr %q", code, stderr.String())
	}
	if got := sentenceOf(stdout.String()); got != sentence {
		t.Errorf("output text %q, want %q", got, sentence)
	}
	rendered, err := styledWords(stdout.String())
	if err != nil {
		t.Fatal(err)
	}

	// swap returns a copy of ws with the styles of words a and b exchanged.
	swap := func(ws []styledWord, a, b int) []styledWord {
		out := append([]styledWord(nil), ws...)
		out[a].values, out[b].values = out[b].values, out[a].values
		return out
	}
	// set returns a copy of ws with property p of word i set to v.
	set := func(ws []styledWord, i, p int, v float64) []styledWord {
		out := append([]styledWord(nil), ws...)
		out[i].values[p] = v
		return out
	}

	tests := []struct {
		name string
		got  []styledWord
		want []string // substrings the failures must contain; nil means no failure allowed
	}{
		{name: "Run output matches the target", got: rendered},
		{name: "target matches itself", got: want},
		{name: "swapped good and horrible styles", got: swap(want, 4, 8),
			want: []string{`colour: "good" (word 5) vs "horrible" (word 9)`, "direction differs"}},
		{name: "broken weight tie between The and was", got: set(want, 0, 0, 800),
			want: []string{`weight: "The" (word 1) vs "meal" (word 2)`, `weight: "The" (word 1) vs "was" (word 3)`}},
		{name: "hedge not slanted", got: set(want, 3, 3, 0),
			want: []string{`slant: "perhaps" (word 4)`}},
		{name: "surprise spacing on the wrong word", got: swap(want, 7, 8),
			want: []string{`"was" (word 8) vs "horrible" (word 9)`}},
		{name: "words reordered", got: func() []styledWord {
			out := append([]styledWord(nil), want...)
			out[0], out[1] = out[1], out[0]
			return out
		}(), want: []string{`word 1: target "The", output "meal"`}},
		{name: "word missing", got: want[:8], want: []string{"word count: target has 9, output has 8"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failures := compareStyles(want, tc.got)
			if tc.want == nil {
				for _, f := range failures {
					t.Error(f)
				}
				return
			}
			if len(failures) == 0 {
				t.Fatal("comparison accepted a deliberately wrong output")
			}
			all := strings.Join(failures, "\n")
			for _, s := range tc.want {
				if !strings.Contains(all, s) {
					t.Errorf("failures lack %q; got:\n%s", s, all)
				}
			}
		})
	}
}
