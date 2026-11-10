package lexiscore_test

import (
	"bufio"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bijaya777pokhrel-hue/dark-factory-lab/lexiscore"
)

// tol absorbs only floating-point evaluation-order differences; scores are
// exact floats, so any rounding (e.g. to 2 or 3 decimals) still fails.
const tol = 1e-12

// largeBudget is far above what a linear Analyze needs for the large inputs
// (even under -race) and far below what a quadratic one would need.
const largeBudget = 20 * time.Second

// sep is a non-word token.
func sep(text string) lexiscore.Token { return lexiscore.Token{Text: text} }

// word is a word token with the given scores.
func word(text string, valence, salience, surprise, certainty float64) lexiscore.Token {
	return lexiscore.Token{Text: text, Word: true, Scores: lexiscore.Scores{
		Valence: valence, Salience: salience, Surprise: surprise, Certainty: certainty,
	}}
}

func near(a, b float64) bool { return math.Abs(a-b) <= tol }

func isApostrophe(r rune) bool { return r == '\'' || r == '’' }

// checkInvariants asserts the properties every result must have, whatever
// the input: lossless, no empty tokens, no adjacent non-word tokens, words
// are maximal letter runs with single inner apostrophes, separators hold no
// letters, and every score is inside its documented range.
func checkInvariants(t *testing.T, in string, got []lexiscore.Token) {
	t.Helper()
	if in == "" && len(got) != 0 {
		t.Fatalf("empty input gave %d tokens", len(got))
	}
	var b strings.Builder
	for i, tok := range got {
		b.WriteString(tok.Text)
		if tok.Text == "" {
			t.Fatalf("token %d is empty", i)
		}
		if i > 0 && got[i-1].Word == tok.Word {
			t.Fatalf("tokens %d and %d are both Word=%v: %q %q", i-1, i, tok.Word, got[i-1].Text, tok.Text)
		}
		if !tok.Word {
			if tok.Scores != (lexiscore.Scores{}) {
				t.Fatalf("non-word token %q has scores %+v", tok.Text, tok.Scores)
			}
			for _, r := range tok.Text {
				if unicode.IsLetter(r) {
					t.Fatalf("non-word token %q contains letter %q", tok.Text, r)
				}
			}
			if i > 0 && i < len(got)-1 && utf8.RuneCountInString(tok.Text) == 1 {
				if r, _ := utf8.DecodeRuneInString(tok.Text); isApostrophe(r) {
					t.Fatalf("single apostrophe %q between two words was not joined", tok.Text)
				}
			}
			continue
		}
		checkWordShape(t, tok.Text)
		checkRanges(t, tok)
	}
	if b.String() != in {
		t.Fatalf("not lossless:\n in: %q\nout: %q", in, b.String())
	}
}

func checkWordShape(t *testing.T, w string) {
	t.Helper()
	if !utf8.ValidString(w) {
		t.Fatalf("word %q is not valid UTF-8", w)
	}
	runes := []rune(w)
	for j, r := range runes {
		switch {
		case unicode.IsLetter(r):
		case isApostrophe(r):
			if j == 0 || j == len(runes)-1 || !unicode.IsLetter(runes[j-1]) || !unicode.IsLetter(runes[j+1]) {
				t.Fatalf("word %q has an apostrophe not between letters", w)
			}
		default:
			t.Fatalf("word %q contains non-letter %q", w, r)
		}
	}
}

func checkRanges(t *testing.T, tok lexiscore.Token) {
	t.Helper()
	s := tok.Scores
	if !(s.Valence >= -1 && s.Valence <= 1) ||
		!(s.Salience >= 0 && s.Salience <= 1) ||
		!(s.Surprise >= 0 && s.Surprise <= 1) ||
		!(s.Certainty >= 0 && s.Certainty <= 1) {
		t.Fatalf("word %q has out-of-range scores %+v", tok.Text, s)
	}
}

// expectTokens compares text, kind and every score.
func expectTokens(want []lexiscore.Token) func(*testing.T, string, []lexiscore.Token) {
	return func(t *testing.T, _ string, got []lexiscore.Token) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %d tokens %v, want %d %v", len(got), texts(got), len(want), texts(want))
		}
		for i := range want {
			g, w := got[i], want[i]
			if g.Text != w.Text || g.Word != w.Word {
				t.Fatalf("token %d = (%q, word=%v), want (%q, word=%v)", i, g.Text, g.Word, w.Text, w.Word)
			}
			if !near(g.Scores.Valence, w.Scores.Valence) || !near(g.Scores.Salience, w.Scores.Salience) ||
				!near(g.Scores.Surprise, w.Scores.Surprise) || !near(g.Scores.Certainty, w.Scores.Certainty) {
				t.Fatalf("token %d %q scores %+v, want %+v", i, g.Text, g.Scores, w.Scores)
			}
		}
	}
}

