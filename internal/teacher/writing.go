package teacher

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// MaxPhrases is the most "useful phrases" a writing task keeps.
const MaxPhrases = 4

// MaxMajors is the most errors a correction marks as major. Focused
// feedback on a few errors helps more than correcting everything.
const MaxMajors = 3

// RubricMax is the top score of each rubric criterion.
const RubricMax = 10

// MaxTotal is the top total of a correction: four criteria of RubricMax.
const MaxTotal = 4 * RubricMax

// WritingTask is a short writing assignment.
type WritingTask struct {
	Task    string   `json:"task"`    // in English, what to write
	Hint    string   `json:"hint"`    // in Spanish, how to start
	Phrases []string `json:"phrases"` // 2 to MaxPhrases chunks to use, preferably the learner's own words
}

// Rubric scores a text like the english-master homework reviews: each
// criterion goes from 0 to RubricMax.
type Rubric struct {
	Grammar    int `json:"grammar"`
	Vocabulary int `json:"vocabulary"`
	Coherence  int `json:"coherence"`
	Task       int `json:"task"` // task completion
}

// clamp keeps every criterion between 0 and RubricMax.
func (r Rubric) clamp() Rubric {
	c := func(n int) int { return min(max(n, 0), RubricMax) }
	return Rubric{Grammar: c(r.Grammar), Vocabulary: c(r.Vocabulary), Coherence: c(r.Coherence), Task: c(r.Task)}
}

// Correction is the teacher's feedback on a text.
type Correction struct {
	Corrected string `json:"corrected"` // the whole text, corrected with as few changes as possible
	Rubric    Rubric `json:"rubric"`
	Comment   string `json:"comment"` // one or two encouraging sentences, in Spanish
	Fixes     []Fix  `json:"fixes"`
}

// Total is the sum of the rubric, out of MaxTotal.
func (c Correction) Total() int {
	r := c.Rubric
	return r.Grammar + r.Vocabulary + r.Coherence + r.Task
}

// Grade is the letter of Total: A from 90%, B from 80%, C from 70%, D from
// 60% and F below.
func (c Correction) Grade() string {
	percent := c.Total() * 100 / MaxTotal
	switch {
	case percent >= 90:
		return "A"
	case percent >= 80:
		return "B"
	case percent >= 70:
		return "C"
	case percent >= 60:
		return "D"
	}
	return "F"
}

// majorIndexes are the positions in Fixes of the first MaxMajors fixes the
// model marked major. Unmarked fixes are never promoted: a text with only
// minor slips has no major errors to work on.
func (c Correction) majorIndexes() map[int]bool {
	marked := map[int]bool{}
	for i, f := range c.Fixes {
		if f.Major && len(marked) < MaxMajors {
			marked[i] = true
		}
	}
	return marked
}

// Majors are the at most MaxMajors errors the learner works on, in order.
func (c Correction) Majors() []Fix {
	return c.split(true)
}

// Minors are every fix that is not in Majors, in order.
func (c Correction) Minors() []Fix {
	return c.split(false)
}

func (c Correction) split(major bool) []Fix {
	majors := c.majorIndexes()
	var out []Fix
	for i, f := range c.Fixes {
		if majors[i] == major {
			out = append(out, f)
		}
	}
	return out
}

// Fix is one error and its correction.
type Fix struct {
	Wrong   string `json:"wrong"`   // what the learner wrote
	Right   string `json:"right"`   // the correct chunk
	Meaning string `json:"meaning"` // what the right chunk means, in Spanish
	Why     string `json:"why"`     // a short explanation, in Spanish
	Major   bool   `json:"major"`   // the error matters most at the learner's level
	Kind    string `json:"kind"`    // one of the names in Kinds
}

// Card turns the fix into a card that practices the correct chunk. The
// learner's version is kept as the last wrong answer, so the review shows
// it in the margin notes; the learner's sentence is the example.
func (f Fix) Card(sentence string) domain.Card {
	c := domain.NewCard(strings.TrimSpace(f.Right))
	c.Meaning = strings.TrimSpace(f.Meaning)
	if c.Meaning == "" {
		c.Meaning = strings.TrimSpace(f.Why)
	}
	c.Example = strings.TrimSpace(sentence)
	c.LastWrong = strings.TrimSpace(f.Wrong)
	return c
}

