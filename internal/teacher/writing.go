package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// MaxScore is the top score of a correction.
const MaxScore = 10

// WritingTask is a short writing assignment.
type WritingTask struct {
	Task string `json:"task"` // in English, what to write
	Hint string `json:"hint"` // in Spanish, how to start
}

// Correction is the teacher's feedback on a text.
type Correction struct {
	Corrected string `json:"corrected"` // the whole text, corrected with as few changes as possible
	Score     int    `json:"score"`     // 0 to MaxScore
	Comment   string `json:"comment"`   // one or two encouraging sentences, in Spanish
	Fixes     []Fix  `json:"fixes"`
}

// Fix is one error and its correction.
type Fix struct {
	Wrong   string `json:"wrong"`   // what the learner wrote
	Right   string `json:"right"`   // the correct chunk
	Meaning string `json:"meaning"` // what the right chunk means, in Spanish
	Why     string `json:"why"`     // a short explanation, in Spanish
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

// WritingTask asks for a short assignment that practices recent words.
func (t *Teacher) WritingTask(ctx context.Context, level domain.Level, recent []string) (WritingTask, error) {
	r := Request{
		System: learner(level) + jsonOnly + `{"task": string, "hint": string}`,
		Prompt: fmt.Sprintf("Give one short writing task for level %s about everyday life (3 to 6 sentences). "+
			"It should give the learner a reason to use some of these recent words: %s. "+
			"The task is in simple English; the hint is in Spanish and suggests how to start.", level, list(recent)),
	}
	var out WritingTask
	if err := ask(ctx, t.models.Writing, r, &out); err != nil {
		return WritingTask{}, err
	}
	if strings.TrimSpace(out.Task) == "" {
		return WritingTask{}, errors.New("teacher: the writing task came back empty")
	}
	return out, nil
}

// Correct corrects text, written for task. Fixes are recasts: the correct
// chunk is given, not only the rule, because learners notice and keep a
// recast more often than a bare rule.
func (t *Teacher) Correct(ctx context.Context, level domain.Level, task, text string) (Correction, error) {
	r := Request{
		System: learner(level) + jsonOnly +
			`{"corrected": string, "score": number, "comment": string, ` +
			`"fixes": [{"wrong": string, "right": string, "meaning": string, "why": string}]}`,
		Prompt: fmt.Sprintf("Correct the learner's text. Change as little as possible: fix grammar, word choice, "+
			"spelling and punctuation, and keep the learner's own ideas. Each fix is the smallest wrong chunk "+
			"and its correct version (for example \"I has 30 years\" -> \"I am 30 years old\"), its Spanish meaning, "+
			"and a one-sentence explanation in Spanish. Score the text from 0 to %d for its level. "+
			"If there are no errors, return an empty fixes list.\n\nTask: %s\n\nText:\n%s", MaxScore, task, text),
	}
	var out Correction
	if err := ask(ctx, t.models.Writing, r, &out); err != nil {
		return Correction{}, err
	}
	out.Score = min(max(out.Score, 0), MaxScore)
	var fixes []Fix
	for _, f := range out.Fixes {
		wrong, right := strings.TrimSpace(f.Wrong), strings.TrimSpace(f.Right)
		if wrong != "" && right != "" && !strings.EqualFold(wrong, right) {
			fixes = append(fixes, f)
		}
	}
	out.Fixes = fixes
	return out, nil
}
