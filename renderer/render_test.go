package renderer_test

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bijaya777pokhrel-hue/dark-factory-lab/renderer"
)

// The six style properties the spec defines, in the order the spec requires.
var sixProps = []string{"font-weight", "font-size", "color", "font-style", "letter-spacing", "opacity"}

// Documented page backgrounds: light from spec/target.html, dark from the renderer's CSS comment.
const (
	lightBackground = "#ffffff"
	darkBackground  = "#121212"
)

var spanRe = regexp.MustCompile(`<span data-valence="([^"]*)" data-salience="([^"]*)" data-surprise="([^"]*)" data-certainty="([^"]*)" style="([^"]*)">([^<]*)</span>`)

type prop struct{ name, value string }

type span struct {
	attrs [4]string // valence, salience, surprise, certainty as written
	props []prop
	text  string
}

func (s span) get(name string) string {
	for _, p := range s.props {
		if p.name == name {
			return p.value
		}
	}
	return ""
}

func parseStyle(style string) []prop {
	var out []prop
	for decl := range strings.SplitSeq(style, ";") {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		name, value, _ := strings.Cut(decl, ":")
		out = append(out, prop{strings.TrimSpace(name), strings.TrimSpace(value)})
	}
	return out
}

func parseSpans(html string) []span {
	var out []span
	for _, m := range spanRe.FindAllStringSubmatch(html, -1) {
		out = append(out, span{attrs: [4]string{m[1], m[2], m[3], m[4]}, props: parseStyle(m[5]), text: m[6]})
	}
	return out
}

// renderOne renders a single word and returns its parsed span.
func renderOne(t *testing.T, tok renderer.Token) span {
	t.Helper()
	tok.Word = true
	if tok.Text == "" {
		tok.Text = "w"
	}
	spans := parseSpans(renderer.Render([]renderer.Token{tok}))
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	return spans[0]
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("not a plain number: %q", s)
	}
	return v
}

func unit(t *testing.T, s, suffix string) float64 {
	t.Helper()
	if !strings.HasSuffix(s, suffix) {
		t.Fatalf("%q lacks unit %q", s, suffix)
	}
	return mustFloat(t, strings.TrimSuffix(s, suffix))
}

func slantDeg(t *testing.T, s string) float64 {
	t.Helper()
	if s == "normal" {
		return 0
	}
	if !strings.HasPrefix(s, "oblique ") {
		t.Fatalf("bad font-style %q", s)
	}
	return unit(t, strings.TrimPrefix(s, "oblique "), "deg")
}

// ---- WCAG helpers (independent of the renderer) ----