// WritingTask asks for a short assignment that practices recent words and,
// when focus names error kinds the learner keeps making, makes the learner
// use those structures. Planning with ready-made chunks before writing
// helps a beginner write more and more accurately.
func (t *Teacher) WritingTask(ctx context.Context, level domain.Level, recent, focus []string) (WritingTask, error) {
	prompt := fmt.Sprintf("Give one short writing task for level %s about everyday life (3 to 6 sentences). "+
		"It should give the learner a reason to use some of these recent words: %s. "+
		"The task is in simple English; the hint is in Spanish and suggests how to start. "+
		"Add 2 to %d useful phrases: short English chunks the learner can reuse in the text, "+
		"taken from the recent words when they fit.", level, list(recent), MaxPhrases)
	if len(focus) > 0 {
		labels := make([]string, len(focus))
		for i, k := range focus {
			labels[i] = KindLabel(k)
		}
		prompt += " The learner has recurring errors with: " + strings.Join(labels, ", ") +
			". Choose a task that makes the learner use those structures, and include a phrase that models them."
	}
	r := Request{
		System: learner(level) + jsonOnly + `{"task": string, "hint": string, "phrases": [string]}`,
		Prompt: prompt,
	}
	var out WritingTask
	if err := ask(ctx, t.models.Writing, r, &out); err != nil {
		return WritingTask{}, err
	}
	if strings.TrimSpace(out.Task) == "" {
		return WritingTask{}, errors.New("teacher: the writing task came back empty")
	}
	var phrases []string
	for _, p := range out.Phrases {
		if p = strings.TrimSpace(p); p != "" && len(phrases) < MaxPhrases && !hasFold(phrases, p) {
			phrases = append(phrases, p)
		}
	}
	out.Phrases = fillPhrases(phrases, recent)
	return out, nil
}

// MinPhrases is the fewest "useful phrases" a task shows when the learner
// has words to draw on.
const MinPhrases = 2

// fillPhrases tops phrases up to MinPhrases with the learner's recent
// words, skipping any already there, because the model may return fewer.
func fillPhrases(phrases, recent []string) []string {
	for _, w := range recent {
		if len(phrases) >= MinPhrases {
			break
		}
		w = strings.TrimSpace(w)
		if w != "" && !hasFold(phrases, w) {
			phrases = append(phrases, w)
		}
	}
	return phrases
}

// hasFold reports whether phrases already holds s, ignoring case.
func hasFold(phrases []string, s string) bool {
	return slices.ContainsFunc(phrases, func(p string) bool { return strings.EqualFold(p, s) })
}

// Correct corrects text, written for task. Fixes are recasts: the correct
// chunk is given, not only the rule, because learners notice and keep a
// recast more often than a bare rule. At most MaxMajors fixes are marked
// major; the app asks the learner to fix those first.
func (t *Teacher) Correct(ctx context.Context, level domain.Level, task, text string) (Correction, error) {
	r := Request{
		System: learner(level) + jsonOnly +
			`{"corrected": string, "rubric": {"grammar": number, "vocabulary": number, "coherence": number, "task": number}, ` +
			`"comment": string, "fixes": [{"wrong": string, "right": string, "meaning": string, "why": string, ` +
			`"major": boolean, "kind": string}]}`,
		Prompt: fmt.Sprintf("Correct the learner's text. Change as little as possible: fix grammar, word choice, "+
			"spelling and punctuation, and keep the learner's own ideas. Each fix is the smallest wrong chunk, "+
			"copied exactly as the learner wrote it, and its correct version (for example \"I has 30 years\" -> "+
			"\"I am 30 years old\"), its Spanish meaning, and a one-sentence explanation in Spanish. "+
			"Mark at most %d fixes as major (major true): the errors that matter most for being understood at "+
			"this level, repeated patterns first; every other fix is minor (major false). "+
			"The kind of each fix is exactly one of: %s. "+
			"Score the text for its level from 0 to %d on each rubric criterion: grammar, vocabulary, "+
			"coherence and task (how well it does the task). "+
			"If there are no errors, return an empty fixes list.\n\nTask: %s\n\nText:\n%s",
			MaxMajors, kindNames(), RubricMax, task, text),
	}
	var out Correction
	if err := ask(ctx, t.models.Writing, r, &out); err != nil {
		return Correction{}, err
	}
	out.Rubric = out.Rubric.clamp()
	var fixes []Fix
	for _, f := range out.Fixes {
		wrong, right := strings.TrimSpace(f.Wrong), strings.TrimSpace(f.Right)
		if wrong != "" && right != "" && !strings.EqualFold(wrong, right) {
			f.Kind = NormalizeKind(f.Kind)
			fixes = append(fixes, f)
		}
	}
	out.Fixes = fixes
	return out, nil
}
