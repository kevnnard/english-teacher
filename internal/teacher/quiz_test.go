package teacher

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// quizItemJSON writes one grammar item as the model would.
func quizItemJSON(q string, options []string, answer int) string {
	quoted := make([]string, len(options))
	for i, o := range options {
		quoted[i] = fmt.Sprintf("%q", o)
	}
	return fmt.Sprintf(`{"question":%q,"options":[%s],"answer":%d,"why":"Con I se usa am."}`,
		q, strings.Join(quoted, ","), answer)
}

func quizReply(items ...string) string {
	return `{"items":[` + strings.Join(items, ",") + `]}`
}

var four = []string{"am", "is", "are", "be"}

func TestGrammarQuizParsesTheItemsAndUsesTheQuizModel(t *testing.T) {
	quiz := &fakeModel{reply: "```json\n" + quizReply(
		quizItemJSON("I ___ a student.", []string{" am ", "is", "are", "be"}, 0),
		quizItemJSON("She ___ from Brazil.", four, 1),
		quizItemJSON("They ___ my friends.", four, 2),
	) + "\n```"}
	other := &fakeModel{err: errors.New("wrong model")}
	tc := New(Models{Reading: other, Writing: other, Conversation: other, Quiz: quiz})

	got, err := tc.GrammarQuiz(context.Background(), domain.A1, nil)

	if err != nil {
		t.Fatal(err)
	}
	if len(quiz.got) != 1 || len(other.got) != 0 {
		t.Fatalf("quiz model calls %d, other calls %d; want 1 and 0", len(quiz.got), len(other.got))
	}
	want := QuizItem{Question: "I ___ a student.", Options: []string{"am", "is", "are", "be"}, Answer: 0, Why: "Con I se usa am."}
	if len(got.Items) != 3 || !reflect.DeepEqual(got.Items[0], want) {
		t.Fatalf("items = %+v", got.Items)
	}
	if got.Items[2].Right() != "are" {
		t.Errorf("Right() = %q, want are", got.Items[2].Right())
	}
	if !reflect.DeepEqual(got.Topics, CoreGrammar(domain.A1)) {
		t.Errorf("topics = %v, want the A1 core grammar", got.Topics)
	}
}

func TestGrammarQuizPromptTargetsTheLevelCoreGrammarWithoutFocus(t *testing.T) {
	m := &fakeModel{reply: quizReply(quizItemJSON("a", four, 0), quizItemJSON("b", four, 1), quizItemJSON("c", four, 2))}
	if _, err := New(models(m)).GrammarQuiz(context.Background(), domain.A1, nil); err != nil {
		t.Fatal(err)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	mustHave := []string{"A1", "8 multiple-choice", "exactly 4 options", "index", "Spanish",
		"core grammar of level A1", "to be", "present simple", "articles", "plurals", "possessives",
		`"items"`, `"answer": number`}
	for _, want := range mustHave {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "recurring errors") {
		t.Errorf("a prompt without focus talks about recurring errors:\n%s", prompt)
	}
}

func TestGrammarQuizPromptTargetsTheRecurringErrors(t *testing.T) {
	m := &fakeModel{reply: quizReply(quizItemJSON("a", four, 0), quizItemJSON("b", four, 1), quizItemJSON("c", four, 2))}
	got, err := New(models(m)).GrammarQuiz(context.Background(), domain.A2, []string{"article", "be"})
	if err != nil {
		t.Fatal(err)
	}
	prompt := m.got[0].Prompt
	mustHave := []string{"recurring errors", "articles (a/an/the)", "to be (am/is/are)"}
	for _, want := range mustHave {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "core grammar") {
		t.Errorf("a focused prompt falls back to the core grammar:\n%s", prompt)
	}
	if want := []string{"articles (a/an/the)", "to be (am/is/are)"}; !reflect.DeepEqual(got.Topics, want) {
		t.Errorf("topics = %v, want %v", got.Topics, want)
	}
}

func TestCoreGrammarCoversEveryLevel(t *testing.T) {
	for _, l := range []domain.Level{domain.A1, domain.A2, domain.B1, domain.B2, domain.C1, domain.C2} {
		if len(CoreGrammar(l)) < 3 {
			t.Errorf("CoreGrammar(%s) = %v", l, CoreGrammar(l))
		}
	}
	if !reflect.DeepEqual(CoreGrammar(domain.Level("")), CoreGrammar(domain.A1)) {
		t.Error("an unknown level must fall back to A1")
	}
}

func TestGrammarQuizDropsBadItems(t *testing.T) {
	m := &fakeModel{reply: quizReply(
		quizItemJSON("good one", four, 3),
		quizItemJSON("index too big", four, 4),
		quizItemJSON("negative index", four, -1),
		quizItemJSON("three options", []string{"am", "is", "are"}, 0),
		quizItemJSON("five options", []string{"am", "is", "are", "be", "been"}, 0),
		quizItemJSON("an empty option", []string{"am", " ", "are", "be"}, 0),
		quizItemJSON("same option twice", []string{"am", "AM", "are", "be"}, 0),
		quizItemJSON("  ", four, 0),
		quizItemJSON("good two", four, 1),
		quizItemJSON("good three", four, 2),
	)}
	got, err := New(models(m)).GrammarQuiz(context.Background(), domain.A1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var questions []string
	for _, it := range got.Items {
		questions = append(questions, it.Question)
	}
	if want := []string{"good one", "good two", "good three"}; !reflect.DeepEqual(questions, want) {
		t.Errorf("kept %v, want %v", questions, want)
	}
}

func TestGrammarQuizKeepsAtMostEightItems(t *testing.T) {
	var items []string
	for i := range 11 {
		items = append(items, quizItemJSON(fmt.Sprint("q", i), four, i%4))
	}
	got, err := New(models(&fakeModel{reply: quizReply(items...)})).GrammarQuiz(context.Background(), domain.A1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != GrammarItems || got.Items[7].Question != "q7" {
		t.Errorf("kept %d items, want the first %d", len(got.Items), GrammarItems)
	}
}

func TestGrammarQuizWithTooFewValidItemsIsAnError(t *testing.T) {
	m := &fakeModel{reply: quizReply(quizItemJSON("a", four, 0), quizItemJSON("b", four, 9), quizItemJSON("c", four, 1))}
	_, err := New(models(m)).GrammarQuiz(context.Background(), domain.A1, nil)
	if err == nil || !strings.Contains(err.Error(), "2 valid") {
		t.Errorf("err = %v, want an error naming the 2 valid items", err)
	}
}

func TestGrammarQuizErrors(t *testing.T) {
	if _, err := New(Models{}).GrammarQuiz(context.Background(), domain.A1, nil); !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel", err)
	}
	_, err := New(models(&fakeModel{err: errors.New("pi: timeout")})).GrammarQuiz(context.Background(), domain.A1, nil)
	if err == nil || !strings.Contains(err.Error(), "pi: timeout") {
		t.Errorf("err = %v", err)
	}
}
