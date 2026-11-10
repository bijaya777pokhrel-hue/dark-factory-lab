// Package lexiscore splits text into words and the text between them, and
// gives every word four scores: valence, salience, surprise and certainty.
//
// Valence comes from the VADER sentiment lexicon (MIT licensed; see
// THIRD_PARTY_NOTICES.md at the repository root), embedded in the package.
package lexiscore

import (
	_ "embed"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Scores are the four scores derived for one word.
type Scores struct {
	Valence   float64 // -1 (negative) .. 1 (positive)
	Salience  float64 // 0 .. 1, how much the word should stand out
	Surprise  float64 // 0 .. 1, contrast with the sentiment before it
	Certainty float64 // 0 (hedged) .. 1 (confident)
}

// Token is a word (Word true, with Scores) or the text between words.
type Token struct {
	Text   string
	Word   bool
	Scores Scores // zero value when Word is false
}

// Score levels.
const (
	salienceFunction = 0.15 // English function words
	salienceContent  = 0.35 // any other word without sentiment, and the floor for sentiment words
	salienceSpan     = 0.65 // added in proportion to |valence| for sentiment words

	certaintyHedge     = 0.15
	certaintyNeutral   = 0.5
	certaintyAssertive = 0.95
)

// sentenceEnds are the characters that, inside the text between words, end a
// sentence and so reset the surprise context.
const sentenceEnds = ".!?\n"

// Word lists, all lowercase, kept together in this one place.
var (
	// functionWords are common English articles, prepositions, conjunctions,
	// pronouns and auxiliaries: words that carry grammar rather than content.
	functionWords = wordSet(
		// articles and determiners
		"a", "an", "the", "this", "that", "these", "those",
		// prepositions
		"about", "above", "across", "after", "against", "along", "among", "around", "as", "at",
		"before", "behind", "below", "beneath", "beside", "between", "beyond", "by", "down", "during",
		"for", "from", "in", "inside", "into", "near", "of", "off", "on", "onto",
		"out", "over", "through", "to", "toward", "towards", "under", "until", "up", "upon",
		"with", "within", "without",
		// conjunctions
		"and", "but", "or", "nor", "so", "yet", "because", "although", "though", "if",
		"unless", "while", "whereas", "since", "than", "whether",
		// pronouns
		"i", "me", "my", "mine", "we", "us", "our", "ours", "you", "your", "yours",
		"he", "him", "his", "she", "her", "hers", "it", "its", "they", "them", "their", "theirs",
		"who", "whom", "whose", "which", "what",
		// auxiliaries
		"am", "is", "are", "was", "were", "be", "been", "being",
		"have", "has", "had", "do", "does", "did",
		"will", "would", "shall", "should", "can", "could", "may", "might", "must",
	)

	// hedgeWords soften or qualify a claim.
	hedgeWords = wordSet(
		"perhaps", "maybe", "possibly", "probably", "seemingly", "apparently", "arguably",
		"allegedly", "supposedly", "roughly", "somewhat", "might", "presumably", "likely",
		"conceivably", "reportedly", "approximately", "kinda", "sorta", "guess",
	)

	// assertiveWords state a claim with confidence.
	assertiveWords = wordSet(
		"definitely", "certainly", "clearly", "absolutely", "undoubtedly", "obviously", "surely",
		"always", "never", "totally", "truly", "indeed", "must", "unquestionably", "undeniably",
		"plainly", "entirely", "completely", "invariably", "assuredly",
	)
)

func wordSet(words ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

//go:embed vader_lexicon.txt
var lexiconData string

// lexicon maps each lexicon entry to its valence (mean / 4). It is parsed once,
// on first use, and only read afterwards, so it is safe for concurrent use.
var lexicon = sync.OnceValue(func() map[string]float64 { return parseLexicon(lexiconData) })

// parseLexicon reads lines of "entry<TAB>mean<TAB>...". Lines without a tab or
// with a mean that is not a number in -4..4 are skipped. When an entry repeats,
// the later line wins.
func parseLexicon(data string) map[string]float64 {
	m := make(map[string]float64, 8192)
	for line := range strings.Lines(data) {
		line = strings.TrimRight(line, "\r\n")
		entry, rest, ok := strings.Cut(line, "\t")
		if !ok || entry == "" {
			continue
		}
		field, _, _ := strings.Cut(rest, "\t")
		mean, err := strconv.ParseFloat(field, 64)
		if err != nil || !(mean >= -4 && mean <= 4) {
			continue
		}
		m[entry] = mean / 4
	}
	return m
}

// Analyze splits text into Tokens and scores every word.
//
// Concatenating the Text of the returned tokens gives back text exactly.
// A word is a maximal run of Unicode letters, possibly joined by single
// apostrophes (' or ’) between letters; everything else, including invalid
// UTF-8, goes into the non-word tokens between words. Analyze runs in time
// linear in len(text) and is safe for concurrent use.
func Analyze(text string) []Token {
	lex := lexicon()
	var tokens []Token
	var sum float64 // sum of valences of earlier sentiment words in the sentence
	var count int   // how many there are
	gap := 0        // start of the pending non-word text
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsLetter(r) {
			i += size
			continue
		}
		if gap < i {
			between := text[gap:i]
			tokens = append(tokens, Token{Text: between})
			if strings.ContainsAny(between, sentenceEnds) {
				sum, count = 0, 0
			}
		}
		end := wordEnd(text, i+size)
		scores := score(lex, text[i:end], sum, count)
		if scores.Valence != 0 {
			sum += scores.Valence
			count++
		}
		tokens = append(tokens, Token{Text: text[i:end], Word: true, Scores: scores})
		i, gap = end, end
	}
	if gap < len(text) {
		tokens = append(tokens, Token{Text: text[gap:]})
	}
	return tokens
}

// wordEnd returns the end of the word whose letters continue at i.
func wordEnd(text string, i int) int {
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.IsLetter(r) {
			i += size
			continue
		}
		if r != '\'' && r != '’' {
			return i
		}
		next, nextSize := utf8.DecodeRuneInString(text[i+size:])
		if !unicode.IsLetter(next) {
			return i
		}
		i += size + nextSize
	}
	return i
}

// score derives a word's scores, given the sum and count of the earlier
// sentiment words in its sentence.
func score(lex map[string]float64, word string, sum float64, count int) Scores {
	key := strings.ToLower(strings.ReplaceAll(word, "’", "'"))
	s := Scores{Valence: lex[key], Salience: salienceContent, Certainty: certaintyNeutral}

	if _, ok := functionWords[key]; ok {
		s.Salience = salienceFunction
	}
	if s.Valence != 0 {
		s.Salience = salienceContent + salienceSpan*math.Abs(s.Valence)
		if count > 0 {
			context := sum / float64(count)
			s.Surprise = math.Min(1, math.Abs(s.Valence-context)/2)
		}
	}

	if _, ok := hedgeWords[key]; ok {
		s.Certainty = certaintyHedge
	} else if _, ok := assertiveWords[key]; ok {
		s.Certainty = certaintyAssertive
	}
	return s
}
