package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

func checkReadingReply(questions ...string) string {
	return fmt.Sprintf(`{"title":"The market","text":"Every Sunday Ana goes to the market.","questions":[%s]}`,
		strings.Join(questions, ","))
}

func TestCheckReadingParsesThreeQuestionsAndUsesTheReadingModel(t *testing.T) {
	reading := &fakeModel{reply: checkReadingReply(
		quizItemJSON("When does Ana go?", []string{"Sunday", "Monday", "Friday", "Never"}, 0),
		quizItemJSON("Where does she go?", []string{"School", "The market", "Work", "Home"}, 1),
		quizItemJSON("bad", []string{"one"}, 0), // dropped: not four options
		quizItemJSON("Who goes?", []string{"Ana", "Ben", "Tom", "Sue"}, 0),
		quizItemJSON("Extra?", []string{"a", "b", "c", "d"}, 0), // only three are kept
	)}
	other := &fakeModel{err: errors.New("wrong model")}
	tc := New(Models{Reading: reading, Writing: other, Conversation: other, Quiz: other})

	got, err := tc.CheckReading(context.Background(), domain.A2)

	if err != nil {
		t.Fatal(err)
	}
	if len(reading.got) != 1 || len(other.got) != 0 {
		t.Fatalf("reading calls %d, other calls %d", len(reading.got), len(other.got))
	}
	if got.Title != "The market" || len(got.Questions) != CheckQuestions || got.Questions[2].Question != "Who goes?" {
		t.Errorf("reading = %+v", got)
	}
	if !strings.Contains(reading.got[0].Prompt, "level A2") || !strings.Contains(reading.got[0].Prompt, "3 multiple-choice") {
		t.Errorf("prompt = %q", reading.got[0].Prompt)
	}
}

func TestCheckReadingNeedsThreeValidQuestions(t *testing.T) {
	m := &fakeModel{reply: checkReadingReply(quizItemJSON("a", four, 0), quizItemJSON("b", four, 1))}
	tc := New(Models{Reading: m})
	if _, err := tc.CheckReading(context.Background(), domain.A2); err == nil {
		t.Error("want an error with only 2 questions")
	}
	m.reply = `{"title":"x","text":"  ","questions":[]}`
	if _, err := tc.CheckReading(context.Background(), domain.A2); err == nil {
		t.Error("want an error for an empty text")
	}
}
