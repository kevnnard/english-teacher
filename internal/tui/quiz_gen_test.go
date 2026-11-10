package tui

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/check"
	"github.com/kevnnard/english-teacher/internal/domain"
	"github.com/kevnnard/english-teacher/internal/teacher"
)

// quizDeck has twelve words with different meanings, each in both
// directions like a real deck.
func quizDeck() []domain.Card {
	words := [][2]string{
		{"hello", "hola"}, {"goodbye", "adiós"}, {"thank you", "gracias"}, {"please", "por favor"},
		{"teacher", "profesor/a"}, {"student", "estudiante"}, {"brother", "hermano"}, {"sister", "hermana"},
		{"house", "casa"}, {"dog", "perro"}, {"coffee", "café"}, {"water", "agua"},
	}
	var cards []domain.Card
	for _, w := range words {
		rec := word(w[0], "", w[1], "I say "+w[0]+" every day.")
		prod := rec
		prod.Direction = domain.Production
		cards = append(cards, rec, prod)
	}
	return cards
}

func seeded(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, 0)) }

func TestVocabQuizMixesMultipleChoiceAndTypedFromTheDeck(t *testing.T) {
	cards := quizDeck()
	meaningOf := map[string]string{}
	for _, c := range cards {
		meaningOf[c.Word] = c.Meaning
	}

	qs := vocabQuiz(cards, seeded(1))

	if len(qs) != vocabQuestions {
		t.Fatalf("%d questions, want %d", len(qs), vocabQuestions)
	}
	var seen []string
	choice := 0
	for i, q := range qs {
		if slices.Contains(seen, q.word) {
			t.Errorf("question %d repeats %q", i, q.word)
		}
		seen = append(seen, q.word)
		if q.prompt != meaningOf[q.word] {
			t.Errorf("question %d: prompt %q is not the meaning of %q", i, q.prompt, q.word)
		}
		if q.typed() {
			if want, _ := check.Gap("I say "+q.word+" every day.", q.word); q.example != want || !strings.Contains(want, "___") {
				t.Errorf("question %d: example %q, want %q", i, q.example, want)
			}
			continue
		}
		choice++
		if len(q.options) != teacher.QuizOptions || q.options[q.answer] != q.word {
			t.Errorf("question %d: options %v, answer %d, word %q", i, q.options, q.answer, q.word)
		}
		for _, o := range q.options {
			if _, ok := meaningOf[o]; !ok {
				t.Errorf("question %d: option %q is not in the deck", i, o)
			}
		}
		if distinct := slices.Compact(slices.Sorted(slices.Values(q.options))); len(distinct) != teacher.QuizOptions {
			t.Errorf("question %d repeats an option: %v", i, q.options)
		}
	}
	if choice != vocabQuestions/2 {
		t.Errorf("%d multiple-choice questions, want %d", choice, vocabQuestions/2)
	}
}

func TestVocabQuizIsDeterministicForASeed(t *testing.T) {
	a, b := vocabQuiz(quizDeck(), seeded(7)), vocabQuiz(quizDeck(), seeded(7))
	if !reflect.DeepEqual(a, b) {
		t.Error("the same seed gave two different quizzes")
	}
	if c := vocabQuiz(quizDeck(), seeded(8)); reflect.DeepEqual(a, c) {
		t.Error("two seeds gave the same quiz")
	}
}

func TestVocabQuizWithFewerThanFourWordsIsTypedOnly(t *testing.T) {
	cards := quizDeck()[:6] // three words, both directions
	qs := vocabQuiz(cards, seeded(1))
	if len(qs) != 3 {
		t.Fatalf("%d questions, want one per word", len(qs))
	}
	for _, q := range qs {
		if !q.typed() {
			t.Errorf("%q is multiple choice with only 3 words", q.word)
		}
	}
}

func TestVocabQuizOfAnEmptyDeckHasNoQuestions(t *testing.T) {
	if qs := vocabQuiz(nil, seeded(1)); len(qs) != 0 {
		t.Errorf("questions %v, want none", qs)
	}
	// Cards without a meaning cannot be asked.
	if qs := vocabQuiz([]domain.Card{domain.NewCard("hello")}, seeded(1)); len(qs) != 0 {
		t.Errorf("questions %v, want none", qs)
	}
}

func TestVocabQuizNeverOffersADistractorWithTheSameMeaning(t *testing.T) {
	cards := []domain.Card{
		word("hello", "", "Hola", ""), word("hi", "", "hola", ""),
		word("dog", "", "perro", ""), word("cat", "", "gato", ""), word("cow", "", "vaca", ""),
	}
	for seed := range uint64(30) {
		for _, q := range vocabQuiz(cards, seeded(seed)) {
			if q.typed() {
				continue
			}
			if (q.word == "hello" && slices.Contains(q.options, "hi")) || (q.word == "hi" && slices.Contains(q.options, "hello")) {
				t.Fatalf("seed %d: %q offers a synonym: %v", seed, q.word, q.options)
			}
		}
	}
}

func TestVocabQuizFallsBackToTypedWithoutEnoughDistractors(t *testing.T) {
	// Four words, but two share a meaning: "hello" has only two distractors.
	cards := []domain.Card{
		word("hello", "", "hola", ""), word("hi", "", "Hola", ""),
		word("dog", "", "perro", ""), word("cat", "", "gato", ""),
	}
	for seed := range uint64(30) {
		for _, q := range vocabQuiz(cards, seeded(seed)) {
			if (q.word == "hello" || q.word == "hi") && !q.typed() {
				t.Fatalf("seed %d: %q is multiple choice with %v", seed, q.word, q.options)
			}
		}
	}
}

func TestVocabQuizCountsEachWordOnce(t *testing.T) {
	qs := vocabQuiz(deck(), seeded(3)) // seven words
	var words []string
	for _, q := range qs {
		words = append(words, strings.ToLower(q.word))
	}
	if len(qs) != 7 || len(slices.Compact(slices.Sorted(slices.Values(words)))) != 7 {
		t.Errorf("questions for %v, want each of the 7 words once", words)
	}
}