// expectSplit compares only the token texts and kinds (word boundaries).
func expectSplit(want ...string) func(*testing.T, string, []lexiscore.Token) {
	want = append([]string{}, want...) // no arguments means an empty, non-nil list
	return func(t *testing.T, _ string, got []lexiscore.Token) {
		t.Helper()
		if !reflect.DeepEqual(texts(got), want) {
			t.Fatalf("split = %q, want %q", texts(got), want)
		}
	}
}

// texts renders tokens as their text, with non-word tokens in brackets.
func texts(toks []lexiscore.Token) []string {
	out := make([]string, len(toks))
	for i, tok := range toks {
		if tok.Word {
			out[i] = tok.Text
		} else {
			out[i] = "[" + tok.Text + "]"
		}
	}
	return out
}

func words(toks []lexiscore.Token) []lexiscore.Token {
	var out []lexiscore.Token
	for _, tok := range toks {
		if tok.Word {
			out = append(out, tok)
		}
	}
	return out
}

// lastWord returns the scores of the final word token.
func lastWord(t *testing.T, toks []lexiscore.Token) lexiscore.Scores {
	t.Helper()
	ws := words(toks)
	if len(ws) == 0 {
		t.Fatal("no words")
	}
	return ws[len(ws)-1].Scores
}

// readLexicon parses the shipped lexicon file independently of the package.
// Later entries for the same word replace earlier ones.
func readLexicon(t *testing.T) map[string]float64 {
	t.Helper()
	f, err := os.Open("vader_lexicon.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	m := map[string]float64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 2 {
			continue
		}
		mean, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("lexicon line %q: %v", sc.Text(), err)
		}
		m[fields[0]] = mean
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return m
}