func hexRGB(t *testing.T, s string) [3]float64 {
	t.Helper()
	if len(s) != 7 || s[0] != '#' {
		t.Fatalf("bad colour %q", s)
	}
	var c [3]float64
	for i := range 3 {
		v, err := strconv.ParseUint(s[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("bad colour %q", s)
		}
		c[i] = float64(v)
	}
	return c
}

func linear(c float64) float64 {
	c /= 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func luminance(c [3]float64) float64 {
	return 0.2126*linear(c[0]) + 0.7152*linear(c[1]) + 0.0722*linear(c[2])
}

func contrast(a, b [3]float64) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// composite blends fg at alpha over bg (in sRGB, as browsers do for opacity).
func composite(fg, bg [3]float64, alpha float64) [3]float64 {
	var c [3]float64
	for i := range 3 {
		c[i] = alpha*fg[i] + (1-alpha)*bg[i]
	}
	return c
}

func colourDistance(a, b [3]float64) float64 {
	return math.Abs(a[0]-b[0]) + math.Abs(a[1]-b[1]) + math.Abs(a[2]-b[2])
}

// ---- page structure helpers ----

func styleBlock(t *testing.T, html string) string {
	t.Helper()
	_, rest, ok := strings.Cut(html, "<style>")
	if !ok {
		t.Fatal("no <style>")
	}
	css, _, ok := strings.Cut(rest, "</style>")
	if !ok {
		t.Fatal("unterminated <style>")
	}
	return css
}

// mediaBlock returns the body of the first @media rule whose header starts with prefix.
func mediaBlock(t *testing.T, css, prefix string) string {
	t.Helper()
	i := strings.Index(css, prefix)
	if i < 0 {
		t.Fatalf("no %q block", prefix)
	}
	open := strings.Index(css[i:], "{")
	if open < 0 {
		t.Fatalf("no body for %q", prefix)
	}
	start := i + open + 1
	depth := 1
	for j := start; j < len(css); j++ {
		switch css[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[start:j]
			}
		}
	}
	t.Fatalf("unbalanced %q block", prefix)
	return ""
}

var commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

func stripComments(css string) string { return commentRe.ReplaceAllString(css, "") }

func normalizeSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func mainInner(t *testing.T, html string) string {
	t.Helper()
	if strings.Count(html, "<main>") != 1 || strings.Count(html, "</main>") != 1 {
		t.Fatal("want exactly one <main>")
	}
	_, rest, _ := strings.Cut(html, "<main>")
	inner, _, _ := strings.Cut(rest, "</main>")
	return inner
}

var paraRe = regexp.MustCompile(`(?s)<p>(.*?)</p>`)

// paragraphs returns each paragraph with spans written as [text], <br> kept, whitespace collapsed.
func paragraphs(t *testing.T, html string) []string {
	t.Helper()
	inner := mainInner(t, html)
	var out []string
	for _, m := range paraRe.FindAllStringSubmatch(inner, -1) {
		p := spanRe.ReplaceAllString(m[1], "[$6]")
		p = strings.ReplaceAll(p, "<br>", " <br> ")
		out = append(out, normalizeSpace(p))
	}
	if rest := normalizeSpace(paraRe.ReplaceAllString(inner, "")); rest != "" {
		t.Fatalf("content outside paragraphs: %q", rest)
	}
	return out
}

func w(text string) renderer.Token {
	return renderer.Token{Text: text, Word: true, Certainty: 0.5}
}

func gap(text string) renderer.Token { return renderer.Token{Text: text} }

// ---- tests ----

// TestRender_reproducesTargetStyles checks the formulas against the human-approved spec/target.html.
func TestRender_reproducesTargetStyles(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../spec/target.html")
	if err != nil {
		t.Fatal(err)
	}
	target := parseSpans(string(raw))
	if len(target) == 0 {
		t.Fatal("no spans parsed from target")
	}
	var tokens []renderer.Token
	for i, s := range target {
		if i > 0 {
			tokens = append(tokens, gap(" "))
		}
		tokens = append(tokens, renderer.Token{
			Text: s.text, Word: true,
			Valence: mustFloat(t, s.attrs[0]), Salience: mustFloat(t, s.attrs[1]),
			Surprise: mustFloat(t, s.attrs[2]), Certainty: mustFloat(t, s.attrs[3]),
		})
	}
	got := parseSpans(renderer.Render(tokens))
	if len(got) != len(target) {
		t.Fatalf("got %d spans, target has %d", len(got), len(target))
	}
	for i, want := range target {
		t.Run(fmt.Sprintf("%d_%s", i, want.text), func(t *testing.T) {
			t.Parallel()
			g := got[i]
			if g.text != want.text {
				t.Errorf("text %q, want %q", g.text, want.text)
			}
			if g.attrs != want.attrs {
				t.Errorf("data attributes %v, want %v", g.attrs, want.attrs)
			}
			if len(g.props) < 6 {
				t.Fatalf("only %d style properties", len(g.props))
			}
			for j, wp := range want.props {
				if g.props[j] != wp {
					t.Errorf("property %d = %q, want %q", j, g.props[j], wp)
				}
			}
		})
	}
}

// TestRender_styleMapping checks scores the target does not contain against the spec's formulas.
func TestRender_styleMapping(t *testing.T) {
	t.Parallel()
	nan, inf := math.NaN(), math.Inf(1)
	tests := []struct {
		name  string
		tok   renderer.Token
		attrs [4]string
		want  [6]string
	}{
		{"all zero (most hedged)", renderer.Token{},
			[4]string{"0", "0", "0", "0"},
			[6]string{"300", "0.9em", "#1c1c1c", "oblique 14deg", "0em", "0.8"}},
		{"assertive positive extreme", renderer.Token{Valence: 1, Salience: 1, Surprise: 1, Certainty: 1},
			[4]string{"1", "1", "1", "1"},
			[6]string{"800", "1.4em", "#0b4fa8", "normal", "0.12em", "1"}},
		{"negative extreme", renderer.Token{Valence: -1, Certainty: 0.5},
			[4]string{"-1", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#a82e00", "normal", "0em", "0.9"}},
		{"positive quarter (t=0.5)", renderer.Token{Valence: 0.25, Certainty: 0.5},
			[4]string{"0.25", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#143662", "normal", "0em", "0.9"}},
		{"negative quarter (t=0.5)", renderer.Token{Valence: -0.25, Certainty: 0.5},
			[4]string{"-0.25", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#62250e", "normal", "0em", "0.9"}},
		{"half-up rounding", renderer.Token{Salience: 0.05, Surprise: 0.0125, Certainty: 0.25},
			[4]string{"0", "0.05", "0.0125", "0.25"},
			[6]string{"350", "0.93em", "#1c1c1c", "oblique 7deg", "0.002em", "0.85"}},
		{"slant rounds to zero is normal", renderer.Token{Certainty: 0.49},
			[4]string{"0", "0", "0", "0.49"},
			[6]string{"300", "0.9em", "#1c1c1c", "normal", "0em", "0.9"}},
		{"slant just below", renderer.Token{Certainty: 0.48},
			[4]string{"0", "0", "0", "0.48"},
			[6]string{"300", "0.9em", "#1c1c1c", "oblique 1deg", "0em", "0.9"}},
		{"out of range clamps", renderer.Token{Valence: 5, Salience: -3, Surprise: 7, Certainty: -2},
			[4]string{"1", "0", "1", "0"},
			[6]string{"300", "0.9em", "#0b4fa8", "oblique 14deg", "0.12em", "0.8"}},
		{"out of range clamps high", renderer.Token{Valence: -9, Salience: 4, Surprise: -1, Certainty: 3},
			[4]string{"-1", "1", "0", "1"},
			[6]string{"800", "1.4em", "#a82e00", "normal", "0em", "1"}},
		{"NaN is neutral", renderer.Token{Valence: nan, Salience: nan, Surprise: nan, Certainty: nan},
			[4]string{"0", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#1c1c1c", "normal", "0em", "0.9"}},
		{"+Inf is neutral", renderer.Token{Valence: inf, Salience: inf, Surprise: inf, Certainty: inf},
			[4]string{"0", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#1c1c1c", "normal", "0em", "0.9"}},
		{"-Inf is neutral", renderer.Token{Valence: -inf, Salience: -inf, Surprise: -inf, Certainty: -inf},
			[4]string{"0", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#1c1c1c", "normal", "0em", "0.9"}},
		{"negative zero is plain zero", renderer.Token{Valence: math.Copysign(0, -1), Certainty: 0.5},
			[4]string{"0", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#1c1c1c", "normal", "0em", "0.9"}},
		{"tiny value is not exponent notation", renderer.Token{Valence: 1e-7, Certainty: 0.5},
			[4]string{"0.0000001", "0", "0", "0.5"},
			[6]string{"300", "0.9em", "#1c1c1c", "normal", "0em", "0.9"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := renderOne(t, tc.tok)
			if s.attrs != tc.attrs {
				t.Errorf("data attributes %v, want %v", s.attrs, tc.attrs)
			}
			for i, name := range sixProps {
				if i >= len(s.props) || s.props[i].name != name || s.props[i].value != tc.want[i] {
					t.Errorf("property %d: got %v, want %s: %s", i, s.props, name, tc.want[i])
				}
			}
		})
	}
}

// TestRender_monotonicChannels sweeps each score and asserts documented ranges and direction.
func TestRender_monotonicChannels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		make  func(x float64) renderer.Token
		from  float64
		to    float64
		check func(t *testing.T, prev, cur span, x float64)
	}{
		{"weight and size rise with salience", func(x float64) renderer.Token {
			return renderer.Token{Salience: x, Certainty: 0.5}
		}, 0, 1, func(t *testing.T, prev, cur span, x float64) {
			wt := mustFloat(t, cur.get("font-weight"))
			sz := unit(t, cur.get("font-size"), "em")
			if wt < 300 || wt > 800 || math.Mod(wt, 50) != 0 {
				t.Errorf("salience %v: weight %v outside 300..800 or not a multiple of 50", x, wt)
			}
			if sz < 0.9 || sz > 1.4 {
				t.Errorf("salience %v: size %v outside 0.9..1.4", x, sz)
			}
			if wt < mustFloat(t, prev.get("font-weight")) || sz < unit(t, prev.get("font-size"), "em") {
				t.Errorf("salience %v: weight or size fell", x)
			}
		}},
		{"spacing rises with surprise", func(x float64) renderer.Token {
			return renderer.Token{Surprise: x, Certainty: 0.5}
		}, 0, 1, func(t *testing.T, prev, cur span, x float64) {
			sp := unit(t, cur.get("letter-spacing"), "em")
			if sp < 0 || sp > 0.12 {
				t.Errorf("surprise %v: spacing %v outside 0..0.12", x, sp)
			}
			if sp < unit(t, prev.get("letter-spacing"), "em") {
				t.Errorf("surprise %v: spacing fell", x)
			}
		}},
		{"opacity rises and slant falls with certainty", func(x float64) renderer.Token {
			return renderer.Token{Certainty: x}
		}, 0, 1, func(t *testing.T, prev, cur span, x float64) {
			op := mustFloat(t, cur.get("opacity"))
			sl := slantDeg(t, cur.get("font-style"))
			if op < 0.8 || op > 1 {
				t.Errorf("certainty %v: opacity %v outside 0.8..1", x, op)
			}
			if sl < 0 || sl > 14 {
				t.Errorf("certainty %v: slant %v outside 0..14", x, sl)
			}
			if x >= 0.5 && sl != 0 {
				t.Errorf("certainty %v: slanted (%v) at or above 0.5", x, sl)
			}
			if x < 0.4 && sl == 0 {
				t.Errorf("certainty %v: hedged word is upright", x)
			}
			if op < mustFloat(t, prev.get("opacity")) || sl > slantDeg(t, prev.get("font-style")) {
				t.Errorf("certainty %v: opacity fell or slant rose", x)
			}
		}},
		{"colour moves vermilion -> ink -> blue with valence", func(x float64) renderer.Token {
			return renderer.Token{Valence: x, Certainty: 0.5}
		}, -1, 1, func(t *testing.T, prev, cur span, x float64) {
			verm, ink, blue := [3]float64{0xa8, 0x2e, 0x00}, [3]float64{0x1c, 0x1c, 0x1c}, [3]float64{0x0b, 0x4f, 0xa8}
			c, p := hexRGB(t, cur.get("color")), hexRGB(t, prev.get("color"))
			switch {
			case x <= 0:
				if colourDistance(c, verm) < colourDistance(p, verm) || colourDistance(c, ink) > colourDistance(p, ink) {
					t.Errorf("valence %v: colour %v did not move from vermilion toward ink", x, c)
				}
			default:
				if colourDistance(c, blue) > colourDistance(p, blue) || colourDistance(c, ink) < colourDistance(p, ink) {
					t.Errorf("valence %v: colour %v did not move from ink toward blue", x, c)
				}
			}
			if x == -1 && c != verm || x == 0 && c != ink || x == 1 && c != blue {
				t.Errorf("valence %v: colour %v is not the anchor", x, c)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const steps = 200
			prev := renderOne(t, tc.make(tc.from))
			tc.check(t, prev, prev, tc.from)
			for i := 1; i <= steps; i++ {
				x := tc.from + (tc.to-tc.from)*float64(i)/steps
				x = math.Round(x*1e6) / 1e6
				cur := renderOne(t, tc.make(x))
				tc.check(t, prev, cur, x)
				prev = cur
			}
			first, last := renderOne(t, tc.make(tc.from)), renderOne(t, tc.make(tc.to))
			same := true
			for i := range 6 {
				if first.props[i] != last.props[i] {
					same = false
				}
			}
			if same {
				t.Errorf("sweep from %v to %v changed nothing", tc.from, tc.to)
			}
		})
	}
}

// TestRender_page checks the page skeleton, self-containment and the target's page look.
func TestRender_page(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../spec/target.html")
	if err != nil {
		t.Fatal(err)
	}
	targetCSS := styleBlock(t, string(raw))
	tests := []struct {
		name   string
		tokens []renderer.Token
		empty  bool
	}{
		{"nil input", nil, true},
		{"empty slice", []renderer.Token{}, true},
		{"only blank text", []renderer.Token{gap(" \n\n \r\n ")}, true},
		{"sentence", []renderer.Token{w("Hello"), gap(", "), w("world"), gap(".")}, false},
	}
	required := []string{
		`<html lang="en">`,
		`<meta charset="utf-8">`,
		`<meta name="viewport" content="width=device-width, initial-scale=1">`,
		`<meta name="color-scheme" content="light dark">`,
	}
	forbidden := []string{"<script", "<link", "<img", "@import", "url(", "@font-face", "<iframe", "<object", "<embed", "src=", "href="}
	absUnit := regexp.MustCompile(`\d(px|pt|pc|cm|mm|in)\b`)
	titleRe := regexp.MustCompile(`<title>([^<]+)</title>`)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := renderer.Render(tc.tokens)
			if !strings.HasPrefix(strings.ToLower(out), "<!doctype html>") {
				t.Error("missing doctype")
			}
			for _, r := range required {
				if !strings.Contains(out, r) {
					t.Errorf("missing %s", r)
				}
			}
			if m := titleRe.FindStringSubmatch(out); m == nil || strings.TrimSpace(m[1]) == "" {
				t.Error("missing non-empty <title>")
			}
			if n := strings.Count(out, "<style"); n != 1 {
				t.Errorf("%d style blocks, want 1", n)
			}
			for _, f := range forbidden {
				if strings.Contains(strings.ToLower(out), f) {
					t.Errorf("contains external or active reference %q", f)
				}
			}
			if m := absUnit.FindString(out); m != "" {
				t.Errorf("absolute unit %q; only relative units allowed", m)
			}
			css := styleBlock(t, out)
			plain := normalizeSpace(stripComments(css))
			for rule := range strings.SplitSeq(stripComments(targetCSS), "}") {
				if r := normalizeSpace(rule); r != "" && !strings.Contains(plain, r+" }") {
					t.Errorf("target rule %q missing from page CSS", r)
				}
			}
			for _, want := range []string{"max-width: 40rem", "font-size: 2rem", "line-height: 1.6", "system-ui"} {
				if !strings.Contains(plain, want) {
					t.Errorf("page CSS lacks %q", want)
				}
			}
			inner := mainInner(t, out)
			if tc.empty && strings.TrimSpace(inner) != "" {
				t.Errorf("main should be empty, got %q", inner)
			}
			if !tc.empty && !strings.Contains(inner, "<p>") {
				t.Error("text is not inside a paragraph in <main>")
			}
			if !utf8.ValidString(out) {
				t.Error("output is not valid UTF-8")
			}
		})
	}
}

// TestRender_words checks one span per word with its real text and plain text between words.
func TestRender_words(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tokens []renderer.Token
		words  []string
		para   string
	}{
		{"sentence", []renderer.Token{w("The"), gap(" "), w("meal"), gap(", "), w("perhaps"), gap("!")},
			[]string{"The", "meal", "perhaps"}, "[The] [meal], [perhaps]!"},
		{"adjacent words", []renderer.Token{w("a"), w("b")}, []string{"a", "b"}, "[a][b]"},
		{"leading text", []renderer.Token{gap("« "), w("oui"), gap(" »")}, []string{"oui"}, "« [oui] »"},
		{"unicode word", []renderer.Token{w("naïve"), gap(" "), w("日本")}, []string{"naïve", "日本"}, "[naïve] [日本]"},
		{"empty word still one span", []renderer.Token{gap("x "), w(""), gap(" y")}, []string{""}, "x [] y"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := renderer.Render(tc.tokens)
			spans := parseSpans(out)
			if got := strings.Count(out, "<span"); got != len(tc.words) {
				t.Fatalf("%d spans, want %d", got, len(tc.words))
			}
			for i, s := range spans {
				if s.text != tc.words[i] {
					t.Errorf("span %d text %q, want %q", i, s.text, tc.words[i])
				}
				if len(s.props) < 6 {
					t.Errorf("span %d has %d style properties", i, len(s.props))
				}
				for j, name := range sixProps {
					if j < len(s.props) && s.props[j].name != name {
						t.Errorf("span %d property %d is %q, want %q", i, j, s.props[j].name, name)
					}
				}
			}
			if strings.Contains(out, "aria-hidden") || strings.Contains(styleBlock(t, out), "content:") {
				t.Error("text must be real, not hidden or generated")
			}
			ps := paragraphs(t, out)
			if len(ps) != 1 || ps[0] != tc.para {
				t.Errorf("paragraphs %q, want [%q]", ps, tc.para)
			}
		})
	}
}

// TestRender_escaping checks that all text is inert and attributes stay plain numbers.
func TestRender_escaping(t *testing.T) {
	t.Parallel()
	evil := `<script>alert(1)</script>`
	tests := []struct {
		name   string
		tokens []renderer.Token
		want   string
	}{
		{"script in word", []renderer.Token{w(evil)}, "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"script in text", []renderer.Token{w("a"), gap(evil)}, "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"attribute breakout in word", []renderer.Token{w(`" onmouseover="x`)}, "&#34; onmouseover=&#34;x"},
		{"closing tags in text", []renderer.Token{gap("</p></main><img src=x>")}, "&lt;/p&gt;&lt;/main&gt;&lt;img src=x&gt;"},
		{"ampersand", []renderer.Token{w("R&D"), gap(" &amp; ")}, "R&amp;D"},
		{"invalid utf-8", []renderer.Token{w("a\xffb"), gap("\xfe")}, "a�b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := renderer.Render(tc.tokens)
			if !strings.Contains(out, tc.want) {
				t.Errorf("escaped form %q not found", tc.want)
			}
			body := mainInner(t, out)
			for _, bad := range []string{"<script", "<img", "</p></main>", "onmouseover=\""} {
				if strings.Contains(body, bad) {
					t.Errorf("unescaped %q in output", bad)
				}
			}
			if strings.Count(out, "<main>") != 1 {
				t.Error("structure broken")
			}
			if !utf8.ValidString(out) {
				t.Error("output is not valid UTF-8")
			}
			for _, s := range parseSpans(out) {
				for _, a := range s.attrs {
					mustFloat(t, a)
				}
			}
		})
	}
}

// TestRender_paragraphs checks line breaks: blank lines split paragraphs, single breaks become <br>.
func TestRender_paragraphs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tokens []renderer.Token
		want   []string
	}{
		{"blank line LF", []renderer.Token{w("a"), gap("\n\n"), w("b")}, []string{"[a]", "[b]"}},
		{"blank line CRLF", []renderer.Token{w("a"), gap(".\r\n\r\nThen "), w("b")}, []string{"[a].", "Then [b]"}},
		{"blank line with spaces", []renderer.Token{w("a"), gap(" \n \t \n "), w("b")}, []string{"[a]", "[b]"}},
		{"many blank lines, no empty p", []renderer.Token{w("a"), gap("\n\n\n\n\n"), w("b")}, []string{"[a]", "[b]"}},
		{"break split across tokens", []renderer.Token{w("a"), gap("\n"), gap("\n"), w("b")}, []string{"[a]", "[b]"}},
		{"single LF is br", []renderer.Token{w("a"), gap("\n"), w("b")}, []string{"[a] <br> [b]"}},
		{"single CRLF is br", []renderer.Token{w("a"), gap(",\r\n"), w("b")}, []string{"[a], <br> [b]"}},
		{"two single breaks", []renderer.Token{w("a"), gap("\n"), w("b"), gap("\nc")}, []string{"[a] <br> [b] <br> c"}},
		{"leading blank lines", []renderer.Token{gap("\n\n\n"), w("a")}, []string{"[a]"}},
		{"leading single break", []renderer.Token{gap("\n"), w("a")}, []string{"[a]"}},
		{"trailing blank lines", []renderer.Token{w("a"), gap("\n\n")}, []string{"[a]"}},
		{"trailing single break", []renderer.Token{w("a"), gap("\n")}, []string{"[a]"}},
		{"text-only paragraphs", []renderer.Token{gap("one\n\ntwo\nthree")}, []string{"one", "two <br> three"}},
		{"three paragraphs", []renderer.Token{w("a"), gap("\n\n"), w("b"), gap("\r\n\r\n"), w("c")}, []string{"[a]", "[b]", "[c]"}},
		{"only breaks", []renderer.Token{gap("\n\n\r\n\n")}, nil},
	}
	emptyP := regexp.MustCompile(`<p>\s*(<br>\s*)*</p>`)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := renderer.Render(tc.tokens)
			got := paragraphs(t, out)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
				t.Errorf("paragraphs %q, want %q", got, tc.want)
			}
			if emptyP.MatchString(out) {
				t.Error("empty <p> emitted")
			}
		})
	}
}

// TestRender_contrast checks WCAG 4.5:1 over the full valence range at the opacity floor, in both themes.
func TestRender_contrast(t *testing.T) {
	t.Parallel()
	var tokens []renderer.Token
	for i := 0; i <= 4000; i++ {
		v := -1 + float64(i)/2000
		for _, c := range []float64{0, 0.5, 1} {
			tokens = append(tokens, renderer.Token{Text: "w", Word: true, Valence: v, Certainty: c}, gap(" "))
		}
	}
	out := renderer.Render(tokens)
	css := stripComments(styleBlock(t, out))
	dark := mediaBlock(t, css, "@media (prefers-color-scheme: dark)")
	spans := parseSpans(out)
	if len(spans) != len(tokens)/2 {
		t.Fatalf("%d spans, want %d", len(spans), len(tokens)/2)
	}
	tests := []struct {
		name     string
		bg       string
		colour   string
		bgSource string
	}{
		{"light", lightBackground, "color", css},
		{"dark", darkBackground, "--dark-color", dark},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(tc.bgSource, "background: "+tc.bg) {
				t.Fatalf("%s theme background %s not set in CSS", tc.name, tc.bg)
			}
			bg := hexRGB(t, tc.bg)
			worst := math.Inf(1)
			for _, s := range spans {
				fg := hexRGB(t, s.get(tc.colour))
				alpha := mustFloat(t, s.get("opacity"))
				if alpha < 0.8 {
					t.Fatalf("opacity %v below floor", alpha)
				}
				r := contrast(composite(fg, bg, alpha), bg)
				worst = math.Min(worst, r)
				if r < 4.5 {
					t.Errorf("%s: valence %s certainty %s gives %.2f:1", tc.name, s.attrs[0], s.attrs[3], r)
				}
			}
			t.Logf("%s theme worst contrast %.2f:1", tc.name, worst)
		})
	}
}

