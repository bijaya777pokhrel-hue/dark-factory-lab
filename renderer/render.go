// Package renderer turns scored tokens into one self-contained HTML page in
// which every word's typography carries its scores, as specified by
// spec/target.html.
package renderer

import (
	"html"
	"math"
	"strconv"
	"strings"
)

// Token is a word (Word true, with scores) or the text between words.
type Token struct {
	Text      string
	Word      bool
	Valence   float64 // -1 .. 1
	Salience  float64 // 0 .. 1
	Surprise  float64 // 0 .. 1
	Certainty float64 // 0 (hedged) .. 1 (confident)
}

// Colour anchors. Light ones come from spec/target.html; dark ones are
// documented in the page CSS below.
var (
	lightInk       = [3]float64{0x1c, 0x1c, 0x1c}
	lightNegative  = [3]float64{0xa8, 0x2e, 0x00}
	lightPositive  = [3]float64{0x0b, 0x4f, 0xa8}
	darkInk        = [3]float64{0xe6, 0xe6, 0xe6}
	darkNegative   = [3]float64{0xff, 0x9e, 0x7a}
	darkPositive   = [3]float64{0x8c, 0xb8, 0xff}
	roundingMargin = 1e-9 // absorbs float error so 0.975 rounds half-up to 0.98
)

