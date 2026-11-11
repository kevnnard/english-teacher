package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// CheckQuestions is how many comprehension questions the reading of a
// level check has.
const CheckQuestions = 3

// CheckReading is the graded reading of a level check: a short text at
// the level being checked and multiple-choice questions about it.
type CheckReading struct {
	Title     string
	Text      string
	Questions []QuizItem // exactly CheckQuestions valid items
}

// CheckReading asks the reading model for a short text at level, the
// level the learner wants to reach, with CheckQuestions multiple-choice
// comprehension questions. Invalid questions are dropped like in
// GrammarQuiz; fewer than CheckQuestions valid ones is an error.
func (t *Teacher) CheckReading(ctx context.Context, level domain.Level) (CheckReading, error) {
	r := Request{
		System: learner(level) + jsonOnly +
			`{"title": string, "text": string, "questions": [{"question": string, "options": [string], "answer": number, "why": string}]}`,
		Prompt: fmt.Sprintf("This is a level check. Write a short text (120 to 160 words) at level %s on an everyday topic, "+
			"then %d multiple-choice comprehension questions in English about it. "+
			"Each question has exactly %d options, all different, and only one is correct; answer is the index of the "+
			"correct option, counting from 0. why is one short sentence in Spanish that points to the part of the text "+
			"with the answer.",
			level, CheckQuestions, QuizOptions),
	}
	var out struct {
		Title     string     `json:"title"`
		Text      string     `json:"text"`
		Questions []QuizItem `json:"questions"`
	}
	if err := ask(ctx, t.models.Reading, r, &out); err != nil {
		return CheckReading{}, err
	}
	if strings.TrimSpace(out.Text) == "" {
		return CheckReading{}, errors.New("teacher: the level check reading came back empty")
	}
	var questions []QuizItem
	for _, q := range out.Questions {
		if q, ok := cleanItem(q); ok && len(questions) < CheckQuestions {
			questions = append(questions, q)
		}
	}
	if len(questions) < CheckQuestions {
		return CheckReading{}, fmt.Errorf("teacher: the level check reading came back with only %d valid questions", len(questions))
	}
	return CheckReading{Title: strings.TrimSpace(out.Title), Text: strings.TrimSpace(out.Text), Questions: questions}, nil
}
