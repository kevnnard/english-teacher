package teacher

import (
	"context"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// GrammarItems is how many items a grammar quiz asks for, and the most it keeps.
const GrammarItems = 8

// MinGrammarItems is the fewest valid items a grammar quiz needs; with
// fewer, the quiz is an error.
const MinGrammarItems = 3

// QuizOptions is the number of options of every multiple-choice item.
const QuizOptions = 4

// QuizItem is one multiple-choice grammar question.
type QuizItem struct {
	Question string   `json:"question"` // in English, usually a sentence with a ___ blank
	Options  []string `json:"options"`  // exactly QuizOptions different answers
	Answer   int      `json:"answer"`   // the index in Options of the right answer, from 0
	Why      string   `json:"why"`      // why the answer is right, in Spanish
}

// Right is the text of the right option.
func (q QuizItem) Right() string {
	return q.Options[q.Answer]
}

// GrammarQuiz is a set of grammar items and what they practice.
type GrammarQuiz struct {
	Topics []string   // the learner's labels of what the items practice
	Items  []QuizItem // between MinGrammarItems and GrammarItems valid items
}

// coreGrammar is the grammar each level works on first, for a quiz when
// the learner has no recurring errors yet.
var coreGrammar = map[domain.Level][]string{
	domain.A1: {"to be", "present simple", "articles", "plurals", "possessives"},
	domain.A2: {"past simple", "going to", "comparatives", "countable and uncountable nouns", "prepositions of time and place"},
	domain.B1: {"present perfect", "first and second conditionals", "modal verbs", "gerunds and infinitives"},
	domain.B2: {"third conditional", "passive voice", "reported speech", "relative clauses"},
	domain.C1: {"mixed conditionals", "inversion", "cleft sentences", "modals of deduction"},
	domain.C2: {"subtle tense contrasts", "advanced inversion", "ellipsis and substitution", "nuanced modality"},
}

// CoreGrammar is the core grammar of level; an unknown level gets A1's.
func CoreGrammar(level domain.Level) []string {
	if g, ok := coreGrammar[level]; ok {
		return g
	}
	return coreGrammar[domain.A1]
}

// GrammarTopics is what a grammar quiz practices: the labels of the
// learner's recurring error kinds in focus, or the core grammar of level
// when there are none.
func GrammarTopics(level domain.Level, focus []string) []string {
	if len(focus) == 0 {
		return CoreGrammar(level)
	}
	labels := make([]string, len(focus))
	for i, k := range focus {
		labels[i] = KindLabel(k)
	}
	return labels
}

// GrammarQuiz asks the quiz model for about GrammarItems multiple-choice
// items for the learner's current lesson: the recurring error kinds in
// focus, or the core grammar of the level. Items with a bad answer index
// or the wrong number of options are dropped; fewer than MinGrammarItems
// valid items is an error.
func (t *Teacher) GrammarQuiz(ctx context.Context, level domain.Level, focus []string) (GrammarQuiz, error) {
	topics := GrammarTopics(level, focus)
	lesson := fmt.Sprintf("The current lesson is the core grammar of level %s: %s.", level, strings.Join(topics, ", "))
	if len(focus) > 0 {
		lesson = "The current lesson is the learner's recurring errors with: " + strings.Join(topics, ", ") +
			". Every item practices one of them."
	}
	r := Request{
		System: learner(level) + jsonOnly +
			`{"items": [{"question": string, "options": [string], "answer": number, "why": string}]}`,
		Prompt: fmt.Sprintf("Write a grammar quiz of %d multiple-choice items for level %s. %s "+
			"Each question is one short English sentence with a ___ blank, or a short question, at the learner's level. "+
			"Each item has exactly %d options, all different, and only one is correct; answer is the index of the "+
			"correct option, counting from 0. why is one short sentence in Spanish that explains the answer.",
			GrammarItems, level, lesson, QuizOptions),
	}
	var out struct {
		Items []QuizItem `json:"items"`
	}
	if err := ask(ctx, t.models.Quiz, r, &out); err != nil {
		return GrammarQuiz{}, err
	}
	var items []QuizItem
	for _, it := range out.Items {
		if it, ok := cleanItem(it); ok && len(items) < GrammarItems {
			items = append(items, it)
		}
	}
	if len(items) < MinGrammarItems {
		return GrammarQuiz{}, fmt.Errorf("teacher: the grammar quiz came back with only %d valid items", len(items))
	}
	return GrammarQuiz{Topics: topics, Items: items}, nil
}

// cleanItem trims an item and reports whether it is valid: a question,
// exactly QuizOptions different options and an answer index among them.
func cleanItem(it QuizItem) (QuizItem, bool) {
	return cleanChoice(it, QuizOptions)
}

// cleanChoice is cleanItem for a question with n options.
func cleanChoice(it QuizItem, n int) (QuizItem, bool) {
	it.Question, it.Why = strings.TrimSpace(it.Question), strings.TrimSpace(it.Why)
	if it.Question == "" || len(it.Options) != n || it.Answer < 0 || it.Answer >= n {
		return QuizItem{}, false
	}
	options := make([]string, 0, n)
	for _, o := range it.Options {
		o = strings.TrimSpace(o)
		if o == "" || hasFold(options, o) {
			return QuizItem{}, false
		}
		options = append(options, o)
	}
	it.Options = options
	return it, true
}
