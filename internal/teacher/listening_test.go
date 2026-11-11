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

// listeningReply is a model answer with the given lines (as JSON) and
// questions.
func listeningReply(lines string, questions ...string) string {
	return fmt.Sprintf(`{"title":" At the market ","lines":%s,"questions":[%s]}`, lines, strings.Join(questions, ","))
}

const dialogueLines = `[` +
	`{"speaker":"Ana","text":"Hi Tom! Are you hungry? I want some fruit."},` +
	`{"speaker":" Tom ","text":"Yes, I am."},` +
	`{"speaker":"Ana","text":"  "},` +
	`{"speaker":"","text":"Mr. Lee sells apples here."}]`

var abc = []string{"Fruit", "Bread", "Fish"}

func TestListeningTextWritesADialogueSentenceBySentence(t *testing.T) {
	reading := &fakeModel{reply: listeningReply(dialogueLines,
		quizItemJSON("What does Ana want?", abc, 0),
		quizItemJSON("Is Tom hungry?", []string{"Yes", "No", "Maybe"}, 0),
		quizItemJSON("Bad: two options", []string{"a", "b"}, 0),
		quizItemJSON("Who sells apples?", []string{"Mr. Lee", "Ana", "Tom"}, 0))}
	other := &fakeModel{err: errors.New("wrong model")}
	tc := New(Models{Reading: reading, Writing: other, Conversation: other, Quiz: other})

	got, err := tc.ListeningText(context.Background(), domain.A1, []string{"apple", "hungry"}, ListeningDialogue)

	if err != nil {
		t.Fatal(err)
	}
	if len(reading.got) != 1 || len(other.got) != 0 {
		t.Fatalf("reading calls %d, other calls %d", len(reading.got), len(other.got))
	}
	p := reading.got[0].Prompt
	mustHave(t, p, "dialogue between 2 people", "level A1", "250 to 600 words", "apple, hungry",
		"at most 3 new words", "exactly 3 options", "3 to 5")
	if got.Title != "At the market" || got.Kind != ListeningDialogue {
		t.Errorf("title %q kind %q", got.Title, got.Kind)
	}
	// One line per sentence; blank lines go; a line without a speaker
	// keeps the one before; "Mr." does not end a sentence.
	want := []ListeningLine{
		{"Ana", "Hi Tom!"}, {"Ana", "Are you hungry?"}, {"Ana", "I want some fruit."},
		{"Tom", "Yes, I am."},
		{"Tom", "Mr. Lee sells apples here."},
	}
	if !reflect.DeepEqual(got.Lines, want) {
		t.Errorf("lines = %q\nwant    %q", got.Lines, want)
	}
	if len(got.Questions) != 3 || got.Questions[2].Right() != "Mr. Lee" {
		t.Errorf("questions = %+v", got.Questions)
	}
}

func TestListeningTextStoryHasOneNarrator(t *testing.T) {
	m := &fakeModel{reply: listeningReply(`[{"speaker":"Ana","text":"Ana has a dog. It is big."}]`,
		quizItemJSON("What does Ana have?", []string{"A dog", "A cat", "A bird"}, 0),
		quizItemJSON("Is it big?", []string{"Yes", "No", "Maybe"}, 0),
		quizItemJSON("Q3?", []string{"x", "y", "z"}, 2),
		quizItemJSON("Q4?", []string{"x", "y", "z"}, 2),
		quizItemJSON("Q5?", []string{"x", "y", "z"}, 2),
		quizItemJSON("Q6 is one too many?", []string{"x", "y", "z"}, 2))}
	got, err := New(models(m)).ListeningText(context.Background(), domain.B1, nil, ListeningStory)
	if err != nil {
		t.Fatal(err)
	}
	mustHave(t, m.got[0].Prompt, "short story", "level B1", "at most 6 new words", "(none yet)")
	want := []ListeningLine{{ListeningNarrator, "Ana has a dog."}, {ListeningNarrator, "It is big."}}
	if !reflect.DeepEqual(got.Lines, want) {
		t.Errorf("lines = %q, want %q", got.Lines, want)
	}
	if len(got.Questions) != MaxListeningQuestions {
		t.Errorf("kept %d questions, want %d", len(got.Questions), MaxListeningQuestions)
	}
}

func TestListeningTextErrors(t *testing.T) {
	two := []string{quizItemJSON("Q1?", abc, 0), quizItemJSON("Q2?", abc, 1)}
	tests := []struct {
		name  string
		model Model
		want  string
	}{
		{"no model", nil, ErrNoModel.Error()},
		{"model fails", &fakeModel{err: errors.New("pi crashed")}, "pi crashed"},
		{"no text", &fakeModel{reply: listeningReply(`[{"speaker":"Ana","text":" "}]`, two...)}, "came back empty"},
		{"too few questions", &fakeModel{reply: listeningReply(dialogueLines, two...)}, "only 2 valid questions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(Models{Reading: tt.model}).ListeningText(context.Background(), domain.A1, nil, ListeningDialogue)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSplitSentences(t *testing.T) {
	tests := map[string][]string{
		"One. Two!  Three?":             {"One.", "Two!", "Three?"},
		`She said "Hi." Then she left.`: {`She said "Hi."`, "Then she left."},
		"Dr. Smith is here. Mrs. Brown": {"Dr. Smith is here.", "Mrs. Brown"},
		"It costs 2.50 dollars.":        {"It costs 2.50 dollars."},
		"  ":                            nil,
	}
	for in, want := range tests {
		if got := splitSentences(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitSentences(%q) = %q, want %q", in, got, want)
		}
	}
}
