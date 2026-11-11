package tui

import (
	"math/rand/v2"
	"strings"
	"unicode"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// dictationItems is how many sentences one dictation plays; a deck with
// fewer short examples plays the ones it has.
const dictationItems = 10

// minDictationItems is the fewest sentences worth a dictation.
const minDictationItems = 3

// maxDictationWords is the longest example played, in words: longer
// sentences are too much to keep in mind and type at A1.
const maxDictationWords = 12

// dictationSentences picks the sentences of a dictation from the examples
// of the learner's cards. It is pure: the same cards and the same rng
// state give the same sentences.
//
// Each example counts once, whatever its directions, and only if it has at
// most maxDictationWords words. Examples of words already studied come
// first, so the learner hears what Review has taught; new words fill the
// rest. Each group is shuffled, and at most dictationItems are kept.
func dictationSentences(cards []domain.Card, rng *rand.Rand) []string {
	seen := map[string]bool{}
	var studied, fresh []string
	for _, c := range cards {
		s := strings.TrimSpace(c.Example)
		k := dictationKey(s)
		if k == "" || seen[k] || len(strings.Fields(k)) > maxDictationWords {
			continue
		}
		seen[k] = true
		if c.LastReviewed != nil {
			studied = append(studied, s)
		} else {
			fresh = append(fresh, s)
		}
	}
	for _, group := range [][]string{studied, fresh} {
		rng.Shuffle(len(group), func(i, j int) { group[i], group[j] = group[j], group[i] })
	}
	all := append(studied, fresh...)
	return all[:min(len(all), dictationItems)]
}

// dictationKey is s as dictation compares it: lowercase, without
// punctuation except the apostrophe (I'm is not Im), and with single
// spaces. A comma the learner cannot hear never makes an answer wrong.
func dictationKey(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "’", "'")
	s = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) && r != '\'' {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// missedWords are the words of expected that typed lacks or gets wrong,
// in order. They are what is left of expected once its longest common
// run of words with typed is taken out.
func missedWords(typed, expected string) []string {
	got, want := strings.Fields(dictationKey(typed)), strings.Fields(dictationKey(expected))
	// lcs[i][j] is the longest common subsequence of got[i:] and want[j:].
	lcs := make([][]int, len(got)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(want)+1)
	}
	for i := len(got) - 1; i >= 0; i-- {
		for j := len(want) - 1; j >= 0; j-- {
			if got[i] == want[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var missed []string
	i, j := 0, 0
	for j < len(want) {
		switch {
		case i < len(got) && got[i] == want[j]:
			i, j = i+1, j+1
		case i < len(got) && lcs[i+1][j] >= lcs[i][j+1]:
			i++ // a word the learner typed that is not there
		default:
			missed = append(missed, want[j])
			j++
		}
	}
	return missed
}