const pageHead = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>Semantic Font</title>
<style>
  html { font-size: 100%; background: #ffffff; color: #1c1c1c; }
  body {
    margin: 0;
    background: #ffffff;
    font-family: system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  }
  main { max-width: 40rem; margin: 0 auto; padding: 3rem 1rem; }
  p { margin: 0; font-size: 2rem; line-height: 1.6; }
  p + p { margin-top: 0.75em; }
  /*
    Dark theme: background #121212, ink #e6e6e6, vermilion #ff9e7a, blue #8cb8ff.
    Each word carries its dark colour in --dark-color, eased exactly like the
    light colour (t = sqrt(|valence|), sRGB interpolation from ink to anchor).
    At the opacity floor 0.8, composited over #121212, the worst case over the
    whole valence range is about 6.3:1 (light theme: about 4.66:1).
  */
  @media (prefers-color-scheme: dark) {
    html, body { background: #121212; color: #e6e6e6; }
    main span { color: var(--dark-color) !important; }
  }
  /*
    More contrast requested (or forced colours active): full opacity, so
    hedging is carried by the oblique slant alone, never by faintness.
  */
  @media (prefers-contrast: more), (forced-colors: active) {
    main span { opacity: 1 !important; }
  }
  /*
    forced-colors: the page does not fight it (no forced-color-adjust). Colour
    carries only valence; salience is carried by weight and size, surprise by
    letter-spacing, certainty by slant and opacity, so those survive without
    colour. There is no animation or transition anywhere, so
    prefers-reduced-motion needs nothing.
  */
</style>
</head>
<body>
<main>
`

const pageTail = `</main>
</body>
</html>
`

// Render returns one complete, self-contained HTML page.
func Render(tokens []Token) string {
	var b strings.Builder
	b.WriteString(pageHead)
	w := paragraphWriter{b: &b}
	var run strings.Builder
	for i := 0; i < len(tokens); {
		if tokens[i].Word {
			w.word(tokens[i])
			i++
			continue
		}
		run.Reset()
		for ; i < len(tokens) && !tokens[i].Word; i++ {
			run.WriteString(tokens[i].Text)
		}
		w.between(run.String())
	}
	w.closeParagraph()
	b.WriteString(pageTail)
	return b.String()
}

// paragraphWriter opens paragraphs lazily so none is ever empty, and holds a
// single line break until more content arrives in the same paragraph.
type paragraphWriter struct {
	b       *strings.Builder
	open    bool
	pending bool // a <br> is owed before the next content
}

func (w *paragraphWriter) closeParagraph() {
	if w.open {
		w.b.WriteString("</p>\n")
	}
	w.open, w.pending = false, false
}

// content prepares for visible content: opens the paragraph, pays any owed <br>.
func (w *paragraphWriter) content() {
	if !w.open {
		w.b.WriteString("<p>")
		w.open = true
	}
	if w.pending {
		w.b.WriteString("<br>")
		w.pending = false
	}
}

func (w *paragraphWriter) text(s string) {
	if strings.Trim(s, " \t\f") == "" && (!w.open || w.pending) {
		return // whitespace at a paragraph start or right after a line break is invisible
	}
	if !w.open {
		s = strings.TrimLeft(s, " \t\f")
	}
	w.content()
	w.b.WriteString(escape(s))
}

// between handles the text between two words. A group of line breaks
// separated only by spaces or tabs is one break (<br>) or, with two or more,
// a paragraph end.
func (w *paragraphWriter) between(s string) {
	seg := 0
	for i := 0; i < len(s); {
		if s[i] != '\n' && s[i] != '\r' {
			i++
			continue
		}
		breaks, end := 0, i
		for k := i; k < len(s); {
			switch s[k] {
			case '\r':
				k++
				if k < len(s) && s[k] == '\n' {
					k++
				}
			case '\n':
				k++
			case ' ', '\t', '\f':
				k++
				continue
			default:
				k = len(s) // stop scanning
				continue
			}
			breaks, end = breaks+1, k
		}
		w.text(s[seg:i])
		if breaks >= 2 {
			w.closeParagraph()
		} else if w.open {
			w.pending = true
		}
		i, seg = end, end
	}
	w.text(s[seg:])
}

func (w *paragraphWriter) word(t Token) {
	w.content()
	v := score(t.Valence, -1, 1, 0)
	s := score(t.Salience, 0, 1, 0)
	su := score(t.Surprise, 0, 1, 0)
	c := score(t.Certainty, 0, 1, 0.5)
	b := w.b
	b.WriteString(`<span data-valence="`)
	b.WriteString(number(v))
	b.WriteString(`" data-salience="`)
	b.WriteString(number(s))
	b.WriteString(`" data-surprise="`)
	b.WriteString(number(su))
	b.WriteString(`" data-certainty="`)
	b.WriteString(number(c))
	b.WriteString(`" style="font-weight: `)
	b.WriteString(number(math.Floor((300+500*s)/50+0.5+roundingMargin) * 50))
	b.WriteString(`; font-size: `)
	b.WriteString(number(roundHalfUp(0.9+0.5*s, 2)))
	b.WriteString(`em; color: `)
	b.WriteString(colour(v, lightInk, lightNegative, lightPositive))
	b.WriteString(`; font-style: `)
	if n := math.Floor(14*math.Max(0, 0.5-c)/0.5 + 0.5 + roundingMargin); n > 0 {
		b.WriteString("oblique ")
		b.WriteString(number(n))
		b.WriteString("deg")
	} else {
		b.WriteString("normal")
	}
	b.WriteString(`; letter-spacing: `)
	b.WriteString(number(roundHalfUp(0.12*su, 3)))
	b.WriteString(`em; opacity: `)
	b.WriteString(number(roundHalfUp(0.8+0.2*c, 2)))
	b.WriteString(`; --dark-color: `)
	b.WriteString(colour(v, darkInk, darkNegative, darkPositive))
	b.WriteString(`">`)
	b.WriteString(escape(t.Text))
	b.WriteString(`</span>`)
}

// score clamps x to [lo, hi]; NaN and ±Inf become neutral; -0 becomes 0.
func score(x, lo, hi, neutral float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return neutral
	}
	x = math.Min(hi, math.Max(lo, x))
	if x == 0 {
		return 0
	}
	return x
}

func roundHalfUp(x float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Floor(x*p+0.5+roundingMargin) / p
}

// number formats a plain decimal number: no exponent, no trailing zeros.
func number(x float64) string {
	return strconv.FormatFloat(x, 'f', -1, 64)
}

// colour eases from ink toward the anchor for the valence's sign by sqrt(|v|).
func colour(v float64, ink, negative, positive [3]float64) string {
	anchor := positive
	if v < 0 {
		anchor = negative
	}
	t := math.Sqrt(math.Abs(v))
	const hex = "0123456789abcdef"
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i := range 3 {
		c := int(math.Floor(ink[i] + (anchor[i]-ink[i])*t + 0.5 + roundingMargin))
		c = min(255, max(0, c))
		out[1+2*i], out[2+2*i] = hex[c>>4], hex[c&15]
	}
	return string(out)
}

func escape(s string) string {
	return html.EscapeString(strings.ToValidUTF8(s, "�"))
}