// TestRender_darkTheme checks the dark custom property, its easing and the media rule applying it.
func TestRender_darkTheme(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		valence float64
		want    string
	}{
		{"neutral is dark ink", 0, "#e6e6e6"},
		{"negative anchor", -1, "#ff9e7a"},
		{"positive anchor", 1, "#8cb8ff"},
		{"positive quarter eases to t=0.5", 0.25, "#b9cff3"},
		{"negative quarter eases to t=0.5", -0.25, "#f3c2b0"},
		{"NaN is neutral", math.NaN(), "#e6e6e6"},
		{"clamped", 4, "#8cb8ff"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := renderOne(t, renderer.Token{Valence: tc.valence, Certainty: 0.5})
			if len(s.props) != 7 || s.props[6].name != "--dark-color" {
				t.Fatalf("want --dark-color as the 7th property after the six, got %v", s.props)
			}
			if s.props[6].value != tc.want {
				t.Errorf("--dark-color %s, want %s", s.props[6].value, tc.want)
			}
			out := renderer.Render([]renderer.Token{w("x")})
			dark := normalizeSpace(mediaBlock(t, stripComments(styleBlock(t, out)), "@media (prefers-color-scheme: dark)"))
			if !strings.Contains(dark, "color: var(--dark-color) !important") {
				t.Errorf("dark media rule does not apply --dark-color with !important: %q", dark)
			}
		})
	}
}