// isPlainWord reports whether s is lowercase letters with single inner
// apostrophes, i.e. something Analyze can return as a single word.
func isPlainWord(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	for j, r := range runes {
		if isApostrophe(r) {
			if j == 0 || j == len(runes)-1 || !unicode.IsLetter(runes[j-1]) || !unicode.IsLetter(runes[j+1]) {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) || unicode.ToLower(r) != r {
			return false
		}
	}
	return true
}

// randomText builds inputs from pieces that stress tokenization.
func randomText(rng *rand.Rand) string {
	pieces := []string{
		"a", "Z", "é", "ж", "日", "ß", "Ω", "'", "’", "''", "-", "_", "1", "42", " ", "\t", "\r\n", "\n",
		".", "!", "?", ",", ";", "😀", "👍🏽", "\u0301", "\u200d", "\xff", "\xc3", "\xe2\x80", "\x00",
		"good", "Horrible", "perhaps", "DEFINITELY", "the", "don't", "can’t", ":)", "xd", "ok",
	}
	n := rng.IntN(40)
	var b strings.Builder
	for range n {
		b.WriteString(pieces[rng.IntN(len(pieces))])
	}
	return b.String()
}

func TestAnalyze(t *testing.T) {
	t.Parallel()

	// A large multi-sentence text (~4 MB) built from varied sentences.
	sentences := []string{
		"The garden looked wonderful this morning, but the fence is clearly broken.\n",
		"Perhaps the delayed train was annoying; the staff were definitely helpful!\r\n",
		"Is the new policy a disaster? Maybe not.\t",
		"Café owners in Zürich reported superb sales—roughly 12% higher—despite awful weather.\n",
		"Ничего страшного, 日本語 text and emoji 😀 sit between words.\n",
	}
	var big strings.Builder
	for i := 0; big.Len() < 4<<20; i++ {
		big.WriteString(sentences[i%len(sentences)])
	}
	bigText := big.String()

	// One huge sentence (~4 MB, 400k sentiment words, no sentence ends):
	// rescanning earlier words per word would be quadratic here.
	const pairs = 200_000
	longSentence := strings.Repeat("love hate ", pairs)

	cases := []struct {
		name  string
		in    string
		check func(t *testing.T, in string, got []lexiscore.Token)
	}{
		// 1. Lossless and token shape.
		{"empty input gives no tokens", "", expectSplit()},
		{"only separators is one non-word token", " \t-_123!?\r\n", expectSplit("[ \t-_123!?\r\n]")},
		{"emoji only", "😀👍🏽", expectSplit("[😀👍🏽]")},
		{"invalid UTF-8 stays in separators", "\xffbad\xc3", expectSplit("[\xff]", "bad", "[\xc3]")},
		{"CRLF and tabs between words", "one\r\ntwo\tthree", expectSplit("one", "[\r\n]", "two", "[\t]", "three")},
		{"leading and trailing separators", "  hi  ", expectSplit("[  ]", "hi", "[  ]")},
		{"very long word is one token", strings.Repeat("abcdefghij", 100_000), func(t *testing.T, in string, got []lexiscore.Token) {
			if len(got) != 1 || !got[0].Word || got[0].Text != in {
				t.Fatalf("got %d tokens, want the whole input as one word", len(got))
			}
		}},
		{"random inputs are lossless and well formed", "", func(t *testing.T, _ string, _ []lexiscore.Token) {
			rng := rand.New(rand.NewPCG(2026, 11))
			for range 5000 {
				in := randomText(rng)
				checkInvariants(t, in, lexiscore.Analyze(in))
			}
		}},

		// 2. Word boundaries.
		{"contraction is one word", "don't stop", expectSplit("don't", "[ ]", "stop")},
		{"curly apostrophe contraction is one word", "won’t go", expectSplit("won’t", "[ ]", "go")},
		{"several inner apostrophes", "rock'n'roll O’Neil's", expectSplit("rock'n'roll", "[ ]", "O’Neil's")},
		{"trailing apostrophe ends the word", "dogs' bowls", expectSplit("dogs", "[' ]", "bowls")},
		{"leading apostrophe is a separator", "'twas fine", expectSplit("[']", "twas", "[ ]", "fine")},
		{"double apostrophe splits", "it''s", expectSplit("it", "['']", "s")},
		{"hyphen splits", "well-known", expectSplit("well", "[-]", "known")},
		{"digits split", "abc123def", expectSplit("abc", "[123]", "def")},
		{"underscore splits", "snake_case", expectSplit("snake", "[_]", "case")},
		{"non-Latin letters form words", "Привет, мир! 日本語 naïve Ωmega",
			expectSplit("Привет", "[, ]", "мир", "[! ]", "日本語", "[ ]", "naïve", "[ ]", "Ωmega")},
		{"combining marks are not letters", "cafe\u0301 ok", expectSplit("cafe", "[\u0301 ]", "ok")},
		{"emoticon is not a word", "nice :) day", expectSplit("nice", "[ :) ]", "day")},

		// 3/4. Lookup and valence.
		{"lookup ignores case", "GOOD Good gOoD good", expectTokens([]lexiscore.Token{
			word("GOOD", 0.475, 0.65875, 0, 0.5), sep(" "),
			word("Good", 0.475, 0.65875, 0, 0.5), sep(" "),
			word("gOoD", 0.475, 0.65875, 0, 0.5), sep(" "),
			word("good", 0.475, 0.65875, 0, 0.5),
		})},
		{"curly and straight apostrophes look up alike", "can’t can't", func(t *testing.T, _ string, got []lexiscore.Token) {
			ws := words(got)
			if len(ws) != 2 || ws[0].Text != "can’t" || ws[1].Text != "can't" || ws[0].Scores != ws[1].Scores {
				t.Fatalf("got %v, want equal scores for both spellings", got)
			}
		}},
		{"unknown and unstemmed words have valence 0", "zorbleflux superbly disastrously", func(t *testing.T, _ string, got []lexiscore.Token) {
			for _, w := range words(got) {
				if w.Scores.Valence != 0 || w.Scores.Salience != 0.35 {
					t.Fatalf("%q = %+v, want valence 0 and salience 0.35", w.Text, w.Scores)
				}
			}
		}},
		{"multi-word lexicon entries never match single words", "can't stand", func(t *testing.T, _ string, got []lexiscore.Token) {
			for _, w := range words(got) {
				if w.Scores.Valence != 0 {
					t.Fatalf("%q valence %v, want 0", w.Text, w.Scores.Valence)
				}
			}
		}},
		{"duplicate lexicon entry: the later one wins", "ok", expectTokens([]lexiscore.Token{
			word("ok", 0.3, 0.35+0.65*0.3, 0, 0.5),
		})},
		{"strong negatives score below strong positives", "horrible disaster awful. superb wonderful love.", func(t *testing.T, _ string, got []lexiscore.Token) {
			ws := words(got)
			for _, neg := range ws[:3] {
				for _, pos := range ws[3:] {
					if !(neg.Scores.Valence < -0.4 && pos.Scores.Valence > 0.4) {
						t.Fatalf("%q=%v, %q=%v: want strongly negative < -0.4 < 0.4 < strongly positive",
							neg.Text, neg.Scores.Valence, pos.Text, pos.Scores.Valence)
					}
				}
			}
		}},
		{"every lexicon word scores mean/4 with sentiment salience", "", func(t *testing.T, _ string, _ []lexiscore.Token) {
			n := 0
			for w, mean := range readLexicon(t) {
				if !isPlainWord(w) {
					continue
				}
				n++
				got := lexiscore.Analyze(w)
				checkInvariants(t, w, got)
				if len(got) != 1 {
					t.Fatalf("%q split into %v", w, texts(got))
				}
				s := got[0].Scores
				v := mean / 4
				if !near(s.Valence, v) {
					t.Fatalf("%q valence %v, want %v", w, s.Valence, v)
				}
				if v != 0 && !near(s.Salience, 0.35+0.65*math.Abs(v)) {
					t.Fatalf("%q salience %v, want %v", w, s.Salience, 0.35+0.65*math.Abs(v))
				}
				if s.Surprise != 0 {
					t.Fatalf("lone word %q has surprise %v", w, s.Surprise)
				}
			}
			if n < 7000 {
				t.Fatalf("only %d plain words checked; lexicon not read?", n)
			}
		}},

		// 5. Salience.
		{"salience: sentiment > neutral content > function word", "The wonderful table", expectTokens([]lexiscore.Token{
			word("The", 0, 0.15, 0, 0.5), sep(" "),
			word("wonderful", 0.675, 0.78875, 0, 0.5), sep(" "),
			word("table", 0, 0.35, 0, 0.5),
		})},
		{"common function words have salience 0.15", "a an the of in on at to from with and but or if he she it we they you is was are be have has do", func(t *testing.T, _ string, got []lexiscore.Token) {
			for _, w := range words(got) {
				if w.Scores.Salience != 0.15 || w.Scores.Valence != 0 {
					t.Fatalf("%q = %+v, want function-word salience 0.15", w.Text, w.Scores)
				}
			}
		}},
		{"stronger sentiment stands out more", "nice good superb", func(t *testing.T, _ string, got []lexiscore.Token) {
			ws := words(got)
			if !(ws[0].Scores.Salience < ws[1].Scores.Salience && ws[1].Scores.Salience < ws[2].Scores.Salience) {
				t.Fatalf("salience not increasing with |valence|: %v", ws)
			}
		}},

		// 6. Surprise.
		{"surprise: first sentiment word 0, reversal scored, neutral 0", "Happy dog, sad cat.", expectTokens([]lexiscore.Token{
			word("Happy", 0.675, 0.78875, 0, 0.5), sep(" "),
			word("dog", 0, 0.35, 0, 0.5), sep(", "),
			word("sad", -0.525, 0.69125, 0.6, 0.5), sep(" "),
			word("cat", 0, 0.35, 0, 0.5), sep("."),
		})},
		{"surprise grows with the size of the reversal", "", func(t *testing.T, _ string, _ []lexiscore.Token) {
			agree := lastWord(t, lexiscore.Analyze("happy then nice")).Surprise
			mild := lastWord(t, lexiscore.Analyze("happy then annoying")).Surprise
			strong := lastWord(t, lexiscore.Analyze("happy then disaster")).Surprise
			if !near(agree, 0.1125) || !near(mild, 0.55) || !near(strong, 0.725) {
				t.Fatalf("surprise agree=%v mild=%v strong=%v, want 0.1125 0.55 0.725", agree, mild, strong)
			}
		}},
		{"context is the mean of all earlier sentiment words", "love, hate, superb", expectTokens([]lexiscore.Token{
			word("love", 0.8, 0.87, 0, 0.5), sep(", "),
			word("hate", -0.675, 0.78875, 0.7375, 0.5), sep(", "),
			word("superb", 0.775, 0.85375, 0.35625, 0.5),
		})},
		{"surprise resets at sentence ends", "", func(t *testing.T, _ string, _ []lexiscore.Token) {
			for _, end := range []string{". ", "! ", "? ", "\n", "\r\n", "...\t", " .The "} {
				in := "superb" + end + "disaster"
				if s := lastWord(t, lexiscore.Analyze(in)).Surprise; s != 0 {
					t.Fatalf("%q: surprise %v after sentence end, want 0", in, s)
				}
			}
			for _, mid := range []string{", ", "; ", ": ", " - ", " and the "} {
				in := "superb" + mid + "disaster"
				if s := lastWord(t, lexiscore.Analyze(in)).Surprise; !near(s, 0.775) {
					t.Fatalf("%q: surprise %v within a sentence, want 0.775", in, s)
				}
			}
		}},

		// 7. Certainty.
		{"certainty: hedge < neutral < assertive, independent of valence", "Perhaps the table is definitely nice, maybe.", expectTokens([]lexiscore.Token{
			word("Perhaps", 0, 0.35, 0, 0.15), sep(" "),
			word("the", 0, 0.15, 0, 0.5), sep(" "),
			word("table", 0, 0.35, 0, 0.5), sep(" "),
			word("is", 0, 0.15, 0, 0.5), sep(" "),
			word("definitely", 0.425, 0.62625, 0, 0.95), sep(" "),
			word("nice", 0.45, 0.6425, 0.0125, 0.5), sep(", "),
			word("maybe", 0, 0.35, 0, 0.15), sep("."),
		})},
		{"named hedge words are 0.15", "perhaps MAYBE possibly probably seemingly apparently arguably allegedly supposedly roughly somewhat might", func(t *testing.T, _ string, got []lexiscore.Token) {
			for _, w := range words(got) {
				if w.Scores.Certainty != 0.15 {
					t.Fatalf("%q certainty %v, want 0.15", w.Text, w.Scores.Certainty)
				}
			}
		}},
		{"named assertive words are 0.95", "definitely Certainly clearly absolutely undoubtedly obviously surely always never totally truly indeed", func(t *testing.T, _ string, got []lexiscore.Token) {
			for _, w := range words(got) {
				if w.Scores.Certainty != 0.95 {
					t.Fatalf("%q certainty %v, want 0.95", w.Text, w.Scores.Certainty)
				}
			}
		}},
		{"other words are 0.5", "river horrible quickly blue walked", func(t *testing.T, _ string, got []lexiscore.Token) {
			for _, w := range words(got) {
				if w.Scores.Certainty != 0.5 {
					t.Fatalf("%q certainty %v, want 0.5", w.Text, w.Scores.Certainty)
				}
			}
		}},

		// 9. Concurrency and size.
		{"concurrent calls agree with sequential ones", "", func(t *testing.T, _ string, _ []lexiscore.Token) {
			inputs := append([]string{}, sentences...)
			inputs = append(inputs, "Absolutely brilliant. Possibly tragic?", "")
			want := make([][]lexiscore.Token, len(inputs))
			for i, in := range inputs {
				want[i] = lexiscore.Analyze(in)
			}
			var wg sync.WaitGroup
			errs := make(chan string, 64*len(inputs))
			for g := range 64 {
				wg.Go(func() {
					for k := range inputs {
						i := (g + k) % len(inputs)
						if got := lexiscore.Analyze(inputs[i]); !reflect.DeepEqual(got, want[i]) {
							errs <- inputs[i]
						}
					}
				})
			}
			wg.Wait()
			close(errs)
			for in := range errs {
				t.Fatalf("concurrent result differs for %q", in)
			}
		}},
		{"large multi-sentence input is fast and well formed", bigText, func(t *testing.T, in string, got []lexiscore.Token) {
			start := time.Now()
			again := lexiscore.Analyze(in)
			if d := time.Since(start); d > largeBudget {
				t.Fatalf("%d bytes took %v, budget %v", len(in), d, largeBudget)
			}
			if len(again) != len(got) || len(got) < 500_000 {
				t.Fatalf("got %d tokens (rerun %d), want a stable count above 500000", len(got), len(again))
			}
		}},
		{"one huge sentence is linear and keeps an exact running mean", longSentence, func(t *testing.T, in string, got []lexiscore.Token) {
			start := time.Now()
			_ = lexiscore.Analyze(in)
			if d := time.Since(start); d > largeBudget {
				t.Fatalf("%d bytes took %v, budget %v", len(in), d, largeBudget)
			}
			// The last word is "hate"; before it come pairs "love" and pairs-1 "hate".
			ctx := (pairs*0.8 + (pairs-1)*-0.675) / (2*pairs - 1)
			want := math.Abs(-0.675-ctx) / 2
			if s := lastWord(t, got).Surprise; math.Abs(s-want) > 1e-9 {
				t.Fatalf("last surprise %v, want %v", s, want)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := lexiscore.Analyze(tc.in)
			checkInvariants(t, tc.in, got)
			tc.check(t, tc.in, got)
		})
	}
}
