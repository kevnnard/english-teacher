package tui

import (
	"math/rand/v2"
	"strings"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// vocabQuestions is how many questions a vocabulary quiz asks; a smaller
// deck asks one per word.
const vocabQuestions = 10

// quizQuestion is one question of a quiz: multiple choice when it has
// options, typed otherwise.
type quizQuestion struct {
	prompt  string   // vocabulary: the Spanish meaning; grammar: the English question
	example string   // typed vocabulary: the card's example with the word gapped, or ""
	options []string // multiple choice: teacher.QuizOptions options; typed: nil
	answer  int      // multiple choice: the index of the right option
	word    string   // the right answer
	why     string   // grammar: why the answer is right, in Spanish
}

// typed reports whether the learner types the answer instead of choosing it.
func (q quizQuestion) typed() bool { return len(q.options) == 0 }

// vocabQuiz builds a vocabulary quiz from the learner's cards. It is pure:
// the same cards and the same rng state give the same quiz.
//
// Each word counts once, whatever its directions, and needs a meaning. The
// quiz asks up to vocabQuestions words in a random order, alternating
// multiple choice (the meaning → choose the English word, with three
// other words of the deck as distractors) and typed (the meaning → type
// the word). A word whose distractors would be too few, as in a deck of
// fewer than four words, is asked typed.
func vocabQuiz(cards []domain.Card, rng *rand.Rand) []quizQuestion {
	words := quizWords(cards)
	rng.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })

	n := min(len(words), vocabQuestions)
	qs := make([]quizQuestion, n)
	for i, c := range words[:n] {
		q := quizQuestion{prompt: c.Meaning, word: c.Word}
		if i%2 == 0 {
			q.options, q.answer = choices(c, words, rng)
		}
		if q.typed() {
			if gapped, ok := check.Gap(c.Example, c.Word); ok {
				q.example = gapped
			}
		}
		qs[i] = q
	}
	return qs
}

// quizWords is one card per word with a meaning, in deck order.
func quizWords(cards []domain.Card) []domain.Card {
	seen := map[string]bool{}
	var words []domain.Card
	for _, c := range cards {
		k := strings.ToLower(strings.TrimSpace(c.Word))
		if k == "" || strings.TrimSpace(c.Meaning) == "" || seen[k] {
			continue
		}
		seen[k] = true
		words = append(words, c)
	}
	return words
}

// choices are the options of a multiple-choice question on c: its word and
// three distractors from words, shuffled, and the index of c's word. A
// distractor never shares c's meaning, so only one option is right. With
// fewer than three possible distractors there are no options.
func choices(c domain.Card, words []domain.Card, rng *rand.Rand) ([]string, int) {
	var pool []string
	for _, w := range words {
		if !strings.EqualFold(w.Word, c.Word) && check.Normalize(w.Meaning) != check.Normalize(c.Meaning) {
			pool = append(pool, w.Word)
		}
	}
	if len(pool) < teacher.QuizOptions-1 {
		return nil, 0
	}
	options := []string{c.Word}
	for _, i := range rng.Perm(len(pool))[:teacher.QuizOptions-1] {
		options = append(options, pool[i])
	}
	rng.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })
	for i, o := range options {
		if o == c.Word {
			return options, i
		}
	}
	return nil, 0 // not reached: c.Word is always among the options
}