// TestRender_userPreferences checks prefers-contrast, forced-colors and motion handling.
func TestRender_userPreferences(t *testing.T) {
	t.Parallel()
	out := renderer.Render([]renderer.Token{
		{Text: "maybe", Word: true, Certainty: 0},
		gap(" "),
		{Text: "sure", Word: true, Salience: 1, Surprise: 1, Certainty: 1},
	})
	rawCSS := styleBlock(t, out)
	css := stripComments(rawCSS)
	comments := strings.Join(commentRe.FindAllString(rawCSS, -1), " ")
	// Non-colour channels, per score: changing that score alone must change one of them.
	nonColour := []string{"font-weight", "font-size", "font-style", "letter-spacing", "opacity"}
	base := renderer.Token{Valence: 0.5, Salience: 0.5, Surprise: 0.5, Certainty: 0.5}
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"prefers-contrast more forces opacity 1", func(t *testing.T) {
			b := normalizeSpace(mediaBlock(t, css, "@media (prefers-contrast: more)"))
			if !strings.Contains(b, "opacity: 1 !important") {
				t.Errorf("prefers-contrast block %q does not force opacity 1", b)
			}
			// Hedging must still be visible without faintness: the hedged word is slanted.
			if s := parseSpans(out)[0]; slantDeg(t, s.get("font-style")) == 0 {
				t.Error("hedged word not slanted")
			}
		}},
		{"forced-colors is not fought", func(t *testing.T) {
			if strings.Contains(css, "forced-color-adjust") {
				t.Error("page overrides forced-colors")
			}
			if !strings.Contains(comments, "forced-colors") {
				t.Error("no CSS comment explaining forced-colors")
			}
		}},
		{"meaning survives without colour", func(t *testing.T) {
			for _, tweak := range []func(*renderer.Token){
				func(k *renderer.Token) { k.Salience = 1 },
				func(k *renderer.Token) { k.Surprise = 1 },
				func(k *renderer.Token) { k.Certainty = 0 },
			} {
				k := base
				tweak(&k)
				a, b := renderOne(t, base), renderOne(t, k)
				changed := false
				for _, p := range nonColour {
					if a.get(p) != b.get(p) {
						changed = true
					}
				}
				if !changed {
					t.Errorf("score change %+v invisible without colour", k)
				}
			}
		}},
		{"no animation, reduced motion documented", func(t *testing.T) {
			for _, bad := range []string{"animation", "transition", "@keyframes", "scroll-behavior"} {
				if strings.Contains(css, bad) {
					t.Errorf("CSS contains %q", bad)
				}
			}
			if !strings.Contains(comments, "reduced-motion") {
				t.Error("no CSS comment stating reduced-motion needs nothing")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t)
		})
	}
}

