package check

import (
	"fmt"
	"strings"
	"unicode"
)

// WordMark is one word of an expected sentence and whether a transcript
// of the learner's voice has it.
type WordMark struct {
	Word       string // the word as the sentence writes it, punctuation included
	Understood bool   // the transcript has this word, in this place
}

// Understood marks each word of expected that transcript has, in order.
// It is how shadowing tells which words the speech recognizer understood.
//
// Words are compared by wordKey: case and punctuation do not count,
// except the apostrophe (I'm is not Im), and numbers written as digits
// equal the same numbers in words (30 is thirty). Spellings that sound
// the same match (see sameWord), but a changed vowel or ending never
// does, so cut is never cat. The words are aligned with a longest common
// subsequence, so extra words in the transcript are ignored and a word
// counts only in its place. A token with no letters, like a dash, has
// nothing to say and is always understood.
func Understood(expected, transcript string) []WordMark {
	words := strings.Fields(expected)
	marks := make([]WordMark, len(words))
	var want []string // the keys to align
	var at []int      // the index in marks of each key in want
	for i, w := range words {
		marks[i].Word = w
		if k := wordKey(w); k != "" {
			want = append(want, k)
			at = append(at, i)
		} else {
			marks[i].Understood = true
		}
	}
	var got []string
	for _, w := range strings.Fields(transcript) {
		if k := wordKey(w); k != "" {
			got = append(got, k)
		}
	}

	// lcs[i][j] is the longest common subsequence of want[i:] and got[j:].
	lcs := make([][]int, len(want)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(got)+1)
	}
	for i := len(want) - 1; i >= 0; i-- {
		for j := len(got) - 1; j >= 0; j-- {
			if sameWord(want[i], got[j]) {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	for i, j := 0, 0; i < len(want) && j < len(got); {
		switch {
		case sameWord(want[i], got[j]): // a match is always on a longest path
			marks[at[i]].Understood = true
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++ // a word of the sentence the transcript lacks
		default:
			j++ // a word the transcript adds
		}
	}
	return marks
}

// sameWord reports whether the transcript word got is the expected word
// want. Only spelling that sounds the same is forgiven: British and
// American spellings (colour, color; grey, gray) and a doubled letter
// (Ana, Anna). A changed vowel (ship, shop) or a dropped ending (lives,
// live; worked, work) is another sound, so it is never the same word:
// those are exactly the mistakes shadowing must show.
func sameWord(want, got string) bool {
	return want == got || spelling(want) == spelling(got)
}

// spelling is word with the spelling differences that do not change its
// sound removed: doubled letters collapsed, British our as or and grey
// as gray.
func spelling(word string) string {
	if word == "grey" {
		word = "gray"
	}
	if len(word) > 4 { // favourite, colour; but four and your stay
		word = strings.ReplaceAll(word, "our", "or")
	}
	var b strings.Builder
	var last rune
	for _, r := range word {
		if r != last {
			b.WriteRune(r)
		}
		last = r
	}
	return b.String()
}

// wordKey is word as Understood compares it: lowercase letters, digits
// and apostrophes only, with numbers from 0 to 100 written as digits.
// A word with neither letters nor digits is "".
func wordKey(word string) string {
	word = strings.ReplaceAll(strings.ToLower(word), "’", "'")
	var b strings.Builder
	for _, r := range word {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			b.WriteRune(r)
		}
	}
	k := strings.Trim(b.String(), "'") // quotes around a word are not apostrophes
	if n, ok := numberWords[k]; ok {
		return fmt.Sprint(n)
	}
	return k
}

// numberWords are the numbers 0 to 100 in words, without hyphens as
// wordKey leaves them: "thirtyfive" is 35.
var numberWords = func() map[string]int {
	ones := []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
		"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	tens := []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	m := map[string]int{"hundred": 100}
	for n, w := range ones {
		m[w] = n
	}
	for t := 2; t < len(tens); t++ {
		m[tens[t]] = t * 10
		for o := 1; o < 10; o++ {
			m[tens[t]+ones[o]] = t*10 + o
		}
	}
	return m
}()
