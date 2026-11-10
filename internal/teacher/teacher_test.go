package teacher

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kevnnard/english-teacher/internal/domain"
)

// fakeModel answers every request with reply and remembers the requests.
type fakeModel struct {
	reply string
	err   error
	got   []Request
}

func (f *fakeModel) Complete(_ context.Context, r Request) (string, error) {
	f.got = append(f.got, r)
	return f.reply, f.err
}

func models(m Model) Models {
	return Models{Reading: m, Writing: m, Conversation: m, Quiz: m}
}

func TestReadingParsesTheTextAndGlossary(t *testing.T) {
	m := &fakeModel{reply: "```json\n" + `{"title":"A new friend","text":"Hello! My name is Ana. I am a teacher.",
		"glossary":[{"word":"teacher","meaning":"profesor/a","example":"I am a teacher.","new":true},
		            {"word":"hello","meaning":"hola","example":"Hello!","new":false}]}` + "\n```"}
	tc := New(models(m))

	r, err := tc.Reading(context.Background(), domain.A1, []string{"hello", "name"})

	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "A new friend" || !strings.Contains(r.Text, "My name is Ana") || len(r.Glossary) != 2 {
		t.Fatalf("reading = %+v", r)
	}
	if g := r.Glossary[0]; g.Word != "teacher" || g.Meaning != "profesor/a" || !g.New {
		t.Errorf("glossary[0] = %+v", g)
	}
	prompt := m.got[0].System + m.got[0].Prompt
	for _, want := range []string{"A1", "hello, name", "at most 3 new"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
}

func TestReadingRejectsAnEmptyText(t *testing.T) {
	tc := New(models(&fakeModel{reply: `{"title":"x","text":"  ","glossary":[]}`}))
	if _, err := tc.Reading(context.Background(), domain.A1, nil); err == nil {
		t.Error("want an error for an empty text")
	}
}

func TestBadJSONIsAClearError(t *testing.T) {
	tc := New(models(&fakeModel{reply: "Sure! Here is a text about cats."}))
	_, err := tc.Reading(context.Background(), domain.A1, nil)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("err = %v, want a JSON error", err)
	}
}

func TestModelErrorsAreWrapped(t *testing.T) {
	tc := New(models(&fakeModel{err: errors.New("pi: not found")}))
	_, err := tc.Correct(context.Background(), domain.A1, "Introduce yourself", "I has 30 years.")
	if err == nil || !strings.Contains(err.Error(), "pi: not found") {
		t.Errorf("err = %v", err)
	}
}

func TestMissingModelIsAnError(t *testing.T) {
	tc := New(Models{})
	if _, err := tc.WritingTask(context.Background(), domain.A1, nil); !errors.Is(err, ErrNoModel) {
		t.Errorf("err = %v, want ErrNoModel", err)
	}
}

func TestWritingTask(t *testing.T) {
	m := &fakeModel{reply: `{"task":"Write 4 sentences about your family.","hint":"Usá: I have, my brother is..."}`}

	task, err := New(models(m)).WritingTask(context.Background(), domain.A1, []string{"brother"})

	if err != nil || task.Task != "Write 4 sentences about your family." || !strings.HasPrefix(task.Hint, "Usá") {
		t.Errorf("task = %+v, %v", task, err)
	}
}

func TestCorrectParsesFixesAndClampsTheScore(t *testing.T) {
	m := &fakeModel{reply: `{"corrected":"I am 30 years old.","score":14,"comment":"¡Bien!",
		"fixes":[{"wrong":"I has 30 years","right":"I am 30 years old","meaning":"tengo 30 años","why":"La edad va con to be."}]}`}

	c, err := New(models(m)).Correct(context.Background(), domain.A1, "Introduce yourself", "I has 30 years.")

	if err != nil {
		t.Fatal(err)
	}
	if c.Corrected != "I am 30 years old." || c.Score != 10 || len(c.Fixes) != 1 {
		t.Fatalf("correction = %+v", c)
	}
	if !strings.Contains(m.got[0].Prompt, "I has 30 years.") || !strings.Contains(m.got[0].Prompt, "Introduce yourself") {
		t.Errorf("prompt = %q", m.got[0].Prompt)
	}
}

func TestCorrectDropsFixesThatChangeNothing(t *testing.T) {
	m := &fakeModel{reply: `{"corrected":"Hi.","score":9,"fixes":[{"wrong":"Hi","right":"Hi","why":"ok"},{"wrong":"","right":"x"}]}`}
	c, err := New(models(m)).Correct(context.Background(), domain.A1, "t", "Hi.")
	if err != nil || len(c.Fixes) != 0 {
		t.Errorf("fixes = %+v, %v", c.Fixes, err)
	}
}

func TestFixCard(t *testing.T) {
	f := Fix{Wrong: "I has 30 years", Right: "I am 30 years old", Meaning: "tengo 30 años", Why: "La edad va con to be."}

	c := f.Card("I has 30 years and I am from Argentina.")

	if c.Word != "I am 30 years old" || c.Meaning != "tengo 30 años" || c.LastWrong != "I has 30 years" {
		t.Errorf("card = %+v", c)
	}
	if c.Example != "I has 30 years and I am from Argentina." || c.EaseFactor != domain.DefaultEaseFactor {
		t.Errorf("card = %+v", c)
	}
}

func TestGlossCard(t *testing.T) {
	c := Gloss{Word: "teacher", Meaning: "profesor/a", Example: "I am a teacher."}.Card()
	if c.Word != "teacher" || c.Meaning != "profesor/a" || c.Example != "I am a teacher." || c.LastReviewed != nil {
		t.Errorf("card = %+v", c)
	}
}