// TestRender_robust checks determinism, no panics and linear-time rendering of large inputs.
func TestRender_robust(t *testing.T) {
	t.Parallel()
	nan := math.NaN()
	var big []renderer.Token
	for i := range 100_000 {
		sep := " \n"
		if i%7 == 0 {
			sep = "\n\n"
		}
		big = append(big, renderer.Token{Text: "abcdefghijklmnopqrs", Word: true, Valence: float64(i%201-100) / 100, Salience: 0.5, Certainty: 0.3}, gap(sep))
	}
	tests := []struct {
		name     string
		tokens   []renderer.Token
		minBytes int
	}{
		{"nil", nil, 0},
		{"NaN and Inf everywhere", []renderer.Token{{Text: "x", Word: true, Valence: nan, Salience: math.Inf(-1), Surprise: nan, Certainty: math.Inf(1)}}, 0},
		{"huge words input (~2.3 MB)", big, 2_000_000},
		{"huge single text with breaks (~3 MB)", []renderer.Token{gap(strings.Repeat("x \n \n\r\ny\n", 350_000))}, 3_000_000},
		{"huge single word (~3 MB)", []renderer.Token{w(strings.Repeat("<&>", 1_000_000))}, 3_000_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := 0
			for _, k := range tc.tokens {
				in += len(k.Text)
			}
			if in < tc.minBytes {
				t.Fatalf("input only %d bytes", in)
			}
			start := time.Now()
			a := renderer.Render(tc.tokens)
			took := time.Since(start)
			if b := renderer.Render(tc.tokens); a != b {
				t.Error("output not deterministic")
			}
			// A quadratic renderer on MB inputs takes minutes; a linear one well under this bound, even with -race.
			if took > 15*time.Second {
				t.Errorf("rendering %d bytes took %v", in, took)
			}
			if !strings.HasSuffix(strings.TrimSpace(a), "</html>") {
				t.Error("page not complete")
			}
		})
	}
}
