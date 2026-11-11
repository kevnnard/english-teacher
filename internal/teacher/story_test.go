package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

var three = []string{"Her key", "Her cat", "Her bag"}

func chapterReply(title, question string) string {
	return fmt.Sprintf(`{"title":%q,"text":"Ana is a teacher. On Monday she cannot find her key.",`+
		`"summary":"Ana loses her key.","glossary":[`+
		`{"word":"key","meaning":"llave","example":"she cannot find her key","new":true},`+
		`{"word":"find","meaning":"encontrar","example":"she cannot find her key","new":true},`+
		`{"word":"lose","meaning":"perder","example":"x","new":true},`+
		`{"word":"wallet","meaning":"billetera","example":"x","new":true},`+
		`{"word":"teacher","meaning":"profesora","example":"Ana is a teacher."},`+
		`{"word":"","meaning":"vacío"}],`+
		`"question":%s}`, title, question)
}

func TestStoryChapterStartsANewStory(t *testing.T) {
	reading := &fakeModel{reply: chapterReply("The lost key", quizItemJSON("What does Ana lose?", three, 0))}
	other := &fakeModel{err: errors.New("wrong model")}
	tc := New(Models{Reading: reading, Writing: other, Conversation: other, Quiz: other})

	got, err := tc.StoryChapter(context.Background(), domain.A1, []string{"teacher", "cat"}, StorySoFar{})

	if err != nil {
		t.Fatal(err)
	}
	if len(reading.got) != 1 || len(other.got) != 0 {
		t.Fatalf("reading calls %d, other calls %d", len(reading.got), len(other.got))
	}
	if got.Title != "The lost key" || got.Summary != "Ana loses her key." || got.Question.Right() != "Her key" {
		t.Errorf("chapter = %+v", got)
	}
	p := reading.got[0].Prompt
	mustHave(t, p, "chapter 1 of 5", "level A1", "250 to 400 words", "teacher, cat", "at most 3 new words", "exactly 3 options")
	if strings.Contains(p, "Continue") {
		t.Errorf("a new story must not continue anything: %q", p)
	}
	// Blank entries go; only the first 3 new words stay new.
	news := 0
	for _, g := range got.Glossary {
		if g.New {
			news++
		}
	}
	if len(got.Glossary) != 5 || news != 3 {
		t.Errorf("glossary = %+v, want 5 entries and 3 new", got.Glossary)
	}
}

func TestStoryChapterContinuesTheSameStory(t *testing.T) {
	m := &fakeModel{reply: chapterReply("A different title", quizItemJSON("Who has the key?", three, 1))}
	tc := New(models(m))
	so := StorySoFar{Title: "The lost key", Chapter: 2, Summary: "Ana loses her key. She looks at school."}

	got, err := tc.StoryChapter(context.Background(), domain.A1, nil, so)

	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "The lost key" {
		t.Errorf("title = %q, want the story's title kept", got.Title)
	}
	mustHave(t, m.got[0].Prompt, "Continue the story \"The lost key\"", "chapter 3 of 5",
		"Ana loses her key. She looks at school.")
}

func TestStoryChapterEndsTheStoryInTheLastChapter(t *testing.T) {
	m := &fakeModel{reply: chapterReply("The lost key", quizItemJSON("Q?", three, 2))}
	tc := New(models(m))
	_, err := tc.StoryChapter(context.Background(), domain.B1, nil, StorySoFar{Title: "The lost key", Chapter: 4, Summary: "s"})
	if err != nil {
		t.Fatal(err)
	}
	mustHave(t, m.got[0].Prompt, "chapter 5 of 5", "last chapter", "at most 6 new words")
}

func TestStoryChapterRejectsBadAnswers(t *testing.T) {
	tests := map[string]string{
		"empty text":       `{"title":"t","text":"  ","summary":"s","question":` + quizItemJSON("Q?", three, 0) + `}`,
		"four options":     chapterReply("t", quizItemJSON("Q?", four, 0)),
		"answer out of 3":  chapterReply("t", quizItemJSON("Q?", three, 3)),
		"no question":      chapterReply("t", `{}`),
		"repeated options": chapterReply("t", quizItemJSON("Q?", []string{"a", "A", "b"}, 0)),
	}
	for name, reply := range tests {
		t.Run(name, func(t *testing.T) {
			tc := New(models(&fakeModel{reply: reply}))
			if _, err := tc.StoryChapter(context.Background(), domain.A1, nil, StorySoFar{}); err == nil {
				t.Error("want an error")
			}
		})
	}
}

func TestStoryChapterKeepsTheOldSummaryWhenNoneComesBack(t *testing.T) {
	reply := strings.Replace(chapterReply("t", quizItemJSON("Q?", three, 0)), `"summary":"Ana loses her key."`, `"summary":""`, 1)
	tc := New(models(&fakeModel{reply: reply}))
	got, err := tc.StoryChapter(context.Background(), domain.A1, nil, StorySoFar{Title: "t", Chapter: 1, Summary: "Before."})
	if err != nil || got.Summary != "Before." {
		t.Errorf("summary = %q, err %v; want the previous summary", got.Summary, err)
	}
}

func TestStoryNewWords(t *testing.T) {
	if StoryNewWords(domain.A1) != 3 || StoryNewWords(domain.A2) != 6 || StoryNewWords(domain.C2) != 6 {
		t.Errorf("StoryNewWords = %d %d %d", StoryNewWords(domain.A1), StoryNewWords(domain.A2), StoryNewWords(domain.C2))
	}
}

func mustHave(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %q", w, s)
		}
	}
}
